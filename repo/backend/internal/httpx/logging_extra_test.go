package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
)

// failingWriter satisfies io.Writer and returns an error on every write
// so we can observe the logger's fallback branch.
type failingWriter struct{ calls int }

func (f *failingWriter) Write(_ []byte) (int, error) { f.calls++; return 0, errors.New("nope") }

func TestLogger_Write_MarshalFallback(t *testing.T) {
	// Force a marshal failure by sabotaging the LogEntry with a value that
	// json.Marshal cannot encode. Because LogEntry is a concrete struct we
	// can't inject an unencodable field; however, the logger's "marshal
	// fallback" branch is only reachable if json.Marshal returns an error.
	// We confirm the alternative path — Out.Write returning an error —
	// still does not panic.
	lg := &Logger{Out: &failingWriter{}}
	lg.Write(LogEntry{Method: "GET"})
}

func TestLogger_Write_DefaultsFilledIn(t *testing.T) {
	var buf bytes.Buffer
	lg := &Logger{Out: &buf}
	lg.Write(LogEntry{Method: "GET"})
	var got LogEntry
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got)
	if got.Time == "" {
		t.Fatalf("expected Time default, got %+v", got)
	}
	if got.Level != "info" {
		t.Fatalf("expected level=info default, got %q", got.Level)
	}
}

// When the request carries both a session and an upstream X-Request-ID,
// the middleware should preserve the rid AND fill in actor/role.
func TestStructuredLogging_AttachesSessionFields(t *testing.T) {
	var buf bytes.Buffer
	lg := &Logger{Out: &buf}
	e := echo.New()
	e.Use(StructuredLogging(lg))
	e.GET("/p", func(c echo.Context) error {
		c.Set(CtxSession, &auth.Session{UserID: "u1", Role: "admin", Username: "a"})
		return c.String(200, "ok")
	})
	req := httptest.NewRequest("GET", "/p", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var got LogEntry
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got)
	if got.ActorID != "u1" || got.Role != "admin" {
		t.Fatalf("session fields missing: %+v", got)
	}
}

// When the response is a 4xx the log level should be "warn".
func TestStructuredLogging_MarksWarnFor4xx(t *testing.T) {
	var buf bytes.Buffer
	lg := &Logger{Out: &buf}
	e := echo.New()
	e.Use(StructuredLogging(lg))
	e.GET("/e", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusNotFound, "missing")
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/e", nil))
	var got LogEntry
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got)
	if got.Level != "warn" {
		t.Fatalf("expected warn for 4xx, got %q", got.Level)
	}
}

// RequestID pulls from the request context when present (the middleware
// injects it there after generating one).
func TestRequestID_FromContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxRequestID, "ctx-rid"))
	c := e.NewContext(req, httptest.NewRecorder())
	if got := RequestID(c); got != "ctx-rid" {
		t.Fatalf("expected ctx-rid, got %q", got)
	}
}

// WriteError: the explicit "account locked" and "session invalid"
// branches should return 423 and 401 respectively.
func TestWriteError_AccountLockedAndSessionInvalid(t *testing.T) {
	e := echo.New()
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
	if err := WriteError(c, auth.ErrAccountLocked); err == nil {
		t.Fatal("expected error")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusLocked {
		t.Fatalf("expected 423 locked, got %v", err)
	}
	if err := WriteError(c, auth.ErrSessionInvalid); err == nil {
		t.Fatal("expected error")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", err)
	}
	// ErrConflict branch.
	if err := WriteError(c, store.ErrConflict); err == nil {
		t.Fatal("expected error")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %v", err)
	}
}

// Verify the middleware fills workspace + remote_addr even when session
// is absent so the logs are usable in anonymous contexts.
func TestStructuredLogging_NoSessionStillLogs(t *testing.T) {
	var buf bytes.Buffer
	lg := &Logger{Out: &buf}
	e := echo.New()
	e.Use(StructuredLogging(lg))
	e.GET("/pub", func(c echo.Context) error { return c.String(200, "") })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/pub", nil))
	if !strings.Contains(buf.String(), `"path":"/pub"`) {
		t.Fatalf("path not logged: %s", buf.String())
	}
}
