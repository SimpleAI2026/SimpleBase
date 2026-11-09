package kv

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
)

// openTestStoreRaw 返回底层 *sql.DB（供需要自定义 Option 的用例）。
func openTestStoreRaw(t *testing.T) *sql.DB {
	t.Helper()
	projectID := "kvtest01"
	databasePath, err := godatabase.PathForProject(projectID, godatabase.EngineDuckDB)
	if err != nil {
		t.Fatal(err)
	}
	f := &filedb.Factory{
		DataRoot: t.TempDir(),
		CacheDir: t.TempDir(),
		Syncer:   filedb.NewNopSyncer(),
	}
	db, err := f.Open(context.Background(), godatabase.Spec{
		ID:           uuid.NewString(),
		ProjectID:    projectID,
		DatabasePath: databasePath,
		Status:       godatabase.StatusReady,
	}, godatabase.ReadWrite)
	if err != nil {
		t.Fatalf("open project database: %v", err)
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
