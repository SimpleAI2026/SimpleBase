// 文件说明：naming_test.go — gocloudagent/mcp 的单元测试（TestToolNameSanitizesAndPrefixes、TestToolNameTruncatesWithHashSuffix 等）
//
// 用法
//
//	go test ./gocloudagent/mcp -run 'TestToolNameSanitizesAndPrefixes|TestToolNameTruncatesWithHashSuffix|TestDisplayName|TestParseToolID'
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/tools, internal/agentbridge 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/mcp/naming_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] adapter.go, adapter_test.go, client.go, http_transport.go, http_transport_test.go 等 10 个
//
// 主要符号
//
//	TestToolNameSanitizesAndPrefixes(t *testing.T)
//	TestToolNameTruncatesWithHashSuffix(t *testing.T)
//	TestDisplayName(t *testing.T)
//	TestParseToolID(t *testing.T)

package mcp

import (
	"strings"
	"testing"
)

func TestToolNameSanitizesAndPrefixes(t *testing.T) {
	got := ToolName("github", "create_issue")
	if got != "mcp__github__create_issue" {
		t.Fatalf("name=%q", got)
	}
	// 非法字符替换为下划线，首尾下划线被裁掉。
	got = ToolName("my server!", "do/thing")
	if got != "mcp__my_server__do_thing" {
		t.Fatalf("sanitized name=%q", got)
	}
	// 空段回落到默认值，避免生成 mcp____x 这类名字。
	if got = ToolName("", ""); got != "mcp__server__tool" {
		t.Fatalf("empty name=%q", got)
	}
}

func TestToolNameTruncatesWithHashSuffix(t *testing.T) {
	long := strings.Repeat("a", 80)
	got := ToolName("server", long)
	if len(got) > 64 {
		t.Fatalf("name too long: %d", len(got))
	}
	if !strings.HasPrefix(got, "mcp__server__") {
		t.Fatalf("prefix lost: %q", got)
	}
	// 同名不同源必须得到不同结果（hash 后缀消歧）。
	other := ToolName("server2", long)
	if got == other {
		t.Fatal("truncated names collided")
	}
}

func TestDisplayName(t *testing.T) {
	if got := DisplayName("mcp__github__create_issue"); got != "github · create_issue" {
		t.Fatalf("display=%q", got)
	}
	// 非 MCP 工具名原样返回。
	if got := DisplayName("custom"); got != "custom" {
		t.Fatalf("display=%q", got)
	}
	// 缺分隔符时原样返回，不 panic。
	if got := DisplayName("mcp__broken"); got != "mcp__broken" {
		t.Fatalf("display=%q", got)
	}
}

func TestParseToolID(t *testing.T) {
	cases := []struct {
		id     string
		server string
		tool   string
		ok     bool
	}{
		{"mcp:github", "github", "", true},
		{"mcp:github/create_issue", "github", "create_issue", true},
		{" mcp:fs/read ", "fs", "read", true},
		{"mcp:", "", "", false},
		{"mcp:/tool", "", "", false},
		{"custom", "", "", false},
	}
	for _, c := range cases {
		server, tool, ok := ParseToolID(c.id)
		if ok != c.ok || server != c.server || tool != c.tool {
			t.Fatalf("ParseToolID(%q) = (%q,%q,%v), want (%q,%q,%v)", c.id, server, tool, ok, c.server, c.tool, c.ok)
		}
	}
	if !IsMCPToolID("mcp:x") || IsMCPToolID("custom") {
		t.Fatal("IsMCPToolID mismatch")
	}
}
