// 文件说明：netx_test.go — gosandbox/netx 的单元测试（TestParseMode、TestParseModeRejectsUnknown 等）
//
// 用法
//
//	go test ./gosandbox/netx -run 'TestParseMode|TestParseModeRejectsUnknown|TestPolicyZeroValueIsNone|TestErrUnsupportedSentinel'
//
// 调用架构
//
//	[上游] gosandbox/box, gosandbox/vm/box, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/netx/netx_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] netx.go, pasta_linux.go, pasta_other.go
//
// 主要符号
//
//	TestParseMode(t *testing.T)
//	TestParseModeRejectsUnknown(t *testing.T)
//	TestPolicyZeroValueIsNone(t *testing.T)
//	TestErrUnsupportedSentinel(t *testing.T)

package netx

// netx_test.go：出网档位的解析。
//
// 白名单 DNS 代理与地址拦截（CheckAllowed）已随 GSB-10 整体移除，
// 对应的单测一并删除；egress 档目前不做任何地址或域名过滤。

import (
	"errors"
	"testing"
)

// ParseMode：空值取 none（最安全的一档）。
func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{
		"": ModeNone, "none": ModeNone, "NONE": ModeNone, " none ": ModeNone,
		"egress": ModeEgress, "EGRESS": ModeEgress,
	} {
		got, err := ParseMode(in)
		if err != nil {
			t.Errorf("ParseMode(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
}

// 未知档位报错并说明合法值；已移除的 egress-allowlist 也必须被拒绝，
// 不能悄悄降级成 egress（那会让用户以为白名单在生效）。
func TestParseModeRejectsUnknown(t *testing.T) {
	for _, in := range []string{"bridge", "egress-allowlist", "allowlist"} {
		if _, err := ParseMode(in); err == nil {
			t.Errorf("ParseMode(%q) must fail", in)
		}
	}
}

// Policy 零值就是 none。
func TestPolicyZeroValueIsNone(t *testing.T) {
	var p Policy
	if p.Mode != "" {
		t.Fatalf("zero policy mode = %q", p.Mode)
	}
	m, err := ParseMode(string(p.Mode))
	if err != nil || m != ModeNone {
		t.Fatalf("zero policy must parse as none: %q, %v", m, err)
	}
}

// ErrUnsupported 是可 errors.Is 判断的哨兵。
func TestErrUnsupportedSentinel(t *testing.T) {
	if !errors.Is(ErrUnsupported, ErrUnsupported) {
		t.Fatal("sentinel must match itself")
	}
}
