// Package cronjob 实现云函数定时调用（ui-cronjob-plan §5.2）。
// 调度范式镜像 internal/cloudagent/scheduler.go：tick 扫描 + CAS 认领 + 并发上限；
// 执行器通过 Runner 接口注入，本包不直接依赖 gofunction 内核。
package cronjob

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/crontab"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
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

// Scheduler 周期扫描到期的 CronJob 并执行一次云函数调用。
type Scheduler struct {
	Store  Store
	Runner Runner

	tick time.Duration

	mu       sync.Mutex
	inFlight map[string]struct{}
	wg       sync.WaitGroup
}

// NewScheduler 构造调度器。
func NewScheduler(store Store, runner Runner) *Scheduler {
	return &Scheduler{Store: store, Runner: runner, tick: schedulerTick, inFlight: map[string]struct{}{}}
}

// Start 启动后台循环；ctx 取消后停止。
func (s *Scheduler) Start(ctx context.Context) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return
	}
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

// Stop 等待后台循环与 in-flight 执行结束。
func (s *Scheduler) Stop() {
	if s == nil {
		return
	}
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
func (s *Scheduler) Trigger(ctx context.Context, j systemdb.CronJob) error {
	if s == nil || s.Store == nil || s.Runner == nil {
		return fmt.Errorf("cron scheduler is not configured")
	}
	if !s.claimInFlight(j.ID) {
		return fmt.Errorf("cron job is already executing")
	}
	s.wg.Add(1)
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
	out, runErr := s.Runner.RunFunction(ctx, j.ProjectID, j.FuncFile, j.FuncExport, json.RawMessage(j.InputJSON))
	duration := time.Since(start).Milliseconds()

	fin := time.Now().UTC()
	if runErr != nil {
		status := systemdb.CronJobRunFailed
		if strings.Contains(runErr.Error(), "context canceled") {
			status = systemdb.CronJobRunCanceled
		}
		_ = s.Store.UpdateCronJobRunStatus(ctx, j.ProjectID, run.ID, status, truncate(runErr.Error(), runErrorLimit), duration, "", &fin)
		s.updateJobSnapshot(ctx, j, status, truncate(runErr.Error(), runErrorLimit))
		s.log(fmt.Sprintf("cron job executed: job=%s trigger=%s status=%s", j.ID, trigger, status), fin)
		return
	}
	_ = s.Store.UpdateCronJobRunStatus(ctx, j.ProjectID, run.ID, systemdb.CronJobRunCompleted, "", duration, truncate(string(out), runResponseLimit), &fin)
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

// truncate 裁剪字符串，避免大返回值整段落库。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
