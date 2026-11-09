// 文件说明：command.go — gocloudagent/cli 中 RunCommand 等符号的实现
//
// 用法
//
//	RunCommand(env []string, args []string, stdout, stderr *os.File) int  // RunCommand 处理子命令分派，返回退出码
//	同包调用：RunCommand() → config.go；cmdRun() → config.go；cmdResume() → config.go
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/command.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] client.go, compaction.go, config.go, prompt.go, repeat_detector.go 等 9 个
//	[关键引用] RunCommand() ← config.go
//
// 主要符号
//
//	RunCommand(env []string, args []string, stdout, stderr *os.File) int
//	... 另有 11 个顶层符号

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RunCommand 处理子命令分派，返回退出码。
//
// 命令面刻意克制（§5.1）：不要把 sbcli 的 72 个命令再包一层。
func RunCommand(env []string, args []string, stdout, stderr *os.File) int {
	opts := Options{Env: env, Stdout: stdout, Stderr: stderr}
	if len(args) == 0 {
		return Run(opts)
	}
	switch args[0] {
	case "run":
		return cmdRun(env, args[1:], stdout, stderr)
	case "resume":
		return cmdResume(env, args[1:], stdout, stderr)
	case "sessions":
		return cmdSessions(env, args[1:], stdout, stderr)
	case "config":
		return cmdConfig(env, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return exitOK
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, version)
		return exitOK
	default:
		// 无子命令时把参数当 prompt（`gocloudagent "查一下..."`）。
		if strings.HasPrefix(args[0], "-") {
			return Run(parseFlags(opts, args))
		}
		opts.Headless = true
		opts.Prompt = strings.Join(args, " ")
		return Run(opts)
	}
}

// parseFlags 解析 `gocloudagent [-p prompt] [--json] [--yolo] [--session id]`。
func parseFlags(opts Options, args []string) Options {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--print", "--prompt":
			if i+1 < len(args) {
				i++
				opts.Prompt = args[i]
				opts.Headless = true
			}
		case "--json":
			opts.JSON = true
		case "--yolo", "--dangerously-skip-permissions":
			opts.Yolo = true
		case "--session", "-s":
			if i+1 < len(args) {
				i++
				opts.SessionID = args[i]
			}
		case "--cwd":
			if i+1 < len(args) {
				i++
				opts.Cwd = args[i]
			}
		case "--shell-isolation":
			if i+1 < len(args) {
				i++
				iso, err := ShellIsolationFromFlag(args[i])
				if err != nil {
					opts.FlagErr = err
					break
				}
				opts.Config.ShellIsolation = overlayShellIsolation(opts.Config.ShellIsolation, iso)
			}
		}
	}
	// 位置参数作为 prompt 的补充。
	var rest []string
	skip := map[int]bool{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--print", "--prompt", "--session", "-s", "--cwd", "--shell-isolation":
			skip[i] = true
			skip[i+1] = true
		}
	}
	for i, a := range args {
		if skip[i] || strings.HasPrefix(a, "-") {
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) > 0 {
		opts.Prompt = strings.TrimSpace(opts.Prompt + " " + strings.Join(rest, " "))
		opts.Headless = true
	}
	return opts
}

func cmdRun(env []string, args []string, stdout, stderr *os.File) int {
	opts := Options{Env: env, Stdout: stdout, Stderr: stderr, Headless: true}
	opts = parseFlags(opts, args)
	return Run(opts)
}

func cmdResume(env []string, args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: gocloudagent resume <session-id> [prompt]")
		return exitUsage
	}
	opts := Options{Env: env, Stdout: stdout, Stderr: stderr, SessionID: args[0]}
	opts = parseFlags(opts, args[1:])
	return Run(opts)
}

func cmdSessions(env []string, args []string, stdout, stderr *os.File) int {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list", "ls":
		sessions, err := ListSessions(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		if len(sessions) == 0 {
			fmt.Fprintln(stdout, "no sessions yet")
			return exitOK
		}
		for _, s := range sessions {
			fmt.Fprintf(stdout, "%s  %s  %s\n", s.ID, s.CreatedAt.Local().Format(time.DateTime), s.Cwd)
		}
		return exitOK
	case "show":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: gocloudagent sessions show <session-id>")
			return exitUsage
		}
		path, err := FindSession(env, args[1])
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		s, err := OpenSession(path)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		for _, e := range s.Lines {
			switch e.Type {
			case EntrySession:
				fmt.Fprintf(stdout, "session %s  cwd=%s  project=%s  model=%s\n", s.ID, e.Cwd, e.ProjectID, e.Model)
			case EntryCompaction:
				fmt.Fprintf(stdout, "[compaction] %s\n", firstLine(e.Summary, 120))
			default:
				fmt.Fprintf(stdout, "%s: %s\n", e.Role, firstLine(e.Content, 200))
			}
		}
		return exitOK
	case "rm", "remove", "delete":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: gocloudagent sessions rm <session-id>")
			return exitUsage
		}
		path, err := FindSession(env, args[1])
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		if err := RemoveSession(path); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		fmt.Fprintln(stdout, "removed", filepath.Base(path))
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown sessions subcommand %q\n", sub)
		return exitUsage
	}
}

// configKeys 是 config get/set 支持的键。
var configKeys = []struct {
	Name string
	Get  func(Config) string
	Set  func(*Config, string)
}{
	{"endpoint", func(c Config) string { return c.Endpoint }, func(c *Config, v string) { c.Endpoint = v }},
	{"token", func(c Config) string { return c.Token }, func(c *Config, v string) { c.Token = v }},
	{"project_id", func(c Config) string { return c.ProjectID }, func(c *Config, v string) { c.ProjectID = v }},
	{"model", func(c Config) string { return c.Model }, func(c *Config, v string) { c.Model = v }},
	{"output", func(c Config) string { return c.Output }, func(c *Config, v string) { c.Output = v }},
	{"search_provider", func(c Config) string { return c.SearchProvider }, func(c *Config, v string) { c.SearchProvider = v }},
	{"search_api_key", func(c Config) string { return c.SearchAPIKey }, func(c *Config, v string) { c.SearchAPIKey = v }},
	{"yolo", func(c Config) string { return fmt.Sprintf("%t", c.Yolo) }, func(c *Config, v string) { c.Yolo = v == "true" || v == "1" }},
	{"max_run_seconds", func(c Config) string { return fmt.Sprintf("%d", c.MaxRunSeconds) }, func(c *Config, v string) { setPositiveInt(&c.MaxRunSeconds, v) }},
	{"compact_trigger_tokens", func(c Config) string { return fmt.Sprintf("%d", c.CompactTriggerTokens) }, func(c *Config, v string) { setPositiveInt(&c.CompactTriggerTokens, v) }},
	{"compact_keep_tokens", func(c Config) string { return fmt.Sprintf("%d", c.CompactKeepTokens) }, func(c *Config, v string) { setPositiveInt(&c.CompactKeepTokens, v) }},
}

// setPositiveInt 解析正整数配置；非法值保持原样（0 即用默认值），
// 避免把 `config set compact_keep_tokens abc` 变成静默清零。
func setPositiveInt(dst *int, v string) {
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		*dst = n
	}
}

func cmdConfig(env []string, args []string, stdout, stderr *os.File) int {
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "path":
		fmt.Fprintln(stdout, configPath(env))
		return exitOK
	case "show", "list":
		cfg, err := LoadConfig(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		for _, k := range configKeys {
			v := k.Get(cfg)
			if k.Name == "token" && v != "" {
				v = maskSecret(v)
			}
			fmt.Fprintf(stdout, "%s=%s\n", k.Name, v)
		}
		return exitOK
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: gocloudagent config get <key>")
			return exitUsage
		}
		cfg, err := LoadConfig(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		for _, k := range configKeys {
			if k.Name == args[1] {
				fmt.Fprintln(stdout, k.Get(cfg))
				return exitOK
			}
		}
		fmt.Fprintf(stderr, "unknown key %q; known: %s\n", args[1], knownKeys())
		return exitUsage
	case "set":
		if len(args) < 3 {
			fmt.Fprintln(stderr, "usage: gocloudagent config set <key> <value>")
			return exitUsage
		}
		cfg, err := LoadConfig(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		found := false
		for _, k := range configKeys {
			if k.Name == args[1] {
				k.Set(&cfg, args[2])
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(stderr, "unknown key %q; known: %s\n", args[1], knownKeys())
			return exitUsage
		}
		path, err := SaveConfig(env, cfg)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		fmt.Fprintln(stdout, "wrote", path)
		return exitOK
	case "test":
		cfg := MergeEnv(mustConfig(env), env)
		if err := cfg.Validate(); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		client := &HTTPClient{Endpoint: cfg.Endpoint, Token: cfg.Token, ProjectID: cfg.ProjectID}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := client.Probe(ctx); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitAPI
		}
		fmt.Fprintln(stdout, "ok")
		return exitOK
	case "json":
		cfg, err := LoadConfig(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		cfg.Token = maskSecret(cfg.Token)
		b, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Fprintln(stdout, string(b))
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown config subcommand %q\n", sub)
		return exitUsage
	}
}

func mustConfig(env []string) Config {
	cfg, err := LoadConfig(env)
	if err != nil {
		return Config{}
	}
	return cfg
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		if s == "" {
			return ""
		}
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

func knownKeys() string {
	names := make([]string, 0, len(configKeys))
	for _, k := range configKeys {
		names = append(names, k.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `gocloudagent — local coding assistant CLI

Usage:
  gocloudagent                        interactive REPL (needs a TTY)
  gocloudagent "prompt"               one-shot headless run
  gocloudagent -p "prompt"            same, explicit flag
  gocloudagent run [flags] "prompt"   headless with flags
  gocloudagent resume <id> [prompt]   continue an existing session
  gocloudagent sessions [list|show|rm]  manage local sessions
  gocloudagent config [show|get|set|path|test|json]  manage ~/.gocloudagent/config.json
  gocloudagent --version | --help

Flags:
  -p, --print <prompt>   prompt for a one-shot run
      --json             emit one JSON event per line (same event names as the server SSE)
      --session <id>     resume the given session
      --cwd <dir>        workspace root for local file and shell tools
      --yolo             skip confirmation for destructive shell commands (risky)
      --shell-isolation <mode>
                         run the shell tool in a sandbox: read-only | workspace-write |
                         danger-full-access (off). --yolo does NOT disable the sandbox.

Layout:
  ~/.gocloudagent/config.json      CLI config (mode 0600, contains the SimpleBase token)
  ~/.gocloudagent/sessions/*.jsonl append-only session log
  ~/.gocloudagent/prompts/*.md     prompt templates, invoked with /name

Environment (GOCLOUDAGENT_* overrides SIMPLEBASE_*):
  GOCLOUDAGENT_HOME, GOCLOUDAGENT_ENDPOINT, GOCLOUDAGENT_TOKEN, GOCLOUDAGENT_PROJECT_ID,
  GOCLOUDAGENT_MODEL, GOCLOUDAGENT_OUTPUT, GOCLOUDAGENT_SEARCH_PROVIDER,
  GOCLOUDAGENT_SEARCH_API_KEY, GOCLOUDAGENT_YOLO, GOCLOUDAGENT_MAX_ITERATIONS,
  GOCLOUDAGENT_COMPACT_TRIGGER_TOKENS, GOCLOUDAGENT_COMPACT_KEEP_TOKENS
  SIMPLEBASE_URL, SIMPLEBASE_TOKEN, SIMPLEBASE_PROJECT_ID

Shell tool environment:
  Commands run by the shell tool do not inherit SIMPLEBASE_* / GOCLOUDAGENT_* or any
  variable whose name contains TOKEN, SECRET, PASSWORD, API_KEY, ACCESS_KEY or SESSION.
  A "simplebase" CLI started from the shell therefore reports config_missing unless
  its own config file is set up beforehand (never print that file to the model).
Context compaction:
  Long histories are summarized automatically (threshold ~32000 estimated tokens
  or 120 messages) and the summary replaces the older turns in the model context.
  Originals stay in the session file. Use /compact in the REPL to compact on
  demand; tune with compact_trigger_tokens / compact_keep_tokens.

The model runs through the SimpleBase server, so an endpoint and token are required.
`)
}
