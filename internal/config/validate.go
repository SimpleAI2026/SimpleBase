// 文件说明：validate.go — 配置整体校验：Validate 与存储 / 沙盒 / Agent / LLM / Shell 隔离的子校验
//
// 用法
//
//	Validate（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api, internal/api/perfbench 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/config/validate.go
//	         │
//	         ▼
//	[下游] internal/utils/strutil
//	[同包] backup.go, env.go, http_boundary.go, yaml.go
//
// 主要符号
//
//	func validateLLMAgent(l LLMConfig) []error
//	func validateShellIsolation(c ShellIsolationConfig) []error
//	func validateAgent(a AgentConfig) []error
//	func (c Config) Validate() error
//	func validateStorage(s StorageConfig, cacheDir string) error
//	func validateDirDisjoint(dataDir, cacheDir string) error
//	func isWithin(inner, outer string) bool
//	func validateSandbox(s SandboxConfig) error
//	... 另有 2 个顶层符号

package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

func validateLLMAgent(l LLMConfig) []error {
	var errs []error
	switch strutil.Normalize(l.AgentToolProtocol) {
	case "", AgentToolProtocolAuto, AgentToolProtocolNative, AgentToolProtocolText:
	default:
		errs = append(errs, fmt.Errorf("llm.agent_tool_protocol must be auto, native or text (got %q)", l.AgentToolProtocol))
	}
	if l.AgentMaxIterations < 0 || l.AgentMaxIterations > 20 {
		errs = append(errs, fmt.Errorf("llm.agent_max_iterations must be 1-20 (got %d)", l.AgentMaxIterations))
	}
	if l.AgentRunTimeout != 0 && (l.AgentRunTimeout < 10*time.Second || l.AgentRunTimeout > 15*time.Minute) {
		errs = append(errs, fmt.Errorf("llm.agent_run_timeout must be 10s-15m (got %s)", l.AgentRunTimeout))
	}
	return errs
}

func validateShellIsolation(c ShellIsolationConfig) []error {
	var errs []error
	switch c.EffectiveMode() {
	case "read-only", "workspace-write", "danger-full-access":
	default:
		errs = append(errs, fmt.Errorf("agent.shell_isolation.mode must be read-only, workspace-write or danger-full-access (got %q)", c.Mode))
	}
	if c.MemoryMaxBytes < 0 || c.PidsMax < 0 || c.Timeout < 0 {
		errs = append(errs, errors.New("agent.shell_isolation.memory_max_bytes, pids_max and timeout must not be negative"))
	}
	if c.CPUMax != "" {
		f := strings.Fields(c.CPUMax)
		ok := len(f) >= 1 && len(f) <= 2
		for i, tok := range f {
			if tok == "max" && i == 0 {
				continue
			}
			if tok == "" || strings.Trim(tok, "0123456789") != "" {
				ok = false
			}
		}
		if !ok {
			errs = append(errs, fmt.Errorf("agent.shell_isolation.cpu_max must be \"<quota|max> [period]\" (got %q)", c.CPUMax))
		}
	}
	for _, group := range []struct {
		name  string
		paths []string
	}{{"writable_roots", c.WritableRoots}, {"deny_read", c.DenyRead}} {
		for _, p := range group.paths {
			if !strings.HasPrefix(p, "/") && p != "~" && !strings.HasPrefix(p, "~/") {
				errs = append(errs, fmt.Errorf("agent.shell_isolation.%s entries must be absolute or start with ~ (got %q)", group.name, p))
			}
		}
	}
	for _, p := range []struct{ name, v string }{{"bwrap_path", c.BwrapPath}, {"helper_path", c.HelperPath}} {
		if p.v != "" && !strings.HasPrefix(p.v, "/") {
			errs = append(errs, fmt.Errorf("agent.shell_isolation.%s must be absolute (got %q)", p.name, p.v))
		}
	}
	return errs
}

func validateAgent(a AgentConfig) []error {
	var errs []error
	if a.DelegationTTL != 0 && (a.DelegationTTL < time.Minute || a.DelegationTTL > 30*time.Minute) {
		errs = append(errs, fmt.Errorf("agent.delegation_ttl must be 1m-30m (got %s)", a.DelegationTTL))
	}
	if a.ConfirmTimeout != 0 && (a.ConfirmTimeout < 10*time.Second || a.ConfirmTimeout > 10*time.Minute) {
		errs = append(errs, fmt.Errorf("agent.confirm_timeout must be 10s-10m (got %s)", a.ConfirmTimeout))
	}
	errs = append(errs, validateShellIsolation(a.ShellIsolation)...)
	return errs
}

// Validate 聚合校验字段。
// 只有 storage.database=s3 或 storage.files=s3 时才要求 S3 配置完整。
func (c Config) Validate() error {
	var errs []error

	if c.HTTP.Address == "" {
		errs = append(errs, errors.New("http.address is required"))
	}
	if c.HTTP.ReadTimeout <= 0 || c.HTTP.WriteTimeout <= 0 || c.HTTP.IdleTimeout <= 0 {
		errs = append(errs, errors.New("http timeouts must be positive"))
	}
	if c.HTTP.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("http.shutdown_timeout must be positive"))
	}
	if c.Instance.ID == "" {
		errs = append(errs, errors.New("instance.id is required"))
	}
	switch strutil.Normalize(c.Instance.Profile) {
	case "", ProfileDevelopment, ProfileProduction:
	default:
		errs = append(errs, fmt.Errorf("instance.profile must be development or production (got %q)", c.Instance.Profile))
	}
	switch strings.ToLower(c.Instance.Lease.OnLost) {
	case "", "release_db", "exit":
	default:
		errs = append(errs, fmt.Errorf("instance.lease.on_lost must be release_db or exit (got %q)", c.Instance.Lease.OnLost))
	}
	if c.Instance.Lease.TTL < 0 || c.Instance.Lease.Grace < 0 || c.Instance.Lease.RenewInterval < 0 {
		errs = append(errs, errors.New("instance.lease durations must be non-negative"))
	}
	switch strutil.Normalize(c.Database.Engine) {
	case "", EngineDuckDB, EngineSQLite:
	default:
		errs = append(errs, fmt.Errorf("database.engine must be duckdb or sqlite (got %q)", c.Database.Engine))
	}
	if c.Database.DuckLake.Threads < 0 {
		errs = append(errs, errors.New("database.ducklake.threads must be non-negative"))
	}
	switch strutil.Normalize(c.Database.DuckLake.CatalogEngine) {
	case "", CatalogEngineDuckDB, CatalogEngineSQLite:
	default:
		errs = append(errs, fmt.Errorf("database.ducklake.catalog_engine must be duckdb or sqlite (got %q)", c.Database.DuckLake.CatalogEngine))
	}
	if c.Database.CacheDir == "" {
		errs = append(errs, errors.New("database.cache_dir is required"))
	}
	if c.Database.CacheMaxBytes <= 0 {
		errs = append(errs, errors.New("database.cache_max_bytes must be positive"))
	}
	if c.Database.CacheMaxDatabases <= 0 {
		errs = append(errs, errors.New("database.cache_max_databases must be positive"))
	}
	if c.Database.IdleTimeout <= 0 {
		errs = append(errs, errors.New("database.idle_timeout must be positive"))
	}
	if c.Database.MaxOpen <= 0 {
		errs = append(errs, errors.New("database.max_open must be positive"))
	}
	if c.Auth.APIKeyHashSecret == "" {
		errs = append(errs, errors.New("auth.api_key_hash_secret is required"))
	}
	if c.Limits.MaxRequestBytes <= 0 || c.Limits.MaxQueryRows <= 0 ||
		c.Limits.QueryTimeout <= 0 || c.Limits.MaxConcurrentQueries <= 0 ||
		c.Limits.MaxBatchStatements <= 0 || c.Limits.MaxSQLBytes <= 0 {
		errs = append(errs, errors.New("limits fields must be positive"))
	}
	// GF-RUN-04：负数拒绝；0 由 Load 的 normalize 回落到默认值。
	if c.Limits.MaxConcurrentGoRuns < 0 {
		errs = append(errs, errors.New("limits.max_concurrent_go_runs must not be negative"))
	}
	switch strings.ToLower(c.Observability.LogFormat) {
	case "", "json", "console":
	default:
		errs = append(errs, fmt.Errorf("observability.log_format must be json or console (got %q)", c.Observability.LogFormat))
	}
	switch strings.ToLower(c.Observability.LogOutput) {
	case "", "stderr", "stdout":
	default:
		errs = append(errs, fmt.Errorf("observability.log_output must be stderr or stdout (got %q)", c.Observability.LogOutput))
	}
	if c.SystemDatabase.LogKeepDays <= 0 {
		errs = append(errs, errors.New("system_database.log_keep_days must be positive"))
	}
	if err := validateSandbox(c.Sandbox); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, validateLLMAgent(c.LLM)...)
	errs = append(errs, validateAgent(c.Agent)...)
	errs = append(errs, validateHTTPBoundary(c)...)
	if err := validateBackup(c.Backup); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, validateSites(c.Sites)...)

	if err := validateStorage(c.Storage, c.Database.CacheDir); err != nil {
		errs = append(errs, err)
	}

	// 只要有任一持久层走对象存储，就必须具备完整的 S3 连接信息。
	if c.IsDBRemote() || c.IsFilesRemote() {
		if c.S3.Bucket == "" {
			errs = append(errs, errors.New("s3.bucket is required when storage.database or storage.files is s3"))
		}
		if c.S3.Region == "" {
			errs = append(errs, errors.New("s3.region is required when storage.database or storage.files is s3"))
		}
		if c.S3.AccessKey != "" && c.S3.SecretKey == "" {
			errs = append(errs, errors.New("s3.secret_key is required when s3.access_key is set"))
		}
		if c.S3.SecretKey != "" && c.S3.AccessKey == "" {
			errs = append(errs, errors.New("s3.access_key is required when s3.secret_key is set"))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %w", joinErrors(errs))
	}
	return nil
}

// validateStorage 校验 storage 段：模式取值合法，且两个目录不重叠。
// cache_dir 会被缓存淘汰整体删除，因此绝不能落在 data_dir 之内（反之亦然）。
func validateStorage(s StorageConfig, cacheDir string) error {
	var errs []error
	switch strutil.Normalize(s.Database) {
	case "", StorageModeLocal, StorageModeS3:
	default:
		errs = append(errs, fmt.Errorf("storage.database must be local or s3 (got %q)", s.Database))
	}
	switch strutil.Normalize(s.Files) {
	case "", StorageModeLocal, StorageModeS3:
	default:
		errs = append(errs, fmt.Errorf("storage.files must be local or s3 (got %q)", s.Files))
	}
	if strings.TrimSpace(s.DataDir) == "" {
		errs = append(errs, errors.New("storage.data_dir is required"))
	}
	if err := validateDirDisjoint(s.DataDir, cacheDir); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("storage validation failed: %w", joinErrors(errs))
	}
	return nil
}

// validateDirDisjoint 拒绝两个目录相等或互为父子目录。
// 缓存目录在容量不足时会被整体删除（cache.Manager.Evict），落在数据目录里会丢数据。
func validateDirDisjoint(dataDir, cacheDir string) error {
	d := filepath.Clean(strings.TrimSpace(dataDir))
	c := filepath.Clean(strings.TrimSpace(cacheDir))
	if d == "" || c == "" || d == "." || c == "." {
		return nil
	}
	if d == c {
		return fmt.Errorf("storage.data_dir and database.cache_dir must differ (both %q)", d)
	}
	if isWithin(d, c) || isWithin(c, d) {
		return fmt.Errorf("storage.data_dir (%q) and database.cache_dir (%q) must not contain each other", d, c)
	}
	return nil
}

// isWithin 返回 inner 是否位于 outer 之内（含相等）。
// 用 filepath.Rel 判断，避免前缀字符串误判（如 /data2 与 /data）。
func isWithin(inner, outer string) bool {
	rel, err := filepath.Rel(outer, inner)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// validateSandbox 校验 sandbox 配置段。enabled=false 时忽略。
func validateSandbox(s SandboxConfig) error {
	if !s.Enabled {
		return nil
	}
	var errs []error
	switch s.EffectiveBackend() {
	case SandboxBackendLocal, SandboxBackendBox:
		if s.BwrapPath != "" && !filepath.IsAbs(s.BwrapPath) {
			errs = append(errs, errors.New("sandbox.bwrap_path must be an absolute path"))
		}
		if s.HelperPath != "" && !filepath.IsAbs(s.HelperPath) {
			errs = append(errs, errors.New("sandbox.helper_path must be an absolute path"))
		}
		if s.Root != "" && !filepath.IsAbs(s.Root) {
			errs = append(errs, errors.New("sandbox.root must be an absolute path"))
		}
	case SandboxBackendFake:
		if !allowFakeSandbox() {
			errs = append(errs, errors.New("sandbox.backend=fake is only allowed when SIMPLEBASE_SANDBOX_ALLOW_FAKE=1 (e2e / unit tests)"))
		}
	default:
		errs = append(errs, fmt.Errorf("sandbox.backend must be local or fake (got %q)", s.Backend))
	}
	if s.PidsMax < 0 {
		errs = append(errs, errors.New("sandbox.pids_max must not be negative"))
	}
	if len(s.Images) > 0 && strings.TrimSpace(s.Image) != "" {
		found := false
		for _, img := range s.Images {
			if strings.TrimSpace(img) == strings.TrimSpace(s.Image) {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Errorf("sandbox.images must contain sandbox.image (%q)", s.Image))
		}
	}
	if s.ExecTimeoutMax != 0 && s.ExecTimeoutMax < s.ExecTimeout {
		errs = append(errs, errors.New("sandbox.exec_timeout must not exceed sandbox.exec_timeout_max"))
	}
	if s.MaxPerProject < 0 || s.MaxPerProject > 100 {
		errs = append(errs, fmt.Errorf("sandbox.max_per_project must be 1-100 (got %d)", s.MaxPerProject))
	}
	if s.ReapInterval < 0 {
		errs = append(errs, errors.New("sandbox.reap_interval must not be negative"))
	}
	switch strutil.Normalize(s.Network) {
	case "", "none", "egress":
	case "egress-allowlist", "allowlist":
		// 给出明确的迁移说明：悄悄降级成 egress 会让人以为白名单还在生效。
		errs = append(errs, fmt.Errorf("sandbox.network=%q has been removed (the domain allowlist was never enforced); "+
			"use none or egress", s.Network))
	default:
		errs = append(errs, fmt.Errorf("sandbox.network must be none or egress (got %q)", s.Network))
	}
	if s.DiskLimitMiB < 0 {
		errs = append(errs, fmt.Errorf("sandbox.disk_limit_mib must not be negative (got %d)", s.DiskLimitMiB))
	}
	if s.CPUs < 1 || s.CPUs > 4 {
		errs = append(errs, fmt.Errorf("sandbox.cpus must be 1-4 (got %d)", s.CPUs))
	}
	if s.MemoryMiB < 128 || s.MemoryMiB > 4096 {
		errs = append(errs, fmt.Errorf("sandbox.memory_mib must be 128-4096 (got %d)", s.MemoryMiB))
	}
	if s.ExecTimeout < time.Second {
		errs = append(errs, errors.New("sandbox.exec_timeout must be at least 1s"))
	}
	if s.MaxDuration <= 0 || s.IdleTimeout <= 0 {
		errs = append(errs, errors.New("sandbox.max_duration and sandbox.idle_timeout must be positive"))
	}
	if s.MaxOutputBytes <= 0 || s.MaxFileBytes <= 0 {
		errs = append(errs, errors.New("sandbox.max_output_bytes and sandbox.max_file_bytes must be positive"))
	}
	// 终端子段零值 = 取默认（EffectiveTerminal），所以只校验显式设置过的值。
	if n := s.Terminal.MaxSessionsPerSandbox; n != 0 && (n < 1 || n > 32) {
		errs = append(errs, fmt.Errorf("sandbox.terminal.max_sessions_per_sandbox must be 1-32 (got %d)", n))
	}
	if d := s.Terminal.IdleTimeout; d != 0 && d < time.Minute {
		errs = append(errs, errors.New("sandbox.terminal.idle_timeout must be at least 1m"))
	}
	if l := strings.TrimSpace(s.SSH.Listen); l != "" {
		if _, port, err := net.SplitHostPort(l); err != nil || port == "" {
			errs = append(errs, fmt.Errorf("sandbox.ssh.listen must be host:port (got %q)", s.SSH.Listen))
		}
	}
	if s.SSH.HostKeyPath != "" && !filepath.IsAbs(s.SSH.HostKeyPath) {
		errs = append(errs, errors.New("sandbox.ssh.host_key_path must be an absolute path"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("sandbox validation failed: %w", joinErrors(errs))
	}
	return nil
}

// allowFakeSandbox 返回是否显式允许 fake 沙盒后端（e2e / 单测）。
func allowFakeSandbox() bool {
	return os.Getenv("SIMPLEBASE_SANDBOX_ALLOW_FAKE") == "1"
}

func joinErrors(errs []error) error {
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e.Error())
	}
	return errors.New(b.String())
}
