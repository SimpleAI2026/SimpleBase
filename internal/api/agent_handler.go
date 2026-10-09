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

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/usage"
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
		BuiltinKey: a.BuiltinKey, TeamEnabled: a.TeamEnabled, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func toThreadDTO(t systemdb.AgentThread) threadDTO {
	return threadDTO{ID: t.ID, Title: t.Title, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
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
	return h.store.SeedDefaultCloudAgentsWithSandbox(ctx, projectID, h.sandboxAvailable())
}

func (h *cloudAgentHandler) ListModules(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"modules": cloudagent.Modules(h.sandboxAvailable())})
}

func (h *cloudAgentHandler) ListAgents(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
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
		return WriteError(c, database.ErrWriterUnavailable)
	}
	return nil
}

func (h *cloudAgentHandler) CreateAgent(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	var body upsertAgentBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	a, err := h.normalizeAgent(pc.ID, body, systemdb.CloudAgent{})
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
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
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
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
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
	next, err := h.normalizeAgent(pc.ID, body, cur)
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
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
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
func (h *cloudAgentHandler) normalizeAgent(projectID string, body upsertAgentBody, cur systemdb.CloudAgent) (systemdb.CloudAgent, error) {
	a, err := normalizeAgent(projectID, body, cur)
	if err != nil {
		return systemdb.CloudAgent{}, err
	}
	if !h.sandboxAvailable() {
		if strings.EqualFold(a.Module, cloudagent.ModuleSandbox) {
			return systemdb.CloudAgent{}, errors.New("cloud sandbox is not enabled; sandbox module is unavailable")
		}
		for _, id := range a.ToolIDs {
			if cloudagent.IsSandboxTool(id) {
				return systemdb.CloudAgent{}, errors.New("cloud sandbox is not enabled; sandbox tools are unavailable")
			}
		}
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
		}
	}
	a.ToolIDs = filtered
	return a, nil
}

func (h *cloudAgentHandler) ListThreads(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	limit := 50
	if raw := c.QueryParam("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid limit"))
		}
		limit = n
	}
	list, next, err := h.store.PageAgentThreads(c.Request().Context(), pc.ID, c.QueryParam("cursor"), limit)
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
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	if h.writable != nil && !*h.writable {
		return WriteError(c, database.ErrWriterUnavailable)
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
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
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

func (h *cloudAgentHandler) CreateThread(c echo.Context) error {
	if err := h.denyReadonly(c); err != nil {
		return err
	}
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	var body struct {
		Title string `json:"title"`
	}
	_ = c.Bind(&body)
	createdBy := ""
	if p, ok := PrincipalFromContext(c.Request().Context()); ok {
		createdBy = p.APIKeyID
	}
	th, err := h.store.CreateAgentThread(c.Request().Context(), systemdb.AgentThread{
		ProjectID: pc.ID, Title: body.Title, CreatedBy: createdBy,
	})
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusCreated, toThreadDTO(th))
}

func (h *cloudAgentHandler) GetThread(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
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
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
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
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
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

type mentionDTO struct {
	AgentID string `json:"agent_id"`
}

type createRunBody struct {
	Content  string       `json:"content"`
	Mentions []mentionDTO `json:"mentions"`
	Skills   []string     `json:"skills"`
	Stream   bool         `json:"stream"`
	// RetryOfRunID 非空表示重试该 run：复用其 user 消息，不重复落库（BUG-03）。
	RetryOfRunID string `json:"retry_of_run_id"`
}

func (h *cloudAgentHandler) CreateRun(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	threadID := c.Param("threadID")
	if _, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, threadID); errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	var body createRunBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	// 重试请求 content 由被重试 run 的原消息提供（BUG-03）。
	if strings.TrimSpace(body.Content) == "" && body.RetryOfRunID == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "content is required"))
	}
	if err := h.ensureAgents(c.Request().Context(), pc.ID); err != nil {
		return WriteError(c, err)
	}
	agent, err := h.resolveMentionedAgent(c.Request().Context(), pc.ID, body.Mentions)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, err.Error()))
	}
	skills, err := cloudagent.NormalizeSkills(body.Skills, agent.Module)
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, err.Error()))
	}
	body.Skills = skills
	// 重试校验（BUG-03）：目标 run 必须属于本 thread、状态 failed/canceled、且是 thread 最新一次 run。
	var retryUser systemdb.AgentMessage
	if body.RetryOfRunID != "" {
		prev, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, body.RetryOfRunID)
		if errors.Is(err, sql.ErrNoRows) {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid_retry"))
		}
		if err != nil {
			return WriteError(c, err)
		}
		if prev.ThreadID != threadID || (prev.Status != systemdb.AgentRunFailed && prev.Status != systemdb.AgentRunCanceled) {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid_retry"))
		}
		latest, err := h.store.ListAgentRuns(c.Request().Context(), pc.ID, threadID, 1)
		if err != nil {
			return WriteError(c, err)
		}
		if len(latest) == 0 || latest[0].ID != prev.ID {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid_retry"))
		}
		msgs, err := h.store.ListAgentMessages(c.Request().Context(), pc.ID, threadID, 200)
		if err != nil {
			return WriteError(c, err)
		}
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].RunID == prev.ID && msgs[i].Role == "user" {
				retryUser = msgs[i]
				break
			}
		}
		if retryUser.ID == "" {
			return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "invalid_retry"))
		}
		body.Content = retryUser.Content
	}
	if h.runtime == nil || h.runtime.LLM == nil {
		if !body.Stream {
			return WriteError(c, &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"})
		}
		// SSE 消费端仍收到规范的 error → end 帧；不持久化用户正文。
		return h.streamRun(c, pc.ID, systemdb.AgentRun{ID: uuid.NewString()}, agent, cloudagent.RunRequest{})
	}
	if h.usage != nil {
		if err := h.usage.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) {
				return WriteError(c, NewAPIError(http.StatusTooManyRequests, "quota_exceeded", "项目模型调用配额已用完", RequestIDFromContext(c.Request().Context())))
			}
			return WriteError(c, err)
		}
	}
	claimID := uuid.NewString()
	if h.runtime != nil {
		if err := h.runtime.ClaimThread(threadID, claimID); err != nil {
			return WriteError(c, err)
		}
		defer h.runtime.ReleaseThread(threadID, claimID)
	}
	mentionsJSON, _ := json.Marshal(body.Mentions)
	if len(body.Mentions) == 0 {
		mentionsJSON, _ = json.Marshal([]mentionDTO{{AgentID: agent.ID}})
	}

	now := time.Now().UTC()
	run, err := h.store.CreateAgentRun(c.Request().Context(), systemdb.AgentRun{
		ID: claimID, ThreadID: threadID, ProjectID: pc.ID, AgentID: agent.ID,
		Status: systemdb.AgentRunRunning, StartedAt: now, RetryOfRunID: body.RetryOfRunID,
	})
	if err != nil {
		return WriteError(c, err)
	}
	// 重试复用原 user 消息，不再 Append（BUG-03）。
	var userMsg systemdb.AgentMessage
	if body.RetryOfRunID != "" {
		userMsg = retryUser
	} else {
		userMsg, err = h.store.AppendAgentMessage(c.Request().Context(), systemdb.AgentMessage{
			ThreadID: threadID, ProjectID: pc.ID, Role: "user", Content: body.Content,
			AgentID: agent.ID, MentionsJSON: string(mentionsJSON), RunID: run.ID,
		})
		if err != nil {
			return WriteError(c, err)
		}
	}
	hist, err := h.store.ListAgentMessages(c.Request().Context(), pc.ID, threadID, 40)
	if err != nil {
		return WriteError(c, err)
	}
	// Drop the just-appended user turn from history (passed separately).
	// 重试时历史中该 user 消息属于旧 run，同样不能重复出现在模型上下文。
	filtered := hist[:0]
	for _, m := range hist {
		if m.ID == userMsg.ID {
			continue
		}
		if body.RetryOfRunID != "" && m.RunID == body.RetryOfRunID {
			continue
		}
		filtered = append(filtered, m)
	}
	hist = filtered

	req := cloudagent.RunRequest{
		ProjectID: pc.ID, Principal: principal, Agent: agent,
		ThreadID: threadID, RunID: run.ID, UserText: body.Content, History: hist, Stream: body.Stream,
		Skills: skills,
	}

	if body.Stream {
		return h.streamRun(c, pc.ID, run, agent, req)
	}
	return h.completeRun(c, pc.ID, run, agent, req)
}

func (h *cloudAgentHandler) finishAgentRun(ctx context.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, res cloudagent.RunResult, runErr error) error {
	status, code := systemdb.AgentRunCompleted, ""
	if runErr != nil {
		status = systemdb.AgentRunFailed
		if errors.Is(runErr, context.Canceled) {
			status = systemdb.AgentRunCanceled
		} else {
			code = cloudagent.ClassifyError(runErr).Code
		}
	}
	if res.Content != "" || len(res.ToolCallsJSON) > 2 {
		if _, err := h.store.AppendAgentMessage(ctx, systemdb.AgentMessage{
			ThreadID: run.ThreadID, ProjectID: projectID, Role: "assistant", Content: res.Content,
			AgentID: agent.ID, ToolCallsJSON: res.ToolCallsJSON, RunID: run.ID,
		}); err != nil {
			return err
		}
	}
	finished := time.Now().UTC()
	if err := h.store.UpdateAgentRunStatus(ctx, projectID, run.ID, status, code, nil, &finished); err != nil {
		return err
	}
	if err := h.store.UpdateAgentRunMetrics(ctx, projectID, run.ID, systemdb.AgentRun{
		DurationMS: res.DurationMS, PromptTokens: res.PromptTokens, CompletionTokens: res.CompletionTokens,
		ReasoningTokens: res.ReasoningTokens, ToolCalls: res.ToolCalls, ErrorCode: code,
	}); err != nil {
		return err
	}
	if h.audit != nil {
		_ = h.audit.Record(ctx, AuditEvent{ProjectID: projectID, Kind: "agent.run", Status: status,
			Detail: fmt.Sprintf("run=%s thread=%s agent=%s duration_ms=%d prompt_tokens=%d completion_tokens=%d tool_calls=%d error_code=%s",
				run.ID, run.ThreadID, agent.ID, res.DurationMS, res.PromptTokens, res.CompletionTokens, res.ToolCalls, code)})
	}
	return nil
}

func (h *cloudAgentHandler) completeRun(c echo.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, req cloudagent.RunRequest) error {
	if h.runtime == nil {
		_ = h.failRun(c.Request().Context(), projectID, run.ID, "llm_not_configured")
		return WriteError(c, &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"})
	}
	res, runErr := h.runtime.StartRun(c.Request().Context(), req, nil)
	if err := h.finishAgentRun(context.Background(), projectID, run, agent, res, runErr); err != nil {
		return WriteError(c, err)
	}
	if runErr != nil {
		return WriteError(c, cloudagent.ClassifyError(runErr))
	}
	row, err := h.store.GetAgentRun(c.Request().Context(), projectID, run.ID)
	if err != nil {
		return WriteError(c, err)
	}
	msgs, err := h.store.ListAgentMessages(c.Request().Context(), projectID, run.ThreadID, 200)
	if err != nil {
		return WriteError(c, err)
	}
	var asst messageDTO
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].RunID == run.ID && msgs[i].Role == "assistant" {
			asst = toMessageDTO(msgs[i])
			break
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"run": toRunDTO(row), "message": asst, "agent_id": agent.ID})
}

func (h *cloudAgentHandler) streamRun(c echo.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, req cloudagent.RunRequest) error {
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	var mu sync.Mutex
	writeBytes := func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = c.Response().Write(data)
		if flusher != nil {
			flusher.Flush()
		}
	}
	var seq int64
	write := func(ev cloudagent.Event) {
		if ev.RunID == "" {
			ev.RunID = run.ID
		}
		seq++
		id := seq
		if h.events != nil {
			id = h.events.append(run.ID, ev)
		}
		data, _ := json.Marshal(ev)
		writeBytes([]byte(fmt.Sprintf("id: %d\ndata: %s\n\n", id, data)))
	}
	write(cloudagent.Event{Type: "run", RunID: run.ID})
	if h.runtime == nil || h.runtime.LLM == nil {
		write(cloudagent.Event{Type: "error", Code: "llm_not_configured", Message: "模型服务未配置"})
		write(cloudagent.Event{Type: "end"})
		return nil
	}
	stopHeartbeat := make(chan struct{})
	defer close(stopHeartbeat)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				writeBytes([]byte(": ping\n\n"))
			}
		}
	}()
	started := time.Now()
	lastOutput := time.Now()
	toolStarted := map[string]time.Time{}
	stopThinking := make(chan struct{})
	defer close(stopThinking)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopThinking:
				return
			case <-ticker.C:
				mu.Lock()
				idle := time.Since(lastOutput)
				var activeID string
				var activeSince time.Time
				for id, since := range toolStarted {
					activeID, activeSince = id, since
					break
				}
				mu.Unlock()
				if activeID != "" {
					write(cloudagent.Event{Type: "tool_progress", CallID: activeID, ElapsedMS: time.Since(activeSince).Milliseconds()})
				} else if idle >= 2*time.Second {
					write(cloudagent.Event{Type: "thinking", ElapsedMS: time.Since(started).Milliseconds()})
				}
			}
		}
	}()
	res, runErr := h.runtime.StartRun(c.Request().Context(), req, func(ev cloudagent.Event) {
		switch ev.Type {
		case "token", "tool_call", "tool_call_delta", "tool_start", "tool_result", "confirmation_required", "confirmation_resolved":
			mu.Lock()
			lastOutput = time.Now()
			if (ev.Type == "tool_call" || ev.Type == "tool_start") && ev.CallID != "" {
				toolStarted[ev.CallID] = lastOutput
			} else if ev.Type == "tool_result" {
				delete(toolStarted, ev.CallID)
			}
			mu.Unlock()
		}
		write(ev)
	})
	if err := h.finishAgentRun(context.Background(), projectID, run, agent, res, runErr); err != nil {
		write(cloudagent.Event{Type: "error", Code: "llm_upstream_error", Message: "保存运行结果失败"})
		write(cloudagent.Event{Type: "end"})
		return nil
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		re := cloudagent.ClassifyError(runErr)
		write(cloudagent.Event{Type: "error", Code: re.Code, Message: re.Message})
	}
	write(cloudagent.Event{Type: "usage", DurationMS: res.DurationMS, PromptTokens: res.PromptTokens,
		CompletionTokens: res.CompletionTokens, ReasoningTokens: res.ReasoningTokens, ToolCalls: res.ToolCalls})
	if errors.Is(runErr, context.Canceled) {
		res.Reason = "canceled"
	}
	write(cloudagent.Event{Type: "end", Reason: res.Reason})
	return nil
}

func (h *cloudAgentHandler) CancelRun(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
	}
	runID := c.Param("runID")
	run, err := h.store.GetAgentRun(c.Request().Context(), pc.ID, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "run not found"))
	}
	if err != nil {
		return WriteError(c, err)
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
	return c.JSON(http.StatusOK, toRunDTO(run))
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
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "project context missing"))
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
