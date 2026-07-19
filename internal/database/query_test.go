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
	if res.LastInsertID == 0 {
		t.Errorf("LastInsertID = 0, want nonzero")
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
