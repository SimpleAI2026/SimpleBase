package ducklake

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// Durability 级别（§4.4）：写 API 返回的持久化承诺。
const (
	DurabilityCommittedLocal = "committed_local"
	DurabilitySyncedS3       = "synced_s3"
)

// boundDB 是打开中的库连接，供 Sync 做 COPY FROM DATABASE。
type boundDB struct {
	sqlDB *sql.DB
	meta  catalog.Database
	alias string
	// cacheDir 是打开该库的 Factory 的本地缓存目录（§3.5：系统库与用户库
	// 目录布局不同，水位/暂存文件必须按库定位，不能混用 syncer 全局目录）。
	cacheDir string
}

// CatalogSyncer 把本地 catalog.sqlite 同步到 S3（§4.4）。
// 默认 debounce；sync_on_commit 时 MarkDirty 后同步等待完成。
//
// v2（multi-instance-consistency-plan §4.4）：远端表示改为不可变快照序列
// snapshots/{snap}-{epoch}.sqlite + manifest/{seq}.json，全部用 PutIfAbsent 写入；
// manifest 冲突即 split-brain 确证（ErrPreconditionFailed）。
type CatalogSyncer struct {
	Store    objectstore.BlobStore
	Remote   RemoteStorage
	CacheDir string
	Options  CatalogSyncOptions
	Logger   observability.Logger
	Metrics  *observability.Metrics

	// Engine 是实例级 catalog 引擎（ducklake-duckdb-catalog-plan §3）；
	// 空 = duckdb。决定 staging 后缀、ATTACH 类型、Content-Type 与快照 key 后缀。
	Engine string

	// WriterEpoch 是本实例的写纪元：有租约时为租约 epoch，否则为进程启动 UUID。
	// 用于快照对象 key 的抢占可辨识性（被抢占的旧 writer 写出的对象一眼可辨）。
	WriterEpoch int64

	mu          sync.Mutex
	last        map[string]int64 // last_synced_snapshot_id
	lastSeq     map[string]int64 // last_synced_manifest_seq
	dirty       map[string]int64 // pending max snapshot
	bound       map[string]*boundDB
	timers      map[string]*time.Timer
	inflight    map[string]bool
	closed      bool
	splitIDs    map[string]bool        // split-brain 已标记的库（需人工按 §9 流程恢复）
	lostLease   map[string]bool        // 失租的库（§3.2：per-DB 作用域，不再全局连坐）
	prunedBelow map[string]int64       // 快照清理进度（不含该 seq；限速，见 pruneVersions）
	pruneTimers map[string]*time.Timer // 低优先级清理不进入写同步路径
}

// NewCatalogSyncer 构造 Phase 2 同步器。Store 不可为 nil。
// writerEpoch <= 0 时生成一个进程级随机 epoch（无租约部署的保守值）。
func NewCatalogSyncer(store objectstore.BlobStore, remote RemoteStorage, cacheDir string, opts CatalogSyncOptions, engine string, logger observability.Logger, metrics *observability.Metrics) *CatalogSyncer {
	if opts.Mode == "" {
		opts.Mode = "interval"
	}
	if opts.Debounce <= 0 {
		opts.Debounce = 200 * time.Millisecond
	}
	if opts.KeepVersions <= 0 {
		opts.KeepVersions = 10
	}
	if opts.Interval <= 0 {
		opts.Interval = 15 * time.Second
	}
	if opts.MaxLag <= 0 {
		opts.MaxLag = 30 * time.Second
	}
	if opts.Mode == "sync_on_commit" {
		opts.Mode = "interval"
		if logger != nil {
			logger.Warn("sync_on_commit is deprecated; using interval with committed_local durability")
		}
	}
	return &CatalogSyncer{
		Store:       store,
		Remote:      remote,
		CacheDir:    cacheDir,
		Options:     opts,
		Engine:      NormalizeCatalogEngine(engine),
		Logger:      logger,
		Metrics:     metrics,
		WriterEpoch: randomWriterEpoch(),
		last:        map[string]int64{},
		lastSeq:     map[string]int64{},
		dirty:       map[string]int64{},
		bound:       map[string]*boundDB{},
		timers:      map[string]*time.Timer{},
		inflight:    map[string]bool{},
		splitIDs:    map[string]bool{},
		lostLease:   map[string]bool{},
		prunedBelow: map[string]int64{},
		pruneTimers: map[string]*time.Timer{},
	}
}

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
func (s *CatalogSyncer) Bind(dbID string, sqlDB *sql.DB, meta catalog.Database, alias string) {
	if s == nil {
		return
	}
	s.BindWithCacheDir(dbID, s.CacheDir, sqlDB, meta, alias)
}

// BindWithCacheDir 同 Bind，但记录打开该库的 Factory 的本地缓存目录（§3.5）。
func (s *CatalogSyncer) BindWithCacheDir(dbID, cacheDir string, sqlDB *sql.DB, meta catalog.Database, alias string) {
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
	if t := s.pruneTimers[dbID]; t != nil {
		t.Stop()
		delete(s.pruneTimers, dbID)
	}
	s.mu.Unlock()
	return err
}

// MarkDirty 记录写提交后的快照水位。debounce 模式下合并触发；sync_on_commit 同步执行 Sync。
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

// LastSynced 返回内存中的 last_synced_snapshot_id。
func (s *CatalogSyncer) LastSynced(dbID string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last[dbID]
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
		if !s.closed && !s.lostLease[dbID] && s.bound[dbID] == b && s.dirty[dbID] > s.last[dbID] {
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

func (s *CatalogSyncer) syncOnce(ctx context.Context, sqlDB *sql.DB, meta catalog.Database, alias string) error {
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
		s.mu.Unlock()
		return nil
	}

	// 内联行必须先刷成 Parquet，再复制 catalog；否则远端恢复可能缺数据文件。
	if _, err := sqlDB.ExecContext(ctx, "CALL ducklake_flush_inlined_data(?)", alias); err != nil {
		return fmt.Errorf("ducklake: flush inlined data: %w", err)
	}

	// 顺序修正（§3.1 实现缺陷）：先取 snapshot，COPY 后复核不变，
	// 确保 snapshots/{snap}-{epoch} 写入的文件体与 snap 严格匹配。
	snapBefore, err := CurrentSnapshot(ctx, sqlDB, alias)
	if err != nil {
		return err
	}

	cacheDir := s.cacheDirFor(meta.ID)
	engine := s.engineFor()
	layout := layoutForEngine(cacheDir, meta.ID, engine)
	stagingDir := filepath.Join(layout.Root, "sync-staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("ducklake: staging dir: %w", err)
	}
	staging := filepath.Join(stagingDir, fmt.Sprintf("catalog-%d%s", time.Now().UnixNano(), catalogExtFor(engine)))
	defer os.Remove(staging)
	defer os.Remove(staging + ".wal")

	backupAlias := "sb_catalog_backup"
	var attach string
	if engine == EngineSQLite {
		attach = fmt.Sprintf("ATTACH %s AS %s (TYPE SQLITE)", sqlPath(staging), quoteIdent(backupAlias))
	} else {
		// 显式 TYPE DUCKDB：避免 .ducklake 后缀被扩展自动识别为 DuckLake 目录。
		attach = fmt.Sprintf("ATTACH %s AS %s (TYPE DUCKDB)", sqlPath(staging), quoteIdent(backupAlias))
	}
	if _, err := sqlDB.ExecContext(ctx, attach); err != nil {
		return fmt.Errorf("ducklake: attach staging catalog: %w", err)
	}
	detach := func() {
		_, _ = sqlDB.ExecContext(context.Background(), "DETACH "+quoteIdent(backupAlias))
	}

	// DuckLake catalog 在 ATTACH 后通常暴露为 __ducklake_metadata_{alias}
	metaName := "__ducklake_metadata_" + alias
	copySQL := fmt.Sprintf("COPY FROM DATABASE %s TO %s", quoteIdent(metaName), quoteIdent(backupAlias))
	if _, err := sqlDB.ExecContext(ctx, copySQL); err != nil {
		detach()
		return fmt.Errorf("ducklake: copy catalog backup: %w", err)
	}
	// duckdb 引擎：COPY 的内容可能仍在 staging 的 WAL 里；显式 CHECKPOINT
	// 合并进主文件，保证下面 ReadFile 读到的是完整快照。
	if engine == EngineDuckDB {
		if _, err := sqlDB.ExecContext(ctx, "CHECKPOINT "+quoteIdent(backupAlias)); err != nil {
			detach()
			return fmt.Errorf("ducklake: checkpoint catalog backup: %w", err)
		}
	}
	detach()

	snap, err := CurrentSnapshot(ctx, sqlDB, alias)
	if err != nil {
		return err
	}
	if snap != snapBefore {
		// COPY 期间发生了新提交：放弃本次上传（下次 Sync 重做），
		// 避免把与 snap 不匹配的文件体写成不可变快照。
		if s.Logger != nil {
			s.Logger.Warn("ducklake catalog snapshot moved during copy; retry next sync",
				zap.String("database_id", meta.ID),
				zap.Int64("before", snapBefore),
				zap.Int64("after", snap),
			)
		}
		return fmt.Errorf("ducklake: snapshot changed during copy (%d -> %d)", snapBefore, snap)
	}

	data, err := os.ReadFile(staging)
	if err != nil {
		return fmt.Errorf("ducklake: read staging catalog: %w", err)
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])

	kb := s.Remote.keyBuilder()
	epoch := s.writerEpochFor(meta.ID)

	// 1) 不可变快照对象：PutIfAbsent。同 snapshot_id 不同 epoch 写不同 key，
	//    结构上无法覆盖；同 key 已存在说明本 epoch 重复同步（幂等成功）。
	snapKey, err := kb.DuckLakeSnapshotKey(meta.TenantID, meta.ID, snap, epoch, engine)
	if err != nil {
		return err
	}
	if _, err := s.Store.PutIfAbsent(ctx, snapKey, data, catalogContentTypeFor(engine)); err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			// 同 epoch 同 snap 已上传过：幂等，继续推进 manifest。
			if s.Logger != nil {
				s.Logger.Debug("ducklake snapshot object already uploaded",
					zap.String("database_id", meta.ID), zap.Int64("snapshot_id", snap))
			}
		} else {
			return fmt.Errorf("ducklake: put snapshot: %w", err)
		}
	}

	// 2) manifest 推进：PutIfAbsent 写 manifest/{seq}.json。
	//    冲突 = split-brain 确证（第二个 writer 存在）。
	//    必须先于一切可覆盖写：manifest 失败时不得污染任何回退源（§3.1）。
	nextSeq, err := s.nextManifestSeq(ctx, meta)
	if err != nil {
		return err
	}
	m := NewManifest(nextSeq, snap, snapKey, engine, epoch, int64(len(data)), sha)
	if err := WriteManifest(ctx, s.Store, s.Remote, meta.TenantID, meta.ID, m); err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			s.markSplitBrain(meta, snap, nextSeq)
			return fmt.Errorf("ducklake: split-brain detected on manifest seq %d: %w", nextSeq, objectstore.ErrPreconditionFailed)
		}
		return err
	}

	// 清理快照不进入同步关键路径，独立限频执行。
	s.schedulePrune(meta, snap, nextSeq)

	s.mu.Lock()
	s.last[meta.ID] = snap
	s.lastSeq[meta.ID] = nextSeq
	if s.dirty[meta.ID] <= snap {
		delete(s.dirty, meta.ID)
	}
	s.mu.Unlock()

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

	if s.Metrics != nil {
		if s.Metrics.CatalogSizeBytes != nil {
			s.Metrics.CatalogSizeBytes.WithLabelValues(meta.ID).Set(float64(len(data)))
		}
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

// nextManifestSeq 返回下一个 manifest seq（远端最大 seq + 1）。
// 锚点取内存水位，进程重启后回退到 local-state 的 SyncedSeq（O(1) 探测）。
// 锚点缺失（断链/远端被重置）时退化到全链发现：全空说明远端被重置，
// 允许从 seq 1 重建；否则断链 fail closed，拒绝推进（§3.1：不得退回旧镜像）。
func (s *CatalogSyncer) nextManifestSeq(ctx context.Context, meta catalog.Database) (int64, error) {
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
	latest, err := ReadLatestManifest(ctx, s.Store, s.Remote, meta.TenantID, meta.ID, cached)
	if err != nil && IsManifestChainBroken(err) && cached > 0 {
		latest, err = ReadLatestManifest(ctx, s.Store, s.Remote, meta.TenantID, meta.ID, 0)
	}
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
func (s *CatalogSyncer) markSplitBrain(meta catalog.Database, snap, seq int64) {
	s.mu.Lock()
	already := s.splitIDs[meta.ID]
	s.splitIDs[meta.ID] = true
	s.mu.Unlock()
	if s.Metrics != nil && s.Metrics.CatalogSyncConflicts != nil {
		s.Metrics.CatalogSyncConflicts.WithLabelValues(meta.ID).Inc()
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
	ext, err := objectstore.CatalogFileExt(engine)
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

func (s *CatalogSyncer) schedulePrune(meta catalog.Database, snap, seq int64) {
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
func (s *CatalogSyncer) pruneVersions(ctx context.Context, meta catalog.Database, currentSnap, currentSeq int64) error {
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
		m, err := getManifestAt(ctx, s.Store, kb, meta.TenantID, meta.ID, seq)
		if err != nil {
			return err
		}
		if m != nil && m.SnapshotKey != "" {
			referenced[m.SnapshotKey] = true
		}
	}

	lastOK := progress
	for seq := start; seq <= end; seq++ {
		m, err := getManifestAt(ctx, s.Store, kb, meta.TenantID, meta.ID, seq)
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

func (s *CatalogSyncer) recordSync(err error, d time.Duration, dbID string) {
	if s.Metrics == nil {
		return
	}
	outcome := "ok"
	if err != nil {
		outcome = "error"
		if s.Metrics.CatalogSyncFailures != nil {
			s.Metrics.CatalogSyncFailures.Inc()
		}
	}
	if s.Metrics.CatalogSyncTotal != nil {
		s.Metrics.CatalogSyncTotal.WithLabelValues(outcome).Inc()
	}
	if s.Metrics.CatalogSyncDuration != nil {
		s.Metrics.CatalogSyncDuration.Observe(d.Seconds())
	}
	if s.Metrics.CatalogSyncLag != nil {
		s.Metrics.CatalogSyncLag.WithLabelValues(dbID).Set(float64(s.SyncLag(dbID)))
	}
	if err != nil && s.Logger != nil {
		s.Logger.Warn("ducklake catalog sync failed",
			zap.String("database_id", dbID),
			zap.Error(err),
		)
	}
}

// EnsureLocalCatalog 冷启动：把远端最新 catalog 拉到本地（multi-instance-consistency-plan §4.3）。
//
// v3 行为（ducklake-duckdb-catalog-plan §3/§4.4）：
//  1. 读远端 manifest 序列，取最大 seq 的 snapshot_key 下载；
//     manifest 的 catalog_engine 与配置不一致 → ErrCatalogEngineMismatch，不下载；
//  2. 无 manifest = 新库（旧 key 回退路径已删除，不兼容旧 SQLite 部署）；
//  3. 本地 local-state.json.snapshot_id > 远端水位 → 不下载（本地领先，
//     Warn + 计数），返回 localAhead=true，调用方应触发一次 Sync 推上去；
//  4. state 损坏/缺失但 catalog 文件存在 → 保守视为 0，允许下载；
//  5. 下载后按 manifest.size/sha256 校验，通过后才 rename 覆盖；
//     覆盖前删除本地残留 .wal（DuckDB WAL 与新 catalog 不匹配会损坏）。
func EnsureLocalCatalog(ctx context.Context, store objectstore.BlobStore, remote RemoteStorage, cacheDir string, meta catalog.Database, engine string) (downloaded bool, err error) {
	if store == nil || !remote.Enabled {
		return false, nil
	}
	engine = NormalizeCatalogEngine(engine)

	layout := layoutForEngine(cacheDir, meta.ID, engine)
	if err := layout.ensure(); err != nil {
		return false, err
	}

	// 本地水位先读：SyncedSeq 作为远端发现锚点（O(1) 探测），
	// 并用于下方本地领先比较。仅当本地 catalog 文件确实存在时才信任锚点。
	local, hasState := ReadLocalState(cacheDir, meta.ID)
	anchor := int64(0)
	if hasState && local.SyncedSeq > 0 {
		if _, statErr := os.Stat(layout.CatalogFile); statErr == nil {
			anchor = local.SyncedSeq
		}
	}

	// 远端最新水位：manifest 序列（无兼容回退）。
	latest, err := ReadLatestManifest(ctx, store, remote, meta.TenantID, meta.ID, anchor)
	if err != nil && IsManifestChainBroken(err) && anchor > 0 {
		// 锚点失效（远端被重置/回退）：退化到全链发现再判一次；
		// 全链发现仍断链则 fail closed（§3.1：不得退回旧镜像）。
		latest, err = ReadLatestManifest(ctx, store, remote, meta.TenantID, meta.ID, 0)
	}
	if err != nil {
		return false, err
	}
	if latest == nil {
		return false, nil // 新库：远端无任何 manifest
	}
	// §4.4 防御：manifest 引擎与配置不一致 → 拒绝下载。
	if NormalizeCatalogEngine(latest.CatalogEngine) != engine {
		return false, fmt.Errorf("%w: remote manifest seq %d was written with engine %q, config requires %q (tenant=%s db=%s)",
			ErrCatalogEngineMismatch, latest.Seq, latest.CatalogEngine, engine, meta.TenantID, meta.ID)
	}

	remoteSnap := latest.SnapshotID
	remoteKey := latest.SnapshotKey
	// §4.4 防御：snapshot_key 后缀必须与配置引擎一致（防手工篡改/残留）。
	if remoteKey != "" && !strings.HasSuffix(remoteKey, catalogExtFor(engine)) {
		return false, fmt.Errorf("%w: remote snapshot key %q suffix does not match engine %q (tenant=%s db=%s)",
			ErrCatalogEngineMismatch, remoteKey, engine, meta.TenantID, meta.ID)
	}

	if _, statErr := os.Stat(layout.CatalogFile); statErr == nil && hasState && local.SnapshotID > 0 {
		if local.SnapshotID > remoteSnap && remoteSnap > 0 {
			// 本地领先远端（Sync 失败路径，§3.8）：不覆盖。
			return false, errLocalAhead{dbID: meta.ID, local: local.SnapshotID, remote: remoteSnap}
		}
		if local.SnapshotID == remoteSnap && remoteSnap > 0 {
			// 已是最新：跳过下载。
			return false, nil
		}
	}

	if remoteKey == "" {
		return false, nil
	}
	// staging 下载成功后校验 + rename 覆盖（避免半写文件）。
	tmp := layout.CatalogFile + ".dl.tmp"
	if _, err := store.DownloadFile(ctx, remoteKey, tmp); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("ducklake: download catalog: %w", err)
	}
	data, err := os.ReadFile(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("ducklake: read downloaded catalog: %w", err)
	}
	if err := verifySnapshotBody(data, latest); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	// 覆盖前清理本地残留 WAL（与旧 catalog 配套，不清理会导致新 catalog + 旧 WAL）。
	if wal := layout.WALFile(engine); wal != "" {
		if err := os.Remove(wal); err != nil && !os.IsNotExist(err) {
			_ = os.Remove(tmp)
			return false, fmt.Errorf("ducklake: remove stale catalog WAL: %w", err)
		}
	}
	if err := os.Rename(tmp, layout.CatalogFile); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("ducklake: rename downloaded catalog: %w", err)
	}

	// 同步本地水位：远端快照已知时记录，供下次 Open 比较与 syncOnce 推进。
	if remoteSnap > 0 {
		st := local
		if !hasState {
			st = LocalState{}
		}
		st.SnapshotID = remoteSnap
		st.SyncedSnapshotID = remoteSnap
		if latest != nil {
			st.SyncedSeq = latest.Seq
		}
		if err := SaveLocalState(cacheDir, meta.ID, st); err != nil {
			// 水位文件写失败不阻塞打开（下次 syncOnce 会重写）。
			_ = err
		}
	}
	return true, nil
}

// verifySnapshotBody 校验下载的快照与 manifest 记录一致（size + sha256）。
func verifySnapshotBody(data []byte, m *Manifest) error {
	if m == nil || m.Size <= 0 || len(m.SHA256) != 64 {
		return fmt.Errorf("ducklake: catalog manifest missing required size or sha256")
	}
	if int64(len(data)) != m.Size {
		return fmt.Errorf("ducklake: downloaded catalog size %d != manifest %d (seq %d)", len(data), m.Size, m.Seq)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != m.SHA256 {
		return fmt.Errorf("ducklake: downloaded catalog sha256 mismatch (seq %d)", m.Seq)
	}
	return nil
}

// errLocalAhead 表示本地 catalog 水位领先远端（§3.8 路径被拦截）。
type errLocalAhead struct {
	dbID          string
	local, remote int64
}

func (e errLocalAhead) Error() string {
	return fmt.Sprintf("ducklake: local catalog snapshot %d is ahead of remote %d for %s (skip download)",
		e.local, e.remote, e.dbID)
}

// IsLocalAhead 判定 EnsureLocalCatalog 返回的错误是否为「本地领先」。
func IsLocalAhead(err error) bool {
	var la errLocalAhead
	return errors.As(err, &la)
}
