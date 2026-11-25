// 文件说明：principal.go — internal/agentbridge 中 Agent、Messages 等符号的实现
//
// 用法
//
//	Agent(a systemdb.CloudAgent) cloudagent.Agent  // Agent converts a stored SimpleBase agent to the generic runtime DTO.
//	Messages(msgs []systemdb.AgentMessage) []cloudagent.Message
//	Message(m systemdb.AgentMessage) cloudagent.Message
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/principal.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] access_db.go, access_llm.go, access_object.go, access_test.go, bridge_test.go 等 33 个
//	[关键引用] Agent ← cron_agent.go；Messages ← cron_agent.go
//
// 主要符号
//
//	Agent(a systemdb.CloudAgent) cloudagent.Agent
//	Messages(msgs []systemdb.AgentMessage) []cloudagent.Message
//	Message(m systemdb.AgentMessage) cloudagent.Message

package agentbridge

import (
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// Agent converts a stored SimpleBase agent to the generic runtime DTO.
func Agent(a systemdb.CloudAgent) cloudagent.Agent {
	return cloudagent.Agent{
		ID: a.ID, Name: a.Name, Module: a.Module, Description: a.Description,
		SystemPrompt: a.SystemPrompt, ToolIDs: append([]string(nil), a.ToolIDs...),
		ModelOverride: a.ModelOverride, Writable: true,
	}
}

func Messages(msgs []systemdb.AgentMessage) []cloudagent.Message {
	if msgs == nil {
		return nil
	}
	out := make([]cloudagent.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, Message(m))
	}
	return out
}

func Message(m systemdb.AgentMessage) cloudagent.Message {
	return cloudagent.Message{Role: m.Role, Content: m.Content, ToolCallsJSON: m.ToolCallsJSON}
}
