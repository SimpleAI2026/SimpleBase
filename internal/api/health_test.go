// 文件说明：health_test.go — internal/api 的单元测试（TestHealthHandler_NilChecker、TestHealthHandler_Success 等）
//
// 用法
//
//	go test ./internal/api -run 'TestHealthHandler_NilChecker|TestHealthHandler_Success|TestHealthHandler_Failure'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/health_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	serveHealth(h *HealthHandler, method, path string) *httptest.ResponseRecorder
//	TestHealthHandler_NilChecker(t *testing.T)
//	TestHealthHandler_Success(t *testing.T)
//	TestHealthHandler_Failure(t *testing.T)

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
