// catalog_resolver.go 从 catalog 解析 project 的 LLM 供应商配置（plan8.md）。
//
// 密钥来源：catalog 中 project 的 provider 配置（CredentialRef 指向密钥服务）。
// 禁止从环境变量自动发现密钥；禁止日志/审计回显密钥原文。
package llmgateway

import (
	"context"
	"fmt"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// CredentialResolver 从 CredentialRef 解析出实际密钥。
// 由独立密钥服务实现（如 KMS 封装）；llmgateway 不关心存储细节。
type CredentialResolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

// CatalogResolver 通过 catalog.Service 解析 project 的供应商配置。
type CatalogResolver struct {
	catalog    *catalog.Service
	credential CredentialResolver
}

// NewCatalogResolver 构造 resolver。credential 为 nil 时只返回配置不含密钥
// （仅适用于测试或无密钥供应商）。
func NewCatalogResolver(cat *catalog.Service, cred CredentialResolver) *CatalogResolver {
	return &CatalogResolver{catalog: cat, credential: cred}
}

// Resolve 从 catalog 读取 project 的 LLM 供应商配置并通过密钥服务解析密钥。
func (r *CatalogResolver) Resolve(ctx context.Context, projectID string) (ProjectProviders, error) {
	if r.catalog == nil {
		return ProjectProviders{}, fmt.Errorf("llmgateway: catalog service is nil")
	}
	raw, err := r.catalog.GetLLMProviders(ctx, projectID)
	if err != nil {
		return ProjectProviders{}, fmt.Errorf("llmgateway: get providers from catalog: %w", err)
	}
	pp := ProjectProviders{ProjectID: projectID, Default: raw.Default}
	for _, p := range raw.Providers {
		apiKey := ""
		if r.credential != nil && p.CredentialRef != "" {
			key, kerr := r.credential.Resolve(ctx, p.CredentialRef)
			if kerr != nil {
				// 密钥解析失败不回显 ref；跳过该供应商。
				continue
			}
			apiKey = key
		}
		pp.Providers = append(pp.Providers, ProviderConfig{
			Name:    p.Provider,
			APIKey:  apiKey,
			BaseURL: "",
			Model:   p.DefaultModel,
		})
	}
	return pp, nil
}
