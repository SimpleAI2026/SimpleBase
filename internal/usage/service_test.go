package usage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	_ "github.com/uglyer/go-sqlite3"
)

func newTestRepoForUsage(t *testing.T) catalog.Repository {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := catalog.ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return catalog.NewSQLiteRepository(db)
}

func TestRecordAndCheckQuota(t *testing.T) {
	repo := newTestRepoForUsage(t)
	svc := NewService(repo, observability.NewLogger("error", "console", nil))
	ctx := context.Background()

	// 设置 LLM 配额为 2 请求/小时。
	if err := svc.SetQuota(ctx, catalog.ProjectQuota{
		ProjectID:      "proj-1",
		MaxLLMRequests: 2,
		PeriodSeconds:  3600,
	}); err != nil {
		t.Fatalf("SetQuota: %v", err)
	}

	// 记录 2 次 LLM 用量。
	for i := 0; i < 2; i++ {
		if err := svc.RecordLLM(ctx, "proj-1", "openai", "gpt-4", LLMUsage{
			PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
		}); err != nil {
			t.Fatalf("RecordLLM: %v", err)
		}
	}

	// 第 3 次应超配额。
	err := svc.CheckQuota(ctx, "proj-1", "llm")
	if err == nil {
		t.Fatal("expected quota exceeded")
	}
}

func TestQuotaZeroMeansUnlimited(t *testing.T) {
	repo := newTestRepoForUsage(t)
	svc := NewService(repo, observability.NewLogger("error", "console", nil))
	ctx := context.Background()
	// 不设配额 → 零值（不限）。
	if err := svc.CheckQuota(ctx, "proj-1", "llm"); err != nil {
		t.Fatalf("expected no quota error, got: %v", err)
	}
}

func TestGetQuotaReturnsDefault(t *testing.T) {
	repo := newTestRepoForUsage(t)
	svc := NewService(repo, observability.NewLogger("error", "console", nil))
	ctx := context.Background()
	q, err := svc.GetQuota(ctx, "proj-1")
	if err != nil {
		t.Fatalf("GetQuota: %v", err)
	}
	if q.PeriodSeconds != 3600 {
		t.Fatalf("expected default period 3600, got %d", q.PeriodSeconds)
	}
}

// 避免未使用 import 报错（uuid 在其他测试用到）。
var _ = uuid.NewString
var _ = time.Now
