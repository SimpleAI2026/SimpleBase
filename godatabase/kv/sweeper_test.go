package kv

import (
	"context"
	"errors"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/godatabase/registry"
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

// fixedResolver 固定返回项目文件路径，替代宿主 catalog 的路径补全。
type fixedResolver struct{ path string }

func (r fixedResolver) ResolveDatabasePath(context.Context, string, string) (string, error) {
	return r.path, nil
}

// TestSweeperSweepOnce 走真实 registry + filedb 引擎验证端到端清扫。
func TestSweeperSweepOnce(t *testing.T) {
	ctx := context.Background()
	tenantID, projectID := uuid.NewString(), blob.NewProjectID()
	dbID := uuid.NewString()
	databasePath, err := godatabase.PathForProject(projectID, godatabase.EngineDuckDB)
	if err != nil {
		t.Fatal(err)
	}

	factory := &filedb.Factory{
		DataRoot: t.TempDir(),
		CacheDir: t.TempDir(),
		Syncer:   filedb.NewNopSyncer(),
	}
	reg := registry.New(factory, fixedResolver{path: databasePath}, registry.Options{Writable: true}, nil, nil)
	t.Cleanup(func() { _ = reg.Shutdown(context.Background()) })

	// 打开库并写入：一个永久 key + 一个已过期 key
	dbRow := godatabase.Spec{ID: dbID, TenantID: tenantID, ProjectID: projectID,
		Kind: godatabase.KindUser, Status: godatabase.StatusReady, DatabasePath: databasePath}
	lease, err := reg.Acquire(ctx, dbRow, godatabase.ReadWrite)
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
		_, err := tx.tx.ExecContext(ctx, `UPDATE __sb_kv_keys SET etime = ? WHERE "key" = 'gone'`, past)
		return err
	})
	lease.Release()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	sweeper := NewSweeper(reg, time.Hour, nil)
	got := sweeper.SweepOnce(ctx)
	if got[projectID] != 1 {
		t.Fatalf("sweep = %v, want %s:1", got, projectID)
	}
	// 第二轮：无过期 key
	got = sweeper.SweepOnce(ctx)
	if len(got) != 0 {
		t.Errorf("second sweep = %v, want empty", got)
	}
	// 数据校验
	lease2, err := reg.Acquire(ctx, dbRow, godatabase.ReadOnly)
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
