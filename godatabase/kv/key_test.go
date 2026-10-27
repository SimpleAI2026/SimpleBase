package kv

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testBaseTime() time.Time {
	return time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
}

func TestKeyGetAndDelete(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k1", []byte("v"))
	})
	// Get
	view(t, s, func(tx *Tx) error {
		m, err := tx.Key().Get(ctx, "k1")
		if err != nil {
			return err
		}
		if m.Type != TypeString || m.Key != "k1" {
			t.Errorf("meta = %+v", m)
		}
		if m.Len != nil {
			t.Errorf("string len should be nil, got %v", *m.Len)
		}
		return nil
	})
	// Delete
	var n int64
	update(t, s, func(tx *Tx) error {
		var err error
		n, err = tx.Key().Delete(ctx, "k1", "nope")
		return err
	})
	if n != 1 {
		t.Errorf("delete = %d, want 1", n)
	}
	view(t, s, func(tx *Tx) error {
		_, err := tx.Key().Get(ctx, "k1")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("after delete: %v", err)
		}
		return nil
	})
}

func TestKeyCount(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		for _, k := range []string{"user:1", "user:2", "sess:1"} {
			if err := tx.Str().Set(ctx, k, []byte("v")); err != nil {
				return err
			}
		}
		return nil
	})
	view(t, s, func(tx *Tx) error {
		n, err := tx.Key().Count(ctx, "")
		if err != nil {
			return err
		}
		if n != 3 {
			t.Errorf("count all = %d, want 3", n)
		}
		n, err = tx.Key().Count(ctx, "user:*")
		if err != nil {
			return err
		}
		if n != 2 {
			t.Errorf("count user:* = %d, want 2", n)
		}
		return nil
	})
}

func TestKeyExpirePersist(t *testing.T) {
	base := testBaseTime()
	s, cur := openTestStoreAt(t, base)
	ctx := context.Background()

	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k", []byte("v"))
	})
	var ok bool
	update(t, s, func(tx *Tx) error {
		var err error
		ok, err = tx.Key().Expire(ctx, "k", 5000)
		return err
	})
	if !ok {
		t.Fatal("expire should succeed")
	}
	view(t, s, func(tx *Tx) error {
		m, err := tx.Key().Get(ctx, "k")
		if err != nil {
			return err
		}
		if m.Etime == nil {
			t.Fatal("etime should be set")
		}
		if ttl := m.TTLms(*cur); ttl == nil || *ttl != 5000 {
			t.Errorf("ttl = %v, want 5000", ttl)
		}
		return nil
	})
	// Persist
	update(t, s, func(tx *Tx) error {
		var err error
		ok, err = tx.Key().Persist(ctx, "k")
		return err
	})
	if !ok {
		t.Fatal("persist should succeed")
	}
	view(t, s, func(tx *Tx) error {
		m, err := tx.Key().Get(ctx, "k")
		if err != nil {
			return err
		}
		if m.Etime != nil {
			t.Errorf("etime should be nil after persist")
		}
		return nil
	})
	// 过期后物理行仍在（惰性），读视为不存在；写同类型可重建
	update(t, s, func(tx *Tx) error {
		_, err := tx.Key().Expire(ctx, "k", 100)
		return err
	})
	*cur += 200
	view(t, s, func(tx *Tx) error {
		_, err := tx.Key().Get(ctx, "k")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expired key: %v", err)
		}
		n, err := tx.Key().Count(ctx, "")
		if err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("count should exclude expired, got %d", n)
		}
		return nil
	})
	// Expire 不存在的 key
	update(t, s, func(tx *Tx) error {
		ok, err := tx.Key().Expire(ctx, "ghost", 1000)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if ok {
			t.Error("expire ghost should return false")
		}
		return nil
	})
}

func TestKeyExpireImmediate(t *testing.T) {
	s, _ := openTestStoreAt(t, testBaseTime())
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k", []byte("v"))
	})
	var ok bool
	update(t, s, func(tx *Tx) error {
		var err error
		ok, err = tx.Key().Expire(ctx, "k", 0) // 非正 TTL = 立即删除
		return err
	})
	if !ok {
		t.Error("expire with 0 should succeed")
	}
	view(t, s, func(tx *Tx) error {
		_, err := tx.Key().Get(ctx, "k")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("immediate expire: %v", err)
		}
		return nil
	})
}

func TestKeyRename(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		if err := tx.Str().Set(ctx, "old", []byte("v")); err != nil {
			return err
		}
		return tx.Str().Set(ctx, "existing", []byte("x"))
	})
	// 普通 rename 覆盖目标
	update(t, s, func(tx *Tx) error {
		return tx.Key().Rename(ctx, "old", "existing")
	})
	view(t, s, func(tx *Tx) error {
		v, err := tx.Str().Get(ctx, "existing")
		if err != nil {
			return err
		}
		if string(v) != "v" {
			t.Errorf("renamed value = %q, want v", v)
		}
		_, err = tx.Key().Get(ctx, "old")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("old key should be gone: %v", err)
		}
		return nil
	})
	// RenameNX 冲突
	update(t, s, func(tx *Tx) error {
		if err := tx.Str().Set(ctx, "a", []byte("1")); err != nil {
			return err
		}
		return tx.Str().Set(ctx, "b", []byte("2"))
	})
	update(t, s, func(tx *Tx) error {
		err := tx.Key().RenameNX(ctx, "a", "b")
		if !errors.Is(err, ErrKeyExists) {
			t.Errorf("renameNX = %v, want ErrKeyExists", err)
		}
		return nil
	})
	// 源不存在
	update(t, s, func(tx *Tx) error {
		err := tx.Key().Rename(ctx, "ghost", "c")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("rename ghost = %v, want ErrNotFound", err)
		}
		return nil
	})
}

func TestKeyScan(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		for _, k := range []string{"a", "b", "c", "d", "e"} {
			if err := tx.Str().Set(ctx, k, []byte("v")); err != nil {
				return err
			}
		}
		return nil
	})
	// 全量
	view(t, s, func(tx *Tx) error {
		metas, next, err := tx.Key().Scan(ctx, "", "", TypeAll, 100)
		if err != nil {
			return err
		}
		if len(metas) != 5 || next != "" {
			t.Errorf("scan all = %d keys next=%q", len(metas), next)
		}
		if metas[0].Key != "a" || metas[4].Key != "e" {
			t.Errorf("scan order wrong: %v", metas)
		}
		return nil
	})
	// 分页：count=2
	view(t, s, func(tx *Tx) error {
		metas, next, err := tx.Key().Scan(ctx, "", "", TypeAll, 2)
		if err != nil {
			return err
		}
		if len(metas) != 2 || next != "b" {
			t.Fatalf("page1 = %d keys next=%q, want 2/b", len(metas), next)
		}
		metas2, next2, err := tx.Key().Scan(ctx, next, "", TypeAll, 2)
		if err != nil {
			return err
		}
		if len(metas2) != 2 || metas2[0].Key != "c" || next2 != "d" {
			t.Fatalf("page2 = %v next=%q", metas2, next2)
		}
		metas3, next3, err := tx.Key().Scan(ctx, next2, "", TypeAll, 2)
		if err != nil {
			return err
		}
		if len(metas3) != 1 || next3 != "" {
			t.Errorf("page3 = %d keys next=%q, want 1/\"\"", len(metas3), next3)
		}
		return nil
	})
	// pattern + type 过滤
	view(t, s, func(tx *Tx) error {
		metas, _, err := tx.Key().Scan(ctx, "", "[ac]", TypeString, 10)
		if err != nil {
			return err
		}
		if len(metas) != 2 {
			t.Errorf("glob [ac] = %d keys, want 2", len(metas))
		}
		metas, _, err = tx.Key().Scan(ctx, "", "*", TypeHash, 10)
		if err != nil {
			return err
		}
		if len(metas) != 0 {
			t.Errorf("type filter hash = %d keys, want 0", len(metas))
		}
		return nil
	})
}

func TestHasSchemaLazy(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	ok, err := s.HasSchema(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("fresh db should not have kv schema")
	}
	// 写一次后建立
	mustSchema(t, s)
	ok, err = s.HasSchema(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("schema should exist after first write")
	}
}

func TestTypeIDStringAndParse(t *testing.T) {
	for _, tc := range []struct {
		id   TypeID
		name string
	}{
		{TypeString, "string"}, {TypeList, "list"}, {TypeSet, "set"},
		{TypeHash, "hash"}, {TypeZSet, "zset"},
	} {
		if tc.id.String() != tc.name {
			t.Errorf("TypeID(%d).String() = %q, want %q", tc.id, tc.id.String(), tc.name)
		}
		got, ok := ParseType(tc.name)
		if !ok || got != tc.id {
			t.Errorf("ParseType(%q) = %v,%v", tc.name, got, ok)
		}
	}
	if _, ok := ParseType("bogus"); ok {
		t.Error("ParseType(bogus) should fail")
	}
}

func TestOnWriteCallback(t *testing.T) {
	f := openTestStoreRaw(t)
	var calls int
	s := New(f, WithOnWrite(func(ctx context.Context) { calls++ }))
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k", []byte("v"))
	})
	if calls != 1 {
		t.Errorf("onWrite calls = %d, want 1 (per commit, not per statement)", calls)
	}
	// 读路径不触发
	view(t, s, func(tx *Tx) error {
		_, _ = tx.Str().Get(ctx, "k")
		return nil
	})
	if calls != 1 {
		t.Errorf("read should not trigger onWrite, got %d", calls)
	}
	// 失败回滚不触发
	_ = s.Update(ctx, func(tx *Tx) error {
		_ = tx.Str().Set(ctx, "k2", []byte("v"))
		return errors.New("boom")
	})
	if calls != 1 {
		t.Errorf("rolled-back tx should not trigger onWrite, got %d", calls)
	}
}
