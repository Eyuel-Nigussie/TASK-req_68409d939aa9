package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestClientTime_ParsesRFC3339(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Workstation-Time", "2030-01-01T12:34:56Z")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	got := ClientTime(c)
	want := time.Date(2030, 1, 1, 12, 34, 56, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestClientTime_MissingHeaderReturnsZero(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if !ClientTime(c).IsZero() {
		t.Fatalf("expected zero time when header missing")
	}
}

func TestClientTime_MalformedReturnsZero(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Workstation-Time", "yesterday")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if !ClientTime(c).IsZero() {
		t.Fatalf("expected zero time on malformed header")
	}
}

func TestWriteError_PreservesEchoHTTPError(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	he := echo.NewHTTPError(http.StatusTeapot, "I'm a teapot")
	got := WriteError(c, he)
	var out *echo.HTTPError
	if !errors.As(got, &out) {
		t.Fatalf("expected *echo.HTTPError, got %T", got)
	}
	if out.Code != http.StatusTeapot {
		t.Fatalf("expected 418, got %d", out.Code)
	}
}

func TestWorkstation_FallsBackToRemoteAddr(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.10:55555"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if got := Workstation(c); got != "192.0.2.10:55555" {
		t.Fatalf("expected remote addr fallback, got %q", got)
	}
	req.Header.Set("X-Workstation", "lab-5")
	if got := Workstation(c); got != "lab-5" {
		t.Fatalf("expected header value, got %q", got)
	}
}
