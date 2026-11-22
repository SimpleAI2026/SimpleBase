package filedb

import (
	"context"
	"database/sql"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"sync"
)

// Syncer 管理项目文件的恢复与持久化；本地模式使用 LocalSyncer。
type Syncer interface {
	Restore(ctx context.Context, meta godatabase.Spec, filePath string) error
	Bind(meta godatabase.Spec, sqlDB *sql.DB, filePath string)
	MarkDirty(projectID string)
	Flush(ctx context.Context, projectID string) error
	Unbind(ctx context.Context, projectID string) error
	LastSynced(projectID string) int64
	SyncLag(projectID string) int64
}

// LocalSyncer 是本地持久文件模式的同步器。
type LocalSyncer struct {
	mu   sync.Mutex
	last map[string]int64
}

func NewLocalSyncer() *LocalSyncer {
	return &LocalSyncer{last: map[string]int64{}}
}

func (s *LocalSyncer) Restore(context.Context, godatabase.Spec, string) error { return nil }
func (s *LocalSyncer) Bind(godatabase.Spec, *sql.DB, string)                  {}
func (s *LocalSyncer) MarkDirty(projectID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.last[projectID]++
	s.mu.Unlock()
}
func (s *LocalSyncer) Flush(context.Context, string) error  { return nil }
func (s *LocalSyncer) Unbind(context.Context, string) error { return nil }
func (s *LocalSyncer) SyncLag(string) int64                 { return 0 }
func (s *LocalSyncer) LastSynced(projectID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last[projectID]
}
