//go:build !linux && !darwin

// 文件说明：ptyx_other_test.go — gosandbox/internal/ptyx 的单元测试（TestUnsupported）
//
// 用法
//
//	go test ./internal/ptyx -run TestUnsupported
//
// 调用架构
//
//	[上游] 测试
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/internal/ptyx/ptyx_other_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] ptyx.go, ptyx_other.go
//
// 主要符号
//
//	TestUnsupported(t *testing.T)

package ptyx

import (
	"errors"
	"testing"
)

// 既不是 linux 也不是 darwin 时，Open / SetWinsize 都返回 ErrUnsupported，调用方用 errors.Is 判定。
func TestUnsupported(t *testing.T) {
	master, slave, err := Open()
	if !errors.Is(err, ErrUnsupported) || master != nil || slave != nil {
		t.Fatalf("open = %v, %v, %v; want nil, nil, ErrUnsupported", master, slave, err)
	}
	if err := SetWinsize(nil, 24, 80); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("set winsize = %v, want ErrUnsupported", err)
	}
}
