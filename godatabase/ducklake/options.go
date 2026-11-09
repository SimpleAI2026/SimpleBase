// 文件说明：options.go — godatabase/ducklake 中 DefaultLakeAlias、EngineDuckDB 等符号的实现
//
// 用法
//
//	NormalizeCatalogEngine(s string) string  // NormalizeCatalogEngine 规范化配置值
//	IsValidCatalogEngine(s string) bool  // IsValidCatalogEngine 校验配置取值
//	同包调用：NormalizeCatalogEngine() → catalog_syncer.go, extension.go 等；EngineDuckDB → catalog_syncer.go, factory.go 等；EngineSQLite → catalog_syncer.go, extension.go 等
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/options.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] catalog_syncer.go, errors.go, extension.go, factory.go, gated_conn.go 等 14 个
//	[关键引用] NormalizeCatalogEngine() ← catalog_syncer.go, extension.go, factory.go
//
// 主要符号
//
//	const DefaultLakeAlias
//	const EngineDuckDB
//	const EngineSQLite
//	NormalizeCatalogEngine(s string) string
//	IsValidCatalogEngine(s string) bool
//	type Options struct {…}
//	... 另有 4 个顶层符号

package ducklake

import (
	"strings"
	"time"
)

// DefaultLakeAlias 是每个 DuckDB 实例 ATTACH DuckLake 时使用的固定别名。
// 用户 SQL 无法 ATTACH/USE，因此不会与用户对象冲突。
const DefaultLakeAlias = "lake"

// catalog engine 取值（ducklake-duckdb-catalog-plan §2）。
const (
	// EngineDuckDB 是默认推荐引擎：每库一个 DuckDB 实例、单连接单写，
	// 属于 DuckLake 官方「单客户端」形态，无需加载 sqlite 扩展。
	EngineDuckDB = "duckdb"
	// EngineSQLite 是已知可用的回退引擎（多本地客户端形态）。
	EngineSQLite = "sqlite"
)

// NormalizeCatalogEngine 规范化配置值；空串取默认 duckdb。
func NormalizeCatalogEngine(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return EngineDuckDB
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}

// IsValidCatalogEngine 校验配置取值。
func IsValidCatalogEngine(s string) bool {
	switch NormalizeCatalogEngine(s) {
	case EngineDuckDB, EngineSQLite:
		return true
	}
	return false
}

// Options 对应 config.DuckLakeConfig，由 app 装配层映射，本包不依赖 config。
type Options struct {
	MemoryLimit          string
	Threads              int
	ExtensionDir         string
	DataInliningRowLimit int
	ParquetCompression   string
	TargetFileSize       string
	RequireCommitMessage bool
	LakeAlias            string
	// CatalogEngine 决定本地 catalog 文件、ATTACH 语法与快照 key 后缀
	//（ducklake-duckdb-catalog-plan §3）。实例级、一次性选择。
	CatalogEngine string

	CatalogSync CatalogSyncOptions
	Maintenance MaintenanceOptions
}

// CatalogSyncOptions 控制 catalog.sqlite 同步策略。Phase 1 仅本地空实现。
type CatalogSyncOptions struct {
	Mode         string // interval | debounce
	Debounce     time.Duration
	Interval     time.Duration
	MaxLag       time.Duration
	KeepVersions int
}

// MaintenanceOptions 供 Phase 3 周期任务使用；Phase 1 只保存默认值。
type MaintenanceOptions struct {
	CheckpointInterval     time.Duration
	ExpireOlderThan        time.Duration
	DeleteOlderThan        time.Duration
	RewriteDeleteThreshold float64
}

// DefaultOptions 返回 §4.8 的产品默认值。
func DefaultOptions() Options {
	return Options{
		MemoryLimit:          "512MB",
		Threads:              2,
		DataInliningRowLimit: 1000,
		ParquetCompression:   "zstd",
		TargetFileSize:       "64MB",
		RequireCommitMessage: false,
		LakeAlias:            DefaultLakeAlias,
		CatalogEngine:        EngineDuckDB,
		CatalogSync: CatalogSyncOptions{
			Mode:         "interval",
			Debounce:     200 * time.Millisecond,
			Interval:     15 * time.Second,
			MaxLag:       30 * time.Second,
			KeepVersions: 10,
		},
		Maintenance: MaintenanceOptions{
			CheckpointInterval:     time.Hour,
			ExpireOlderThan:        7 * 24 * time.Hour,
			DeleteOlderThan:        7 * 24 * time.Hour,
			RewriteDeleteThreshold: 0.95,
		},
	}
}

func (o Options) normalized() Options {
	def := DefaultOptions()
	if o.MemoryLimit == "" {
		o.MemoryLimit = def.MemoryLimit
	}
	if o.Threads <= 0 {
		o.Threads = def.Threads
	}
	if o.DataInliningRowLimit < 0 {
		o.DataInliningRowLimit = def.DataInliningRowLimit
	}
	if o.ParquetCompression == "" {
		o.ParquetCompression = def.ParquetCompression
	}
	if o.TargetFileSize == "" {
		o.TargetFileSize = def.TargetFileSize
	}
	if o.LakeAlias == "" {
		o.LakeAlias = def.LakeAlias
	}
	o.CatalogEngine = NormalizeCatalogEngine(o.CatalogEngine)
	if o.CatalogSync.Mode == "" {
		o.CatalogSync.Mode = def.CatalogSync.Mode
	}
	if o.CatalogSync.Debounce <= 0 {
		o.CatalogSync.Debounce = def.CatalogSync.Debounce
	}
	if o.CatalogSync.Interval <= 0 {
		o.CatalogSync.Interval = def.CatalogSync.Interval
	}
	if o.CatalogSync.MaxLag <= 0 {
		o.CatalogSync.MaxLag = def.CatalogSync.MaxLag
	}
	if o.CatalogSync.KeepVersions <= 0 {
		o.CatalogSync.KeepVersions = def.CatalogSync.KeepVersions
	}
	if o.Maintenance.CheckpointInterval <= 0 {
		o.Maintenance.CheckpointInterval = def.Maintenance.CheckpointInterval
	}
	if o.Maintenance.ExpireOlderThan <= 0 {
		o.Maintenance.ExpireOlderThan = def.Maintenance.ExpireOlderThan
	}
	if o.Maintenance.DeleteOlderThan <= 0 {
		o.Maintenance.DeleteOlderThan = def.Maintenance.DeleteOlderThan
	}
	if o.Maintenance.RewriteDeleteThreshold <= 0 {
		o.Maintenance.RewriteDeleteThreshold = def.Maintenance.RewriteDeleteThreshold
	}
	return o
}
