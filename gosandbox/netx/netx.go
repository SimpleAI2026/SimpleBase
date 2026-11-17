// 文件说明：netx.go — 沙盒出网档位
//
// 两档，默认 none：
//
//	none              netns 里只有 lo。入站仍可用（走 agent 中继），
//	                  所以「不许出网」不等于「无法被访问」。
//	egress            pasta 提供用户态出网（rootless，不需要 root、不需要 veth）。
//
// 为什么默认 none：沙盒没有 --unshare-net 时能直接访问宿主网络与云元数据地址，
// 这是 SSRF 的经典落点，必须先堵住，再谈「怎么放行」。
//
// 不提供的能力（已于 2026-11-17 移除，见 plan/planv6.6/gosandbox-cleanup-plan.md GSB-10）：
// 域名白名单（egress-allowlist）与地址层拦截（元数据 / 私网）。它们曾以
// DNSProxy 与 CheckAllowed 的形式实现过，但从未接入生产路径，保留只会让人误以为
// 已经生效。egress 档**没有**地址或域名层面的限制，相关风险见 plan/planv6.6/
// gosandbox-egress-isolation-plan.md。
//
// 用法
//
//	mode, err := netx.ParseMode("egress")
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
//	[同包] pasta_linux.go, pasta_other.go
//
// 主要符号
//
//	type Mode string
//	const ModeNone / ModeEgress
//	ParseMode(s string) (Mode, error)
//	type Policy struct {…}
//	var ErrUnsupported

package netx

import (
	"errors"
	"fmt"
	"strings"
)

// Mode 是出网档位。
type Mode string

const (
	// ModeNone 不提供出网（netns 里只有 lo）。
	ModeNone Mode = "none"
	// ModeEgress 由 pasta 提供用户态出网。
	//
	// 注意：这一档不做任何地址或域名过滤，沙盒能访问宿主可达的一切
	// （含云元数据与私网）。需要出网时请自行评估风险。
	ModeEgress Mode = "egress"
)

// ErrUnsupported 表示当前环境没有出网能力（缺 pasta）。
var ErrUnsupported = errors.New("netx: egress is not available on this host")

// ParseMode 解析档位字符串。空值按 none 处理（最安全的一档）。
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none":
		return ModeNone, nil
	case "egress":
		return ModeEgress, nil
	}
	return "", fmt.Errorf("netx: unknown network mode %q (want none or egress)", s)
}

// Policy 是出网策略。
type Policy struct {
	// Mode 是档位。零值按 none 处理。
	Mode Mode
}
