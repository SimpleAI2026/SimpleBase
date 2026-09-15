package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 汇总 SimpleBase 各模块的低基数 Prometheus 指标。
// 禁止把 databaseID/tenantID/projectID/requestID/SQL 文本作为 label。
type Metrics struct {
	HTTPRequests        *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	DBOpenHandles       prometheus.Gauge
	DBOpenSeconds       *prometheus.HistogramVec
	DBQuerySeconds      *prometheus.HistogramVec
	S3Operations        *prometheus.CounterVec
	S3OperationSeconds  *prometheus.HistogramVec
	CacheBytes          prometheus.Gauge
	CacheEvictions      prometheus.Counter
	LLMRequests         *prometheus.CounterVec
	LLMFirstTokenSeconds *prometheus.HistogramVec
	LLMTokens           *prometheus.CounterVec
	JobsTotal           *prometheus.CounterVec
	CatalogSyncTotal    *prometheus.CounterVec
	CatalogSyncFailures prometheus.Counter
	CatalogSyncDuration prometheus.Histogram
	CatalogSyncLag      *prometheus.GaugeVec
	JobDurationSeconds  *prometheus.HistogramVec
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

// IncJobTotal 增加后台任务计数。满足 internal/jobs.JobMetrics 接口。
func (m *Metrics) IncJobTotal(jobType string, outcome string) {
	m.JobsTotal.WithLabelValues(jobType, outcome).Inc()
}

// ObserveJobDuration 记录后台任务耗时。满足 internal/jobs.JobMetrics 接口。
func (m *Metrics) ObserveJobDuration(jobType string, seconds float64) {
	m.JobDurationSeconds.WithLabelValues(jobType).Observe(seconds)
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
		JobsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "simplebase_jobs_total",
			Help: "Background jobs by type and outcome.",
		}, []string{"type", "outcome"}),
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

		JobDurationSeconds: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "simplebase_job_duration_seconds",
			Help:    "Background job duration.",
			Buckets: prometheus.DefBuckets,
		}, []string{"type"}),
	}
}
