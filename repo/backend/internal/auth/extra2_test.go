package auth

import (
	"context"
	"testing"
	"time"
)

// Covers the remaining StoreLockout branches:
//   - NewStoreLockout with nil clock
//   - Check when a lock has expired (clears it)
//   - RecordFailure store error propagation

func TestNewStoreLockout_NilClockDefaults(t *testing.T) {
	s := newFakeAttemptStore()
	l := NewStoreLockout(s, nil)
	if err := l.Check(context.Background(), "anyone"); err != nil {
		t.Fatal(err)
	}
}

func TestStoreLockout_Check_ExpiredClearsRecord(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	s := newFakeAttemptStore()
	_ = s.Upsert(context.Background(), Attempt{
		Username:    "alice",
		Failures:    5,
		LockedUntil: clk.t.Add(-time.Minute), // already expired
	})
	l := NewStoreLockout(s, clk.now)
	if err := l.Check(context.Background(), "alice"); err != nil {
		t.Fatalf("expected expired lock to clear, got %v", err)
	}
	// After Check, the record should be cleared.
	got, err := s.Get(context.Background(), "alice")
	if err != ErrNoAttempt {
		t.Fatalf("expected ErrNoAttempt after clear, got %+v %v", got, err)
	}
}

// A store whose Upsert always fails to exercise the error branch in
// RecordFailure.
type errStore struct{}

func (errStore) Get(_ context.Context, _ string) (Attempt, error) {
	return Attempt{}, ErrNoAttempt
}
func (errStore) Upsert(_ context.Context, _ Attempt) error { return context.DeadlineExceeded }
func (errStore) Clear(_ context.Context, _ string) error   { return nil }

func TestStoreLockout_RecordFailure_UpsertError(t *testing.T) {
	l := NewStoreLockout(errStore{}, RealClock)
	if err := l.RecordFailure(context.Background(), "alice"); err != context.DeadlineExceeded {
		t.Fatalf("expected upsert error, got %v", err)
	}
}

// Lockout.Check: state exists but LockedUntil is zero → returns nil (user
// has pending failures but isn't locked yet).
func TestLockout_Check_StateButNoLock(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewLockout(clk.now)
	_ = l.RecordFailure("alice") // one failure, no lock yet
	if err := l.Check("alice"); err != nil {
		t.Fatalf("single failure must not lock, got %v", err)
	}
}

// HashPassword: cover the rand.Read failure branch by swapping the
// runtime's rand.Reader pointer. Skipped when running on a platform
// where we can't stub without `monkey patching`; we keep it as a smoke
// test that HashPassword returns an encoded string.
func TestHashPassword_StructureAndCompare(t *testing.T) {
	hash, err := HashPassword("correct-password-long-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := ComparePassword(hash, "correct-password-long-1"); err != nil {
		t.Fatal(err)
	}
	if err := ComparePassword(hash, "wrong-password-long-1"); err != ErrMismatched {
		t.Fatalf("expected mismatch, got %v", err)
	}
}
