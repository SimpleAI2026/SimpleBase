// 文件说明：sandbox_network_test.go — internal/config 的单元测试（TestSandboxNetworkModes、TestSandboxNetworkAllowlistRemoved 等）
//
// 用法
//
//	go test ./internal/config -run 'TestSandboxNetworkModes|TestSandboxNetworkAllowlistRemoved|TestSandboxDiskLimitNegative'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench, internal/app 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/sandbox_network_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_config_test.go, backup.go, config.go, config_agent_types.go, config_example_test.go 等 24 个
//
// 主要符号
//
//	baseSandbox() SandboxConfig
//	testDuration(seconds int) time.Duration
//	TestSandboxNetworkModes(t *testing.T)
//	TestSandboxNetworkAllowlistRemoved(t *testing.T)
//	TestSandboxDiskLimitNegative(t *testing.T)

package config

// sandbox_network_test.go：沙盒网络与磁盘配置校验。
//
//  1. network 只接受 none / egress；
//  2. egress-allowlist（域名白名单）已移除，必须报错而不是悄悄降级成 egress；
//  3. disk_limit_mib 不能为负。

import (
	"strings"
	"testing"
	"time"
)

// baseSandbox 是一个能通过校验的最小配置。
func baseSandbox() SandboxConfig {
	return SandboxConfig{
		Enabled: true, Backend: SandboxBackendBox,
		Image: "host", CPUs: 1, MemoryMiB: 256,
		MaxDuration: testDuration(1800), IdleTimeout: testDuration(300),
		MaxOutputBytes: 1 << 20, MaxFileBytes: 1 << 20,
		ExecTimeout: testDuration(120), ExecTimeoutMax: testDuration(600),
		ReapInterval: testDuration(60),
	}
}

// testDuration 把秒数转成 Duration。
func testDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}

// network 档位：none / egress 通过，其他报错。
func TestSandboxNetworkModes(t *testing.T) {
	for _, mode := range []string{"", "none", "egress", " EGRESS "} {
		cfg := baseSandbox()
		cfg.Network = mode
		if err := validateSandbox(cfg); err != nil {
			t.Errorf("network=%q should be valid: %v", mode, err)
		}
	}
	// 未知档位：报错并说明合法值。
	cfg := baseSandbox()
	cfg.Network = "bridge"
	err := validateSandbox(cfg)
	if err == nil {
		t.Fatal("unknown mode must fail")
	}
	if !strings.Contains(err.Error(), "none or egress") {
		t.Errorf("error should list valid modes: %v", err)
	}
}

// 已移除的白名单档：必须报错并说明原因，不能降级成 egress。
func TestSandboxNetworkAllowlistRemoved(t *testing.T) {
	for _, mode := range []string{"egress-allowlist", "allowlist"} {
		cfg := baseSandbox()
		cfg.Network = mode
		err := validateSandbox(cfg)
		if err == nil {
			t.Fatalf("network=%q must fail: the allowlist was removed", mode)
		}
		if !strings.Contains(err.Error(), "removed") {
			t.Errorf("error should explain the removal: %v", err)
		}
	}
}

// 磁盘限额不能为负。
func TestSandboxDiskLimitNegative(t *testing.T) {
	cfg := baseSandbox()
	cfg.DiskLimitMiB = -1
	if err := validateSandbox(cfg); err == nil {
		t.Fatal("negative disk_limit_mib must fail")
	}
	cfg.DiskLimitMiB = 512
	if err := validateSandbox(cfg); err != nil {
		t.Fatalf("positive limit should be valid: %v", err)
	}
}
