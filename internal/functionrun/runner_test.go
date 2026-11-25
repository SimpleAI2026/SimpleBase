// 文件说明：runner_test.go — internal/functionrun 的单元测试（TestRunnerActiveVersionAndInvokeRecord、TestRunnerSharedConcurrencyGate 等）
//
// 用法
//
//	go test ./internal/functionrun -run 'TestRunnerActiveVersionAndInvokeRecord|TestRunnerSharedConcurrencyGate'
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/trigger
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/functionrun/runner_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] runner.go
//
// 主要符号
//
//	type fakeStore struct{…}
//	(s *fakeStore) ResolveActiveSource(context.Context, string, string) (systemdb.GoFuncVersion, error)
//	(s *fakeStore) RecordGoFuncInvoke(_ context.Context, _, _, _ string, _ int64, channel string, _ int, _ int64, ...
//	TestRunnerActiveVersionAndInvokeRecord(t *testing.T)
//	TestRunnerSharedConcurrencyGate(t *testing.T)

package functionrun

import (
	"context"
	"errors"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type fakeStore struct {
	version systemdb.GoFuncVersion
	calls   int
	channel string
}

func (s *fakeStore) ResolveActiveSource(context.Context, string, string) (systemdb.GoFuncVersion, error) {
	return s.version, nil
}
func (s *fakeStore) RecordGoFuncInvoke(_ context.Context, _, _, _ string, _ int64, channel string, _ int, _ int64, _, _ string) {
	s.calls++
	s.channel = channel
}

func TestRunnerActiveVersionAndInvokeRecord(t *testing.T) {
	store := &fakeStore{version: systemdb.GoFuncVersion{Version: 7, Source: "package main\nfunc Hello(m map[string]interface{}) string { return \"ok\" }\n"}}
	runner := NewRunner(store, 1)
	result, err := runner.Run(context.Background(), Request{ProjectID: "p1", FuncFile: "hello", FuncExport: "Hello", Channel: "trigger"})
	if err != nil || string(result.Output) != `"ok"` || result.Version != 7 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if store.calls != 1 || store.channel != "trigger" {
		t.Fatalf("invoke records=%d channel=%q", store.calls, store.channel)
	}
}

func TestRunnerSharedConcurrencyGate(t *testing.T) {
	store := &fakeStore{}
	runner := NewRunner(store, 1)
	if !runner.Acquire() {
		t.Fatal("first acquire failed")
	}
	defer runner.Release()
	_, err := runner.Run(context.Background(), Request{ProjectID: "p1", FuncFile: "hello", FuncExport: "Hello"})
	if !errors.Is(err, ErrTooManyRuns) {
		t.Fatalf("error=%v", err)
	}
	if runner.Acquire() {
		t.Fatal("second acquire succeeded while slot was occupied")
	}
}
