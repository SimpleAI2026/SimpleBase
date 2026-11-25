// 文件说明：mcp_builtin.go — internal/api 的内部实现（builtinAPITool、builtinMCP 等）
//
// 用法
//
//	同包调用：builtinAPITool → mcp_builtin_simplebase.go, mcp_builtin_test.go；builtinMCP → mcp_builtin_session.go, mcp_builtin_simplebase.go；builtinMCPHandler → mcp_builtin_session.go, mcp_builtin_session_test.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/mcp_builtin.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//	[关键引用] builtinAPITool ← mcp_builtin_simplebase.go, mcp_builtin_test.go；builtinMCP ← mcp_builtin_session.go, mcp_builtin_simplebase.go；builtinMCPHandler ← mcp_builtin_session.go, mcp_builtin_session_test.go
//
// 主要符号
//
//	type builtinAPITool struct{…}
//	type builtinMCP struct{…}
//	var builtinMCPs
//	type builtinMCPHandler struct{…}
//	type cachedSpec struct{…}
//	newBuiltinMCPHandler(e *echo.Echo, deps Dependencies) *builtinMCPHandler
//	(h *builtinMCPHandler) Serve(c echo.Context) error
//	(h *builtinMCPHandler) MethodNotAllowed(c echo.Context) error
//	... 另有 6 个顶层符号

// mcp_builtin.go — 内置 MCP Server 的注册表与 HTTP 端点（planv6.7 §3.4）
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// builtinAPITool 是「一条 /v1 路由的薄封装」的工具定义（§3.3）。
type builtinAPITool struct {
	Name, Description string
	Method            string   // GET / POST / PUT / PATCH / DELETE
	Path              string   // 例 "/v1/projects/{projectID}/databases/{databaseID}/query"
	PathArgs          []string // 从 arguments 取出并 PathEscape 的路径参数（不含 projectID）
	QueryArgs         []string // 拼进 query 的参数
	BodyArg           string   // 作为 JSON body 的参数名；"" = 整个剩余 arguments；"-" = 无 body
	Schema            json.RawMessage
	ReadOnly          bool
	Destructive       bool
}

// builtinMCP 是一个内置 MCP Server 的注册项；新增内置 MCP 只需往 builtinMCPs 追加。
type builtinMCP struct {
	Name      string // simplebase / sb-xxx
	TitleKey  string // 前端文案 key
	Available func(deps Dependencies) bool
	Build     func(b *builtinMCPBuilder, pc ProjectContext) *mcp.ServerSpec
}

var builtinMCPs = []builtinMCP{simplebaseAPIMCP}

// builtinMCPHandler 持有完整路由树 *echo.Echo，负责解析请求、鉴权与 spec 缓存。
type builtinMCPHandler struct {
	e         *echo.Echo
	deps      Dependencies
	mu        sync.RWMutex
	specCache map[string]*cachedSpec // key = (pid, name)
	policies  map[string]map[string]cloudagent.ToolDecision
}

type cachedSpec struct {
	spec      *mcp.ServerSpec
	expiresAt time.Time
}

func newBuiltinMCPHandler(e *echo.Echo, deps Dependencies) *builtinMCPHandler {
	return &builtinMCPHandler{
		e: e, deps: deps,
		specCache: make(map[string]*cachedSpec),
		policies:  make(map[string]map[string]cloudagent.ToolDecision),
	}
}

// Serve 是 `POST /v1/projects/<pid>/mcp/builtin/<name>` 和 `POST /v1/projects/<pid>/mcp` 的统一入口。
func (h *builtinMCPHandler) Serve(c echo.Context) error {
	name := c.Param("name")
	if name == "" {
		name = "simplebase" // `/mcp` = `/mcp/builtin/simplebase`
	}
	if !h.deps.Config.Agent.MCP.Builtin {
		return c.JSON(http.StatusNotFound, map[string]any{"status": 404, "message": "builtin mcp not enabled"})
	}
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	// 系统项目禁止 MCP 暴露（§3.4 #3）。
	if catalog.IsSystemProject(pc.ID) {
		return c.JSON(http.StatusNotFound, map[string]any{"status": 404, "message": "builtin mcp not available for system project"})
	}
	spec, found := h.resolveSpec(name, pc)
	if !found {
		return c.JSON(http.StatusNotFound, map[string]any{"status": 404, "message": "builtin mcp not found"})
	}
	// 项目级开关（§3.4 #2）：`mcp.builtin.<name>.enabled`。
	enabled, err := h.checkProjectSetting(c.Request().Context(), pc.ID, name)
	if err != nil {
		return WriteError(c, err)
	}
	if !enabled {
		return c.JSON(http.StatusNotFound, map[string]any{"status": 404, "message": "builtin mcp disabled for this project"})
	}
	// 交给 SDK handler；主体来自子请求的 Authorization 头，不从 spec 闭包绑定。
	req := c.Request()
	caller := mcpCaller{
		Authorization: req.Header.Get(echo.HeaderAuthorization),
		RemoteAddr:    req.RemoteAddr,
		RequestID:     c.Response().Header().Get(echo.HeaderXRequestID),
	}
	httpHandler := mcp.NewHTTPHandler(func(*http.Request) *mcp.ServerSpec { return spec })
	httpHandler.ServeHTTP(c.Response(), req.WithContext(withMCPCaller(req.Context(), caller)))
	return nil
}

// MethodNotAllowed 处理 GET / DELETE 返回 405 JSON。
func (h *builtinMCPHandler) MethodNotAllowed(c echo.Context) error {
	return c.JSON(http.StatusMethodNotAllowed, map[string]any{
		"status":  http.StatusMethodNotAllowed,
		"message": "method not allowed; use POST for MCP protocol",
	})
}

// resolveSpec 按 (name, pid) 返回 spec；缓存 60 秒。未找到注册或 Available=false 返回 nil。
func (h *builtinMCPHandler) resolveSpec(name string, pc ProjectContext) (*mcp.ServerSpec, bool) {
	cacheKey := pc.ID + ":" + name
	now := time.Now()
	h.mu.RLock()
	cached, ok := h.specCache[cacheKey]
	if ok && now.Before(cached.expiresAt) {
		defer h.mu.RUnlock()
		return cached.spec, true
	}
	h.mu.RUnlock()

	var entry *builtinMCP
	for i := range builtinMCPs {
		if builtinMCPs[i].Name == name {
			entry = &builtinMCPs[i]
			break
		}
	}
	if entry == nil || !entry.Available(h.deps) {
		return nil, false
	}
	builder := &builtinMCPBuilder{h: h}
	spec := entry.Build(builder, pc)
	if spec == nil {
		return nil, false
	}
	h.mu.Lock()
	h.specCache[cacheKey] = &cachedSpec{spec: spec, expiresAt: now.Add(60 * time.Second)}
	h.mu.Unlock()
	return spec, true
}

// checkProjectSetting 读取项目级开关 `mcp.builtin.<name>.enabled`；默认 true。
func (h *builtinMCPHandler) checkProjectSetting(ctx context.Context, projectID, name string) (bool, error) {
	if h.deps.System == nil {
		return true, nil
	}
	key := "mcp.builtin." + name + ".enabled"
	kv, err := h.deps.System.GetProjectSetting(ctx, projectID, key)
	if err != nil {
		return false, err
	}
	if kv.ValueJSON == "" {
		return true, nil
	}
	var enabled bool
	if err := json.Unmarshal([]byte(kv.ValueJSON), &enabled); err != nil {
		return true, nil
	}
	return enabled, nil
}

// builtinMCPBuilder 把 builtinAPITool 转成 mcp.ServerTool 并构造 ServerSpec。
type builtinMCPBuilder struct {
	h *builtinMCPHandler
}

// buildServerFromTools 是工具表转 spec 的快捷入口。
func (b *builtinMCPBuilder) buildServerFromTools(tools []builtinAPITool, pc ProjectContext) *mcp.ServerSpec {
	out := make([]mcp.ServerTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, b.toServerTool(t, pc))
	}
	return &mcp.ServerSpec{
		Name:         "simplebase-" + pc.ID,
		Version:      "1",
		Instructions: "SimpleBase project API; databaseID can be omitted for single-database projects.",
		Tools:        out,
	}
}

func (b *builtinMCPBuilder) toServerTool(t builtinAPITool, pc ProjectContext) mcp.ServerTool {
	return mcp.ServerTool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.Schema,
		ReadOnly:    t.ReadOnly,
		Destructive: t.Destructive,
		Handler: func(ctx context.Context, arguments []byte) (mcp.CallResult, error) {
			return dispatchBuiltinTool(ctx, b.h.e, b.h.deps, pc, t, arguments)
		},
	}
}

// reservedMCPName 检查是否为保留的内置 MCP 名（§3.5）：simplebase 和 sb-* 前缀，不区分大小写。
func reservedMCPName(name string) bool {
	lower := strings.ToLower(name)
	return lower == "simplebase" || strings.HasPrefix(lower, "sb-")
}
