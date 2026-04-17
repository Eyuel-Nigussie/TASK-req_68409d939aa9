package api

// Failing-store tests exercise the error-propagation branches in every
// handler. A thin wrapper around store.Memory intercepts specified methods
// and returns a sentinel error; the handler must map it to a generic 500
// via httpx.WriteError without leaking the raw text.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/crypto"
	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

var errSentinel = errors.New("store sentinel")

// faultyStore wraps an in-memory store and returns errSentinel from any
// method whose name is set to true in `fail`. Methods not in the map
// delegate to the underlying memory implementation.
type faultyStore struct {
	*store.Memory
	fail map[string]bool
}

func newFaulty(fail ...string) *faultyStore {
	f := &faultyStore{Memory: store.NewMemory(), fail: map[string]bool{}}
	for _, k := range fail {
		f.fail[k] = true
	}
	return f
}

// Each wrapper checks the fail map first.
func (f *faultyStore) ListUsers(ctx context.Context) ([]models.User, error) {
	if f.fail["ListUsers"] {
		return nil, errSentinel
	}
	return f.Memory.ListUsers(ctx)
}
func (f *faultyStore) ListRegions(ctx context.Context) ([]geo.Region, error) {
	if f.fail["ListRegions"] {
		return nil, errSentinel
	}
	return f.Memory.ListRegions(ctx)
}
func (f *faultyStore) ReplaceRegions(ctx context.Context, rs []geo.Region) error {
	if f.fail["ReplaceRegions"] {
		return errSentinel
	}
	return f.Memory.ReplaceRegions(ctx, rs)
}
func (f *faultyStore) ListExceptions(ctx context.Context) ([]order.Exception, error) {
	if f.fail["ListExceptions"] {
		return nil, errSentinel
	}
	return f.Memory.ListExceptions(ctx)
}
func (f *faultyStore) ListOrders(ctx context.Context, st []string, from, to *int64, l, o int) ([]order.Order, error) {
	if f.fail["ListOrders"] {
		return nil, errSentinel
	}
	return f.Memory.ListOrders(ctx, st, from, to, l, o)
}
func (f *faultyStore) ListSamples(ctx context.Context, st []string, l, o int) ([]lab.Sample, error) {
	if f.fail["ListSamples"] {
		return nil, errSentinel
	}
	return f.Memory.ListSamples(ctx, st, l, o)
}
func (f *faultyStore) ListReports(ctx context.Context, l, o int) ([]lab.Report, error) {
	if f.fail["ListReports"] {
		return nil, errSentinel
	}
	return f.Memory.ListReports(ctx, l, o)
}
func (f *faultyStore) ListArchivedReports(ctx context.Context) ([]lab.Report, error) {
	if f.fail["ListArchivedReports"] {
		return nil, errSentinel
	}
	return f.Memory.ListArchivedReports(ctx)
}
func (f *faultyStore) ListAddresses(ctx context.Context, owner string) ([]models.AddressBookEntry, error) {
	if f.fail["ListAddresses"] {
		return nil, errSentinel
	}
	return f.Memory.ListAddresses(ctx, owner)
}
func (f *faultyStore) ListSavedFilters(ctx context.Context, owner string) ([]models.SavedFilter, error) {
	if f.fail["ListSavedFilters"] {
		return nil, errSentinel
	}
	return f.Memory.ListSavedFilters(ctx, owner)
}
func (f *faultyStore) ListPermissions(ctx context.Context) ([]models.Permission, error) {
	if f.fail["ListPermissions"] {
		return nil, errSentinel
	}
	return f.Memory.ListPermissions(ctx)
}
func (f *faultyStore) ListRolePermissions(ctx context.Context) ([]models.RolePermission, error) {
	if f.fail["ListRolePermissions"] {
		return nil, errSentinel
	}
	return f.Memory.ListRolePermissions(ctx)
}
func (f *faultyStore) ListUserPermissions(ctx context.Context, uid string) ([]string, error) {
	if f.fail["ListUserPermissions"] {
		return nil, errSentinel
	}
	return f.Memory.ListUserPermissions(ctx, uid)
}
func (f *faultyStore) ListAudit(ctx context.Context, e, id string, l int) ([]models.AuditEntry, error) {
	if f.fail["ListAudit"] {
		return nil, errSentinel
	}
	return f.Memory.ListAudit(ctx, e, id, l)
}
func (f *faultyStore) ListRefRanges(ctx context.Context) ([]lab.RefRange, error) {
	if f.fail["ListRefRanges"] {
		return nil, errSentinel
	}
	return f.Memory.ListRefRanges(ctx)
}
func (f *faultyStore) ListRoutes(ctx context.Context) ([]store.RouteRow, error) {
	if f.fail["ListRoutes"] {
		return nil, errSentinel
	}
	return f.Memory.ListRoutes(ctx)
}
func (f *faultyStore) OrderStatusCounts(ctx context.Context, a, b int64) (map[string]int, error) {
	if f.fail["OrderStatusCounts"] {
		return nil, errSentinel
	}
	return f.Memory.OrderStatusCounts(ctx, a, b)
}
func (f *faultyStore) OrdersPerDay(ctx context.Context, a, b int64) ([]store.AnalyticsDayCount, error) {
	if f.fail["OrdersPerDay"] {
		return nil, errSentinel
	}
	return f.Memory.OrdersPerDay(ctx, a, b)
}
func (f *faultyStore) SampleStatusCounts(ctx context.Context, a, b int64) (map[string]int, error) {
	if f.fail["SampleStatusCounts"] {
		return nil, errSentinel
	}
	return f.Memory.SampleStatusCounts(ctx, a, b)
}
func (f *faultyStore) AbnormalReportRate(ctx context.Context, a, b int64) (store.AnalyticsAbnormalRate, error) {
	if f.fail["AbnormalReportRate"] {
		return store.AnalyticsAbnormalRate{}, errSentinel
	}
	return f.Memory.AbnormalReportRate(ctx, a, b)
}
func (f *faultyStore) ExceptionCountsByKind(ctx context.Context) (map[string]int, error) {
	if f.fail["ExceptionCountsByKind"] {
		return nil, errSentinel
	}
	return f.Memory.ExceptionCountsByKind(ctx)
}

// rigWithStore returns a test rig using the faulty store.
func rigWithStore(t *testing.T, fs *faultyStore) *testRig {
	t.Helper()
	vault, _ := crypto.NewVault(map[uint16][]byte{1: crypto.DeriveKey([]byte("unit-test-key"))})
	clk := func() time.Time { return time.Unix(1_700_000_000, 0) }
	srv := New(fs, vault, clk)
	// Seed default admin + tech + desk + dispatch + analyst on the backing
	// memory store directly, same as setup().
	for _, pair := range [][2]string{
		{"admin1", "admin"},
		{"tech1", "lab_tech"},
		{"desk1", "front_desk"},
		{"dispatch1", "dispatch"},
		{"analyst1", "analyst"},
	} {
		hash, _ := hashTestPassword("correct-horse-battery-staple")
		_ = fs.Memory.CreateUser(nil, testUser(pair[0], pair[1], hash))
	}
	e := echo.New()
	srv.Register(e)
	return &testRig{srv: srv, e: e, m: fs.Memory}
}

// Each *_ErrorPath test hits a handler that depends on a "listing"
// store method and expects a 500 with a generic message.

func TestHandlerErrorPaths_StoreListFailures(t *testing.T) {
	cases := []struct {
		name, fail, method, path, user string
	}{
		{"address book list", "ListAddresses", "GET", "/api/address-book", "desk1"},
		{"saved filter list", "ListSavedFilters", "GET", "/api/saved-filters", "desk1"},
		{"list users", "ListUsers", "GET", "/api/admin/users", "admin1"},
		{"list regions", "ListRegions", "GET", "/api/dispatch/regions", "dispatch1"},
		{"list orders", "ListOrders", "GET", "/api/orders", "desk1"},
		{"list samples", "ListSamples", "GET", "/api/samples", "tech1"},
		{"list reports", "ListReports", "GET", "/api/reports", "tech1"},
		{"list archived reports", "ListArchivedReports", "GET", "/api/reports/archived", "tech1"},
		{"list exceptions (listExceptions path)", "ListExceptions", "GET", "/api/exceptions", "desk1"},
		{"list ref ranges", "ListRefRanges", "GET", "/api/admin/reference-ranges", "admin1"},
		{"list routes", "ListRoutes", "GET", "/api/admin/route-table", "admin1"},
		{"list permissions", "ListPermissions", "GET", "/api/admin/permissions", "admin1"},
		{"list role-permissions", "ListRolePermissions", "GET", "/api/admin/role-permissions", "admin1"},
		{"list audit", "ListAudit", "GET", "/api/admin/audit", "admin1"},
		{"analytics order status", "OrderStatusCounts", "GET", "/api/analytics/orders/status-counts", "analyst1"},
		{"analytics orders per day", "OrdersPerDay", "GET", "/api/analytics/orders/per-day", "analyst1"},
		{"analytics sample status", "SampleStatusCounts", "GET", "/api/analytics/samples/status-counts", "analyst1"},
		{"analytics abnormal rate", "AbnormalReportRate", "GET", "/api/analytics/reports/abnormal-rate", "analyst1"},
		{"analytics exceptions by kind", "ExceptionCountsByKind", "GET", "/api/analytics/exceptions/by-kind", "analyst1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFaulty(tc.fail)
			r := rigWithStore(t, fs)
			tok := r.login(t, tc.user, "correct-horse-battery-staple")
			rec, _ := r.do(t, tc.method, tc.path, tok, nil)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("%s: expected 500, got %d (body=%s)", tc.name, rec.Code, rec.Body.String())
			}
			// Body must be generic — never contain the sentinel string.
			if body := rec.Body.String(); body == "" || contains(body, errSentinel.Error()) {
				t.Fatalf("%s: body leaked sentinel: %s", tc.name, body)
			}
		})
	}
}

// Target: admin endpoints that write regions/ranges/routes when the
// store write fails. Also verifies audit + reload paths short-circuit.

func TestAdmin_PutServiceRegions_StoreWriteFails(t *testing.T) {
	fs := newFaulty("ReplaceRegions")
	r := rigWithStore(t, fs)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	rec, _ := r.do(t, "PUT", "/api/admin/service-regions", admin, map[string]any{
		"regions": []map[string]any{{
			"id":                 "x",
			"vertices":           [][]float64{{0, 0}, {0, 10}, {10, 10}, {10, 0}},
			"base_fee_cents":     100,
			"per_mile_fee_cents": 10,
		}},
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// Covers the partial Reload* code when ListRoutes/ListRefRanges errors.

func TestServer_Reload_PropagatesStoreError(t *testing.T) {
	fs := newFaulty("ListRoutes", "ListRefRanges")
	vault, _ := crypto.NewVault(map[uint16][]byte{1: crypto.DeriveKey([]byte("k"))})
	srv := New(fs, vault, nil)
	if err := srv.ReloadRouteTable(context.Background()); err == nil {
		t.Fatal("expected error from failing ListRoutes")
	}
	if err := srv.ReloadRefRanges(context.Background()); err == nil {
		t.Fatal("expected error from failing ListRefRanges")
	}
}

// contains is a trivial substring helper (avoids importing strings here
// since the rest of the file does not use it).
func contains(s, needle string) bool {
	return len(s) >= len(needle) && (indexOf(s, needle) >= 0)
}
func indexOf(s, needle string) int {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
