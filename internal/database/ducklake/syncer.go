package ducklake

import (
	"context"
	"database/sql"
	"sync"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// Syncer 把本地 catalog.sqlite 同步到持久层。
// Phase 1 LocalSyncer 只记水位；Phase 2 CatalogSyncer 上传 S3。
type Syncer interface {
	MarkDirty(dbID string, snapshotID int64)
	LastSynced(dbID string) int64
	Sync(ctx context.Context, dbID string) error
	Flush(ctx context.Context, dbID string) error
}

// BindingSyncer 在 Open/Close 时绑定 *sql.DB，供 COPY FROM DATABASE 备份。
type BindingSyncer interface {
	Syncer
	Bind(dbID string, sqlDB *sql.DB, meta catalog.Database, alias string)
	Unbind(ctx context.Context, dbID string) error
}

// LocalSyncer 是 Phase 1 / DevMode 的本地空实现：只在内存中推进 last_synced_snapshot_id。
type LocalSyncer struct {
	mu   sync.Mutex
	last map[string]int64
}

// NewLocalSyncer 构造内存水位同步器。
func NewLocalSyncer() *LocalSyncer {
	return &LocalSyncer{last: map[string]int64{}}
}

func (s *LocalSyncer) MarkDirty(dbID string, snapshotID int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if snapshotID > s.last[dbID] {
		s.last[dbID] = snapshotID
	}
}

func (s *LocalSyncer) LastSynced(dbID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last[dbID]
}

func (s *LocalSyncer) Sync(ctx context.Context, dbID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_ = dbID
	return nil
}

func (s *LocalSyncer) Flush(ctx context.Context, dbID string) error {
	return s.Sync(ctx, dbID)
}
