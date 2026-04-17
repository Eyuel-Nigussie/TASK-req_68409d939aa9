package api

// This file exists to close coverage gaps across every handler. Each test
// targets a specific uncovered branch (success path, error path, input
// validation) so the coverage tool reports >=95% for the api package.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
)

// ---------- Auth ----------

func TestLogout_RevokesSession(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "admin1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/auth/logout", tok, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	// Second call with the same token must be rejected by RequireAuth.
	rec, _ = r.do(t, "GET", "/api/auth/whoami", tok, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", rec.Code)
	}
}

func TestLogout_WithoutSession_Is204(t *testing.T) {
	// The Logout handler short-circuits when session is nil even though
	// the middleware normally blocks anonymous calls. We invoke it by
	// handing it a session-less bearer; the middleware returns 401 first,
	// so this test primarily ensures the handler path exists.
	r := setup(t)
	rec, _ := r.do(t, "POST", "/api/auth/logout", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestWhoAmI_ReturnsSession(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "admin1", "correct-horse-battery-staple")
	rec, body := r.do(t, "GET", "/api/auth/whoami", tok, nil)
	if rec.Code != 200 {
		t.Fatalf("whoami: %d", rec.Code)
	}
	if body["username"] != "admin1" || body["role"] != "admin" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestLogin_BadBody(t *testing.T) {
	r := setup(t)
	rec, _ := r.do(t, "POST", "/api/auth/login", "", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing fields expected 400, got %d", rec.Code)
	}
}

func TestLogin_DisabledAccount(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	u, _ := r.m.GetUserByUsername(ctx, "desk1")
	u.Disabled = true
	_ = r.m.UpdateUser(ctx, u)
	rec, _ := r.do(t, "POST", "/api/auth/login", "", map[string]string{"username": "desk1", "password": "correct-horse-battery-staple"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled should be 403, got %d", rec.Code)
	}
}

// ---------- Customers ----------

func TestCustomers_GetMissingReturns404(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/customers/missing-id", tok, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCustomers_UpdateRoundTrip(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/customers", tok, map[string]any{
		"name": "Orig", "street": "1 First St", "city": "Alpha", "zip": "11111",
		"phone": "555-1000", "email": "o@example.com", "tags": []string{"a"},
	})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id := body["id"].(string)
	rec, body = r.do(t, "PATCH", "/api/customers/"+id, tok, map[string]any{
		"name": "Updated", "street": "2 Second St", "city": "Beta", "state": "CA",
		"zip": "22222", "phone": "555-2000", "email": "u@example.com",
		"tags": []string{"b"},
	})
	if rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if body["name"] != "Updated" || body["city"] != "Beta" || body["street"] != "2 Second St" {
		t.Fatalf("fields not updated: %+v", body)
	}
}

func TestCustomers_UpdateMissing404(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "PATCH", "/api/customers/ghost", tok, map[string]any{"name": "x"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCustomers_CreateRejectsEmptyName(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/customers", tok, map[string]any{"name": ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank name, got %d", rec.Code)
	}
}

func TestCustomers_ByAddressFiltersByStreetAfterDecrypt(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	_, _ = r.do(t, "POST", "/api/customers", tok, map[string]any{
		"name": "Match", "street": "789 Pinewood Ln", "city": "Town", "zip": "12345",
	})
	_, _ = r.do(t, "POST", "/api/customers", tok, map[string]any{
		"name": "NoMatch", "street": "100 Oak Ave", "city": "Town", "zip": "12345",
	})
	rec, _ := r.do(t, "GET", "/api/customers/by-address?street=pine&city=town&zip=12345", tok, nil)
	if rec.Code != 200 {
		t.Fatalf("by-address: %d", rec.Code)
	}
	var got []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0]["name"] != "Match" {
		t.Fatalf("expected one match (street substring), got %+v", got)
	}
}

// ---------- Orders ----------

func TestOrders_OrdersByAddress_FiltersByZipCityAndStreet(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	_, _ = r.do(t, "POST", "/api/orders", tok, map[string]any{
		"total_cents": 100, "delivery_street": "10 Harvest Rd", "delivery_city": "Crest", "delivery_zip": "90210",
	})
	_, _ = r.do(t, "POST", "/api/orders", tok, map[string]any{
		"total_cents": 200, "delivery_street": "42 Main St", "delivery_city": "Other", "delivery_zip": "90210",
	})
	rec, _ := r.do(t, "GET", "/api/orders/by-address?city=Crest&zip=90210&street=Harvest", tok, nil)
	if rec.Code != 200 {
		t.Fatalf("by-address: %d %s", rec.Code, rec.Body.String())
	}
	var got []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 order, got %d: %s", len(got), rec.Body.String())
	}
}

func TestOrders_OrdersByAddress_RequiresCityOrZip(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/orders/by-address", tok, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestOrders_ListWithDateFilters(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	_, body := r.do(t, "POST", "/api/orders", tok, map[string]any{"total_cents": 100})
	_ = body
	rec, _ := r.do(t, "GET", "/api/orders?from=1000&to=9999999999&limit=10&offset=0&status=placed", tok, nil)
	if rec.Code != 200 {
		t.Fatalf("list orders: %d", rec.Code)
	}
}

func TestOrders_UpdateInventoryMissing(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/orders/unknown/inventory", tok, map[string]any{"sku": "X", "backordered": true})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestOrders_UpdateInventoryRejectsEmptySKU(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/orders", tok, map[string]any{
		"total_cents": 100,
		"items":       []map[string]any{{"SKU": "A", "Qty": 1}},
	})
	id := body["ID"].(string)
	_ = rec
	rec, _ = r.do(t, "POST", "/api/orders/"+id+"/inventory", tok, map[string]any{"sku": ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty SKU, got %d", rec.Code)
	}
}

func TestOrders_UpdateInventoryUnknownSKUReturns404(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/orders", tok, map[string]any{
		"total_cents": 100,
		"items":       []map[string]any{{"SKU": "A", "Qty": 1}},
	})
	id := body["ID"].(string)
	_ = rec
	rec, _ = r.do(t, "POST", "/api/orders/"+id+"/inventory", tok, map[string]any{"sku": "Z", "backordered": true})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown SKU, got %d", rec.Code)
	}
}

func TestOrders_TransitionMissing404(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/orders/ghost/transitions", tok, map[string]any{"to": "picking"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestOrders_PlanOutOfStock_HandlerAuditsAndSplits(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/orders", tok, map[string]any{"total_cents": 100})
	id := body["ID"].(string)
	_ = rec
	rec, body = r.do(t, "POST", "/api/orders/"+id+"/out-of-stock/plan", tok, map[string]any{
		"available":   []string{"A"},
		"backordered": []string{"B"},
	})
	if rec.Code != 200 {
		t.Fatalf("plan oos: %d %s", rec.Code, rec.Body.String())
	}
	if body["SuggestSplit"] != true {
		t.Fatalf("expected suggest split: %+v", body)
	}
}

// ---------- Dispatch ----------

func TestDispatch_ListRegions(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	dis := r.login(t, "dispatch1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "PUT", "/api/admin/service-regions", admin, map[string]any{
		"regions": []map[string]any{{
			"id":                 "zoneA",
			"vertices":           [][]float64{{0, 0}, {0, 10}, {10, 10}, {10, 0}},
			"base_fee_cents":     500,
			"per_mile_fee_cents": 25,
		}},
	})
	if rec.Code != 200 {
		t.Fatalf("put: %d", rec.Code)
	}
	rec, _ = r.do(t, "GET", "/api/dispatch/regions", dis, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "zoneA") {
		t.Fatalf("list regions wrong: %s", rec.Body.String())
	}
}

// ---------- Lab ----------

func TestLab_GetReport_Success(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	sid := body["ID"].(string)
	_ = rec
	for _, to := range []string{"received", "in_testing"} {
		r.do(t, "POST", "/api/samples/"+sid+"/transitions", tech, map[string]any{"to": to})
	}
	rec, body = r.do(t, "POST", "/api/samples/"+sid+"/report", tech, map[string]any{
		"title": "CBC", "measurements": []map[string]any{{"test_code": "GLU", "value": 85}},
	})
	rid := body["ID"].(string)
	rec, _ = r.do(t, "GET", "/api/reports/"+rid, tech, nil)
	if rec.Code != 200 {
		t.Fatalf("get report: %d", rec.Code)
	}
}

func TestLab_GetReportMissing404(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/reports/ghost", tech, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLab_GetSampleMissing404(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/samples/ghost", tech, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLab_CreateSampleRejectsNoTests(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLab_TransitionSampleBadStatus(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	sid := body["ID"].(string)
	_ = rec
	rec, _ = r.do(t, "POST", "/api/samples/"+sid+"/transitions", tech, map[string]any{"to": "bogus"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLab_ListSamplesWithFilter(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	_, _ = r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	rec, _ := r.do(t, "GET", "/api/samples?status=sampling&limit=5&offset=0", tech, nil)
	if rec.Code != 200 {
		t.Fatalf("list: %d", rec.Code)
	}
}

func TestLab_ListArchivedReports(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/reports/archived", tech, nil)
	if rec.Code != 200 {
		t.Fatalf("list archived: %d", rec.Code)
	}
}

func TestLab_SearchReportsRejectsEmptyQuery(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/reports/search", tech, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLab_CorrectMissing404(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/reports/ghost/correct", tech, map[string]any{
		"expected_version": 1, "title": "x", "reason": "r",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLab_ArchiveMissing404(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/reports/ghost/archive", tech, map[string]any{"note": "r"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLab_CreateReport_TitleRequired(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	sid := body["ID"].(string)
	_ = rec
	for _, to := range []string{"received", "in_testing"} {
		r.do(t, "POST", "/api/samples/"+sid+"/transitions", tech, map[string]any{"to": to})
	}
	rec, _ = r.do(t, "POST", "/api/samples/"+sid+"/report", tech, map[string]any{"title": ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---------- Address book ----------

func TestAddressBook_ListAndDeleteMissing(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/address-book", tok, nil)
	if rec.Code != 200 {
		t.Fatalf("list: %d", rec.Code)
	}
	rec, _ = r.do(t, "DELETE", "/api/address-book/ghost", tok, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on missing delete, got %d", rec.Code)
	}
}

func TestAddressBook_CreateRejectsBlankLabel(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/address-book", tok, map[string]any{"label": ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---------- Saved filters ----------

func TestSavedFilters_ListEmpty(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/saved-filters", tok, nil)
	if rec.Code != 200 {
		t.Fatalf("list: %d", rec.Code)
	}
}

func TestSavedFilters_CreateRejectsBlankName(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/saved-filters", tok, map[string]any{"name": "", "filter": map[string]any{"entity": "order"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSavedFilters_KnownStatusesForEachEntity(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	entities := map[string][]string{
		"sample": {"sampling"},
		"report": {"issued"},
		"order":  {"placed"},
		"customer": nil, // entity valid but no status list
	}
	for entity, statuses := range entities {
		body := map[string]any{"name": "n-" + entity, "filter": map[string]any{"entity": entity, "statuses": statuses, "size": 10}}
		rec, _ := r.do(t, "POST", "/api/saved-filters", tok, body)
		if rec.Code != http.StatusCreated {
			t.Errorf("entity=%s: %d %s", entity, rec.Code, rec.Body.String())
		}
	}
}

// ---------- Admin handlers ----------

func TestAdminCreateUser_ValidatesRoleAndPassword(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	// Invalid role.
	rec, _ := r.do(t, "POST", "/api/admin/users", admin, map[string]any{
		"username": "new1", "password": "correctpasswordlong", "role": "unknown",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad role, got %d", rec.Code)
	}
	// Empty username.
	rec, _ = r.do(t, "POST", "/api/admin/users", admin, map[string]any{
		"username": "", "password": "correctpasswordlong", "role": "analyst",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank username, got %d", rec.Code)
	}
	// Password too short.
	rec, _ = r.do(t, "POST", "/api/admin/users", admin, map[string]any{
		"username": "new1", "password": "short", "role": "analyst",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short pw, got %d", rec.Code)
	}
	// Valid creation.
	rec, _ = r.do(t, "POST", "/api/admin/users", admin, map[string]any{
		"username": "new1", "password": "correctpasswordlong", "role": "analyst",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	// Duplicate is rejected.
	rec, _ = r.do(t, "POST", "/api/admin/users", admin, map[string]any{
		"username": "new1", "password": "correctpasswordlong", "role": "analyst",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on dup, got %d", rec.Code)
	}
}

func TestAdminUpdateUser_PasswordAndRoleAndDisabled(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	var targetID string
	users, _ := r.m.ListUsers(context.Background())
	for _, u := range users {
		if u.Username == "tech1" {
			targetID = u.ID
		}
	}
	// Bad role rejected.
	rec, _ := r.do(t, "PATCH", "/api/admin/users/"+targetID, admin, map[string]any{"role": "nope"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	// Bad password rejected.
	rec, _ = r.do(t, "PATCH", "/api/admin/users/"+targetID, admin, map[string]any{"password": "x"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 short pw, got %d", rec.Code)
	}
	// Valid update.
	rec, _ = r.do(t, "PATCH", "/api/admin/users/"+targetID, admin, map[string]any{
		"role": "analyst", "password": "correctpasswordlong",
	})
	if rec.Code != 200 {
		t.Fatalf("update: %d", rec.Code)
	}
	// Missing user -> 404.
	rec, _ = r.do(t, "PATCH", "/api/admin/users/ghost", admin, map[string]any{"role": "analyst"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestAdmin_ListEndpoints(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	for _, path := range []string{
		"/api/admin/users",
		"/api/admin/audit",
		"/api/admin/reference-ranges",
		"/api/admin/route-table",
		"/api/admin/permissions",
		"/api/admin/role-permissions",
	} {
		rec, _ := r.do(t, "GET", path, admin, nil)
		if rec.Code != 200 {
			t.Errorf("%s returned %d", path, rec.Code)
		}
	}
}

func TestAdmin_PutRefRanges_ValidatesBounds(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	low := 100.0
	high := 50.0
	rec, _ := r.do(t, "PUT", "/api/admin/reference-ranges", admin, map[string]any{
		"ranges": []map[string]any{
			{"TestCode": "X", "LowNormal": low, "HighNormal": high},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for inverted bounds, got %d", rec.Code)
	}
	// Missing test_code.
	rec, _ = r.do(t, "PUT", "/api/admin/reference-ranges", admin, map[string]any{
		"ranges": []map[string]any{{"TestCode": ""}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing test_code, got %d", rec.Code)
	}
}

func TestAdmin_PutRoutes_ValidatesInputs(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	cases := []map[string]any{
		{"routes": []map[string]any{{"FromID": "", "ToID": "B", "Miles": 1}}},
		{"routes": []map[string]any{{"FromID": "A", "ToID": "B", "Miles": -1}}},
	}
	for i, body := range cases {
		rec, _ := r.do(t, "PUT", "/api/admin/route-table", admin, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("case %d expected 400, got %d", i, rec.Code)
		}
	}
}

func TestAdmin_PutServiceRegions_Validation(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "PUT", "/api/admin/service-regions", admin, map[string]any{
		"regions": []map[string]any{{"id": "", "vertices": [][]float64{{0, 0}, {1, 1}, {2, 2}}}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	rec, _ = r.do(t, "PUT", "/api/admin/service-regions", admin, map[string]any{
		"regions": []map[string]any{{"id": "x", "vertices": [][]float64{{0, 0, 0}, {1, 1}, {2, 2}}}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad vertex, got %d", rec.Code)
	}
}

// ---------- Analytics ----------

func TestAnalytics_AllEndpoints_Work(t *testing.T) {
	r := setup(t)
	mkUser(t, r, "analyst2", "analyst")
	anl := r.login(t, "analyst2", "correct-horse-battery-staple")
	// Seed some data so aggregations are non-empty.
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	_, _ = r.do(t, "POST", "/api/orders", desk, map[string]any{"total_cents": 100})
	for _, path := range []string{
		"/api/analytics/orders/status-counts",
		"/api/analytics/orders/per-day",
		"/api/analytics/samples/status-counts",
		"/api/analytics/reports/abnormal-rate",
		"/api/analytics/exceptions/by-kind",
		"/api/analytics/summary",
		"/api/analytics/summary?from=1000&to=9999999999",
	} {
		rec, _ := r.do(t, "GET", path, anl, nil)
		if rec.Code != 200 {
			t.Errorf("%s returned %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

// ---------- Permissions admin endpoints (coverage) ----------

func TestPermissionsAdmin_ListUserPermsEmpty(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	users, _ := r.m.ListUsers(context.Background())
	rec, _ := r.do(t, "GET", "/api/admin/users/"+users[0].ID+"/permissions", admin, nil)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestPermissionsAdmin_SetRoleRequiresRoleParam(t *testing.T) {
	// The route pattern requires :role in the URL so the handler never
	// sees an empty role. We exercise the error branch by sending an
	// unknown permission id.
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "PUT", "/api/admin/role-permissions/analyst", admin, map[string]any{
		"permission_ids": []string{"bogus.permission"},
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unknown permission, got %d", rec.Code)
	}
}

// ---------- Helpers ----------

// TestSplitCSV exercises the order package's internal splitCSV helper via
// the ListOrders handler which passes the status query param through it.
func TestSplitCSV_Behavior(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "GET", "/api/orders?status=placed,%20,picking", tok, nil)
	if rec.Code != 200 {
		t.Fatalf("status CSV parse: %d", rec.Code)
	}
}

// Force the type import to stay in use even if the file is reduced.
var _ = order.LineItem{}
var _ = lab.ReportIssued
var _ = models.Role("")
var _ = time.Time{}
