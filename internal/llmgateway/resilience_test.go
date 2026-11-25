// 文件说明：resilience_test.go — internal/llmgateway 的单元测试（TestServiceAppliesResilienceConfig 等）
//
// 用法
//
//	go test ./internal/llmgateway -run 'TestServiceAppliesResilienceConfig'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/llmgateway/resilience_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_tools_test.go, catalog_resolver.go, client_chat_usage_test.go, fallback_resolver.go, fallback_resolver_test.go 等 14 个
//
// 主要符号
//
//	TestServiceAppliesResilienceConfig(t *testing.T)
//	type fakeCatalog struct{…}
//	(f *fakeCatalog) Resolve(_ context.Context, projectID string) (ProjectProviders, error)
//	(f *fakeCatalog) GetLLMProviders(_ context.Context, projectID string) (ProjectProviders, error)
//	(f *fakeCatalog) RecordLLM(_ context.Context, _, _, _ string, _ Usage) error

package llmgateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/voocel/litellm/providers"
)

// 回归：计划 §2 定了 MaxRetries=3，ConnectTimeout=10s。
// 这个测试验证配置确实传入了 litellm，并且网关瞬时故障时能重试成功。
func TestServiceAppliesResilienceConfig(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"r1","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	cat := &fakeCatalog{providers: ProjectProviders{
		ProjectID: "p1",
		Default:   "openai",
		Providers: []ProviderConfig{{Name: "openai", APIKey: "k", BaseURL: srv.URL, Model: "m"}},
	}}
	s := NewService(cat, cat, nil, nil)

	resp, err := s.Chat(context.Background(), "p1", Request{Model: "m", Messages: []providers.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("retry should succeed after 2 failures: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 calls (2 fail + 1 succeed), got %d", got)
	}
}

type fakeCatalog struct {
	providers ProjectProviders
	err       error
}

func (f *fakeCatalog) Resolve(_ context.Context, projectID string) (ProjectProviders, error) {
	if f.err != nil {
		return ProjectProviders{}, f.err
	}
	if f.providers.ProjectID != projectID {
		return ProjectProviders{}, errors.New("project not found")
	}
	return f.providers, nil
}

func (f *fakeCatalog) GetLLMProviders(_ context.Context, projectID string) (ProjectProviders, error) {
	if f.err != nil {
		return ProjectProviders{}, f.err
	}
	if f.providers.ProjectID != projectID {
		return ProjectProviders{}, errors.New("project not found")
	}
	return f.providers, nil
}

func (f *fakeCatalog) RecordLLM(_ context.Context, _, _, _ string, _ Usage) error {
	return nil
}
