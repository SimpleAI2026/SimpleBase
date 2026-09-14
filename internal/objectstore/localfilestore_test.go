package objectstore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalFileStore_CRUDAndPersist(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	fs, err := NewLocalFileStore(filepath.Join(root, "files"))
	if err != nil {
		t.Fatal(err)
	}

	// 初始列表为空
	objs, err := fs.List(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 0 {
		t.Fatalf("expected empty list, got %d", len(objs))
	}

	// Put
	body := []byte("hello world")
	obj, err := fs.Put(ctx, "docs/readme.txt", bytes.NewReader(body), int64(len(body)), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if obj.Key != "docs/readme.txt" || obj.Size != int64(len(body)) {
		t.Fatalf("unexpected object: %+v", obj)
	}

	// List with prefix
	objs, err = fs.List(ctx, "docs/", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs[0].Key != "docs/readme.txt" {
		t.Fatalf("expected 1 object docs/readme.txt, got %+v", objs)
	}

	// PresignGet 返回 data URL
	url, err := fs.PresignGet(ctx, "docs/readme.txt", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "data:") {
		t.Fatalf("expected data URL, got %s", url[:30])
	}

	// 关键验证：重建 FileStore 实例（模拟进程重启）后数据仍在
	fs2, err := NewLocalFileStore(filepath.Join(root, "files"))
	if err != nil {
		t.Fatal(err)
	}
	objs, err = fs2.List(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs[0].Key != "docs/readme.txt" {
		t.Fatalf("expected data survive reopen, got %+v", objs)
	}

	// PresignGet missing
	if _, err = fs2.PresignGet(ctx, "nonexistent", time.Minute); err == nil {
		t.Error("expected error for missing object")
	}

	// Delete 幂等
	if err := fs2.Delete(ctx, "docs/readme.txt"); err != nil {
		t.Fatal(err)
	}
	if err := fs2.Delete(ctx, "ghost"); err != nil {
		t.Errorf("expected nil for deleting missing key, got %v", err)
	}
	objs, err = fs2.List(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(objs))
	}
}

func TestLocalFileStore_InvalidKey(t *testing.T) {
	ctx := context.Background()
	fs, err := NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Put(ctx, "../escape", bytes.NewReader([]byte("x")), 1, ""); err == nil {
		t.Error("expected error for invalid key")
	}
	if err := fs.Delete(ctx, "/abs"); err == nil {
		t.Error("expected error for invalid key")
	}
	if _, err := fs.PresignGet(ctx, "..", time.Minute); err == nil {
		t.Error("expected error for invalid key")
	}

	// 确认没有文件逃逸到 root 之外
	if _, err := os.Stat(filepath.Join(fs.(*localFileStore).root, "..", "escape")); err == nil {
		t.Error("file escaped root")
	}
}
