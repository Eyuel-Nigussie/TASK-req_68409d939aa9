// Package api wires the HTTP router: handlers, middleware, and the
// dependency graph of stores and services.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strings"

	"github.com/eaglepoint/oops/backend/internal/audit"
	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/crypto"
	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Server bundles the HTTP dependencies. All durable state is held by the
// store; the Server itself is stateless beyond the in-memory session cache.
type Server struct {
	Store      store.Store
	Vault      *crypto.Vault
	Sessions   *auth.SessionStore
	Lockout    *auth.StoreLockout // persistent lockout backed by store
	Audit      *audit.Logger
	Clock      auth.Clock
	RouteTable *geo.RouteTable
	// ranges is populated by AdminPutRefRanges, or seeded at startup.
	ranges *lab.RangeSet
}

// New constructs a Server with the given dependencies.
func New(s store.Store, v *crypto.Vault, clk auth.Clock) *Server {
	if clk == nil {
		clk = auth.RealClock
	}
	srv := &Server{
		Store:      s,
		Vault:      v,
		Sessions:   auth.NewSessionStore(0, clk),
		Lockout:    auth.NewStoreLockout(newLoginAttemptAdapter(s), clk),
		Audit:      audit.New(s, clk),
		Clock:      clk,
		RouteTable: geo.NewRouteTable(),
		ranges:     lab.NewRangeSet(),
	}
	// Best-effort load of route table and reference ranges from persistent
	// storage. Failures are logged but non-fatal so an operator can still
	// boot an empty system and populate via the admin API.
	if err := srv.ReloadRouteTable(context.Background()); err != nil {
		log.Printf("route table load: %v", err)
	}
	if err := srv.ReloadRefRanges(context.Background()); err != nil {
		log.Printf("ref ranges load: %v", err)
	}
	return srv
}

// ReloadRouteTable hydrates the in-process RouteTable from the persisted
// route rows. Called at startup and after admin edits.
func (s *Server) ReloadRouteTable(ctx context.Context) error {
	rows, err := s.Store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	rt := geo.NewRouteTable()
	for _, r := range rows {
		rt.Add(r.FromID, r.ToID, r.Miles)
	}
	s.RouteTable = rt
	return nil
}

// ReloadRefRanges hydrates the in-process RangeSet from persisted rows.
func (s *Server) ReloadRefRanges(ctx context.Context) error {
	rows, err := s.Store.ListRefRanges(ctx)
	if err != nil {
		return err
	}
	rs := lab.NewRangeSet()
	for _, r := range rows {
		rs.Add(r)
	}
	s.ranges = rs
	return nil
}

// Register installs all route handlers on the Echo instance.
func (s *Server) Register(e *echo.Echo) {
	e.Use(middleware.Recover())
	e.Use(httpx.StructuredLogging(nil))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		// Default origins target the bundled SPA on the LAN. Operators
		// can override by setting ALLOWED_ORIGINS="http://host:port,..."
		// at deploy time. A wildcard is intentionally NOT the default
		// so a stray cross-origin script cannot ride a session token
		// stored in localStorage via an authenticated fetch.
		AllowOrigins:  parseAllowedOrigins(),
		AllowMethods:  []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:  []string{"Authorization", "Content-Type", "X-Workstation", "X-Workstation-Time", "X-Request-ID"},
		ExposeHeaders: []string{"X-Request-ID"},
	}))
	e.Use(httpx.SecurityHeaders())

	// Public routes (auth only).
	e.POST("/api/auth/login", s.Login)
	e.GET("/api/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	// Authenticated routes. Renamed from `auth` to `authGroup` to stop
	// shadowing the imported `auth` package (A10).
	authGroup := e.Group("", httpx.RequireAuth(s.Sessions))
	authGroup.POST("/api/auth/logout", s.Logout)
	authGroup.GET("/api/auth/whoami", s.WhoAmI)

	// All mutations are gated by a single permission pulled from the
	// admin-configurable catalog. This closes A1-A3: the Analyst role
	// now has only `*.read` / `analytics.view` / `orders.export` grants,
	// so every `customers.write` / `orders.write` / `samples.write` /
	// `reports.*` route below is denied for Analyst (and for any other
	// role whose catalog entry lacks the permission). A dedicated role
	// group is NOT layered in front of RequirePermission on mutations
	// because the permission check is already the tighter constraint —
	// adding a coarse role filter would only drift out of sync.
	perm := permissionResolver{s: s.Store}
	needs := func(permID string) echo.MiddlewareFunc {
		return httpx.RequirePermission(perm, permID)
	}

	// Customers — reads gated by customers.read, writes by customers.write.
	authGroup.POST("/api/customers", s.CreateCustomer, needs("customers.write"))
	authGroup.GET("/api/customers/:id", s.GetCustomer, needs("customers.read"))
	authGroup.GET("/api/customers", s.SearchCustomers, needs("customers.read"))
	authGroup.PATCH("/api/customers/:id", s.UpdateCustomer, needs("customers.write"))
	authGroup.GET("/api/customers/by-address", s.CustomersByAddress, needs("customers.read"))

	// Address book is strictly per-user (owner-scoped at the store layer)
	// so no extra permission gate is needed beyond authentication.
	authGroup.GET("/api/address-book", s.ListAddressBook)
	authGroup.POST("/api/address-book", s.CreateAddressBookEntry)
	authGroup.DELETE("/api/address-book/:id", s.DeleteAddressBookEntry)

	// Orders.
	authGroup.POST("/api/orders", s.CreateOrder, needs("orders.write"))
	authGroup.GET("/api/orders", s.ListOrders, needs("orders.read"))
	authGroup.POST("/api/orders/query", s.QueryOrders, needs("orders.read"))
	authGroup.GET("/api/orders/by-address", s.OrdersByAddress, needs("orders.read"))
	authGroup.GET("/api/orders/:id", s.GetOrder, needs("orders.read"))
	authGroup.POST("/api/orders/:id/transitions", s.TransitionOrder, needs("orders.write"))
	authGroup.GET("/api/exceptions", s.ListExceptions, needs("orders.read"))
	authGroup.POST("/api/orders/:id/out-of-stock/plan", s.PlanOutOfStock, needs("orders.write"))
	authGroup.POST("/api/orders/:id/inventory", s.UpdateInventory, needs("orders.write"))

	// Samples.
	authGroup.POST("/api/samples", s.CreateSample, needs("samples.write"))
	authGroup.POST("/api/samples/:id/transitions", s.TransitionSample, needs("samples.write"))
	authGroup.GET("/api/samples/:id", s.GetSample, needs("samples.read"))
	authGroup.GET("/api/samples/:id/test-items", s.ListTestItems, needs("samples.read"))
	authGroup.GET("/api/samples", s.ListSamples, needs("samples.read"))

	// Reports.
	authGroup.POST("/api/samples/:id/report", s.CreateReportDraft, needs("reports.write"))
	authGroup.POST("/api/reports/:id/correct", s.CorrectReport, needs("reports.write"))
	authGroup.POST("/api/reports/:id/archive", s.ArchiveReport, needs("reports.archive"))
	authGroup.GET("/api/reports", s.ListReports, needs("reports.read"))
	authGroup.GET("/api/reports/archived", s.ListArchivedReports, needs("reports.read"))
	authGroup.GET("/api/reports/search", s.SearchReports, needs("reports.read"))
	authGroup.GET("/api/reports/:id", s.GetReport, needs("reports.read"))

	// Dispatch (map pin + geofence + fee). Validate and read ops share
	// `dispatch.validate`; write operations (regions/routes) sit under
	// `dispatch.configure` on the admin block below.
	authGroup.POST("/api/dispatch/validate-pin", s.ValidatePin, needs("dispatch.validate"))
	authGroup.POST("/api/dispatch/fee-quote", s.QuoteFee, needs("dispatch.validate"))
	authGroup.GET("/api/dispatch/regions", s.ListRegions, needs("dispatch.validate"))
	authGroup.GET("/api/dispatch/map-config", s.GetMapConfig, needs("dispatch.validate"))

	// Saved filters (owner-scoped; no permission beyond auth).
	authGroup.POST("/api/saved-filters", s.CreateSavedFilter)
	authGroup.GET("/api/saved-filters", s.ListSavedFilters)
	authGroup.DELETE("/api/saved-filters/:id", s.DeleteSavedFilter)

	// Global search suggestion endpoint (read-only, authed only).
	authGroup.GET("/api/search", s.GlobalSearch)

	// Admin endpoints — each gated by a dedicated permission so an
	// operator can hand out "read the audit log" without handing out
	// "edit users". admin role has all of them via the seed.
	authGroup.POST("/api/admin/users", s.AdminCreateUser, needs("admin.users"))
	authGroup.GET("/api/admin/users", s.AdminListUsers, needs("admin.users"))
	authGroup.PATCH("/api/admin/users/:id", s.AdminUpdateUser, needs("admin.users"))
	authGroup.GET("/api/admin/audit", s.AdminAudit, needs("admin.audit"))
	authGroup.PUT("/api/admin/service-regions", s.AdminPutServiceRegions, needs("dispatch.configure"))
	authGroup.GET("/api/admin/reference-ranges", s.AdminListRefRanges, needs("admin.reference"))
	authGroup.PUT("/api/admin/reference-ranges", s.AdminPutRefRanges, needs("admin.reference"))
	authGroup.GET("/api/admin/route-table", s.AdminListRoutes, needs("dispatch.configure"))
	authGroup.PUT("/api/admin/route-table", s.AdminPutRoutes, needs("dispatch.configure"))
	authGroup.GET("/api/admin/permissions", s.AdminListPermissions, needs("admin.users"))
	authGroup.GET("/api/admin/role-permissions", s.AdminListRolePermissions, needs("admin.users"))
	authGroup.PUT("/api/admin/role-permissions/:role", s.AdminPutRolePermissions, needs("admin.users"))
	authGroup.GET("/api/admin/users/:id/permissions", s.AdminListUserPermissions, needs("admin.users"))
	authGroup.PUT("/api/admin/users/:id/permissions", s.AdminPutUserPermissions, needs("admin.users"))
	authGroup.PUT("/api/admin/map-config", s.AdminPutMapConfig, needs("admin.settings"))

	// Analytics, under the admin-configurable "analytics.view" permission.
	authGroup.GET("/api/analytics/orders/status-counts", s.AnalyticsOrderStatus, needs("analytics.view"))
	authGroup.GET("/api/analytics/orders/per-day", s.AnalyticsOrdersPerDay, needs("analytics.view"))
	authGroup.GET("/api/analytics/samples/status-counts", s.AnalyticsSampleStatus, needs("analytics.view"))
	authGroup.GET("/api/analytics/reports/abnormal-rate", s.AnalyticsAbnormalRate, needs("analytics.view"))
	authGroup.GET("/api/analytics/exceptions/by-kind", s.AnalyticsExceptionsByKind, needs("analytics.view"))
	authGroup.GET("/api/analytics/summary", s.AnalyticsSummary, needs("analytics.view"))

	// Bounded CSV export — separately granted so an operator can have
	// `analytics.view` without `orders.export`.
	authGroup.POST("/api/exports/orders.csv", s.ExportOrdersCSV, needs("orders.export"))
}

// permissionResolver adapts store.Permissions to httpx.PermissionResolver
// so the middleware doesn't need to import the store package.
type permissionResolver struct{ s store.Store }

func (p permissionResolver) GrantsForUser(ctx context.Context, userID, role string) ([]string, error) {
	return p.s.GrantsForUser(ctx, userID, role)
}

// parseAllowedOrigins returns the allowed CORS origins. Precedence:
//   1. ALLOWED_ORIGINS env var — comma-separated, trimmed, empty values
//      skipped. Setting the literal value "*" re-enables the wildcard
//      for operators who explicitly want it.
//   2. Otherwise, a safe default that covers the bundled SPA on the
//      LAN on its default ports.
func parseAllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
	if raw == "" {
		return []string{
			"http://localhost:3000",
			"http://127.0.0.1:3000",
		}
	}
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"http://localhost:3000"}
	}
	return out
}

// newID returns a short, opaque identifier used for non-security-critical
// object IDs (orders, samples, reports). Uses 12 random bytes → 24 hex chars.
func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
