// async_flush.go 实现 §7.2 P3：请求路径永不 flush、后台批量写、指标预聚合。
//
// 改造前：Record* 缓冲到 64 条时由当前请求 goroutine 同步逐条 INSERT
//（每条一个事务），长尾 max 300–560ms 且独占系统库单连接。
// 改造后：
//   - Record* 只入队并非阻塞通知后台 flusher（O(1) 请求路径）；
//   - flusher 单事务批量 INSERT（64 行 1 事务），notifyWrite 每批 1 次；
//   - 指标按 (project, name) 在内存聚合 count/sum/max，flush 周期落盘
//     聚合行，写入量约降 2 个量级；查询口径（Summary/Trend）仍读
//     sys_metric_samples 的 SUM(value_double)，与单样本行兼容。
package systemdb

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// flushQueueCap 是日志/指标缓冲的硬上限；超出即丢弃并计数（请求路径 O(1)）。
const flushQueueCap = 10000

// asyncFlusher 是日志/指标共用的后台刷写器。
// 信号通道容量 1：入队方非阻塞通知；flusher 空闲时立即处理，忙碌时合并。
type asyncFlusher struct {
	signal   chan struct{}
	dropped  int64 // 丢弃计数（诊断用；无锁读近似值）
	mu       sync.Mutex
	closed   bool
	flushAll func(ctx context.Context) error
}

func newAsyncFlusher(flushAll func(ctx context.Context) error) *asyncFlusher {
	return &asyncFlusher{
		signal:   make(chan struct{}, 1),
		flushAll: flushAll,
	}
}

// start 启动后台循环；返回 stop 函数（排空后退出）。
func (f *asyncFlusher) start() (stop func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				// 收尾：停止接收后再排空一次，尽量不丢尾部数据。
				_ = f.flushAll(context.Background())
				return
			case <-f.signal:
				_ = f.flushAll(context.Background())
			}
		}
	}()
	return func(stopCtx context.Context) {
		cancel()
		select {
		case <-done:
		case <-stopCtx.Done():
		}
	}
}

// notify 非阻塞通知 flusher（忙碌时信号合并，不阻塞调用方）。
func (f *asyncFlusher) notify() {
	select {
	case f.signal <- struct{}{}:
	default:
	}
}

// drainQueued 批量取空缓冲（由 Store.mu 保护调用约定）。
// 返回 nil 切片表示无待写数据。
func (s *Store) drainLogs() []LogEvent {
	s.mu.Lock()
	batch := s.logBuf
	s.logBuf = nil
	s.mu.Unlock()
	return batch
}

func (s *Store) drainMetrics() []MetricSample {
	s.mu.Lock()
	batch := s.metricsBuf
	s.metricsBuf = nil
	s.mu.Unlock()
	return batch
}

// execer 抽象 *sql.DB 与 *sql.Tx 的执行能力（单事务合批用）。
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertLogs 向 ex 批量插入日志行；失败由调用方负责退回缓冲。
func insertLogs(ctx context.Context, ex execer, batch []LogEvent) error {
	var sb strings.Builder
	sb.WriteString(`INSERT INTO sys_log_events(id, project_id, level, logger, message, fields_json, request_id, occurred_at) VALUES `)
	args := make([]any, 0, len(batch)*8)
	for i, e := range batch {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?, ?, ?, ?, ?, ?, ?, ?)")
		var project any
		if e.ProjectID != "" {
			project = e.ProjectID
		}
		args = append(args, e.ID, project, e.Level, e.Logger, e.Message, e.FieldsJSON, e.RequestID, e.OccurredAt.UTC())
	}
	_, err := ex.ExecContext(ctx, sb.String(), args...)
	return err
}

// insertMetrics 向 ex 批量插入指标样本行；失败由调用方负责退回缓冲。
func insertMetrics(ctx context.Context, ex execer, batch []MetricSample) error {
	var sb strings.Builder
	sb.WriteString(`INSERT INTO sys_metric_samples(id, project_id, name, value_double, labels_json, occurred_at) VALUES `)
	args := make([]any, 0, len(batch)*6)
	for i, m := range batch {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?, ?, ?, ?, ?, ?)")
		args = append(args, m.ID, m.ProjectID, m.Name, m.Value, m.LabelsJSON, m.OccurredAt.UTC())
	}
	_, err := ex.ExecContext(ctx, sb.String(), args...)
	return err
}

// flushLogsBatch 单事务批量写日志（P3.2）；空批 no-op。
func (s *Store) flushLogsBatch(ctx context.Context, batch []LogEvent) error {
	if s == nil || s.db == nil || len(batch) == 0 {
		return nil
	}
	if err := insertLogs(ctx, s.db, batch); err != nil {
		// 批量失败：退回缓冲（下轮重试），避免整批丢失。
		s.mu.Lock()
		s.logBuf = append(batch, s.logBuf...)
		s.mu.Unlock()
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// flushMetricsBatch 单事务批量写指标样本（P3.2）。
func (s *Store) flushMetricsBatch(ctx context.Context, batch []MetricSample) error {
	if s == nil || s.db == nil || len(batch) == 0 {
		return nil
	}
	if err := insertMetrics(ctx, s.db, batch); err != nil {
		s.mu.Lock()
		s.metricsBuf = append(batch, s.metricsBuf...)
		s.mu.Unlock()
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// —— 指标预聚合（P3.3）——
//
// http_requests / http_errors 的 value 恒为 1，http_latency_ms 每请求一条。
// 聚合后每 (project, name) 每周期落一行：requests/errors 落 SUM，
// latency 落 (count, sum) 两行（name 加后缀区分），写入量 ~100×↓。
// Summary/Trend 的 SUM 口径对 count 行不敏感（count 行独立 name，不参与聚合）。

type metricAggregate struct {
	Count      float64
	Sum        float64
	OccurredAt time.Time
}

// recordMetricAggregated 按 (projectID, name) 聚合；超过容量直接透传原始样本。
func (s *Store) recordMetricAggregated(sample MetricSample) {
	switch sample.Name {
	case "http_requests", "http_errors", "http_latency_ms":
	default:
		// 非热指标：维持原逐条缓冲。
		s.mu.Lock()
		s.metricsBuf = append(s.metricsBuf, sample)
		n := len(s.metricsBuf)
		s.mu.Unlock()
		if n >= 64 {
			s.asyncNotifyLocked()
		}
		return
	}
	s.mu.Lock()
	if s.metricAgg == nil {
		s.metricAgg = map[string]metricAggregate{}
	}
	key := sample.ProjectID + "\x00" + sample.Name
	agg := s.metricAgg[key]
	agg.Count++
	agg.Sum += sample.Value
	if agg.OccurredAt.IsZero() {
		agg.OccurredAt = sample.OccurredAt
	}
	s.metricAgg[key] = agg
	if sample.Name == "http_latency_ms" {
		if s.latencyHist == nil {
			s.latencyHist = map[string]latencyHistogram{}
		}
		h, ok := s.latencyHist[sample.ProjectID]
		if !ok {
			h = newLatencyHistogram()
			s.latencyHist[sample.ProjectID] = h
		}
		h.observe(sample.Value)
	}
	s.mu.Unlock()
}

// drainMetricAggregates 把聚合桶折算为落盘样本行：
//   - http_requests/http_errors → 一行 value=Sum（即请求数）
//   - http_latency_ms → 一行 value=Sum（总毫秒）+ 一行 name 加 _count 后缀（请求数）
func (s *Store) drainMetricAggregates() []MetricSample {
	s.mu.Lock()
	aggs := s.metricAgg
	s.metricAgg = nil
	hists := s.latencyHist
	s.latencyHist = nil
	s.mu.Unlock()
	if len(aggs) == 0 && len(hists) == 0 {
		return nil
	}
	out := make([]MetricSample, 0, len(aggs)*2+len(hists))
	now := time.Now().UTC()
	// 直方图：每项目一行；失败时随 metricsBuf 原样重入队，查询侧逐桶相加，不会覆盖。
	for projectID, h := range hists {
		out = append(out, MetricSample{
			ID: uuid.NewString(), ProjectID: projectID, Name: metricLatencyHistogram,
			Value: float64(h.total()), LabelsJSON: h.encode(), OccurredAt: now,
		})
	}
	for key, agg := range aggs {
		idx := strings.IndexByte(key, 0)
		if idx < 0 {
			continue
		}
		projectID, name := key[:idx], key[idx+1:]
		out = append(out, MetricSample{
			ProjectID: projectID, Name: name, Value: agg.Sum, OccurredAt: now,
		})
		if name == "http_latency_ms" {
			out = append(out, MetricSample{
				ProjectID: projectID, Name: name + "_count", Value: agg.Count, OccurredAt: now,
			})
		}
	}
	return out
}

// flushAllAsync 后台 flusher 的统一入口：日志批 + 指标批 + 聚合批。
// perf §1 P1-B：三类数据合入同一事务提交，末尾仅 notifyWrite 一次，
// 避免一轮刷写产生多个 DuckLake snapshot / 多次 catalog 同步触发。
func (s *Store) flushAllAsync(ctx context.Context) error {
	logs := s.drainLogs()
	metrics := append(s.drainMetrics(), s.drainMetricAggregates()...)
	if len(logs) == 0 && len(metrics) == 0 {
		return nil
	}
	if s == nil || s.db == nil {
		// 连接不可用：退回缓冲等待下轮。
		s.requeue(logs, metrics)
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.requeue(logs, metrics)
		return err
	}
	if len(logs) > 0 {
		if err := insertLogs(ctx, tx, logs); err != nil {
			_ = tx.Rollback()
			s.requeue(logs, metrics)
			return err
		}
	}
	if len(metrics) > 0 {
		if err := insertMetrics(ctx, tx, metrics); err != nil {
			_ = tx.Rollback()
			s.requeue(logs, metrics)
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		s.requeue(logs, metrics)
		return err
	}
	s.notifyWrite(ctx)
	return nil
}

// requeue 把未落盘的批次退回缓冲头部（下轮重试，避免整批丢失）。
func (s *Store) requeue(logs []LogEvent, metrics []MetricSample) {
	s.mu.Lock()
	if len(logs) > 0 {
		s.logBuf = append(logs, s.logBuf...)
	}
	if len(metrics) > 0 {
		s.metricsBuf = append(metrics, s.metricsBuf...)
	}
	s.mu.Unlock()
}
