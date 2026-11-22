// 文件说明：collab_coordinator.go — 协作群的轮次编排（planv6.7 v2 §6）。
//
// 职责
//
//	RunTurn  ─ 建轮次 → 路由 → 建任务 → 串行执行 → 归约状态 → 释放占用
//	resumeTurn ─ 人工回复后从 pending/waiting_human 继续推进
//	convergeCollaboration ─ 进程启动时把残留 routing/running 收敛为 interrupted
//
// 设计要点
//
//   - 一次轮次 = 一条会话内的一个「发送」，内部拆成多个任务逐个执行。
//   - 助手任务复用现有单助手管道（Runtime.StartRun + finishAgentRun），
//     因此工具审批、事件缓存、审计全部沿用，不重写一套 run 生命周期。
//   - 人工任务没有模型 run：只落任务 + 消息，轮次转 waiting_human 后释放占用。
//   - 会话级互斥用 sys_agent_thread_collab.active_turn_id 做 CAS，进程内不另设锁。
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/usage"
)

// mentionTargetDTO 是结构化 @ 目标：{kind,id}。
type mentionTargetDTO struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// mentionAgentID 返回该目标指向的助手 ID（非助手返回空串）。
func (m mentionTargetDTO) mentionAgentID() string {
	if m.Kind == cloudagent.CollabTargetAgent {
		return strings.TrimSpace(m.ID)
	}
	return ""
}

// mentionIsHuman 报告该目标是否指向人。
func (m mentionTargetDTO) mentionIsHuman() bool {
	return m.Kind == cloudagent.CollabTargetHuman
}

// createTurnBody 是 POST /agent-threads/:threadID/turns 的请求体。
type createTurnBody struct {
	Content        string             `json:"content"`
	Mentions       []mentionTargetDTO `json:"mentions"`
	IdempotencyKey string             `json:"idempotency_key"`
	Stream         bool               `json:"stream"`
}

// turnContext 是一次轮次执行的上下文快照。
type turnContext struct {
	pc        ProjectContext
	principal auth.Principal
	thread    systemdb.AgentThread
	ws        systemdb.AgentThreadWorkspace
	collab    systemdb.AgentThreadCollab
	tag       systemdb.AgentTag
	turn      systemdb.AgentTurn
	tasks     []systemdb.AgentTask
	// humanID 是当前可指派的人（API Key 主体为空，表示不能指派）。
	humanID string
}

type turnInitiation struct {
	kind         string
	id           string
	speakerKind  string
	content      string
	mentionsJSON string
}

// CreateTurn 处理 POST /agent-threads/:threadID/turns。
func (h *cloudAgentHandler) CreateTurn(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	threadID := c.Param("threadID")
	thread, err := h.store.GetAgentThread(c.Request().Context(), pc.ID, threadID)
	if errors.Is(err, sql.ErrNoRows) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "thread not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	var body createTurnBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "content is required"))
	}
	ws, err := h.store.GetAgentThreadWorkspace(c.Request().Context(), pc.ID, threadID)
	if err != nil {
		return WriteError(c, err)
	}
	collab, err := h.store.GetAgentThreadCollab(c.Request().Context(), pc.ID, threadID)
	if err != nil {
		return WriteError(c, err)
	}
	// 群归属以会话元数据为准。
	tagID := ws.TagID
	if collab.TagID != "" {
		tagID = collab.TagID
	}
	var tag systemdb.AgentTag
	if tagID != "" {
		tag, err = h.resolveAgentTag(c.Request().Context(), pc.ID, tagID)
		if err != nil {
			// 群已归档：历史可读，但不接受新发送（v2 §9.2）。
			if errors.Is(err, systemdb.ErrAgentTagNotFound) {
				return WriteError(c, ErrGroupArchived)
			}
			return WriteError(c, err)
		}
	}
	// 协作表未就绪：明确告知需要先建表，而不是 500。
	if err := h.store.CollabSchemaReady(c.Request().Context()); err != nil {
		return WriteError(c, ErrWorkspaceUpgradeRequired)
	}
	if h.runtime == nil || h.runtime.LLM == nil {
		return WriteError(c, &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"})
	}
	if h.usage != nil {
		if err := h.usage.CheckQuota(c.Request().Context(), pc.ID, "llm"); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) {
				return WriteError(c, NewAPIError(http.StatusTooManyRequests, "quota_exceeded", "项目模型调用配额已用完", RequestIDFromContext(c.Request().Context())))
			}
			return WriteError(c, err)
		}
	}
	tc := turnContext{pc: pc, principal: principal, thread: thread, ws: ws, collab: collab, tag: tag}
	// 人只能由登录态担任：API Key 没有「人」的身份（v2 §7.3）。
	tc.humanID = principal.UserID
	if humanPolicyDisabled(tag) {
		tc.humanID = ""
	}
	return h.runTurn(c, tc, body, content)
}

// humanPolicyDisabled 报告群是否禁止人工任务。
func humanPolicyDisabled(tag systemdb.AgentTag) bool {
	return tag.Config.HumanPolicy == systemdb.HumanPolicyDisabled
}

// runTurn 准备并执行一个 HTTP 轮次。
func (h *cloudAgentHandler) runTurn(c echo.Context, tc turnContext, body createTurnBody, content string) error {
	ctx := c.Request().Context()
	reused, err := h.prepareTurn(ctx, &tc, body, content, turnInitiation{
		kind: systemdb.TurnInitiatorUser, id: principalIdentity(tc.principal), speakerKind: "user",
		content: content, mentionsJSON: marshalMentions(body.Mentions),
	})
	if err != nil {
		return WriteError(c, err)
	}
	if reused {
		return h.replayTurnSnapshot(c, tc.pc.ID, tc.turn)
	}

	var stream *turnStream
	if body.Stream {
		stream = h.openTurnStream(c, tc.turn, len(tc.tasks))
		defer stream.close()
		stream.emitRoute(tc.tasks)
	}
	// 串行推进（依赖就绪才执行）。等人不是错误：轮次转入 waiting_human 后正常收尾。
	err = h.advanceTurn(ctx, &tc, stream)
	if err != nil && !errors.Is(err, errTurnWaitingHuman) {
		h.failTurn(context.Background(), tc.pc.ID, tc.thread.ID, tc.turn.ID, err)
	}
	h.settleTurn(context.Background(), &tc, stream)
	if body.Stream {
		return nil
	}
	return h.respondTurnSnapshot(c, tc.pc.ID, tc)
}

// prepareTurn 建轮次、占锁、落消息、路由并持久化任务。
func (h *cloudAgentHandler) prepareTurn(ctx context.Context, tc *turnContext, body createTurnBody, content string, initiation turnInitiation) (bool, error) {
	pid, threadID := tc.pc.ID, tc.thread.ID
	turn, reused, err := h.store.CreateAgentTurn(ctx, systemdb.AgentTurn{
		ProjectID: pid, ThreadID: threadID, TagID: tagIDOf(*tc),
		InitiatorKind: initiation.kind, InitiatorID: initiation.id,
		IdempotencyKey:   strings.TrimSpace(body.IdempotencyKey),
		Status:           systemdb.TurnStatusRouting,
		ConfigVersion:    tc.tag.Version,
		ConfigSnapshotJS: marshalTagSnapshot(tc.tag),
	})
	if err != nil {
		return false, err
	}
	tc.turn = turn
	if reused {
		return true, nil
	}
	if err := h.claimTurn(ctx, pid, threadID, turn.ID); err != nil {
		_ = h.store.UpdateAgentTurnStatus(context.Background(), pid, turn.ID, systemdb.TurnStatusFailed, "", "turn_busy", 0)
		return false, err
	}
	userMsg, err := h.store.AppendAgentMessage(ctx, systemdb.AgentMessage{
		ThreadID: threadID, ProjectID: pid, Role: "user", Content: initiation.content,
		MentionsJSON: initiation.mentionsJSON,
	})
	if err != nil {
		h.releaseTurn(context.Background(), pid, threadID, turn.ID)
		return false, err
	}
	_ = h.store.AppendAgentMessageMeta(ctx, systemdb.AgentMessageMeta{
		ProjectID: pid, MessageID: userMsg.ID, TurnID: turn.ID,
		SpeakerKind: initiation.speakerKind, SpeakerID: initiation.id, Kind: systemdb.MessageKindUser,
	})

	plan, err := h.resolveDispatchPlan(ctx, *tc, body, content)
	if err != nil {
		h.failTurn(context.Background(), pid, threadID, turn.ID, err)
		return false, err
	}
	tasks, err := h.persistDispatchPlan(ctx, *tc, plan)
	if err != nil {
		h.failTurn(context.Background(), pid, threadID, turn.ID, err)
		return false, err
	}
	tc.tasks = tasks
	if len(tasks) == 0 {
		h.failTurn(context.Background(), pid, threadID, turn.ID, ErrRoutingFailed)
		return false, ErrRoutingFailed
	}
	routeJSON := marshalRouteCard(*tc, tasks, plan)
	_ = h.store.UpdateAgentTurnStatus(ctx, pid, turn.ID, systemdb.TurnStatusRunning, routeJSON, "", 0)
	return false, nil
}

// tagIDOf 返回会话所属群 ID（会话元数据优先）。
func tagIDOf(tc turnContext) string {
	if tc.ws.TagID != "" {
		return tc.ws.TagID
	}
	return tc.collab.TagID
}

// claimTurn 抢占会话活动轮次；旧会话缺协作行时先补行再抢占。
func (h *cloudAgentHandler) claimTurn(ctx context.Context, projectID, threadID, turnID string) error {
	err := h.store.ClaimAgentTurn(ctx, projectID, threadID, turnID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, systemdb.ErrTurnBusy) {
		return err
	}
	// 协作行缺失（旧 auto 会话）：补一行后重试一次。
	existing, getErr := h.store.GetAgentThreadCollab(ctx, projectID, threadID)
	if getErr != nil {
		return getErr
	}
	if existing.ThreadID != "" {
		return ErrTurnBusy
	}
	if _, err := h.store.CreateAgentThreadCollab(ctx, systemdb.AgentThreadCollab{
		ProjectID: projectID, ThreadID: threadID,
	}); err != nil {
		return err
	}
	if err := h.store.ClaimAgentTurn(ctx, projectID, threadID, turnID); err != nil {
		return ErrTurnBusy
	}
	return nil
}

// releaseTurn 释放会话活动轮次（幂等）。
func (h *cloudAgentHandler) releaseTurn(ctx context.Context, projectID, threadID, turnID string) {
	_ = h.store.ReleaseAgentTurn(ctx, projectID, threadID, turnID)
}

// resolveDispatchPlan 决定本轮分工：显式 @ → 直接分派；否则调路由器。
func (h *cloudAgentHandler) resolveDispatchPlan(ctx context.Context, tc turnContext, body createTurnBody, content string) (cloudagent.DispatchPlan, error) {
	cands, err := h.workspaceCandidates(ctx, tc.pc.ID, tc.ws)
	if err != nil {
		return cloudagent.DispatchPlan{}, err
	}
	if len(cands) == 0 {
		return cloudagent.DispatchPlan{}, ErrNoAvailableAgent
	}
	memberIDs := make([]string, 0, len(cands))
	for _, a := range cands {
		memberIDs = append(memberIDs, a.ID)
	}
	// 显式 @：逐个校验成员资格与人身份。
	var agentIDs []string
	var wantsHuman bool
	for _, m := range body.Mentions {
		if m.mentionIsHuman() {
			if tc.humanID == "" {
				return cloudagent.DispatchPlan{}, ErrHumanAssigneeRequired
			}
			wantsHuman = true
			continue
		}
		id := m.mentionAgentID()
		if id == "" {
			continue
		}
		if !slices.Contains(memberIDs, id) {
			return cloudagent.DispatchPlan{}, ErrAgentNotInTag
		}
		if !slices.Contains(agentIDs, id) {
			agentIDs = append(agentIDs, id)
		}
	}
	if len(agentIDs) > 0 || wantsHuman {
		human := ""
		if wantsHuman {
			human = tc.humanID
		}
		plan := cloudagent.BuildMentionDispatchPlan(content, agentIDs, human)
		if err := cloudagent.ValidateDispatchPlan(plan, cloudagent.DefaultPlanLimits(), memberIDs, human, human != ""); err != nil {
			return cloudagent.DispatchPlan{}, ErrInvalidDispatchGraph
		}
		return plan, nil
	}
	// 单候选：不调模型，直接分派（与旧 auto 行为一致）。
	if len(cands) == 1 {
		plan := cloudagent.BuildMentionDispatchPlan(content, []string{cands[0].ID}, "")
		return plan, nil
	}
	if h.dispatcher == nil {
		// 模型未配置：只用群配置的兜底助手，绝不静默取第一个成员。
		fallback, fErr := h.fallbackMember(tc, cands)
		if fErr != nil {
			return cloudagent.DispatchPlan{}, fErr
		}
		return cloudagent.BuildMentionDispatchPlan(content, []string{fallback.ID}, ""), nil
	}
	return h.dispatchByModel(ctx, tc, cands, memberIDs, content)
}

// fallbackMember 返回群配置的兜底助手（必须仍是未归档成员）。
func (h *cloudAgentHandler) fallbackMember(tc turnContext, cands []systemdb.CloudAgent) (systemdb.CloudAgent, error) {
	id := strings.TrimSpace(tc.tag.Config.FallbackAgentID)
	if id == "" {
		return systemdb.CloudAgent{}, ErrRoutingFailed
	}
	for _, a := range cands {
		if a.ID == id {
			return a, nil
		}
	}
	return systemdb.CloudAgent{}, ErrGroupMemberUnavailable
}

// dispatchByModel 调用路由器得到多任务分工。
func (h *cloudAgentHandler) dispatchByModel(ctx context.Context, tc turnContext, cands []systemdb.CloudAgent, memberIDs []string, content string) (cloudagent.DispatchPlan, error) {
	routeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	// 近期历史（排除本轮用户消息，它还没进模型上下文）。
	hist, _ := h.store.ListAgentMessages(ctx, tc.pc.ID, tc.thread.ID, 20)
	names := agentNamesOf(cands)
	recent := make([]cloudagent.CollabTurn, 0, 6)
	start := 0
	if len(hist) > 6 {
		start = len(hist) - 6
	}
	for _, m := range hist[start:] {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		recent = append(recent, cloudagent.CollabTurn{
			Role: m.Role, Speaker: names[m.AgentID], Content: m.Content,
		})
	}
	input := cloudagent.CollabDispatchInput{
		RoutingPrompt: tc.tag.Config.RoutingPrompt,
		Candidates:    collabCandidatesOf(cands),
		HumanID:       tc.humanID,
		HumanName:     tc.humanID,
		UserText:      content,
		Recent:        recent,
		Limits:        cloudagent.DefaultPlanLimits(),
	}
	raw, err := h.dispatcher.RoutePlan(routeCtx, tc.pc.ID, input)
	if err != nil {
		if fallback, fErr := h.fallbackMember(tc, cands); fErr == nil {
			return cloudagent.BuildMentionDispatchPlan(content, []string{fallback.ID}, ""), nil
		}
		if tc.humanID != "" {
			// 没有兜底但有当前人：请人澄清，不静默选成员。
			return cloudagent.BuildMentionDispatchPlan(content, nil, tc.humanID), nil
		}
		return cloudagent.DispatchPlan{}, ErrRoutingFailed
	}
	// 没有可指派的人时仍要能解析出「本轮需要人」，以便返回准确的错误码：
	// 先按哨兵人解析，命中 human 目标且无人可派 → human_assignee_required。
	parseHuman := tc.humanID
	anonymousHuman := ""
	if parseHuman == "" {
		anonymousHuman = collabHumanPlaceholder
		parseHuman = anonymousHuman
	}
	plan, err := cloudagent.ParseDispatchPlan(raw, memberIDs, parseHuman)
	if err != nil {
		return cloudagent.DispatchPlan{}, ErrRoutingFailed
	}
	if anonymousHuman != "" && planNeedsHuman(plan, anonymousHuman) {
		return cloudagent.DispatchPlan{}, ErrHumanAssigneeRequired
	}
	if err := cloudagent.ValidateDispatchPlan(plan, cloudagent.DefaultPlanLimits(), memberIDs,
		tc.humanID, tc.humanID != ""); err != nil {
		if anonymousHuman != "" {
			return cloudagent.DispatchPlan{}, ErrHumanAssigneeRequired
		}
		return cloudagent.DispatchPlan{}, ErrInvalidDispatchGraph
	}
	return plan, nil
}

// collabHumanPlaceholder 是「没有人可派」时用于解析的临时人标识，
// 只存在于内存，永远不会落库。
const collabHumanPlaceholder = "\x00human"

// planNeedsHuman 报告分工里是否包含指向该人的任务。
func planNeedsHuman(plan cloudagent.DispatchPlan, humanID string) bool {
	for _, t := range plan.Tasks {
		if t.TargetKind == cloudagent.CollabTargetHuman && t.TargetID == humanID {
			return true
		}
	}
	return false
}

// collabCandidatesOf 把助手行转成路由器可见的候选（不含密钥与完整工具 schema）。
func collabCandidatesOf(cands []systemdb.CloudAgent) []cloudagent.CollabCandidate {
	out := make([]cloudagent.CollabCandidate, 0, len(cands))
	for _, a := range cands {
		// 所有助手都可写：不再把写能力作为路由依据（planv6.7 v2 §3.4）。
		item := cloudagent.CollabCandidate{
			ID: a.ID, Name: a.Name, Module: a.Module, Description: a.Description,
		}
		if len(a.ToolIDs) > 0 {
			hints := make([]string, 0, 3)
			for i, id := range a.ToolIDs {
				if i >= 3 {
					break
				}
				hints = append(hints, id)
			}
			item.SkillHints = hints
		}
		out = append(out, item)
	}
	return out
}

// persistDispatchPlan 落任务行，并把依赖 ref 换成真实任务 ID。
func (h *cloudAgentHandler) persistDispatchPlan(ctx context.Context, tc turnContext, plan cloudagent.DispatchPlan) ([]systemdb.AgentTask, error) {
	refToID := make(map[string]string, len(plan.Tasks))
	// 先分配 ID，保证依赖可以指向后面的任务行（校验时已保证只引用更早的 ref）。
	for i := range plan.Tasks {
		refToID[plan.Tasks[i].Ref] = uuid.NewString()
	}
	out := make([]systemdb.AgentTask, 0, len(plan.Tasks))
	for i, t := range plan.Tasks {
		depends := make([]string, 0, len(t.DependsOn))
		for _, ref := range t.DependsOn {
			if id, ok := refToID[ref]; ok {
				depends = append(depends, id)
			}
		}
		target := t.TargetID
		status := systemdb.TaskStatusPending
		assignee := ""
		if t.TargetKind == cloudagent.CollabTargetHuman {
			status = systemdb.TaskStatusWaitingHuman
			assignee = target
		}
		created, err := h.store.CreateAgentTask(ctx, systemdb.AgentTask{
			ID: refToID[t.Ref], ProjectID: tc.pc.ID, TurnID: tc.turn.ID, ThreadID: tc.thread.ID,
			Seq: int64(i + 1), TargetKind: t.TargetKind, TargetID: target,
			Instruction: t.Instruction, DependsJSON: systemdb.MarshalAgentTaskDepends(depends),
			Status: status, AssigneeID: assignee,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, created)
	}
	return out, nil
}

// advanceTurn 按依赖串行推进任务，直到没有可执行任务。
func (h *cloudAgentHandler) advanceTurn(ctx context.Context, tc *turnContext, stream *turnStream) error {
	for {
		tasks, err := h.store.ListAgentTasks(ctx, tc.pc.ID, tc.turn.ID)
		if err != nil {
			return err
		}
		statuses := make([]string, len(tasks))
		depends := make([][]int, len(tasks))
		index := make(map[string]int, len(tasks))
		for i, t := range tasks {
			index[t.ID] = i
		}
		for i, t := range tasks {
			statuses[i] = t.Status
			for _, dep := range systemdb.UnmarshalAgentTaskDepends(t.DependsJSON) {
				if j, ok := index[dep]; ok {
					depends[i] = append(depends[i], j)
				}
			}
		}
		// 上游失败 → 下游置 blocked，避免「等一个永远不会完成的依赖」。
		for _, i := range cloudagent.BlockedTasks(statuses, depends) {
			h.setTaskStatus(ctx, tc, &tasks[i], systemdb.TaskStatusBlocked, "", "")
			statuses[i] = systemdb.TaskStatusBlocked
			if stream != nil {
				stream.emitTask(tasks[i], cloudagent.CollabTaskBlocked)
			}
		}
		// 人工任务落库时就是 waiting_human；对推进循环而言它与 pending 等价
		// （需要被「通知到」，而不是被跳过）。
		ready := make([]string, len(statuses))
		copy(ready, statuses)
		for i, t := range tasks {
			if t.TargetKind == systemdb.TaskTargetHuman && ready[i] == systemdb.TaskStatusWaitingHuman {
				ready[i] = systemdb.TaskStatusPending
			}
		}
		next := cloudagent.NextTaskIndex(ready, depends)
		if next < 0 {
			return nil
		}
		task := tasks[next]
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if task.TargetKind == systemdb.TaskTargetHuman {
			// 人任务：转 waiting_human 并立即发 human 帧，然后停止推进（等回复接口续跑）。
			if task.Status != systemdb.TaskStatusWaitingHuman {
				h.setTaskStatus(ctx, tc, &task, systemdb.TaskStatusWaitingHuman, task.AssigneeID, "")
			}
			if stream != nil {
				stream.emitHuman(&task)
			}
			return errTurnWaitingHuman
		}
		if err := h.executeAgentTask(ctx, tc, &task, stream); err != nil {
			return err
		}
	}
}

// errTurnWaitingHuman 是「轮次停下等人」的内部信号，不是错误。
var errTurnWaitingHuman = errors.New("turn is waiting for human")

// executeAgentTask 执行一个助手任务：建 run → StartRun → 收尾。
func (h *cloudAgentHandler) executeAgentTask(ctx context.Context, tc *turnContext, task *systemdb.AgentTask, stream *turnStream) error {
	agent, err := h.store.GetCloudAgent(ctx, tc.pc.ID, task.TargetID)
	if errors.Is(err, sql.ErrNoRows) {
		h.setTaskStatus(ctx, tc, task, systemdb.TaskStatusFailed, "", string(ErrGroupMemberUnavailable.Error()))
		if stream != nil {
			stream.emitTask(*task, cloudagent.CollabTaskFailed)
		}
		return nil
	}
	if err != nil {
		return err
	}
	// 能力门槛：与单助手路径保持一致。
	if (agent.Module == agentbridge.ModuleKV || agent.Module == agentbridge.ModuleGoFunction || agent.Module == agentbridge.ModuleCron) && (!h.skillsCLI()) {
		h.setTaskStatus(ctx, tc, task, systemdb.TaskStatusFailed, "", "cli_skills_unavailable")
		return nil
	}
	if agent.Module == agentbridge.ModuleSandbox && !h.sandboxAvailable() {
		h.setTaskStatus(ctx, tc, task, systemdb.TaskStatusFailed, "", "sandbox_unavailable")
		return nil
	}
	runID := uuid.NewString()
	now := time.Now().UTC()
	run, err := h.store.CreateAgentRun(ctx, systemdb.AgentRun{
		ID: runID, ThreadID: tc.thread.ID, ProjectID: tc.pc.ID, AgentID: agent.ID,
		Status: systemdb.AgentRunRunning, StartedAt: now,
		InitiatorID: principalIdentity(tc.principal),
	})
	if err != nil {
		return err
	}
	task.Attempt++
	h.setTaskStatus(ctx, tc, task, systemdb.TaskStatusRunning, "", "")
	task.RunID = run.ID
	if _, err := h.store.UpdateAgentTask(ctx, *task, 0); err != nil {
		return err
	}
	_ = h.store.AppendAgentTaskRun(ctx, systemdb.AgentTaskRun{
		ProjectID: tc.pc.ID, TaskID: task.ID, Attempt: task.Attempt, RunID: run.ID, Status: systemdb.TaskStatusRunning,
	})
	// 历史投影：跨助手消息加 [助手名] 前缀，避免本轮助手模仿别人的工具调用。
	hist, _ := h.store.ListAgentMessages(ctx, tc.pc.ID, tc.thread.ID, 40)
	names := agentNamesOfFromStore(ctx, h, tc.pc.ID)
	req := cloudagent.RunRequest{
		Agent: agentbridge.Agent(agent), ThreadID: tc.thread.ID, RunID: run.ID, UserText: task.Instruction,
		History: agentbridge.MessagesFor(hist, agent.ID, names), Stream: stream != nil, Subject: principalIdentity(tc.principal),
		Context: agentbridge.RunContext{ProjectID: tc.pc.ID, Principal: tc.principal, Skills: agentbridge.SkillsForModule(agent.Module)},
	}
	runCtx := llmgateway.WithTraceMeta(ctx, run.ID, "agent")
	if stream != nil {
		stream.emitTask(*task, cloudagent.CollabTaskRunning)
		stream.emitRun(run, agent.ID)
	}
	res, runErr := h.runtime.StartRun(runCtx, req, func(ev cloudagent.Event) {
		if stream == nil {
			return
		}
		// 子 run 的流事件带上 task_id，前端按任务分组渲染。
		ev.RunID = run.ID
		stream.emitSubRun(task, ev)
	})
	// 收尾复用既有逻辑（写 assistant 消息 + run 状态 + metrics + 审计）。
	if err := h.finishAgentRun(context.Background(), tc.pc.ID, run, agent, res, runErr); err != nil {
		return err
	}
	status := systemdb.TaskStatusCompleted
	errCode := ""
	if runErr != nil {
		status = systemdb.TaskStatusFailed
		errCode = agentbridge.ClassifyError(runErr).Code
	}
	h.setTaskStatus(ctx, tc, task, status, "", errCode)
	_ = h.store.AppendAgentTaskRun(ctx, systemdb.AgentTaskRun{
		ProjectID: tc.pc.ID, TaskID: task.ID, Attempt: task.Attempt, RunID: run.ID, Status: status,
	})
	if stream != nil {
		stream.emitTask(*task, status)
	}
	return nil
}

// setTaskStatus 更新任务状态（尽力而为：失败只记日志，不中断整轮）。
func (h *cloudAgentHandler) setTaskStatus(ctx context.Context, tc *turnContext, task *systemdb.AgentTask, status, assignee, errCode string) {
	current, err := h.store.GetAgentTask(ctx, tc.pc.ID, task.ID)
	if err != nil {
		return
	}
	current.Status = status
	if assignee != "" || status == systemdb.TaskStatusWaitingHuman {
		current.AssigneeID = assignee
	}
	if errCode != "" {
		current.ErrorCode = errCode
	}
	updated, err := h.store.UpdateAgentTask(ctx, current, current.Version)
	if err != nil {
		return
	}
	*task = updated
}

// settleTurn 归约轮次终态并释放占用。
func (h *cloudAgentHandler) settleTurn(ctx context.Context, tc *turnContext, stream *turnStream) {
	tasks, err := h.store.ListAgentTasks(ctx, tc.pc.ID, tc.turn.ID)
	if err != nil {
		return
	}
	statuses := make([]string, len(tasks))
	for i, t := range tasks {
		statuses[i] = t.Status
	}
	state := cloudagent.ReduceTurnState(statuses)
	// 失败依赖把下游 blocked 后，若仍有人任务在等，则整轮等人而不是失败。
	_ = h.store.UpdateAgentTurnStatus(ctx, tc.pc.ID, tc.turn.ID, state.Status, "", state.ErrorCode, 0)
	// 轮次结束或等待人工时都释放占用；人工回复会重新抢占。
	h.releaseTurn(ctx, tc.pc.ID, tc.thread.ID, tc.turn.ID)
	if stream != nil {
		stream.emitTurn(state)
		if state.Status == "" || state.Status == systemdb.TurnStatusCompleted ||
			state.Status == systemdb.TurnStatusFailed || state.Status == systemdb.TurnStatusCanceled {
			stream.writeEnd(state.Status)
		}
	}
}

// failTurn 把轮次置失败并释放占用（defer 兜底路径用）。
func (h *cloudAgentHandler) failTurn(ctx context.Context, projectID, threadID, turnID string, cause error) {
	code := "routing_failed"
	switch {
	case errors.Is(cause, ErrRoutingFailed):
		code = "routing_failed"
	case errors.Is(cause, ErrInvalidDispatchGraph):
		code = "invalid_dispatch_graph"
	case errors.Is(cause, ErrHumanAssigneeRequired):
		code = "human_assignee_required"
	case errors.Is(cause, ErrNoAvailableAgent):
		code = "no_available_agent"
	case errors.Is(cause, context.Canceled):
		_ = h.store.UpdateAgentTurnStatus(ctx, projectID, turnID, systemdb.TurnStatusCanceled, "", "canceled", 0)
		h.releaseTurn(ctx, projectID, threadID, turnID)
		return
	}
	_ = h.store.UpdateAgentTurnStatus(ctx, projectID, turnID, systemdb.TurnStatusFailed, "", code, 0)
	h.releaseTurn(ctx, projectID, threadID, turnID)
}

// marshalMentions 序列化 @ 目标。
func marshalMentions(mentions []mentionTargetDTO) string {
	if len(mentions) == 0 {
		return "[]"
	}
	b, err := json.Marshal(mentions)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// marshalTagSnapshot 记录群配置快照（不含任何凭据）。
func marshalTagSnapshot(tag systemdb.AgentTag) string {
	snapshot := map[string]any{
		"id": tag.ID, "name": tag.Name, "version": tag.Version,
		"agent_ids": tag.Config.AgentIDs, "routing_prompt": tag.Config.RoutingPrompt,
		"fallback_agent_id": tag.Config.FallbackAgentID, "human_policy": tag.Config.HumanPolicy,
	}
	b, err := json.Marshal(snapshot)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// marshalRouteCard 生成路由卡（SSE 与 route_json 共用）。
func marshalRouteCard(tc turnContext, tasks []systemdb.AgentTask, plan cloudagent.DispatchPlan) string {
	card := map[string]any{"turn_id": tc.turn.ID, "need_summary": plan.NeedSummary, "reason": plan.Reason}
	steps := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		steps = append(steps, map[string]any{
			"task_id": t.ID, "seq": t.Seq, "target_kind": t.TargetKind,
			"target_id": t.TargetID, "instruction": t.Instruction, "status": t.Status,
		})
	}
	card["steps"] = steps
	b, err := json.Marshal(card)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func agentNamesOfFromStore(ctx context.Context, h *cloudAgentHandler, projectID string) map[string]string {
	agents, err := h.store.ListCloudAgents(ctx, projectID)
	if err != nil {
		return nil
	}
	out := make(map[string]string, len(agents))
	for _, a := range agents {
		out[a.ID] = a.Name
	}
	return out
}

// convergeCollaboration 在进程启动时把残留的 routing/running 轮次收敛为 interrupted，
// 不自动重跑（避免重复副作用）：用户可在界面上手动重试任务。
func (h *cloudAgentHandler) convergeCollaboration(ctx context.Context) {
	if h.store == nil {
		return
	}
	if err := h.store.CollabSchemaReady(ctx); err != nil {
		// 未升级环境静默跳过：协作功能不可用，但普通会话不受影响。
		return
	}
	turns, err := h.store.ListRunningAgentTurns(ctx, time.Now().UTC())
	if err != nil {
		return
	}
	for _, t := range turns {
		_ = h.store.UpdateAgentTurnStatus(ctx, t.ProjectID, t.ID, systemdb.TurnStatusInterrupted, "", "interrupted", 0)
		h.releaseTurn(ctx, t.ProjectID, t.ThreadID, t.ID)
		tasks, err := h.store.ListAgentTasks(ctx, t.ProjectID, t.ID)
		if err != nil {
			continue
		}
		for i := range tasks {
			if tasks[i].Status == systemdb.TaskStatusRunning || tasks[i].Status == systemdb.TaskStatusPending {
				tasks[i].Status = systemdb.TaskStatusInterrupted
				_, _ = h.store.UpdateAgentTask(ctx, tasks[i], 0)
			}
		}
	}
}

// turnStream 是轮次级 SSE 写入器：所有帧共用一条连接与单调 id。
type turnStream struct {
	c       echo.Context
	flusher http.Flusher
	mu      sync.Mutex
	seq     int64
	h       *cloudAgentHandler
	turnID  string
	project string
	// total 是轮次任务数（构造时确定，避免每帧重复查库）。
	total int
}

func (h *cloudAgentHandler) openTurnStream(c echo.Context, turn systemdb.AgentTurn, total int) *turnStream {
	beginSSE(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set("X-Turn-Id", turn.ID)
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	return &turnStream{c: c, flusher: flusher, h: h, turnID: turn.ID, project: turn.ProjectID, total: total}
}

func (t *turnStream) write(ev cloudagent.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	writeSSEFrame(t.c.Response().Writer, t.flusher, t.seq, ev)
}

func (t *turnStream) close() {
	t.write(cloudagent.Event{Type: "end"})
}

// recordEvent 落事件表（快照 + 续传的持久面）。
func (t *turnStream) recordEvent(kind, entityID string, payload map[string]any) {
	if t.h == nil || t.h.store == nil {
		return
	}
	seq, err := t.h.store.LastAgentTurnSeq(context.Background(), t.project, t.turnID)
	if err != nil {
		return
	}
	body, _ := json.Marshal(payload)
	_, _ = t.h.store.AppendAgentTurnEvent(context.Background(), systemdb.AgentTurnEvent{
		ProjectID: t.project, TurnID: t.turnID, Seq: seq + 1,
		Kind: kind, EntityID: entityID, PayloadJSON: string(body),
	})
}

func (t *turnStream) emitRun(run systemdb.AgentRun, agentID string) {
	t.write(cloudagent.Event{Type: "run", RunID: run.ID, AgentID: agentID})
}

// emitSubRun 转发子 run 事件（跳过每个子 run 自己的 end，避免提前结束轮次流）。
func (t *turnStream) emitSubRun(task *systemdb.AgentTask, ev cloudagent.Event) {
	if ev.Type == "end" || ev.Type == "run" {
		return
	}
	ev.AgentID = task.TargetID
	t.write(ev)
}

func (t *turnStream) emitTask(task systemdb.AgentTask, status string) {
	t.write(cloudagent.Event{
		Type: "task", TaskID: task.ID, AgentID: task.TargetID, Index: int(task.Seq), Total: t.total,
		Status: status, Message: task.Instruction,
	})
	t.recordEvent("task", task.ID, map[string]any{
		"status": status, "target_kind": task.TargetKind, "target_id": task.TargetID,
	})
}

func (t *turnStream) emitRoute(tasks []systemdb.AgentTask) {
	steps := make([]cloudagent.EventStep, 0, len(tasks))
	for _, task := range tasks {
		step := cloudagent.EventStep{TaskID: task.ID, AgentID: task.TargetID, Instruction: task.Instruction}
		if task.TargetKind == systemdb.TaskTargetHuman {
			step.Kind = cloudagent.CollabTargetHuman
		} else {
			step.Kind = cloudagent.CollabTargetAgent
		}
		steps = append(steps, step)
	}
	t.write(cloudagent.Event{Type: "route", Status: "done", Steps: steps})
	t.recordEvent("route", "", map[string]any{"tasks": len(tasks)})
}

func (t *turnStream) emitHuman(task *systemdb.AgentTask) {
	t.write(cloudagent.Event{
		Type: "human", TaskID: task.ID, HumanID: task.AssigneeID, HumanName: task.AssigneeID,
		Index: int(task.Seq), Message: task.Instruction,
	})
	t.recordEvent("human", task.ID, map[string]any{"assignee_id": task.AssigneeID})
}

func (t *turnStream) emitTurn(state cloudagent.TurnState) {
	t.write(cloudagent.Event{Type: "turn", Status: state.Status, Total: state.Total, Index: state.Completed})
	t.recordEvent("turn", t.turnID, map[string]any{"status": state.Status, "completed": state.Completed})
}

func (t *turnStream) writeEnd(reason string) {
	t.write(cloudagent.Event{Type: "end", Reason: reason})
}

// turnDTO 是轮次的响应形态（含任务与进度）。
type turnDTO struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"thread_id"`
	TagID     string    `json:"tag_id,omitempty"`
	Status    string    `json:"status"`
	ErrorCode string    `json:"error_code,omitempty"`
	RouteJSON string    `json:"route,omitempty"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Tasks     []taskDTO `json:"tasks"`
	// LastEventSeq 是事件表里的最大序号，供客户端续传。
	LastEventSeq int64 `json:"last_event_seq"`
}

// taskDTO 是任务的响应形态。
type taskDTO struct {
	ID           string    `json:"id"`
	TurnID       string    `json:"turn_id"`
	Seq          int64     `json:"seq"`
	TargetKind   string    `json:"target_kind"`
	TargetID     string    `json:"target_id"`
	Instruction  string    `json:"instruction"`
	Status       string    `json:"status"`
	Attempt      int64     `json:"attempt"`
	RunID        string    `json:"run_id,omitempty"`
	AssigneeID   string    `json:"assignee_id,omitempty"`
	ReplyText    string    `json:"reply_text,omitempty"`
	ReplyOutcome string    `json:"reply_outcome,omitempty"`
	RepliedBy    string    `json:"replied_by,omitempty"`
	DependsOn    []string  `json:"depends_on"`
	ErrorCode    string    `json:"error_code,omitempty"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
}

func toTaskDTO(t systemdb.AgentTask) taskDTO {
	return taskDTO{
		ID: t.ID, TurnID: t.TurnID, Seq: t.Seq, TargetKind: t.TargetKind, TargetID: t.TargetID,
		Instruction: t.Instruction, Status: t.Status, Attempt: t.Attempt, RunID: t.RunID,
		AssigneeID: t.AssigneeID, ReplyText: t.ReplyText, ReplyOutcome: t.ReplyOutcome,
		RepliedBy: t.RepliedBy, DependsOn: systemdb.UnmarshalAgentTaskDepends(t.DependsJSON),
		ErrorCode: t.ErrorCode, Version: t.Version, CreatedAt: t.CreatedAt,
	}
}

// buildTurnDTO 组装轮次快照（含任务与事件序号）。
func (h *cloudAgentHandler) buildTurnDTO(ctx context.Context, projectID string, turn systemdb.AgentTurn) (turnDTO, error) {
	tasks, err := h.store.ListAgentTasks(ctx, projectID, turn.ID)
	if err != nil {
		return turnDTO{}, err
	}
	dto := turnDTO{
		ID: turn.ID, ThreadID: turn.ThreadID, TagID: turn.TagID, Status: turn.Status,
		ErrorCode: turn.ErrorCode, RouteJSON: turn.RouteJSON, Version: turn.Version,
		CreatedAt: turn.CreatedAt, UpdatedAt: turn.UpdatedAt,
		Tasks: make([]taskDTO, 0, len(tasks)),
	}
	for _, t := range tasks {
		dto.Tasks = append(dto.Tasks, toTaskDTO(t))
	}
	if seq, err := h.store.LastAgentTurnSeq(ctx, projectID, turn.ID); err == nil {
		dto.LastEventSeq = seq
	}
	return dto, nil
}

// replayTurnSnapshot 幂等重放：同幂等键命中时直接返回既有快照。
func (h *cloudAgentHandler) replayTurnSnapshot(c echo.Context, projectID string, turn systemdb.AgentTurn) error {
	dto, err := h.buildTurnDTO(c.Request().Context(), projectID, turn)
	if err != nil {
		return WriteError(c, err)
	}
	if strings.TrimSpace(c.QueryParam("stream")) == "true" {
		// 流式幂等重放：写一帧 route 卡 + 各任务状态 + end，让客户端状态自洽。
		tasks, _ := h.store.ListAgentTasks(c.Request().Context(), projectID, turn.ID)
		stream := h.openTurnStream(c, turn, len(tasks))
		defer stream.close()
		stream.emitRoute(tasks)
		for _, t := range tasks {
			stream.emitTask(t, t.Status)
		}
		stream.writeEnd(turn.Status)
		return nil
	}
	return c.JSON(http.StatusAccepted, dto)
}

// respondTurnSnapshot 非流式响应：返回轮次与任务快照（202）。
func (h *cloudAgentHandler) respondTurnSnapshot(c echo.Context, projectID string, tc turnContext) error {
	turn, err := h.store.GetAgentTurn(c.Request().Context(), projectID, tc.turn.ID)
	if err != nil {
		return WriteError(c, err)
	}
	dto, err := h.buildTurnDTO(c.Request().Context(), projectID, turn)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusAccepted, dto)
}

// ListThreadTurns 返回会话的轮次列表（GET /agent-threads/:threadID/turns）。
func (h *cloudAgentHandler) ListThreadTurns(c echo.Context) error {
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
	list, err := h.store.ListAgentTurns(c.Request().Context(), pc.ID, threadID, limit)
	if err != nil {
		return WriteError(c, err)
	}
	out := make([]turnDTO, 0, len(list))
	for _, turn := range list {
		dto, err := h.buildTurnDTO(c.Request().Context(), pc.ID, turn)
		if err != nil {
			return WriteError(c, err)
		}
		out = append(out, dto)
	}
	return c.JSON(http.StatusOK, map[string]any{"turns": out})
}

// GetTurn 返回单个轮次快照（GET /agent-turns/:turnID）。
func (h *cloudAgentHandler) GetTurn(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	turn, err := h.store.GetAgentTurn(c.Request().Context(), pc.ID, c.Param("turnID"))
	if errors.Is(err, systemdb.ErrTurnNotFound) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "turn not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	dto, err := h.buildTurnDTO(c.Request().Context(), pc.ID, turn)
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, dto)
}

// TurnEvents 是轮次事件的 SSE：先按 after 回放事件表，再等待新事件或结束。
func (h *cloudAgentHandler) TurnEvents(c echo.Context) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	turn, err := h.store.GetAgentTurn(c.Request().Context(), pc.ID, c.Param("turnID"))
	if errors.Is(err, systemdb.ErrTurnNotFound) {
		return WriteError(c, echo.NewHTTPError(http.StatusNotFound, "turn not found"))
	} else if err != nil {
		return WriteError(c, err)
	}
	after, _ := strconv.ParseInt(c.QueryParam("after"), 10, 64)
	beginSSE(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	// 回放：事件表是唯一持久来源；超出上限时按 stream_gap 让客户端刷新快照。
	events, err := h.store.ListAgentTurnEvents(c.Request().Context(), pc.ID, turn.ID, after, 500)
	if err != nil {
		return WriteError(c, err)
	}
	for _, ev := range events {
		payload := map[string]any{"type": ev.Kind, "seq": ev.Seq, "task_id": ev.EntityID, "data": json.RawMessage(ev.PayloadJSON)}
		writeSSEFrame(c.Response().Writer, flusher, ev.Seq, payload)
	}
	// 终态直接关流；否则保持连接直到客户端断开（新事件由 SSE 首连写完后由前端轮询快照）。
	if terminalTurn(turn.Status) {
		end, _ := json.Marshal(cloudagent.Event{Type: "end"})
		_, _ = c.Response().Write([]byte("data: " + string(end) + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}
	<-c.Request().Context().Done()
	return nil
}

// terminalTurn 报告轮次是否已经结束（无需保持 SSE）。
func terminalTurn(status string) bool {
	switch status {
	case systemdb.TurnStatusCompleted, systemdb.TurnStatusFailed,
		systemdb.TurnStatusCanceled, systemdb.TurnStatusInterrupted:
		return true
	}
	return false
}
