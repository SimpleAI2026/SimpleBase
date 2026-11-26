package iamhttp

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

type endUserCtxKey struct{}

// EndUserAuth 构造终端用户守卫：验 aud=enduser JWT、强制项目匹配路由参数。
func EndUserAuth(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw, err := auth.ExtractBearerToken(c.Request().Header.Get(echo.HeaderAuthorization))
			if err != nil {
				return httpkit.WriteError(c, httpkit.NewAPIError(401, "unauthorized", "end user token required", httpkit.RequestID(c)))
			}
			projectID := c.Param("projectID")
			claims, err := auth.VerifyEndUserToken(secret, raw, projectID)
			if err != nil {
				return httpkit.WriteError(c, httpkit.NewAPIError(401, "unauthorized", "invalid end user token", httpkit.RequestID(c)))
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
