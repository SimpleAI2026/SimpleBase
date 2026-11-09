// 文件说明：syncer_schedule.go — godatabase/ducklake 中 CatalogSyncer 的调度域：epoch/split-brain 标记、MarkDirty debounce、Pause/Resume、Sync/Flush/Close
// 由 catalog_syncer.go 按职责拆分而来（C6，纯移动，不改逻辑）。

package ducklake

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"go.uber.org/zap"
)

// randomWriterEpoch 生成进程级写纪元：非租约部署下用纳秒时间戳+随机低位，
// 保证重启后 epoch 前进（与租约 epoch 的单调语义对齐）。
func randomWriterEpoch() int64 {
	var b [4]byte
	if _, err := crand.Read(b[:]); err != nil {
		return time.Now().UnixNano()
	}
	return time.Now().UnixNano()&^0xFF | int64(b[0])
}

// SetWriterEpoch 由租约管理器调用：获取租约后以租约 epoch 覆盖进程默认值。
func (s *CatalogSyncer) SetWriterEpoch(dbID string, epoch int64) {
	if s == nil || epoch <= 0 {
		return
	}
	s.mu.Lock()
	s.lastSeq[dbID+"_epoch"] = epoch // 记录 per-DB epoch（key 加后缀避免与 seq 冲突）
	s.mu.Unlock()
}

// writerEpochFor 返回指定库的写纪元：per-DB 租约 epoch 优先，否则进程默认。
func (s *CatalogSyncer) writerEpochFor(dbID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.lastSeq[dbID+"_epoch"]; e > 0 {
		return e
	}
	return s.WriterEpoch
}

// IsSplitBrain 返回指定库是否已被标记 split-brain。
func (s *CatalogSyncer) IsSplitBrain(dbID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.splitIDs[dbID]
}

// CloseNoFlush 置位该库的失租标志：之后该库的 Flush/Sync 不再发起任何 PUT
// （multi-instance-consistency-plan §4.6 onLost 第 1 步）。
// §3.2：作用域为 per-DB——任一库失租不得连坐其他库（含共享同步器的系统库）。
func (s *CatalogSyncer) CloseNoFlush(dbID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lostLease[dbID] = true
	delete(s.dirty, dbID)
	delete(s.dirtySince, dbID)
	if t := s.timers[dbID]; t != nil {
		t.Stop()
		delete(s.timers, dbID)
	}
	s.mu.Unlock()
}

// HasLostLease 返回该库是否已标记失租（观测/测试用）。
func (s *CatalogSyncer) HasLostLease(dbID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lostLease[dbID]
}

// Bind 在 Factory.Open 成功后注册连接，供后台 Sync 使用。
func (s *CatalogSyncer) Bind(dbID string, sqlDB *sql.DB, meta godatabase.Spec, alias string) {
	if s == nil {
		return
	}
	s.BindWithCacheDir(dbID, s.CacheDir, sqlDB, meta, alias)
}

// BindWithCacheDir 同 Bind，但记录打开该库的 Factory 的本地缓存目录（§3.5）。
func (s *CatalogSyncer) BindWithCacheDir(dbID, cacheDir string, sqlDB *sql.DB, meta godatabase.Spec, alias string) {
	if s == nil {
		return
	}
	if alias == "" {
		alias = DefaultLakeAlias
	}
	if cacheDir == "" {
		cacheDir = s.CacheDir
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.bound[dbID]; old != nil && old.sqlDB != sqlDB {
		delete(s.last, dbID)
		delete(s.lastSeq, dbID)
	}
	s.bound[dbID] = &boundDB{sqlDB: sqlDB, meta: meta, alias: alias, cacheDir: cacheDir}
}

// cacheDirFor 返回该库绑定的缓存目录；未绑定时退回 syncer 全局目录。
func (s *CatalogSyncer) cacheDirFor(dbID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.bound[dbID]; b != nil && b.cacheDir != "" {
		return b.cacheDir
	}
	return s.CacheDir
}

// Unbind 在关闭连接前调用；会尽力 Flush。
func (s *CatalogSyncer) Unbind(ctx context.Context, dbID string) error {
	if s == nil {
		return nil
	}
	err := s.Flush(ctx, dbID)
	s.mu.Lock()
	delete(s.bound, dbID)
	if t := s.timers[dbID]; t != nil {
		t.Stop()
		delete(s.timers, dbID)
	}
	delete(s.dirty, dbID)
	delete(s.dirtySince, dbID)
	if t := s.pruneTimers[dbID]; t != nil {
		t.Stop()
		delete(s.pruneTimers, dbID)
	}
	s.mu.Unlock()
	return err
}

// MarkDirty 记录写提交后的快照水位。debounce 模式下合并触发。
func (s *CatalogSyncer) MarkDirty(dbID string, snapshotID int64) {
	if s == nil || snapshotID <= 0 {
		return
	}
	s.mu.Lock()
	if s.closed || s.lostLease[dbID] {
		// 失租库不再调度任何同步（§3.2）；写路径已被 WriteGate 阻断。
		s.mu.Unlock()
		return
	}
	if snapshotID > s.dirty[dbID] {
		s.dirty[dbID] = snapshotID
		if s.dirtySince[dbID].IsZero() && snapshotID > s.last[dbID] {
			s.dirtySince[dbID] = time.Now()
		}
	}
	// seed 完成前只记账，不启动周期同步。interval 模式不会因为后续 MarkDirty
	// 推迟已有计时器，所以必须在第一次 MarkDirty 之前就置上暂停。
	if s.paused[dbID] {
		s.mu.Unlock()
		return
	}
	delay := s.Options.Debounce
	if s.Options.Mode == "interval" {
		delay = s.Options.Interval
		if s.Options.MaxLag < delay {
			delay = s.Options.MaxLag
		}
		if _, scheduled := s.timers[dbID]; scheduled {
			s.mu.Unlock()
			return
		}
	}
	s.scheduleLocked(dbID, delay)
	s.mu.Unlock()
}

// scheduleLocked 必须持有 mu；后台失败保留 dirty，并延时重试以避免对象存储故障时忙循环。
func (s *CatalogSyncer) scheduleLocked(dbID string, delay time.Duration) {
	if t := s.timers[dbID]; t != nil {
		t.Stop()
	}
	s.timers[dbID] = time.AfterFunc(delay, func() {
		s.mu.Lock()
		delete(s.timers, dbID)
		s.mu.Unlock()
		if err := s.Sync(context.Background(), dbID); err != nil && s.Logger != nil {
			s.Logger.Warn("ducklake background catalog sync failed", zap.String("database_id", dbID), zap.Error(err))
		}
	})
}

// PausePeriodic 暂停该库的周期同步调度。已有计时器会被取消。
// 进行中的 Sync 会跑完，但结束时若仍处于暂停则不再预约下一轮。
// Bootstrap 在第一次 MarkDirty 之前调用，避免 seed 与 15s interval flush 重叠。
func (s *CatalogSyncer) PausePeriodic(dbID string) {
	if s == nil || dbID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paused == nil {
		s.paused = map[string]bool{}
	}
	s.paused[dbID] = true
	if t := s.timers[dbID]; t != nil {
		t.Stop()
		delete(s.timers, dbID)
	}
}

// ResumePeriodic 恢复周期同步。暂停期间积累的 dirty 会按当前模式预约一轮。
func (s *CatalogSyncer) ResumePeriodic(dbID string) {
	if s == nil || dbID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paused == nil || !s.paused[dbID] {
		return
	}
	delete(s.paused, dbID)
	if s.closed || s.lostLease[dbID] {
		return
	}
	if s.dirty[dbID] > s.last[dbID] && s.timers[dbID] == nil {
		delay := s.Options.Debounce
		if s.Options.Mode == "interval" {
			delay = s.Options.Interval
			if s.Options.MaxLag < delay {
				delay = s.Options.MaxLag
			}
		}
		s.scheduleLocked(dbID, delay)
	}
}

// LastSynced 返回内存中的 last_synced_snapshot_id。
func (s *CatalogSyncer) LastSynced(dbID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last[dbID]
}

// OldestUnsyncedAge 返回本进程记录的最旧待同步提交年龄。
// 进程重启后未恢复提交时间，因此不能把零值当作远端已持久的证明。
func (s *CatalogSyncer) OldestUnsyncedAge(dbID string) time.Duration {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dirty[dbID] <= s.last[dbID] || s.dirtySince[dbID].IsZero() {
		return 0
	}
	return time.Since(s.dirtySince[dbID])
}

// SyncLag 返回 dirty/current 与 last_synced 的差值（未绑定时用 dirty）。
func (s *CatalogSyncer) SyncLag(dbID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.dirty[dbID]
	if cur < s.last[dbID] {
		return 0
	}
	return cur - s.last[dbID]
}

// Sync 执行一次一致性 catalog 备份并上传 S3。
func (s *CatalogSyncer) Sync(ctx context.Context, dbID string) error {
	return s.sync(ctx, dbID, false, false)
}

// sync 是 Sync 的内部实现。allowClosed 供 Close 路径在 closed 置位后仍能
// flush（§3.6：关停顺序是先停新提交、等 inflight、Flush 成功）；waitInflight
// 供关停路径等待进行中的同步收尾，而不是直接跳过。
func (s *CatalogSyncer) sync(ctx context.Context, dbID string, allowClosed, waitInflight bool) error {
	if s == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for {
		s.mu.Lock()
		if s.closed && !allowClosed {
			s.mu.Unlock()
			return errors.New("ducklake: catalog syncer closed")
		}
		if s.lostLease[dbID] {
			s.mu.Unlock()
			return nil // 该库失租：不再上传（per-DB，§3.2）
		}
		if s.inflight[dbID] {
			s.mu.Unlock()
			if !waitInflight {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(20 * time.Millisecond):
				continue
			}
		}
		b := s.bound[dbID]
		targetSnap := s.dirty[dbID]
		already := s.last[dbID]
		if b == nil {
			s.mu.Unlock()
			return fmt.Errorf("ducklake: sync %s: database not bound", dbID)
		}
		// 显式 dirty 已被同步水位覆盖可直接跳过；零 dirty 仅信任本进程成功写入的 manifest。
		if already > 0 && targetSnap <= already && (targetSnap > 0 || s.lastSeq[dbID] > 0) {
			s.mu.Unlock()
			return nil
		}
		s.inflight[dbID] = true
		s.mu.Unlock()

		start := time.Now()
		err := s.syncOnce(ctx, b.sqlDB, b.meta, b.alias)
		s.recordSync(err, time.Since(start), dbID)

		s.mu.Lock()
		s.inflight[dbID] = false
		if !s.closed && !s.lostLease[dbID] && !s.paused[dbID] && s.bound[dbID] == b && s.dirty[dbID] > s.last[dbID] {
			delay := s.Options.Debounce
			if s.Options.Mode == "interval" {
				delay = s.Options.Interval
				if s.Options.MaxLag < delay {
					delay = s.Options.MaxLag
				}
			}
			if err != nil && delay < time.Second {
				delay = time.Second
			}
			if s.timers[dbID] == nil {
				s.scheduleLocked(dbID, delay)
			}
		}
		s.mu.Unlock()
		return err
	}
}

// Flush 取消防抖并强制同步（关闭/空闲淘汰前）。
// 失租（CloseNoFlush 已置位）时跳过一切 PUT，直接返回。
func (s *CatalogSyncer) Flush(ctx context.Context, dbID string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	lost := s.lostLease[dbID]
	if t := s.timers[dbID]; t != nil {
		t.Stop()
		delete(s.timers, dbID)
	}
	s.mu.Unlock()
	if lost {
		return nil
	}
	return s.sync(ctx, dbID, false, true)
}

// Close 停止接受新的 MarkDirty，等待进行中的同步收尾后 flush 全部已绑定库
// （§3.6：先停新提交 → 等 inflight → Flush 成功，避免关停放大未同步损失）。
func (s *CatalogSyncer) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true // 先停新提交（MarkDirty/对外 Sync 拒绝）
	ids := make([]string, 0, len(s.bound))
	for id, t := range s.timers {
		t.Stop()
		delete(s.timers, id)
	}
	for id, t := range s.pruneTimers {
		t.Stop()
		delete(s.pruneTimers, id)
	}
	for id := range s.bound {
		ids = append(ids, id)
	}
	s.mu.Unlock()

	var first error
	for _, id := range ids {
		// closed 已置位：走 allowClosed 内部路径；等待 inflight 后补最后一次同步。
		if err := s.sync(ctx, id, true, true); err != nil && first == nil {
			first = err
		}
	}
	return first
}
