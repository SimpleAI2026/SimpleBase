package filedb

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

func TestProjectFileRoundTrip(t *testing.T) {
	for _, engine := range []string{godatabase.EngineDuckDB, godatabase.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			dataRoot := t.TempDir()
			cacheDir := t.TempDir()
			databasePath, err := godatabase.PathForProject("prj00001", engine)
			if err != nil {
				t.Fatal(err)
			}
			meta := godatabase.Spec{ID: "db1", ProjectID: "prj00001", DatabasePath: databasePath, Status: godatabase.StatusReady}
			factory := &Factory{DataRoot: dataRoot, CacheDir: cacheDir, Syncer: NewNopSyncer()}
			ctx := context.Background()
			db, err := factory.Open(ctx, meta, godatabase.ReadWrite)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "CREATE TABLE items (id VARCHAR, value BIGINT)"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO items VALUES ('a', 42)"); err != nil {
				t.Fatal(err)
			}
			if _, err := factory.Open(ctx, meta, godatabase.ReadOnly); err == nil {
				t.Fatal("expected same-process double-open rejection")
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = factory.Open(ctx, meta, godatabase.ReadOnly)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var value int
			if err := db.QueryRowContext(ctx, "SELECT value FROM items WHERE id = 'a'").Scan(&value); err != nil {
				t.Fatal(err)
			}
			if value != 42 {
				t.Fatalf("value=%d", value)
			}
			if _, err := os.Stat(dataRoot + "/prj00001/database." + map[string]string{godatabase.EngineDuckDB: "duckdb", godatabase.EngineSQLite: "sqlite"}[engine]); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDatabasePathValidation(t *testing.T) {
	factory := &Factory{DataRoot: t.TempDir(), CacheDir: t.TempDir()}
	for _, databasePath := range []string{"", "/tmp/a.duckdb", "../a.duckdb", `..\a.duckdb`, "prj00001/other.duckdb", "prj00001/database.txt"} {
		_, err := factory.Open(context.Background(), godatabase.Spec{ProjectID: "prj00001", DatabasePath: databasePath}, godatabase.ReadWrite)
		if err == nil {
			t.Fatalf("expected path %q to fail", databasePath)
		}
	}
}

func TestSQLiteDialectMetadata(t *testing.T) {
	ctx := context.Background()
	databasePath, err := godatabase.PathForProject("prj00002", godatabase.EngineSQLite)
	if err != nil {
		t.Fatal(err)
	}
	factory := &Factory{DataRoot: t.TempDir(), CacheDir: t.TempDir(), Syncer: NewNopSyncer()}
	db, err := factory.Open(ctx, godatabase.Spec{ID: "db2", ProjectID: "prj00002", DatabasePath: databasePath}, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		"CREATE TABLE users (id INTEGER NOT NULL, email TEXT)",
		"CREATE TABLE __sb_kv_keys (k TEXT)",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	dialect, err := godatabase.DialectFor(godatabase.EngineSQLite)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, dialect.TableColumnsSQL(""))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]int{}
	for rows.Next() {
		var table, column, typ, nullable string
		if err := rows.Scan(&table, &column, &typ, &nullable); err != nil {
			t.Fatal(err)
		}
		seen[table]++
	}
	if seen["users"] != 2 {
		t.Fatalf("users columns=%d", seen["users"])
	}
	if seen["__sb_kv_keys"] != 0 {
		t.Fatal("internal table leaked into dialect metadata")
	}
}

func TestOpenCorruptFileErrorHidesPhysicalPath(t *testing.T) {
	for _, engine := range []string{godatabase.EngineDuckDB, godatabase.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			dataRoot := t.TempDir()
			cacheDir := t.TempDir()
			databasePath, err := godatabase.PathForProject("prj00002", engine)
			if err != nil {
				t.Fatal(err)
			}
			meta := godatabase.Spec{ID: "db2", ProjectID: "prj00002", DatabasePath: databasePath, Status: godatabase.StatusReady}
			factory := &Factory{DataRoot: dataRoot, CacheDir: cacheDir, Syncer: NewNopSyncer()}
			full, err := factory.filePath(meta)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte("this is not a database file, just garbage bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = factory.Open(context.Background(), meta, godatabase.ReadWrite)
			if err == nil {
				t.Fatal("expected open of corrupt file to fail")
			}
			for _, leak := range []string{dataRoot, cacheDir, databasePath} {
				if strings.Contains(err.Error(), leak) {
					t.Fatalf("error leaks %q: %v", leak, err)
				}
			}
		})
	}
}
