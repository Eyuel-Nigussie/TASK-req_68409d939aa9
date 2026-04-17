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
	"github.com/eaglepoint/oops/backend/internal/models"
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

	// Authenticated routes.
	auth := e.Group("", httpx.RequireAuth(s.Sessions))
	auth.POST("/api/auth/logout", s.Logout)
	auth.GET("/api/auth/whoami", s.WhoAmI)

	// Customers: front desk + admin.
	fd := auth.Group("", httpx.RequireRoles(models.RoleFrontDesk, models.RoleAdmin, models.RoleAnalyst, models.RoleLabTech, models.RoleDispatch))
	fd.POST("/api/customers", s.CreateCustomer)
	fd.GET("/api/customers/:id", s.GetCustomer)
	fd.GET("/api/customers", s.SearchCustomers)
	fd.PATCH("/api/customers/:id", s.UpdateCustomer)
	fd.GET("/api/customers/by-address", s.CustomersByAddress)

	// Address book (per-user).
	auth.GET("/api/address-book", s.ListAddressBook)
	auth.POST("/api/address-book", s.CreateAddressBookEntry)
	auth.DELETE("/api/address-book/:id", s.DeleteAddressBookEntry)

	// Orders.
	orderRoles := auth.Group("", httpx.RequireRoles(models.RoleFrontDesk, models.RoleAdmin, models.RoleDispatch, models.RoleAnalyst))
	orderRoles.POST("/api/orders", s.CreateOrder)
	orderRoles.GET("/api/orders", s.ListOrders)
	orderRoles.POST("/api/orders/query", s.QueryOrders)
	orderRoles.GET("/api/orders/by-address", s.OrdersByAddress)
	orderRoles.GET("/api/orders/:id", s.GetOrder)
	orderRoles.POST("/api/orders/:id/transitions", s.TransitionOrder)
	orderRoles.GET("/api/exceptions", s.ListExceptions)
	orderRoles.POST("/api/orders/:id/out-of-stock/plan", s.PlanOutOfStock)
	orderRoles.POST("/api/orders/:id/inventory", s.UpdateInventory)

	// Samples & reports (lab tech + admin).
	lab := auth.Group("", httpx.RequireRoles(models.RoleLabTech, models.RoleAdmin, models.RoleAnalyst))
	lab.POST("/api/samples", s.CreateSample)
	lab.POST("/api/samples/:id/transitions", s.TransitionSample)
	lab.GET("/api/samples/:id", s.GetSample)
	lab.GET("/api/samples/:id/test-items", s.ListTestItems)
	lab.GET("/api/samples", s.ListSamples)
	lab.POST("/api/samples/:id/report", s.CreateReportDraft)
	lab.POST("/api/reports/:id/correct", s.CorrectReport)
	lab.POST("/api/reports/:id/archive", s.ArchiveReport)
	lab.GET("/api/reports", s.ListReports)
	lab.GET("/api/reports/archived", s.ListArchivedReports)
	lab.GET("/api/reports/search", s.SearchReports)
	lab.GET("/api/reports/:id", s.GetReport)

	// Dispatch (map pin + geofence + fee).
	dispatch := auth.Group("", httpx.RequireRoles(models.RoleDispatch, models.RoleAdmin))
	dispatch.POST("/api/dispatch/validate-pin", s.ValidatePin)
	dispatch.POST("/api/dispatch/fee-quote", s.QuoteFee)
	dispatch.GET("/api/dispatch/regions", s.ListRegions)
	// Map-config read is available to any dispatch-capable user so the
	// OfflineMap component can hydrate on first render.
	dispatch.GET("/api/dispatch/map-config", s.GetMapConfig)

	// Saved filters.
	auth.POST("/api/saved-filters", s.CreateSavedFilter)
	auth.GET("/api/saved-filters", s.ListSavedFilters)
	auth.DELETE("/api/saved-filters/:id", s.DeleteSavedFilter)

	// Global search suggestion endpoint.
	auth.GET("/api/search", s.GlobalSearch)

	// Admin (users + service regions + reference ranges + route table).
	admin := auth.Group("", httpx.RequireRoles(models.RoleAdmin))
	admin.POST("/api/admin/users", s.AdminCreateUser)
	admin.GET("/api/admin/users", s.AdminListUsers)
	admin.PATCH("/api/admin/users/:id", s.AdminUpdateUser)
	admin.PUT("/api/admin/service-regions", s.AdminPutServiceRegions)
	admin.GET("/api/admin/audit", s.AdminAudit)
	admin.GET("/api/admin/reference-ranges", s.AdminListRefRanges)
	admin.PUT("/api/admin/reference-ranges", s.AdminPutRefRanges)
	admin.GET("/api/admin/route-table", s.AdminListRoutes)
	admin.PUT("/api/admin/route-table", s.AdminPutRoutes)
	// Permission administration.
	admin.GET("/api/admin/permissions", s.AdminListPermissions)
	admin.GET("/api/admin/role-permissions", s.AdminListRolePermissions)
	admin.PUT("/api/admin/role-permissions/:role", s.AdminPutRolePermissions)
	admin.GET("/api/admin/users/:id/permissions", s.AdminListUserPermissions)
	admin.PUT("/api/admin/users/:id/permissions", s.AdminPutUserPermissions)

	// Analytics: gated by the admin-configurable "analytics.view" permission
	// so an administrator can grant or revoke access at runtime without
	// shipping code. This is what makes the permission system visible at
	// the HTTP layer.
	perm := permissionResolver{s: s.Store}
	analytics := auth.Group("", httpx.RequirePermission(perm, "analytics.view"))
	analytics.GET("/api/analytics/orders/status-counts", s.AnalyticsOrderStatus)
	analytics.GET("/api/analytics/orders/per-day", s.AnalyticsOrdersPerDay)
	analytics.GET("/api/analytics/samples/status-counts", s.AnalyticsSampleStatus)
	analytics.GET("/api/analytics/reports/abnormal-rate", s.AnalyticsAbnormalRate)
	analytics.GET("/api/analytics/exceptions/by-kind", s.AnalyticsExceptionsByKind)
	analytics.GET("/api/analytics/summary", s.AnalyticsSummary)

	// Bounded CSV export, gated by a dedicated permission so an admin
	// can grant "analyst can view but not export" by removing just this
	// grant. The handler itself enforces filter.MaxExportSize as a
	// second line of defense.
	exporters := auth.Group("", httpx.RequirePermission(perm, "orders.export"))
	exporters.POST("/api/exports/orders.csv", s.ExportOrdersCSV)

	// Map-image admin endpoint gated by the `admin.settings` permission.
	settings := auth.Group("", httpx.RequirePermission(perm, "admin.settings"))
	settings.PUT("/api/admin/map-config", s.AdminPutMapConfig)
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
