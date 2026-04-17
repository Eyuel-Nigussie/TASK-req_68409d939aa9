package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
	_ "github.com/lib/pq"
)

// TestPostgres_FullSurface exercises every Postgres Store method so the
// backend reports full coverage under INTEGRATION_DB. The test uses a
// fresh schema (drop + re-apply) and is safe to run repeatedly.
func TestPostgres_FullSurface(t *testing.T) {
	dsn := os.Getenv("INTEGRATION_DB")
	if dsn == "" {
		t.Skip("INTEGRATION_DB not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skip("postgres not reachable: " + err.Error())
	}
	resetSchema(t, db)
	applyMigration(t, db)
	p := NewPostgres(db)

	// Users
	u1 := models.User{ID: "u1", Username: "alice", Role: models.RoleAdmin, PasswordHash: "h", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	u2 := models.User{ID: "u2", Username: "bob", Role: models.RoleFrontDesk, PasswordHash: "h", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	mustNoErr(t, p.CreateUser(ctx, u1))
	mustNoErr(t, p.CreateUser(ctx, u2))
	if err := p.CreateUser(ctx, u1); err != ErrConflict {
		t.Fatalf("expected conflict on duplicate username, got %v", err)
	}
	if _, err := p.GetUserByID(ctx, "u1"); err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if _, err := p.GetUserByID(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	users, _ := p.ListUsers(ctx)
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	u1.Disabled = true
	mustNoErr(t, p.UpdateUser(ctx, u1))
	if err := p.UpdateUser(ctx, models.User{ID: "ghost", Role: models.RoleAdmin}); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}

	// Customers
	cu := models.Customer{ID: "c1", Name: "Jane", City: "Alpha", ZIP: "11111",
		Identifier: "enc-id", Street: "enc-street", Tags: []string{"vip"}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	mustNoErr(t, p.CreateCustomer(ctx, cu))
	if _, err := p.GetCustomer(ctx, "c1"); err != nil {
		t.Fatalf("get customer: %v", err)
	}
	if _, err := p.GetCustomer(ctx, "ghost"); err != ErrNotFound {
		t.Fatal("expected NotFound")
	}
	cu.Name = "Jane X"
	mustNoErr(t, p.UpdateCustomer(ctx, cu))
	if err := p.UpdateCustomer(ctx, models.Customer{ID: "ghost"}); err != ErrNotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	// SearchCustomers: hit both the tsquery and the ILIKE paths.
	if _, err := p.SearchCustomers(ctx, "jane", 5); err != nil {
		t.Fatalf("search: %v", err)
	}
	if _, err := p.SearchCustomers(ctx, "ja", 0); err != nil {
		t.Fatalf("search default limit: %v", err)
	}
	// FindByAddress with city+zip and empty filters.
	out, _ := p.FindByAddress(ctx, "", "Alpha", "11111")
	if len(out) != 1 {
		t.Fatalf("find: %+v", out)
	}
	out, _ = p.FindByAddress(ctx, "", "", "")
	if len(out) != 1 {
		t.Fatalf("no filter: %+v", out)
	}

	// AddressBook
	ab := models.AddressBookEntry{ID: "a1", OwnerID: "u1", Label: "home", Street: "enc", City: "X", ZIP: "111", Lat: 1, Lng: 2, CreatedAt: time.Now()}
	mustNoErr(t, p.CreateAddress(ctx, ab))
	list, _ := p.ListAddresses(ctx, "u1")
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
	if err := p.DeleteAddress(ctx, "u2", "a1"); err != ErrNotFound {
		t.Fatalf("cross-owner delete: %v", err)
	}
	if err := p.DeleteAddress(ctx, "u1", "ghost"); err != ErrNotFound {
		t.Fatalf("ghost delete: %v", err)
	}
	mustNoErr(t, p.DeleteAddress(ctx, "u1", "a1"))

	// Service regions
	regions := []geo.Region{
		{Polygon: geo.Polygon{ID: "z1", Vertices: []geo.Point{{0, 0}, {0, 10}, {10, 10}, {10, 0}}}, BaseFeeCents: 100, PerMileFeeCents: 10},
	}
	mustNoErr(t, p.ReplaceRegions(ctx, regions))
	rlist, _ := p.ListRegions(ctx)
	if len(rlist) != 1 || rlist[0].Polygon.ID != "z1" {
		t.Fatalf("regions: %+v", rlist)
	}

	// Orders
	now := time.Now()
	o1 := order.Order{
		ID: "o1", Status: order.StatusPlaced, CustomerID: "c1", TotalCents: 1000, Priority: "standard",
		Tags: []string{"inbound"}, PlacedAt: now, UpdatedAt: now,
		DeliveryStreet: "enc", DeliveryCity: "Alpha", DeliveryZIP: "11111",
		Items: []order.LineItem{{SKU: "A", Qty: 1}},
	}
	mustNoErr(t, p.CreateOrder(ctx, o1))
	if err := p.CreateOrder(ctx, o1); err != ErrConflict {
		t.Fatalf("dup order: %v", err)
	}
	if _, err := p.GetOrder(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("ghost order: %v", err)
	}
	gotOrder, _ := p.GetOrder(ctx, "o1")
	if gotOrder.CustomerID != "c1" {
		t.Fatalf("order: %+v", gotOrder)
	}
	o1.TotalCents = 2000
	mustNoErr(t, p.UpdateOrder(ctx, o1))
	if err := p.UpdateOrder(ctx, order.Order{ID: "ghost"}); err != ErrNotFound {
		t.Fatalf("ghost update: %v", err)
	}
	mustNoErr(t, p.AppendOrderEvent(ctx, order.Event{ID: "e1", OrderID: "o1", At: time.Now(), From: order.StatusPlaced, To: order.StatusPicking, Actor: "u1"}))
	gotOrder, _ = p.GetOrder(ctx, "o1")
	if len(gotOrder.Events) != 1 {
		t.Fatalf("events not loaded: %+v", gotOrder.Events)
	}
	fromUnix := now.Add(-time.Hour).Unix()
	toUnix := now.Add(time.Hour).Unix()
	olist, _ := p.ListOrders(ctx, []string{string(order.StatusPlaced)}, &fromUnix, &toUnix, 10, 0)
	if len(olist) != 1 {
		t.Fatalf("list: %+v", olist)
	}
	// Pagination > count.
	olist, _ = p.ListOrders(ctx, nil, nil, nil, 10, 100)
	if len(olist) != 0 {
		t.Fatalf("offset beyond end: %+v", olist)
	}
	// QueryOrders with all fields.
	minC := 500
	maxC := 3000
	orders, total, _ := p.QueryOrders(ctx, OrderQuery{
		Keyword: "o1", Statuses: []string{"placed"}, Tags: []string{"inbound"},
		Priority: "standard", StartUnix: &fromUnix, EndUnix: &toUnix,
		MinCents: &minC, MaxCents: &maxC,
		SortBy: "total_cents", SortDesc: true, Limit: 10,
	})
	if total == 0 || len(orders) == 0 {
		t.Fatalf("query: total=%d orders=%+v", total, orders)
	}
	// OrdersByAddress
	addrOrders, _ := p.OrdersByAddress(ctx, "Alpha", "11111")
	if len(addrOrders) != 1 {
		t.Fatalf("by address: %+v", addrOrders)
	}

	// Exceptions
	ex := order.Exception{OrderID: "o1", Kind: "picking_timeout", DetectedAt: time.Now(), Description: "stuck"}
	mustNoErr(t, p.PutException(ctx, ex))
	mustNoErr(t, p.PutException(ctx, ex)) // ON CONFLICT path
	elist, _ := p.ListExceptions(ctx)
	if len(elist) != 1 {
		t.Fatalf("exceptions: %+v", elist)
	}

	// Samples
	s1 := lab.Sample{ID: "s1", OrderID: "o1", CustomerID: "c1", Status: lab.SampleSampling, CollectedAt: now, UpdatedAt: now, TestCodes: []string{"GLU"}}
	mustNoErr(t, p.CreateSample(ctx, s1))
	if err := p.CreateSample(ctx, s1); err != ErrConflict {
		t.Fatalf("dup sample: %v", err)
	}
	got, _ := p.GetSample(ctx, "s1")
	if got.OrderID != "o1" {
		t.Fatalf("sample: %+v", got)
	}
	s1.Status = lab.SampleReceived
	mustNoErr(t, p.UpdateSample(ctx, s1))
	if err := p.UpdateSample(ctx, lab.Sample{ID: "ghost"}); err != ErrNotFound {
		t.Fatalf("ghost update: %v", err)
	}
	sl, _ := p.ListSamples(ctx, []string{"received"}, 10, 0)
	if len(sl) != 1 {
		t.Fatalf("sample list: %+v", sl)
	}
	sl, _ = p.ListSamples(ctx, nil, 10, 100) // offset beyond end
	if len(sl) != 0 {
		t.Fatalf("expected empty: %+v", sl)
	}

	// Advance sample to in_testing so we can issue a report.
	s1.Status = lab.SampleInTesting
	mustNoErr(t, p.UpdateSample(ctx, s1))
	r1 := lab.Report{ID: "r1", SampleID: "s1", Version: 1, Status: lab.ReportIssued,
		Title: "CBC", Narrative: "n", Measurements: []lab.Measurement{{TestCode: "GLU", Value: 85, Flag: lab.FlagNormal}},
		AuthorID: "u1", IssuedAt: now,
	}
	mustNoErr(t, p.CreateReport(ctx, r1))
	if _, err := p.GetReport(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("ghost: %v", err)
	}
	if _, err := p.LatestReportForSample(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("latest ghost: %v", err)
	}
	r1.Narrative = "updated"
	mustNoErr(t, p.UpdateReport(ctx, r1))
	if err := p.UpdateReport(ctx, lab.Report{ID: "ghost"}); err != ErrNotFound {
		t.Fatalf("ghost update: %v", err)
	}
	// Correction.
	old := r1
	old.Status = lab.ReportSuperseded
	old.SupersededByID = "r2"
	next := lab.Report{ID: "r2", SampleID: "s1", Version: 2, Status: lab.ReportIssued, Title: "CBC", Narrative: "corrected", ReasonNote: "typo", IssuedAt: now}
	mustNoErr(t, p.ReplaceWithCorrection(ctx, old, next))
	// Search.
	sr, _ := p.SearchReports(ctx, "corrected", 5)
	if len(sr) == 0 {
		t.Fatalf("search report: %+v", sr)
	}
	reports, _ := p.ListReports(ctx, 10, 0)
	if len(reports) == 0 {
		t.Fatalf("list reports: %+v", reports)
	}
	// Archive.
	r2, _ := p.GetReport(ctx, "r2")
	r2.ArchivedAt = time.Now()
	r2.ArchivedBy = "u1"
	r2.ArchiveNote = "retention"
	mustNoErr(t, p.UpdateReport(ctx, r2))
	archived, _ := p.ListArchivedReports(ctx)
	if len(archived) != 1 {
		t.Fatalf("archived list: %+v", archived)
	}

	// Saved filters
	mustNoErr(t, p.CreateSavedFilter(ctx, models.SavedFilter{ID: "f1", OwnerID: "u1", Name: "n", Payload: []byte(`{"entity":"order"}`), Key: "k1", CreatedAt: now}))
	fl, _ := p.ListSavedFilters(ctx, "u1")
	if len(fl) != 1 {
		t.Fatalf("saved filters: %+v", fl)
	}
	if err := p.DeleteSavedFilter(ctx, "u2", "f1"); err != ErrNotFound {
		t.Fatal("cross-owner delete")
	}
	mustNoErr(t, p.DeleteSavedFilter(ctx, "u1", "f1"))

	// Reference ranges
	low := 1.0
	high := 10.0
	mustNoErr(t, p.ReplaceRefRanges(ctx, []lab.RefRange{{TestCode: "GLU", Units: "mg", LowNormal: &low, HighNormal: &high, Demographic: "adult"}}))
	rr, _ := p.ListRefRanges(ctx)
	if len(rr) != 1 {
		t.Fatalf("ref ranges: %+v", rr)
	}

	// Route table
	mustNoErr(t, p.ReplaceRoutes(ctx, []RouteRow{{FromID: "A", ToID: "B", Miles: 5}}))
	routes, _ := p.ListRoutes(ctx)
	if len(routes) != 1 {
		t.Fatalf("routes: %+v", routes)
	}
	// Replacing again triggers DELETE then INSERT path + upsert.
	mustNoErr(t, p.ReplaceRoutes(ctx, []RouteRow{{FromID: "B", ToID: "A", Miles: 7}, {FromID: "C", ToID: "D", Miles: 1}}))

	// Permissions
	perms, _ := p.ListPermissions(ctx)
	if len(perms) == 0 {
		t.Fatal("expected seeded perms")
	}
	mustNoErr(t, p.UpsertPermission(ctx, models.Permission{ID: "custom.perm", Description: "c"}))
	mustNoErr(t, p.UpsertPermission(ctx, models.Permission{ID: "custom.perm", Description: "c2"}))
	rp, _ := p.ListRolePermissions(ctx)
	if len(rp) == 0 {
		t.Fatal("expected role grants")
	}
	mustNoErr(t, p.SetRolePermissions(ctx, "analyst", []string{"customers.read"}))
	mustNoErr(t, p.SetUserPermissions(ctx, "u1", []string{"analytics.view"}))
	up, _ := p.ListUserPermissions(ctx, "u1")
	if len(up) != 1 {
		t.Fatalf("user perms: %+v", up)
	}
	// Empty replacement.
	mustNoErr(t, p.SetUserPermissions(ctx, "u1", nil))

	// Login attempts
	mustNoErr(t, p.UpsertLoginAttempt(ctx, models.LoginAttempt{Username: "alice", Failures: 2, UpdatedAt: time.Now()}))
	la, err := p.GetLoginAttempt(ctx, "alice")
	if err != nil || la.Failures != 2 {
		t.Fatalf("get attempt: %+v %v", la, err)
	}
	if _, err := p.GetLoginAttempt(ctx, "ghost"); err != ErrNotFound {
		t.Fatalf("ghost attempt: %v", err)
	}
	mustNoErr(t, p.UpsertLoginAttempt(ctx, models.LoginAttempt{Username: "alice", Failures: 5, LockedUntil: time.Now().Add(15 * time.Minute), UpdatedAt: time.Now()}))
	mustNoErr(t, p.ClearLoginAttempt(ctx, "alice"))

	// Analytics
	osc, _ := p.OrderStatusCounts(ctx, 0, 0)
	if len(osc) == 0 {
		t.Fatalf("status counts: %+v", osc)
	}
	_, _ = p.OrderStatusCounts(ctx, fromUnix, toUnix)
	opd, _ := p.OrdersPerDay(ctx, 0, 0)
	if len(opd) == 0 {
		t.Fatal("orders per day empty")
	}
	_, _ = p.OrdersPerDay(ctx, fromUnix, toUnix)
	ssc, _ := p.SampleStatusCounts(ctx, 0, 0)
	if len(ssc) == 0 {
		t.Fatal("sample counts empty")
	}
	_, _ = p.SampleStatusCounts(ctx, fromUnix, toUnix)
	ar, _ := p.AbnormalReportRate(ctx, 0, 0)
	if ar.TotalMeasurements == 0 {
		t.Fatal("abnormal rate empty")
	}
	_, _ = p.AbnormalReportRate(ctx, fromUnix, toUnix)
	ek, _ := p.ExceptionCountsByKind(ctx)
	if ek["picking_timeout"] != 1 {
		t.Fatalf("exception kinds: %+v", ek)
	}

	// Audit with filters.
	mustNoErr(t, p.AppendAudit(ctx, models.AuditEntry{ID: "a1", At: time.Now(), Entity: "order", EntityID: "o1", Action: "create", WorkstationTime: time.Now()}))
	al, _ := p.ListAudit(ctx, "order", "o1", 10)
	if len(al) == 0 {
		t.Fatalf("audit list: %+v", al)
	}
	// No filters.
	_, _ = p.ListAudit(ctx, "", "", 0)

	// GrantsForUser
	grants, _ := p.GrantsForUser(ctx, "u1", "admin")
	if len(grants) == 0 {
		t.Fatal("expected grants")
	}

	// TestItems (new surface).
	items := []models.TestItem{
		{ID: "ti1", SampleID: "s1", TestCode: "GLU", Instructions: "fasting", CreatedAt: time.Now()},
	}
	if err := p.ReplaceTestItems(ctx, "s1", items); err != nil {
		t.Fatalf("replace test items: %v", err)
	}
	gotItems, _ := p.ListTestItems(ctx, "s1")
	if len(gotItems) != 1 || gotItems[0].TestCode != "GLU" {
		t.Fatalf("unexpected test items: %+v", gotItems)
	}
	if err := p.ReplaceTestItems(ctx, "s1", nil); err != nil {
		t.Fatal(err)
	}

	// SystemSettings (new surface) — missing key returns ErrNotFound,
	// Put+Get roundtrip, and List returns the full map.
	if _, err := p.GetSetting(ctx, "map.image.url"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := p.PutSetting(ctx, "map.image.url", "/static/map.png"); err != nil {
		t.Fatal(err)
	}
	if v, err := p.GetSetting(ctx, "map.image.url"); err != nil || v != "/static/map.png" {
		t.Fatalf("setting get: %q %v", v, err)
	}
	// Upsert: second put on the same key must not conflict.
	if err := p.PutSetting(ctx, "map.image.url", "/static/v2.png"); err != nil {
		t.Fatal(err)
	}
	settings, _ := p.ListSettings(ctx)
	if settings["map.image.url"] != "/static/v2.png" {
		t.Fatalf("expected updated value, got %v", settings)
	}
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
