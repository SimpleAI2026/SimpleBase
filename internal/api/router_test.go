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
			HTTP:          config.HTTPConfig{Address: ":0"},
			Observability: config.ObservabilityConfig{MetricsPath: "/metrics"},
			Limits:        config.LimitsConfig{MaxRequestBytes: 1 << 20},
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

func TestNilStoreAndMissingProjectHandlers(t *testing.T) {
	e := echo.New()
	gh := NewGoFunctionHandler(nil, true, nil)
	cj := NewCronJobHandler(nil, true, nil, nil)
	sch := &agentScheduleHandler{}

	mount := func(injectProject bool) *echo.Echo {
		ee := echo.New()
		ee.HideBanner = true
		ee.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				ctx := c.Request().Context()
				if injectProject {
					ctx = WithProject(ctx, ProjectContext{ID: "proj-1"})
				}
				c.SetRequest(c.Request().WithContext(ctx))
				return next(c)
			}
		})
		ee.GET("/gf", gh.List)
		ee.GET("/gf/:name", gh.Get)
		ee.DELETE("/gf/:name", gh.Delete)
		ee.POST("/gf", gh.Create)
		ee.GET("/cj", cj.List)
		ee.GET("/cj/:jobID", cj.Get)
		ee.GET("/cj/:jobID/runs", cj.ListRuns)
		ee.DELETE("/cj/:jobID", cj.Delete)
		ee.GET("/as", sch.ListSchedules)
		ee.GET("/as/:scheduleID", sch.GetSchedule)
		ee.GET("/as/:scheduleID/runs", sch.ListScheduleRuns)
		ee.DELETE("/as/:scheduleID", sch.DeleteSchedule)
		ee.PATCH("/as/:scheduleID", sch.PatchSchedule)
		ee.POST("/as", sch.CreateSchedule)
		ee.POST("/as/:scheduleID/run", sch.TriggerScheduleRun)
		return ee
	}

	eNo := mount(false)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/gf"},
		{http.MethodGet, "/gf/n"},
		{http.MethodDelete, "/gf/n"},
		{http.MethodPost, "/gf"},
		{http.MethodGet, "/cj"},
		{http.MethodGet, "/cj/j"},
		{http.MethodGet, "/cj/j/runs"},
		{http.MethodDelete, "/cj/j"},
		{http.MethodGet, "/as"},
		{http.MethodGet, "/as/s"},
		{http.MethodGet, "/as/s/runs"},
		{http.MethodDelete, "/as/s"},
		{http.MethodPatch, "/as/s"},
		{http.MethodPost, "/as"},
		{http.MethodPost, "/as/s/run"},
	} {
		rec := doRequest(eNo, tc.method, tc.path, map[string]any{"name": "X", "source": "package main"})
		if rec.Code == http.StatusOK || rec.Code == http.StatusCreated || rec.Code == http.StatusNoContent {
			t.Fatalf("%s %s expected missing project, got %d", tc.method, tc.path, rec.Code)
		}
	}

	eYes := mount(true)
	for _, path := range []string{"/gf", "/gf/n", "/cj", "/cj/j", "/cj/j/runs"} {
		rec := doRequest(eYes, http.MethodGet, path, nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s nil store status=%d %s", path, rec.Code, rec.Body.String())
		}
	}

	_ = e
}
