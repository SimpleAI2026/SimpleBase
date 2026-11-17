package config

// sandbox_network_test.go：M4 新增的两项配置校验。
//
//  1. network 只接受 none / egress / egress-allowlist；
//  2. egress-allowlist 必须有白名单（空白名单等于全拒，配了也跑不通）；
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

// network 档位：合法的三种通过，其他报错。
func TestSandboxNetworkModes(t *testing.T) {
	for _, mode := range []string{"", "none", "egress"} {
		cfg := baseSandbox()
		cfg.Network = mode
		if err := validateSandbox(cfg); err != nil {
			t.Errorf("network=%q should be valid: %v", mode, err)
		}
	}
	// allowlist 没配白名单：报错。
	cfg := baseSandbox()
	cfg.Network = "egress-allowlist"
	if err := validateSandbox(cfg); err == nil {
		t.Fatal("egress-allowlist without network_allow must fail")
	}
	// 配了白名单但没接受实验室状态：仍然报错，且说清为什么。
	cfg.NetworkAllow = []string{"pypi.org"}
	err := validateSandbox(cfg)
	if err == nil {
		t.Fatal("experimental mode must be explicitly accepted")
	}
	if !strings.Contains(err.Error(), "allow_experimental_network") {
		t.Errorf("error should tell how to accept it: %v", err)
	}
	// 显式接受后通过。
	cfg.AllowExperimentalNetwork = true
	if err := validateSandbox(cfg); err != nil {
		t.Fatalf("should be valid: %v", err)
	}
	// 未知档位：报错并说明合法值。
	cfg = baseSandbox()
	cfg.Network = "bridge"
	err = validateSandbox(cfg)
	if err == nil {
		t.Fatal("unknown mode must fail")
	}
	if !strings.Contains(err.Error(), "egress-allowlist") {
		t.Errorf("error should list valid modes: %v", err)
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
