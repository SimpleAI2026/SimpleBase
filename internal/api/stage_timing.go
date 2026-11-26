// 文件说明：stage_timing.go — api-db-perf-validation-plan §2.1 的请求分段计时
//
// 用法
//
//	(t *StageTimer) WithContext(ctx context.Context) context.Context  // WithContext 返回携带本 timer 的 context（供 adapter/handler 深层取用）
//	StageTimerFrom(ctx context.Context) *StageTimer  // StageTimerFrom 取出 ctx 上的 timer
//	同包调用：Done() → 等；StageScope → 等；Release() → 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/stage_timing.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase, internal/auth
//	[同包] backup_handler.go, data_compat.go, error.go, file_compat.go, function_compat.go, health.go, mcp_builtin.go, mcp_builtin_admin.go 等 28 个
//	[关键引用] Done() ←（见各子包）
//
// 主要符号
//
//	type Stage string
//	const StageTotal
//	const StageAuth
//	const StageProject
//	const StageDecode
//	const StageCatalog
//	... 另有 54 个顶层符号

package api

import (
	"context"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
)

// httpkit.Stage / httpkit.StageTimer / PerfStages / newStageTimer / stageDurationsJSON 已迁到 internal/api/httpkit（D3-a），
// 生产代码直接使用 httpkit 导出名；同包测试经 kernel_compat_test.go 使用旧名。

// perfStageMiddleware / stageTimingRecorder / httpkit.LoggerLike 已迁到 internal/api/httpkit。

func timedAuthMiddleware(inner echo.MiddlewareFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		wrapped := inner(func(c echo.Context) error {
			// 认证成功放行前先结算 auth 段：next 的耗时不再计入 auth。
			httpkit.StageTimerFrom(c.Request().Context()).Settle(httpkit.StageAuth)
			return next(c)
		})
		return func(c echo.Context) error {
			timer := httpkit.StageTimerFrom(c.Request().Context())
			if timer == nil {
				return wrapped(c)
			}
			scope := timer.Scoped(httpkit.StageAuth)
			err := wrapped(c)
			scope.Done() // 认证失败路径兜底；已 Settle 时为 no-op
			return err
		}
	}
}

// 无 timer（分段计时关闭）时静默丢弃。
type stageAuthObserver struct{}

// newStageAuthObserver 返回桥接观察者；auth 侧接口已 nil-safe。
func newStageAuthObserver() *stageAuthObserver {
	return &stageAuthObserver{}
}

func (o *stageAuthObserver) ObserveAuthStage(ctx context.Context, stage auth.AuthStage, d time.Duration) {
	timer := httpkit.StageTimerFrom(ctx)
	if timer == nil {
		return
	}
	switch stage {
	case auth.AuthStageLockWait:
		timer.Observe(httpkit.StageAuthLock, d)
	case auth.AuthStageUserLoad:
		timer.Observe(httpkit.StageAuthUser, d)
	case auth.AuthStageUserConn:
		timer.Observe(httpkit.StageAuthUserConn, d)
	case auth.AuthStageUserQuery:
		timer.Observe(httpkit.StageAuthUserQuery, d)
	case auth.AuthStageUserScan:
		timer.Observe(httpkit.StageAuthUserScan, d)
	case auth.AuthStageFlightWait:
		timer.Observe(httpkit.StageAuthFlightWait, d)
	case auth.AuthStageFlightLoad:
		timer.Observe(httpkit.StageAuthFlightLoad, d)
	case auth.AuthStageProjectExpand:
		timer.Observe(httpkit.StageAuthProject, d)
	case auth.AuthStageSessionLoad:
		timer.Observe(httpkit.StageAuthSession, d)
	case auth.AuthStageKeyLoad:
		timer.Observe(httpkit.StageAuthKey, d)
	case auth.AuthStageSessionUpdate:
		timer.Observe(httpkit.StageAuthSessionUpdate, d)
	case auth.AuthStageSessionIssue:
		timer.Observe(httpkit.StageAuthSessionIssue, d)
	}
}

func (o *stageAuthObserver) ObserveAuthCache(ctx context.Context, state auth.AuthCacheState) {
	timer := httpkit.StageTimerFrom(ctx)
	if timer == nil {
		return
	}
	switch state {
	case auth.AuthCacheHit:
		timer.SetMeta("auth_cache", 1)
	case auth.AuthCacheMiss:
		timer.SetMeta("auth_cache", 0)
	}
}
