// Package audit wraps the append-only audit log so every state change
// records who did what, when, and from which workstation. Callers never
// construct AuditEntry records directly; they call Log and supply only the
// operational context.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/store"
)

// Entity is the typed name used in the `entity` column of the audit
// log. Using constants instead of free-form strings (L6) prevents a
// typo in one handler from silently splitting an entity's history in
// the audit table.
type Entity string

const (
	EntityUser            Entity = "user"
	EntityCustomer        Entity = "customer"
	EntityOrder           Entity = "order"
	EntityOrderException  Entity = "order_exception"
	EntitySample          Entity = "sample"
	EntityReport          Entity = "report"
	EntityAddressBook     Entity = "address_book"
	EntitySavedFilter     Entity = "saved_filter"
	EntityServiceRegions  Entity = "service_regions"
	EntityReferenceRanges Entity = "reference_ranges"
	EntityRouteTable      Entity = "route_table"
	EntityRolePermissions Entity = "role_permissions"
	EntityUserPermissions Entity = "user_permissions"
	EntitySystemSettings  Entity = "system_settings"
)

// Logger writes audit entries via the store.
type Logger struct {
	s   store.Audit
	now auth.Clock
	// onError is invoked whenever AppendAudit fails. Default is a
	// log.Printf at the process stderr stream so a silently-dropped
	// audit row at least surfaces in `docker compose logs backend`.
	// Tests can inject a custom sink to assert the branch runs.
	onError func(entity, entityID, action, actor string, err error)
}

// New returns a Logger backed by the given store. `now` may be nil; tests
// can inject a deterministic clock.
func New(s store.Audit, now auth.Clock) *Logger {
	if now == nil {
		now = auth.RealClock
	}
	return &Logger{
		s:   s,
		now: now,
		onError: func(entity, entityID, action, actor string, err error) {
			log.Printf("[audit-drop] entity=%s id=%s action=%s actor=%s err=%v",
				entity, entityID, action, actor, err)
		},
	}
}

// SetErrorSink replaces the default error handler. Intended for tests
// that need to capture the "audit write failed" event.
func (l *Logger) SetErrorSink(fn func(entity, entityID, action, actor string, err error)) {
	if fn != nil {
		l.onError = fn
	}
}

// Log persists an audit entry. `before` and `after` may be nil. The
// `workstationTime` argument is the operator-local clock value captured
// from the X-Workstation-Time header; when zero the server omits it from
// the record (but never silently substitutes server time).
//
// The `entity` parameter is typed (Entity) so callers cannot accidentally
// split an entity's history with a typo — every call site must pass one
// of the named constants above (L6).
func (l *Logger) Log(ctx context.Context, actor, workstation string, workstationTime time.Time, entity Entity, entityID, action, reason string, before, after any) error {
	var (
		bb, ab []byte
		err    error
	)
	if before != nil {
		bb, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("audit marshal before: %w", err)
		}
	}
	if after != nil {
		ab, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("audit marshal after: %w", err)
		}
	}
	e := models.AuditEntry{
		ID:              newID(l.now()),
		At:              l.now(),
		WorkstationTime: workstationTime,
		ActorID:         actor,
		Workstation:     workstation,
		Entity:          string(entity),
		EntityID:        entityID,
		Action:          action,
		Before:          bb,
		After:           ab,
		Reason:          reason,
	}
	if err := l.s.AppendAudit(ctx, e); err != nil {
		// Callers historically discard this error with `_ =`, which
		// made an AppendAudit outage invisible. Route the drop
		// through onError so an operator can spot it in logs even
		// when the HTTP handler ignores the return value.
		if l.onError != nil {
			l.onError(string(entity), entityID, action, actor, err)
		}
		return err
	}
	return nil
}

// List returns audit entries for an entity, bounded by limit. Passing zero
// for limit returns all entries. Intended for admin UIs and investigations.
func (l *Logger) List(ctx context.Context, entity, entityID string, limit int) ([]models.AuditEntry, error) {
	return l.s.ListAudit(ctx, entity, entityID, limit)
}

// newID returns a monotonic, human-readable audit ID with a 4-character
// random suffix. The DB-backed implementation replaces this with a
// gen_random_uuid() default but keeping a generator here makes tests
// independent of the DB.
func newID(at time.Time) string {
	return fmt.Sprintf("a_%d_%04d", at.UnixNano(), (at.UnixNano()/1000)%10000)
}
