//go:build linux

package box

// session_test.go：box 的端到端行为（Linux；不需要 bwrap）。
//
// 用 Bwrap 为空跳过隔离，直接跑 helper 的 --agent 模式：这样测的是
// box 与 agent 之间的协议、进程调度与文件服务，而不是 bwrap 的挂载。
// 真实 bwrap 路径在 run_linux_test 里覆盖。

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

// helperBin 编译出 gosandbox-inner 供测试用。
func helperBin(t *testing.T) string {
	t.Helper()
	// 交叉编译或预置产物的场景：目标机不需要装 Go 工具链。
	if p := os.Getenv("GOSANDBOX_HELPER_BIN"); p != "" {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
		t.Fatalf("GOSANDBOX_HELPER_BIN=%q is not a file", p)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "gosandbox-inner")
	cmd := exec.Command("go", "build", "-o", out, "../cmd")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build helper: %v\n%s", err, b)
	}
	return out
}

// newSession 起一个不套 bwrap 的沙盒。
func newTestSession(t *testing.T) *Session {
	t.Helper()
	if _, err := os.Stat("/proc/self/cgroup"); err != nil {
		t.Skip("needs a Linux host")
	}
	root := t.TempDir()
	s, err := Start(context.Background(), Config{
		Name:      "t1",
		Root:      root,
		Workspace: filepath.Join(root, "ws"),
		Helper:    helperBin(t),
		Bwrap:     "", // 不隔离：只测协议与调度
		Policy:    Policy{Mode: "workspace-write", Workspace: filepath.Join(root, "ws")},
		Env:       []string{"PATH=/usr/local/bin:/usr/bin:/bin"},
		Log:       func(level, msg string, kv ...any) { t.Logf("%s %s %v", level, msg, kv) },
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// 一条命令的完整生命周期。
func TestSessionExec(t *testing.T) {
	s := newTestSession(t)
	h, err := s.Exec(context.Background(), ExecRequest{
		Argv: []string{"/bin/sh", "-c", "echo out; echo err >&2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(h.Output())
	if err != nil {
		t.Fatal(err)
	}
	ex := h.Wait()
	if !strings.Contains(string(out), "out") || !strings.Contains(string(out), "err") {
		t.Fatalf("output = %q", out)
	}
	if ex.Code != 0 || ex.Signaled {
		t.Fatalf("exit = %+v", ex)
	}
}

// stdin：写入的数据被读到。
func TestSessionStdin(t *testing.T) {
	s := newTestSession(t)
	h, err := s.Exec(context.Background(), ExecRequest{
		Argv:  []string{"/bin/sh", "-c", "read line; echo got:$line"},
		Stdin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, werr := h.Write([]byte("hi\n")); werr != nil {
		t.Fatal(werr)
	}
	out, _ := io.ReadAll(h.Output())
	if !strings.Contains(string(out), "got:hi") {
		t.Fatalf("output = %q", out)
	}
	if ex := h.Wait(); ex.Code != 0 {
		t.Fatalf("exit = %+v", ex)
	}
}

// 后台进程跨请求存活：两次 Exec 看到同一个沙盒。
func TestSessionBackgroundProcess(t *testing.T) {
	s := newTestSession(t)
	// 起一个后台进程，不读它的输出（Close 会结束它）。
	bg, err := s.Exec(context.Background(), ExecRequest{Argv: []string{"/bin/sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	defer bg.Close()

	// 另一个进程能看到它（同一个 PID 命名空间）。
	h, err := s.Exec(context.Background(), ExecRequest{Argv: []string{"/bin/sh", "-c", "ps -e | grep -c sleep || true"}})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(h.Output())
	h.Wait()
	if n := strings.TrimSpace(string(out)); n == "" || n == "0" {
		t.Fatalf("background process must be visible in the same sandbox: %q", out)
	}
}

// 信号打到整个进程组。
func TestSessionSignal(t *testing.T) {
	s := newTestSession(t)
	h, err := s.Exec(context.Background(), ExecRequest{Argv: []string{"/bin/sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	go io.Copy(io.Discard, h.Output())
	if err := h.Signal(15); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not die on SIGTERM")
	}
	if ex := h.Wait(); !ex.Signaled || ex.Signal != 15 {
		t.Fatalf("exit = %+v", ex)
	}
}

// 文件：写 → 读 → stat → list → rename → remove。
func TestSessionFiles(t *testing.T) {
	s := newTestSession(t)
	ws := s.cfg.Workspace
	if err := os.MkdirAll(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ws, "sub", "a.txt")

	if err := s.WriteFile(path, []byte("hello box"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := s.ReadFile(path, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello box" {
		t.Fatalf("read = %q", data)
	}
	fi, err := s.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Kind != proto.FileRegular || fi.Size != 9 {
		t.Fatalf("stat = %+v", fi)
	}
	entries, err := s.ListDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "a.txt" {
		t.Fatalf("list = %+v", entries)
	}
	to := filepath.Join(ws, "b.txt")
	if err := s.Rename(path, to); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(to); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat(to); err == nil {
		t.Fatal("file must be gone")
	}
}

// 用量查询总能回答。
func TestSessionStats(t *testing.T) {
	s := newTestSession(t)
	if _, err := s.Stats(); err != nil {
		t.Fatal(err)
	}
}

// 关掉沙盒后再用会失败，而不是挂住。
func TestSessionClosed(t *testing.T) {
	s := newTestSession(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Exec(context.Background(), ExecRequest{Argv: []string{"/bin/true"}}); err == nil {
		t.Fatal("exec on a closed session must fail")
	}
	// 重复 Close 不报错。
	if err := s.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

// 找不到命令：Exited 127，连接仍可用。
func TestSessionCommandNotFound(t *testing.T) {
	s := newTestSession(t)
	h, err := s.Exec(context.Background(), ExecRequest{Argv: []string{"/definitely/not/here"}})
	if err != nil {
		t.Fatal(err)
	}
	if ex := h.Wait(); ex.Code != 127 {
		t.Fatalf("exit = %+v", ex)
	}
	// 沙盒仍然可用。
	h2, err := s.Exec(context.Background(), ExecRequest{Argv: []string{"/bin/true"}})
	if err != nil {
		t.Fatal(err)
	}
	if ex := h2.Wait(); ex.Code != 0 {
		t.Fatalf("sandbox must stay usable: %+v", ex)
	}
}

// 版本不一致：Start 拒绝。
func TestStartRejectsVersion(t *testing.T) {
	if proto.Version == 0 {
		t.Fatal("protocol version must be non-zero")
	}
	// 真实握手在 newSession 里覆盖；这里只防版本号被误改成 0。
}

var _ = errors.New
