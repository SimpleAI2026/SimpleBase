// 文件说明：coverage_more_test.go — gofunction/internal/interp 的单元测试（TestNewSequenceIDAndParseInit、TestIsDeadlineExceeded 等）
//
// 用法
//
//	go test ./gofunction/internal/interp -run 'TestNewSequenceIDAndParseInit|TestIsDeadlineExceeded|TestRunCompileError'
//
// 调用架构
//
//	[上游] gofunction
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gofunction/internal/interp/coverage_more_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bench_test.go, cachetest_test.go, call.go, call_test.go, cheap_test.go 等 40 个
//
// 主要符号
//
//	TestNewSequenceIDAndParseInit(t *testing.T)
//	TestIsDeadlineExceeded(t *testing.T)
//	TestRunCompileError(t *testing.T)

package interp

import (
	"context"
	"errors"
	"testing"
)

func TestNewSequenceIDAndParseInit(t *testing.T) {
	id := newSequenceID()
	if len(id) < 5 || id[:4] != "seq-" {
		t.Fatalf("id=%q", id)
	}
	src := `package main
func init() {}
func Exported() {}
`
	pub, err := ParseFuncList(src, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range pub {
		if n == "init" {
			t.Fatal("init must not be exported")
		}
	}
}

func TestIsDeadlineExceeded(t *testing.T) {
	if isDeadlineExceeded(nil) {
		t.Fatal("nil")
	}
	if !isDeadlineExceeded(context.DeadlineExceeded) {
		t.Fatal("sentinel")
	}
	if !isDeadlineExceeded(errors.New("recover: context deadline exceeded")) {
		t.Fatal("wrapped string")
	}
	if isDeadlineExceeded(errors.New("other")) {
		t.Fatal("other")
	}
}

func TestRunCompileError(t *testing.T) {
	_, err := Run("t", "not go", "F")
	if err == nil {
		t.Fatal("expected compile error")
	}
}
