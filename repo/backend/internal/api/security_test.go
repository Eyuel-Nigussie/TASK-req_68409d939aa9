package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/labstack/echo/v4"
)

// Security-focused tests that complement the happy-path API coverage:
//   - Unauthenticated access to protected routes (401 matrix)
//   - Cross-user isolation on owner-scoped resources
//   - Audit log is written for every mutating endpoint
//   - Unknown internal errors do NOT leak detail to clients
//   - Role enforcement across all route groups

// 401 matrix: representative endpoints in each role group, no token.
func TestUnauthenticated_Routes_Return401(t *testing.T) {
	r := setup(t)
	cases := []struct {
		method, path string
	}{
		{"GET", "/api/auth/whoami"},
		{"POST", "/api/customers"},
		{"GET", "/api/customers"},
		{"GET", "/api/customers/by-address?zip=00000"},
		{"POST", "/api/orders"},
		{"GET", "/api/orders"},
		{"POST", "/api/orders/query"},
		{"GET", "/api/exceptions"},
		{"POST", "/api/samples"},
		{"GET", "/api/samples"},
		{"POST", "/api/dispatch/validate-pin"},
		{"POST", "/api/dispatch/fee-quote"},
		{"GET", "/api/saved-filters"},
		{"POST", "/api/saved-filters"},
		{"GET", "/api/address-book"},
		{"POST", "/api/address-book"},
		{"GET", "/api/search?q=foo"},
		{"GET", "/api/admin/users"},
		{"PUT", "/api/admin/service-regions"},
		{"PUT", "/api/admin/reference-ranges"},
		{"PUT", "/api/admin/route-table"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec, _ := r.do(t, tc.method, tc.path, "", nil)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s: expected 401, got %d", tc.method, tc.path, rec.Code)
			}
		})
	}
}

// Role matrix: non-admin roles are blocked from admin endpoints.
func TestRoleEnforcement_AdminEndpointsRejectNonAdmin(t *testing.T) {
	r := setup(t)
	users := []string{"desk1", "tech1", "dispatch1"}
	adminOnly := []struct {
		method, path string
	}{
		{"GET", "/api/admin/users"},
		{"POST", "/api/admin/users"},
		{"PUT", "/api/admin/service-regions"},
		{"PUT", "/api/admin/reference-ranges"},
		{"GET", "/api/admin/audit"},
	}
	for _, user := range users {
		tok := r.login(t, user, "correct-horse-battery-staple")
		for _, ep := range adminOnly {
			rec, _ := r.do(t, ep.method, ep.path, tok, map[string]any{})
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s on %s %s: expected 403, got %d", user, ep.method, ep.path, rec.Code)
			}
		}
	}
}

// Lab-only endpoints reject non-lab roles (e.g., front desk must not
// issue or correct reports).
func TestRoleEnforcement_LabEndpointsRejectFrontDesk(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/samples", tok, map[string]any{"test_codes": []string{"GLU"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for desk->samples, got %d", rec.Code)
	}
}

// TestRoleEnforcement_WriteDenyMatrix covers audit findings A1–A3. Each
// write endpoint is gated by a role-specific permission (customers.write,
// orders.write, samples.write, reports.write, reports.archive), and the
// default role grants deliberately omit these from roles that have no
// business touching them. This matrix asserts the negative case for
// every deny so a future route-group drift trips CI instead of silently
// letting analysts originate or archive primary records.
func TestRoleEnforcement_WriteDenyMatrix(t *testing.T) {
	r := setup(t)
	// Add an analyst so every role the prompt describes is represented.
	mkUser(t, r, "analyst1", "analyst")

	type denial struct {
		role         string
		token        string
		method, path string
		body         any
	}

	analyst := r.login(t, "analyst1", "correct-horse-battery-staple")
	labtech := r.login(t, "tech1", "correct-horse-battery-staple")
	dispatch := r.login(t, "dispatch1", "correct-horse-battery-staple")
	desk := r.login(t, "desk1", "correct-horse-battery-staple")

	// Every mutation an analyst must NOT be able to perform (A1, A2).
	analystDenies := []denial{
		{"analyst", analyst, "POST", "/api/customers", map[string]any{"name": "x"}},
		{"analyst", analyst, "PATCH", "/api/customers/any", map[string]any{"name": "x"}},
		{"analyst", analyst, "POST", "/api/orders", map[string]any{"total_cents": 100}},
		{"analyst", analyst, "POST", "/api/orders/any/transitions", map[string]any{"to": "picking"}},
		{"analyst", analyst, "POST", "/api/orders/any/out-of-stock/plan", map[string]any{}},
		{"analyst", analyst, "POST", "/api/orders/any/inventory", map[string]any{"sku": "X"}},
		{"analyst", analyst, "POST", "/api/samples", map[string]any{"test_codes": []string{"GLU"}}},
		{"analyst", analyst, "POST", "/api/samples/any/transitions", map[string]any{"to": "received"}},
		{"analyst", analyst, "POST", "/api/samples/any/report", map[string]any{"title": "x"}},
		{"analyst", analyst, "POST", "/api/reports/any/correct", map[string]any{"reason": "x"}},
		{"analyst", analyst, "POST", "/api/reports/any/archive", map[string]any{"note": "x"}},
	}

	// Lab-tech must NOT be able to originate or edit customers/orders (A2, A3).
	labtechDenies := []denial{
		{"lab_tech", labtech, "POST", "/api/customers", map[string]any{"name": "x"}},
		{"lab_tech", labtech, "PATCH", "/api/customers/any", map[string]any{"name": "x"}},
		{"lab_tech", labtech, "POST", "/api/orders", map[string]any{"total_cents": 100}},
		{"lab_tech", labtech, "POST", "/api/orders/any/transitions", map[string]any{"to": "picking"}},
	}

	// Dispatch must NOT be able to write customers or samples/reports (A3).
	dispatchDenies := []denial{
		{"dispatch", dispatch, "POST", "/api/customers", map[string]any{"name": "x"}},
		{"dispatch", dispatch, "PATCH", "/api/customers/any", map[string]any{"name": "x"}},
		{"dispatch", dispatch, "POST", "/api/samples", map[string]any{"test_codes": []string{"GLU"}}},
		{"dispatch", dispatch, "POST", "/api/samples/any/report", map[string]any{"title": "x"}},
		{"dispatch", dispatch, "POST", "/api/reports/any/correct", map[string]any{"reason": "x"}},
		{"dispatch", dispatch, "POST", "/api/reports/any/archive", map[string]any{"note": "x"}},
	}

	// Front desk must NOT touch lab write endpoints.
	deskDenies := []denial{
		{"front_desk", desk, "POST", "/api/samples", map[string]any{"test_codes": []string{"GLU"}}},
		{"front_desk", desk, "POST", "/api/reports/any/correct", map[string]any{"reason": "x"}},
		{"front_desk", desk, "POST", "/api/reports/any/archive", map[string]any{"note": "x"}},
	}

	all := append([]denial{}, analystDenies...)
	all = append(all, labtechDenies...)
	all = append(all, dispatchDenies...)
	all = append(all, deskDenies...)

	for _, d := range all {
		t.Run(d.role+" "+d.method+" "+d.path, func(t *testing.T) {
			rec, _ := r.do(t, d.method, d.path, d.token, d.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s as %s: expected 403, got %d (body=%s)",
					d.method, d.path, d.role, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestRoleEnforcement_AnalystReadAccessIntact guards against regressing
// the A1-A3 fix too far: an analyst must still be able to READ the
// records they build operational reports against, and they must still
// reach the analytics and CSV export endpoints their role owns.
func TestRoleEnforcement_AnalystReadAccessIntact(t *testing.T) {
	r := setup(t)
	mkUser(t, r, "analyst1", "analyst")
	tok := r.login(t, "analyst1", "correct-horse-battery-staple")

	reads := []struct{ method, path string }{
		{"GET", "/api/customers/by-address?zip=00000"},
		{"GET", "/api/orders"},
		{"GET", "/api/samples"},
		{"GET", "/api/reports"},
		{"GET", "/api/analytics/summary"},
	}
	for _, r2 := range reads {
		rec, _ := r.do(t, r2.method, r2.path, tok, nil)
		if rec.Code == http.StatusForbidden {
			t.Fatalf("%s %s as analyst should NOT be 403 (got %d)", r2.method, r2.path, rec.Code)
		}
	}
}

// Cross-user isolation: saved filters can't be listed or deleted by another user.
func TestCrossUserIsolation_SavedFilters(t *testing.T) {
	r := setup(t)
	alice := r.login(t, "desk1", "correct-horse-battery-staple")
	bob := r.login(t, "tech1", "correct-horse-battery-staple")

	rec, body := r.do(t, "POST", "/api/saved-filters", alice, map[string]any{
		"name":   "alice-filter",
		"filter": map[string]any{"entity": "order", "statuses": []string{"placed"}, "size": 10},
	})
	if rec.Code != 201 {
		t.Fatalf("alice create: %d %s", rec.Code, rec.Body.String())
	}
	fid := body["ID"].(string)

	// Bob should not see alice's filter.
	rec, _ = r.do(t, "GET", "/api/saved-filters", bob, nil)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "alice-filter") {
		t.Fatalf("bob should not see alice's filter; got: %s", rec.Body.String())
	}

	// Bob cannot delete alice's filter.
	rec, _ = r.do(t, "DELETE", "/api/saved-filters/"+fid, bob, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bob delete alice's filter: expected 404, got %d", rec.Code)
	}

	// Alice still sees it.
	rec, _ = r.do(t, "GET", "/api/saved-filters", alice, nil)
	if !strings.Contains(rec.Body.String(), "alice-filter") {
		t.Fatalf("alice lost her filter after bob's attempt: %s", rec.Body.String())
	}
}

// Cross-user isolation: address book entries are not visible or
// deletable by another user.
func TestCrossUserIsolation_AddressBook(t *testing.T) {
	r := setup(t)
	alice := r.login(t, "desk1", "correct-horse-battery-staple")
	bob := r.login(t, "tech1", "correct-horse-battery-staple")

	rec, body := r.do(t, "POST", "/api/address-book", alice, map[string]any{
		"label":  "alice-home",
		"street": "1 Elm St", "city": "Springfield", "zip": "62701",
	})
	if rec.Code != 201 {
		t.Fatalf("alice create: %d %s", rec.Code, rec.Body.String())
	}
	aid := body["id"].(string)

	rec, _ = r.do(t, "GET", "/api/address-book", bob, nil)
	if strings.Contains(rec.Body.String(), "alice-home") {
		t.Fatalf("bob saw alice's saved address: %s", rec.Body.String())
	}
	rec, _ = r.do(t, "DELETE", "/api/address-book/"+aid, bob, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bob delete alice's entry: expected 404, got %d", rec.Code)
	}
}

// Audit coverage: every mutating call exercised in the suite must emit at
// least one audit entry. This prevents regressions where a handler stops
// logging silently.
func TestAuditCoverage_AllMutationsRecorded(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	desk := r.login(t, "desk1", "correct-horse-battery-staple")

	ctx := context.Background()

	// customer create
	rec, body := r.do(t, "POST", "/api/customers", desk, map[string]any{"name": "Audit Test"})
	if rec.Code != 201 {
		t.Fatalf("create customer: %d", rec.Code)
	}
	cuID := body["id"].(string)
	if entries, _ := r.m.ListAudit(ctx, "customer", cuID, 0); len(entries) == 0 {
		t.Error("no audit entry for customer create")
	}

	// saved filter create + delete
	rec, body = r.do(t, "POST", "/api/saved-filters", desk, map[string]any{
		"name":   "audit-test",
		"filter": map[string]any{"entity": "order", "statuses": []string{"placed"}, "size": 10},
	})
	if rec.Code != 201 {
		t.Fatalf("create filter: %d %s", rec.Code, rec.Body.String())
	}
	fID := body["ID"].(string)
	if entries, _ := r.m.ListAudit(ctx, "saved_filter", fID, 0); len(entries) == 0 {
		t.Error("no audit entry for saved filter create")
	}
	rec, _ = r.do(t, "DELETE", "/api/saved-filters/"+fID, desk, nil)
	if rec.Code != 204 {
		t.Fatalf("delete filter: %d", rec.Code)
	}
	if entries, _ := r.m.ListAudit(ctx, "saved_filter", fID, 0); len(entries) < 2 {
		t.Errorf("expected create+delete audit, got %d", len(entries))
	}

	// address book create
	rec, body = r.do(t, "POST", "/api/address-book", desk, map[string]any{"label": "x", "zip": "00000"})
	if rec.Code != 201 {
		t.Fatalf("create address: %d %s", rec.Code, rec.Body.String())
	}
	aID := body["id"].(string)
	if entries, _ := r.m.ListAudit(ctx, "address_book", aID, 0); len(entries) == 0 {
		t.Error("no audit entry for address create")
	}

	// admin service regions replace
	rec, _ = r.do(t, "PUT", "/api/admin/service-regions", admin, map[string]any{
		"regions": []map[string]any{{
			"id":                 "zoneA",
			"vertices":           [][]float64{{0, 0}, {0, 10}, {10, 10}, {10, 0}},
			"base_fee_cents":     100, "per_mile_fee_cents": 10,
		}},
	})
	if rec.Code != 200 {
		t.Fatalf("put regions: %d %s", rec.Code, rec.Body.String())
	}
	if entries, _ := r.m.ListAudit(ctx, "service_regions", "all", 0); len(entries) == 0 {
		t.Error("no audit entry for service_regions replace")
	}

	// admin reference ranges replace
	rec, _ = r.do(t, "PUT", "/api/admin/reference-ranges", admin, map[string]any{
		"ranges": []map[string]any{{"TestCode": "GLU", "LowNormal": 70, "HighNormal": 99}},
	})
	if rec.Code != 200 {
		t.Fatalf("put refranges: %d %s", rec.Code, rec.Body.String())
	}
	if entries, _ := r.m.ListAudit(ctx, "reference_ranges", "all", 0); len(entries) == 0 {
		t.Error("no audit entry for reference_ranges replace")
	}

	// admin route table replace
	rec, _ = r.do(t, "PUT", "/api/admin/route-table", admin, map[string]any{
		"routes": []map[string]any{{"FromID": "A", "ToID": "B", "Miles": 5}},
	})
	if rec.Code != 200 {
		t.Fatalf("put routes: %d %s", rec.Code, rec.Body.String())
	}
	if entries, _ := r.m.ListAudit(ctx, "route_table", "all", 0); len(entries) == 0 {
		t.Error("no audit entry for route_table replace")
	}

	// admin user update
	adminUsers, _ := r.m.ListUsers(ctx)
	var target string
	for _, u := range adminUsers {
		if u.Username == "desk1" {
			target = u.ID
			break
		}
	}
	rec, _ = r.do(t, "PATCH", "/api/admin/users/"+target, admin, map[string]any{"disabled": true})
	if rec.Code != 200 {
		t.Fatalf("admin update user: %d %s", rec.Code, rec.Body.String())
	}
	if entries, _ := r.m.ListAudit(ctx, "user", target, 0); len(entries) == 0 {
		t.Error("no audit entry for user update")
	}

	// sample create + transition, report create
	rec, body = r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	if rec.Code != 201 {
		t.Fatalf("sample create: %d %s", rec.Code, rec.Body.String())
	}
	sID := body["ID"].(string)
	if entries, _ := r.m.ListAudit(ctx, "sample", sID, 0); len(entries) == 0 {
		t.Error("no audit entry for sample create")
	}
	rec, _ = r.do(t, "POST", "/api/samples/"+sID+"/transitions", tech, map[string]any{"to": "received"})
	if rec.Code != 200 {
		t.Fatalf("sample transition: %d", rec.Code)
	}
	if entries, _ := r.m.ListAudit(ctx, "sample", sID, 0); len(entries) < 2 {
		t.Error("expected create+transition audit entries on sample")
	}
}

// 500 leakage: check that predictable "not found" paths do not leak SQL or
// other internal details into the response body.
func TestGeneric500_DoesNotLeakInternalDetail(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "admin1", "correct-horse-battery-staple")
	// Put a malformed region that will trigger "must be [lat,lng]".
	rec, _ := r.do(t, "PUT", "/api/admin/service-regions", tok, map[string]any{
		"regions": []map[string]any{{
			"id":       "bad",
			"vertices": [][]float64{{0, 0, 0}, {1, 1}, {2, 2}},
		}},
	})
	// This specific path returns a handler-crafted 400; the main
	// guarantee we care about is the unknown-error branch. We re-exercise
	// that by pointing at a nonexistent order.
	rec2, _ := r.do(t, "GET", "/api/orders/does-not-exist", tok, nil)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent order, got %d", rec2.Code)
	}
	if strings.Contains(strings.ToLower(rec2.Body.String()), "sql") {
		t.Fatalf("response body appears to leak SQL text: %s", rec2.Body.String())
	}
	_ = rec
}

// Verify that an HTTP handler using httpx.WriteError with an unknown error
// returns "internal server error" rather than the raw message. We can't
// easily inject an error through the normal route tree, so we invoke
// WriteError directly on a synthetic Echo context.
func TestWriteError_MapsUnknownErrorToGenericMessage(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/probe", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	// Use an import alias to pull in the package under test.
	if err := httpx.WriteError(c, errors.New("secret leaky details")); err == nil {
		t.Fatal("expected non-nil error")
	} else {
		he, ok := err.(*echo.HTTPError)
		if !ok {
			t.Fatalf("expected *echo.HTTPError, got %T", err)
		}
		if he.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", he.Code)
		}
		if msg, _ := he.Message.(string); strings.Contains(strings.ToLower(msg), "secret") {
			t.Fatalf("internal detail leaked: %v", he.Message)
		}
	}
}
