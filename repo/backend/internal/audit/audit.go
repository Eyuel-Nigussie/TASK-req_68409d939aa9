// Package audit wraps the append-only audit log so every state change
// records who did what, when, and from which workstation. Callers never
// construct AuditEntry records directly; they call Log and supply only the
// operational context.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/store"
)

// Logger writes audit entries via the store.
type Logger struct {
	s   store.Audit
	now auth.Clock
}

// New returns a Logger backed by the given store. `now` may be nil; tests
// can inject a deterministic clock.
func New(s store.Audit, now auth.Clock) *Logger {
	if now == nil {
		now = auth.RealClock
	}
	return &Logger{s: s, now: now}
}

// Log persists an audit entry. `before` and `after` may be nil. The
// `workstationTime` argument is the operator-local clock value captured
// from the X-Workstation-Time header; when zero the server omits it from
// the record (but never silently substitutes server time).
func (l *Logger) Log(ctx context.Context, actor, workstation string, workstationTime time.Time, entity, entityID, action, reason string, before, after any) error {
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
		Entity:          entity,
		EntityID:        entityID,
		Action:          action,
		Before:          bb,
		After:           ab,
		Reason:          reason,
	}
	return l.s.AppendAudit(ctx, e)
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
