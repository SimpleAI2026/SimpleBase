// 文件说明：mcp_access.go — gocloudagent 中 MCPConnector、MCPPolicy 等符号的实现
//
// 用法
//
//	MCPConnector.Connect(ctx, p, projectID, serverIDs) (mcp.SessionSet, error)
//	MCPPolicy.Classify(serverID, tool) (CommandDecision, error)
//
// 调用架构
//
//	[上游] gocloudagent/runtime.go, gocloudagent/cli_tool.go
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/mcp_access.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/mcp
//	[同包] mcp_tool.go, runtime.go, tools.go
//
// 主要符号
//
//	type MCPConnector interface {…}
//	type MCPPolicy interface {…}
//	MCPToolIDs(ids []string) []string
//	MCPToolRefs(ids []string) []MCPToolRef

package gocloudagent

import (
	"context"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
)

// MCPConnector 由宿主实现：凭据与网络出口策略都留在宿主，模块拿不到密钥（planv6.2 §4.1）。
//
// Connect 返回本轮可用的 MCP 会话；serverIDs 已由宿主按项目授权校验。
// 单个 Server 失败不应终止 run——实现方应跳过该 Server 并在错误里说明，
// 由调用方决定是否降级（§4.3）。
type MCPConnector interface {
	Connect(ctx context.Context, p Principal, projectID string, serverIDs []string) (mcp.SessionSet, error)
}

// MCPPolicy 给出每个 MCP 工具的只读/破坏性判定（planv6.2 §4.2-1）。
//
// 事实来源是宿主配置，不是 Server 自报的 annotations：
// 未标记的工具一律按「写」处理，由调用方在确认之前拦截。
type MCPPolicy interface {
	// Classify 按项目 + Server + 工具判定。projectID 显式传入，
	// 避免策略实现持有可变的「当前项目」状态。
	Classify(ctx context.Context, projectID, serverID, tool string) (CommandDecision, error)
}

// MCPToolRef 是一条 mcp: 引用（整个 Server 或单个工具）。
type MCPToolRef struct {
	ServerID string
	// Tool 为空表示启用整个 Server。
	Tool string
}

// MCPToolRefs 从 Agent 的 tool_ids 里挑出 mcp: 引用。
// 非法条目（缺 server id）被丢弃，由 normalizeAgent 在保存时拒绝。
func MCPToolRefs(ids []string) []MCPToolRef {
	out := make([]MCPToolRef, 0, len(ids))
	for _, id := range ids {
		serverID, tool, ok := mcp.ParseToolID(id)
		if !ok {
			continue
		}
		out = append(out, MCPToolRef{ServerID: serverID, Tool: tool})
	}
	return out
}

// MCPServerIDs 返回去重后的 Server 标识列表（保持首次出现顺序）。
func MCPServerIDs(refs []MCPToolRef) []string {
	seen := make(map[string]bool, len(refs))
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		if seen[r.ServerID] {
			continue
		}
		seen[r.ServerID] = true
		out = append(out, r.ServerID)
	}
	return out
}

// HasMCPTools 报告 tool_ids 里是否有 MCP 引用。
func HasMCPTools(ids []string) bool {
	for _, id := range ids {
		if mcp.IsMCPToolID(id) {
			return true
		}
	}
	return false
}

// mcpToolFilter 决定某个 Server 的某个工具是否被 Agent 选中。
// 空 tool 表示整个 Server 都启用。
func mcpToolFilter(refs []MCPToolRef, serverID string) (all bool, only map[string]bool) {
	only = make(map[string]bool)
	for _, r := range refs {
		if r.ServerID != serverID {
			continue
		}
		if strings.TrimSpace(r.Tool) == "" {
			all = true
			continue
		}
		only[r.Tool] = true
	}
	return all, only
}
