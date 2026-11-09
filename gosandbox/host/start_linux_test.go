//go:build linux

package host

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// 这些用例调用真 bwrap：没有 bwrap、用户命名空间被禁用或 helper 构建失败时自动 Skip。

func drainLinux(p Proc) func() string {
	var mu sync.Mutex
	var buf bytes.Buffer
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := p.Output().Read(b)
			if n > 0 {
				mu.Lock()
				buf.Write(b[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return func() string { mu.Lock(); defer mu.Unlock(); return buf.String() }
}

func waitForLinux(t *testing.T, get func() string, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(get(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("output never contained %q; got %q", want, get())
}

func TestLinuxStartPipe(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	proc, err := Start(context.Background(), p, StartSpec{Command: Command{
		Argv: []string{"/bin/sh", "-c", "echo out; echo err 1>&2; cat"}, Dir: ws, Env: []string{"PATH=/usr/bin:/bin"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	get := drainLinux(proc)
	waitForLinux(t, get, "out")
	waitForLinux(t, get, "err")
	io.WriteString(proc.Stdin(), "hello\n")
	waitForLinux(t, get, "hello")
	proc.Stdin().Close()
	if !WaitTimeout(proc, 5*time.Second) {
		t.Fatal("process did not exit after stdin EOF")
	}
}

// PTY 内有控制终端：tty 有输出、stty size 与 Resize 一致。
func TestLinuxStartPTY(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	proc, err := Start(context.Background(), p, StartSpec{
		PTY: &PTYSize{Cols: 100, Rows: 30},
		Command: Command{Argv: []string{"/bin/sh"}, Dir: ws,
			Env: []string{"PATH=/usr/bin:/bin", "TERM=xterm"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	get := drainLinux(proc)
	io.WriteString(proc.Stdin(), "stty size\n")
	waitForLinux(t, get, "30 100")
	if err := proc.Resize(PTYSize{Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	io.WriteString(proc.Stdin(), "stty size\n")
	waitForLinux(t, get, "40 120")
	io.WriteString(proc.Stdin(), "tty\n")
	waitForLinux(t, get, "/dev/")
	io.WriteString(proc.Stdin(), "exit 3\n")
	if !WaitTimeout(proc, 5*time.Second) {
		t.Fatal("shell did not exit")
	}
	if info := proc.Wait(); info.ExitCode != 3 {
		t.Fatalf("exit=%+v", info)
	}
}

// PTY 会话里隔离依然生效：根只读。
func TestLinuxStartPTYIsolated(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	proc, err := Start(context.Background(), p, StartSpec{
		PTY:     &PTYSize{Cols: 80, Rows: 24},
		Command: Command{Argv: []string{"/bin/sh"}, Dir: ws, Env: []string{"PATH=/usr/bin:/bin"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	get := drainLinux(proc)
	io.WriteString(proc.Stdin(), "stty -echo\n")
	time.Sleep(150 * time.Millisecond)
	io.WriteString(proc.Stdin(), "touch /etc/sb-pty-escape && echo WROTE_OK")
	io.WriteString(proc.Stdin(), " || echo BLOCKED_OK\n")
	waitForLinux(t, get, "BLOCKED_OK")
	if strings.Contains(get(), "WROTE_OK") {
		t.Fatal("PTY session wrote to a read-only root")
	}
}

// Close 杀掉整个进程组，包括后台子进程。
func TestLinuxStartCloseKillsTree(t *testing.T) {
	p, ws := linuxPolicy(t, ModeWorkspaceWrite)
	proc, err := Start(context.Background(), p, StartSpec{Command: Command{
		Argv: []string{"/bin/sh", "-c", "sleep 60 & sleep 60 & wait"}, Dir: ws, Env: []string{"PATH=/usr/bin:/bin"}}})
	if err != nil {
		t.Fatal(err)
	}
	drainLinux(proc)
	time.Sleep(300 * time.Millisecond)
	pid := proc.PID()
	proc.Close()
	time.Sleep(200 * time.Millisecond)
	if err := syscall.Kill(-pid, 0); err == nil {
		t.Fatalf("process group %d still alive after Close", pid)
	}
}
