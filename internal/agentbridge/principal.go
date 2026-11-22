package agentbridge

import (
	"encoding/json"
	"strings"

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

func MessagesFor(msgs []systemdb.AgentMessage, currentAgentID string, names map[string]string) []cloudagent.Message {
	if msgs == nil {
		return nil
	}
	out := make([]cloudagent.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role != "assistant" || m.AgentID == "" || m.AgentID == currentAgentID {
			out = append(out, Message(m))
			continue
		}
		name := names[m.AgentID]
		if name == "" {
			name = "其他助手"
		}
		content := "[" + name + "] " + m.Content
		if tools := toolNames(m.ToolCallsJSON); len(tools) > 0 {
			content += "\n[" + name + "] 调用了 " + strings.Join(tools, "、")
		}
		out = append(out, cloudagent.Message{Role: "assistant", Content: content, ToolCallsJSON: "[]"})
	}
	return out
}

func toolNames(raw string) []string {
	if strings.TrimSpace(raw) == "" || raw == "[]" {
		return nil
	}
	var cards []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &cards); err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(cards))
	for _, card := range cards {
		if card.Name == "" || seen[card.Name] {
			continue
		}
		seen[card.Name] = true
		out = append(out, card.Name)
	}
	return out
}

func Message(m systemdb.AgentMessage) cloudagent.Message {
	return cloudagent.Message{Role: m.Role, Content: m.Content, ToolCallsJSON: m.ToolCallsJSON}
}
