// 文件说明：atomicfile.go — 「同目录临时文件 + rename」的原子文件写入
//
// 用法
//
//	Write(path string, data []byte, perm os.FileMode) error  // Write 先把 data 写入 path+".tmp"，再 rename 覆盖 path
//	WriteEnsureDir(path string, data []byte, perm os.FileMode) error  // WriteEnsureDir 与 Write 相同，但先 MkdirAll(filepath.Dir(path), 0o755)
//
// 调用架构
//
//	[上游] internal/app, internal/objectstore, internal/sandbox/sshd 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/utils/atomicfile/atomicfile.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	Write(path string, data []byte, perm os.FileMode) error
//	WriteEnsureDir(path string, data []byte, perm os.FileMode) error

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
