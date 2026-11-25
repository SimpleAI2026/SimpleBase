// 文件说明：log_level_test.go — internal/config 的单元测试（TestNormalizeLogLevel、TestValidLogLevel 等）
//
// 用法
//
//	go test ./internal/config -run 'TestNormalizeLogLevel|TestValidLogLevel|TestLoadNormalizesEmptyLogLevel|TestProfileName'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench, internal/app 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/log_level_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_config_test.go, backup.go, config.go, config_agent_types.go, config_example_test.go 等 24 个
//
// 主要符号
//
//	TestNormalizeLogLevel(t *testing.T)
//	TestValidLogLevel(t *testing.T)
//	TestLoadNormalizesEmptyLogLevel(t *testing.T)
//	TestProfileName(t *testing.T)

package config

import "testing"

// TestNormalizeLogLevel 锁定默认 debug 与归一化语义：
// 空值回落默认级别、大小写/空白不敏感、未知值原样保留（由启动日志提示）。
func TestNormalizeLogLevel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", DefaultLogLevel},
		{"   ", DefaultLogLevel},
		{"DEBUG", LogLevelDebug},
		{" Info ", LogLevelInfo},
		{"warn", LogLevelWarn},
		{"error", LogLevelError},
		{"verbose", "verbose"},
	}
	for _, tc := range cases {
		if got := normalizeLogLevel(tc.in); got != tc.want {
			t.Errorf("normalizeLogLevel(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if DefaultLogLevel != LogLevelDebug {
		t.Fatalf("default log level must be debug, got %q", DefaultLogLevel)
	}
}

func TestValidLogLevel(t *testing.T) {
	for _, ok := range []string{"debug", "INFO", "Warn", "error", " error "} {
		if !ValidLogLevel(ok) {
			t.Errorf("ValidLogLevel(%q) should be true", ok)
		}
	}
	for _, bad := range []string{"", "verbose", "trace", "fatal"} {
		if ValidLogLevel(bad) {
			t.Errorf("ValidLogLevel(%q) should be false", bad)
		}
	}
}

// TestLoadNormalizesEmptyLogLevel：YAML 显式写空串后必须回落到 debug，
// 而不是让 zap 静默退化成 info。
func TestLoadNormalizesEmptyLogLevel(t *testing.T) {
	empty := ""
	cfg := loadFromEnv()
	cfg.Observability.LogLevel = empty
	var yc yamlConfig
	yc.Observability.LogLevel = &empty
	if _, err := applyYAML(&cfg, yc); err != nil {
		t.Fatal(err)
	}
	if cfg.Observability.LogLevel != empty {
		t.Fatalf("precondition: yaml empty must zero the value, got %q", cfg.Observability.LogLevel)
	}
	if got := normalizeLogLevel(cfg.Observability.LogLevel); got != DefaultLogLevel {
		t.Fatalf("empty log level must fall back to %q, got %q", DefaultLogLevel, got)
	}
}

func TestProfileName(t *testing.T) {
	cfg := Config{}
	if got := cfg.ProfileName(); got != ProfileDevelopment {
		t.Fatalf("empty profile should be development, got %q", got)
	}
	cfg.Instance.Profile = " Production "
	if got := cfg.ProfileName(); got != ProfileProduction {
		t.Fatalf("profile should normalize to production, got %q", got)
	}
}
