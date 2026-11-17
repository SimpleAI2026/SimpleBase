// 文件说明：history.go — 历史消息转换与 prompt 快照

package cloudagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

func (r *Runtime) buildSnapshot(ctx context.Context, req RunRequest) string {
	var b strings.Builder
	b.WriteString("Readonly snapshot (truncated, no secrets):\n")
	switch req.Agent.Module {
	case ModuleDatabase:
		if r.DB == nil {
			b.WriteString("- databases: unavailable\n")
			break
		}
		list, err := r.DB.ListDatabases(ctx, req.Principal, req.ProjectID)
		if err != nil {
			b.WriteString("- databases: " + err.Error() + "\n")
			break
		}
		b.WriteString(fmt.Sprintf("- databases (%d):\n", len(list)))
		for i, d := range list {
			if i >= 20 {
				b.WriteString("  …\n")
				break
			}
			b.WriteString(fmt.Sprintf("  - %s %s status=%s\n", d.Name, d.ID, d.Status))
		}
	case ModuleS3:
		if r.Obj == nil {
			b.WriteString("- objects: unavailable\n")
			break
		}
		list, err := r.Obj.ListObjects(ctx, req.ProjectID, "", 20)
		if err != nil {
			b.WriteString("- objects: " + err.Error() + "\n")
			break
		}
		b.WriteString(fmt.Sprintf("- objects (%d shown):\n", len(list)))
		for _, o := range list {
			b.WriteString(fmt.Sprintf("  - %s size=%d\n", o.Key, o.Size))
		}
	case ModuleLogs:
		if r.Logs == nil {
			b.WriteString("- logs: unavailable\n")
			break
		}
		if stats, err := r.Logs.LevelStats(ctx, req.ProjectID); err == nil {
			b.WriteString("- log levels: ")
			for i, s := range stats {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(fmt.Sprintf("%s=%d", s.Level, s.Count))
			}
			b.WriteString("\n")
		}
	default:
		b.WriteString("- no module snapshot\n")
	}
	return agentcore.RedactSecrets(b.String())
}

func historyToSchema(hist []Message) []*schema.Message {
	// 从最近的会话向前选取，正文优先，工具摘要在剩余预算中插入。
	selected := make([]Message, 0, len(hist))
	budget := maxHistoryChars
	for i := len(hist) - 1; i >= 0; i-- {
		content := truncate(hist[i].Content, 2000)
		if len(content) > budget {
			break
		}
		budget -= len(content)
		selected = append(selected, hist[i])
	}
	out := make([]*schema.Message, 0, len(selected)*2)
	for i := len(selected) - 1; i >= 0; i-- {
		m := selected[i]
		content := truncate(m.Content, 2000)
		switch m.Role {
		case "assistant":
			if summary := toolHistorySummary(m.ToolCallsJSON); summary != "" && len(summary) <= budget {
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
