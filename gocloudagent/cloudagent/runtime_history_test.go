// 文件说明：runtime_history_test.go — gocloudagent/cloudagent 的单元测试（TestRuntimeNilAndCancel、TestHistoryAndAgentName 等）
//
// 用法
//
//	go test ./gocloudagent/cloudagent -run 'TestRuntimeNilAndCancel|TestHistoryAndAgentName|TestConsumeVariantAndAssistant'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/runtime_history_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//
// 主要符号
//
//	TestRuntimeNilAndCancel(t *testing.T)
//	TestHistoryAndAgentName(t *testing.T)
//	TestConsumeVariantAndAssistant(t *testing.T)

package cloudagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

func TestRuntimeNilAndCancel(t *testing.T) {
	var rt *Runtime
	if _, err := rt.StartRun(context.Background(), RunRequest{}, nil); err == nil {
		t.Fatal("nil runtime")
	}
	rt = &Runtime{}
	if _, err := rt.StartRun(context.Background(), RunRequest{}, nil); err == nil {
		t.Fatal("nil llm")
	}
	if rt.CancelRun("") || rt.CancelRun("missing") {
		t.Fatal("cancel empty/missing")
	}
	rt.storeCancel("", func() {})
	called := false
	rt.storeCancel("r1", func() { called = true })
	if !rt.CancelRun("r1") || !called {
		t.Fatal("cancel in-flight")
	}
	rt.clearCancel("r1")
	rt.clearCancel("gone")
	(*Runtime)(nil).storeCancel("x", func() {})
	(*Runtime)(nil).clearCancel("x")
	if (*Runtime)(nil).CancelRun("x") {
		t.Fatal("nil cancel")
	}
}

func TestHistoryAndAgentName(t *testing.T) {
	history := historyToSchema([]Message{
		{Role: "assistant", Content: "a"}, {Role: "tool", Content: "t"},
		{Role: "system", Content: "s"}, {Role: "user", Content: "u"},
	})
	if len(history) != 4 || history[0].Role != schema.Assistant || history[2].Role != schema.System {
		t.Fatalf("history=%+v", history)
	}
	big := strings.Repeat("x", 3000)
	var lots []Message
	for i := 0; i < 8; i++ {
		lots = append(lots, Message{Role: "user", Content: big})
	}
	if len(historyToSchema(lots)) >= 8 {
		t.Fatal("history should stop after maxHistoryChars")
	}
	if safeAgentName("  ") != "agent" || safeAgentName(" Bot ") != "Bot" {
		t.Fatal("safeAgentName")
	}
}

func TestConsumeVariantAndAssistant(t *testing.T) {
	if content, name := agentcore.ConsumeVariant(nil); content != "" || name != "" {
		t.Fatal("nil variant")
	}
	content, name := agentcore.ConsumeVariant(&adk.MessageVariant{Message: &schema.Message{Content: "body", ToolName: "tn"}})
	if content != "body" || name != "tn" {
		t.Fatalf("non-stream %s %s", content, name)
	}
	if content, name = agentcore.ConsumeVariant(&adk.MessageVariant{ToolName: "only"}); content != "" || name != "only" {
		t.Fatalf("empty message %s %s", content, name)
	}

	sr, sw := schema.Pipe[*schema.Message](4)
	sw.Send(&schema.Message{Content: "p1", ToolName: "tool-a"}, nil)
	sw.Send(nil, nil)
	sw.Send(&schema.Message{Content: "p2"}, nil)
	sw.Close()
	content, name = agentcore.ConsumeVariant(&adk.MessageVariant{IsStreaming: true, MessageStream: sr})
	if content != "p1p2" || name != "tool-a" {
		t.Fatalf("stream variant %q %q", content, name)
	}
	if msg, err := consumeAssistant(nil, nil, nil); msg != nil || err != nil {
		t.Fatal("nil assistant")
	}
	var acc strings.Builder
	var emitted []Event
	emit := func(e Event) { emitted = append(emitted, e) }
	msg, err := consumeAssistant(&adk.MessageVariant{Message: schema.AssistantMessage("tok", nil)}, emit, &acc)
	if err != nil || msg.Content != "tok" || acc.String() != "tok" || len(emitted) != 1 {
		t.Fatalf("acc=%q events=%v err=%v", acc.String(), emitted, err)
	}
	msg, err = consumeAssistant(&adk.MessageVariant{Message: schema.AssistantMessage("", []schema.ToolCall{{Function: schema.FunctionCall{Name: "t"}}})}, emit, &acc)
	if err != nil || msg == nil || len(msg.ToolCalls) != 1 {
		t.Fatal("tool-call message not accumulated")
	}
	if msg, err = consumeAssistant(&adk.MessageVariant{}, nil, &acc); msg != nil || err != nil {
		t.Fatal("empty variant")
	}

	sr, sw = schema.Pipe[*schema.Message](8)
	sw.Send(nil, nil)
	sw.Send(&schema.Message{Content: "chunk", Role: schema.Assistant}, nil)
	sw.Send(&schema.Message{Content: "TOOL_CALL {}", Role: schema.Assistant}, nil)
	sw.Send(&schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{Function: schema.FunctionCall{Name: "x", Arguments: "{}"}}}}, nil)
	sw.Send(nil, errors.New("mid-stream"))
	sw.Close()
	acc.Reset()
	emitted = nil
	last, err := consumeAssistant(&adk.MessageVariant{IsStreaming: true, MessageStream: sr}, emit, &acc)
	if err == nil || last == nil || !strings.Contains(acc.String(), "chunk") {
		t.Fatalf("last=%v acc=%q err=%v", last, acc.String(), err)
	}
	sr, sw = schema.Pipe[*schema.Message](2)
	sw.Close()
	if _, err := consumeAssistant(&adk.MessageVariant{IsStreaming: true, MessageStream: sr}, nil, &acc); err != nil {
		t.Fatal(err)
	}
}
