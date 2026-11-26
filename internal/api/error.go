// 文件说明：error.go — Echo 层的错误兜底（HTTPErrorHandler）：E2B 路径分流 + 协议内核的通用处理器
//
// 用法
//
//	errorHandler(deps Dependencies) echo.HTTPErrorHandler   // NewRouter 装配：handler 未处理的错误按协议写出；E2B 路径用 E2B 错误体
//
// 调用架构
//
//	[上游] router.go
// │ 同包调用
//	         ▼
//	[本文件] internal/api/error.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/httpkit（WriteError / MapEchoError / SafeMessage 的实现）
//	[同包] backup_handler.go, data_compat.go, file_compat.go, function_compat.go, health.go, mcp_builtin.go, mcp_builtin_admin.go, mcp_builtin_session.go 等 28 个
//
// 主要符号
//
//	errorHandler(deps Dependencies) echo.HTTPErrorHandler
//
// APIError / Error / WriteError / sentinelErrorMappings / mapEchoError 等已迁到 internal/api/httpkit（D3-a）；
// 同包旧名经 kernel_compat_test.go 转发（仅测试；生产代码直接用 httpkit）。

package api

import (
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/api/sandboxhttp"
)

func errorHandler(deps Dependencies) echo.HTTPErrorHandler {
	inner := httpkit.ErrorHandler(deps.Logger)
	return func(err error, c echo.Context) {
		// E2B 兼容面（含认证 / BodyLimit 等中间件产生的错误）统一用 E2B 的错误体；其余交给协议内核。
		if !c.Response().Committed && sandboxhttp.IsE2BPath(c.Request().URL.Path) {
			_ = sandboxhttp.WriteE2BErr(c, err)
			return
		}
		inner(err, c)
	}
}
