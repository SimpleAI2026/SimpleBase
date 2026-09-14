package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/uglyer/go-sqlite3"
)

func newMemDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestExecute_InsertReturnsRowsAffectedAndLastInsertID(t *testing.T) {
	db := newMemDB(t)
	res, err := Execute(context.Background(), db, Statement{SQL: "INSERT INTO t(name) VALUES(?)", Args: []any{"a"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("RowsAffected = %d, want 1", res.RowsAffected)
	}
	if res.LastInsertID != 0 {
		t.Errorf("LastInsertID = %d, want 0 (deprecated on DuckLake)", res.LastInsertID)
	}
}

func TestQuery_ReturnsRowsAndRespectsMaxRows(t *testing.T) {
	db := newMemDB(t)
	for i := 0; i < 5; i++ {
		if _, err := db.Exec("INSERT INTO t(name) VALUES(?)", "x"); err != nil {
			t.Fatalf("seed insert: %v", err)
		}
	}

	res, err := Query(context.Background(), db, Statement{SQL: "SELECT id, name FROM t ORDER BY id"}, 0)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res.Rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(res.Rows))
	}
	if len(res.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(res.Columns))
	}

	_, err = Query(context.Background(), db, Statement{SQL: "SELECT id FROM t"}, 2)
	if !errors.Is(err, ErrRowLimitExceeded) {
		t.Fatalf("expected ErrRowLimitExceeded, got %v", err)
	}
}

func TestBatch_TransactionalRollsBackOnFailure(t *testing.T) {
	db := newMemDB(t)
	stmts := []Statement{
		{SQL: "INSERT INTO t(name) VALUES(?)", Args: []any{"ok"}},
		{SQL: "INSERT INTO nonexistent_table(name) VALUES(?)", Args: []any{"bad"}},
	}
	_, err := Batch(context.Background(), db, stmts, true)
	if err == nil {
		t.Fatal("expected batch error")
	}

	res, err := Query(context.Background(), db, Statement{SQL: "SELECT COUNT(1) FROM t"}, 0)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if res.Rows[0][0].(int64) != 0 {
		t.Fatalf("expected rollback to leave table empty, got count=%v", res.Rows[0][0])
	}
}

func TestBatch_NonTransactionalStopsAtFailureButKeepsPriorResults(t *testing.T) {
	db := newMemDB(t)
	stmts := []Statement{
		{SQL: "INSERT INTO t(name) VALUES(?)", Args: []any{"ok1"}},
		{SQL: "INSERT INTO nonexistent_table(name) VALUES(?)", Args: []any{"bad"}},
		{SQL: "INSERT INTO t(name) VALUES(?)", Args: []any{"ok2"}},
	}
	results, err := Batch(context.Background(), db, stmts, false)
	if err == nil {
		t.Fatal("expected batch error")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 successful result before failure, got %d", len(results))
	}

	res, err := Query(context.Background(), db, Statement{SQL: "SELECT COUNT(1) FROM t"}, 0)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if res.Rows[0][0].(int64) != 1 {
		t.Fatalf("expected 1 row committed independently, got %v", res.Rows[0][0])
	}
}

func TestAccessModeString(t *testing.T) {
	if ReadWrite.String() != "read_write" {
		t.Errorf("ReadWrite.String() = %q", ReadWrite.String())
	}
	if ReadOnly.String() != "read_only" {
		t.Errorf("ReadOnly.String() = %q", ReadOnly.String())
	}
}

func TestQuery_EmptyResultSet(t *testing.T) {
	db := newMemDB(t)
	res, err := Query(context.Background(), db, Statement{SQL: "SELECT id, name FROM t WHERE id > 100"}, 0)
	if err != nil {
		t.Fatalf("Query empty result: %v", err)
	}
	if len(res.Rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(res.Rows))
	}
	if len(res.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(res.Columns))
	}
}

func TestQuery_ContextCanceled(t *testing.T) {
	db := newMemDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	_, err := Query(ctx, db, Statement{SQL: "SELECT 1"}, 0)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

func TestQuery_MaxRowsZeroMeansUnlimited(t *testing.T) {
	db := newMemDB(t)
	for i := 0; i < 10; i++ {
		if _, err := db.Exec("INSERT INTO t(name) VALUES(?)", "x"); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	res, err := Query(context.Background(), db, Statement{SQL: "SELECT id FROM t"}, 0)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res.Rows) != 10 {
		t.Fatalf("expected 10 rows (unlimited), got %d", len(res.Rows))
	}
}

func TestExecute_UpdateReturnsRowsAffected(t *testing.T) {
	db := newMemDB(t)
	for _, name := range []string{"a", "b", "c"} {
		if _, err := db.Exec("INSERT INTO t(name) VALUES(?)", name); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	res, err := Execute(context.Background(), db, Statement{SQL: "UPDATE t SET name = ?", Args: []any{"updated"}})
	if err != nil {
		t.Fatalf("Execute UPDATE: %v", err)
	}
	if res.RowsAffected != 3 {
		t.Fatalf("expected 3 rows affected, got %d", res.RowsAffected)
	}
}

func TestExecute_DeleteReturnsRowsAffected(t *testing.T) {
	db := newMemDB(t)
	if _, err := db.Exec("INSERT INTO t(name) VALUES(?)", "x"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := Execute(context.Background(), db, Statement{SQL: "DELETE FROM t WHERE name = ?", Args: []any{"x"}})
	if err != nil {
		t.Fatalf("Execute DELETE: %v", err)
	}
	if res.RowsAffected != 1 {
		t.Fatalf("expected 1 row affected, got %d", res.RowsAffected)
	}
}

func TestBatch_EmptyStatementsReturnsNilNil(t *testing.T) {
	db := newMemDB(t)
	results, err := Batch(context.Background(), db, nil, true)
	if err != nil {
		t.Fatalf("Batch(nil): %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results, got %v", results)
	}
}

func TestBatch_TransactionalAllSucceed(t *testing.T) {
	db := newMemDB(t)
	stmts := []Statement{
		{SQL: "INSERT INTO t(name) VALUES(?)", Args: []any{"a"}},
		{SQL: "INSERT INTO t(name) VALUES(?)", Args: []any{"b"}},
	}
	results, err := Batch(context.Background(), db, stmts, true)
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	res, _ := Query(context.Background(), db, Statement{SQL: "SELECT COUNT(1) FROM t"}, 0)
	if res.Rows[0][0].(int64) != 2 {
		t.Fatalf("expected 2 rows, got %v", res.Rows[0][0])
	}
}
