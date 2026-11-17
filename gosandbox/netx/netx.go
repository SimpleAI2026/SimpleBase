// 文件说明：netx.go — 沙盒出网的三档策略与私网拦截
//
// 三档，默认 none：
//
//	none              netns 里只有 lo。入站仍可用（走 agent 中继），
//	                  所以「不许出网」不等于「无法被访问」。
//	egress            pasta 提供用户态出网（rootless，不需要 root、不需要 veth）。
//	egress-allowlist  同 egress，但只放行白名单域名解析出的地址。
//
// 为什么默认 none：改这个包之前，沙盒根本没有 --unshare-net，也就是说
// 它能直接访问宿主网络与云元数据地址。这是 SSRF 的经典落点，必须先堵住，
// 再谈「怎么放行」。
//
// 必须挡的地址（云上 SSRF 最常见的落点，见计划文档 §6.5）：
//
//	169.254.0.0/16    链路本地，含 169.254.169.254 元数据
//	100.64.0.0/10     运营商级 NAT（云上常作内部服务网段）
//	10.0.0.0/8        私网
//	172.16.0.0/12     私网
//	192.168.0.0/16    私网
//	127.0.0.0/8       回环（沙盒内的 lo 已在 netns 里，不需要出宿主回环）
//	0.0.0.0/8         本网络
//	::/128           未指定
//	fc00::/7          IPv6 唯一本地
//	fe80::/10         IPv6 链路本地
//
// 用法
//
//	mode, err := netx.ParseMode("egress")
//	if err := netx.CheckAllowed(ip, netx.Policy{}); err != nil { … }
//
// 调用架构
//
//	[上游] gosandbox/box（launch）、gosandbox/vm/box、internal/sandbox
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/netx/netx.go
//	         │
//	         ▼
//	[下游] 仅标准库
//	[同包] pasta_linux.go, pasta_other.go, allowlist.go
//
// 主要符号
//
//	type Mode string
//	const ModeNone / ModeEgress / ModeAllowlist
//	ParseMode(s string) (Mode, error)
//	type Policy struct {…}
//	BlockedCIDRs() []netip.Prefix
//	CheckAllowed(ip netip.Addr, p Policy) error

package netx

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// Mode 是出网档位。
type Mode string

const (
	// ModeNone 不提供出网（netns 里只有 lo）。
	ModeNone Mode = "none"
	// ModeEgress 由 pasta 提供用户态出网，私网地址仍被拦截。
	ModeEgress Mode = "egress"
	// ModeAllowlist 只放行白名单域名解析出的地址。
	ModeAllowlist Mode = "egress-allowlist"
)

// 错误分类。
var (
	// ErrBlockedAddress 表示目标地址在必须拦截的范围里。
	ErrBlockedAddress = errors.New("netx: address is blocked")
	// ErrNotAllowed 表示目标域名不在白名单里。
	ErrNotAllowed = errors.New("netx: host is not in the allowlist")
	// ErrUnsupported 表示当前环境没有出网能力（缺 pasta）。
	ErrUnsupported = errors.New("netx: egress is not available on this host")
)

// ParseMode 解析档位字符串。空值按 none 处理（最安全的一档）。
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none":
		return ModeNone, nil
	case "egress":
		return ModeEgress, nil
	case "egress-allowlist", "allowlist":
		return ModeAllowlist, nil
	}
	return "", fmt.Errorf("netx: unknown network mode %q (want none, egress or egress-allowlist)", s)
}

// Policy 是出网策略。
type Policy struct {
	// Mode 是档位。
	Mode Mode
	// Allow 是白名单域名（仅 ModeAllowlist 生效）。
	//
	// 支持两种写法：
	//	"example.com"     只匹配它自己
	//	"*.example.com"   匹配任意子域（不匹配 example.com 本身）
	Allow []string
	// AllowPrivate 为真时放行私网地址。
	//
	// 只应在「沙盒之间要互相访问」这类内部部署里打开；默认关。
	AllowPrivate bool
	// AllowMetadata 为真时放行 169.254.169.254。
	//
	// 存在的唯一理由是测试与本地调试；生产永远不该打开。
	AllowMetadata bool
}

// 必须拦截的网段。
var blocked = mustPrefixes(
	"169.254.0.0/16",
	"100.64.0.0/10",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.0/8",
	"0.0.0.0/8",
	"fc00::/7",
	"fe80::/10",
)

// metadataAddr 是云元数据地址。
var metadataAddr = netip.MustParseAddr("169.254.169.254")

// BlockedCIDRs 返回必须拦截的网段（含 ::/128）。
//
// 返回副本：调用方改了也不会污染包级状态。
func BlockedCIDRs() []netip.Prefix {
	out := make([]netip.Prefix, 0, len(blocked)+1)
	out = append(out, blocked...)
	out = append(out, netip.MustParsePrefix("::/128"))
	return out
}

// CheckAllowed 判断一个地址能否出网。
//
// 顺序是有意的：先查元数据，再查私网，最后查档位。
// 元数据地址即使 AllowPrivate 打开也要先被 AllowMetadata 单独放行——
// 它落在 169.254/16 里，把两者混成一个开关太容易误开。
func CheckAllowed(ip netip.Addr, p Policy) error {
	if p.Mode == ModeNone {
		return fmt.Errorf("%w: network mode is none", ErrBlockedAddress)
	}
	ip = ip.Unmap()
	if !ip.IsValid() {
		return fmt.Errorf("%w: invalid address", ErrBlockedAddress)
	}
	// 元数据地址单独判定，且判定通过后直接放行——它落在链路本地段里，
	// 若继续往下走会被 IsLinkLocalUnicast 拒掉。只豁免这一个地址，
	// 不放行整个 169.254/16。
	if ip == metadataAddr {
		if p.AllowMetadata {
			return nil
		}
		return fmt.Errorf("%w: %s is the instance metadata endpoint", ErrBlockedAddress, ip)
	}
	if !p.AllowPrivate {
		for _, pref := range blocked {
			if pref.Contains(ip) {
				return fmt.Errorf("%w: %s is in %s", ErrBlockedAddress, ip, pref)
			}
		}
	}
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
		return fmt.Errorf("%w: %s is not a routable unicast address", ErrBlockedAddress, ip)
	}
	return nil
}

// HostAllowed 判断一个域名是否在白名单里。
//
// 匹配规则见 Policy.Allow 的说明。域名比较不区分大小写，
// 末尾的点会被忽略（FQDN 写法）。
func HostAllowed(host string, p Policy) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return false
	}
	for _, rule := range p.Allow {
		rule = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(rule), "."))
		if rule == "" {
			continue
		}
		if suffix, ok := strings.CutPrefix(rule, "*."); ok {
			// "*.example.com" 匹配 a.example.com 与 a.b.example.com，
			// 但不匹配 example.com 本身（与 TLS 证书的 wildcard 语义一致）。
			if strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if rule == host {
			return true
		}
	}
	return false
}

// mustPrefixes 解析网段列表；解析失败是编码错误，直接 panic。
func mustPrefixes(list ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			panic("netx: bad cidr " + s + ": " + err.Error())
		}
		out = append(out, p)
	}
	return out
}
