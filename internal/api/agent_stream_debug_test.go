// 文件说明：agent_stream_debug_test.go — internal/api 的单元测试（TestDbg16 等）
//
// 用法
//
//	go test ./internal/api -run 'TestDbg16'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_stream_debug_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestDbg16(t *testing.T)

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
)

func TestDbg16(t *testing.T) {
	env := newSkillRouterEnv(t)
	env.llm.reset([][]cloudagent.StreamDelta{
		toolDeltas("c1", []string{"function", "list"}),
		textDeltas("done"),
	})
	thread := env.createThreadFor(t, "gofunction")
	body, _ := json.Marshal(map[string]any{"content": "x", "stream": true})
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/agent-threads/"+thread+"/runs", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-router-e2e-key")
	rec := httptest.NewRecorder()
	env.echo.ServeHTTP(rec, req)
	t.Logf("code=%d", rec.Code)
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if strings.Contains(line, "tool_result") || strings.Contains(line, "\"error\"") {
			t.Logf("%s", line)
		}
	}
}
