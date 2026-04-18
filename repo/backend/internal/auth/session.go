package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ErrSessionInvalid means the token is unknown or expired.
var ErrSessionInvalid = errors.New("session invalid or expired")

// Session represents an authenticated user on a workstation.
//
// MustRotatePassword mirrors the flag on the User record at the moment
// of issuance. When true, a middleware denies every authenticated
// request except the small allowlist needed to change the password.
// The flag is cleared on a successful rotation; the session continues
// to be valid for its remaining TTL without forcing a re-login (L2).
type Session struct {
	Token              string
	UserID             string
	Username           string
	Role               string
	IssuedAt           time.Time
	ExpiresAt          time.Time
	MustRotatePassword bool
}

// SessionStore is a goroutine-safe, in-memory session cache. A single-node
// deployment survives a restart by requiring users to log in again: this is
// acceptable because sessions are purely a web-UI convenience and the portal
// is local.
type SessionStore struct {
	mu   sync.RWMutex
	ttl  time.Duration
	now  Clock
	byTk map[string]*Session
}

// NewSessionStore returns a session store with the given TTL. A zero TTL
// defaults to 8 hours, matching a work shift.
func NewSessionStore(ttl time.Duration, now Clock) *SessionStore {
	if ttl == 0 {
		ttl = 8 * time.Hour
	}
	if now == nil {
		now = RealClock
	}
	return &SessionStore{ttl: ttl, now: now, byTk: make(map[string]*Session)}
}

// NewToken returns 32 bytes of cryptographically random hex, suitable as a
// session identifier.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Issue creates a new session for the given user and returns the token.
func (s *SessionStore) Issue(userID, username, role string) (*Session, error) {
	return s.IssueWithFlags(userID, username, role, false)
}

// IssueWithFlags is the extended form of Issue used by the login
// handler to propagate per-user flags (currently MustRotatePassword)
// into the session record.
func (s *SessionStore) IssueWithFlags(userID, username, role string, mustRotate bool) (*Session, error) {
	tok, err := NewToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	sess := &Session{
		Token:              tok,
		UserID:             userID,
		Username:           username,
		Role:               role,
		IssuedAt:           now,
		ExpiresAt:          now.Add(s.ttl),
		MustRotatePassword: mustRotate,
	}
	s.mu.Lock()
	s.byTk[tok] = sess
	s.mu.Unlock()
	return sess, nil
}

// ClearMustRotate flips MustRotatePassword off on the live session so
// subsequent requests pass the rotation gate. Called after a successful
// /api/auth/rotate-password so the user doesn't have to log in again.
func (s *SessionStore) ClearMustRotate(token string) {
	s.mu.Lock()
	if sess, ok := s.byTk[token]; ok {
		sess.MustRotatePassword = false
	}
	s.mu.Unlock()
}

// Lookup returns the session for a token, or ErrSessionInvalid.
func (s *SessionStore) Lookup(token string) (*Session, error) {
	s.mu.RLock()
	sess, ok := s.byTk[token]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrSessionInvalid
	}
	if s.now().After(sess.ExpiresAt) {
		s.Revoke(token)
		return nil, ErrSessionInvalid
	}
	return sess, nil
}

// Revoke removes a session, used at logout.
func (s *SessionStore) Revoke(token string) {
	s.mu.Lock()
	delete(s.byTk, token)
	s.mu.Unlock()
}
