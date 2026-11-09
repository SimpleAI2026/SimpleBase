// 文件说明：restore.go — godatabase/filedb 的内部实现（recoverRestoreFiles 等）
//
// 用法
//
//	同包调用：recoverRestoreFiles() → factory.go
//
// 调用架构
//
//	[上游] internal/app, internal/backup
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/filedb/restore.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] factory.go, path.go, remote.go, restore_remote.go, snapshot.go 等 6 个
//	[关键引用] recoverRestoreFiles() ← factory.go
//
// 主要符号
//
//	recoverRestoreFiles(filePath string) error

package filedb

import (
	"fmt"
	"os"
)

func recoverRestoreFiles(filePath string) error {
	oldPath := filePath + ".restore-old"
	_, mainErr := os.Stat(filePath)
	_, oldErr := os.Stat(oldPath)
	if mainErr != nil && !os.IsNotExist(mainErr) {
		return fmt.Errorf("filedb: inspect restored database: %w", mainErr)
	}
	if oldErr == nil {
		if os.IsNotExist(mainErr) {
			if err := os.Rename(oldPath, filePath); err != nil {
				return fmt.Errorf("filedb: recover previous database: %w", err)
			}
		} else if err := os.Remove(oldPath); err != nil {
			return fmt.Errorf("filedb: clean restore backup: %w", err)
		}
	}
	if err := os.Remove(filePath + ".restore-tmp"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("filedb: clean restore staging: %w", err)
	}
	return nil
}
