package api

// Tests that feed malformed JSON bodies to every handler that calls
// c.Bind so the decode-error branch is covered.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postBad(t *testing.T, r *testRig, path, token string) int {
	t.Helper()
	req := httptest.NewRequest("POST", path, bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Workstation", "ws-test")
	rec := httptest.NewRecorder()
	r.e.ServeHTTP(rec, req)
	return rec.Code
}

func putBad(t *testing.T, r *testRig, path, token string) int {
	t.Helper()
	req := httptest.NewRequest("PUT", path, bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Workstation", "ws-test")
	rec := httptest.NewRecorder()
	r.e.ServeHTTP(rec, req)
	return rec.Code
}

func patchBad(t *testing.T, r *testRig, path, token string) int {
	t.Helper()
	req := httptest.NewRequest("PATCH", path, bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Workstation", "ws-test")
	rec := httptest.NewRecorder()
	r.e.ServeHTTP(rec, req)
	return rec.Code
}

// Each row sends a malformed body to a specific handler and expects 400.
func TestBindErrors_Across_Handlers(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	dis := r.login(t, "dispatch1", "correct-horse-battery-staple")

	// We need a real order and sample/report for the path-parameterized endpoints.
	_, body := r.do(t, "POST", "/api/customers", desk, map[string]any{"name": "Bind"})
	customerID := body["id"].(string)
	_, body = r.do(t, "POST", "/api/orders", desk, map[string]any{"total_cents": 100})
	orderID := body["ID"].(string)
	_, body = r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	sampleID := body["ID"].(string)
	// Advance sample to in_testing and issue a report so /reports/:id/* exists.
	r.do(t, "POST", "/api/samples/"+sampleID+"/transitions", tech, map[string]any{"to": "received"})
	r.do(t, "POST", "/api/samples/"+sampleID+"/transitions", tech, map[string]any{"to": "in_testing"})
	_, body = r.do(t, "POST", "/api/samples/"+sampleID+"/report", tech, map[string]any{
		"title": "T", "measurements": []map[string]any{{"test_code": "GLU", "value": 80}},
	})
	reportID := body["ID"].(string)

	// Find the tech user ID for admin /users/:id/permissions.
	users, _ := r.m.ListUsers(nil)
	var techID string
	for _, u := range users {
		if u.Username == "tech1" {
			techID = u.ID
		}
	}

	cases := []struct {
		method, path, token string
	}{
		{"POST", "/api/auth/login", ""},
		{"POST", "/api/customers", desk},
		{"PATCH", "/api/customers/" + customerID, desk},
		{"POST", "/api/orders", desk},
		{"POST", "/api/orders/query", desk},
		{"POST", "/api/orders/" + orderID + "/transitions", desk},
		{"POST", "/api/orders/" + orderID + "/out-of-stock/plan", desk},
		{"POST", "/api/orders/" + orderID + "/inventory", desk},
		{"POST", "/api/samples", tech},
		{"POST", "/api/samples/" + sampleID + "/transitions", tech},
		{"POST", "/api/samples/" + sampleID + "/report", tech},
		{"POST", "/api/reports/" + reportID + "/correct", tech},
		{"POST", "/api/reports/" + reportID + "/archive", tech},
		{"POST", "/api/dispatch/validate-pin", dis},
		{"POST", "/api/dispatch/fee-quote", dis},
		{"POST", "/api/address-book", desk},
		{"POST", "/api/saved-filters", desk},
		{"POST", "/api/admin/users", admin},
		{"PATCH", "/api/admin/users/" + techID, admin},
		{"PUT", "/api/admin/service-regions", admin},
		{"PUT", "/api/admin/reference-ranges", admin},
		{"PUT", "/api/admin/route-table", admin},
		{"PUT", "/api/admin/role-permissions/analyst", admin},
		{"PUT", "/api/admin/users/" + techID + "/permissions", admin},
	}
	for _, c := range cases {
		var code int
		switch c.method {
		case "POST":
			code = postBad(t, r, c.path, c.token)
		case "PUT":
			code = putBad(t, r, c.path, c.token)
		case "PATCH":
			code = patchBad(t, r, c.path, c.token)
		}
		// Every one of these is bind-before-anything-else, so a 400 is
		// the expected bind-error response.
		if code != http.StatusBadRequest {
			t.Errorf("%s %s: expected 400, got %d", c.method, c.path, code)
		}
	}
}

// TestAddressBook_DeleteDecryptsForAuditSnapshot covers the "before"
// lookup in DeleteAddressBookEntry so every branch of the handler runs.
func TestAddressBook_DeleteHappyPath(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/address-book", tok, map[string]any{
		"label": "x", "street": "1 Main", "city": "C", "zip": "11111",
	})
	id := body["id"].(string)
	_ = rec
	rec, _ = r.do(t, "DELETE", "/api/address-book/"+id, tok, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
}

// TestAddressBook_List_HappyPath ensures the vault-decrypt branch
// executes, filling in the street field.
func TestAddressBook_List_HappyPathIncludesStreet(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	_, _ = r.do(t, "POST", "/api/address-book", tok, map[string]any{
		"label": "home", "street": "42 Elm", "city": "C", "zip": "11111",
	})
	rec, _ := r.do(t, "GET", "/api/address-book", tok, nil)
	if rec.Code != 200 {
		t.Fatalf("list: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "42 Elm") {
		t.Fatalf("street not decrypted: %s", rec.Body.String())
	}
}
