// 文件说明：scheduler.go — 定时任务调度（ui-cronjob-plan §5.2）：tick 扫描 + CAS 认领 + 并发上限
//
// 用法
//
//	NewScheduler(store Store, runner Runner) *Scheduler
//	(s *Scheduler) WithAgentRunner(a AgentRunner) *Scheduler  // WithAgentRunner 挂接 Agent 执行器（target_kind=agent），返回自身便于链式装配
//	(s *Scheduler) Start(ctx context.Context)  // Start 启动后台循环
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/cronjob/scheduler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/systemdb, internal/utils/crontab, internal/utils/strutil
//
// 主要符号
//
//	const TriggerScheduled
//	const TriggerManual
//	type Runner interface {…}
//	type AgentResult struct {…}
//	type AgentRunner interface {…}
//	type Store interface {…}
//	... 另有 21 个顶层符号

package cronjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/utils/crontab"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

// Trigger 来源常量。
const (
	TriggerScheduled = "scheduled"
	TriggerManual    = "manual"
)

// schedulerTick 是调度轮询间隔；schedulerConcurrency 是同时执行的任务上限。
const (
	schedulerTick        = 30 * time.Second
	schedulerConcurrency = 4
	runErrorLimit        = 300
	runResponseLimit     = 4 * 1024 // response_json 截断（§4.2）
)

// Runner 抽象一次云函数执行；由 api 层注入（实现调 gofunction.RunJSON）。
type Runner interface {
	RunFunction(ctx context.Context, projectID, funcFile, funcExport string, input json.RawMessage) ([]byte, error)
}

// AgentResult 是一次 Agent 定时执行的回显摘要（调度器负责截断后落 response_json）。
type AgentResult struct {
	Response string
}

// AgentRunner 抽象一次 Agent 定时执行（target_kind=agent）；由 internal/agentbridge 实现。
// 实现方负责 Agent 侧的全部落库（会话/消息/run/指标）；调度器只记录
// sys_cron_job_runs 与任务快照。错误文本不得包含密钥或 LLM 正文。
type AgentRunner interface {
	RunAgent(ctx context.Context, j systemdb.CronJob, trigger string) (AgentResult, error)
}

// Store 抽象调度器所需的系统库操作；*systemdb.Store 天然满足。
type Store interface {
	ListDueCronJobs(ctx context.Context, now time.Time, limit int) ([]systemdb.CronJob, error)
	ClaimCronJob(ctx context.Context, id string, expectNext, newNext, lastRun time.Time) (bool, error)
	GetCronJob(ctx context.Context, projectID, id string) (systemdb.CronJob, error)
	UpdateCronJob(ctx context.Context, j systemdb.CronJob) (systemdb.CronJob, error)
	InsertCronJobRun(ctx context.Context, r systemdb.CronJobRun) (systemdb.CronJobRun, error)
	UpdateCronJobRunStatus(ctx context.Context, projectID, id, status, errMsg string, durationMs int64, response string, finished *time.Time) error
	RecordLog(ev systemdb.LogEvent)
}

// ErrSchedulerStopped 表示调度器已停止，拒绝新的触发（LIFE-02）。
var ErrSchedulerStopped = errors.New("cron scheduler stopped")

// Scheduler 周期扫描到期的 CronJob 并执行一次云函数调用。
type Scheduler struct {
	Store  Store
	Runner Runner
	// Agent 为 nil 时 target_kind=agent 的任务执行失败（不静默降级成云函数）。
	Agent AgentRunner

	tick time.Duration

	mu       sync.Mutex
	inFlight map[string]struct{}
	stopped  bool            // LIFE-02：停止后拒绝新的 Trigger / wg.Add
	sctx     context.Context // LIFE-01：调度器自己的 ctx，手动触发与请求解耦
	wg       sync.WaitGroup
}

// NewScheduler 构造调度器。Agent 目标通过 WithAgentRunner 追加。
func NewScheduler(store Store, runner Runner) *Scheduler {
	return &Scheduler{Store: store, Runner: runner, tick: schedulerTick, inFlight: map[string]struct{}{}}
}

// WithAgentRunner 挂接 Agent 执行器（target_kind=agent），返回自身便于链式装配。
func (s *Scheduler) WithAgentRunner(a AgentRunner) *Scheduler {
	if s != nil {
		s.Agent = a
	}
	return s
}

// Start 启动后台循环；ctx 取消后停止。
// ctx 同时作为手动触发任务的执行 ctx（LIFE-01）：
// 手动任务与请求生命周期解耦，但受调度器停止（Shutdown）控制。
func (s *Scheduler) Start(ctx context.Context) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.sctx = ctx
	s.mu.Unlock()
	interval := s.tick
	if interval <= 0 {
		interval = schedulerTick
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tickOnce(ctx)
			}
		}
	}()
}

// Stop 停止接收新的触发并等待后台循环与 in-flight 执行结束。
// 手动任务通过调度器 ctx 取消（LIFE-01）：调用方应取消传给 Start 的 ctx
// （App.Shutdown 已这样做），Stop 随即有界返回。
func (s *Scheduler) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Scheduler) tickOnce(ctx context.Context) {
	now := time.Now().UTC()
	due, err := s.Store.ListDueCronJobs(ctx, now, 100)
	if err != nil {
		s.log(fmt.Sprintf("list due cron jobs: %v", err), now)
		return
	}
	for _, j := range due {
		if s.claimInFlight(j.ID) {
			s.wg.Add(1)
			go func(j systemdb.CronJob) {
				defer s.wg.Done()
				defer s.releaseInFlight(j.ID)
				s.runDue(ctx, j)
			}(j)
		}
	}
}

// runDue 认领并执行一条到期任务（scheduled 触发）。
func (s *Scheduler) runDue(ctx context.Context, j systemdb.CronJob) {
	now := time.Now().UTC()
	next, err := s.nextRun(j, now)
	if err != nil {
		// cron 表达式被改坏（历史脏数据）：停用并告警，不执行。
		s.disableJob(ctx, j, fmt.Sprintf("disable cron job %s: %v", j.ID, err))
		return
	}
	ok, err := s.Store.ClaimCronJob(ctx, j.ID, j.NextRunAt, next, now)
	if err != nil || !ok {
		return
	}
	s.execute(ctx, j, TriggerScheduled)
	// once：执行后自动停用，不再排期。
	if j.ScheduleKind == systemdb.CronJobKindOnce {
		s.disableJob(ctx, j, "disable one-shot cron job "+j.ID)
	}
}

// nextRun 计算下一次触发：cron 用 NextAfter；interval 以认领时刻为基准（防漂移，§3）；
// once 执行后不再排期（返回零值并由 runDue 停用）。
func (s *Scheduler) nextRun(j systemdb.CronJob, now time.Time) (time.Time, error) {
	switch j.ScheduleKind {
	case systemdb.CronJobKindCron:
		spec, err := crontab.ParseCron(j.CronExpr)
		if err != nil {
			return time.Time{}, err
		}
		return spec.NextAfter(now)
	case systemdb.CronJobKindOnce:
		return time.Time{}, nil
	default:
		if j.IntervalSeconds <= 0 {
			return time.Time{}, fmt.Errorf("invalid interval_seconds %d", j.IntervalSeconds)
		}
		return now.Add(time.Duration(j.IntervalSeconds) * time.Second), nil
	}
}

// Trigger 手动立即执行一条任务（不改 next_run_at）。异步执行，立即返回。
// LIFE-01：忽略调用方 ctx，改用调度器 ctx —— 手动任务不再受请求断开影响，
// 但受调度器停止控制；调度器已停止时返回 ErrSchedulerStopped（LIFE-02）。
func (s *Scheduler) Trigger(_ context.Context, j systemdb.CronJob) error {
	if s == nil || s.Store == nil || s.Runner == nil {
		return fmt.Errorf("cron scheduler is not configured")
	}
	// stopped 判定与 wg.Add 必须在同一把锁内（LIFE-02），
	// 否则 Stop 中的 wg.Wait 与新的 wg.Add 竞争，可能 panic 或漏等任务。
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return ErrSchedulerStopped
	}
	if !s.claimInFlightLocked(j.ID) {
		s.mu.Unlock()
		return fmt.Errorf("cron job is already executing")
	}
	ctx := s.sctx
	if ctx == nil {
		ctx = context.Background()
	}
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer s.releaseInFlight(j.ID)
		s.execute(ctx, j, TriggerManual)
	}()
	return nil
}

// execute 执行一次定时/手动调用，全程落库（§5.2）。
func (s *Scheduler) execute(ctx context.Context, j systemdb.CronJob, trigger string) {
	now := time.Now().UTC()
	run := systemdb.CronJobRun{
		JobID: j.ID, ProjectID: j.ProjectID, Trigger: trigger,
		Status: systemdb.CronJobRunRunning, StartedAt: now,
	}
	run, err := s.Store.InsertCronJobRun(ctx, run)
	if err != nil {
		s.log(fmt.Sprintf("insert cron job run: %v", err), now)
		return
	}

	start := time.Now()
	var out []byte
	var runErr error
	switch j.EffectiveTargetKind() {
	case systemdb.CronJobTargetAgent:
		if s.Agent == nil {
			runErr = errors.New("agent runner is not configured")
		} else {
			var res AgentResult
			res, runErr = s.Agent.RunAgent(ctx, j, trigger)
			out = []byte(res.Response)
		}
	default:
		out, runErr = s.Runner.RunFunction(ctx, j.ProjectID, j.FuncFile, j.FuncExport, json.RawMessage(j.InputJSON))
	}
	duration := time.Since(start).Milliseconds()

	fin := time.Now().UTC()
	if runErr != nil {
		status := systemdb.CronJobRunFailed
		if strings.Contains(runErr.Error(), "context canceled") {
			status = systemdb.CronJobRunCanceled
		}
		_ = s.Store.UpdateCronJobRunStatus(ctx, j.ProjectID, run.ID, status, strutil.Truncate(runErr.Error(), runErrorLimit), duration, "", &fin)
		s.updateJobSnapshot(ctx, j, status, strutil.Truncate(runErr.Error(), runErrorLimit))
		s.log(fmt.Sprintf("cron job executed: job=%s trigger=%s status=%s", j.ID, trigger, status), fin)
		return
	}
	_ = s.Store.UpdateCronJobRunStatus(ctx, j.ProjectID, run.ID, systemdb.CronJobRunCompleted, "", duration, strutil.Truncate(string(out), runResponseLimit), &fin)
	s.updateJobSnapshot(ctx, j, systemdb.CronJobRunCompleted, "")
	s.log(fmt.Sprintf("cron job executed: job=%s trigger=%s status=completed duration_ms=%d", j.ID, trigger, duration), fin)
}

// updateJobSnapshot 回写任务快照：last_status / last_error / run_count+1。
// 先重取最新行再合并，避免用执行前的旧行覆盖 ClaimCronJob 刚推进的 next_run_at / last_run_at。
func (s *Scheduler) updateJobSnapshot(ctx context.Context, j systemdb.CronJob, status, errMsg string) {
	now := time.Now().UTC()
	fresh, err := s.Store.GetCronJob(ctx, j.ProjectID, j.ID)
	if err != nil {
		// 重取失败退回旧行，至少保证快照落库。
		fresh = j
	}
	fresh.LastStatus = status
	fresh.LastError = errMsg
	fresh.RunCount++
	if _, err := s.Store.UpdateCronJob(ctx, fresh); err != nil {
		s.log(fmt.Sprintf("update cron job snapshot: %v", err), now)
	}
}

func (s *Scheduler) disableJob(ctx context.Context, j systemdb.CronJob, reason string) {
	j.Enabled = false
	if _, err := s.Store.UpdateCronJob(ctx, j); err != nil {
		s.log(fmt.Sprintf("%s (update failed: %v)", reason, err), time.Now().UTC())
		return
	}
	s.log(reason, time.Now().UTC())
}

func (s *Scheduler) claimInFlight(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claimInFlightLocked(id)
}

// claimInFlightLocked 要求调用方已持有 s.mu（Trigger 在同一锁窗口内判定 stopped）。
func (s *Scheduler) claimInFlightLocked(id string) bool {
	if len(s.inFlight) >= schedulerConcurrency {
		return false
	}
	if _, ok := s.inFlight[id]; ok {
		return false
	}
	if s.inFlight == nil {
		s.inFlight = map[string]struct{}{}
	}
	s.inFlight[id] = struct{}{}
	return true
}

func (s *Scheduler) releaseInFlight(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, id)
}

func (s *Scheduler) log(message string, at time.Time) {
	if s.Store == nil {
		return
	}
	s.Store.RecordLog(systemdb.LogEvent{
		Level: "info", Logger: "cronjob", Message: message,
		FieldsJSON: `{}`, OccurredAt: at,
	})
}
