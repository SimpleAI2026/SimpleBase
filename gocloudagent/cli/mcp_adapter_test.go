// 文件说明：mcp_adapter_test.go — gocloudagent/cli 的单元测试（TestMCPLocalToolConfirmGate、TestMCPLocalToolReadOnlySkipsConfirm 等）
//
// 用法
//
//	go test ./gocloudagent/cli -run 'TestMCPLocalToolConfirmGate|TestMCPLocalToolReadOnlySkipsConfirm|TestMCPLocalToolYoloSkipsConfirm|TestMCPLocalToolNoWorkspaceFailsClosed'
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/mcp_adapter_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_loop.go, agent_loop_test.go, budget_test.go, client.go, client_conn_reuse_test.go 等 39 个
//
// 主要符号
//
//	type workspaceForTest struct{…}
//	(w *workspaceForTest) workspace() *tools.Workspace
//	type fakeSession struct{…}
//	(f *fakeSession) ServerID() string
//	(f *fakeSession) ListTools(context.Context) ([]mcp.ToolInfo, error)
//	(f *fakeSession) CallTool(_ context.Context, name string, _ []byte) (mcp.CallResult, error)
//	(f *fakeSession) Close() error
//	objectSchema() []byte
//	... 另有 15 个顶层符号

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/gocloudagent/tools"
)

// workspaceForTest 构造一个可控的 tools.Workspace。
type workspaceForTest struct {
	confirm func(string) bool
	record  func(string, string, string, bool)
	yolo    bool
}

func (w *workspaceForTest) workspace() *tools.Workspace {
	return &tools.Workspace{Confirm: w.confirm, Record: w.record, Yolo: w.yolo}
}

// fakeSession 是 mcp.Session 的测试替身。
type fakeSession struct {
	id      string
	tools   []mcp.ToolInfo
	listErr error
	result  mcp.CallResult
	callErr error
	closed  bool
	calls   []string
}

func (f *fakeSession) ServerID() string { return f.id }
func (f *fakeSession) ListTools(context.Context) ([]mcp.ToolInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.tools, nil
}
func (f *fakeSession) CallTool(_ context.Context, name string, _ []byte) (mcp.CallResult, error) {
	f.calls = append(f.calls, name)
	if f.callErr != nil {
		return mcp.CallResult{}, f.callErr
	}
	return f.result, nil
}
func (f *fakeSession) Close() error { f.closed = true; return nil }

func objectSchema() []byte { return []byte(`{"type":"object","properties":{}}`) }

func TestMCPLocalToolConfirmGate(t *testing.T) {
	sess := &fakeSession{id: "s", result: mcp.CallResult{Text: "ok"}}
	asks := 0
	ws := &workspaceForTest{confirm: func(string) bool { asks++; return false }}
	env := mcpServerEnv{Workspace: ws.workspace(), Env: []string{}, Cwd: "."}
	tool := newMCPLocalTool("s", MCPServerConfig{URL: "https://x/mcp"}, sess, mcp.ToolInfo{Name: "delete", InputSchema: objectSchema()}, env)

	out, err := tool.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	// 非只读工具需确认；拒绝时不执行。
	if asks != 1 {
		t.Fatalf("confirm asked %d times", asks)
	}
	if len(sess.calls) != 0 {
		t.Fatal("denied tool must not run")
	}
	if !strings.Contains(out, mcp.CodeConfirmationDenied) {
		t.Fatalf("out=%q", out)
	}
}

func TestMCPLocalToolReadOnlySkipsConfirm(t *testing.T) {
	sess := &fakeSession{id: "s", result: mcp.CallResult{Text: "ok"}}
	asks := 0
	ws := &workspaceForTest{confirm: func(string) bool { asks++; return true }}
	env := mcpServerEnv{Workspace: ws.workspace(), Env: []string{}, Cwd: "."}
	tool := newMCPLocalTool("s", MCPServerConfig{URL: "https://x/mcp", ReadOnlyTools: []string{"search"}}, sess, mcp.ToolInfo{Name: "search", InputSchema: objectSchema()}, env)

	out, err := tool.InvokableRun(context.Background(), `{}`)
	if err != nil || out != "ok" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if asks != 0 {
		t.Fatalf("read-only tool asked %d times", asks)
	}
	if len(sess.calls) != 1 {
		t.Fatalf("calls=%v", sess.calls)
	}
}

func TestMCPLocalToolYoloSkipsConfirm(t *testing.T) {
	sess := &fakeSession{id: "s", result: mcp.CallResult{Text: "ok"}}
	asks := 0
	ws := &workspaceForTest{confirm: func(string) bool { asks++; return false }}
	ws.yolo = true
	env := mcpServerEnv{Workspace: ws.workspace(), Env: []string{}, Cwd: "."}
	tool := newMCPLocalTool("s", MCPServerConfig{URL: "https://x/mcp"}, sess, mcp.ToolInfo{Name: "delete", InputSchema: objectSchema()}, env)
	if _, err := tool.InvokableRun(context.Background(), `{}`); err != nil {
		t.Fatal(err)
	}
	if asks != 0 {
		t.Fatalf("yolo should skip confirm, asked %d", asks)
	}
}

func TestMCPLocalToolNoWorkspaceFailsClosed(t *testing.T) {
	sess := &fakeSession{id: "s", result: mcp.CallResult{Text: "ok"}}
	env := mcpServerEnv{Env: []string{}, Cwd: "."}
	tool := newMCPLocalTool("s", MCPServerConfig{URL: "https://x/mcp"}, sess, mcp.ToolInfo{Name: "delete", InputSchema: objectSchema()}, env)
	out, err := tool.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	// 非交互（无 Workspace）：一律拒绝。
	if !strings.Contains(out, mcp.CodeConfirmationDenied) {
		t.Fatalf("out=%q", out)
	}
}

func TestMCPLocalToolErrorAndTimeoutEnvelopes(t *testing.T) {
	sess := &fakeSession{id: "s", result: mcp.CallResult{Text: "bad", IsError: true}}
	env := mcpServerEnv{Workspace: (&workspaceForTest{}).workspace(), Env: []string{}, Cwd: "."}
	tool := newMCPLocalTool("s", MCPServerConfig{URL: "https://x/mcp", ReadOnlyTools: []string{"t"}}, sess, mcp.ToolInfo{Name: "t", InputSchema: objectSchema()}, env)
	out, _ := tool.InvokableRun(context.Background(), `{}`)
	if !strings.Contains(out, mcp.CodeToolError) || !strings.Contains(out, "bad") {
		t.Fatalf("out=%q", out)
	}

	failing := &fakeSession{id: "s", callErr: errors.New("dial failed")}
	tool = newMCPLocalTool("s", MCPServerConfig{URL: "https://x/mcp", ReadOnlyTools: []string{"t"}}, failing, mcp.ToolInfo{Name: "t", InputSchema: objectSchema()}, env)
	out, _ = tool.InvokableRun(context.Background(), `{}`)
	if !strings.Contains(out, mcp.CodeUnavailable) {
		t.Fatalf("out=%q", out)
	}
}

func TestMCPLocalToolTruncatedAndRecorded(t *testing.T) {
	sess := &fakeSession{id: "s", result: mcp.CallResult{Text: "partial", Truncated: true}}
	recorded := 0
	ws := &workspaceForTest{record: func(string, string, string, bool) { recorded++ }}
	env := mcpServerEnv{Workspace: ws.workspace(), Env: []string{}, Cwd: "."}
	tool := newMCPLocalTool("s", MCPServerConfig{URL: "https://x/mcp", ReadOnlyTools: []string{"t"}}, sess, mcp.ToolInfo{Name: "t", InputSchema: objectSchema()}, env)
	out, _ := tool.InvokableRun(context.Background(), `{}`)
	if !strings.Contains(out, "[truncated]") {
		t.Fatalf("out=%q", out)
	}
	if recorded != 1 {
		t.Fatalf("recorded=%d", recorded)
	}
}

func TestMCPCallTimeout(t *testing.T) {
	if got := mcpCallTimeout(MCPServerConfig{}); got != mcp.DefaultCallTimeout {
		t.Fatalf("default=%v", got)
	}
	if got := mcpCallTimeout(MCPServerConfig{TimeoutSeconds: 30}); got != 30*time.Second {
		t.Fatalf("30s=%v", got)
	}
	// 超过上限截到 300s。
	if got := mcpCallTimeout(MCPServerConfig{TimeoutSeconds: 9999}); got != mcp.MaxCallTimeout {
		t.Fatalf("clamped=%v", got)
	}
}

func TestLocalToolInfoSkipsNonObjectSchema(t *testing.T) {
	info := localToolInfo("mcp__s__t", mcp.ToolInfo{
		Name:        "t",
		Description: "d",
		InputSchema: []byte(`{"type":"array"}`),
	})
	// 非 object schema 不挂 ParamsOneOf，工具仍可用（无参数）。
	if info.ParamsOneOf != nil {
		t.Fatal("non-object schema should not produce params")
	}
	if info.Name != "mcp__s__t" || info.Desc != "d" {
		t.Fatalf("info=%#v", info)
	}
}

func TestConnectMCPServersEmptyConfig(t *testing.T) {
	tools, closeAll, err := connectMCPServers(context.Background(), Config{}, mcpServerEnv{})
	if err != nil || len(tools) != 0 {
		t.Fatalf("tools=%d err=%v", len(tools), err)
	}
	closeAll()
}

func TestConnectMCPServersSkipsDisabledAndFailures(t *testing.T) {
	warned := 0
	env := mcpServerEnv{
		Env:  []string{},
		Cwd:  ".",
		Warn: func(string, ...zap.Field) { warned++ },
	}
	cfg := Config{MCPServers: map[string]MCPServerConfig{
		"off": {URL: "https://x/mcp", Disabled: true},
		"bad": {URL: "://bad"},
	}}
	tools, closeAll, err := connectMCPServers(context.Background(), cfg, env)
	defer closeAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools=%d want 0", len(tools))
	}
	// 非法 URL 的 Server 被跳过并 warn。
	if warned == 0 {
		t.Fatal("unreachable server should warn")
	}
}

func TestMCPToolsSubcommandUnknownServer(t *testing.T) {
	env := []string{"GOCLOUDAGENT_HOME=" + t.TempDir()}
	read, out, errw := captureStdio(t)
	code := cmdMCP(env, []string{"tools", "nope"}, out, errw)
	if code != exitUsage {
		t.Fatalf("code=%d want %d", code, exitUsage)
	}
	if _, stderr := read(); !strings.Contains(stderr, "unknown mcp server") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestMCPSubcommandHelpAndUnknown(t *testing.T) {
	env := []string{"GOCLOUDAGENT_HOME=" + t.TempDir()}
	read, out, errw := captureStdio(t)
	if code := cmdMCP(env, []string{"help"}, out, errw); code != exitOK {
		t.Fatalf("help code=%d", code)
	}
	if stdout, _ := read(); !strings.Contains(stdout, "mcp_servers") {
		t.Fatalf("stdout=%q", stdout)
	}
	if code := cmdMCP(env, []string{"bogus"}, out, errw); code != exitUsage {
		t.Fatalf("unknown code=%d", code)
	}
}

func TestMCPListSubcommand(t *testing.T) {
	home := t.TempDir()
	env := []string{"GOCLOUDAGENT_HOME=" + home}
	// 空配置：直接返回。
	read, out, errw := captureStdio(t)
	if code := cmdMCP(env, []string{"list"}, out, errw); code != exitOK {
		t.Fatalf("empty code=%d", code)
	}
	if stdout, _ := read(); !strings.Contains(stdout, "no mcp servers configured") {
		t.Fatalf("stdout=%q", stdout)
	}

	// 写一份配置：stdio + http，http URL 带 token 必须被掩码。
	cfg := Config{
		Endpoint: "http://127.0.0.1:8080",
		Token:    "t",
		MCPServers: map[string]MCPServerConfig{
			"fs":  {Command: "npx", Args: []string{"-y", "fs"}},
			"net": {URL: "https://x/mcp?token=secret", Headers: map[string]string{"Authorization": "Bearer s"}},
			"off": {URL: "https://y/mcp", Disabled: true},
		},
	}
	if _, err := SaveConfig(env, cfg); err != nil {
		t.Fatal(err)
	}
	read, out, errw = captureStdio(t)
	if code := cmdMCP(env, []string{"list"}, out, errw); code != exitOK {
		t.Fatalf("list code=%d", code)
	}
	stdout, _ := read()
	for _, want := range []string{"fs\tstdio", "net\thttp", "disabled", "headers=1"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("missing %q in %q", want, stdout)
		}
	}
	// query 里的 token 必须被掩码，原文不得出现。
	if strings.Contains(stdout, "secret") {
		t.Fatalf("token leaked: %q", stdout)
	}
}

func TestMCPToolsSubcommandEmptyConfig(t *testing.T) {
	home := t.TempDir()
	env := []string{"GOCLOUDAGENT_HOME=" + home}
	cfg := Config{Endpoint: "http://127.0.0.1:8080", Token: "t"}
	if _, err := SaveConfig(env, cfg); err != nil {
		t.Fatal(err)
	}
	read, out, errw := captureStdio(t)
	if code := cmdMCP(env, []string{"tools"}, out, errw); code != exitOK {
		t.Fatalf("code=%d", code)
	}
	if stdout, _ := read(); !strings.Contains(stdout, "no mcp servers configured") {
		t.Fatalf("stdout=%q", stdout)
	}
}

// 确保 JSON 信封是合法的（前端与模型都按 code 分支）。
func TestToolsErrEnvelope(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal([]byte(tools.ErrEnvelope(mcp.CodeTimeout, "x")), &v); err != nil {
		t.Fatal(err)
	}
	if v["ok"] != false {
		t.Fatalf("envelope=%v", v)
	}
}
