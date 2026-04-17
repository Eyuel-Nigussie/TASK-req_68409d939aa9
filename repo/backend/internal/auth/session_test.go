package auth

import (
	"testing"
	"time"
)

func TestSessionStore_IssueAndLookup(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	s := NewSessionStore(time.Hour, clk.now)
	sess, err := s.Issue("u1", "alice", "front_desk")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := s.Lookup(sess.Token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Username != "alice" {
		t.Fatalf("username = %q", got.Username)
	}
}

func TestSessionStore_Expiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	s := NewSessionStore(time.Hour, clk.now)
	sess, _ := s.Issue("u1", "alice", "front_desk")
	clk.advance(time.Hour + time.Second)
	if _, err := s.Lookup(sess.Token); err != ErrSessionInvalid {
		t.Fatalf("expected ErrSessionInvalid after expiry, got %v", err)
	}
}

func TestSessionStore_Revoke(t *testing.T) {
	s := NewSessionStore(time.Hour, nil)
	sess, _ := s.Issue("u1", "alice", "admin")
	s.Revoke(sess.Token)
	if _, err := s.Lookup(sess.Token); err != ErrSessionInvalid {
		t.Fatalf("expected ErrSessionInvalid after revoke, got %v", err)
	}
}

func TestNewToken_Unique(t *testing.T) {
	a, _ := NewToken()
	b, _ := NewToken()
	if a == b || len(a) != 64 {
		t.Fatalf("token uniqueness/length broken: %q %q", a, b)
	}
}
