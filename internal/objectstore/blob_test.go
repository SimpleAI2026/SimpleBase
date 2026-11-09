package objectstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryBlobStoreRoundTrip(t *testing.T) {
	s := NewMemoryBlobStore()
	ctx := context.Background()
	key := "tenants/x/catalog/catalog.sqlite"
	if err := s.PutBytes(ctx, key, []byte("sqlite-bytes"), "application/x-sqlite3"); err != nil {
		t.Fatal(err)
	}
	got, info, err := s.GetBytes(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "sqlite-bytes" || info.Size != 12 {
		t.Fatalf("got %q size %d", got, info.Size)
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "catalog.sqlite")
	if _, err := s.DownloadFile(ctx, key, dest); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dest)
	if err != nil || string(b) != "sqlite-bytes" {
		t.Fatalf("download: %v %q", err, b)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Head(ctx, key); err == nil {
		t.Fatal("expected not found after delete")
	}
}
