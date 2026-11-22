package cloudagent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type scriptedLLM struct {
	calls int
	scope string
	last  ChatRequest
}

func (s *scriptedLLM) Chat(_ context.Context, scope string, req ChatRequest) (ChatResponse, error) {
	s.calls++
	s.scope = scope
	s.last = req
	last := ""
	if len(req.Messages) > 0 {
		last = req.Messages[len(req.Messages)-1].Content
	}
	if strings.Contains(last, "TOOL_RESULT") || s.calls > 1 {
		return ChatResponse{Content: "Tool returned default."}, nil
	}
	return ChatResponse{Content: `TOOL_CALL {"name":"echo","arguments":{}}`}, nil
}

func (s *scriptedLLM) Stream(ctx context.Context, scope string, req ChatRequest) (TokenStream, error) {
	resp, err := s.Chat(ctx, scope, req)
	if err != nil {
		return nil, err
	}
	return &onceStream{content: resp.Content}, nil
}

type onceStream struct {
	content string
	done    bool
}

func (o *onceStream) Next() (string, bool, error) {
	if o.done {
		return "", true, io.EOF
	}
	o.done = true
	return o.content, true, nil
}
func (o *onceStream) Close() error { return nil }

type emptyToolInput struct{}

func testTool(t *testing.T, name string, callErr error) einotool.BaseTool {
	t.Helper()
	tool, err := utils.InferTool(name, "test tool", func(context.Context, emptyToolInput) (string, error) {
		if callErr != nil {
			return "", callErr
		}
		return "default", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func TestRuntimeSingleAgentToolRoundtrip(t *testing.T) {
	llm := &scriptedLLM{}
	rt := &Runtime{LLM: llm, Tools: []einotool.BaseTool{testTool(t, "echo", nil)}}
	res, err := rt.StartRun(context.Background(), RunRequest{
		Agent: Agent{ID: "a1", Name: "Agent"}, RunID: "run-1", UserText: "use the tool",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "default") || res.ToolCalls != 1 {
		t.Fatalf("result=%+v", res)
	}
}

func TestRuntimeProviderControlsPreparationAndCleanup(t *testing.T) {
	llm := &scriptedLLM{}
	cleaned := false
	called := false
	rt := &Runtime{LLM: llm, Provider: RunProviderFunc(func(_ context.Context, req RunRequest, _ func(Event)) (RunPreparation, error) {
		return RunPreparation{
			Model: "provider-model", ModelScope: "provider-scope", Instruction: "provider instruction",
			Tools:   []einotool.BaseTool{testTool(t, "echo", nil)},
			Cleanup: func() { cleaned = true }, OnToolCall: func(string) { called = true },
		}, nil
	})}
	_, err := rt.StartRun(context.Background(), RunRequest{
		Agent: Agent{ID: "a", Name: "Agent"}, RunID: "run-provider", UserText: "run",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if llm.scope != "provider-scope" || llm.last.Model != "provider-model" {
		t.Fatalf("scope=%q model=%q", llm.scope, llm.last.Model)
	}
	if !cleaned || !called {
		t.Fatalf("cleanup=%v tool callback=%v", cleaned, called)
	}
	foundInstruction := false
	for _, message := range llm.last.Messages {
		if strings.Contains(message.Content, "provider instruction") {
			foundInstruction = true
		}
	}
	if !foundInstruction {
		t.Fatalf("provider instruction missing: %+v", llm.last.Messages)
	}
}

func TestAssembleInstructionRedactsSecrets(t *testing.T) {
	got := AssembleInstruction("bucket keys: AccessKey=AKIAIOSFODNN7EXAMPLE SecretKey=wJalrXUtnFEMI/K7MDENG")
	if strings.Contains(got, "wJalrXUtnFEMI") || strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("secret in prompt: %s", got)
	}
}

func TestToolErrorWrapping(t *testing.T) {
	tool := testTool(t, "fail", errors.New("boom password=hunter2"))
	wrapped := wrapToolErrors([]einotool.BaseTool{tool})[0].(einotool.InvokableTool)
	out, err := wrapped.InvokableRun(context.Background(), `{}`)
	if err != nil || !strings.Contains(out, "is_error") || !strings.Contains(out, "boom") || strings.Contains(out, "hunter2") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
