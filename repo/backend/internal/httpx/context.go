// Package httpx centralizes HTTP concerns: middleware, error mapping,
// request context keys. Keeping these separate from handlers lets the
// handlers read as plain "translate request -> service call -> response".
package httpx

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

// Context keys used by middleware and handlers.
const (
	CtxSession     = "session"
	CtxWorkstation = "workstation"
)

// CurrentSession returns the *auth.Session attached by RequireAuth. Panics
// if called on an unauthenticated route; callers should route through
// RequireAuth in advance.
func CurrentSession(c echo.Context) *auth.Session {
	v := c.Get(CtxSession)
	if v == nil {
		return nil
	}
	return v.(*auth.Session)
}

// Workstation returns the operator workstation header, falling back to the
// remote address when absent so the audit log never contains an empty field.
func Workstation(c echo.Context) string {
	if w := c.Request().Header.Get("X-Workstation"); w != "" {
		return w
	}
	return c.Request().RemoteAddr
}

// ClientTime parses the X-Workstation-Time header into a UTC time.
// The client is expected to send RFC3339 (ISO 8601) so the audit log can
// record the operator-local clock value alongside the server's. When the
// header is absent or malformed this returns the zero value; callers pass
// the zero value into the audit log, which omits it rather than silently
// substituting server time. We intentionally do NOT fall back to
// time.Now() here — that would defeat the purpose of a separate field.
func ClientTime(c echo.Context) time.Time {
	raw := c.Request().Header.Get("X-Workstation-Time")
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// WriteError maps store/service errors to HTTP responses. Unknown errors
// are logged server-side and returned to the client with a generic message
// so internal details (SQL, stack traces, encryption errors) never leak.
func WriteError(c echo.Context, err error) error {
	if err == nil {
		return nil
	}
	// Preserve echo.HTTPError messages the handler created on purpose.
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return he
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrConflict):
		return echo.NewHTTPError(http.StatusConflict, "conflict")
	case errors.Is(err, auth.ErrAccountLocked):
		return echo.NewHTTPError(http.StatusLocked, "account is locked; try again later")
	case errors.Is(err, auth.ErrMismatched), errors.Is(err, auth.ErrSessionInvalid):
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	// Unclassified: log internally, hide detail from the client.
	method := c.Request().Method
	path := c.Request().URL.Path
	log.Printf("[err] %s %s: %v", method, path, err)
	return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
}
