// 文件说明：mcp_bugs_test.go — gocloudagent/cli 的单元测试（TestMCPToolsCommandHonoursShellIsolationForStdio、TestMCPHTTPHeadersExpandEnvVars 等）
//
// 用法
//
//	go test ./gocloudagent/cli -run 'TestMCPToolsCommandHonoursShellIsolationForStdio|TestMCPHTTPHeadersExpandEnvVars|TestConnectStdioSessionRefusesWhenIsolationEnabled'
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/mcp_bugs_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_loop.go, agent_loop_test.go, budget_test.go, client.go, client_conn_reuse_test.go 等 39 个
//
// 主要符号
//
//	TestMCPToolsCommandHonoursShellIsolationForStdio(t *testing.T)
//	TestMCPHTTPHeadersExpandEnvVars(t *testing.T)
//	TestConnectStdioSessionRefusesWhenIsolationEnabled(t *testing.T)

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// BUG-11：shell_isolation 开启时，`mcp tools` 入口同样不得启动 stdio 子进程。
func TestMCPToolsCommandHonoursShellIsolationForStdio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	home := t.TempDir()
	marker := filepath.Join(t.TempDir(), "started")
	script := filepath.Join(t.TempDir(), "fake-mcp.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\nsleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	enabled := true
	cfg := Config{
		Endpoint:       "http://127.0.0.1:1",
		Token:          "t",
		ShellIsolation: ShellIsolation{Enabled: &enabled},
		MCPServers:     map[string]MCPServerConfig{"local": {Command: script}},
	}
	env := []string{"GOCLOUDAGENT_HOME=" + home}
	if _, err := SaveConfig(env, cfg); err != nil {
		t.Fatal(err)
	}
	_, out, errw := captureStdio(t)
	_ = cmdMCP(env, []string{"tools"}, out, errw)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("stdio MCP server was started although shell_isolation is enabled")
	}
}

// BUG-12：HTTP MCP 的 Header 值支持 ${VAR} 展开（凭据不落配置文件明文）。
func TestMCPHTTPHeadersExpandEnvVars(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	srvCfg := MCPServerConfig{URL: srv.URL, Headers: map[string]string{
		"Authorization": "Bearer ${MCP_TEST_TOKEN}",
		"X-Plain":       "plain",
		"X-Unknown":     "a${NOT_DEFINED}b",
	}}
	client, err := mcpHTTPClient("s", srvCfg, []string{"MCP_TEST_TOKEN=real-secret"})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Get("Authorization") != "Bearer real-secret" {
		t.Fatalf("Authorization=%q", got.Get("Authorization"))
	}
	if got.Get("X-Plain") != "plain" || got.Get("X-Unknown") != "ab" {
		t.Fatalf("headers=%v", got)
	}
	if strings.Contains(srvCfg.Headers["Authorization"], "real-secret") {
		t.Fatal("config must keep the placeholder, not the expanded secret")
	}
}

// connectStdioSession 在隔离开启时直接拒绝，不依赖 Workspace，也不启动进程。
func TestConnectStdioSessionRefusesWhenIsolationEnabled(t *testing.T) {
	_, err := connectStdioSession(context.Background(), "s", MCPServerConfig{Command: "/definitely/not/run"}, mcpServerEnv{IsolationEnabled: true})
	if err == nil || !strings.Contains(err.Error(), "shell_isolation") {
		t.Fatalf("err=%v", err)
	}
}
