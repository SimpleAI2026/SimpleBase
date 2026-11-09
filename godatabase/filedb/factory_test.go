package filedb

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

// toSlash 将本地路径转为 SQL 字符串里使用的正斜杠形式。
func toSlash(p string) string { return filepath.ToSlash(p) }

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

// —— S2：DuckDB 用户库关闭外部文件访问（planv6.4 §1）——

func TestDuckDBUserDBRejectsExternalFileRead(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	cacheDir := t.TempDir()
	databasePath, err := godatabase.PathForProject("prj00003", godatabase.EngineDuckDB)
	if err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dataRoot, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	factory := &Factory{DataRoot: dataRoot, CacheDir: cacheDir, Syncer: NewNopSyncer()}
	meta := godatabase.Spec{ID: "db3", ProjectID: "prj00003", DatabasePath: databasePath, Status: godatabase.StatusReady}
	db, err := factory.Open(ctx, meta, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, sql := range []string{
		"SELECT * FROM read_text('" + toSlash(secret) + "')",
		"SELECT * FROM read_csv('" + toSlash(secret) + "')",
		"SELECT * FROM glob('" + toSlash(dataRoot) + "/*')",
	} {
		if _, err := db.ExecContext(ctx, sql); err == nil {
			t.Errorf("external file read must fail: %s", sql)
		}
	}

	// 项目 tmp 目录（allowed_directories 白名单）内的文件仍可读。
	tmpFile := filepath.Join(cacheDir, "projects", "prj00003", "tmp", "ok.csv")
	if err := os.WriteFile(tmpFile, []byte("a,b\n1,2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "SELECT * FROM read_csv('"+toSlash(tmpFile)+"')"); err != nil {
		t.Errorf("whitelisted tmp dir read must succeed: %v", err)
	}

	// 正常读写不受影响；溢写（temp_directory 已提前设置）可用。
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (x INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO t VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	var v int
	if err := db.QueryRowContext(ctx, "SELECT x FROM t").Scan(&v); err != nil || v != 1 {
		t.Fatalf("normal write/read broken: v=%d err=%v", v, err)
	}
}

// —— S1 纵深防御：SQLite 只读租约强制 query_only ——
// （CTE DELETE 绕过 sqlguard 的场景在这里被第二层拦截。）

func TestSQLiteReadOnlyConnectorForcesQueryOnly(t *testing.T) {
	ctx := context.Background()
	databasePath, err := godatabase.PathForProject("prj00004", godatabase.EngineSQLite)
	if err != nil {
		t.Fatal(err)
	}
	factory := &Factory{DataRoot: t.TempDir(), CacheDir: t.TempDir(), Syncer: NewNopSyncer()}
	meta := godatabase.Spec{ID: "db4", ProjectID: "prj00004", DatabasePath: databasePath, Status: godatabase.StatusReady}

	rw, err := factory.Open(ctx, meta, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rw.ExecContext(ctx, "CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := rw.ExecContext(ctx, "INSERT INTO t VALUES (1), (2), (3)"); err != nil {
		t.Fatal(err)
	}
	if err := rw.Close(); err != nil {
		t.Fatal(err)
	}

	ro, err := factory.Open(ctx, meta, godatabase.ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()

	// 绕过 sqlguard 的写语句（守卫第一层已拦；这里验证第二层）：
	// CTE DELETE / 普通 DELETE / INSERT 在 query_only 连接上必须失败。
	for _, sql := range []string{
		"WITH x AS (SELECT 1) DELETE FROM t",
		"DELETE FROM t",
		"INSERT INTO t VALUES (4)",
		"UPDATE t SET id = 9",
	} {
		if _, err := ro.ExecContext(ctx, sql); err == nil {
			t.Errorf("write must fail on read-only connector: %s", sql)
		}
	}
	// 只读查询正常。
	var n int
	if err := ro.QueryRowContext(ctx, "SELECT count(*) FROM t").Scan(&n); err != nil || n != 3 {
		t.Fatalf("read broken: n=%d err=%v", n, err)
	}
}
