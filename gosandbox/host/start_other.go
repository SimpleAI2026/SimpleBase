//go:build !linux && !darwin

// 文件说明：start_other.go — gosandbox/host 中 Start 等符号的实现
//
// 用法
//
//	Start(_ context.Context, p Policy, _ StartSpec) (Proc, error)  // Start 在不支持的平台上 fail-closed
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start_other.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//
// 主要符号
//
//	Start(_ context.Context, p Policy, _ StartSpec) (Proc, error)
//	(p *process) Resize(PTYSize) error

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
