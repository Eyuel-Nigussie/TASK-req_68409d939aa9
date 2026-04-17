package httpx

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestStructuredLogging_EmitsOneJSONLinePerRequest(t *testing.T) {
	var buf bytes.Buffer
	lg := &Logger{Out: &buf}
	e := echo.New()
	e.Use(StructuredLogging(lg))
	e.GET("/ok", func(c echo.Context) error { return c.String(200, "ok") })

	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set("X-Workstation", "ws-test")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d: %q", len(lines), buf.String())
	}
	var got LogEntry
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("line is not JSON: %v %q", err, lines[0])
	}
	if got.Method != "GET" || got.Path != "/ok" || got.Status != 200 {
		t.Fatalf("unexpected entry: %+v", got)
	}
	if got.Workstation != "ws-test" {
		t.Fatalf("workstation not captured: %+v", got)
	}
	if got.RequestID == "" {
		t.Fatalf("request_id missing")
	}
	// The echo response must expose the generated request ID so the
	// client can include it in subsequent requests / support tickets.
	if rec.Header().Get(CtxRequestIDHeader) == "" {
		t.Fatalf("X-Request-ID not on response")
	}
}

func TestStructuredLogging_PreservesUpstreamRequestID(t *testing.T) {
	var buf bytes.Buffer
	lg := &Logger{Out: &buf}
	e := echo.New()
	e.Use(StructuredLogging(lg))
	e.GET("/ok", func(c echo.Context) error { return c.String(200, "ok") })

	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set(CtxRequestIDHeader, "abc123")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var got LogEntry
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got)
	if got.RequestID != "abc123" {
		t.Fatalf("request id should be propagated: %+v", got)
	}
	if rec.Header().Get(CtxRequestIDHeader) != "abc123" {
		t.Fatalf("response header missing: %v", rec.Header())
	}
}

func TestStructuredLogging_MarksErrorLevelFor500(t *testing.T) {
	var buf bytes.Buffer
	lg := &Logger{Out: &buf}
	e := echo.New()
	e.Use(StructuredLogging(lg))
	e.GET("/fail", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "boom")
	})
	req := httptest.NewRequest("GET", "/fail", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var got LogEntry
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got)
	if got.Level != "error" {
		t.Fatalf("expected level=error for 500, got %q (entry=%+v)", got.Level, got)
	}
	if got.Error == "" {
		t.Fatalf("expected error message on entry: %+v", got)
	}
}
