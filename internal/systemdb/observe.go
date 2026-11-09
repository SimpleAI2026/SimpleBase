// 文件说明：observe.go — planv5.0 §4 P0.1/P0.2 的系统库读路径低基数观测
//
// 用法
//
//	ConnStats(db *sql.DB) sql.DBStats  // ConnStats 返回连接池水位快照（planv5.0 §4 P0.2）
//	ObserveConnWait(before, after sql.DBStats) (count int64, duration time.Duration)  // ObserveConnWait 计算两次采样间的新增连接等待（窗口差）
//	同包调用：observeRead() → agents.go, logs.go 等；observeReadPart() → agents.go, logs.go 等
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/observe.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//	[关键引用] observeRead() ← agents.go, logs.go, metrics.go
//
// 主要符号
//
//	ConnStats(db *sql.DB) sql.DBStats
//	ObserveConnWait(before, after sql.DBStats) (count int64, duration time.Duration)
//	... 另有 2 个顶层符号

package systemdb

import (
	"database/sql"
	"time"

	"go.uber.org/zap"
)

// observeRead 记录一次系统库读的耗时与返回行数（logger 为 nil 时 no-op）。
// operation 必须是固定低基数集合（如 "query_logs"、"metrics_summary"、
// "list_agent_messages"、"metrics_trend"）。
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

// observeReadPart 把同一系统库读操作的查询与扫描分开记录。
// part 和 operation 仅传调用点写死的名称，不接受请求参数。
func (s *Store) observeReadPart(operation, part string, d time.Duration, err error) {
	if s == nil || s.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("operation", operation),
		zap.String("part", part),
		zap.Duration("duration", d),
	}
	if err != nil {
		fields = append(fields, zap.Bool("error", true))
	}
	s.logger.Debug("systemdb read stage", fields...)
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
