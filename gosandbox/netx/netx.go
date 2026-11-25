// 文件说明：netx.go — gosandbox/netx 中 Mode、ParseMode 等符号的实现
//
// 用法
//
//	type Mode string  // Mode 是出网档位
//	ParseMode(s string) (Mode, error)  // ParseMode 解析档位字符串
//	type Policy struct{…}  // Policy 是出网策略
//
// 调用架构
//
//	[上游] gosandbox/box, gosandbox/vm/box, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/netx/netx.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] netx_test.go, pasta_linux.go, pasta_other.go
//	[关键引用] Mode ← netx_test.go, pasta_linux.go；ParseMode ← netx_test.go
//
// 主要符号
//
//	type Mode string
//	const ModeNone / ModeEgress
//	var ErrUnsupported
//	ParseMode(s string) (Mode, error)
//	type Policy struct{…}

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
