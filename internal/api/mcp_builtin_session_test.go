// 文件说明：mcp_builtin_session_test.go — internal/api 的单元测试（TestBuiltinMCPSession_ReadWithContextPrincipal、TestBuiltinMCPSession_ReadOnlyPrincipalCannotWrite 等）
//
// 用法
//
//	go test ./internal/api -run 'TestBuiltinMCPSession_ReadWithContextPrincipal|TestBuiltinMCPSession_ReadOnlyPrincipalCannotWrite|TestBuiltinMCPSession_SpecGates|TestBuiltinMCPSession_PolicyTable'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/mcp_builtin_session_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	sessionPrincipal(perms ...auth.Permission) auth.Principal
//	builtinForTest(t *testing.T) (*builtinMCPHandler, Dependencies)
//	callSessionTool(t *testing.T, spec *mcp.ServerSpec, tool, args string) mcp.CallResult
//	TestBuiltinMCPSession_ReadWithContextPrincipal(t *testing.T)
//	TestBuiltinMCPSession_ReadOnlyPrincipalCannotWrite(t *testing.T)
//	TestBuiltinMCPSession_SpecGates(t *testing.T)
//	TestBuiltinMCPSession_PolicyTable(t *testing.T)
//	TestInternalPrincipalCannotBeForgedFromHTTP(t *testing.T)

package api

import (
	"context"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

func sessionPrincipal(perms ...auth.Permission) auth.Principal {
	set := make(map[auth.Permission]struct{}, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return auth.Principal{
		APIKeyID:    "key-1",
		TenantID:    "tenant-1",
		ProjectIDs:  map[string]struct{}{"proj-1": {}},
		Permissions: set,
		Delegated:   true,
	}
}

func builtinForTest(t *testing.T) (*builtinMCPHandler, Dependencies) {
	t.Helper()
	e, deps, _ := builtinMCPRouter(t, true)
	_ = e
	return newBuiltinMCPHandler(e, deps), deps
}

func callSessionTool(t *testing.T, spec *mcp.ServerSpec, tool, args string) mcp.CallResult {
	t.Helper()
	for _, st := range spec.Tools {
		if st.Name != tool {
			continue
		}
		res, err := st.Handler(context.Background(), []byte(args))
		if err != nil {
			t.Fatalf("%s: handler error: %v", tool, err)
		}
		return res
	}
	t.Fatalf("tool %s not found", tool)
	return mcp.CallResult{}
}

func TestBuiltinMCPSession_ReadWithContextPrincipal(t *testing.T) {
	h, _ := builtinForTest(t)
	p := sessionPrincipal(auth.DatabaseRead)
	spec, ok := h.Spec(context.Background(), p, "proj-1", "simplebase")
	if !ok {
		t.Fatal("expected builtin spec")
	}
	res := callSessionTool(t, spec, "list_databases", `{}`)
	if res.IsError || !strings.Contains(res.Text, "db-1") {
		t.Fatalf("unexpected result: %+v", res)
	}
}

// 只读（已剥离写权限）的主体经进程内会话调写工具，必须被路由层 403 挡下，
// 且作为 isError 结果返回，而不是协议错误或静默成功。
func TestBuiltinMCPSession_ReadOnlyPrincipalCannotWrite(t *testing.T) {
	h, _ := builtinForTest(t)
	p := sessionPrincipal(auth.DatabaseRead)
	spec, ok := h.Spec(context.Background(), p, "proj-1", "simplebase")
	if !ok {
		t.Fatal("expected builtin spec")
	}
	res := callSessionTool(t, spec, "sql_execute", `{"sql":"create table t(id int)"}`)
	if !res.IsError {
		t.Fatalf("expected isError for write by read-only principal: %+v", res)
	}
}

func TestBuiltinMCPSession_SpecGates(t *testing.T) {
	h, deps := builtinForTest(t)
	p := sessionPrincipal(auth.DatabaseRead)

	if _, ok := h.Spec(context.Background(), p, "proj-1", "nope"); ok {
		t.Fatal("unknown name must not resolve")
	}
	if _, ok := h.Spec(context.Background(), p, catalog.ReservedSystemProjectID, "simplebase"); ok {
		t.Fatal("system project must not resolve")
	}
	if err := deps.System.PutProjectSetting(context.Background(), "proj-1", "mcp.builtin.simplebase.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Spec(context.Background(), p, "proj-1", "simplebase"); ok {
		t.Fatal("project-level disable must not resolve")
	}
}

func TestBuiltinMCPSession_PolicyTable(t *testing.T) {
	h, _ := builtinForTest(t)

	if d, ok := h.Policy("simplebase", "list_databases"); !ok || !d.ReadOnly {
		t.Fatalf("list_databases should be read-only: %+v ok=%v", d, ok)
	}
	if d, ok := h.Policy("simplebase", "sql_execute"); !ok || d.ReadOnly {
		t.Fatalf("sql_execute must not be read-only: %+v ok=%v", d, ok)
	}
	// 内置 Server 里未登记的工具：ok=true 且按写处理（fail-closed）。
	if d, ok := h.Policy("simplebase", "no_such_tool"); !ok || d.ReadOnly {
		t.Fatalf("unknown tool must be treated as write: %+v ok=%v", d, ok)
	}
	// 非内置 Server：交回外部策略表。
	if _, ok := h.Policy("external-x", "t"); ok {
		t.Fatal("non-builtin server must report ok=false")
	}
}

// 请求头无法伪造进程内主体：外部请求带不上 context 值，仍走正常鉴权。
func TestInternalPrincipalCannotBeForgedFromHTTP(t *testing.T) {
	e, _, _ := builtinMCPRouter(t, true)
	rec := mcpPost(t, e, "/v1/projects/proj-1/mcp", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if rec.Code != 401 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
