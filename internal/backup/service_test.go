package backup

import (
	"context"
	"database/sql"
	"errors"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	_ "github.com/uglyer/go-sqlite3"
)

type testCatalog struct{ meta catalog.Database }

func (c testCatalog) ResolveProjectStorage(context.Context, string, string) (catalog.Database, error) {
	return c.meta, nil
}

func TestBackupRestoreAndIntegrity(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			ctx := context.Background()
			path, err := catalog.DatabasePathForProject("prj00001", engine)
			if err != nil {
				t.Fatal(err)
			}
			meta := catalog.Database{ID: "db1", ProjectID: "prj00001", TenantID: "tenant1", DatabasePath: path, Status: catalog.DatabaseReady}
			factory := &filedb.Factory{DataRoot: t.TempDir(), CacheDir: t.TempDir(), Syncer: filedb.NewNopSyncer()}
			reg := registry.New(factory, nil, registry.Options{Writable: true}, nil, nil)
			t.Cleanup(func() { _ = reg.Shutdown(context.Background()) })
			system, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			system.SetMaxOpenConns(1)
			defer system.Close()
			_, err = system.Exec(`CREATE TABLE sys_backups(id VARCHAR, project_id VARCHAR, storage_key VARCHAR, size_bytes BIGINT, sha256 VARCHAR, engine VARCHAR, format_version BIGINT, status VARCHAR, error VARCHAR, created_at TIMESTAMP, expires_at TIMESTAMP)`)
			if err != nil {
				t.Fatal(err)
			}
			repo := NewRepository(system, nil)
			cfg := config.BackupConfig{Enabled: true, Dir: t.TempDir(), KeepLast: 7, KeepDays: 30, MaxPerProject: 14, SnapshotTimeout: time.Minute}
			service := NewService(cfg, "test", testCatalog{meta}, reg, factory, repo, nil, objectstoreKeys())
			execute := func(statement string) {
				lease, err := reg.Acquire(ctx, dbbridge.SpecOf(meta), godatabase.ReadWrite)
				if err != nil {
					t.Fatal(err)
				}
				_, err = lease.Handle.Execute(ctx, godatabase.Statement{SQL: statement})
				lease.Release()
				if err != nil {
					t.Fatal(err)
				}
			}
			count := func() int {
				lease, err := reg.Acquire(ctx, dbbridge.SpecOf(meta), godatabase.ReadOnly)
				if err != nil {
					t.Fatal(err)
				}
				result, err := lease.Handle.Query(ctx, godatabase.Statement{SQL: "SELECT COUNT(*) FROM items"}, 1)
				lease.Release()
				if err != nil {
					t.Fatal(err)
				}
				return int(result.Rows[0][0].(int64))
			}
			execute("CREATE TABLE items(id VARCHAR)")
			execute("INSERT INTO items VALUES('before')")
			item, err := service.Create(ctx, "tenant1", "prj00001")
			if err != nil {
				t.Fatalf("create backup: %v", err)
			}
			if item.SizeBytes == 0 || item.SHA256 == "" {
				t.Fatalf("invalid backup metadata: %+v", item)
			}
			execute("INSERT INTO items VALUES('after')")
			if count() != 2 {
				t.Fatal("seed did not commit")
			}
			if err := service.Restore(ctx, "tenant1", "prj00001", item.ID); err != nil {
				t.Fatalf("restore: %v", err)
			}
			if count() != 1 {
				t.Fatalf("restored count=%d, want 1", count())
			}
			if _, err := system.Exec(`UPDATE sys_backups SET sha256 = 'broken' WHERE id = ?`, item.ID); err != nil {
				t.Fatal(err)
			}
			if err := service.Restore(ctx, "tenant1", "prj00001", item.ID); !errors.Is(err, ErrSnapshotMismatch) {
				t.Fatalf("corrupt hash restore: %v", err)
			}
			if count() != 1 {
				t.Fatal("failed restore changed current database")
			}
		})
	}
}

func objectstoreKeys() objectstore.KeyBuilder { return objectstore.KeyBuilder{} }
