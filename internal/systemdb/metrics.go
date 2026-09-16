package systemdb

import (
	"context"
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

// RecordMetric 缓冲一条指标样本（异步 flush）。
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
	s.mu.Lock()
	s.metricsBuf = append(s.metricsBuf, sample)
	n := len(s.metricsBuf)
	s.mu.Unlock()
	if n >= 64 {
		_ = s.FlushMetrics(context.Background())
	}
}

// FlushMetrics 将缓冲样本写入 sys_metric_samples。
func (s *Store) FlushMetrics(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	batch := s.metricsBuf
	s.metricsBuf = nil
	s.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	for _, m := range batch {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO sys_metric_samples(id, project_id, name, value_double, labels_json, occurred_at)
			 VALUES(?, ?, ?, ?, ?, ?)`,
			m.ID, m.ProjectID, m.Name, m.Value, m.LabelsJSON, m.OccurredAt.UTC()); err != nil {
			return err
		}
	}
	s.notifyWrite(ctx)
	return nil
}

// MetricsSummary 聚合项目近期样本与库数量。
func (s *Store) MetricsSummary(ctx context.Context, projectID string) (MetricsSummary, error) {
	var out MetricsSummary
	if s == nil || s.db == nil {
		return out, ErrUnavailable
	}
	since := time.Now().UTC().Add(-24 * time.Hour)
	var requests, errors float64
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(value_double), 0) FROM sys_metric_samples
		 WHERE project_id = ? AND name = 'http_requests' AND occurred_at >= ?`,
		projectID, since).Scan(&requests)
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(value_double), 0) FROM sys_metric_samples
		 WHERE project_id = ? AND name = 'http_errors' AND occurred_at >= ?`,
		projectID, since).Scan(&errors)
	var avg float64
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(AVG(value_double), 0) FROM sys_metric_samples
		 WHERE project_id = ? AND name = 'http_latency_ms' AND occurred_at >= ?`,
		projectID, since).Scan(&avg)
	var active int64
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sys_databases WHERE project_id = ? AND deleted_at IS NULL AND kind = 'user'`,
		projectID).Scan(&active)
	out.TotalRequests = int64(requests)
	if requests > 0 {
		out.ErrorRate = errors / requests
	}
	out.AvgLatencyMS = avg
	out.ActiveDatabases = active
	return out, nil
}

// MetricsTrend 按天聚合最近 days 天的请求/错误。
func (s *Store) MetricsTrend(ctx context.Context, projectID string, days int) ([]TrendPoint, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if days <= 0 || days > 90 {
		days = 7
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	rows, err := s.db.QueryContext(ctx,
		`SELECT CAST(occurred_at AS DATE), name, COALESCE(SUM(value_double), 0)
		 FROM sys_metric_samples
		 WHERE project_id = ? AND occurred_at >= ? AND name IN ('http_requests', 'http_errors')
		 GROUP BY CAST(occurred_at AS DATE), name
		 ORDER BY 1 ASC`,
		projectID, since)
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
