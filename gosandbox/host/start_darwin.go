//go:build darwin

// 文件说明：start_darwin.go — gosandbox/host 中 Start 等符号的实现
//
// 用法
//
//	Start(ctx context.Context, p Policy, spec StartSpec) (Proc, error)  // Start 在 Seatbelt 沙盒里启动一个进程并保持句柄
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start_darwin.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//
// 主要符号
//
//	Start(ctx context.Context, p Policy, spec StartSpec) (Proc, error)

package host

import (
	"context"
	"errors"
	"os/exec"
)

// Start 在 Seatbelt 沙盒里启动一个进程并保持句柄。语义与 Linux 实现一致；
// 配置了限额时同样由采样监控执行，进程结束后监控随之停止。
func Start(ctx context.Context, p Policy, spec StartSpec) (Proc, error) {
	if !p.Isolates() {
		return nil, ErrNotIsolated
	}
	if _, err := Probe(ctx, p); err != nil {
		return nil, err
	}
	if len(spec.Command.Argv) == 0 {
		return nil, errors.New("host: empty argv")
	}
	args, err := SeatbeltArgs(p, spec.Command.Argv)
	if err != nil {
		return nil, err
	}
	c := exec.Command(SeatbeltExecutable, args...)
	c.Dir = spec.Command.Dir
	if c.Dir == "" {
		c.Dir = p.Workspace
	}
	c.Env = spec.Command.Env

	stdin, output, ptmx, releasers, closeParent, err := attachStdio(c, spec.PTY, true)
	if err != nil {
		return nil, err
	}
	defer closeParent()

	if err := c.Start(); err != nil {
		closeAll(releasers)
		return nil, unavailable("start sandbox-exec: %v", err)
	}
	var closers []func()
	if mon := startLimitMonitor(c.Process.Pid, p); mon != nil {
		closers = append(closers, func() { mon.stop() })
	}
	proc := newProcess(c, stdin, output, ptmx, releasers, closers...)
	watchContextKillGroup(ctx, c.Process.Pid, proc.Done())
	return proc, nil
}
