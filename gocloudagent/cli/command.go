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
	"fmt"
	"os"
	"sort"
	"strings"
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
	case "mcp":
		return cmdMCP(env, args[1:], stdout, stderr)
	case "group":
		return cmdGroup(env, args[1:], stdout, stderr)
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
  gocloudagent mcp [list|tools]       manage and inspect MCP servers
  gocloudagent group [list|show|new|run]  local groups: every message is planned first
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
  ~/.gocloudagent/config.json      CLI config (mode 0600, contains the model token)
  ~/.gocloudagent/sessions/*.jsonl append-only session log
  ~/.gocloudagent/prompts/*.md     prompt templates, invoked with /name
  ~/.gocloudagent/groups/<name>.json  group definitions (members = model + instruction + tools)
  ~/.gocloudagent/groups/<name>/   plan files (plan.md, progress.md) and turn state per group

Environment:
  GOCLOUDAGENT_HOME, GOCLOUDAGENT_ENDPOINT, GOCLOUDAGENT_TOKEN,
  GOCLOUDAGENT_MODEL, GOCLOUDAGENT_OUTPUT, GOCLOUDAGENT_SEARCH_PROVIDER,
  GOCLOUDAGENT_SEARCH_API_KEY, GOCLOUDAGENT_YOLO, GOCLOUDAGENT_MAX_ITERATIONS,
  GOCLOUDAGENT_COMPACT_TRIGGER_TOKENS, GOCLOUDAGENT_COMPACT_KEEP_TOKENS

Shell tool environment:
  Commands run by the shell tool do not inherit GOCLOUDAGENT_* or any variable
  whose name contains TOKEN, SECRET, PASSWORD, API_KEY, ACCESS_KEY or SESSION.
Context compaction:
  Long histories are summarized automatically (threshold ~32000 estimated tokens
  or 120 messages) and the summary replaces the older turns in the model context.
  Originals stay in the session file. Use /compact in the REPL to compact on
  demand; tune with compact_trigger_tokens / compact_keep_tokens.

The endpoint must be an OpenAI-compatible API base URL; a token is required.
`)
}
