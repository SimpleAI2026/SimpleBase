package kv

import (
	"context"
	"testing"
)

// K3：批量路径边界用例——重复 key、空入参。
func TestBatchOpsEdgeCases(t *testing.T) {
	base := testBaseTime()
	s, cur := openTestStoreAt(t, base)
	ctx := context.Background()

	update(t, s, func(tx *Tx) error {
		if err := tx.Str().Set(ctx, "dup", []byte("v")); err != nil {
			return err
		}
		if _, err := tx.List().PushBack(ctx, "lst", []byte("a"), []byte("b")); err != nil {
			return err
		}
		return nil
	})

	// Exists：重复 key 各计一次（Redis 语义）。
	view(t, s, func(tx *Tx) error {
		n, err := tx.Key().Exists(ctx, "dup", "dup", "missing")
		if err != nil {
			return err
		}
		if n != 2 {
			t.Errorf("Exists with dup = %d, want 2", n)
		}
		// 空入参。
		if n, err := tx.Key().Exists(ctx); err != nil || n != 0 {
			t.Errorf("Exists empty = %d, %v", n, err)
		}
		return nil
	})

	// Delete：重复 key 只删一次、计数一次；过期 key 不计数。
	update(t, s, func(tx *Tx) error {
		ok, err := tx.Key().Expire(ctx, "lst", 1000)
		if err != nil || !ok {
			t.Fatalf("expire = %v, %v", ok, err)
		}
		return nil
	})
	*cur += 2000 // lst 过期
	update(t, s, func(tx *Tx) error {
		n, err := tx.Key().Delete(ctx, "dup", "dup", "lst")
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("Delete with dup + expired = %d, want 1", n)
		}
		if n, err := tx.Key().Delete(ctx); err != nil || n != 0 {
			t.Errorf("Delete empty = %d, %v", n, err)
		}
		return nil
	})

	// List push 空入参报 ErrArgument；多元素一次插入后顺序正确。
	update(t, s, func(tx *Tx) error {
		if _, err := tx.List().PushBack(ctx, "l2"); err == nil {
			t.Error("PushBack empty should fail")
		} else if err != ErrArgument {
			t.Errorf("PushBack empty err = %v, want ErrArgument", err)
		}
		if _, err := tx.List().PushBack(ctx, "l2", []byte("x"), []byte("y"), []byte("z")); err != nil {
			return err
		}
		return nil
	})
	view(t, s, func(tx *Tx) error {
		elems, err := tx.List().Range(ctx, "l2", 0, -1)
		if err != nil {
			return err
		}
		if len(elems) != 3 || string(elems[0]) != "x" || string(elems[2]) != "z" {
			t.Errorf("batched push order = %q", elems)
		}
		return nil
	})
}
