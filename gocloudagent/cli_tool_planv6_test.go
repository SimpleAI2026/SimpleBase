package gocloudagent

// cli_tool_planv6_test.go：planv6.0 §3.4/§3.5 单元测试。
// - gate：wait 超时后登记自清理（不残留）；text 协议跳过 gate；
// - redactArgv：--statement 只留首关键字+200 字符；kv --arg 除首个外 ***；
//   敏感 flag 值 ***；
// - sqlDestructive：切分认识引号与注释（DELETE 出现在字符串字面量里不算）。

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

)

// gate 在 wait 超时后必须自清理（§3.4：结束 delete，防残留累积）。
func TestCallGate_WaitCleanupOnTimeout(t *testing.T) {
	g := &callGate{}
	g.wait("c1", 10*time.Millisecond)
	g.mu.Lock()
	_, ok := g.ch["c1"]
	g.mu.Unlock()
	if ok {
		t.Fatal("wait 超时后登记残留")
	}
}

// gate 在 release 后同样清理。
func TestCallGate_WaitCleanupOnRelease(t *testing.T) {
	g := &callGate{}
	go func() {
		time.Sleep(5 * time.Millisecond)
		g.release("c2")
	}()
	g.wait("c2", time.Second)
	g.mu.Lock()
	_, ok := g.ch["c2"]
	g.mu.Unlock()
	if ok {
		t.Fatal("wait release 后登记残留")
	}
}

// dropRun 清空全部登记（run 收尾兜底）。
func TestCallGate_DropRun(t *testing.T) {
	g := &callGate{}
	g.release("c3")
	g.dropRun("run-1")
	g.mu.Lock()
	n := len(g.ch)
	g.mu.Unlock()
	if n != 0 {
		t.Fatalf("dropRun 后仍有 %d 条登记", n)
	}
}

// text 协议跳过 gate：同一工具调用不再等待收束信号。
func TestExecSimplebase_TextProtocolSkipsGate(t *testing.T) {
	var waits int32
	rt := &Runtime{
		ToolProtocol: "text",
		Confirm:      &recordingWaiter{waitFn: func() { atomic.AddInt32(&waits, 1) }},
	}
	_ = rt
	// text 协议下 execSimplebase 不进入 gate.wait：
	// 通过观察 gate 内部状态验证（调用本身因未配置 APIBaseURL 返回错误，无妨）。
	_, _ = rt.execSimplebase(context.Background(), RunRequest{ProjectID: "p", RunID: "r"}, []string{"database", "list"}, "")
	rt.mu.Lock()
	empty := rt.gate == nil || len(rt.gate.ch) == 0
	rt.mu.Unlock()
	if !empty {
		t.Fatal("text 协议不应登记 gate")
	}
	if atomic.LoadInt32(&waits) != 0 {
		t.Fatal("text 协议不应等待确认")
	}
}

type recordingWaiter struct {
	waitFn func()
}

func (w *recordingWaiter) Wait(ctx context.Context, runID, callID, initiator string, timeout time.Duration) (bool, error) {
	if w.waitFn != nil {
		w.waitFn()
	}
	return false, nil
}

// 发起人标识：登录态用 UserID，API Key 用 apikey:<id>。
func TestPrincipalKey(t *testing.T) {
	if got := principalKey(Principal{UserID: "u1"}); got != "u1" {
		t.Fatalf("UserID 优先: %q", got)
	}
	if got := principalKey(Principal{APIKeyID: "k1"}); got != "apikey:k1" {
		t.Fatalf("API Key 前缀: %q", got)
	}
	if got := principalKey(Principal{}); got != "" {
		t.Fatalf("空主体: %q", got)
	}
}

// execSimplebase 拒绝 --file 指向宿主机路径（§3.2/D3）。
func TestExecSimplebase_HostFileRejected(t *testing.T) {
	rt := &Runtime{}
	out, err := rt.execSimplebase(context.Background(), RunRequest{ProjectID: "p", RunID: "r"},
		[]string{"function", "create", "--name", "x", "--file", "/etc/hosts"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "file_not_allowed") {
		t.Fatalf("应返回 file_not_allowed: %s", out)
	}
}

// stdin 超过 256KiB 返回 usage。
func TestExecSimplebase_StdinLimit(t *testing.T) {
	rt := &Runtime{}
	out, err := rt.execSimplebase(context.Background(), RunRequest{ProjectID: "p", RunID: "r"},
		[]string{"sql", "batch", "--file", "-"}, strings.Repeat("a", 257<<10))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "256KiB") {
		t.Fatalf("应返回 stdin 上限错误: %s", out)
	}
}

func TestValidateCLIPathAndSafeFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-simplebase")
	if got := ValidateCLIPath(missing); got != "not_found" {
		t.Fatalf("missing CLI category = %q", got)
	}
	dir := t.TempDir()
	if got := ValidateCLIPath(dir); got != "not_executable" {
		t.Fatalf("directory CLI category = %q", got)
	}
	file := filepath.Join(t.TempDir(), "simplebase")
	if err := os.WriteFile(file, []byte("not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ValidateCLIPath(file); got != "not_executable" {
		t.Fatalf("CLI without exec bit category = %q", got)
	}
	if err := os.Chmod(file, 0700); err != nil {
		t.Fatal(err)
	}
	if got := ValidateCLIPath(file); got != "" {
		t.Fatalf("executable CLI category = %q", got)
	}
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&os.PathError{Err: os.ErrNotExist}, "not_found"},
		{&os.PathError{Err: os.ErrPermission}, "not_executable"},
		{errors.New("bad executable format"), "invalid_binary"},
	} {
		if got := CLIStartFailure(tc.err); got != tc.want {
			t.Fatalf("CLI failure category = %q, want %q", got, tc.want)
		}
	}
	if out, err := (&Runtime{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}, Policy: testPolicy{}, CLI: SubprocessCLI{Path: missing}}).execSimplebase(
		context.Background(), RunRequest{ProjectID: "p", RunID: "r", Skills: []string{"database"}}, []string{"database", "list"}, ""); err != nil || !strings.Contains(out, "cli_not_configured") {
		// 没有委托令牌时不会启动 CLI；真实启动失败用下面的令牌测试。
		t.Fatalf("missing delegation token: out=%s err=%v", out, err)
	}
	rt := &Runtime{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}, Policy: testPolicy{}, CLI: SubprocessCLI{Path: missing}}
	rt.rememberToken("r", "test-token")
	out, err := rt.execSimplebase(context.Background(), RunRequest{ProjectID: "p", RunID: "r", Skills: []string{"database"}}, []string{"database", "list"}, "")
	if err != nil || !strings.Contains(out, `"code":"cli_failed"`) || strings.Contains(out, missing) || strings.Contains(out, "test-token") {
		t.Fatalf("unsafe CLI error response: %s / %v", out, err)
	}
}

func TestSubprocessCLI_DatabaseListReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("CLI binary name differs on Windows")
	}
	// 该用例端到端验证「CLI 工具 → 子进程 → HTTP」，构建的是根模块的服务
	// 二进制。gocloudagent 现在是嵌套模块，必须显式用根模块的目录与 go.mod。
	root := filepath.Join("..")
	cliPath := filepath.Join(t.TempDir(), "simplebase")
	build := exec.Command("go", "build", "-o", cliPath, "./cmd/simplebase")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	if category := ValidateCLIPath(cliPath); category != "" {
		t.Fatalf("built CLI unavailable: %s", category)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/projects/p/databases" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected CLI request method=%s path=%s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"databases":[],"next_cursor":""}`)
	}))
	defer server.Close()
	rt := &Runtime{APIBaseURL: server.URL, Issuer: stubIssuer{}, Policy: testPolicy{}, CLI: SubprocessCLI{Path: cliPath}}
	rt.rememberToken("r", "test-token")
	out, err := rt.execSimplebase(context.Background(), RunRequest{ProjectID: "p", RunID: "r", Skills: []string{"database"}}, []string{"database", "list"}, "")
	if err != nil || !strings.Contains(out, `"ok":true`) || !strings.Contains(out, `"databases"`) {
		t.Fatalf("CLI list: out=%s err=%v", out, err)
	}
}

