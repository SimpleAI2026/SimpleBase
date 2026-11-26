// 文件说明：project_context.go — :projectID 路由参数 → 项目上下文（projectContextMiddlewareEcho）
//
// 用法
//
//	projectContextMiddlewareEcho(cat)   // NewRouter 装配；所有带 :projectID 的路由经它注入 httpkit.ProjectContext
//
// 调用架构
//
//	[上游] router.go
// │ 同包调用
//	         ▼
//	[本文件] internal/api/project_context.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/httpkit（ProjectContext 与协议载荷）
//
// 主要符号
//
//	projectContextMiddlewareEcho(cat projectTenantResolver) echo.MiddlewareFunc
//
// 自数据面 handler（现 internal/api/datahttp）迁出：项目上下文中间件是入口守卫，不随数据面 handler 迁走。

package api

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// projectTenantResolver 是解析项目租户的最小端口（datahttp.CatalogService 满足）。
type projectTenantResolver interface {
	ResolveProjectTenant(ctx context.Context, projectID string) (string, error)
}

// projectContextMiddleware 解析 :projectID path 参数并通过 catalog 解析其 tenant，
// 构造 httpkit.ProjectContext 注入 context。project 不存在返回 404。
// 使用 echo.Context 以便传递请求 context 给 catalog。
func projectContextMiddlewareEcho(cat projectTenantResolver) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			projectID := c.Param("projectID")
			if projectID == "" {
				return c.JSON(http.StatusBadRequest, httpkit.APIErrorBody{Error: httpkit.APIErrorDetail{Code: "invalid_request", Message: "project_id required"}})
			}
			if cat == nil {
				return c.JSON(http.StatusServiceUnavailable, httpkit.APIErrorBody{Error: httpkit.APIErrorDetail{Code: "service_unavailable", Message: "catalog not configured"}})
			}
			timer := httpkit.StageTimerFrom(c.Request().Context())
			scope := timer.StageScope(httpkit.StageProject)
			tenantID, err := cat.ResolveProjectTenant(c.Request().Context(), projectID)
			scope.Done()
			if err != nil {
				return httpkit.WriteError(c, err)
			}
			principal, ok := httpkit.PrincipalFromContext(c.Request().Context())
			if !ok {
				return httpkit.WriteError(c, auth.ErrMissingCredentials)
			}
			if !auth.CheckProjectAccess(principal, projectID, tenantID, catalog.IsSystemProject(projectID)) {
				return httpkit.WriteError(c, catalog.ErrCrossProject)
			}
			// §7.2 P1.1：把解析结果注入 ctx，下游 catalog 校验免重复点查。
			ctx := catalog.WithResolvedProjectTenant(c.Request().Context(), projectID, tenantID)
			ctx = httpkit.WithProject(ctx, httpkit.ProjectContext{ID: projectID, TenantID: tenantID})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
