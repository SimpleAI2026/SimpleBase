// 文件说明：service_test.go — internal/audit 的单元测试（TestRecordAndList、TestRecordRejectsEmptyKind 等）
//
// 用法
//
//	go test ./internal/audit -run 'TestRecordAndList|TestRecordRejectsEmptyKind'
//	同包调用：newTestRepoForAudit() → record_test.go
//
// 调用架构
//
//	[上游] internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/audit/service_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] record_test.go, service.go
//	[关键引用] newTestRepoForAudit ← record_test.go
//
// 主要符号
//
//	newTestRepoForAudit(t *testing.T) catalog.Repository
//	TestRecordAndList(t *testing.T)
//	TestRecordRejectsEmptyKind(t *testing.T)

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
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
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
