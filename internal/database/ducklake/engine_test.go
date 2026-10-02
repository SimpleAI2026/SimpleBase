package ducklake

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

// engine_test.go 覆盖 ducklake-duckdb-catalog-plan §3/§4.4/§7-1：
// 按引擎的本地布局与 ATTACH、另一引擎残留拒绝打开、同进程重复打开守卫。

func TestBuildBootSQLAttachPerEngine(t *testing.T) {
	layout := layoutForEngine("/c", "id", EngineDuckDB)
	boot := strings.Join(buildBootSQL(layout, Options{CatalogEngine: EngineDuckDB}.normalized(), RemoteStorage{}, "/d/"), "\n")
	if !strings.Contains(boot, "ATTACH 'ducklake:/c/id/catalog/catalog.ducklake'") || strings.Contains(boot, "sqlite") {
		t.Fatalf("duckdb boot:\n%s", boot)
	}
	layout = layoutForEngine("/c", "id", EngineSQLite)
	boot = strings.Join(buildBootSQL(layout, Options{CatalogEngine: EngineSQLite}.normalized(), RemoteStorage{}, "/d/"), "\n")
	if !strings.Contains(boot, "ATTACH 'ducklake:sqlite:/c/id/catalog/catalog.sqlite'") || !strings.Contains(boot, "LOAD sqlite") {
		t.Fatalf("sqlite boot:\n%s", boot)
	}
}

func TestOpenRejectsForeignEngineFile(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	foreign := layoutForEngine(dir, id, EngineSQLite)
	if err := os.MkdirAll(filepath.Dir(foreign.CatalogFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign.CatalogFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &Factory{CacheDir: dir, Options: DefaultOptions()}
	if _, err := f.Open(context.Background(), catalog.Database{ID: id}, database.ReadWrite); !IsCatalogEngineMismatch(err) {
		t.Fatalf("want engine mismatch, got %v", err)
	}
}

func TestOpenRejectsInvalidEngine(t *testing.T) {
	opts := DefaultOptions()
	opts.CatalogEngine = "postgres"
	f := &Factory{CacheDir: t.TempDir(), Options: opts}
	if _, err := f.Open(context.Background(), catalog.Database{ID: uuid.NewString()}, database.ReadWrite); err == nil {
		t.Fatal("invalid engine must fail")
	}
}

func TestOpenGuardsSameProcessDoubleOpen(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	opts := DefaultOptions()
	opts.ExtensionDir = filepath.Join(dir, "extensions")
	f1 := &Factory{CacheDir: dir, Options: opts}
	f2 := &Factory{CacheDir: dir, Options: opts} // 不同 Factory 实例也必须互斥
	db, err := f1.Open(context.Background(), catalog.Database{ID: id}, database.ReadWrite)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f2.Open(context.Background(), catalog.Database{ID: id}, database.ReadWrite); err == nil {
		t.Fatal("second open of the same catalog must be rejected")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db2, err := f2.Open(context.Background(), catalog.Database{ID: id}, database.ReadWrite)
	if err != nil {
		t.Fatalf("reopen after close: %v", err)
	}
	_ = db2.Close()
}

// TestSQLiteEngineSyncAndColdStart：sqlite 引擎全链路（写 → sync → 删本地 → 冷启动恢复）。
func TestEngineSyncAndColdStart(t *testing.T) {
	for _, engine := range []string{EngineDuckDB, EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			meta := catalog.Database{ID: uuid.NewString(), TenantID: "11111111-1111-1111-1111-111111111111"}
			blobs := objectstore.NewMemoryBlobStore()
			remote := testRemote()
			opts := DefaultOptions()
			opts.CatalogEngine = engine
			opts.ExtensionDir = filepath.Join(dir, "extensions")
			cs := NewCatalogSyncer(blobs, remote, dir, opts.CatalogSync, engine, nil, nil)
			f := &Factory{CacheDir: dir, Options: opts, Syncer: cs}
			db, err := f.Open(ctx, meta, database.ReadWrite)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if _, err := db.ExecContext(ctx, `CREATE TABLE t (x INTEGER)`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO t VALUES (42)`); err != nil {
				t.Fatal(err)
			}
			snap, _ := CurrentSnapshot(ctx, db, DefaultLakeAlias)
			cs.MarkDirty(meta.ID, snap)
			if err := cs.Flush(ctx, meta.ID); err != nil {
				t.Fatalf("flush: %v", err)
			}
			latest, err := ReadLatestManifest(ctx, blobs, remote, meta.TenantID, meta.ID, 0)
			if err != nil || latest == nil || latest.CatalogEngine != engine {
				t.Fatalf("manifest: %+v %v", latest, err)
			}
			_ = cs.Unbind(ctx, meta.ID)
			_ = db.Close()

			// 冷启动：删除本地 catalog 与水位（模拟缓存淘汰），从远端恢复。
			// DATA_PATH 仍在本地同一目录（单测不走 S3），恢复后数据应完整可读。
			layout := layoutForEngine(dir, meta.ID, engine)
			_ = os.Remove(layout.CatalogFile)
			_ = os.Remove(layout.WALFile(engine))
			_ = os.Remove(localStatePath(layout))
			if ok, err := EnsureLocalCatalog(ctx, blobs, remote, dir, meta, engine); err != nil || !ok {
				t.Fatalf("cold download: ok=%v err=%v", ok, err)
			}
			f2 := &Factory{CacheDir: dir, Options: opts}
			db2, err := f2.Open(ctx, meta, database.ReadWrite)
			if err != nil {
				t.Fatalf("cold open: %v", err)
			}
			defer db2.Close()
			var n int
			if err := db2.QueryRowContext(ctx, `SELECT x FROM t`).Scan(&n); err != nil || n != 42 {
				t.Fatalf("query after cold start: n=%d err=%v", n, err)
			}
		})
	}
}
