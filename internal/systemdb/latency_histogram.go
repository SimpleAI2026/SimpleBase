// 文件说明：latency_histogram.go — 接口耗时固定桶直方图（planv4.0 dashboard-api-latency-percentiles-plan）
//
// 用法
//
//	type latencyHistogram（包内使用）
//	type latencyHistogramJSON（包内使用）
//	同包调用：latencyHistogram → async_flush.go, metrics.go 等；metricLatencyHistogram → async_flush.go, metrics.go；newLatencyHistogram() → async_flush.go, metrics.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/latency_histogram.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//	[关键引用] latencyHistogram ← async_flush.go, metrics.go, store.go
//
// 主要符号
//
//	var LatencyOverflowMS
//	... 另有 13 个顶层符号

package systemdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
)

const (
	metricLatencyHistogram = "http_latency_histogram_ms"
	latencyHistogramV1     = 1
)

// latencyBucketsV1 是 v1 的有限桶上界（毫秒，左开右闭）；末尾隐含 +Inf 桶。
// 修改边界必须新增版本号，旧版本行仍按旧边界解码。
var latencyBucketsV1 = []float64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 30000}

// LatencyOverflowMS 是最高有限桶上界；分位数落入 +Inf 桶时返回该下界（UI 显示 ≥30s）。
var LatencyOverflowMS = latencyBucketsV1[len(latencyBucketsV1)-1]

// latencyHistogram 是单个项目的桶计数（长度 = 有限桶数 + 1）。
type latencyHistogram []uint64

func newLatencyHistogram() latencyHistogram {
	return make(latencyHistogram, len(latencyBucketsV1)+1)
}

// observe 记录一个耗时（毫秒）。
func (h latencyHistogram) observe(ms float64) {
	if math.IsNaN(ms) || ms < 0 {
		ms = 0
	}
	i := sort.SearchFloat64s(latencyBucketsV1, ms) // 首个 >= ms 的上界，即左开右闭
	h[i]++
}

func (h latencyHistogram) total() uint64 {
	var n uint64
	for _, c := range h {
		n += c
	}
	return n
}

type latencyHistogramJSON struct {
	V int      `json:"v"`
	B []uint64 `json:"b"`
}

func (h latencyHistogram) encode() string {
	b, _ := json.Marshal(latencyHistogramJSON{V: latencyHistogramV1, B: h})
	return string(b)
}

var errBadLatencyHistogram = errors.New("systemdb: invalid latency histogram row")

// decodeLatencyHistogram 解码并校验一行；total 为该行 value_double。
func decodeLatencyHistogram(labels string, total float64) (latencyHistogram, error) {
	var raw latencyHistogramJSON
	if err := json.Unmarshal([]byte(labels), &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", errBadLatencyHistogram, err)
	}
	if raw.V != latencyHistogramV1 {
		return nil, fmt.Errorf("%w: unsupported version %d", errBadLatencyHistogram, raw.V)
	}
	if len(raw.B) != len(latencyBucketsV1)+1 {
		return nil, fmt.Errorf("%w: bucket count %d", errBadLatencyHistogram, len(raw.B))
	}
	h := latencyHistogram(raw.B)
	if math.IsNaN(total) || math.IsInf(total, 0) || float64(h.total()) != total {
		return nil, fmt.Errorf("%w: total %v != buckets %d", errBadLatencyHistogram, total, h.total())
	}
	return h, nil
}

// merge 逐桶相加。
func (h latencyHistogram) merge(o latencyHistogram) {
	for i := range h {
		h[i] += o[i]
	}
}

// quantile 估算分位数（0<q<=1）；无样本返回 nil。
// 命中有限桶时线性插值；命中 +Inf 桶返回 LatencyOverflowMS。
func (h latencyHistogram) quantile(q float64) *float64 {
	n := h.total()
	if n == 0 {
		return nil
	}
	rank := q * float64(n)
	var cum uint64
	for i, c := range h {
		if c == 0 {
			continue
		}
		if float64(cum+c) >= rank {
			var v float64
			if i == len(latencyBucketsV1) {
				v = LatencyOverflowMS
			} else {
				lower := 0.0
				if i > 0 {
					lower = latencyBucketsV1[i-1]
				}
				upper := latencyBucketsV1[i]
				v = lower + (rank-float64(cum))/float64(c)*(upper-lower)
			}
			return &v
		}
		cum += c
	}
	v := LatencyOverflowMS
	return &v
}
