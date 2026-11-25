// 文件说明：run.go — 固定助手会话的 run 用例（创建 / 执行 / 取消 / 审批放行 / 统一收尾）
//
// 用法
//
//	d, err := svc.StartDirectRun(ctx, DirectRunInput{…})        // 校验 + 落 run 行与用户消息 + 组装 RunRequest
//	out := svc.ExecuteRun(extraCtx, d, emit)                    // 执行 run 并收尾；extraCtx 为 nil 表示不随请求取消（流式）
//	run, err := svc.CancelRun(ctx, projectID, runID)            // 取消在途 run 并作废其待审批
//	svc.ResolveConfirm(ctx, projectID, runID, callID, approve, actor)  // 审批放行 + 持久审批收敛
//	svc.LookupThread(ctx, projectID, threadID)                  // 入口的前置读取（404 → ErrThreadNotFound）
//
// 调用架构
//
//	[上游] internal/api（agent_run_handler / agent_approval_handler）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/run.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/cloudagent, internal/agentbridge, internal/llmgateway, internal/systemdb, internal/usage
//	[同包] service.go, plan_tools.go, confirm.go, run_events.go
//
// 主要符号
//
//	type Mention struct {…}
//	type DirectRunInput struct {…}
//	type DirectRun struct {…}
//	type RunOutcome struct {…}
//	(s *Service) LookupThread(ctx context.Context, projectID, threadID string) (systemdb.AgentThread, error)
//	(s *Service) StartDirectRun(ctx context.Context, in DirectRunInput) (DirectRun, error)
//	(s *Service) ExecuteRun(extra context.Context, d DirectRun, emit func(cloudagent.Event)) RunOutcome
//	(s *Service) CancelRun(ctx context.Context, projectID, runID string) (systemdb.AgentRun, error)
//	(s *Service) ResolveConfirm(ctx context.Context, projectID, runID, callID string, approve bool, actor string) error
//	(s *Service) Finish(ctx, in agentbridge.RunFinish) error   // 实现 agentbridge.RunFinalizer（cron run 的统一收尾）
//	(s *Service) finishRun(ctx, run, agent, res, runErr) error
//	... 另有 7 个顶层符号
//
// 取消语义（internal-ddd-hexagonal-implementation-plan §3.3、D6-b）：执行 ctx = 服务根 ctx ∪ extra 的取消——
// 非流式传请求 ctx（客户端断开即取消，与迁移前一致），流式传 nil（只受服务停机 / CancelRun / RunTimeout 影响）。
// 收尾写库用独立于取消的 ctx（对齐迁移前 finishAgentRun(context.Background())）。

package agentrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/usage"
)

// Mention 是固定助手会话的 @ 点名（与入口请求体同形：{"agent_id"}）。
type Mention struct {
	AgentID string `json:"agent_id"`
}

// DirectRunInput 是创建一次固定助手会话 run 的输入；Thread 由 LookupThread 取得。
type DirectRunInput struct {
	ProjectID    string
	Thread       systemdb.AgentThread
	Principal    auth.Principal
	Content      string
	Mentions     []Mention
	RetryOfRunID string
	Stream       bool
}

// DirectRun 是已落库、待执行的 run。
type DirectRun struct {
	Run     systemdb.AgentRun
	Agent   systemdb.CloudAgent
	Request cloudagent.RunRequest
	// LLMNotConfigured 为 true 表示模型服务未配置：run 未创建，入口按流式 / 非流式各自给出 503。
	LLMNotConfigured bool
}

// RunOutcome 是 ExecuteRun 的结果：运行结果、收尾后的 run 行与本 run 的最后一条 assistant 消息。
type RunOutcome struct {
	Result    cloudagent.RunResult
	RunErr    error
	Run       systemdb.AgentRun
	Assistant *systemdb.AgentMessage
	// PersistErr 非空表示收尾写库失败：run 结果不可信，入口按 500 处理。
	PersistErr error
	// ReadErr 非空表示收尾后回表失败（只有非流式响应需要回表）。
	ReadErr error
}

// LookupThread 读取会话；不存在返回 ErrThreadNotFound。
func (s *Service) LookupThread(ctx context.Context, projectID, threadID string) (systemdb.AgentThread, error) {
	thread, err := s.ports.Threads.GetAgentThread(ctx, projectID, threadID)
	if errors.Is(err, sql.ErrNoRows) {
		return systemdb.AgentThread{}, ErrThreadNotFound
	}
	return thread, err
}

// StartDirectRun 处理固定助手会话的 run 创建：校验内容 / @ / 重试，落 run 行与用户消息，组装 RunRequest。
// 群会话必须经 Plan 栅栏：工作区派发模式为 collaborative 时返回 ErrTurnsRequired。
func (s *Service) StartDirectRun(ctx context.Context, in DirectRunInput) (DirectRun, error) {
	if s.stopping.Load() {
		return DirectRun{}, ErrStopping
	}
	threadID := in.Thread.ID
	if strings.TrimSpace(in.Content) == "" && in.RetryOfRunID == "" {
		return DirectRun{}, ErrRunContentRequired
	}
	if err := s.ports.Agents.SeedDefaultCloudAgentsWithCapabilities(ctx, in.ProjectID, s.cfg.Sandbox, s.cfg.SkillsCLI); err != nil {
		return DirectRun{}, err
	}
	if len(in.Mentions) > 1 {
		for _, mention := range in.Mentions[1:] {
			if mention.AgentID != in.Mentions[0].AgentID {
				return DirectRun{}, ErrMultipleAgents
			}
		}
	}
	agent, err := s.resolveDirectAgent(ctx, in.ProjectID, in.Thread, in.Mentions)
	if err != nil {
		return DirectRun{}, err
	}
	// 技能由助手所属 module 决定（`@` 只点名助手，不能点名技能）。规划工具组（plan / explore / reflect）
	// 在构造 RunRequest 时按会话历史装配，见 plan_tools.go。
	skills := agentbridge.SkillsForModule(agent.Module)
	// 重试校验（BUG-03）：目标 run 必须属于本 thread、状态 failed/canceled、且是 thread 最新一次 run。
	retryUser, err := s.loadRetry(ctx, in.ProjectID, threadID, in.RetryOfRunID)
	if err != nil {
		return DirectRun{}, err
	}
	if retryUser != nil {
		in.Content = retryUser.Content
	}
	if !s.llmConfigured() {
		return DirectRun{Agent: agent, LLMNotConfigured: true}, nil
	}
	if s.ports.Quota != nil {
		if err := s.ports.Quota.CheckQuota(ctx, in.ProjectID, "llm"); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) {
				return DirectRun{}, ErrQuotaExceeded
			}
			return DirectRun{}, err
		}
	}
	claimID := uuid.NewString()
	if s.runtime != nil {
		if err := s.runtime.ClaimThread(threadID, claimID); err != nil {
			return DirectRun{}, err
		}
		defer s.runtime.ReleaseThread(threadID, claimID)
	}
	mentionsJSON, _ := json.Marshal(in.Mentions)
	if len(in.Mentions) == 0 {
		mentionsJSON, _ = json.Marshal([]Mention{{AgentID: agent.ID}})
	}

	now := time.Now().UTC()
	run, err := s.ports.Runs.CreateAgentRun(ctx, systemdb.AgentRun{
		ID: claimID, ThreadID: threadID, ProjectID: in.ProjectID, AgentID: agent.ID,
		Status: systemdb.AgentRunRunning, StartedAt: now, RetryOfRunID: in.RetryOfRunID,
		InitiatorID: in.Principal.Actor(),
	})
	if err != nil {
		return DirectRun{}, err
	}
	// 重试复用原 user 消息，不再 Append（BUG-03）。
	var userMsg systemdb.AgentMessage
	if retryUser != nil {
		userMsg = *retryUser
	} else {
		userMsg, err = s.ports.Runs.AppendAgentMessage(ctx, systemdb.AgentMessage{
			ThreadID: threadID, ProjectID: in.ProjectID, Role: "user", Content: in.Content,
			AgentID: agent.ID, MentionsJSON: string(mentionsJSON), RunID: run.ID,
		})
		if err != nil {
			return DirectRun{}, err
		}
	}
	hist, err := s.historyWithoutUserTurn(ctx, in.ProjectID, threadID, userMsg, in.RetryOfRunID)
	if err != nil {
		return DirectRun{}, err
	}

	// 身份与历史消息在边界转换成模块 DTO：gocloudagent 是独立模块，
	// 只认识自己的无密钥类型（见 internal/agentbridge）。
	req := cloudagent.RunRequest{
		Agent: agentbridge.Agent(agent), ThreadID: threadID, RunID: run.ID, UserText: in.Content,
		History: agentbridge.Messages(hist), Stream: in.Stream, Subject: in.Principal.Actor(),
		Context: agentbridge.RunContext{
			ProjectID: in.ProjectID, Principal: in.Principal, Skills: skills,
			ExtraTools: s.runPlanningTools(ctx, in.ProjectID, run.ID, agent, hist),
		},
	}
	return DirectRun{Run: run, Agent: agent, Request: req}, nil
}

// resolveDirectAgent 解析固定助手会话这次 run 的目标助手，并校验：
//   - 会话已绑定助手时必须沿用（@ 另一个助手 → ErrAgentSwitchRequired）；
//   - 会话未绑定助手时取 @ 提到的那个；
//   - 助手依赖的能力（CLI 技能 / 云沙盒）在当前实例可用。
func (s *Service) resolveDirectAgent(ctx context.Context, projectID string, thread systemdb.AgentThread, mentions []Mention) (systemdb.CloudAgent, error) {
	var agent systemdb.CloudAgent
	var err error
	if len(mentions) == 0 && thread.AgentID != "" {
		agent, err = s.ports.Agents.GetCloudAgent(ctx, projectID, thread.AgentID)
	} else {
		agent, err = s.resolveMentionedAgent(ctx, projectID, mentions)
	}
	if err != nil {
		return systemdb.CloudAgent{}, &InvalidError{Message: err.Error()}
	}
	if thread.AgentID != "" && thread.AgentID != agent.ID {
		return systemdb.CloudAgent{}, ErrAgentSwitchRequired
	}
	needsCLI := agent.Module == agentbridge.ModuleKV || agent.Module == agentbridge.ModuleGoFunction || agent.Module == agentbridge.ModuleCron
	if needsCLI && !s.cfg.SkillsCLI {
		return systemdb.CloudAgent{}, ErrCLISkillsUnavailable
	}
	if agent.Module == agentbridge.ModuleSandbox && !s.cfg.Sandbox {
		return systemdb.CloudAgent{}, ErrAgentSandboxUnavailable
	}
	return agent, nil
}

func (s *Service) resolveMentionedAgent(ctx context.Context, projectID string, mentions []Mention) (systemdb.CloudAgent, error) {
	if len(mentions) > 0 && mentions[0].AgentID != "" {
		a, err := s.ports.Agents.GetCloudAgent(ctx, projectID, mentions[0].AgentID)
		if errors.Is(err, sql.ErrNoRows) {
			return systemdb.CloudAgent{}, errors.New("mentioned agent not found")
		}
		return a, err
	}
	list, err := s.ports.Agents.ListCloudAgents(ctx, projectID)
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

// historyWithoutUserTurn 取最近 40 条消息作为模型历史，去掉刚落库的这条 user 消息
// （它单独放进 RunRequest.UserText）；重试时被重试 run 的消息也不重复出现。
func (s *Service) historyWithoutUserTurn(ctx context.Context, projectID, threadID string, userMsg systemdb.AgentMessage, retryOfRunID string) ([]systemdb.AgentMessage, error) {
	hist, err := s.ports.Runs.ListAgentMessages(ctx, projectID, threadID, 40)
	if err != nil {
		return nil, err
	}
	filtered := hist[:0]
	for _, m := range hist {
		if m.ID == userMsg.ID {
			continue
		}
		if retryOfRunID != "" && m.RunID == retryOfRunID {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered, nil
}

// loadRetry 校验重试请求（BUG-03）：目标 run 必须属于本 thread、状态
// failed/canceled、且是 thread 最新一次 run；返回原 user 消息。
// retryOfRunID 为空返回 (nil, nil)。
func (s *Service) loadRetry(ctx context.Context, projectID, threadID, retryOfRunID string) (*systemdb.AgentMessage, error) {
	if retryOfRunID == "" {
		return nil, nil
	}
	prev, err := s.ports.Runs.GetAgentRun(ctx, projectID, retryOfRunID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidRetry
	}
	if err != nil {
		return nil, err
	}
	if prev.ThreadID != threadID || (prev.Status != systemdb.AgentRunFailed && prev.Status != systemdb.AgentRunCanceled) {
		return nil, ErrInvalidRetry
	}
	latest, err := s.ports.Runs.ListAgentRuns(ctx, projectID, threadID, 1)
	if err != nil {
		return nil, err
	}
	if len(latest) == 0 || latest[0].ID != prev.ID {
		return nil, ErrInvalidRetry
	}
	msgs, err := s.ports.Runs.ListAgentMessages(ctx, projectID, threadID, 200)
	if err != nil {
		return nil, err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].RunID == prev.ID && msgs[i].Role == "user" {
			return &msgs[i], nil
		}
	}
	return nil, ErrInvalidRetry
}

// ExecuteRun 执行 run 并统一收尾，在调用方 goroutine 上运行（登记进 WaitGroup，供 Wait 等待）。
//   - extra：额外的取消源。非流式传请求 ctx（客户端断开即取消）；流式传 nil。
//   - emit：事件回调（流式写出）。nil 表示不产生 run 事件（与迁移前的非流式路径一致）。
//
// 返回的 RunOutcome 里 RunErr 是运行错误（入口据此写 error 帧），PersistErr / ReadErr 是落库与回表错误；
// Run 与 Assistant 只在同步等待结果的调用（extra != nil）时回填。
func (s *Service) ExecuteRun(extra context.Context, d DirectRun, emit func(cloudagent.Event)) RunOutcome {
	s.wg.Add(1)
	defer s.wg.Done()
	out := RunOutcome{}
	if s.runtime == nil {
		_ = s.failRun(context.WithoutCancel(s.root), d.Run, "llm_not_configured")
		out.RunErr = &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"}
		return out
	}
	runCtx, cancel := context.WithCancel(s.root)
	defer cancel()
	if extra != nil {
		defer context.AfterFunc(extra, cancel)()
	}
	// 二期 trace（planv6.5 §10.2）：run 级 ctx 注入 run_id 作为 trace_id；llmgateway 落 llm span 时携带。
	runCtx = llmgateway.WithTraceMeta(runCtx, d.Run.ID, "agent")
	var onEvent func(cloudagent.Event)
	if emit != nil {
		onEvent = s.onRunEvent(runCtx, d.Run, d.Agent, emit)
	}
	res, runErr := s.runtime.StartRun(runCtx, d.Request, onEvent)
	out.Result, out.RunErr = res, runErr
	if err := s.finishRun(context.WithoutCancel(s.root), d.Run, d.Agent, res, runErr); err != nil {
		out.PersistErr = err
		return out
	}
	// 回表：只有同步等待结果的调用方（非流式，extra != nil）需要收尾后的 run 行与 assistant 消息；
	// 流式路径只写帧，不回表。
	if extra == nil {
		return out
	}
	row, err := s.ports.Runs.GetAgentRun(extra, d.Run.ProjectID, d.Run.ID)
	if err != nil {
		out.ReadErr = err
		return out
	}
	out.Run = row
	msgs, err := s.ports.Runs.ListAgentMessages(extra, d.Run.ProjectID, d.Run.ThreadID, 200)
	if err != nil {
		out.ReadErr = err
		return out
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].RunID == d.Run.ID && msgs[i].Role == "assistant" {
			asst := msgs[i]
			out.Assistant = &asst
			break
		}
	}
	return out
}

// CancelRun 取消在途 run：运行时取消 + running/queued → canceled + 作废残留待审批；返回更新后的 run。
// 权限与发起人校验在入口完成。
func (s *Service) CancelRun(ctx context.Context, projectID, runID string) (systemdb.AgentRun, error) {
	run, err := s.ports.Runs.GetAgentRun(ctx, projectID, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return systemdb.AgentRun{}, ErrRunNotFound
	}
	if err != nil {
		return systemdb.AgentRun{}, err
	}
	if s.runtime != nil {
		s.runtime.CancelRun(runID)
	}
	finished := time.Now().UTC()
	if run.Status == systemdb.AgentRunRunning || run.Status == systemdb.AgentRunQueued {
		_ = s.transitionRun(ctx, run, systemdb.AgentRunCanceled, "canceled", nil, &finished)
		run.Status = systemdb.AgentRunCanceled
		run.FinishedAt = finished
	}
	// run 已终止，残留的 pending 审批不可能再被执行，直接作废，
	// 避免审批页留下永远点不动的待办。
	_ = s.ports.Approvals.CancelRunApprovals(ctx, projectID, runID)
	return run, nil
}

// ResolveConfirm 放行一次工具确认并把持久审批记录收敛为终态；actor 为确认人 Principal.Actor()。
// 等待表里没有该确认项时返回 ErrConfirmationNotFound（run 不存在同样按 ErrRunNotFound）。
func (s *Service) ResolveConfirm(ctx context.Context, projectID, runID, callID string, approve bool, actor string) error {
	if _, err := s.ports.Runs.GetAgentRun(ctx, projectID, runID); err != nil {
		return ErrRunNotFound
	}
	if !s.confirms.Resolve(runID, callID, approve, actor) {
		return ErrConfirmationNotFound
	}
	// 审批队列同步收敛：内存 hub 放行成功后，把持久记录也标成终态，
	// 否则列表页会一直显示这条待办（confirmation_resolved 事件也会再兜一次）。
	if pending, err := s.ports.Approvals.GetPendingAgentApproval(ctx, projectID, runID, callID); err == nil {
		status := systemdb.AgentApprovalDenied
		if approve {
			status = systemdb.AgentApprovalApproved
		}
		_ = s.ports.Approvals.ResolveAgentApproval(ctx, projectID, pending.ID, status, actor)
	}
	return nil
}

// onRunEvent 统一 run 事件的审批落库、RunID 补充与后续写出。
func (s *Service) onRunEvent(ctx context.Context, run systemdb.AgentRun, agent systemdb.CloudAgent, emit func(cloudagent.Event)) func(cloudagent.Event) {
	return func(ev cloudagent.Event) {
		s.recordApprovalEvent(ctx, run, agent, ev)
		if ev.RunID == "" {
			ev.RunID = run.ID
		}
		emit(ev)
	}
}

// recordApprovalEvent 把 confirmation_required / confirmation_resolved 事件
// 同步到 sys_agent_approvals，形成可跨请求查询的审批队列。
//
// 只做尽力而为的持久化：写库失败不阻断 run（内存 confirmHub 仍是执行路径的
// 唯一真相），但会留下「列表页看不到这条待办」的降级，属于可接受损失。
func (s *Service) recordApprovalEvent(ctx context.Context, run systemdb.AgentRun, agent systemdb.CloudAgent, ev cloudagent.Event) {
	if ev.CallID == "" {
		return
	}
	switch ev.Type {
	case "confirmation_required":
		argv, _ := json.Marshal(ev.Argv)
		timeout := s.confirmTimeout()
		var expires time.Time
		if timeout > 0 {
			expires = time.Now().UTC().Add(timeout)
		}
		_, _ = s.ports.Approvals.CreateAgentApproval(ctx, systemdb.AgentApproval{
			ProjectID:   run.ProjectID,
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
		pending, err := s.ports.Approvals.GetPendingAgentApproval(ctx, run.ProjectID, run.ID, ev.CallID)
		if err != nil {
			return
		}
		_ = s.ports.Approvals.ResolveAgentApproval(ctx, run.ProjectID, pending.ID, status, "")
	}
}

// failRun 把 run 直接标成失败（LLM 未配置等前置失败）。
func (s *Service) failRun(ctx context.Context, run systemdb.AgentRun, msg string) error {
	finished := time.Now().UTC()
	return s.transitionRun(ctx, run, systemdb.AgentRunFailed, msg, nil, &finished)
}

var _ agentbridge.RunFinalizer = (*Service)(nil)

// Finish 实现 agentbridge.RunFinalizer：cron 触发的助手 run 与 HTTP 触发的 run 共用同一套收尾。
func (s *Service) Finish(ctx context.Context, in agentbridge.RunFinish) error {
	return s.finishRun(ctx, in.Run, in.Agent, in.Result, in.Err)
}

// runTerminalTargets 是 run 状态的合法迁移目标（终态无出边；终态 → 同状态视为幂等重复）。
var runTerminalTargets = map[string][]string{
	systemdb.AgentRunQueued:  {systemdb.AgentRunCompleted, systemdb.AgentRunFailed, systemdb.AgentRunCanceled},
	systemdb.AgentRunRunning: {systemdb.AgentRunCompleted, systemdb.AgentRunFailed, systemdb.AgentRunCanceled},
}

// transitionRun 是 run 状态迁移的唯一出口：按 run 的当前快照校验目标状态，
// 非法迁移与「终态后重复写」都不落库（仓储另有 LIFE-04 条件更新兜底并发交错）。
// 错误码规则由调用方给定：运行时取消为空码、用户取消为 canceled、失败为分类码。
func (s *Service) transitionRun(ctx context.Context, run systemdb.AgentRun, to, code string, started, finished *time.Time) error {
	if run.Status == to {
		return nil
	}
	if !slices.Contains(runTerminalTargets[run.Status], to) {
		return nil
	}
	return s.ports.Runs.UpdateAgentRunStatus(ctx, run.ProjectID, run.ID, to, code, started, finished)
}

// finishRun 是 run 的统一收尾：落 assistant 消息（正文或工具卡片）、run 状态与用量、
// 工具 span，并写审计（只记元数据，不落 argv / stdin / LLM 正文）。
// 固定助手会话、群任务与取消路径共用；仓储的单向迁移语义（终态不可覆盖）保证并发收尾幂等。
func (s *Service) finishRun(ctx context.Context, run systemdb.AgentRun, agent systemdb.CloudAgent, res cloudagent.RunResult, runErr error) error {
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
	_ = s.ports.Runs.RecordAgentToolSpans(run.ProjectID, run.ID, res.ToolCallsJSON)
	if res.Content != "" || len(res.ToolCallsJSON) > 2 {
		if _, err := s.ports.Runs.AppendAgentMessage(ctx, systemdb.AgentMessage{
			ThreadID: run.ThreadID, ProjectID: run.ProjectID, Role: "assistant", Content: res.Content,
			AgentID: agent.ID, ToolCallsJSON: res.ToolCallsJSON, RunID: run.ID,
		}); err != nil {
			return err
		}
	}
	finished := time.Now().UTC()
	if err := s.transitionRun(ctx, run, status, code, nil, &finished); err != nil {
		return err
	}
	if err := s.ports.Runs.UpdateAgentRunMetrics(ctx, run.ProjectID, run.ID, systemdb.AgentRun{
		DurationMS: res.DurationMS, PromptTokens: res.PromptTokens, CompletionTokens: res.CompletionTokens,
		ReasoningTokens: res.ReasoningTokens, ToolCalls: res.ToolCalls, ErrorCode: code,
	}); err != nil {
		return err
	}
	if s.ports.Audit != nil {
		// planv6.0 v2 §3.3：写能力的审计只记元数据（是否可写 + 工具调用数），
		// 不落 argv / stdin / LLM 正文。
		detail := fmt.Sprintf("run=%s thread=%s agent=%s duration_ms=%d prompt_tokens=%d completion_tokens=%d tool_calls=%d error_code=%s",
			run.ID, run.ThreadID, agent.ID, res.DurationMS, res.PromptTokens, res.CompletionTokens, res.ToolCalls, code)
		_ = s.ports.Audit.Record(ctx, audit.Event{ProjectID: run.ProjectID, PrincipalID: run.InitiatorID, Kind: "agent.run", Status: status, Detail: detail})
	}
	return nil
}
