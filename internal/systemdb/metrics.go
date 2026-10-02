package systemdb

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/google/uuid"
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
type MetricsSummary struct {
	TotalRequests   int64   `json:"total_requests"`
	ErrorRate       float64 `json:"error_rate"`
	AvgLatencyMS    float64 `json:"avg_latency_ms"`
	ActiveDatabases int64   `json:"active_databases"`
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
// perf §1 P1-D：三条独立聚合合并为一条条件聚合，单遍扫描 sys_metric_samples；
// 口径与旧实现一致（latency 仍是对 name='http_latency_ms' 行的 AVG）。
func (s *Store) MetricsSummary(ctx context.Context, projectID string) (MetricsSummary, error) {
	var out MetricsSummary
	if s == nil || s.db == nil {
		return out, ErrUnavailable
	}
	admin := IsAdminProject(projectID)
	since := time.Now().UTC().Add(-24 * time.Hour)
	var requests, errors, avg float64
	query := `SELECT
		COALESCE(SUM(CASE WHEN name = 'http_requests' THEN value_double END), 0),
		COALESCE(SUM(CASE WHEN name = 'http_errors' THEN value_double END), 0),
		COALESCE(AVG(CASE WHEN name = 'http_latency_ms' THEN value_double END), 0)
		FROM sys_metric_samples
		WHERE occurred_at >= ? AND name IN ('http_requests', 'http_errors', 'http_latency_ms')`
	args := []any{since}
	if !admin {
		query += " AND project_id = ?"
		args = append(args, projectID)
	}
	_ = s.db.QueryRowContext(ctx, query, args...).Scan(&requests, &errors, &avg)
	var active int64
	if admin {
		_ = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sys_databases WHERE deleted_at IS NULL AND kind = 'user'`).Scan(&active)
	} else {
		_ = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sys_databases WHERE project_id = ? AND deleted_at IS NULL AND kind = 'user'`,
			projectID).Scan(&active)
	}
	out.TotalRequests = int64(requests)
	if requests > 0 {
		out.ErrorRate = errors / requests
	}
	out.AvgLatencyMS = avg
	out.ActiveDatabases = active
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
