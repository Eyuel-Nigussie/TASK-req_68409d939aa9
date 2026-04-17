package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// --- test_items endpoint ---------------------------------------------

func TestTestItems_PersistedOnSampleCreate(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/samples", tech, map[string]any{
		"test_items": []map[string]any{
			{"test_code": "GLU", "instructions": "fasting"},
			{"test_code": "LIP"},
		},
	})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	sampleID := body["ID"].(string)
	// Returned body should carry both the sample and its test_items.
	if body["TestItems"] == nil {
		t.Fatalf("expected TestItems in response: %v", body)
	}
	// Dedicated endpoint returns the same items.
	rec, _ = r.do(t, "GET", "/api/samples/"+sampleID+"/test-items", tech, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "fasting") {
		t.Fatalf("list test items: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTestItems_FallbackToTestCodesWhenItemsAbsent(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/samples", tech, map[string]any{
		"test_codes": []string{"GLU", "LIP"},
	})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	// Under-the-hood, synthesized test_items should exist for each code.
	items, _ := r.m.ListTestItems(context.Background(), body["ID"].(string))
	if len(items) != 2 {
		t.Fatalf("expected 2 synthesized items, got %d", len(items))
	}
}

func TestTestItems_MissingBothFieldsIsBadRequest(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/samples", tech, map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// --- map-config endpoints --------------------------------------------

func TestMapConfig_AdminPutThenDispatchGet(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	dispatch := r.login(t, "dispatch1", "correct-horse-battery-staple")

	// Admin sets the map image URL.
	rec, _ := r.do(t, "PUT", "/api/admin/map-config", admin, map[string]any{
		"map_image_url": "/static/map.png",
	})
	if rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	// Dispatch role reads it to hydrate the OfflineMap component.
	rec, body := r.do(t, "GET", "/api/dispatch/map-config", dispatch, nil)
	if rec.Code != 200 || body["map_image_url"] != "/static/map.png" {
		t.Fatalf("get: %d %v", rec.Code, body)
	}
}

func TestMapConfig_PutRejectsNonWhitelistedScheme(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "PUT", "/api/admin/map-config", admin, map[string]any{
		"map_image_url": "javascript:alert(1)",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad scheme, got %d", rec.Code)
	}
}

func TestMapConfig_PutAcceptsDataURI(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "PUT", "/api/admin/map-config", admin, map[string]any{
		"map_image_url": "data:image/png;base64,AAA",
	})
	if rec.Code != 200 {
		t.Fatalf("expected 200 for data URI, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestMapConfig_PutRejectsMalformedBody(t *testing.T) {
	r := setup(t)
	tok := r.login(t, "admin1", "correct-horse-battery-staple")
	code := putBad(t, r, "/api/admin/map-config", tok)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}
}

func TestMapConfig_NonAdminIsForbidden(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "PUT", "/api/admin/map-config", desk, map[string]any{"map_image_url": "/x.png"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

// --- CSV export -------------------------------------------------------

func TestExportOrdersCSV_BoundsAndShape(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	admin := r.login(t, "admin1", "correct-horse-battery-staple")

	// Seed a single order so the CSV has at least one data row.
	rec, _ := r.do(t, "POST", "/api/orders", desk, map[string]any{
		"total_cents":     123,
		"priority":        "standard",
		"delivery_city":   "Metro",
		"delivery_zip":    "10001",
	})
	if rec.Code != 201 {
		t.Fatalf("seed order: %d %s", rec.Code, rec.Body.String())
	}

	// Admin has orders.export; the bounded-filter guard accepts a
	// non-broad filter.
	rec, _ = r.do(t, "POST", "/api/exports/orders.csv", admin, map[string]any{
		"statuses": []string{"placed"},
		"size":     50,
	})
	if rec.Code != 200 {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("expected text/csv, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id,status,priority") {
		t.Fatalf("missing header row: %s", body)
	}
	if !strings.Contains(body, "placed") {
		t.Fatalf("missing data row: %s", body)
	}
}

func TestExportOrdersCSV_RejectsBroadFilter(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	// size > 100 with no narrowing clause — rejected by filter.Validate.
	rec, _ := r.do(t, "POST", "/api/exports/orders.csv", admin, map[string]any{"size": 300})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for broad export, got %d", rec.Code)
	}
}

func TestExportOrdersCSV_RequiresPermission(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	// front_desk role does NOT have orders.export.
	rec, _ := r.do(t, "POST", "/api/exports/orders.csv", desk, map[string]any{
		"statuses": []string{"placed"},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

// --- CORS tightening --------------------------------------------------

func TestCORS_ParsesAllowedOriginsEnv(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "http://a.example, http://b.example , ")
	got := parseAllowedOrigins()
	if len(got) != 2 || got[0] != "http://a.example" || got[1] != "http://b.example" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestCORS_DefaultsToLocalhostWhenUnset(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "")
	got := parseAllowedOrigins()
	if len(got) == 0 || got[0] != "http://localhost:3000" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
}

func TestCORS_AllCommaOnlyYieldsSafeDefault(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", ", , ,")
	got := parseAllowedOrigins()
	if len(got) != 1 || got[0] != "http://localhost:3000" {
		t.Fatalf("empty-list input should fall back: %+v", got)
	}
}
