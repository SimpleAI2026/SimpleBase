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
