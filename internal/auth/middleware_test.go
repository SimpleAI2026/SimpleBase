// 文件说明：middleware_test.go — 仅测试使用的 API Key 中间件（planv6.6 §1.3 T-2）
//
// APIKeyMiddleware 是 AuthMiddlewareWithDelegation 里 API Key 通道的早期独立版本，
// 生产路由全部走 AuthMiddlewareWithDelegation；把这份实现留在 _test.go 里，
// 既保留既有测试覆盖面，又不让它继续作为生产 API 暴露。

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
