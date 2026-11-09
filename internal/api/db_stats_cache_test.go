package api

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

func readyDB(id string) catalog.Database {
	return catalog.Database{ID: id, Status: catalog.DatabaseReady}
}

func TestRowCountCacheMissThenHit(t *testing.T) {
	var calls atomic.Int64
	c := NewRowCountCache(RowCountCacheOptions{
		TTL: time.Minute,
		Count: func(ctx context.Context, db catalog.Database) (int64, error) {
			calls.Add(1)
			return 42, nil
		},
	})
	db := readyDB("db-1")

	if _, ok := c.Get(db.ID); ok {
		t.Fatal("expect miss before any refresh")
	}
	n, err := c.RefreshSync(context.Background(), db, time.Second)
	if err != nil || n != 42 {
		t.Fatalf("refresh: %v %v", n, err)
	}
	if n, ok := c.Get(db.ID); !ok || n != 42 {
		t.Fatalf("expect hit 42, got %v %v", n, ok)
	}
	// 命中后不再触发统计。
	c.RefreshAsync([]catalog.Database{db})
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("count called %d times, want 1", calls.Load())
	}
}

func TestRowCountCacheWatermarkInvalidation(t *testing.T) {
	var wm atomic.Int64
	c := NewRowCountCache(RowCountCacheOptions{
		TTL: time.Minute,
		Count: func(ctx context.Context, db catalog.Database) (int64, error) {
			return 7, nil
		},
		Watermark: func(dbID string) (int64, int64) { return wm.Load(), 0 },
	})
	db := readyDB("db-2")
	if _, err := c.RefreshSync(context.Background(), db, time.Second); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(db.ID); !ok {
		t.Fatal("expect hit")
	}
	wm.Store(1) // 写后同步水位前进 → 缓存必须失效
	if _, ok := c.Get(db.ID); ok {
		t.Fatal("watermark change must invalidate cache")
	}
}

func TestRowCountCacheAsyncDedup(t *testing.T) {
	var calls atomic.Int64
	release := make(chan struct{})
	c := NewRowCountCache(RowCountCacheOptions{
		TTL: time.Minute,
		Count: func(ctx context.Context, db catalog.Database) (int64, error) {
			calls.Add(1)
			<-release // 阻塞以制造并发窗口
			return 1, nil
		},
	})
	db := readyDB("db-3")
	c.RefreshAsync([]catalog.Database{db})
	c.RefreshAsync([]catalog.Database{db}) // 去重：不得重复刷新同一库
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := c.Get(db.ID); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent refresh dedup broken: %d calls", calls.Load())
	}
	// 非 ready 库不刷新。
	c.RefreshAsync([]catalog.Database{{ID: "db-4", Status: catalog.DatabaseCreating}})
	if calls.Load() != 1 {
		t.Fatal("non-ready db must not be refreshed")
	}
}

func TestRowCountCacheTTLExpiry(t *testing.T) {
	c := NewRowCountCache(RowCountCacheOptions{
		TTL:   20 * time.Millisecond,
		Count: func(context.Context, catalog.Database) (int64, error) { return 1, nil },
	})
	db := readyDB("db-5")
	if _, err := c.RefreshSync(context.Background(), db, time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := c.Get(db.ID); ok {
		t.Fatal("TTL expiry must invalidate cache")
	}
}
