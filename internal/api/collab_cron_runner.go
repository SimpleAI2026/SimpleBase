// 文件说明：collab_cron_runner.go — 群定时任务的执行器实现（planv6.7 v2 §11）。
//
// 定位
//
//	internal/agentbridge 只声明窄接口 GroupCronRunner；实现必须落在本包，
//	因为轮次协调器（RunTurn）、run 行、消息、审批、SSE 缓存都在 internal/api。
//	internal/app 在装配 cron 调度器时先创建本类型，再在 router 装配后 Attach。
//
// 语义
//
//   - reuse_thread：每次触发复用绑定里的会话（缺失或已归档时新建并回写）。
//   - new_thread_per_fire：每次触发新建一条会话，类型由绑定决定（默认 scheduled）。
//   - fire_key 由「任务 + 计划触发时间」组成，重复投递只执行一次。
//   - 执行身份是只读 Headless 主体（APIKeyID=system-scheduler），
//     因此不会产生人工任务；需要人时按 human_assignee_required 失败。
package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// GroupCronRunner 是延迟绑定的群定时执行器。
//
// app 的装配顺序是「cron 调度器 → api router」，而 handler 在 router 内部创建，
// 因此这里先返回一个可 Attach 的句柄：调度器持有它，router 装配时把 handler 装进来。
type GroupCronRunner struct {
	mu      sync.RWMutex
	handler *cloudAgentHandler
}

// NewGroupCronRunner 创建延迟绑定的执行器。
func NewGroupCronRunner() *GroupCronRunner { return &GroupCronRunner{} }

// Attach 注入 handler（router 装配完成后调用一次）。
func (r *GroupCronRunner) Attach(h *cloudAgentHandler) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.handler = h
	r.mu.Unlock()
}

func (r *GroupCronRunner) current() *cloudAgentHandler {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handler
}

var _ agentbridge.GroupCronRunner = (*GroupCronRunner)(nil)

// cronFireKey 生成触发去重键：任务 + 计划触发时间（秒级）+ 触发来源。
//
// 调度器不向 AgentRunner 传计划触发时间，因此这里用「当前分钟」做窗口：
// 同一分钟内重复投递同一条目只执行一次（调度器的最小粒度是分钟）。
func cronFireKey(jobID string, now time.Time) string {
	return fmt.Sprintf("%s@%s", jobID, now.UTC().Format("20060102T1504"))
}

// RunGroupCron 执行一次群定时（实现 agentbridge.GroupCronRunner）。
func (r *GroupCronRunner) RunGroupCron(ctx context.Context, projectID, jobID string, bind agentbridge.GroupCronBinding) (agentbridge.GroupCronResult, error) {
	h := r.current()
	if h == nil {
		return agentbridge.GroupCronResult{}, errors.New("collaboration runner is not attached")
	}
	if err := h.store.CollabSchemaReady(ctx); err != nil {
		return agentbridge.GroupCronResult{}, ErrWorkspaceUpgradeRequired
	}
	// 触发去重：同分钟重复投递直接跳过（返回上次结果摘要）。
	fireKey := cronFireKey(jobID, time.Now())
	claimed, err := h.store.ClaimCronFire(ctx, projectID, jobID, fireKey)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	if !claimed {
		return agentbridge.GroupCronResult{Response: "skipped: duplicate fire"}, nil
	}
	// 群必须仍存在且未归档：归档即暂停调度（不报错，避免刷失败记录）。
	tag, err := h.resolveAgentTag(ctx, projectID, bind.TagID)
	if err != nil {
		return agentbridge.GroupCronResult{}, ErrGroupArchived
	}
	threadID, err := h.groupCronThread(ctx, projectID, jobID, bind, tag)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	thread, err := h.store.GetAgentThread(ctx, projectID, threadID)
	if err != nil {
		return agentbridge.GroupCronResult{}, errors.New("schedule thread unavailable")
	}
	ws, err := h.store.GetAgentThreadWorkspace(ctx, projectID, threadID)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	collab, err := h.store.GetAgentThreadCollab(ctx, projectID, threadID)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	// 只读 Headless 主体：与既有 cron agent 口径一致（system-scheduler）。
	principal := auth.Principal{
		APIKeyID:   agentbridge.CronAgentPrincipalID,
		TenantID:   catalog.ReservedTenantID,
		ProjectIDs: map[string]struct{}{projectID: {}},
		Permissions: map[auth.Permission]struct{}{
			auth.DatabaseRead: {},
		},
	}
	tc := turnContext{
		pc:        ProjectContext{ID: projectID, TenantID: catalog.ReservedTenantID},
		principal: principal,
		thread:    thread,
		ws:        ws,
		collab:    collab,
		tag:       tag,
		turn:      systemdb.AgentTurn{},
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
	turn, err := h.runScheduledTurn(ctx, tc, prompt, fireKey)
	if err != nil {
		return agentbridge.GroupCronResult{}, err
	}
	_ = h.store.CompleteCronFire(ctx, projectID, jobID, fireKey, turn.ID, threadID)
	return agentbridge.GroupCronResult{
		TurnID: turn.ID, ThreadID: threadID,
		// 只用状态做摘要：cron 记录不落用户正文。
		Response: "turn " + turn.Status,
	}, nil
}

// groupCronThread 按策略返回本次触发的会话 ID（必要时新建并回写绑定）。
func (h *cloudAgentHandler) groupCronThread(ctx context.Context, projectID, jobID string, bind agentbridge.GroupCronBinding, tag systemdb.AgentTag) (string, error) {
	if bind.ThreadPolicy == systemdb.GroupCronReuseThread {
		if id := strings.TrimSpace(bind.ThreadID); id != "" {
			// 会话仍存在且未归档时直接复用。
			if _, err := h.store.GetAgentThread(ctx, projectID, id); err == nil {
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
	th, err := h.store.CreateAgentThreadWithWorkspace(ctx, systemdb.AgentThread{
		ProjectID: projectID, Title: title, CreatedBy: agentbridge.CronAgentPrincipalID,
	}, systemdb.AgentThreadWorkspace{
		TagID: tag.ID, DispatchMode: systemdb.DispatchCollaborative,
		Source: systemdb.ThreadSourceWorkspace,
	})
	if err != nil {
		return "", err
	}
	if _, err := h.store.CreateAgentThreadCollab(ctx, systemdb.AgentThreadCollab{
		ProjectID: projectID, ThreadID: th.ID, TagID: tag.ID, ThreadKind: kind,
	}); err != nil {
		_ = h.store.ArchiveAgentThread(ctx, projectID, th.ID)
		return "", err
	}
	// reuse_thread 需要把新会话回写到绑定，后续触发才能继续复用。
	if bind.ThreadPolicy == systemdb.GroupCronReuseThread || bind.ThreadPolicy == "" {
		if _, err := h.store.UpsertAgentGroupCron(ctx, systemdb.AgentGroupCron{
			ProjectID: projectID, JobID: jobID, TagID: tag.ID,
			ThreadPolicy: systemdb.GroupCronReuseThread, ThreadKind: kind,
			ThreadID: th.ID, OwnerUserID: bind.OwnerUserID, Prompt: bind.Prompt,
		}); err != nil {
			return "", err
		}
	}
	return th.ID, nil
}

// runScheduledTurn 在后台同步执行一轮（无 SSE、无请求上下文），返回落库后的轮次。
func (h *cloudAgentHandler) runScheduledTurn(ctx context.Context, tc turnContext, prompt, idempotencyKey string) (systemdb.AgentTurn, error) {
	reused, err := h.prepareTurn(ctx, &tc, createTurnBody{Content: prompt, IdempotencyKey: idempotencyKey}, prompt, turnInitiation{
		kind: systemdb.TurnInitiatorCron, id: agentbridge.CronAgentPrincipalID, speakerKind: "scheduler",
		content: "[scheduled] " + prompt,
	})
	if err != nil || reused {
		return tc.turn, err
	}
	advErr := h.advanceTurn(ctx, &tc, nil)
	if advErr != nil && !errors.Is(advErr, errTurnWaitingHuman) {
		h.failTurn(context.Background(), tc.pc.ID, tc.thread.ID, tc.turn.ID, advErr)
	} else {
		h.settleTurn(context.Background(), &tc, nil)
	}
	return h.store.GetAgentTurn(ctx, tc.pc.ID, tc.turn.ID)
}
