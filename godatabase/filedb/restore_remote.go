// 文件说明：restore_remote.go — godatabase/filedb 中 PrepareRestore 等符号的实现
//
// 用法
//
//	(s *FileSyncer) PrepareRestore(projectID string) error
//
// 调用架构
//
//	[上游] internal/app, internal/backup
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/filedb/restore_remote.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] factory.go, path.go, remote.go, restore.go, snapshot.go 等 6 个
//
// 主要符号
//
//	(s *FileSyncer) PrepareRestore(projectID string) error

package filedb

import "os"

func (s *FileSyncer) PrepareRestore(projectID string) error {
	if s == nil {
		return nil
	}
	if err := os.Remove(s.localStatePath(projectID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	s.mu.Lock()
	s.restored[projectID] = true
	s.dirty[projectID] = true
	s.mu.Unlock()
	return nil
}
