package kv

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/blob"

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

// TestHasExpired K2：只读探测准确反映是否有过期 key（Sweeper 据此决定取写租约）。
func TestHasExpired(t *testing.T) {
	base := testBaseTime()
	s, cur := openTestStoreAt(t, base)
	ctx := context.Background()

	update(t, s, func(tx *Tx) error {
		if err := tx.Str().Set(ctx, "k", []byte("v")); err != nil {
			return err
		}
		_, err := tx.Key().Expire(ctx, "k", 1000)
		return err
	})
	if has, err := s.HasExpired(ctx); err != nil || has {
		t.Fatalf("before expiry HasExpired = %v, %v; want false", has, err)
	}
	*cur += 2000
	if has, err := s.HasExpired(ctx); err != nil || !has {
		t.Fatalf("after expiry HasExpired = %v, %v; want true", has, err)
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

// K2：DeleteExpired 单轮上限——超过 maxExpiredPerSweep 的过期 key 分多轮删除。
func TestDeleteExpiredPerSweepCap(t *testing.T) {
	base := testBaseTime()
	s, cur := openTestStoreAt(t, base)
	ctx := context.Background()

	const total = maxExpiredPerSweep + 137
	update(t, s, func(tx *Tx) error {
		for i := 0; i < total; i++ {
			if err := tx.Str().Set(ctx, fmt.Sprintf("cap-%05d", i), []byte("v")); err != nil {
				return err
			}
		}
		for i := 0; i < total; i++ {
			if _, err := tx.Key().Expire(ctx, fmt.Sprintf("cap-%05d", i), 1000); err != nil {
				return err
			}
		}
		return nil
	})
	*cur += 2000 // 全部过期

	var rounds []int64
	for len(rounds) < 3 {
		var n int64
		update(t, s, func(tx *Tx) error {
			var err error
			n, err = tx.DeleteExpired(ctx)
			return err
		})
		rounds = append(rounds, n)
		if n == 0 {
			break
		}
	}
	if len(rounds) < 2 {
		t.Fatalf("expected cap to force multiple rounds, rounds = %v", rounds)
	}
	if rounds[0] != maxExpiredPerSweep {
		t.Fatalf("first round = %d, want %d", rounds[0], maxExpiredPerSweep)
	}
	var sum int64
	for _, n := range rounds {
		sum += n
	}
	if sum != total {
		t.Fatalf("total deleted = %d, want %d (rounds %v)", sum, total, rounds)
	}
	view(t, s, func(tx *Tx) error {
		raw, err := tx.Key().rawCount(ctx)
		if err != nil {
			return err
		}
		if raw != 0 {
			t.Errorf("raw rows = %d, want 0", raw)
		}
		return nil
	})
}
