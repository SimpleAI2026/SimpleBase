// 文件说明：service_test.go — internal/llmgateway 的单元测试（TestListProviders、TestProviderModelsIncludesDefaultWithoutAllowedModels 等）
//
// 用法
//
//	go test ./internal/llmgateway -run 'TestListProviders|TestProviderModelsIncludesDefaultWithoutAllowedModels|TestListProvidersEmpty|TestListProvidersResolveError'
//	同包调用：fakeResolver → agent_tools_test.go, client_chat_usage_test.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/llmgateway/service_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_tools_test.go, catalog_resolver.go, client_chat_usage_test.go, fallback_resolver.go, fallback_resolver_test.go 等 14 个
//	[关键引用] fakeResolver ← agent_tools_test.go, client_chat_usage_test.go
//
// 主要符号
//
//	type fakeResolver struct{…}
//	(f *fakeResolver) Resolve(ctx context.Context, projectID string) (ProjectProviders, error)
//	TestListProviders(t *testing.T)
//	TestProviderModelsIncludesDefaultWithoutAllowedModels(t *testing.T)
//	TestListProvidersEmpty(t *testing.T)
//	TestListProvidersResolveError(t *testing.T)
//	TestRecordLLMUsageRecorder(t *testing.T)
//	type fakeRecorder struct{…}
//	... 另有 1 个顶层符号

package llmgateway

import (
	"context"
	"errors"
	"testing"
)

// fakeResolver 返回固定配置。
type fakeResolver struct {
	providers ProjectProviders
	err       error
}

func (f *fakeResolver) Resolve(ctx context.Context, projectID string) (ProjectProviders, error) {
	if f.err != nil {
		return ProjectProviders{}, f.err
	}
	f.providers.ProjectID = projectID
	return f.providers, nil
}

func TestListProviders(t *testing.T) {
	r := &fakeResolver{providers: ProjectProviders{
		Default: "openai",
		Providers: []ProviderConfig{
			{Name: "openai", APIKey: "k1"},
			{Name: "anthropic", APIKey: "k2"},
		},
	}}
	svc := NewService(r, nil, nil, nil)
	names, err := svc.ListProviders(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(names))
	}
	if names[0] != "openai" {
		t.Fatalf("expected openai, got %s", names[0])
	}
}

func TestProviderModelsIncludesDefaultWithoutAllowedModels(t *testing.T) {
	svc := NewService(&fakeResolver{providers: ProjectProviders{Providers: []ProviderConfig{
		{Name: "openai", Model: "zai-org/GLM-5.3-Flash"},
		{Name: "anthropic", Model: "claude", AllowedModels: []string{"claude", "other"}},
		{Name: "restricted", Model: "unlisted", AllowedModels: []string{"allowed"}},
	}}}, nil, nil, nil)
	models, err := svc.ProviderModels(context.Background(), "proj-1")
	if err != nil || len(models["openai"]) != 1 || models["openai"][0] != "zai-org/GLM-5.3-Flash" {
		t.Fatalf("fallback model: %+v, err=%v", models, err)
	}
	if len(models["anthropic"]) != 2 {
		t.Fatalf("default model duplicated: %+v", models["anthropic"])
	}
	if len(models["restricted"]) != 1 || models["restricted"][0] != "allowed" {
		t.Fatalf("disallowed default model exposed: %+v", models["restricted"])
	}
}

func TestListProvidersEmpty(t *testing.T) {
	r := &fakeResolver{providers: ProjectProviders{}}
	svc := NewService(r, nil, nil, nil)
	names, err := svc.ListProviders(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected 0 providers, got %d", len(names))
	}
}

func TestListProvidersResolveError(t *testing.T) {
	r := &fakeResolver{err: errors.New("catalog down")}
	svc := NewService(r, nil, nil, nil)
	_, err := svc.ListProviders(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error from resolver")
	}
}

func TestRecordLLMUsageRecorder(t *testing.T) {
	r := &fakeResolver{providers: ProjectProviders{
		Default:   "openai",
		Providers: []ProviderConfig{{Name: "openai"}},
	}}
	rec := &fakeRecorder{}
	svc := NewService(r, rec, nil, nil)
	// 仅验证 recorder 可被注入；Chat 实际调用需 litellm client。
	_ = svc
	if rec.called {
		t.Fatal("recorder should not be called yet")
	}
}

// fakeRecorder 实现 UsageRecorder。
type fakeRecorder struct {
	called bool
}

func (f *fakeRecorder) RecordLLM(ctx context.Context, projectID, provider, model string, u Usage) error {
	f.called = true
	return nil
}
