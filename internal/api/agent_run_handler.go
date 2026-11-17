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
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
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
	// StarterStage 是搭建会话的阶段（home-starter-prompts-plan §2.7）：仅搭建会话接受，
	// 服务端按阶段白名单换算技能；客户端不能直接传技能 id。
	StarterStage string `json:"starter_stage"`
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
	// 重试请求 content 由被重试 run 的原消息提供（BUG-03）。
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
	if (agent.Module == cloudagent.ModuleKV || agent.Module == cloudagent.ModuleGoFunction || agent.Module == cloudagent.ModuleCron) && (h.runtime == nil || !h.runtime.SkillsCLI) {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "cli_skills_unavailable", "this agent requires CLI skills", RequestIDFromContext(c.Request().Context())))
	}
	if agent.Module == cloudagent.ModuleSandbox && !h.sandboxAvailable() {
		return WriteError(c, NewAPIError(http.StatusServiceUnavailable, "sandbox_unavailable", "sandbox is unavailable", RequestIDFromContext(c.Request().Context())))
	}
	// 技能由助手所属 module 决定（`@` 只点名助手，不能点名技能）。
	skills := cloudagent.SkillsForModule(agent.Module)
	// 搭建会话例外（home-starter-prompts-plan §2.4）：技能只由服务端登记的会话与阶段白名单决定。
	var readonlySkills []string
	starterStage := strings.TrimSpace(body.StarterStage)
	if starterStage != "" || thread.StarterTemplate != "" {
		stageSkills, stageReadonly, err := h.starterRunSkills(c.Request().Context(), thread, agent, starterStage)
		if err != nil {
			return WriteError(c, err)
		}
		skills, readonlySkills = stageSkills, stageReadonly
	}
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

	// 身份与历史消息在边界转换成模块 DTO：gocloudagent 是独立模块，
	// 只认识自己的无密钥类型（见 internal/agentbridge）。
	runAgent := agentbridge.Agent(agent)
	if thread.StarterTemplate != "" {
		// 搭建会话视为可写；委托令牌仍按调用者权限签发，破坏性命令照常确认，
		// 不改通用助手持久化的 writable 字段。
		runAgent.Writable = true
	}
	req := cloudagent.RunRequest{
		ProjectID: pc.ID, Principal: agentbridge.ToModulePrincipal(principal), Agent: runAgent,
		ThreadID: threadID, RunID: run.ID, UserText: body.Content, History: agentbridge.Messages(hist), Stream: body.Stream,
		Skills: skills, ReadOnlySkills: readonlySkills,
	}
	if thread.StarterTemplate != "" {
		req.StarterStage = starterStage
		if req.StarterStage == "" {
			req.StarterStage = cloudagent.StarterStagePlan
		}
	}

	// 二期 trace（planv6.5 §10.2）：run 级 ctx 注入 run_id 作为 trace_id；
	// completeRun / streamRun 内部继承此 ctx，llmgateway 落 llm span 时携带。
	runCtx := llmgateway.WithTraceMeta(c.Request().Context(), run.ID, "agent")

	if body.Stream {
		return h.streamRun(c, runCtx, pc.ID, run, agent, req)
	}
	return h.completeRun(c, runCtx, pc.ID, run, agent, req)
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
		// 不落 argv / stdin / LLM 正文。
		_ = h.audit.Record(ctx, AuditEvent{ProjectID: projectID, Kind: "agent.run", Status: status,
			Detail: fmt.Sprintf("run=%s thread=%s agent=%s writable=%t duration_ms=%d prompt_tokens=%d completion_tokens=%d tool_calls=%d error_code=%s",
				run.ID, run.ThreadID, agent.ID, agent.Writable, res.DurationMS, res.PromptTokens, res.CompletionTokens, res.ToolCalls, code)})
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
	return c.JSON(http.StatusOK, map[string]any{"run": toRunDTO(row), "message": asst, "agent_id": agent.ID})
}

func (h *cloudAgentHandler) streamRun(c echo.Context, runCtx context.Context, projectID string, run systemdb.AgentRun, agent systemdb.CloudAgent, req cloudagent.RunRequest) error {
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
	var mu sync.Mutex
	// RACE-01：事件 ID 分配、事件缓存写入与 SSE 输出必须在同一把锁内完成；
	// StartRun 回调与 thinking/heartbeat goroutine 会并发调用 write/writeBytes。
	var seq int64
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
		data, _ := json.Marshal(ev)
		_, _ = c.Response().Write([]byte(fmt.Sprintf("id: %d\ndata: %s\n\n", id, data)))
		if flusher != nil {
			flusher.Flush()
		}
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
		return nil
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
		return nil
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
	return nil
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

// starterRunSkills 换算搭建会话某阶段的技能（home-starter-prompts-plan §2.7）。
//
// 非搭建会话传了 starter_stage、阶段不在白名单、会话与助手不匹配，或运行前提
// 不再满足（实例变只读、CLI 技能关闭、调用方失去 database:admin）都返回
// ErrStarterStageInvalid / ErrStarterNotAllowed，且不泄露内部原因。
func (h *cloudAgentHandler) starterRunSkills(ctx context.Context, thread systemdb.AgentThread, agent systemdb.CloudAgent, stage string) (skills, readonly []string, err error) {
	if thread.StarterTemplate == "" {
		return nil, nil, ErrStarterStageInvalid
	}
	if stage == "" {
		stage = cloudagent.StarterStagePlan
	}
	stageSkills, stageReadonly, ok := cloudagent.StarterStage(stage)
	if !ok {
		return nil, nil, ErrStarterStageInvalid
	}
	principal, hasPrincipal := PrincipalFromContext(ctx)
	if !cloudagent.IsStarterTemplate(thread.StarterTemplate) {
		return nil, nil, ErrStarterNotAllowed
	}
	if err := h.checkStarterAllowed(thread.ProjectID, thread.StarterTemplate, agent, principal, hasPrincipal); err != nil {
		return nil, nil, err
	}
	return stageSkills, stageReadonly, nil
}
