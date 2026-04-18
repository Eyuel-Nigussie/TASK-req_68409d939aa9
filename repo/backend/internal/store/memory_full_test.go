package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
)

// These tests exercise every memory-store code path so the package's
// coverage reflects real behavior. The Postgres-backed methods live
// under a separate integration test (see postgres_integration_test.go).

func TestMemory_Users_UpdateAndList(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	u := models.User{ID: "u1", Username: "u1", Role: models.RoleAdmin, PasswordHash: "h"}
	_ = m.CreateUser(ctx, u)
	// ListUsers returns sorted.
	list, _ := m.ListUsers(ctx)
	if len(list) != 1 {
		t.Fatalf("list size: %d", len(list))
	}
	// Update missing is NotFound.
	if err := m.UpdateUser(ctx, models.User{ID: "ghost"}); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	// Update existing.
	u.Disabled = true
	if err := m.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetUserByID(ctx, "u1")
	if !got.Disabled {
		t.Fatal("update did not persist")
	}
	if _, err := m.GetUserByID(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemory_Customers_UpdateAndFindByAddress(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	c := models.Customer{ID: "c1", Name: "Jane", City: "Alpha", ZIP: "11111"}
	_ = m.CreateCustomer(ctx, c)
	if err := m.CreateCustomer(ctx, c); err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	c.Name = "Jane X"
	if err := m.UpdateCustomer(ctx, c); err != nil {
		t.Fatal(err)
	}
	// Update missing → NotFound.
	if err := m.UpdateCustomer(ctx, models.Customer{ID: "ghost"}); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	// FindByAddress with city and zip.
	got, _ := m.FindByAddress(ctx, "", "Alpha", "11111")
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	// No filter (both city and zip missing) matches all.
	got, _ = m.FindByAddress(ctx, "", "", "")
	if len(got) != 1 {
		t.Fatalf("expected 1 (no filter), got %d", len(got))
	}
}

func TestMemory_AddressBook_CRUD(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	a := models.AddressBookEntry{ID: "a1", OwnerID: "u1", Label: "home"}
	_ = m.CreateAddress(ctx, a)
	list, _ := m.ListAddresses(ctx, "u1")
	if len(list) != 1 {
		t.Fatalf("size: %d", len(list))
	}
	// Delete by wrong owner → NotFound.
	if err := m.DeleteAddress(ctx, "otheruser", "a1"); err != ErrNotFound {
		t.Fatalf("expected NotFound cross-owner, got %v", err)
	}
	if err := m.DeleteAddress(ctx, "u1", "ghost"); err != ErrNotFound {
		t.Fatalf("expected NotFound ghost, got %v", err)
	}
	if err := m.DeleteAddress(ctx, "u1", "a1"); err != nil {
		t.Fatal(err)
	}
}

func TestMemory_ServiceRegions_ReplaceRoundTrip(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	regions := []geo.Region{
		{Polygon: geo.Polygon{ID: "z1", Vertices: []geo.Point{{0, 0}, {0, 1}, {1, 1}}}, BaseFeeCents: 100},
	}
	if err := m.ReplaceRegions(ctx, regions); err != nil {
		t.Fatal(err)
	}
	got, _ := m.ListRegions(ctx)
	if len(got) != 1 || got[0].Polygon.ID != "z1" {
		t.Fatalf("regions not persisted: %+v", got)
	}
}

func TestMemory_Orders_EventAppendAndList(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	o := order.Order{ID: "o1", Status: order.StatusPlaced, PlacedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0)}
	_ = m.CreateOrder(ctx, o)
	if err := m.CreateOrder(ctx, o); err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	// Update non-existent.
	if err := m.UpdateOrder(ctx, order.Order{ID: "ghost"}); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	// Event append to non-existent order.
	if err := m.AppendOrderEvent(ctx, order.Event{OrderID: "ghost"}); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	// Append to existing.
	if err := m.AppendOrderEvent(ctx, order.Event{ID: "e1", OrderID: "o1", To: order.StatusPicking}); err != nil {
		t.Fatal(err)
	}
	// List with pagination offset past the end returns an empty
	// slice — the JSON envelope is always an array so the SPA can
	// `.length` and `.map` the response without a null-check.
	out, _ := m.ListOrders(ctx, nil, nil, nil, 10, 100)
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty slice past offset, got %v", out)
	}
	// Limit-less list returns everything.
	out, _ = m.ListOrders(ctx, nil, nil, nil, 0, 0)
	if len(out) != 1 {
		t.Fatalf("expected 1, got %d", len(out))
	}
}

func TestMemory_Samples_CRUDAndList(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	s := lab.Sample{ID: "s1", Status: lab.SampleSampling, CollectedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), TestCodes: []string{"A"}}
	_ = m.CreateSample(ctx, s)
	if err := m.CreateSample(ctx, s); err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	if err := m.UpdateSample(ctx, lab.Sample{ID: "ghost"}); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	// List with offset past end → empty slice (never nil, so JSON
	// marshals to `[]` and the SPA can iterate without a null check).
	out, _ := m.ListSamples(ctx, nil, 10, 999)
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty slice past offset, got %v", out)
	}
	// List filtered by an unknown status returns an empty slice.
	out, _ = m.ListSamples(ctx, []string{"unknown"}, 10, 0)
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty slice, got %v", out)
	}
}

func TestMemory_Reports_LookupAndCorrection(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if _, err := m.GetReport(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	if _, err := m.LatestReportForSample(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	r := lab.Report{ID: "r1", SampleID: "s1", Version: 1, Status: lab.ReportIssued, Title: "t", SearchText: "alpha beta"}
	_ = m.CreateReport(ctx, r)
	if err := m.CreateReport(ctx, r); err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	// UpdateReport missing.
	if err := m.UpdateReport(ctx, lab.Report{ID: "ghost"}); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	// SearchReports scores results deterministically.
	found, _ := m.SearchReports(ctx, "alpha", 5)
	if len(found) != 1 {
		t.Fatalf("expected 1 search hit, got %d", len(found))
	}
	// ListReports with pagination offset returns an empty slice.
	out, _ := m.ListReports(ctx, 0, 100)
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty slice, got %v", out)
	}
}

func TestMemory_SavedFilters_CRUD(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	f := models.SavedFilter{ID: "f1", OwnerID: "u1", Key: "k"}
	_ = m.CreateSavedFilter(ctx, f)
	// Delete by wrong owner → NotFound.
	if err := m.DeleteSavedFilter(ctx, "other", "f1"); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	if err := m.DeleteSavedFilter(ctx, "u1", "ghost"); err != ErrNotFound {
		t.Fatalf("expected NotFound ghost, got %v", err)
	}
	if err := m.DeleteSavedFilter(ctx, "u1", "f1"); err != nil {
		t.Fatal(err)
	}
}

func TestMemory_Audit_ListLimitAndEntityFilter(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = m.AppendAudit(ctx, models.AuditEntry{ID: "a", At: time.Unix(int64(i), 0), Entity: "order", EntityID: "o1"})
	}
	// Limit narrows to the newest N.
	out, _ := m.ListAudit(ctx, "order", "o1", 2)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
	// Entity filter that matches nothing returns an empty slice.
	out, _ = m.ListAudit(ctx, "nothing", "", 0)
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty slice for no-match filter, got %v", out)
	}
}

func TestMemory_Exceptions_Dedup(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	ex := order.Exception{OrderID: "o1", Kind: "picking_timeout", DetectedAt: time.Unix(1, 0)}
	_ = m.PutException(ctx, ex)
	_ = m.PutException(ctx, ex) // same key overwrites, not duplicate
	out, _ := m.ListExceptions(ctx)
	if len(out) != 1 {
		t.Fatalf("expected single exception, got %d", len(out))
	}
}

func TestMemory_Analytics_SampleStatusAndExceptionKinds(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateSample(ctx, lab.Sample{ID: "s1", Status: lab.SampleSampling, CollectedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), TestCodes: []string{"A"}})
	_ = m.CreateSample(ctx, lab.Sample{ID: "s2", Status: lab.SampleSampling, CollectedAt: time.Unix(2, 0), UpdatedAt: time.Unix(2, 0), TestCodes: []string{"A"}})
	counts, _ := m.SampleStatusCounts(ctx, 0, 0)
	if counts[string(lab.SampleSampling)] != 2 {
		t.Fatalf("expected 2 sampling: %+v", counts)
	}
	// With a time window that excludes everything.
	counts, _ = m.SampleStatusCounts(ctx, 100, 200)
	if len(counts) != 0 {
		t.Fatalf("window should filter out all: %+v", counts)
	}

	_ = m.PutException(ctx, order.Exception{OrderID: "o1", Kind: "picking_timeout", DetectedAt: time.Now()})
	_ = m.PutException(ctx, order.Exception{OrderID: "o2", Kind: "out_of_stock", DetectedAt: time.Now()})
	kinds, _ := m.ExceptionCountsByKind(ctx)
	if kinds["picking_timeout"] != 1 || kinds["out_of_stock"] != 1 {
		t.Fatalf("exception counts wrong: %+v", kinds)
	}
}

func TestMemory_OrdersPerDay_WindowAndSort(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		_ = m.CreateOrder(ctx, order.Order{
			ID:        "o-" + strings.Repeat("x", i+1),
			Status:    order.StatusPlaced,
			PlacedAt:  base.AddDate(0, 0, i),
			UpdatedAt: base,
		})
	}
	series, _ := m.OrdersPerDay(ctx, 0, 0)
	if len(series) < 3 {
		t.Fatalf("expected 3 bucket days, got %d", len(series))
	}
	// Window limiting.
	limited, _ := m.OrdersPerDay(ctx, base.AddDate(0, 0, 1).Unix(), base.AddDate(0, 0, 2).Unix())
	if len(limited) != 2 {
		t.Fatalf("expected 2 windowed buckets, got %d", len(limited))
	}
}

func TestMemory_Search_CustomerRanking(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateCustomer(ctx, models.Customer{ID: "c1", Name: "Alice"})
	_ = m.CreateCustomer(ctx, models.Customer{ID: "c2", Name: "Bob"})
	out, _ := m.SearchCustomers(ctx, "alice", 5)
	if len(out) == 0 || out[0].Name != "Alice" {
		t.Fatalf("search wrong: %+v", out)
	}
}
