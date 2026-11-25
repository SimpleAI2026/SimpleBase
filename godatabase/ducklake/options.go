// 文件说明：options.go — godatabase/ducklake 中 NormalizeCatalogEngine、IsValidCatalogEngine 等符号的实现
//
// 用法
//
//	NormalizeCatalogEngine(engine string) string  // NormalizeCatalogEngine normalizes a configured catalog engine.
//	IsValidCatalogEngine(engine string) bool  // IsValidCatalogEngine reports whether engine is supported.
//	type Options struct{…}  // Options configures a DuckLake factory without depending on host configuration ty...
//	type CatalogSyncOptions struct{…}  // CatalogSyncOptions controls catalog snapshot publication.
//	type MaintenanceOptions struct{…}  // MaintenanceOptions controls periodic DuckLake maintenance.
//	DefaultOptions() Options  // DefaultOptions returns the standalone library defaults.
//
// 调用架构
//
//	[上游] internal/app, internal/systemdb
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/ducklake/options.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] c4_syncer_lease_test.go, c4_syncer_ordering_test.go, catalog_syncer.go, catalog_syncer_test.go, coverage_test.go 等 35 个
//	[关键引用] NormalizeCatalogEngine ← catalog_syncer.go, extension.go；IsValidCatalogEngine ← factory.go, instance_format.go；Options ← coverage_test.go, engine_test.go
//
// 主要符号
//
//	const DefaultLakeAlias
//	const EngineDuckDB / EngineSQLite
//	NormalizeCatalogEngine(engine string) string
//	IsValidCatalogEngine(engine string) bool
//	type Options struct{…}
//	type CatalogSyncOptions struct{…}
//	(o CatalogSyncOptions) nextDelay() time.Duration
//	type MaintenanceOptions struct{…}
//	... 另有 6 个顶层符号

package ducklake

import (
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/godatabase"
)

// DefaultLakeAlias is the fixed DuckLake ATTACH alias.
const DefaultLakeAlias = "lake"

const (
	EngineDuckDB = godatabase.EngineDuckDB
	EngineSQLite = godatabase.EngineSQLite
)

// NormalizeCatalogEngine normalizes a configured catalog engine.
func NormalizeCatalogEngine(engine string) string {
	normalized := strings.ToLower(strings.TrimSpace(engine))
	if normalized == "" {
		return EngineDuckDB
	}
	return normalized
}

// IsValidCatalogEngine reports whether engine is supported.
func IsValidCatalogEngine(engine string) bool {
	switch NormalizeCatalogEngine(engine) {
	case EngineDuckDB, EngineSQLite:
		return true
	default:
		return false
	}
}

// Options configures a DuckLake factory without depending on host configuration types.
type Options struct {
	MemoryLimit          string
	Threads              int
	ExtensionDir         string
	DataInliningRowLimit int
	ParquetCompression   string
	TargetFileSize       string
	RequireCommitMessage bool
	LakeAlias            string
	CatalogEngine        string
	CatalogSync          CatalogSyncOptions
	Maintenance          MaintenanceOptions
}

// CatalogSyncOptions controls catalog snapshot publication.
type CatalogSyncOptions struct {
	Mode         string
	Debounce     time.Duration
	Interval     time.Duration
	MaxLag       time.Duration
	KeepVersions int
}

// nextDelay 是同步调度的唯一延迟策略：debounce 模式用 Debounce；
// interval 模式用 min(Interval, MaxLag)（连续写入不推迟已有计时器，
// 只在无计时器时按此延迟预约）。失败退避（>=1s）由调用方叠加。
func (o CatalogSyncOptions) nextDelay() time.Duration {
	if o.Mode == "interval" {
		delay := o.Interval
		if o.MaxLag < delay {
			delay = o.MaxLag
		}
		return delay
	}
	return o.Debounce
}

// MaintenanceOptions controls periodic DuckLake maintenance.
type MaintenanceOptions struct {
	CheckpointInterval     time.Duration
	ExpireOlderThan        time.Duration
	DeleteOlderThan        time.Duration
	RewriteDeleteThreshold float64
}

func defaultCatalogSyncOptions() CatalogSyncOptions {
	return CatalogSyncOptions{
		Mode:         "interval",
		Debounce:     200 * time.Millisecond,
		Interval:     15 * time.Second,
		MaxLag:       30 * time.Second,
		KeepVersions: 10,
	}
}

func defaultMaintenanceOptions() MaintenanceOptions {
	return MaintenanceOptions{
		CheckpointInterval:     time.Hour,
		ExpireOlderThan:        7 * 24 * time.Hour,
		DeleteOlderThan:        7 * 24 * time.Hour,
		RewriteDeleteThreshold: 0.95,
	}
}

// DefaultOptions returns the standalone library defaults.
func DefaultOptions() Options {
	return Options{
		MemoryLimit:          "512MB",
		Threads:              2,
		DataInliningRowLimit: 1000,
		ParquetCompression:   "zstd",
		TargetFileSize:       "64MB",
		LakeAlias:            DefaultLakeAlias,
		CatalogEngine:        EngineDuckDB,
		CatalogSync:          defaultCatalogSyncOptions(),
		Maintenance:          defaultMaintenanceOptions(),
	}
}

func (o Options) normalized() Options {
	defaults := DefaultOptions()
	if o.MemoryLimit == "" {
		o.MemoryLimit = defaults.MemoryLimit
	}
	if o.Threads <= 0 {
		o.Threads = defaults.Threads
	}
	if o.DataInliningRowLimit < 0 {
		o.DataInliningRowLimit = defaults.DataInliningRowLimit
	}
	if o.ParquetCompression == "" {
		o.ParquetCompression = defaults.ParquetCompression
	}
	if o.TargetFileSize == "" {
		o.TargetFileSize = defaults.TargetFileSize
	}
	if o.LakeAlias == "" {
		o.LakeAlias = defaults.LakeAlias
	}
	o.CatalogEngine = NormalizeCatalogEngine(o.CatalogEngine)
	o.CatalogSync = o.CatalogSync.normalized()
	o.Maintenance = o.Maintenance.normalized()
	return o
}

func (o CatalogSyncOptions) normalized() CatalogSyncOptions {
	defaults := defaultCatalogSyncOptions()
	if o.Mode == "" {
		o.Mode = defaults.Mode
	}
	if o.Debounce <= 0 {
		o.Debounce = defaults.Debounce
	}
	if o.Interval <= 0 {
		o.Interval = defaults.Interval
	}
	if o.MaxLag <= 0 {
		o.MaxLag = defaults.MaxLag
	}
	if o.KeepVersions <= 0 {
		o.KeepVersions = defaults.KeepVersions
	}
	return o
}

func (o MaintenanceOptions) normalized() MaintenanceOptions {
	defaults := defaultMaintenanceOptions()
	if o.CheckpointInterval <= 0 {
		o.CheckpointInterval = defaults.CheckpointInterval
	}
	if o.ExpireOlderThan <= 0 {
		o.ExpireOlderThan = defaults.ExpireOlderThan
	}
	if o.DeleteOlderThan <= 0 {
		o.DeleteOlderThan = defaults.DeleteOlderThan
	}
	if o.RewriteDeleteThreshold <= 0 {
		o.RewriteDeleteThreshold = defaults.RewriteDeleteThreshold
	}
	return o
}
