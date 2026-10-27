package kv

import (
	"context"
	"errors"
	"testing"
)

func TestStringSetGet(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k1", []byte("v1"))
	})
	view(t, s, func(tx *Tx) error {
		v, err := tx.Str().Get(ctx, "k1")
		if err != nil {
			return err
		}
		if string(v) != "v1" {
			t.Errorf("get = %q, want v1", v)
		}
		return nil
	})
	// 覆盖写
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k1", []byte("v2"))
	})
	view(t, s, func(tx *Tx) error {
		v, err := tx.Str().Get(ctx, "k1")
		if err != nil {
			return err
		}
		if string(v) != "v2" {
			t.Errorf("get = %q, want v2", v)
		}
		return nil
	})
}

func TestStringGetNotFound(t *testing.T) {
	s := openTestStore(t)
	mustSchema(t, s)
	view(t, s, func(tx *Tx) error {
		_, err := tx.Str().Get(context.Background(), "nope")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		return nil
	})
}

func TestStringSetNX(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// NX：不存在时写入成功
	update(t, s, func(tx *Tx) error {
		res, err := tx.Str().SetWith(ctx, "k", []byte("v1"), SetOptions{NX: true})
		if err != nil {
			return err
		}
		if !res.Set {
			t.Error("NX first set should succeed")
		}
		return nil
	})
	// NX：已存在时不写
	update(t, s, func(tx *Tx) error {
		res, err := tx.Str().SetWith(ctx, "k", []byte("v2"), SetOptions{NX: true})
		if err != nil {
			return err
		}
		if res.Set {
			t.Error("NX second set should be skipped")
		}
		return nil
	})
	view(t, s, func(tx *Tx) error {
		v, err := tx.Str().Get(ctx, "k")
		if err != nil {
			return err
		}
		if string(v) != "v1" {
			t.Errorf("get = %q, want v1 (unchanged)", v)
		}
		return nil
	})
}

func TestStringSetXX(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// XX：不存在时不写
	update(t, s, func(tx *Tx) error {
		res, err := tx.Str().SetWith(ctx, "k", []byte("v1"), SetOptions{XX: true})
		if err != nil {
			return err
		}
		if res.Set {
			t.Error("XX on missing key should be skipped")
		}
		return nil
	})
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k", []byte("v1"))
	})
	// XX：存在时写
	update(t, s, func(tx *Tx) error {
		res, err := tx.Str().SetWith(ctx, "k", []byte("v2"), SetOptions{XX: true})
		if err != nil {
			return err
		}
		if !res.Set {
			t.Error("XX on existing key should succeed")
		}
		return nil
	})
}

func TestStringSetGet_Old(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k", []byte("old"))
	})
	update(t, s, func(tx *Tx) error {
		res, err := tx.Str().SetWith(ctx, "k", []byte("new"), SetOptions{Get: true})
		if err != nil {
			return err
		}
		if string(res.Old) != "old" {
			t.Errorf("old = %q, want old", res.Old)
		}
		return nil
	})
}

func TestStringSetTTL(t *testing.T) {
	base := testBaseTime()
	s, cur := openTestStoreAt(t, base)
	ctx := context.Background()

	update(t, s, func(tx *Tx) error {
		ttl := int64(1000)
		_, err := tx.Str().SetWith(ctx, "k", []byte("v"), SetOptions{TTLms: &ttl})
		return err
	})
	// 未过期可读
	view(t, s, func(tx *Tx) error {
		if _, err := tx.Str().Get(ctx, "k"); err != nil {
			t.Errorf("before expire: %v", err)
		}
		return nil
	})
	// 时钟前进 2s → 过期
	*cur += 2000
	view(t, s, func(tx *Tx) error {
		_, err := tx.Str().Get(ctx, "k")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("after expire: err = %v, want ErrNotFound", err)
		}
		return nil
	})
	// 过期后重写（惰性删除 + 重建）
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k", []byte("v2"))
	})
	view(t, s, func(tx *Tx) error {
		v, err := tx.Str().Get(ctx, "k")
		if err != nil {
			return err
		}
		if string(v) != "v2" {
			t.Errorf("get = %q, want v2", v)
		}
		m, err := tx.Key().Get(ctx, "k")
		if err != nil {
			return err
		}
		if m.Etime != nil {
			t.Errorf("plain SET should clear TTL, got etime=%v", *m.Etime)
		}
		return nil
	})
}

func TestStringIncr(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	var n int64
	update(t, s, func(tx *Tx) error {
		var err error
		n, err = tx.Str().Incr(ctx, "counter", 1)
		return err
	})
	if n != 1 {
		t.Fatalf("incr missing key = %d, want 1", n)
	}
	update(t, s, func(tx *Tx) error {
		var err error
		n, err = tx.Str().Incr(ctx, "counter", 5)
		return err
	})
	if n != 6 {
		t.Fatalf("incr = %d, want 6", n)
	}
	// 负 delta
	update(t, s, func(tx *Tx) error {
		var err error
		n, err = tx.Str().Incr(ctx, "counter", -10)
		return err
	})
	if n != -4 {
		t.Fatalf("incr negative = %d, want -4", n)
	}
	// 非数值
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "str", []byte("abc"))
	})
	update(t, s, func(tx *Tx) error {
		_, err := tx.Str().Incr(ctx, "str", 1)
		if !errors.Is(err, ErrValueType) {
			t.Errorf("incr on text: err = %v, want ErrValueType", err)
		}
		return nil
	})
}

func TestStringIncrFloat(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	var f float64
	update(t, s, func(tx *Tx) error {
		var err error
		f, err = tx.Str().IncrFloat(ctx, "f", 1.5)
		return err
	})
	if f != 1.5 {
		t.Fatalf("incrfloat = %v, want 1.5", f)
	}
	update(t, s, func(tx *Tx) error {
		var err error
		f, err = tx.Str().IncrFloat(ctx, "f", 0.25)
		return err
	})
	if f != 1.75 {
		t.Fatalf("incrfloat = %v, want 1.75", f)
	}
}

func TestStringGetManySetMany(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		return tx.Str().SetMany(ctx, map[string][]byte{
			"a": []byte("1"), "b": []byte("2"), "c": []byte("3"),
		})
	})
	view(t, s, func(tx *Tx) error {
		vals, found, err := tx.Str().GetMany(ctx, "a", "x", "c")
		if err != nil {
			return err
		}
		if len(vals) != 3 || !found[0] || found[1] || !found[2] {
			t.Fatalf("found = %v, want [true false true]", found)
		}
		if string(vals[0]) != "1" || string(vals[2]) != "3" {
			t.Errorf("vals = %q,%q", vals[0], vals[2])
		}
		return nil
	})
}

func TestStringTypeGuard(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k", []byte("v"))
	})
	// 用 key 仓库误报类型守卫：直接 insertKey 一个 hash 同名 key 会破坏一致性，
	// 这里验证 guardWrite 本身：写入不同类型时应报 ErrKeyType。
	// （通过 hash 仓库桩无法测试，P2 后补交叉类型用例；此处验证 key.Get 类型）
	view(t, s, func(tx *Tx) error {
		m, err := tx.Key().Get(ctx, "k")
		if err != nil {
			return err
		}
		if m.Type != TypeString {
			t.Errorf("type = %v, want string", m.Type)
		}
		return nil
	})
}
