// 文件说明：mcp_handler.go — internal/api 的内部实现（mcpServerHandler、mcpServerDTO 等）
//
// 用法
//
//	同包调用：mcpServerHandler → mcp_builtin_admin.go, ；mcpTestResult → mcp_builtin_admin.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/mcp_handler.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backup_handler.go, data_compat.go, error.go, file_compat.go, function_compat.go, health.go, mcp_builtin.go, mcp_builtin_admin.go 等 28 个
//	[关键引用] mcpServerHandler ← mcp_builtin_admin.go, ；mcpTestResult ← mcp_builtin_admin.go
//
// 主要符号
//
//	type mcpServerHandler struct{…}
//	var ErrMCPNameReserved
//	type mcpServerDTO struct{…}
//	toMCPServerDTO(m systemdb.MCPServer) mcpServerDTO
//	(h *mcpServerHandler) List(c echo.Context) error
//	type mcpServerBody struct{…}
//	(h *mcpServerHandler) Create(c echo.Context) error
//	(h *mcpServerHandler) Update(c echo.Context) error
//	... 另有 7 个顶层符号

// mcp_handler.go — 项目级 MCP Server 配置的 HTTP 面（planv6.2 §6-2）。
//
// 凭据口径：headers 只单向写，响应只回 has_credential 与脱敏后的 header 名，
// 原文不出网、不进日志。tools_policy 是权限判定的唯一事实来源。
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type mcpServerHandler struct {
	store *systemdb.Store
	// limit 是单项目 Server 上限；<=0 表示不限制。
	limit int
	// testConnect 做一次临时连通性测试；为 nil 时 test 接口返回 503（MCP 未启用）。
	testConnect func(ctx context.Context, srv systemdb.MCPServer) ([]string, error)
	// builtin 为 nil 表示实例级开关关闭（agent.mcp.builtin=false）。
	builtin *builtinMCPHandler
	audit   httpkit.AuditService
	// publicURL / publicOverride 用于拼内置 MCP 的对外端点（httpkit.PublicBaseURL 三级回退）。
	publicURL, publicOverride string
}

// httpkit.ErrMCPNameReserved 表示 Server 名占用了内置 MCP 的保留名（simplebase、sb-*）。

type mcpServerDTO struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Transport     string         `json:"transport"`
	URL           string         `json:"url"`
	TimeoutS      int            `json:"timeout_s"`
	Enabled       bool           `json:"enabled"`
	ToolsPolicy   map[string]any `json:"tools_policy"`
	HeaderNames   []string       `json:"header_names"`
	HasCredential bool           `json:"has_credential"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	// NameConflict 标记历史配置占用了内置保留名；Agent 引用时内置 Server 优先。
	NameConflict bool              `json:"name_conflict,omitempty"`
	Extra        map[string]string `json:"-"`
}

// toMCPServerDTO 把存储行转成对外 DTO：headers 只暴露名字，不暴露值。
func toMCPServerDTO(m systemdb.MCPServer) mcpServerDTO {
	headers := systemdb.ParseMCPHeaders(m.HeadersJSON)
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	policies := map[string]any{}
	if strings.TrimSpace(m.ToolsPolicyJSON) != "" {
		_ = json.Unmarshal([]byte(m.ToolsPolicyJSON), &policies)
	}
	return mcpServerDTO{
		ID:          m.ID,
		Name:        m.Name,
		Transport:   m.Transport,
		URL:         m.URL,
		TimeoutS:    m.TimeoutS,
		Enabled:     m.Enabled,
		ToolsPolicy: policies,
		HeaderNames: names,
		// 有任一 Header 就视为带凭据；具体值永不回显。
		HasCredential: len(headers) > 0,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
		NameConflict:  reservedMCPName(m.Name),
	}
}

func (h *mcpServerHandler) List(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	list, err := h.store.ListMCPServers(c.Request().Context(), pc.ID)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	out := make([]mcpServerDTO, 0, len(list))
	for _, m := range list {
		out = append(out, toMCPServerDTO(m))
	}
	return c.JSON(http.StatusOK, map[string]any{"servers": out, "builtin": h.builtinList(c, pc)})
}

type mcpServerBody struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	TimeoutS    int               `json:"timeout_s"`
	Enabled     *bool             `json:"enabled"`
	ToolsPolicy map[string]any    `json:"tools_policy"`
}

func (h *mcpServerHandler) Create(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.writableGuard(c) != nil {
		return nil
	}
	var body mcpServerBody
	if err := c.Bind(&body); err != nil {
		return httpkit.WriteError(c, err)
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "name is required"))
	}
	if !validMCPName(name) {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "name must match [A-Za-z0-9_-]+"))
	}
	if reservedMCPName(name) {
		return httpkit.WriteError(c, httpkit.ErrMCPNameReserved)
	}
	url := strings.TrimSpace(body.URL)
	if url == "" {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "url is required"))
	}
	// 上限校验：避免单项目把配置表撑爆（§6-4）。
	n, err := h.store.CountMCPServers(c.Request().Context(), pc.ID)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if limit := h.maxServers(); limit > 0 && n >= limit {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "too many mcp servers for this project"))
	}
	headersJSON, err := json.Marshal(body.Headers)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	policyJSON, err := json.Marshal(body.ToolsPolicy)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	saved, err := h.store.CreateMCPServer(c.Request().Context(), systemdb.MCPServer{
		ProjectID:       pc.ID,
		Name:            name,
		Transport:       systemdb.MCPTransportHTTP,
		URL:             url,
		HeadersJSON:     string(headersJSON),
		TimeoutS:        body.TimeoutS,
		Enabled:         enabled,
		ToolsPolicyJSON: string(policyJSON),
	})
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toMCPServerDTO(saved))
}

func (h *mcpServerHandler) Update(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.writableGuard(c) != nil {
		return nil
	}
	id := c.Param("serverID")
	cur, err := h.store.GetMCPServer(c.Request().Context(), pc.ID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusNotFound, "mcp server not found"))
	}
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	var body mcpServerBody
	if err := c.Bind(&body); err != nil {
		return httpkit.WriteError(c, err)
	}
	if v := strings.TrimSpace(body.Name); v != "" {
		if !validMCPName(v) {
			return httpkit.WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "name must match [A-Za-z0-9_-]+"))
		}
		if reservedMCPName(v) && !strings.EqualFold(v, cur.Name) {
			return httpkit.WriteError(c, httpkit.ErrMCPNameReserved)
		}
		cur.Name = v
	}
	if v := strings.TrimSpace(body.URL); v != "" {
		cur.URL = v
	}
	// headers 传 null 表示沿用既有值（前端只拿到名字，不回传原文）。
	if body.Headers != nil {
		b, err := json.Marshal(body.Headers)
		if err != nil {
			return httpkit.WriteError(c, err)
		}
		cur.HeadersJSON = string(b)
	}
	if body.ToolsPolicy != nil {
		b, err := json.Marshal(body.ToolsPolicy)
		if err != nil {
			return httpkit.WriteError(c, err)
		}
		cur.ToolsPolicyJSON = string(b)
	}
	if body.TimeoutS > 0 {
		cur.TimeoutS = body.TimeoutS
	}
	if body.Enabled != nil {
		cur.Enabled = *body.Enabled
	}
	saved, err := h.store.UpdateMCPServer(c.Request().Context(), cur)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toMCPServerDTO(saved))
}

func (h *mcpServerHandler) Delete(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.writableGuard(c) != nil {
		return nil
	}
	if err := h.store.ArchiveMCPServer(c.Request().Context(), pc.ID, c.Param("serverID")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return httpkit.WriteError(c, echo.NewHTTPError(http.StatusNotFound, "mcp server not found"))
		}
		return httpkit.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// testResult 是连通性测试的结果：只回工具名与判定，不回凭据。
type mcpTestResult struct {
	OK    bool     `json:"ok"`
	Tools []string `json:"tools"`
	Error string   `json:"error,omitempty"`
}

// Test 建立一次临时连接并列出工具（planv6.2 §6-2）。
func (h *mcpServerHandler) Test(c echo.Context) error {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	if h.testConnect == nil {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusServiceUnavailable, "mcp is not enabled"))
	}
	srv, err := h.store.GetMCPServer(c.Request().Context(), pc.ID, c.Param("serverID"))
	if errors.Is(err, sql.ErrNoRows) {
		return httpkit.WriteError(c, echo.NewHTTPError(http.StatusNotFound, "mcp server not found"))
	}
	if err != nil {
		return httpkit.WriteError(c, err)
	}
	// 测试有明确超时上限：不跟随 Server 配置里的长超时，避免挂住请求。
	ctx, cancel := context.WithTimeout(c.Request().Context(), mcp.DefaultConnectTimeout*2)
	defer cancel()
	tools, err := h.testConnect(ctx, srv)
	if err != nil {
		// 错误只回稳定分类，不回上游原文（可能含 URL 或凭据片段）。
		return c.JSON(http.StatusOK, mcpTestResult{OK: false, Error: "connect_failed"})
	}
	return c.JSON(http.StatusOK, mcpTestResult{OK: true, Tools: tools})
}

// writableGuard 是写接口的前置检查：只读实例拒绝写入。
func (h *mcpServerHandler) writableGuard(c echo.Context) error {
	p, ok := httpkit.PrincipalFromContext(c.Request().Context())
	if !ok {
		return nil
	}
	if !canWriteMCP(p) {
		_ = httpkit.WriteError(c, echo.NewHTTPError(http.StatusForbidden, "write permission required"))
		return errors.New("forbidden")
	}
	return nil
}

func (h *mcpServerHandler) maxServers() int { return h.limit }

// canWriteMCP 与 cloudAgentHandler.canWritePrincipal 同口径：
// 登录态看角色，API Key 看 database:write 权限。
func canWriteMCP(p auth.Principal) bool {
	if p.UserID != "" {
		return p.Role.CanWrite()
	}
	return p.HasPermission(auth.DatabaseWrite)
}

// validMCPName 限制 Server 名称字符集：它会被拼进 mcp__<server>__<tool>。
func validMCPName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
