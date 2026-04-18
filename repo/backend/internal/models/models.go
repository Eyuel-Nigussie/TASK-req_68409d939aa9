// Package models holds the shared domain types used across the store,
// handlers, and business-logic packages. Keeping them in one place avoids
// circular imports between lab/order/geo code and the persistence layer.
package models

import "time"

// Role enumerates the five product roles; an administrator may hold
// multiple roles by concatenating them on the user record, but for this
// product's scope each user has a single primary role.
type Role string

const (
	RoleFrontDesk  Role = "front_desk"
	RoleLabTech    Role = "lab_tech"
	RoleDispatch   Role = "dispatch"
	RoleAnalyst    Role = "analyst"
	RoleAdmin      Role = "admin"
)

// AllRoles is exposed for admin UIs.
var AllRoles = []Role{RoleFrontDesk, RoleLabTech, RoleDispatch, RoleAnalyst, RoleAdmin}

// User is an authenticated operator.
//
// MustRotatePassword is set when the account was provisioned with a
// shared/demo password (e.g. by SeedDemoUsers) and the operator has
// not yet chosen their own. When true, the auth layer refuses all API
// requests for the session except /api/auth/rotate-password,
// /api/auth/logout, and /api/auth/whoami so the published demo
// credential cannot be used in production without being rotated (L2).
type User struct {
	ID                 string
	Username           string
	Role               Role
	PasswordHash       string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Disabled           bool
	MustRotatePassword bool
}

// Customer represents a person who uses the lab or orders from fulfillment.
// Identifier and Street are encrypted at rest; in-memory they are plaintext.
type Customer struct {
	ID          string
	Name        string
	Identifier  string // e.g., patient/customer ID; encrypted in DB
	Street      string // encrypted in DB
	City        string
	State       string
	ZIP         string
	Phone       string
	Email       string
	Tags        []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AddressBookEntry is a favorite address saved for re-use.
type AddressBookEntry struct {
	ID         string
	OwnerID    string // user ID
	Label      string
	CustomerID string // optional customer reference
	Street     string
	City       string
	State      string
	ZIP        string
	Lat        float64
	Lng        float64
	CreatedAt  time.Time
}

// SavedFilter persists a canonicalized filter payload for a specific user.
type SavedFilter struct {
	ID        string
	OwnerID   string
	Name      string
	Payload   []byte // JSON encoding of filter.Filter
	Key       string // canonical key used for dedupe
	CreatedAt time.Time
}

// TestItem is a normalized per-sample requested test. The prompt
// explicitly lists "test items" as a persisted entity; this row maps
// one-to-one to each test the lab is asked to run on a sample, and
// carries optional technician-facing instructions.
type TestItem struct {
	ID           string
	SampleID     string
	TestCode     string
	Instructions string
	CreatedAt    time.Time
}

// Permission is an atom of authorization policy — e.g., "orders.write" or
// "admin.users". Administrators grant permissions to roles and/or to
// individual users; the authorization layer takes the union.
type Permission struct {
	ID          string
	Description string
}

// RolePermission is a single role → permission grant.
type RolePermission struct {
	Role         string
	PermissionID string
}

// LoginAttempt records the persisted failed-attempt counter for one
// username. Lockout is active when LockedUntil is in the future.
type LoginAttempt struct {
	Username    string
	Failures    int
	LockedUntil time.Time
	UpdatedAt   time.Time
}

// AuditEntry is the immutable record persisted for every state change.
//
// The prompt specifies the audit must capture "who acted, what changed, and
// the workstation time". We store both a server-assigned `At` (for strict
// ordering and to detect skewed clocks) and a client-asserted
// `WorkstationTime` (the moment the operator actually hit the button on
// their machine, captured from X-Workstation-Time). Auditors can reconcile
// the two when investigating timeline disputes.
type AuditEntry struct {
	ID              string
	At              time.Time
	WorkstationTime time.Time
	ActorID         string
	Workstation     string
	Entity          string // "order", "report", "user", etc.
	EntityID        string
	Action          string // "create", "transition", "correct", "login", etc.
	Before          []byte // JSON snapshot (nullable)
	After           []byte // JSON snapshot (nullable)
	Reason          string
}
