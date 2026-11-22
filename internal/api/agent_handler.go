// 文件说明：agent_handler.go — internal/api 的内部实现（cloudAgentHandler、sandboxAvailable 等）
//
// 用法
//
//	type cloudAgentHandler（包内使用）
//	type agentDTO（包内使用）
//	同包调用：cloudAgentHandler → router.go, run_events.go；CancelRun() → router.go, run_events.go；ListModules() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_handler.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, godatabase, internal/agentbridge, internal/auth 等
//	[同包] adapter.go, api_key_policy.go, api_keys_handler.go, auth_handler.go 等 59 个
//	[关键引用] cloudAgentHandler ← router.go, run_events.go
//
// 主要符号
//
//	type cloudAgentHandler struct {…}
//	(h *cloudAgentHandler) sandboxAvailable() bool
//	type agentDTO struct {…}
//	type threadDTO struct {…}
//	type messageDTO struct {…}
//	type runDTO struct {…}
//	... 另有 38 个顶层符号

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

type cloudAgentHandler struct {
	store    *systemdb.Store
	runtime  *cloudagent.Runtime
	usage    UsageService
	audit    AuditService
	writable *bool
	// llm 提供模型列表（BUG-07）；可为 nil（LLM 未启用）。
	llm      LLMService
	events   *runEventHub
	confirms *confirmHub
	// dispatcher 是自动路由的分类器（planv6.7 AGC-ROUTE）；nil 表示 LLM 未
	// 配置，需要路由时直接 fallback。
	dispatcher agentDispatcher
	// routeCancels 登记 runID → 路由期 cancel（AGC-ROUTE §3.3）：
	// CancelRun 同时调用它，保证路由期间按「停止」立即生效。
	routeCancels sync.Map
}

func (h *cloudAgentHandler) provider() *agentbridge.RuntimeProvider {
	if h.runtime == nil {
		return nil
	}
	provider, _ := h.runtime.Provider.(*agentbridge.RuntimeProvider)
	return provider
}

func (h *cloudAgentHandler) skillsCLI() bool {
	provider := h.provider()
	return provider != nil && provider.SkillsCLI
}

func (h *cloudAgentHandler) sandboxAvailable() bool {
	provider := h.provider()
	return provider != nil && provider.Sandbox != nil && provider.Sandbox.Available()
}

type agentDTO struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Module        string    `json:"module"`
	Description   string    `json:"description"`
	SystemPrompt  string    `json:"system_prompt"`
	ToolIDs       []string  `json:"tool_ids"`
	ModelOverride string    `json:"model_override,omitempty"`
	BuiltinKey    string    `json:"builtin_key,omitempty"`
	TeamEnabled   bool      `json:"team_enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type threadDTO struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	AgentID            string    `json:"agent_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	LastMessagePreview string    `json:"last_message_preview,omitempty"`
	// TagID 是工作台标签归属（planv6.7 AGC-NAV）；#对话 与旧会话为空。
	TagID string `json:"tag_id,omitempty"`
	// DispatchMode 为 "auto" 表示一轮一助手的共享会话；空为旧绑定模式。
	// v2 §5.1：存储里的 auto 在 DTO 读时统一映射为 collaborative（群会话），
	// 旧客户端不带 workspace_version 时仍看到 "auto"，见 workspaceV2Requested。
	DispatchMode string `json:"dispatch_mode,omitempty"`
	// ThreadKind 是会话类型（chat/alert/statistics/scheduled，v2 §5.3）；旧会话为空。
	ThreadKind string `json:"thread_kind,omitempty"`
}

type messageDTO struct {
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	AgentID   string          `json:"agent_id,omitempty"`
	Mentions  json.RawMessage `json:"mentions,omitempty"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	// RunStatus / ErrorCode 来自所属 run（BUG-06）：刷新后还原「已停止 / 失败」。
	RunStatus string `json:"run_status,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	// RouteKind 是本条 assistant 消息的路由结论（planv6.7 AGC-ROUTE：
	// auto/mention/fallback/retry，来自 sys_agent_run_routes）。
	RouteKind string    `json:"route_kind,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type runDTO struct {
	ID               string     `json:"id"`
	ThreadID         string     `json:"thread_id"`
	AgentID          string     `json:"agent_id"`
	Status           string     `json:"status"`
	Error            string     `json:"error,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	DurationMS       int64      `json:"duration_ms"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	ReasoningTokens  int        `json:"reasoning_tokens"`
	ToolCalls        int        `json:"tool_calls"`
	ErrorCode        string     `json:"error_code,omitempty"`
}

func toAgentDTO(a systemdb.CloudAgent) agentDTO {
	ids := a.ToolIDs
	if ids == nil {
		ids = []string{}
	}
	return agentDTO{
		ID: a.ID, Name: a.Name, Module: a.Module, Description: a.Description,
		SystemPrompt: a.SystemPrompt, ToolIDs: ids, ModelOverride: a.ModelOverride,
		BuiltinKey: a.BuiltinKey, TeamEnabled: a.TeamEnabled,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func toThreadDTO(t systemdb.AgentThread) threadDTO {
	return threadDTO{ID: t.ID, Title: t.Title, AgentID: t.AgentID, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

// toThreadDTOWithWorkspace 在 threadDTO 基础上补工作台元数据（planv6.7 AGC-NAV）。
// v2 §5.1：存储里的 auto 是「群会话」的旧写法，DTO 统一按 collaborative 输出；
// 旧客户端不带 workspace_version=2 时由 workspaceV2Requested 保持 "auto"（见 toThreadDTOResolved）。
func toThreadDTOWithWorkspace(t systemdb.AgentThread, ws systemdb.AgentThreadWorkspace) threadDTO {
	dto := toThreadDTO(t)
	dto.TagID = ws.TagID
	dto.DispatchMode = ws.DispatchMode
	if dto.DispatchMode == systemdb.DispatchAuto {
		dto.DispatchMode = systemdb.DispatchCollaborative
	}
	return dto
}

// toThreadDTOResolved 是带版本兼容的 DTO 转换：v2 客户端看到 collaborative，
// 旧客户端保持 "auto"，避免旧前端把群会话当成未知模式。
func toThreadDTOResolved(t systemdb.AgentThread, ws systemdb.AgentThreadWorkspace, collab systemdb.AgentThreadCollab, v2 bool) threadDTO {
	dto := toThreadDTOWithWorkspace(t, ws)
	if !v2 && dto.DispatchMode == systemdb.DispatchCollaborative {
		dto.DispatchMode = systemdb.DispatchAuto
	}
	dto.ThreadKind = collab.ThreadKind
	return dto
}

// workspaceV2Requested 判断请求是否声明 v2 协议（query 或 header 均可）。
func workspaceV2Requested(c echo.Context) bool {
	if strings.TrimSpace(c.QueryParam("workspace_version")) == "2" {
		return true
	}
	return strings.TrimSpace(c.Request().Header.Get("X-Workspace-Version")) == "2"
}

func toMessageDTO(m systemdb.AgentMessage) messageDTO {
	mentions := json.RawMessage(m.MentionsJSON)
	if len(mentions) == 0 {
		mentions = json.RawMessage("[]")
	}
	tools := json.RawMessage(m.ToolCallsJSON)
	if len(tools) == 0 {
		tools = json.RawMessage("[]")
	}
	return messageDTO{
		ID: m.ID, Role: m.Role, Content: m.Content, AgentID: m.AgentID,
		Mentions: mentions, ToolCalls: tools, RunID: m.RunID, CreatedAt: m.CreatedAt,
		RunStatus: m.RunStatus, ErrorCode: m.ErrorCode,
	}
}

func (h *cloudAgentHandler) ensureAgents(ctx context.Context, projectID string) error {
	if h.store == nil {
		return systemdb.ErrUnavailable
	}
	return h.store.SeedDefaultCloudAgentsWithCapabilities(ctx, projectID, h.sandboxAvailable(), h.skillsCLI())
}

func (h *cloudAgentHandler) ListModules(c echo.Context) error {
	modules := agentbridge.Modules(h.sandboxAvailable())
	if !h.skillsCLI() {
		available := modules[:0]
		for _, module := range modules {
			switch module.ID {
			case agentbridge.ModuleKV, agentbridge.ModuleGoFunction, agentbridge.ModuleCron:
				continue
			}
			available = append(available, module)
		}
		modules = available
	}
	return c.JSON(http.StatusOK, map[string]any{"modules": modules})
}

func (h *cloudAgentHandler) ListAgents(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	if h.writable == nil || *h.writable {
		if err := h.ensureAgents(c.Request().Context(), pc.ID); err != nil {
			return WriteError(c, err)
		}
	}
	list, err := h.store.ListCloudAgents(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]agentDTO, 0, len(list))
	for _, a := range list {
		out = append(out, toAgentDTO(a))
	}
	return c.JSON(http.StatusOK, map[string]any{"agents": out})
}

type upsertAgentBody struct {
	Name          string   `json:"name"`
	Module        string   `json:"module"`
	Description   string   `json:"description"`
	SystemPrompt  string   `json:"system_prompt"`
	ToolIDs       []string `json:"tool_ids"`
	ModelOverride *string  `json:"model_override"`
	TeamEnabled   *bool    `json:"team_enabled"`
}

func (h *cloudAgentHandler) denyReadonly(c echo.Context) error {
	if h.writable != nil && !*h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
	return nil
}

func (h *cloudAgentHandler) CreateAgent(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	var body upsertAgentBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	a, err := h.normalizeAgent(pc.ID, body, systemdb.CloudAgent{}, principal)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, err.Error()))
	}
	created, err := h.store.CreateCloudAgent(c.Request().Context(), a)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, toAgentDTO(created))
}

func (h *cloudAgentHandler) GetAgent(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	a, err := h.store.GetCloudAgent(c.Request().Context(), pc.ID, c.Param("agentID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toAgentDTO(a))
}

func (h *cloudAgentHandler) PatchAgent(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	cur, err := h.store.GetCloudAgent(c.Request().Context(), pc.ID, c.Param("agentID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	var body upsertAgentBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	next, err := h.normalizeAgent(pc.ID, body, cur, principal)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, err.Error()))
	}
	next.ID = cur.ID
	updated, err := h.store.UpdateCloudAgent(c.Request().Context(), next)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toAgentDTO(updated))
}

func (h *cloudAgentHandler) DeleteAgent(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	if err := h.store.ArchiveCloudAgent(c.Request().Context(), pc.ID, c.Param("agentID")); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// normalizeAgent 校验并归一 agent 写入。云沙盒未启用时拒绝 sandbox 模块
// 与沙盒工具（cloud-agent-sandbox-plan §7）。
func (h *cloudAgentHandler) normalizeAgent(projectID string, body upsertAgentBody, cur systemdb.CloudAgent, p auth.Principal) (systemdb.CloudAgent, error) {
	a, err := normalizeAgent(projectID, body, cur)
	if err != nil {
		return systemdb.CloudAgent{}, err
	}
	if (a.Module == agentbridge.ModuleKV || a.Module == agentbridge.ModuleGoFunction || a.Module == agentbridge.ModuleCron) && (!h.skillsCLI()) {
		return systemdb.CloudAgent{}, errors.New("this module requires CLI skills")
	}
	if !h.sandboxAvailable() {
		if strings.EqualFold(a.Module, agentbridge.ModuleSandbox) {
			return systemdb.CloudAgent{}, errors.New("sandbox is not enabled; sandbox module is unavailable")
		}
		for _, id := range a.ToolIDs {
			if agentbridge.IsSandboxTool(id) {
				return systemdb.CloudAgent{}, errors.New("sandbox is not enabled; sandbox tools are unavailable")
			}
		}
	}
	// MCP 引用需要宿主装配了连接器，否则保存后 run 会直接报错（planv6.2 §4.1）。
	if cloudagent.HasMCPTools(a.ToolIDs) && (h.runtime == nil || h.runtime.MCP == nil) {
		return systemdb.CloudAgent{}, errors.New("mcp is not enabled; remove mcp: tool ids from this agent")
	}
	return a, nil
}

func normalizeAgent(projectID string, body upsertAgentBody, cur systemdb.CloudAgent) (systemdb.CloudAgent, error) {
	a := cur
	a.ProjectID = projectID
	if body.Name != "" {
		a.Name = strings.TrimSpace(body.Name)
	}
	if body.Module != "" {
		a.Module = strutil.Normalize(body.Module)
	}
	if body.Description != "" || cur.ID == "" {
		a.Description = body.Description
	}
	if body.SystemPrompt != "" || cur.ID == "" {
		a.SystemPrompt = body.SystemPrompt
	}
	if body.ToolIDs != nil {
		a.ToolIDs = body.ToolIDs
	}
	if body.ModelOverride != nil {
		a.ModelOverride = strings.TrimSpace(*body.ModelOverride)
	}
	if body.TeamEnabled != nil {
		a.TeamEnabled = *body.TeamEnabled
	}
	if strings.TrimSpace(a.Name) == "" {
		return systemdb.CloudAgent{}, errors.New("name is required")
	}
	if a.Module == "" {
		a.Module = agentbridge.ModuleGeneral
	}
	if !agentbridge.KnownModule(a.Module) {
		return systemdb.CloudAgent{}, errors.New("unknown module")
	}
	if a.ToolIDs == nil {
		a.ToolIDs = agentbridge.DefaultToolsForModule(a.Module)
	}
	filtered := make([]string, 0, len(a.ToolIDs))
	for _, id := range a.ToolIDs {
		if agentbridge.KnownTool(id) {
			filtered = append(filtered, id)
			continue
		}
		// mcp: 引用（planv6.2 §3.2）：格式非法直接 400，不静默丢弃——
		// 静默丢弃会让用户以为能力已启用，实际没有。
		if cloudagent.HasMCPTools([]string{id}) {
			if _, _, ok := mcp.ParseToolID(id); !ok {
				return systemdb.CloudAgent{}, fmt.Errorf("invalid mcp tool id: %s", id)
			}
			filtered = append(filtered, id)
		}
	}
	a.ToolIDs = filtered
	return a, nil
}

func (h *cloudAgentHandler) ListThreads(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	limit := 50
	if raw := c.QueryParam("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid limit"))
		}
		limit = n
	}
	// agent_id 与 tag_id 同传时取交集；不带时返回全部会话。
	agentID := strings.TrimSpace(c.QueryParam("agent_id"))
	tagID := strings.TrimSpace(c.QueryParam("tag_id"))
	if tagID != "" {
		if _, err := h.resolveAgentTag(c.Request().Context(), pc.ID, tagID); err != nil {
			return WriteError(c, err)
		}
	}
	list, next, err := h.store.PageAgentThreadsFiltered(c.Request().Context(), pc.ID, systemdb.AgentThreadFilter{
		AgentID:    agentID,
		TagID:      tagID,
		ThreadKind: strings.TrimSpace(c.QueryParam("thread_kind")),
	}, c.QueryParam("cursor"), limit)
	if errors.Is(err, systemdb.ErrAgentThreadCursor) {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid cursor"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	// 批量补工作台元数据，避免 N+1（planv6.7 AGC-NAV §7.8）。
	threadIDs := make([]string, 0, len(list))
	for _, t := range list {
		threadIDs = append(threadIDs, t.ID)
	}
	workspaces, err := h.store.ListAgentThreadWorkspaces(c.Request().Context(), pc.ID, threadIDs)
	if err != nil {
		return WriteError(c, err)
	}
	// v2：批量补会话类型（旧会话无行按 chat 处理），同样避免 N+1。
	collabs, err := h.store.ListAgentThreadCollabs(c.Request().Context(), pc.ID, threadIDs)
	if err != nil {
		return WriteError(c, err)
	}
	v2 := workspaceV2Requested(c)
	out := make([]threadDTO, 0, len(list))
	for _, t := range list {
		row := toThreadDTOResolved(t, workspaces[t.ID], collabs[t.ID], v2)
		row.LastMessagePreview, err = h.store.AgentThreadPreview(c.Request().Context(), pc.ID, t.ID)
		if err != nil {
			return WriteError(c, err)
		}
		out = append(out, row)
	}
	return c.JSON(http.StatusOK, map[string]any{"threads": out, "next_cursor": next})
}

func (h *cloudAgentHandler) PatchThread(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	if h.writable != nil && !*h.writable {
		return WriteError(c, godatabase.ErrWriterUnavailable)
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	body.Title = strings.TrimSpace(body.Title)
	if n := len([]rune(body.Title)); n < 1 || n > 80 {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "title must be 1-80 characters"))
	}
	th, err := h.store.RenameAgentThread(c.Request().Context(), pc.ID, c.Param("threadID"), body.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	ws, err := h.store.GetAgentThreadWorkspace(c.Request().Context(), pc.ID, th.ID)
	if err != nil {
		return WriteError(c, err)
	}
	collab, err := h.store.GetAgentThreadCollab(c.Request().Context(), pc.ID, th.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toThreadDTOResolved(th, ws, collab, workspaceV2Requested(c)))
}

func (h *cloudAgentHandler) ListThreadRuns(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	threadID := c.Param("threadID")
	if _, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, threadID); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	limit := 20
	if raw := c.QueryParam("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid limit"))
		}
		limit = n
	}
	list, err := h.store.ListAgentRuns(c.Request().Context(), pc.ID, threadID, limit)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]runDTO, 0, len(list))
	for _, r := range list {
		out = append(out, toRunDTO(r))
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": out})
}

// activeRunDTO 是「正在回复」指示所需的最小元数据，不含正文。
type activeRunDTO struct {
	RunID     string     `json:"run_id"`
	AgentID   string     `json:"agent_id"`
	ThreadID  string     `json:"thread_id"`
	StartedAt *time.Time `json:"started_at,omitempty"`
}

// ListActiveRuns 返回项目内正在执行的 run（交互 + 定时任务），供助手列表 / 会话切换器显示转圈。
// 库里 running 的行还要与运行时内存态核对，过滤掉进程重启后残留的僵尸记录。
func (h *cloudAgentHandler) ListActiveRuns(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	out := make([]activeRunDTO, 0)
	if h.runtime == nil {
		return c.JSON(http.StatusOK, map[string]any{"runs": out})
	}
	list, err := h.store.ListActiveAgentRuns(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	for _, r := range list {
		if !h.runtime.IsRunActive(r.ID) {
			continue
		}
		dto := activeRunDTO{RunID: r.ID, AgentID: r.AgentID, ThreadID: r.ThreadID}
		if !r.StartedAt.IsZero() {
			t := r.StartedAt
			dto.StartedAt = &t
		}
		out = append(out, dto)
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": out})
}

func (h *cloudAgentHandler) CreateThread(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	var body struct {
		Title   string `json:"title"`
		AgentID string `json:"agent_id"`
		// planv6.7 AGC-NAV：工作台元数据（三者都不传时不写元数据行，旧请求完全不变）。
		TagID        string `json:"tag_id"`
		DispatchMode string `json:"dispatch_mode"`
		Source       string `json:"source"`
		// planv6.7 v2：会话类型（chat/alert/statistics/scheduled），缺省 chat。
		ThreadKind string `json:"thread_kind"`
	}
	_ = c.Bind(&body)
	// v2 §5.4：createdBy 与 principalIdentity 统一口径（登录态写 UserID、API Key 写
	// "apikey:<id>"），不再只写 APIKeyID —— 否则 #对话 分区只能靠「空串」隐式区分。
	createdBy := ""
	principal, hasPrincipal := PrincipalFromContext(c.Request().Context())
	if hasPrincipal {
		createdBy = principalIdentity(principal)
	}
	// agent_id 可选：带上即把会话归属到该助手，后续列表按它过滤。
	agentID := strings.TrimSpace(body.AgentID)
	if agentID != "" {
		if _, err := h.store.GetCloudAgent(c.Request().Context(), pc.ID, agentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
			}
			return WriteError(c, err)
		}
	}
	// planv6.7 AGC-NAV §2.2 + v2 §5：tag_id / dispatch_mode / source / thread_kind 校验。
	tagID := strings.TrimSpace(body.TagID)
	dispatchMode := strings.TrimSpace(body.DispatchMode)
	source := strings.TrimSpace(body.Source)
	threadKind := strings.TrimSpace(body.ThreadKind)
	switch dispatchMode {
	case "", systemdb.DispatchAuto, systemdb.DispatchCollaborative:
	default:
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "dispatch_mode must be auto or collaborative"))
	}
	switch source {
	case "", systemdb.ThreadSourceWorkspace, systemdb.ThreadSourceStarter:
	default:
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid source"))
	}
	switch threadKind {
	case "", systemdb.ThreadKindChat, systemdb.ThreadKindAlert,
		systemdb.ThreadKindStatistics, systemdb.ThreadKindScheduled:
	default:
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid thread_kind"))
	}
	// v2：群会话（协作/auto）必须带 tag_id 或落在 #对话；两者都支持，故 tag 不再强制 auto。
	if tagID != "" {
		if dispatchMode == "" {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "tag_id requires dispatch_mode"))
		}
		if _, err := h.resolveAgentTag(c.Request().Context(), pc.ID, tagID); err != nil {
			return WriteError(c, err)
		}
	}
	// 群会话（auto / collaborative）不允许再绑定单个助手：成员由群配置决定。
	if (dispatchMode == systemdb.DispatchAuto || dispatchMode == systemdb.DispatchCollaborative) && agentID != "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "group thread cannot set agent_id"))
	}
	if dispatchMode != "" && source == "" {
		source = systemdb.ThreadSourceWorkspace
	}
	if dispatchMode == "" && threadKind != "" && threadKind != systemdb.ThreadKindChat {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "thread_kind requires a group dispatch mode"))
	}
	var ws systemdb.AgentThreadWorkspace
	if tagID != "" || dispatchMode != "" || source != "" {
		ws = systemdb.AgentThreadWorkspace{TagID: tagID, DispatchMode: dispatchMode, Source: source}
	}
	th, err := h.store.CreateAgentThreadWithWorkspace(c.Request().Context(), systemdb.AgentThread{
		ProjectID: pc.ID, Title: body.Title, AgentID: agentID, CreatedBy: createdBy,
	}, ws)
	if err != nil {
		return WriteError(c, err)
	}
	// 协作会话写会话级元数据（类型 + 活动轮次占用位）；失败时回滚会话，避免半成品。
	var collab systemdb.AgentThreadCollab
	if dispatchMode != "" {
		if threadKind == "" {
			threadKind = threadKindForTag(tagID)
		}
		collab, err = h.store.CreateAgentThreadCollab(c.Request().Context(), systemdb.AgentThreadCollab{
			ProjectID: pc.ID, ThreadID: th.ID, TagID: tagID, ThreadKind: threadKind,
		})
		if err != nil {
			_ = h.store.ArchiveAgentThread(c.Request().Context(), pc.ID, th.ID)
			return WriteError(c, err)
		}
	}
	dto := toThreadDTOWithWorkspace(th, ws)
	dto.ThreadKind = collab.ThreadKind
	return c.JSON(http.StatusCreated, dto)
}

// threadKindForTag 按群推出默认会话类型：告警群=alert、统计群=statistics、
// 定时群=scheduled，其余群=chat。
func threadKindForTag(tagID string) string {
	switch tagID {
	case "":
		return systemdb.ThreadKindChat
	case systemdb.AgentTagAlerts:
		return systemdb.ThreadKindAlert
	case systemdb.AgentTagStats:
		return systemdb.ThreadKindStatistics
	case systemdb.AgentTagScheduled:
		return systemdb.ThreadKindScheduled
	default:
		return systemdb.ThreadKindChat
	}
}

func (h *cloudAgentHandler) GetThread(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	th, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, c.Param("threadID"))
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	ws, err := h.store.GetAgentThreadWorkspace(c.Request().Context(), pc.ID, th.ID)
	if err != nil {
		return WriteError(c, err)
	}
	collab, err := h.store.GetAgentThreadCollab(c.Request().Context(), pc.ID, th.ID)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, toThreadDTOResolved(th, ws, collab, workspaceV2Requested(c)))
}

func (h *cloudAgentHandler) DeleteThread(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	threadID := c.Param("threadID")
	if err := h.store.ArchiveAgentThread(c.Request().Context(), pc.ID, threadID); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	// 释放该 thread 的云沙盒；失败只记日志，不阻塞软删（cloud-agent-sandbox-plan §3.2）。
	if provider := h.provider(); provider != nil && provider.Sandbox != nil && provider.Sandbox.Available() {
		if err := provider.Sandbox.ReleaseThread(c.Request().Context(), pc.ID, threadID); err != nil {
			c.Logger().Warnf("release sandbox for thread %s failed: %v", threadID, err)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *cloudAgentHandler) ListMessages(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	if _, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, c.Param("threadID")); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	msgs, err := h.store.ListAgentMessages(c.Request().Context(), pc.ID, c.Param("threadID"), 200)
	if err != nil {
		return WriteError(c, err)
	}
	// planv6.7 AGC-ROUTE：assistant 消息按 run 补 route_kind（刷新后恢复回答原因）。
	routes, err := h.store.ListAgentRunRoutes(c.Request().Context(), pc.ID, c.Param("threadID"))
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]messageDTO, 0, len(msgs))
	for _, m := range msgs {
		row := toMessageDTO(m)
		if m.Role == "assistant" && m.RunID != "" {
			if r, ok := routes[m.RunID]; ok {
				row.RouteKind = r.RouteKind
			}
		}
		out = append(out, row)
	}
	return c.JSON(http.StatusOK, map[string]any{"messages": out})
}

// confirmTimeout 返回确认窗口；未配置时用 2m（与 config.example.yaml 默认一致）。
func (h *cloudAgentHandler) confirmTimeout() time.Duration {
	if h.runtime != nil && h.runtime.ConfirmTimeout > 0 {
		return h.runtime.ConfirmTimeout
	}
	return 2 * time.Minute
}

type approvalDTO struct {
	ID          string     `json:"id"`
	RunID       string     `json:"run_id"`
	CallID      string     `json:"call_id"`
	ThreadID    string     `json:"thread_id,omitempty"`
	AgentID     string     `json:"agent_id,omitempty"`
	ToolName    string     `json:"tool_name,omitempty"`
	Argv        []string   `json:"argv"`
	Message     string     `json:"message,omitempty"`
	Status      string     `json:"status"`
	InitiatorID string     `json:"initiator_id,omitempty"`
	ResolvedBy  string     `json:"resolved_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

func toApprovalDTO(a systemdb.AgentApproval) approvalDTO {
	argv := []string{}
	if a.ArgvJSON != "" {
		_ = json.Unmarshal([]byte(a.ArgvJSON), &argv)
	}
	d := approvalDTO{
		ID: a.ID, RunID: a.RunID, CallID: a.CallID, ThreadID: a.ThreadID, AgentID: a.AgentID,
		ToolName: a.ToolName, Argv: argv, Message: a.Message, Status: a.Status,
		InitiatorID: a.InitiatorID, ResolvedBy: a.ResolvedBy, CreatedAt: a.CreatedAt,
	}
	if !a.ExpiresAt.IsZero() {
		t := a.ExpiresAt
		d.ExpiresAt = &t
	}
	if !a.ResolvedAt.IsZero() {
		t := a.ResolvedAt
		d.ResolvedAt = &t
	}
	return d
}

// ListApprovals 列出项目审批队列。status 为空表示全部；查询前先收敛过期项。
func (h *cloudAgentHandler) ListApprovals(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	ctx := c.Request().Context()
	// 惰性收敛：不引入后台 goroutine，列表查询顺带把超时项标成 expired。
	_, _ = h.store.ExpireAgentApprovals(ctx, pc.ID)
	status := strings.TrimSpace(c.QueryParam("status"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	rows, err := h.store.ListAgentApprovals(ctx, pc.ID, status, limit)
	if err != nil {
		return WriteError(c, err)
	}
	pending, err := h.store.CountPendingAgentApprovals(ctx, pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]approvalDTO, 0, len(rows))
	for _, a := range rows {
		out = append(out, toApprovalDTO(a))
	}
	return c.JSON(http.StatusOK, map[string]any{"approvals": out, "pending": pending})
}

func (h *cloudAgentHandler) CancelRun(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	// SEC-04：取消终止的是他人的运行，属于破坏性操作；只读主体（登录态
	// 只读角色或仅持 DatabaseRead 的 Key）拒绝，写主体允许。
	p, hasPrincipal := PrincipalFromContext(c.Request().Context())
	if hasPrincipal && !h.canWritePrincipal(p) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "write permission required"))
	}
	runID := c.Param("runID")
	run, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	// planv6.0 D5：发起人校验。仅发起人本人、project:admin、或历史数据
	//（initiator_id 为空）可取消；同项目其他写主体拒绝。
	if hasPrincipal && !canCancelConfirm(p, run.InitiatorID) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "only the initiator or a project admin can cancel this run"))
	}
	// planv6.7 AGC-ROUTE：路由期取消优先——run 还没交给 runtime 时
	// CancelRun 只能靠 routeCancels 生效。
	if f, ok := h.routeCancels.LoadAndDelete(runID); ok {
		f.(context.CancelFunc)()
	}
	if h.runtime != nil {
		h.runtime.CancelRun(runID)
	}
	finished := time.Now().UTC()
	if run.Status == systemdb.AgentRunRunning || run.Status == systemdb.AgentRunQueued {
		_ = h.store.UpdateAgentRunStatus(c.Request().Context(), pc.ID, runID, systemdb.AgentRunCanceled, "canceled", nil, &finished)
		run.Status = systemdb.AgentRunCanceled
		run.FinishedAt = finished
	}
	// run 已终止，残留的 pending 审批不可能再被执行，直接作废，
	// 避免审批页留下永远点不动的待办。
	_ = h.store.CancelRunApprovals(c.Request().Context(), pc.ID, runID)
	return c.JSON(http.StatusOK, toRunDTO(run))
}

// principalIdentity 生成发起人标识（planv6.0 D5）：登录态为 UserID，
// API Key 为 "apikey:<id>"。与 gocloudagent 包的判定保持一致。
func principalIdentity(p auth.Principal) string {
	if p.UserID != "" {
		return p.UserID
	}
	if p.APIKeyID != "" {
		return "apikey:" + p.APIKeyID
	}
	return ""
}

// canCancelConfirm 判断主体能否确认/取消 run（planv6.0 D5）：
// 发起人本人、project:admin，或历史数据（initiator_id 为空，仍要求写权限）。
func canCancelConfirm(p auth.Principal, initiatorID string) bool {
	if initiatorID == "" {
		return true
	}
	if initiatorID == principalIdentity(p) {
		return true
	}
	return p.HasPermission(auth.ProjectAdmin)
}

func (h *cloudAgentHandler) resolveMentionedAgent(ctx context.Context, projectID string, mentions []mentionDTO) (systemdb.CloudAgent, error) {
	if len(mentions) > 0 && mentions[0].AgentID != "" {
		a, err := h.store.GetCloudAgent(ctx, projectID, mentions[0].AgentID)
		if errors.Is(err, sql.ErrNoRows) {
			return systemdb.CloudAgent{}, errors.New("mentioned agent not found")
		}
		return a, err
	}
	list, err := h.store.ListCloudAgents(ctx, projectID)
	if err != nil {
		return systemdb.CloudAgent{}, err
	}
	for _, agent := range list {
		if agent.BuiltinKey == agentbridge.ModuleGeneral {
			return agent, nil
		}
	}
	return systemdb.CloudAgent{}, &cloudagent.RunError{Code: "agent_required", Message: "请先选择一个 Agent"}
}

// canWritePrincipal 判断主体是否可执行破坏性 run 操作（SEC-04）：
// 登录态按角色 CanWrite（admin 只读拒绝），API Key 按 DatabaseWrite 位。
func (h *cloudAgentHandler) canWritePrincipal(p auth.Principal) bool {
	if p.UserID != "" {
		return p.Role.CanWrite()
	}
	return p.HasPermission(auth.DatabaseWrite)
}

func (h *cloudAgentHandler) failRun(ctx context.Context, projectID, runID, msg string) error {
	finished := time.Now().UTC()
	return h.store.UpdateAgentRunStatus(ctx, projectID, runID, systemdb.AgentRunFailed, msg, nil, &finished)
}

// agentModelDTO 是 /agents/models 返回的单个模型（只含名字，无 base_url/key）。
type agentModelDTO struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

// ListAgentModels 返回项目可用模型列表（BUG-07）。
// 来源为 provider allowed_models；只返回名字，不暴露 base_url / api_key。
// LLM 未启用时返回空列表（200）。
func (h *cloudAgentHandler) ListAgentModels(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	out := struct {
		DefaultModel string          `json:"default_model"`
		Models       []agentModelDTO `json:"models"`
	}{Models: []agentModelDTO{}}
	if h.llm == nil {
		return c.JSON(http.StatusOK, out)
	}
	models, err := h.llm.ProviderModels(c.Request().Context(), pc.ID)
	if err != nil {
		return WriteError(c, err)
	}
	defaultModel := ""
	if provider := h.provider(); provider != nil && provider.Settings != nil {
		if st, serr := provider.Settings.LLMSettings(c.Request().Context(), pc.ID); serr == nil && st.DefaultModel != "" {
			defaultModel = st.DefaultModel
		}
	}
	providers, _ := h.llm.ListProviders(c.Request().Context(), pc.ID)
	for _, p := range providers {
		for _, m := range models[p] {
			if defaultModel == "" {
				defaultModel = m
			}
			out.Models = append(out.Models, agentModelDTO{Provider: p, Name: m})
		}
	}
	out.DefaultModel = defaultModel
	return c.JSON(http.StatusOK, out)
}

func toRunDTO(r systemdb.AgentRun) runDTO {
	dto := runDTO{ID: r.ID, ThreadID: r.ThreadID, AgentID: r.AgentID, Status: r.Status, Error: r.Error,
		DurationMS: r.DurationMS, PromptTokens: r.PromptTokens, CompletionTokens: r.CompletionTokens,
		ReasoningTokens: r.ReasoningTokens, ToolCalls: r.ToolCalls, ErrorCode: r.ErrorCode}
	if !r.StartedAt.IsZero() {
		t := r.StartedAt
		dto.StartedAt = &t
	}
	if !r.FinishedAt.IsZero() {
		t := r.FinishedAt
		dto.FinishedAt = &t
	}
	return dto
}

// planv6.7 AGC-ROUTE：自动路由会话的领域错误。
var (
	// ErrNoAvailableAgent：auto 会话无可用候选（在 ClaimThread 之前返回，不落记录）。
	ErrNoAvailableAgent = errors.New("no available agent for this thread")
	// ErrAgentNotInTag：mention 的助手不在标签候选中。
	ErrAgentNotInTag = errors.New("mentioned agent is not in this tag")
	// ErrRetryAgentUnavailable：失败重试时原助手已归档或不在候选中。
	ErrRetryAgentUnavailable = errors.New("the original agent is no longer available")
)

// planv6.7 v2：协作群的领域错误（统一进 error.go 的 sentinelErrorMappings）。
var (
	// ErrWorkspaceUpgradeRequired：协作表尚未建好（未升级环境），需要用户执行建表。
	ErrWorkspaceUpgradeRequired = errors.New("collaboration schema is not ready")
	// ErrGroupArchived：群已归档，拒绝新发送与新调度。
	ErrGroupArchived = errors.New("this group has been archived")
	// ErrGroupMemberUnavailable：目标助手已归档或不在群成员中。
	ErrGroupMemberUnavailable = errors.New("the target agent is not an available member of this group")
	// ErrTurnBusy：同一会话已有活动轮次。
	ErrTurnBusy = errors.New("this thread already has an active turn")
	// ErrTurnConflict：轮次/任务版本冲突（并发推进）。
	ErrTurnConflict = errors.New("agent turn was modified, reload and retry")
	// ErrRoutingFailed：路由器超时、输出越界或无法给出可用分工。
	ErrRoutingFailed = errors.New("routing failed")
	// ErrInvalidDispatchGraph：任务图不合法（越界、环、深度、数量）。
	ErrInvalidDispatchGraph = errors.New("invalid dispatch graph")
	// ErrHumanAssigneeRequired：需要人但没有可指派的人（API Key 主体或无人负责人）。
	ErrHumanAssigneeRequired = errors.New("a human assignee is required")
	// ErrHumanTaskForbidden：非接收人（且非项目管理员）处理他人待办。
	ErrHumanTaskForbidden = errors.New("only the assignee or a project admin can resolve this task")
	// ErrTaskNotRetryable：任务状态不允许重试。
	ErrTaskNotRetryable = errors.New("this task cannot be retried")
	// ErrCollaborationBudgetExceeded：超出协作预算（任务/交接次数）。
	ErrCollaborationBudgetExceeded = errors.New("collaboration budget exceeded")
)
