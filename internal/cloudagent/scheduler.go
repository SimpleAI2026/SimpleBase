package cloudagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// AgentScheduleTrigger 是触发方式常量。
const (
	TriggerScheduled = "scheduled"
	TriggerManual    = "manual"
)

// schedulerTick 是调度轮询间隔；schedulerConcurrency 是同时执行的定时任务上限。
const (
	schedulerTick        = 30 * time.Second
	schedulerConcurrency = 4
	scheduleHistoryLimit = 40
	schedulerPrincipalID = "system-scheduler"
)

// ScheduleRunner 抽象执行器；*cloudagent.Runtime 天然满足。
type ScheduleRunner interface {
	StartRun(ctx context.Context, req RunRequest, emit func(Event)) (RunResult, error)
}

// ScheduleStore 抽象调度器所需的系统库操作；*systemdb.Store 天然满足。
type ScheduleStore interface {
	GetCloudAgent(ctx context.Context, projectID, id string) (systemdb.CloudAgent, error)
	GetAgentThread(ctx context.Context, projectID, id string) (systemdb.AgentThread, error)
	CreateAgentThread(ctx context.Context, t systemdb.AgentThread) (systemdb.AgentThread, error)
	AppendAgentMessage(ctx context.Context, msg systemdb.AgentMessage) (systemdb.AgentMessage, error)
	ListAgentMessages(ctx context.Context, projectID, threadID string, limit int) ([]systemdb.AgentMessage, error)
	CreateAgentRun(ctx context.Context, r systemdb.AgentRun) (systemdb.AgentRun, error)
	UpdateAgentRunStatus(ctx context.Context, projectID, id, status, errMsg string, started, finished *time.Time) error
	ListDueAgentSchedules(ctx context.Context, now time.Time, limit int) ([]systemdb.AgentSchedule, error)
	ClaimAgentSchedule(ctx context.Context, id string, expectNext, newNext, lastRun time.Time) (bool, error)
	UpdateAgentSchedule(ctx context.Context, sc systemdb.AgentSchedule) (systemdb.AgentSchedule, error)
	InsertAgentScheduleRun(ctx context.Context, r systemdb.AgentScheduleRun) error
	UpdateAgentScheduleRunStatus(ctx context.Context, projectID, id, status, errMsg string, finished *time.Time) error
	RecordLog(ev systemdb.LogEvent)
}

// ScheduleQuota 抽象配额检查；*usage.Service 满足。
type ScheduleQuota interface {
	CheckQuota(ctx context.Context, projectID string, kind string) error
}

// Scheduler 周期扫描到期的 AgentSchedule 并以只读身份执行一次非流式 run。
type Scheduler struct {
	Store  ScheduleStore
	Runner ScheduleRunner
	Quota  ScheduleQuota

	tick time.Duration

	mu      sync.Mutex
	inFlight map[string]struct{}
	wg       sync.WaitGroup
}

// NewScheduler 构造调度器。
func NewScheduler(store ScheduleStore, runner ScheduleRunner, quota ScheduleQuota) *Scheduler {
	return &Scheduler{Store: store, Runner: runner, Quota: quota, tick: schedulerTick, inFlight: map[string]struct{}{}}
}

// Start 启动后台循环；ctx 取消后停止并等待 in-flight 收敛。
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
	due, err := s.Store.ListDueAgentSchedules(ctx, now, 100)
	if err != nil {
		s.log(fmt.Sprintf("list due schedules: %v", err), now)
		return
	}
	for _, sc := range due {
		if s.claimInFlight(sc.ID) {
			s.wg.Add(1)
			go func(sc systemdb.AgentSchedule) {
				defer s.wg.Done()
				defer s.releaseInFlight(sc.ID)
				s.runDue(ctx, sc)
			}(sc)
		}
	}
}

// runDue 认领并执行一条到期 schedule（scheduled 触发）。
func (s *Scheduler) runDue(ctx context.Context, sc systemdb.AgentSchedule) {
	now := time.Now().UTC()
	spec, err := ParseCron(sc.CronExpr)
	if err != nil {
		// 历史数据被改坏：停用并告警，不执行。
		sc.Enabled = false
		_, _ = s.Store.UpdateAgentSchedule(ctx, sc)
		s.log(fmt.Sprintf("disable schedule %s: %v", sc.ID, err), now)
		return
	}
	next, err := spec.NextAfter(now)
	if err != nil {
		sc.Enabled = false
		_, _ = s.Store.UpdateAgentSchedule(ctx, sc)
		s.log(fmt.Sprintf("disable schedule %s: %v", sc.ID, err), now)
		return
	}
	ok, err := s.Store.ClaimAgentSchedule(ctx, sc.ID, sc.NextRunAt, next, now)
	if err != nil || !ok {
		return
	}
	s.execute(ctx, sc, TriggerScheduled)
}

// Trigger 手动立即执行一条 schedule（不改 next_run_at）。返回触发记录 ID。
func (s *Scheduler) Trigger(ctx context.Context, sc systemdb.AgentSchedule) (string, error) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return "", fmt.Errorf("agent scheduler is not configured")
	}
	if !s.claimInFlight(sc.ID) {
		return "", fmt.Errorf("schedule is already executing")
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.releaseInFlight(sc.ID)
		s.execute(ctx, sc, TriggerManual)
	}()
	return "", nil
}

// execute 执行一次定时/手动 run，全程落库；prompt 原文不进日志。
func (s *Scheduler) execute(ctx context.Context, sc systemdb.AgentSchedule, trigger string) {
	now := time.Now().UTC()
	run := systemdb.AgentScheduleRun{
		ScheduleID: sc.ID, ProjectID: sc.ProjectID, AgentID: sc.AgentID, ThreadID: sc.ThreadID,
		Trigger: trigger, Status: systemdb.AgentRunRunning, StartedAt: now,
	}
	if err := s.Store.InsertAgentScheduleRun(ctx, run); err != nil {
		s.log(fmt.Sprintf("insert schedule run: %v", err), now)
		return
	}

	finish := func(status, errMsg string) {
		fin := time.Now().UTC()
		_ = s.Store.UpdateAgentScheduleRunStatus(ctx, sc.ProjectID, run.ID, status, errMsg, &fin)
		s.log(fmt.Sprintf("agent schedule executed: schedule_id=%s trigger=%s status=%s", sc.ID, trigger, status), fin)
	}

	if s.Quota != nil {
		if err := s.Quota.CheckQuota(ctx, sc.ProjectID, "llm"); err != nil {
			finish(systemdb.AgentRunFailed, "llm quota exceeded")
			return
		}
	}

	agent, err := s.Store.GetCloudAgent(ctx, sc.ProjectID, sc.AgentID)
	if err != nil {
		finish(systemdb.AgentRunFailed, "agent not found")
		s.disableSchedule(ctx, sc)
		return
	}

	thread, err := s.Store.GetAgentThread(ctx, sc.ProjectID, sc.ThreadID)
	if err != nil {
		// 专属 thread 被归档：新建并回写 schedule。
		created, cerr := s.Store.CreateAgentThread(ctx, systemdb.AgentThread{
			ProjectID: sc.ProjectID, Title: "Scheduled: " + agent.Name, CreatedBy: schedulerPrincipalID,
		})
		if cerr != nil {
			finish(systemdb.AgentRunFailed, "schedule thread unavailable")
			return
		}
		thread = created
		sc.ThreadID = created.ID
		if _, uerr := s.Store.UpdateAgentSchedule(ctx, sc); uerr != nil {
			s.log(fmt.Sprintf("update schedule thread: %v", uerr), time.Now().UTC())
		}
	}

	principal := auth.Principal{
		APIKeyID:   schedulerPrincipalID,
		ProjectIDs: map[string]struct{}{sc.ProjectID: {}},
		Permissions: map[auth.Permission]struct{}{
			auth.DatabaseRead: {},
		},
	}

	agentRun, err := s.Store.CreateAgentRun(ctx, systemdb.AgentRun{
		ThreadID: thread.ID, ProjectID: sc.ProjectID, AgentID: agent.ID,
		Status: systemdb.AgentRunRunning, StartedAt: now,
	})
	if err != nil {
		finish(systemdb.AgentRunFailed, "create agent run failed")
		return
	}
	userMsg, err := s.Store.AppendAgentMessage(ctx, systemdb.AgentMessage{
		ThreadID: thread.ID, ProjectID: sc.ProjectID, Role: "user",
		Content: "[scheduled] " + sc.Prompt, AgentID: agent.ID,
		MentionsJSON: `[{"agent_id":"` + agent.ID + `"}]`, RunID: agentRun.ID,
	})
	if err != nil {
		finish(systemdb.AgentRunFailed, "append user message failed")
		return
	}
	hist, err := s.Store.ListAgentMessages(ctx, sc.ProjectID, thread.ID, scheduleHistoryLimit)
	if err == nil && len(hist) > 0 && hist[len(hist)-1].ID == userMsg.ID {
		hist = hist[:len(hist)-1]
	}

	res, err := s.Runner.StartRun(ctx, RunRequest{
		ProjectID: sc.ProjectID, Principal: principal, Agent: agent,
		ThreadID: thread.ID, RunID: agentRun.ID, UserText: sc.Prompt,
		History: hist, Stream: false,
	}, nil)
	fin := time.Now().UTC()
	if err != nil {
		status := systemdb.AgentRunFailed
		if strings.Contains(err.Error(), "context canceled") {
			status = systemdb.AgentRunCanceled
		}
		_ = s.Store.UpdateAgentRunStatus(ctx, sc.ProjectID, agentRun.ID, status, sanitizeErr(err), nil, &fin)
		finish(status, sanitizeErr(err))
		return
	}
	_, _ = s.Store.AppendAgentMessage(ctx, systemdb.AgentMessage{
		ThreadID: thread.ID, ProjectID: sc.ProjectID, Role: "assistant", Content: res.Content,
		AgentID: agent.ID, ToolCallsJSON: res.ToolCallsJSON, RunID: agentRun.ID,
	})
	_ = s.Store.UpdateAgentRunStatus(ctx, sc.ProjectID, agentRun.ID, systemdb.AgentRunCompleted, "", nil, &fin)
	finish(systemdb.AgentRunCompleted, "")
}

func (s *Scheduler) disableSchedule(ctx context.Context, sc systemdb.AgentSchedule) {
	sc.Enabled = false
	if _, err := s.Store.UpdateAgentSchedule(ctx, sc); err != nil {
		s.log(fmt.Sprintf("disable schedule %s: %v", sc.ID, err), time.Now().UTC())
	}
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
		Level: "info", Logger: "scheduler", Message: message,
		FieldsJSON: `{}`, OccurredAt: at,
	})
}

// sanitizeErr 裁剪错误信息，避免把内部细节整段落库。
func sanitizeErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}
