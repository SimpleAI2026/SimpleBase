// 文件说明：bugfix_test.go — godatabase/kv 的单元测试（TestUpdatePanicRollsBackTx、TestDeleteExpiredVictimSetStable 等）
//
// 用法
//
//	go test ./godatabase/kv -run 'TestUpdatePanicRollsBackTx|TestDeleteExpiredVictimSetStable|TestGetManyDuplicateInputs|TestSetDiffSameKey'
//
// 调用架构
//
//	[上游] godatabase/cmd, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/kv/bugfix_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] collections_test.go, conn_schema_test.go, core.go, executor.go, export_test.go 等 20 个
//
// 主要符号
//
//	TestUpdatePanicRollsBackTx(t *testing.T)
//	TestDeleteExpiredVictimSetStable(t *testing.T)
//	TestGetManyDuplicateInputs(t *testing.T)
//	TestSetDiffSameKey(t *testing.T)
//	TestHashDeleteDuplicateArgs(t *testing.T)

package kv

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestUpdatePanicRollsBackTx B16：fn panic 时事务必须回滚、panic 继续传播，
// 后续 Update 可正常前进（未提交写入不得残留）。
func TestUpdatePanicRollsBackTx(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	update(t, store, func(tx *Tx) error {
		return tx.Str().Set(ctx, "before", []byte("1"))
	})

	func() {
		defer func() {
			if p := recover(); p == nil {
				t.Fatal("panic must propagate")
			}
		}()
		_ = store.Update(ctx, func(tx *Tx) error {
			if err := tx.Str().Set(ctx, "dirty", []byte("x")); err != nil {
				return err
			}
			panic("boom")
		})
	}()

	// 未提交的写入必须已回滚，已提交数据存活，连接可继续使用。
	view(t, store, func(tx *Tx) error {
		if _, err := tx.Str().Get(ctx, "dirty"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("dirty write must be rolled back, got err=%v", err)
		}
		if v, err := tx.Str().Get(ctx, "before"); err != nil || string(v) != "1" {
			t.Fatalf("committed data must survive, v=%q err=%v", v, err)
		}
		return nil
	})
	update(t, store, func(tx *Tx) error {
		return tx.Str().Set(ctx, "after", []byte("2"))
	})
}

// TestDeleteExpiredVictimSetStable B17：victim 集合一次扫描固定；两类数据
// （string / hash）跨过 TTL 边界时，元数据与数据行的删除集合必须严格一致。
func TestDeleteExpiredVictimSetStable(t *testing.T) {
	base := time.UnixMilli(1_000_000)
	store, cur := openTestStoreAt(t, base)
	ctx := context.Background()

	past := *cur - 10_000
	future := *cur + 10_000
	update(t, store, func(tx *Tx) error {
		// 直接构造过期/将过期的元数据（SetWith 会拒绝非正 TTL）。
		for _, k := range []string{"s1", "s2"} {
			if err := tx.Key().insertKey(ctx, k, TypeString, &past, nil); err != nil {
				return err
			}
			if _, err := tx.tx.ExecContext(ctx,
				`INSERT INTO __sb_kv_strings ("key", value) VALUES (?, 'v')`, k); err != nil {
				return err
			}
		}
		l := int64(1)
		if err := tx.Key().insertKey(ctx, "h1", TypeHash, &future, &l); err != nil {
			return err
		}
		_, err := tx.tx.ExecContext(ctx,
			`INSERT INTO __sb_kv_hashes ("key", field, value) VALUES ('h1', 'f', X'01')`)
		return err
	})

	// 推进时钟使 hash 也过期后清扫：两类数据行与元数据必须一起删除。
	*cur += 20_000
	var deleted int64
	update(t, store, func(tx *Tx) error {
		var err error
		deleted, err = tx.DeleteExpired(ctx)
		return err
	})
	if deleted != 3 {
		t.Fatalf("expected 3 victims deleted, got %d", deleted)
	}
	view(t, store, func(tx *Tx) error {
		for table, want := range map[string]string{
			"__sb_kv_keys":    "expired meta rows",
			"__sb_kv_hashes":  "hash data rows",
			"__sb_kv_strings": "string data rows",
		} {
			var n int
			if err := tx.tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				t.Fatalf("%s must be deleted together, got %d", want, n)
			}
		}
		return nil
	})
}

// TestGetManyDuplicateInputs B18：MGET/HMGET 重复输入必须所有位置返回
// 同一结果（对齐 Redis），而不是只有首次出现的位置。
func TestGetManyDuplicateInputs(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	update(t, store, func(tx *Tx) error {
		if err := tx.Str().Set(ctx, "a", []byte("va")); err != nil {
			return err
		}
		_, err := tx.Hash().Set(ctx, "h", map[string][]byte{"f": []byte("vf")})
		return err
	})
	view(t, store, func(tx *Tx) error {
		values, found, err := tx.Str().GetMany(ctx, "a", "a", "missing", "a")
		if err != nil {
			return err
		}
		wantV := [][]byte{[]byte("va"), []byte("va"), nil, []byte("va")}
		wantF := []bool{true, true, false, true}
		for i := range values {
			if string(values[i]) != string(wantV[i]) || found[i] != wantF[i] {
				t.Fatalf("MGET dup at %d: got %q/%v want %q/%v", i, values[i], found[i], wantV[i], wantF[i])
			}
		}
		hv, hf, err := tx.Hash().GetMany(ctx, "h", "f", "f", "nof")
		if err != nil {
			return err
		}
		for i, want := range []struct {
			v string
			f bool
		}{{"vf", true}, {"vf", true}, {"", false}} {
			if string(hv[i]) != want.v || hf[i] != want.f {
				t.Fatalf("HMGET dup at %d: got %q/%v want %q/%v", i, hv[i], hf[i], want.v, want.f)
			}
		}
		return nil
	})
}

// TestSetDiffSameKey B19：SDIFF a a 必须为空。
func TestSetDiffSameKey(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	update(t, store, func(tx *Tx) error {
		_, err := tx.Set().Add(ctx, "a", []byte("x"), []byte("y"))
		return err
	})
	view(t, store, func(tx *Tx) error {
		got, err := tx.Set().Diff(ctx, "a", "a")
		if err != nil {
			return err
		}
		if len(got) != 0 {
			t.Fatalf("SDIFF a a must be empty, got %v", got)
		}
		got, err = tx.Set().Diff(ctx, "a", "b", "a")
		if err != nil {
			return err
		}
		if len(got) != 0 {
			t.Fatalf("SDIFF a b a must be empty, got %v", got)
		}
		got, err = tx.Set().Diff(ctx, "a")
		if err != nil {
			return err
		}
		if len(got) != 2 {
			t.Fatalf("SDIFF a must return both members, got %v", got)
		}
		return nil
	})
}

// TestHashDeleteDuplicateArgs：K-01 收敛后重复参数按实际 RowsAffected 计数，
// 零删除不更新 version/mtime。
func TestHashDeleteDuplicateArgs(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	update(t, store, func(tx *Tx) error {
		_, err := tx.Hash().Set(ctx, "h", map[string][]byte{"f1": []byte("1"), "f2": []byte("2")})
		return err
	})
	view(t, store, func(tx *Tx) error {
		before, err := tx.Key().Get(ctx, "h")
		if err != nil {
			return err
		}
		n, err := tx.Hash().Delete(ctx, "h", "missing", "missing")
		if err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("missing fields must delete 0, got %d", n)
		}
		after, err := tx.Key().Get(ctx, "h")
		if err != nil {
			return err
		}
		if after.Version != before.Version || after.Mtime != before.Mtime {
			t.Fatalf("zero-delete must not touch meta: before=%+v after=%+v", before, after)
		}
		return nil
	})
	// 重复 field 只删一次。
	view(t, store, func(tx *Tx) error {
		n, err := tx.Hash().Delete(ctx, "h", "f1", "f1")
		if err != nil {
			return err
		}
		if n != 1 {
			t.Fatalf("duplicate field args must count actual deletions, got %d", n)
		}
		m, err := tx.Hash().Items(ctx, "h")
		if err != nil {
			return err
		}
		if len(m) != 1 {
			t.Fatalf("expected 1 field left, got %d", len(m))
		}
		return nil
	})
}
