//go:build linux

package host

import (
	"context"
)

// Start 在沙盒里启动一个进程并保持句柄。语义与 Run 一致：隔离不可用时 fail-closed，
// 命令不会被执行；Enabled=false 或 danger-full-access 返回 ErrNotIsolated。
//
// 进程的生命周期由 ctx 控制：ctx 取消时杀整个进程组。Policy.Timeout 与 MaxOutputBytes 不适用于
// 长会话（输出走背压而不是截断），由调用方自己限制时长。
func Start(ctx context.Context, p Policy, spec StartSpec) (Proc, error) {
	c, pr, caps, err := prepareBwrapCmd(ctx, p, spec.Command, spec.PTY != nil)
	if err != nil {
		return nil, err
	}
	defer pr.Close()

	stdin, output, ptmx, releasers, closeParent, err := attachStdio(c, spec.PTY, false)
	if err != nil {
		return nil, err
	}
	defer closeParent()

	var closers []func()
	grp, _, err := prepareCgroup(p, caps)
	if err != nil {
		closeAll(closers)
		return nil, err
	}
	if err := c.Start(); err != nil {
		if grp != nil {
			grp.remove()
		}
		closeAll(releasers)
		return nil, unavailable("start bwrap: %v", err)
	}
	if grp != nil {
		_ = grp.addPID(c.Process.Pid)
		closers = append(closers, grp.remove)
	}
	proc := newProcess(c, stdin, output, ptmx, releasers, closers...)
	watchContextKillGroup(ctx, c.Process.Pid, proc.Done())
	return proc, nil
}
