// Package atomicfile 提供「同目录临时文件 + rename」的原子文件写入。
//
// 只允许 import 标准库。本包刻意不做两件事：
//   - 不 fsync：现有所有调用点都未 fsync，加上会改变水位文件等高频路径的 IO 特性；
//   - 不改临时文件命名：沿用 path+".tmp"，保持残留文件可被固定名识别与清理。
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write 先把 data 写入 path+".tmp"，再 rename 覆盖 path。
// 任一步失败都会清理临时文件，且不会留下半截目标文件。
//
// 错误原样返回（*os.PathError / *os.LinkError），由调用方按自身前缀包装。
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

// WriteEnsureDir 与 Write 相同，但先 MkdirAll(filepath.Dir(path), 0o755)。
func WriteEnsureDir(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return Write(path, data, perm)
}
