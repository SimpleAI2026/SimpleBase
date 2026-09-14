package objectstore

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestValidateFileKey(t *testing.T) {
	tests := []struct {
		key string
		ok  bool
	}{
		{"hello.txt", true},
		{"images/photo.png", true},
		{"a/b/c/d", true},
		{"", false},
		{"/abs/path", false},
		{"back\\slash", false},
		{"../escape", false},
		{"a/../../etc", false},
		{"a/./b", false},
		{"nul\x00byte", false},
		{strings.Repeat("a", maxFileKeyLen+1), false},
	}
	for _, tt := range tests {
		err := ValidateFileKey(tt.key)
		if tt.ok && err != nil {
			t.Errorf("ValidateFileKey(%q): unexpected error: %v", tt.key, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("ValidateFileKey(%q): expected error, got nil", tt.key)
		}
	}
}

func TestMemoryFileStore_CRUD(t *testing.T) {
	ctx := context.Background()
	fs := NewMemoryFileStore()

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
	if obj.Key != "docs/readme.txt" {
		t.Errorf("expected key docs/readme.txt, got %s", obj.Key)
	}
	if obj.Size != int64(len(body)) {
		t.Errorf("expected size %d, got %d", len(body), obj.Size)
	}
	if obj.LastModified.IsZero() {
		t.Error("expected non-zero LastModified")
	}

	// List
	objs, err = fs.List(ctx, "docs/", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs[0].Key != "docs/readme.txt" {
		t.Fatalf("expected 1 object docs/readme.txt, got %+v", objs)
	}

	// List with empty prefix returns all
	objs, err = fs.List(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}

	// PresignGet
	url, err := fs.PresignGet(ctx, "docs/readme.txt", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "data:text/plain;base64,") {
		t.Errorf("expected data URL, got %s", url[:30])
	}

	// PresignGet missing object
	_, err = fs.PresignGet(ctx, "nonexistent", time.Minute)
	if err == nil {
		t.Error("expected error for missing object")
	}

	// Delete
	if err := fs.Delete(ctx, "docs/readme.txt"); err != nil {
		t.Fatal(err)
	}
	objs, err = fs.List(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(objs))
	}

	// Delete missing key is idempotent (no error)
	if err := fs.Delete(ctx, "ghost"); err != nil {
		t.Errorf("expected nil for deleting missing key, got %v", err)
	}
}

func TestMemoryFileStore_InvalidKey(t *testing.T) {
	ctx := context.Background()
	fs := NewMemoryFileStore()

	// Put with invalid key
	_, err := fs.Put(ctx, "../escape", bytes.NewReader([]byte("x")), 1, "")
	if err == nil {
		t.Error("expected error for invalid key")
	}

	// List with invalid prefix
	_, err = fs.List(ctx, "../bad", 100)
	if err == nil {
		t.Error("expected error for invalid prefix")
	}

	// Delete with invalid key
	err = fs.Delete(ctx, "/abs")
	if err == nil {
		t.Error("expected error for invalid key")
	}

	// PresignGet with invalid key
	_, err = fs.PresignGet(ctx, "..", time.Minute)
	if err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestMemoryFileStore_MultipleProjects(t *testing.T) {
	ctx := context.Background()
	fs := NewMemoryFileStore()

	// 模拟项目隔离：handler 层拼接 "{projectID}/" 前缀
	fs.Put(ctx, "proj-a/data.txt", bytes.NewReader([]byte("a")), 1, "")
	fs.Put(ctx, "proj-b/data.txt", bytes.NewReader([]byte("b")), 1, "")

	// 列 proj-a 前缀只返回 proj-a 的对象
	objs, err := fs.List(ctx, "proj-a/", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs[0].Key != "proj-a/data.txt" {
		t.Fatalf("expected only proj-a/data.txt, got %+v", objs)
	}
}
