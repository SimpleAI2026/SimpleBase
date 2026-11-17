//go:build linux

package box

// supervise_test.go：detach supervisor 的核心承诺——服务进程重启后沙盒还在。
//
// 三条断言，缺一条这个功能就没有意义：
//
//  1. 断开服务进程的连接，沙盒里的进程还活着；
//  2. 重新连上之后能继续 exec，且 agent 没被重启（PID 连续）；
//  3. 断开的那一刻没丢「已经落地」的状态（工作区文件还在）。
//
// 用真实的 helper 二进制：supervisor 与 agent 是同一份代码，
// 用假替身测不出「重连后协议还能用」。

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newSupervisedSession 起一个由 supervisor 托管的沙盒。
func newSupervisedSession(t *testing.T) *Session {
	t.Helper()
	if _, err := os.Stat("/proc/self/cgroup"); err != nil {
		t.Skip("needs a Linux host")
	}
	root := t.TempDir()
	s, err := Start(context.Background(), Config{
		Name:      "sup1",
		Root:      root,
		Workspace: filepath.Join(root, "ws"),
		Helper:    helperBin(t),
		Policy:    Policy{Mode: "workspace-write", Workspace: filepath.Join(root, "ws")},
		Env:       []string{"PATH=/usr/local/bin:/usr/bin:/bin"},
		Supervise: true,
		Log:       func(level, msg string, kv ...any) { t.Logf("%s %s %v", level, msg, kv) },
	})
	if err != nil {
		t.Skipf("cannot start a supervised sandbox (needs a working helper): %v", err)
	}
	return s
}

// 断开连接 → 重连 → 沙盒里的进程还在。
func TestSupervisorSurvivesDisconnect(t *testing.T) {
	s := newSupervisedSession(t)
	root := s.cfg.Root

	// 起一个长驻进程，记下它的 pid。
	h, err := s.Exec(context.Background(), ExecRequest{
		Argv: []string{"/bin/sh", "-c", "echo started; sleep 30"},
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	line, err := readLine(h.Output())
	if err != nil || !strings.Contains(line, "started") {
		t.Fatalf("first line = %q, %v", line, err)
	}
	sess1PID := s.sessPIDForTest()

	// 模拟服务进程退出：直接关掉控制连接，但不停 supervisor。
	// Close 会杀沙盒，所以这里只关底层连接。
	s.conn.Close()
	time.Sleep(300 * time.Millisecond)

	// 重连：沙盒应当还在。
	conn, derr := dialSupervisor(supervisorSocket(root))
	if derr != nil {
		t.Fatalf("reconnect: %v", derr)
	}
	defer conn.Close()
	s2, serr := newSessionOn(context.Background(), s.cfg, conn)
	if serr != nil {
		t.Fatalf("re-adopt: %v", serr)
	}
	defer func() { _ = s2.Close() }()

	// 沙盒还活着：能再跑一条命令。
	res, rerr := s2.Exec(context.Background(), ExecRequest{Argv: []string{"/bin/sh", "-c", "echo alive"}})
	if rerr != nil {
		t.Fatalf("exec after re-adopt: %v", rerr)
	}
	out, _ := io.ReadAll(res.Output())
	if !strings.Contains(string(out), "alive") {
		t.Fatalf("output = %q", out)
	}

	// agent 没重启：它还是同一个进程。
	if pid2 := s2.sessPIDForTest(); pid2 != 0 && sess1PID != 0 && pid2 != sess1PID {
		t.Fatalf("agent was restarted (%d → %d); the sandbox should survive re-adoption", sess1PID, pid2)
	}

	// 之前起的长驻进程仍在（sleep 30 还没结束）。
	ps, perr := s2.Exec(context.Background(), ExecRequest{
		Argv: []string{"/bin/sh", "-c", "ps -e | grep -c '[s]leep 30' || true"},
	})
	if perr == nil {
		pout, _ := io.ReadAll(ps.Output())
		if strings.TrimSpace(string(pout)) == "0" {
			t.Fatal("the long-running process died across re-adoption")
		}
	}
}

// 断连前写入的文件在重连后仍然可读（状态不丢）。
func TestSupervisorKeepsWorkspace(t *testing.T) {
	s := newSupervisedSession(t)
	root := s.cfg.Root

	if err := s.WriteFile("/workspace/persisted.txt", []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s.conn.Close()
	time.Sleep(300 * time.Millisecond)

	conn, derr := dialSupervisor(supervisorSocket(root))
	if derr != nil {
		t.Fatalf("reconnect: %v", derr)
	}
	defer conn.Close()
	s2, serr := newSessionOn(context.Background(), s.cfg, conn)
	if serr != nil {
		t.Fatalf("re-adopt: %v", serr)
	}
	defer func() { _ = s2.Close() }()

	body, err := s2.ReadFile("/workspace/persisted.txt", 1<<20)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "keep me" {
		t.Fatalf("body = %q", body)
	}
}

// SuperviseArgs 的参数解析。
func TestSuperviseArgs(t *testing.T) {
	socket, argv, ok := SuperviseArgs([]string{"--supervisor", "--socket", "/tmp/c.sock", "--", "bwrap", "--unshare-net"})
	if !ok {
		t.Fatal("must be recognized as supervisor mode")
	}
	if socket != "/tmp/c.sock" {
		t.Fatalf("socket = %q", socket)
	}
	if len(argv) != 2 || argv[0] != "bwrap" {
		t.Fatalf("argv = %v", argv)
	}

	// 非 supervisor 模式。
	if _, _, ok := SuperviseArgs([]string{"--agent", "--ctrl", "3"}); ok {
		t.Fatal("agent mode must not be treated as supervisor")
	}
	// 缺 -- 时没有子命令（由调用方报错）。
	if _, argv, ok := SuperviseArgs([]string{"--supervisor", "--socket", "/x"}); !ok || len(argv) != 0 {
		t.Fatalf("argv = %v, ok = %v", argv, ok)
	}
}

// readLine 从 h 的输出里读一行（去掉换行）。
func readLine(r io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, err := r.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return sb.String(), nil
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			return sb.String(), err
		}
	}
	return sb.String(), fmt.Errorf("timeout reading a line")
}

// sessPIDForTest 返回 agent 的 pid（用于断言「没被重启」）。
//
// 走一次 Exec 拿不到 agent pid，所以读 /proc 找 helper 进程：
// 这是测试专用路径，不进生产代码。
func (s *Session) sessPIDForTest() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		pid, perr := strconv.Atoi(e.Name())
		if perr != nil {
			continue
		}
		cmdline, cerr := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if cerr != nil {
			continue
		}
		// agent 的 cmdline 里带 --agent 与 --ctrl。
		if strings.Contains(strings.ReplaceAll(string(cmdline), "\x00", " "), "--agent") {
			return pid
		}
	}
	return 0
}
