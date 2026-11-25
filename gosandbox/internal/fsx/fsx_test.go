// 文件说明：fsx_test.go — gosandbox/internal/fsx 的单元测试（TestWriteAtomic、TestRemoveAllReadonlyDirectory 等）
//
// 用法
//
//	go test ./gosandbox/internal/fsx -run 'TestWriteAtomic|TestRemoveAllReadonlyDirectory'
//
// 调用架构
//
//	[上游] gosandbox/image, gosandbox/snapshot, gosandbox/vm/box, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/internal/fsx/fsx_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] fsx.go
//
// 主要符号
//
//	TestWriteAtomic(t *testing.T)
//	TestRemoveAllReadonlyDirectory(t *testing.T)

package fsx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	if err := WriteAtomic(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "second" {
		t.Fatalf("data = %q, %v", data, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains: %v", err)
	}
}

func TestRemoveAllReadonlyDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sandbox")
	if err := os.MkdirAll(filepath.Join(path, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "nested", "file"), []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(path, "nested"), 0o500); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("path remains: %v", err)
	}
}
