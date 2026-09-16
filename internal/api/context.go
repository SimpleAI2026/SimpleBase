package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"go.uber.org/zap"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	projectKey   contextKey = "project"
	principalKey contextKey = "principal"
)

// WithRequestID 把 request ID 注入 context。
func WithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, requestIDKey, rid)
}

// RequestIDFromContext 取出 request ID；缺失返回 ""。
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// ProjectContext 表示经中间件解析、已校验归属的 project 边界。
// handler 不应信任 body 中的 tenant/project ID，只能从这里取。
type ProjectContext struct {
	ID       string
	TenantID string
}

func WithProject(ctx context.Context, p ProjectContext) context.Context {
	return context.WithValue(ctx, projectKey, p)
}

func ProjectFromContext(ctx context.Context) (ProjectContext, bool) {
	if v, ok := ctx.Value(projectKey).(ProjectContext); ok {
		return v, true
	}
	return ProjectContext{}, false
}

// MustProject 取出 project；缺失时 panic 由 recover 兜底（不应发生）。
func MustProject(ctx context.Context) ProjectContext {
	p, ok := ProjectFromContext(ctx)
	if !ok {
		panic("project context missing")
	}
	return p
}

// WithPrincipal 把已认证的 Principal 注入 context。
func WithPrincipal(ctx context.Context, p auth.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext 取出 Principal；缺失返回 zero value 和 false。
func PrincipalFromContext(ctx context.Context) (auth.Principal, bool) {
	if v, ok := ctx.Value(principalKey).(auth.Principal); ok {
		return v, true
	}
	return auth.Principal{}, false
}

// MustPrincipal 取出 Principal；缺失时 panic（不应发生，中间件必须先注入）。
func MustPrincipal(ctx context.Context) auth.Principal {
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		panic("principal context missing")
	}
	return p
}

// itoa 是 strconv.Itoa 的局部别名，避免在多处 import strconv。
func itoa(i int) string { return strconv.Itoa(i) }

// fieldString/fieldDuration 是 zap.Field 构造的别名，便于中间件内联。
func fieldString(k, v string) zap.Field                 { return zap.String(k, v) }
func fieldDuration(k string, d time.Duration) zap.Field { return zap.Duration(k, d) }
func fieldInt(k string, v int) zap.Field                { return zap.Int(k, v) }

// nullWriter 是一个丢弃所有输出的 io.Writer，用于静音 echo 自带 logger。
type nullWriter struct{}

func (nullWriter) Write(p []byte) (int, error) { return len(p), nil }

// joinPath 组装多段 path，去除重复斜杠。
func joinPath(parts ...string) string {
	var b strings.Builder
	for i, p := range parts {
		p = strings.Trim(p, "/")
		if p == "" {
			continue
		}
		if i > 0 || b.Len() > 0 {
			b.WriteByte('/')
		}
		b.WriteString(p)
	}
	if b.Len() == 0 {
		return "/"
	}
	return "/" + b.String()
}
