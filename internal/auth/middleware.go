// middleware.go 提供 Echo 中间件：APIKeyMiddleware 完成认证并注入 Principal，
// Require 校验特定权限。两者都不直接依赖 internal/api，避免循环 import；
// 中间件返回标准 echo.HTTPError，由 internal/api 的错误协议统一序列化。
package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// APIKeyMiddleware 解析 "Authorization: Bearer <key>"，认证失败返回 401；
// 成功后通过 inject 把 Principal 写入请求 context。
//
// inject 由调用方传入 internal/api.WithPrincipal，避免 auth 包反向依赖 api 包。
func APIKeyMiddleware(svc *Service, inject func(ctx context.Context, p Principal) context.Context) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token, err := ExtractBearerToken(c.Request().Header.Get(echo.HeaderAuthorization))
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing or malformed authorization header")
			}
			principal, err := svc.Authenticate(c.Request().Context(), token)
			if err != nil {
				switch {
				case errors.Is(err, ErrKeyRevoked):
					return echo.NewHTTPError(http.StatusUnauthorized, "api key revoked")
				default:
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid api key")
				}
			}
			ctx := inject(c.Request().Context(), principal)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// Require 构造一个校验 principal 是否拥有指定权限的中间件。
// 它只检查全局权限位；project 归属校验由业务 handler/service 通过
// Principal.CanAccessProject 完成（因为 project ID 通常来自 path 参数，
// 在本中间件运行时可能尚未解析）。
//
// extract 由调用方传入 internal/api.PrincipalFromContext，避免反向依赖。
func Require(permission Permission, extract func(ctx context.Context) (Principal, bool)) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p, ok := extract(c.Request().Context())
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}
			if !p.HasPermission(permission) {
				return echo.NewHTTPError(http.StatusForbidden, "permission denied")
			}
			return next(c)
		}
	}
}
