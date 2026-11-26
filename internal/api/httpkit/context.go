// 文件说明：context.go — 请求上下文：request ID / 项目上下文 / 主体的 context key 与读写访问器，及小型请求解析辅助
//
// 用法
//
//	WithRequestID / RequestIDFromContext                      // 请求 ID（访问日志、错误协议、审计共用）
//	WithProject / ProjectFromContext / RequireProject         // 项目上下文（中间件注入；缺失 → ErrProjectMissing）
//	WithPrincipal / PrincipalFromContext                      // 主体（auth.Principal）
//	RequestID(c) / ProjectOf(c) / ProjectID(c) / PrincipalOf(c)   // 从 echo.Context 取值的薄访问器
//	DecodeJSONBody(c, dst, strict) / QueryLimit(c, def, max) / ParseQueryLimit   // 请求体与分页参数解析
//	FieldString / FieldDuration / NullWriter                  // zap 字段与空写出器
//
// 调用架构
//
//	[上游] internal/api（中间件与 handler）、internal/api/<context>http
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/httpkit/context.go
//	         │ 依赖
//	         ▼
//	[下游] internal/auth, go.uber.org/zap
//	[同包] errors.go, sse.go
//
// 主要符号
//
//	type ProjectContext struct {…}
//	var ErrProjectMissing
//	WithRequestID(ctx context.Context, rid string) context.Context
//	RequestIDFromContext(ctx context.Context) string
//	... 另有 16 个顶层符号
//
// context key 在这里且只在这里有一份实现：中间件（api）写入、各业务域 handler 读取。

package httpkit

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"go.uber.org/zap"
)

type contextKey string

const (
	RequestIDKey    contextKey = "request_id"
	ProjectKey      contextKey = "project"
	PrincipalCtxKey contextKey = "principal"
)

// WithRequestID 把 request ID 注入 context。
func WithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, RequestIDKey, rid)
}

// RequestIDFromContext 取出 request ID；缺失返回 ""。
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(RequestIDKey).(string); ok {
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
	return context.WithValue(ctx, ProjectKey, p)
}

func ProjectFromContext(ctx context.Context) (ProjectContext, bool) {
	if v, ok := ctx.Value(ProjectKey).(ProjectContext); ok {
		return v, true
	}
	return ProjectContext{}, false
}

// ErrProjectMissing 表示 handler 拿不到中间件注入的 project 边界。
//
// 所有 /projects/:ProjectID（以及 /go/:ProjectID）路由都经 projectContextMiddlewareEcho
// 注入，正常不可达，因此这是防御性分支。它必须是领域错误：裸 errors.New 会落进
// Error() 的未知分支返回 500 internal_error，与同语义的 400 invalid_request 并存。
var ErrProjectMissing = errors.New("project context missing")

// WithPrincipal 把已认证的 Principal 注入 context。
func WithPrincipal(ctx context.Context, p auth.Principal) context.Context {
	return context.WithValue(ctx, PrincipalCtxKey, p)
}

// PrincipalFromContext 取出 Principal；缺失返回 zero value 和 false。
func PrincipalFromContext(ctx context.Context) (auth.Principal, bool) {
	if v, ok := ctx.Value(PrincipalCtxKey).(auth.Principal); ok {
		return v, true
	}
	return auth.Principal{}, false
}

// RequestID 取当前请求的 request ID；缺失返回 ""。
func RequestID(c echo.Context) string {
	return RequestIDFromContext(c.Request().Context())
}

// RequireProject 取中间件注入的项目边界；缺失返回 ErrProjectMissing（防御性分支）。
func RequireProject(c echo.Context) (ProjectContext, error) {
	pc, ok := ProjectFromContext(c.Request().Context())
	if !ok {
		return ProjectContext{}, ErrProjectMissing
	}
	return pc, nil
}

// ProjectOf 取项目边界，缺失时为零值（调用方确定路由已挂项目中间件时使用）。
func ProjectOf(c echo.Context) ProjectContext {
	pc, _ := ProjectFromContext(c.Request().Context())
	return pc
}

// ProjectID 取当前请求的项目 ID，缺失时为 ""。
func ProjectID(c echo.Context) string { return ProjectOf(c).ID }

// ProjectIDFrom 从 ctx 取项目 ID，缺失时为 ""。
func ProjectIDFrom(ctx context.Context) string {
	project, _ := ProjectFromContext(ctx)
	return project.ID
}

// PrincipalOf 取当前请求的主体，缺失时为零值。
func PrincipalOf(c echo.Context) auth.Principal {
	p, _ := PrincipalFromContext(c.Request().Context())
	return p
}

// DecodeJSONBody 解码单个 JSON 请求对象，保持各处理器既有错误协议。
func DecodeJSONBody(c echo.Context, dst any, strict bool) error {
	dec := json.NewDecoder(c.Request().Body)
	if strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(dst); err != nil {
		return BadRequest(c, "malformed JSON body")
	}
	return nil
}

// QueryLimit 返回合法的正数 limit；max 为零时不限制上限。
func QueryLimit(c echo.Context, def, max int) int {
	limit, _ := ParseQueryLimit(c, def, max)
	return limit
}

func ParseQueryLimit(c echo.Context, def, max int) (int, bool) {
	if c.QueryParam("limit") == "" {
		return def, true
	}
	n, err := strconv.Atoi(c.QueryParam("limit"))
	if err != nil || n < 1 || (max > 0 && n > max) {
		return def, false
	}
	return n, true
}

// FieldString/FieldDuration 是 zap.Field 构造的别名，便于中间件内联。
func FieldString(k, v string) zap.Field                 { return zap.String(k, v) }
func FieldDuration(k string, d time.Duration) zap.Field { return zap.Duration(k, d) }

// NullWriter 是一个丢弃所有输出的 io.Writer，用于静音 echo 自带 logger。
type NullWriter struct{}

func (NullWriter) Write(p []byte) (int, error) { return len(p), nil }
