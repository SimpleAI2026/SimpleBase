// 文件说明：middleware_bodylimit.go — 全局请求体上限及其例外（对象上传、沙盒文件上传走各自路由上的更大上限）
//
// 用法
//
//	同包调用：bodyLimit() / skipLargeUploadBodyLimit() ← router.go 的 useGlobalMiddleware
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/middleware_bodylimit.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方（echo）
//	[同包] backup_handler.go, data_compat.go, error.go, file_compat.go, function_compat.go, health.go, mcp_builtin.go, mcp_builtin_admin.go 等 28 个
//
// 主要符号
//
//	const s3UploadBodyLimit
//	skipS3UploadBodyLimit(...) / skipLargeUploadBodyLimit(...) / bodyLimit(...)

package api

import (
	"github.com/linkxzhou/SimpleBase/internal/api/sandboxhttp"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// s3UploadBodyLimit 是对象上传的请求体上限。
// 控制台单文件最大 50MiB（S3Manager MAX_UPLOAD_BYTES），另留 64KiB 给 multipart 帧。
const s3UploadBodyLimit = 50<<20 + 64<<10

// skipS3UploadBodyLimit 让 POST /s3/objects 跳过全局 max_request_bytes，改由路由上的更大上限约束。
func skipS3UploadBodyLimit(enabled bool) middleware.Skipper {
	if !enabled {
		return middleware.DefaultSkipper
	}
	const suffix = "/s3/objects"
	return func(c echo.Context) bool {
		r := c.Request()
		if r.Method != http.MethodPost || r.URL == nil {
			return false
		}
		p := r.URL.Path
		return len(p) >= len(suffix) && p[len(p)-len(suffix):] == suffix
	}
}

// skipLargeUploadBodyLimit 允许沙盒文件上传（envd POST /files）使用自己的上限，而不是全局 max_request_bytes。
func skipLargeUploadBodyLimit(s3Enabled, sandboxEnabled bool) middleware.Skipper {
	s3 := skipS3UploadBodyLimit(s3Enabled)
	return func(c echo.Context) bool {
		if s3(c) {
			return true
		}
		if c.Request().URL == nil || c.Request().Method != http.MethodPost {
			return false
		}
		p := c.Request().URL.Path
		if strings.HasSuffix(p, "/import") {
			return true
		}
		return sandboxEnabled && (p == "/files" || p == sandboxhttp.E2BEnvdPrefix+"/files")
	}
}

// bodyLimit 将字节数转换为 echo BodyLimit 字符串（K/M）。
// BodyLimit 解析支持 1024 形式与 "1M"/"512K" 形式；这里输出后者。
func bodyLimit(bytes int64) string {
	if bytes <= 0 {
		return "1M"
	}
	const k = 1 << 10
	const m = 1 << 20
	if bytes%m == 0 {
		return strconv.Itoa(int(bytes/m)) + "M"
	}
	if bytes%k == 0 {
		return strconv.Itoa(int(bytes/k)) + "K"
	}
	return strconv.FormatInt(bytes, 10)
}
