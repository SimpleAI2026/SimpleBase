// 文件说明：context.go — internal/api 中 WithRequestID、RequestIDFromContext 等符号的实现
//
// 用法
//
//	WithRequestID(ctx context.Context, rid string) context.Context  // WithRequestID 把 request ID 注入 context
//	RequestIDFromContext(ctx context.Context) string  // RequestIDFromContext 取出 request ID
//	同包调用：RequestIDFromContext() → agent_handler.go, agent_schedule_handler.go 等；PrincipalFromContext() → agent_handler.go, api_keys_handler.go 等；ProjectFromContext() → agent_handler.go, agent_schedule_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/context.go
//	         │ 依赖
//	         ▼
//	[下游] internal/auth
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] RequestIDFromContext() ← agent_handler.go, agent_schedule_handler.go, api_keys_handler.go
//
// 主要符号
//
//	WithRequestID(ctx context.Context, rid string) context.Context
//	RequestIDFromContext(ctx context.Context) string
//	type ProjectContext struct {…}
//	WithProject(ctx context.Context, p ProjectContext) context.Context
//	ProjectFromContext(ctx context.Context) (ProjectContext, bool)
//	WithPrincipal(ctx context.Context, p auth.Principal) context.Context
//	... 另有 11 个顶层符号

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
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

// decodeJSONBody 解码单个 JSON 请求对象，保持各处理器既有错误协议。
func decodeJSONBody(c echo.Context, dst any, strict bool) error {
	dec := json.NewDecoder(c.Request().Body)
	if strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(dst); err != nil {
		return NewAPIError(http.StatusBadRequest, "invalid_request", "malformed JSON body", RequestIDFromContext(c.Request().Context()))
	}
	return nil
}

// queryLimit 返回合法的正数 limit；max 为零时不限制上限。
func queryLimit(c echo.Context, def, max int) int {
	limit, _ := parseQueryLimit(c, def, max)
	return limit
}

func parseQueryLimit(c echo.Context, def, max int) (int, bool) {
	if c.QueryParam("limit") == "" {
		return def, true
	}
	n, err := strconv.Atoi(c.QueryParam("limit"))
	if err != nil || n < 1 || (max > 0 && n > max) {
		return def, false
	}
	return n, true
}

// fieldString/fieldDuration 是 zap.Field 构造的别名，便于中间件内联。
func fieldString(k, v string) zap.Field                 { return zap.String(k, v) }
func fieldDuration(k string, d time.Duration) zap.Field { return zap.Duration(k, d) }

// nullWriter 是一个丢弃所有输出的 io.Writer，用于静音 echo 自带 logger。
type nullWriter struct{}

func (nullWriter) Write(p []byte) (int, error) { return len(p), nil }
