package api

import (
	"errors"
	"net/http"
	"sync"

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
		return echo.NewHTTPError(http.StatusForbidden, "account disabled")
	}
	if err := auth.ComparePassword(u.PasswordHash, body.Password); err != nil {
		if lerr := s.Lockout.RecordFailure(ctx, body.Username); lerr != nil {
			return httpx.WriteError(c, lerr)
		}
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	_ = s.Lockout.RecordSuccess(ctx, body.Username)
	sess, err := s.Sessions.Issue(u.ID, u.Username, string(u.Role))
	if err != nil {
		return httpx.WriteError(c, err)
	}
	_ = s.Audit.Log(ctx, u.ID, httpx.Workstation(c), httpx.ClientTime(c), "user", u.ID, "login", "", nil, nil)
	return c.JSON(http.StatusOK, map[string]any{
		"token":   sess.Token,
		"user": map[string]any{
			"id": u.ID, "username": u.Username, "role": u.Role,
		},
		"expires_at": sess.ExpiresAt,
	})
}

// Logout revokes the current session token.
func (s *Server) Logout(c echo.Context) error {
	sess := httpx.CurrentSession(c)
	if sess == nil {
		return c.NoContent(http.StatusNoContent)
	}
	s.Sessions.Revoke(sess.Token)
	_ = s.Audit.Log(c.Request().Context(), sess.UserID, httpx.Workstation(c), httpx.ClientTime(c), "user", sess.UserID, "logout", "", nil, nil)
	return c.NoContent(http.StatusNoContent)
}

// WhoAmI returns the authenticated user.
func (s *Server) WhoAmI(c echo.Context) error {
	sess := httpx.CurrentSession(c)
	return c.JSON(http.StatusOK, map[string]any{
		"id": sess.UserID, "username": sess.Username, "role": sess.Role,
		"expires_at": sess.ExpiresAt,
	})
}
