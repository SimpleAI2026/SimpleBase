package audit

import (
	"context"
	"database/sql"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	_ "github.com/uglyer/go-sqlite3"
)

func newTestRepoForAudit(t *testing.T) catalog.Repository {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.ApplySystemMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return catalog.NewSQLRepository(db)
}

func TestRecordAndList(t *testing.T) {
	repo := newTestRepoForAudit(t)
	svc := NewService(repo, observability.NewLogger("error", "console", nil))
	ctx := context.Background()
	if err := svc.Record(ctx, Event{
		ProjectID:   "proj-1",
		PrincipalID: "user-1",
		Kind:        "create",
		Status:      "ok",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	ops, err := svc.ListOperations(ctx, Query{ProjectID: "proj-1", Limit: 10})
	if err != nil {
		t.Fatalf("ListOperations: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Kind != "create" {
		t.Fatalf("expected kind create, got %s", ops[0].Kind)
	}
}

func TestRecordRejectsEmptyKind(t *testing.T) {
	repo := newTestRepoForAudit(t)
	svc := NewService(repo, observability.NewLogger("error", "console", nil))
	err := svc.Record(context.Background(), Event{ProjectID: "proj-1"})
	if err == nil {
		t.Fatal("expected error for empty kind")
	}
}

func TestRedact(t *testing.T) {
	got := Redact("sk-abcdef123456")
	// 长度 15：前2 + 11星 + 后2
	if got != "sk***********56" {
		t.Fatalf("unexpected redaction: %s", got)
	}
	got = Redact("ab")
	if got != "**" {
		t.Fatalf("expected ** for short string, got %s", got)
	}
}
