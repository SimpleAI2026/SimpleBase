// 文件说明：web.go — 将前端构建产物嵌入二进制，并提供 SPA 静态文件服务
//
// 用法
//
//	StaticFS() fs.FS  // StaticFS 返回剥离 "dist" 前缀后的根文件系统，用于 http.FileServer
//	Register(e *echo.Echo)  // Register 将静态文件服务挂载到 echo 实例
//
// 调用架构
//
//	[上游] internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/web/web.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	StaticFS() fs.FS
//	Register(e *echo.Echo)
//	... 另有 4 个顶层符号

package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

//go:embed all:dist
var distFS embed.FS

// StaticFS 返回剥离 "dist" 前缀后的根文件系统，用于 http.FileServer。
func StaticFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist 目录缺失时 embed 会在编译期失败，这里不会到达。
		panic("web: embedded dist fs: " + err.Error())
	}
	return sub
}

// indexHTML 缓存 index.html 内容，用于 SPA fallback。
// 若嵌入资源中不含 index.html（仅占位文件的情况），则懒加载为空，
// 此时 SPA fallback 会返回 404，提示需要先运行 build.sh 构建前端。
var indexHTML = loadIndexHTML()

func loadIndexHTML() []byte {
	b, err := distFS.ReadFile("dist/index.html")
	if err != nil {
		return nil
	}
	return b
}

// Register 将静态文件服务挂载到 echo 实例。
// 任何未匹配到已注册路由的 GET 请求：
//   - 若对应静态文件存在，则返回该文件；
//   - 否则返回 index.html（SPA fallback），支持 Vue history 模式前端路由。
//
// 该函数应在所有 API 路由注册完成后调用，作为兜底处理。
// Echo 按注册顺序匹配路由，因此 /v1、/health、/metrics 等先注册的 API
// 路由会优先命中，不会被此通配路由拦截。
func Register(e *echo.Echo) {
	staticFS := StaticFS()
	fileServer := http.FileServer(http.FS(staticFS))

	// 显式注册 "/"：e.Group("", mw) 会为根路径注册带鉴权中间件的 Any("")，
	// 精确匹配优先于 "/*" 通配，导致访问根路径返回 401。同方法同路径后注册者覆盖。
	e.GET("/", spaHandler(staticFS, fileServer))
	e.GET("/*", spaHandler(staticFS, fileServer))
}

func spaHandler(staticFS fs.FS, fileServer http.Handler) echo.HandlerFunc {
	return func(c echo.Context) error {
		// 未构建前端时返回明确提示，避免返回空白页面。
		if indexHTML == nil {
			return c.String(http.StatusServiceUnavailable,
				"管理端未构建：请先运行 ./build.sh 编译前端。\n")
		}

		reqPath := strings.TrimPrefix(c.Request().URL.Path, "/")

		// 空路径或无文件扩展名的路径视为前端路由，返回 index.html。
		if reqPath == "" || !strings.Contains(reqPath, ".") {
			return c.Blob(http.StatusOK, "text/html; charset=utf-8", indexHTML)
		}

		// 检查静态文件是否存在；不存在则回退到 index.html（SPA fallback）。
		if _, err := fs.Stat(staticFS, reqPath); err != nil {
			return c.Blob(http.StatusOK, "text/html; charset=utf-8", indexHTML)
		}

		fileServer.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}
