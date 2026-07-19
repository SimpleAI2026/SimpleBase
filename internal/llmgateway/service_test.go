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
	svc := NewService(r, nil, nil)
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

func TestListProvidersEmpty(t *testing.T) {
	r := &fakeResolver{providers: ProjectProviders{}}
	svc := NewService(r, nil, nil)
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
	svc := NewService(r, nil, nil)
	_, err := svc.ListProviders(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error from resolver")
	}
}

func TestRecordLLMUsageRecorder(t *testing.T) {
	r := &fakeResolver{providers: ProjectProviders{
		Default: "openai",
		Providers: []ProviderConfig{{Name: "openai"}},
	}}
	rec := &fakeRecorder{}
	svc := NewService(r, rec, nil)
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
