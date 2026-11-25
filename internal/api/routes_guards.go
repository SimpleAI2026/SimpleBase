// 文件说明：routes_guards.go — /v1 路由共用的权限闸门集合
//
// 用法
//
//	g := newRouteGuards(deps); p.GET("/x", h.X, g.require(auth.DatabaseRead), g.writeGate)
//
// 调用架构
//
//	NewRouter（router.go）→ mountV1Routes / mountAgentRoutes / mountGoFunctionAndCronRoutes 共用同一份 routeGuards
//	权限闸门统一经 routeGuards（routes_guards.go），禁止在路由块里直接写 auth.Require。
//
// 主要符号
//
//	type routeGuards struct {…}；func newRouteGuards(deps Dependencies) routeGuards

package api

import (
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// routeGuards 把三种路由闸门收成一个值，替代「闭包逐个当参数传」。
//
//   - require：不带 delegable，路由对委托令牌默认拒绝（planv6.0 D1）。
//   - delegable：在 require 基础上标记路由可被委托令牌访问（planv6.0 §3.1 表）。
//   - writeGate：统一写门禁（纵深防御）。只读实例当前不启动系统库、不挂这些路由；
//     若未来只读实例也打开系统库，缺少这层门禁的写接口会立即变成可写漏洞。
//     写路由是否漏挂由 writeGateRoutes 驱动测试保证。
//
// require 与 delegable 都先经 policyGate，API Key policy 对所有挂了它们的路由生效。
type routeGuards struct {
	require   func(auth.Permission) echo.MiddlewareFunc
	delegable func(auth.Permission) echo.MiddlewareFunc
	writeGate echo.MiddlewareFunc
}

func newRouteGuards(deps Dependencies) routeGuards {
	return routeGuards{
		require: func(perm auth.Permission) echo.MiddlewareFunc {
			return func(next echo.HandlerFunc) echo.HandlerFunc {
				return policyGate(auth.Require(perm, PrincipalFromContext)(next))
			}
		},
		delegable: func(perm auth.Permission) echo.MiddlewareFunc {
			return func(next echo.HandlerFunc) echo.HandlerFunc {
				return policyGate(auth.RequireDelegable(perm, PrincipalFromContext, auth.DelegationMark())(next))
			}
		},
		writeGate: func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				if !deps.Config.Instance.Writable {
					return WriteError(c, godatabase.ErrWriterUnavailable)
				}
				return next(c)
			}
		},
	}
}
