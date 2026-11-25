// 文件说明：netguard.go — 出站网络防护（SSRF），供 http / web_fetch 工具与宿主 MCP 客户端共用
//
// 从 internal/agentbridge/mcp.go 下沉（planv6.8 §13.7）。防护分三层：
//   - CheckURL：scheme 白名单、禁止 userinfo、host 非空；
//   - GuardDialContext：在 Dialer.Control 里校验**实际连接 IP**（挡 DNS rebinding）；
//   - NewGuardedClient：禁止重定向、TLS ≥ 1.2、不复用连接（IP 校验不被连接池绕过）。
//
// AllowPrivate=true 只放行私网/回环（本机 CLI 显式 opt-in），永远不放行未指定地址与组播。

package tools

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// NetPolicy 是出站网络策略。零值：只允许 https 与 http、拒绝私网。
type NetPolicy struct {
	// AllowPrivate 放行回环 / 私网 / 链路本地地址（局域网服务）；默认拒绝。
	AllowPrivate bool
	// HTTPSOnly 为真时拒绝 http://。
	HTTPSOnly bool
	// Timeout 是单次请求超时；零值 30 秒。
	Timeout time.Duration
}

// ErrBlockedAddress 表示目标地址被出站策略拒绝。
var ErrBlockedAddress = errors.New("address is not allowed by network policy")

// CheckURL 做请求前的静态校验（不解析 DNS；IP 校验在连接时做）。
func (p NetPolicy) CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if p.HTTPSOnly {
			return nil, errors.New("http urls are disabled; use https")
		}
	default:
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.User != nil {
		return nil, errors.New("urls with userinfo are not allowed")
	}
	if u.Hostname() == "" {
		return nil, errors.New("url host is empty")
	}
	// 字面量 IP 提前拦截，错误更清楚；域名在连接时校验。
	if ip := net.ParseIP(u.Hostname()); ip != nil && p.blocked(ip) {
		return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, ip)
	}
	return u, nil
}

func (p NetPolicy) blocked(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 0 {
		return true
	}
	if p.AllowPrivate {
		return false
	}
	return IsBlockedIP(ip)
}

// IsBlockedIP 判定回环 / 私网 / 链路本地（含云元数据 169.254.169.254）/ 组播 / 未指定 / 0.0.0.0/8。
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 0 {
		return true
	}
	return false
}

// GuardDialContext 返回在实际连接 IP 上做校验的 DialContext（拒绝私网，宿主默认策略）。
func GuardDialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	return NetPolicy{}.DialContext()
}

// DialContext 返回按本策略校验连接 IP 的拨号函数。
func (p NetPolicy) DialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("cannot resolve %q", host)
			}
			if p.blocked(ip) {
				return fmt.Errorf("%w: %s", ErrBlockedAddress, ip)
			}
			return nil
		},
	}
	return dialer.DialContext
}

// NewGuardedTransport 返回加固的 Transport（不复用连接、TLS ≥ 1.2、拨号校验 IP）。
func (p NetPolicy) NewGuardedTransport() *http.Transport {
	return &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext:       p.DialContext(),
		Proxy:             nil,
	}
}

// NewGuardedClient 返回禁止重定向的加固客户端。
func (p NetPolicy) NewGuardedClient() *http.Client {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{
		Transport: p.NewGuardedTransport(),
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// 禁止跟随重定向：SSRF 最常用的绕过手段；调用方看到 3xx 自行决定。
			return http.ErrUseLastResponse
		},
	}
}
