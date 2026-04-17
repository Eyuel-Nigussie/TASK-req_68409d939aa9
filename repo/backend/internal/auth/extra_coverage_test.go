package auth

import (
	"strings"
	"testing"
	"time"
)

func TestNewLockout_NilClockDefaultsToRealClock(t *testing.T) {
	l := NewLockout(nil)
	// Should not panic and Check/RecordFailure should accept a real call.
	if err := l.Check("alice"); err != nil {
		t.Fatalf("nil clock Check: %v", err)
	}
}

func TestSnapshot_UnknownUserReturnsZero(t *testing.T) {
	l := NewLockout(func() time.Time { return time.Unix(1_700_000_000, 0) })
	snap := l.Snapshot("ghost")
	if snap.Failures != 0 || !snap.LockedUntil.IsZero() {
		t.Fatalf("expected zero snapshot, got %+v", snap)
	}
}

func TestRecordFailure_ReturnsLockedWhenAlreadyLocked(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewLockout(clk.now)
	for i := 0; i < 5; i++ {
		_ = l.RecordFailure("alice")
	}
	// 6th during lockout window is ErrAccountLocked and doesn't touch the
	// state further.
	if err := l.RecordFailure("alice"); err != ErrAccountLocked {
		t.Fatalf("expected locked, got %v", err)
	}
}

func TestCheck_ReturnsLockedWhileInLockout(t *testing.T) {
	clk := &fakeClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewLockout(clk.now)
	for i := 0; i < 5; i++ {
		_ = l.RecordFailure("alice")
	}
	// Check reports locked for a user with a still-live lockedUntil.
	if err := l.Check("alice"); err != ErrAccountLocked {
		t.Fatalf("expected ErrAccountLocked, got %v", err)
	}
}

func TestHashPassword_EmptyRejected(t *testing.T) {
	if _, err := HashPassword(""); err != ErrPasswordBlank {
		t.Fatalf("expected ErrPasswordBlank, got %v", err)
	}
}

func TestComparePassword_MismatchedMetadata(t *testing.T) {
	// Empty, wrong prefix, and malformed Sscanf args all return ErrInvalidHash.
	cases := []string{
		"",
		"$argon2id$v=19$m=bad,t=2,p=2$aaa$bbb",
		"$argon2id$v=19$m=64,t=2,p=2$@@@$!!!", // chars outside base64 alphabet
	}
	for _, h := range cases {
		if err := ComparePassword(h, "x"); err != ErrInvalidHash {
			t.Errorf("%q: expected ErrInvalidHash, got %v", h, err)
		}
	}
}

func TestComparePassword_InvalidSalt(t *testing.T) {
	// Valid structural prefix but un-decodable salt → ErrInvalidHash.
	bad := "$argon2id$v=19$m=65536,t=2,p=2$*bad*$*bad*"
	if err := ComparePassword(bad, "x"); err != ErrInvalidHash {
		t.Fatalf("expected ErrInvalidHash, got %v", err)
	}
}

func TestHashPassword_ProducesEncodedPrefix(t *testing.T) {
	h, err := HashPassword("strong-password-long")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=") {
		t.Fatalf("unexpected hash: %s", h)
	}
}

func TestSessionStore_DefaultsAndRealClock(t *testing.T) {
	s := NewSessionStore(0, nil) // zero TTL → 8h default, nil clock → real
	sess, err := s.Issue("u", "n", "r")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Lookup(sess.Token)
	if err != nil || got.Token != sess.Token {
		t.Fatalf("issue/lookup: %+v %v", got, err)
	}
}

func TestSessionStore_LookupUnknownToken(t *testing.T) {
	s := NewSessionStore(time.Hour, nil)
	if _, err := s.Lookup("nope"); err != ErrSessionInvalid {
		t.Fatalf("expected ErrSessionInvalid, got %v", err)
	}
}
