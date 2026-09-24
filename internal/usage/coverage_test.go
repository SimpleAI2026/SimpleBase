package usage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

type stubRepo struct {
	quota     catalog.ProjectQuota
	quotaErr  error
	sum       catalog.UsageSummary
	sumErr    error
	appendErr error
	appended  []catalog.UsageEvent
}

func (s *stubRepo) CreateTenant(context.Context, catalog.Tenant) error { return nil }
func (s *stubRepo) CreateProject(context.Context, catalog.Project) error {
	return nil
}
func (s *stubRepo) ListProjectsByTenant(context.Context, string) ([]catalog.Project, error) {
	return nil, nil
}
func (s *stubRepo) CreateDatabase(context.Context, catalog.Database) error { return nil }
func (s *stubRepo) GetDatabase(context.Context, string, string) (catalog.Database, error) {
	return catalog.Database{}, nil
}
func (s *stubRepo) ListDatabases(context.Context, string, catalog.Page) ([]catalog.Database, string, error) {
	return nil, "", nil
}
func (s *stubRepo) ListDatabasesByKind(context.Context, string, string, catalog.Page) ([]catalog.Database, string, error) {
	return nil, "", nil
}
func (s *stubRepo) ListDatabasesByStatuses(context.Context, []catalog.DatabaseStatus) ([]catalog.Database, error) {
	return nil, nil
}
func (s *stubRepo) TransitionDatabase(context.Context, string, []catalog.DatabaseStatus, catalog.DatabaseStatus, time.Time) (catalog.Database, error) {
	return catalog.Database{}, nil
}
func (s *stubRepo) MarkDeleted(context.Context, string, time.Time) error { return nil }
func (s *stubRepo) UpsertProviderConfig(context.Context, catalog.LLMProviderConfig) error {
	return nil
}
func (s *stubRepo) ListEnabledProviders(context.Context, string) ([]catalog.LLMProviderConfig, error) {
	return nil, nil
}
func (s *stubRepo) AppendUsage(_ context.Context, events []catalog.UsageEvent) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	s.appended = append(s.appended, events...)
	return nil
}
func (s *stubRepo) AppendOperation(context.Context, catalog.Operation) error { return nil }
func (s *stubRepo) ListOperations(context.Context, string, string, int) ([]catalog.Operation, error) {
	return nil, nil
}
func (s *stubRepo) GetLLMProviders(context.Context, string) (catalog.LLMProviders, error) {
	return catalog.LLMProviders{}, nil
}
func (s *stubRepo) SetLLMProviders(context.Context, string, catalog.LLMProviders) error {
	return nil
}
func (s *stubRepo) GetQuota(context.Context, string) (catalog.ProjectQuota, error) {
	return s.quota, s.quotaErr
}
func (s *stubRepo) UpsertQuota(context.Context, catalog.ProjectQuota) error { return nil }
func (s *stubRepo) SumUsageSince(context.Context, string, time.Time) (catalog.UsageSummary, error) {
	return s.sum, s.sumErr
}
func (s *stubRepo) ProjectBelongsToTenant(context.Context, string, string) (bool, error) {
	return true, nil
}
func (s *stubRepo) GetProjectTenant(context.Context, string) (string, error) { return "", nil }

func TestRecordDatabaseFlushAndQuotaBranches(t *testing.T) {
	repo := &stubRepo{quota: catalog.ProjectQuota{PeriodSeconds: 0, MaxLLMRequests: 1, MaxLLMTokens: 10, MaxDatabases: 2, MaxStorageBytes: 100}}
	var logBuf strings.Builder
	svc := NewService(repo, observability.NewLogger("warn", "json", &logBuf))
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	svc.now = func() time.Time { return fixed }
	ctx := context.Background()

	if err := svc.RecordDatabase(ctx, "p1", "r1", 99); err != nil {
		t.Fatal(err)
	}
	if len(repo.appended) != 1 || repo.appended[0].Kind != "database" {
		t.Fatalf("%+v", repo.appended)
	}

	if err := svc.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	svc.flushAt = fixed
	svc.buffer = []catalog.UsageEvent{{ID: "keep", ProjectID: "p1"}}
	if err := svc.append(ctx, catalog.UsageEvent{ID: "buffered", ProjectID: "p1"}); err != nil {
		t.Fatal(err)
	}
	if len(svc.buffer) != 2 {
		t.Fatalf("should not flush: %d", len(svc.buffer))
	}

	repo.appendErr = errors.New("write")
	if err := svc.Flush(ctx); err == nil {
		t.Fatal("flush error")
	}
	if !strings.Contains(logBuf.String(), "flush failed") {
		t.Fatalf("log=%s", logBuf.String())
	}
	svc.logger = nil
	svc.buffer = []catalog.UsageEvent{{ID: "x"}}
	if err := svc.Flush(ctx); err == nil {
		t.Fatal("flush error no logger")
	}

	repo.appendErr = nil
	repo.quotaErr = errors.New("quota")
	if err := svc.CheckQuota(ctx, "p1", "llm"); err == nil {
		t.Fatal("quota err")
	}
	repo.quotaErr = nil
	repo.sumErr = errors.New("sum")
	if err := svc.CheckQuota(ctx, "p1", "llm"); err == nil {
		t.Fatal("sum err")
	}
	repo.sumErr = nil
	repo.sum = catalog.UsageSummary{LLMRequests: 1, LLMTokens: 0}
	if err := svc.CheckQuota(ctx, "p1", "llm"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("llm req: %v", err)
	}
	repo.sum = catalog.UsageSummary{LLMRequests: 0, LLMTokens: 10}
	if err := svc.CheckQuota(ctx, "p1", "llm"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("llm tokens: %v", err)
	}
	repo.sum = catalog.UsageSummary{DatabaseStorage: 100}
	if err := svc.CheckQuota(ctx, "p1", "database"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("storage: %v", err)
	}
	repo.sum = catalog.UsageSummary{}
	if err := svc.CheckQuota(ctx, "p1", "database"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckQuota(ctx, "p1", "other"); err != nil {
		t.Fatal(err)
	}
}

func TestLLMRecorderAdapter(t *testing.T) {
	repo := newTestRepoForUsage(t)
	svc := NewService(repo, nil)
	rec := NewLLMRecorder(svc)
	if err := rec.RecordLLM(context.Background(), "p1", "openai", "gpt", llmgateway.Usage{
		PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAppendFlushThreshold(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, nil)
	svc.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	svc.flushAt = svc.now().Add(-6 * time.Second)
	svc.buffer = []catalog.UsageEvent{{ID: "old"}}
	if err := svc.append(context.Background(), catalog.UsageEvent{ID: "new"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.appended) != 2 {
		t.Fatalf("time flush appended=%d", len(repo.appended))
	}

	svc = NewService(repo, nil)
	svc.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	svc.flushAt = svc.now()
	for i := 0; i < 63; i++ {
		svc.buffer = append(svc.buffer, catalog.UsageEvent{ID: "x"})
	}
	if err := svc.append(context.Background(), catalog.UsageEvent{ID: "64"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.appended) < 64 {
		t.Fatalf("size flush %d", len(repo.appended))
	}
}
