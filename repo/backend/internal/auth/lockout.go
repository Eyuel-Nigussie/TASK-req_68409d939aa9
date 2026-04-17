package auth

import (
	"errors"
	"sync"
	"time"
)

// Lockout policy constants.
const (
	MaxFailedAttempts = 5
	LockoutDuration   = 15 * time.Minute
)

// ErrAccountLocked is returned when an account is currently in lockout.
var ErrAccountLocked = errors.New("account is locked; try again later")

// Clock allows tests to override wall-clock time deterministically.
type Clock func() time.Time

// RealClock returns the current wall-clock time.
func RealClock() time.Time { return time.Now() }

// accountState tracks consecutive failed attempts and active lock expiry.
type accountState struct {
	failures   int
	lockedUntil time.Time
}

// Lockout is a goroutine-safe per-username failed-attempt tracker. It does
// not persist state across process restarts by design: the product runs on a
// single machine and a restart is rare enough that rebooting-to-unlock is
// acceptable; persisting locks would introduce a denial-of-service surface
// via dropped-attack sessions.
type Lockout struct {
	mu    sync.Mutex
	now   Clock
	state map[string]*accountState
	max   int
	dur   time.Duration
}

// NewLockout constructs a Lockout with defaults from the policy constants.
// Passing a nil clock uses wall time.
func NewLockout(now Clock) *Lockout {
	if now == nil {
		now = RealClock
	}
	return &Lockout{
		now:   now,
		state: make(map[string]*accountState),
		max:   MaxFailedAttempts,
		dur:   LockoutDuration,
	}
}

// Check returns ErrAccountLocked if username is in active lockout.
func (l *Lockout) Check(username string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.state[username]
	if !ok {
		return nil
	}
	if s.lockedUntil.IsZero() {
		return nil
	}
	if l.now().Before(s.lockedUntil) {
		return ErrAccountLocked
	}
	// Lock expired: clear failure counter so the user gets a fresh slate.
	delete(l.state, username)
	return nil
}

// RecordFailure increments the failure counter and locks the account when the
// threshold is crossed. It returns ErrAccountLocked if the attempt caused a
// new lock or if the account was already locked.
func (l *Lockout) RecordFailure(username string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.state[username]
	if s == nil {
		s = &accountState{}
		l.state[username] = s
	}
	if !s.lockedUntil.IsZero() && l.now().Before(s.lockedUntil) {
		return ErrAccountLocked
	}
	s.failures++
	if s.failures >= l.max {
		s.lockedUntil = l.now().Add(l.dur)
		return ErrAccountLocked
	}
	return nil
}

// RecordSuccess clears any tracked failures for the username.
func (l *Lockout) RecordSuccess(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.state, username)
}

// Snapshot returns observable state for diagnostics. Callers must not mutate
// the returned struct.
type LockoutSnapshot struct {
	Failures    int
	LockedUntil time.Time
}

// Snapshot returns the current state for a username, used by admin UIs.
func (l *Lockout) Snapshot(username string) LockoutSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.state[username]
	if !ok {
		return LockoutSnapshot{}
	}
	return LockoutSnapshot{Failures: s.failures, LockedUntil: s.lockedUntil}
}
