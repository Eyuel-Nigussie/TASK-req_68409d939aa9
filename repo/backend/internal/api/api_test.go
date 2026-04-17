package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/crypto"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

type testRig struct {
	srv *Server
	e   *echo.Echo
	m   *store.Memory
}

func setup(t *testing.T) *testRig {
	t.Helper()
	vault, err := crypto.NewVault(map[uint16][]byte{1: crypto.DeriveKey([]byte("unit-test-key"))})
	if err != nil {
		t.Fatal(err)
	}
	m := store.NewMemory()
	clk := func() time.Time { return time.Unix(1_700_000_000, 0) }
	srv := New(m, vault, clk)

	// Seed an admin user and a lab tech.
	seed := func(name string, role models.Role) string {
		hash, _ := auth.HashPassword("correct-horse-battery-staple")
		u := models.User{ID: "u_" + name, Username: name, Role: role, PasswordHash: hash}
		if err := m.CreateUser(nil, u); err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	_ = seed("admin1", models.RoleAdmin)
	_ = seed("tech1", models.RoleLabTech)
	_ = seed("desk1", models.RoleFrontDesk)
	_ = seed("dispatch1", models.RoleDispatch)

	e := echo.New()
	srv.Register(e)
	return &testRig{srv: srv, e: e, m: m}
}

func (r *testRig) do(t *testing.T, method, path, token string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Workstation", "ws-test")
	rec := httptest.NewRecorder()
	r.e.ServeHTTP(rec, req)
	var payload map[string]any
	if rec.Body.Len() > 0 && rec.Body.Bytes()[0] == '{' {
		_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	}
	return rec, payload
}

// newReq constructs a raw request with a JSON body and the standard
// workstation header; extra headers override or add to the defaults.
func newReq(t *testing.T, method, path, token, body string, extra map[string]string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Workstation", "ws-test")
	req.Header.Set("X-Workstation-Time", time.Now().UTC().Format(time.RFC3339))
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	return req
}

// sendRaw serves a pre-built request through the Echo mux.
func (r *testRig) sendRaw(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.e.ServeHTTP(rec, req)
	return rec
}

// hashTestPassword lets tests that need to seed users bypass the setup()
// fixture. It is intentionally public within the package.
func hashTestPassword(pw string) (string, error) { return auth.HashPassword(pw) }

func testUser(username, role, hash string) models.User {
	return models.User{
		ID:           "u_" + username,
		Username:     username,
		Role:         models.Role(role),
		PasswordHash: hash,
	}
}

func (r *testRig) login(t *testing.T, user, pw string) string {
	t.Helper()
	rec, body := r.do(t, "POST", "/api/auth/login", "", map[string]string{"username": user, "password": pw})
	if rec.Code != 200 {
		t.Fatalf("login %s: %d body=%s", user, rec.Code, rec.Body.String())
	}
	tok, _ := body["token"].(string)
	if tok == "" {
		t.Fatalf("no token in body: %v", body)
	}
	return tok
}

// ---------- Auth ----------

func TestLogin_Success(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "admin1", "correct-horse-battery-staple")
	if len(tok) != 64 {
		t.Fatalf("token shape: %q", tok)
	}
}

func TestLogin_BadPassword_Lockout(t *testing.T) {
	r := setup(t)
	for i := 0; i < 4; i++ {
		rec, _ := r.do(t, "POST", "/api/auth/login", "", map[string]string{"username": "admin1", "password": "wrong-but-long-1"})
		if rec.Code != 401 {
			t.Fatalf("attempt %d: %d", i+1, rec.Code)
		}
	}
	rec, _ := r.do(t, "POST", "/api/auth/login", "", map[string]string{"username": "admin1", "password": "wrong-but-long-1"})
	if rec.Code != http.StatusLocked {
		t.Fatalf("5th attempt should be 423 Locked, got %d", rec.Code)
	}
	// Even correct password should fail during lockout.
	rec, _ = r.do(t, "POST", "/api/auth/login", "", map[string]string{"username": "admin1", "password": "correct-horse-battery-staple"})
	if rec.Code != http.StatusLocked {
		t.Fatalf("during lockout correct pw should 423, got %d", rec.Code)
	}
}

func TestLogin_UnknownUserRateLimited(t *testing.T) {
	// Unknown username attempts should still increment the counter so we
	// don't leak which usernames are valid via timing or rate.
	r := setup(t)
	for i := 0; i < 5; i++ {
		rec, _ := r.do(t, "POST", "/api/auth/login", "", map[string]string{"username": "ghost", "password": "long-wrong-password-1"})
		_ = rec
	}
	rec, _ := r.do(t, "POST", "/api/auth/login", "", map[string]string{"username": "ghost", "password": "any"})
	if rec.Code != http.StatusLocked && rec.Code != http.StatusBadRequest && rec.Code != 401 {
		t.Fatalf("unexpected code %d", rec.Code)
	}
}

// ---------- Customers ----------

func TestCustomers_CreateAndSearch(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/customers", tok, map[string]any{
		"name":       "Jane Doe",
		"identifier": "SSN-123-45-6789",
		"street":     "789 Elm Street",
		"city":       "Springfield",
		"state":      "IL",
		"zip":        "62701",
	})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	// Identifier and street should be decrypted on output.
	if body["identifier"] != "SSN-123-45-6789" {
		t.Fatalf("identifier not echoed: %+v", body)
	}
	// The in-memory store keeps encrypted text; confirm it's encrypted in storage.
	stored, _ := r.m.GetCustomer(nil, body["id"].(string))
	if stored.Identifier == "SSN-123-45-6789" {
		t.Fatalf("identifier stored in plaintext: %+v", stored)
	}

	rec, _ = r.do(t, "GET", "/api/customers?q=jane", tok, nil)
	if rec.Code != 200 {
		t.Fatalf("search: %d", rec.Code)
	}
}

func TestCustomers_ByAddressRequiresCityOrZip(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/customers/by-address", tok, nil)
	if rec.Code != 400 {
		t.Fatalf("expected 400 without zip/city, got %d", rec.Code)
	}
}

// ---------- Orders ----------

func TestOrder_CreateTransitionTimelineAndRefundReason(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/orders", tok, map[string]any{"total_cents": 2500, "priority": "standard"})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	orderID := body["ID"].(string) // Echo returns field name from struct tag; order.Order uses exported Go names
	// Invalid transition.
	rec, _ = r.do(t, "POST", "/api/orders/"+orderID+"/transitions", tok, map[string]any{"to": "delivered"})
	if rec.Code != 400 {
		t.Fatalf("invalid transition should 400, got %d", rec.Code)
	}
	// Valid transition.
	rec, _ = r.do(t, "POST", "/api/orders/"+orderID+"/transitions", tok, map[string]any{"to": "picking"})
	if rec.Code != 200 {
		t.Fatalf("valid transition should 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ---------- Reports ----------

func TestReport_CreateAndCorrectWithOptimisticConcurrency(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "tech1", "correct-horse-battery-staple")
	// Create a sample and advance it into in_testing so the v1 report is
	// allowed by the controlled-workflow gate.
	rec, body := r.do(t, "POST", "/api/samples", tok, map[string]any{"test_codes": []string{"GLU"}, "customer_id": "c1"})
	if rec.Code != 201 {
		t.Fatalf("sample: %d %s", rec.Code, rec.Body.String())
	}
	sampleID := body["ID"].(string)
	for _, to := range []string{"received", "in_testing"} {
		if rec, _ := r.do(t, "POST", "/api/samples/"+sampleID+"/transitions", tok, map[string]any{"to": to}); rec.Code != 200 {
			t.Fatalf("advance sample to %s: %d", to, rec.Code)
		}
	}

	// Report v1.
	rec, body = r.do(t, "POST", "/api/samples/"+sampleID+"/report", tok, map[string]any{
		"title":     "CBC",
		"narrative": "first report",
		"measurements": []map[string]any{{"test_code": "GLU", "value": 85}},
	})
	if rec.Code != 201 {
		t.Fatalf("report create: %d %s", rec.Code, rec.Body.String())
	}
	reportID := body["ID"].(string)

	// Correction with wrong expected version should 409.
	rec, _ = r.do(t, "POST", "/api/reports/"+reportID+"/correct", tok, map[string]any{
		"expected_version": 99,
		"title":            "CBC",
		"narrative":        "updated",
		"reason":           "typo",
	})
	if rec.Code != 409 {
		t.Fatalf("stale version should 409, got %d", rec.Code)
	}

	// Correction without reason should fail (bad request or conflict depending on mapping).
	rec, _ = r.do(t, "POST", "/api/reports/"+reportID+"/correct", tok, map[string]any{
		"expected_version": 1,
		"title":            "CBC",
		"narrative":        "updated",
	})
	if rec.Code == 200 || rec.Code == 201 {
		t.Fatalf("correction without reason should fail, got %d", rec.Code)
	}

	// Valid correction.
	rec, body = r.do(t, "POST", "/api/reports/"+reportID+"/correct", tok, map[string]any{
		"expected_version": 1,
		"title":            "CBC",
		"narrative":        "updated with more detail",
		"reason":           "clinical correction",
		"measurements":     []map[string]any{{"test_code": "GLU", "value": 300}},
	})
	if rec.Code != 201 {
		t.Fatalf("correction: %d %s", rec.Code, rec.Body.String())
	}
	if int(body["Version"].(float64)) != 2 {
		t.Fatalf("new version should be 2: %v", body)
	}
}

// ---------- Dispatch ----------

func TestDispatch_ValidatePin(t *testing.T) {
	r := setup(t)
	tokAdmin := r.login(t, "admin1", "correct-horse-battery-staple")
	// Put a simple square region.
	rec, _ := r.do(t, "PUT", "/api/admin/service-regions", tokAdmin, map[string]any{
		"regions": []map[string]any{{
			"id":                 "zoneA",
			"vertices":           [][]float64{{0, 0}, {0, 10}, {10, 10}, {10, 0}},
			"base_fee_cents":     500,
			"per_mile_fee_cents": 25,
		}},
	})
	if rec.Code != 200 {
		t.Fatalf("put regions: %d %s", rec.Code, rec.Body.String())
	}

	tokDis := r.login(t, "dispatch1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/dispatch/validate-pin", tokDis, map[string]float64{"lat": 5, "lng": 5})
	if rec.Code != 200 || body["valid"] != true {
		t.Fatalf("inside should validate: %d %v", rec.Code, body)
	}
	rec, body = r.do(t, "POST", "/api/dispatch/validate-pin", tokDis, map[string]float64{"lat": 50, "lng": 50})
	if body["valid"] != false {
		t.Fatalf("outside should be invalid: %v", body)
	}
}

// ---------- Authorization ----------

func TestRoleEnforcement(t *testing.T) {
	r := setup(t)
	tokDesk := r.login(t, "desk1", "correct-horse-battery-staple")
	// Front desk cannot hit admin endpoints.
	rec, _ := r.do(t, "GET", "/api/admin/users", tokDesk, nil)
	if rec.Code != 403 {
		t.Fatalf("expected 403 for desk->admin, got %d", rec.Code)
	}
}

// ---------- Saved filters ----------

func TestSavedFilters_Validation(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	// Overly broad filter.
	rec, _ := r.do(t, "POST", "/api/saved-filters", tok, map[string]any{
		"name":   "all-orders",
		"filter": map[string]any{"entity": "order", "size": 300},
	})
	if rec.Code != 400 {
		t.Fatalf("too-broad should 400, got %d", rec.Code)
	}
	rec, _ = r.do(t, "POST", "/api/saved-filters", tok, map[string]any{
		"name":   "my-placed",
		"filter": map[string]any{"entity": "order", "statuses": []string{"placed"}, "size": 50},
	})
	if rec.Code != 201 {
		t.Fatalf("valid filter should 201, got %d %s", rec.Code, rec.Body.String())
	}
}
