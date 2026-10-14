package cronjob

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// fakeAgentRunner 记录调用并可注入错误/输出。
type fakeAgentRunner struct {
	mu       sync.Mutex
	jobs     []systemdb.CronJob
	triggers []string
	err      error
	response string
}

func (f *fakeAgentRunner) RunAgent(_ context.Context, j systemdb.CronJob, trigger string) (AgentResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs = append(f.jobs, j)
	f.triggers = append(f.triggers, trigger)
	if f.err != nil {
		return AgentResult{}, f.err
	}
	return AgentResult{Response: f.response}, nil
}

func (f *fakeAgentRunner) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

func agentJob(id string) systemdb.CronJob {
	return systemdb.CronJob{
		ID: id, ProjectID: "p1", Name: "digest", ScheduleKind: systemdb.CronJobKindInterval,
		IntervalSeconds: 600, TargetKind: systemdb.CronJobTargetAgent,
		AgentID: "a1", AgentPrompt: "summarize", Enabled: true,
		NextRunAt: time.Now().UTC().Add(-time.Minute),
	}
}

// target_kind=agent 的到期任务走 AgentRunner，不触碰云函数 Runner；
// 排期推进、run 与快照照常落库，回显进 response_json。
func TestScheduler_AgentTargetDispatch(t *testing.T) {
	fs := newFakeStore(agentJob("job-a"))
	fn := &fakeRunner{}
	ag := &fakeAgentRunner{response: "all good"}
	s := NewScheduler(fs, fn).WithAgentRunner(ag)

	s.tickOnce(context.Background())
	s.Stop()

	if ag.callCount() != 1 {
		t.Fatalf("agent calls = %d, want 1", ag.callCount())
	}
	if fn.callCount() != 0 {
		t.Fatal("function runner must not be called for agent targets")
	}
	if ag.triggers[0] != TriggerScheduled {
		t.Fatalf("trigger = %q", ag.triggers[0])
	}
	got := fs.job("job-a")
	if got.RunCount != 1 || got.LastStatus != systemdb.CronJobRunCompleted {
		t.Fatalf("snapshot not updated: %+v", got)
	}
	if !got.NextRunAt.After(got.LastRunAt) {
		t.Fatalf("next_run_at not advanced: next=%v last=%v", got.NextRunAt, got.LastRunAt)
	}
	runs := fs.runList()
	if len(runs) != 1 || runs[0].Status != systemdb.CronJobRunCompleted || runs[0].ResponseJSON != "all good" {
		t.Fatalf("unexpected runs: %+v", runs)
	}
}

// target_kind 为空（迁移前的历史行）仍走云函数。
func TestScheduler_EmptyTargetKindStaysFunction(t *testing.T) {
	j := systemdb.CronJob{
		ID: "job-f", ProjectID: "p1", Name: "f", ScheduleKind: systemdb.CronJobKindInterval,
		IntervalSeconds: 600, FuncFile: "hello", FuncExport: "Hello", InputJSON: `{}`,
		Enabled: true, NextRunAt: time.Now().UTC().Add(-time.Minute),
	}
	fs := newFakeStore(j)
	fn := &fakeRunner{}
	ag := &fakeAgentRunner{}
	s := NewScheduler(fs, fn).WithAgentRunner(ag)

	s.tickOnce(context.Background())
	s.Stop()

	if fn.callCount() != 1 || ag.callCount() != 0 {
		t.Fatalf("fn=%d agent=%d, want 1/0", fn.callCount(), ag.callCount())
	}
}

// 未挂载 AgentRunner 时 agent 任务明确失败，而不是静默当作云函数执行。
func TestScheduler_AgentTargetWithoutRunnerFails(t *testing.T) {
	fs := newFakeStore(agentJob("job-a"))
	fn := &fakeRunner{}
	s := NewScheduler(fs, fn) // 无 WithAgentRunner

	s.tickOnce(context.Background())
	s.Stop()

	if fn.callCount() != 0 {
		t.Fatal("agent target must never fall back to the function runner")
	}
	runs := fs.runList()
	if len(runs) != 1 || runs[0].Status != systemdb.CronJobRunFailed || !strings.Contains(runs[0].Error, "not configured") {
		t.Fatalf("unexpected runs: %+v", runs)
	}
	if got := fs.job("job-a"); got.LastStatus != systemdb.CronJobRunFailed || got.RunCount != 1 {
		t.Fatalf("snapshot: %+v", got)
	}
}

// AgentRunner 失败：run 落 failed 并带截断错误；context canceled 落 canceled。
func TestScheduler_AgentTargetFailureStatuses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status string
	}{
		{"failed", errors.New("llm quota exceeded"), systemdb.CronJobRunFailed},
		{"canceled", errors.New("context canceled"), systemdb.CronJobRunCanceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFakeStore(agentJob("job-a"))
			s := NewScheduler(fs, &fakeRunner{}).WithAgentRunner(&fakeAgentRunner{err: tc.err})
			s.tickOnce(context.Background())
			s.Stop()
			runs := fs.runList()
			if len(runs) != 1 || runs[0].Status != tc.status || runs[0].Error != tc.err.Error() {
				t.Fatalf("unexpected runs: %+v", runs)
			}
			if fs.job("job-a").LastStatus != tc.status {
				t.Fatalf("snapshot status = %q", fs.job("job-a").LastStatus)
			}
		})
	}
}

// 手动触发 agent 任务：trigger=manual，不改 next_run_at。
func TestScheduler_AgentTargetManualTrigger(t *testing.T) {
	j := agentJob("job-a")
	fs := newFakeStore(j)
	ag := &fakeAgentRunner{response: "ok"}
	s := NewScheduler(fs, &fakeRunner{}).WithAgentRunner(ag)
	// Start 的后台循环只在 ctx 取消时退出，必须先 cancel 再 Stop，否则 wg.Wait 永不返回。
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	defer func() {
		cancel()
		s.Stop()
	}()

	if err := s.Trigger(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for ag.callCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if ag.callCount() != 1 || ag.triggers[0] != TriggerManual {
		t.Fatalf("calls=%d triggers=%v", ag.callCount(), ag.triggers)
	}
	if !fs.job("job-a").NextRunAt.Equal(j.NextRunAt) {
		t.Fatal("manual trigger must not move next_run_at")
	}
}

// 超长回显在落库前被截断到 runResponseLimit。
func TestScheduler_AgentResponseTruncated(t *testing.T) {
	fs := newFakeStore(agentJob("job-a"))
	ag := &fakeAgentRunner{response: strings.Repeat("z", runResponseLimit+50)}
	s := NewScheduler(fs, &fakeRunner{}).WithAgentRunner(ag)
	s.tickOnce(context.Background())
	s.Stop()
	runs := fs.runList()
	if len(runs) != 1 || len(runs[0].ResponseJSON) != runResponseLimit {
		t.Fatalf("response len = %d", len(runs[0].ResponseJSON))
	}
}

func TestWithAgentRunnerNilSafe(t *testing.T) {
	var s *Scheduler
	if s.WithAgentRunner(&fakeAgentRunner{}) != nil {
		t.Fatal("nil scheduler must stay nil")
	}
}
