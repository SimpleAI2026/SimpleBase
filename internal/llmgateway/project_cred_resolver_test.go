package llmgateway

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

type staticCreds struct {
	creds ProjectCreds
	err   error
}

func (s staticCreds) LoadProjectCreds(context.Context, string) (ProjectCreds, error) {
	if s.err != nil {
		return ProjectCreds{}, s.err
	}
	return s.creds, nil
}

func TestLitellmDriverMapsCompatiblePresets(t *testing.T) {
	driver, ok := litellmDriver("custom_openai")
	if !ok || driver != "openai" {
		t.Fatalf("custom_openai driver=%q ok=%v", driver, ok)
	}
	driver, ok = litellmDriver(" Google ")
	if !ok || driver != "gemini" {
		t.Fatalf("google driver=%q ok=%v", driver, ok)
	}
	if _, ok := litellmDriver("not-a-real-provider"); ok {
		t.Fatal("unknown provider should stay unmapped")
	}
}

func TestProjectCredResolverUsesSettingsDefaultAndSkipsEmptyKeys(t *testing.T) {
	r := NewProjectCredResolver(staticCreds{creds: ProjectCreds{
		DefaultProvider: "missing",
		DefaultModel:    "from-settings",
		Providers: []ProviderConfig{
			{Name: "openai", APIKey: ""},
			{Name: "custom_openai", APIKey: "sk-live", BaseURL: "http://127.0.0.1:9/v1", Model: "fake-model"},
		},
	}})
	pp, err := r.Resolve(context.Background(), "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if pp.ProjectID != "proj-1" || pp.Default != "custom_openai" || len(pp.Providers) != 1 {
		t.Fatalf("%+v", pp)
	}
	if pp.Providers[0].APIKey != "sk-live" || pp.Providers[0].Model != "fake-model" {
		t.Fatalf("%+v", pp.Providers[0])
	}

	empty, err := NewProjectCredResolver(nil).Resolve(context.Background(), "proj-1")
	if err != nil || len(empty.Providers) != 0 {
		t.Fatalf("nil source: %+v %v", empty, err)
	}
	if _, err := NewProjectCredResolver(staticCreds{err: errors.New("db down")}).Resolve(context.Background(), "p"); err == nil {
		t.Fatal("source error")
	}
}

func TestProjectCredResolverFillsModelFromSettings(t *testing.T) {
	r := NewProjectCredResolver(staticCreds{creds: ProjectCreds{
		DefaultProvider: "custom_openai",
		DefaultModel:    "from-settings",
		Providers: []ProviderConfig{
			{Name: "custom_openai", APIKey: "sk", BaseURL: "http://127.0.0.1:9/v1"},
		},
	}})
	pp, err := r.Resolve(context.Background(), "p")
	if err != nil {
		t.Fatal(err)
	}
	if pp.Default != "custom_openai" || pp.Providers[0].Model != "from-settings" {
		t.Fatalf("%+v", pp)
	}
}

func TestSystemCredSourceReadsProjectCreds(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := systemdb.ApplySystemMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)
	if err := store.PutLLMSettings(ctx, systemdb.LLMSettings{
		ProjectID: "proj-1", DefaultProvider: "custom_openai", DefaultModel: "fake-model",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertLLMProviderCred(ctx, "proj-1", "custom_openai", "fake-model", map[string]string{
		"api_key": "sk-live", "base_url": "http://127.0.0.1:9/v1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertLLMProviderCred(ctx, "proj-1", "openai", "", map[string]string{
		"api_key": "sk-other",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE sys_llm_provider_creds SET enabled=0 WHERE project_id=? AND provider=?`, "proj-1", "openai"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertLLMProviderCred(ctx, "proj-2", "custom_openai", "other", map[string]string{
		"api_key": "sk-other-project", "base_url": "http://127.0.0.1:8/v1",
	}); err != nil {
		t.Fatal(err)
	}

	src := NewSystemCredSource(store)
	got, err := NewProjectCredResolver(src).Resolve(ctx, "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Default != "custom_openai" || len(got.Providers) != 1 || got.Providers[0].APIKey != "sk-live" {
		t.Fatalf("proj-1: %+v", got)
	}
	if got.Providers[0].BaseURL != "http://127.0.0.1:9/v1" || got.Providers[0].Model != "fake-model" {
		t.Fatalf("fields: %+v", got.Providers[0])
	}
	other, err := NewProjectCredResolver(src).Resolve(ctx, "proj-2")
	if err != nil || other.Providers[0].APIKey != "sk-other-project" {
		t.Fatalf("proj-2: %+v %v", other, err)
	}
	none, err := NewProjectCredResolver(src).Resolve(ctx, "proj-missing")
	if err != nil || len(none.Providers) != 0 {
		t.Fatalf("missing: %+v %v", none, err)
	}
	if _, err := NewSystemCredSource(nil).LoadProjectCreds(ctx, "proj-1"); err != nil {
		t.Fatal(err)
	}
}

func TestFirstUsablePrefersProjectKeysThenCatalog(t *testing.T) {
	project := NewProjectCredResolver(staticCreds{creds: ProjectCreds{
		DefaultProvider: "custom_openai",
		Providers:       []ProviderConfig{{Name: "custom_openai", APIKey: "project-key"}},
	}})
	catalogRes := &fakeResolver{providers: ProjectProviders{
		Default:   "openai",
		Providers: []ProviderConfig{{Name: "openai", APIKey: "catalog-key"}},
	}}
	pp, err := NewFirstUsableResolver(project, catalogRes).Resolve(context.Background(), "p")
	if err != nil || len(pp.Providers) != 1 || pp.Providers[0].APIKey != "project-key" {
		t.Fatalf("%+v %v", pp, err)
	}

	emptyProject := NewProjectCredResolver(staticCreds{})
	pp, err = NewFirstUsableResolver(emptyProject, catalogRes).Resolve(context.Background(), "p")
	if err != nil || pp.Providers[0].APIKey != "catalog-key" {
		t.Fatalf("catalog fallback: %+v %v", pp, err)
	}

	// 与 app 装配一致：项目凭证 → catalog（无 key）→ YAML 实例供应商。
	repoDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repoDB.Close() })
	if err := systemdb.ApplySystemMigrations(context.Background(), repoDB); err != nil {
		t.Fatal(err)
	}
	cat := catalog.NewService(catalog.NewSQLRepository(repoDB), objectstore.KeyBuilder{}, nil, objectstore.DuckLakeStorage{}, nil)
	chain := NewFallbackResolver(NewFirstUsableResolver(
		NewProjectCredResolver(staticCreds{}),
		NewCatalogResolver(cat, nil),
	), map[string]InstanceProvider{
		"openai": {APIKey: "instance-key", BaseURL: "http://127.0.0.1:9/v1", DefaultModel: "m"},
	})
	pp, err = chain.Resolve(context.Background(), "p")
	if err != nil || len(pp.Providers) != 1 || pp.Providers[0].APIKey != "instance-key" {
		t.Fatalf("yaml fallback: %+v %v", pp, err)
	}

	if _, err := NewFirstUsableResolver(NewProjectCredResolver(staticCreds{err: errors.New("db")}), catalogRes).Resolve(context.Background(), "p"); err == nil {
		t.Fatal("primary error should surface")
	}
}
