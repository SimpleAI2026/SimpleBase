package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 汇总 SimpleBase 各模块的低基数 Prometheus 指标。
// 禁止把 databaseID/tenantID/projectID/requestID/SQL 文本作为 label。
type Metrics struct {
	HTTPRequests         *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	DBOpenHandles        prometheus.Gauge
	DBOpenSeconds        *prometheus.HistogramVec
	DBQuerySeconds       *prometheus.HistogramVec
	S3Operations         *prometheus.CounterVec
	S3OperationSeconds   *prometheus.HistogramVec
	CacheBytes           prometheus.Gauge
	CacheEvictions       prometheus.Counter
	LLMRequests          *prometheus.CounterVec
	LLMFirstTokenSeconds *prometheus.HistogramVec
	LLMTokens            *prometheus.CounterVec
	CatalogSyncTotal     *prometheus.CounterVec
	CatalogSyncFailures  prometheus.Counter
	CatalogSyncDuration  prometheus.Histogram
	CatalogSyncLag       *prometheus.GaugeVec

	// 多实例一致性（multi-instance-consistency-plan §4.9）。
	LeaseState         *prometheus.GaugeVec   // 1=持租 / 0=未持租
	LeaseRenewFailures *prometheus.CounterVec // 续约失败
	LeaseLost          *prometheus.CounterVec // 失租（释放句柄）
	LeaseEpoch         *prometheus.GaugeVec   // 当前租约 epoch
	LeaseHeldRejected  *prometheus.CounterVec // 他人持租时被拒的 Acquire
	CatalogSyncConflicts *prometheus.CounterVec // PutIfAbsent 冲突（split-brain 确证）
	CatalogLocalAhead    prometheus.Counter     // 本地领先远端（§3.8 路径被拦截）
	CatalogInlinedRows   *prometheus.GaugeVec   // catalog 内联行数（P0-0 后应恒为 0）
	CatalogSizeBytes     *prometheus.GaugeVec   // catalog 文件体积（P4 决策前置信号）
	CASSupported         prometheus.Gauge       // 启动探针结果：1=条件写可用
	CatalogEngineMismatch prometheus.Gauge      // catalog 引擎与已有数据不一致：1=拒绝启动
	MaintenanceSkipped   *prometheus.CounterVec // 维护任务跳过（no_lease / sync_lag）

	// 分段计时（api-db-perf-validation-plan §2.1）。route/stage 均为低基数。
	APIStageSeconds *prometheus.HistogramVec
}

// ObserveCacheBytes 更新当前缓存字节用量。
// 满足 internal/database/cache.CacheMetrics 接口。
func (m *Metrics) ObserveCacheBytes(bytes int64) {
	m.CacheBytes.Set(float64(bytes))
}

// IncCacheEvictions 增加缓存淘汰计数。
// 满足 internal/database/cache.CacheMetrics 接口。
func (m *Metrics) IncCacheEvictions() {
	m.CacheEvictions.Inc()
}

// NewMetrics 在给定 Registerer 上注册指标。若 registerer 为 nil，使用默认 registry。
func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	f := promauto.With(reg)
	return &Metrics{
		HTTPRequests: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_http_requests_total",
			Help: "Number of HTTP requests by route, method and status.",
		}, []string{"route", "method", "status"}),
		HTTPRequestDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "simplebase_http_request_seconds",
			Help:    "HTTP request latency.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "method"}),
		DBOpenHandles: f.NewGauge(prometheus.GaugeOpts{
			Name: "simplebase_database_open_handles",
			Help: "Current number of open database handles.",
		}),
		DBOpenSeconds: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "simplebase_database_open_seconds",
			Help:    "Time to open a database handle.",
			Buckets: prometheus.DefBuckets,
		}, []string{"outcome"}),
		DBQuerySeconds: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "simplebase_database_query_seconds",
			Help:    "Database query/execute latency.",
			Buckets: prometheus.DefBuckets,
		}, []string{"kind", "outcome"}),
		S3Operations: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_s3_operations_total",
			Help: "S3 operations by type and outcome.",
		}, []string{"operation", "outcome"}),
		S3OperationSeconds: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "simplebase_s3_operation_seconds",
			Help:    "S3 operation latency.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
		CacheBytes: f.NewGauge(prometheus.GaugeOpts{
			Name: "simplebase_cache_bytes",
			Help: "Current local cache usage in bytes.",
		}),
		CacheEvictions: f.NewCounter(prometheus.CounterOpts{
			Name: "simplebase_cache_evictions_total",
			Help: "Number of cache directory evictions.",
		}),
		LLMRequests: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_llm_requests_total",
			Help: "LLM requests by provider, model and outcome.",
		}, []string{"provider", "model", "outcome"}),
		LLMFirstTokenSeconds: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "simplebase_llm_first_token_seconds",
			Help:    "Time to first LLM stream token.",
			Buckets: prometheus.DefBuckets,
		}, []string{"provider", "model"}),
		LLMTokens: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_llm_tokens_total",
			Help: "LLM token usage.",
		}, []string{"provider", "model", "direction"}),
		CatalogSyncTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_catalog_sync_total",
			Help: "DuckLake catalog sync attempts by outcome",
		}, []string{"outcome"}),
		CatalogSyncFailures: f.NewCounter(prometheus.CounterOpts{
			Name: "simplebase_catalog_sync_failures_total",
			Help: "DuckLake catalog sync failures",
		}),
		CatalogSyncDuration: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "simplebase_catalog_sync_duration_seconds",
			Help:    "DuckLake catalog sync duration",
			Buckets: prometheus.DefBuckets,
		}),
		CatalogSyncLag: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "simplebase_catalog_sync_lag_snapshots",
			Help: "DuckLake catalog sync lag in snapshot ids",
		}, []string{"database_id"}),
		LeaseState: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "simplebase_instance_lease_state",
			Help: "Per-database write lease state: 1=held, 0=not held.",
		}, []string{"database_id"}),
		LeaseRenewFailures: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_instance_lease_renew_failures_total",
			Help: "Write lease renewal failures by database.",
		}, []string{"database_id"}),
		LeaseLost: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_instance_lease_lost_total",
			Help: "Write leases lost (handle released) by database.",
		}, []string{"database_id"}),
		LeaseEpoch: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "simplebase_instance_lease_epoch",
			Help: "Current write lease epoch by database.",
		}, []string{"database_id"}),
		LeaseHeldRejected: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_instance_lease_held_rejected_total",
			Help: "Acquire attempts rejected because another instance holds the lease.",
		}, []string{"database_id"}),
		CatalogSyncConflicts: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_catalog_sync_conflicts_total",
			Help: "PutIfAbsent conflicts on manifest/snapshot writes (split-brain confirmation).",
		}, []string{"database_id"}),
		CatalogLocalAhead: f.NewCounter(prometheus.CounterOpts{
			Name: "simplebase_catalog_local_ahead_total",
			Help: "Local catalog snapshot ahead of remote at cold start (rollback-overwrite blocked).",
		}),
		CatalogInlinedRows: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "simplebase_catalog_inlined_rows",
			Help: "Rows inlined into catalog SQLite (must be 0 after P0-0 disables remote inlining).",
		}, []string{"database_id"}),
		CatalogSizeBytes: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "simplebase_catalog_size_bytes",
			Help: "DuckLake catalog file size in bytes (early signal for shared-catalog decision).",
		}, []string{"database_id"}),
		CASSupported: f.NewGauge(prometheus.GaugeOpts{
			Name: "simplebase_objectstore_cas_supported",
			Help: "Startup probe result for create-if-absent conditional writes: 1=supported, 0=unsupported.",
		}),
		CatalogEngineMismatch: f.NewGauge(prometheus.GaugeOpts{
			Name: "simplebase_catalog_engine_mismatch",
			Help: "1 when configured catalog engine does not match existing data (startup refused; reset required).",
		}),
		MaintenanceSkipped: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_maintenance_skipped_total",
			Help: "Maintenance runs skipped by reason (no_lease, sync_lag).",
		}, []string{"reason"}),
		APIStageSeconds: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "simplebase_api_stage_seconds",
			Help:    "Per-stage request timing (perf validation); buckets span 1ms to 40s.",
			Buckets: PerfStageBuckets,
		}, []string{"route", "stage"}),
	}
}
