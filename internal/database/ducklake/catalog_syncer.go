package ducklake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
}

// CatalogSyncer 把本地 catalog.sqlite 同步到 S3（§4.4）。
// 默认 debounce；sync_on_commit 时 MarkDirty 后同步等待完成。
type CatalogSyncer struct {
	Store    objectstore.BlobStore
	Remote   RemoteStorage
	CacheDir string
	Options  CatalogSyncOptions
	Logger   observability.Logger
	Metrics  *observability.Metrics

	mu       sync.Mutex
	last     map[string]int64 // last_synced_snapshot_id
	dirty    map[string]int64 // pending max snapshot
	bound    map[string]*boundDB
	timers   map[string]*time.Timer
	inflight map[string]bool
	closed   bool
}

// NewCatalogSyncer 构造 Phase 2 同步器。Store 不可为 nil。
func NewCatalogSyncer(store objectstore.BlobStore, remote RemoteStorage, cacheDir string, opts CatalogSyncOptions, logger observability.Logger, metrics *observability.Metrics) *CatalogSyncer {
	if opts.Mode == "" {
		opts.Mode = "debounce"
	}
	if opts.Debounce <= 0 {
		opts.Debounce = 200 * time.Millisecond
	}
	if opts.KeepVersions <= 0 {
		opts.KeepVersions = 10
	}
	return &CatalogSyncer{
		Store:    store,
		Remote:   remote,
		CacheDir: cacheDir,
		Options:  opts,
		Logger:   logger,
		Metrics:  metrics,
		last:     map[string]int64{},
		dirty:    map[string]int64{},
		bound:    map[string]*boundDB{},
		timers:   map[string]*time.Timer{},
		inflight: map[string]bool{},
	}
}

// Bind 在 Factory.Open 成功后注册连接，供后台 Sync 使用。
func (s *CatalogSyncer) Bind(dbID string, sqlDB *sql.DB, meta catalog.Database, alias string) {
	if s == nil {
		return
	}
	if alias == "" {
		alias = DefaultLakeAlias
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bound[dbID] = &boundDB{sqlDB: sqlDB, meta: meta, alias: alias}
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
	s.mu.Unlock()
	return err
}

// MarkDirty 记录写提交后的快照水位。debounce 模式下合并触发；sync_on_commit 同步执行 Sync。
func (s *CatalogSyncer) MarkDirty(dbID string, snapshotID int64) {
	if s == nil || snapshotID <= 0 {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if snapshotID > s.dirty[dbID] {
		s.dirty[dbID] = snapshotID
	}
	mode := s.Options.Mode
	debounce := s.Options.Debounce
	s.mu.Unlock()

	if mode == "sync_on_commit" {
		_ = s.Sync(context.Background(), dbID)
		return
	}

	s.mu.Lock()
	if t := s.timers[dbID]; t != nil {
		t.Stop()
	}
	s.timers[dbID] = time.AfterFunc(debounce, func() {
		_ = s.Sync(context.Background(), dbID)
	})
	s.mu.Unlock()
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
	if s == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("ducklake: catalog syncer closed")
	}
	if s.inflight[dbID] {
		s.mu.Unlock()
		return nil
	}
	b := s.bound[dbID]
	targetSnap := s.dirty[dbID]
	already := s.last[dbID]
	if b == nil {
		s.mu.Unlock()
		return fmt.Errorf("ducklake: sync %s: database not bound", dbID)
	}
	if targetSnap > 0 && targetSnap <= already {
		s.mu.Unlock()
		return nil
	}
	s.inflight[dbID] = true
	meta := b.meta
	sqlDB := b.sqlDB
	alias := b.alias
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.inflight[dbID] = false
		s.mu.Unlock()
	}()

	start := time.Now()
	err := s.syncOnce(ctx, sqlDB, meta, alias)
	s.recordSync(err, time.Since(start), dbID)
	return err
}

// Flush 取消防抖并强制同步（关闭/空闲淘汰前）。
func (s *CatalogSyncer) Flush(ctx context.Context, dbID string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if t := s.timers[dbID]; t != nil {
		t.Stop()
		delete(s.timers, dbID)
	}
	s.mu.Unlock()
	return s.Sync(ctx, dbID)
}

// Close 停止接受新的 MarkDirty，并 flush 全部已绑定库。
func (s *CatalogSyncer) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.closed = true
	ids := make([]string, 0, len(s.bound))
	for id, t := range s.timers {
		t.Stop()
		delete(s.timers, id)
	}
	for id := range s.bound {
		ids = append(ids, id)
	}
	s.mu.Unlock()

	var first error
	for _, id := range ids {
		if err := s.Flush(ctx, id); err != nil && first == nil {
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
		delete(s.dirty, meta.ID)
		s.mu.Unlock()
		return nil
	}
	if s.Store == nil {
		return errors.New("ducklake: catalog blob store is nil")
	}

	layout := layoutFor(s.CacheDir, meta.ID)
	stagingDir := filepath.Join(layout.Root, "sync-staging")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("ducklake: staging dir: %w", err)
	}
	staging := filepath.Join(stagingDir, fmt.Sprintf("catalog-%d.sqlite", time.Now().UnixNano()))
	defer os.Remove(staging)

	backupAlias := "sb_catalog_backup"
	attach := fmt.Sprintf("ATTACH %s AS %s (TYPE SQLITE)", sqlPath(staging), quoteIdent(backupAlias))
	if _, err := sqlDB.ExecContext(ctx, attach); err != nil {
		return fmt.Errorf("ducklake: attach staging catalog: %w", err)
	}
	detach := func() {
		_, _ = sqlDB.ExecContext(context.Background(), "DETACH "+quoteIdent(backupAlias))
	}

	// DuckLake SQLite catalog 在 ATTACH 后通常暴露为 __ducklake_metadata_{alias}
	metaName := "__ducklake_metadata_" + alias
	copySQL := fmt.Sprintf("COPY FROM DATABASE %s TO %s", quoteIdent(metaName), quoteIdent(backupAlias))
	if _, err := sqlDB.ExecContext(ctx, copySQL); err != nil {
		detach()
		return fmt.Errorf("ducklake: copy catalog backup: %w", err)
	}
	detach()

	snap, err := CurrentSnapshot(ctx, sqlDB, alias)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(staging)
	if err != nil {
		return fmt.Errorf("ducklake: read staging catalog: %w", err)
	}

	kb := s.Remote.keyBuilder()
	catKey, err := kb.DuckLakeCatalogKey(meta.TenantID, meta.ID)
	if err != nil {
		return err
	}
	if err := s.Store.PutBytes(ctx, catKey, data, "application/x-sqlite3"); err != nil {
		return fmt.Errorf("ducklake: put catalog: %w", err)
	}

	verKey, err := kb.DuckLakeCatalogVersionKey(meta.TenantID, meta.ID, snap)
	if err != nil {
		return err
	}
	if err := s.Store.PutBytes(ctx, verKey, data, "application/x-sqlite3"); err != nil {
		return fmt.Errorf("ducklake: put catalog version: %w", err)
	}

	_ = s.pruneVersions(ctx, meta, snap)

	s.mu.Lock()
	s.last[meta.ID] = snap
	if s.dirty[meta.ID] <= snap {
		delete(s.dirty, meta.ID)
	}
	s.mu.Unlock()

	if s.Logger != nil {
		s.Logger.Info("ducklake catalog synced",
			zap.String("database_id", meta.ID),
			zap.Int64("snapshot_id", snap),
			zap.String("key", catKey),
		)
	}
	return nil
}

func (s *CatalogSyncer) pruneVersions(ctx context.Context, meta catalog.Database, current int64) error {
	keep := int64(s.Options.KeepVersions)
	if keep <= 0 || current <= keep {
		return nil
	}
	kb := s.Remote.keyBuilder()
	// 只删除 current-keep 之前的一个候选版本，避免 List 依赖；完整列目录可在 Phase 3 补。
	old := current - keep
	if old <= 0 {
		return nil
	}
	key, err := kb.DuckLakeCatalogVersionKey(meta.TenantID, meta.ID, old)
	if err != nil {
		return err
	}
	_ = s.Store.Delete(ctx, key)
	return nil
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

// EnsureLocalCatalog 冷启动：若 S3 上有 catalog 则下载到本地；不存在则保留空目录供新建。
func EnsureLocalCatalog(ctx context.Context, store objectstore.BlobStore, remote RemoteStorage, cacheDir string, meta catalog.Database) (downloaded bool, err error) {
	if store == nil || !remote.Enabled {
		return false, nil
	}
	kb := remote.keyBuilder()
	key, err := kb.DuckLakeCatalogKey(meta.TenantID, meta.ID)
	if err != nil {
		return false, err
	}
	if _, err := store.Head(ctx, key); err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	layout := layoutFor(cacheDir, meta.ID)
	if err := layout.ensure(); err != nil {
		return false, err
	}
	if _, err := store.DownloadFile(ctx, key, layout.CatalogFile); err != nil {
		return false, fmt.Errorf("ducklake: download catalog: %w", err)
	}
	return true, nil
}
