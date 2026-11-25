// 文件说明：fallback_resolver_test.go — internal/llmgateway 的单元测试（TestFallbackResolverPrefersCatalogKeys、TestFallbackResolverUsesInstanceWhenCatalogHasNoKey 等）
//
// 用法
//
//	go test ./internal/llmgateway -run 'TestFallbackResolverPrefersCatalogKeys|TestFallbackResolverUsesInstanceWhenCatalogHasNoKey|TestFallbackResolverPrimaryErrorWithoutInstance'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/llmgateway/fallback_resolver_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_tools_test.go, catalog_resolver.go, client_chat_usage_test.go, fallback_resolver.go, model_resolution_test.go 等 14 个
//
// 主要符号
//
//	TestFallbackResolverPrefersCatalogKeys(t *testing.T)
//	TestFallbackResolverUsesInstanceWhenCatalogHasNoKey(t *testing.T)
//	TestFallbackResolverPrimaryErrorWithoutInstance(t *testing.T)

package llmgateway

import (
	"context"
	"errors"
	"testing"
)

func TestFallbackResolverPrefersCatalogKeys(t *testing.T) {
	t.Parallel()
	primary := &fakeResolver{providers: ProjectProviders{
		Default:   "openai",
		Providers: []ProviderConfig{{Name: "openai", APIKey: "catalog-key"}},
	}}
	r := NewFallbackResolver(primary, map[string]InstanceProvider{
		"openai": {APIKey: "instance-key"},
	})
	pp, err := r.Resolve(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pp.Providers) != 1 || pp.Providers[0].APIKey != "catalog-key" {
		t.Fatalf("%+v", pp)
	}
}

func TestFallbackResolverUsesInstanceWhenCatalogHasNoKey(t *testing.T) {
	t.Parallel()
	primary := &fakeResolver{providers: ProjectProviders{
		Providers: []ProviderConfig{{Name: "openai", APIKey: ""}},
	}}
	r := NewFallbackResolver(primary, map[string]InstanceProvider{
		"openai": {APIKey: "instance-key", DefaultModel: "gpt-4o-mini"},
	})
	pp, err := r.Resolve(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pp.Providers) != 1 || pp.Providers[0].APIKey != "instance-key" || pp.Providers[0].Model != "gpt-4o-mini" {
		t.Fatalf("%+v", pp)
	}
}

func TestFallbackResolverPrimaryErrorWithoutInstance(t *testing.T) {
	t.Parallel()
	primary := &fakeResolver{err: errors.New("catalog down")}
	r := NewFallbackResolver(primary, nil)
	if _, err := r.Resolve(context.Background(), "p1"); err == nil {
		t.Fatal("expected error")
	}
}
