// 文件说明：atomicfile.go — godatabase/atomicfile 中 Write、WriteEnsureDir 等符号的实现
//
// 用法
//
//	Write(path string, data []byte, perm os.FileMode) error  // Write writes path+".tmp" and renames it over path. Errors are returned unchanged...
//	WriteEnsureDir(path string, data []byte, perm os.FileMode) error  // WriteEnsureDir creates the parent directory before calling Write.
//
// 调用架构
//
//	[上游] godatabase/blob, godatabase/ducklake, godatabase/filedb, internal/app 等 8 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/atomicfile/atomicfile.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] atomicfile_test.go
//	[关键引用] Write ← atomicfile_test.go；WriteEnsureDir ← atomicfile_test.go
//
// 主要符号
//
//	Write(path string, data []byte, perm os.FileMode) error
//	WriteEnsureDir(path string, data []byte, perm os.FileMode) error

// Package atomicfile writes files atomically within their existing directory.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write writes path+".tmp" and renames it over path.
// Errors are returned unchanged and temporary files are removed on failure.
func Write(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// WriteEnsureDir creates the parent directory before calling Write.
func WriteEnsureDir(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return Write(path, data, perm)
}
