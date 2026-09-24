package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/cache"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/usage"
	"github.com/voocel/litellm/providers"

	_ "github.com/uglyer/go-sqlite3"
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
	closedID    string
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

type stubRegistry struct {
	lease      *registry.Lease
	acquireErr error
	closeErr   error
	closedID   string
}

func (s *stubRegistry) Acquire(context.Context, catalog.Database, database.AccessMode) (*registry.Lease, error) {
	if s.acquireErr != nil {
		return nil, s.acquireErr
	}
	return s.lease, nil
}
func (s *stubRegistry) CloseDatabase(_ context.Context, id string) error {
	s.closedID = id
	return s.closeErr
}

type stubPurger struct{ n int }

func (s *stubPurger) DeletePrefix(context.Context, string) error {
	s.n++
	return nil
}

type stubSystemStore struct {
	meta catalog.Database
	db   *sql.DB
}

func (s *stubSystemStore) Meta() catalog.Database { return s.meta }
func (s *stubSystemStore) DB() *sql.DB            { return s.db }

type memFactory struct{}

func (memFactory) Open(_ context.Context, _ catalog.Database, _ database.AccessMode) (*sql.DB, error) {
	return sql.Open("sqlite3", ":memory:")
}

func TestDatabaseServiceAdapter(t *testing.T) {
	cat := &stubCatalog{
		db:     catalog.Database{ID: "db-1", Name: "n", ProjectID: "p"},
		list:   []catalog.Database{{ID: "db-1"}},
		tenant: "ten",
	}
	reg := &stubRegistry{lease: &registry.Lease{}}
	purger := &stubPurger{}
	svc := NewDatabaseServiceAdapter(cat, reg, purger)

	ctx := context.Background()
	p := auth.Principal{APIKeyID: "k"}
	if _, err := svc.CreateDatabase(ctx, catalog.CreateDatabaseInput{Name: "x", ProjectID: "p"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetDatabase(ctx, p, "p", "db-1"); err != nil {
		t.Fatal(err)
	}
	if list, cur, err := svc.ListDatabases(ctx, p, "p", catalog.Page{}); err != nil || cur != "next" || len(list) != 1 {
		t.Fatalf("list=%v cur=%s err=%v", list, cur, err)
	}
	if _, err := svc.BeginDeleteDatabase(ctx, p, "p", "db-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteDatabase(ctx, p, "p", "db-1"); err != nil {
		t.Fatal(err)
	}
	if purger.n != 1 {
		t.Fatalf("purger calls=%d", purger.n)
	}
	if reg.closedID != "db-1" {
		t.Fatalf("close id=%s", reg.closedID)
	}
	// nil purger still works
	svc2 := NewDatabaseServiceAdapter(cat, reg, nil)
	if _, err := svc2.DeleteDatabase(ctx, p, "p", "db-1"); err != nil {
		t.Fatal(err)
	}

	reg.acquireErr = errors.New("no lease")
	sqlSvc := NewSQLServiceAdapter(cat, reg, nil)
	if _, err := sqlSvc.Acquire(ctx, cat.db, database.ReadWrite); err == nil {
		t.Fatal("expected acquire error")
	}
}

func TestSQLServiceAdapter(t *testing.T) {
	cat := &stubCatalog{db: catalog.Database{ID: "u1", Kind: catalog.DatabaseKindUser}, list: []catalog.Database{{ID: "u1"}}}
	reg := &stubRegistry{lease: &registry.Lease{}}
	sysDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sysDB.Close() })
	sys := &stubSystemStore{meta: catalog.Database{ID: "sys", Kind: catalog.DatabaseKindSystem}, db: sysDB}
	svc := NewSQLServiceAdapter(cat, reg, sys)

	ctx := context.Background()
	p := auth.Principal{}
	if _, err := svc.GetDatabase(ctx, p, "p", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.(*sqlServiceAdapter).ListDatabases(ctx, p, "p", catalog.Page{}); err != nil {
		t.Fatal(err)
	}

	// system + readonly → systemLeaseAdapter
	sysDBRow := catalog.Database{ID: "sys", Kind: catalog.DatabaseKindSystem}
	lease, err := svc.Acquire(ctx, sysDBRow, database.ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lease.(*systemLeaseAdapter); !ok {
		t.Fatalf("want systemLeaseAdapter, got %T", lease)
	}

	// user db → sqlLeaseAdapter (or acquire error)
	userLease, err := svc.Acquire(ctx, cat.db, database.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := userLease.(*sqlLeaseAdapter); !ok {
		t.Fatalf("want sqlLeaseAdapter, got %T", userLease)
	}

	reg.acquireErr = errors.New("boom")
	if _, err := svc.Acquire(ctx, cat.db, database.ReadOnly); err == nil {
		t.Fatal("expected error")
	}

	// nil system: even system db goes to registry
	reg.acquireErr = nil
	svcNoSys := NewSQLServiceAdapter(cat, reg, nil)
	l, err := svcNoSys.Acquire(ctx, sysDBRow, database.ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := l.(*sqlLeaseAdapter); !ok {
		t.Fatalf("nil system should use registry, got %T", l)
	}
}

func TestSystemLeaseAdapter(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE t(id INTEGER); INSERT INTO t VALUES (7)`); err != nil {
		t.Fatal(err)
	}
	s := &systemLeaseAdapter{conn: db}
	s.Release() // no-op
	res, err := s.Query(context.Background(), database.Statement{SQL: "SELECT id FROM t"}, 10)
	if err != nil || len(res.Rows) != 1 {
		t.Fatalf("query: %+v err=%v", res, err)
	}
	if _, err := s.Execute(context.Background(), database.Statement{SQL: "DELETE FROM t"}); !errors.Is(err, catalog.ErrSystemProtected) {
		t.Fatalf("execute: %v", err)
	}
	if _, err := s.Batch(context.Background(), nil, true); !errors.Is(err, catalog.ErrSystemProtected) {
		t.Fatalf("batch: %v", err)
	}
}

func TestSQLLeaseAdapter_WithRegistry(t *testing.T) {
	reg := registry.New(memFactory{}, nil, registry.Options{Writable: true}, nil, nil)
	lease, err := reg.Acquire(context.Background(), catalog.Database{ID: "db-sql", Status: catalog.DatabaseReady}, database.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	ad := &sqlLeaseAdapter{lease: lease}
	if _, err := ad.Query(context.Background(), database.Statement{SQL: "SELECT 1"}, 10); err != nil {
		t.Fatalf("query: %v", err)
	}
	if _, err := ad.Execute(context.Background(), database.Statement{SQL: "CREATE TABLE x(id INT)"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := ad.Batch(context.Background(), []database.Statement{{SQL: "INSERT INTO x VALUES (1)"}}, true); err != nil {
		t.Fatalf("batch: %v", err)
	}
	ad.Release()
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
	if err := systemdb.ApplySystemMigrations(context.Background(), db); err != nil {
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
	bad := &llmStreamReaderAdapter{inner: &fakeGWStream{err: errors.New("chunk")}}
	if _, err := bad.Next(); err == nil {
		t.Fatal("expected next err")
	}
}

var _ objectstore.Deleter = (*stubPurger)(nil)
