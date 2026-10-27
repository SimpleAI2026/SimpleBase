package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

func TestSecurityHeadersAndGzipSkip(t *testing.T) {
	e := echo.New()
	e.Use(securityHeadersMiddleware())
	e.Use(gzipMiddleware())
	e.GET("/v1/projects/p/realtime", func(c echo.Context) error {
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
		return c.String(http.StatusOK, "data: ok\n\n")
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/p/realtime", nil)
	req.Header.Set(echo.HeaderAcceptEncoding, "gzip")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get(echo.HeaderContentEncoding) != "" {
		t.Fatalf("unexpected headers: %#v", rec.Header())
	}
}

func TestRateLimiter429AndExpiry(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newRateLimiter(config.RateLimitConfig{Enabled: true, Rules: []config.RateLimitRule{{Match: "/v1/auth/login", Max: 1, Window: time.Minute}}}, nil)
	limiter.now = func() time.Time { return now }
	e := echo.New()
	e.Use(limiter.middleware())
	e.POST("/v1/auth/login", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	for i, want := range []int{http.StatusNoContent, http.StatusNoContent, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("call %d: got %d want %d", i, rec.Code, want)
		}
		if want == http.StatusTooManyRequests && rec.Header().Get("Retry-After") == "" {
			t.Fatal("missing Retry-After")
		}
	}
	limiter.buckets["stale"] = &rateBucket{seen: now.Add(-time.Hour)}
	limiter.calls = 255
	limiter.allow("new", config.RateLimitRule{Max: 1, Window: time.Second})
	if _, ok := limiter.buckets["stale"]; ok {
		t.Fatal("stale bucket not removed")
	}
}

func TestTrustedIPExtractor(t *testing.T) {
	e := echo.New()
	e.IPExtractor = trustedIPExtractor(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if c.RealIP() == "203.0.113.9" {
		t.Fatal("untrusted XFF was accepted")
	}
}

func TestCollectionImportBodyLimit(t *testing.T) {
	e := echo.New()
	e.POST("/import", func(c echo.Context) error {
		if _, err := io.Copy(io.Discard, c.Request().Body); err != nil {
			return err
		}
		return c.NoContent(http.StatusCreated)
	}, middleware.BodyLimit(bodyLimit(32)))
	req := httptest.NewRequest(http.MethodPost, "/import", strings.NewReader(`{"title":"this body exceeds the configured limit"}`))
	req.Header.Set(echo.HeaderContentType, "application/x-ndjson")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
