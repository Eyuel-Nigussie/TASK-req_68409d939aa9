package store

import (
	"context"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
)

// Targeted tests to cover remaining memory-store branches the earlier
// files didn't hit.

func TestMemory_GetCustomer_MissingAndPresent(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if _, err := m.GetCustomer(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	_ = m.CreateCustomer(ctx, models.Customer{ID: "c1", Name: "Z"})
	got, err := m.GetCustomer(ctx, "c1")
	if err != nil || got.Name != "Z" {
		t.Fatalf("get: %+v %v", got, err)
	}
}

func TestMemory_GetOrder_MissingAndPresent(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if _, err := m.GetOrder(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	_ = m.CreateOrder(ctx, order.Order{ID: "o1", Status: order.StatusPlaced})
	got, err := m.GetOrder(ctx, "o1")
	if err != nil || got.ID != "o1" {
		t.Fatalf("get: %+v %v", got, err)
	}
}

func TestMemory_GetSample_MissingAndPresent(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if _, err := m.GetSample(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	_ = m.CreateSample(ctx, lab.Sample{ID: "s1", Status: lab.SampleSampling})
	got, err := m.GetSample(ctx, "s1")
	if err != nil || got.ID != "s1" {
		t.Fatalf("get: %+v %v", got, err)
	}
}

func TestMemory_ListSavedFilters_Empty(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	out, err := m.ListSavedFilters(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("expected nil for empty owner, got %v", out)
	}
}

func TestMemory_ListSavedFilters_Populated(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateSavedFilter(ctx, models.SavedFilter{ID: "f1", OwnerID: "u1", Key: "a", Name: "A"})
	_ = m.CreateSavedFilter(ctx, models.SavedFilter{ID: "f2", OwnerID: "u1", Key: "b", Name: "B"})
	_ = m.CreateSavedFilter(ctx, models.SavedFilter{ID: "f3", OwnerID: "u2", Key: "c", Name: "C"})
	out, _ := m.ListSavedFilters(ctx, "u1")
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}

func TestMemory_ListPermissions_And_UpsertPermission(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	// Seeded catalog is non-empty.
	list, _ := m.ListPermissions(ctx)
	if len(list) == 0 {
		t.Fatal("expected seeded permissions")
	}
	// Upsert a new one.
	if err := m.UpsertPermission(ctx, models.Permission{ID: "custom.perm", Description: "c"}); err != nil {
		t.Fatal(err)
	}
	list, _ = m.ListPermissions(ctx)
	found := false
	for _, p := range list {
		if p.ID == "custom.perm" {
			found = true
		}
	}
	if !found {
		t.Fatal("upsert did not persist")
	}
	// Upsert again (same ID) updates description.
	if err := m.UpsertPermission(ctx, models.Permission{ID: "custom.perm", Description: "c2"}); err != nil {
		t.Fatal(err)
	}
}

func TestMemory_ListRolePermissions_And_Set(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	out, _ := m.ListRolePermissions(ctx)
	if len(out) == 0 {
		t.Fatal("expected seeded role grants")
	}
	// Replace a role's grants with a different set.
	if err := m.SetRolePermissions(ctx, "analyst", []string{"customers.read"}); err != nil {
		t.Fatal(err)
	}
	out, _ = m.ListRolePermissions(ctx)
	count := 0
	for _, g := range out {
		if g.Role == "analyst" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 analyst grant after replace, got %d", count)
	}
	// Setting an unknown permission returns error.
	if err := m.SetRolePermissions(ctx, "analyst", []string{"no.such.permission"}); err == nil {
		t.Fatal("expected error for unknown permission")
	}
}

func TestMemory_ListUserPermissions_EmptyAndPresent(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	// Empty for unknown user.
	got, _ := m.ListUserPermissions(ctx, "u1")
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
	// After SetUserPermissions.
	_ = m.SetUserPermissions(ctx, "u1", []string{"customers.read"})
	got, _ = m.ListUserPermissions(ctx, "u1")
	if len(got) != 1 || got[0] != "customers.read" {
		t.Fatalf("expected single perm: %v", got)
	}
}

func TestMemory_OrdersPerDay_NoWindowReturnsAll(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	// No orders = empty slice, not nil.
	out, _ := m.OrdersPerDay(ctx, 0, 0)
	if len(out) != 0 {
		t.Fatalf("expected empty, got %v", out)
	}
}

func TestMemory_UpdateOrder_UpdatesFields(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateOrder(ctx, order.Order{ID: "o1", Status: order.StatusPlaced})
	o, _ := m.GetOrder(ctx, "o1")
	o.TotalCents = 500
	if err := m.UpdateOrder(ctx, o); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetOrder(ctx, "o1")
	if got.TotalCents != 500 {
		t.Fatalf("update did not persist: %+v", got)
	}
}

func TestMemory_UpdateSample_UpdatesFields(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateSample(ctx, lab.Sample{ID: "s1", Status: lab.SampleSampling})
	s, _ := m.GetSample(ctx, "s1")
	s.Notes = "updated"
	if err := m.UpdateSample(ctx, s); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetSample(ctx, "s1")
	if got.Notes != "updated" {
		t.Fatalf("update did not persist: %+v", got)
	}
}

func TestMemory_UpdateReport_Applies(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateReport(ctx, lab.Report{ID: "r1", SampleID: "s1", Version: 1, Status: lab.ReportIssued})
	r, _ := m.GetReport(ctx, "r1")
	r.Title = "Updated"
	if err := m.UpdateReport(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetReport(ctx, "r1")
	if got.Title != "Updated" {
		t.Fatalf("update did not persist: %+v", got)
	}
}

func TestMemory_LatestReportForSample_PicksHighestVersion(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateReport(ctx, lab.Report{ID: "r1", SampleID: "s1", Version: 1, Status: lab.ReportSuperseded})
	_ = m.CreateReport(ctx, lab.Report{ID: "r2", SampleID: "s1", Version: 2, Status: lab.ReportIssued})
	latest, err := m.LatestReportForSample(ctx, "s1")
	if err != nil || latest.ID != "r2" {
		t.Fatalf("latest wrong: %+v %v", latest, err)
	}
}

func TestMemory_ListArchivedReports_SortedNewestFirst(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateReport(ctx, lab.Report{ID: "r1", SampleID: "s1", Version: 1, Status: lab.ReportIssued,
		ArchivedAt: time.Unix(100, 0)})
	_ = m.CreateReport(ctx, lab.Report{ID: "r2", SampleID: "s2", Version: 1, Status: lab.ReportIssued,
		ArchivedAt: time.Unix(200, 0)})
	out, _ := m.ListArchivedReports(ctx)
	if len(out) != 2 || out[0].ID != "r2" {
		t.Fatalf("order wrong: %+v", out)
	}
}

func TestMemory_ListOrders_DateWindowFilter(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0)
	_ = m.CreateOrder(ctx, order.Order{ID: "early", PlacedAt: base, Status: order.StatusPlaced})
	_ = m.CreateOrder(ctx, order.Order{ID: "later", PlacedAt: base.Add(time.Hour), Status: order.StatusPlaced})
	from := base.Add(30 * time.Minute).Unix()
	out, _ := m.ListOrders(ctx, nil, &from, nil, 0, 0)
	if len(out) != 1 || out[0].ID != "later" {
		t.Fatalf("from filter wrong: %+v", out)
	}
	to := base.Add(15 * time.Minute).Unix()
	out, _ = m.ListOrders(ctx, nil, nil, &to, 0, 0)
	if len(out) != 1 || out[0].ID != "early" {
		t.Fatalf("to filter wrong: %+v", out)
	}
}

func TestMemory_QueryOrders_WindowAndKeyword(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0)
	_ = m.CreateOrder(ctx, order.Order{ID: "a-one", Status: order.StatusPlaced, PlacedAt: base})
	_ = m.CreateOrder(ctx, order.Order{ID: "b-two", Status: order.StatusPlaced, PlacedAt: base.Add(time.Hour)})
	from := base.Add(30 * time.Minute).Unix()
	out, total, _ := m.QueryOrders(ctx, OrderQuery{StartUnix: &from})
	if total != 1 || out[0].ID != "b-two" {
		t.Fatalf("window: total=%d out=%v", total, out)
	}
	out, _, _ = m.QueryOrders(ctx, OrderQuery{Keyword: "a-"})
	if len(out) != 1 || out[0].ID != "a-one" {
		t.Fatalf("keyword: %+v", out)
	}
	// Sort by status (string) descending.
	out, _, _ = m.QueryOrders(ctx, OrderQuery{SortBy: "status", SortDesc: true})
	if len(out) != 2 {
		t.Fatalf("sort: %+v", out)
	}
}

func TestMemory_ListAudit_WithinLimit(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = m.AppendAudit(ctx, models.AuditEntry{ID: "a", At: time.Unix(int64(i), 0)})
	}
	out, _ := m.ListAudit(ctx, "", "", 10)
	if len(out) != 3 {
		t.Fatalf("expected 3 entries (limit larger than list), got %d", len(out))
	}
}
