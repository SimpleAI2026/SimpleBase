package systemdb

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/uglyer/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := ApplySystemMigrations(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStoreForTest(db)
}

func TestGoFunctionCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pid := "proj-1"

	created, err := s.CreateGoFunction(ctx, GoFunction{
		ProjectID: pid, Name: "hello", Source: "package main\nfunc Hello(x int) int { return x }",
		Exports: []string{"Hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.UpdatedAt.IsZero() {
		t.Fatalf("unexpected created row: %+v", created)
	}

	// 同名再建由应用层查重（store 不做唯一约束），此处验证 Get 命中第一条
	got, err := s.GetGoFunction(ctx, pid, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "hello" || len(got.Exports) != 1 || got.Exports[0] != "Hello" {
		t.Fatalf("mismatch: %+v", got)
	}

	// 跨项目不可见
	if _, err := s.GetGoFunction(ctx, "proj-2", "hello"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want ErrNoRows, got %v", err)
	}

	// 更新源码与 exports
	upd, err := s.UpdateGoFunction(ctx, GoFunction{
		ProjectID: pid, Name: "hello",
		Source:  "package main\nfunc Hello(x int) int { return x }\nfunc Ping() int { return 0 }",
		Exports: []string{"Hello", "Ping"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(upd.Exports) != 2 {
		t.Fatalf("exports not updated: %+v", upd)
	}

	// 列表
	list, err := s.ListGoFunctions(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Source != upd.Source {
		t.Fatalf("list mismatch: %+v", list)
	}

	// 计数
	n, err := s.CountGoFunctions(ctx, pid)
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}

	// 软删后不可见
	if err := s.ArchiveGoFunction(ctx, pid, "hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetGoFunction(ctx, pid, "hello"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want ErrNoRows after archive, got %v", err)
	}
	n, err = s.CountGoFunctions(ctx, pid)
	if err != nil || n != 0 {
		t.Fatalf("count after archive=%d err=%v", n, err)
	}

	// 软删不存在的 → ErrNoRows
	if err := s.ArchiveGoFunction(ctx, pid, "hello"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want ErrNoRows, got %v", err)
	}
}
