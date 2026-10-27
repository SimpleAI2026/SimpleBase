// 文件说明：stage_timing_metrics.go — perfStageMiddleware 的 Prometheus 输出端
//
// 用法
//
//	(m *Metrics) ObserveStage(route string, stage string, seconds float64)  // ObserveStage 满足 internal/api 的 stageTimingRecorder 接口
//	同包调用：PerfStageBuckets → metrics.go
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/audit 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/observability/stage_timing_metrics.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] logger.go, metrics.go, zaplog.go
//	[关键引用] PerfStageBuckets ← metrics.go
//
// 主要符号
//
//	var PerfStageBuckets
//	(m *Metrics) ObserveStage(route string, stage string, seconds float64)

package observability

// PerfStageBuckets 覆盖 1ms–40s：上限须容纳租约 TTL+Grace=40s 等待尾部
// （plan §2.1；prometheus.DefBuckets 最大 10s 不可用）。
var PerfStageBuckets = []float64{
	0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5,
	1, 2.5, 5, 10, 15, 20, 30, 40,
}

// ObserveStage 满足 internal/api 的 stageTimingRecorder 接口。
// route/stage 都是固定低基数集合；禁止把 SQL 文本或 ID 作 label。
func (m *Metrics) ObserveStage(route string, stage string, seconds float64) {
	if m == nil || m.APIStageSeconds == nil {
		return
	}
	m.APIStageSeconds.WithLabelValues(route, stage).Observe(seconds)
}
