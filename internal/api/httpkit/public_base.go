// 文件说明：public_base.go — 生成访问 URL 用的外部基址（planv6.7 §1.3 / §3.1）
//
// 用法
//
//	PublicBaseURL(c echo.Context, publicURL, override string) string  // 请求 Host / 转发头 → 外部基址；无配置时用请求自身
//
// 调用架构
//
//	[上游] internal/api（内置 MCP 管理面、站点、S3 访问链接）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/httpkit/public_base.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	PublicBaseURL(c echo.Context, publicURL, override string) string

package httpkit

import (
	"strings"

	"github.com/labstack/echo/v4"
)

// PublicBaseURL 返回生成访问 URL 用的外部基址（去掉结尾 "/"）。
//
// override（对应 sites.public_base_url 或其他可选基址）优先；空时回退 publicURL（http.public_url）；
// 再空时使用请求的 scheme+Host（不采信 X-Forwarded-Proto，只在 trusted_proxies 之内生效）。
//
// 注意最后一级只能用于展示 URL，不参与路由判定或鉴权。
func PublicBaseURL(c echo.Context, publicURL, override string) string {
	if v := strings.TrimSpace(override); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(publicURL); v != "" {
		return strings.TrimRight(v, "/")
	}
	return c.Scheme() + "://" + c.Request().Host
}
