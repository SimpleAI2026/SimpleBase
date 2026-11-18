// 文件说明：env_helpers_test.go — 仅测试使用的 env 读取与环境构造辅助（planv6.6 §1.3 T-1）
//
// 这些函数原本位于 env.go / config.go，但生产代码只走 setEnv* 与 Load，
// 保留在非测试文件里会把只给测试用的符号带进生产二进制。

package config

import (
	"os"
	"strconv"
	"time"
)

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return def
		}
		return b
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return def
		}
		return n
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return def
		}
		return n
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		d, err := parseDuration(v)
		if err != nil {
			return def
		}
		return d
	}
	return def
}

func envFloat64(key string, def float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return def
		}
		return n
	}
	return def
}

// loadLLMProviders 只读环境变量构造 provider 表。
func loadLLMProviders() map[string]ProviderConfig {
	cfg := defaults()
	applyEnvLLM(&cfg)
	if len(cfg.LLM.Providers) == 0 {
		return map[string]ProviderConfig{}
	}
	return cfg.LLM.Providers
}

// loadFromEnv 仅从默认值 + 环境变量构造配置（不读 YAML、不 Validate）。
func loadFromEnv() Config {
	cfg := defaults()
	applyEnv(&cfg)
	return cfg
}
