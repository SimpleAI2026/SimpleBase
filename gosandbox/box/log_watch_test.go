//go:build linux

package box

// log_watch_test.go：日志落盘回放与目录监听的端到端行为。
//
// 用 Bwrap 为空跳过隔离（只测协议与调度），与 session_test.go 同一套路。
// 监听走真实 inotify：这些用例只在 Linux 上有意义。

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// newLoggedSession 起一个带日志目录的沙盒。
func newLoggedSession(t *testing.T) *Session {
	t.Helper()
	root := t.TempDir()
	s, err := Start(context.Background(), Config{
		Name:      "log1",
		Root:      root,
		Workspace: filepath.Join(root, "ws"),
		Helper:    helperBin(t),
		Policy:    Policy{Mode: "workspace-write", Workspace: filepath.Join(root, "ws")},
		Env:       []string{"PATH=/usr/local/bin:/usr/bin:/bin"},
		LogDir:    filepath.Join(root, "logs"),
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := os.MkdirAll(s.cfg.Workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	return s
}

// 进程输出落盘，退出后仍能回放。
func TestSessionLogsPersist(t *testing.T) {
	s := newLoggedSession(t)
	h, err := s.Exec(context.Background(), ExecRequest{
		Argv: []string{"/bin/sh", "-c", "echo first; echo second >&2; echo third"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(h.Output())
	if ex := h.Wait(); ex.Code != 0 {
		t.Fatalf("exit = %+v", ex)
	}
	_ = h.Close()

	// 进程已退出，日志仍可回放。
	recs, err := s.Logs(nil, TailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	streams := map[Stream]bool{}
	for _, r := range recs {
		all.Write(r.Data)
		streams[r.Stream] = true
	}
	text := all.String()
	for _, want := range []string{"first", "second", "third"} {
		if !strings.Contains(text, want) {
			t.Errorf("log missing %q: %q", want, text)
		}
	}
	if !streams[StreamStdout] || !streams[StreamStderr] {
		t.Errorf("both streams must be recorded: %v", streams)
	}
}

// 按行数截取。
func TestSessionLogsTail(t *testing.T) {
	s := newLoggedSession(t)
	h, err := s.Exec(context.Background(), ExecRequest{
		Argv: []string{"/bin/sh", "-c", "echo 1; echo 2; echo 3; echo 4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(h.Output())
	h.Wait()

	recs, err := s.Logs(nil, TailOptions{Lines: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("records = %d", len(recs))
	}
}

// 没有日志目录时 Logs 明确报错，而不是返回空。
func TestSessionLogsWithoutDir(t *testing.T) {
	s := newTestSession(t) // 不带 LogDir
	if _, err := s.Logs(nil, TailOptions{}); err == nil {
		t.Fatal("must report that logs are not kept")
	}
}

// 目录监听：新建文件产生事件。
func TestSessionWatch(t *testing.T) {
	s := newLoggedSession(t)
	ws := s.cfg.Workspace
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w, err := s.Watch(ctx, ws, WatchOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// inotify 注册与文件创建之间有竞态，稍等一下再写。
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(ws, "created.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	var got []string
	for {
		select {
		case ev := <-w.Events():
			got = append(got, filepath.Base(ev.Path))
			if strings.Contains(strings.Join(got, ","), "created.txt") {
				return
			}
		case <-deadline:
			t.Fatalf("no event for created.txt (got %v)", got)
		case <-ctx.Done():
			t.Fatalf("watch timed out (got %v)", got)
		}
	}
}

// 子目录里新建的文件也能看到（递归）。
func TestSessionWatchRecursive(t *testing.T) {
	s := newLoggedSession(t)
	ws := s.cfg.Workspace
	sub := filepath.Join(ws, "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	w, err := s.Watch(ctx, ws, WatchOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(sub, "deep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-w.Events():
			if filepath.Base(ev.Path) == "deep.txt" {
				return
			}
		case <-deadline:
			t.Fatal("no event for a file created in a subdirectory")
		}
	}
}

// 非递归时子目录里的变化不报。
func TestSessionWatchNotRecursive(t *testing.T) {
	s := newLoggedSession(t)
	ws := s.cfg.Workspace
	sub := filepath.Join(ws, "norec")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w, err := s.Watch(ctx, ws, WatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(100 * time.Millisecond)
	_ = os.WriteFile(filepath.Join(sub, "hidden.txt"), []byte("x"), 0o644)
	// 顶层的文件应该报。
	_ = os.WriteFile(filepath.Join(ws, "top.txt"), []byte("x"), 0o644)

	deadline := time.After(3 * time.Second)
	sawTop := false
	for {
		select {
		case ev := <-w.Events():
			switch filepath.Base(ev.Path) {
			case "top.txt":
				sawTop = true
			case "hidden.txt":
				t.Fatal("non-recursive watch must not report subdirectory files")
			}
		case <-deadline:
			if !sawTop {
				t.Fatal("top-level file must be reported")
			}
			return
		}
	}
}

// Close 之后通道关闭，不 panic。
func TestWatchClose(t *testing.T) {
	s := newLoggedSession(t)
	ctxx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w, err := s.Watch(ctxx, s.cfg.Workspace, WatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// 通道已关闭：读会立刻返回零值。
	select {
	case _, ok := <-w.Events():
		if ok {
			t.Fatal("events channel must be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("events channel must close promptly")
	}
	// 重复 Close 不报错。
	if err := w.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

// 事件类型：新建目录报 FileDir。
func TestWatchEventKind(t *testing.T) {
	s := newLoggedSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	w, err := s.Watch(ctx, s.cfg.Workspace, WatchOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(100 * time.Millisecond)
	if err := os.Mkdir(filepath.Join(s.cfg.Workspace, "newdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-w.Events():
			if filepath.Base(ev.Path) == "newdir" {
				if ev.Kind != proto.FileDir {
					t.Fatalf("kind = %v, want dir", ev.Kind)
				}
				return
			}
		case <-deadline:
			t.Fatal("no event for the new directory")
		}
	}
}
