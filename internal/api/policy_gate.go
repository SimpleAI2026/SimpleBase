// 文件说明：policy_gate.go — internal/api 的内部实现（policyGate、policyResource 等）
//
// 用法
//
//	同包调用：scopedPolicy() → realtime_handler.go；policyGate() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/policy_gate.go
//	         │ 依赖
//	         ▼
//	[下游] internal/auth
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] scopedPolicy() ← realtime_handler.go
//
// 主要符号
//
//	policyGate(next echo.HandlerFunc) echo.HandlerFunc
//	policyResource(c echo.Context) string
//	scopedPolicy(c echo.Context) *auth.KeyPolicy

package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

func policyGate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		principal, ok := PrincipalFromContext(c.Request().Context())
		if !ok || principal.Scope == nil {
			return next(c)
		}
		if strings.HasSuffix(c.Path(), "/realtime") || strings.HasSuffix(c.Path(), "/realtime/subscriptions") {
			return next(c)
		}
		action := "write"
		if c.Request().Method == http.MethodGet || c.Request().Method == http.MethodHead || strings.HasSuffix(c.Path(), "/realtime/subscriptions") {
			action = "read"
		}
		if strings.HasSuffix(c.Path(), "/presign") || strings.HasSuffix(c.Path(), "/link") {
			action = "presign"
		}
		if strings.HasPrefix(c.Path(), "/go/") || strings.HasSuffix(c.Path(), "/test") || strings.HasSuffix(c.Path(), "/invoke") {
			action = "invoke"
		}
		if !principal.Scope.Allows(policyResource(c), action, time.Now().UTC()) {
			return WriteError(c, NewAPIError(http.StatusForbidden, "key_scope_denied", "key scope denied", RequestIDFromContext(c.Request().Context())))
		}
		return next(c)
	}
}

func policyResource(c echo.Context) string {
	path := c.Path()
	switch {
	case strings.Contains(path, "/data/collections"):
		if name := c.Param("collection"); name != "" {
			return "collection:" + name
		}
		return "collection:*"
	case strings.Contains(path, "/s3/"):
		key := c.QueryParam("key")
		if key == "" {
			key = c.QueryParam("prefix")
		}
		return "object:" + key
	case strings.Contains(path, "/gofunctions"), strings.HasPrefix(path, "/go/"):
		return "function:" + c.Param("name")
	case strings.Contains(path, "/cron-jobs"):
		return "cron:*"
	case strings.Contains(path, "/databases"):
		if id := c.Param("databaseID"); id != "" {
			return "database:" + id
		}
		return "database:*"
	default:
		return "project:*"
	}
}

func scopedPolicy(c echo.Context) *auth.KeyPolicy {
	principal, _ := PrincipalFromContext(c.Request().Context())
	return principal.Scope
}
