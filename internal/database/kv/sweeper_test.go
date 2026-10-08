package kv

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
	"github.com/linkxzhou/SimpleBase/internal/database/registry"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func TestDeleteExpired(t *testing.T) {
	base := testBaseTime()
	s, cur := openTestStoreAt(t, base)
	ctx := context.Background()

	update(t, s, func(tx *Tx) error {
		if err := tx.Str().Set(ctx, "permanent", []byte("v")); err != nil {
			return err
		}
		if err := tx.Str().Set(ctx, "expiring", []byte("v")); err != nil {
			return err
		}
		ok, err := tx.Key().Expire(ctx, "expiring", 1000)
		if err != nil || !ok {
			t.Fatalf("expire = %v, %v", ok, err)
		}
		if _, err := tx.Hash().Set(ctx, "expiring-h", map[string][]byte{"f": []byte("v")}); err != nil {
			return err
		}
		ok, err = tx.Key().Expire(ctx, "expiring-h", 1000)
		if err != nil || !ok {
			t.Fatalf("expire hash = %v, %v", ok, err)
		}
		return nil
	})
	*cur += 2000 // 两个 key 过期

	var n int64
	update(t, s, func(tx *Tx) error {
		var err error
		n, err = tx.DeleteExpired(ctx)
		return err
	})
	if n != 2 {
		t.Fatalf("DeleteExpired = %d, want 2", n)
	}
	view(t, s, func(tx *Tx) error {
		raw, err := tx.Key().rawCount(ctx)
		if err != nil {
			return err
		}
		if raw != 1 {
			t.Errorf("raw rows = %d, want 1 (permanent only)", raw)
		}
		return nil
	})
	// 空跑不再产生写
	update(t, s, func(tx *Tx) error {
		var err error
		n, err = tx.DeleteExpired(ctx)
		return err
	})
	if n != 0 {
		t.Errorf("second sweep = %d, want 0", n)
	}
}

// TestSweeperSweepOnce 走真实 registry + ducklake 引擎验证端到端清扫。
func TestSweeperSweepOnce(t *testing.T) {
	ctx := context.Background()

	// 系统 catalog（内存 sqlite）
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	for _, stmt := range []string{
		`CREATE TABLE sys_tenants (id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE sys_projects (id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE sys_databases (
			id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, name VARCHAR NOT NULL,
			kind VARCHAR NOT NULL, status VARCHAR NOT NULL, storage_prefix VARCHAR NOT NULL, format_version BIGINT NOT NULL,
			deleted_at TIMESTAMP, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
			data_model VARCHAR NOT NULL DEFAULT 'collection')`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	repo := catalog.NewSQLRepository(raw)
	keys := objectstore.KeyBuilder{RootPrefix: "simplebase", Environment: "test"}
	svc := catalog.NewService(repo, keys, nil, objectstore.DuckLakeStorage{}, nil)
	tenantID, projectID := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	if err := repo.CreateTenant(ctx, catalog.Tenant{ID: tenantID, Name: "t", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateProject(ctx, catalog.Project{ID: projectID, TenantID: tenantID, Name: "p", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	dbID := uuid.NewString()
	if err := repo.CreateDatabase(ctx, catalog.Database{
		ID: dbID, TenantID: tenantID, ProjectID: projectID, Name: catalog.KVDatabaseName,
		Kind: catalog.DatabaseKindKV, Status: catalog.DatabaseReady,
		StoragePrefix: "p", FormatVersion: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	factory := &ducklake.Factory{
		CacheDir: t.TempDir(),
		Options:  ducklake.DefaultOptions(),
		Syncer:   ducklake.NewLocalSyncer(),
	}
	reg := registry.New(factory, svc, registry.Options{Writable: true}, nil, nil)
	t.Cleanup(func() { _ = reg.Shutdown(context.Background()) })

	// 打开库并写入：一个永久 key + 一个已过期 key
	dbRow := catalog.Database{ID: dbID, TenantID: tenantID, ProjectID: projectID, Name: catalog.KVDatabaseName,
		Kind: catalog.DatabaseKindKV, Status: catalog.DatabaseReady, StoragePrefix: "p", FormatVersion: 1}
	lease, err := reg.Acquire(ctx, dbRow, database.ReadWrite)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	past := time.Now().Add(-time.Second).UnixMilli()
	store := New(lease.Handle.Conn())
	err = store.Update(ctx, func(tx *Tx) error {
		if err := tx.Str().Set(ctx, "stay", []byte("v")); err != nil {
			return err
		}
		if err := tx.Str().Set(ctx, "gone", []byte("v")); err != nil {
			return err
		}
		// 直接写入已过期的 etime
		_, err := tx.tx.ExecContext(ctx, `UPDATE kv.keys SET etime = ? WHERE "key" = 'gone'`, past)
		return err
	})
	lease.Release()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	sweeper := NewSweeper(reg, time.Hour, nil)
	got := sweeper.SweepOnce(ctx)
	if got[dbID] != 1 {
		t.Fatalf("sweep = %v, want %s:1", got, dbID)
	}
	// 第二轮：无过期 key
	got = sweeper.SweepOnce(ctx)
	if len(got) != 0 {
		t.Errorf("second sweep = %v, want empty", got)
	}
	// 数据校验
	lease2, err := reg.Acquire(ctx, dbRow, database.ReadOnly)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	defer lease2.Release()
	store2 := New(lease2.Handle.Conn())
	_ = store2.View(ctx, func(tx *Tx) error {
		if _, err := tx.Str().Get(ctx, "stay"); err != nil {
			t.Errorf("stay should survive: %v", err)
		}
		if _, err := tx.Str().Get(ctx, "gone"); !errors.Is(err, ErrNotFound) {
			t.Errorf("gone should be swept: %v", err)
		}
		return nil
	})
}
