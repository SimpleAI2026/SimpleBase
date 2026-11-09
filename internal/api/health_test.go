package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func serveHealth(h *HealthHandler, method, path string) *httptest.ResponseRecorder {
	e := echo.New()
	e.HideBanner = true
	e.GET("/health/live", h.Live)
	e.GET("/health/ready", h.Ready)
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestHealthHandler_NilChecker(t *testing.T) {
	h := &HealthHandler{}
	for _, path := range []string{"/health/live", "/health/ready"} {
		rec := serveHealth(h, http.MethodGet, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s nil checker: status %d", path, rec.Code)
		}
	}
}

func TestHealthHandler_Success(t *testing.T) {
	h := &HealthHandler{checker: &fakeHealth{}}
	for _, path := range []string{"/health/live", "/health/ready"} {
		rec := serveHealth(h, http.MethodGet, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s success: status %d", path, rec.Code)
		}
	}
}

func TestHealthHandler_Failure(t *testing.T) {
	h := &HealthHandler{checker: &fakeHealth{liveErr: errors.New("proc"), readyErr: errors.New("s3")}}
	live := serveHealth(h, http.MethodGet, "/health/live")
	if live.Code != http.StatusServiceUnavailable {
		t.Fatalf("live fail: %d", live.Code)
	}
	if !strings.Contains(live.Body.String(), "unhealthy") {
		t.Fatalf("live body=%s", live.Body.String())
	}
	ready := serveHealth(h, http.MethodGet, "/health/ready")
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready fail: %d", ready.Code)
	}
	if !strings.Contains(ready.Body.String(), "not_ready") {
		t.Fatalf("ready body=%s", ready.Body.String())
	}
}
