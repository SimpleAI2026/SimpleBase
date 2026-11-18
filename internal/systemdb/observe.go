// 文件说明：observe.go — planv5.0 §4 P0.1/P0.2 的系统库读路径低基数观测
//
// 用法
//
//	同包调用：observeRead() → agents.go, logs.go 等；observeReadPart() → agents.go, logs.go 等
//
// 调用架构
//
//	[上游] godatabase/bench, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/observe.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//	[关键引用] observeRead() ← agents.go, logs.go, metrics.go
//
// 主要符号
//
//	... 另有 2 个顶层符号

package systemdb

import (
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
