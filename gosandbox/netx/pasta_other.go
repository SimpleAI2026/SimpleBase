//go:build !linux

// 文件说明：pasta_other.go — gosandbox/netx 中 Runner、Available 等符号的实现
//
// 用法
//
//	type Runner struct{…}  // Runner 在非 Linux 上是空类型，只为让签名一致
//	Available() bool  // Available 恒为 false：netns 与 pasta 都是 Linux 专有
//	PastaPath() string  // PastaPath 恒为空
//	type StartOptions struct{…}  // StartOptions 与 Linux 实现同形
//	Start(context.Context, StartOptions) (*Runner, error)  // Start 恒返回 ErrUnsupported
//
// 调用架构
//
//	[上游] gosandbox/box, gosandbox/vm/box, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/netx/pasta_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] netx.go, netx_test.go, pasta_linux.go
//
// 主要符号
//
//	type Runner struct{…}
//	Available() bool
//	PastaPath() string
//	type StartOptions struct{…}
//	Start(context.Context, StartOptions) (*Runner, error)
//	(*Runner) Done() <-chan struct{}
//	(*Runner) Err() error
//	(*Runner) Stop() error

package netx

import (
	"context"
	"fmt"
)

// Runner 在非 Linux 上是空类型，只为让签名一致。
type Runner struct{}

// Available 恒为 false：netns 与 pasta 都是 Linux 专有。
func Available() bool { return false }

// PastaPath 恒为空。
func PastaPath() string { return "" }

// StartOptions 与 Linux 实现同形。
type StartOptions struct {
	NetnsPID int
	Mode     Mode
	MTU      int
	Log      func(line string)
}

// Start 恒返回 ErrUnsupported。
func Start(context.Context, StartOptions) (*Runner, error) {
	return nil, fmt.Errorf("%w: only available on linux", ErrUnsupported)
}

// Done 实现最小接口。
func (*Runner) Done() <-chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

// Err 恒为 nil。
func (*Runner) Err() error { return nil }

// Stop 无事可做。
func (*Runner) Stop() error { return nil }
