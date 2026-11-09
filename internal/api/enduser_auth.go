// 文件说明：enduser_auth.go — 终端用户守卫（production-readiness-plan §8.2.6）
//
// 用法
//
//	endUserAuth(secret) echo.MiddlewareFunc      // 只认 aud=enduser JWT；注入 ctx
//	endUserClaimsFromContext(ctx) (JWTClaims, bool)
//
// 密钥域隔离：平台 AuthMiddlewareWithDelegation 对 aud=enduser 显式拒绝（middleware.go），
// 本守卫对非 enduser token 拒绝——两个域互不可达。
package api

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

type endUserCtxKey struct{}

// endUserAuth 构造终端用户守卫：验 aud=enduser JWT、强制项目匹配路由参数。
func endUserAuth(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw, err := auth.ExtractBearerToken(c.Request().Header.Get(echo.HeaderAuthorization))
			if err != nil {
				return WriteError(c, NewAPIError(401, "unauthorized", "end user token required", RequestIDFromContext(c.Request().Context())))
			}
			projectID := c.Param("projectID")
			claims, err := auth.VerifyEndUserToken(secret, raw, projectID)
			if err != nil {
				return WriteError(c, NewAPIError(401, "unauthorized", "invalid end user token", RequestIDFromContext(c.Request().Context())))
			}
			ctx := context.WithValue(c.Request().Context(), endUserCtxKey{}, claims)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// endUserClaimsFromContext 取出终端用户声明。
func endUserClaimsFromContext(ctx context.Context) (auth.JWTClaims, bool) {
	claims, ok := ctx.Value(endUserCtxKey{}).(auth.JWTClaims)
	return claims, ok
}
