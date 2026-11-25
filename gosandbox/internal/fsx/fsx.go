// 文件说明：fsx.go — gosandbox/internal/fsx 中 WriteAtomic、RemoveAll 等符号的实现
//
// 用法
//
//	WriteAtomic(path string, data []byte, perm fs.FileMode) error  // WriteAtomic 在同目录写临时文件后 rename，避免读取方看到半截内容
//	RemoveAll(path string) error  // RemoveAll 删除目录树；沙盒命令留下只读目录时，先恢复目录权限再重试
//
// 调用架构
//
//	[上游] gosandbox/image, gosandbox/snapshot, gosandbox/vm/box, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/internal/fsx/fsx.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] fsx_test.go
//	[关键引用] WriteAtomic ← fsx_test.go；RemoveAll ← fsx_test.go
//
// 主要符号
//
//	WriteAtomic(path string, data []byte, perm fs.FileMode) error
//	RemoveAll(path string) error

package fsx

import (
	"io/fs"
	"os"
	"path/filepath"
)

// WriteAtomic 在同目录写临时文件后 rename，避免读取方看到半截内容。
func WriteAtomic(path string, data []byte, perm fs.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RemoveAll 删除目录树；沙盒命令留下只读目录时，先恢复目录权限再重试。
func RemoveAll(path string) error {
	if err := os.RemoveAll(path); err == nil {
		return nil
	}
	_ = filepath.WalkDir(path, func(p string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})
	return os.RemoveAll(path)
}
