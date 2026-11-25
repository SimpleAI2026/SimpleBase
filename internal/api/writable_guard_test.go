// 文件说明：writable_guard_test.go — internal/api 的单元测试（TestWritableGuardsRejectAgentAndStorageWrites 等）
//
// 用法
//
//	go test ./internal/api -run 'TestWritableGuardsRejectAgentAndStorageWrites'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/writable_guard_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestWritableGuardsRejectAgentAndStorageWrites(t *testing.T)

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestWritableGuardsRejectAgentAndStorageWrites(t *testing.T) {
	off := false
	e := echo.New()
	agents := &cloudAgentHandler{writable: off}
	storage := &S3Handler{writable: off}
	e.POST("/agents", agents.CreateAgent)
	e.PATCH("/agents/:agentID", agents.PatchAgent)
	e.DELETE("/agents/:agentID", agents.DeleteAgent)
	e.POST("/agent-threads", agents.CreateThread)
	e.DELETE("/agent-threads/:threadID", agents.DeleteThread)
	e.POST("/s3/objects", storage.UploadObject)
	e.DELETE("/s3/objects", storage.DeleteObject)

	cases := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/agents", `{"name":"a","module":"general"}`},
		{http.MethodPatch, "/agents/a1", `{"name":"b"}`},
		{http.MethodDelete, "/agents/a1", ``},
		{http.MethodPost, "/agent-threads", `{"title":"t"}`},
		{http.MethodDelete, "/agent-threads/t1", ``},
		{http.MethodPost, "/s3/objects", ``},
		{http.MethodDelete, "/s3/objects?key=readme", ``},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "writer_unavailable") {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}
