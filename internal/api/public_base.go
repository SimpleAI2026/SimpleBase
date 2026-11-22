// 文件说明：public_base.go — 生成访问 URL 用的外部基址（planv6.7 §1.3 / §3.1）
//
// 用法
//
//	publicBaseURL(c, httpPublicURL, override)  // 三级回退：sites.public_base_url → http.public_url → c.Scheme+"://"+c.Host
//
// 优先级按配置->请求回退，最后一级只用于展示，不参与路由或鉴权；
// 反向代理后面的生产环境必须配置前两项之一，否则会拿到内网地址。

package api

import (
	"strings"

	"github.com/labstack/echo/v4"
)

// publicBaseURL 返回生成访问 URL 用的外部基址（去掉结尾 "/"）。
//
// override（对应 sites.public_base_url 或其他可选基址）优先；空时回退 publicURL（http.public_url）；
// 再空时使用请求的 scheme+Host（不采信 X-Forwarded-Proto，只在 trusted_proxies 之内生效）。
//
// 注意最后一级只能用于展示 URL，不参与路由判定或鉴权。
func publicBaseURL(c echo.Context, publicURL, override string) string {
	if v := strings.TrimSpace(override); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(publicURL); v != "" {
		return strings.TrimRight(v, "/")
	}
	return c.Scheme() + "://" + c.Request().Host
}
