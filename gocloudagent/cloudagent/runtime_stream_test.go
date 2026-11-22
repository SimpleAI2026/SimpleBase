package cloudagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
)

type c3NoUsageLLM struct{}

func (*c3NoUsageLLM) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, errors.New("unexpected chat")
}
func (*c3NoUsageLLM) Stream(context.Context, string, ChatRequest) (TokenStream, error) {
	return &c3MultiStream{chunks: []string{"你好", "，世界"}}, nil
}

type c3MultiStream struct {
	chunks []string
	idx    int
}

func (s *c3MultiStream) Next() (string, bool, error) {
	if s.idx >= len(s.chunks) {
		return "", true, io.EOF
	}
	s.idx++
	return s.chunks[s.idx-1], false, nil
}
func (*c3MultiStream) Close() error { return nil }

func TestStreamMultiDeltaWithoutUsage(t *testing.T) {
	rt := &Runtime{LLM: &c3NoUsageLLM{}}
	var events []Event
	result, err := rt.StartRun(context.Background(), RunRequest{
		ThreadID: "t", RunID: "r-m", UserText: "hi", Stream: true,
		Agent: Agent{ID: "a", Name: "General"},
	}, func(ev Event) { events = append(events, ev) })
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "你好，世界" || result.Reason != "stop" {
		t.Fatalf("result=%+v", result)
	}
	if result.PromptTokens != 0 || result.CompletionTokens != 0 || result.ReasoningTokens != 0 {
		t.Fatalf("usage must stay zero: %+v", result)
	}
	var joined strings.Builder
	for _, event := range events {
		if event.Type != "token" {
			t.Fatalf("unexpected event type %q", event.Type)
		}
		joined.WriteString(event.Content)
	}
	if len(events) < 2 || joined.String() != result.Content {
		t.Fatalf("events=%v content=%q", events, result.Content)
	}
}

func TestToolFailureResultCard(t *testing.T) {
	rt := &Runtime{LLM: &scriptedLLM{}, Tools: []einotool.BaseTool{testTool(t, "echo", errors.New("tool down"))}}
	var events []Event
	result, err := rt.StartRun(context.Background(), RunRequest{
		ThreadID: "t", RunID: "r-f", UserText: "run", Stream: true,
		Agent: Agent{ID: "a", Name: "Agent"},
	}, func(ev Event) { events = append(events, ev) })
	if err != nil {
		t.Fatalf("tool failure must not fail the run: %v", err)
	}
	var callEvent, resultEvent *Event
	for i := range events {
		switch events[i].Type {
		case "tool_call":
			if callEvent == nil {
				callEvent = &events[i]
			}
		case "tool_result":
			if resultEvent == nil {
				resultEvent = &events[i]
			}
		}
	}
	if callEvent == nil || resultEvent == nil || callEvent.Name != "echo" || resultEvent.Name != "echo" || !resultEvent.IsError {
		t.Fatalf("events=%+v", events)
	}
	var cards []map[string]any
	if err := json.Unmarshal([]byte(result.ToolCallsJSON), &cards); err != nil || len(cards) == 0 {
		t.Fatalf("cards=%s err=%v", result.ToolCallsJSON, err)
	}
	if cards[0]["is_error"] != true || result.ToolCalls != 1 {
		t.Fatalf("cards=%v result=%+v", cards, result)
	}
}
