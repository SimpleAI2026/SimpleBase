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
