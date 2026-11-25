// 文件说明：mcp_config_test.go — gocloudagent/cli 的单元测试（TestExpandEnvValue、TestMCPChildEnvDropsInheritedSecrets 等）
//
// 用法
//
//	go test ./gocloudagent/cli -run 'TestExpandEnvValue|TestMCPChildEnvDropsInheritedSecrets|TestMCPChildEnvFallbackPATH|TestMCPServerConfigValidate'
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/mcp_config_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_loop.go, agent_loop_test.go, budget_test.go, client.go, client_conn_reuse_test.go 等 39 个
//
// 主要符号
//
//	TestExpandEnvValue(t *testing.T)
//	TestMCPChildEnvDropsInheritedSecrets(t *testing.T)
//	TestMCPChildEnvFallbackPATH(t *testing.T)
//	TestMCPServerConfigValidate(t *testing.T)
//	TestMCPServerConfigReadOnly(t *testing.T)
//	TestConfigValidateRejectsBadMCPServer(t *testing.T)
//	TestMaskURLHidesQuery(t *testing.T)
//	TestOverlayConfigMergesMCPServersByKey(t *testing.T)

package cli

import (
	"strings"
	"testing"
)

func TestExpandEnvValue(t *testing.T) {
	env := []string{"HOME=/root", "GITHUB_TOKEN=ghp_x"}
	cases := map[string]string{
		"plain":                  "plain",
		"${HOME}":                "/root",
		"Bearer ${GITHUB_TOKEN}": "Bearer ghp_x",
		// 未定义变量展开为空串，不报错。
		"${MISSING}": "",
		// 未闭合的占位符原样保留。
		"${HOME": "${HOME",
	}
	for in, want := range cases {
		if got := expandEnvValue(in, env); got != want {
			t.Fatalf("expandEnvValue(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMCPChildEnvDropsInheritedSecrets(t *testing.T) {
	env := []string{"HOME=/root", "PATH=/usr/bin:/bin", "PLATFORM_TOKEN=leak", "GH_TOKEN=leak"}
	got := mcpChildEnv(map[string]string{
		"FOO":       "bar",
		"MY_SECRET": "should-be-dropped",
		"DATA_DIR":  "${HOME}/t",
	}, env)
	joined := strings.Join(got, "\n")
	// 显式配置的普通变量保留并展开。
	if !strings.Contains(joined, "FOO=bar") {
		t.Fatalf("FOO missing: %v", got)
	}
	if !strings.Contains(joined, "DATA_DIR=/root/t") {
		t.Fatalf("DATA_DIR missing: %v", got)
	}
	// 名字里带 TOKEN 的变量即使不是凭据也被过滤（保守策略）。
	if strings.Contains(joined, "TOKEN") {
		t.Fatalf("TOKEN-like key kept: %v", got)
	}
	// 凭据类变量一律过滤（即使显式写入）。
	for _, bad := range []string{"MY_SECRET", "LEAK", "GH_TOKEN"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("%s leaked: %v", bad, got)
		}
	}
	// PATH/HOME 是子进程的最小依赖。
	if !strings.Contains(joined, "PATH=/usr/bin:/bin") || !strings.Contains(joined, "HOME=/root") {
		t.Fatalf("PATH/HOME missing: %v", got)
	}
}

func TestMCPChildEnvFallbackPATH(t *testing.T) {
	got := mcpChildEnv(nil, []string{"HOME=/root"})
	if !strings.Contains(strings.Join(got, ","), "PATH=/usr/bin:/bin") {
		t.Fatalf("fallback PATH missing: %v", got)
	}
}

func TestMCPServerConfigValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  MCPServerConfig
		ok   bool
	}{
		{"stdio", MCPServerConfig{Command: "npx", Args: []string{"-y", "server"}}, true},
		{"http", MCPServerConfig{URL: "https://example.com/mcp"}, true},
		{"both", MCPServerConfig{Command: "npx", URL: "https://x/mcp"}, false},
		{"neither", MCPServerConfig{}, false},
		{"timeout too big", MCPServerConfig{URL: "https://x/mcp", TimeoutSeconds: 301}, false},
		{"timeout ok", MCPServerConfig{URL: "https://x/mcp", TimeoutSeconds: 300}, true},
	}
	for _, c := range cases {
		err := c.cfg.Validate()
		if c.ok && err != nil {
			t.Fatalf("%s: unexpected err %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Fatalf("%s: expected err", c.name)
		}
	}
}

func TestMCPServerConfigReadOnly(t *testing.T) {
	cfg := MCPServerConfig{ReadOnlyTools: []string{"search", "list"}}
	if !cfg.ReadOnly("search") || !cfg.ReadOnly("list") {
		t.Fatal("whitelisted tools should be read-only")
	}
	if cfg.ReadOnly("delete") {
		t.Fatal("unlisted tool must not be read-only")
	}
}

func TestConfigValidateRejectsBadMCPServer(t *testing.T) {
	cfg := Config{
		Endpoint: "http://127.0.0.1:8080",
		Token:    "t",
		MCPServers: map[string]MCPServerConfig{
			"broken": {},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid mcp server must fail validation")
	}
}

func TestMaskURLHidesQuery(t *testing.T) {
	if got := maskURL("https://x/mcp?token=abc"); got != "https://x/mcp?***" {
		t.Fatalf("maskURL=%q", got)
	}
	if got := maskURL("https://x/mcp"); got != "https://x/mcp" {
		t.Fatalf("maskURL=%q", got)
	}
}

func TestOverlayConfigMergesMCPServersByKey(t *testing.T) {
	base := Config{MCPServers: map[string]MCPServerConfig{
		"a": {URL: "https://a/mcp"},
		"b": {URL: "https://b/mcp"},
	}}
	over := Config{MCPServers: map[string]MCPServerConfig{
		"b": {URL: "https://b2/mcp"},
		"c": {URL: "https://c/mcp"},
	}}
	got := overlayConfig(base, over)
	if len(got.MCPServers) != 3 {
		t.Fatalf("servers=%d want 3", len(got.MCPServers))
	}
	if got.MCPServers["b"].URL != "https://b2/mcp" {
		t.Fatalf("b not overridden: %+v", got.MCPServers["b"])
	}
	if got.MCPServers["a"].URL != "https://a/mcp" {
		t.Fatalf("a lost: %+v", got.MCPServers["a"])
	}
}
