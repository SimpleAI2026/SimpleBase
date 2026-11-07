// 文件说明：syncer_observe.go — godatabase/ducklake 中 CatalogSyncer 的观测域：recordSyncStage、classifySyncError、recordSync
// 由 catalog_syncer.go 按职责拆分而来（C6，纯移动，不改逻辑）。

package ducklake

import (
	"errors"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"go.uber.org/zap"
)

// recordSyncStage 记录 syncOnce 内部分段（planv5.0 §4 P0.3）：
// flush_inlined_data / copy_catalog / upload_snapshot / manifest。
// 只输出固定 stage 标签、耗时与错误类别（不落错误正文，可能含 SQL 上下文）。
func (s *CatalogSyncer) recordSyncStage(dbID, stage string, d time.Duration, err error) {
	if s.Logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("database_id", dbID),
		zap.String("stage", stage),
		zap.Duration("duration", d),
		zap.Duration("oldest_unsynced_age", s.OldestUnsyncedAge(dbID)),
	}
	if err != nil {
		fields = append(fields,
			zap.Bool("error", true),
			zap.String("error_class", classifySyncError(err)),
		)
	}
	s.Logger.Debug("ducklake catalog sync stage", fields...)
}

// classifySyncError 把同步错误映射到固定低基数类别。
// 事务冲突（如 flush inlined data 与并发写争用同一表索引）单独归类，
// 便于统计 sys_cloud_agents 等表的冲突频率（planv5.0 §1 异常）。
func classifySyncError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case IsTransactionConflict(err):
		return "transaction_conflict"
	case errors.Is(err, blob.ErrPreconditionFailed):
		return "precondition_failed"
	case strings.Contains(msg, "snapshot changed during copy"):
		return "snapshot_moved"
	default:
		return "other"
	}
}

func (s *CatalogSyncer) recordSync(err error, d time.Duration, dbID string) {
	if s.Observer == nil {
		return
	}
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	s.Observer.CatalogSync(outcome, d, err != nil)
	s.Observer.CatalogSyncLag(dbID, s.SyncLag(dbID))
	if err != nil && s.Logger != nil {
		s.Logger.Warn("ducklake catalog sync failed",
			zap.String("database_id", dbID),
			zap.Error(err),
		)
	}
}
