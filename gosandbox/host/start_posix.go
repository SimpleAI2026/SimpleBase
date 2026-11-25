//go:build linux || darwin

// 文件说明：start_posix.go — gosandbox/host 的内部实现（killGroup、exitInfo 等）
//
// 用法
//
//	同包调用：attachStdio() → start_darwin.go, start_linux.go；watchContextKillGroup() → start_darwin.go, start_linux.go；closeAll() → start_darwin.go, start_linux.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/box, gosandbox/vm/box 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/start_posix.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] bwrap_args.go, bwrap_args_extra_test.go, cgroup.go, cgroup_linux.go, cgroup_linux_test.go 等 36 个
//	[关键引用] attachStdio ← start_darwin.go, start_linux.go；watchContextKillGroup ← start_darwin.go, start_linux.go；closeAll ← start_darwin.go, start_linux.go
//
// 主要符号
//
//	killGroup(pid int, sig syscall.Signal) error
//	exitInfo(st *os.ProcessState, waitErr error) ExitInfo
//	attachStdio(c *exec.Cmd, pty *PTYSize, setDarwinAttr bool) ( stdin io.WriteCloser, output io.Reader, ptmx *os....
//	watchContextKillGroup(ctx context.Context, pid int, done <-chan struct{})
//	closeAll(fs []func())
//	type ptyWriter struct{…}
//	(w ptyWriter) Write(b []byte) (int, error)
//	(w ptyWriter) Close() error
//	... 另有 1 个顶层符号

package host

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// killGroup 向整个进程组发信号（子进程以 Setpgid 启动，pgid == pid）。
func killGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}

func exitInfo(st *os.ProcessState, waitErr error) ExitInfo {
	if ws, ok := st.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return ExitInfo{Signaled: true, ExitCode: 128 + int(ws.Signal())}
		}
		return ExitInfo{ExitCode: ws.ExitStatus()}
	}
	if waitErr != nil {
		return ExitInfo{ExitCode: -1}
	}
	return ExitInfo{}
}

// attachStdio 为长会话进程绑定伪终端或合并管道。
// setDarwinAttr 仅在 macOS 下为真：PTY 模式设 Setsid+Setctty，管道模式设 Setpgid
// （Linux 下由 bwrap --new-session 与 helper --ctty 负责控制终端，不能再设 Setsid）。
func attachStdio(c *exec.Cmd, pty *PTYSize, setDarwinAttr bool) (
	stdin io.WriteCloser,
	output io.Reader,
	ptmx *os.File,
	releasers []func(),
	closeParent func(),
	err error,
) {
	if pty != nil {
		master, slave, perr := openPTY(*pty)
		if perr != nil {
			return nil, nil, nil, nil, nil, unavailable("open pty: %v", perr)
		}
		c.Stdin, c.Stdout, c.Stderr = slave, slave, slave
		if setDarwinAttr {
			c.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
		}
		return ptyWriter{master}, master, master, []func(){func() { master.Close() }}, func() { slave.Close() }, nil
	}
	if setDarwinAttr {
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	pipeR, pipeW, perr := os.Pipe()
	if perr != nil {
		return nil, nil, nil, nil, nil, unavailable("pipe: %v", perr)
	}
	inR, inW, perr := os.Pipe()
	if perr != nil {
		pipeR.Close()
		pipeW.Close()
		return nil, nil, nil, nil, nil, unavailable("pipe: %v", perr)
	}
	c.Stdin, c.Stdout, c.Stderr = inR, pipeW, pipeW
	return inW, pipeR, nil,
		[]func(){func() { pipeR.Close(); inW.Close() }},
		func() { inR.Close(); pipeW.Close() },
		nil
}

func watchContextKillGroup(ctx context.Context, pid int, done <-chan struct{}) {
	go func() {
		select {
		case <-ctx.Done():
			_ = killGroup(pid, syscall.SIGKILL)
		case <-done:
		}
	}()
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
