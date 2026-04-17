// Package store defines the persistence interface used by handlers and
// business logic. A Postgres-backed implementation powers production; an
// in-memory implementation backs unit and HTTP integration tests so tests
// can run on a laptop without a database.
package store

import (
	"context"
	"errors"

	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
)

// ErrNotFound is returned by any Getter when the target is absent. Handlers
// map this to HTTP 404.
var ErrNotFound = errors.New("not found")

// ErrConflict indicates an optimistic-concurrency or unique-constraint
// violation.
var ErrConflict = errors.New("conflict")

// Store is the full persistence contract. Interfaces are grouped by
// aggregate to keep each concern small and to make mocking easier.
type Store interface {
	Users
	Customers
	AddressBook
	ServiceAreas
	Orders
	Samples
	Reports
	SavedFilters
	Audit
	RefRanges
	Routes
	Permissions
	LoginAttempts
	Analytics
	TestItems
	SystemSettings
}

// Users handles authentication accounts.
type Users interface {
	CreateUser(ctx context.Context, u models.User) error
	GetUserByUsername(ctx context.Context, username string) (models.User, error)
	GetUserByID(ctx context.Context, id string) (models.User, error)
	ListUsers(ctx context.Context) ([]models.User, error)
	UpdateUser(ctx context.Context, u models.User) error
}

// Customers handles the customer directory.
type Customers interface {
	CreateCustomer(ctx context.Context, c models.Customer) error
	GetCustomer(ctx context.Context, id string) (models.Customer, error)
	UpdateCustomer(ctx context.Context, c models.Customer) error
	SearchCustomers(ctx context.Context, query string, limit int) ([]models.Customer, error)
	FindByAddress(ctx context.Context, street, city, zip string) ([]models.Customer, error)
}

// AddressBook handles saved addresses.
type AddressBook interface {
	CreateAddress(ctx context.Context, a models.AddressBookEntry) error
	ListAddresses(ctx context.Context, ownerID string) ([]models.AddressBookEntry, error)
	DeleteAddress(ctx context.Context, ownerID, id string) error
}

// ServiceAreas handles the polygons and fee tables used for dispatch.
type ServiceAreas interface {
	ListRegions(ctx context.Context) ([]geo.Region, error)
	ReplaceRegions(ctx context.Context, regions []geo.Region) error
}

// Orders handles fulfillment order CRUD and event persistence.
type Orders interface {
	CreateOrder(ctx context.Context, o order.Order) error
	GetOrder(ctx context.Context, id string) (order.Order, error)
	UpdateOrder(ctx context.Context, o order.Order) error
	AppendOrderEvent(ctx context.Context, ev order.Event) error
	ListOrders(ctx context.Context, status []string, from, to *int64, limit, offset int) ([]order.Order, error)
	QueryOrders(ctx context.Context, q OrderQuery) ([]order.Order, int, error)
	OrdersByAddress(ctx context.Context, city, zip string) ([]order.Order, error)
	ListExceptions(ctx context.Context) ([]order.Exception, error)
	PutException(ctx context.Context, ex order.Exception) error
}

// OrderQuery is the executable form of a validated filter.Filter for the
// order entity. Unlike the UI payload, every field is already parsed.
type OrderQuery struct {
	Keyword     string
	Statuses    []string
	Tags        []string
	Priority    string
	StartUnix   *int64
	EndUnix     *int64
	MinCents    *int
	MaxCents    *int
	SortBy      string
	SortDesc    bool
	Limit       int
	Offset      int
}

// Samples handles lab specimen records.
type Samples interface {
	CreateSample(ctx context.Context, s lab.Sample) error
	GetSample(ctx context.Context, id string) (lab.Sample, error)
	UpdateSample(ctx context.Context, s lab.Sample) error
	ListSamples(ctx context.Context, status []string, limit, offset int) ([]lab.Sample, error)
}

// Reports handles report versions.
type Reports interface {
	CreateReport(ctx context.Context, r lab.Report) error
	GetReport(ctx context.Context, id string) (lab.Report, error)
	LatestReportForSample(ctx context.Context, sampleID string) (lab.Report, error)
	UpdateReport(ctx context.Context, r lab.Report) error
	ReplaceWithCorrection(ctx context.Context, old lab.Report, next lab.Report) error
	SearchReports(ctx context.Context, query string, limit int) ([]lab.Report, error)
	ListReports(ctx context.Context, limit, offset int) ([]lab.Report, error)
	ListArchivedReports(ctx context.Context) ([]lab.Report, error)
}

// SavedFilters handles per-user saved filter library entries.
type SavedFilters interface {
	CreateSavedFilter(ctx context.Context, f models.SavedFilter) error
	ListSavedFilters(ctx context.Context, ownerID string) ([]models.SavedFilter, error)
	DeleteSavedFilter(ctx context.Context, ownerID, id string) error
}

// Audit handles the immutable audit log.
type Audit interface {
	AppendAudit(ctx context.Context, e models.AuditEntry) error
	ListAudit(ctx context.Context, entity, entityID string, limit int) ([]models.AuditEntry, error)
}

// RefRanges manages the reference-range dictionary used for lab flagging.
// Entries are versioned implicitly by UpdatedAt; the admin UI replaces the
// whole set atomically.
type RefRanges interface {
	ListRefRanges(ctx context.Context) ([]lab.RefRange, error)
	ReplaceRefRanges(ctx context.Context, rr []lab.RefRange) error
}

// Routes persists the preloaded road-distance matrix used for fee quotes.
type Routes interface {
	ListRoutes(ctx context.Context) ([]RouteRow, error)
	ReplaceRoutes(ctx context.Context, rows []RouteRow) error
}

// RouteRow is a single persisted entry of the route table.
type RouteRow struct {
	FromID string
	ToID   string
	Miles  float64
}

// Permissions exposes admin-configurable authorization policy. Grants are
// additive: a user is permitted to do X iff (user_permissions contains X)
// OR (role_permissions contains X for the user's role).
type Permissions interface {
	// ListPermissions returns the full catalog of known permissions.
	ListPermissions(ctx context.Context) ([]models.Permission, error)
	// UpsertPermission adds or updates a single permission in the catalog.
	UpsertPermission(ctx context.Context, p models.Permission) error
	// ListRolePermissions returns every (role -> permission) grant.
	ListRolePermissions(ctx context.Context) ([]models.RolePermission, error)
	// SetRolePermissions replaces the grant set for a single role atomically.
	SetRolePermissions(ctx context.Context, role string, permissionIDs []string) error
	// GrantsForUser returns the full set of permission IDs for a user,
	// combining role and individual grants.
	GrantsForUser(ctx context.Context, userID string, role string) ([]string, error)
	// ListUserPermissions returns just the individual grants on a user.
	ListUserPermissions(ctx context.Context, userID string) ([]string, error)
	// SetUserPermissions replaces the individual grants on a user.
	SetUserPermissions(ctx context.Context, userID string, permissionIDs []string) error
}

// LoginAttempts persists the failed-attempt counter so a process restart
// does not reset the 5-fail/15-minute lockout window (compliance control).
type LoginAttempts interface {
	GetLoginAttempt(ctx context.Context, username string) (models.LoginAttempt, error)
	UpsertLoginAttempt(ctx context.Context, a models.LoginAttempt) error
	ClearLoginAttempt(ctx context.Context, username string) error
}

// Analytics is a read-only interface producing aggregations for the
// analyst workspace. Every method returns deterministic, bounded result
// sets so analysts cannot accidentally dump the entire dataset.
type Analytics interface {
	OrderStatusCounts(ctx context.Context, fromUnix, toUnix int64) (map[string]int, error)
	OrdersPerDay(ctx context.Context, fromUnix, toUnix int64) ([]AnalyticsDayCount, error)
	SampleStatusCounts(ctx context.Context, fromUnix, toUnix int64) (map[string]int, error)
	AbnormalReportRate(ctx context.Context, fromUnix, toUnix int64) (AnalyticsAbnormalRate, error)
	ExceptionCountsByKind(ctx context.Context) (map[string]int, error)
}

// AnalyticsDayCount is one bucket of a daily time series.
type AnalyticsDayCount struct {
	Day   string // "YYYY-MM-DD" (UTC)
	Count int
}

// AnalyticsAbnormalRate is the "what fraction of results are abnormal" KPI.
type AnalyticsAbnormalRate struct {
	TotalMeasurements    int
	AbnormalMeasurements int
	Rate                 float64 // in [0,1]
}

// TestItems is the normalized "test items" entity the prompt names
// alongside samples. Each sample has one row per requested test with
// optional technician instructions. Samples.TestCodes is kept as a
// denormalized array column for fast filter queries.
type TestItems interface {
	ListTestItems(ctx context.Context, sampleID string) ([]models.TestItem, error)
	ReplaceTestItems(ctx context.Context, sampleID string, items []models.TestItem) error
}

// SystemSettings is a small key/value store for deployment-wide
// configuration. The UI reads values here at runtime (e.g., the
// service-area map image URL).
type SystemSettings interface {
	GetSetting(ctx context.Context, key string) (string, error)
	PutSetting(ctx context.Context, key, value string) error
	ListSettings(ctx context.Context) (map[string]string, error)
}
