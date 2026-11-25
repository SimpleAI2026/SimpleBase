// 文件说明：runtime_optimization_test.go — gocloudagent/cloudagent 的单元测试（TestAgentStreamForwardsBeforeUpstreamCompletes、TestAgentStreamToolPrefixAndLiteral 等）
//
// 用法
//
//	go test ./gocloudagent/cloudagent -run 'TestAgentStreamForwardsBeforeUpstreamCompletes|TestAgentStreamToolPrefixAndLiteral|TestAgentHistoryIncludesToolResults|TestAgentRunToolEventsHaveMatchingCallID'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/runtime_optimization_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//
// 主要符号
//
//	type blockingStream struct{…}
//	(s *blockingStream) Next() (string, bool, error)
//	(*blockingStream) Close() error
//	type blockingChat struct{…}
//	(*blockingChat) Chat(context.Context, string, ChatRequest) (ChatResponse, error)
//	(c *blockingChat) Stream(context.Context, string, ChatRequest) (TokenStream, error)
//	TestAgentStreamForwardsBeforeUpstreamCompletes(t *testing.T)
//	TestAgentStreamToolPrefixAndLiteral(t *testing.T)
//	... 另有 3 个顶层符号

package cloudagent

import (
	"context"
	"errors"
	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"io"
	"strings"
	"testing"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type blockingStream struct {
	start chan struct{}
	first bool
}

func (s *blockingStream) Next() (string, bool, error) {
	if !s.first {
		s.first = true
		return "你好", false, nil
	}
	<-s.start
	return "世界", true, nil
}
func (*blockingStream) Close() error { return nil }

type blockingChat struct{ stream *blockingStream }

func (*blockingChat) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, nil
}
func (c *blockingChat) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	return c.stream, nil
}

func TestAgentStreamForwardsBeforeUpstreamCompletes(t *testing.T) {
	upstream := &blockingStream{start: make(chan struct{})}
	defer close(upstream.start)
	m := agentcore.NewModel(&blockingChat{stream: upstream}, "p", "m")
	sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	got := make(chan string, 1)
	go func() {
		msg, _ := sr.Recv()
		if msg != nil {
			got <- msg.Content
		}
	}()
	select {
	case text := <-got:
		if text != "你好" {
			t.Fatalf("first chunk=%q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("stream buffered whole response")
	}
}

func TestAgentStreamToolPrefixAndLiteral(t *testing.T) {
	for _, tc := range []struct {
		chunks   []string
		wantTool bool
		wantText string
	}{
		{[]string{"TOOL", `_CALL {"name":"echo","arguments":{}}`}, true, ""},
		{[]string{"示例：", "TOOL_CALL 是关键字"}, false, "示例：TOOL_CALL 是关键字"},
		{[]string{"\n  TOOL", `_CALL {"name":"echo","arguments":{}}`}, true, ""},
	} {
		m := agentcore.NewModel(&scriptedChat{streamChunks: tc.chunks}, "p", "m")
		sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("q")})
		if err != nil {
			t.Fatal(err)
		}
		var text string
		var tools int
		for {
			msg, err := sr.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			text += msg.Content
			tools += len(msg.ToolCalls)
		}
		if text != tc.wantText || (tools > 0) != tc.wantTool {
			t.Fatalf("chunks=%v text=%q tools=%d", tc.chunks, text, tools)
		}
	}
}

func TestAgentHistoryIncludesToolResults(t *testing.T) {
	msg := Message{Role: "assistant", Content: "Done", ToolCallsJSON: `[{"name":"echo","content":"orders, users; sk-abcdefghijklmnopqrstuvwxyz"}]`}
	hist := historyToSchema([]Message{msg})
	if len(hist) != 2 || !strings.Contains(hist[0].Content, "orders, users") || strings.Contains(hist[0].Content, "sk-abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("history=%+v", hist)
	}
}

func TestAgentRunToolEventsHaveMatchingCallID(t *testing.T) {
	rt := &Runtime{LLM: &scriptedLLM{}, Tools: []einotool.BaseTool{testTool(t, "echo", nil)}}
	var calls, results []Event
	res, err := rt.StartRun(context.Background(), RunRequest{
		ThreadID: "th", RunID: "r1", Agent: Agent{ID: "a", Name: "Agent"},
		UserText: "run", Stream: true,
	}, func(e Event) {
		if e.Type == "tool_call" {
			calls = append(calls, e)
		}
		if e.Type == "tool_result" {
			results = append(results, e)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || len(results) != 1 || calls[0].CallID == "" || results[0].CallID != calls[0].CallID || res.ToolCalls != 1 {
		t.Fatalf("calls=%+v results=%+v res=%+v", calls, results, res)
	}
}

func TestAgentThreadBusy(t *testing.T) {
	rt := &Runtime{}
	if err := rt.ClaimThread("th", "r1"); err != nil {
		t.Fatal(err)
	}
	if err := rt.ClaimThread("th", "r2"); !errors.Is(err, ErrThreadBusy) {
		t.Fatalf("busy=%v", err)
	}
	rt.ReleaseThread("th", "r1")
	if err := rt.ClaimThread("th", "r2"); err != nil {
		t.Fatal(err)
	}
}
