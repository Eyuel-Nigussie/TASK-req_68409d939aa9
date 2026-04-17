package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/order"
)

// TestSampleGate_RejectsReportOnWrongStatus asserts the controlled-workflow
// guard inside CreateReportDraft: a report may only be issued for a sample
// in `in_testing` (advanced to `reported` atomically) or `reported`. Every
// other status returns 409.
func TestSampleGate_RejectsReportOnWrongStatus(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	// Create a sample in the default "sampling" status.
	rec, body := r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	if rec.Code != 201 {
		t.Fatalf("seed sample: %d %s", rec.Code, rec.Body.String())
	}
	sampleID := body["ID"].(string)

	// Attempt report — must be rejected with 409.
	rec, _ = r.do(t, "POST", "/api/samples/"+sampleID+"/report", tech, map[string]any{
		"title": "Illegal", "measurements": []map[string]any{{"test_code": "GLU", "value": 85}},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on sampling-stage report, got %d %s", rec.Code, rec.Body.String())
	}

	// Move through received -> in_testing.
	for _, to := range []string{"received", "in_testing"} {
		if rec, _ := r.do(t, "POST", "/api/samples/"+sampleID+"/transitions", tech, map[string]any{"to": to}); rec.Code != 200 {
			t.Fatalf("transition %s: %d", to, rec.Code)
		}
	}
	// Now the report create is allowed and auto-advances to reported.
	rec, _ = r.do(t, "POST", "/api/samples/"+sampleID+"/report", tech, map[string]any{
		"title": "OK", "measurements": []map[string]any{{"test_code": "GLU", "value": 85}},
	})
	if rec.Code != 201 {
		t.Fatalf("expected 201 on in_testing sample, got %d %s", rec.Code, rec.Body.String())
	}

	// Second report on the same sample must be rejected (409) because the
	// correction endpoint is the correct tool.
	rec, _ = r.do(t, "POST", "/api/samples/"+sampleID+"/report", tech, map[string]any{
		"title": "Duplicate",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate report expected 409, got %d", rec.Code)
	}
}

// TestSampleGate_Missing404 verifies a nonexistent sample yields 404 rather
// than exposing the stage check.
func TestSampleGate_Missing404(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "POST", "/api/samples/does-not-exist/report", tech, map[string]any{"title": "x"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// TestOOS_AutoFlagOnInventorySignal verifies that flipping an item to
// backordered via /inventory surfaces an exception automatically, without
// any manual "plan" prompt.
func TestOOS_AutoFlagOnInventorySignal(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/orders", desk, map[string]any{
		"total_cents": 100,
		"items": []map[string]any{
			{"SKU": "A", "Qty": 1, "Backordered": false},
			{"SKU": "B", "Qty": 2, "Backordered": false},
		},
	})
	if rec.Code != 201 {
		t.Fatalf("seed order: %d %s", rec.Code, rec.Body.String())
	}
	orderID := body["ID"].(string)

	// Before the signal, no exceptions.
	rec, _ = r.do(t, "GET", "/api/exceptions", desk, nil)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), orderID) {
		t.Fatalf("expected empty exception queue: %s", rec.Body.String())
	}
	// Send the signal.
	rec, _ = r.do(t, "POST", "/api/orders/"+orderID+"/inventory", desk, map[string]any{
		"sku": "B", "backordered": true, "note": "not on truck",
	})
	if rec.Code != 200 {
		t.Fatalf("inventory signal: %d %s", rec.Code, rec.Body.String())
	}
	// Exception queue now contains an out_of_stock entry for this order.
	rec, _ = r.do(t, "GET", "/api/exceptions", desk, nil)
	if !strings.Contains(rec.Body.String(), orderID) || !strings.Contains(rec.Body.String(), "out_of_stock") {
		t.Fatalf("expected OOS exception for order %s: %s", orderID, rec.Body.String())
	}
	// Audit entry was written.
	entries, _ := r.m.ListAudit(context.Background(), "order_exception", orderID, 0)
	if len(entries) == 0 {
		t.Fatalf("expected order_exception audit entry")
	}
}

// TestOOS_AutoFlagOnCreate verifies that if an order is created with an
// already-backordered item the exception is raised synchronously.
func TestOOS_AutoFlagOnCreate(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	rec, body := r.do(t, "POST", "/api/orders", desk, map[string]any{
		"total_cents": 100,
		"items": []map[string]any{{"SKU": "A", "Qty": 1, "Backordered": true}},
	})
	if rec.Code != 201 {
		t.Fatalf("create: %d", rec.Code)
	}
	orderID := body["ID"].(string)
	// Read exceptions directly via the store to avoid the detector in the
	// list handler (isolating the "create-time" behavior).
	exs, _ := r.m.ListExceptions(context.Background())
	found := false
	for _, e := range exs {
		if e.OrderID == orderID && e.Kind == "out_of_stock" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected OOS exception at create: %+v", exs)
	}
}

// TestArchiveReport_RoundTrip archives an issued report, verifies it
// disappears from the default list but remains searchable, and the archive
// listing returns it.
func TestArchiveReport_RoundTrip(t *testing.T) {
	r := setup(t)
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	// Create sample in_testing.
	rec, body := r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	sampleID := body["ID"].(string)
	_ = rec
	_, _ = r.do(t, "POST", "/api/samples/"+sampleID+"/transitions", tech, map[string]any{"to": "received"})
	_, _ = r.do(t, "POST", "/api/samples/"+sampleID+"/transitions", tech, map[string]any{"to": "in_testing"})
	rec, body = r.do(t, "POST", "/api/samples/"+sampleID+"/report", tech, map[string]any{
		"title": "Archive Me", "narrative": "retention narrative",
		"measurements": []map[string]any{{"test_code": "GLU", "value": 85}},
	})
	if rec.Code != 201 {
		t.Fatalf("create report: %d %s", rec.Code, rec.Body.String())
	}
	reportID := body["ID"].(string)

	// Archive without note -> 400.
	rec, _ = r.do(t, "POST", "/api/reports/"+reportID+"/archive", tech, map[string]any{"note": ""})
	if rec.Code != 400 {
		t.Fatalf("expected 400 for blank note, got %d", rec.Code)
	}
	// Archive with note succeeds.
	rec, _ = r.do(t, "POST", "/api/reports/"+reportID+"/archive", tech, map[string]any{"note": "retention policy"})
	if rec.Code != 200 {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body.String())
	}
	// Second archive attempt -> 409.
	rec, _ = r.do(t, "POST", "/api/reports/"+reportID+"/archive", tech, map[string]any{"note": "again"})
	if rec.Code != 409 {
		t.Fatalf("double archive expected 409, got %d", rec.Code)
	}
	// Default list excludes it.
	rec, _ = r.do(t, "GET", "/api/reports", tech, nil)
	if strings.Contains(rec.Body.String(), reportID) {
		t.Fatalf("archived report leaked into default list: %s", rec.Body.String())
	}
	// Archive listing includes it.
	rec, _ = r.do(t, "GET", "/api/reports/archived", tech, nil)
	if !strings.Contains(rec.Body.String(), reportID) {
		t.Fatalf("archived listing missing report: %s", rec.Body.String())
	}
	// Search still finds it (full-text over narrative).
	rec, _ = r.do(t, "GET", "/api/reports/search?q=retention", tech, nil)
	if !strings.Contains(rec.Body.String(), reportID) {
		t.Fatalf("archived report not searchable: %s", rec.Body.String())
	}
	// Audit entry present with action=archive.
	entries, _ := r.m.ListAudit(context.Background(), "report", reportID, 0)
	found := false
	for _, e := range entries {
		if e.Action == "archive" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no archive audit entry: %+v", entries)
	}
}

// TestFeeQuoteEndpoint exercises the /dispatch/fee-quote handler path,
// including the route-table → haversine fallback decision.
func TestFeeQuoteEndpoint(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	dis := r.login(t, "dispatch1", "correct-horse-battery-staple")

	// Put a square region including (5,5) and preload a route table.
	rec, _ := r.do(t, "PUT", "/api/admin/service-regions", admin, map[string]any{
		"regions": []map[string]any{{
			"id":                 "sq",
			"vertices":           [][]float64{{0, 0}, {0, 10}, {10, 10}, {10, 0}},
			"base_fee_cents":     500,
			"per_mile_fee_cents": 25,
		}},
	})
	if rec.Code != 200 {
		t.Fatalf("put regions: %d", rec.Code)
	}
	rec, _ = r.do(t, "PUT", "/api/admin/route-table", admin, map[string]any{
		"routes": []map[string]any{{"FromID": "depot", "ToID": "drop", "Miles": 4.0}},
	})
	if rec.Code != 200 {
		t.Fatalf("put routes: %d", rec.Code)
	}

	// Route-table hit: miles=4, method=route_table, fee=500+4*25=600.
	rec, body := r.do(t, "POST", "/api/dispatch/fee-quote", dis, map[string]any{
		"from_id": "depot", "to_id": "drop",
		"from": map[string]float64{"lat": 0, "lng": 0},
		"to":   map[string]float64{"lat": 5, "lng": 5},
	})
	if rec.Code != 200 {
		t.Fatalf("quote: %d %s", rec.Code, rec.Body.String())
	}
	if method, _ := body["method"].(string); method != "route_table" {
		t.Fatalf("expected route_table method, got %v", body["method"])
	}
	if fee, _ := body["fee_cents"].(float64); fee != 600 {
		t.Fatalf("expected fee=600, got %v", body["fee_cents"])
	}

	// Unknown pair: haversine fallback, method=haversine.
	rec, body = r.do(t, "POST", "/api/dispatch/fee-quote", dis, map[string]any{
		"from_id": "a", "to_id": "b",
		"from": map[string]float64{"lat": 0, "lng": 0},
		"to":   map[string]float64{"lat": 5, "lng": 5},
	})
	if rec.Code != 200 {
		t.Fatalf("fallback quote: %d %s", rec.Code, rec.Body.String())
	}
	if method, _ := body["method"].(string); method != "haversine" {
		t.Fatalf("expected haversine fallback, got %v", body["method"])
	}

	// Destination outside service area: 422.
	rec, _ = r.do(t, "POST", "/api/dispatch/fee-quote", dis, map[string]any{
		"from_id": "a", "to_id": "b",
		"from": map[string]float64{"lat": 0, "lng": 0},
		"to":   map[string]float64{"lat": 99, "lng": 99},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for outside destination, got %d", rec.Code)
	}
}

// TestWorkstationTime_PersistedInAudit verifies the client-supplied
// X-Workstation-Time header reaches the audit entry on every mutation.
func TestWorkstationTime_PersistedInAudit(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	// r.do always sets X-Workstation; we add X-Workstation-Time via a raw
	// fetch to have precise control over the header value.
	body := `{"name":"Workstation-Time"}`
	// Use the helper: since it only sets X-Workstation, inject our own
	// X-Workstation-Time via an explicit HTTP request.
	req := newReq(t, "POST", "/api/customers", desk, body, map[string]string{
		"X-Workstation-Time": "2030-01-01T00:00:00Z",
	})
	rec := r.sendRaw(req)
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	entries, _ := r.m.ListAudit(context.Background(), "customer", "", 0)
	if len(entries) == 0 {
		t.Fatal("no audit entry")
	}
	last := entries[len(entries)-1]
	want := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if !last.WorkstationTime.Equal(want) {
		t.Fatalf("expected workstation_time=%v, got %v", want, last.WorkstationTime)
	}
}

// TestWorkstationTime_ZeroWhenHeaderMissing verifies that the server does
// not silently substitute its own clock when the header is absent.
func TestWorkstationTime_ZeroWhenHeaderMissing(t *testing.T) {
	r := setup(t)
	desk := r.login(t, "desk1", "correct-horse-battery-staple")
	req := newReq(t, "POST", "/api/customers", desk, `{"name":"no-ws-time"}`, nil)
	// Explicitly strip the header.
	req.Header.Del("X-Workstation-Time")
	rec := r.sendRaw(req)
	if rec.Code != 201 {
		t.Fatalf("create: %d", rec.Code)
	}
	entries, _ := r.m.ListAudit(context.Background(), "customer", "", 0)
	last := entries[len(entries)-1]
	if !last.WorkstationTime.IsZero() {
		t.Fatalf("expected zero workstation_time without header, got %v", last.WorkstationTime)
	}
}

// TestOrderInventory_DetectExceptionUnit is a direct unit test of the
// pure detector to complement the handler-level coverage.
func TestOrderInventory_DetectExceptionUnit(t *testing.T) {
	o := &order.Order{
		ID: "o1",
		Items: []order.LineItem{
			{SKU: "A", Backordered: false},
			{SKU: "B", Backordered: true},
		},
	}
	ex := order.DetectOutOfStock(o, time.Now())
	if ex == nil || ex.Kind != "out_of_stock" {
		t.Fatalf("expected OOS detection, got %+v", ex)
	}
	o.Items[1].Backordered = false
	if ex := order.DetectOutOfStock(o, time.Now()); ex != nil {
		t.Fatalf("expected no exception after fix, got %+v", ex)
	}
}
