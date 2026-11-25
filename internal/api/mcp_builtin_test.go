// 文件说明：mcp_builtin_test.go — internal/api 的单元测试（TestBuiltinMCP_Disabled、TestBuiltinMCP_RequiresAuth 等）
//
// 用法
//
//	go test ./internal/api -run 'TestBuiltinMCP_Disabled|TestBuiltinMCP_RequiresAuth|TestBuiltinMCP_ListToolsAndCall|TestBuiltinMCP_SubRequestKeepsRouteErrors'
//	同包调用：mcpPost() → mcp_builtin_admin_test.go, mcp_builtin_session_test.go；builtinMCPRouter() → mcp_builtin_admin_test.go, mcp_builtin_session_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/mcp_builtin_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] mcpPost ← mcp_builtin_admin_test.go, mcp_builtin_session_test.go；builtinMCPRouter ← mcp_builtin_admin_test.go, mcp_builtin_session_test.go
//
// 主要符号
//
//	const mcpAccept
//	mcpPost(t *testing.T, e *echo.Echo, path, key, body string) *httptest.ResponseRecorder
//	builtinMCPRouter(t *testing.T, builtin bool) (*echo.Echo, Dependencies, string)
//	TestBuiltinMCP_Disabled(t *testing.T)
//	TestBuiltinMCP_RequiresAuth(t *testing.T)
//	TestBuiltinMCP_ListToolsAndCall(t *testing.T)
//	TestBuiltinMCP_SubRequestKeepsRouteErrors(t *testing.T)
//	TestBuiltinMCP_UnknownName(t *testing.T)
//	... 另有 4 个顶层符号

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

const mcpAccept = "application/json, text/event-stream"

func mcpPost(t *testing.T, e *echo.Echo, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Accept", mcpAccept)
	if key != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func builtinMCPRouter(t *testing.T, builtin bool) (*echo.Echo, Dependencies, string) {
	t.Helper()
	deps, key := fullRouterDeps(t)
	deps.Config.Agent.MCP.Builtin = builtin
	return NewRouter(deps), deps, key
}

func TestBuiltinMCP_Disabled(t *testing.T) {
	e, _, key := builtinMCPRouter(t, false)
	rec := mcpPost(t, e, "/v1/projects/proj-1/mcp", key, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuiltinMCP_RequiresAuth(t *testing.T) {
	e, _, _ := builtinMCPRouter(t, true)
	rec := mcpPost(t, e, "/v1/projects/proj-1/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuiltinMCP_ListToolsAndCall(t *testing.T) {
	e, _, key := builtinMCPRouter(t, true)

	for _, path := range []string{"/v1/projects/proj-1/mcp", "/v1/projects/proj-1/mcp/builtin/simplebase"} {
		rec := mcpPost(t, e, path, key, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s tools/list status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		var list struct {
			Result struct {
				Tools []struct {
					Name        string         `json:"name"`
					Annotations map[string]any `json:"annotations"`
				} `json:"tools"`
			} `json:"result"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode: %v body=%s", err, rec.Body.String())
		}
		names := map[string]bool{}
		for _, tool := range list.Result.Tools {
			names[tool.Name] = true
		}
		for _, want := range []string{"list_databases", "sql_query", "sql_execute", "kv_command"} {
			if !names[want] {
				t.Fatalf("%s: missing tool %s in %v", path, want, names)
			}
		}
	}

	// 工具调用经子请求走真实路由：list_databases -> GET /v1/projects/proj-1/databases。
	rec := mcpPost(t, e, "/v1/projects/proj-1/mcp", key,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_databases","arguments":{}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("call status=%d body=%s", rec.Code, rec.Body.String())
	}
	var call struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &call); err != nil {
		t.Fatalf("decode call: %v body=%s", err, rec.Body.String())
	}
	if call.Result.IsError || len(call.Result.Content) == 0 || !strings.Contains(call.Result.Content[0].Text, "db-1") {
		t.Fatalf("unexpected call result: %s", rec.Body.String())
	}
}

func TestBuiltinMCP_SubRequestKeepsRouteErrors(t *testing.T) {
	e, _, key := builtinMCPRouter(t, true)
	// 缺少必填的 sql 参数：子请求返回 4xx，应作为 isError 结果回给模型，而不是协议错误。
	rec := mcpPost(t, e, "/v1/projects/proj-1/mcp", key,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"sql_query","arguments":{"sql":""}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"isError":true`) {
		t.Fatalf("expected tool-level isError result: %s", rec.Body.String())
	}
}

func TestBuiltinMCP_UnknownName(t *testing.T) {
	e, _, key := builtinMCPRouter(t, true)
	rec := mcpPost(t, e, "/v1/projects/proj-1/mcp/builtin/nope", key, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuiltinMCP_ProjectSettingDisables(t *testing.T) {
	e, deps, key := builtinMCPRouter(t, true)
	if err := deps.System.PutProjectSetting(t.Context(), "proj-1", "mcp.builtin.simplebase.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	rec := mcpPost(t, e, "/v1/projects/proj-1/mcp", key, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuiltinMCP_GetAndDeleteNotAllowed(t *testing.T) {
	e, _, key := builtinMCPRouter(t, true)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req := httptest.NewRequest(method, "/v1/projects/proj-1/mcp", nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status=%d body=%s", method, rec.Code, rec.Body.String())
		}
	}
}

func TestDispatchBuiltinTool_NoCaller(t *testing.T) {
	_, err := dispatchBuiltinTool(t.Context(), echo.New(), Dependencies{}, ProjectContext{ID: "p"}, builtinAPITool{Name: "x", Method: http.MethodGet, Path: "/x", BodyArg: "-"}, nil)
	if err == nil {
		t.Fatal("expected error without caller identity")
	}
}

func TestMCPScalarArg(t *testing.T) {
	args := map[string]json.RawMessage{
		"s": json.RawMessage(`"a b"`), "n": json.RawMessage(`12`), "z": json.RawMessage(`null`), "o": json.RawMessage(`{"x":1}`),
	}
	if v, err := mcpScalarArg(args, "s"); err != nil || v != "a b" {
		t.Fatalf("s=%q err=%v", v, err)
	}
	if v, err := mcpScalarArg(args, "n"); err != nil || v != "12" {
		t.Fatalf("n=%q err=%v", v, err)
	}
	if v, err := mcpScalarArg(args, "z"); err != nil || v != "" {
		t.Fatalf("z=%q err=%v", v, err)
	}
	if v, err := mcpScalarArg(args, "missing"); err != nil || v != "" {
		t.Fatalf("missing=%q err=%v", v, err)
	}
	if _, err := mcpScalarArg(args, "o"); err == nil {
		t.Fatal("object must be rejected as scalar")
	}
}
