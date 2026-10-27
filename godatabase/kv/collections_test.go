package kv

import (
	"context"
	"errors"
	"testing"
)

func TestHashSetGetItems(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	var added int
	update(t, s, func(tx *Tx) error {
		var err error
		added, err = tx.Hash().Set(ctx, "h", map[string][]byte{
			"name": []byte("alice"), "age": []byte("30"),
		})
		return err
	})
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	// 重复写入不算新增
	update(t, s, func(tx *Tx) error {
		var err error
		added, err = tx.Hash().Set(ctx, "h", map[string][]byte{
			"name": []byte("bob"), "city": []byte("SZ"),
		})
		return err
	})
	if added != 1 {
		t.Fatalf("second added = %d, want 1 (city only)", added)
	}
	view(t, s, func(tx *Tx) error {
		v, err := tx.Hash().Get(ctx, "h", "name")
		if err != nil {
			return err
		}
		if string(v) != "bob" {
			t.Errorf("name = %q, want bob", v)
		}
		items, err := tx.Hash().Items(ctx, "h")
		if err != nil {
			return err
		}
		if len(items) != 3 {
			t.Errorf("items = %d, want 3", len(items))
		}
		m, err := tx.Key().Get(ctx, "h")
		if err != nil {
			return err
		}
		if m.Len == nil || *m.Len != 3 {
			t.Errorf("len = %v, want 3", m.Len)
		}
		return nil
	})
}

func TestHashDeleteAutoRemoveKey(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		_, err := tx.Hash().Set(ctx, "h", map[string][]byte{"f": []byte("v")})
		return err
	})
	var removed int
	update(t, s, func(tx *Tx) error {
		var err error
		removed, err = tx.Hash().Delete(ctx, "h", "f")
		return err
	})
	if removed != 1 {
		t.Fatalf("removed = %d", removed)
	}
	// 空 hash → key 行应被删除
	view(t, s, func(tx *Tx) error {
		_, err := tx.Key().Get(ctx, "h")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("empty hash key should be gone: %v", err)
		}
		return nil
	})
}

func TestHashIncr(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	var n int64
	update(t, s, func(tx *Tx) error {
		var err error
		n, err = tx.Hash().Incr(ctx, "h", "count", 5)
		return err
	})
	if n != 5 {
		t.Fatalf("hincr = %d, want 5", n)
	}
	update(t, s, func(tx *Tx) error {
		var err error
		n, err = tx.Hash().Incr(ctx, "h", "count", -2)
		return err
	})
	if n != 3 {
		t.Fatalf("hincr = %d, want 3", n)
	}
	// 非数值 field
	update(t, s, func(tx *Tx) error {
		_, err := tx.Hash().Set(ctx, "h", map[string][]byte{"s": []byte("abc")})
		return err
	})
	update(t, s, func(tx *Tx) error {
		_, err := tx.Hash().Incr(ctx, "h", "s", 1)
		if !errors.Is(err, ErrValueType) {
			t.Errorf("incr on text field = %v, want ErrValueType", err)
		}
		return nil
	})
}

func TestListPushRangePop(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	var length int64
	update(t, s, func(tx *Tx) error {
		var err error
		length, err = tx.List().PushBack(ctx, "q", []byte("a"), []byte("b"), []byte("c"))
		return err
	})
	if length != 3 {
		t.Fatalf("len = %d, want 3", length)
	}
	// LPUSH 顺序：LPUSH q x y → y,x,a,b,c
	update(t, s, func(tx *Tx) error {
		var err error
		length, err = tx.List().PushFront(ctx, "q", []byte("x"), []byte("y"))
		return err
	})
	if length != 5 {
		t.Fatalf("len = %d, want 5", length)
	}
	view(t, s, func(tx *Tx) error {
		elems, err := tx.List().Range(ctx, "q", 0, -1)
		if err != nil {
			return err
		}
		want := []string{"y", "x", "a", "b", "c"}
		if len(elems) != len(want) {
			t.Fatalf("range = %v", elems)
		}
		for i, w := range want {
			if string(elems[i]) != w {
				t.Fatalf("range[%d] = %q, want %q", i, elems[i], w)
			}
		}
		// 负下标区间
		elems, err = tx.List().Range(ctx, "q", -2, -1)
		if err != nil {
			return err
		}
		if len(elems) != 2 || string(elems[0]) != "b" || string(elems[1]) != "c" {
			t.Errorf("range -2..-1 = %v", elems)
		}
		return nil
	})
	// Pop 两端
	var v []byte
	update(t, s, func(tx *Tx) error {
		var err error
		v, err = tx.List().PopFront(ctx, "q")
		return err
	})
	if string(v) != "y" {
		t.Errorf("popfront = %q, want y", v)
	}
	update(t, s, func(tx *Tx) error {
		var err error
		v, err = tx.List().PopBack(ctx, "q")
		return err
	})
	if string(v) != "c" {
		t.Errorf("popback = %q, want c", v)
	}
	// 弹空后 key 删除
	update(t, s, func(tx *Tx) error {
		for range 3 {
			if _, err := tx.List().PopFront(ctx, "q"); err != nil {
				return err
			}
		}
		return nil
	})
	view(t, s, func(tx *Tx) error {
		_, err := tx.Key().Get(ctx, "q")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("drained list key should be gone: %v", err)
		}
		return nil
	})
}

func TestListSetAndTrim(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		_, err := tx.List().PushBack(ctx, "l", []byte("a"), []byte("b"), []byte("c"), []byte("d"))
		return err
	})
	// LSET -1
	update(t, s, func(tx *Tx) error {
		return tx.List().Set(ctx, "l", -1, []byte("D"))
	})
	view(t, s, func(tx *Tx) error {
		elems, _ := tx.List().Range(ctx, "l", 0, -1)
		if string(elems[3]) != "D" {
			t.Errorf("after lset: %v", elems)
		}
		return nil
	})
	// 越界
	update(t, s, func(tx *Tx) error {
		err := tx.List().Set(ctx, "l", 10, []byte("x"))
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("lset out of range = %v", err)
		}
		return nil
	})
	// LTRIM 保留 [1,2] → [b, c]（D 在下标 3 被裁掉）
	update(t, s, func(tx *Tx) error {
		return tx.List().Trim(ctx, "l", 1, 2)
	})
	view(t, s, func(tx *Tx) error {
		elems, _ := tx.List().Range(ctx, "l", 0, -1)
		if len(elems) != 2 || string(elems[0]) != "b" || string(elems[1]) != "c" {
			t.Errorf("after trim: %v", elems)
		}
		m, _ := tx.Key().Get(ctx, "l")
		if *m.Len != 2 {
			t.Errorf("len = %d, want 2", *m.Len)
		}
		return nil
	})
	// LTRIM 空区间 → 删 key
	update(t, s, func(tx *Tx) error {
		return tx.List().Trim(ctx, "l", 5, 9)
	})
	view(t, s, func(tx *Tx) error {
		_, err := tx.Key().Get(ctx, "l")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("trim to empty should delete key: %v", err)
		}
		return nil
	})
}

func TestSetBasic(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	var added int
	update(t, s, func(tx *Tx) error {
		var err error
		added, err = tx.Set().Add(ctx, "s1", []byte("a"), []byte("b"), []byte("a"))
		return err
	})
	if added != 2 {
		t.Fatalf("added = %d, want 2 (dedup)", added)
	}
	update(t, s, func(tx *Tx) error {
		var err error
		added, err = tx.Set().Add(ctx, "s1", []byte("b"), []byte("c"))
		return err
	})
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	view(t, s, func(tx *Tx) error {
		ok, err := tx.Set().IsMember(ctx, "s1", []byte("a"))
		if err != nil || !ok {
			t.Errorf("ismember a = %v, %v", ok, err)
		}
		mems, err := tx.Set().Members(ctx, "s1")
		if err != nil {
			return err
		}
		if len(mems) != 3 {
			t.Errorf("members = %d, want 3", len(mems))
		}
		return nil
	})
	// 删空自动删 key
	update(t, s, func(tx *Tx) error {
		_, err := tx.Set().Delete(ctx, "s1", []byte("a"), []byte("b"), []byte("c"))
		return err
	})
	view(t, s, func(tx *Tx) error {
		_, err := tx.Key().Get(ctx, "s1")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("empty set key should be gone: %v", err)
		}
		return nil
	})
}

func TestSetAlgebra(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		if _, err := tx.Set().Add(ctx, "a", []byte("1"), []byte("2"), []byte("3")); err != nil {
			return err
		}
		if _, err := tx.Set().Add(ctx, "b", []byte("2"), []byte("3"), []byte("4")); err != nil {
			return err
		}
		return nil
	})
	view(t, s, func(tx *Tx) error {
		u, err := tx.Set().Union(ctx, "a", "b")
		if err != nil || len(u) != 4 {
			t.Errorf("union = %v, %v", u, err)
		}
		i, err := tx.Set().Inter(ctx, "a", "b")
		if err != nil || len(i) != 2 {
			t.Errorf("inter = %v, %v", i, err)
		}
		d, err := tx.Set().Diff(ctx, "a", "b")
		if err != nil || len(d) != 1 || string(d[0]) != "1" {
			t.Errorf("diff = %v, %v", d, err)
		}
		// 含不存在 key 的交集为空
		i2, err := tx.Set().Inter(ctx, "a", "ghost")
		if err != nil || len(i2) != 0 {
			t.Errorf("inter with ghost = %v, %v", i2, err)
		}
		return nil
	})
	// Store 变体
	var n int
	update(t, s, func(tx *Tx) error {
		var err error
		n, err = tx.Set().InterStore(ctx, "dest", "a", "b")
		return err
	})
	if n != 2 {
		t.Fatalf("interstore = %d, want 2", n)
	}
	view(t, s, func(tx *Tx) error {
		m, err := tx.Key().Get(ctx, "dest")
		if err != nil || m.Type != TypeSet {
			t.Errorf("dest meta = %+v, %v", m, err)
		}
		return nil
	})
}

func TestSetPop(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		_, err := tx.Set().Add(ctx, "s", []byte("only"))
		return err
	})
	var v []byte
	update(t, s, func(tx *Tx) error {
		var err error
		v, err = tx.Set().Pop(ctx, "s")
		return err
	})
	if string(v) != "only" {
		t.Errorf("pop = %q", v)
	}
	view(t, s, func(tx *Tx) error {
		_, err := tx.Key().Get(ctx, "s")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("popped-empty set key should be gone: %v", err)
		}
		return nil
	})
}

func TestZSetBasic(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	var added int
	update(t, s, func(tx *Tx) error {
		var err error
		added, err = tx.ZSet().Add(ctx, "z",
			ZSetItem{Elem: []byte("alice"), Score: 90},
			ZSetItem{Elem: []byte("bob"), Score: 80},
			ZSetItem{Elem: []byte("carol"), Score: 95})
		return err
	})
	if added != 3 {
		t.Fatalf("added = %d", added)
	}
	// 更新分数不算新增
	update(t, s, func(tx *Tx) error {
		var err error
		added, err = tx.ZSet().Add(ctx, "z", ZSetItem{Elem: []byte("bob"), Score: 85})
		return err
	})
	if added != 0 {
		t.Fatalf("update added = %d, want 0", added)
	}
	view(t, s, func(tx *Tx) error {
		score, err := tx.ZSet().Score(ctx, "z", []byte("bob"))
		if err != nil || score != 85 {
			t.Errorf("score bob = %v, %v", score, err)
		}
		rank, score, err := tx.ZSet().Rank(ctx, "z", []byte("bob"))
		if err != nil || rank != 0 || score != 85 {
			t.Errorf("rank bob = %d/%v, %v", rank, score, err)
		}
		rank, _, err = tx.ZSet().Rank(ctx, "z", []byte("carol"))
		if err != nil || rank != 2 {
			t.Errorf("rank carol = %d, want 2", rank)
		}
		// 升序 range
		items, err := tx.ZSet().RangeByRank(ctx, "z", 0, -1, false)
		if err != nil {
			return err
		}
		want := []string{"bob", "alice", "carol"}
		for i, w := range want {
			if string(items[i].Elem) != w {
				t.Errorf("asc[%d] = %q, want %q", i, items[i].Elem, w)
			}
		}
		// 降序 range
		items, err = tx.ZSet().RangeByRank(ctx, "z", 0, 1, true)
		if err != nil {
			return err
		}
		if len(items) != 2 || string(items[0].Elem) != "carol" || string(items[1].Elem) != "alice" {
			t.Errorf("desc = %v", items)
		}
		// 按分数
		items, err = tx.ZSet().RangeByScore(ctx, "z", 80, 90, 0, 0)
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("byscore [80,90] = %d, want 2", len(items))
		}
		n, err := tx.ZSet().CountByScore(ctx, "z", 90, 100)
		if err != nil || n != 2 {
			t.Errorf("zcount [90,100] = %d, want 2", n)
		}
		return nil
	})
}

func TestZSetIncrAndDelete(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	var score float64
	update(t, s, func(tx *Tx) error {
		var err error
		score, err = tx.ZSet().Incr(ctx, "z", []byte("a"), 2.5)
		return err
	})
	if score != 2.5 {
		t.Fatalf("zincr new = %v, want 2.5", score)
	}
	update(t, s, func(tx *Tx) error {
		var err error
		score, err = tx.ZSet().Incr(ctx, "z", []byte("a"), 1.5)
		return err
	})
	if score != 4.0 {
		t.Fatalf("zincr = %v, want 4.0", score)
	}
	// 删除唯一成员 → key 删除
	var removed int
	update(t, s, func(tx *Tx) error {
		var err error
		removed, err = tx.ZSet().Delete(ctx, "z", []byte("a"))
		return err
	})
	if removed != 1 {
		t.Fatalf("removed = %d", removed)
	}
	view(t, s, func(tx *Tx) error {
		_, err := tx.Key().Get(ctx, "z")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("empty zset key should be gone: %v", err)
		}
		return nil
	})
}

func TestCrossTypeGuard(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	update(t, s, func(tx *Tx) error {
		return tx.Str().Set(ctx, "k", []byte("v"))
	})
	// 同 key 写 hash → ErrKeyType
	update(t, s, func(tx *Tx) error {
		_, err := tx.Hash().Set(ctx, "k", map[string][]byte{"f": []byte("v")})
		if !errors.Is(err, ErrKeyType) {
			t.Errorf("hash on string key = %v, want ErrKeyType", err)
		}
		return nil
	})
	update(t, s, func(tx *Tx) error {
		_, err := tx.List().PushBack(ctx, "k", []byte("e"))
		if !errors.Is(err, ErrKeyType) {
			t.Errorf("list on string key = %v, want ErrKeyType", err)
		}
		return nil
	})
	update(t, s, func(tx *Tx) error {
		_, err := tx.Set().Add(ctx, "k", []byte("e"))
		if !errors.Is(err, ErrKeyType) {
			t.Errorf("set on string key = %v, want ErrKeyType", err)
		}
		return nil
	})
	update(t, s, func(tx *Tx) error {
		_, err := tx.ZSet().Add(ctx, "k", ZSetItem{Elem: []byte("e"), Score: 1})
		if !errors.Is(err, ErrKeyType) {
			t.Errorf("zset on string key = %v, want ErrKeyType", err)
		}
		return nil
	})
	// 读路径类型不符同样报错
	view(t, s, func(tx *Tx) error {
		_, err := tx.Hash().Items(ctx, "k")
		if !errors.Is(err, ErrKeyType) {
			t.Errorf("read hash on string key = %v, want ErrKeyType", err)
		}
		return nil
	})
	// 原值未被破坏
	view(t, s, func(tx *Tx) error {
		v, err := tx.Str().Get(ctx, "k")
		if err != nil || string(v) != "v" {
			t.Errorf("string value corrupted: %q, %v", v, err)
		}
		return nil
	})
}
