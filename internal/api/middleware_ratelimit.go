// 文件说明：middleware_ratelimit.go — internal/api 的内部实现（rateBucket、rateLimiter 等）
//
// 用法
//
//	type rateBucket（包内使用）
//	type rateLimiter（包内使用）
//	同包调用：key() → agent_handler.go, api_keys_handler.go 等；middleware() → middleware_http.go, router.go；allow() → auth_handler.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/middleware_ratelimit.go
//	         │ 依赖
//	         ▼
//	[下游] internal/config, internal/observability
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] key() ← agent_handler.go, api_keys_handler.go, auth_handler.go
//
// 主要符号
//
//	type rateBucket struct {…}
//	type rateLimiter struct {…}
//	newRateLimiter(cfg config.RateLimitConfig, metrics *observability.Metrics) ...
//	(l *rateLimiter) middleware() echo.MiddlewareFunc
//	(l *rateLimiter) key(c echo.Context, rule string) string
//	(l *rateLimiter) allow(key string, rule config.RateLimitRule) time.Duration
//	... 另有 1 个顶层符号

package api

import (
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

type rateBucket struct {
	tokens float64
	last   time.Time
	seen   time.Time
}

type rateLimiter struct {
	cfg     config.RateLimitConfig
	metrics *observability.Metrics
	mu      sync.Mutex
	buckets map[string]*rateBucket
	calls   uint64
	now     func() time.Time
}

func newRateLimiter(cfg config.RateLimitConfig, metrics *observability.Metrics) *rateLimiter {
	return &rateLimiter{cfg: cfg, metrics: metrics, buckets: map[string]*rateBucket{}, now: time.Now}
}

func (l *rateLimiter) middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !l.cfg.Enabled || (l.cfg.ExemptSSE && isSSERequest(c)) {
				return next(c)
			}
			for _, rule := range l.cfg.Rules {
				matched, err := path.Match(rule.Match, c.Request().URL.Path)
				if err != nil || !matched {
					continue
				}
				retry := l.allow(l.key(c, rule.Match), rule)
				if retry > 0 {
					c.Response().Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
					if l.metrics != nil {
						l.metrics.APIRateLimited.WithLabelValues(rule.Match).Inc()
					}
					return WriteError(c, NewAPIError(http.StatusTooManyRequests, "rate_limited", "rate limit exceeded", RequestIDFromContext(c.Request().Context())))
				}
				break
			}
			return next(c)
		}
	}
}

func (l *rateLimiter) key(c echo.Context, rule string) string {
	project := ""
	parts := strings.Split(c.Request().URL.Path, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "projects" {
			project = parts[i+1]
			break
		}
	}
	principal, _ := PrincipalFromContext(c.Request().Context())
	id := principal.APIKeyID + principal.UserID
	return rule + "|" + project + "|" + id + "|" + c.RealIP()
}

func (l *rateLimiter) allow(key string, rule config.RateLimitRule) time.Duration {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.calls%256 == 0 {
		for k, bucket := range l.buckets {
			if now.Sub(bucket.seen) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
	}
	burst := l.cfg.Burst
	if burst <= 0 {
		burst = 2
	}
	bucket := l.buckets[key]
	if bucket == nil {
		bucket = &rateBucket{tokens: float64(rule.Max * burst), last: now}
		l.buckets[key] = bucket
	}
	bucket.tokens = math.Min(float64(rule.Max*burst), bucket.tokens+now.Sub(bucket.last).Seconds()*float64(rule.Max)/rule.Window.Seconds())
	bucket.last = now
	bucket.seen = now
	if bucket.tokens >= 1 {
		bucket.tokens--
		return 0
	}
	return time.Duration((1 - bucket.tokens) * rule.Window.Seconds() / float64(rule.Max) * float64(time.Second))
}

func isSSERequest(c echo.Context) bool {
	p := c.Request().URL.Path
	return strings.HasSuffix(p, "/events") || strings.HasSuffix(p, "/realtime")
}
