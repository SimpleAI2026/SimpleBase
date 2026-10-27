//go:build !linux && !darwin

// 文件说明：start_other.go — gosandbox/host 中 Start 等符号的实现
//
// 用法
//
//	Start(_ context.Context, p Policy, _ StartSpec) (Proc, error)  // Start 在不支持的平台上 fail-closed
//	同包调用：Resize() → policy.go, start.go 等；Start() → run_darwin.go, run_linux.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start_other.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 14 个
//	[关键引用] Resize() ← policy.go, start.go, start_darwin.go
//
// 主要符号
//
//	Start(_ context.Context, p Policy, _ StartSpec) (Proc, error)
//	... 另有 1 个顶层符号

package host

import (
	"context"
	"runtime"
)

// Start 在不支持的平台上 fail-closed。
func Start(_ context.Context, p Policy, _ StartSpec) (Proc, error) {
	if !p.Isolates() {
		return nil, ErrNotIsolated
	}
	return nil, unavailable("unsupported OS %s", runtime.GOOS)
}

// Resize 实现 Proc（此平台永不会构造 process）。
func (p *process) Resize(PTYSize) error { return ErrPTYUnsupported }
