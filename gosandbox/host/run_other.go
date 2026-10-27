//go:build !linux && !darwin

// 文件说明：run_other.go — gosandbox/host 中 Run 等符号的实现
//
// 用法
//
//	Run(_ context.Context, p Policy, _ Command) (Result, error)  // Run 在不支持的平台上 fail-closed
//	同包调用：probeDarwin() → probe.go, probe_linux.go 等；Run() → run_darwin.go, run_linux.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/run_other.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 14 个
//	[关键引用] probeDarwin() ← probe.go, probe_linux.go, run_darwin.go
//
// 主要符号
//
//	Run(_ context.Context, p Policy, _ Command) (Result, error)
//	... 另有 1 个顶层符号

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
