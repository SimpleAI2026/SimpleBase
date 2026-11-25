// 文件说明：naming.go — gocloudagent/mcp 中 ToolName、DisplayName 等符号的实现
//
// 用法
//
//	ToolName(serverID, tool string) string  // ToolName 生成暴露给模型的工具名：mcp__<server>__<tool>（§3.2）
//	DisplayName(name string) string  // DisplayName 把 mcp__server__tool 还原成「server · tool」供前端展示（§4.4）
//	IsMCPToolID(id string) bool  // IsMCPToolID 报告 Agent 的 tool_ids 条目是否为 MCP 引用（§3.2）
//	ParseToolID(id string) (serverID, tool string, ok bool)  // ParseToolID 拆解 mcp:<server>[/<tool>]
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/tools, internal/agentbridge 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/mcp/naming.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] adapter.go, adapter_test.go, client.go, http_transport.go, http_transport_test.go 等 10 个
//	[关键引用] ToolName ← naming_test.go；DisplayName ← naming_test.go；IsMCPToolID ← naming_test.go
//
// 主要符号
//
//	ToolName(serverID, tool string) string
//	sanitizeSegment(v string) string
//	DisplayName(name string) string
//	IsMCPToolID(id string) bool
//	ParseToolID(id string) (serverID, tool string, ok bool)

package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// ToolName 生成暴露给模型的工具名：mcp__<server>__<tool>（§3.2）。
//
// 两段都清洗为 [a-zA-Z0-9_-]，整体 ≤ 64 字符；超长时截断并追加 6 位 hash，
// 保证同名不同源的工具不会互相覆盖。
func ToolName(serverID, tool string) string {
	s := sanitizeSegment(serverID)
	t := sanitizeSegment(tool)
	if s == "" {
		s = "server"
	}
	if t == "" {
		t = "tool"
	}
	name := "mcp__" + s + "__" + t
	if len(name) <= 64 {
		return name
	}
	sum := sha256.Sum256([]byte(serverID + "\x00" + tool))
	suffix := "__" + hex.EncodeToString(sum[:])[:6]
	keep := 64 - len(suffix)
	if keep < len("mcp__") {
		keep = len("mcp__")
	}
	return name[:keep] + suffix
}

// sanitizeSegment 把任意标识清洗为 [a-zA-Z0-9_-]，其余字符替换为下划线。
func sanitizeSegment(v string) string {
	var b strings.Builder
	b.Grow(len(v))
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteByte('_')
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// DisplayName 把 mcp__server__tool 还原成「server · tool」供前端展示（§4.4）。
// 非 MCP 工具名原样返回。
func DisplayName(name string) string {
	if !strings.HasPrefix(name, "mcp__") {
		return name
	}
	rest := strings.TrimPrefix(name, "mcp__")
	idx := strings.Index(rest, "__")
	if idx <= 0 {
		return name
	}
	return rest[:idx] + " · " + rest[idx+2:]
}

// IsMCPToolID 报告 Agent 的 tool_ids 条目是否为 MCP 引用（§3.2）。
// 形如 mcp:<server_id> 或 mcp:<server_id>/<tool>。
func IsMCPToolID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "mcp:")
}

// ParseToolID 拆解 mcp:<server>[/<tool>]。
// tool 为空表示启用整个 Server。
func ParseToolID(id string) (serverID, tool string, ok bool) {
	v := strings.TrimSpace(id)
	if !strings.HasPrefix(v, "mcp:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(v, "mcp:")
	if rest == "" {
		return "", "", false
	}
	if idx := strings.Index(rest, "/"); idx >= 0 {
		serverID, tool = rest[:idx], rest[idx+1:]
	} else {
		serverID = rest
	}
	if strings.TrimSpace(serverID) == "" {
		return "", "", false
	}
	return serverID, tool, true
}
