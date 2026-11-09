// 文件说明：middleware_ratelimit.go — 统一令牌桶限流（§8.1.6 重构：分阶段、可选键、/** 前缀匹配、max_buckets fail-open）
//
// 用法
//
//	newRateLimiter(cfg, metrics).middleware(stage)  // stage = preAuthStage / postAuthStage
//	(l *rateLimiter) allow(key, rule) time.Duration  // loginGuard 复用同一令牌桶
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

// rateLimitStage 限流阶段（§8.1.6）：pre_auth 全局挂载（认证前，只跑 key=ip 的规则）；
// post_auth 挂在 /v1 与 /go 组的 authMW 之后（跑 principal/project 维度规则）。
type rateLimitStage int

const (
	preAuthStage  rateLimitStage = iota
	postAuthStage rateLimitStage = iota
)

type rateLimiter struct {
	cfg     config.RateLimitConfig
	metrics *observability.Metrics
	mu      sync.Mutex
	buckets map[string]*rateBucket
	calls   uint64
	// overflow 记录 max_buckets 触顶后的 fail-open 放行次数（指标输出）。
	overflow uint64
	now      func() time.Time
}

// defaultMaxBuckets 与 config.defaultRateLimitMaxBuckets 保持一致（0 = 不限）。
const fallbackMaxBuckets = 100000

func newRateLimiter(cfg config.RateLimitConfig, metrics *observability.Metrics) *rateLimiter {
	return &rateLimiter{cfg: cfg, metrics: metrics, buckets: map[string]*rateBucket{}, now: time.Now}
}

// ruleMatches 判定规则是否命中路径。支持两种模式：
//   - 以 /** 结尾：前缀匹配（去掉 /** 后路径前缀一致即命中；
//     前缀本身或其子路径都命中，但跨段错拼（/golang vs /go/**）不命中）；
//   - 其余：path.Match 通配（* 不跨 /）。
func ruleMatches(match, reqPath string) bool {
	if strings.HasSuffix(match, "/**") {
		prefix := strings.TrimSuffix(match, "/**")
		return reqPath == prefix || strings.HasPrefix(reqPath, prefix+"/")
	}
	matched, err := path.Match(match, reqPath)
	return err == nil && matched
}

// ruleKeyIsIP 判定规则是否属于 pre_auth 阶段：key=ip，以及未配置 key 的
// legacy 规则（向后兼容：两阶段都执行；principal 未认证时退化为 ip 维度）。
func ruleKeyIsIP(rule config.RateLimitRule) bool {
	return rule.Key == config.RateLimitKeyIP || rule.Key == ""
}

func (l *rateLimiter) middleware(stage rateLimitStage) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !l.cfg.Enabled || (l.cfg.ExemptSSE && isSSERequest(c)) {
				return next(c)
			}
			for _, rule := range l.cfg.Rules {
				if stage == preAuthStage && !ruleKeyIsIP(rule) {
					continue
				}
				if stage == postAuthStage && ruleKeyIsIP(rule) {
					continue
				}
				if !ruleMatches(rule.Match, c.Request().URL.Path) {
					continue
				}
				retry := l.allow(l.key(c, rule), rule)
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

// key 按规则的 key 维度构造桶键。principal 取 APIKeyID/UserID/EndUserID，未认证退化为 ip。
func (l *rateLimiter) key(c echo.Context, rule config.RateLimitRule) string {
	ip := c.RealIP()
	principal, _ := PrincipalFromContext(c.Request().Context())
	principalID := principal.APIKeyID
	if principalID == "" {
		principalID = principal.UserID
	}
	project := projectFromPath(c.Request().URL.Path)

	var identity string
	switch rule.Key {
	case config.RateLimitKeyIP:
		identity = ip
	case config.RateLimitKeyPrincipal:
		if principalID == "" {
			identity = ip
		} else {
			identity = principalID
		}
	case config.RateLimitKeyProject:
		identity = project
	default: // 空 = ip+principal（向后兼容）
		identity = principalID + "|" + ip
	}
	return rule.Match + "|" + project + "|" + identity
}

func projectFromPath(p string) string {
	parts := strings.Split(p, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "projects" {
			return parts[i+1]
		}
	}
	return ""
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
	maxBuckets := l.cfg.MaxBuckets
	if maxBuckets <= 0 {
		maxBuckets = fallbackMaxBuckets
	}
	bucket := l.buckets[key]
	if bucket == nil {
		// 容量触顶：先清理过期桶，仍超则 fail-open 放行并计数（§8.1.6）。
		if len(l.buckets) >= maxBuckets {
			for k, b := range l.buckets {
				if now.Sub(b.seen) > 10*time.Minute {
					delete(l.buckets, k)
				}
			}
			if len(l.buckets) >= maxBuckets {
				l.overflow++
				return 0
			}
		}
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
