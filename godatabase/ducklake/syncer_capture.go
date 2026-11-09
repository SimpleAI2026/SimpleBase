// 文件说明：syncer_capture.go — godatabase/ducklake 中 CatalogSyncer 的采集域：captureCatalog 快照捕获与 syncOnce 单轮同步
// 由 catalog_syncer.go 按职责拆分而来（C6，纯移动，不改逻辑）。

package ducklake

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"database/sql"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"go.uber.org/zap"
	"path/filepath"
)

// captureCatalog 在写锁内完成 flush_inlined_data 与 catalog 复制，返回快照号和文件体。
// 锁覆盖 flush 到 copy 复核结束；调用方在锁外上传，避免把对象存储往返算进写临界区。
func (s *CatalogSyncer) captureCatalog(ctx context.Context, sqlDB *sql.DB, meta godatabase.Spec, alias string) (int64, []byte, error) {
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer conn.Close()
	// 先拿到连接，再持有写锁。wctx 让锁内的 CALL/COPY 不再重复加锁。
	wctx, release := holdWriteGate(ctx, sqlDB)
	defer release()

	// 内联行必须先刷成 Parquet，再复制 catalog；否则远端恢复可能缺数据文件。
	// planv5.0 §4 P0.3：分段计量 flush/copy/upload/manifest 各阶段耗时。
	flushStart := time.Now()
	if _, err := conn.ExecContext(wctx, "CALL ducklake_flush_inlined_data(?)", alias); err != nil {
		err = fmt.Errorf("ducklake: flush inlined data: %w", err)
		s.recordSyncStage(meta.ID, "flush_inlined_data", time.Since(flushStart), err)
		return 0, nil, err
	}
	s.recordSyncStage(meta.ID, "flush_inlined_data", time.Since(flushStart), nil)

	// 顺序修正（§3.1 实现缺陷）：先取 snapshot，COPY 后复核不变，
	// 确保 snapshots/{snap}-{epoch} 写入的文件体与 snap 严格匹配。
	snapBefore, err := CurrentSnapshot(wctx, conn, alias)
	if err != nil {
		return 0, nil, err
	}

	copyStart := time.Now()
	cacheDir := s.cacheDirFor(meta.ID)
	engine := s.engineFor()
	layout := layoutForEngine(cacheDir, meta.ID, engine)
	stagingDir := filepath.Join(layout.Root, "sync-staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return 0, nil, fmt.Errorf("ducklake: staging dir: %w", err)
	}
	staging := filepath.Join(stagingDir, fmt.Sprintf("catalog-%d%s", time.Now().UnixNano(), catalogExtFor(engine)))
	defer os.Remove(staging)
	defer os.Remove(staging + ".wal")

	backupAlias := "sb_catalog_backup"
	var attach string
	if engine == EngineSQLite {
		attach = fmt.Sprintf("ATTACH %s AS %s (TYPE SQLITE)", sqlPath(staging), sqlx.Ident(backupAlias))
	} else {
		// 显式 TYPE DUCKDB：避免 .ducklake 后缀被扩展自动识别为 DuckLake 目录。
		attach = fmt.Sprintf("ATTACH %s AS %s (TYPE DUCKDB)", sqlPath(staging), sqlx.Ident(backupAlias))
	}
	if _, err := conn.ExecContext(wctx, attach); err != nil {
		return 0, nil, fmt.Errorf("ducklake: attach staging catalog: %w", err)
	}
	detach := func() {
		// 必须沿用持锁的 wctx，否则 DETACH 会在同一 goroutine 上再次抢写锁。
		_, _ = conn.ExecContext(wctx, "DETACH "+sqlx.Ident(backupAlias))
	}

	// DuckLake catalog 在 ATTACH 后通常暴露为 __ducklake_metadata_{alias}
	metaName := "__ducklake_metadata_" + alias
	copySQL := fmt.Sprintf("COPY FROM DATABASE %s TO %s", sqlx.Ident(metaName), sqlx.Ident(backupAlias))
	if _, err := conn.ExecContext(wctx, copySQL); err != nil {
		detach()
		return 0, nil, fmt.Errorf("ducklake: copy catalog backup: %w", err)
	}
	// duckdb 引擎：COPY 的内容可能仍在 staging 的 WAL 里；显式 CHECKPOINT
	// 合并进主文件，保证下面 ReadFile 读到的是完整快照。
	if engine == EngineDuckDB {
		if _, err := conn.ExecContext(wctx, "CHECKPOINT "+sqlx.Ident(backupAlias)); err != nil {
			detach()
			return 0, nil, fmt.Errorf("ducklake: checkpoint catalog backup: %w", err)
		}
	}
	detach()

	snap, err := CurrentSnapshot(wctx, conn, alias)
	if err != nil {
		return 0, nil, err
	}
	if snap != snapBefore {
		// 写锁下不应再出现。保留检查：锁外的路径或未装写锁的连接仍可能插入。
		if s.Logger != nil {
			s.Logger.Warn("ducklake catalog snapshot moved during copy; retry next sync",
				zap.String("database_id", meta.ID),
				zap.Int64("before", snapBefore),
				zap.Int64("after", snap),
			)
		}
		return 0, nil, fmt.Errorf("ducklake: snapshot changed during copy (%d -> %d)", snapBefore, snap)
	}

	data, err := os.ReadFile(staging)
	if err != nil {
		err = fmt.Errorf("ducklake: read staging catalog: %w", err)
		s.recordSyncStage(meta.ID, "copy_catalog", time.Since(copyStart), err)
		return 0, nil, err
	}
	s.recordSyncStage(meta.ID, "copy_catalog", time.Since(copyStart), nil)
	return snap, data, nil
}

func (s *CatalogSyncer) syncOnce(ctx context.Context, sqlDB *sql.DB, meta godatabase.Spec, alias string) error {
	if !s.Remote.Enabled {
		// 无远程时只推进水位（与 LocalSyncer 语义对齐，便于单测）。
		snap, err := CurrentSnapshot(ctx, sqlDB, alias)
		if err != nil {
			return err
		}
		s.mu.Lock()
		if snap > s.last[meta.ID] {
			s.last[meta.ID] = snap
		}
		if s.dirty[meta.ID] <= snap {
			delete(s.dirty, meta.ID)
			delete(s.dirtySince, meta.ID)
		}
		s.mu.Unlock()
		return nil
	}
	if s.Store == nil {
		return errors.New("ducklake: catalog blob store is nil")
	}
	s.mu.Lock()
	lost := s.lostLease[meta.ID]
	s.mu.Unlock()
	if lost {
		// 该库失租（CloseNoFlush 已置位）：不再上传，仅推进内存水位（§3.2）。
		if _, err := CurrentSnapshot(ctx, sqlDB, alias); err != nil {
			return err
		}
		s.mu.Lock()
		delete(s.dirty, meta.ID)
		delete(s.dirtySince, meta.ID)
		s.mu.Unlock()
		return nil
	}

	// flush/copy 与同一库的写事务互斥，且不把 S3 上传算进临界区。
	// 先借出连接再拿写锁，避免和「占着连接等锁」的写路径交叉死锁。
	snap, data, err := s.captureCatalog(ctx, sqlDB, meta, alias)
	if err != nil {
		return err
	}

	nextSeq, epoch, snapKey, err := s.publishSnapshot(ctx, meta, snap, data)
	if err != nil {
		return err
	}

	// 清理快照不进入同步关键路径，独立限频执行。
	s.schedulePrune(meta, snap, nextSeq)

	s.mu.Lock()
	s.last[meta.ID] = snap
	s.lastSeq[meta.ID] = nextSeq
	if s.dirty[meta.ID] <= snap {
		delete(s.dirty, meta.ID)
		delete(s.dirtySince, meta.ID)
	} else {
		// COPY 后产生的新写入尚未被本次 manifest 覆盖；保留最早时间。
		if s.dirtySince[meta.ID].IsZero() {
			s.dirtySince[meta.ID] = time.Now()
		}
	}
	s.mu.Unlock()

	cacheDir := s.cacheDirFor(meta.ID)
	if st, ok := ReadLocalState(cacheDir, meta.ID); ok || snap > 0 {
		if st.SnapshotID < snap {
			st.SnapshotID = snap
		}
		st.SyncedSnapshotID = snap
		st.SyncedSeq = nextSeq
		if err := SaveLocalState(cacheDir, meta.ID, st); err != nil {
			if s.Logger != nil {
				s.Logger.Warn("ducklake local-state save failed",
					zap.String("database_id", meta.ID), zap.Error(err))
			}
		}
	}

	if s.Observer != nil {
		s.Observer.CatalogSize(meta.ID, int64(len(data)))
	}

	if s.Logger != nil {
		s.Logger.Info("ducklake catalog synced",
			zap.String("database_id", meta.ID),
			zap.Int64("snapshot_id", snap),
			zap.Int64("manifest_seq", nextSeq),
			zap.Int64("writer_epoch", epoch),
			zap.String("key", snapKey),
		)
	}
	return nil
}
