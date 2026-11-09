// 文件说明：observability_routes.go — /metrics 与 pprof 的收口挂载（§8.1.7）
//
// 用法
//
//	mountObservabilityRoutes(e, deps)            // 业务端口挂载（按配置收口或 404）
//	NewObservabilityHandler(deps) http.Handler   // 独立监听（observability.metrics_address）
//
// 组合矩阵（§8.1.7 表）：
//   - 配了 metrics_address → 业务端口 404；独立监听开放 /metrics +（pprof=true 时）pprof，无鉴权；
//   - 只配 token → 业务端口 /metrics 需 Bearer token（常量时间比较），否则 404；
//   - dev 都没配 → 开放（现状）。
package api

import (
	"crypto/subtle"
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// bearerOK 常量时间比较 token。
func bearerOK(got, want string) bool {
	got = strings.TrimSpace(strings.TrimPrefix(got, "Bearer "))
	return len(got) == len(want) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// observabilityGate 返回业务端口上 /metrics 的守卫：ok=false 时返回 404。
func observabilityGate(deps Dependencies) (guarded func(next echo.HandlerFunc) echo.HandlerFunc) {
	token := deps.Config.Observability.MetricsToken
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if token != "" {
				if !bearerOK(c.Request().Header.Get("Authorization"), token) {
					return c.NoContent(http.StatusNotFound)
				}
			}
			return next(c)
		}
	}
}

// mountObservabilityRoutes 在业务端口挂载 /metrics 与 pprof（§8.1.7）。
// 配了 metrics_address 时业务端口直接 404（观测改走独立监听）。
func mountObservabilityRoutes(e *echo.Echo, deps Dependencies) {
	cfg := deps.Config.Observability
	onSeparate := cfg.MetricsAddress != ""

	gate := observabilityGate(deps)
	if deps.Metrics != nil && !onSeparate {
		reg := deps.MetricsRegistry
		if reg == nil {
			reg = prometheus.DefaultGatherer
		}
		e.GET(cfg.MetricsPath, echo.WrapHandler(promhttp.HandlerFor(reg, promhttp.HandlerOpts{})), gate)
	}
	if cfg.PProf && !onSeparate {
		mountPprof(e, gate)
	}
}

// mountPprof 挂 pprof 路由（与 perf_stage_timing 解耦，§8.1.7）。
func mountPprof(e *echo.Echo, mws ...echo.MiddlewareFunc) {
	pprofRoute := func(path string, h http.Handler) {
		e.GET(path, echo.WrapHandler(h), mws...)
	}
	pprofRoute("/debug/pprof/", http.HandlerFunc(pprof.Index))
	pprofRoute("/debug/pprof/heap", pprof.Handler("heap"))
	pprofRoute("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	pprofRoute("/debug/pprof/mutex", pprof.Handler("mutex"))
	pprofRoute("/debug/pprof/block", pprof.Handler("block"))
}

// NewObservabilityHandler 构造独立监听的 http.Handler（/metrics + 可选 pprof）。
// 无鉴权——靠监听地址隔离（通常 127.0.0.1:9090）。
func NewObservabilityHandler(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	if deps.Metrics != nil {
		reg := deps.MetricsRegistry
		if reg == nil {
			reg = prometheus.DefaultGatherer
		}
		mux.Handle(deps.Config.Observability.MetricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	}
	if deps.Config.Observability.PProf {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	}
	return mux
}
