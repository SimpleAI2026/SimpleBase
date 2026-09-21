package llmgateway

import (
	"context"
	"time"
)

// InstanceProvider is an optional process-level LLM provider used when catalog
// credentials are unavailable (CredentialResolver nil / empty API keys).
type InstanceProvider struct {
	APIKey        string
	BaseURL       string
	DefaultModel  string
	AllowedModels []string
	Timeout       time.Duration
}

// FallbackResolver tries Primary first and falls back to instance-level providers
// when the catalog result has no usable API key.
type FallbackResolver struct {
	Primary  ProviderResolver
	Instance map[string]InstanceProvider
}

func NewFallbackResolver(primary ProviderResolver, instance map[string]InstanceProvider) *FallbackResolver {
	return &FallbackResolver{Primary: primary, Instance: instance}
}

func (r *FallbackResolver) Resolve(ctx context.Context, projectID string) (ProjectProviders, error) {
	if r == nil {
		return ProjectProviders{ProjectID: projectID}, nil
	}
	if r.Primary != nil {
		pp, err := r.Primary.Resolve(ctx, projectID)
		if err == nil && hasUsableKey(pp) {
			return pp, nil
		}
		if err != nil && len(r.Instance) == 0 {
			return ProjectProviders{}, err
		}
	}
	return instanceProviders(projectID, r.Instance), nil
}

func hasUsableKey(pp ProjectProviders) bool {
	for _, p := range pp.Providers {
		if p.APIKey != "" {
			return true
		}
	}
	return false
}

func instanceProviders(projectID string, in map[string]InstanceProvider) ProjectProviders {
	pp := ProjectProviders{ProjectID: projectID}
	if len(in) == 0 {
		return pp
	}
	for name, p := range in {
		pp.Providers = append(pp.Providers, ProviderConfig{
			Name:    name,
			APIKey:  p.APIKey,
			BaseURL: p.BaseURL,
			Model:   p.DefaultModel,
		})
		if pp.Default == "" {
			pp.Default = name
		}
	}
	return pp
}
