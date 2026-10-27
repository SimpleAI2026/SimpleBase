// 文件说明：syncer.go — godatabase/filedb 中 Syncer、NopSyncer 等符号的实现
//
// 用法
//
//	NewNopSyncer() *NopSyncer
//	(s *NopSyncer) Restore(context.Context, godatabase.Spec, string) error
//	(s *NopSyncer) Bind(godatabase.Spec, *sql.DB, string)
//	同包调用：Restore() → factory.go, remote.go；Bind() → factory.go, remote.go；MarkDirty() → factory.go, remote.go
//
// 调用架构
//
//	[上游] internal/app, internal/backup
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/filedb/syncer.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase
//	[同包] factory.go, path.go, remote.go, restore.go, restore_remote.go 等 6 个
//	[关键引用] Restore() ← factory.go, remote.go
//
// 主要符号
//
//	type Syncer interface {…}
//	type NopSyncer struct {…}
//	NewNopSyncer() *NopSyncer
//	(s *NopSyncer) Restore(context.Context, godatabase.Spec, string) error
//	(s *NopSyncer) Bind(godatabase.Spec, *sql.DB, string)
//	(s *NopSyncer) MarkDirty(projectID string)
//	... 另有 4 个顶层符号

package filedb

import (
	"context"
	"database/sql"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"sync"
)

// Syncer 管理项目文件的恢复与持久化；本地模式使用 NopSyncer。
type Syncer interface {
	Restore(ctx context.Context, meta godatabase.Spec, filePath string) error
	Bind(meta godatabase.Spec, sqlDB *sql.DB, filePath string)
	MarkDirty(projectID string)
	Flush(ctx context.Context, projectID string) error
	Unbind(ctx context.Context, projectID string) error
	LastSynced(projectID string) int64
	SyncLag(projectID string) int64
}

// NopSyncer 是本地持久文件模式的同步器。
type NopSyncer struct {
	mu   sync.Mutex
	last map[string]int64
}

func NewNopSyncer() *NopSyncer {
	return &NopSyncer{last: map[string]int64{}}
}

func (s *NopSyncer) Restore(context.Context, godatabase.Spec, string) error { return nil }
func (s *NopSyncer) Bind(godatabase.Spec, *sql.DB, string)                  {}
func (s *NopSyncer) MarkDirty(projectID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.last[projectID]++
	s.mu.Unlock()
}
func (s *NopSyncer) Flush(context.Context, string) error  { return nil }
func (s *NopSyncer) Unbind(context.Context, string) error { return nil }
func (s *NopSyncer) SyncLag(string) int64                 { return 0 }
func (s *NopSyncer) LastSynced(projectID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last[projectID]
}
