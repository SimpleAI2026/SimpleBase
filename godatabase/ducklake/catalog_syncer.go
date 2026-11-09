// 文件说明：catalog_syncer.go — godatabase/ducklake 中 DurabilityCommittedLocal、DurabilitySyncedS3 等符号…
//
// 用法
//
//	NewCatalogSyncer(store blob.Store, remote RemoteStorage, cacheDir string, opts ...
//	(s *CatalogSyncer) SetWriterEpoch(dbID string, epoch int64)  // SetWriterEpoch 由租约管理器调用：获取租约后以租约 epoch 覆盖进程默认值
//	(s *CatalogSyncer) IsSplitBrain(dbID string) bool  // IsSplitBrain 返回指定库是否已被标记 split-brain
//	同包调用：sync() → factory.go, gated_conn.go 等；Close() → extension.go, factory.go 等；Sync() → factory.go, orphan_ledger.go 等
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/catalog_syncer.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, godatabase/blob, godatabase/internal/hashx, godatabase/sqlx
//	[同包] errors.go, extension.go, factory.go, gated_conn.go, inspect.go 等 14 个
//	[关键引用] sync() ← factory.go, gated_conn.go, inspect.go
//
// 主要符号
//
//	const DurabilityCommittedLocal
//	const DurabilitySyncedS3
//	type CatalogSyncer struct {…}
//	NewCatalogSyncer(store blob.Store, remote RemoteStorage, cacheDir string, opts ...
//	(s *CatalogSyncer) SetWriterEpoch(dbID string, epoch int64)
//	(s *CatalogSyncer) IsSplitBrain(dbID string) bool
//	... 另有 40 个顶层符号

package ducklake

import (
	"database/sql"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/blob"
)

// Durability 级别（§4.4）：写 API 返回的持久化承诺。
const (
	DurabilityCommittedLocal = "committed_local"
	DurabilitySyncedS3       = "synced_s3"
)

// boundDB 是打开中的库连接，供 Sync 做 COPY FROM DATABASE。
type boundDB struct {
	sqlDB *sql.DB
	meta  godatabase.Spec
	alias string
	// cacheDir 是打开该库的 Factory 的本地缓存目录（§3.5：系统库与用户库
	// 目录布局不同，水位/暂存文件必须按库定位，不能混用 syncer 全局目录）。
	cacheDir string
}

// CatalogSyncer 把本地 catalog.sqlite 同步到 S3（§4.4）。
// 默认 interval；MarkDirty 只推进水位，上传由后台按间隔进行。
//
// v2（multi-instance-consistency-plan §4.4）：远端表示改为不可变快照序列
// snapshots/{snap}-{epoch}.sqlite + manifest/{seq}.json，全部用 PutIfAbsent 写入；
// manifest 冲突即 split-brain 确证（ErrPreconditionFailed）。
type CatalogSyncer struct {
	Store    blob.Store
	Remote   RemoteStorage
	CacheDir string
	Options  CatalogSyncOptions
	Logger   godatabase.Logger
	// Observer 接收 catalog 同步指标；可为 nil。
	Observer godatabase.Observer

	// Engine 是实例级 catalog 引擎（ducklake-duckdb-catalog-plan §3）；
	// 空 = duckdb。决定 staging 后缀、ATTACH 类型、Content-Type 与快照 key 后缀。
	Engine string

	// WriterEpoch 是本实例的写纪元：有租约时为租约 epoch，否则为进程启动 UUID。
	// 用于快照对象 key 的抢占可辨识性（被抢占的旧 writer 写出的对象一眼可辨）。
	WriterEpoch int64

	mu          sync.Mutex
	last        map[string]int64     // last_synced_snapshot_id
	lastSeq     map[string]int64     // last_synced_manifest_seq
	dirty       map[string]int64     // pending max snapshot
	dirtySince  map[string]time.Time // 最旧尚未由成功 manifest 覆盖的本地提交时间（进程内估计）
	bound       map[string]*boundDB
	timers      map[string]*time.Timer
	inflight    map[string]bool
	closed      bool
	splitIDs    map[string]bool        // split-brain 已标记的库（需人工按 §9 流程恢复）
	lostLease   map[string]bool        // 失租的库（§3.2：per-DB 作用域，不再全局连坐）
	paused      map[string]bool        // 暂停周期调度（seed 完成前不启动 interval sync）
	prunedBelow map[string]int64       // 快照清理进度（不含该 seq；限速，见 pruneVersions）
	pruneTimers map[string]*time.Timer // 低优先级清理不进入写同步路径
}

// NewCatalogSyncer 构造 Phase 2 同步器。Store 不可为 nil。
// writerEpoch <= 0 时生成一个进程级随机 epoch（无租约部署的保守值）。
func NewCatalogSyncer(store blob.Store, remote RemoteStorage, cacheDir string, opts CatalogSyncOptions, engine string, logger godatabase.Logger, observer godatabase.Observer) *CatalogSyncer {
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
	return &CatalogSyncer{
		Store:       store,
		Remote:      remote,
		CacheDir:    cacheDir,
		Options:     opts,
		Engine:      NormalizeCatalogEngine(engine),
		Logger:      logger,
		Observer:    observer,
		WriterEpoch: randomWriterEpoch(),
		last:        map[string]int64{},
		lastSeq:     map[string]int64{},
		dirty:       map[string]int64{},
		dirtySince:  map[string]time.Time{},
		bound:       map[string]*boundDB{},
		timers:      map[string]*time.Timer{},
		inflight:    map[string]bool{},
		splitIDs:    map[string]bool{},
		lostLease:   map[string]bool{},
		paused:      map[string]bool{},
		prunedBelow: map[string]int64{},
		pruneTimers: map[string]*time.Timer{},
	}
}
