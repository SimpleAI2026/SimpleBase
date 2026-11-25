//go:build !linux && !darwin

// 文件说明：run_other.go — gosandbox/host 中 Run 等符号的实现
//
// 用法
//
//	Run(_ context.Context, p Policy, _ Command) (Result, error)  // Run 在不支持的平台上 fail-closed
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/run_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//
// 主要符号
//
//	probeDarwin() (Caps, error)
//	Run(_ context.Context, p Policy, _ Command) (Result, error)

package host

import (
	"context"
	"runtime"
)

func probeDarwin() (Caps, error) {
	return Caps{OS: runtime.GOOS}, unavailable("unsupported OS %s", runtime.GOOS)
}

// Run 在不支持的平台上 fail-closed。
func Run(_ context.Context, p Policy, _ Command) (Result, error) {
	if !p.Isolates() {
		return Result{}, ErrNotIsolated
	}
	return Result{}, unavailable("unsupported OS %s", runtime.GOOS)
}
