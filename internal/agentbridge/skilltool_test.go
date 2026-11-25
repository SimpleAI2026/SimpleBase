// 文件说明：skilltool_test.go — internal/agentbridge 的单元测试（TestExecSimplebase_TextProtocolSkipsGate、TestExecSimplebase_HostFileRejected 等）
//
// 用法
//
//	go test ./internal/agentbridge -run 'TestExecSimplebase_TextProtocolSkipsGate|TestExecSimplebase_HostFileRejected|TestExecSimplebase_StdinLimit|TestSafeCLIFailureResponse'
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/skilltool_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//
// 主要符号
//
//	TestExecSimplebase_TextProtocolSkipsGate(t *testing.T)
//	type recordingWaiter struct{…}
//	(w *recordingWaiter) Wait(ctx context.Context, runID, callID, initiator string, timeout time.Duration) (bool, ...
//	TestExecSimplebase_HostFileRejected(t *testing.T)
//	TestExecSimplebase_StdinLimit(t *testing.T)
//	TestSafeCLIFailureResponse(t *testing.T)
//	TestSubprocessCLI_DatabaseListReadOnly(t *testing.T)

package agentbridge

// cli_tool_planv6_test.go：planv6.0 §3.4/§3.5 单元测试。
// - gate：wait 超时后登记自清理（不残留）；text 协议跳过 gate；
// - redactArgv：--statement 只留首关键字+200 字符；kv --arg 除首个外 ***；
//   敏感 flag 值 ***；
// - sqlDestructive：切分认识引号与注释（DELETE 出现在字符串字面量里不算）。

import (
	"context"

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

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
)

// text 协议跳过 gate：同一工具调用不再等待收束信号。
// 注意：eino 没有公开的 call ID 注入器，这里 callID 恒为空，因此该用例只能证明
// 「text 协议下 execSimplebase 不进入确认等待」，不能区分 text/native 的 gate 分支；
// gate 本身的时序语义由 cloudagent.ToolCallGate 的单元测试保证。
func TestExecSimplebase_TextProtocolSkipsGate(t *testing.T) {
	var waits int32
	rt := &RuntimeProvider{
		ToolProtocol: "text",
		Confirm:      &recordingWaiter{waitFn: func() { atomic.AddInt32(&waits, 1) }},
	}
	// 调用本身因未配置 APIBaseURL 返回错误，无妨。
	_, _ = rt.execSimplebase(context.Background(), cloudagent.RunRequest{RunID: "r", Context: RunContext{ProjectID: "p"}}, []string{"database", "list"}, "")
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

// execSimplebase 拒绝 --file 指向宿主机路径（§3.2/D3）。
func TestExecSimplebase_HostFileRejected(t *testing.T) {
	rt := &RuntimeProvider{}
	out, err := rt.execSimplebase(context.Background(), cloudagent.RunRequest{RunID: "r", Context: RunContext{ProjectID: "p"}},
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
	rt := &RuntimeProvider{}
	out, err := rt.execSimplebase(context.Background(), cloudagent.RunRequest{RunID: "r", Context: RunContext{ProjectID: "p"}},
		[]string{"sql", "batch", "--file", "-"}, strings.Repeat("a", 257<<10))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "256KiB") {
		t.Fatalf("应返回 stdin 上限错误: %s", out)
	}
}

// 分类逻辑在 cloudagent.ValidateExecutable / ProcessStartFailure 测试；这里只验证宿主：
// 启动失败的响应不泄露二进制路径与委托令牌。
func TestSafeCLIFailureResponse(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-simplebase")
	if out, err := (&RuntimeProvider{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}, Policy: testPolicy{}, CLI: SubprocessCLI{Path: missing}}).execSimplebase(
		context.Background(), cloudagent.RunRequest{RunID: "r", Context: RunContext{ProjectID: "p", Skills: []string{"database"}}}, []string{"database", "list"}, ""); err != nil || !strings.Contains(out, "cli_not_configured") {
		// 没有委托令牌时不会启动 CLI；真实启动失败用下面的令牌测试。
		t.Fatalf("missing delegation token: out=%s err=%v", out, err)
	}
	rt := &RuntimeProvider{APIBaseURL: "http://127.0.0.1:9", Issuer: stubIssuer{}, Policy: testPolicy{}, CLI: SubprocessCLI{Path: missing}}
	rt.rememberToken("r", "test-token")
	out, err := rt.execSimplebase(context.Background(), cloudagent.RunRequest{RunID: "r", Context: RunContext{ProjectID: "p", Skills: []string{"database"}}}, []string{"database", "list"}, "")
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
	root := filepath.Join("..", "..")
	cliPath := filepath.Join(t.TempDir(), "simplebase")
	build := exec.Command("go", "build", "-o", cliPath, "./cmd/simplebase")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	if category := cloudagent.ValidateExecutable(cliPath); category != "" {
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
	rt := &RuntimeProvider{APIBaseURL: server.URL, Issuer: stubIssuer{}, Policy: testPolicy{}, CLI: SubprocessCLI{Path: cliPath}}
	rt.rememberToken("r", "test-token")
	out, err := rt.execSimplebase(context.Background(), cloudagent.RunRequest{RunID: "r", Context: RunContext{ProjectID: "p", Skills: []string{"database"}}}, []string{"database", "list"}, "")
	if err != nil || !strings.Contains(out, `"ok":true`) || !strings.Contains(out, `"databases"`) {
		t.Fatalf("CLI list: out=%s err=%v", out, err)
	}
}
