package auth

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeAttemptStore is an in-memory AttemptStore used to exercise the
// StoreLockout logic without a Postgres dependency. It is intentionally
// separate from the package's memory.Lockout so the test covers the
// store-backed branch specifically.
type fakeAttemptStore struct {
	mu sync.Mutex
	m  map[string]Attempt
}

func newFakeAttemptStore() *fakeAttemptStore {
	return &fakeAttemptStore{m: map[string]Attempt{}}
}

func (f *fakeAttemptStore) Get(_ context.Context, u string) (Attempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.m[u]
	if !ok {
		return Attempt{Username: u}, ErrNoAttempt
	}
	return a, nil
}

func (f *fakeAttemptStore) Upsert(_ context.Context, a Attempt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[a.Username] = a
	return nil
}

func (f *fakeAttemptStore) Clear(_ context.Context, u string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, u)
	return nil
}

func TestStoreLockout_LocksAfterFiveFailures(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	s := newFakeAttemptStore()
	l := NewStoreLockout(s, clk.now)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if err := l.RecordFailure(ctx, "alice"); err != nil {
			t.Fatalf("fail %d returned %v", i+1, err)
		}
	}
	if err := l.RecordFailure(ctx, "alice"); err != ErrAccountLocked {
		t.Fatalf("5th failure should lock: got %v", err)
	}
	if err := l.Check(ctx, "alice"); err != ErrAccountLocked {
		t.Fatalf("Check during lockout: got %v", err)
	}
}

func TestStoreLockout_SurvivesRestart(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	s := newFakeAttemptStore()
	l1 := NewStoreLockout(s, clk.now)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = l1.RecordFailure(ctx, "alice")
	}
	// Build a brand-new lockout instance pointed at the same store to
	// simulate a process restart. The lock must still be active because
	// the attempt record is persisted.
	l2 := NewStoreLockout(s, clk.now)
	if err := l2.Check(ctx, "alice"); err != ErrAccountLocked {
		t.Fatalf("expected lock after restart, got %v", err)
	}
}

func TestStoreLockout_AutoUnlocksAfter15Min(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	s := newFakeAttemptStore()
	l := NewStoreLockout(s, clk.now)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = l.RecordFailure(ctx, "alice")
	}
	clk.advance(14*time.Minute + 59*time.Second)
	if err := l.Check(ctx, "alice"); err != ErrAccountLocked {
		t.Fatalf("still locked in window, got %v", err)
	}
	clk.advance(time.Second)
	if err := l.Check(ctx, "alice"); err != nil {
		t.Fatalf("expected unlock after 15min, got %v", err)
	}
}

func TestStoreLockout_SuccessClears(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	s := newFakeAttemptStore()
	l := NewStoreLockout(s, clk.now)
	ctx := context.Background()
	_ = l.RecordFailure(ctx, "alice")
	_ = l.RecordFailure(ctx, "alice")
	if err := l.RecordSuccess(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	// After success, 4 more fails must not lock (fresh counter).
	for i := 0; i < 4; i++ {
		if err := l.RecordFailure(ctx, "alice"); err != nil {
			t.Fatalf("unexpected lock on fail %d: %v", i+1, err)
		}
	}
}
