// 文件说明：turn.go — 协作群轮次用例（提交 / 订阅 / 动作 / 快照读取）
//
// 用法
//
//	turnID, err := svc.SubmitTurn(ctx, TurnInput{…})                        // POST /agent-threads/:id/turns（Plan 栅栏）
//	sub, err := svc.SubscribeTurn(ctx, projectID, turnID, after)            // GET /agent-turns/:id/events（SSE 只是订阅者）
//	svc.ConfirmTurnPlan / CancelTurn / RetryTask / ResumeTurn / ReplyHuman  // 动作：状态迁移全部在 gocloudagent/group 引擎
//	svc.LoadTurnTasks / TurnPlan                                             // 入口组装轮次快照（DTO）用
//
// 调用架构
//
//	[上游] internal/api（collab_coordinator / collab_handler / collab_query_handler）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/turn.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/group, internal/auth, internal/systemdb, internal/usage
//	[同包] group_engine.go, group_store.go, cron.go
//
// 主要符号
//
//	type MentionTarget struct {…}
//	type TurnInput struct {…}
//	type turnContext struct {…}
//	type HeadlessTurnInput struct {…}
//	(s *Service) SubmitTurn(ctx context.Context, in TurnInput) (string, error)
//	(s *Service) RunHeadlessTurn(ctx context.Context, in HeadlessTurnInput) (string, error)
//	(s *Service) SubscribeTurn(ctx context.Context, projectID, turnID string, after int) (*group.Subscription, error)
//	(s *Service) ConfirmTurnPlan(ctx, projectID, principal, turnID, ap group.Approval) error
//	(s *Service) ReplyHuman(ctx, projectID, principal, task, text, outcome) error
//	(s *Service) claimTurn(ctx, projectID, threadID, turnID) error
//	... 另有 8 个顶层符号
//
// 归属判定（entry 留什么、用例拿什么）与迁移前逐条对应：主体权限 / 归属校验、请求体解析、DTO 编码在入口；
// 会话元数据读取、标签解析、表就绪 / 模型 / 配额前置、引擎调用与「回复元数据 + 引擎动作 + 消息落库」在这里。

package agentrun

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/usage"
)

// MentionTarget 是结构化 @ 目标：{kind,id}（kind = agent / human，与入口请求体同形）。
type MentionTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func (m MentionTarget) mentionAgentID() string {
	if m.Kind == plan.TargetAgent {
		return strings.TrimSpace(m.ID)
	}
	return ""
}

func (m MentionTarget) mentionIsHuman() bool {
	return m.Kind == plan.TargetHuman
}

// TurnInput 是发起一轮群消息的输入；Thread 由入口读取后传入。
type TurnInput struct {
	ProjectID      string
	Thread         systemdb.AgentThread
	Principal      auth.Principal
	Content        string
	Mentions       []MentionTarget
	IdempotencyKey string
}

// turnContext 是一次轮次提交的上下文快照。
type turnContext struct {
	projectID string
	principal auth.Principal
	thread    systemdb.AgentThread
	ws        systemdb.AgentThreadWorkspace
	collab    systemdb.AgentThreadCollab
	tag       systemdb.AgentTag
	// humanID 是当前可指派的人（API Key 主体为空，表示不能指派）。
	humanID string
}

// SubmitTurn 发起一轮群消息：群归属以会话元数据为准，经 Plan 栅栏后由引擎后台推进。
// 返回 turnID；执行在后台，SSE 仅订阅。入口已校验 content 非空。
func (s *Service) SubmitTurn(ctx context.Context, in TurnInput) (string, error) {
	if s.stopping.Load() {
		return "", ErrStopping
	}
	ws, err := s.ports.Threads.GetAgentThreadWorkspace(ctx, in.ProjectID, in.Thread.ID)
	if err != nil {
		return "", err
	}
	collab, err := s.ports.Threads.GetAgentThreadCollab(ctx, in.ProjectID, in.Thread.ID)
	if err != nil {
		return "", err
	}
	// 群归属以会话元数据为准。
	tagID := ws.TagID
	if collab.TagID != "" {
		tagID = collab.TagID
	}
	var tag systemdb.AgentTag
	if tagID != "" {
		tag, err = s.ResolveAgentTag(ctx, in.ProjectID, tagID)
		if err != nil {
			// 群已归档：历史可读，但不接受新发送。
			if errors.Is(err, systemdb.ErrAgentTagNotFound) {
				return "", ErrGroupArchived
			}
			return "", err
		}
	}
	if err := s.ports.Collab.CollabSchemaReady(ctx); err != nil {
		return "", ErrWorkspaceUpgradeRequired
	}
	if !s.llmConfigured() {
		return "", &cloudagent.RunError{Code: "llm_not_configured", Message: "模型服务未配置"}
	}
	if s.ports.Quota != nil {
		if err := s.ports.Quota.CheckQuota(ctx, in.ProjectID, "llm"); err != nil {
			if errors.Is(err, usage.ErrQuotaExceeded) {
				return "", ErrQuotaExceeded
			}
			return "", err
		}
	}
	tc := turnContext{projectID: in.ProjectID, principal: in.Principal, thread: in.Thread, ws: ws, collab: collab, tag: tag}
	// 人只能由登录态担任：API Key 没有「人」的身份。
	tc.humanID = HumanActorOf(in.Principal)
	if humanPolicyDisabled(tag) {
		tc.humanID = ""
	}
	return s.runGroupTurn(ctx, tc, in.Content, in.Mentions, strings.TrimSpace(in.IdempotencyKey), false)
}

// HeadlessTurnInput 是机器入口（告警 / 统计事件）在已建好的会话上同步执行一轮的输入；
// 表就绪 / 模型等前置校验由入口完成，这里不做配额检查（与迁移前一致）。
type HeadlessTurnInput struct {
	ProjectID      string
	Thread         systemdb.AgentThread
	Workspace      systemdb.AgentThreadWorkspace
	Collab         systemdb.AgentThreadCollab
	Tag            systemdb.AgentTag
	Principal      auth.Principal
	Content        string
	IdempotencyKey string
}

// RunHeadlessTurn 在已确定的会话 / 群上同步执行一轮 headless 轮次，返回 turnID。
// 机器入口没有「当前人」：不指派人工任务；需要确认按策略拒绝并记录。
func (s *Service) RunHeadlessTurn(ctx context.Context, in HeadlessTurnInput) (string, error) {
	if s.stopping.Load() {
		return "", ErrStopping
	}
	tc := turnContext{projectID: in.ProjectID, principal: in.Principal, thread: in.Thread, ws: in.Workspace, collab: in.Collab, tag: in.Tag}
	return s.runGroupTurn(ctx, tc, in.Content, nil, strings.TrimSpace(in.IdempotencyKey), true)
}

// runGroupTurn 提交一轮（HTTP 或 headless）：组装 TurnInput，用户消息在栅栏前落库（BeforeFence）。
// headless 为 true 时同步执行到终态或待人工（cron / 告警）。
func (s *Service) runGroupTurn(ctx context.Context, tc turnContext, content string, mentions []MentionTarget, idempotencyKey string, headless bool) (string, error) {
	ctx = withCollabGroupIdentity(ctx, tc.principal, tc.humanID, headless)
	engine, err := s.groupEngineFor(tc.projectID)
	if err != nil {
		return "", err
	}
	history, err := (&collabGroupMembers{svc: s, projectID: tc.projectID}).History(ctx, tc.thread.ID, 6)
	if err != nil {
		return "", err
	}
	agentMentions := make([]string, 0, len(mentions))
	mentionedHuman := false
	for _, mention := range mentions {
		if id := mention.mentionAgentID(); id != "" && !slices.Contains(agentMentions, id) {
			agentMentions = append(agentMentions, id)
		}
		mentionedHuman = mentionedHuman || mention.mentionIsHuman()
	}
	input := group.TurnInput{
		ThreadID: tc.thread.ID, GroupID: tagIDOf(tc), Initiator: tc.principal.Actor(),
		UserText: content, Mentions: agentMentions, MentionedHuman: mentionedHuman,
		History: history, IdempotencyKey: idempotencyKey,
		RoutingPrompt: tc.tag.Config.RoutingPrompt,
		BeforeFence: func(ctx context.Context, turn group.Turn) error {
			message, err := s.ports.Runs.AppendAgentMessage(ctx, systemdb.AgentMessage{
				ThreadID: tc.thread.ID, ProjectID: tc.projectID, Role: "user", Content: content,
				MentionsJSON: marshalMentions(mentions),
			})
			if err != nil {
				return err
			}
			return s.ports.Collab.AppendAgentMessageMeta(ctx, systemdb.AgentMessageMeta{
				ProjectID: tc.projectID, MessageID: message.ID, TurnID: turn.ID,
				SpeakerKind: "user", SpeakerID: tc.principal.Actor(), Kind: systemdb.MessageKindUser,
			})
		},
	}
	if headless {
		result, err := engine.RunHeadless(ctx, input)
		return result.TurnID, err
	}
	turn, err := engine.Submit(ctx, input)
	return turn.ID, err
}

func tagIDOf(tc turnContext) string {
	if tc.ws.TagID != "" {
		return tc.ws.TagID
	}
	return tc.collab.TagID
}

// claimTurn 抢占会话活动轮次；旧会话缺协作行时先补行再抢占。
func (s *Service) claimTurn(ctx context.Context, projectID, threadID, turnID string) error {
	err := s.ports.Collab.ClaimAgentTurn(ctx, projectID, threadID, turnID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, systemdb.ErrTurnBusy) {
		return err
	}
	existing, getErr := s.ports.Threads.GetAgentThreadCollab(ctx, projectID, threadID)
	if getErr != nil {
		return getErr
	}
	if existing.ThreadID != "" {
		return ErrTurnBusy
	}
	if _, err := s.ports.Threads.CreateAgentThreadCollab(ctx, systemdb.AgentThreadCollab{
		ProjectID: projectID, ThreadID: threadID,
	}); err != nil {
		return err
	}
	if err := s.ports.Collab.ClaimAgentTurn(ctx, projectID, threadID, turnID); err != nil {
		return ErrTurnBusy
	}
	return nil
}

func marshalMentions(mentions []MentionTarget) string {
	if len(mentions) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(mentions)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// SubscribeTurn 订阅轮次事件流（SSE 只是订阅者；after 为已收到的最大 seq）。
func (s *Service) SubscribeTurn(ctx context.Context, projectID, turnID string, after int) (*group.Subscription, error) {
	engine, err := s.groupEngineFor(projectID)
	if err != nil {
		return nil, err
	}
	return engine.Subscribe(ctx, turnID, after)
}

// ConfirmTurnPlan 批准 / 编辑后批准 / 拒绝整份计划；推进交给引擎后台。
func (s *Service) ConfirmTurnPlan(ctx context.Context, projectID string, principal auth.Principal, turnID string, ap group.Approval) error {
	engine, err := s.groupEngineFor(projectID)
	if err != nil {
		return err
	}
	ctx = withCollabGroupIdentity(ctx, principal, HumanActorOf(principal), false)
	_, err = engine.ConfirmPlan(ctx, turnID, ap)
	return err
}

// CancelTurn 通过轮次级 context 取消执行器，由引擎统一收尾。
func (s *Service) CancelTurn(ctx context.Context, projectID string, principal auth.Principal, turnID string) error {
	engine, err := s.groupEngineFor(projectID)
	if err != nil {
		return err
	}
	_, err = engine.Cancel(withCollabGroupIdentity(ctx, principal, "", false), turnID)
	return err
}

// RetryTask 重试单个失败任务：重置该任务与依赖它的未完成下游，然后继续推进。
func (s *Service) RetryTask(ctx context.Context, projectID string, principal auth.Principal, taskID string) error {
	engine, err := s.groupEngineFor(projectID)
	if err != nil {
		return err
	}
	_, err = engine.RetryTask(withCollabGroupIdentity(ctx, principal, "", false), taskID)
	return err
}

// ResumeTurn 恢复中断 / 等人的轮次，后台推进由引擎负责。
func (s *Service) ResumeTurn(ctx context.Context, projectID string, principal auth.Principal, turnID string) error {
	engine, err := s.groupEngineFor(projectID)
	if err != nil {
		return err
	}
	_, err = engine.Resume(withCollabGroupIdentity(ctx, principal, "", false), turnID)
	return err
}

// ReplyHuman 处理人工待办的回复：先落回复元数据，再交给引擎迁移状态并继续推进，
// 最后把回复作为一条用户消息写入会话。归属 / 状态 / 版本 / 长度校验在入口完成。
func (s *Service) ReplyHuman(ctx context.Context, projectID string, principal auth.Principal, task systemdb.AgentTask, text, outcome string) error {
	caller := principal.Actor()
	// 回复元数据与状态迁移分开：状态只能由 Engine 状态机修改。
	task.ReplyText = text
	task.ReplyOutcome = outcome
	task.RepliedBy = caller
	if _, err := s.ports.Collab.UpdateAgentTask(ctx, task, task.Version); err != nil {
		return err
	}
	engine, err := s.groupEngineFor(projectID)
	if err != nil {
		return err
	}
	if _, err := engine.ReplyHuman(withCollabGroupIdentity(ctx, principal, HumanActorOf(principal), false), task.ID, group.Reply{Text: text, Outcome: outcome}); err != nil {
		return err
	}
	if text != "" {
		message, err := s.ports.Runs.AppendAgentMessage(ctx, systemdb.AgentMessage{
			ThreadID: task.ThreadID, ProjectID: projectID, Role: "user", Content: "[人工回复] " + text,
		})
		if err != nil {
			return err
		}
		_ = s.ports.Collab.AppendAgentMessageMeta(ctx, systemdb.AgentMessageMeta{
			ProjectID: projectID, MessageID: message.ID, TurnID: task.TurnID, TaskID: task.ID,
			SpeakerKind: systemdb.TaskTargetHuman, SpeakerID: caller, Kind: systemdb.MessageKindHuman,
		})
	}
	return nil
}

// LoadTurnTasks 返回轮次任务的引擎视图（含依赖 / 上下文来源 / 结果），入口组装轮次快照用。
func (s *Service) LoadTurnTasks(ctx context.Context, projectID, turnID string) ([]group.Task, error) {
	return (&collabGroupStore{store: s.ports.Collab, projectID: projectID}).LoadTasks(ctx, turnID)
}

// TurnPlan 返回 route_json 信封的 JSON（入口的 plan 字段）；没有决策时 ok=false。
func (s *Service) TurnPlan(turn systemdb.AgentTurn) (json.RawMessage, bool) {
	route, err := parseGroupRoute(turn.RouteJSON)
	if err != nil || route.Decision.Action == "" {
		return nil, false
	}
	raw, err := json.Marshal(route)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// CollabSchemaReady 报告协作表是否就绪；入口的能力探测用。
func (s *Service) CollabSchemaReady(ctx context.Context) error {
	return s.ports.Collab.CollabSchemaReady(ctx)
}
