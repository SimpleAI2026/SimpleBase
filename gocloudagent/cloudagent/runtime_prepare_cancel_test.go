// 文件说明：runtime_prepare_cancel_test.go — gocloudagent/cloudagent 的单元测试（TestRunTimeoutCoversPreparation、TestCancelRunDuringPreparation 等）
//
// 用法
//
//	go test ./gocloudagent/cloudagent -run 'TestRunTimeoutCoversPreparation|TestCancelRunDuringPreparation|TestRunCleanupOrderIsStopThenCleanupThenReleaseThread|TestRunTimeoutStillAppliesToNormalRuns'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/runtime_prepare_cancel_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//
// 主要符号
//
//	blockingProvider(entered chan<- struct{}, cleaned *bool) RunProvider
//	TestRunTimeoutCoversPreparation(t *testing.T)
//	TestCancelRunDuringPreparation(t *testing.T)
//	TestRunCleanupOrderIsStopThenCleanupThenReleaseThread(t *testing.T)
//	TestRunTimeoutStillAppliesToNormalRuns(t *testing.T)
//	TestRunReleasesEverythingWhenPrepareFails(t *testing.T)

package cloudagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// blockingProvider 的 Prepare 只在 ctx 结束时才返回，模拟准备期卡在外部依赖上。
func blockingProvider(entered chan<- struct{}, cleaned *bool) RunProvider {
	return RunProviderFunc(func(ctx context.Context, _ RunRequest, _ func(Event)) (RunPreparation, error) {
		close(entered)
		<-ctx.Done()
		return RunPreparation{Cleanup: func() { *cleaned = true }}, ctx.Err()
	})
}

// BUG-16a：准备期同样受 RunTimeout 约束（之前 Prepare 之后才开始计时）。
func TestRunTimeoutCoversPreparation(t *testing.T) {
	entered := make(chan struct{})
	var cleaned bool
	rt := &Runtime{LLM: &scriptedLLM{}, RunTimeout: 150 * time.Millisecond, Provider: blockingProvider(entered, &cleaned)}
	done := make(chan error, 1)
	go func() {
		_, err := rt.StartRun(context.Background(), RunRequest{
			ThreadID: "t", RunID: "run-prep-timeout", UserText: "hi", Agent: Agent{ID: "a", Name: "Agent"},
		}, nil)
		done <- err
	}()
	<-entered
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err=%v, want deadline exceeded", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunTimeout did not interrupt Prepare")
	}
	if !cleaned {
		t.Fatal("Cleanup must still run when Prepare fails")
	}
}

// BUG-16b：准备期间 CancelRun 能取消该 run，且 IsRunActive 与之一致。
func TestCancelRunDuringPreparation(t *testing.T) {
	entered := make(chan struct{})
	var cleaned bool
	rt := &Runtime{LLM: &scriptedLLM{}, Provider: blockingProvider(entered, &cleaned)}
	done := make(chan error, 1)
	go func() {
		_, err := rt.StartRun(context.Background(), RunRequest{
			ThreadID: "t", RunID: "run-prep-cancel", UserText: "hi", Agent: Agent{ID: "a", Name: "Agent"},
		}, nil)
		done <- err
	}()
	<-entered
	if !rt.IsRunActive("run-prep-cancel") {
		t.Fatal("run must be active while preparing")
	}
	if !rt.CancelRun("run-prep-cancel") {
		t.Fatal("CancelRun must find the run while it is still preparing")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v, want context canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("CancelRun did not interrupt Prepare")
	}
	if !cleaned {
		t.Fatal("Cleanup must run after cancellation")
	}
	if rt.IsRunActive("run-prep-cancel") || rt.CancelRun("run-prep-cancel") {
		t.Fatal("run must be fully released after StartRun returns")
	}
}

// 清理顺序：stop(取消登记) → mcpSet.Close → prep.Cleanup → releaseThread。
// 通过观察「Cleanup 执行时，取消登记已清除、线程仍被占用」来验证。
func TestRunCleanupOrderIsStopThenCleanupThenReleaseThread(t *testing.T) {
	var cancelRegistered, threadHeld bool
	var rt *Runtime
	rt = &Runtime{LLM: &scriptedLLM{}, Provider: RunProviderFunc(func(context.Context, RunRequest, func(Event)) (RunPreparation, error) {
		return RunPreparation{Cleanup: func() {
			cancelRegistered = rt.CancelRun("run-order")
			threadHeld = rt.IsRunActive("run-order")
		}}, nil
	})}
	if _, err := rt.StartRun(context.Background(), RunRequest{
		ThreadID: "t", RunID: "run-order", UserText: "hi", Agent: Agent{ID: "a", Name: "Agent"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if cancelRegistered {
		t.Fatal("cancel registration must be cleared before prep.Cleanup runs")
	}
	if !threadHeld {
		t.Fatal("thread must still be claimed while prep.Cleanup runs")
	}
	if rt.IsRunActive("run-order") {
		t.Fatal("thread must be released after StartRun returns")
	}
}

// 超时按整个 run 计，且正常 run 的耗时不受影响。
func TestRunTimeoutStillAppliesToNormalRuns(t *testing.T) {
	rt := &Runtime{LLM: &c3NoUsageLLM{}, RunTimeout: 5 * time.Second}
	res, err := rt.StartRun(context.Background(), RunRequest{
		ThreadID: "t", RunID: "run-normal", UserText: "hi", Stream: true, Agent: Agent{ID: "a", Name: "Agent"},
	}, nil)
	if err != nil || !strings.Contains(res.Content, "你好") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// 无论成功还是 Prepare 失败，StartRun 返回后取消登记与线程占用都必须清除。
func TestRunReleasesEverythingWhenPrepareFails(t *testing.T) {
	boom := errors.New("prepare failed")
	var cleaned bool
	rt := &Runtime{LLM: &scriptedLLM{}, Provider: RunProviderFunc(func(context.Context, RunRequest, func(Event)) (RunPreparation, error) {
		return RunPreparation{Cleanup: func() { cleaned = true }}, boom
	})}
	_, err := rt.StartRun(context.Background(), RunRequest{
		ThreadID: "t", RunID: "run-prep-fail", UserText: "hi", Agent: Agent{ID: "a", Name: "Agent"},
	}, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if !cleaned {
		t.Fatal("Cleanup must run even when Prepare returns an error")
	}
	if rt.IsRunActive("run-prep-fail") || rt.CancelRun("run-prep-fail") {
		t.Fatal("nothing may remain registered after a failed Prepare")
	}
	if err := rt.ClaimThread("t", "another-run"); err != nil {
		t.Fatalf("thread must be released after failure: %v", err)
	}
}
