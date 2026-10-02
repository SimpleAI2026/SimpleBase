// stage_timing_metrics.go 提供 perfStageMiddleware 的 Prometheus 输出端
//（api-db-perf-validation-plan §2.1）。指标在 Metrics 上定义，本文件只做
// 观测转发，保持 api → observability 单向依赖。
package observability

// PerfStageBuckets 覆盖 1ms–40s：上限须容纳租约 TTL+Grace=40s 等待尾部
//（plan §2.1；prometheus.DefBuckets 最大 10s 不可用）。
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
