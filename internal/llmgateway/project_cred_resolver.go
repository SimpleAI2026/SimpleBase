package llmgateway

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// ProjectCreds 是设置 API 写入系统库的项目级供应商（含解密后的 key，禁止记日志）。
type ProjectCreds struct {
	DefaultProvider string
	DefaultModel    string
	Providers       []ProviderConfig
}

// ProjectCredSource 每次 Resolve 现读项目凭证，保存后无需重启进程。
type ProjectCredSource interface {
	LoadProjectCreds(ctx context.Context, projectID string) (ProjectCreds, error)
}

// ProjectCredResolver 从系统库项目凭证解析供应商。
// 不用启动时的 YAML/env 快照，也不读 sys_llm_provider_configs。
type ProjectCredResolver struct {
	Source ProjectCredSource
}

// NewProjectCredResolver 构造项目凭证 resolver。Source 为 nil 时解析为空。
func NewProjectCredResolver(src ProjectCredSource) *ProjectCredResolver {
	return &ProjectCredResolver{Source: src}
}

// Resolve 按 projectID 读取已启用且带 api_key 的供应商。
// 默认供应商优先用 llm/settings；对不上时退到第一个可用供应商。
func (r *ProjectCredResolver) Resolve(ctx context.Context, projectID string) (ProjectProviders, error) {
	if r == nil || r.Source == nil {
		return ProjectProviders{ProjectID: projectID}, nil
	}
	creds, err := r.Source.LoadProjectCreds(ctx, projectID)
	if err != nil {
		return ProjectProviders{}, err
	}
	pp := ProjectProviders{ProjectID: projectID}
	wantDefault := strings.TrimSpace(creds.DefaultProvider)
	for _, p := range creds.Providers {
		if strings.TrimSpace(p.APIKey) == "" || strings.TrimSpace(p.Name) == "" {
			continue
		}
		if p.Model == "" && p.Name == wantDefault {
			p.Model = strings.TrimSpace(creds.DefaultModel)
		}
		pp.Providers = append(pp.Providers, p)
	}
	if providerNamed(pp.Providers, wantDefault) {
		pp.Default = wantDefault
	} else if len(pp.Providers) > 0 {
		pp.Default = pp.Providers[0].Name
	}
	return pp, nil
}

func providerNamed(list []ProviderConfig, name string) bool {
	if name == "" {
		return false
	}
	for _, p := range list {
		if p.Name == name {
			return true
		}
	}
	return false
}

// systemCredSource 把 sys_llm_provider_creds 与 sys_llm_settings 转成 ProjectCreds。
type systemCredSource struct {
	store *systemdb.Store
}

// NewSystemCredSource 用系统库作为项目凭证来源。store 为 nil 时解析为空。
func NewSystemCredSource(store *systemdb.Store) ProjectCredSource {
	return &systemCredSource{store: store}
}

func (s *systemCredSource) LoadProjectCreds(ctx context.Context, projectID string) (ProjectCreds, error) {
	if s == nil || s.store == nil {
		return ProjectCreds{}, nil
	}
	st, err := s.store.GetLLMSettings(ctx, projectID)
	if err != nil {
		return ProjectCreds{}, err
	}
	rows, err := s.store.ListLLMProviderCreds(ctx, projectID)
	if err != nil {
		return ProjectCreds{}, err
	}
	out := ProjectCreds{DefaultProvider: st.DefaultProvider, DefaultModel: st.DefaultModel}
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		out.Providers = append(out.Providers, providerFromStoredCred(row))
	}
	return out, nil
}

// providerFromStoredCred 解析 credentials_json。密钥只留在返回值里。
func providerFromStoredCred(row systemdb.LLMProviderCred) ProviderConfig {
	fields := map[string]string{}
	if row.CredentialsJSON != "" {
		_ = json.Unmarshal([]byte(row.CredentialsJSON), &fields)
	}
	base := strings.TrimSpace(fields["base_url"])
	if base == "" {
		base = strings.TrimSpace(fields["endpoint"])
	}
	return ProviderConfig{
		Name:    row.Provider,
		APIKey:  strings.TrimSpace(fields["api_key"]),
		BaseURL: base,
		Model:   strings.TrimSpace(row.DefaultModel),
	}
}

// FirstUsableResolver 按顺序采用第一个带可用 API key 的结果。
// 项目凭证优先于 catalog；都没有 key 时把空结果交给 FallbackResolver 决定是否用实例 YAML。
type FirstUsableResolver struct {
	Sources []ProviderResolver
}

// NewFirstUsableResolver 构造链式 resolver。
func NewFirstUsableResolver(sources ...ProviderResolver) *FirstUsableResolver {
	return &FirstUsableResolver{Sources: sources}
}

// Resolve 返回第一个含密钥的供应商集合。首选源出错时直接返回该错误。
func (r *FirstUsableResolver) Resolve(ctx context.Context, projectID string) (ProjectProviders, error) {
	if r == nil {
		return ProjectProviders{ProjectID: projectID}, nil
	}
	var fallback ProjectProviders
	haveFallback := false
	for i, src := range r.Sources {
		if src == nil {
			continue
		}
		pp, err := src.Resolve(ctx, projectID)
		if err != nil {
			if i == 0 {
				return ProjectProviders{}, err
			}
			continue
		}
		if hasUsableKey(pp) {
			return pp, nil
		}
		if !haveFallback {
			fallback = pp
			haveFallback = true
		}
	}
	if haveFallback {
		return fallback, nil
	}
	return ProjectProviders{ProjectID: projectID}, nil
}
