package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReplacesAndCleansTmp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.json")
	if err := Write(p, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(p, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "two" {
		t.Fatalf("content = %q", got)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp should be gone, err=%v", err)
	}
}

func TestWriteRenameFailureCleansTmp(t *testing.T) {
	dir := t.TempDir()
	// 目标是非空目录：rename 文件覆盖目录必然失败。
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Write(target, []byte("x"), 0o644); err == nil {
		t.Fatal("expected rename error")
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp must be removed after rename failure, err=%v", err)
	}
}

func TestWriteMissingDirFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope", "f")
	if err := Write(p, []byte("x"), 0o644); err == nil {
		t.Fatal("Write must not create parent dirs")
	}
}
