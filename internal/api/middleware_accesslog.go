// 文件说明：middleware_accesslog.go — 请求级中间件：request ID 的生成 / 校验与访问日志（含该不该记录的判定）
//
// 用法
//
//	同包调用：requestIDMiddleware() / accessLogMiddleware() ← router.go 的 useGlobalMiddleware
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/middleware_accesslog.go
//	         │ 依赖
//	         ▼
//	[下游] internal/observability（日志与指标）
//	[同包] router.go, context.go（WithRequestID）
//
// 主要符号
//
//	requestIDMiddleware() / isValidRequestID(...)
//	shouldRecordRequestLog(...) / accessLogMiddleware(deps Dependencies)

package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func requestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			rid := req.Header.Get("X-Request-ID")
			if rid == "" || !isValidRequestID(rid) {
				rid = uuid.NewString()
			}
			c.Response().Header().Set("X-Request-ID", rid)
			ctx := WithRequestID(req.Context(), rid)
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}

// isValidRequestID 仅允许 UUID 或 [a-zA-Z0-9_-]{8,64}，避免日志注入。
func isValidRequestID(s string) bool {
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// shouldRecordRequestLog 访问日志选择性记录（perf §1 P1-A：日志页不再是全量 access log）：
//   - 跳过：健康检查、Prometheus 指标、SPA 静态回退、日志查询接口自身；
//   - 保留：status ≥ 400、耗时 ≥ 500ms 的慢请求、所有写方法（POST/PUT/PATCH/DELETE）。
//
// 指标（RecordMetric）口径不受影响，仍全量记录。
func shouldRecordRequestLog(method, route string, status int, latency time.Duration, metricsPath string) bool {
	if route == "" || route == "/*" || strings.HasPrefix(route, "/health/") {
		return false
	}
	if metricsPath == "" {
		metricsPath = "/metrics"
	}
	if route == metricsPath {
		return false
	}
	// 日志页自身（列表 / retention）不产生日志，避免自我放大。
	if strings.HasPrefix(route, "/v1/projects/:id/logs") {
		return false
	}
	if status >= 400 || latency >= 500*time.Millisecond {
		return true
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func accessLogMiddleware(deps Dependencies) echo.MiddlewareFunc {
	logger := deps.Logger
	metricsPath := deps.Config.Observability.MetricsPath
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			latency := time.Since(start)
			rid := requestID(c)
			status := c.Response().Status
			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					status = he.Code
				} else {
					status = http.StatusInternalServerError
				}
			}
			if logger != nil {
				logger.Info("http",
					fieldString("request_id", rid),
					fieldString("method", c.Request().Method),
					fieldString("route", c.Path()),
					fieldString("status", strconv.Itoa(status)),
					fieldDuration("duration", latency),
				)
			}
			if deps.Metrics != nil {
				deps.Metrics.HTTPRequests.WithLabelValues(c.Path(), c.Request().Method, strconv.Itoa(status)).Inc()
				deps.Metrics.HTTPRequestDuration.WithLabelValues(c.Path(), c.Request().Method).Observe(latency.Seconds())
			}
			if deps.System != nil {
				pc := projectOf(c)
				projectID := pc.ID
				if shouldRecordRequestLog(c.Request().Method, c.Path(), status, latency, metricsPath) {
					deps.System.RecordLog(systemdb.LogEvent{
						ProjectID:  projectID,
						Level:      "info",
						Logger:     "http",
						Message:    c.Request().Method + " " + c.Path(),
						FieldsJSON: `{"status":` + strconv.Itoa(status) + `,"duration_ms":` + strconv.Itoa(int(latency.Milliseconds())) + `}`,
						RequestID:  rid,
					})
				}
				if projectID != "" {
					deps.System.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_requests", Value: 1})
					deps.System.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_latency_ms", Value: float64(latency.Microseconds()) / 1000})
					if status >= 500 {
						deps.System.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_errors", Value: 1})
					}
				}
			}
			return err
		}
	}
}
