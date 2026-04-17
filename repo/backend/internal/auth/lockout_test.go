package auth

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestLockout_CleanAccountAllowed(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewLockout(clk.now)
	if err := l.Check("alice"); err != nil {
		t.Fatalf("fresh account should not be locked: %v", err)
	}
}

func TestLockout_LocksAfterFiveFailures(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewLockout(clk.now)
	for i := 0; i < 4; i++ {
		if err := l.RecordFailure("alice"); err != nil {
			t.Fatalf("failure %d should not lock: %v", i+1, err)
		}
	}
	if err := l.RecordFailure("alice"); err != ErrAccountLocked {
		t.Fatalf("5th failure should lock: got %v", err)
	}
	if err := l.Check("alice"); err != ErrAccountLocked {
		t.Fatalf("Check after lock should return ErrAccountLocked, got %v", err)
	}
}

func TestLockout_AutoUnlockAfter15Min(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewLockout(clk.now)
	for i := 0; i < 5; i++ {
		_ = l.RecordFailure("alice")
	}
	// 14m59s later still locked.
	clk.advance(14*time.Minute + 59*time.Second)
	if err := l.Check("alice"); err != ErrAccountLocked {
		t.Fatalf("still within lockout window: got %v", err)
	}
	// 15m exactly - boundary. We require that at exactly-expiry the lock
	// has been released; this matches user expectation reading the policy.
	clk.advance(time.Second)
	if err := l.Check("alice"); err != nil {
		t.Fatalf("after 15m the lock should clear: %v", err)
	}
}

func TestLockout_SuccessResetsCounter(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewLockout(clk.now)
	_ = l.RecordFailure("alice")
	_ = l.RecordFailure("alice")
	l.RecordSuccess("alice")
	// Four more failures must not lock: the counter is back at zero.
	for i := 0; i < 4; i++ {
		if err := l.RecordFailure("alice"); err != nil {
			t.Fatalf("failure %d should not lock after reset: %v", i+1, err)
		}
	}
}

func TestLockout_PerUsernameIsolation(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewLockout(clk.now)
	for i := 0; i < 5; i++ {
		_ = l.RecordFailure("alice")
	}
	if err := l.Check("bob"); err != nil {
		t.Fatalf("bob should not be locked: %v", err)
	}
}

func TestLockout_SnapshotReflectsState(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewLockout(clk.now)
	_ = l.RecordFailure("alice")
	_ = l.RecordFailure("alice")
	snap := l.Snapshot("alice")
	if snap.Failures != 2 {
		t.Fatalf("expected 2 failures, got %d", snap.Failures)
	}
	if !snap.LockedUntil.IsZero() {
		t.Fatalf("should not be locked yet, got %v", snap.LockedUntil)
	}
}
