package kv

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// K1：同一 *sql.DB 上每请求重建 Store 时，HasSchema 探测只执行一次；
// 后续 Store（含并发）全部命中连接级缓存；写路径 DDL 也不重复执行。
func TestConnLevelSchemaCache(t *testing.T) {
	db := openTestStoreRaw(t)

	var probes atomic.Int64
	schemaProbeHook = func() { probes.Add(1) }
	t.Cleanup(func() { schemaProbeHook = nil })

	// 首次探测（表尚不存在）：1 次探测，返回 false，不缓存 known。
	s0 := New(db)
	if ok, err := s0.HasSchema(context.Background()); err != nil || ok {
		t.Fatalf("first probe on absent schema = %v, %v", ok, err)
	}
	// 写路径建表：ensureSchema 直接执行 DDL，无需探测。
	mustSchema(t, s0)

	// 模拟上游每请求 kv.New：100 次重建 Store 并读 HasSchema，零额外探测。
	for i := 0; i < 100; i++ {
		s := New(db)
		ok, err := s.HasSchema(context.Background())
		if err != nil || !ok {
			t.Fatalf("iter %d: HasSchema = %v, %v", i, ok, err)
		}
	}
	// 再做一次写：ensureSchema 不得重复执行 DDL / 探测。
	s2 := New(db)
	update(t, s2, func(tx *Tx) error { return nil })

	// 并发重建也不产生额外探测。
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := New(db)
			if ok, err := s.HasSchema(context.Background()); err != nil || !ok {
				t.Errorf("concurrent HasSchema = %v, %v", ok, err)
			}
		}()
	}
	wg.Wait()

	if got := probes.Load(); got != 1 {
		t.Fatalf("probe count = %d, want 1 (conn-level cache must dedupe)", got)
	}
}

// K1：空表（表存在但 0 行）也视为 schema 已就绪，不得误判为无 schema。
func TestHasSchemaEmptyTableIsKnown(t *testing.T) {
	db := openTestStoreRaw(t)
	mustSchema(t, New(db))

	// 建表后同一连接上的新 Store：conn 级状态已知，HasSchema 直接命中。
	s2 := New(db)
	if !s2.connKnown.known {
		t.Fatal("conn state should be known after schema ensured on same db")
	}
	if !s2.schemaKnown.Load() {
		// Store 级缓存未热是允许的，但首次调用必须立即命中连接级缓存。
		ok, err := s2.HasSchema(context.Background())
		if err != nil || !ok {
			t.Fatalf("HasSchema on empty-but-created schema = %v, %v", ok, err)
		}
	}
	if !s2.connKnown.known {
		t.Fatal("empty table should mark conn schema as known")
	}
}

// K1：无 schema 的库首次探测返回 false 且不缓存错误状态，
// 建表后同连接的新 Store 能感知 schema 就绪。
func TestHasSchemaAbsentThenCreated(t *testing.T) {
	db := openTestStoreRaw(t)

	s := New(db)
	ok, err := s.HasSchema(context.Background())
	if err != nil {
		t.Fatalf("HasSchema on absent schema: %v", err)
	}
	if ok {
		t.Fatal("absent schema should report false")
	}
	if s.connKnown.known {
		t.Fatal("absent schema must not be cached as known")
	}

	// 建表后：新 Store 探测到已就绪。
	mustSchema(t, New(db))
	s2 := New(db)
	ok, err = s2.HasSchema(context.Background())
	if err != nil || !ok {
		t.Fatalf("HasSchema after create = %v, %v", ok, err)
	}
}
