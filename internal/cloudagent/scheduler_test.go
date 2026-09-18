package cloudagent

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// fakeScheduleStore 实现 ScheduleStore（内存版）。
type fakeScheduleStore struct {
	mu sync.Mutex

	agents     map[string]systemdb.CloudAgent
	threads    map[string]systemdb.AgentThread
	messages   []systemdb.AgentMessage
	agentRuns  map[string]systemdb.AgentRun
	schedules  map[string]systemdb.AgentSchedule
	schedRuns  []systemdb.AgentScheduleRun
	claimCalls int
	logs       []systemdb.LogEvent

	claimFail bool // ClaimAgentSchedule 返回 false
	quotaErr  error
}

func newFakeScheduleStore() *fakeScheduleStore {
	return &fakeScheduleStore{
		agents:    map[string]systemdb.CloudAgent{},
		threads:   map[string]systemdb.AgentThread{},
		agentRuns: map[string]systemdb.AgentRun{},
		schedules: map[string]systemdb.AgentSchedule{},
	}
}

func (f *fakeScheduleStore) GetCloudAgent(_ context.Context, _, id string) (systemdb.CloudAgent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.agents[id]
	if !ok {
		return systemdb.CloudAgent{}, sql.ErrNoRows
	}
	return a, nil
}

func (f *fakeScheduleStore) GetAgentThread(_ context.Context, _, id string) (systemdb.AgentThread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.threads[id]
	if !ok {
		return systemdb.AgentThread{}, sql.ErrNoRows
	}
	return t, nil
}

func (f *fakeScheduleStore) CreateAgentThread(_ context.Context, t systemdb.AgentThread) (systemdb.AgentThread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t.ID = "thread-" + t.Title
	f.threads[t.ID] = t
	return t, nil
}

func (f *fakeScheduleStore) AppendAgentMessage(_ context.Context, m systemdb.AgentMessage) (systemdb.AgentMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m.ID = "msg-" + string(rune('0'+len(f.messages)))
	f.messages = append(f.messages, m)
	return m, nil
}

func (f *fakeScheduleStore) ListAgentMessages(_ context.Context, _, _ string, _ int) ([]systemdb.AgentMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]systemdb.AgentMessage, len(f.messages))
	copy(out, f.messages)
	return out, nil
}

func (f *fakeScheduleStore) CreateAgentRun(_ context.Context, r systemdb.AgentRun) (systemdb.AgentRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.ID = "run-" + string(rune('0'+len(f.agentRuns)))
	f.agentRuns[r.ID] = r
	return r, nil
}

func (f *fakeScheduleStore) UpdateAgentRunStatus(_ context.Context, _, id, status, errMsg string, _, _ *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.agentRuns[id]
	if !ok {
		return sql.ErrNoRows
	}
	r.Status = status
	r.Error = errMsg
	f.agentRuns[id] = r
	return nil
}

func (f *fakeScheduleStore) ListDueAgentSchedules(_ context.Context, now time.Time, _ int) ([]systemdb.AgentSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]systemdb.AgentSchedule, 0)
	for _, sc := range f.schedules {
		if sc.Enabled && !sc.NextRunAt.IsZero() && !sc.NextRunAt.After(now) {
			out = append(out, sc)
		}
	}
	return out, nil
}

func (f *fakeScheduleStore) ClaimAgentSchedule(_ context.Context, id string, expectNext, newNext, lastRun time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCalls++
	sc, ok := f.schedules[id]
	if !ok || f.claimFail {
		return false, nil
	}
	if !sc.NextRunAt.Equal(expectNext) {
		return false, nil
	}
	sc.NextRunAt = newNext
	sc.LastRunAt = lastRun
	f.schedules[id] = sc
	return true, nil
}

func (f *fakeScheduleStore) UpdateAgentSchedule(_ context.Context, sc systemdb.AgentSchedule) (systemdb.AgentSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.schedules[sc.ID]; !ok {
		return systemdb.AgentSchedule{}, sql.ErrNoRows
	}
	f.schedules[sc.ID] = sc
	return sc, nil
}

func (f *fakeScheduleStore) InsertAgentScheduleRun(_ context.Context, r systemdb.AgentScheduleRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.ID == "" {
		r.ID = "srun-1"
	}
	f.schedRuns = append(f.schedRuns, r)
	return nil
}

func (f *fakeScheduleStore) UpdateAgentScheduleRunStatus(_ context.Context, _, _, status, errMsg string, _ *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.schedRuns) == 0 {
		return sql.ErrNoRows
	}
	f.schedRuns[len(f.schedRuns)-1].Status = status
	f.schedRuns[len(f.schedRuns)-1].Error = errMsg
	return nil
}

func (f *fakeScheduleStore) RecordLog(ev systemdb.LogEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, ev)
}

func (f *fakeScheduleStore) lastSchedRun() (systemdb.AgentScheduleRun, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.schedRuns) == 0 {
		return systemdb.AgentScheduleRun{}, false
	}
	return f.schedRuns[len(f.schedRuns)-1], true
}

// fakeRunner 记录 RunRequest。
type fakeRunner struct {
	mu    sync.Mutex
	calls []RunRequest
	err   error
}

func (r *fakeRunner) StartRun(_ context.Context, req RunRequest, _ func(Event)) (RunResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, req)
	if r.err != nil {
		return RunResult{}, r.err
	}
	return RunResult{Content: "ok", ToolCallsJSON: "[]"}, nil
}

type fakeQuota struct{ err error }

func (q fakeQuota) CheckQuota(_ context.Context, _, _ string) error { return q.err }

func testSchedule() systemdb.AgentSchedule {
	return systemdb.AgentSchedule{
		ID: "sc-1", ProjectID: "p1", AgentID: "a1", ThreadID: "t1",
		Prompt: "check today's orders", CronExpr: "*/15 * * * *", Enabled: true,
		NextRunAt: time.Now().UTC().Add(-time.Minute),
	}
}

// seedSchedule 写入 store 并返回写入的实例，避免两次构造产生纳秒级不同的 NextRunAt。
func seedSchedule(store *fakeScheduleStore) systemdb.AgentSchedule {
	sc := testSchedule()
	store.schedules[sc.ID] = sc
	return sc
}

func TestSchedulerScheduledSuccess(t *testing.T) {
	store := newFakeScheduleStore()
	store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "Database", Module: "database"}
	store.threads["t1"] = systemdb.AgentThread{ID: "t1", ProjectID: "p1", Title: "Scheduled: Database"}
	sc := seedSchedule(store)
	runner := &fakeRunner{}
	s := NewScheduler(store, runner, fakeQuota{})

	s.runDue(context.Background(), sc)
	s.Stop()

	if len(runner.calls) != 1 {
		t.Fatalf("StartRun calls = %d, want 1", len(runner.calls))
	}
	req := runner.calls[0]
	if req.ProjectID != "p1" || req.Agent.ID != "a1" || req.UserText != "check today's orders" {
		t.Errorf("unexpected RunRequest: %+v", req)
	}
	if req.Stream {
		t.Error("scheduled run must be non-stream")
	}
	if req.Principal.APIKeyID != schedulerPrincipalID {
		t.Errorf("principal = %q, want %q", req.Principal.APIKeyID, schedulerPrincipalID)
	}
	if !req.Principal.HasPermission("database_read") && len(req.Principal.Permissions) != 1 {
		t.Errorf("principal permissions = %v, want exactly database_read", req.Principal.Permissions)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	// 用户消息带 [scheduled] 前缀，assistant 回复已落库。
	var user, assistant systemdb.AgentMessage
	for _, m := range store.messages {
		if m.Role == "user" {
			user = m
		}
		if m.Role == "assistant" {
			assistant = m
		}
	}
	if user.Content != "[scheduled] check today's orders" {
		t.Errorf("user message = %q", user.Content)
	}
	if assistant.Content != "ok" {
		t.Errorf("assistant message = %q", assistant.Content)
	}
	// agent run 状态 completed。
	var runStatus string
	for _, r := range store.agentRuns {
		runStatus = r.Status
	}
	if runStatus != systemdb.AgentRunCompleted {
		t.Errorf("agent run status = %q, want completed", runStatus)
	}
	// schedule run 记录 completed。
	if len(store.schedRuns) != 1 || store.schedRuns[0].Status != systemdb.AgentRunCompleted {
		t.Errorf("schedule runs = %+v", store.schedRuns)
	}
	if store.schedRuns[0].Trigger != TriggerScheduled {
		t.Errorf("trigger = %q, want scheduled", store.schedRuns[0].Trigger)
	}
	// next_run_at 已推进且晚于原值；last_run_at 已写入。
	after := store.schedules["sc-1"]
	if after.NextRunAt.Before(time.Now().UTC()) {
		t.Errorf("next_run_at = %v, want future", after.NextRunAt)
	}
	if after.LastRunAt.IsZero() {
		t.Error("last_run_at should be set after claim")
	}
	// 日志已记录且不含 prompt 原文。
	if len(store.logs) == 0 {
		t.Fatal("expected scheduler log")
	}
	for _, l := range store.logs {
		if contains(l.Message, "check today's orders") {
			t.Errorf("log leaks prompt: %q", l.Message)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestSchedulerQuotaExceeded(t *testing.T) {
	store := newFakeScheduleStore()
	store.schedules["sc-1"] = testSchedule()
	runner := &fakeRunner{}
	s := NewScheduler(store, runner, fakeQuota{err: errors.New("quota exceeded")})

	s.execute(context.Background(), testSchedule(), TriggerScheduled)
	s.Stop()

	r, ok := store.lastSchedRun()
	if !ok {
		t.Fatal("expected schedule run record")
	}
	if r.Status != systemdb.AgentRunFailed || r.Error != "llm quota exceeded" {
		t.Errorf("run = %+v", r)
	}
	if len(runner.calls) != 0 {
		t.Errorf("StartRun should not be called, got %d calls", len(runner.calls))
	}
}

func TestSchedulerAgentArchivedDisablesSchedule(t *testing.T) {
	store := newFakeScheduleStore()
	sc := seedSchedule(store) // agent 不存在
	runner := &fakeRunner{}
	s := NewScheduler(store, runner, fakeQuota{})

	s.execute(context.Background(), sc, TriggerScheduled)
	s.Stop()

	r, ok := store.lastSchedRun()
	if !ok || r.Status != systemdb.AgentRunFailed {
		t.Fatalf("run = %+v, ok=%v", r, ok)
	}
	store.mu.Lock()
	cur := store.schedules["sc-1"]
	store.mu.Unlock()
	if cur.Enabled {
		t.Error("schedule should be disabled when agent is archived")
	}
}

func TestSchedulerThreadArchivedRecreates(t *testing.T) {
	store := newFakeScheduleStore()
	store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "Database", Module: "database"}
	sc := seedSchedule(store) // thread 不存在
	runner := &fakeRunner{}
	s := NewScheduler(store, runner, fakeQuota{})

	s.execute(context.Background(), sc, TriggerScheduled)
	s.Stop()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.messages) != 2 { // user + assistant
		t.Errorf("messages = %d, want 2", len(store.messages))
	}
	cur := store.schedules["sc-1"]
	if cur.ThreadID == "t1" || cur.ThreadID == "" {
		t.Errorf("thread_id = %q, want recreated", cur.ThreadID)
	}
}

func TestSchedulerBadCronDisablesSchedule(t *testing.T) {
	store := newFakeScheduleStore()
	sc := testSchedule()
	sc.CronExpr = "bad expr"
	store.schedules["sc-1"] = sc
	runner := &fakeRunner{}
	s := NewScheduler(store, runner, fakeQuota{})

	s.runDue(context.Background(), sc)
	s.Stop()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.schedules["sc-1"].Enabled {
		t.Error("schedule with bad cron should be disabled")
	}
	if len(runner.calls) != 0 {
		t.Errorf("StartRun should not be called")
	}
}

func TestSchedulerClaimLostSkips(t *testing.T) {
	store := newFakeScheduleStore()
	store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "Database"}
	sc := seedSchedule(store)
	store.claimFail = true
	runner := &fakeRunner{}
	s := NewScheduler(store, runner, fakeQuota{})

	s.runDue(context.Background(), sc)
	s.Stop()

	if len(runner.calls) != 0 {
		t.Errorf("StartRun should not be called when claim lost")
	}
	if len(store.schedRuns) != 0 {
		t.Errorf("no schedule run record expected")
	}
}

func TestSchedulerManualTriggerKeepsNextRun(t *testing.T) {
	store := newFakeScheduleStore()
	store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "Database", Module: "database"}
	store.threads["t1"] = systemdb.AgentThread{ID: "t1", ProjectID: "p1"}
	before := time.Now().UTC().Add(time.Hour)
	sc := seedSchedule(store)
	sc.NextRunAt = before
	store.schedules["sc-1"] = sc
	runner := &fakeRunner{}
	s := NewScheduler(store, runner, fakeQuota{})

	_, err := s.Trigger(context.Background(), sc)
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	s.Stop()

	if len(runner.calls) != 1 {
		t.Fatalf("StartRun calls = %d, want 1", len(runner.calls))
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.schedules["sc-1"].NextRunAt.Equal(before) {
		t.Errorf("manual trigger must not change next_run_at: %v", store.schedules["sc-1"].NextRunAt)
	}
	if len(store.schedRuns) != 1 || store.schedRuns[0].Trigger != TriggerManual {
		t.Errorf("schedRuns = %+v", store.schedRuns)
	}
}

func TestSchedulerManualTriggerInFlightRejected(t *testing.T) {
	store := newFakeScheduleStore()
	store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "Database"}
	store.schedules["sc-1"] = testSchedule()
	runner := &fakeRunner{}
	s := NewScheduler(store, runner, fakeQuota{})

	if _, err := s.Trigger(context.Background(), testSchedule()); err != nil {
		t.Fatalf("first Trigger: %v", err)
	}
	if _, err := s.Trigger(context.Background(), testSchedule()); err == nil {
		t.Error("second Trigger while in-flight should be rejected")
	}
	s.Stop()
}
