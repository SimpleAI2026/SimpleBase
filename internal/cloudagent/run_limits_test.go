package cloudagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type repeatingToolLLM struct{}

func (*repeatingToolLLM) Chat(context.Context, string, ChatRequest) (ChatResponse, error) {
	return ChatResponse{Content: `TOOL_CALL {"name":"list_databases","arguments":{}}`}, nil
}
func (l *repeatingToolLLM) Stream(ctx context.Context, project string, req ChatRequest) (TokenStream, error) {
	resp, err := l.Chat(ctx, project, req)
	return &onceStream{content: resp.Content}, err
}

func TestRunMaxIterationsReturnsPartialConclusion(t *testing.T) {
	rt := &Runtime{LLM: &repeatingToolLLM{}, DB: &fakeDB{}, MaxIterations: 1}
	result, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p", ThreadID: "t", RunID: "r", UserText: "list",
		Agent: systemdb.CloudAgent{ID: "a", Name: "Database", Module: ModuleDatabase, ToolIDs: []string{ToolListDatabases}},
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
	result, err := rt.StartRun(ctx, RunRequest{ProjectID: "p", ThreadID: "t", RunID: "r", UserText: "hello", Stream: true,
		Agent: systemdb.CloudAgent{ID: "a", Name: "General", Module: ModuleGeneral}}, func(ev Event) {
		if ev.Type == "token" {
			rt.CancelRun("r")
		}
	})
	if !errors.Is(err, context.Canceled) || result.Content != "partial answer" || result.Reason != "canceled" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
