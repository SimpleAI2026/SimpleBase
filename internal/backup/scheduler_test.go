// 文件说明：scheduler_test.go — 备份调度器 catchup/单飞/上报测试（§8.3.2）
package backup

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/crontab"
)

// fakeSchedRepo / fakeSchedService 桩：无需真实存储。
type fakeSchedRepo struct {
	mu     sync.Mutex
	listed int
}

func (f *fakeSchedRepo) ListProjectsCalls() int { f.mu.Lock(); defer f.mu.Unlock(); return f.listed }

type recordingReporter struct {
	mu        sync.Mutex
	succeeded []time.Time
	failed    []error
}

func (r *recordingReporter) BackupSucceeded(at time.Time, projects int) {
	r.mu.Lock()
	r.succeeded = append(r.succeeded, at)
	r.mu.Unlock()
}

func (r *recordingReporter) BackupFailed(at time.Time, err error) {
	r.mu.Lock()
	r.failed = append(r.failed, err)
	r.mu.Unlock()
}

// lastBackupFake 实现 projectLister + lastBackupReader（§8.3.2 catchup 判定注入）。
type lastBackupFake struct {
	lastAt time.Time
	err    error
}

func (f *lastBackupFake) ListProjects(ctx context.Context) ([]ProjectTarget, error) {
	return nil, nil
}

func (f *lastBackupFake) LastBackupAt(ctx context.Context) (time.Time, error) {
	return f.lastAt, f.err
}

func TestMissedWindowDetection(t *testing.T) {
	// 每日 4 点备份；现在是 2026-11-09 14:37（上一窗口：今晨 4 点）。
	spec, err := crontab.ParseCron("0 4 * * *")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 14, 37, 0, 0, time.UTC)

	// 最近备份 = 昨晨 3 点（早于今晨窗口）→ 错过 → catchup。
	s := &Scheduler{repo: &lastBackupFake{lastAt: time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC)},
		spec: spec, opts: SchedulerOptions{Now: func() time.Time { return now }}}
	if !s.missedWindow(context.Background()) {
		t.Fatal("backup older than previous window must trigger catchup")
	}

	// 最近备份 = 今晨 4:30（晚于窗口）→ 未错过。
	s.repo = &lastBackupFake{lastAt: time.Date(2026, 11, 9, 4, 30, 0, 0, time.UTC)}
	if s.missedWindow(context.Background()) {
		t.Fatal("fresh backup must not trigger catchup")
	}

	// 全新部署（无备份记录）→ 不补跑，等首个窗口。
	s.repo = &lastBackupFake{}
	if s.missedWindow(context.Background()) {
		t.Fatal("no backup history must not trigger catchup")
	}

	// miss 策略下 Start 不做 catchup（missedWindow 不被调用——此处验证判定本身仍可用）。
	// 仓库不支持 LastBackupAt（仅 ListProjects）→ 保守 false。
	bare := &Scheduler{repo: &errRepo{}, spec: spec,
		opts: SchedulerOptions{Now: func() time.Time { return now }}}
	if bare.missedWindow(context.Background()) {
		t.Fatal("repo without LastBackupAt must be conservative")
	}
}

func TestSchedulerSingleFlightAndReport(t *testing.T) {
	reporter := &recordingReporter{}
	// RunOnce 会走 repo.ListProjects；用真实仓库接口太重，改为直接测 runOnceGuarded
	// 的单飞逻辑：把 running 置真模拟进行中。
	s := &Scheduler{opts: SchedulerOptions{Reporter: reporter, Now: time.Now}}

	// 单飞：进行中时第二次调用直接跳过。
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	if err := s.runOnceGuarded(context.Background()); err != nil {
		t.Fatalf("single-flight skip must not error, got %v", err)
	}
	if s.Skipped() != 1 {
		t.Fatalf("skipped=%d want 1", s.Skipped())
	}
	// Reporter 未被调用（跳过路径不上报）。
	reporter.mu.Lock()
	succ, fail := len(reporter.succeeded), len(reporter.failed)
	reporter.mu.Unlock()
	if succ != 0 || fail != 0 {
		t.Fatalf("skip path must not report, got succ=%d fail=%d", succ, fail)
	}
}

func TestSchedulerReportsFailure(t *testing.T) {
	reporter := &recordingReporter{}
	// service/repo 为 nil：RunOnceDetailed 在 repo.ListProjects 处 panic 风险——
	// 改为注入一个返回错误的桩 repo。
	s := &Scheduler{
		opts: SchedulerOptions{Reporter: reporter, Now: time.Now},
		repo: &errRepo{},
	}
	err := s.runOnceGuarded(context.Background())
	if err == nil {
		t.Fatal("must propagate error")
	}
	reporter.mu.Lock()
	fail := len(reporter.failed)
	reporter.mu.Unlock()
	if fail != 1 {
		t.Fatalf("failure must be reported once, got %d", fail)
	}
}

type errRepo struct{}

func (e *errRepo) ListProjects(ctx context.Context) ([]ProjectTarget, error) {
	return nil, errors.New("repo down")
}
