// 文件说明：syncer_publish.go — godatabase/ducklake 的内部实现（publishSnapshot、verifyExistingSnapshot 等）
//
// 用法
//
//	同包调用：catalogExtFor() → local_catalog.go, syncer_capture.go
//
// 调用架构
//
//	[上游] internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/syncer_publish.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] c4_syncer_lease_test.go, c4_syncer_ordering_test.go, catalog_syncer.go, catalog_syncer_test.go, coverage_test.go 等 35 个
//	[关键引用] catalogExtFor ← local_catalog.go, syncer_capture.go
//
// 主要符号
//
//	(s *CatalogSyncer) publishSnapshot(ctx context.Context, meta godatabase.Spec, snap int64, data []byte) (int64,...
//	(s *CatalogSyncer) verifyExistingSnapshot(ctx context.Context, snapKey string, size int64, sha string) error
//	(s *CatalogSyncer) nextManifestSeq(ctx context.Context, meta godatabase.Spec) (int64, error)
//	(s *CatalogSyncer) markSplitBrain(meta godatabase.Spec, snap, seq int64)
//	(s *CatalogSyncer) engineFor() string
//	catalogExtFor(engine string) string
//	catalogContentTypeFor(engine string) string

package ducklake

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
	"github.com/linkxzhou/SimpleBase/godatabase/hashx"
	"go.uber.org/zap"
)

// publishSnapshot 执行 syncOnce 的远端发布段（原连续逻辑，保序移动）：
//  1. 不可变快照对象 PutIfAbsent；
//  2. manifest 推进 PutIfAbsent（冲突 = split-brain 确证）。
//
// 必须先于一切可覆盖写：manifest 失败时不得污染任何回退源（§3.1）。
// 返回发布成功后的 manifest seq、writer epoch 与快照 key（供调用方日志）。
func (s *CatalogSyncer) publishSnapshot(ctx context.Context, meta godatabase.Spec, snap int64, data []byte) (int64, int64, string, error) {
	sha := hashx.Sum(data)

	uploadStart := time.Now()
	kb := s.Remote.keyBuilder()
	engine := s.engineFor()
	epoch := s.writerEpochFor(meta.ID)

	// 1) 不可变快照对象：PutIfAbsent。同 snapshot_id 不同 epoch 写不同 key，
	//    结构上无法覆盖；同 key 已存在说明本 epoch 重复同步（幂等成功）。
	snapKey, err := kb.DuckLakeSnapshotKey(meta.ProjectID, meta.ID, snap, epoch, engine)
	if err != nil {
		return 0, 0, "", err
	}
	if _, err := s.Store.PutIfAbsent(ctx, snapKey, data, catalogContentTypeFor(engine)); err != nil {
		if errors.Is(err, blob.ErrPreconditionFailed) {
			// B12：同 key 已存在时必须核对内容——若本次生成的字节与已存在
			// 对象不同（引擎重试可产生不同字节），继续用本次摘要登记 manifest
			// 会让恢复时校验失败。不一致 fail closed，不推进 manifest。
			if verr := s.verifyExistingSnapshot(ctx, snapKey, int64(len(data)), sha); verr != nil {
				err = fmt.Errorf("ducklake: snapshot object exists with different content: %w", verr)
				s.recordSyncStage(meta.ID, "upload_snapshot", time.Since(uploadStart), err)
				return 0, 0, "", err
			}
			if s.Logger != nil {
				s.Logger.Debug("ducklake snapshot object already uploaded",
					zap.String("database_id", meta.ID), zap.Int64("snapshot_id", snap))
			}
		} else {
			err = fmt.Errorf("ducklake: put snapshot: %w", err)
			s.recordSyncStage(meta.ID, "upload_snapshot", time.Since(uploadStart), err)
			return 0, 0, "", err
		}
	}
	s.recordSyncStage(meta.ID, "upload_snapshot", time.Since(uploadStart), nil)

	// 2) manifest 推进：PutIfAbsent 写 manifest/{seq}.json。
	//    冲突 = split-brain 确证（第二个 writer 存在）。
	manifestStart := time.Now()
	nextSeq, err := s.nextManifestSeq(ctx, meta)
	if err != nil {
		return 0, 0, "", err
	}
	m := NewManifest(nextSeq, snap, snapKey, engine, epoch, int64(len(data)), sha)
	if err := WriteManifest(ctx, s.Store, s.Remote, meta.ProjectID, meta.ID, m); err != nil {
		if errors.Is(err, blob.ErrPreconditionFailed) {
			s.markSplitBrain(meta, snap, nextSeq)
			serr := fmt.Errorf("ducklake: split-brain detected on manifest seq %d: %w", nextSeq, blob.ErrPreconditionFailed)
			s.recordSyncStage(meta.ID, "manifest", time.Since(manifestStart), serr)
			return 0, 0, "", serr
		}
		s.recordSyncStage(meta.ID, "manifest", time.Since(manifestStart), err)
		return 0, 0, "", err
	}
	s.recordSyncStage(meta.ID, "manifest", time.Since(manifestStart), nil)
	return nextSeq, epoch, snapKey, nil
}

// verifyExistingSnapshot 核对已存在的快照对象与本次生成内容一致（B12）。
// 先 Head 比对大小，再读内容比对 sha256；任何一步失败都返回错误。
func (s *CatalogSyncer) verifyExistingSnapshot(ctx context.Context, snapKey string, size int64, sha string) error {
	info, err := s.Store.Head(ctx, snapKey)
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	if info.Size != size {
		return fmt.Errorf("size mismatch: existing=%d current=%d", info.Size, size)
	}
	data, _, err := s.Store.GetBytes(ctx, snapKey)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	if got := hashx.Sum(data); got != sha {
		return fmt.Errorf("sha256 mismatch: existing=%s current=%s", got, sha)
	}
	return nil
}

// nextManifestSeq 返回下一个 manifest seq（远端最大 seq + 1）。
// 锚点取内存水位，进程重启后回退到 local-state 的 SyncedSeq（O(1) 探测）。
// 锚点缺失（断链/远端被重置）时退化到全链发现：全空说明远端被重置，
// 允许从 seq 1 重建；否则断链 fail closed，拒绝推进（§3.1：不得退回旧镜像）。
func (s *CatalogSyncer) nextManifestSeq(ctx context.Context, meta godatabase.Spec) (int64, error) {
	s.mu.Lock()
	cached := s.lastSeq[meta.ID]
	s.mu.Unlock()
	if cached > 0 {
		return cached + 1, nil
	}
	if cached <= 0 {
		if st, ok := ReadLocalState(s.cacheDirFor(meta.ID), meta.ID); ok && st.SyncedSeq > 0 {
			cached = st.SyncedSeq
		}
	}
	latest, err := readLatestManifestAnchored(ctx, s.Store, s.Remote, meta.ProjectID, meta.ID, cached)
	if err != nil {
		return 0, err
	}
	if latest == nil {
		return 1, nil
	}
	return latest.Seq + 1, nil
}

// markSplitBrain 标记 split-brain：计数 + 库标记 + Error 日志。
// 该状态不自动恢复，需按 multi-instance-consistency-plan §9 流程人工处理。
func (s *CatalogSyncer) markSplitBrain(meta godatabase.Spec, snap, seq int64) {
	s.mu.Lock()
	already := s.splitIDs[meta.ID]
	s.splitIDs[meta.ID] = true
	s.mu.Unlock()
	if s.Observer != nil {
		s.Observer.CatalogSyncConflict(meta.ID)
	}
	if s.Logger != nil && !already {
		s.Logger.Error("ducklake catalog split-brain detected: another writer advanced the manifest",
			zap.String("database_id", meta.ID),
			zap.Int64("snapshot_id", snap),
			zap.Int64("manifest_seq", seq),
			zap.Int64("writer_epoch", s.writerEpochFor(meta.ID)),
		)
	}
}

// engineFor 返回同步器使用的 catalog 引擎（Engine 字段，默认 duckdb）。
func (s *CatalogSyncer) engineFor() string {
	return NormalizeCatalogEngine(s.Engine)
}

// catalogExtFor 返回引擎对应的文件后缀（含点）。
func catalogExtFor(engine string) string {
	ext, err := blob.CatalogFileExt(engine)
	if err != nil {
		return ".ducklake"
	}
	return ext
}

// catalogContentTypeFor 返回快照对象的 Content-Type。
func catalogContentTypeFor(engine string) string {
	if NormalizeCatalogEngine(engine) == EngineSQLite {
		return "application/x-sqlite3"
	}
	return "application/octet-stream"
}
