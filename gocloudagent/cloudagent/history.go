// 文件说明：history.go — gocloudagent/cloudagent 的内部实现（historyToSchema、toolHistorySummary 等）
//
// 用法
//
//	同包调用：historyToSchema() → runtime.go, runtime_events_test.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/history.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] historyToSchema ← runtime.go, runtime_events_test.go
//
// 主要符号
//
//	historyToSchema(history []Message) []*schema.Message
//	toolHistorySummary(raw string) string

package cloudagent

import (
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

func historyToSchema(history []Message) []*schema.Message {
	selected := make([]Message, 0, len(history))
	budget := maxHistoryChars
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		message.Content = agentcore.Truncate(message.Content, 2000)
		if len(message.Content) > budget {
			break
		}
		budget -= len(message.Content)
		selected = append(selected, message)
	}
	out := make([]*schema.Message, 0, len(selected)*2)
	for i := len(selected) - 1; i >= 0; i-- {
		message := selected[i]
		switch message.Role {
		case "assistant":
			if summary := toolHistorySummary(message.ToolCallsJSON); summary != "" && len(summary) <= budget {
				out = append(out, schema.UserMessage(summary))
				budget -= len(summary)
			}
			out = append(out, schema.AssistantMessage(message.Content, nil))
		case "tool":
			out = append(out, schema.UserMessage("TOOL_RESULT\n"+message.Content))
		case "system":
			out = append(out, schema.SystemMessage(message.Content))
		default:
			out = append(out, schema.UserMessage(message.Content))
		}
	}
	return out
}

func toolHistorySummary(raw string) string {
	if raw == "" || raw == "[]" {
		return ""
	}
	var cards []struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Content   string `json:"content"`
	}
	if json.Unmarshal([]byte(raw), &cards) != nil || len(cards) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("TOOL_HISTORY\n")
	for i, card := range cards {
		if i >= 5 {
			break
		}
		b.WriteString("- " + agentcore.Truncate(card.Name+" "+card.Arguments+" → "+agentcore.RedactSecrets(card.Content), 600) + "\n")
	}
	return b.String()
}
