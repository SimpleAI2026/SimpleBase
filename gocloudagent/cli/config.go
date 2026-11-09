// 文件说明：config.go — gocloudagent 的本地命令行外壳（planv6.0
//
// 用法
//
//	HomeDir(env []string) string  // HomeDir 返回 ~/.gocloudagent（尊重 GOCLOUDAGENT_HOME 覆盖，便于测试隔离）
//	LoadConfig(env []string) (Config, error)  // LoadConfig 读取 CLI 配置
//	同包调用：Config → command.go, repl.go 等；Validate() → command.go, repl.go 等；LoadConfig() → command.go, repl.go
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/config.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] client.go, command.go, compaction.go, prompt.go, repeat_detector.go 等 9 个
//	[关键引用] Config ← command.go, repl.go, session.go
//
// 主要符号
//
//	var ErrInsecureConfig
//	type Config struct {…}
//	HomeDir(env []string) string
//	LoadConfig(env []string) (Config, error)
//	SaveConfig(env []string, cfg Config) (string, error)
//	MergeEnv(cfg Config, env []string) Config
//	... 另有 4 个顶层符号

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrInsecureConfig 表示配置文件权限过宽（含 group/other 位）。
var ErrInsecureConfig = errors.New("config_insecure")

// Config 是 ~/.gocloudagent/config.json 的内容。
type Config struct {
	// Endpoint 是 SimpleBase 服务地址，例如 http://127.0.0.1:8080。
	Endpoint string `json:"endpoint"`
	// Token 是 SimpleBase 的 API Key。CLI 不保存任何供应商密钥（决策 D-D1）。
	Token string `json:"token"`
	// ProjectID 是默认项目。
	ProjectID string `json:"project_id"`
	// Model 是默认模型名；空表示用服务端 llm/settings 的默认模型。
	Model string `json:"model"`
	// Output 是默认输出格式："text" 或 "json"。
	Output string `json:"output"`
	// MaxIterations 是单次 run 的工具调用轮数上限；0 用默认值。
	MaxIterations int `json:"max_iterations"`
	// MaxRunSeconds 是单次 run 的墙钟上限（秒）；0 表示不限。
	// 防止卡在上游响应或反复试错时无限占用进程（planv6.1 O3-6）。
	MaxRunSeconds int `json:"max_run_seconds,omitempty"`
	// SearchProvider 是 web 搜索后端：duckduckgo（免 key）或 brave（需 key）。
	SearchProvider string `json:"search_provider"`
	// SearchAPIKey 是 brave 等需 key 后端的凭据。
	SearchAPIKey string `json:"search_api_key"`
	// Yolo 为真时跳过破坏性 shell 命令的确认（显式承担风险，§5.3）。
	Yolo bool `json:"yolo"`
	// CompactTriggerTokens 是触发上下文压缩的估算 token 阈值；0 用默认值（§13.5）。
	CompactTriggerTokens int `json:"compact_trigger_tokens"`
	// CompactKeepTokens 是压缩后保留的近期 token 预算；0 用默认值（§13.5）。
	CompactKeepTokens int `json:"compact_keep_tokens"`
	// ShellIsolation 是本机 shell 的隔离策略（planv6.2）。缺省 enabled=false，行为与旧版一致。
	// Yolo 只跳过确认，不会关闭隔离。
	ShellIsolation ShellIsolation `json:"shell_isolation,omitempty"`
}

// HomeDir 返回 ~/.gocloudagent（尊重 GOCLOUDAGENT_HOME 覆盖，便于测试隔离）。
func HomeDir(env []string) string {
	if v, ok := envGet(env, "GOCLOUDAGENT_HOME"); ok && strings.TrimSpace(v) != "" {
		return v
	}
	home, ok := envGet(env, "HOME")
	if !ok || home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".gocloudagent")
}

func configPath(env []string) string { return filepath.Join(HomeDir(env), "config.json") }

// LoadConfig 读取 CLI 配置。文件不存在返回零值（不报错）；权限过宽返回 ErrInsecureConfig。
func LoadConfig(env []string) (Config, error) {
	path := configPath(env)
	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	// 与 sbcli 同口径：任何 group/other 权限位都拒绝，避免同机其他用户读到 token。
	if perm := st.Mode().Perm(); perm&0o077 != 0 {
		return Config{}, fmt.Errorf("%w: %s has mode %04o, want 0600", ErrInsecureConfig, path, perm)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// SaveConfig 以 0600 写入配置（目录 0700）。
func SaveConfig(env []string, cfg Config) (string, error) {
	dir := HomeDir(env)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := configPath(env)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return "", err
	}
	// WriteFile 不改既有文件权限：显式收敛，避免旧文件带着 0644 继续用。
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// overlayConfig 用调用方显式提供的字段覆盖文件配置：零值视为「没传」。
// 调用方（RunCommand / cmdRun / cmdResume）构造的 Options.Config 只带命令行给出的值，
// 未给出的字段是零值，不能把文件里已配好的 endpoint/token 抹掉。
func overlayConfig(base, override Config) Config {
	if strings.TrimSpace(override.Endpoint) != "" {
		base.Endpoint = override.Endpoint
	}
	if strings.TrimSpace(override.Token) != "" {
		base.Token = override.Token
	}
	if strings.TrimSpace(override.ProjectID) != "" {
		base.ProjectID = override.ProjectID
	}
	if strings.TrimSpace(override.Model) != "" {
		base.Model = override.Model
	}
	if strings.TrimSpace(override.Output) != "" {
		base.Output = override.Output
	}
	if strings.TrimSpace(override.SearchProvider) != "" {
		base.SearchProvider = override.SearchProvider
	}
	if strings.TrimSpace(override.SearchAPIKey) != "" {
		base.SearchAPIKey = override.SearchAPIKey
	}
	if override.MaxIterations > 0 {
		base.MaxIterations = override.MaxIterations
	}
	if override.MaxRunSeconds > 0 {
		base.MaxRunSeconds = override.MaxRunSeconds
	}
	if override.CompactTriggerTokens > 0 {
		base.CompactTriggerTokens = override.CompactTriggerTokens
	}
	if override.CompactKeepTokens > 0 {
		base.CompactKeepTokens = override.CompactKeepTokens
	}
	if override.Yolo {
		base.Yolo = true
	}
	base.ShellIsolation = overlayShellIsolation(base.ShellIsolation, override.ShellIsolation)
	return base
}

// MergeEnv 叠加环境变量覆盖（GOCLOUDAGENT_* 优先于 SIMPLEBASE_*，便于命令行临时切换）。
func MergeEnv(cfg Config, env []string) Config {
	if v, ok := envGet(env, "SIMPLEBASE_URL"); ok {
		cfg.Endpoint = v
	}
	if v, ok := envGet(env, "SIMPLEBASE_TOKEN"); ok {
		cfg.Token = v
	}
	if v, ok := envGet(env, "SIMPLEBASE_PROJECT_ID"); ok {
		cfg.ProjectID = v
	}
	if v, ok := envGet(env, "SIMPLEBASE_OUTPUT"); ok {
		cfg.Output = v
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_ENDPOINT"); ok {
		cfg.Endpoint = v
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_TOKEN"); ok {
		cfg.Token = v
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_PROJECT_ID"); ok {
		cfg.ProjectID = v
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_MODEL"); ok {
		cfg.Model = v
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_OUTPUT"); ok {
		cfg.Output = v
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_SEARCH_PROVIDER"); ok {
		cfg.SearchProvider = v
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_SEARCH_API_KEY"); ok {
		cfg.SearchAPIKey = v
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_YOLO"); ok && (v == "1" || strings.EqualFold(v, "true")) {
		cfg.Yolo = true
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_MAX_ITERATIONS"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxIterations = n
		}
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_MAX_RUN_SECONDS"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxRunSeconds = n
		}
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_COMPACT_TRIGGER_TOKENS"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.CompactTriggerTokens = n
		}
	}
	if v, ok := envGet(env, "GOCLOUDAGENT_COMPACT_KEEP_TOKENS"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.CompactKeepTokens = n
		}
	}
	if cfg.Output == "" {
		cfg.Output = "text"
	}
	return cfg
}

// Validate 报告配置是否足以发起一次 run。
func (c Config) Validate() error {
	if strings.TrimSpace(c.Endpoint) == "" {
		return errors.New("endpoint is not configured: set it in ~/.gocloudagent/config.json, `gocloudagent config set endpoint <url>`, or $GOCLOUDAGENT_ENDPOINT")
	}
	if strings.TrimSpace(c.Token) == "" {
		return errors.New("token is not configured: set $GOCLOUDAGENT_TOKEN or run `gocloudagent config set token <key>`")
	}
	if strings.TrimSpace(c.ProjectID) == "" {
		return errors.New("project_id is not configured: run `gocloudagent config set project_id <id>`")
	}
	return nil
}

func envGet(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix), true
		}
	}
	return "", false
}
