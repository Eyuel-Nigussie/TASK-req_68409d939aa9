package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
)

// TestAuditRedaction_NeverContainsPlaintextSensitiveFields ensures that
// before/after JSON snapshots stored on the audit log do NOT contain the
// plaintext customer identifier, customer street, or order delivery
// street. The redaction helpers should scrub these to a boolean marker.
func TestAuditRedaction_NeverContainsPlaintextSensitiveFields(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")

	// Customer create: identifier + street plaintext must NOT appear in audit.
	rec, _ := r.do(t, "POST", "/api/customers", desk, map[string]any{
		"name": "Leaky", "identifier": "SSN-SECRET-12345",
		"street": "123 SensitiveLane Apt Z", "city": "Town", "zip": "99999",
	})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	// Order create with a delivery street.
	rec, _ = r.do(t, "POST", "/api/orders", desk, map[string]any{
		"total_cents": 100,
		"delivery_street": "456 DropOffSecret Blvd",
		"delivery_city":   "Town",
		"delivery_zip":    "99999",
	})
	if rec.Code != 201 {
		t.Fatalf("order: %d %s", rec.Code, rec.Body.String())
	}

	entries, _ := r.m.ListAudit(context.Background(), "", "", 0)
	if len(entries) == 0 {
		t.Fatal("no audit entries")
	}
	for _, e := range entries {
		// Combine before + after bytes and search for any plaintext
		// sensitive marker; the audit log must never contain them.
		blob := string(e.Before) + " " + string(e.After)
		for _, needle := range []string{"SSN-SECRET-12345", "SensitiveLane", "DropOffSecret"} {
			if strings.Contains(blob, needle) {
				t.Fatalf("audit entry %s leaked %q: %s", e.ID, needle, blob)
			}
		}
	}
}

// TestQueryOrders_CombinedFilters exercises the backend filter execution
// path with every dimension simultaneously: keyword + status + tag +
// priority + price range + date range + sort + pagination.
func TestQueryOrders_CombinedFilters(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")

	now := time.Unix(1_700_000_000, 0)
	// Clear clock so orders all land on a single day; we seed directly
	// through the store to control PlacedAt exactly.
	seed := func(id, status, priority string, cents int, tags []string, offset int64) {
		o := order.Order{
			ID: id, Status: order.Status(status), Priority: priority, TotalCents: cents,
			Tags: tags, PlacedAt: now.Add(time.Duration(offset) * time.Second), UpdatedAt: now,
		}
		if err := r.m.CreateOrder(context.Background(), o); err != nil {
			t.Fatal(err)
		}
	}
	seed("needle1", "placed", "rush", 3000, []string{"inbound"}, 0)
	seed("needle2", "picking", "rush", 5000, []string{"inbound", "retail"}, 100)
	seed("decoy1", "placed", "standard", 1500, []string{"wholesale"}, 200)
	seed("decoy2", "placed", "rush", 100, []string{"inbound"}, 300)

	body := map[string]any{
		"keyword":       "needle",
		"statuses":      []string{"placed", "picking"},
		"tags":          []string{"inbound"},
		"priority":      "rush",
		"min_price_usd": 10.0, // 1000 cents
		"max_price_usd": 200.0,
		"start_date":    "11/01/2023", // broad enough window
		"end_date":      "11/30/2023",
		"sort_by":       "total_cents",
		"sort_desc":     true,
		"size":          25,
	}
	rec, out := r.do(t, "POST", "/api/orders/query", desk, body)
	if rec.Code != 200 {
		t.Fatalf("query: %d %s", rec.Code, rec.Body.String())
	}
	items, _ := out["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(items), items)
	}
	// Descending by total_cents: needle2 (5000) before needle1 (3000).
	firstID := items[0].(map[string]any)["ID"]
	if firstID != "needle2" {
		t.Fatalf("expected needle2 first, got %v", firstID)
	}
}

// TestFeeRoundingBoundaries covers fractional-mile rounding for fee quotes,
// including the float-drift edge case that the constant-time offset was
// added to avoid. Runs against the fee-quote handler end-to-end.
func TestFeeRoundingBoundaries(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	dis := r.login(t, "dispatch1", "correct-horse-battery-staple")

	// Place a region covering our test point and a route table entry with
	// a fractional-mile value that floats would otherwise round DOWN.
	if rec, _ := r.do(t, "PUT", "/api/admin/service-regions", admin, map[string]any{
		"regions": []map[string]any{{
			"id":                 "sq",
			"vertices":           [][]float64{{0, 0}, {0, 10}, {10, 10}, {10, 0}},
			"base_fee_cents":     500,
			"per_mile_fee_cents": 25,
		}},
	}); rec.Code != 200 {
		t.Fatalf("regions: %d", rec.Code)
	}
	// 4.02 mi * 25 = 100.5, must round up to 101 (501 total + 500 base = 601).
	if rec, _ := r.do(t, "PUT", "/api/admin/route-table", admin, map[string]any{
		"routes": []map[string]any{{"FromID": "x", "ToID": "y", "Miles": 4.02}},
	}); rec.Code != 200 {
		t.Fatalf("routes: %d", rec.Code)
	}
	rec, body := r.do(t, "POST", "/api/dispatch/fee-quote", dis, map[string]any{
		"from_id": "x", "to_id": "y",
		"from": map[string]float64{"lat": 0, "lng": 0},
		"to":   map[string]float64{"lat": 5, "lng": 5},
	})
	if rec.Code != 200 {
		t.Fatalf("quote: %d %s", rec.Code, rec.Body.String())
	}
	if fee, _ := body["fee_cents"].(float64); fee != 601 {
		t.Fatalf("expected half-up rounding to 601, got %v", body["fee_cents"])
	}
	// 0-mile edge case yields the base fee only.
	if rec, _ := r.do(t, "PUT", "/api/admin/route-table", admin, map[string]any{
		"routes": []map[string]any{{"FromID": "a", "ToID": "a", "Miles": 0}},
	}); rec.Code != 200 {
		t.Fatalf("zero route: %d", rec.Code)
	}
	rec, body = r.do(t, "POST", "/api/dispatch/fee-quote", dis, map[string]any{
		"from_id": "a", "to_id": "a",
		"from": map[string]float64{"lat": 5, "lng": 5},
		"to":   map[string]float64{"lat": 5, "lng": 5},
	})
	if fee, _ := body["fee_cents"].(float64); fee != 500 {
		t.Fatalf("expected 500 base fee at 0 miles, got %v", body["fee_cents"])
	}
}

// TestPickingTimeout_ScaleDoesNotDuplicate exercises the exception queue
// at a larger scale to confirm that the detector is idempotent (repeated
// list calls do not create duplicate exceptions).
func TestPickingTimeout_ScaleDoesNotDuplicate(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	// Seed 50 orders all stuck in picking for 31 minutes.
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 50; i++ {
		o := order.Order{
			ID:        fmt.Sprintf("stuck-%03d", i),
			Status:    order.StatusPicking,
			PlacedAt:  base,
			UpdatedAt: base,
		}
		_ = r.m.CreateOrder(context.Background(), o)
	}
	// Fast-forward the server clock by reconstructing with a future clock.
	r.srv.Clock = func() time.Time { return base.Add(31 * time.Minute) }
	// Call list exceptions three times — each should find the same set.
	for i := 0; i < 3; i++ {
		rec, _ := r.do(t, "GET", "/api/exceptions", desk, nil)
		if rec.Code != 200 {
			t.Fatalf("round %d: %d", i, rec.Code)
		}
	}
	// Read exceptions straight from the store: still exactly 50.
	exs, _ := r.m.ListExceptions(context.Background())
	stuck := 0
	for _, e := range exs {
		if e.Kind == "picking_timeout" {
			stuck++
		}
	}
	if stuck != 50 {
		t.Fatalf("expected 50 picking_timeout exceptions, got %d", stuck)
	}
}

// TestOrderSearch_QualityIncludesOrdersInGlobalResults is a backend-side
// quality check that the global search path surfaces orders even when the
// keyword matches a status or tag rather than a name.
func TestOrderSearch_QualityIncludesOrdersInGlobalResults(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")

	// Seed an order with a distinctive tag.
	o := order.Order{
		ID: "order-xyzzy", Status: order.StatusPlaced, Priority: "rush",
		Tags: []string{"xyzzy"}, PlacedAt: time.Now(), UpdatedAt: time.Now(),
	}
	_ = r.m.CreateOrder(context.Background(), o)

	rec, _ := r.do(t, "GET", "/api/search?q=xyzzy", desk, nil)
	if rec.Code != 200 {
		t.Fatalf("search: %d", rec.Code)
	}
	var hits []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &hits)
	found := false
	for _, h := range hits {
		if h["Kind"] == "order" && h["ID"] == "order-xyzzy" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected order in global results: %s", rec.Body.String())
	}
}

// TestConcurrentCorrection_OptimisticConcurrency is an in-memory race test
// that hammers the correction handler to verify only one of N concurrent
// correction attempts succeeds; the rest must receive 409.
func TestConcurrentCorrection_OptimisticConcurrency(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	if rec.Code != 201 {
		t.Fatalf("sample: %d", rec.Code)
	}
	sampleID := body["ID"].(string)
	for _, to := range []string{"received", "in_testing"} {
		r.do(t, "POST", "/api/samples/"+sampleID+"/transitions", tech, map[string]any{"to": to})
	}
	rec, body = r.do(t, "POST", "/api/samples/"+sampleID+"/report", tech, map[string]any{
		"title": "v1", "measurements": []map[string]any{{"test_code": "GLU", "value": 85}},
	})
	if rec.Code != 201 {
		t.Fatalf("report: %d", rec.Code)
	}
	reportID := body["ID"].(string)

	// Fire 10 correction attempts with the same expected version; at most
	// one should succeed, the rest must be 409.
	const N = 10
	results := make(chan int, N)
	for i := 0; i < N; i++ {
		go func(i int) {
			rc, _ := r.do(t, "POST", "/api/reports/"+reportID+"/correct", tech, map[string]any{
				"expected_version": 1,
				"title":            "v2",
				"narrative":        fmt.Sprintf("attempt %d", i),
				"reason":           "race",
				"measurements":     []map[string]any{{"test_code": "GLU", "value": 80 + i}},
			})
			results <- rc.Code
		}(i)
	}
	var ok, conflict, other int
	for i := 0; i < N; i++ {
		switch <-results {
		case http.StatusCreated:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			other++
		}
	}
	if ok > 1 || conflict < N-1 {
		t.Fatalf("race broke invariant: ok=%d conflict=%d other=%d", ok, conflict, other)
	}
}

// Ensure the models import is used — the redaction test references the
// audit entry struct by type name indirectly through iteration.
var _ = models.AuditEntry{}
