// loadgen.go 实现闭环压测（plan §2.2/§2.4.3）：固定 worker 并发，
// 每 worker 固定请求数；输出 p50/p95/p99/max 与错误率；不引入第三方依赖。
package perfbench

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// benchCase 是一条压测用例（plan §2.4.2 benchCase 的落地形态）。
//
// setup 在计时开始前执行（建库建表/预热，不计入样本）；
// step 是一次完整 HTTP 请求（含断言）；返回 error 即记为失败样本。
type benchCase struct {
	ID       string
	Desc     string
	Setup    func(env *benchEnv) error
	Step     func(env *benchEnv, i int) error
	Endpoint string // 标签（报告分组用）
	// ReqCount 是一个样本包含的 HTTP 请求数（T2：复合用例须声明，默认 1）。
	ReqCount int
}

// RequestsPerSample 返回一个样本的请求数。
func (bc benchCase) RequestsPerSample() int {
	if bc.ReqCount > 1 {
		return bc.ReqCount
	}
	return 1
}

// benchOptions 控制一轮压测。
type benchOptions struct {
	Concurrency int           // worker 数（1 = 串行）
	Samples     int           // 总请求数（<1 时默认 500）
	Warmup      int           // 预热请求数（不计样本；默认 50）
	MinDur      time.Duration // 最短持续时间（样本数与时长取大；默认 0）
	Label       string        // 场景标签（报告分组用）
}

// benchResult 一轮压测的聚合结果。
type benchResult struct {
	CaseID      string             `json:"case"`
	Label       string             `json:"scenario"`
	Endpoint    string             `json:"endpoint"`
	Concurrency int                `json:"concurrency"`
	Samples     int                `json:"samples"`
	Errors      int                `json:"errors"`
	ErrorCodes  map[string]int     `json:"error_codes,omitempty"`
	Duration    string             `json:"duration"`
	QPS         float64            `json:"qps"`
	LatencyMS   latencySummary     `json:"latency_ms"`
	StageShare  map[string]float64 `json:"stage_share_percent,omitempty"` // 段占比（来自 stage timing）
	// RequestsPerSample（T2）：一个样本包含的 HTTP 请求数；>1 时延迟为复合值。
	RequestsPerSample int               `json:"requests_per_sample"`
	RunEnv            map[string]string `json:"env,omitempty"`
}

type latencySummary struct {
	P50  float64 `json:"p50"`
	P90  float64 `json:"p90"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
	Max  float64 `json:"max"`
	Mean float64 `json:"mean"`
}

// runCase 执行一条用例一轮并返回结果。样本 = 每 worker 固定请求数闭环。
func runCase(env *benchEnv, bc benchCase, opt benchOptions) (*benchResult, error) {
	if opt.Samples < 1 {
		opt.Samples = 500
	}
	if opt.Warmup < 0 {
		opt.Warmup = 50
	}
	if opt.Concurrency < 1 {
		opt.Concurrency = 1
	}
	if bc.Setup != nil {
		if err := bc.Setup(env); err != nil {
			return nil, fmt.Errorf("perfbench: setup %s: %w", bc.ID, err)
		}
	}

	// T5：用例开始前排空日志/指标缓冲并落盘，避免上一用例的 flush 干扰本轮长尾。
	env.quiesce()

	// 预热：跑 Warmup 次串行（触发冷开/建表/JIT 等一次性成本）。
	for i := 0; i < opt.Warmup; i++ {
		if err := bc.Step(env, i); err != nil {
			return nil, fmt.Errorf("perfbench: warmup %s #%d: %w", bc.ID, i, err)
		}
	}

	perWorker := opt.Samples / opt.Concurrency
	if perWorker < 1 {
		perWorker = 1
	}
	total := perWorker * opt.Concurrency

	latencies := make([]float64, total)
	var errCount int64
	errCodes := sync.Map{}
	var wg sync.WaitGroup
	var seq int64
	stageBefore := env.stageSnapshot()

	// elapsed 累计实际执行时长（T4）：补测段单独计时，不含段间间隔。
	elapsed := time.Duration(0)
	start := time.Now()
	for w := 0; w < opt.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				i := int(atomic.AddInt64(&seq, 1)) - 1
				t0 := time.Now()
				err := bc.Step(env, i)
				latencies[i] = msFloat(time.Since(t0))
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					code := errCode(err)
					v, _ := errCodes.LoadOrStore(code, new(int64))
					atomic.AddInt64(v.(*int64), 1)
				}
			}
		}()
	}
	wg.Wait()
	elapsed += time.Since(start)

	// 样本量不足最短持续时间时按比例补测一轮（保持闭环语义）。
	if opt.MinDur > 0 && elapsed < opt.MinDur {
		extra := int(float64(total) * float64(opt.MinDur) / float64(elapsed))
		extra = clampInt(extra, total, total*8)
		extraLat := make([]float64, extra)
		var ewg sync.WaitGroup
		var eseq int64
		extraStart := time.Now()
		extraPerWorker := extra / opt.Concurrency
		for w := 0; w < opt.Concurrency; w++ {
			ewg.Add(1)
			go func() {
				defer ewg.Done()
				for j := 0; j < extraPerWorker; j++ {
					i := int(atomic.AddInt64(&eseq, 1)) - 1
					t0 := time.Now()
					err := bc.Step(env, i)
					extraLat[i] = msFloat(time.Since(t0))
					if err != nil {
						atomic.AddInt64(&errCount, 1)
						code := errCode(err)
						v, _ := errCodes.LoadOrStore(code, new(int64))
						atomic.AddInt64(v.(*int64), 1)
					}
				}
			}()
		}
		ewg.Wait()
		elapsed += time.Since(extraStart) // T4：补测段真实执行时长
		latencies = append(latencies, extraLat[:eseqLoad(&eseq)]...)
	}

	sort.Float64s(latencies)
	res := &benchResult{
		CaseID:            bc.ID,
		Label:             opt.Label,
		Endpoint:          bc.Endpoint,
		Concurrency:       opt.Concurrency,
		Samples:           len(latencies),
		Errors:            int(atomic.LoadInt64(&errCount)),
		ErrorCodes:        map[string]int{},
		Duration:          elapsed.String(),
		QPS:               float64(len(latencies)) / elapsed.Seconds(),
		RequestsPerSample: bc.RequestsPerSample(),
	}
	errCodes.Range(func(k, v any) bool {
		res.ErrorCodes[k.(string)] = int(atomic.LoadInt64(v.(*int64)))
		return true
	})
	res.LatencyMS = summarize(latencies)
	if shares := env.stageDelta(stageBefore, bc); shares != nil {
		res.StageShare = shares
	}
	return res, nil
}

// quiesce 同步排空系统库日志/指标缓冲（T5：消除跨用例 flush 干扰）。
func (e *benchEnv) quiesce() {
	if e.App == nil {
		return
	}
	if st := e.App.SystemStore(); st != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = st.FlushLogs(ctx)
		_ = st.FlushMetrics(ctx)
	}
}

func msFloat(d time.Duration) float64 { return float64(d.Nanoseconds()) / 1e6 }

func summarize(sorted []float64) latencySummary {
	n := len(sorted)
	if n == 0 {
		return latencySummary{}
	}
	pct := func(p float64) float64 {
		idx := int(float64(n-1) * p)
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return sorted[idx]
	}
	var sum float64
	for _, v := range sorted {
		sum += v
	}
	return latencySummary{
		P50:  pct(0.50),
		P90:  pct(0.90),
		P95:  pct(0.95),
		P99:  pct(0.99),
		Max:  sorted[n-1],
		Mean: sum / float64(n),
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func eseqLoad(p *int64) int { return int(*p) }

// errCode 从错误中提取稳定错误码（失败样本分类）。
func errCode(err error) string {
	if err == nil {
		return ""
	}
	type coder interface{ ErrCode() string }
	if c, ok := err.(coder); ok {
		return c.ErrCode()
	}
	return "error"
}
