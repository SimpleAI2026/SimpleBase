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
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/usage"
)

// workspaceCapabilitiesDTO 是能力探测的响应（前端据此决定是否露出协作控件）。
type workspaceCapabilitiesDTO struct {
	ProtocolVersion int      `json:"protocol_version"`
	Collaboration   bool     `json:"collaboration"`
	ThreadKinds     []string `json:"thread_kinds"`
	// Limits 与 cloudagent.DefaultPlanLimits 对齐，供前端做输入预校验。
	MaxTasks       int `json:"max_tasks"`
	MaxDepth       int `json:"max_depth"`
	MaxInstruction int `json:"max_instruction"`
	// MissingTables 非空表示未升级：前端应提示先建表而不是直接报错。
	MissingTables []string `json:"missing_tables,omitempty"`
}

// WorkspaceCapabilities 返回协作协议的可用性（GET /agent-workspace-capabilities）。
func (h *cloudAgentHandler) WorkspaceCapabilities(c echo.Context) error {
	limits := cloudagent.DefaultPlanLimits()
	dto := workspaceCapabilitiesDTO{
		ProtocolVersion: 2,
		ThreadKinds: []string{systemdb.ThreadKindChat, systemdb.ThreadKindAlert,
			systemdb.ThreadKindStatistics, systemdb.ThreadKindScheduled},
		MaxTasks: limits.MaxTasks, MaxDepth: limits.MaxDepth, MaxInstruction: limits.MaxInstruction,
	}
	if err := h.store.CollabSchemaReady(c.Request().Context()); err != nil {
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

// turnActionBody 是 POST /agent-turns/:turnID/actions 的请求体。
type turnActionBody struct {
	Action          string `json:"action"`
	TaskID          string `json:"task_id"`
	ExpectedVersion int64  `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// TurnAction 处理取消 / 重试单个任务 / 恢复（POST /agent-turns/:turnID/actions）。
func (h *cloudAgentHandler) TurnAction(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
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

// cancelTurn 取消整轮：终止所有在执行的子 run，任务置 canceled，释放占用。
func (h *cloudAgentHandler) cancelTurn(c echo.Context, pc ProjectContext, turn systemdb.AgentTurn) error {
	ctx := c.Request().Context()
	tasks, err := h.store.ListAgentTasks(ctx, pc.ID, turn.ID)
	if err != nil {
		return WriteError(c, err)
	}
	for i := range tasks {
		if tasks[i].RunID != "" {
			if h.runtime != nil {
				h.runtime.CancelRun(tasks[i].RunID)
			}
			finished := time.Now().UTC()
			_ = h.store.UpdateAgentRunStatus(ctx, pc.ID, tasks[i].RunID, systemdb.AgentRunCanceled, "canceled", nil, &finished)
			_ = h.store.CancelRunApprovals(ctx, pc.ID, tasks[i].RunID)
		}
		switch tasks[i].Status {
		case systemdb.TaskStatusRunning, systemdb.TaskStatusPending, systemdb.TaskStatusWaitingHuman:
			tasks[i].Status = systemdb.TaskStatusCanceled
			if _, err := h.store.UpdateAgentTask(ctx, tasks[i], 0); err != nil {
				return WriteError(c, err)
			}
		}
	}
	if err := h.store.UpdateAgentTurnStatus(ctx, pc.ID, turn.ID, systemdb.TurnStatusCanceled, "", "canceled", 0); err != nil {
		return WriteError(c, err)
	}
	h.releaseTurn(ctx, pc.ID, turn.ThreadID, turn.ID)
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
		return WriteError(c, ErrTaskNotRetryable)
	}
	switch task.Status {
	case systemdb.TaskStatusFailed, systemdb.TaskStatusInterrupted,
		systemdb.TaskStatusBlocked, systemdb.TaskStatusCanceled:
	default:
		return WriteError(c, ErrTaskNotRetryable)
	}
	if body.ExpectedVersion > 0 && task.Version != body.ExpectedVersion {
		return WriteError(c, ErrTurnConflict)
	}
	if err := h.resetTaskSubtree(ctx, pc.ID, turn.ID, taskID); err != nil {
		return WriteError(c, err)
	}
	if err := h.store.UpdateAgentTurnStatus(ctx, pc.ID, turn.ID, systemdb.TurnStatusRunning, "", "", 0); err != nil {
		return WriteError(c, err)
	}
	return h.continueTurn(c, pc, turn, principalOf(c))
}

// resetTaskSubtree 把目标任务与依赖它的未完成下游重置为 pending（人工任务不动）。
func (h *cloudAgentHandler) resetTaskSubtree(ctx context.Context, projectID, turnID, rootID string) error {
	tasks, err := h.store.ListAgentTasks(ctx, projectID, turnID)
	if err != nil {
		return err
	}
	reset := map[string]bool{rootID: true}
	for changed := true; changed; {
		changed = false
		for _, t := range tasks {
			if reset[t.ID] || t.TargetKind == systemdb.TaskTargetHuman ||
				t.Status == systemdb.TaskStatusCompleted {
				continue
			}
			for _, dep := range systemdb.UnmarshalAgentTaskDepends(t.DependsJSON) {
				if reset[dep] {
					reset[t.ID] = true
					changed = true
					break
				}
			}
		}
	}
	for i := range tasks {
		if !reset[tasks[i].ID] {
			continue
		}
		tasks[i].Status = systemdb.TaskStatusPending
		tasks[i].ErrorCode = ""
		if _, err := h.store.UpdateAgentTask(ctx, tasks[i], 0); err != nil {
			return err
		}
	}
	return nil
}

// resumeTurn 恢复中断/等人的轮次：中断与阻塞的助手任务重新排队后继续推进。
func (h *cloudAgentHandler) resumeTurn(c echo.Context, pc ProjectContext, turn systemdb.AgentTurn, principal auth.Principal) error {
	ctx := c.Request().Context()
	tasks, err := h.store.ListAgentTasks(ctx, pc.ID, turn.ID)
	if err != nil {
		return WriteError(c, err)
	}
	for i := range tasks {
		if tasks[i].TargetKind == systemdb.TaskTargetHuman {
			continue
		}
		if tasks[i].Status == systemdb.TaskStatusInterrupted || tasks[i].Status == systemdb.TaskStatusBlocked {
			tasks[i].Status = systemdb.TaskStatusPending
			tasks[i].ErrorCode = ""
			if _, err := h.store.UpdateAgentTask(ctx, tasks[i], 0); err != nil {
				return WriteError(c, err)
			}
		}
	}
	return h.continueTurn(c, pc, turn, principal)
}

// principalOf 从上下文取主体（内部续跑用；缺失时为零值）。
func principalOf(c echo.Context) auth.Principal {
	p, _ := PrincipalFromContext(c.Request().Context())
	return p
}

// continueTurn 在既有轮次上继续推进（不新建轮次、不重复落用户消息）。
func (h *cloudAgentHandler) continueTurn(c echo.Context, pc ProjectContext, turn systemdb.AgentTurn, principal auth.Principal) error {
	ctx := c.Request().Context()
	tc, err := h.turnContextOf(ctx, pc, turn, principal)
	if err != nil {
		return WriteError(c, err)
	}
	if h.usage != nil {
		if err := h.usage.CheckQuota(ctx, pc.ID, "llm"); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) {
				return WriteError(c, NewAPIError(http.StatusTooManyRequests, "quota_exceeded", "项目模型调用配额已用完", RequestIDFromContext(ctx)))
			}
			return WriteError(c, err)
		}
	}
	// 重新抢占会话占用（等人期间已释放）。
	if err := h.claimTurn(ctx, pc.ID, turn.ThreadID, turn.ID); err != nil {
		return WriteError(c, err)
	}
	_ = h.store.UpdateAgentTurnStatus(ctx, pc.ID, turn.ID, systemdb.TurnStatusRunning, "", "", 0)
	stream := h.openTurnStream(c, turn, len(mustTasks(ctx, h, pc.ID, turn.ID)))
	defer stream.close()
	advErr := h.advanceTurn(ctx, &tc, stream)
	if advErr != nil && !errors.Is(advErr, errTurnWaitingHuman) {
		h.failTurn(context.Background(), pc.ID, turn.ThreadID, turn.ID, advErr)
	}
	h.settleTurn(context.Background(), &tc, stream)
	return nil
}

// turnContextOf 装配继续推进所需的上下文（会话、元数据、群配置、人身份）。
func (h *cloudAgentHandler) turnContextOf(ctx context.Context, pc ProjectContext, turn systemdb.AgentTurn, principal auth.Principal) (turnContext, error) {
	thread, err := h.store.GetAgentThread(ctx, pc.ID, turn.ThreadID)
	if errors.Is(err, sql.ErrNoRows) {
		return turnContext{}, echo.NewHTTPError(http.StatusNotFound, "thread not found")
	} else if err != nil {
		return turnContext{}, err
	}
	ws, err := h.store.GetAgentThreadWorkspace(ctx, pc.ID, turn.ThreadID)
	if err != nil {
		return turnContext{}, err
	}
	collab, err := h.store.GetAgentThreadCollab(ctx, pc.ID, turn.ThreadID)
	if err != nil {
		return turnContext{}, err
	}
	var tag systemdb.AgentTag
	if turn.TagID != "" {
		if tag, err = h.resolveAgentTag(ctx, pc.ID, turn.TagID); err != nil {
			// 群已归档：历史可读，但不再推进新工作。
			return turnContext{}, ErrGroupArchived
		}
	}
	tc := turnContext{pc: pc, principal: principal, thread: thread, ws: ws, collab: collab, tag: tag, turn: turn}
	tc.humanID = principal.UserID
	if humanPolicyDisabled(tag) {
		tc.humanID = ""
	}
	return tc, nil
}

// mustTasks 是小工具：列表查询失败时返回 nil（仅用于取计数）。
func mustTasks(ctx context.Context, h *cloudAgentHandler, projectID, turnID string) []systemdb.AgentTask {
	tasks, err := h.store.ListAgentTasks(ctx, projectID, turnID)
	if err != nil {
		return nil
	}
	return tasks
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
	return c.JSON(http.StatusOK, dto)
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
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
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
		return WriteError(c, ErrTaskNotRetryable)
	}
	// 接收人本人或超级管理员可处理。
	//
	// 注意：不能用 ProjectAdmin 作为「管理员例外」——RoleUser 的默认权限集里
	// 就包含 ProjectAdmin（见 auth.PermissionsForRole），那等于项目内任何人
	// 都能代答他人待办。这里只放行 RoleSuperAdmin。
	caller := principalIdentity(principal)
	isOwner := task.AssigneeID != "" && task.AssigneeID == caller
	isSuper := hasPrincipal && principal.Role == auth.RoleSuperAdmin
	if !isOwner && !isSuper {
		return WriteError(c, ErrHumanTaskForbidden)
	}
	if task.Status != systemdb.TaskStatusWaitingHuman && task.Status != systemdb.TaskStatusPending {
		return WriteError(c, ErrTaskNotRetryable)
	}
	var body humanReplyBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if err := h.applyHumanReply(ctx, pc, &task, body, caller); err != nil {
		return WriteError(c, err)
	}
	turn, err := h.store.GetAgentTurn(ctx, pc.ID, task.TurnID)
	if err != nil {
		return WriteError(c, err)
	}
	// 回复后自动续跑；若另一轮已占用该会话，回复已记录，由前端稍后 resume。
	tc, err := h.turnContextOf(ctx, pc, turn, principal)
	if err != nil {
		return WriteError(c, err)
	}
	if err := h.claimTurn(ctx, pc.ID, turn.ThreadID, turn.ID); err != nil {
		dto, dErr := h.buildTurnDTO(ctx, pc.ID, turn)
		if dErr != nil {
			return WriteError(c, dErr)
		}
		return c.JSON(http.StatusAccepted, dto)
	}
	_ = h.store.UpdateAgentTurnStatus(ctx, pc.ID, turn.ID, systemdb.TurnStatusRunning, "", "", 0)
	stream := h.openTurnStream(c, turn, len(mustTasks(ctx, h, pc.ID, turn.ID)))
	defer stream.close()
	advErr := h.advanceTurn(ctx, &tc, stream)
	if advErr != nil && !errors.Is(advErr, errTurnWaitingHuman) {
		h.failTurn(context.Background(), pc.ID, turn.ThreadID, turn.ID, advErr)
	}
	h.settleTurn(context.Background(), &tc, stream)
	return nil
}

// applyHumanReply 校验并写入人工回复（人 task 的终态），后把回复作为历史消息落库。
func (h *cloudAgentHandler) applyHumanReply(ctx context.Context, pc ProjectContext, task *systemdb.AgentTask, body humanReplyBody, caller string) error {
	outcome := strings.TrimSpace(body.Outcome)
	if outcome == "" {
		outcome = systemdb.TaskReplyContinue
	}
	if outcome != systemdb.TaskReplyContinue && outcome != systemdb.TaskReplyUnable {
		return echo.NewHTTPError(http.StatusBadRequest, "outcome must be continue or unable")
	}
	text := strings.TrimSpace(body.Text)
	if n := len([]rune(text)); n > 8000 {
		return echo.NewHTTPError(http.StatusBadRequest, "reply is too long (max 8000 characters)")
	}
	if body.ExpectedVersion > 0 && task.Version != body.ExpectedVersion {
		return ErrTurnConflict
	}
	task.ReplyText = text
	task.ReplyOutcome = outcome
	task.RepliedBy = caller
	if outcome == systemdb.TaskReplyContinue {
		task.Status = systemdb.TaskStatusCompleted
	} else {
		task.Status = systemdb.TaskStatusFailed
		task.ErrorCode = "human_unable"
	}
	if _, err := h.store.UpdateAgentTask(ctx, *task, task.Version); err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	// 人工回复进入会话历史（带前缀，跨助手投影可识别）。
	msg, err := h.store.AppendAgentMessage(ctx, systemdb.AgentMessage{
		ThreadID: task.ThreadID, ProjectID: pc.ID, Role: "user", Content: "[人工回复] " + text,
	})
	if err != nil {
		return err
	}
	_ = h.store.AppendAgentMessageMeta(ctx, systemdb.AgentMessageMeta{
		ProjectID: pc.ID, MessageID: msg.ID, TurnID: task.TurnID, TaskID: task.ID,
		SpeakerKind: systemdb.TaskTargetHuman, SpeakerID: caller, Kind: systemdb.MessageKindHuman,
	})
	return nil
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
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	assignee := strings.TrimSpace(c.QueryParam("assignee"))
	// "me" 解析为当前登录用户；API Key 主体没有「人」的身份，只能看全部。
	if assignee == "me" {
		assignee = principal.UserID
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
