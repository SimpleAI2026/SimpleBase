// 文件说明：confirmation_test.go — gocloudagent/cloudagent 的单元测试（TestAwaitConfirmationOrderAndApproval、TestAwaitConfirmationHeadlessDoesNotAskOrPrepare 等）
//
// 用法
//
//	go test ./gocloudagent/cloudagent -run 'TestAwaitConfirmationOrderAndApproval|TestAwaitConfirmationHeadlessDoesNotAskOrPrepare|TestAwaitConfirmationNilEmitStillWaits|TestAwaitConfirmationNoWaiterOrCallIDDenies'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/confirmation_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] callgate.go, callgate_test.go, confirmation.go, errors.go, errors_test.go 等 32 个
//
// 主要符号
//
//	type fakeWaiter struct{…}
//	(f *fakeWaiter) Prepare(runID, callID, subject string)
//	(f *fakeWaiter) Wait(_ context.Context, _, _, subject string, timeout time.Duration) (bool, error)
//	type waitOnly struct{…}
//	(w waitOnly) Wait(context.Context, string, string, string, time.Duration) (bool, error)
//	confirmReq(log *[]string, w ConfirmationWaiter) ConfirmationRequest
//	TestAwaitConfirmationOrderAndApproval(t *testing.T)
//	TestAwaitConfirmationHeadlessDoesNotAskOrPrepare(t *testing.T)
//	... 另有 5 个顶层符号

package cloudagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeWaiter struct {
	log      *[]string
	approve  bool
	err      error
	prepared bool
	timeout  time.Duration
	subject  string
}

func (f *fakeWaiter) Prepare(runID, callID, subject string) {
	f.prepared = true
	*f.log = append(*f.log, "prepare:"+subject)
}

func (f *fakeWaiter) Wait(_ context.Context, _, _, subject string, timeout time.Duration) (bool, error) {
	*f.log = append(*f.log, "wait")
	f.subject, f.timeout = subject, timeout
	return f.approve, f.err
}

// 没有 Prepare 能力的 waiter。
type waitOnly struct{ approve bool }

func (w waitOnly) Wait(context.Context, string, string, string, time.Duration) (bool, error) {
	return w.approve, nil
}

func confirmReq(log *[]string, w ConfirmationWaiter) ConfirmationRequest {
	return ConfirmationRequest{
		RunID: "r", CallID: "c", Subject: "user:1", Name: "tool",
		Argv: []string{"a"}, Message: "msg", Waiter: w,
		Emit:        func(ev Event) { *log = append(*log, "emit:"+ev.Type) },
		OnRequested: func() { *log = append(*log, "requested") },
		OnResolved:  func(ok bool) { *log = append(*log, "resolved") },
	}
}

func TestAwaitConfirmationOrderAndApproval(t *testing.T) {
	var log []string
	w := &fakeWaiter{log: &log, approve: true}
	out, err := AwaitConfirmation(context.Background(), confirmReq(&log, w))
	if err != nil || out != ConfirmationApproved {
		t.Fatalf("out=%v err=%v", out, err)
	}
	want := "prepare:user:1,emit:confirmation_required,requested,wait,emit:confirmation_resolved,resolved"
	if strings.Join(log, ",") != want {
		t.Fatalf("order=%v\nwant %s", log, want)
	}
	if w.timeout != DefaultConfirmTimeout || w.subject != "user:1" {
		t.Fatalf("timeout=%v subject=%q", w.timeout, w.subject)
	}
}

func TestAwaitConfirmationHeadlessDoesNotAskOrPrepare(t *testing.T) {
	var log []string
	w := &fakeWaiter{log: &log, approve: true}
	req := confirmReq(&log, w)
	req.Headless = true
	out, err := AwaitConfirmation(context.Background(), req)
	if err != nil || out != ConfirmationUnavailable || w.prepared {
		t.Fatalf("out=%v err=%v prepared=%v", out, err, w.prepared)
	}
	if strings.Join(log, ",") != "emit:confirmation_resolved" {
		t.Fatalf("headless must only emit resolved(false): %v", log)
	}
}

// emit 为 nil：不 Prepare、不发事件，但仍 Wait；回调仍触发。
func TestAwaitConfirmationNilEmitStillWaits(t *testing.T) {
	var log []string
	w := &fakeWaiter{log: &log, approve: true}
	req := confirmReq(&log, w)
	req.Emit = nil
	out, err := AwaitConfirmation(context.Background(), req)
	if err != nil || out != ConfirmationApproved || w.prepared {
		t.Fatalf("out=%v err=%v prepared=%v", out, err, w.prepared)
	}
	if strings.Join(log, ",") != "requested,wait,resolved" {
		t.Fatalf("log=%v", log)
	}
}

func TestAwaitConfirmationNoWaiterOrCallIDDenies(t *testing.T) {
	var log []string
	req := confirmReq(&log, nil)
	if out, err := AwaitConfirmation(context.Background(), req); err != nil || out != ConfirmationDenied {
		t.Fatalf("nil waiter: out=%v err=%v", out, err)
	}
	log = nil
	w := &fakeWaiter{log: &log, approve: true}
	req = confirmReq(&log, w)
	req.CallID = ""
	if out, err := AwaitConfirmation(context.Background(), req); err != nil || out != ConfirmationDenied {
		t.Fatalf("empty call id: out=%v err=%v", out, err)
	}
	for _, entry := range log {
		if entry == "wait" || strings.HasPrefix(entry, "prepare") {
			t.Fatalf("empty call id must not register or wait: %v", log)
		}
	}
	if out, _ := AwaitConfirmation(context.Background(), confirmReq(new([]string), waitOnly{approve: true})); out != ConfirmationApproved {
		t.Fatalf("waiter without Prepare must still work: %v", out)
	}
}

func TestAwaitConfirmationWaitErrorAndTimeoutDeny(t *testing.T) {
	for _, werr := range []error{errors.New("boom"), context.DeadlineExceeded} {
		var log []string
		w := &fakeWaiter{log: &log, approve: true, err: werr}
		out, err := AwaitConfirmation(context.Background(), confirmReq(&log, w))
		if err != nil || out != ConfirmationDenied {
			t.Fatalf("wait error %v: out=%v err=%v", werr, out, err)
		}
	}
}

// Wait 报告取消、而父 ctx 仍有效时，必须返回非 nil 的取消错误（不再是 nil）。
func TestAwaitConfirmationCancelNeverReturnsNilError(t *testing.T) {
	var log []string
	w := &fakeWaiter{log: &log, err: context.Canceled}
	_, err := AwaitConfirmation(context.Background(), confirmReq(&log, w))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	log = nil
	w = &fakeWaiter{log: &log}
	if _, err := AwaitConfirmation(ctx, confirmReq(&log, w)); !errors.Is(err, context.Canceled) {
		t.Fatalf("parent canceled: err=%v", err)
	}
}

// BUG-06：MCP 确认摘要必须先脱敏再截断，凭据不进确认事件。
func TestMCPConfirmArgvRedactsSecrets(t *testing.T) {
	argv := mcpConfirmArgv("srv", "tool", `{"password":"hunter2","q":"x"}`)
	if len(argv) != 3 || strings.Contains(argv[2], "hunter2") {
		t.Fatalf("argv=%v", argv)
	}
}
