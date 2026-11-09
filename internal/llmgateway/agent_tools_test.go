package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/voocel/litellm/providers"
)

func TestAgentNativeToolAndUsage(t *testing.T) {
	var captured struct {
		Tools  []json.RawMessage `json:"tools"`
		Stream bool              `json:"stream"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Error(err)
			return
		}
		if captured.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"id\":\"1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"id\":\"1\",\"model\":\"m\",\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4,\"total_tokens\":7,\"completion_tokens_details\":{\"reasoning_tokens\":2}}}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"list_databases","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`)
	}))
	defer srv.Close()
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, nil)
	req := Request{Model: "m", Messages: []providers.Message{{Role: "user", Content: "list"}}, Tools: []providers.Tool{{Type: "function", Function: providers.FunctionDef{Name: "list_databases", Parameters: map[string]any{"type": "object"}}}}}
	resp, err := svc.Chat(context.Background(), "p", req)
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Tools) != 1 || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Function.Name != "list_databases" {
		t.Fatalf("tools=%d resp=%+v", len(captured.Tools), resp)
	}
	stream, err := svc.Stream(context.Background(), "p", req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var usage *providers.Usage
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.Done {
			break
		}
	}
	if usage == nil || usage.PromptTokens != 3 || usage.ReasoningTokens != 2 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestAgentModelAllowlist(t *testing.T) {
	cfg := ProviderConfig{AllowedModels: []string{"allowed"}}
	if !errors.Is(checkModelAllowed(cfg, "other"), ErrModelNotAllowed) {
		t.Fatal("model not rejected")
	}
	if err := checkModelAllowed(cfg, "allowed"); err != nil {
		t.Fatal(err)
	}
}
