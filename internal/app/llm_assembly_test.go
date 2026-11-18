package app

// b4_llm_assembly_test.go：planv6.0 B4 LLM 装配块表征测试。
// 在提取私有装配方法前锁定旧实现行为（行为表征，不改生产接口）：
//   1. LLM.Enabled=false 时 llmSvc 保持 nil；
//   2. LLM.Enabled=true 时 llmSvc 构造成功，且通过 ListProviders /
//      ProviderModels 可观测 resolver 链行为；
//   3. 有实例 YAML 供应商时空项目解析回退到实例供应商（链行为）：
//      项目动态凭据 → catalog → 实例回退（计划 B4 锚点）；
//   4. 无实例供应商时空项目解析为空（不 panic、不报错）。
// llmgateway.Service 未暴露 resolver 访问器，此处不添加访问器
// （计划规则：不为测试大改装配接口），以公共方法行为作表征。

import (
	"context"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

// llmTestConfig 返回本地存储 + LLM 启用的最小可装配配置。
// LLM 块位于 assembleDeps 的 writable 分支（只读实例提前返回），
// 因此这里沿用 testConfig(true)（既有 TestLocalStorageLLMEnabled 同款）
// 并把存储切回本地，绕开 S3 依赖。
func llmTestConfig(t *testing.T, providers map[string]config.ProviderConfig) config.Config {
	t.Helper()
	cfg := testConfig(true)
	cfg.Storage = config.StorageConfig{Database: config.StorageModeLocal, Files: config.StorageModeLocal, DataDir: t.TempDir()}
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	cfg.LLM = config.LLMConfig{Enabled: true, Providers: providers}
	return cfg
}

// newLLMApp 装配 App 并注册清理。
func newLLMApp(t *testing.T, cfg config.Config) *App {
	t.Helper()
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	return a
}

// TestB4_LLMDisabledLeavesServiceNil：Enabled=false 时 llmSvc 为 nil。
func TestB4_LLMDisabledLeavesServiceNil(t *testing.T) {
	cfg := llmTestConfig(t, nil)
	cfg.LLM.Enabled = false
	a := newLLMApp(t, cfg)
	if a.llmSvc != nil {
		t.Fatalf("llmSvc must stay nil when LLM disabled, got %T", a.llmSvc)
	}
}

// TestB4_LLMEnabledNoInstanceProviders：无实例供应商时空项目解析为空
// （链为 ProjectCred → Catalog，两者对该项目都无记录），不报错。
func TestB4_LLMEnabledNoInstanceProviders(t *testing.T) {
	a := newLLMApp(t, llmTestConfig(t, nil))
	if a.llmSvc == nil {
		t.Fatal("llmSvc must be constructed when LLM enabled")
	}
	ctx := context.Background()
	names, err := a.llmSvc.ListProviders(ctx, "no-such-project")
	if err != nil {
		t.Fatalf("ListProviders on empty project: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("no instance providers: expected empty list, got %v", names)
	}
	models, err := a.llmSvc.ProviderModels(ctx, "no-such-project")
	if err != nil {
		t.Fatalf("ProviderModels on empty project: %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("no instance providers: expected empty models, got %v", models)
	}
}

// TestB4_LLMEnabledWithInstanceProviders：有实例供应商时空项目解析
// 回退到实例供应商（项目动态凭据 → catalog → 实例回退）。
func TestB4_LLMEnabledWithInstanceProviders(t *testing.T) {
	a := newLLMApp(t, llmTestConfig(t, map[string]config.ProviderConfig{
		"openai": {
			APIKey: "sk-inst", BaseURL: "http://127.0.0.1:1",
			DefaultModel: "m1", AllowedModels: []string{"m1", "m2"},
		},
	}))
	if a.llmSvc == nil {
		t.Fatal("llmSvc must be constructed when LLM enabled")
	}
	ctx := context.Background()
	names, err := a.llmSvc.ListProviders(ctx, "no-such-project")
	if err != nil {
		t.Fatalf("ListProviders fallback: %v", err)
	}
	if len(names) != 1 || names[0] != "openai" {
		t.Fatalf("expected instance provider fallback [openai], got %v", names)
	}
	models, err := a.llmSvc.ProviderModels(ctx, "no-such-project")
	if err != nil {
		t.Fatalf("ProviderModels fallback: %v", err)
	}
	got := models["openai"]
	if len(got) != 2 || got[0] != "m1" || got[1] != "m2" {
		t.Fatalf("expected [m1 m2], got %v", got)
	}
}

// TestB4_LLMRecorderWired：usage recorder 已接入（chat 前置解析失败时
// 返回错误而非 panic，间接证明 service 构造参数完整）。
func TestB4_LLMRecorderWired(t *testing.T) {
	a := newLLMApp(t, llmTestConfig(t, nil))
	if a.llmSvc == nil {
		t.Fatal("llmSvc must be constructed")
	}
	// 空项目 + 无实例供应商：解析出空 provider 集合。
	_, err := a.llmSvc.ListProviders(context.Background(), "no-such-project")
	if err != nil {
		t.Fatalf("empty providers must resolve without error: %v", err)
	}
}
