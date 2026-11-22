// 文件说明：agent_run_handler.go — internal/api 的内部实现（CreateRun、streamRun、completeRun 等 run 生命周期）
//
// 用法
//
//	func (h *cloudAgentHandler) CreateRun / streamRun / completeRun / finishAgentRun / recordApprovalEvent / confirmTimeout
//	同包调用：CreateRun() → router.go；confirmTimeout() → run_events.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_run_handler.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, gocloudagent/mcp, internal/agentbridge, internal/systemdb 等
//	[同包] agent_handler.go, run_events.go, router.go 等 60 个
//	[关键引用] CreateRun ← router.go；confirmTimeout ← run_events.go
//
// 主要符号
//
//	type mentionDTO struct {…}
//	type createRunBody struct {…}
//	(h *cloudAgentHandler) CreateRun(c echo.Context) error
//	(h *cloudAgentHandler) finishAgentRun(...)
//	(h *cloudAgentHandler) completeRun(...)
//	(h *cloudAgentHandler) streamRun(...)
//	(h *cloudAgentHandler) recordApprovalEvent(...)
//	(h *cloudAgentHandler) confirmTimeout() time.Duration

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

type mentionDTO struct {
	AgentID string `json:"agent_id"`
}

type createRunBody struct {
	Content  string       `json:"content"`
	Mentions []mentionDTO `json:"mentions"`
	Stream   bool         `json:"stream"`
	// RetryOfRunID 非空表示重试该 run：复用其 user 消息，不重复落库（BUG-03）。
	RetryOfRunID string `json:"retry_of_run_id"`
}

func (h *cloudAgentHandler) CreateRun(c echo.Context) error {
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
	var body createRunBody
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	// planv6.7 AGC-ROUTE：auto 会话走共享路由，其余完全沿用旧逻辑。
	ws, err := h.store.GetAgentThreadWorkspace(c.Request().Context(), pc.ID, threadID)
	if err != nil {
		return WriteError(c, err)
	}
	// planv6.7 v2 §5.1：存储里的 auto 与 collaborative 都是群会话，统一走同一分支，
	// 但 /runs 只承担单目标兼容（旧 SDK / 旧前端）；多目标任务走 /turns。
	if isGroupDispatch(ws.DispatchMode) {
		// 活动轮次守卫：同一会话同一时刻只允许一个轮次（含 /runs 的单目标 run）。
		collab, cErr := h.store.GetAgentThreadCollab(c.Request().Context(), pc.ID, threadID)
		if cErr != nil {
			return WriteError(c, cErr)
		}
		if collab.ActiveTurnID != "" {
			return WriteError(c, ErrTurnBusy)
		}
		return h.createAutoRun(c, pc, principal, thread, ws, body)
	}
	return h.createDirectRun(c, pc, principal, thread, body)
}

// isGroupDispatch 报告该会话是否属于群（协作）模式。
// 旧数据里的 "auto" 在 v2 中等价于 collaborative；空串是固定助手会话。
func isGroupDispatch(mode string) bool {
	return mode == systemdb.DispatchAuto || mode == systemdb.DispatchCollaborative
}

// createDirectRun 处理固定助手会话的 CreateRun。
func (h *cloudAgentHandler) createDirectRun(c echo.Context, pc ProjectContext, principal auth.Principal, thread systemdb.AgentThread, body createRunBody) error {
	threadID := thread.ID
	if strings.TrimSpace(body.Content) == "" && body.RetryOfRunID == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "content is required"))
	}
	if err := h.ensureAgents(c.Request().Context(), pc.ID); err != nil {
		return WriteError(c, err)
	}
	if len(body.Mentions) > 1 {
		for _, mention := range body.Mentions[1:] {
			if mention.AgentID != body.Mentions[0].AgentID {
				return WriteError(c, NewAPIError(http.StatusBadRequest, "multiple_agents_not_supported", "mention only one agent per run", RequestIDFromContext(c.Request().Context())))
			}
		}
	}
	var agent systemdb.CloudAgent
	var err error
	if len(body.Mentions) == 0 && thread.AgentID != "" {
		agent, err = h.store.GetCloudAgent(c.Request().Context(), pc.ID, thread.AgentID)
	} else {
		agent, err = h.resolveMentionedAgent(c.Request().Context(), pc.ID, body.Mentions)
	}
	if err != nil {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, err.Error()))
	}
	if thread.AgentID != "" && thread.AgentID != agent.ID {
		return WriteError(c, NewAPIError(http.StatusConflict, "agent_switch_required", "switch to the mentioned agent before sending", RequestIDFromContext(c.Request().Context())))
	}
	if (agent.Module == agentbridge.ModuleKV || agent.Module == agentbridge.ModuleGoFunction || agent.Module == agentbridge.ModuleCron) && (!h.skillsCLI()) {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "cli_skills_unavailable", "this agent requires CLI skills", RequestIDFromContext(c.Request().Context())))
	}
	if agent.Module == agentbridge.ModuleSandbox && !h.sandboxAvailable() {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "sandbox_unavailable", "sandbox is unavailable", RequestIDFromContext(c.Request().Context())))
	}
	// 技能由助手所属 module 决定（`@` 只点名助手，不能点名技能）。
	// 计划模式的阶段技能由计划协调器直接构造 RunRequest，不走这条路径。
	skills := agentbridge.SkillsForModule(agent.Module)
	// 重试校验（BUG-03）：目标 run 必须属于本 thread、状态 failed/canceled、且是 thread 最新一次 run。
	retryUser, err := h.loadRetry(c.Request().Context(), pc.ID, threadID, body.RetryOfRunID)
	if err != nil {
		return WriteError(c, err)
	}
	if retryUser != nil {
		body.Content = retryUser.Content
	}
	if h.runtime == nil || h.runtime.LLM == nil {
		if !body.Stream {
			return WriteError(c, &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"})
		}
		// SSE 消费端仍收到规范的 error → end 帧；不持久化用户正文。
		return h.streamRun(c, c.Request().Context(), pc.ID, systemdb.AgentRun{ID: uuid.NewString()}, agent, cloudagent.RunRequest{})
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
		InitiatorID: principalIdentity(principal),
	})
	if err != nil {
		return WriteError(c, err)
	}
	// 重试复用原 user 消息，不再 Append（BUG-03）。
	var userMsg systemdb.AgentMessage
	if retryUser != nil {
		userMsg = *retryUser
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

	// 身份与历史消息在边界转换成模块 DTO：gocloudagent 是独立模块，
	// 只认识自己的无密钥类型（见 internal/agentbridge）。
	runAgent := agentbridge.Agent(agent)
	req := cloudagent.RunRequest{
		Agent: runAgent, ThreadID: threadID, RunID: run.ID, UserText: body.Content,
		History: agentbridge.Messages(hist), Stream: body.Stream, Subject: principalIdentity(principal),
		Context: agentbridge.RunContext{ProjectID: pc.ID, Principal: principal, Skills: skills},
	}
	// 二期 trace（planv6.5 §10.2）：run 级 ctx 注入 run_id 作为 trace_id；
	// completeRun / streamRun 内部继承此 ctx，llmgateway 落 llm span 时携带。
	runCtx := llmgateway.WithTraceMeta(c.Request().Context(), run.ID, "agent")

	if body.Stream {
		return h.streamRun(c, runCtx, pc.ID, run, agent, req)
	}
	return h.completeRun(c, runCtx, pc.ID, run, agent, req)
}

// loadRetry 校验重试请求（BUG-03）：目标 run 必须属于本 thread、状态
// failed/canceled、且是 thread 最新一次 run；返回原 user 消息。
// retryOfRunID 为空返回 (nil, nil)。
func (h *cloudAgentHandler) loadRetry(ctx context.Context, projectID, threadID, retryOfRunID string) (*systemdb.AgentMessage, error) {
	if retryOfRunID == "" {
		return nil, nil
	}
	prev, err := h.store.GetAgentRun(ctx, projectID, retryOfRunID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid_retry")
	}
	if err != nil {
		return nil, err
	}
	if prev.ThreadID != threadID || (prev.Status != systemdb.AgentRunFailed && prev.Status != systemdb.AgentRunCanceled) {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid_retry")
	}
	latest, err := h.store.ListAgentRuns(ctx, projectID, threadID, 1)
	if err != nil {
		return nil, err
	}
	if len(latest) == 0 || latest[0].ID != prev.ID {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid_retry")
	}
	msgs, err := h.store.ListAgentMessages(ctx, projectID, threadID, 200)
	if err != nil {
		return nil, err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].RunID == prev.ID && msgs[i].Role == "user" {
			return &msgs[i], nil
		}
	}
	return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid_retry")
}

// createAutoRun 处理 dispatch_mode=auto 的共享会话（planv6.7 AGC-ROUTE §3）。
// 步骤：候选生成与校验（ClaimThread 之前）→ 配额/锁 → run(agent_id 可空) →
// 落用户消息 → SSE run/route 事件 → 路由器 → 回填 → 投影历史 → 执行。
func (h *cloudAgentHandler) createAutoRun(c echo.Context, pc ProjectContext, principal auth.Principal, thread systemdb.AgentThread, ws systemdb.AgentThreadWorkspace, body createRunBody) error {
	ctx := c.Request().Context()
	threadID := thread.ID
	rid := RequestIDFromContext(ctx)
	if strings.TrimSpace(body.Content) == "" && body.RetryOfRunID == "" {
		return WriteError(c, echo.NewHTTPError(http.StatusBadRequest, "content is required"))
	}
	if err := h.ensureAgents(ctx, pc.ID); err != nil {
		return WriteError(c, err)
	}
	if len(body.Mentions) > 1 {
		for _, mention := range body.Mentions[1:] {
			if mention.AgentID != body.Mentions[0].AgentID {
				// v2：多目标任务走 /turns；/runs 只保留单目标兼容口径。
				return WriteError(c, NewAPIError(http.StatusBadRequest, "multiple_agents_not_supported",
					"mention only one agent per run; use /turns for multi-agent collaboration", rid))
			}
		}
	}
	// 重试校验与原消息。
	retryUser, err := h.loadRetry(ctx, pc.ID, threadID, body.RetryOfRunID)
	if err != nil {
		return WriteError(c, err)
	}
	if retryUser != nil {
		body.Content = retryUser.Content
	}
	// 候选生成（ClaimThread 之前完成所有不需要模型的判定）。
	cands, err := h.workspaceCandidates(ctx, pc.ID, ws)
	if err != nil {
		return WriteError(c, err)
	}
	decision, err := h.decideRoute(ctx, pc.ID, ws, body, cands, retryUser)
	if err != nil {
		return WriteError(c, err)
	}
	if h.runtime == nil || h.runtime.LLM == nil {
		if !body.Stream {
			return WriteError(c, &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"})
		}
		return h.streamRun(c, ctx, pc.ID, systemdb.AgentRun{ID: uuid.NewString()}, decision.Agent, cloudagent.RunRequest{})
	}
	if h.usage != nil {
		if err := h.usage.CheckQuota(ctx, pc.ID, "llm"); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) {
				return WriteError(c, NewAPIError(http.StatusTooManyRequests, "quota_exceeded", "项目模型调用配额已用完", rid))
			}
			return WriteError(c, err)
		}
	}
	claimID := uuid.NewString()
	if err := h.runtime.ClaimThread(threadID, claimID); err != nil {
		return WriteError(c, err)
	}
	defer h.runtime.ReleaseThread(threadID, claimID)

	now := time.Now().UTC()
	agentID := decision.Agent.ID
	if decision.NeedLLM {
		agentID = "" // 路由完成后回填
	}
	run, err := h.store.CreateAgentRun(ctx, systemdb.AgentRun{
		ID: claimID, ThreadID: threadID, ProjectID: pc.ID, AgentID: agentID,
		Status: systemdb.AgentRunRunning, StartedAt: now, RetryOfRunID: body.RetryOfRunID,
		InitiatorID: principalIdentity(principal),
	})
	if err != nil {
		return WriteError(c, err)
	}
	// 用户消息：agent_id 空串、mentions_json 写真实 mentions（自动路由为 []）。
	var userMsg systemdb.AgentMessage
	if retryUser != nil {
		userMsg = *retryUser
	} else {
		mentionsJSON, _ := json.Marshal(body.Mentions)
		userMsg, err = h.store.AppendAgentMessage(ctx, systemdb.AgentMessage{
			ThreadID: threadID, ProjectID: pc.ID, Role: "user", Content: body.Content,
			AgentID: "", MentionsJSON: string(mentionsJSON), RunID: run.ID,
		})
		if err != nil {
			return WriteError(c, err)
		}
	}
	// 无需路由：直接落路由记录（mention/retry/单候选直选）。
	if !decision.NeedLLM {
		if err := h.store.InsertAgentRunRoute(ctx, systemdb.AgentRunRoute{
			ProjectID: pc.ID, RunID: run.ID, ThreadID: threadID,
			AgentID: decision.Agent.ID, RouteKind: decision.Kind, RouterMS: decision.RouterMS,
		}); err != nil {
			return WriteError(c, err)
		}
	}

	runCtx := llmgateway.WithTraceMeta(ctx, run.ID, "agent")
	if body.Stream {
		return h.streamAutoRun(c, runCtx, pc.ID, run, threadID, decision, cands, userMsg, body, principal)
	}
	return h.completeAutoRun(c, runCtx, pc.ID, run, threadID, decision, cands, userMsg, body, principal)
}

// decideRoute 在 ClaimThread 之前完成所有「不需要模型」的判定与校验
// （AGC-ROUTE §3.2）：重试优先，其次显式点名，再自动分派（单候选直选）。
func (h *cloudAgentHandler) decideRoute(ctx context.Context, projectID string, ws systemdb.AgentThreadWorkspace, body createRunBody, cands []systemdb.CloudAgent, retryUser *systemdb.AgentMessage) (routeDecision, error) {
	if len(cands) == 0 {
		return routeDecision{}, ErrNoAvailableAgent
	}
	// 重试：固定原 run 的 agent_id。
	if body.RetryOfRunID != "" {
		prev, err := h.store.GetAgentRun(ctx, projectID, body.RetryOfRunID)
		if err != nil {
			return routeDecision{}, err
		}
		if prev.AgentID == "" {
			return routeDecision{}, ErrRetryAgentUnavailable
		}
		for _, a := range cands {
			if a.ID == prev.AgentID {
				return routeDecision{Agent: a, Kind: systemdb.RouteRetry}, nil
			}
		}
		return routeDecision{}, ErrRetryAgentUnavailable
	}
	// 显式点名：验证项目与候选范围。
	if len(body.Mentions) > 0 && body.Mentions[0].AgentID != "" {
		mentionID := strings.TrimSpace(body.Mentions[0].AgentID)
		agent, err := h.store.GetCloudAgent(ctx, projectID, mentionID)
		if errors.Is(err, sql.ErrNoRows) {
			return routeDecision{}, echo.NewHTTPError(http.StatusBadRequest, "mentioned agent not found")
		}
		if err != nil {
			return routeDecision{}, err
		}
		for _, a := range cands {
			if a.ID == agent.ID {
				return routeDecision{Agent: a, Kind: systemdb.RouteMention}, nil
			}
		}
		return routeDecision{}, ErrAgentNotInTag
	}
	// 候选只有一个：直接使用，不调模型。
	if len(cands) == 1 {
		return routeDecision{Agent: cands[0], Kind: systemdb.RouteAuto}, nil
	}
	// 需要路由器；LLM 未配置时直接 fallback。
	if h.dispatcher == nil {
		return routeDecision{Agent: fallbackAgent(cands), Kind: systemdb.RouteFallback}, nil
	}
	return routeDecision{Kind: systemdb.RouteAuto, NeedLLM: true}, nil
}

// dispatchRecent 从历史消息提取最近 3 轮（user/assistant 交替）供路由器。
func dispatchRecent(msgs []systemdb.AgentMessage, names map[string]string, excludeRunID string) []dispatchTurn {
	var turns []dispatchTurn
	for i := len(msgs) - 1; i >= 0 && len(turns) < 6; i-- {
		m := msgs[i]
		if m.RunID == excludeRunID {
			continue
		}
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		agentName := names[m.AgentID]
		turns = append(turns, dispatchTurn{Role: m.Role, AgentName: agentName, Content: m.Content})
	}
	// 反转成时间正序，再截最近 3 轮（user+assistant 对）。
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	if len(turns) > 6 {
		turns = turns[len(turns)-6:]
	}
	return turns
}

// runAutoRoute 执行路由器调用并回填 run/路由记录；返回最终决策。
// canceled 返回 true 表示路由期被取消（run 已置 canceled，调用方写 end 帧返回）。
func (h *cloudAgentHandler) runAutoRoute(ctx context.Context, projectID string, run systemdb.AgentRun, decision routeDecision, cands []systemdb.CloudAgent, userMsg systemdb.AgentMessage, body createRunBody) (routeDecision, bool, error) {
	routeCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	h.routeCancels.Store(run.ID, cancel)
	defer func() {
		if _, loaded := h.routeCancels.LoadAndDelete(run.ID); loaded {
			cancel()
		}
	}()
	started := time.Now()
	// 最近 3 轮上下文（排除本轮 run 的消息）与候选名映射。
	hist, _ := h.store.ListAgentMessages(ctx, projectID, run.ThreadID, 40)
	names := agentNamesOf(cands)
	var recent []dispatchTurn
	if hist != nil {
		recent = dispatchRecent(hist, names, run.ID)
	}
	picked, routeErr := h.dispatcher.Route(routeCtx, projectID, dispatchInput{
		UserText: body.Content, Recent: recent, Candidates: dispatchCandidatesOf(cands),
	})
	routerMS := time.Since(started).Milliseconds()
	final := decision
	final.RouterMS = routerMS
	if routeErr != nil {
		if errors.Is(routeErr, context.Canceled) {
			_ = h.store.UpdateAgentRunStatus(ctx, projectID, run.ID, systemdb.AgentRunCanceled, "canceled", nil, nil)
			return final, true, nil
		}
		// 超时/上游错误/none 一律 fallback。
		final.Agent = fallbackAgent(cands)
		final.Kind = systemdb.RouteFallback
	} else if picked != "" {
		for _, a := range cands {
			if a.ID == picked {
				final.Agent = a
				final.Kind = systemdb.RouteAuto
				break
			}
		}
		if final.Agent.ID == "" {
			final.Agent = fallbackAgent(cands)
			final.Kind = systemdb.RouteFallback
		}
	} else {
		final.Agent = fallbackAgent(cands)
		final.Kind = systemdb.RouteFallback
	}
	if err := h.store.UpdateAgentRunAgent(ctx, projectID, run.ID, final.Agent.ID); err != nil {
		return final, false, err
	}
	if err := h.store.InsertAgentRunRoute(ctx, systemdb.AgentRunRoute{
		ProjectID: projectID, RunID: run.ID, ThreadID: run.ThreadID,
		AgentID: final.Agent.ID, RouteKind: final.Kind, RouterMS: routerMS,
	}); err != nil {
		return final, false, err
	}
	return final, false, nil
}

func dispatchCandidatesOf(cands []systemdb.CloudAgent) []dispatchCandidate {
	out := make([]dispatchCandidate, 0, len(cands))
	for _, a := range cands {
		out = append(out, dispatchCandidate{ID: a.ID, Name: a.Name, Module: a.Module, Description: a.Description})
	}
	return out
}

// agentNamesOf 返回候选的 agentID → 显示名映射。
func agentNamesOf(cands []systemdb.CloudAgent) map[string]string {
	out := make(map[string]string, len(cands))
	for _, a := range cands {
		out[a.ID] = a.Name
	}
	return out
}

// buildAutoRunRequest 构造最终 RunRequest（历史投影 + 能力检查已在候选过滤完成）。
func (h *cloudAgentHandler) buildAutoRunRequest(ctx context.Context, pc ProjectContext, principal auth.Principal, run systemdb.AgentRun, decision routeDecision, userMsg systemdb.AgentMessage, body createRunBody, hist []systemdb.AgentMessage) cloudagent.RunRequest {
	agent := decision.Agent
	names := map[string]string{}
	if agents, err := h.store.ListCloudAgents(ctx, pc.ID); err == nil {
		for _, a := range agents {
			names[a.ID] = a.Name
		}
	}
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
	return cloudagent.RunRequest{
		Agent: agentbridge.Agent(agent), ThreadID: run.ThreadID, RunID: run.ID, UserText: body.Content,
		History: agentbridge.MessagesFor(filtered, agent.ID, names), Stream: body.Stream, Subject: principalIdentity(principal),
		Context: agentbridge.RunContext{ProjectID: pc.ID, Principal: principal, Skills: agentbridge.SkillsForModule(agent.Module)},
	}
}

func (h *cloudAgentHandler) finishAgentRun(ctx context.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, res cloudagent.RunResult, runErr error) error {
	status, code := systemdb.AgentRunCompleted, ""
	if runErr != nil {
		status = systemdb.AgentRunFailed
		if errors.Is(runErr, context.Canceled) {
			status = systemdb.AgentRunCanceled
		} else {
			code = agentbridge.ClassifyError(runErr).Code
		}
	}
	// 二期 trace（§10.3）：run 收尾时把工具卡片补写为 tool span（trace_id=run.ID）。
	// 解析失败静默丢弃：工具卡片格式由 gocloudagent 保证，坏数据不应影响 run 收尾。
	if h.store != nil {
		_ = h.store.RecordAgentToolSpans(projectID, run.ID, res.ToolCallsJSON)
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
		// planv6.0 v2 §3.3：写能力的审计只记元数据（是否可写 + 工具调用数），
		// 不落 argv / stdin / LLM 正文。planv6.7 AGC-ROUTE：追加 route 类别。
		routeKind := ""
		if routes, rErr := h.store.ListAgentRunRoutes(ctx, projectID, run.ThreadID); rErr == nil {
			if r, ok := routes[run.ID]; ok {
				routeKind = r.RouteKind
			}
		}
		detail := fmt.Sprintf("run=%s thread=%s agent=%s duration_ms=%d prompt_tokens=%d completion_tokens=%d tool_calls=%d error_code=%s",
			run.ID, run.ThreadID, agent.ID, res.DurationMS, res.PromptTokens, res.CompletionTokens, res.ToolCalls, code)
		if routeKind != "" {
			detail += " route=" + routeKind
		}
		_ = h.audit.Record(ctx, AuditEvent{ProjectID: projectID, Kind: "agent.run", Status: status, Detail: detail})
	}
	return nil
}

func (h *cloudAgentHandler) completeRun(c echo.Context, runCtx context.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, req cloudagent.RunRequest) error {
	if h.runtime == nil {
		_ = h.failRun(c.Request().Context(), projectID, run.ID, "llm_not_configured")
		return WriteError(c, &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"})
	}
	res, runErr := h.runtime.StartRun(runCtx, req, nil)
	if err := h.finishAgentRun(context.Background(), projectID, run, agent, res, runErr); err != nil {
		return WriteError(c, err)
	}
	if runErr != nil {
		return WriteError(c, agentbridge.ClassifyError(runErr))
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
	resp := map[string]any{"run": toRunDTO(row), "message": asst, "agent_id": agent.ID}
	// planv6.7 AGC-ROUTE：非流式响应附带 route_kind（auto 会话）。
	if routes, err := h.store.ListAgentRunRoutes(c.Request().Context(), projectID, run.ThreadID); err == nil {
		if r, ok := routes[run.ID]; ok {
			resp["route_kind"] = r.RouteKind
		}
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *cloudAgentHandler) streamRun(c echo.Context, runCtx context.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, req cloudagent.RunRequest) error {
	w := h.openStream(c, run)
	h.runStream(w, c, runCtx, projectID, run, agent, req)
	return nil
}

// openStream 建立 SSE 连接并返回写入器（planv6.7 AGC-ROUTE：route 事件需要
// 在 run 帧之后、StartRun 之前写入，因此把流建立与运行拆开）。
type sseWriter struct {
	c       echo.Context
	flusher http.Flusher
	mu      sync.Mutex
	seq     int64
}

func writeSSEFrame(dst io.Writer, flusher http.Flusher, id int64, payload any) {
	data, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(dst, "id: %d\ndata: %s\n\n", id, data)
	if flusher != nil {
		flusher.Flush()
	}
}

func (h *cloudAgentHandler) openStream(c echo.Context, run systemdb.AgentRun) *sseWriter {
	// planv6.0 v2 §11：先解除 WriteTimeout，否则 run 会在 write_timeout
	// （默认 30s）到点时被服务端切断，context 取消 → reason=canceled、无输出。
	beginSSE(c)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	// BUG-B5（planv6.0 v2 §2.2）：runID 此前只在首个 SSE 帧里下发，连接在
	// run 帧到达前断开时客户端没有任何 runID，无法续传。响应头先给一份。
	c.Response().Header().Set("X-Run-Id", run.ID)
	c.Response().WriteHeader(http.StatusOK)
	flusher, _ := c.Response().Writer.(http.Flusher)
	return &sseWriter{c: c, flusher: flusher}
}

func (w *sseWriter) write(ev cloudagent.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	writeSSEFrame(w.c.Response().Writer, w.flusher, w.seq, ev)
}

// runStream 在已建立的 SSE 连接上执行 run（原 streamRun 主体）。
func (h *cloudAgentHandler) runStream(w *sseWriter, c echo.Context, runCtx context.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, req cloudagent.RunRequest) {
	// RACE-01：事件 ID 分配、事件缓存写入与 SSE 输出必须在同一把锁内完成；
	// StartRun 回调与 thinking/heartbeat goroutine 会并发调用 write/writeBytes。
	var mu sync.Mutex
	var seq int64
	flusher := w.flusher
	write := func(ev cloudagent.Event) {
		if ev.RunID == "" {
			ev.RunID = run.ID
		}
		mu.Lock()
		defer mu.Unlock()
		var id int64
		if h.events != nil {
			id = h.events.append(run.ID, ev)
		} else {
			seq++
			id = seq
		}
		writeSSEFrame(c.Response().Writer, flusher, id, ev)
	}
	writeBytes := func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = c.Response().Write(data)
		if flusher != nil {
			flusher.Flush()
		}
	}
	write(cloudagent.Event{Type: "run", RunID: run.ID})
	if h.runtime == nil || h.runtime.LLM == nil {
		write(cloudagent.Event{Type: "error", Code: "llm_not_configured", Message: "模型服务未配置"})
		write(cloudagent.Event{Type: "end"})
		return
	}
	// RACE-01：后台 goroutine 必须在 streamRun 返回前退出（stopWorkers 内 wg.Wait），
	// 避免 handler 返回后仍有 goroutine 写 Response；sync.Once 防 defer 双重 close。
	var wg sync.WaitGroup
	var stopOnce sync.Once
	stopHeartbeat := make(chan struct{})
	stopThinking := make(chan struct{})
	stopWorkers := func() {
		stopOnce.Do(func() {
			close(stopHeartbeat)
			close(stopThinking)
			wg.Wait()
		})
	}
	defer stopWorkers() // panic / 提前返回路径兜底
	wg.Add(2)
	go func() {
		defer wg.Done()
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
	go func() {
		defer wg.Done()
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
	// 运行与 HTTP 连接解耦：前端切换界面 / 刷新只会断开 SSE，run 继续执行，
	// 事件照常写入 h.events 供 ReplayEvents 续接；真正取消只走 CancelRun
	// （runtime 内部登记的 cancel）或 RunTimeout。保留 context 中的身份与 trace 元数据。
	runBase := llmgateway.WithTraceMeta(context.WithoutCancel(runCtx), run.ID, "agent")
	res, runErr := h.runtime.StartRun(runBase, req, func(ev cloudagent.Event) {
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
		// 审批落库：内存 confirmHub 只解决「本次连接内等待」，
		// 列表页与进程重启后的收敛需要持久记录。写失败不影响 run。
		h.recordApprovalEvent(runBase, projectID, run, agent, ev)
		write(ev)
	})
	// 正常路径：StartRun 返回后先停后台 goroutine，再写收尾事件。
	stopWorkers()
	if err := h.finishAgentRun(context.Background(), projectID, run, agent, res, runErr); err != nil {
		write(cloudagent.Event{Type: "error", Code: "llm_upstream_error", Message: "保存运行结果失败"})
		write(cloudagent.Event{Type: "end"})
		return
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		re := agentbridge.ClassifyError(runErr)
		write(cloudagent.Event{Type: "error", Code: re.Code, Message: re.Message})
	}
	write(cloudagent.Event{Type: "usage", DurationMS: res.DurationMS, PromptTokens: res.PromptTokens,
		CompletionTokens: res.CompletionTokens, ReasoningTokens: res.ReasoningTokens, ToolCalls: res.ToolCalls})
	if errors.Is(runErr, context.Canceled) {
		res.Reason = "canceled"
	}
	write(cloudagent.Event{Type: "end", Reason: res.Reason})
}

// recordApprovalEvent 把 confirmation_required / confirmation_resolved 事件
// 同步到 sys_agent_approvals，形成可跨请求查询的审批队列。
//
// 只做尽力而为的持久化：写库失败不阻断 run（内存 confirmHub 仍是执行路径的
// 唯一真相），但会留下「列表页看不到这条待办」的降级，属于可接受损失。
func (h *cloudAgentHandler) recordApprovalEvent(ctx context.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, ev cloudagent.Event) {
	if h.store == nil || ev.CallID == "" {
		return
	}
	switch ev.Type {
	case "confirmation_required":
		argv, _ := json.Marshal(ev.Argv)
		timeout := h.confirmTimeout()
		var expires time.Time
		if timeout > 0 {
			expires = time.Now().UTC().Add(timeout)
		}
		_, _ = h.store.CreateAgentApproval(ctx, systemdb.AgentApproval{
			ProjectID:   projectID,
			RunID:       run.ID,
			CallID:      ev.CallID,
			ThreadID:    run.ThreadID,
			AgentID:     agent.ID,
			ToolName:    ev.Name,
			ArgvJSON:    string(argv),
			Message:     ev.Message,
			InitiatorID: run.InitiatorID,
			ExpiresAt:   expires,
		})
	case "confirmation_resolved":
		status := systemdb.AgentApprovalDenied
		if ev.Approve != nil && *ev.Approve {
			status = systemdb.AgentApprovalApproved
		}
		pending, err := h.store.GetPendingAgentApproval(ctx, projectID, run.ID, ev.CallID)
		if err != nil {
			return
		}
		_ = h.store.ResolveAgentApproval(ctx, projectID, pending.ID, status, "")
	}
}

// streamAutoRun 是 auto 会话的流式执行（planv6.7 AGC-ROUTE §7.7）：
// run 事件先不带 agent_id（NeedLLM 时）→ route{routing} → 路由 → route{done} →
// 复用 runStream 执行。路由期取消直接写 end{canceled}。
func (h *cloudAgentHandler) streamAutoRun(c echo.Context, runCtx context.Context, projectID string, run systemdb.AgentRun, threadID string, decision routeDecision, cands []systemdb.CloudAgent, userMsg systemdb.AgentMessage, body createRunBody, principal auth.Principal) error {
	w := h.openStream(c, run)
	writeRoute := func(ev cloudagent.Event) {
		ev.RunID = run.ID
		w.write(ev)
	}
	if !decision.NeedLLM {
		// 已确定助手：run 帧直接带身份（AGC-ROUTE §3.3）。
		writeRoute(cloudagent.Event{Type: "run", RunID: run.ID, AgentID: decision.Agent.ID, RouteKind: decision.Kind})
	} else {
		writeRoute(cloudagent.Event{Type: "run", RunID: run.ID})
		writeRoute(cloudagent.Event{Type: "route", Status: "routing"})
		final, canceled, err := h.runAutoRoute(runCtx, projectID, run, decision, cands, userMsg, body)
		if err != nil {
			writeRoute(cloudagent.Event{Type: "error", Code: "llm_upstream_error", Message: "路由失败"})
			writeRoute(cloudagent.Event{Type: "end", Reason: "error"})
			return nil
		}
		if canceled {
			writeRoute(cloudagent.Event{Type: "end", Reason: "canceled"})
			return nil
		}
		decision = final
		writeRoute(cloudagent.Event{Type: "route", Status: "done", AgentID: final.Agent.ID, RouteKind: final.Kind})
	}
	hist, err := h.store.ListAgentMessages(c.Request().Context(), projectID, threadID, 40)
	if err != nil {
		writeRoute(cloudagent.Event{Type: "error", Code: "llm_upstream_error", Message: "加载历史失败"})
		writeRoute(cloudagent.Event{Type: "end", Reason: "error"})
		return nil
	}
	req := h.buildAutoRunRequest(c.Request().Context(), pcOfContext(c), principal, run, decision, userMsg, body, hist)
	h.runStream(w, c, runCtx, projectID, run, decision.Agent, req)
	return nil
}

// pcOfContext 从请求上下文提取项目（streamAutoRun 内部使用）。
func pcOfContext(c echo.Context) ProjectContext {
	pc, _ := ProjectFromContext(c.Request().Context())
	return pc
}

// completeAutoRun 是 auto 会话的非流式执行：路由同步完成后走 completeRun
// （route_kind 由 completeRun 响应统一附带）。
func (h *cloudAgentHandler) completeAutoRun(c echo.Context, runCtx context.Context, projectID string, run systemdb.AgentRun, threadID string, decision routeDecision, cands []systemdb.CloudAgent, userMsg systemdb.AgentMessage, body createRunBody, principal auth.Principal) error {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, ErrProjectMissing)
	}
	if decision.NeedLLM {
		final, canceled, err := h.runAutoRoute(runCtx, projectID, run, decision, cands, userMsg, body)
		if err != nil {
			return WriteError(c, err)
		}
		if canceled {
			return WriteError(c, NewAPIError(http.StatusConflict, "run_canceled", "run was canceled while routing", RequestIDFromContext(c.Request().Context())))
		}
		decision = final
	}
	hist, err := h.store.ListAgentMessages(c.Request().Context(), projectID, threadID, 40)
	if err != nil {
		return WriteError(c, err)
	}
	req := h.buildAutoRunRequest(c.Request().Context(), pc, principal, run, decision, userMsg, body, hist)
	return h.completeRun(c, runCtx, projectID, run, decision.Agent, req)
}
