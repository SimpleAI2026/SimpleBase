// 文件说明：runtime_test.go — gofunction/internal/interp 的单元测试（TestRuntimeInitAndReset、TestSetLoggerAndDebugOutput 等）
//
// 用法
//
//	go test ./gofunction/internal/interp -run 'TestRuntimeInitAndReset|TestSetLoggerAndDebugOutput'
//
// 调用架构
//
//	[上游] gofunction
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/internal/interp/runtime_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bench_test.go, cachetest_test.go, call.go, call_test.go, cheap_test.go 等 40 个
//
// 主要符号
//
//	TestRuntimeInitAndReset(t *testing.T)
//	TestSetLoggerAndDebugOutput(t *testing.T)

package interp

import (
	"bytes"
	"io"
	"log/slog"
	"testing"
)

func TestRuntimeInitAndReset(t *testing.T) {
	r := NewRuntime()
	r.Initialize("")
	if r.SequenceID == "" {
		t.Fatal("expected generated sequence id")
	}
	r.Initialize("fixed")
	if r.SequenceID != "fixed" {
		t.Fatalf("got %s", r.SequenceID)
	}
	r.Reset()
	if r.Program != nil {
		t.Fatal("reset should clear program")
	}
	r.Cleanup()
}

func TestSetLoggerAndDebugOutput(t *testing.T) {
	SetLogger(nil) // no-op
	SetLogger(slog.Default())
	SetDebugOutput(nil)
	SetDebugOutput(io.Discard)
	var buf bytes.Buffer
	SetDebugOutput(&buf)
	logDebug("hello %s", "world")
}
