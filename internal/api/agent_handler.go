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
//	[同包] adapter.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go, auth_handler.go 等 59 个
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
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
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
}

// sandboxAvailable 报告云沙盒是否启用（由 Runtime.Sandbox 提供）。
func (h *cloudAgentHandler) sandboxAvailable() bool {
	return h.runtime != nil && h.runtime.Sandbox != nil && h.runtime.Sandbox.Available()
}

type agentDTO struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Module        string   `json:"module"`
	Description   string   `json:"description"`
	SystemPrompt  string   `json:"system_prompt"`
	ToolIDs       []string `json:"tool_ids"`
	ModelOverride string   `json:"model_override,omitempty"`
	BuiltinKey    string   `json:"builtin_key,omitempty"`
	TeamEnabled   bool     `json:"team_enabled"`
	// Writable 是该助手是否允许写（planv6.0 v2 §3.3）。
	Writable  bool      `json:"writable"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type threadDTO struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	AgentID            string    `json:"agent_id,omitempty"`
	StarterTemplate    string    `json:"starter_template,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	LastMessagePreview string    `json:"last_message_preview,omitempty"`
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
	RunStatus string    `json:"run_status,omitempty"`
	ErrorCode string    `json:"error_code,omitempty"`
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
		BuiltinKey: a.BuiltinKey, TeamEnabled: a.TeamEnabled, Writable: a.Writable,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func toThreadDTO(t systemdb.AgentThread) threadDTO {
	return threadDTO{ID: t.ID, Title: t.Title, AgentID: t.AgentID, StarterTemplate: t.StarterTemplate, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
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
	return h.store.SeedDefaultCloudAgentsWithCapabilities(ctx, projectID, h.sandboxAvailable(), h.runtime != nil && h.runtime.SkillsCLI)
}

func (h *cloudAgentHandler) ListModules(c echo.Context) error {
	modules := cloudagent.Modules(h.sandboxAvailable())
	if h.runtime == nil || !h.runtime.SkillsCLI {
		available := modules[:0]
		for _, module := range modules {
			switch module.ID {
			case cloudagent.ModuleKV, cloudagent.ModuleGoFunction, cloudagent.ModuleCron:
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
	// Writable 开启该助手的写权限（planv6.0 v2 §3.3）。
	Writable *bool `json:"writable"`
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
	// planv6.0 v2 §3.3：权限提升防护。只有具备写权限的主体才能把助手设为可写。
	// 已可写的助手允许保持（避免只读主体无意中关掉别人的设置时被 403 挡住）。
	if body.Writable != nil && *body.Writable && !h.canWritePrincipal(p) {
		return systemdb.CloudAgent{}, errors.New("write permission required to enable agent writes")
	}
	if (a.Module == cloudagent.ModuleKV || a.Module == cloudagent.ModuleGoFunction || a.Module == cloudagent.ModuleCron) && (h.runtime == nil || !h.runtime.SkillsCLI) {
		return systemdb.CloudAgent{}, errors.New("this module requires CLI skills")
	}
	if !h.sandboxAvailable() {
		if strings.EqualFold(a.Module, cloudagent.ModuleSandbox) {
			return systemdb.CloudAgent{}, errors.New("sandbox is not enabled; sandbox module is unavailable")
		}
		for _, id := range a.ToolIDs {
			if cloudagent.IsSandboxTool(id) {
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
		a.Module = strings.ToLower(strings.TrimSpace(body.Module))
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
	if body.Writable != nil {
		a.Writable = *body.Writable
	}
	if strings.TrimSpace(a.Name) == "" {
		return systemdb.CloudAgent{}, errors.New("name is required")
	}
	if a.Module == "" {
		a.Module = cloudagent.ModuleGeneral
	}
	if !cloudagent.KnownModule(a.Module) {
		return systemdb.CloudAgent{}, errors.New("unknown module")
	}
	if a.ToolIDs == nil {
		a.ToolIDs = cloudagent.DefaultToolsForModule(a.Module)
	}
	filtered := make([]string, 0, len(a.ToolIDs))
	for _, id := range a.ToolIDs {
		if cloudagent.KnownTool(id) {
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
	// agent_id 可选：指定时只返回该助手的会话（planv6.0 §3.7 修订：每个助手一份）。
	agentID := strings.TrimSpace(c.QueryParam("agent_id"))
	list, next, err := h.store.PageAgentThreads(c.Request().Context(), pc.ID, agentID, c.QueryParam("cursor"), limit)
	if errors.Is(err, systemdb.ErrAgentThreadCursor) {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid cursor"))
	}
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]threadDTO, 0, len(list))
	for _, t := range list {
		row := toThreadDTO(t)
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
	return c.JSON(http.StatusOK, toThreadDTO(th))
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
		// StarterTemplate 登记首页「一句话建应用」搭建会话（home-starter-prompts-plan §2.4）。
		StarterTemplate string `json:"starter_template"`
	}
	_ = c.Bind(&body)
	createdBy := ""
	principal, hasPrincipal := PrincipalFromContext(c.Request().Context())
	if hasPrincipal {
		createdBy = principal.APIKeyID
	}
	// agent_id 可选：带上即把会话归属到该助手，后续列表按它过滤。
	agentID := strings.TrimSpace(body.AgentID)
	var agent systemdb.CloudAgent
	if agentID != "" {
		got, err := h.store.GetCloudAgent(c.Request().Context(), pc.ID, agentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "agent not found"))
			}
			return WriteError(c, err)
		}
		agent = got
	}
	starter := strings.TrimSpace(body.StarterTemplate)
	if starter != "" {
		if err := h.checkStarterAllowed(pc.ID, starter, agent, principal, hasPrincipal); err != nil {
			return WriteError(c, err)
		}
	}
	th, err := h.store.CreateAgentThread(c.Request().Context(), systemdb.AgentThread{
		ProjectID: pc.ID, Title: body.Title, AgentID: agentID, CreatedBy: createdBy, StarterTemplate: starter,
	})
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, toThreadDTO(th))
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
	return c.JSON(http.StatusOK, toThreadDTO(th))
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
	if h.runtime != nil && h.runtime.Sandbox != nil && h.runtime.Sandbox.Available() {
		if err := h.runtime.Sandbox.ReleaseThread(c.Request().Context(), pc.ID, threadID); err != nil {
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
	out := make([]messageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toMessageDTO(m))
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
		if agent.BuiltinKey == cloudagent.ModuleGeneral {
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
	if h.runtime != nil && h.runtime.Settings != nil {
		if st, serr := h.runtime.Settings.LLMSettings(c.Request().Context(), pc.ID); serr == nil && st.DefaultModel != "" {
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

// ErrStarterNotAllowed 表示不能创建搭建会话（助手 / 模板 / 权限 / 实例状态不满足）。
var ErrStarterNotAllowed = errors.New("starter session is not allowed")

// ErrStarterStageInvalid 表示 starter_stage 不在白名单，或会话不是搭建会话。
var ErrStarterStageInvalid = errors.New("starter stage is invalid")

// checkStarterAllowed 校验创建搭建会话的全部前置条件（home-starter-prompts-plan §2.4）：
// 目标必须是内置通用助手、模板在服务端白名单内、调用方具备 database:admin、
// 实例可写、CLI 技能已开启，且不是系统项目。技能只由服务端按会话登记决定，
// 不由消息内容决定。
func (h *cloudAgentHandler) checkStarterAllowed(projectID, template string, agent systemdb.CloudAgent, p auth.Principal, hasPrincipal bool) error {
	switch {
	case !cloudagent.IsStarterTemplate(template):
		return ErrStarterNotAllowed
	case agent.ID == "" || agent.BuiltinKey != "general" || agent.Module != cloudagent.ModuleGeneral:
		return ErrStarterNotAllowed
	case systemdb.IsAdminProject(projectID):
		return ErrStarterNotAllowed
	case h.runtime == nil || !h.runtime.SkillsCLI:
		return ErrStarterNotAllowed
	case h.writable != nil && !*h.writable:
		return ErrStarterNotAllowed
	case !hasPrincipal || p.Delegated || !p.HasPermission(auth.DatabaseAdmin):
		return ErrStarterNotAllowed
	}
	return nil
}
