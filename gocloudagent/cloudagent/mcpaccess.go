// 文件说明：mcpaccess.go — gocloudagent/cloudagent 中 MCPConnector、MCPPolicy 等符号的实现
//
// 用法
//
//	type MCPConnector interface{…}  // MCPConnector establishes the sessions selected for one run. Credentials and netw...
//	type MCPPolicy interface{…}  // MCPPolicy classifies a selected MCP tool. Unknown tools must fail closed.
//	type MCPToolRef struct{…}  // MCPToolRef selects a whole MCP server or one of its tools.
//	HasMCPTools(ids []string) bool  // HasMCPTools reports whether tool IDs contain an MCP reference.
//	同包调用：mcpToolRefs() → mcptool.go, mcptool_test.go；mcpServerIDs() → mcptool.go, mcptool_test.go；mcpToolFilter() → mcptool.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/mcpaccess.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] MCPConnector ← mcptool.go, runtime.go；MCPPolicy ← mcptool.go, runtime.go；mcpToolRefs ← mcptool.go, mcptool_test.go
//
// 主要符号
//
//	type MCPConnector interface{…}
//	type MCPPolicy interface{…}
//	type MCPToolRef struct{…}
//	mcpToolRefs(ids []string) []MCPToolRef
//	mcpServerIDs(refs []MCPToolRef) []string
//	HasMCPTools(ids []string) bool
//	mcpToolFilter(refs []MCPToolRef, serverID string) (all bool, only map[string]bool)

package cloudagent

import (
	"context"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
)

// MCPConnector establishes the sessions selected for one run. Credentials and
// network policy belong to the connector implementation.
type MCPConnector interface {
	Connect(ctx context.Context, req RunRequest, serverIDs []string) (mcp.SessionSet, error)
}

// MCPPolicy classifies a selected MCP tool. Unknown tools must fail closed.
type MCPPolicy interface {
	Classify(ctx context.Context, req RunRequest, serverID, tool string) (ToolDecision, error)
}

// MCPToolRef selects a whole MCP server or one of its tools.
type MCPToolRef struct {
	ServerID string
	Tool     string
}

func mcpToolRefs(ids []string) []MCPToolRef {
	out := make([]MCPToolRef, 0, len(ids))
	for _, id := range ids {
		serverID, tool, ok := mcp.ParseToolID(id)
		if ok {
			out = append(out, MCPToolRef{ServerID: serverID, Tool: tool})
		}
	}
	return out
}

func mcpServerIDs(refs []MCPToolRef) []string {
	seen := make(map[string]bool, len(refs))
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if seen[ref.ServerID] {
			continue
		}
		seen[ref.ServerID] = true
		out = append(out, ref.ServerID)
	}
	return out
}

// HasMCPTools reports whether tool IDs contain an MCP reference.
func HasMCPTools(ids []string) bool {
	for _, id := range ids {
		if mcp.IsMCPToolID(id) {
			return true
		}
	}
	return false
}

func mcpToolFilter(refs []MCPToolRef, serverID string) (all bool, only map[string]bool) {
	only = make(map[string]bool)
	for _, ref := range refs {
		if ref.ServerID != serverID {
			continue
		}
		if strings.TrimSpace(ref.Tool) == "" {
			all = true
			continue
		}
		only[ref.Tool] = true
	}
	return all, only
}
