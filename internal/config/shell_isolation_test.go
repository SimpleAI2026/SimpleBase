// 文件说明：shell_isolation_test.go — internal/config 的单元测试（TestShellIsolationDefaults、TestShellIsolationEnvOverrides 等）
//
// 用法
//
//	go test ./internal/config -run 'TestShellIsolationDefaults|TestShellIsolationEnvOverrides|TestShellIsolationValidation'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench, internal/app 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/shell_isolation_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_config_test.go, backup.go, config.go, config_agent_types.go, config_example_test.go 等 24 个
//
// 主要符号
//
//	TestShellIsolationDefaults(t *testing.T)
//	TestShellIsolationEnvOverrides(t *testing.T)
//	TestShellIsolationValidation(t *testing.T)

package config

import (
	"strings"
	"testing"
	"time"
)

// 默认关闭；未指定档位取 workspace-write。
func TestShellIsolationDefaults(t *testing.T) {
	c := defaults()
	if c.Agent.ShellIsolation.Enabled {
		t.Fatal("shell_isolation must default to disabled")
	}
	if got := c.Agent.ShellIsolation.EffectiveMode(); got != "workspace-write" {
		t.Fatalf("mode = %q", got)
	}
	if errs := validateAgent(c.Agent); len(errs) != 0 {
		t.Fatalf("defaults must validate: %v", errs)
	}
}

func TestShellIsolationEnvOverrides(t *testing.T) {
	t.Setenv("SIMPLEBASE_AGENT_SHELL_ISOLATION_ENABLED", "true")
	t.Setenv("SIMPLEBASE_AGENT_SHELL_ISOLATION_MODE", "read-only")
	t.Setenv("SIMPLEBASE_AGENT_SHELL_ISOLATION_TIMEOUT", "45s")
	c := defaults()
	applyEnvAgent(&c)
	si := c.Agent.ShellIsolation
	if !si.Enabled || si.Mode != "read-only" || si.Timeout != 45*time.Second {
		t.Fatalf("shell isolation = %+v", si)
	}
}

func TestShellIsolationValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    ShellIsolationConfig
		want string
	}{
		{"mode", ShellIsolationConfig{Mode: "yolo"}, "mode"},
		{"negative", ShellIsolationConfig{PidsMax: -1}, "negative"},
		{"cpu", ShellIsolationConfig{CPUMax: "fast"}, "cpu_max"},
		{"cpu-too-many", ShellIsolationConfig{CPUMax: "1 2 3"}, "cpu_max"},
		{"writable-relative", ShellIsolationConfig{WritableRoots: []string{"rel"}}, "writable_roots"},
		{"deny-relative", ShellIsolationConfig{DenyRead: []string{".ssh"}}, "deny_read"},
		{"bwrap-relative", ShellIsolationConfig{BwrapPath: "bwrap"}, "bwrap_path"},
		{"helper-relative", ShellIsolationConfig{HelperPath: "inner"}, "helper_path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateShellIsolation(tc.c)
			if len(errs) == 0 || !strings.Contains(errs[0].Error(), tc.want) {
				t.Fatalf("errors = %v", errs)
			}
		})
	}
	ok := ShellIsolationConfig{Mode: "workspace-write", CPUMax: "max 100000", DenyRead: []string{"~/.ssh", "/etc/secret"},
		WritableRoots: []string{"/var/tmp"}, BwrapPath: "/usr/bin/bwrap"}
	if errs := validateShellIsolation(ok); len(errs) != 0 {
		t.Fatalf("valid config rejected: %v", errs)
	}
}
