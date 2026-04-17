package httpx

import (
	"context"
	"net/http"
	"strings"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/labstack/echo/v4"
)

// RequireAuth is middleware that validates the Authorization header and
// attaches the resolved session to the echo.Context.
func RequireAuth(sessions *auth.SessionStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tok := strings.TrimPrefix(c.Request().Header.Get("Authorization"), "Bearer ")
			if tok == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing token")
			}
			sess, err := sessions.Lookup(tok)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
			}
			c.Set(CtxSession, sess)
			return next(c)
		}
	}
}

// RequireRoles returns middleware that allows the request only if the
// authenticated user's role is in `allowed`. Must be chained after
// RequireAuth.
func RequireRoles(allowed ...models.Role) echo.MiddlewareFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[string(r)] = struct{}{}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sess := CurrentSession(c)
			if sess == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing session")
			}
			if _, ok := set[sess.Role]; !ok {
				return echo.NewHTTPError(http.StatusForbidden, "role not permitted")
			}
			return next(c)
		}
	}
}

// PermissionResolver is the minimal surface the middleware needs to
// answer "does user X hold permission Y?". It matches the store's
// GrantsForUser signature; api/server injects an adapter so httpx
// stays free of a store dependency.
type PermissionResolver interface {
	GrantsForUser(ctx context.Context, userID, role string) ([]string, error)
}

// RequirePermission is middleware that enforces a single permission
// string (e.g. "orders.write"). Must be chained after RequireAuth. The
// permission set is refreshed on every request so administrator changes
// take effect without a session reissue.
func RequirePermission(resolver PermissionResolver, permID string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sess := CurrentSession(c)
			if sess == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing session")
			}
			perms, err := resolver.GrantsForUser(c.Request().Context(), sess.UserID, sess.Role)
			if err != nil {
				return WriteError(c, err)
			}
			for _, p := range perms {
				if p == permID {
					return next(c)
				}
			}
			return echo.NewHTTPError(http.StatusForbidden, "missing permission: "+permID)
		}
	}
}

// SecurityHeaders applies defensive headers appropriate to a LAN-only deploy.
func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			// CSP is strict because the bundled SPA is the only consumer.
			h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'")
			return next(c)
		}
	}
}
