package app

import (
	"context"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

type fakeCollectionCleaner struct {
	items   []collection.Collection
	deleted []string
}

func (f *fakeCollectionCleaner) List(context.Context, string) ([]collection.Collection, error) {
	return f.items, nil
}

func (f *fakeCollectionCleaner) Delete(_ context.Context, _ string, name string) error {
	f.deleted = append(f.deleted, name)
	return nil
}

// 删库只清空用户对象：KV 内部表与项目文件保留，集合元数据被清理（BUG-2）。
func TestProjectPurgerClearsUserDataKeepsKV(t *testing.T) {
	for _, engine := range []string{godatabase.EngineDuckDB, godatabase.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			factory := &filedb.Factory{DataRoot: dir, CacheDir: t.TempDir(), Syncer: filedb.NewNopSyncer()}
			reg := registry.New(factory, nil, registry.Options{Writable: true}, nil, nil)
			t.Cleanup(func() { _ = reg.Shutdown(context.Background()) })
			path, err := godatabase.PathForProject("proj0001", engine)
			if err != nil {
				t.Fatal(err)
			}
			spec := godatabase.Spec{ID: "db-1", ProjectID: "proj0001", Status: godatabase.StatusReady, DatabasePath: path}
			lease, err := reg.Acquire(ctx, spec, godatabase.ReadWrite)
			if err != nil {
				t.Fatal(err)
			}
			for _, stmt := range []string{
				"CREATE TABLE notes (id VARCHAR, data VARCHAR, created_at VARCHAR, updated_at VARCHAR)",
				"INSERT INTO notes VALUES ('a', '{}', 't', NULL)",
				"CREATE TABLE _migrations (name VARCHAR, applied_at VARCHAR)",
				"CREATE VIEW notes_v AS SELECT id FROM notes",
				"CREATE TABLE __sb_kv_strings (\"key\" VARCHAR, val VARCHAR)",
				"INSERT INTO __sb_kv_strings VALUES ('k', 'v')",
			} {
				if _, err := lease.Handle.Execute(ctx, godatabase.Statement{SQL: stmt}); err != nil {
					t.Fatalf("%s: %v", stmt, err)
				}
			}
			lease.Release()

			schemas := &fakeCollectionCleaner{items: []collection.Collection{{Name: "notes"}}}
			purger := newProjectPurger(factory, nil, reg, schemas)
			clearer, ok := purger.(catalog.ProjectDataClearer)
			if !ok {
				t.Fatal("purger with registry must implement ProjectDataClearer")
			}
			if err := clearer.ClearProjectData(ctx, catalog.Database{ID: "db-1", ProjectID: "proj0001", DatabasePath: path}); err != nil {
				t.Fatalf("ClearProjectData: %v", err)
			}

			lease, err = reg.Acquire(ctx, spec, godatabase.ReadOnly)
			if err != nil {
				t.Fatalf("project file must stay usable after delete: %v", err)
			}
			defer lease.Release()
			objs, err := listUserObjects(ctx, lease.Handle, engine)
			if err != nil {
				t.Fatal(err)
			}
			if len(objs) != 0 {
				t.Fatalf("user objects should be dropped, left %v", objs)
			}
			res, err := lease.Handle.Query(ctx, godatabase.Statement{SQL: "SELECT val FROM __sb_kv_strings WHERE \"key\" = 'k'"}, 1)
			if err != nil || len(res.Rows) != 1 || res.Rows[0][0] != "v" {
				t.Fatalf("kv must survive database delete: rows=%v err=%v", res.Rows, err)
			}
			if len(schemas.deleted) != 1 || schemas.deleted[0] != "notes" {
				t.Fatalf("collection metadata should be cleared: %v", schemas.deleted)
			}
		})
	}
}

// 没有 registry 时只能整体删文件（旧行为），不得声称支持清空数据。
func TestProjectPurgerWithoutRegistryFallsBackToFilePurge(t *testing.T) {
	factory := &filedb.Factory{DataRoot: t.TempDir(), CacheDir: t.TempDir(), Syncer: filedb.NewNopSyncer()}
	purger := newProjectPurger(factory, nil, nil, nil)
	if _, ok := purger.(catalog.ProjectDataClearer); ok {
		t.Fatal("purger without registry must not implement ProjectDataClearer")
	}
	if _, ok := purger.(catalog.ProjectFilePurger); !ok {
		t.Fatal("purger without registry must still purge project files")
	}
}
