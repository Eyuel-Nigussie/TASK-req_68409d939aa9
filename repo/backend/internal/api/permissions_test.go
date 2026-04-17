package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestAnalytics_RequiresPermission verifies that the analytics endpoints
// are gated by the admin-configurable "analytics.view" permission.
// By default analysts/admins have it and front desk does not.
func TestAnalytics_RequiresPermission(t *testing.T) {
	r := setup(t)
	// Seed an analyst user since the default setup doesn't include one.
	mkUser(t, r, "analyst1", "analyst")
	anl := r.login(t, "analyst1", "correct-horse-battery-staple")
	desk := r.login(t, "desk1", "correct-horse-battery-staple")

	// Analyst allowed.
	rec, _ := r.do(t, "GET", "/api/analytics/orders/status-counts", anl, nil)
	if rec.Code != 200 {
		t.Fatalf("analyst expected 200, got %d %s", rec.Code, rec.Body.String())
	}
	// Front desk denied.
	rec, _ = r.do(t, "GET", "/api/analytics/orders/status-counts", desk, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("desk expected 403, got %d", rec.Code)
	}
}

// TestPermissionReconfigure_TakesEffectImmediately proves the middleware
// reads grants from the store on every request, so an administrator
// granting a permission does NOT require a user to re-login.
func TestPermissionReconfigure_TakesEffectImmediately(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	desk := r.login(t, "desk1", "correct-horse-battery-staple")

	// Desk starts without analytics.view.
	rec, _ := r.do(t, "GET", "/api/analytics/summary", desk, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("desk expected 403 initially, got %d", rec.Code)
	}

	// Admin grants analytics.view to the front_desk role.
	rec, _ = r.do(t, "PUT", "/api/admin/role-permissions/front_desk", admin, map[string]any{
		"permission_ids": []string{
			"customers.read", "customers.write", "orders.read", "orders.write",
			"analytics.view", // the new grant
		},
	})
	if rec.Code != 200 {
		t.Fatalf("grant: %d %s", rec.Code, rec.Body.String())
	}

	// Desk can now reach analytics without any session refresh.
	rec, _ = r.do(t, "GET", "/api/analytics/summary", desk, nil)
	if rec.Code != 200 {
		t.Fatalf("after grant desk should pass: %d %s", rec.Code, rec.Body.String())
	}

	// Admin revokes analytics.view; access is denied again.
	rec, _ = r.do(t, "PUT", "/api/admin/role-permissions/front_desk", admin, map[string]any{
		"permission_ids": []string{
			"customers.read", "customers.write", "orders.read", "orders.write",
		},
	})
	if rec.Code != 200 {
		t.Fatalf("revoke: %d", rec.Code)
	}
	rec, _ = r.do(t, "GET", "/api/analytics/summary", desk, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("after revoke desk should be 403, got %d", rec.Code)
	}
}

// TestAnalyticsSummary_ShapesKeyKPIs covers the analytics summary endpoint,
// asserting that the response carries each required KPI key.
func TestAnalyticsSummary_ShapesKeyKPIs(t *testing.T) {
	r := setup(t)
	mkUser(t, r, "analyst1", "analyst")
	anl := r.login(t, "analyst1", "correct-horse-battery-staple")
	rec, body := r.do(t, "GET", "/api/analytics/summary", anl, nil)
	if rec.Code != 200 {
		t.Fatalf("summary: %d %s", rec.Code, rec.Body.String())
	}
	for _, key := range []string{"order_status", "sample_status", "orders_per_day", "abnormal_rate", "exceptions"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing KPI %q in body: %v", key, body)
		}
	}
}

// TestPermissionAdmin_CRUD exercises the admin permission CRUD endpoints
// including user-level grants audit writes.
func TestPermissionAdmin_CRUD(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	// Catalog should be non-empty (seeded).
	rec, _ := r.do(t, "GET", "/api/admin/permissions", admin, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "orders.write") {
		t.Fatalf("catalog wrong: %s", rec.Body.String())
	}
	// Grant an individual permission to a specific user.
	users, _ := r.m.ListUsers(nil)
	var target string
	for _, u := range users {
		if u.Username == "tech1" {
			target = u.ID
			break
		}
	}
	rec, _ = r.do(t, "PUT", "/api/admin/users/"+target+"/permissions", admin, map[string]any{
		"permission_ids": []string{"analytics.view"},
	})
	if rec.Code != 200 {
		t.Fatalf("user grant: %d %s", rec.Code, rec.Body.String())
	}
	// Tech can now reach analytics even though tech_lab role doesn't have it.
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ = r.do(t, "GET", "/api/analytics/summary", tech, nil)
	if rec.Code != 200 {
		t.Fatalf("tech with user grant expected 200, got %d", rec.Code)
	}
}

// mkUser adds a user to the in-memory store so tests can exercise roles
// that setup() doesn't seed by default (e.g., analyst).
func mkUser(t *testing.T, r *testRig, username, role string) {
	t.Helper()
	hash, _ := hashTestPassword("correct-horse-battery-staple")
	if err := r.m.CreateUser(nil, testUser(username, role, hash)); err != nil {
		t.Fatal(err)
	}
}
