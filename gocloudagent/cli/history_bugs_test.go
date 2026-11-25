// 文件说明：history_bugs_test.go — gocloudagent/cli 的单元测试（TestHistoryKeepsAssistantToolCallsForNextTurn、TestSecondCompactionCarriesPreviousSummary 等）
//
// 用法
//
//	go test ./gocloudagent/cli -run 'TestHistoryKeepsAssistantToolCallsForNextTurn|TestSecondCompactionCarriesPreviousSummary'
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/history_bugs_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_loop.go, agent_loop_test.go, budget_test.go, client.go, client_conn_reuse_test.go 等 39 个
//
// 主要符号
//
//	TestHistoryKeepsAssistantToolCallsForNextTurn(t *testing.T)
//	TestSecondCompactionCarriesPreviousSummary(t *testing.T)

package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// BUG-09：第二轮（或恢复会话）发给模型的历史里，工具结果必须有对应的
// assistant tool_calls，否则 native 协议下会出现孤儿 tool 结果。
func TestHistoryKeepsAssistantToolCallsForNextTurn(t *testing.T) {
	s := newTestSession(t)
	calls := 0
	client := &scriptedClient{replies: []agentcore.ChatResponse{
		{ToolCalls: []agentcore.ChatToolCall{{ID: "c1", Name: "echo", Arguments: `{"text":"ping"}`}}},
		{Content: "first answer"},
		{Content: "second answer"},
	}}
	r := &SessionRunner{Session: s, Client: client, Tools: []tool.BaseTool{echoTool(t, &calls)}}
	if _, err := r.Run(context.Background(), "turn one", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), "turn two", nil); err != nil {
		t.Fatal(err)
	}
	last := client.requests[len(client.requests)-1]
	var sawCall, sawResult bool
	for _, m := range last.Messages {
		for _, tc := range m.ToolCalls {
			if tc.ID == "c1" && tc.Name == "echo" {
				sawCall = true
			}
		}
		if m.ToolCallID == "c1" {
			sawResult = true
		}
	}
	if !sawResult {
		t.Fatalf("second-turn request must carry the tool result: %+v", last.Messages)
	}
	if !sawCall {
		t.Fatalf("tool result without its assistant tool_calls (orphan): %+v", last.Messages)
	}
}

// BUG-10：连续压缩时，新摘要必须把上一份摘要一并带上，否则更早的事实会被丢掉。
func TestSecondCompactionCarriesPreviousSummary(t *testing.T) {
	s := newTestSession(t)
	for i := 0; i < 6; i++ {
		seedTurn(t, s, strings.Repeat("u", 200), strings.Repeat("a", 200))
	}
	seedTurn(t, s, "mid question", "mid answer")
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "FIRST-SUMMARY"}, {Content: "SECOND-SUMMARY"}}}
	r := &SessionRunner{Session: s, Client: client, CompactKeepTokens: 8}
	if ok, err := r.ForceCompact(context.Background()); err != nil || !ok {
		t.Fatalf("first compaction ok=%v err=%v", ok, err)
	}
	for i := 0; i < 4; i++ {
		seedTurn(t, s, strings.Repeat("v", 200), strings.Repeat("b", 200))
	}
	seedTurn(t, s, "latest question", "latest answer")
	if ok, err := r.ForceCompact(context.Background()); err != nil || !ok {
		t.Fatalf("second compaction ok=%v err=%v", ok, err)
	}
	req := client.requests[len(client.requests)-1]
	var carried bool
	for _, m := range req.Messages {
		if strings.Contains(m.Content, "FIRST-SUMMARY") {
			carried = true
		}
	}
	if !carried {
		t.Fatalf("second summarization request must include the previous summary: %+v", req.Messages)
	}
}
