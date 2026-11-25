// 文件说明：collab_handler.go — 协作群的对外接口（planv6.7 v2 §7）。
//
// 路由（router.go，mountAgentRoutes）
//
//	GET  /agent-workspace-capabilities          能力探测（协议版本 + 表就绪）
//	POST /agent-threads/:threadID/turns         发起一轮（多人/多助手）
//	GET  /agent-threads/:threadID/turns         轮次列表
//	GET  /agent-turns/:turnID                   轮次快照
//	GET  /agent-turns/:turnID/events?after=     轮次事件（SSE）
//	POST /agent-turns/:turnID/actions           cancel / retry_task / resume
//	POST /agent-tasks/:taskID/replies           人工回复（continue / unable）
//	GET  /agent-human-tasks?assignee=me         待办列表 + 角标
//	POST /agent-tags/:tagID/members             加入已存在成员 / 新建助手并加入
//	DELETE /agent-tags/:tagID/members/:agentID  移出成员（不删除助手）
//	POST /agent-tags/:tagID/events              告警/统计输入入口
//
// 权限口径：与既有 run 创建保持 DatabaseRead + writeGate（低门槛）；
// 破坏性动作（cancel / retry / 回复他人待办）在 handler 内再校验主体。
package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// workspaceCapabilitiesDTO 是能力探测的响应（前端据此决定是否露出协作控件）。
type workspaceCapabilitiesDTO struct {
	ProtocolVersion int      `json:"protocol_version"`
	Collaboration   bool     `json:"collaboration"`
	ThreadKinds     []string `json:"thread_kinds"`
	// Limits 与 plan.DefaultLimits 对齐，供前端做输入预校验。
	MaxTasks        int      `json:"max_tasks"`
	MaxDepth        int      `json:"max_depth"`
	MaxInstruction  int      `json:"max_instruction"`
	ConfirmPolicies []string `json:"confirm_policy"`
	// MissingTables 非空表示未升级：前端应提示先建表而不是直接报错。
	MissingTables []string `json:"missing_tables,omitempty"`
}

// WorkspaceCapabilities 返回协作协议的可用性（GET /agent-workspace-capabilities）。
func (h *cloudAgentHandler) WorkspaceCapabilities(c echo.Context) error {
	limits := plan.DefaultLimits()
	dto := workspaceCapabilitiesDTO{
		ProtocolVersion: 3,
		ThreadKinds: []string{systemdb.ThreadKindChat, systemdb.ThreadKindAlert,
			systemdb.ThreadKindStatistics, systemdb.ThreadKindScheduled},
		MaxTasks: limits.MaxDispatches, MaxDepth: limits.MaxDepth, MaxInstruction: limits.MaxMessage,
		ConfirmPolicies: []string{plan.ConfirmAuto, plan.ConfirmAlways, plan.ConfirmNever},
	}
	if err := h.svc.CollabSchemaReady(c.Request().Context()); err != nil {
		if errors.Is(err, systemdb.ErrCollaborationUnavailable) {
			dto.MissingTables = []string{
				"sys_agent_thread_collab", "sys_agent_turns", "sys_agent_tasks",
				"sys_agent_task_runs", "sys_agent_message_meta", "sys_agent_turn_events",
				"sys_agent_group_cron", "sys_agent_cron_fires",
			}
			return c.JSON(http.StatusOK, dto)
		}
		return WriteError(c, err)
	}
	dto.Collaboration = true
	return c.JSON(http.StatusOK, dto)
}

type planConfirmationBody struct {
	Approve         bool        `json:"approve"`
	Edits           []plan.Edit `json:"edits"`
	ExpectedVersion int         `json:"expected_version"`
}

// ConfirmTurnPlan 批准 / 编辑后批准 / 拒绝整份 Plan，返回 202 快照。
func (h *cloudAgentHandler) ConfirmTurnPlan(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	principal := principalOf(c)
	if !h.canWritePrincipal(principal) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "write permission required"))
	}
	var body planConfirmationBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if err := h.svc.ConfirmTurnPlan(c.Request().Context(), pc.ID, principal, c.Param("turnID"), group.Approval{
		Approve: body.Approve, Edits: body.Edits, ExpectedVersion: body.ExpectedVersion,
	}); err != nil {
		return WriteError(c, err)
	}
	return h.respondTurn(c, pc.ID, c.Param("turnID"))
}

// turnActionBody 是 POST /agent-turns/:turnID/actions 的请求体。
type turnActionBody struct {
	Action          string `json:"action"`
	TaskID          string `json:"task_id"`
	ExpectedVersion int64  `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// TurnAction 处理取消 / 重试单个任务 / 恢复（POST /agent-turns/:turnID/actions）。
func (h *cloudAgentHandler) TurnAction(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	principal := principalOf(c)
	if !h.canWritePrincipal(principal) {
		return WriteError(c, echo.NewHTTPError(http.StatusForbidden, "write permission required"))
	}
	turn, err := h.store.GetAgentTurn(c.Request().Context(), pc.ID, c.Param("turnID"))
	if errors.Is(err, systemdb.ErrTurnNotFound) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "turn not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	var body turnActionBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	switch strings.TrimSpace(body.Action) {
	case "cancel":
		return h.cancelTurn(c, pc, turn)
	case "retry_task":
		return h.retryTurnTask(c, pc, turn, body)
	case "resume":
		return h.resumeTurn(c, pc, turn, principal)
	default:
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "action must be cancel, retry_task or resume"))
	}
}

// cancelTurn 通过轮次级 context 取消执行器，由引擎统一收尾。
func (h *cloudAgentHandler) cancelTurn(c echo.Context, pc ProjectContext, turn systemdb.AgentTurn) error {
	if err := h.svc.CancelTurn(c.Request().Context(), pc.ID, principalOf(c), turn.ID); err != nil {
		return WriteError(c, err)
	}
	return h.respondTurn(c, pc.ID, turn.ID)
}

// retryTurnTask 重试单个失败任务：重置该任务与依赖它的未完成下游，然后继续推进。
func (h *cloudAgentHandler) retryTurnTask(c echo.Context, pc ProjectContext, turn systemdb.AgentTurn, body turnActionBody) error {
	ctx := c.Request().Context()
	taskID := strings.TrimSpace(body.TaskID)
	if taskID == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "task_id is required"))
	}
	task, err := h.store.GetAgentTask(ctx, pc.ID, taskID)
	if errors.Is(err, systemdb.ErrTaskNotFound) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "task not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	if task.TurnID != turn.ID {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "task not found"))
	}
	if task.TargetKind == systemdb.TaskTargetHuman {
		return WriteError(c, agentrun.ErrTaskNotRetryable)
	}
	switch task.Status {
	case systemdb.TaskStatusFailed, systemdb.TaskStatusInterrupted,
		systemdb.TaskStatusBlocked, systemdb.TaskStatusCanceled:
	default:
		return WriteError(c, agentrun.ErrTaskNotRetryable)
	}
	if body.ExpectedVersion > 0 && task.Version != body.ExpectedVersion {
		return WriteError(c, agentrun.ErrTurnConflict)
	}
	if err := h.svc.RetryTask(ctx, pc.ID, principalOf(c), taskID); err != nil {
		return WriteError(c, err)
	}
	return h.respondTurn(c, pc.ID, turn.ID)
}

// resumeTurn 恢复中断/等人的轮次，后台推进由 Engine 负责。
func (h *cloudAgentHandler) resumeTurn(c echo.Context, pc ProjectContext, turn systemdb.AgentTurn, principal auth.Principal) error {
	if err := h.svc.ResumeTurn(c.Request().Context(), pc.ID, principal, turn.ID); err != nil {
		return WriteError(c, err)
	}
	return h.respondTurn(c, pc.ID, turn.ID)
}

// respondTurn 返回轮次最新快照。
func (h *cloudAgentHandler) respondTurn(c echo.Context, projectID, turnID string) error {
	turn, err := h.store.GetAgentTurn(c.Request().Context(), projectID, turnID)
	if err != nil {
		return WriteError(c, err)
	}
	dto, err := h.buildTurnDTO(c.Request().Context(), projectID, turn)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusAccepted, dto)
}

// humanReplyBody 是 POST /agent-tasks/:taskID/replies 的请求体。
type humanReplyBody struct {
	// Outcome 为 continue（回复并继续）或 unable（无法处理）。
	Outcome         string `json:"outcome"`
	Text            string `json:"text"`
	ExpectedVersion int64  `json:"expected_version"`
}

// ReplyHumanTask 处理人工待办（POST /agent-tasks/:taskID/replies）。
//
// 与工具审批严格分离：这里只写任务回复，不放行任何工具执行。
func (h *cloudAgentHandler) ReplyHumanTask(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	principal, hasPrincipal := PrincipalFromContext(c.Request().Context())
	ctx := c.Request().Context()
	task, err := h.store.GetAgentTask(ctx, pc.ID, c.Param("taskID"))
	if errors.Is(err, systemdb.ErrTaskNotFound) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "task not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	if task.TargetKind != systemdb.TaskTargetHuman {
		return WriteError(c, agentrun.ErrTaskNotRetryable)
	}
	// 接收人本人或超级管理员可处理。
	//
	// 注意：不能用 ProjectAdmin 作为「管理员例外」——RoleUser 的默认权限集里
	// 就包含 ProjectAdmin（见 auth.PermissionsForRole），那等于项目内任何人
	// 都能代答他人待办。这里只放行 RoleSuperAdmin。
	caller := principal.Actor()
	isOwner := task.AssigneeID != "" && task.AssigneeID == caller
	isSuper := hasPrincipal && principal.Role == auth.RoleSuperAdmin
	if !isOwner && !isSuper {
		return WriteError(c, agentrun.ErrHumanTaskForbidden)
	}
	if task.Status != systemdb.TaskStatusWaitingHuman {
		return WriteError(c, agentrun.ErrTaskNotRetryable)
	}
	var body humanReplyBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	outcome := strings.TrimSpace(body.Outcome)
	if outcome == "" {
		outcome = group.ReplyContinue
	}
	text := strings.TrimSpace(body.Text)
	if outcome != group.ReplyContinue && outcome != group.ReplyUnable {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "outcome must be continue or unable"))
	}
	if len([]rune(text)) > 8000 {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "reply is too long (max 8000 characters)"))
	}
	if body.ExpectedVersion > 0 && task.Version != body.ExpectedVersion {
		return WriteError(c, agentrun.ErrTurnConflict)
	}
	// 回复元数据、引擎状态迁移与消息落库都在 agentrun.ReplyHuman：状态只能由引擎状态机修改。
	if err := h.svc.ReplyHuman(ctx, pc.ID, principal, task, text, outcome); err != nil {
		return WriteError(c, err)
	}
	return h.respondTurn(c, pc.ID, task.TurnID)
}

// humanTaskDTO 是待办列表项。
type humanTaskDTO struct {
	TaskID      string    `json:"task_id"`
	TurnID      string    `json:"turn_id"`
	ThreadID    string    `json:"thread_id"`
	ThreadTitle string    `json:"thread_title,omitempty"`
	TagID       string    `json:"tag_id,omitempty"`
	Instruction string    `json:"instruction"`
	Status      string    `json:"status"`
	AssigneeID  string    `json:"assignee_id"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
}

// ListHumanTasks 返回待办列表与角标（GET /agent-human-tasks?assignee=me&status=pending）。
func (h *cloudAgentHandler) ListHumanTasks(c echo.Context) error {
	pc, err := requireProject(c)
	if err != nil {
		return WriteError(c, err)
	}
	principal := principalOf(c)
	assignee := strings.TrimSpace(c.QueryParam("assignee"))
	// "me" 解析为当前登录用户；API Key 主体没有「人」的身份，只能看全部。
	if assignee == "me" {
		assignee = agentrun.HumanActorOf(principal)
		if assignee == "" {
			return c.JSON(http.StatusOK, map[string]any{"tasks": []humanTaskDTO{}, "pending": 0})
		}
	}
	status := strings.TrimSpace(c.QueryParam("status"))
	if status == "" || status == "pending" {
		status = systemdb.TaskStatusWaitingHuman
	}
	tasks, err := h.store.ListHumanAgentTasks(c.Request().Context(), pc.ID, assignee, status, 50)
	if err != nil {
		return WriteError(c, err)
	}
	pending, err := h.store.CountPendingHumanTasks(c.Request().Context(), pc.ID, assignee)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]humanTaskDTO, 0, len(tasks))
	for _, t := range tasks {
		item := humanTaskDTO{
			TaskID: t.ID, TurnID: t.TurnID, ThreadID: t.ThreadID, Instruction: t.Instruction,
			Status: t.Status, AssigneeID: t.AssigneeID, Version: t.Version, CreatedAt: t.CreatedAt,
		}
		if th, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, t.ThreadID); err == nil {
			item.ThreadTitle = th.Title
			if ws, wErr := h.store.GetAgentThreadWorkspace(c.Request().Context(), pc.ID, t.ThreadID); wErr == nil {
				item.TagID = ws.TagID
			}
		}
		out = append(out, item)
	}
	return c.JSON(http.StatusOK, map[string]any{"tasks": out, "pending": pending})
}
