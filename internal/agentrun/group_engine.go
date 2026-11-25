// 文件说明：group_engine.go — 项目群引擎的装配与 group.Members / 身份上下文
//
// 用法
//
//	(s *Service) groupEngineFor(projectID)                      // 取或建项目的 group.Engine（按项目隔离）
//	(s *Service) workspaceCandidates(ctx, projectID, tagID)     // 群可参与的助手（剔除能力不可用者）
//	withCollabGroupIdentity(ctx, principal, humanID, headless)  // 把发起人 / 可指派的人 / headless 标记放进 ctx
//	HumanActorOf(p auth.Principal) string                           // 登录用户才能担任「人」；入口的待办列表用
//
// 调用架构
//
//	[上游] internal/agentrun（turn.go SubmitTurn、cron.go、service.go Converge）
//	         │ 同包调用
//	         ▼
//	[本文件] internal/agentrun/group_engine.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/group, gocloudagent/plan, internal/agentbridge, internal/auth, internal/systemdb
//	[同包] group_store.go, group_runner.go, service.go
//
// 主要符号
//
//	type collabGroupIdentity struct {…}
//	type collabGroupMembers struct {…}
//	(s *Service) groupEngineFor(projectID string) (*group.Engine, error)
//	(s *Service) workspaceCandidates(ctx context.Context, projectID, tagID string) ([]systemdb.CloudAgent, error)
//	... 另有 7 个顶层符号
//
// 引擎生命周期归应用服务：每项目一个 Engine，共享该项目的轮次登记与事件 broker；
// 引擎的 Store / Runner / Members 三个端口分别由 group_store.go、group_runner.go 与本文件实现。

package agentrun

import (
	"context"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type collabGroupIdentity struct {
	principal auth.Principal
	humanID   string
	headless  bool
}

type collabGroupIdentityKey struct{}

func withCollabGroupIdentity(ctx context.Context, principal auth.Principal, humanID string, headless bool) context.Context {
	return context.WithValue(ctx, collabGroupIdentityKey{}, collabGroupIdentity{
		principal: principal, humanID: strings.TrimSpace(humanID), headless: headless,
	})
}

func collabGroupIdentityFrom(ctx context.Context) collabGroupIdentity {
	identity, _ := ctx.Value(collabGroupIdentityKey{}).(collabGroupIdentity)
	return identity
}

// groupEngineFor 返回（必要时创建）项目的群引擎。竞态时后建者被丢弃；停机后不再创建，
// 且与 BeginShutdown 并发创建出的引擎立即进入停机，保证 Wait 能等到它的收尾。
func (s *Service) groupEngineFor(projectID string) (*group.Engine, error) {
	if s.stopping.Load() {
		return nil, ErrStopping
	}
	if cached, ok := s.groupEngines.Load(projectID); ok {
		return cached.(*group.Engine), nil
	}
	var planner *plan.Planner
	if s.runtime != nil && s.runtime.LLM != nil {
		planner = &plan.Planner{LLM: s.runtime.LLM, Scope: projectID}
	}
	store := &collabGroupStore{store: s.ports.Collab, projectID: projectID, claimTurn: s.claimTurn}
	engine, err := group.NewEngine(
		planner,
		&collabGroupRunner{svc: s, projectID: projectID},
		store,
		&collabGroupMembers{svc: s, projectID: projectID},
	)
	if err != nil {
		return nil, err
	}
	actual, _ := s.groupEngines.LoadOrStore(projectID, engine)
	engine = actual.(*group.Engine)
	if s.stopping.Load() {
		engine.BeginShutdown()
	}
	return engine, nil
}

// humanPolicyDisabled 报告群是否关闭了人工任务。
func humanPolicyDisabled(tag systemdb.AgentTag) bool {
	return tag.Config.HumanPolicy == systemdb.HumanPolicyDisabled
}

// HumanActorOf 返回主体作为「人」的操作人标识：只有登录用户可以担任，
// API Key 没有人的身份，返回空串（v2 §7.3）。assignee_id / 待办归属都用这个格式。
func HumanActorOf(p auth.Principal) string {
	if p.UserID == "" {
		return ""
	}
	return p.Actor()
}

// collabGroupMembers 实现 group.Members：群描述、可 @ 的助手与群聊历史投影。
type collabGroupMembers struct {
	svc       *Service
	projectID string
}

func (m *collabGroupMembers) Group(ctx context.Context, groupID string) (group.Group, error) {
	tagID := groupID
	if tagID == "" {
		tagID = systemdb.AgentTagGeneral
	}
	tag, err := m.svc.ResolveAgentTag(ctx, m.projectID, tagID)
	if err != nil {
		return group.Group{}, err
	}
	candidates, err := m.svc.workspaceCandidates(ctx, m.projectID, tag.ID)
	if err != nil {
		return group.Group{}, err
	}
	identity := collabGroupIdentityFrom(ctx)
	members := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		members = append(members, candidate.ID)
	}
	humanName := identity.principal.Username
	if humanName == "" && identity.humanID != "" {
		humanName = "当前用户"
	}
	return group.Group{
		ID: groupID, MemberIDs: members, HumanID: identity.humanID, HumanName: humanName,
		HumanDisabled: humanPolicyDisabled(tag),
		Policy: group.Policy{
			Confirm:       plan.ConfirmPolicy{Mode: strings.ToLower(strings.TrimSpace(tag.Config.ConfirmPolicy))},
			FallbackAgent: tag.Config.FallbackAgentID,
		},
	}, nil
}

func (m *collabGroupMembers) Candidates(ctx context.Context, groupID string) ([]plan.Candidate, error) {
	tagID := groupID
	if tagID == "" {
		tagID = systemdb.AgentTagGeneral
	}
	candidates, err := m.svc.workspaceCandidates(ctx, m.projectID, tagID)
	if err != nil {
		return nil, err
	}
	out := make([]plan.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, plan.Candidate{
			ID: candidate.ID, Name: candidate.Name, Module: candidate.Module,
			Description: candidate.Description, Skills: append([]string(nil), candidate.ToolIDs...),
		})
	}
	return out, nil
}

func (m *collabGroupMembers) History(ctx context.Context, threadID string, limit int) ([]plan.Turn, error) {
	messages, err := m.svc.ports.Runs.ListAgentMessages(ctx, m.projectID, threadID, 40)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	out := make([]plan.Turn, 0, len(messages))
	for _, message := range messages {
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		out = append(out, plan.Turn{Role: message.Role, Speaker: message.AgentID, Content: message.Content})
	}
	return out, nil
}

// workspaceCandidates 返回一个群可参与的助手：标签配置顺序 ∩ 未归档，再剔除当前实例能力不可用的
// （CLI 技能 / 云沙盒），保持顺序。
func (s *Service) workspaceCandidates(ctx context.Context, projectID, tagID string) ([]systemdb.CloudAgent, error) {
	agents, err := s.ports.Agents.ListCloudAgents(ctx, projectID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]systemdb.CloudAgent, len(agents))
	for _, a := range agents {
		byID[a.ID] = a
	}
	tag, err := s.ResolveAgentTag(ctx, projectID, tagID)
	if err != nil {
		return nil, err
	}
	out := make([]systemdb.CloudAgent, 0, len(tag.Config.AgentIDs))
	for _, id := range tag.Config.AgentIDs {
		a, ok := byID[id]
		if !ok {
			continue
		}
		if (a.Module == agentbridge.ModuleKV || a.Module == agentbridge.ModuleGoFunction || a.Module == agentbridge.ModuleCron) && !s.cfg.SkillsCLI {
			continue
		}
		if a.Module == agentbridge.ModuleSandbox && !s.cfg.Sandbox {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}
