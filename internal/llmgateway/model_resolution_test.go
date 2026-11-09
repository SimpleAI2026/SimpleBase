package llmgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/voocel/litellm/providers"
)

// 空模型名的语义核查（用于定位「gocloudagent: run start 日志里 model=""」）。
//
// 结论：不是缺陷。云助手 runtime 的 model 字段只表示「助手级/项目级显式指定的模型」，
// 为空时由网关回填供应商的 default_model，最终上游请求带的是真实模型名。
// 本组测试把这条回填链固定下来，并让「实际用了哪个模型」在日志里可见。

// upstreamModelCapture 记录上游收到的 model 字段。
type upstreamModelCapture struct {
	model  string
	stream bool
}

func captureModelServer(t *testing.T, cap *upstreamModelCapture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		cap.model, _ = body["model"].(string)
		cap.stream, _ = body["stream"].(bool)
		if cap.stream {
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

// 空模型名时，上游必须收到供应商的 default_model（而不是空串）。
func TestEmptyModelFallsBackToProviderDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	var cap upstreamModelCapture
	srv := captureModelServer(t, &cap)
	defer srv.Close()

	svc := NewService(&fakeResolver{providers: ProjectProviders{
		Default: "openai",
		Providers: []ProviderConfig{
			{Name: "openai", APIKey: "sk-test", BaseURL: srv.URL, Model: "zai-org/GLM-5.3-Flash"},
		},
	}}, nil, nil)
	ctx := context.Background()

	// 非流式：req.Model 为空 → 回填 default_model。
	if _, err := svc.Chat(ctx, "p1", Request{Messages: []providers.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if cap.model != "zai-org/GLM-5.3-Flash" {
		t.Fatalf("chat upstream model=%q want provider default", cap.model)
	}

	// 流式：同样回填。
	cap.model = ""
	reader, err := svc.Stream(ctx, "p1", Request{Messages: []providers.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	_ = reader.Close()
	if cap.model != "zai-org/GLM-5.3-Flash" {
		t.Fatalf("stream upstream model=%q want provider default", cap.model)
	}

	// 显式模型优先于供应商默认值。
	cap.model = ""
	if _, err := svc.Chat(ctx, "p1", Request{Model: "explicit-model", Messages: []providers.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("chat explicit: %v", err)
	}
	if cap.model != "explicit-model" {
		t.Fatalf("explicit model=%q", cap.model)
	}
}

// 请求与供应商都没有模型时必须显式报错，不能把空模型名发给上游。
func TestMissingModelIsExplicitError(t *testing.T) {
	svc := NewService(&fakeResolver{providers: ProjectProviders{
		Default:   "openai",
		Providers: []ProviderConfig{{Name: "openai", APIKey: "sk-test", BaseURL: "http://127.0.0.1:9/v1"}},
	}}, nil, nil)
	ctx := context.Background()
	if _, err := svc.Chat(ctx, "p1", Request{}); !errors.Is(err, ErrModelNotConfigured) {
		t.Fatalf("chat err=%v want ErrModelNotConfigured", err)
	}
	if _, err := svc.Stream(ctx, "p1", Request{}); !errors.Is(err, ErrModelNotConfigured) {
		t.Fatalf("stream err=%v want ErrModelNotConfigured", err)
	}
	// 错误码到前端文案的映射由 gocloudagent.ClassifyError 负责，
	// 断言放在 gocloudagent 包（本包是 llmgateway，不能反向 import）。
}

// 解析后的模型必须写进日志，便于排查「日志显示 model="" 时到底用了什么模型」。
func TestResolvedModelIsLogged(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	var cap upstreamModelCapture
	srv := captureModelServer(t, &cap)
	defer srv.Close()

	var buf bytes.Buffer
	logger := observability.NewLogger("debug", "json", &buf)
	svc := NewService(&fakeResolver{providers: ProjectProviders{
		Default: "openai",
		Providers: []ProviderConfig{
			{Name: "openai", APIKey: "sk-test", BaseURL: srv.URL, Model: "resolved-model"},
		},
	}}, nil, logger)

	if _, err := svc.Chat(context.Background(), "p1", Request{Messages: []providers.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	logs := buf.String()
	if !strings.Contains(logs, "llmgateway: model resolved") || !strings.Contains(logs, "resolved-model") {
		t.Fatalf("resolved model not logged: %s", logs)
	}
	// 日志不得出现密钥。
	if strings.Contains(logs, "sk-test") {
		t.Fatalf("api key leaked into log: %s", logs)
	}
}
