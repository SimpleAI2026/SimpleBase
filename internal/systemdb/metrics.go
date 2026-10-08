package systemdb

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MetricSample 是写入 sys_metric_samples 的一行。
type MetricSample struct {
	ID         string
	ProjectID  string
	Name       string
	Value      float64
	LabelsJSON string
	OccurredAt time.Time
}

// MetricsSummary 是 GET metrics/summary 的载荷。
// LatencyP*MS 为近 24h 接口耗时估算分位数（固定桶插值）；无直方图样本时为 null。
// 分位数落入超量程桶时取值 = LatencyOverflowMS（UI 据此显示 ≥30s）。
type MetricsSummary struct {
	TotalRequests      int64    `json:"total_requests"`
	ErrorRate          float64  `json:"error_rate"`
	AvgLatencyMS       float64  `json:"avg_latency_ms"`
	ActiveDatabases    int64    `json:"active_databases"`
	LatencyP50MS       *float64 `json:"latency_p50_ms"`
	LatencyP90MS       *float64 `json:"latency_p90_ms"`
	LatencyP99MS       *float64 `json:"latency_p99_ms"`
	LatencySampleCount int64    `json:"latency_sample_count"`
	LatencyOverflowMS  float64  `json:"latency_overflow_ms"`
}

// TrendPoint 是 GET metrics/trend 的一个点。
type TrendPoint struct {
	Date     string `json:"date"`
	Requests int64  `json:"requests"`
	Errors   int64  `json:"errors"`
}

// RecordMetric 缓冲一条指标样本（§7.2 P3：热指标内存预聚合，请求路径 O(1)）。
func (s *Store) RecordMetric(sample MetricSample) {
	if s == nil {
		return
	}
	if sample.ID == "" {
		sample.ID = uuid.NewString()
	}
	if sample.OccurredAt.IsZero() {
		sample.OccurredAt = time.Now().UTC()
	}
	s.recordMetricAggregated(sample)
}

// FlushMetrics 将缓冲样本与预聚合桶写入 sys_metric_samples（批量单事务）。
func (s *Store) FlushMetrics(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	batch := s.drainMetrics()
	agg := s.drainMetricAggregates()
	batch = append(batch, agg...)
	return s.flushMetricsBatch(ctx, batch)
}

// MetricsSummary 聚合项目近期样本与库数量。admin 项目聚合全系统。
// perf §1 P1-D：请求/错误/耗时合并为一条条件聚合，单遍扫描 sys_metric_samples。
// 平均耗时 = SUM(http_latency_ms) / SUM(http_latency_ms_count)（预聚合行加权）；
// 窗口内无 _count 行时（仅旧版逐请求样本）回退为 AVG(http_latency_ms)。
// 查询/扫描错误直接返回，不再吞掉后返回全 0。
func (s *Store) MetricsSummary(ctx context.Context, projectID string) (MetricsSummary, error) {
	out := MetricsSummary{LatencyOverflowMS: LatencyOverflowMS}
	if s == nil || s.db == nil {
		return out, ErrUnavailable
	}
	start := time.Now()
	admin := IsAdminProject(projectID)
	since := time.Now().UTC().Add(-24 * time.Hour)
	var requests, errors, latencySum, latencyCount, latencyAvg float64
	query := `SELECT
		COALESCE(SUM(CASE WHEN name = 'http_requests' THEN value_double END), 0),
		COALESCE(SUM(CASE WHEN name = 'http_errors' THEN value_double END), 0),
		COALESCE(SUM(CASE WHEN name = 'http_latency_ms' THEN value_double END), 0),
		COALESCE(SUM(CASE WHEN name = 'http_latency_ms_count' THEN value_double END), 0),
		COALESCE(AVG(CASE WHEN name = 'http_latency_ms' THEN value_double END), 0)
		FROM sys_metric_samples
		WHERE occurred_at >= ? AND name IN ('http_requests', 'http_errors', 'http_latency_ms', 'http_latency_ms_count')`
	args := []any{since}
	if !admin {
		query += " AND project_id = ?"
		args = append(args, projectID)
	}
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&requests, &errors, &latencySum, &latencyCount, &latencyAvg); err != nil {
		return out, err
	}
	var active int64
	var err error
	if admin {
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sys_databases WHERE deleted_at IS NULL AND kind = 'user'`).Scan(&active)
	} else {
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sys_databases WHERE project_id = ? AND deleted_at IS NULL AND kind = 'user'`,
			projectID).Scan(&active)
	}
	if err != nil {
		return out, err
	}
	hist, err := s.latencyHistogramSince(ctx, projectID, admin, since)
	if err != nil {
		return out, err
	}
	out.TotalRequests = int64(requests)
	if requests > 0 {
		out.ErrorRate = errors / requests
	}
	if latencyCount > 0 {
		out.AvgLatencyMS = latencySum / latencyCount
	} else {
		out.AvgLatencyMS = latencyAvg
	}
	out.ActiveDatabases = active
	out.LatencySampleCount = int64(hist.total())
	out.LatencyP50MS = hist.quantile(0.50)
	out.LatencyP90MS = hist.quantile(0.90)
	out.LatencyP99MS = hist.quantile(0.99)
	s.observeRead("metrics_summary", time.Since(start), int(out.LatencySampleCount), nil)
	return out, nil
}

// latencyHistogramSince 逐桶合并窗口内的直方图行。
// 损坏/未知版本的行不混入结果：跳过并记 warn（显式降级，不影响其余统计）。
func (s *Store) latencyHistogramSince(ctx context.Context, projectID string, admin bool, since time.Time) (latencyHistogram, error) {
	query := `SELECT labels_json, value_double FROM sys_metric_samples WHERE occurred_at >= ? AND name = ?`
	args := []any{since, metricLatencyHistogram}
	if !admin {
		query += " AND project_id = ?"
		args = append(args, projectID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := newLatencyHistogram()
	skipped := 0
	for rows.Next() {
		var labels string
		var total float64
		if err := rows.Scan(&labels, &total); err != nil {
			return nil, err
		}
		h, err := decodeLatencyHistogram(labels, total)
		if err != nil {
			skipped++
			continue
		}
		out.merge(h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if skipped > 0 && s.logger != nil {
		s.logger.Warn("systemdb latency histogram rows skipped",
			zap.String("project_id", projectID), zap.Int("skipped", skipped))
	}
	return out, nil
}

// MetricsSummaryCached / MetricsTrendCached：仪表盘读路径的短 TTL（10s）+ 单飞缓存，
// 避免每次刷新都打系统库唯一连接与远端对象存储（perf §1 P1-D）。
// 指标本身是近似值，写入路径无需主动失效。
func (s *Store) MetricsSummaryCached(ctx context.Context, projectID string) (MetricsSummary, error) {
	s.initReadCaches()
	return s.summaryCache.Do(projectID, func() (MetricsSummary, error) {
		return s.MetricsSummary(ctx, projectID)
	})
}

func (s *Store) MetricsTrendCached(ctx context.Context, projectID string, days int) ([]TrendPoint, error) {
	s.initReadCaches()
	return s.trendCache.Do(projectID+"/"+strconv.Itoa(days), func() ([]TrendPoint, error) {
		return s.MetricsTrend(ctx, projectID, days)
	})
}

// MetricsTrend 按天聚合最近 days 天的请求/错误。admin 项目聚合全系统。
func (s *Store) MetricsTrend(ctx context.Context, projectID string, days int) ([]TrendPoint, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if days <= 0 || days > 90 {
		days = 7
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	query := `SELECT CAST(occurred_at AS DATE), name, COALESCE(SUM(value_double), 0)
		 FROM sys_metric_samples
		 WHERE occurred_at >= ? AND name IN ('http_requests', 'http_errors')
		 GROUP BY CAST(occurred_at AS DATE), name
		 ORDER BY 1 ASC`
	var rows *sql.Rows
	var err error
	if IsAdminProject(projectID) {
		rows, err = s.db.QueryContext(ctx, query, since)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT CAST(occurred_at AS DATE), name, COALESCE(SUM(value_double), 0)
		 FROM sys_metric_samples
		 WHERE project_id = ? AND occurred_at >= ? AND name IN ('http_requests', 'http_errors')
		 GROUP BY CAST(occurred_at AS DATE), name
		 ORDER BY 1 ASC`, projectID, since)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDate := map[string]*TrendPoint{}
	order := make([]string, 0)
	for rows.Next() {
		var day, name string
		var val float64
		if err := rows.Scan(&day, &name, &val); err != nil {
			return nil, err
		}
		p, ok := byDate[day]
		if !ok {
			p = &TrendPoint{Date: day}
			byDate[day] = p
			order = append(order, day)
		}
		switch name {
		case "http_requests":
			p.Requests = int64(val)
		case "http_errors":
			p.Errors = int64(val)
		}
	}
	out := make([]TrendPoint, 0, len(order))
	for _, d := range order {
		out = append(out, *byDate[d])
	}
	return out, rows.Err()
}
