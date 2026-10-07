package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// validSandboxTestConfig 构造除 sandbox 段外全部合法的配置。
func validTestConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		HTTP: HTTPConfig{Address: ":8080", ReadTimeout: time.Second, WriteTimeout: time.Second,
			IdleTimeout: time.Second, ShutdownTimeout: time.Second},
		Instance: InstanceConfig{ID: "i1", Writable: false},
		Database: DatabaseConfig{Engine: EngineDuckLake, CacheDir: "/tmp/x", CacheMaxBytes: 1,
			CacheMaxDatabases: 1, IdleTimeout: time.Second, MaxOpen: 1},
		Auth:    AuthConfig{APIKeyHashSecret: "s"},
		Limits:  LimitsConfig{MaxRequestBytes: 1, MaxQueryRows: 1, QueryTimeout: time.Second,
			MaxConcurrentQueries: 1, MaxBatchStatements: 1, MaxSQLBytes: 1},
		Observability:  ObservabilityConfig{LogFormat: "json", LogOutput: "stderr"},
		SystemDatabase: SystemDatabaseConfig{LogKeepDays: 1},
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSandboxDefaults(t *testing.T) {
	cfg := defaults()
	if cfg.Sandbox.Enabled {
		t.Fatal("sandbox must default to disabled")
	}
	if cfg.Sandbox.Image != "python:3.12-slim" || cfg.Sandbox.CPUs != 1 || cfg.Sandbox.MemoryMiB != 256 {
		t.Fatalf("unexpected sandbox defaults: %+v", cfg.Sandbox)
	}
	if cfg.Sandbox.EffectiveBackend() != SandboxBackendCloud || cfg.Sandbox.MaxPerProject != 5 ||
		cfg.Sandbox.IdleTimeout != 5*time.Minute || cfg.Sandbox.ExecTimeoutMax != 300*time.Second {
		t.Fatalf("unexpected sandbox defaults: %+v", cfg.Sandbox)
	}
	if cfg.Sandbox.Network != "none" || cfg.Sandbox.Workdir != "/workspace" {
		t.Fatalf("unexpected sandbox defaults: %+v", cfg.Sandbox)
	}
}

func TestSandboxValidateEnabledWithoutKey(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox.Enabled = true
	cfg.Sandbox.APIKey = ""
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "sandbox.api_key") {
		t.Fatalf("enabled without key must fail, got %v", err)
	}
}

func TestSandboxValidateDisabledIgnoresKey(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox.Enabled = false
	cfg.Sandbox.APIKey = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled sandbox must not fail: %v", err)
	}
}

func TestSandboxValidateNetwork(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = SandboxConfig{Enabled: true, APIKey: "k", Network: "allowall",
		CPUs: 1, MemoryMiB: 512, MaxDuration: time.Minute, IdleTimeout: time.Minute,
		ExecTimeout: time.Second, MaxOutputBytes: 1, MaxFileBytes: 1}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sandbox.network") {
		t.Fatalf("bad network must fail, got %v", err)
	}
}

func TestSandboxValidateResourceBounds(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = SandboxConfig{Enabled: true, APIKey: "k", Network: "none",
		CPUs: 8, MemoryMiB: 99999, MaxDuration: time.Minute, IdleTimeout: time.Minute,
		ExecTimeout: time.Second, MaxOutputBytes: 1, MaxFileBytes: 1}
	if err := cfg.Validate(); err == nil {
		t.Fatal("cpus/memory out of range must fail")
	}
	cfg.Sandbox.CPUs, cfg.Sandbox.MemoryMiB = 1, 512
	cfg.Sandbox.ExecTimeout = 500 * time.Millisecond
	if err := cfg.Validate(); err == nil {
		t.Fatal("exec_timeout below 1s must fail")
	}
}

func TestSandboxEnvOverrides(t *testing.T) {
	t.Setenv("SIMPLEBASE_SANDBOX_ENABLED", "true")
	t.Setenv("SIMPLEBASE_SANDBOX_API_KEY", "sk-test")
	t.Setenv("SIMPLEBASE_SANDBOX_IMAGE", "node:20")
	t.Setenv("SIMPLEBASE_SANDBOX_NETWORK", "public")
	t.Setenv("SIMPLEBASE_SANDBOX_EXEC_TIMEOUT", "45s")
	cfg := loadFromEnv()
	if !cfg.Sandbox.Enabled || cfg.Sandbox.APIKey != "sk-test" {
		t.Fatalf("env not applied: %+v", cfg.Sandbox)
	}
	if cfg.Sandbox.Image != "node:20" || cfg.Sandbox.Network != "public" {
		t.Fatalf("env not applied: %+v", cfg.Sandbox)
	}
	if cfg.Sandbox.ExecTimeout != 45*time.Second {
		t.Fatalf("exec timeout = %v", cfg.Sandbox.ExecTimeout)
	}
}

func TestSandboxYAMLKeyRejectedInProduction(t *testing.T) {
	yaml := `
instance:
  id: inst-1
  writable: false
database:
  cache_dir: /tmp/x
sandbox:
  enabled: true
  api_key: yaml-secret-key
`
	path := writeTempConfig(t, yaml)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "env-secret")
	os.Unsetenv("SIMPLEBASE_SANDBOX_API_KEY")
	defer os.Unsetenv("SIMPLEBASE_CONFIG_PATH")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "sandbox.api_key") {
		t.Fatalf("production YAML secret must be rejected, got %v", err)
	}
}

func TestSandboxYAMLNonSecretFieldsApplied(t *testing.T) {
	yaml := `
instance:
  id: inst-1
database:
  cache_dir: /tmp/x
sandbox:
  enabled: true
  image: "node:20"
  network: public
  cpus: 2
  memory_mib: 1024
`
	path := writeTempConfig(t, yaml)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	t.Setenv("SIMPLEBASE_INSTANCE_WRITABLE", "false")
	t.Setenv("SIMPLEBASE_SANDBOX_API_KEY", "env-key")
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "env-secret")
	defer os.Unsetenv("SIMPLEBASE_CONFIG_PATH")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Sandbox.Enabled || cfg.Sandbox.APIKey != "env-key" {
		t.Fatalf("env key must win: %+v", cfg.Sandbox)
	}
	if cfg.Sandbox.Image != "node:20" || cfg.Sandbox.Network != "public" || cfg.Sandbox.CPUs != 2 || cfg.Sandbox.MemoryMiB != 1024 {
		t.Fatalf("yaml fields not applied: %+v", cfg.Sandbox)
	}
}

func TestSandboxRedactedHasNoKey(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = SandboxConfig{Enabled: true, APIKey: "super-secret", Network: "public",
		CPUs: 1, MemoryMiB: 512, MaxDuration: time.Minute, IdleTimeout: time.Minute,
		ExecTimeout: time.Second, MaxOutputBytes: 1, MaxFileBytes: 1, Image: "python:3.12"}
	r := cfg.Redacted()
	sb := r["sandbox"].(map[string]any)
	if sb["has_api_key"] != true {
		t.Fatal("has_api_key must be true")
	}
	s, _ := marshalTestJSON(sb)
	if strings.Contains(s, "super-secret") {
		t.Fatalf("redacted view leaked api key: %s", s)
	}
}

func marshalTestJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func validEnabledSandbox() SandboxConfig {
	return SandboxConfig{Enabled: true, APIKey: "k", Network: "none", Image: "python:3.12-slim",
		CPUs: 1, MemoryMiB: 256, MaxDuration: time.Minute, IdleTimeout: time.Minute,
		ExecTimeout: time.Second, ExecTimeoutMax: time.Minute, MaxOutputBytes: 1, MaxFileBytes: 1,
		MaxPerProject: 5}
}

func TestSandboxValidateFakeBackend(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = validEnabledSandbox()
	cfg.Sandbox.Backend = "fake"
	cfg.Sandbox.APIKey = ""
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "dev_mode") {
		t.Fatalf("fake backend in production must fail, got %v", err)
	}
	cfg.DevMode = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("fake backend in dev_mode must pass without key: %v", err)
	}
	cfg.Sandbox.Backend = "local"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "sandbox.backend") {
		t.Fatalf("unknown backend must fail, got %v", err)
	}
}

func TestSandboxValidateImagesAndLimits(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = validEnabledSandbox()
	cfg.Sandbox.Images = []string{"node:22-alpine"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "sandbox.images") {
		t.Fatalf("images without default image must fail, got %v", err)
	}
	cfg.Sandbox.Images = []string{"python:3.12-slim", "node:22-alpine"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Sandbox.ExecTimeout = 2 * time.Minute
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "exec_timeout_max") {
		t.Fatalf("exec_timeout > max must fail, got %v", err)
	}
	cfg.Sandbox.ExecTimeout = time.Second
	cfg.Sandbox.MaxPerProject = 101
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "max_per_project") {
		t.Fatalf("max_per_project out of range must fail, got %v", err)
	}
}

func TestSandboxEnvV4Fields(t *testing.T) {
	t.Setenv("SIMPLEBASE_SANDBOX_BACKEND", "fake")
	t.Setenv("SIMPLEBASE_SANDBOX_IMAGES", "python:3.12-slim, alpine:3.20 ,")
	t.Setenv("SIMPLEBASE_SANDBOX_MAX_PER_PROJECT", "9")
	cfg := loadFromEnv()
	if cfg.Sandbox.Backend != "fake" || cfg.Sandbox.MaxPerProject != 9 {
		t.Fatalf("env not applied: %+v", cfg.Sandbox)
	}
	if len(cfg.Sandbox.Images) != 2 || cfg.Sandbox.Images[1] != "alpine:3.20" {
		t.Fatalf("images = %#v", cfg.Sandbox.Images)
	}
}
