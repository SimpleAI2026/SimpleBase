// 文件说明：cron_agent.go — 把一条 target_kind=agent 的定时任务执行成一次 Cloud Agent run
//
// 用法
//
//	NewCronAgentExecutor(store CronAgentStore, runner CronAgentRunner, quota ...
//	(e *CronAgentExecutor) RunAgent(ctx context.Context, j systemdb.CronJob, trigger ...  // RunAgent 执行一次 agent 定时任务
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/cron_agent.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, internal/auth, internal/cronjob, internal/systemdb 等
//	[同包] cli.go, command.go, delegation.go, errors.go, principal.go 等 6 个
//
// 主要符号
//
//	const CronAgentPrincipalID
//	const CronAgentPromptLimit
//	type CronAgentRunner interface {…}
//	type CronAgentStore interface {…}
//	type CronAgentQuota interface {…}
//	type CronAgentExecutor struct {…}
//	... 另有 10 个顶层符号

package agentbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

const (
	// CronAgentPrincipalID 是定时任务执行 Agent 时使用的只读系统身份。
	CronAgentPrincipalID = "system-scheduler"
	// cronAgentHistoryLimit 是带入 run 的历史消息条数上限。
	cronAgentHistoryLimit = 40
	// cronAgentErrorLimit 是落库错误文本的截断长度。
	cronAgentErrorLimit = 300
	// CronAgentPromptLimit 是 agent 定时任务提示词的字节上限（16KiB）。API 层按它校验。
	CronAgentPromptLimit = 16 * 1024
)

// CronAgentRunner 抽象一次 Agent run；*gocloudagent.Runtime 天然满足。
type CronAgentRunner interface {
	StartRun(ctx context.Context, req gocloudagent.RunRequest, emit func(gocloudagent.Event)) (gocloudagent.RunResult, error)
}

// CronAgentStore 是本类型需要的系统库操作；*systemdb.Store 天然满足。
type CronAgentStore interface {
	GetCloudAgent(ctx context.Context, projectID, id string) (systemdb.CloudAgent, error)
	GetAgentThread(ctx context.Context, projectID, id string) (systemdb.AgentThread, error)
	CreateAgentThread(ctx context.Context, t systemdb.AgentThread) (systemdb.AgentThread, error)
	AppendAgentMessage(ctx context.Context, msg systemdb.AgentMessage) (systemdb.AgentMessage, error)
	ListAgentMessages(ctx context.Context, projectID, threadID string, limit int) ([]systemdb.AgentMessage, error)
	CreateAgentRun(ctx context.Context, r systemdb.AgentRun) (systemdb.AgentRun, error)
	UpdateAgentRunStatus(ctx context.Context, projectID, id, status, errMsg string, started, finished *time.Time) error
	// 回写任务：thread 被归档后重建，或 agent 不存在时停用任务。
	GetCronJob(ctx context.Context, projectID, id string) (systemdb.CronJob, error)
	UpdateCronJob(ctx context.Context, j systemdb.CronJob) (systemdb.CronJob, error)
	RecordLog(ev systemdb.LogEvent)
}

// cronAgentMetricsWriter 是 CronAgentStore 的可选扩展：真实 Store 支持时同步记录 run 指标。
type cronAgentMetricsWriter interface {
	UpdateAgentRunMetrics(ctx context.Context, projectID, runID string, m systemdb.AgentRun) error
}

// CronAgentQuota 抽象配额检查；*usage.Service 满足。
type CronAgentQuota interface {
	CheckQuota(ctx context.Context, projectID string, kind string) error
}

// CronAgentExecutor 实现 cronjob.AgentRunner。
type CronAgentExecutor struct {
	Store  CronAgentStore
	Runner CronAgentRunner
	Quota  CronAgentQuota // 可为 nil
}

// NewCronAgentExecutor 构造执行器。
func NewCronAgentExecutor(store CronAgentStore, runner CronAgentRunner, quota CronAgentQuota) *CronAgentExecutor {
	return &CronAgentExecutor{Store: store, Runner: runner, Quota: quota}
}

var _ cronjob.AgentRunner = (*CronAgentExecutor)(nil)

// RunAgent 执行一次 agent 定时任务。失败返回带稳定短文本的 error（不含密钥/LLM 正文），
// 由调度器写入 sys_cron_job_runs.error 与任务快照。prompt 原文不进日志。
func (e *CronAgentExecutor) RunAgent(ctx context.Context, j systemdb.CronJob, trigger string) (cronjob.AgentResult, error) {
	if e == nil || e.Store == nil || e.Runner == nil {
		return cronjob.AgentResult{}, errors.New("agent executor is not configured")
	}
	now := time.Now().UTC()

	if e.Quota != nil {
		if err := e.Quota.CheckQuota(ctx, j.ProjectID, "llm"); err != nil {
			return cronjob.AgentResult{}, errors.New("llm quota exceeded")
		}
	}

	agent, err := e.Store.GetCloudAgent(ctx, j.ProjectID, j.AgentID)
	if err != nil {
		e.disableJob(ctx, j)
		return cronjob.AgentResult{}, errors.New("agent not found")
	}
	// 定时任务同样按助手 module 加载技能：无人值守运行没有 `@` 点名。
	skills := gocloudagent.SkillsForModule(agent.Module)

	threadID := j.AgentThreadID
	thread, err := e.Store.GetAgentThread(ctx, j.ProjectID, threadID)
	if err != nil {
		// 专属 thread 不存在或被归档：新建并回写任务。
		created, cerr := e.Store.CreateAgentThread(ctx, systemdb.AgentThread{
			ProjectID: j.ProjectID, AgentID: agent.ID, Title: "Scheduled: " + agent.Name, CreatedBy: CronAgentPrincipalID,
		})
		if cerr != nil {
			return cronjob.AgentResult{}, errors.New("schedule thread unavailable")
		}
		thread = created
		e.rebindThread(ctx, j, created.ID)
	}
	if thread.AgentID != "" && thread.AgentID != agent.ID {
		return cronjob.AgentResult{}, errors.New("schedule thread belongs to another agent")
	}

	principal := auth.Principal{
		APIKeyID:   CronAgentPrincipalID,
		ProjectIDs: map[string]struct{}{j.ProjectID: {}},
		Permissions: map[auth.Permission]struct{}{
			auth.DatabaseRead: {},
		},
	}

	agentRun, err := e.Store.CreateAgentRun(ctx, systemdb.AgentRun{
		ThreadID: thread.ID, ProjectID: j.ProjectID, AgentID: agent.ID,
		Status: systemdb.AgentRunRunning, StartedAt: now,
	})
	if err != nil {
		return cronjob.AgentResult{}, errors.New("create agent run failed")
	}
	userMsg, err := e.Store.AppendAgentMessage(ctx, systemdb.AgentMessage{
		ThreadID: thread.ID, ProjectID: j.ProjectID, Role: "user",
		Content: "[scheduled] " + j.AgentPrompt, AgentID: agent.ID,
		MentionsJSON: `[{"agent_id":"` + agent.ID + `"}]`, RunID: agentRun.ID,
	})
	if err != nil {
		return cronjob.AgentResult{}, errors.New("append user message failed")
	}
	hist, err := e.Store.ListAgentMessages(ctx, j.ProjectID, thread.ID, cronAgentHistoryLimit)
	if err == nil && len(hist) > 0 && hist[len(hist)-1].ID == userMsg.ID {
		hist = hist[:len(hist)-1]
	}

	res, runErr := e.Runner.StartRun(ctx, gocloudagent.RunRequest{
		ProjectID: j.ProjectID, Principal: ToModulePrincipal(principal), Agent: Agent(agent), Skills: skills,
		ThreadID: thread.ID, RunID: agentRun.ID, UserText: j.AgentPrompt,
		History: Messages(hist), Stream: false, Headless: true,
	}, nil)
	fin := time.Now().UTC()

	// 旧 Store fake 无指标写入方法，真实 Store 支持时同步记录。
	if mw, ok := e.Store.(cronAgentMetricsWriter); ok {
		code := ""
		if runErr != nil {
			code = ClassifyError(runErr).Code
		}
		_ = mw.UpdateAgentRunMetrics(ctx, j.ProjectID, agentRun.ID, systemdb.AgentRun{
			DurationMS: res.DurationMS, PromptTokens: res.PromptTokens, CompletionTokens: res.CompletionTokens,
			ReasoningTokens: res.ReasoningTokens, ToolCalls: res.ToolCalls, ErrorCode: code,
		})
	}

	if runErr != nil {
		status := systemdb.AgentRunFailed
		if strings.Contains(runErr.Error(), "context canceled") {
			status = systemdb.AgentRunCanceled
		}
		msg := cronAgentErr(runErr)
		_ = e.Store.UpdateAgentRunStatus(ctx, j.ProjectID, agentRun.ID, status, msg, nil, &fin)
		return cronjob.AgentResult{}, errors.New(msg)
	}
	_, _ = e.Store.AppendAgentMessage(ctx, systemdb.AgentMessage{
		ThreadID: thread.ID, ProjectID: j.ProjectID, Role: "assistant", Content: res.Content,
		AgentID: agent.ID, ToolCallsJSON: res.ToolCallsJSON, RunID: agentRun.ID,
	})
	_ = e.Store.UpdateAgentRunStatus(ctx, j.ProjectID, agentRun.ID, systemdb.AgentRunCompleted, "", nil, &fin)
	return cronjob.AgentResult{Response: res.Content}, nil
}

// rebindThread 把重建后的 thread 回写到任务。先重取再合并，避免用旧行覆盖调度器
// 刚推进的 next_run_at / last_run_at。
func (e *CronAgentExecutor) rebindThread(ctx context.Context, j systemdb.CronJob, threadID string) {
	fresh, err := e.Store.GetCronJob(ctx, j.ProjectID, j.ID)
	if err != nil {
		fresh = j
	}
	fresh.AgentThreadID = threadID
	if _, err := e.Store.UpdateCronJob(ctx, fresh); err != nil {
		e.log(fmt.Sprintf("update cron job thread: %v", err))
	}
}

// disableJob 在 agent 已不存在时停用任务（避免每轮都空跑失败）。
func (e *CronAgentExecutor) disableJob(ctx context.Context, j systemdb.CronJob) {
	fresh, err := e.Store.GetCronJob(ctx, j.ProjectID, j.ID)
	if err != nil {
		fresh = j
	}
	fresh.Enabled = false
	fresh.NextRunAt = time.Time{}
	if _, err := e.Store.UpdateCronJob(ctx, fresh); err != nil {
		e.log(fmt.Sprintf("disable cron job %s: %v", j.ID, err))
	}
}

func (e *CronAgentExecutor) log(message string) {
	if e.Store == nil {
		return
	}
	e.Store.RecordLog(systemdb.LogEvent{
		// Logger 保持 "agentjob"：系统库 logs 表按该值检索，改包名不应改变检索口径。
		Level: "info", Logger: "agentjob", Message: message,
		FieldsJSON: `{}`, OccurredAt: time.Now().UTC(),
	})
}

// cronAgentErr 裁剪错误信息，避免把内部细节整段落库。
func cronAgentErr(err error) string {
	if err == nil {
		return ""
	}
	return strutil.Truncate(err.Error(), cronAgentErrorLimit)
}
