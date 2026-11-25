// 文件说明：mcp_builtin_session.go — internal/api 的内部实现（internalPrincipalCtxKey、withInternalPrincipal 等）
//
// 用法
//
//	同包调用：internalPrincipalFrom() → mcp_dispatch.go；withInternalPrincipalAuth() → router.go；attachBuiltinMCP() → routes_v1.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/mcp_builtin_session.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] internalPrincipalFrom ← mcp_dispatch.go；withInternalPrincipalAuth ← router.go；attachBuiltinMCP ← routes_v1.go
//
// 主要符号
//
//	type internalPrincipalCtxKey struct{…}
//	withInternalPrincipal(ctx context.Context, p auth.Principal) context.Context
//	internalPrincipalFrom(ctx context.Context) (auth.Principal, bool)
//	withInternalPrincipalAuth(inner echo.MiddlewareFunc) echo.MiddlewareFunc
//	(h *builtinMCPHandler) Spec(ctx context.Context, p auth.Principal, projectID, name string) (*mcp.ServerSpec, b...
//	(h *builtinMCPHandler) Policy(name, tool string) (cloudagent.ToolDecision, bool)
//	(h *builtinMCPHandler) policyTable(name string) (map[string]cloudagent.ToolDecision, bool)
//	attachBuiltinMCP(rt *cloudagent.Runtime, b agentbridge.BuiltinMCP)

// mcp_builtin_session.go — 内置 MCP 的进程内会话支持（planv6.7 §3.6）
//
// 云助手不经 HTTP 回连本机：agentbridge.MCPConnector 通过 BuiltinMCP 取到 spec，
// 用 mcp.ConnectInMemory 直连。进程内没有 Authorization 头，主体改由 context 携带。
package api

import (
	"context"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// internalPrincipalCtxKey 的类型不导出，只有本包能写入；外部请求没有任何途径
// 把值放进 context，因此不存在通过请求头伪造主体的入口。
type internalPrincipalCtxKey struct{}

func withInternalPrincipal(ctx context.Context, p auth.Principal) context.Context {
	return context.WithValue(ctx, internalPrincipalCtxKey{}, p)
}

func internalPrincipalFrom(ctx context.Context) (auth.Principal, bool) {
	p, ok := ctx.Value(internalPrincipalCtxKey{}).(auth.Principal)
	return p, ok
}

// withInternalPrincipalAuth 让进程内子请求跳过头部鉴权，直接使用 context 里的主体。
// 其余请求原样走 inner。
func withInternalPrincipalAuth(inner echo.MiddlewareFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		authed := inner(next)
		return func(c echo.Context) error {
			if p, ok := internalPrincipalFrom(c.Request().Context()); ok {
				c.SetRequest(c.Request().WithContext(WithPrincipal(c.Request().Context(), p)))
				return next(c)
			}
			return authed(c)
		}
	}
}

var _ agentbridge.BuiltinMCP = (*builtinMCPHandler)(nil)

// Spec 实现 agentbridge.BuiltinMCP：返回绑定了主体的 spec 副本。
//
// 进程内会话的服务端 ctx 不继承客户端调用的 ctx，所以主体必须绑进工具
// Handler 的闭包，而不能依赖调用时的 ctx。缓存里的 spec 不被修改。
func (h *builtinMCPHandler) Spec(ctx context.Context, p auth.Principal, projectID, name string) (*mcp.ServerSpec, bool) {
	if catalog.IsSystemProject(projectID) {
		return nil, false
	}
	spec, ok := h.resolveSpec(name, ProjectContext{ID: projectID, TenantID: p.TenantID})
	if !ok {
		return nil, false
	}
	enabled, err := h.checkProjectSetting(ctx, projectID, name)
	if err != nil || !enabled {
		return nil, false
	}
	bound := *spec
	bound.Tools = make([]mcp.ServerTool, len(spec.Tools))
	for i, t := range spec.Tools {
		inner := t.Handler
		t.Handler = func(ctx context.Context, arguments []byte) (mcp.CallResult, error) {
			return inner(withInternalPrincipal(ctx, p), arguments)
		}
		bound.Tools[i] = t
	}
	return &bound, true
}

// Policy 实现 agentbridge.BuiltinMCP：事实来源是工具表的 ReadOnly / Destructive。
// name 是内置 Server 但 tool 未登记时按「写」处理（fail-closed）。
func (h *builtinMCPHandler) Policy(name, tool string) (cloudagent.ToolDecision, bool) {
	table, ok := h.policyTable(name)
	if !ok {
		return cloudagent.ToolDecision{}, false
	}
	if d, found := table[tool]; found {
		return d, true
	}
	return cloudagent.ToolDecision{ReadOnly: false, Destructive: false}, true
}

// policyTable 按 Server 名懒构建并缓存工具的读写判定。工具名与读写属性与项目无关。
func (h *builtinMCPHandler) policyTable(name string) (map[string]cloudagent.ToolDecision, bool) {
	h.mu.RLock()
	table, ok := h.policies[name]
	h.mu.RUnlock()
	if ok {
		return table, true
	}
	var entry *builtinMCP
	for i := range builtinMCPs {
		if builtinMCPs[i].Name == name {
			entry = &builtinMCPs[i]
			break
		}
	}
	if entry == nil {
		return nil, false
	}
	spec := entry.Build(&builtinMCPBuilder{h: h}, ProjectContext{})
	if spec == nil {
		return nil, false
	}
	table = make(map[string]cloudagent.ToolDecision, len(spec.Tools))
	for _, t := range spec.Tools {
		table[t.Name] = cloudagent.ToolDecision{ReadOnly: t.ReadOnly, Destructive: t.Destructive}
	}
	h.mu.Lock()
	h.policies[name] = table
	h.mu.Unlock()
	return table, true
}

// attachBuiltinMCP 把内置 MCP 注入云助手运行时的连接器与策略。
// 必须在服务开始接收请求之前调用（NewRouter 内完成）。
func attachBuiltinMCP(rt *cloudagent.Runtime, b agentbridge.BuiltinMCP) {
	if rt == nil {
		return
	}
	type builtinSetter interface{ SetBuiltin(agentbridge.BuiltinMCP) }
	if s, ok := rt.MCP.(builtinSetter); ok {
		s.SetBuiltin(b)
	}
	if s, ok := rt.MCPPolicy.(builtinSetter); ok {
		s.SetBuiltin(b)
	}
}
