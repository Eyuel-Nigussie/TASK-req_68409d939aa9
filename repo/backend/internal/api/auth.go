package api

import (
	"errors"
	"net/http"
	"sync"

	"github.com/eaglepoint/oops/backend/internal/audit"
	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/httpx"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

// dummyHashOnce lazily generates a single Argon2id hash the login
// handler can compare against when a username does not exist. The hash
// is thrown away (cannot match any real password because the salt is
// fresh), but running the compare keeps the not-found branch's timing
// within the same order of magnitude as a real failed login, closing
// the username-enumeration timing channel.
var (
	dummyHashOnce sync.Once
	dummyHashStr  string
)

func loginDummyHash() string {
	dummyHashOnce.Do(func() {
		h, err := auth.HashPassword("x-not-a-real-user-timing-pad")
		if err != nil {
			dummyHashStr = ""
			return
		}
		dummyHashStr = h
	})
	return dummyHashStr
}

// Login accepts username + password, validates policy and lockout, then
// returns a bearer session token. Failures increment the lockout counter.
func (s *Server) Login(c echo.Context) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if body.Username == "" || body.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username and password required")
	}
	ctx := c.Request().Context()
	if err := s.Lockout.Check(ctx, body.Username); err != nil {
		return httpx.WriteError(c, err)
	}
	u, err := s.Store.GetUserByUsername(ctx, body.Username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Burn roughly the same CPU as the real path so a timing
			// probe can't enumerate usernames. The compare is discarded.
			if dh := loginDummyHash(); dh != "" {
				_ = auth.ComparePassword(dh, body.Password)
			}
			// Record a failure anyway to rate-limit username enumeration.
			_ = s.Lockout.RecordFailure(ctx, body.Username)
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
		return httpx.WriteError(c, err)
	}
	if u.Disabled {
		// Run the real compare before returning so the disabled path
		// costs the same CPU as the active-account path. Returning 401
		// (not 403) also closes the status-code enumeration channel —
		// an external probe can't distinguish disabled from wrong
		// password anymore. The lockout counter is incremented for the
		// same reason.
		_ = auth.ComparePassword(u.PasswordHash, body.Password)
		_ = s.Lockout.RecordFailure(ctx, body.Username)
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := auth.ComparePassword(u.PasswordHash, body.Password); err != nil {
		if lerr := s.Lockout.RecordFailure(ctx, body.Username); lerr != nil {
			return httpx.WriteError(c, lerr)
		}
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	_ = s.Lockout.RecordSuccess(ctx, body.Username)
	sess, err := s.Sessions.IssueWithFlags(u.ID, u.Username, string(u.Role), u.MustRotatePassword)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	_ = s.Audit.Log(ctx, u.ID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityUser, u.ID, "login", "", nil, nil)
	return c.JSON(http.StatusOK, map[string]any{
		"token":   sess.Token,
		"user": map[string]any{
			"id": u.ID, "username": u.Username, "role": u.Role,
			// Tell the SPA the first thing it must do is redirect to
			// the password-rotation screen. Without the flag the
			// frontend would happily try to fetch /api/orders etc. and
			// every request would 403 behind the rotation gate.
			"must_rotate_password": u.MustRotatePassword,
		},
		"expires_at":           sess.ExpiresAt,
		"must_rotate_password": u.MustRotatePassword,
	})
}

// RotatePassword lets a signed-in user replace their own password. It
// doubles as the gate to clear MustRotatePassword on accounts seeded
// with the shared demo credential (L2). The caller must supply their
// current password so a stolen session token alone cannot rotate a
// credential.
func (s *Server) RotatePassword(c echo.Context) error {
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if body.OldPassword == "" || body.NewPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "old_password and new_password required")
	}
	if body.OldPassword == body.NewPassword {
		return echo.NewHTTPError(http.StatusBadRequest, "new_password must differ from old_password")
	}
	sess := httpx.CurrentSession(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := c.Request().Context()
	u, err := s.Store.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return httpx.WriteError(c, err)
	}
	if err := auth.ComparePassword(u.PasswordHash, body.OldPassword); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "current password does not match")
	}
	newHash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		// ValidatePolicy errors (too short, single class, blocklist) surface
		// as 400s so the operator knows to pick another value.
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	u.PasswordHash = newHash
	u.MustRotatePassword = false
	u.UpdatedAt = s.Clock()
	if err := s.Store.UpdateUser(ctx, u); err != nil {
		return httpx.WriteError(c, err)
	}
	// Drop the rotation flag on the live session so follow-up requests
	// pass the gate without needing a second login round-trip.
	s.Sessions.ClearMustRotate(sess.Token)
	_ = s.Audit.Log(ctx, u.ID, httpx.Workstation(c), httpx.ClientTime(c),
		audit.EntityUser, u.ID, "rotate_password", "", nil, nil)
	return c.NoContent(http.StatusNoContent)
}

// Logout revokes the current session token.
func (s *Server) Logout(c echo.Context) error {
	sess := httpx.CurrentSession(c)
	if sess == nil {
		return c.NoContent(http.StatusNoContent)
	}
	s.Sessions.Revoke(sess.Token)
	_ = s.Audit.Log(c.Request().Context(), sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), audit.EntityUser, sess.UserID, "logout", "", nil, nil)
	return c.NoContent(http.StatusNoContent)
}

// WhoAmI returns the authenticated user.
func (s *Server) WhoAmI(c echo.Context) error {
	sess := httpx.CurrentSession(c)
	return c.JSON(http.StatusOK, map[string]any{
		"id": sess.UserID, "username": sess.Username, "role": sess.Role,
		"expires_at":           sess.ExpiresAt,
		"must_rotate_password": sess.MustRotatePassword,
	})
}
