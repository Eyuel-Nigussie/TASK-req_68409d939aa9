package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/labstack/echo/v4"
)

func newEchoWithSession(sess *auth.Session) (*echo.Echo, *auth.SessionStore) {
	store := auth.NewSessionStore(time.Hour, nil)
	return echo.New(), store
}

// RequireAuth tests

func TestRequireAuth_RejectsMissingToken(t *testing.T) {
	e, store := newEchoWithSession(nil)
	called := false
	h := RequireAuth(store)(func(c echo.Context) error { called = true; return c.String(200, "ok") })
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h(c)
	if called {
		t.Fatal("handler should not be invoked")
	}
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", err)
	}
}

func TestRequireAuth_RejectsInvalidToken(t *testing.T) {
	e, store := newEchoWithSession(nil)
	h := RequireAuth(store)(func(c echo.Context) error { return c.String(200, "ok") })
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-token")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h(c)
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", err)
	}
}

func TestRequireAuth_AcceptsValidToken(t *testing.T) {
	e, store := newEchoWithSession(nil)
	sess, _ := store.Issue("u1", "alice", "admin")
	invoked := false
	h := RequireAuth(store)(func(c echo.Context) error { invoked = true; return c.String(200, "ok") })
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h(c); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !invoked {
		t.Fatal("handler not called")
	}
	if CurrentSession(c) == nil || CurrentSession(c).Username != "alice" {
		t.Fatalf("session not attached: %+v", CurrentSession(c))
	}
}

// RequireRoles

func TestRequireRoles_AllowsMatching(t *testing.T) {
	e := echo.New()
	h := RequireRoles(models.RoleAdmin)(func(c echo.Context) error { return c.String(200, "ok") })
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(CtxSession, &auth.Session{Role: "admin"})
	if err := h(c); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
}

func TestRequireRoles_RejectsMissingSession(t *testing.T) {
	e := echo.New()
	h := RequireRoles(models.RoleAdmin)(func(c echo.Context) error { return nil })
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	err := h(c)
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", err)
	}
}

func TestRequireRoles_RejectsWrongRole(t *testing.T) {
	e := echo.New()
	h := RequireRoles(models.RoleAdmin)(func(c echo.Context) error { return nil })
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	c.Set(CtxSession, &auth.Session{Role: "front_desk"})
	err := h(c)
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %v", err)
	}
}

// SecurityHeaders

func TestSecurityHeaders_Applied(t *testing.T) {
	e := echo.New()
	e.Use(SecurityHeaders())
	e.GET("/", func(c echo.Context) error { return c.String(200, "ok") })
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Content-Security-Policy"} {
		if rec.Header().Get(h) == "" {
			t.Errorf("header %s not set", h)
		}
	}
}

// RequirePermission

type stubResolver struct{ grants []string; err error }

func (s stubResolver) GrantsForUser(_ context.Context, _, _ string) ([]string, error) {
	return s.grants, s.err
}

func TestRequirePermission_NoSessionReturns401(t *testing.T) {
	e := echo.New()
	h := RequirePermission(stubResolver{}, "x")(func(c echo.Context) error { return nil })
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	err := h(c)
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", err)
	}
}

func TestRequirePermission_StoreErrorBubbles(t *testing.T) {
	e := echo.New()
	h := RequirePermission(stubResolver{err: errors.New("boom")}, "x")(func(c echo.Context) error { return nil })
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	c.Set(CtxSession, &auth.Session{UserID: "u1", Role: "admin"})
	err := h(c)
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %v", err)
	}
}

func TestRequirePermission_MissingGrantIs403(t *testing.T) {
	e := echo.New()
	h := RequirePermission(stubResolver{grants: []string{"other"}}, "x")(func(c echo.Context) error { return nil })
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	c.Set(CtxSession, &auth.Session{UserID: "u1", Role: "admin"})
	err := h(c)
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %v", err)
	}
}

func TestRequirePermission_GrantAllows(t *testing.T) {
	e := echo.New()
	called := false
	h := RequirePermission(stubResolver{grants: []string{"x"}}, "x")(func(c echo.Context) error {
		called = true
		return nil
	})
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	c.Set(CtxSession, &auth.Session{UserID: "u1", Role: "admin"})
	if err := h(c); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !called {
		t.Fatal("handler not called")
	}
}

// WriteError additional branches

func TestWriteError_Nil(t *testing.T) {
	e := echo.New()
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	if err := WriteError(c, nil); err != nil {
		t.Fatalf("nil -> nil, got %v", err)
	}
}

// CurrentSession with nil

func TestCurrentSession_ReturnsNilWhenAbsent(t *testing.T) {
	e := echo.New()
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	if CurrentSession(c) != nil {
		t.Fatal("expected nil session")
	}
}

// Workstation fallbacks — covered in context_test.go already.
// RequestID helper from the logging path

func TestRequestID_FromContextAndHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	// Without middleware, RequestID falls back to the header.
	req.Header.Set(CtxRequestIDHeader, "rid-direct")
	c := e.NewContext(req, httptest.NewRecorder())
	if RequestID(c) != "rid-direct" {
		t.Fatalf("expected header fallback, got %q", RequestID(c))
	}
}
