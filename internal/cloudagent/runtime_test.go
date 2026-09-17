package cloudagent

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type scriptedLLM struct {
	calls int
}

func (s *scriptedLLM) Chat(_ context.Context, _ string, req ChatRequest) (ChatResponse, error) {
	s.calls++
	last := ""
	if len(req.Messages) > 0 {
		last = req.Messages[len(req.Messages)-1].Content
	}
	if strings.Contains(last, "TOOL_RESULT") || s.calls > 1 {
		return ChatResponse{Content: "Project has 1 database named default."}, nil
	}
	return ChatResponse{Content: `TOOL_CALL {"name":"list_databases","arguments":{}}`}, nil
}

func (s *scriptedLLM) Stream(ctx context.Context, projectID string, req ChatRequest) (TokenStream, error) {
	resp, err := s.Chat(ctx, projectID, req)
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

func TestRuntimeSingleAgentToolRoundtrip(t *testing.T) {
	rt := &Runtime{
		LLM: &scriptedLLM{},
		DB:  &fakeDB{dbs: []DatabaseInfo{{ID: "d1", Name: "default", Status: "ready"}}},
	}
	res, err := rt.StartRun(context.Background(), RunRequest{
		ProjectID: "p1",
		Principal: auth.Principal{},
		Agent: systemdb.CloudAgent{
			ID:      "a1",
			Name:    "Database",
			Module:  ModuleDatabase,
			ToolIDs: []string{ToolListDatabases},
		},
		RunID:    "run-1",
		UserText: "What databases exist?",
		Stream:   false,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "default") {
		t.Fatalf("content=%q", res.Content)
	}
}

func TestAssembleNeverPutsS3Keys(t *testing.T) {
	got := AssembleInstruction(PromptParts{
		ProjectEnv: "bucket keys: AccessKey=AKIAIOSFODNN7EXAMPLE SecretKey=wJalrXUtnFEMI/K7MDENG",
	})
	if strings.Contains(got, "wJalrXUtnFEMI") || strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("secret in prompt: %s", got)
	}
}
