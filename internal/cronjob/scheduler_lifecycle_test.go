package cronjob

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// LIFE-01：Stop 必须取消手动触发的任务。
func TestScheduler_StopCancelsManualTrigger(t *testing.T) {
	store := newFakeStore(jobRow("j-stop"))
	runner := newBlockingRunner()
	s := NewScheduler(store, runner)
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(base)

	reqCtx, reqCancel := context.WithCancel(context.Background())
	// 模拟 handler：断开取消链的请求 ctx。
	if err := s.Trigger(context.WithoutCancel(reqCtx), jobRow("j-stop")); err != nil {
		t.Fatal(err)
	}
	reqCancel() // 请求立刻返回
	<-runner.started

	stopDone := make(chan struct{})
	go func() {
		s.Stop()
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("Stop returned while manual task still running (should cancel it first)")
	case <-time.After(150 * time.Millisecond):
	}
	// 取消调度器 base ctx（Shutdown 路径）→ 任务被取消 → Stop 有界返回。
	cancel()
	select {
	case <-stopDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return after scheduler ctx canceled")
	}
}

// LIFE-02：Stop 后 Trigger 必须返回错误，不能向已等待的 WaitGroup Add。
func TestScheduler_TriggerAfterStop(t *testing.T) {
	store := newFakeStore(jobRow("j-1"))
	runner := newBlockingRunner()
	s := NewScheduler(store, runner)
	base, cancel := context.WithCancel(context.Background())
	s.Start(base)

	// 先触发一个任务再停（Stop 需等待任务收敛）。
	if err := s.Trigger(context.WithoutCancel(context.Background()), jobRow("j-1")); err != nil {
		t.Fatal(err)
	}
	<-runner.started
	cancel()
	s.Stop()

	err := s.Trigger(context.WithoutCancel(context.Background()), jobRow("j-1"))
	if err == nil {
		t.Fatal("Trigger after Stop must return an error")
	}
	if err == nil || err.Error() == "" {
		t.Fatal("error must be non-empty")
	}
}

// LIFE-02：并发 Stop 与 Trigger 不得 panic（wg.Add 与 Wait 竞争）。
func TestScheduler_StopTriggerConcurrent(t *testing.T) {
	store := newFakeStore(jobRow("j-race"))
	var fired atomic.Int64
	quick := &quickRunner{fired: &fired}
	s := NewScheduler(store, quick)
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(base)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Trigger(context.Background(), jobRow("j-race"))
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		cancel()
		s.Stop()
	}()
	wg.Wait()
	// 重复 Stop 幂等。
	s.Stop()
}

type quickRunner struct{ fired *atomic.Int64 }

func (r *quickRunner) RunFunction(ctx context.Context, projectID, funcFile, funcExport string, input json.RawMessage) ([]byte, error) {
	r.fired.Add(1)
	return []byte(`{}`), nil
}

func jobRow(id string) systemdb.CronJob {
	return systemdb.CronJob{ID: id, ProjectID: "p1", Enabled: true, ScheduleKind: systemdb.CronJobKindOnce, NextRunAt: time.Now().Add(time.Hour)}
}

// blockingRunner 阻塞直到 ctx 取消或放行，用于观察 Stop 是否取消手动任务。
type blockingRunner struct {
	started  chan struct{}
	doneOnce sync.Once
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{started: make(chan struct{})}
}

func (r *blockingRunner) RunFunction(ctx context.Context, projectID, funcFile, funcExport string, input json.RawMessage) ([]byte, error) {
	r.doneOnce.Do(func() { close(r.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}
