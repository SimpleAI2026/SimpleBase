// 文件说明：routes_function.go — 云函数 / 定时任务 / 函数触发器管理面与 /go 调用面路由挂载
//
// 用法
//
//	mountGoFunctionAndCronRoutes(p, deps, g); mountGoRoutes(e, deps)
//
// 调用架构
//
//	NewRouter（router.go）→ NewRouter → mountGoRoutes；mountV1Routes → mountSystemRoutes → mountGoFunctionAndCronRoutes
//	权限闸门统一经 routeGuards（routes_guards.go），禁止在路由块里直接写 auth.Require。
//
// 主要符号
//
//	func mountGoFunctionAndCronRoutes(...)；func mountGoRoutes(e *echo.Echo, deps Dependencies)

package api

import (
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/api/functionhttp"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// mountGoFunctionAndCronRoutes 挂载云函数 + 定时任务管理面路由块，
// 两块均可委托（delegable）；注册顺序即 OpenAPI 输出顺序，不得随意调整。
// 调用方保证 deps.System != nil。
func mountGoFunctionAndCronRoutes(p *echo.Group, deps Dependencies, g routeGuards) {
	// 云函数管理面（ui-gofunction-plan §7.1）。writable=false 时写操作返回 503。
	gh := functionhttp.NewGoFunctionHandler(deps.System, deps.Config.Instance.Writable, deps.Audit, deps.Config.Limits.MaxConcurrentGoRuns).WithRunner(deps.FunctionRunner)
	p.GET("/gofunctions", gh.List, g.delegable(auth.DatabaseRead))
	p.POST("/gofunctions", gh.Create, g.delegable(auth.DatabaseWrite), g.writeGate)
	p.GET("/gofunctions/:name", gh.Get, g.delegable(auth.DatabaseRead))
	p.PATCH("/gofunctions/:name", gh.Patch, g.delegable(auth.DatabaseWrite), g.writeGate)
	p.DELETE("/gofunctions/:name", gh.Delete, g.delegable(auth.DatabaseWrite), g.writeGate)
	p.GET("/gofunctions/:name/versions", gh.ListVersions, g.delegable(auth.DatabaseRead))
	p.POST("/gofunctions/:name/versions", gh.CreateVersion, g.delegable(auth.DatabaseWrite), g.writeGate)
	p.GET("/gofunctions/:name/versions/:ver", gh.GetVersion, g.delegable(auth.DatabaseRead))
	p.POST("/gofunctions/:name/versions/:ver/activate", gh.ActivateVersion, g.delegable(auth.DatabaseWrite), g.writeGate)
	p.POST("/gofunctions/:name/versions/:ver/test", gh.TestVersion, g.delegable(auth.DatabaseWrite), g.writeGate)

	// 定时任务管理面（ui-cronjob-plan §6）。writable=false 时写操作返回 503。
	cj := functionhttp.NewCronJobHandler(deps.System, deps.Config.Instance.Writable, deps.CronScheduler, deps.Audit).WithUsage(deps.Usage)
	p.GET("/cron-jobs", cj.List, g.delegable(auth.DatabaseRead))
	p.POST("/cron-jobs", cj.Create, g.delegable(auth.DatabaseWrite), g.writeGate)
	p.GET("/cron-jobs/:jobID", cj.Get, g.delegable(auth.DatabaseRead))
	p.PATCH("/cron-jobs/:jobID", cj.Update, g.delegable(auth.DatabaseWrite), g.writeGate)
	p.DELETE("/cron-jobs/:jobID", cj.Delete, g.delegable(auth.DatabaseWrite), g.writeGate)
	p.GET("/cron-jobs/:jobID/runs", cj.ListRuns, g.delegable(auth.DatabaseRead))
	p.POST("/cron-jobs/:jobID/trigger", cj.Trigger, g.delegable(auth.DatabaseWrite), g.writeGate)
	if deps.FunctionTriggerHandler != nil {
		th := deps.FunctionTriggerHandler
		p.GET("/function-triggers", th.List, g.require(auth.ProjectAdmin))
		p.POST("/function-triggers", th.Create, g.require(auth.ProjectAdmin), g.writeGate)
		p.GET("/function-triggers/:id", th.Get, g.require(auth.ProjectAdmin))
		p.PATCH("/function-triggers/:id", th.Update, g.require(auth.ProjectAdmin), g.writeGate)
		p.DELETE("/function-triggers/:id", th.Delete, g.require(auth.ProjectAdmin), g.writeGate)
		p.POST("/function-triggers/:id/invoke", th.Invoke, g.require(auth.DatabaseWrite), g.writeGate)
	}
}

// mountGoRoutes 挂载对外调用面 /go/:projectID/:name/:functionName（§7.2）。
// 必须在 web.Register（SPA GET fallback）之前执行——由 NewRouter 调用顺序保证。
// 与 mountV1Routes 同一依赖守卫，复用认证与 project context 中间件。
func mountGoRoutes(e *echo.Echo, deps Dependencies) {
	if deps.System == nil {
		return
	}
	authMW := deps.authMW()
	delegable := func(perm auth.Permission) echo.MiddlewareFunc {
		return auth.RequireDelegable(perm, httpkit.PrincipalFromContext, auth.DelegationMark())
	}
	h := functionhttp.NewGoFunctionHandler(deps.System, deps.Config.Instance.Writable, deps.Audit, deps.Config.Limits.MaxConcurrentGoRuns).WithRunner(deps.FunctionRunner)
	goGrp := e.Group("/go/:projectID", authMW, newRateLimiter(deps.Config.Limits.RateLimit, deps.Metrics).middleware(postAuthStage), projectContextMiddlewareEcho(deps.Catalog))
	goGrp.POST("/:name/:functionName", h.Invoke, delegable(auth.DatabaseRead))
	// 405 JSON：防止浏览器 GET 掉进 SPA index.html（验收 G11）；GET 不可委托。
	goGrp.GET("/:name/:functionName", h.MethodNotAllowed, auth.Require(auth.DatabaseRead, httpkit.PrincipalFromContext))
}
