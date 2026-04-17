package store

import (
	"context"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
)

func TestMemory_Users(t *testing.T) {
	m := NewMemory()
	u := models.User{ID: "u1", Username: "alice", Role: models.RoleAdmin, PasswordHash: "h"}
	if err := m.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateUser(context.Background(), u); err != ErrConflict {
		t.Fatalf("duplicate should conflict, got %v", err)
	}
	got, err := m.GetUserByUsername(context.Background(), "alice")
	if err != nil || got.ID != "u1" {
		t.Fatalf("lookup: %v %v", got, err)
	}
	if _, err := m.GetUserByUsername(context.Background(), "ghost"); err != ErrNotFound {
		t.Fatal("missing should be NotFound")
	}
}

func TestMemory_CustomerSearchAndAddressLookup(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateCustomer(ctx, models.Customer{ID: "c1", Name: "Jane Doe", Street: "123 Elm St", City: "Springfield", ZIP: "62701"})
	_ = m.CreateCustomer(ctx, models.Customer{ID: "c2", Name: "John Roe", Street: "45 Oak Ave", City: "Springfield", ZIP: "62702"})

	res, err := m.SearchCustomers(ctx, "jane", 10)
	if err != nil || len(res) == 0 || res[0].ID != "c1" {
		t.Fatalf("search miss: %+v %v", res, err)
	}
	byAddr, _ := m.FindByAddress(ctx, "elm", "springfield", "62701")
	if len(byAddr) != 1 || byAddr[0].ID != "c1" {
		t.Fatalf("address lookup: %+v", byAddr)
	}
}

func TestMemory_OrderLifecycleAndListFilter(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	o := order.Order{ID: "o1", Status: order.StatusPlaced, PlacedAt: time.Unix(1_700_000_000, 0), UpdatedAt: time.Unix(1_700_000_000, 0)}
	_ = m.CreateOrder(ctx, o)
	_ = m.CreateOrder(ctx, order.Order{ID: "o2", Status: order.StatusPicking, PlacedAt: time.Unix(1_700_000_100, 0), UpdatedAt: time.Unix(1_700_000_100, 0)})

	out, _ := m.ListOrders(ctx, []string{"picking"}, nil, nil, 0, 0)
	if len(out) != 1 || out[0].ID != "o2" {
		t.Fatalf("filter by status failed: %+v", out)
	}
}

func TestMemory_ReportCorrection(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	r1 := lab.Report{ID: "r1", SampleID: "s1", Version: 1, Status: lab.ReportIssued, Title: "CBC", Narrative: "narrative", SearchText: "cbc narrative"}
	_ = m.CreateReport(ctx, r1)
	old := r1
	old.Status = lab.ReportSuperseded
	old.SupersededByID = "r2"
	next := lab.Report{ID: "r2", SampleID: "s1", Version: 2, Status: lab.ReportIssued, Title: "CBC", Narrative: "corrected", SearchText: "cbc corrected"}
	_ = m.ReplaceWithCorrection(ctx, old, next)

	latest, err := m.LatestReportForSample(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != "r2" || latest.Version != 2 {
		t.Fatalf("latest wrong: %+v", latest)
	}
	prior, _ := m.GetReport(ctx, "r1")
	if prior.Status != lab.ReportSuperseded {
		t.Fatalf("prior should be superseded: %+v", prior)
	}
}

func TestMemory_AuditAppendOnly(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 3; i++ {
		_ = m.AppendAudit(ctx, models.AuditEntry{
			ID: "a", At: base.Add(time.Duration(i) * time.Second), Entity: "order", EntityID: "o1", Action: "touch",
		})
	}
	entries, _ := m.ListAudit(ctx, "order", "o1", 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Ordering is chronological.
	if !entries[0].At.Before(entries[2].At) {
		t.Fatal("audit not chronological")
	}
}

func TestMemory_OrdersByAddress(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.CreateOrder(ctx, order.Order{
		ID: "o1", Status: order.StatusPlaced, PlacedAt: time.Unix(1_700_000_000, 0), UpdatedAt: time.Unix(1_700_000_000, 0),
		DeliveryCity: "Springfield", DeliveryZIP: "62701",
	})
	_ = m.CreateOrder(ctx, order.Order{
		ID: "o2", Status: order.StatusPlaced, PlacedAt: time.Unix(1_700_000_001, 0), UpdatedAt: time.Unix(1_700_000_001, 0),
		DeliveryCity: "Riverdale", DeliveryZIP: "62702",
	})
	got, _ := m.OrdersByAddress(ctx, "Springfield", "62701")
	if len(got) != 1 || got[0].ID != "o1" {
		t.Fatalf("city+zip match wrong: %+v", got)
	}
	got, _ = m.OrdersByAddress(ctx, "", "62702")
	if len(got) != 1 || got[0].ID != "o2" {
		t.Fatalf("zip-only match wrong: %+v", got)
	}
}

func TestMemory_ArchivedReports(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	r1 := lab.Report{ID: "r1", SampleID: "s1", Version: 1, Status: lab.ReportIssued, Title: "Keep"}
	r2 := lab.Report{
		ID: "r2", SampleID: "s2", Version: 1, Status: lab.ReportIssued, Title: "Gone",
		ArchivedAt: time.Unix(1_700_000_100, 0), ArchivedBy: "u1",
	}
	_ = m.CreateReport(ctx, r1)
	_ = m.CreateReport(ctx, r2)
	// Default list excludes archived.
	all, _ := m.ListReports(ctx, 0, 0)
	for _, r := range all {
		if r.ID == "r2" {
			t.Fatal("archived report leaked into default list")
		}
	}
	// Archive listing shows only archived.
	arc, _ := m.ListArchivedReports(ctx)
	if len(arc) != 1 || arc[0].ID != "r2" {
		t.Fatalf("archive listing wrong: %+v", arc)
	}
}

func TestMemory_QueryOrders_ByFilter(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0)
	mk := func(id, status, priority string, cents int, tags []string, offset int64) order.Order {
		return order.Order{
			ID: id, Status: order.Status(status), Priority: priority, TotalCents: cents,
			Tags: tags, PlacedAt: base.Add(time.Duration(offset) * time.Second), UpdatedAt: base,
		}
	}
	_ = m.CreateOrder(ctx, mk("o1", "placed", "standard", 1000, []string{"a"}, 0))
	_ = m.CreateOrder(ctx, mk("o2", "picking", "rush", 2500, []string{"b"}, 100))
	_ = m.CreateOrder(ctx, mk("o3", "placed", "rush", 5000, []string{"a", "b"}, 200))

	out, total, err := m.QueryOrders(ctx, OrderQuery{Statuses: []string{"placed"}})
	if err != nil || total != 2 || len(out) != 2 {
		t.Fatalf("status filter: total=%d len=%d err=%v", total, len(out), err)
	}
	out, _, _ = m.QueryOrders(ctx, OrderQuery{Priority: "rush"})
	if len(out) != 2 {
		t.Fatalf("priority filter returned %d", len(out))
	}
	min := 2000
	out, _, _ = m.QueryOrders(ctx, OrderQuery{MinCents: &min})
	if len(out) != 2 {
		t.Fatalf("min price filter returned %d", len(out))
	}
	out, _, _ = m.QueryOrders(ctx, OrderQuery{Tags: []string{"a"}})
	if len(out) != 2 {
		t.Fatalf("tag filter returned %d", len(out))
	}
	out, _, _ = m.QueryOrders(ctx, OrderQuery{Keyword: "o2"})
	if len(out) != 1 || out[0].ID != "o2" {
		t.Fatalf("keyword filter wrong: %v", out)
	}
	// Sorting: total_cents asc
	out, _, _ = m.QueryOrders(ctx, OrderQuery{SortBy: "total_cents"})
	if len(out) != 3 || out[0].ID != "o1" || out[2].ID != "o3" {
		t.Fatalf("sort wrong: %v", out)
	}
	// Pagination: limit 1, offset 1 of size-3
	out, total, _ = m.QueryOrders(ctx, OrderQuery{SortBy: "total_cents", Limit: 1, Offset: 1})
	if total != 3 || len(out) != 1 || out[0].ID != "o2" {
		t.Fatalf("pagination wrong: total=%d out=%v", total, out)
	}
}

func TestMemory_RefRanges_RoundTrip(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	low := 70.0
	high := 99.0
	rr := []lab.RefRange{{TestCode: "GLU", LowNormal: &low, HighNormal: &high}}
	if err := m.ReplaceRefRanges(ctx, rr); err != nil {
		t.Fatal(err)
	}
	got, _ := m.ListRefRanges(ctx)
	if len(got) != 1 || got[0].TestCode != "GLU" {
		t.Fatalf("ref ranges not persisted: %+v", got)
	}
}

func TestMemory_Routes_RoundTrip(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	rows := []RouteRow{{FromID: "A", ToID: "B", Miles: 12.5}}
	if err := m.ReplaceRoutes(ctx, rows); err != nil {
		t.Fatal(err)
	}
	got, _ := m.ListRoutes(ctx)
	if len(got) != 1 || got[0].Miles != 12.5 {
		t.Fatalf("routes not persisted: %+v", got)
	}
}

func TestMemory_PermissionGrants_RoleAndUserMerge(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	// admin role has "admin.users" by default seed.
	grants, _ := m.GrantsForUser(ctx, "u1", "admin")
	hasAdminUsers := false
	for _, g := range grants {
		if g == "admin.users" {
			hasAdminUsers = true
		}
	}
	if !hasAdminUsers {
		t.Fatalf("expected admin.users in role grants: %v", grants)
	}
	// Add an individual user grant and verify it merges.
	if err := m.SetUserPermissions(ctx, "u1", []string{"analytics.view"}); err != nil {
		t.Fatal(err)
	}
	grants, _ = m.GrantsForUser(ctx, "u1", "front_desk")
	found := false
	for _, g := range grants {
		if g == "analytics.view" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected analytics.view (user grant) to merge in: %v", grants)
	}
	// Unknown permission should be rejected.
	if err := m.SetUserPermissions(ctx, "u1", []string{"not.real"}); err == nil {
		t.Fatal("expected error for unknown permission")
	}
}

func TestMemory_LoginAttempts_Persistence(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	// Start empty.
	if _, err := m.GetLoginAttempt(ctx, "alice"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	// Upsert once, read back.
	if err := m.UpsertLoginAttempt(ctx, models.LoginAttempt{
		Username: "alice", Failures: 3, UpdatedAt: time.Unix(1_700_000_000, 0),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetLoginAttempt(ctx, "alice")
	if err != nil || got.Failures != 3 {
		t.Fatalf("want 3 failures, got %+v err=%v", got, err)
	}
	// Clear removes.
	_ = m.ClearLoginAttempt(ctx, "alice")
	if _, err := m.GetLoginAttempt(ctx, "alice"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after clear, got %v", err)
	}
}

func TestMemory_Analytics_StatusCountsAndAbnormalRate(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	// Two orders and a reported sample with one abnormal measurement.
	_ = m.CreateOrder(ctx, order.Order{ID: "o1", Status: order.StatusPlaced, PlacedAt: time.Unix(1_700_000_000, 0), UpdatedAt: time.Unix(1_700_000_000, 0)})
	_ = m.CreateOrder(ctx, order.Order{ID: "o2", Status: order.StatusPlaced, PlacedAt: time.Unix(1_700_000_001, 0), UpdatedAt: time.Unix(1_700_000_001, 0)})
	_ = m.CreateOrder(ctx, order.Order{ID: "o3", Status: order.StatusPicking, PlacedAt: time.Unix(1_700_000_002, 0), UpdatedAt: time.Unix(1_700_000_002, 0)})

	counts, _ := m.OrderStatusCounts(ctx, 0, 0)
	if counts[string(order.StatusPlaced)] != 2 || counts[string(order.StatusPicking)] != 1 {
		t.Fatalf("status counts wrong: %+v", counts)
	}

	// Abnormal rate: 1 abnormal out of 2 measurements.
	_ = m.CreateReport(ctx, lab.Report{
		ID: "r1", SampleID: "s1", Version: 1, Status: lab.ReportIssued,
		Title: "t", IssuedAt: time.Unix(1_700_000_000, 0),
		Measurements: []lab.Measurement{
			{TestCode: "A", Value: 1, Flag: lab.FlagNormal},
			{TestCode: "B", Value: 2, Flag: lab.FlagHigh},
		},
	})
	rate, _ := m.AbnormalReportRate(ctx, 0, 0)
	if rate.TotalMeasurements != 2 || rate.AbnormalMeasurements != 1 {
		t.Fatalf("abnormal rate wrong: %+v", rate)
	}
	if rate.Rate < 0.49 || rate.Rate > 0.51 {
		t.Fatalf("expected ~0.5 rate, got %v", rate.Rate)
	}
}

func TestMemory_SavedFilterDedupe(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	f := models.SavedFilter{ID: "f1", OwnerID: "u1", Key: "key1", Name: "n"}
	_ = m.CreateSavedFilter(ctx, f)
	dup := models.SavedFilter{ID: "f2", OwnerID: "u1", Key: "key1", Name: "n"}
	if err := m.CreateSavedFilter(ctx, dup); err != ErrConflict {
		t.Fatalf("duplicate should conflict: %v", err)
	}
}
