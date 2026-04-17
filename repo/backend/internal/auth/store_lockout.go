package auth

import (
	"context"
	"errors"
	"sync"
	"time"
)

// StoreLockout is a persistent implementation of the failed-attempt
// tracker. It delegates state to an external store interface (typically
// Postgres) so lockouts survive a process restart — a compliance
// requirement that the in-memory Lockout cannot satisfy.
//
// The interface is kept minimal here to avoid importing the store package
// from auth (which would create a cycle). Callers wire up an adapter
// around store.LoginAttempts when constructing the Server.
type StoreLockout struct {
	mu   sync.Mutex
	s    AttemptStore
	now  Clock
	max  int
	dur  time.Duration
}

// AttemptStore is the minimal persistence surface used by StoreLockout.
type AttemptStore interface {
	Get(ctx context.Context, username string) (Attempt, error)
	Upsert(ctx context.Context, a Attempt) error
	Clear(ctx context.Context, username string) error
}

// Attempt is the persisted record.
type Attempt struct {
	Username    string
	Failures    int
	LockedUntil time.Time
}

// ErrNoAttempt is returned by AttemptStore implementations when no record
// exists for the username.
var ErrNoAttempt = errors.New("no attempt record")

// NewStoreLockout constructs a durable Lockout with the policy defaults.
func NewStoreLockout(s AttemptStore, now Clock) *StoreLockout {
	if now == nil {
		now = RealClock
	}
	return &StoreLockout{s: s, now: now, max: MaxFailedAttempts, dur: LockoutDuration}
}

// Check returns ErrAccountLocked if username is currently in lockout.
func (l *StoreLockout) Check(ctx context.Context, username string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, err := l.s.Get(ctx, username)
	if errors.Is(err, ErrNoAttempt) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.LockedUntil.IsZero() {
		return nil
	}
	if l.now().Before(a.LockedUntil) {
		return ErrAccountLocked
	}
	// Expired — clear so the next failure starts a fresh counter.
	_ = l.s.Clear(ctx, username)
	return nil
}

// RecordFailure increments the counter and locks on the 5th failure.
func (l *StoreLockout) RecordFailure(ctx context.Context, username string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, err := l.s.Get(ctx, username)
	if errors.Is(err, ErrNoAttempt) {
		a = Attempt{Username: username}
	} else if err != nil {
		return err
	}
	if !a.LockedUntil.IsZero() && l.now().Before(a.LockedUntil) {
		return ErrAccountLocked
	}
	a.Failures++
	if a.Failures >= l.max {
		a.LockedUntil = l.now().Add(l.dur)
	}
	if err := l.s.Upsert(ctx, a); err != nil {
		return err
	}
	if !a.LockedUntil.IsZero() && l.now().Before(a.LockedUntil) {
		return ErrAccountLocked
	}
	return nil
}

// RecordSuccess clears any tracked state for the username.
func (l *StoreLockout) RecordSuccess(ctx context.Context, username string) error {
	return l.s.Clear(ctx, username)
}
