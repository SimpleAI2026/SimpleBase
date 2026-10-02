// stage_report.go 聚合分段计时指标（simplebase_api_stage_seconds）为段占比表，
// 并输出结果报告（终端表格 + output/perf/<date>/<case>.json 原始数据）。
//
// §7.2 M1/T6 修正：快照改为进程内 prometheus.Gatherer（不发 HTTP /metrics，
// 抓取请求不再污染增量）；占比以 total 段为分母（不含 total 自身）；
// 支持按用例 route 过滤并排除 /metrics、/health/*。
package perfbench

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// stageSnapshot 抓取当前各 route×stage 的观测样本计数与累计和，
// runCase 前后做差得到本轮净增量（并发下多轮压测互不污染）。
type stageSnapshot map[stageKey]stageVal

type stageKey struct {
	Route string
	Stage string
}

type stageVal struct {
	Count uint64
	Sum   float64 // seconds
}

// excludedRoutes 不计入段占比的路由（探针/指标通道自身）。
var excludedRoutes = map[string]bool{
	"/metrics":       true,
	"/health/live":   true,
	"/health/ready":  true,
	"/v1/auth/login": true,
}

// stageSnapshotFrom 通过进程内 Gatherer 抓取 stage 直方图（不产生额外请求）。
func (e *benchEnv) stageSnapshot() stageSnapshot {
	snap := stageSnapshot{}
	if e.Registry == nil {
		return snap
	}
	mfs, err := e.Registry.Gather()
	if err != nil || len(mfs) == 0 {
		return snap
	}
	for _, mf := range mfs {
		if mf.GetName() != "simplebase_api_stage_seconds" || mf.GetType() != dto.MetricType_HISTOGRAM {
			continue
		}
		for _, m := range mf.GetMetric() {
			var route, stage string
			for _, lp := range m.GetLabel() {
				switch lp.GetName() {
				case "route":
					route = lp.GetValue()
				case "stage":
					stage = lp.GetValue()
				}
			}
			if route == "" || stage == "" {
				continue
			}
			h := m.GetHistogram()
			if h == nil {
				continue
			}
			key := stageKey{Route: route, Stage: stage}
			snap[key] = stageVal{Count: h.GetSampleCount(), Sum: h.GetSampleSum()}
		}
	}
	return snap
}

// stageSnapshot 兼容保留：进程内版本（fetchMetricsText 已废弃）。
var _ = prometheus.NewRegistry

// routeForEndpoint 把用例端点标签映射回路由模板（stage 指标的 route label）。
func routeForEndpoint(bc benchCase) string {
	switch bc.Endpoint {
	case "sql.query", "sql.execute", "sql.batch":
		return "/v1/projects/:projectID/databases/:databaseID/" + map[string]string{
			"sql.query": "query", "sql.execute": "execute", "sql.batch": "batch",
		}[bc.Endpoint]
	case "databases.list":
		return "/v1/projects/:projectID/databases"
	case "databases.create":
		return "/v1/projects/:projectID/databases"
	case "data.list", "data.create", "data.update_delete":
		return "/v1/projects/:projectID/databases/:databaseID/data/*"
	case "kv.set", "kv.get", "kv.hgetall", "kv.incr", "kv.del":
		return "/v1/projects/:projectID/kv"
	case "health.ready":
		return "/health/ready"
	default:
		return ""
	}
}

// stageDelta 计算本轮净增量的段占比（百分比）。分母为 total 段增量本身
// （不含 total 计入占比），返回 nil 表示无可用观测（指标未启用）。
func (e *benchEnv) stageDelta(before stageSnapshot, bc benchCase) map[string]float64 {
	after := e.stageSnapshot()
	if len(after) == 0 {
		return nil
	}
	wantRoute := routeForEndpoint(bc)
	sums := map[string]float64{}
	var total float64
	for k, av := range after {
		bv := before[k]
		if av.Count <= bv.Count {
			continue
		}
		d := (av.Sum - bv.Sum) * 1000 // ms
		if d <= 0 {
			continue
		}
		if k.Stage == string(StageNameTotal) {
			total = d
			continue
		}
		if excludedRoutes[k.Route] {
			continue
		}
		// 路由过滤：能匹配模板时只保留该路由（复合用例涉及多条路由时退化为全量）。
		if wantRoute != "" && k.Route != wantRoute && !strings.HasPrefix(k.Route, wantRoute) {
			continue
		}
		sums[k.Stage] += d
	}
	if total <= 0 {
		return nil
	}
	out := map[string]float64{}
	for st, v := range sums {
		out[st] = round2(v / total * 100)
	}
	out["total"] = round2(total)
	// unattributed = total - Σ(其余段)；为负截 0。
	var attributed float64
	for _, v := range sums {
		attributed += v
	}
	rest := total - attributed
	if rest < 0 {
		rest = 0
	}
	out["unattributed_ms"] = round2(rest)
	return out
}

// StageNameTotal 与 api.StageTotal 对齐（perfbench 不 import api 内部常量）。
const StageNameTotal = "total"

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

// reportCtx 控制报告输出位置。
type reportCtx struct {
	OutDir string // output/perf/<date>；空则仅打印
	Stdout io.Writer
}

// writeResult 落盘单条结果 JSON 并打印摘要行。
func (rc *reportCtx) writeResult(r *benchResult) error {
	stage := r.StageShare
	total := 0.0
	if stage != nil {
		total = stage["total"]
	}
	p50Unit := r.LatencyMS.P50
	p95Unit := r.LatencyMS.P95
	maxUnit := r.LatencyMS.Max
	if r.RequestsPerSample > 1 {
		p50Unit /= float64(r.RequestsPerSample)
		p95Unit /= float64(r.RequestsPerSample)
		maxUnit /= float64(r.RequestsPerSample)
	}
	lowSample := ""
	if r.Samples < 100 {
		lowSample = " LOW-SAMPLE"
	}
	line := fmt.Sprintf("[%-24s] c=%-3d n=%-5d err=%-3d qps=%-8.1f p50=%-7.2f p95=%-7.2f max=%-8.2f req/sample=%d%s",
		r.CaseID+"."+r.Label, r.Concurrency, r.Samples, r.Errors, r.QPS,
		p50Unit, p95Unit, maxUnit, r.RequestsPerSample, lowSample)
	if rc.Stdout != nil {
		fmt.Fprintln(rc.Stdout, line)
		if stage != nil {
			fmt.Fprintf(rc.Stdout, "    stage%%(of %s): %s\n", durLabel(total), formatShares(stage))
		}
	}
	if rc.OutDir == "" {
		return nil
	}
	bs, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(rc.OutDir, fmt.Sprintf("%s_%s_c%d.json", r.CaseID, sanitize(r.Label), r.Concurrency))
	return os.WriteFile(path, bs, 0o644)
}

// durLabel 输出 total 段平均毫秒数。
func durLabel(totalMS float64) string {
	return fmt.Sprintf("%.1fms avg", totalMS)
}

func formatShares(shares map[string]float64) string {
	// 按 plan §1 的段顺序输出（§7.1 修正：分母为 total，不含 total 自身）。
	order := []string{"auth", "project_resolve", "decode_validate", "catalog_lookup",
		"semaphore", "acquire", "db_conn_queue", "db_exec", "serialize", "on_write", "unattributed"}
	var parts []string
	for _, k := range order {
		if v, ok := shares[k]; ok {
			parts = append(parts, fmt.Sprintf("%s=%.1f%%", k, v))
		}
	}
	for k, v := range shares {
		if k == "total" || k == "unattributed_ms" || contains(order, k) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%.1f%%", k, v))
	}
	if v, ok := shares["unattributed_ms"]; ok {
		parts = append(parts, fmt.Sprintf("(unattributed=%.1fms)", v))
	}
	return strings.Join(parts, " ")
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}

var _ = math.Round
var _ = json.Marshal

// benchClock 返回单调时钟（报告头部环境信息用）。
func benchClock() time.Time { return time.Now() }
