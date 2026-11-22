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
		content := truncate(history[i].Content, 2000)
		if len(content) > budget {
			break
		}
		budget -= len(content)
		selected = append(selected, history[i])
	}
	out := make([]*schema.Message, 0, len(selected)*2)
	for i := len(selected) - 1; i >= 0; i-- {
		message := selected[i]
		content := truncate(message.Content, 2000)
		switch message.Role {
		case "assistant":
			if summary := toolHistorySummary(message.ToolCallsJSON); summary != "" && len(summary) <= budget {
				out = append(out, schema.UserMessage(summary))
				budget -= len(summary)
			}
			out = append(out, schema.AssistantMessage(content, nil))
		case "tool":
			out = append(out, schema.UserMessage("TOOL_RESULT\n"+content))
		case "system":
			out = append(out, schema.SystemMessage(content))
		default:
			out = append(out, schema.UserMessage(content))
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
		b.WriteString("- " + truncate(card.Name+" "+card.Arguments+" → "+agentcore.RedactSecrets(card.Content), 600) + "\n")
	}
	return b.String()
}
