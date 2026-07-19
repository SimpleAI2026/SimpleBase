package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
)

func testDeps(t *testing.T, health HealthChecker) Dependencies {
	t.Helper()
	// 每个测试使用独立 Registry，避免默认 registry 在并行测试中冲突。
	reg := prometheus.NewRegistry()
	return Dependencies{
		Config: config.Config{
			HTTP: config.HTTPConfig{Address: ":0"},
			Observability: config.ObservabilityConfig{MetricsPath: "/metrics"},
			Limits: config.LimitsConfig{MaxRequestBytes: 1 << 20},
		},
		Logger:  observability.NewLogger("debug", "json", nil),
		Metrics: observability.NewMetrics(reg),
		Health:  health,
	}
}

type fakeHealth struct {
	liveErr  error
	readyErr error
}

func (f *fakeHealth) Live(ctx context.Context) error  { return f.liveErr }
func (f *fakeHealth) Ready(ctx context.Context) error { return f.readyErr }

func TestHealthLive(t *testing.T) {
	e := NewRouter(testDeps(t, &fakeHealth{}))
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHealthReadyFailure(t *testing.T) {
	e := NewRouter(testDeps(t, &fakeHealth{readyErr: errors.New("s3 down")}))
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestRequestIDGenerated(t *testing.T) {
	e := NewRouter(testDeps(t, &fakeHealth{}))
	e.GET("/_test", func(c echo.Context) error {
		rid := RequestIDFromContext(c.Request().Context())
		if rid == "" {
			t.Error("request id missing in context")
		}
		return c.String(http.StatusOK, rid)
	})
	req := httptest.NewRequest(http.MethodGet, "/_test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	rid := rec.Header().Get("X-Request-ID")
	if rid == "" {
		t.Fatal("request id header missing")
	}
	if rec.Body.String() != rid {
		t.Fatalf("context rid %q != header rid %q", rec.Body.String(), rid)
	}
}

func TestRequestIDAcceptedFromClient(t *testing.T) {
	e := NewRouter(testDeps(t, &fakeHealth{}))
	e.GET("/_test", func(c echo.Context) error {
		return c.String(http.StatusOK, RequestIDFromContext(c.Request().Context()))
	})
	req := httptest.NewRequest(http.MethodGet, "/_test", nil)
	req.Header.Set("X-Request-ID", "client-supplied-rid-1234")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Body.String() != "client-supplied-rid-1234" {
		t.Fatalf("expected client rid echoed, got %q", rec.Body.String())
	}
}

func TestRequestIDRejectsInjection(t *testing.T) {
	e := NewRouter(testDeps(t, &fakeHealth{}))
	e.GET("/_test", func(c echo.Context) error {
		return c.String(http.StatusOK, RequestIDFromContext(c.Request().Context()))
	})
	req := httptest.NewRequest(http.MethodGet, "/_test", nil)
	req.Header.Set("X-Request-ID", "bad rid with spaces")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	rid := rec.Body.String()
	if rid == "bad rid with spaces" {
		t.Fatal("injected rid accepted; should be regenerated")
	}
	if rid == "" {
		t.Fatal("no rid generated")
	}
}

func TestMetricsEndpointExposed(t *testing.T) {
	e := NewRouter(testDeps(t, &fakeHealth{}))
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
