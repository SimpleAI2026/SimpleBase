package fake

import (
	"bytes"
	"context"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"

	sb "github.com/linkxzhou/SimpleBase/gosandbox/vm"
)

func readUntil(t *testing.T, r io.Reader, want string) string {
	t.Helper()
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		b := make([]byte, 256)
		for !strings.Contains(buf.String(), want) {
			n, err := r.Read(b)
			buf.Write(b[:n])
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for %q, got %q", want, buf.String())
	}
	return buf.String()
}

func tgt() sb.Target { return sb.Target{Name: "f1", Spec: sb.Spec{Workdir: "/workspace"}} }

func TestInteractiveShell(t *testing.T) {
	d := New()
	p, err := d.Start(context.Background(), tgt(), sb.StartSpec{PTY: &sb.PTYSize{Cols: 80, Rows: 24}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	io.WriteString(p.Stdin(), "echo hi\n")
	got := readUntil(t, p.Output(), "hi\r\n")
	if !strings.Contains(got, "echo hi\r\n") {
		t.Fatalf("pty must echo input: %q", got)
	}
	if err := p.Resize(sb.PTYSize{Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	if s := p.(*fakeProc).Size(); s.Cols != 120 || s.Rows != 40 {
		t.Fatalf("size = %+v", s)
	}
	io.WriteString(p.Stdin(), "exit 7\n")
	select {
	case <-p.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("did not exit")
	}
	if e := p.Wait(); e.ExitCode != 7 || e.Signaled {
		t.Fatalf("exit = %+v", e)
	}
}

func TestInteractivePipeRejectsResize(t *testing.T) {
	d := New()
	p, _ := d.Start(context.Background(), tgt(), sb.StartSpec{})
	defer p.Close()
	if err := p.Resize(sb.PTYSize{Cols: 1, Rows: 1}); err == nil {
		t.Fatal("pipe mode must reject Resize")
	}
}

func TestInteractiveOneShotCommand(t *testing.T) {
	d := New()
	p, _ := d.Start(context.Background(), tgt(), sb.StartSpec{Shell: "echo one; exit 3"})
	defer p.Close()
	readUntil(t, p.Output(), "one")
	if e := p.Wait(); e.ExitCode != 3 {
		t.Fatalf("exit = %+v", e)
	}
}

func TestInteractiveSignalAndClose(t *testing.T) {
	d := New()
	p, _ := d.Start(context.Background(), tgt(), sb.StartSpec{})
	if err := p.Signal(syscall.SIGHUP); err == nil {
		t.Fatal("SIGHUP must be rejected")
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if e := p.Wait(); !e.Signaled || e.ExitCode != 128+int(syscall.SIGTERM) {
		t.Fatalf("exit = %+v", e)
	}
	p.Close() // 重复 Close 安全
}

func TestInteractiveContextCancelAndFiles(t *testing.T) {
	d := New()
	ctx, cancel := context.WithCancel(context.Background())
	p, _ := d.Start(ctx, tgt(), sb.StartSpec{})
	io.WriteString(p.Stdin(), "echo data > a.txt\n")
	time.Sleep(50 * time.Millisecond)
	b, err := d.ReadFile(context.Background(), tgt(), "/workspace/a.txt")
	if err != nil || string(b) != "data\n" {
		t.Fatalf("file not shared with driver state: %q %v", b, err)
	}
	cancel()
	p.Close()
}

// cmd 是 shell 时进入交互循环（E2B 的 pty.create 就是启动 /bin/bash）。
func TestInteractiveShellBinaryStaysAlive(t *testing.T) {
	d := New()
	for _, bin := range []string{"/bin/sh", "bash"} {
		p, err := d.Start(context.Background(), tgt(), sb.StartSpec{Cmd: bin, PTY: &sb.PTYSize{Cols: 80, Rows: 24}})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-p.Done():
			t.Fatalf("%s must not exit on its own", bin)
		case <-time.After(150 * time.Millisecond):
		}
		io.WriteString(p.Stdin(), "echo alive\n")
		readUntil(t, p.Output(), "alive")
		p.Close()
	}
	// 非 shell 的命令仍是一次性。
	p, _ := d.Start(context.Background(), tgt(), sb.StartSpec{Cmd: "echo", Args: []string{"once"}})
	defer p.Close()
	readUntil(t, p.Output(), "once")
	select {
	case <-p.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("a one-shot command must exit")
	}
}
