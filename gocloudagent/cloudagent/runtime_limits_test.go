package cloudagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
)

type repeatingToolLLM struct{}

func (*repeatingToolLLM) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{Content: `TOOL_CALL {"name":"echo","arguments":{}}`}, nil
}
func (l *repeatingToolLLM) Stream(ctx context.Context, project string, req ChatRequest) (TokenStream, error) {
	resp, err := l.Chat(ctx, project, req)
	return &onceStream{content: resp.Content}, err
}

func TestRunMaxIterationsReturnsPartialConclusion(t *testing.T) {
	rt := &Runtime{LLM: &repeatingToolLLM{}, Tools: []einotool.BaseTool{testTool(t, "echo", nil)}, MaxIterations: 1}
	result, err := rt.StartRun(context.Background(), RunRequest{
		ThreadID: "t", RunID: "r", UserText: "run",
		Agent: Agent{ID: "a", Name: "Agent"},
	}, nil)
	if err != nil || result.Reason != "max_iterations" || !strings.Contains(result.Content, "已达到工具调用上限（1 次）") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

type cancelStream struct {
	ctx   context.Context
	first bool
}

func (s *cancelStream) Next() (string, bool, error) {
	if !s.first {
		s.first = true
		return "partial answer", false, nil
	}
	<-s.ctx.Done()
	return "", false, s.ctx.Err()
}
func (*cancelStream) Close() error { return nil }

type cancelLLM struct{}

func (*cancelLLM) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{}, errors.New("unexpected chat")
}
func (*cancelLLM) Stream(ctx context.Context, _ string, _ ChatRequest) (TokenStream, error) {
	return &cancelStream{ctx: ctx}, nil
}

func TestRunCancelKeepsPartialResponse(t *testing.T) {
	rt := &Runtime{LLM: &cancelLLM{}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := rt.StartRun(ctx, RunRequest{ThreadID: "t", RunID: "r", UserText: "hello", Stream: true,
		Agent: Agent{ID: "a", Name: "General"}}, func(ev Event) {
		if ev.Type == "token" {
			rt.CancelRun("r")
		}
	})
	if !errors.Is(err, context.Canceled) || result.Content != "partial answer" || result.Reason != "canceled" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
