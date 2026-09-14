package ducklake

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
)

func openTestLake(t *testing.T) (*sql.DB, Layout) {
	t.Helper()
	dir := t.TempDir()
	id := uuid.NewString()
	opts := DefaultOptions()
	opts.ExtensionDir = filepath.Join(dir, "extensions")
	f := &Factory{CacheDir: dir, Options: opts}
	db, err := f.Open(context.Background(), catalog.Database{ID: id}, database.ReadWrite)
	if err != nil {
		t.Fatalf("open ducklake (need CGO + extension download): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, layoutFor(dir, id)
}

func TestPhase1_DuckDBVersionAndExtensions(t *testing.T) {
	db, _ := openTestLake(t)
	version, err := AssertDuckDBVersion(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if version == "" {
		t.Fatal("empty duckdb version")
	}
	if err := AssertExtensionsLoaded(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	t.Logf("duckdb version %s", version)
}

func TestPhase1_CreateInsertTransactionTimeTravelChangeFeed(t *testing.T) {
	db, _ := openTestLake(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `CREATE TABLE people (id INTEGER, name VARCHAR)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO people VALUES (1, 'alice')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	snap1, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatalf("current_snapshot after first insert: %v", err)
	}
	if snap1 <= 0 {
		t.Fatalf("expected snapshot > 0, got %d", snap1)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO people VALUES (2, 'bob')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM people`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rollback should leave 1 row, got %d", count)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO people VALUES (2, 'bob')`); err != nil {
		t.Fatal(err)
	}
	snap2, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	if snap2 <= snap1 {
		t.Fatalf("snapshot did not advance: %d -> %d", snap1, snap2)
	}

	var travelCount int
	q := `SELECT COUNT(*) FROM people AT (VERSION => ` + strconv.FormatInt(snap1, 10) + `)`
	if err := db.QueryRowContext(ctx, q).Scan(&travelCount); err != nil {
		t.Fatalf("time travel: %v", err)
	}
	if travelCount != 1 {
		t.Fatalf("time travel count = %d, want 1", travelCount)
	}

	rows, err := db.QueryContext(ctx, `SELECT * FROM table_changes('people', ?, ?)`, snap1, snap2)
	if err != nil {
		rows, err = db.QueryContext(ctx, `SELECT * FROM lake.table_changes('people', ?, ?)`, snap1, snap2)
	}
	if err != nil {
		t.Fatalf("table_changes: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected change feed rows")
	}
}

func TestPhase1_ResourceLimitsAndSingleConn(t *testing.T) {
	db, _ := openTestLake(t)
	var mem string
	var threads int
	if err := db.QueryRow(`SELECT current_setting('memory_limit')`).Scan(&mem); err != nil {
		t.Fatalf("memory_limit: %v", err)
	}
	if err := db.QueryRow(`SELECT current_setting('threads')`).Scan(&threads); err != nil {
		t.Fatalf("threads: %v", err)
	}
	if mem == "" {
		t.Fatal("empty memory_limit")
	}
	if threads <= 0 {
		t.Fatalf("threads = %d", threads)
	}

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO t VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	var got int
	if err := db.QueryRowContext(ctx, `SELECT x FROM t`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("got %d", got)
	}
}

func TestPhase1_AllowedDirectoriesBlocksEscape(t *testing.T) {
	db, _ := openTestLake(t)
	outside := filepath.Join(t.TempDir(), "secret.csv")
	if err := os.WriteFile(outside, []byte("a\n1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`SELECT * FROM read_csv_auto(?)`, outside)
	if err == nil {
		t.Fatal("expected read outside allowed_directories to fail")
	}
}

func TestPhase1_DataInliningAndCatalogBackup(t *testing.T) {
	db, layout := openTestLake(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE tiny (id INTEGER, note VARCHAR)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tiny VALUES (1, 'a'), (2, 'b')`); err != nil {
		t.Fatal(err)
	}
	snap, err := CurrentSnapshot(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	if snap <= 0 {
		t.Fatalf("snapshot %d", snap)
	}

	if n := countParquet(t, layout.DataDir); n != 0 {
		t.Fatalf("expected inlined insert to create 0 parquet files, found %d", n)
	}

	backup := filepath.Join(layout.Root, "backup.sqlite")
	if _, err := db.ExecContext(ctx, `ATTACH 'sqlite:`+filepathToSlash(backup)+`' AS backup`); err != nil {
		t.Fatalf("attach backup: %v", err)
	}
	if _, err := db.ExecContext(ctx, `COPY FROM DATABASE __ducklake_metadata_lake TO backup`); err != nil {
		t.Fatalf("copy catalog backup: %v", err)
	}
	if st, err := os.Stat(backup); err != nil || st.Size() == 0 {
		t.Fatalf("backup missing or empty: %v", err)
	}
}

func TestPhase1_QueryExecuteSerializeTypes(t *testing.T) {
	db, _ := openTestLake(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE types (
			i INTEGER,
			d DECIMAL(10,2),
			ts TIMESTAMP,
			txt VARCHAR,
			js JSON,
			lst INTEGER[]
		)`); err != nil {
		t.Fatalf("create types: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO types VALUES (
			1,
			12.30,
			TIMESTAMP '2026-09-14 01:02:03',
			'hello',
			'{"a":1}'::JSON,
			[1, 2, 3]
		)`); err != nil {
		t.Fatalf("insert types: %v", err)
	}

	res, err := database.Query(ctx, db, database.Statement{SQL: `SELECT i, d, ts, txt, js, lst FROM types`}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("rows = %d", len(res.Rows))
	}
	out, err := database.SerializeRow(res.Rows[0])
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if len(out) != 6 {
		t.Fatalf("serialized cols = %d", len(out))
	}
}

func TestPhase1_CommitMessageAndSnapshots(t *testing.T) {
	db, _ := openTestLake(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO t VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx,
		`CALL "lake".set_commit_message('tester', 'phase1 insert', extra_info => '{"request_id":"r1"}')`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("set_commit_message: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	snaps, err := ListSnapshots(ctx, db, DefaultLakeAlias)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) == 0 {
		t.Fatal("expected snapshots")
	}
}

func countParquet(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".parquet" {
			n++
		}
		return nil
	})
	return n
}

