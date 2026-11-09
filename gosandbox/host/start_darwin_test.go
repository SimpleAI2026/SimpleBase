//go:build darwin

package host

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// drain 在后台读取 Output，返回当前累计内容的读取函数。
func drain(p Proc) func() string {
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

func waitFor(t *testing.T, get func() string, want string) {
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

func TestDarwinStartPipe(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	proc, err := Start(context.Background(), p, StartSpec{Command: Command{
		Argv: []string{"/bin/sh", "-c", "echo out; echo err 1>&2; cat"}, Dir: ws,
		Env: []string{"PATH=/usr/bin:/bin"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	get := drain(proc)
	waitFor(t, get, "out")
	waitFor(t, get, "err") // stderr 与 stdout 合并为一路
	if _, err := io.WriteString(proc.Stdin(), "hello\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, get, "hello")
	proc.Stdin().Close() // 管道模式：EOF 让 cat 退出
	if !WaitTimeout(proc, 5*time.Second) {
		t.Fatal("process did not exit after stdin EOF")
	}
	if info := proc.Wait(); info.ExitCode != 0 {
		t.Fatalf("exit=%+v", info)
	}
	if err := proc.Resize(PTYSize{Cols: 80, Rows: 24}); err != ErrNotPTY {
		t.Fatalf("Resize on pipe = %v, want ErrNotPTY", err)
	}
}

func TestDarwinStartPTY(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	proc, err := Start(context.Background(), p, StartSpec{
		PTY: &PTYSize{Cols: 100, Rows: 30},
		Command: Command{Argv: []string{"/bin/sh"}, Dir: ws,
			Env: []string{"PATH=/usr/bin:/bin", "TERM=xterm"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	get := drain(proc)
	io.WriteString(proc.Stdin(), "stty size\n")
	waitFor(t, get, "30 100")
	if err := proc.Resize(PTYSize{Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	io.WriteString(proc.Stdin(), "stty size\n")
	waitFor(t, get, "40 120")
	io.WriteString(proc.Stdin(), "test -t 0 && echo TTY_YES\n")
	waitFor(t, get, "TTY_YES")
	io.WriteString(proc.Stdin(), "exit 3\n")
	if !WaitTimeout(proc, 5*time.Second) {
		t.Fatal("shell did not exit")
	}
	if info := proc.Wait(); info.ExitCode != 3 {
		t.Fatalf("exit=%+v", info)
	}
}

// Close 要杀掉整个进程组，包括后台子进程。
func TestDarwinStartCloseKillsGroup(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	proc, err := Start(context.Background(), p, StartSpec{Command: Command{
		Argv: []string{"/bin/sh", "-c", "sleep 60 & sleep 60 & wait"}, Dir: ws, Env: []string{"PATH=/usr/bin:/bin"}}})
	if err != nil {
		t.Fatal(err)
	}
	drain(proc)
	time.Sleep(300 * time.Millisecond)
	pid := proc.PID()
	proc.Close()
	if !WaitTimeout(proc, 5*time.Second) {
		t.Fatal("not done after Close")
	}
	time.Sleep(200 * time.Millisecond)
	if err := syscall.Kill(-pid, 0); err == nil {
		t.Fatalf("process group %d still alive after Close", pid)
	}
}

func TestDarwinStartSignalRestricted(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	proc, err := Start(context.Background(), p, StartSpec{Command: Command{
		Argv: []string{"/bin/sh", "-c", "sleep 30"}, Dir: ws, Env: []string{"PATH=/usr/bin:/bin"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	drain(proc)
	if err := proc.Signal(syscall.SIGHUP); err != ErrUnsupportedSignal {
		t.Fatalf("SIGHUP = %v, want ErrUnsupportedSignal", err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if !WaitTimeout(proc, 5*time.Second) {
		t.Fatal("not terminated by SIGTERM")
	}
	if info := proc.Wait(); !info.Signaled {
		t.Fatalf("want signaled, got %+v", info)
	}
}

// ctx 取消等价于 SIGKILL。
func TestDarwinStartContextCancel(t *testing.T) {
	p, ws := darwinPolicy(t, ModeWorkspaceWrite)
	ctx, cancel := context.WithCancel(context.Background())
	proc, err := Start(ctx, p, StartSpec{Command: Command{
		Argv: []string{"/bin/sh", "-c", "sleep 30"}, Dir: ws, Env: []string{"PATH=/usr/bin:/bin"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	drain(proc)
	cancel()
	if !WaitTimeout(proc, 5*time.Second) {
		t.Fatal("not killed after ctx cancel")
	}
}

// 隔离在 PTY 模式下同样生效：工作区外写不进去。
func TestDarwinStartPTYIsolated(t *testing.T) {
	p, ws := darwinPolicy(t, ModeReadOnly)
	proc, err := Start(context.Background(), p, StartSpec{
		PTY:     &PTYSize{Cols: 80, Rows: 24},
		Command: Command{Argv: []string{"/bin/sh"}, Dir: ws, Env: []string{"PATH=/usr/bin:/bin"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	get := drain(proc)
	// 关掉回显，否则断言会匹配到命令自身的回显。
	io.WriteString(proc.Stdin(), "stty -echo\n")
	time.Sleep(150 * time.Millisecond)
	io.WriteString(proc.Stdin(), "rm -f "+ws+"/nope; echo x > "+ws+"/nope 2>/dev/null && echo WROTE_OK")
	io.WriteString(proc.Stdin(), " || echo BLOCKED_OK\n")
	waitFor(t, get, "BLOCKED_OK")
	if strings.Contains(get(), "WROTE_OK") {
		t.Fatal("read-only mode allowed a write in PTY session")
	}
	if _, err := os.Stat(ws + "/nope"); err == nil {
		t.Fatal("file was created outside the read-only policy")
	}
}

func TestStartNotIsolated(t *testing.T) {
	if _, err := Start(context.Background(), Policy{}, StartSpec{Command: Command{Argv: []string{"/bin/true"}}}); err != ErrNotIsolated {
		t.Fatalf("got %v, want ErrNotIsolated", err)
	}
}
