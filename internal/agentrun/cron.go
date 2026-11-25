// 文件说明：cron.go — 群定时任务的执行（实现 agentbridge.GroupCronRunner）
//
// 用法
//
//	(s *Service) RunGroupCron(ctx, projectID, jobID, bind)   // 一次群定时触发：去重 → 定位 / 新建会话 → headless 轮次 → 回写
//	(s *Service) RunGroupCronWithKey(ctx, projectID, jobID, fireKey, bind)  // 显式去重键
//	svc 由 app 直接注入 cron 执行器（agentbridge.CronAgentExecutor.WithGroupRunner），不再经 HTTP handler 延迟绑定
//
// 调用架构
//
//	[上游] internal/agentbridge（CronAgentExecutor ← cronjob 调度器）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentrun/cron.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/group, internal/agentbridge, internal/auth, internal/catalog, internal/systemdb
//	[同包] turn.go, group_engine.go, tags.go
//
// 主要符号
//
//	cronFireKey(jobID string, now time.Time) string
//	(s *Service) RunGroupCron(ctx context.Context, projectID, jobID string, bind agentbridge.GroupCronBinding) (agentbridge.GroupCronResult, error)
//	(s *Service) RunGroupCronWithKey(ctx context.Context, projectID, jobID, fireKey string, bind agentbridge.GroupCronBinding) (agentbridge.GroupCronResult, error)
//	(s *Service) groupCronThread(ctx, projectID, jobID, bind, tag) (string, error)
//	(s *Service) runScheduledTurn(ctx, tc, prompt, idempotencyKey) (systemdb.AgentTurn, error)
//
// cron 记录不落用户正文：结果摘要只用轮次状态；负责人只用于把人工任务指派给他。

package agentrun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

var _ agentbridge.GroupCronRunner = (*Service)(nil)

// cronFireKey 生成触发去重键：任务 + 计划触发时间（秒级）+ 触发来源。
//
// 调度器不向 AgentRunner 传计划触发时间，因此这里用「当前分钟」做窗口：
// 同一分钟内重复投递同一条目只执行一次（调度器的最小粒度是分钟）。
func cronFireKey(jobID string, now time.Time) string {
	return fmt.Sprintf("%s@%s", jobID, now.UTC().Format("20060102T1504"))
}

// RunGroupCron 执行一次群定时（实现 agentbridge.GroupCronRunner）。
// RunGroupCron 执行一次群定时（实现 agentbridge.GroupCronRunner；由 cron 调度器经 CronAgentExecutor 调用）。
// 同步执行到终态或待人工；服务停机后返回 ErrStopping。
func (s *Service) RunGroupCron(ctx context.Context, projectID, jobID string, bind agentbridge.GroupCronBinding) (agentbridge.GroupCronResult, error) {
	return s.RunGroupCronWithKey(ctx, projectID, jobID, cronFireKey(jobID, time.Now()), bind)
}

// RunGroupCronWithKey 以给定的触发去重键执行一次群定时（RunGroupCron 用「任务 + 当前分钟」作键）。
func (s *Service) RunGroupCronWithKey(ctx context.Context, projectID, jobID, fireKey string, bind agentbridge.GroupCronBinding) (agentbridge.GroupCronResult, error) {
	if s.stopping.Load() {
		return agentbridge.GroupCronResult{}, ErrStopping
	}
	if err := s.ports.Collab.CollabSchemaReady(ctx); err != nil {
		return agentbridge.GroupCronResult{}, ErrWorkspaceUpgradeRequired
	}
	// 触发去重：同键（默认同分钟）重复投递直接跳过（返回上次结果摘要）。
	claimed, err := s.ports.Cron.ClaimCronFire(ctx, projectID, jobID, fireKey)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	if !claimed {
		return agentbridge.GroupCronResult{Response: "skipped: duplicate fire"}, nil
	}
	// 群必须仍存在且未归档：归档即暂停调度（不报错，避免刷失败记录）。
	tag, err := s.ResolveAgentTag(ctx, projectID, bind.TagID)
	if err != nil {
		return agentbridge.GroupCronResult{}, ErrGroupArchived
	}
	threadID, err := s.groupCronThread(ctx, projectID, jobID, bind, tag)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	thread, err := s.ports.Threads.GetAgentThread(ctx, projectID, threadID)
	if err != nil {
		return agentbridge.GroupCronResult{}, errors.New("schedule thread unavailable")
	}
	ws, err := s.ports.Threads.GetAgentThreadWorkspace(ctx, projectID, threadID)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	collab, err := s.ports.Threads.GetAgentThreadCollab(ctx, projectID, threadID)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	// 只读 Headless 主体：与既有 cron agent 口径一致（system:scheduler）。
	principal := auth.Principal{
		APIKeyID:   auth.ActorScheduler,
		TenantID:   catalog.ReservedTenantID,
		ProjectIDs: map[string]struct{}{projectID: {}},
		Permissions: map[auth.Permission]struct{}{
			auth.DatabaseRead: {},
		},
	}
	tc := turnContext{
		projectID: projectID,
		principal: principal,
		thread:    thread,
		ws:        ws,
		collab:    collab,
		tag:       tag,
		// 定时任务没有「当前人」；负责人只用于把人工任务指派给他。
		humanID: strings.TrimSpace(bind.OwnerUserID),
	}
	prompt := strings.TrimSpace(bind.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(tag.Config.RoutingPrompt)
	}
	if prompt == "" {
		return agentbridge.GroupCronResult{}, errors.New("schedule prompt is empty")
	}
	turn, err := s.runScheduledTurn(ctx, tc, prompt, fireKey)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	_ = s.ports.Cron.CompleteCronFire(ctx, projectID, jobID, fireKey, turn.ID, threadID)
	return agentbridge.GroupCronResult{
		TurnID: turn.ID, ThreadID: threadID,
		// 只用状态做摘要：cron 记录不落用户正文。
		Response: "turn " + turn.Status,
	}, nil
}

// groupCronThread 按策略返回本次触发的会话 ID（必要时新建并回写绑定）。
func (s *Service) groupCronThread(ctx context.Context, projectID, jobID string, bind agentbridge.GroupCronBinding, tag systemdb.AgentTag) (string, error) {
	if bind.ThreadPolicy == systemdb.GroupCronReuseThread {
		if id := strings.TrimSpace(bind.ThreadID); id != "" {
			// 会话仍存在且未归档时直接复用。
			if _, err := s.ports.Threads.GetAgentThread(ctx, projectID, id); err == nil {
				return id, nil
			}
		}
	} else if bind.ThreadPolicy != "" && bind.ThreadPolicy != systemdb.GroupCronNewThreadPerFire {
		return "", errors.New("invalid thread_policy")
	}
	kind := strings.TrimSpace(bind.ThreadKind)
	if kind == "" {
		kind = systemdb.ThreadKindScheduled
	}
	title := "Scheduled: " + tag.Name
	th, err := s.ports.Threads.CreateAgentThreadWithWorkspace(ctx, systemdb.AgentThread{
		ProjectID: projectID, Title: title, CreatedBy: auth.ActorScheduler,
	}, systemdb.AgentThreadWorkspace{
		TagID: tag.ID, DispatchMode: systemdb.DispatchCollaborative,
		Source: systemdb.ThreadSourceWorkspace,
	})
	if err != nil {
		return "", err
	}
	if _, err := s.ports.Threads.CreateAgentThreadCollab(ctx, systemdb.AgentThreadCollab{
		ProjectID: projectID, ThreadID: th.ID, TagID: tag.ID, ThreadKind: kind,
	}); err != nil {
		_ = s.ports.Threads.ArchiveAgentThread(ctx, projectID, th.ID)
		return "", err
	}
	// reuse_thread 需要把新会话回写到绑定，后续触发才能继续复用。
	if bind.ThreadPolicy == systemdb.GroupCronReuseThread || bind.ThreadPolicy == "" {
		if _, err := s.ports.Cron.UpsertAgentGroupCron(ctx, systemdb.AgentGroupCron{
			ProjectID: projectID, JobID: jobID, TagID: tag.ID,
			ThreadPolicy: systemdb.GroupCronReuseThread, ThreadKind: kind,
			ThreadID: th.ID, OwnerUserID: bind.OwnerUserID, Prompt: bind.Prompt,
		}); err != nil {
			return "", err
		}
	}
	return th.ID, nil
}

// runScheduledTurn 同步执行一个 headless 轮次并返回最终或待人工快照。
// runScheduledTurn 同步执行一个 headless 轮次并返回最终或待人工快照。
func (s *Service) runScheduledTurn(ctx context.Context, tc turnContext, prompt, idempotencyKey string) (systemdb.AgentTurn, error) {
	ctx = withCollabGroupIdentity(ctx, tc.principal, tc.humanID, true)
	engine, err := s.groupEngineFor(tc.projectID)
	if err != nil {
		return systemdb.AgentTurn{}, err
	}
	history, err := (&collabGroupMembers{svc: s, projectID: tc.projectID}).History(ctx, tc.thread.ID, 6)
	if err != nil {
		return systemdb.AgentTurn{}, err
	}
	result, err := engine.RunHeadless(ctx, group.TurnInput{
		ThreadID: tc.thread.ID, GroupID: tagIDOf(tc), Initiator: auth.ActorScheduler,
		UserText: prompt, History: history, IdempotencyKey: idempotencyKey,
		RoutingPrompt: tc.tag.Config.RoutingPrompt,
		BeforeFence: func(ctx context.Context, turn group.Turn) error {
			message, err := s.ports.Runs.AppendAgentMessage(ctx, systemdb.AgentMessage{
				ThreadID: tc.thread.ID, ProjectID: tc.projectID, Role: "user", Content: "[scheduled] " + prompt,
			})
			if err != nil {
				return err
			}
			return s.ports.Collab.AppendAgentMessageMeta(ctx, systemdb.AgentMessageMeta{
				ProjectID: tc.projectID, MessageID: message.ID, TurnID: turn.ID,
				SpeakerKind: "scheduler", SpeakerID: auth.ActorScheduler, Kind: systemdb.MessageKindUser,
			})
		},
	})
	if err != nil {
		return systemdb.AgentTurn{}, err
	}
	return s.ports.Collab.GetAgentTurn(ctx, tc.projectID, result.TurnID)
}
