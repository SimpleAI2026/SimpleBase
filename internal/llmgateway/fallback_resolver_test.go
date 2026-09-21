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
