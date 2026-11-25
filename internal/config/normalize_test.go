// 文件说明：normalize_test.go — internal/config 的单元测试（TestNormalizeFillsGoRunsDefault、TestLoadZeroGoRunsFallsBack 等）
//
// 用法
//
//	go test ./internal/config -run 'TestNormalizeFillsGoRunsDefault|TestLoadZeroGoRunsFallsBack|TestValidateRejectsNegativeGoRuns'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench, internal/app 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/normalize_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_config_test.go, backup.go, config.go, config_agent_types.go, config_example_test.go 等 24 个
//
// 主要符号
//
//	TestNormalizeFillsGoRunsDefault(t *testing.T)
//	TestLoadZeroGoRunsFallsBack(t *testing.T)
//	TestValidateRejectsNegativeGoRuns(t *testing.T)

package config

import "testing"

// TestNormalizeFillsGoRunsDefault 锁定 Bug-3：显式写 0 的并发上限必须在 Load 阶段
// 回落到默认值，而不是靠值接收者的 Validate 赋值（对调用方无效）。
func TestNormalizeFillsGoRunsDefault(t *testing.T) {
	cfg := Config{}
	cfg.Limits.MaxConcurrentGoRuns = 0
	normalize(&cfg)
	if cfg.Limits.MaxConcurrentGoRuns != defaultMaxConcurrentGoRuns {
		t.Fatalf("zero must fall back to %d, got %d", defaultMaxConcurrentGoRuns, cfg.Limits.MaxConcurrentGoRuns)
	}
	cfg.Limits.MaxConcurrentGoRuns = 3
	normalize(&cfg)
	if cfg.Limits.MaxConcurrentGoRuns != 3 {
		t.Fatalf("explicit value must be kept, got %d", cfg.Limits.MaxConcurrentGoRuns)
	}
}

func TestLoadZeroGoRunsFallsBack(t *testing.T) {
	setRequiredEnvs(t, false, false)
	t.Setenv("SIMPLEBASE_LIMITS_MAX_CONCURRENT_GO_RUNS", "0")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.MaxConcurrentGoRuns != defaultMaxConcurrentGoRuns {
		t.Fatalf("Load must return default for 0, got %d", cfg.Limits.MaxConcurrentGoRuns)
	}
}

func TestValidateRejectsNegativeGoRuns(t *testing.T) {
	setRequiredEnvs(t, false, false)
	t.Setenv("SIMPLEBASE_LIMITS_MAX_CONCURRENT_GO_RUNS", "-1")
	if _, err := Load(); err == nil {
		t.Fatal("negative max_concurrent_go_runs must be rejected")
	}
}
