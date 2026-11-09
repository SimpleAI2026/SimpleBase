package agentbridge

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// fakeStore 实现 CronAgentStore（内存版）。
type fakeStore struct {
	mu sync.Mutex

	agents    map[string]systemdb.CloudAgent
	threads   map[string]systemdb.AgentThread
	messages  []systemdb.AgentMessage
	agentRuns map[string]systemdb.AgentRun
	jobs      map[string]systemdb.CronJob
	logs      []systemdb.LogEvent

	// 故障注入
	createRunErr    error
	appendUserErr   error
	listMessagesErr error
	createThreadErr error
	updateJobErr    error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		agents:    map[string]systemdb.CloudAgent{},
		threads:   map[string]systemdb.AgentThread{},
		agentRuns: map[string]systemdb.AgentRun{},
		jobs:      map[string]systemdb.CronJob{},
	}
}

func (f *fakeStore) GetCloudAgent(_ context.Context, _, id string) (systemdb.CloudAgent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.agents[id]
	if !ok {
		return systemdb.CloudAgent{}, sql.ErrNoRows
	}
	return a, nil
}

func (f *fakeStore) GetAgentThread(_ context.Context, _, id string) (systemdb.AgentThread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.threads[id]
	if !ok {
		return systemdb.AgentThread{}, sql.ErrNoRows
	}
	return t, nil
}

func (f *fakeStore) CreateAgentThread(_ context.Context, t systemdb.AgentThread) (systemdb.AgentThread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createThreadErr != nil {
		return systemdb.AgentThread{}, f.createThreadErr
	}
	t.ID = "thread-new"
	f.threads[t.ID] = t
	return t, nil
}

func (f *fakeStore) AppendAgentMessage(_ context.Context, m systemdb.AgentMessage) (systemdb.AgentMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.appendUserErr != nil && m.Role == "user" {
		return systemdb.AgentMessage{}, f.appendUserErr
	}
	m.ID = "msg-" + string(rune('0'+len(f.messages)))
	f.messages = append(f.messages, m)
	return m, nil
}

func (f *fakeStore) ListAgentMessages(_ context.Context, _, _ string, _ int) ([]systemdb.AgentMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listMessagesErr != nil {
		return nil, f.listMessagesErr
	}
	out := make([]systemdb.AgentMessage, len(f.messages))
	copy(out, f.messages)
	return out, nil
}

func (f *fakeStore) CreateAgentRun(_ context.Context, r systemdb.AgentRun) (systemdb.AgentRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createRunErr != nil {
		return systemdb.AgentRun{}, f.createRunErr
	}
	r.ID = "run-" + string(rune('0'+len(f.agentRuns)))
	f.agentRuns[r.ID] = r
	return r, nil
}

func (f *fakeStore) UpdateAgentRunStatus(_ context.Context, _, id, status, errMsg string, _, _ *time.Time) error {
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

func (f *fakeStore) GetCronJob(_ context.Context, _, id string) (systemdb.CronJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return systemdb.CronJob{}, sql.ErrNoRows
	}
	return j, nil
}

func (f *fakeStore) UpdateCronJob(_ context.Context, j systemdb.CronJob) (systemdb.CronJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateJobErr != nil {
		return systemdb.CronJob{}, f.updateJobErr
	}
	if _, ok := f.jobs[j.ID]; !ok {
		return systemdb.CronJob{}, sql.ErrNoRows
	}
	f.jobs[j.ID] = j
	return j, nil
}

func (f *fakeStore) RecordLog(ev systemdb.LogEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, ev)
}

func (f *fakeStore) onlyRun(t *testing.T) systemdb.AgentRun {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.agentRuns) != 1 {
		t.Fatalf("agent runs = %d, want 1", len(f.agentRuns))
	}
	for _, r := range f.agentRuns {
		return r
	}
	return systemdb.AgentRun{}
}

// fakeRunner 记录 RunRequest。
type fakeRunner struct {
	mu    sync.Mutex
	calls []gocloudagent.RunRequest
	err   error
}

func (r *fakeRunner) StartRun(_ context.Context, req gocloudagent.RunRequest, _ func(gocloudagent.Event)) (gocloudagent.RunResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, req)
	if r.err != nil {
		return gocloudagent.RunResult{}, r.err
	}
	return gocloudagent.RunResult{Content: "ok", ToolCallsJSON: "[]"}, nil
}

func (r *fakeRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

type fakeQuota struct{ err error }

func (q fakeQuota) CheckQuota(_ context.Context, _, _ string) error { return q.err }

func testJob() systemdb.CronJob {
	return systemdb.CronJob{
		ID: "job-1", ProjectID: "p1", Name: "digest", ScheduleKind: systemdb.CronJobKindInterval, IntervalSeconds: 900,
		TargetKind: systemdb.CronJobTargetAgent, AgentID: "a1", AgentThreadID: "t1",
		AgentPrompt: "check today's orders", Enabled: true,
		NextRunAt: time.Now().UTC().Add(time.Minute),
	}
}

func seeded(t *testing.T) (*fakeStore, systemdb.CronJob) {
	t.Helper()
	store := newFakeStore()
	store.agents["a1"] = systemdb.CloudAgent{ID: "a1", ProjectID: "p1", Name: "Database", Module: "database"}
	store.threads["t1"] = systemdb.AgentThread{ID: "t1", ProjectID: "p1", AgentID: "a1", Title: "Scheduled: Database"}
	j := testJob()
	store.jobs[j.ID] = j
	return store, j
}

// 成功路径：Runner 收到只读 principal、Headless、非流式、默认 skill，
// 用户消息带 [scheduled] 前缀，run 落 completed，结果回显。
func TestRunAgentSuccess(t *testing.T) {
	store, j := seeded(t)
	runner := &fakeRunner{}
	e := NewCronAgentExecutor(store, runner, fakeQuota{})

	res, err := e.RunAgent(context.Background(), j, "scheduled")
	if err != nil {
		t.Fatal(err)
	}
	if res.Response != "ok" {
		t.Fatalf("response = %q", res.Response)
	}
	if runner.callCount() != 1 {
		t.Fatalf("runner calls = %d", runner.callCount())
	}
	req := runner.calls[0]
	if req.Stream || !req.Headless {
		t.Fatalf("stream=%v headless=%v, want false/true", req.Stream, req.Headless)
	}
	if req.Principal.APIKeyID != CronAgentPrincipalID {
		t.Fatalf("principal = %q", req.Principal.APIKeyID)
	}
	if _, ok := req.Principal.Permissions[gocloudagent.DatabaseRead]; !ok || len(req.Principal.Permissions) != 1 {
		t.Fatalf("principal must be read-only: %+v", req.Principal.Permissions)
	}
	if len(req.Skills) != 1 || req.Skills[0] != "database" {
		t.Fatalf("skills = %v", req.Skills)
	}
	if req.UserText != j.AgentPrompt || req.ThreadID != "t1" {
		t.Fatalf("req = %+v", req)
	}
	if store.messages[0].Role != "user" || !strings.HasPrefix(store.messages[0].Content, "[scheduled] ") {
		t.Fatalf("user message = %+v", store.messages[0])
	}
	if store.messages[len(store.messages)-1].Role != "assistant" {
		t.Fatal("assistant message not appended")
	}
	if got := store.onlyRun(t).Status; got != systemdb.AgentRunCompleted {
		t.Fatalf("run status = %q", got)
	}
	// 历史里不能包含刚插入的这条用户消息（否则 prompt 会重复）。
	if len(req.History) != 0 {
		t.Fatalf("history should exclude the just-inserted user message: %+v", req.History)
	}
}

// 配额不足：不调用 Runner，不创建 run。
func TestRunAgentQuotaExceeded(t *testing.T) {
	store, j := seeded(t)
	runner := &fakeRunner{}
	e := NewCronAgentExecutor(store, runner, fakeQuota{err: errors.New("quota exceeded")})

	_, err := e.RunAgent(context.Background(), j, "scheduled")
	if err == nil || err.Error() != "llm quota exceeded" {
		t.Fatalf("err = %v", err)
	}
	if runner.callCount() != 0 || len(store.agentRuns) != 0 {
		t.Fatal("runner/run must not be touched when quota is exceeded")
	}
}

// agent 已删除：失败并停用任务，next_run_at 清空。
func TestRunAgentMissingAgentDisablesJob(t *testing.T) {
	store, j := seeded(t)
	delete(store.agents, "a1")
	runner := &fakeRunner{}
	e := NewCronAgentExecutor(store, runner, nil)

	_, err := e.RunAgent(context.Background(), j, "scheduled")
	if err == nil || err.Error() != "agent not found" {
		t.Fatalf("err = %v", err)
	}
	got := store.jobs[j.ID]
	if got.Enabled || !got.NextRunAt.IsZero() {
		t.Fatalf("job should be disabled with cleared schedule: %+v", got)
	}
	if runner.callCount() != 0 {
		t.Fatal("runner must not run")
	}
}

// 专属 thread 被归档：重建并回写 agent_thread_id；回写保留调度器刚推进的排期字段。
func TestRunAgentThreadRecreatedAndRebound(t *testing.T) {
	store, j := seeded(t)
	delete(store.threads, "t1")
	// 模拟调度器 ClaimCronJob 之后库里已有更新的 next_run_at。
	advanced := j
	advanced.NextRunAt = j.NextRunAt.Add(time.Hour)
	store.jobs[j.ID] = advanced
	runner := &fakeRunner{}
	e := NewCronAgentExecutor(store, runner, nil)

	if _, err := e.RunAgent(context.Background(), j, "scheduled"); err != nil {
		t.Fatal(err)
	}
	if runner.calls[0].ThreadID != "thread-new" {
		t.Fatalf("thread = %q", runner.calls[0].ThreadID)
	}
	got := store.jobs[j.ID]
	if got.AgentThreadID != "thread-new" {
		t.Fatalf("thread not rebound: %+v", got)
	}
	if !got.NextRunAt.Equal(advanced.NextRunAt) {
		t.Fatal("rebind must not clobber the scheduler-advanced next_run_at")
	}
	if store.threads["thread-new"].AgentID != "a1" || store.threads["thread-new"].CreatedBy != CronAgentPrincipalID {
		t.Fatalf("recreated thread = %+v", store.threads["thread-new"])
	}
}

// thread 属于另一个 agent：拒绝，不产生 run 与消息。
func TestRunAgentRejectsCrossAgentThread(t *testing.T) {
	store, j := seeded(t)
	store.threads["t1"] = systemdb.AgentThread{ID: "t1", ProjectID: "p1", AgentID: "other"}
	runner := &fakeRunner{}
	e := NewCronAgentExecutor(store, runner, nil)

	_, err := e.RunAgent(context.Background(), j, "scheduled")
	if err == nil || err.Error() != "schedule thread belongs to another agent" {
		t.Fatalf("err = %v", err)
	}
	if runner.callCount() != 0 || len(store.agentRuns) != 0 || len(store.messages) != 0 {
		t.Fatal("no run/message may be produced")
	}
}

// Runner 失败：run 落 failed，错误文本被截断，不返回结果。
func TestRunAgentRunnerFailure(t *testing.T) {
	store, j := seeded(t)
	runner := &fakeRunner{err: errors.New(strings.Repeat("x", 500))}
	e := NewCronAgentExecutor(store, runner, nil)

	res, err := e.RunAgent(context.Background(), j, "manual")
	if err == nil || len(err.Error()) != cronAgentErrorLimit {
		t.Fatalf("err len = %v", err)
	}
	if res.Response != "" {
		t.Fatal("no response on failure")
	}
	run := store.onlyRun(t)
	if run.Status != systemdb.AgentRunFailed || len(run.Error) != cronAgentErrorLimit {
		t.Fatalf("run = %+v", run)
	}
	// 失败时不追加 assistant 消息。
	for _, m := range store.messages {
		if m.Role == "assistant" {
			t.Fatal("assistant message must not be appended on failure")
		}
	}
}

// context canceled 落 canceled，而不是 failed。
func TestRunAgentCanceled(t *testing.T) {
	store, j := seeded(t)
	e := NewCronAgentExecutor(store, &fakeRunner{err: errors.New("context canceled")}, nil)

	if _, err := e.RunAgent(context.Background(), j, "scheduled"); err == nil {
		t.Fatal("want error")
	}
	if got := store.onlyRun(t).Status; got != systemdb.AgentRunCanceled {
		t.Fatalf("status = %q", got)
	}
}

// 各基础设施故障分支：均返回稳定短文本，且不调用 Runner。
func TestRunAgentInfraFailures(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*fakeStore)
		want string
	}{
		{"create run", func(s *fakeStore) { s.createRunErr = errors.New("db") }, "create agent run failed"},
		{"append user", func(s *fakeStore) { s.appendUserErr = errors.New("db") }, "append user message failed"},
		{"recreate thread", func(s *fakeStore) {
			delete(s.threads, "t1")
			s.createThreadErr = errors.New("db")
		}, "schedule thread unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, j := seeded(t)
			tc.mut(store)
			runner := &fakeRunner{}
			_, err := NewCronAgentExecutor(store, runner, nil).RunAgent(context.Background(), j, "scheduled")
			if err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if runner.callCount() != 0 {
				t.Fatal("runner must not run")
			}
		})
	}
}

// 列历史失败不阻断执行（降级为无历史）。
func TestRunAgentListMessagesErrorStillRuns(t *testing.T) {
	store, j := seeded(t)
	store.listMessagesErr = errors.New("db")
	runner := &fakeRunner{}
	if _, err := NewCronAgentExecutor(store, runner, nil).RunAgent(context.Background(), j, "scheduled"); err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 1 {
		t.Fatal("runner should still run")
	}
}

// 回写失败只记日志，不影响本次执行。
func TestRunAgentRebindFailureIsNonFatal(t *testing.T) {
	store, j := seeded(t)
	delete(store.threads, "t1")
	store.updateJobErr = errors.New("db")
	runner := &fakeRunner{}
	if _, err := NewCronAgentExecutor(store, runner, nil).RunAgent(context.Background(), j, "scheduled"); err != nil {
		t.Fatal(err)
	}
	if len(store.logs) == 0 {
		t.Fatal("rebind failure should be logged")
	}
}

// 未配置的执行器返回明确错误，不 panic。
func TestRunAgentNotConfigured(t *testing.T) {
	var nilExec *CronAgentExecutor
	if _, err := nilExec.RunAgent(context.Background(), testJob(), "manual"); err == nil {
		t.Fatal("nil executor must error")
	}
	if _, err := NewCronAgentExecutor(nil, nil, nil).RunAgent(context.Background(), testJob(), "manual"); err == nil {
		t.Fatal("unconfigured executor must error")
	}
}

func TestCronAgentErr(t *testing.T) {
	if cronAgentErr(nil) != "" {
		t.Fatal("nil error must be empty")
	}
	if got := cronAgentErr(errors.New("short")); got != "short" {
		t.Fatalf("got %q", got)
	}
	if got := cronAgentErr(errors.New(strings.Repeat("y", 1000))); len(got) != cronAgentErrorLimit {
		t.Fatalf("len = %d", len(got))
	}
}
