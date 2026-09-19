package cronjob

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// fakeRunner 记录调用并可注入错误/输出。
type fakeRunner struct {
	mu       sync.Mutex
	calls    []string
	err      error
	response []byte
	delay    time.Duration
}

func (f *fakeRunner) RunFunction(ctx context.Context, projectID, file, export string, input json.RawMessage) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
		}
	}
	f.calls = append(f.calls, projectID+"/"+file+"."+export+"("+string(input)+")")
	if f.err != nil {
		return nil, f.err
	}
	if f.response != nil {
		return f.response, nil
	}
	return []byte(`{"ok":true}`), nil
}

func (f *fakeRunner) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// fakeStore 内存实现调度器所需 Store 接口。
type fakeStore struct {
	mu       sync.Mutex
	jobs     map[string]systemdb.CronJob
	runs     []systemdb.CronJobRun
	claimOK  map[string]bool // 覆盖认领结果（默认 true）
	claimN   int
	logs     []string
	updates  []string
}

func newFakeStore(jobs ...systemdb.CronJob) *fakeStore {
	fs := &fakeStore{jobs: map[string]systemdb.CronJob{}, claimOK: map[string]bool{}}
	for _, j := range jobs {
		fs.jobs[j.ID] = j
	}
	return fs
}

func (f *fakeStore) GetCronJob(ctx context.Context, projectID, id string) (systemdb.CronJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return systemdb.CronJob{}, fmt.Errorf("not found")
	}
	return j, nil
}

func (f *fakeStore) ListDueCronJobs(ctx context.Context, now time.Time, limit int) ([]systemdb.CronJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]systemdb.CronJob, 0)
	for _, j := range f.jobs {
		if j.Enabled && !j.NextRunAt.IsZero() && !j.NextRunAt.After(now) {
			out = append(out, j)
		}
	}
	return out, nil
}

func (f *fakeStore) ClaimCronJob(ctx context.Context, id string, expectNext, newNext, lastRun time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimN++
	if ok, forced := f.claimOK[id]; forced && !ok {
		return false, nil
	}
	j, ok := f.jobs[id]
	if !ok || !j.Enabled || !j.NextRunAt.Equal(expectNext) {
		return false, nil
	}
	j.NextRunAt = newNext
	j.LastRunAt = lastRun
	f.jobs[id] = j
	return true, nil
}

func (f *fakeStore) UpdateCronJob(ctx context.Context, j systemdb.CronJob) (systemdb.CronJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.jobs[j.ID]; !ok {
		return systemdb.CronJob{}, fmt.Errorf("not found")
	}
	f.jobs[j.ID] = j
	f.updates = append(f.updates, j.ID)
	return j, nil
}

func (f *fakeStore) InsertCronJobRun(ctx context.Context, r systemdb.CronJobRun) (systemdb.CronJobRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.ID == "" {
		r.ID = fmt.Sprintf("run-%d", len(f.runs)+1)
	}
	r.CreatedAt = time.Now().UTC()
	f.runs = append(f.runs, r)
	return r, nil
}

func (f *fakeStore) UpdateCronJobRunStatus(ctx context.Context, projectID, id, status, errMsg string, durationMs int64, response string, finished *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.runs {
		if f.runs[i].ID == id {
			f.runs[i].Status = status
			f.runs[i].Error = errMsg
			f.runs[i].DurationMs = durationMs
			f.runs[i].ResponseJSON = response
			if finished != nil {
				f.runs[i].FinishedAt = *finished
			}
		}
	}
	return nil
}

func (f *fakeStore) RecordLog(ev systemdb.LogEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, ev.Message)
}

func (f *fakeStore) job(id string) systemdb.CronJob {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jobs[id]
}

func (f *fakeStore) runList() []systemdb.CronJobRun {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]systemdb.CronJobRun, len(f.runs))
	copy(out, f.runs)
	return out
}

// TestScheduler_DueClaimExecute 到期任务被认领并执行，排期推进、快照回写。
func TestScheduler_DueClaimExecute(t *testing.T) {
	now := time.Now().UTC()
	j := systemdb.CronJob{
		ID: "job-1", ProjectID: "p1", Name: "t", ScheduleKind: systemdb.CronJobKindInterval,
		IntervalSeconds: 600, FuncFile: "hello", FuncExport: "Hello",
		InputJSON: `{"n":1}`, Enabled: true, NextRunAt: now.Add(-time.Minute),
	}
	fs := newFakeStore(j)
	fr := &fakeRunner{}
	s := NewScheduler(fs, fr)
	s.tickOnce(context.Background())
	s.Stop()

	if fr.callCount() != 1 {
		t.Fatalf("want 1 call, got %d", fr.callCount())
	}
	got := fs.job("job-1")
	if got.RunCount != 1 || got.LastStatus != systemdb.CronJobRunCompleted || got.LastRunAt.IsZero() {
		t.Fatalf("snapshot not updated: %+v", got)
	}
	if !got.NextRunAt.After(got.LastRunAt) {
		t.Fatalf("next_run_at not advanced beyond claim time: next=%v last=%v", got.NextRunAt, got.LastRunAt)
	}
	runs := fs.runList()
	if len(runs) != 1 || runs[0].Status != systemdb.CronJobRunCompleted || runs[0].Trigger != TriggerScheduled {
		t.Fatalf("unexpected runs: %+v", runs)
	}
	if runs[0].ResponseJSON != `{"ok":true}` {
		t.Fatalf("unexpected response: %q", runs[0].ResponseJSON)
	}
}

// TestScheduler_CronKindAdvance cron 模式按 NextAfter 推进。
func TestScheduler_CronKindAdvance(t *testing.T) {
	now := time.Now().UTC()
	j := systemdb.CronJob{
		ID: "job-c", ProjectID: "p1", Name: "c", ScheduleKind: systemdb.CronJobKindCron,
		CronExpr: "*/10 * * * *", FuncFile: "hello", FuncExport: "Hello",
		Enabled: true, NextRunAt: now.Add(-time.Second),
	}
	fs := newFakeStore(j)
	s := NewScheduler(fs, &fakeRunner{})
	s.tickOnce(context.Background())
	s.Stop()

	next := fs.job("job-c").NextRunAt
	if next.IsZero() || !next.After(now) {
		t.Fatalf("cron next not in future: %v (now=%v)", next, now)
	}
	if next.Minute()%10 != 0 || next.Second() != 0 {
		t.Fatalf("next not on */10 minute boundary: %v", next)
	}
}

// TestScheduler_ClaimConflict CAS 认领失败则不执行。
func TestScheduler_ClaimConflict(t *testing.T) {
	now := time.Now().UTC()
	j := systemdb.CronJob{
		ID: "job-x", ProjectID: "p1", Name: "x", ScheduleKind: systemdb.CronJobKindInterval,
		IntervalSeconds: 60, FuncFile: "f", FuncExport: "F", Enabled: true, NextRunAt: now.Add(-time.Minute),
	}
	fs := newFakeStore(j)
	fs.claimOK["job-x"] = false
	fr := &fakeRunner{}
	s := NewScheduler(fs, fr)
	s.tickOnce(context.Background())
	s.Stop()
	if fr.callCount() != 0 {
		t.Fatalf("claimed-conflict job should not run, calls=%v", fr.calls)
	}
}

// TestScheduler_BadCronDisabled cron 表达式脏数据自动停用。
func TestScheduler_BadCronDisabled(t *testing.T) {
	now := time.Now().UTC()
	j := systemdb.CronJob{
		ID: "job-bad", ProjectID: "p1", Name: "b", ScheduleKind: systemdb.CronJobKindCron,
		CronExpr: "not a cron", FuncFile: "f", FuncExport: "F", Enabled: true, NextRunAt: now.Add(-time.Minute),
	}
	fs := newFakeStore(j)
	fr := &fakeRunner{}
	s := NewScheduler(fs, fr)
	s.tickOnce(context.Background())
	s.Stop()
	if fr.callCount() != 0 {
		t.Fatalf("bad cron should not run")
	}
	if fs.job("job-bad").Enabled {
		t.Fatal("bad cron job should be disabled")
	}
}

// TestScheduler_FailureRunFailed 执行失败落 failed 记录并回写快照。
func TestScheduler_FailureRunFailed(t *testing.T) {
	now := time.Now().UTC()
	j := systemdb.CronJob{
		ID: "job-f", ProjectID: "p1", Name: "f", ScheduleKind: systemdb.CronJobKindInterval,
		IntervalSeconds: 60, FuncFile: "f", FuncExport: "F", Enabled: true, NextRunAt: now.Add(-time.Minute),
	}
	fs := newFakeStore(j)
	fr := &fakeRunner{err: fmt.Errorf("gofunction: execution timeout")}
	s := NewScheduler(fs, fr)
	s.tickOnce(context.Background())
	s.Stop()

	runs := fs.runList()
	if len(runs) != 1 || runs[0].Status != systemdb.CronJobRunFailed {
		t.Fatalf("want failed run, got %+v", runs)
	}
	if runs[0].ResponseJSON != "" {
		t.Fatalf("failed run should have no response, got %q", runs[0].ResponseJSON)
	}
	got := fs.job("job-f")
	if got.LastStatus != systemdb.CronJobRunFailed || got.LastError == "" {
		t.Fatalf("snapshot: %+v", got)
	}
}

// TestScheduler_TriggerAsync 手动触发异步执行且不改排期。
func TestScheduler_TriggerAsync(t *testing.T) {
	now := time.Now().UTC()
	next := now.Add(time.Hour)
	j := systemdb.CronJob{
		ID: "job-t", ProjectID: "p1", Name: "t", ScheduleKind: systemdb.CronJobKindInterval,
		IntervalSeconds: 3600, FuncFile: "f", FuncExport: "F", Enabled: true, NextRunAt: next,
	}
	fs := newFakeStore(j)
	fr := &fakeRunner{}
	s := NewScheduler(fs, fr)
	if err := s.Trigger(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	s.Stop()

	if fr.callCount() != 1 {
		t.Fatalf("trigger should run once, calls=%v", fr.calls)
	}
	runs := fs.runList()
	if len(runs) != 1 || runs[0].Trigger != TriggerManual {
		t.Fatalf("want manual run, got %+v", runs)
	}
	if !fs.job("job-t").NextRunAt.Equal(next) {
		t.Fatal("trigger must not change next_run_at")
	}
}

// TestScheduler_TriggerConflict 同一任务执行中再触发返回错误。
func TestScheduler_TriggerConflict(t *testing.T) {
	now := time.Now().UTC()
	j := systemdb.CronJob{
		ID: "job-d", ProjectID: "p1", Name: "d", ScheduleKind: systemdb.CronJobKindInterval,
		IntervalSeconds: 60, FuncFile: "f", FuncExport: "F", Enabled: true, NextRunAt: now.Add(time.Hour),
	}
	fs := newFakeStore(j)
	fr := &fakeRunner{delay: 50 * time.Millisecond}
	s := NewScheduler(fs, fr)
	if err := s.Trigger(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	if err := s.Trigger(context.Background(), j); err == nil {
		t.Fatal("second trigger while in-flight should fail")
	}
	s.Stop()
	if fr.callCount() != 1 {
		t.Fatalf("want single execution, got %d", fr.callCount())
	}
}

// TestScheduler_NotConfigured 未配置时 Start/Trigger 安全返回。
func TestScheduler_NotConfigured(t *testing.T) {
	s := NewScheduler(nil, nil)
	s.Start(context.Background()) // 不应 panic
	s.Stop()
	if err := s.Trigger(context.Background(), systemdb.CronJob{}); err == nil {
		t.Fatal("trigger on unconfigured scheduler should error")
	}
}
