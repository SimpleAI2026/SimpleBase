package ducklake

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
)

func TestFactoryRejectsInvalidDatabaseID(t *testing.T) {
	f := &Factory{CacheDir: t.TempDir(), Options: DefaultOptions()}
	_, err := f.Open(context.Background(), catalog.Database{ID: "../etc"}, database.ReadWrite)
	if err == nil {
		t.Fatal("expected invalid database id to fail")
	}
}

func TestFactoryRejectsEmptyCacheDir(t *testing.T) {
	f := &Factory{Options: DefaultOptions()}
	_, err := f.Open(context.Background(), catalog.Database{ID: uuid.NewString()}, database.ReadWrite)
	if err == nil {
		t.Fatal("expected empty cache dir to fail")
	}
}

func TestLayoutEnsureCreatesDirs(t *testing.T) {
	root := t.TempDir()
	l := layoutFor(root, uuid.NewString())
	if err := l.ensure(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Dir(l.CatalogFile), l.DataDir, l.TempDir} {
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			t.Fatalf("expected dir %s: %v", dir, err)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		got string
		ok  bool
	}{
		{"1.5.2", true},
		{"1.5.5", true},
		{"1.6.0", true},
		{"1.5.1", false},
		{"1.4.5", false},
		{"v1.5.2", true},
	}
	for _, c := range cases {
		ok, err := versionAtLeast(normalizeDuckDBVersion(c.got), MinDuckDBVersion)
		if err != nil {
			t.Fatalf("parse %s: %v", c.got, err)
		}
		if ok != c.ok {
			t.Errorf("versionAtLeast(%s) = %v, want %v", c.got, ok, c.ok)
		}
	}
}

func TestLocalSyncerRecordsSnapshot(t *testing.T) {
	s := NewLocalSyncer()
	s.MarkDirty("db-1", 3)
	s.MarkDirty("db-1", 2)
	if got := s.LastSynced("db-1"); got != 3 {
		t.Fatalf("LastSynced = %d, want 3", got)
	}
	if err := s.Flush(context.Background(), "db-1"); err != nil {
		t.Fatal(err)
	}
}
