// 文件说明：messages.go — gocloudagent/agentcore 中 UserSchemaMessage、SystemSchemaMessage 等符号的…
//
// 用法
//
//	UserSchemaMessage(content string) *schema.Message
//	SystemSchemaMessage(content string) *schema.Message
//
// 调用架构
//
//	[上游] gocloudagent, gocloudagent/cli
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/agentcore/messages.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] model.go, redact.go, stream.go, types.go
//
// 主要符号
//
//	UserSchemaMessage(content string) *schema.Message
//	SystemSchemaMessage(content string) *schema.Message
//	AssistantSchemaMessage(content string) *schema.Message
//	ToolResultSchemaMessage(name, callID, content string) *schema.Message

package agentcore

import "github.com/cloudwego/eino/schema"

// messages.go 提供构造 eino 消息的小工具，供云端与本地 CLI 共用同一套转换，
// 避免两侧对「tool 结果怎么进历史」的理解分叉（云端历史投影见 gocloudagent.historyToSchema）。

// UserSchemaMessage 构造用户消息。
func UserSchemaMessage(content string) *schema.Message { return schema.UserMessage(content) }

// SystemSchemaMessage 构造系统消息。
func SystemSchemaMessage(content string) *schema.Message { return schema.SystemMessage(content) }

// AssistantSchemaMessage 构造助手消息（无工具调用）。
func AssistantSchemaMessage(content string) *schema.Message {
	return schema.AssistantMessage(content, nil)
}

// ToolResultSchemaMessage 构造工具结果消息（带 name 与 call id，便于原生协议回传）。
func ToolResultSchemaMessage(name, callID, content string) *schema.Message {
	return &schema.Message{Role: schema.Tool, Content: content, ToolName: name, ToolCallID: callID}
}
