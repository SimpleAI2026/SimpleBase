// 文件说明：adapters_test.go — httpkit 共享端口适配器用例（缓存 / 配额 / 审计 / LLM）
//
// 用法
//
//	go test ./internal/api -run 'TestPlan79ConstructorsNil|TestCacheServiceAdapter|TestUsageAndAuditAdapters|TestLLMServiceAdapter'
//
// 调用架构
//
//	[上游] 本包测试（路由级用例复用 stubCatalog）
//	         │ 同包调用
//	         ▼
//	[本文件] internal/api/adapters_test.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/httpkit（被测适配器）、internal/llmgateway、godatabase/cache
//
// 主要符号
//
//	stubCatalog（路由级用例复用的 catalog 桩）
//	TestPlan79ConstructorsNil / TestCacheServiceAdapter / TestUsageAndAuditAdapters / TestLLMServiceAdapter
//
// 数据面适配器（DatabaseService / SQLService / lease）的用例已随实现迁到 internal/api/datahttp。

package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/voocel/litellm/providers"

	"github.com/linkxzhou/SimpleBase/godatabase/cache"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/usage"
)

type stubCatalog struct {
	db          catalog.Database
	list        []catalog.Database
	projects    []catalog.Project
	tenant      string
	createErr   error
	getErr      error
	listErr     error
	deleteErr   error
	resolveErr  error
	listPrjErr  error
	createPrj   catalog.Project
	createPrjEr error
	lastPurger  catalog.StoragePurger
	lastCloser  func(context.Context, string) error
}

func (s *stubCatalog) CreateDatabase(_ context.Context, in catalog.CreateDatabaseInput) (catalog.Database, error) {
	if s.createErr != nil {
		return catalog.Database{}, s.createErr
	}
	return catalog.Database{ID: "db-" + in.Name, Name: in.Name, ProjectID: in.ProjectID, TenantID: in.TenantID}, nil
}
func (s *stubCatalog) GetDatabase(context.Context, auth.Principal, string, string) (catalog.Database, error) {
	if s.getErr != nil {
		return catalog.Database{}, s.getErr
	}
	return s.db, nil
}
func (s *stubCatalog) ListDatabases(context.Context, auth.Principal, string, catalog.Page) ([]catalog.Database, string, error) {
	if s.listErr != nil {
		return nil, "", s.listErr
	}
	return s.list, "next", nil
}
func (s *stubCatalog) BeginDeleteDatabase(context.Context, auth.Principal, string, string) (catalog.Database, error) {
	if s.deleteErr != nil {
		return catalog.Database{}, s.deleteErr
	}
	s.db.Status = catalog.DatabaseDeleting
	return s.db, nil
}
func (s *stubCatalog) DeleteDatabaseSync(_ context.Context, _ auth.Principal, _, id string, closer func(context.Context, string) error, purger catalog.StoragePurger) (catalog.Database, error) {
	s.lastCloser = closer
	s.lastPurger = purger
	if closer != nil {
		_ = closer(context.Background(), id)
	}
	if purger != nil {
		_ = purger.DeletePrefix(context.Background(), "prefix")
	}
	if s.deleteErr != nil {
		return catalog.Database{}, s.deleteErr
	}
	s.db.Status = catalog.DatabaseDeleted
	return s.db, nil
}
func (s *stubCatalog) ResolveProjectTenant(context.Context, string) (string, error) {
	if s.resolveErr != nil {
		return "", s.resolveErr
	}
	return s.tenant, nil
}
func (s *stubCatalog) ListProjects(context.Context, auth.Principal) ([]catalog.Project, error) {
	if s.listPrjErr != nil {
		return nil, s.listPrjErr
	}
	return s.projects, nil
}
func (s *stubCatalog) CreateProject(_ context.Context, _ auth.Principal, in catalog.CreateProjectInput) (catalog.Project, error) {
	if s.createPrjEr != nil {
		return catalog.Project{}, s.createPrjEr
	}
	s.createPrj = catalog.Project{ID: in.ID, Name: in.Name}
	return s.createPrj, nil
}

func TestPlan79ConstructorsNil(t *testing.T) {
	if NewCacheService(nil) != nil || NewUsageService(nil) != nil || NewAuditService(nil) != nil || NewLLMService(nil) != nil {
		t.Fatal("nil constructors should return nil")
	}
}

func TestCacheServiceAdapter(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(dir+"/db-a", 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := cache.NewManager(cache.Options{Root: dir, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewCacheService(m)
	u, err := svc.Usage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.DatabaseDirs != 1 {
		t.Fatalf("dirs=%d", u.DatabaseDirs)
	}

	// error path: root is a file
	f, err := os.CreateTemp(t.TempDir(), "notadir")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	bad, err := cache.NewManager(cache.Options{Root: f.Name()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCacheService(bad).Usage(context.Background()); err == nil {
		t.Fatal("expected usage error on file root")
	}
}

func TestUsageAndAuditAdapters(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	repo := catalog.NewSQLRepository(db)
	us := NewUsageService(usage.NewService(repo, nil))
	// missing quota row → error is fine; we just need the adapter to forward
	_ = us.CheckQuota(context.Background(), "proj-1", "llm")

	as := NewAuditService(audit.NewService(repo, nil))
	if err := as.Record(context.Background(), AuditEvent{Kind: "create", ProjectID: "p", Status: "ok"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := as.ListOperations(context.Background(), "p", "", 10); err != nil {
		t.Fatalf("list: %v", err)
	}
}

type fakeGW struct {
	chatErr   error
	streamErr error
	listErr   error
	names     []string
}

func (f *fakeGW) Chat(_ context.Context, _ string, req llmgateway.Request) (llmgateway.Response, error) {
	if f.chatErr != nil {
		return llmgateway.Response{}, f.chatErr
	}
	return llmgateway.Response{
		Content: "hi", Model: req.Model, Provider: "openai", FinishReason: "stop",
		Usage: llmgateway.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
	}, nil
}
func (f *fakeGW) Stream(context.Context, string, llmgateway.Request) (llmgateway.StreamReader, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	return &fakeGWStream{chunks: []*providers.StreamChunk{{Type: "delta", Content: "x", FinishReason: ""}}}, nil
}
func (f *fakeGW) ListProviders(context.Context, string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.names, nil
}
func (f *fakeGW) ProviderModels(context.Context, string) (map[string][]string, error) {
	return map[string][]string{"openai": {"gpt-4o-mini"}}, nil
}

type fakeGWStream struct {
	chunks []*providers.StreamChunk
	i      int
	err    error
}

func (s *fakeGWStream) Next() (*providers.StreamChunk, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.i >= len(s.chunks) {
		return nil, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	return c, nil
}
func (s *fakeGWStream) Close() error { return nil }

func TestLLMServiceAdapter(t *testing.T) {
	gw := &fakeGW{names: []string{"openai"}}
	svc := NewLLMService(gw)
	max := 16
	temp := 0.2
	resp, err := svc.Chat(context.Background(), "p", LLMRequest{
		Model: "m", Messages: []LLMMessage{{Role: "user", Content: "hi"}}, MaxTokens: &max, Temperature: &temp,
	})
	if err != nil || resp.Content != "hi" || resp.Usage.TotalTokens != 3 {
		t.Fatalf("chat: %+v err=%v", resp, err)
	}
	r, err := svc.Stream(context.Background(), "p", LLMRequest{Messages: []LLMMessage{{Role: "u", Content: "q"}}})
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := r.Next()
	if err != nil || chunk.Content != "x" {
		t.Fatalf("stream next: %+v err=%v", chunk, err)
	}
	if _, err := r.Next(); err == nil {
		t.Fatal("expected EOF")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	names, err := svc.ListProviders(context.Background(), "p")
	if err != nil || len(names) != 1 {
		t.Fatalf("providers: %v %v", names, err)
	}

	gw.chatErr = errors.New("down")
	if _, err := svc.Chat(context.Background(), "p", LLMRequest{}); err == nil {
		t.Fatal("expected chat err")
	}
	gw.streamErr = errors.New("sdown")
	if _, err := svc.Stream(context.Background(), "p", LLMRequest{}); err == nil {
		t.Fatal("expected stream err")
	}

	// Next error path on adapter
	bad := &llmStreamReaderAdapter{Inner: &fakeGWStream{err: errors.New("chunk")}}
	if _, err := bad.Next(); err == nil {
		t.Fatal("expected next err")
	}
}

var _ objectstore.Deleter = (*stubPurger)(nil)

type stubPurger struct{ n int }

func (s *stubPurger) DeletePrefix(context.Context, string) error {
	s.n++
	return nil
}
