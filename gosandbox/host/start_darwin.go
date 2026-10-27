//go:build darwin

// 文件说明：start_darwin.go — gosandbox/host 中 Start 等符号的实现
//
// 用法
//
//	Start(ctx context.Context, p Policy, spec StartSpec) (Proc, error)  // Start 在 Seatbelt 沙盒里启动一个进程并保持句柄
//	type ptyWriter（包内使用）
//	同包调用：ptmx → pty_darwin.go, pty_linux.go 等；Close() → pty_darwin.go, pty_linux.go 等；stdin → bwrap_args.go, pty_linux.go 等
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start_darwin.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, glob.go, mandatory.go, policy.go 等 13 个
//	[关键引用] ptmx ← pty_darwin.go, pty_linux.go, seatbelt.go
//
// 主要符号
//
//	Start(ctx context.Context, p Policy, spec StartSpec) (Proc, error)
//	... 另有 9 个顶层符号

package host

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// Start 在 Seatbelt 沙盒里启动一个进程并保持句柄。语义与 Linux 实现一致。
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
		// 新会话 + 控制终端：Setsid 让子进程成为会话首领，Setctty 把 Ctty 指向的 fd 设为控制终端。
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
		ptmx = master
		stdin = ptyWriter{master}
		output = master
		releasers = append(releasers, func() { master.Close() })
		defer slave.Close()
	} else {
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
	if err := c.Start(); err != nil {
		for _, f := range releasers {
			f()
		}
		return nil, unavailable("start sandbox-exec: %v", err)
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
