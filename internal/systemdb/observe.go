// observe.go 实现 planv5.0 §4 P0.1/P0.2 的系统库读路径低基数观测。
//
// Store 的关键读入口（QueryLogs、MetricsSummary 等）此前只有调用方总耗时，
// 无法区分「SQL 执行」与「扫描/资源等待」。本文件提供：
//   - observeRead：按固定 operation 标签记录耗时与行数（debug 日志，
//     不落 Prometheus——systemdb 包不依赖 observability 注册器）；
//   - ConnStats：连接池水位快照（open/in_use/idle/max_open 与累计等待），
//     供 api 层窗口差值评估整池拥塞。
//
// 安全约束：operation 是固定集合；行数只记录总数；禁止输出 SQL/参数/
// 项目 ID/用户身份。
package systemdb

import (
	"database/sql"
	"time"

	"go.uber.org/zap"
)

// observeRead 记录一次系统库读的耗时与返回行数（logger 为 nil 时 no-op）。
// operation 必须是固定低基数集合（如 "query_logs"、"metrics_summary"）。
func (s *Store) observeRead(operation string, d time.Duration, rows int, err error) {
	if s == nil || s.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("operation", operation),
		zap.Duration("duration", d),
		zap.Int("rows", rows),
	}
	if err != nil {
		// 只记录错误类别标记，不落错误详情（可能含 SQL 上下文）。
		fields = append(fields, zap.Bool("error", true))
	}
	s.logger.Debug("systemdb read", fields...)
}

// ConnStats 返回连接池水位快照（planv5.0 §4 P0.2）。
// db 为 nil 时返回零值。
func ConnStats(db *sql.DB) sql.DBStats {
	if db == nil {
		return sql.DBStats{}
	}
	return db.Stats()
}

// ObserveConnWait 计算两次采样间的新增连接等待（窗口差）。
// 全局累计值不可归因到单个请求，只能用于整池拥塞评估（planv5.0 §4 P0.2）。
func ObserveConnWait(before, after sql.DBStats) (count int64, duration time.Duration) {
	if after.WaitCount < before.WaitCount {
		return 0, 0
	}
	return after.WaitCount - before.WaitCount, after.WaitDuration - before.WaitDuration
}
