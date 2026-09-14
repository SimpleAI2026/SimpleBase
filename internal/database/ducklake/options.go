package ducklake

import "time"

// DefaultLakeAlias 是每个 DuckDB 实例 ATTACH DuckLake 时使用的固定别名。
// 用户 SQL 无法 ATTACH/USE，因此不会与用户对象冲突。
const DefaultLakeAlias = "lake"

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

	CatalogSync CatalogSyncOptions
	Maintenance MaintenanceOptions
}

// CatalogSyncOptions 控制 catalog.sqlite 同步策略。Phase 1 仅本地空实现。
type CatalogSyncOptions struct {
	Mode         string // debounce | sync_on_commit
	Debounce     time.Duration
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
		DataInliningRowLimit: 100,
		ParquetCompression:   "zstd",
		TargetFileSize:       "64MB",
		RequireCommitMessage: false,
		LakeAlias:            DefaultLakeAlias,
		CatalogSync: CatalogSyncOptions{
			Mode:         "debounce",
			Debounce:     200 * time.Millisecond,
			KeepVersions: 10,
		},
		Maintenance: MaintenanceOptions{
			CheckpointInterval:     time.Hour,
			ExpireOlderThan:        7 * 24 * time.Hour,
			DeleteOlderThan:        24 * time.Hour,
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
	if o.CatalogSync.Mode == "" {
		o.CatalogSync.Mode = def.CatalogSync.Mode
	}
	if o.CatalogSync.Debounce <= 0 {
		o.CatalogSync.Debounce = def.CatalogSync.Debounce
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
