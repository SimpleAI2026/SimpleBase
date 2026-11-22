// planv6.7 AGC-ROUTE §7.5：MessagesFor 跨助手历史投影测试。
// 同助手原样 / 他助手前缀+摘要 / 空 agent_id 原样 / 坏 JSON 不 panic。
package agentbridge

import (
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func TestMessagesForProjection(t *testing.T) {
	msgs := []systemdb.AgentMessage{
		{Role: "user", Content: "帮我看看 orders 表结构"},
		{Role: "assistant", AgentID: "db-agent", Content: "orders 包含 id、status 和 created_at。", ToolCallsJSON: `[{"name":"list_databases"},{"name":"readonly_sql"}]`},
		{Role: "assistant", AgentID: "s3-agent", Content: "报表文件位于 reports/ 下。", ToolCallsJSON: `[{"name":"list_objects"},{"name":"head_object"}]`},
		{Role: "assistant", AgentID: "", Content: "无身份历史（旧数据）"},
		{Role: "assistant", AgentID: "db-agent", Content: "本轮助手自己的消息"},
	}
	names := map[string]string{"db-agent": "数据库助手", "s3-agent": "对象存储助手"}

	out := MessagesFor(msgs, "db-agent", names)
	if len(out) != 5 {
		t.Fatalf("len: %d", len(out))
	}
	// user 原样
	if out[0].Role != "user" || out[0].Content != "帮我看看 orders 表结构" {
		t.Fatalf("user: %+v", out[0])
	}
	// 本轮助手（db-agent）：原样，工具卡保留
	if out[1].Content != "orders 包含 id、status 和 created_at。" || out[1].ToolCallsJSON != `[{"name":"list_databases"},{"name":"readonly_sql"}]` {
		t.Fatalf("own assistant: %+v", out[1])
	}
	// 其他助手（s3-agent）：前缀 + 工具摘要 + ToolCallsJSON 清空
	if !strings.HasPrefix(out[2].Content, "[对象存储助手] 报表文件位于 reports/ 下。") {
		t.Fatalf("prefix: %q", out[2].Content)
	}
	if !strings.Contains(out[2].Content, "[对象存储助手] 调用了 list_objects、head_object") {
		t.Fatalf("tool summary: %q", out[2].Content)
	}
	if out[2].ToolCallsJSON != "[]" {
		t.Fatalf("tools cleared: %q", out[2].ToolCallsJSON)
	}
	// 空 agent_id 原样
	if out[3].Content != "无身份历史（旧数据）" {
		t.Fatalf("empty agent: %+v", out[3])
	}
	// 本轮助手自己的消息原样
	if out[4].Content != "本轮助手自己的消息" {
		t.Fatalf("current: %+v", out[4])
	}
}

func TestMessagesForUnknownAgentName(t *testing.T) {
	msgs := []systemdb.AgentMessage{
		{Role: "assistant", AgentID: "ghost", Content: "hi"},
	}
	out := MessagesFor(msgs, "db-agent", nil)
	if !strings.HasPrefix(out[0].Content, "[其他助手] hi") {
		t.Fatalf("fallback name: %q", out[0].Content)
	}
}

func TestMessagesForBadToolJSONNoPanic(t *testing.T) {
	msgs := []systemdb.AgentMessage{
		{Role: "assistant", AgentID: "other", Content: "x", ToolCallsJSON: "{not-json"},
	}
	out := MessagesFor(msgs, "db-agent", nil)
	if len(out) != 1 || out[0].Content != "[其他助手] x" {
		t.Fatalf("bad json: %+v", out)
	}
}

func TestToolNames(t *testing.T) {
	if got := toolNames(`[{"name":"a"},{"name":"b"},{"name":"a"}]`); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("dedupe: %v", got)
	}
	if got := toolNames(""); got != nil {
		t.Fatalf("empty: %v", got)
	}
	if got := toolNames("[]"); got != nil {
		t.Fatalf("empty array: %v", got)
	}
	if got := toolNames("garbage"); got != nil {
		t.Fatalf("garbage: %v", got)
	}
	// 空名字跳过
	if got := toolNames(`[{"name":""},{"name":"c"}]`); len(got) != 1 || got[0] != "c" {
		t.Fatalf("blank names: %v", got)
	}
}

func TestMessagesForNil(t *testing.T) {
	out := MessagesFor(nil, "a", nil)
	if out != nil {
		t.Fatalf("nil: %+v", out)
	}
}
