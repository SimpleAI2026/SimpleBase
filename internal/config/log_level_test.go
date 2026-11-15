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
