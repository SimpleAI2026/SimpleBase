// 文件说明：syncer_prune.go — godatabase/ducklake 中 CatalogSyncer 的版本清理域：schedulePrune、pruneVersions 及进度记账
// 由 catalog_syncer.go 按职责拆分而来（C6，纯移动，不改逻辑）。

package ducklake

import (
	"context"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"go.uber.org/zap"
)

func (s *CatalogSyncer) schedulePrune(meta godatabase.Spec, snap, seq int64) {
	if seq <= int64(s.Options.KeepVersions) {
		return
	}
	s.mu.Lock()
	if s.closed || s.lostLease[meta.ID] || s.pruneTimers[meta.ID] != nil {
		s.mu.Unlock()
		return
	}
	s.pruneTimers[meta.ID] = time.AfterFunc(10*time.Minute, func() {
		s.mu.Lock()
		delete(s.pruneTimers, meta.ID)
		closed := s.closed || s.lostLease[meta.ID]
		latest := s.lastSeq[meta.ID]
		s.mu.Unlock()
		if closed || latest <= 0 {
			return
		}
		if err := s.pruneVersions(context.Background(), meta, snap, latest); err != nil && s.Logger != nil {
			s.Logger.Warn("ducklake snapshot prune failed", zap.String("database_id", meta.ID), zap.Error(err))
		}
	})
	s.mu.Unlock()
}

// pruneVersions 清理不再被保留 manifest 引用的旧快照对象
// （ducklake-storage-latency-optimization-v2.0-plan §3.1/§4-P1）：
//   - manifest 索引连续且不可变，永不删除（发现链依赖它定位最新 seq）；
//   - 只删除保留窗口（最近 KeepVersions 个 manifest）之外、且未被窗口内
//     任何 manifest 引用的快照对象；
//   - 限速：单轮最多处理 KeepVersions 个 seq，进度持久化到 local-state
//     （PrunedSeq）与内存，不再每次从 seq 1 重扫。
func (s *CatalogSyncer) pruneVersions(ctx context.Context, meta godatabase.Spec, currentSnap, currentSeq int64) error {
	keep := int64(s.Options.KeepVersions)
	if keep <= 0 || currentSeq <= keep {
		return nil
	}
	kb := s.Remote.keyBuilder()
	oldestKeep := currentSeq - keep + 1 // [1, oldestKeep) 为到期区间
	cacheDir := s.cacheDirFor(meta.ID)

	progress := s.pruneProgress(meta.ID, cacheDir)
	if progress >= oldestKeep-1 {
		return nil
	}
	start := progress + 1
	if start < 1 {
		start = 1
	}
	end := start + keep - 1 // 限速：单轮最多 keep 个 seq
	if end > oldestKeep-1 {
		end = oldestKeep - 1
	}

	// 收集保留窗口内现存 manifest 引用的快照 key，防止「重试后两个
	// manifest 指向同一快照对象」时被误删。
	referenced := map[string]bool{}
	for seq := oldestKeep; seq <= currentSeq; seq++ {
		m, err := getManifestAt(ctx, s.Store, kb, meta.ProjectID, meta.ID, seq)
		if err != nil {
			return err
		}
		if m != nil && m.SnapshotKey != "" {
			referenced[m.SnapshotKey] = true
		}
	}

	lastOK := progress
	for seq := start; seq <= end; seq++ {
		m, err := getManifestAt(ctx, s.Store, kb, meta.ProjectID, meta.ID, seq)
		if err != nil {
			break // 读失败：停在已连续处理前缀，下轮重试
		}
		if m != nil && m.SnapshotKey != "" && !referenced[m.SnapshotKey] {
			if err := s.Store.Delete(ctx, m.SnapshotKey); err != nil {
				break // 删除失败：不推进进度，下轮重试
			}
		}
		// m == nil：该 seq 的 manifest 缺失（历史同步失败），无快照可清，跳过。
		lastOK = seq
	}
	if lastOK > progress {
		s.setPruneProgress(meta.ID, cacheDir, lastOK)
	}
	return nil
}

// pruneProgress 返回清理进度（内存与 local-state 取较大者）。
func (s *CatalogSyncer) pruneProgress(dbID, cacheDir string) int64 {
	s.mu.Lock()
	p := s.prunedBelow[dbID]
	s.mu.Unlock()
	if st, ok := ReadLocalState(cacheDir, dbID); ok && st.PrunedSeq > p {
		p = st.PrunedSeq
	}
	return p
}

// setPruneProgress 推进清理进度：内存 + local-state 持久化（写失败不致命，
// 代价仅是缓存丢失后重扫一段已处理区间）。
func (s *CatalogSyncer) setPruneProgress(dbID, cacheDir string, seq int64) {
	s.mu.Lock()
	if seq > s.prunedBelow[dbID] {
		s.prunedBelow[dbID] = seq
	}
	s.mu.Unlock()
	st, _ := ReadLocalState(cacheDir, dbID)
	if seq <= st.PrunedSeq {
		return
	}
	st.PrunedSeq = seq
	if err := SaveLocalState(cacheDir, dbID, st); err != nil && s.Logger != nil {
		s.Logger.Warn("ducklake prune progress save failed",
			zap.String("database_id", dbID), zap.Error(err))
	}
}
