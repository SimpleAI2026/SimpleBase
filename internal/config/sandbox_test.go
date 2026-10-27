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
		Storage:  StorageConfig{Database: StorageModeLocal, Files: StorageModeLocal, DataDir: "/tmp/data"},
		Database: DatabaseConfig{Engine: EngineDuckDB, CacheDir: "/tmp/x", CacheMaxBytes: 1,
			CacheMaxDatabases: 1, IdleTimeout: time.Second, MaxOpen: 1},
		Auth: AuthConfig{APIKeyHashSecret: "s"},
		Limits: LimitsConfig{MaxRequestBytes: 1, MaxQueryRows: 1, QueryTimeout: time.Second,
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
	if cfg.Sandbox.Image != SandboxDefaultImage || cfg.Sandbox.CPUs != 1 || cfg.Sandbox.MemoryMiB != 256 {
		t.Fatalf("unexpected sandbox defaults: %+v", cfg.Sandbox)
	}
	if cfg.Sandbox.EffectiveBackend() != SandboxBackendLocal || cfg.Sandbox.MaxPerProject != 5 ||
		cfg.Sandbox.IdleTimeout != 5*time.Minute || cfg.Sandbox.ExecTimeoutMax != 300*time.Second {
		t.Fatalf("unexpected sandbox defaults: %+v", cfg.Sandbox)
	}
	if cfg.Sandbox.Network != "none" || cfg.Sandbox.Workdir != "/workspace" {
		t.Fatalf("unexpected sandbox defaults: %+v", cfg.Sandbox)
	}
}

func TestSandboxValidateLocalNeedsNoKey(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = validEnabledSandbox()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("local backend must validate without any key: %v", err)
	}
}

func TestSandboxValidateDisabledSkipsChecks(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = SandboxConfig{Enabled: false, Network: "bogus"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled sandbox must not fail: %v", err)
	}
}

func TestSandboxValidateLocalPaths(t *testing.T) {
	for name, mut := range map[string]func(*SandboxConfig){
		"sandbox.root":        func(s *SandboxConfig) { s.Root = "relative" },
		"sandbox.bwrap_path":  func(s *SandboxConfig) { s.BwrapPath = "bwrap" },
		"sandbox.helper_path": func(s *SandboxConfig) { s.HelperPath = "./gosandbox-inner" },
		"sandbox.pids_max":    func(s *SandboxConfig) { s.PidsMax = -1 },
	} {
		cfg := validTestConfig(t)
		cfg.Sandbox = validEnabledSandbox()
		mut(&cfg.Sandbox)
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), name) {
			t.Fatalf("%s: want validation error, got %v", name, err)
		}
	}
}

func TestSandboxValidateNetwork(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = SandboxConfig{Enabled: true, Network: "allowall",
		CPUs: 1, MemoryMiB: 512, MaxDuration: time.Minute, IdleTimeout: time.Minute,
		ExecTimeout: time.Second, MaxOutputBytes: 1, MaxFileBytes: 1}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sandbox.network") {
		t.Fatalf("bad network must fail, got %v", err)
	}
}

func TestSandboxValidateRejectsPublicNetwork(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = validEnabledSandbox()
	cfg.Sandbox.Network = "public"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "sandbox.network") {
		t.Fatalf("local backend has no network, public must fail, got %v", err)
	}
}

func TestSandboxValidateResourceBounds(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = SandboxConfig{Enabled: true, Network: "none",
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
	t.Setenv("SIMPLEBASE_SANDBOX_IMAGE", "custom")
	t.Setenv("SIMPLEBASE_SANDBOX_ROOT", "/var/lib/sb")
	t.Setenv("SIMPLEBASE_SANDBOX_EXEC_TIMEOUT", "45s")
	cfg := loadFromEnv()
	if !cfg.Sandbox.Enabled || cfg.Sandbox.Image != "custom" || cfg.Sandbox.Root != "/var/lib/sb" {
		t.Fatalf("env not applied: %+v", cfg.Sandbox)
	}
	if cfg.Sandbox.ExecTimeout != 45*time.Second {
		t.Fatalf("exec timeout = %v", cfg.Sandbox.ExecTimeout)
	}
}

func TestSandboxYAMLFieldsApplied(t *testing.T) {
	yaml := `
instance:
  id: inst-1
database:
  cache_dir: /tmp/x
sandbox:
  enabled: true
  image: "custom"
  cpus: 2
  memory_mib: 1024
  root: /var/lib/sb
  pids_max: 128
`
	path := writeTempConfig(t, yaml)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	t.Setenv("SIMPLEBASE_INSTANCE_WRITABLE", "false")
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "env-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Sandbox.Enabled || cfg.Sandbox.Image != "custom" || cfg.Sandbox.CPUs != 2 || cfg.Sandbox.MemoryMiB != 1024 ||
		cfg.Sandbox.Root != "/var/lib/sb" || cfg.Sandbox.PidsMax != 128 {
		t.Fatalf("yaml fields not applied: %+v", cfg.Sandbox)
	}
}

func TestSandboxRedactedView(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = validEnabledSandbox()
	cfg.Sandbox.Root = "/var/lib/sb"
	sb := cfg.Redacted()["sandbox"].(map[string]any)
	if sb["backend"] != SandboxBackendLocal || sb["root"] != "/var/lib/sb" {
		t.Fatalf("redacted = %+v", sb)
	}
	if _, ok := sb["has_api_key"]; ok {
		t.Fatal("api key no longer exists")
	}
}

func marshalTestJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func validEnabledSandbox() SandboxConfig {
	return SandboxConfig{Enabled: true, Network: "none", Image: "python:3.12-slim",
		CPUs: 1, MemoryMiB: 256, MaxDuration: time.Minute, IdleTimeout: time.Minute,
		ExecTimeout: time.Second, ExecTimeoutMax: time.Minute, MaxOutputBytes: 1, MaxFileBytes: 1,
		MaxPerProject: 5}
}

func TestSandboxValidateFakeBackend(t *testing.T) {
	cfg := validTestConfig(t)
	cfg.Sandbox = validEnabledSandbox()
	cfg.Sandbox.Backend = "fake"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ALLOW_FAKE") {
		t.Fatalf("fake backend without opt-in must fail, got %v", err)
	}
	t.Setenv("SIMPLEBASE_SANDBOX_ALLOW_FAKE", "1")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("fake backend with SIMPLEBASE_SANDBOX_ALLOW_FAKE=1 must pass: %v", err)
	}
	cfg.Sandbox.Backend = "cloud"
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

func TestSandboxTerminalSSHDefaultsAndValidation(t *testing.T) {
	cfg := defaults()
	if !cfg.Sandbox.Terminal.Enabled || cfg.Sandbox.Terminal.EffectiveSessionsPerSandbox() != 4 ||
		cfg.Sandbox.Terminal.EffectiveIdleTimeout() != 15*time.Minute {
		t.Fatalf("terminal defaults: %+v", cfg.Sandbox.Terminal)
	}
	if cfg.Sandbox.SSH.Listen != "" {
		t.Fatal("external SSH must be closed by default")
	}
	if cfg.Sandbox.E2B.DefaultTemplate != "base" {
		t.Fatalf("e2b default template = %q", cfg.Sandbox.E2B.DefaultTemplate)
	}
	// 零值取默认，不应被判为非法。
	if (SandboxTerminalConfig{}).EffectiveSessionsPerSandbox() != 4 || (SandboxTerminalConfig{}).EffectiveIdleTimeout() != 15*time.Minute {
		t.Fatal("zero value must fall back to defaults")
	}
	bad := map[string]func(*SandboxConfig){
		"sessions too many": func(s *SandboxConfig) { s.Terminal.MaxSessionsPerSandbox = 33 },
		"sessions negative": func(s *SandboxConfig) { s.Terminal.MaxSessionsPerSandbox = -1 },
		"idle too short":    func(s *SandboxConfig) { s.Terminal.IdleTimeout = 10 * time.Second },
		"listen no port":    func(s *SandboxConfig) { s.SSH.Listen = "127.0.0.1" },
		"host key relative": func(s *SandboxConfig) { s.SSH.HostKeyPath = "key" },
	}
	for name, mut := range bad {
		c := validTestConfig(t)
		c.Sandbox = validEnabledSandbox()
		mut(&c.Sandbox)
		if err := c.Validate(); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
	good := validTestConfig(t)
	good.Sandbox = validEnabledSandbox()
	good.Sandbox.SSH.Listen = "127.0.0.1:2222"
	good.Sandbox.Terminal.MaxSessionsPerSandbox = 8
	if err := good.Validate(); err != nil {
		t.Fatalf("valid ssh/terminal config rejected: %v", err)
	}
}

func TestSandboxTerminalSSHYAMLAndEnv(t *testing.T) {
	path := writeTempConfig(t, `
instance:
  id: inst-1
database:
  cache_dir: /tmp/x
sandbox:
  enabled: true
  terminal:
    enabled: false
    max_sessions_per_sandbox: 2
    idle_timeout: 3m
  ssh:
    listen: "127.0.0.1:2200"
    host_key_path: /var/lib/sb/host_key
  e2b:
    default_template: py
    envd_version: 0.9.9
`)
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	t.Setenv("SIMPLEBASE_INSTANCE_WRITABLE", "false")
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "env-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	sb := cfg.Sandbox
	if sb.Terminal.Enabled || sb.Terminal.MaxSessionsPerSandbox != 2 || sb.Terminal.IdleTimeout != 3*time.Minute ||
		sb.SSH.Listen != "127.0.0.1:2200" || sb.SSH.HostKeyPath != "/var/lib/sb/host_key" ||
		sb.E2B.DefaultTemplate != "py" || sb.E2B.EnvdVersion != "0.9.9" {
		t.Fatalf("yaml not applied: %+v", sb)
	}
	t.Setenv("SIMPLEBASE_SANDBOX_TERMINAL_ENABLED", "true")
	t.Setenv("SIMPLEBASE_SANDBOX_SSH_LISTEN", "")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Sandbox.Terminal.Enabled || cfg.Sandbox.SSH.Listen != "" {
		t.Fatalf("env override not applied: %+v", cfg.Sandbox)
	}
}
