// 文件说明：project_missing_test.go — internal/api 的单元测试（TestR2_ProjectMissingErrorsAreEquivalent 等）
//
// 用法
//
//	go test ./internal/api -run 'TestR2_ProjectMissingErrorsAreEquivalent'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/project_missing_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestR2_ProjectMissingErrorsAreEquivalent(t *testing.T)

package api

// r2_project_missing_test.go：planv6.0 R2-A2 表征测试。
// 锁定 echo.NewHTTPError(400, "project context missing") 与 ErrProjectMissing
// 经统一错误映射后的 HTTP 响应完全一致（status/code/message/header），
// 作为两种写法互换的等价证据。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestR2_ProjectMissingErrorsAreEquivalent(t *testing.T) {
	echoErr := echo.NewHTTPError(http.StatusBadRequest, "project context missing")

	a := Error(echoErr, "rid-1")
	b := Error(ErrProjectMissing, "rid-1")
	if a == nil || b == nil {
		t.Fatalf("both must map: %v %v", a, b)
	}
	if a.HTTPStatus != b.HTTPStatus || a.Body != b.Body {
		t.Fatalf("mapping differs:\n echo=%d %+v\n sentinel=%d %+v", a.HTTPStatus, a.Body, b.HTTPStatus, b.Body)
	}
	if a.HTTPStatus != http.StatusBadRequest || a.Body.Error.Code != "invalid_request" || a.Body.Error.Message != "project context missing" {
		t.Fatalf("unexpected mapping: %d %+v", a.HTTPStatus, a.Body)
	}

	// 经 WriteError 写出的完整响应（含 header 与 body）也一致。
	write := func(err error) *httptest.ResponseRecorder {
		e := echo.New()
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		if werr := WriteError(c, err); werr != nil {
			t.Fatal(werr)
		}
		return rec
	}
	ra, rb := write(echoErr), write(ErrProjectMissing)
	if ra.Code != rb.Code || ra.Body.String() != rb.Body.String() {
		t.Fatalf("response differs:\n echo=%d %s\n sentinel=%d %s", ra.Code, ra.Body, rb.Code, rb.Body)
	}
	if ra.Header().Get("Retry-After") != "" || rb.Header().Get("Retry-After") != "" {
		t.Fatal("no Retry-After expected")
	}
}
