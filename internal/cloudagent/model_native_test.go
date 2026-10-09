package cloudagent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/voocel/litellm/providers"
)

type nativeChat struct {
	requests []ChatRequest
	failOnce bool
}

func (c *nativeChat) Chat(_ context.Context, _ string, req ChatRequest) (ChatResponse, error) {
	c.requests = append(c.requests, req)
	if c.failOnce && len(c.requests) == 1 {
		return ChatResponse{}, &providers.LiteLLMError{StatusCode: 400, Message: "tools not supported"}
	}
	return ChatResponse{Content: "done", ToolCalls: []ChatToolCall{{ID: "call1", Name: "list_databases", Arguments: "{}"}}}, nil
}
func (c *nativeChat) Stream(_ context.Context, _ string, req ChatRequest) (TokenStream, error) {
	c.requests = append(c.requests, req)
	return &nativeDeltaStream{deltas: []StreamDelta{
		{ToolCall: &ToolCallDelta{Index: 0, ID: "call1", Name: "list_databases"}},
		{ToolCall: &ToolCallDelta{Index: 0, ArgsDelta: "{}"}},
		{Usage: &ChatUsage{PromptTokens: 5, CompletionTokens: 6}, Finish: true},
	}}, nil
}

type nativeDeltaStream struct {
	deltas []StreamDelta
	index  int
}

func (s *nativeDeltaStream) NextDelta() (StreamDelta, error) {
	if s.index >= len(s.deltas) {
		return StreamDelta{}, io.EOF
	}
	out := s.deltas[s.index]
	s.index++
	return out, nil
}
func (s *nativeDeltaStream) Next() (string, bool, error) {
	d, e := s.NextDelta()
	return d.Content, d.Finish, e
}
func (s *nativeDeltaStream) Close() error { return nil }

func TestAgentNativeToolsAndUsage(t *testing.T) {
	client := &nativeChat{}
	m := newGatewayChatModel(client, "p", "native-test")
	m.protocol = "native"
	tools := []*schema.ToolInfo{{Name: "list_databases", Desc: "list"}}
	if err := m.BindTools(tools); err != nil {
		t.Fatal(err)
	}
	msgs := []*schema.Message{schema.UserMessage("list")}
	got, err := m.Generate(context.Background(), msgs)
	if err != nil || len(got.ToolCalls) != 1 {
		t.Fatalf("message=%+v err=%v", got, err)
	}
	if len(client.requests[0].Tools) != 1 || strings.Contains(client.requests[0].Messages[0].Content, "TOOL_CALL") {
		t.Fatalf("native request=%+v", client.requests[0])
	}
	var tokens int
	m.usage = func(u ChatUsage) { tokens += u.PromptTokens + u.CompletionTokens }
	sr, err := m.Stream(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	var chunks []*schema.Message
	var sawDelta bool
	for {
		msg, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, msg)
		if len(msg.ToolCalls) > 0 && msg.ToolCalls[0].Extra != nil && msg.ToolCalls[0].Extra["delta"] == true {
			sawDelta = true
		}
	}
	if !sawDelta {
		t.Fatal("expected tool call deltas")
	}
	msg, err := schema.ConcatMessages(chunks)
	if err != nil || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call1" || msg.ToolCalls[0].Function.Name != "list_databases" || msg.ToolCalls[0].Function.Arguments != "{}" {
		t.Fatalf("stream=%+v err=%v", msg, err)
	}
	if tokens != 11 {
		t.Fatalf("tokens=%d", tokens)
	}
}

func TestAgentAutoFallbackOnToolsUnsupported(t *testing.T) {
	client := &nativeChat{failOnce: true}
	m := newGatewayChatModel(client, "p", "fallback-test")
	m.protocol = "auto"
	_ = m.BindTools([]*schema.ToolInfo{{Name: "list_databases"}})
	if _, err := m.Generate(context.Background(), []*schema.Message{schema.UserMessage("list")}); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 || len(client.requests[0].Tools) == 0 || len(client.requests[1].Tools) != 0 {
		t.Fatalf("requests=%+v", client.requests)
	}
	if _, err := m.Generate(context.Background(), []*schema.Message{schema.UserMessage("list")}); err != nil {
		t.Fatal(err)
	}
	if len(client.requests[2].Tools) != 0 {
		t.Fatal("fallback cache not used")
	}
}

// 上游鉴权或配额错误不能为了“降级流式”再次发非流式请求。
type rejectedStreamLLM struct {
	chatCalls   int
	streamCalls int
}

func (f *rejectedStreamLLM) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	f.streamCalls++
	return nil, &providers.LiteLLMError{StatusCode: 401, Message: "unauthorized"}
}
func (f *rejectedStreamLLM) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	f.chatCalls++
	return ChatResponse{Content: "unexpected fallback"}, nil
}
func TestAgentStreamAuthErrorDoesNotRetryChat(t *testing.T) {
	client := &rejectedStreamLLM{}
	m := newGatewayChatModel(client, "p", "m")
	_, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	var upstream *providers.LiteLLMError
	if !errors.As(err, &upstream) || client.streamCalls != 1 || client.chatCalls != 0 {
		t.Fatalf("stream=%d chat=%d err=%v", client.streamCalls, client.chatCalls, err)
	}
}

type incompleteToolStream struct{ emitted bool }

func (s *incompleteToolStream) Next() (string, bool, error) {
	return "", false, errors.New("broken upstream")
}
func (s *incompleteToolStream) NextDelta() (StreamDelta, error) {
	if !s.emitted {
		s.emitted = true
		return StreamDelta{ToolCall: &ToolCallDelta{Index: 0, ID: "c1", Name: "list_databases", ArgsDelta: "{"}}, nil
	}
	return StreamDelta{}, errors.New("broken upstream")
}
func (s *incompleteToolStream) Close() error { return nil }

type incompleteToolLLM struct{}

func (*incompleteToolLLM) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, errors.New("unexpected")
}
func (*incompleteToolLLM) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	return &incompleteToolStream{}, nil
}

func TestAgentStreamRejectsIncompleteToolAfterReadError(t *testing.T) {
	m := newGatewayChatModel(&incompleteToolLLM{}, "p", "m")
	sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("list")})
	if err != nil {
		t.Fatal(err)
	}
	defer sr.Close()
	for {
		msg, err := sr.Recv()
		if err != nil {
			if msg != nil {
				t.Fatalf("tool must not execute on broken stream, message=%+v err=%v", msg, err)
			}
			return
		}
		if msg != nil && len(msg.ToolCalls) > 0 && !isToolDelta(msg.ToolCalls[0]) {
			t.Fatalf("complete tool call on broken stream: %+v", msg)
		}
	}
}
