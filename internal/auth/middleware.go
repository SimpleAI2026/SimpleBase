// 文件说明：middleware.go — Echo 中间件：AuthMiddlewareWithDelegation 完成 JWT / 委托 / API Key 认证并注入 Principal，
//
// 用法
//
//	Require(permission Permission, extract func(ctx context.Context) (Principal, bool)) ...  // Require 构造一个校验 principal 是否拥有指定权限的中间件
//	同包调用：DelegableContextKey → delegation.go；RequireDelegable() → delegation.go；Require() → principal.go
//
// 调用架构
//
//	[上游] godatabase/bench, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/middleware.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] api_key_repository.go, cache.go, delegation.go, id.go, jwt.go 等 15 个
//	[关键引用] DelegableContextKey ← delegation.go
//
// 主要符号
//
//	Require(permission Permission, extract func(ctx context.Context) (Principal, bool)) ...
//	const DelegableContextKey
//	RequireDelegable(permission Permission, extract func(ctx context.Context) ...
//	type SessionAuthenticator interface {…}
//	AuthMiddlewareWithDelegation(sessions SessionAuthenticator, keys *Service, inject ...

package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// Require 构造一个校验 principal 是否拥有指定权限的中间件。
// 它只检查全局权限位；project 归属校验由业务 handler/service 通过
// Principal.CanAccessProject 完成（因为 project ID 通常来自 path 参数，
// 在本中间件运行时可能尚未解析）。
//
// extract 由调用方传入 internal/api.PrincipalFromContext，避免反向依赖。
func Require(permission Permission, extract func(ctx context.Context) (Principal, bool)) echo.MiddlewareFunc {
	return RequireDelegable(permission, extract, nil)
}

// DelegableContextKey 是 Echo context 上 delegable 标记的键（planv6.0 D1）。
const DelegableContextKey = "simplebase:delegable"

// RequireDelegable 在 Require 基础上执行委托边界（planv6.0 D1/D2）：
// 委托主体（Principal.Delegated）只能访问带 delegable 标记的路由，
// 未标记路由默认拒绝（403），防止白名单字符串表与路由漂移。
// delegable 为 nil 时表示该路由不可委托。
func RequireDelegable(permission Permission, extract func(ctx context.Context) (Principal, bool), delegable echo.MiddlewareFunc) echo.MiddlewareFunc {
	check := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p, ok := extract(c.Request().Context())
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}
			if !p.HasPermission(permission) {
				return echo.NewHTTPError(http.StatusForbidden, "permission denied")
			}
			if p.Delegated && c.Get(DelegableContextKey) != true {
				return echo.NewHTTPError(http.StatusForbidden, "delegated principal cannot access this route")
			}
			return next(c)
		}
	}
	if delegable == nil {
		return check
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return delegable(check(next))
	}
}

// SessionAuthenticator 抽象 JWT 通道校验（由 SessionService 适配，避免 middleware 依赖具体类型）。
type SessionAuthenticator interface {
	VerifyAccess(token string) (JWTClaims, error)
	PrincipalFromClaims(ctx context.Context, claims JWTClaims) (Principal, error)
}

// AuthMiddlewareWithDelegation 在 JWT 通道里识别助手委托令牌。
// registry 为 nil 时委托令牌走原登录态校验（通常会 401）。
func AuthMiddlewareWithDelegation(sessions SessionAuthenticator, keys *Service, inject func(ctx context.Context, p Principal) context.Context, secret string, registry *DelegationRegistry) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw, err := ExtractBearerToken(c.Request().Header.Get(echo.HeaderAuthorization))
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing or malformed authorization header")
			}
			if LooksLikeJWT(raw) {
				// §8.2.6 密钥域隔离：aud=enduser 的 token 属于终端用户域，
				// 在平台会话/委托/API Key 链上一律拒绝（防白名单漂移越权）。
				//
				// 同一次验签结果复用于 aud 判定与委托识别；判断顺序仍是「先 aud 后委托」。
				now := time.Now()
				var platformClaims JWTClaims
				platformVerified := false
				if secret != "" {
					if claims, err := VerifyJWT(secret, raw, now); err == nil {
						platformClaims, platformVerified = claims, true
					}
				}
				if platformVerified && platformClaims.Audience == endUserAudience {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
				}
				if platformVerified && platformClaims.Type == DelegationType {
					if registry == nil || registry.Revoked(platformClaims.JWTID, now) {
						return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
					}
					p, err := PrincipalFromDelegation(platformClaims)
					if err != nil {
						return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
					}
					// planv6.0 D1：路由级委托边界。这里只注入标记后的 Principal，
					// 路径判断全部下沉到路由级 RequireDelegable（默认拒绝），
					// 消除字符串白名单与真实路由的漂移。
					p.Delegated = true
					ctx := inject(c.Request().Context(), p)
					c.SetRequest(c.Request().WithContext(ctx))
					return next(c)
				}
				if sessions == nil {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
				}
				claims, err := sessions.VerifyAccess(raw)
				if err != nil {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
				}
				p, err := sessions.PrincipalFromClaims(c.Request().Context(), claims)
				if err != nil {
					if errors.Is(err, ErrUserDisabled) {
						return echo.NewHTTPError(http.StatusUnauthorized, "user disabled")
					}
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
				}
				ctx := inject(c.Request().Context(), p)
				c.SetRequest(c.Request().WithContext(ctx))
				return next(c)
			}
			// API Key 通道（SDK / 程序化调用）。
			if keys == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid api key")
			}
			principal, err := keys.Authenticate(c.Request().Context(), raw)
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
