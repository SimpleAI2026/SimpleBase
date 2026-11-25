// 文件说明：redaction_test.go — gocloudagent/cli 的单元测试（TestToolCallArgumentsAreRedactedInSessionAndEvents、TestConfigJSONMasksAllCredentials 等）
//
// 用法
//
//	go test ./gocloudagent/cli -run 'TestToolCallArgumentsAreRedactedInSessionAndEvents|TestConfigJSONMasksAllCredentials|TestMaskURLHidesUserinfoQueryAndFragment|TestConfigShowMasksSearchAPIKeyButGetReturnsIt'
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/redaction_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_loop.go, agent_loop_test.go, budget_test.go, client.go, client_conn_reuse_test.go 等 39 个
//
// 主要符号
//
//	TestToolCallArgumentsAreRedactedInSessionAndEvents(t *testing.T)
//	TestConfigJSONMasksAllCredentials(t *testing.T)
//	TestMaskURLHidesUserinfoQueryAndFragment(t *testing.T)
//	TestConfigShowMasksSearchAPIKeyButGetReturnsIt(t *testing.T)

package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// BUG-14a：工具调用参数不得把凭据明文写进会话文件（jsonl）与对外事件。
func TestToolCallArgumentsAreRedactedInSessionAndEvents(t *testing.T) {
	s := newTestSession(t)
	calls := 0
	secretArgs := `{"text":"password=hunter2 sk-abcdefghijklmnopqrstuvwxyz"}`
	client := &scriptedClient{replies: []agentcore.ChatResponse{
		{ToolCalls: []agentcore.ChatToolCall{{ID: "c1", Name: "echo", Arguments: secretArgs}}},
		{Content: "done"},
	}}
	var events []CLIEvent
	r := &SessionRunner{Session: s, Client: client, Tools: []tool.BaseTool{echoTool(t, &calls)}}
	if _, err := r.Run(context.Background(), "go", func(e CLIEvent) { events = append(events, e) }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("the real tool must still run with the original arguments, calls=%d", calls)
	}
	for _, e := range s.Lines {
		for _, tc := range e.ToolCalls {
			if strings.Contains(tc.Arguments, "hunter2") || strings.Contains(tc.Arguments, "sk-abcdefghijklmnopqrstuvwxyz") {
				t.Fatalf("secret persisted in session: %q", tc.Arguments)
			}
		}
	}
	for _, e := range events {
		if e.Type == "tool_call" && (strings.Contains(e.Arguments, "hunter2") || strings.Contains(e.Arguments, "sk-abcdefghijklmnopqrstuvwxyz")) {
			t.Fatalf("secret leaked in tool_call event: %q", e.Arguments)
		}
	}
}

// BUG-14b：`config json` 不得输出任何明文凭据（Token / SearchAPIKey / MCP Headers / MCP Env）。
func TestConfigJSONMasksAllCredentials(t *testing.T) {
	home := t.TempDir()
	env := []string{"GOCLOUDAGENT_HOME=" + home}
	cfg := Config{
		Endpoint:     "http://127.0.0.1:8080",
		Token:        "tok-abcdefghijklmnop",
		SearchAPIKey: "search-key-0123456789",
		MCPServers: map[string]MCPServerConfig{"s": {
			URL:     "https://u:pw@example.com/mcp?token=qq#frag",
			Headers: map[string]string{"Authorization": "Bearer header-secret-123"},
			Env:     map[string]string{"API_TOKEN": "env-secret-456"},
		}},
	}
	if _, err := SaveConfig(env, cfg); err != nil {
		t.Fatal(err)
	}
	read, out, errw := captureStdio(t)
	if code := cmdConfig(env, []string{"json"}, out, errw); code != exitOK {
		t.Fatalf("code=%d", code)
	}
	stdout, _ := read()
	for _, secret := range []string{"tok-abcdefghijklmnop", "search-key-0123456789", "header-secret-123", "env-secret-456", "u:pw", "token=qq", "#frag"} {
		if strings.Contains(stdout, secret) {
			t.Fatalf("config json leaks %q:\n%s", secret, stdout)
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("output must remain valid JSON: %v", err)
	}
	if !strings.Contains(stdout, "example.com") {
		t.Fatalf("host must stay visible for diagnostics:\n%s", stdout)
	}
}

func TestMaskURLHidesUserinfoQueryAndFragment(t *testing.T) {
	cases := map[string]string{
		"":                                      "",
		"https://example.com/mcp":               "https://example.com/mcp",
		"https://example.com/mcp?token=s":       "https://example.com/mcp?***",
		"https://u:pw@example.com/mcp":          "https://***@example.com/mcp",
		"https://u:pw@example.com/mcp?t=1#frag": "https://***@example.com/mcp?***",
		"https://example.com/mcp#frag":          "https://example.com/mcp",
		"  https://example.com/x  ":             "https://example.com/x",
	}
	for in, want := range cases {
		if got := maskURL(in); got != want {
			t.Errorf("maskURL(%q)=%q want %q", in, got, want)
		}
	}
}

// `config show` 同样不输出 search_api_key 明文；`config get` 是显式取值，保持原样。
func TestConfigShowMasksSearchAPIKeyButGetReturnsIt(t *testing.T) {
	home := t.TempDir()
	env := []string{"GOCLOUDAGENT_HOME=" + home}
	if _, err := SaveConfig(env, Config{Endpoint: "http://127.0.0.1:1", Token: "tok-abcdefghijklmnop", SearchAPIKey: "search-key-0123456789"}); err != nil {
		t.Fatal(err)
	}
	read, out, errw := captureStdio(t)
	if code := cmdConfig(env, []string{"show"}, out, errw); code != exitOK {
		t.Fatalf("code=%d", code)
	}
	shown, _ := read()
	if strings.Contains(shown, "search-key-0123456789") || strings.Contains(shown, "tok-abcdefghijklmnop") {
		t.Fatalf("show leaks credentials:\n%s", shown)
	}
	read, out, errw = captureStdio(t)
	if code := cmdConfig(env, []string{"get", "search_api_key"}, out, errw); code != exitOK {
		t.Fatalf("code=%d", code)
	}
	if got, _ := read(); strings.TrimSpace(got) != "search-key-0123456789" {
		t.Fatalf("get must return the raw value for scripts, got %q", got)
	}
}
