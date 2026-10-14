package llmgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/voocel/litellm/providers"
)

// A2 表征（planv6.0 §7 A2）：Chat 与 Stream 的请求准备语义。
// 固定上游收到的请求字段（model 回填、messages、max_tokens、temperature、tools），
// 作为后续抽请求准备 helper 的等价证据。上游为本地 httptest server，不访问外网。

// upstreamRequestCapture 记录上游收到的完整请求体。
type upstreamRequestCapture struct {
	Model       string          `json:"model"`
	Messages    json.RawMessage `json:"messages"`
	MaxTokens   *int            `json:"max_tokens"`
	Temperature *float64        `json:"temperature"`
	Tools       json.RawMessage `json:"tools"`
	Stream      bool            `json:"stream"`
	Calls       int
}

func captureRequestServer(t *testing.T, cap *upstreamRequestCapture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.Calls++
		_ = json.NewDecoder(r.Body).Decode(cap)
		if cap.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"id\":\"1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
}

// Chat 与 Stream 上游请求字段必须逐字段一致地传递（同一准备逻辑的两份拷贝）。
func TestChatStreamUpstreamRequestFields(t *testing.T) {
	var cap upstreamRequestCapture
	srv := captureRequestServer(t, &cap)
	defer srv.Close()

	svc := NewService(&fakeResolver{providers: ProjectProviders{
		Default: "openai",
		Providers: []ProviderConfig{
			{Name: "openai", APIKey: "sk-test", BaseURL: srv.URL, Model: "default-model"},
		},
	}}, nil, nil)
	ctx := context.Background()
	maxTok := 16
	temp := 0.5

	if _, err := svc.Chat(ctx, "p1", Request{
		Messages:    []providers.Message{{Role: "user", Content: "hi"}},
		MaxTokens:   &maxTok,
		Temperature: &temp,
	}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if cap.Calls != 1 || cap.Model != "default-model" || cap.Stream {
		t.Fatalf("chat capture: calls=%d model=%q stream=%v", cap.Calls, cap.Model, cap.Stream)
	}
	if cap.MaxTokens == nil || *cap.MaxTokens != maxTok {
		t.Fatalf("chat max_tokens=%v", cap.MaxTokens)
	}
	if cap.Temperature == nil || *cap.Temperature != temp {
		t.Fatalf("chat temperature=%v", cap.Temperature)
	}

	cap = upstreamRequestCapture{}
	reader, err := svc.Stream(ctx, "p1", Request{
		Messages:    []providers.Message{{Role: "user", Content: "hi"}},
		MaxTokens:   &maxTok,
		Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	_ = reader.Close()
	if cap.Calls != 1 || cap.Model != "default-model" || !cap.Stream {
		t.Fatalf("stream capture: calls=%d model=%q stream=%v", cap.Calls, cap.Model, cap.Stream)
	}
	if cap.MaxTokens == nil || *cap.MaxTokens != maxTok {
		t.Fatalf("stream max_tokens=%v", cap.MaxTokens)
	}
	if cap.Temperature == nil || *cap.Temperature != temp {
		t.Fatalf("stream temperature=%v", cap.Temperature)
	}
}

// 请求带工具时必须原样传递（不复制/排序/过滤）。
func TestChatStreamToolsPassedThrough(t *testing.T) {
	var cap upstreamRequestCapture
	srv := captureRequestServer(t, &cap)
	defer srv.Close()

	svc := NewService(&fakeResolver{providers: ProjectProviders{
		Default: "openai",
		Providers: []ProviderConfig{
			{Name: "openai", APIKey: "sk-test", BaseURL: srv.URL, Model: "default-model"},
		},
	}}, nil, nil)

	tools := []providers.Tool{{
		Type: "function",
		Function: providers.FunctionDef{
			Name:        "echo",
			Description: "echo input",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}}
	if _, err := svc.Chat(context.Background(), "p1", Request{
		Messages: []providers.Message{{Role: "user", Content: "hi"}},
		Tools:    tools,
	}); err != nil {
		t.Fatalf("chat tools: %v", err)
	}
	if string(cap.Tools) == "" {
		t.Fatal("chat: tools not forwarded upstream")
	}
	var gotTools []map[string]any
	if err := json.Unmarshal(cap.Tools, &gotTools); err != nil {
		t.Fatalf("tools json: %v", err)
	}
	if len(gotTools) != 1 || gotTools[0]["type"] != "function" {
		t.Fatalf("tools payload=%s", cap.Tools)
	}

	// 显式模型经 Stream 同样透传。
	cap = upstreamRequestCapture{}
	reader, err := svc.Stream(context.Background(), "p1", Request{
		Model:    "explicit-model",
		Messages: []providers.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	_ = reader.Close()
	if cap.Model != "explicit-model" {
		t.Fatalf("stream model=%q", cap.Model)
	}
}
