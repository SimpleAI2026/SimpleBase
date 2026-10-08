package api

// agent_e2e_stream_test.go：全链路流式回归（planv4.1 §5.2 E01–E05、E14）。
// 复现 V1（假流式）与 V8（离线全链路缺口）：默认 go test 即覆盖真实 HTTP SSE。

import (
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// plainChatScript 构造逐块输出的纯对话剧本。
func plainChatScript(blocks int, delay time.Duration) fakellm.Script {
	steps := make([]fakellm.Step, 0, blocks+2)
	for i := 0; i < blocks; i++ {
		steps = append(steps, fakellm.Step{Content: "hello块", Delay: delay})
	}
	steps = append(steps, fakellm.Step{FinishReason: "stop"}, fakellm.Step{PromptTokens: 11, CompletionTokens: 7, ReasoningTokens: 2})
	return fakellm.Script{Steps: steps}
}

// E01：纯对话流式——真流式回归（V1）。fake 每块 50ms 输出 5 块，
// 断言 token 帧数 ≥5（而非聚合后 1 帧）。
func TestAgentE2EPlainStreamIsRealStream(t *testing.T) {
	hs := newAgentHarness(t, nil, plainChatScript(5, 50*time.Millisecond))
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "打个招呼", nil)

	tokens := eventsOf(events, "token")
	if len(tokens) < 5 {
		t.Fatalf("expected >=5 token frames (real streaming), got %d: %+v", len(tokens), events)
	}
	var full strings.Builder
	for _, ev := range tokens {
		full.WriteString(ev.Content)
	}
	if !strings.Contains(full.String(), "hello") {
		t.Fatalf("content=%q", full.String())
	}
	// 帧序：run → token… → usage → end。
	if events[0].Type != "run" || events[len(events)-1].Type != "end" {
		t.Fatalf("frame order: first=%s last=%s", events[0].Type, events[len(events)-1].Type)
	}
	usage := eventsOf(events, "usage")
	if len(usage) != 1 || usage[0].PromptTokens != 11 || usage[0].CompletionTokens != 7 {
		t.Fatalf("usage=%+v", usage)
	}
}

// E02：文本协议工具往返（R02 场景）——恰好 1 次 list_databases，第二次请求带 TOOL_RESULT。
func TestAgentE2ETextProtocolToolRoundtrip(t *testing.T) {
	toolCall := fakellm.Script{
		Steps: []fakellm.Step{
			{Content: `TOOL_CALL {"name":"list_databases","arguments":{}}`, FinishReason: "tool_calls"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	final := fakellm.Script{
		MatchKeyword: "TOOL_RESULT",
		Steps: []fakellm.Step{
			{Content: "库里有 orders 和 users"},
			{FinishReason: "stop"},
			{PromptTokens: 9, CompletionTokens: 4},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{toolCall, final}, toolCall)
	hs.runtime.ToolProtocol = "text"
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "有哪些数据库", nil)

	calls := eventsOf(events, "tool_call")
	results := eventsOf(events, "tool_result")
	if len(calls) != 1 || len(results) != 1 {
		t.Fatalf("calls=%d results=%d events=%+v", len(calls), len(results), events)
	}
	if calls[0].Name != "list_databases" || calls[0].CallID == "" {
		t.Fatalf("call=%+v", calls[0])
	}
	if calls[0].CallID != results[0].CallID {
		t.Fatalf("call_id mismatch: %s vs %s", calls[0].CallID, results[0].CallID)
	}
	if !strings.Contains(results[0].Content, "orders") {
		t.Fatalf("tool result=%q", results[0].Content)
	}
	var content strings.Builder
	for _, ev := range eventsOf(events, "token") {
		content.WriteString(ev.Content)
	}
	if !strings.Contains(content.String(), "orders") {
		t.Fatalf("final content=%q", content.String())
	}
	// 第二次请求包含 TOOL_RESULT。
	reqs := hs.fake.Requests()
	if len(reqs) != 2 {
		t.Fatalf("upstream requests=%d", len(reqs))
	}
	if !strings.Contains(reqs[1].Messages[len(reqs[1].Messages)-1].Content.String(), "TOOL_RESULT") {
		t.Fatalf("second request should carry TOOL_RESULT, got %+v", reqs[1].Messages)
	}
}

// E03：原生协议工具往返——请求体带 tools；第二次请求含 role=tool + tool_call_id。
func TestAgentE2ENativeToolRoundtrip(t *testing.T) {
	toolCall := fakellm.Script{
		Steps: []fakellm.Step{
			{ToolID: "c1", ToolName: "list_databases", ToolArgs: `{}`, ToolIndex: 0},
			{FinishReason: "tool_calls"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	final := fakellm.Script{
		Seq: 2,
		Steps: []fakellm.Step{
			{Content: "共 2 个库"},
			{FinishReason: "stop"},
			{PromptTokens: 9, CompletionTokens: 4},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{toolCall, final}, toolCall)
	hs.runtime.ToolProtocol = "native"
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "有哪些数据库", nil)

	if len(eventsOf(events, "tool_call")) != 1 || len(eventsOf(events, "tool_result")) != 1 {
		t.Fatalf("events=%+v", events)
	}
	reqs := hs.fake.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests=%d", len(reqs))
	}
	if len(reqs[0].Tools) == 0 {
		t.Fatal("first request should carry tools")
	}
	// 第二次请求应含 role=tool 且 tool_call_id 与第一次一致。
	var toolMsg *fakellm.Message
	for i := range reqs[1].Messages {
		if reqs[1].Messages[i].Role == "tool" {
			toolMsg = &reqs[1].Messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatalf("second request missing role=tool: %+v", reqs[1].Messages)
	}
	if toolMsg.ToolCallID != "c1" {
		t.Fatalf("tool_call_id=%s", toolMsg.ToolCallID)
	}
}

// E04：auto 协议遇 400 tools not supported 时同 run 内降级 text。
func TestAgentE2EAutoFallbackOnToolsUnsupported(t *testing.T) {
	rejected := fakellm.Script{
		HTTPStatus: 400,
		Body:       `{"error":{"message":"tools not supported"}}`,
	}
	textTool := fakellm.Script{
		Seq: 2,
		Steps: []fakellm.Step{
			{Content: `TOOL_CALL {"name":"list_databases","arguments":{}}`, FinishReason: "tool_calls"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	final := fakellm.Script{
		Seq: 3,
		Steps: []fakellm.Step{
			{Content: "降级成功"},
			{FinishReason: "stop"},
			{PromptTokens: 8, CompletionTokens: 2},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{rejected, textTool, final}, rejected)
	hs.runtime.ToolProtocol = "auto"
	threadID := hs.createThread(t)
	events := hs.createRun(t, threadID, "有哪些数据库", nil)

	if len(eventsOf(events, "tool_call")) != 1 {
		t.Fatalf("fallback tool_call missing: %+v", events)
	}
	reqs := hs.fake.Requests()
	if len(reqs) < 2 || len(reqs[0].Tools) == 0 {
		t.Fatalf("requests=%+v", reqs)
	}
	if len(reqs[1].Tools) != 0 {
		t.Fatal("second request should drop tools (text protocol)")
	}
	// 第二个 run 首请求即不带 tools（降级缓存生效）。
	events2 := hs.createRun(t, threadID, "再来一次有哪些数据库", map[string]any{})
	if len(events2) == 0 {
		t.Fatal("second run produced no events")
	}
	if reqs2 := hs.fake.Requests(); len(reqs2) > 0 && len(reqs2[len(reqs2)-1].Tools) != 0 {
		t.Fatal("downgrade cache not applied on subsequent run")
	}
}

// E05：两轮记忆——第二轮上游收到的历史含 TOOL_HISTORY 与上轮结果。
func TestAgentE2ETwoTurnMemoryCarriesHistory(t *testing.T) {
	turn1 := fakellm.Script{
		Steps: []fakellm.Step{
			{Content: "第一轮回答 orders 库"},
			{FinishReason: "stop"},
			{PromptTokens: 5, CompletionTokens: 3},
		},
	}
	turn2 := fakellm.Script{
		MatchKeyword: "第二轮",
		Steps: []fakellm.Step{
			{Content: "第二轮回答"},
			{FinishReason: "stop"},
			{PromptTokens: 8, CompletionTokens: 2},
		},
	}
	hs := newAgentHarness(t, []fakellm.Script{turn1, turn2}, turn1)
	threadID := hs.createThread(t)
	hs.createRun(t, threadID, "第一轮问题", nil)
	hs.createRun(t, threadID, "第二轮问题", nil)

	reqs := hs.fake.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests=%d", len(reqs))
	}
	var joined string
	for _, m := range reqs[1].Messages {
		joined += m.Role + ":" + m.Content.String() + "\n"
	}
	if !strings.Contains(joined, "第一轮回答") {
		t.Fatalf("second turn missing prior assistant reply: %s", joined)
	}
	if !strings.Contains(joined, "第二轮问题") {
		t.Fatalf("second turn missing current question: %s", joined)
	}
}
