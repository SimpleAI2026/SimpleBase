// 文件说明：restore_remote.go — godatabase/filedb 中 FileSyncer.PrepareRestore 等符号的实现
//
// 用法
//
//	(s *FileSyncer) PrepareRestore(projectID string) error
//
// 调用架构
//
//	[上游] godatabase/cmd, internal/app, internal/backup
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/filedb/restore_remote.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] dialect_bucket_test.go, factory.go, factory_test.go, path.go, remote.go 等 9 个
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
	st := s.stateOfLocked(projectID)
	st.restored = true
	st.gen++
	s.mu.Unlock()
	return nil
}
