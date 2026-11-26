// 文件说明：agent_mcp_test.go — internal/api 的单元测试（TestNormalizeAgentKeepsMCPToolIDs、TestNormalizeAgentRejectsInvalidMCPToolID 等）
//
// 用法
//
//	go test ./internal/api -run 'TestNormalizeAgentKeepsMCPToolIDs|TestNormalizeAgentRejectsInvalidMCPToolID|TestNormalizeAgentStillDropsUnknownTools|TestNormalizeAgentRequiresMCPConnector'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_mcp_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestNormalizeAgentKeepsMCPToolIDs(t *testing.T)
//	TestNormalizeAgentRejectsInvalidMCPToolID(t *testing.T)
//	TestNormalizeAgentStillDropsUnknownTools(t *testing.T)
//	TestNormalizeAgentRequiresMCPConnector(t *testing.T)

package agenthttp

import (
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// mcp: 引用必须被保留（planv6.2 §3.2）：静默丢弃会让用户以为能力已启用。
func TestNormalizeAgentKeepsMCPToolIDs(t *testing.T) {
	got, err := normalizeAgent("p", upsertAgentBody{
		Name:    "n",
		Module:  "general",
		ToolIDs: []string{"list_databases", "mcp:github", "mcp:fs/read"},
	}, systemdb.CloudAgent{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"list_databases", "mcp:github", "mcp:fs/read"}
	if len(got.ToolIDs) != len(want) {
		t.Fatalf("tool ids=%v want %v", got.ToolIDs, want)
	}
	for i := range want {
		if got.ToolIDs[i] != want[i] {
			t.Fatalf("tool ids=%v want %v", got.ToolIDs, want)
		}
	}
}

// 格式非法的 mcp: 引用必须 400，而不是静默丢弃。
func TestNormalizeAgentRejectsInvalidMCPToolID(t *testing.T) {
	if _, err := normalizeAgent("p", upsertAgentBody{
		Name:    "n",
		Module:  "general",
		ToolIDs: []string{"mcp:"},
	}, systemdb.CloudAgent{}); err == nil {
		t.Fatal("invalid mcp id should be rejected")
	}
}

// 未知的非 mcp 工具 id 仍按原行为静默过滤。
func TestNormalizeAgentStillDropsUnknownTools(t *testing.T) {
	got, err := normalizeAgent("p", upsertAgentBody{
		Name:    "n",
		Module:  "general",
		ToolIDs: []string{"not-a-tool", "list_databases"},
	}, systemdb.CloudAgent{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolIDs) != 1 || got.ToolIDs[0] != "list_databases" {
		t.Fatalf("tool ids=%v", got.ToolIDs)
	}
}

// 宿主未装配 MCP 连接器时，保存 mcp: 引用必须报错（fail-closed）。
func TestNormalizeAgentRequiresMCPConnector(t *testing.T) {
	h := &CloudAgentHandler{}
	if _, err := h.validateAgentCapabilities("p", upsertAgentBody{
		Name:    "n",
		Module:  "general",
		ToolIDs: []string{"mcp:github"},
	}, systemdb.CloudAgent{}, writePrincipal()); err == nil {
		t.Fatal("mcp ids without a connector should be rejected")
	}
}
