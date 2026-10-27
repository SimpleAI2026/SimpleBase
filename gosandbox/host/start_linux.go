//go:build linux

// 文件说明：start_linux.go — gosandbox/host 中 Start 等符号的实现
//
// 用法
//
//	Start(ctx context.Context, p Policy, spec StartSpec) (Proc, error)  // Start 在沙盒里启动一个进程并保持句柄
//	type ptyWriter（包内使用）
//	同包调用：ptmx → pty_darwin.go, pty_linux.go 等；Close() → pty_darwin.go, pty_linux.go 等；stdin → bwrap_args.go, pty_linux.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start_linux.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 14 个
//	[关键引用] ptmx ← pty_darwin.go, pty_linux.go, seatbelt.go
//
// 主要符号
//
//	Start(ctx context.Context, p Policy, spec StartSpec) (Proc, error)
//	... 另有 10 个顶层符号

package host

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// Start 在沙盒里启动一个进程并保持句柄。语义与 Run 一致：隔离不可用时 fail-closed，
// 命令不会被执行；Enabled=false 或 danger-full-access 返回 ErrNotIsolated。
//
// 进程的生命周期由 ctx 控制：ctx 取消时杀整个进程组。Policy.Timeout 与 MaxOutputBytes 不适用于
// 长会话（输出走背压而不是截断），由调用方自己限制时长。
func Start(ctx context.Context, p Policy, spec StartSpec) (Proc, error) {
	if !p.Isolates() {
		return nil, ErrNotIsolated
	}
	caps, err := Probe(ctx, p)
	if err != nil {
		return nil, err
	}
	helper, err := resolveHelper(p.HelperPath)
	if err != nil {
		return nil, err
	}
	argv := spec.Command.Argv
	if len(argv) == 0 {
		return nil, errors.New("host: empty argv")
	}
	dir := spec.Command.Dir
	if dir == "" {
		dir = p.Workspace
	}
	args, err := BwrapArgs(BwrapInput{Policy: p, Dir: dir, Helper: helper, Argv: argv,
		PolicyFD: 3, DisableUserNS: caps.DisableUserNS, CTTY: spec.PTY != nil})
	if err != nil {
		return nil, err
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, unavailable("pipe: %v", err)
	}
	defer pr.Close()
	body, _ := json.Marshal(innerSpec(p))
	if _, err := pw.Write(body); err != nil {
		pw.Close()
		return nil, unavailable("write policy: %v", err)
	}
	pw.Close()

	c := exec.Command(caps.BwrapPath, args...)
	c.Env = spec.Command.Env
	c.ExtraFiles = []*os.File{pr}
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var (
		stdin     io.WriteCloser
		output    io.Reader
		ptmx      *os.File
		closers   []func()
		releasers []func()
	)
	if spec.PTY != nil {
		master, slave, perr := openPTY(*spec.PTY)
		if perr != nil {
			return nil, unavailable("open pty: %v", perr)
		}
		c.Stdin, c.Stdout, c.Stderr = slave, slave, slave
		// 子进程必须在新会话里把 PTY 从端当作控制终端：bwrap 的 --new-session 负责 setsid，
		// helper 的 --ctty 负责 TIOCSCTTY。此处不能再设 Setsid（与 Setpgid 互斥）。
		ptmx = master
		stdin = ptyWriter{master}
		output = master
		releasers = append(releasers, func() { master.Close() })
		defer slave.Close()
	} else {
		pipeR, pipeW, perr := os.Pipe()
		if perr != nil {
			return nil, unavailable("pipe: %v", perr)
		}
		inR, inW, perr := os.Pipe()
		if perr != nil {
			pipeR.Close()
			pipeW.Close()
			return nil, unavailable("pipe: %v", perr)
		}
		c.Stdin, c.Stdout, c.Stderr = inR, pipeW, pipeW
		stdin, output = inW, pipeR
		releasers = append(releasers, func() { pipeR.Close(); inW.Close() })
		defer func() { inR.Close(); pipeW.Close() }()
	}

	var grp *cgroupGroup
	limits := limitsConfigured(p)
	if limits && caps.Cgroup == cgroupDelegated {
		if g, gerr := newCgroupGroup(); gerr == nil {
			if aerr := g.apply(p); aerr != nil {
				g.remove()
				closeAll(closers)
				return nil, unavailable("apply cgroup limits: %v", aerr)
			}
			grp = g
		}
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
	go func() {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		case <-proc.Done():
		}
	}()
	return proc, nil
}

func closeAll(fs []func()) {
	for _, f := range fs {
		f()
	}
}

// ptyWriter 让 Close 在 PTY 模式下是空操作：主端同时承担读写，关闭它会终止输出。
type ptyWriter struct{ f *os.File }

func (w ptyWriter) Write(b []byte) (int, error) { return w.f.Write(b) }
func (w ptyWriter) Close() error                { return nil }

// Resize 实现 Proc。
func (p *process) Resize(size PTYSize) error {
	if p.ptmx == nil {
		return ErrNotPTY
	}
	return setWinsize(p.ptmx, size)
}
