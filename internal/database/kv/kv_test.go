package kv

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/ducklake"
)

// openTestStoreRaw 返回底层 *sql.DB（供需要自定义 Option 的用例）。
func openTestStoreRaw(t *testing.T) *sql.DB {
	t.Helper()
	f := &ducklake.Factory{
		CacheDir: t.TempDir(),
		Options:  ducklake.DefaultOptions(),
		Syncer:   ducklake.NewLocalSyncer(),
	}
	db, err := f.Open(context.Background(), catalog.Database{
		ID:     uuid.NewString(),
		Name:   "kvtest",
		Status: catalog.DatabaseReady,
	}, database.ReadWrite)
	if err != nil {
		t.Fatalf("open ducklake: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// openTestStore 起一个真实 DuckLake 引擎（本地临时目录 catalog，无远端）。
func openTestStore(t *testing.T) *Store {
	t.Helper()
	return New(openTestStoreRaw(t))
}

// openTestStoreAt 同 openTestStore，但注入固定时钟（TTL 用例用）。
func openTestStoreAt(t *testing.T, base time.Time) (*Store, *int64) {
	t.Helper()
	db := openTestStoreRaw(t)
	cur := base.UnixMilli()
	return New(db, WithNowFunc(func() time.Time { return time.UnixMilli(cur) })), &cur
}

func update(t *testing.T, s *Store, fn func(tx *Tx) error) {
	t.Helper()
	if err := s.Update(context.Background(), fn); err != nil {
		t.Fatalf("update: %v", err)
	}
}

func view(t *testing.T, s *Store, fn func(tx *Tx) error) {
	t.Helper()
	if err := s.View(context.Background(), fn); err != nil {
		t.Fatalf("view: %v", err)
	}
}

func mustSchema(t *testing.T, s *Store) {
	t.Helper()
	update(t, s, func(tx *Tx) error { return nil })
}
