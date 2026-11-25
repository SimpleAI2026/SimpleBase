// 文件说明：mcp_cmd.go — gocloudagent/cli 的内部实现（cmdMCP、mcpList 等）
//
// 用法
//
//	同包调用：cmdMCP() → command.go, mcp_adapter_test.go；maskURL() → config.go, mcp_config_test.go
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/mcp_cmd.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] agent_loop.go, agent_loop_test.go, budget_test.go, client.go, client_conn_reuse_test.go 等 39 个
//	[关键引用] cmdMCP ← command.go, mcp_adapter_test.go；maskURL ← config.go, mcp_config_test.go
//
// 主要符号
//
//	cmdMCP(env []string, args []string, stdout, stderr *os.File) int
//	mcpList(env []string, stdout, stderr *os.File) int
//	mcpTools(env []string, only string, stdout, stderr *os.File) int
//	maskURL(raw string) string

// mcp_cmd.go — `gocloudagent mcp ...` 子命令（planv6.2 §5.2）。
//
// 只做配置管理与连通性检查：不改会话、不写配置文件的凭据以外内容。
// 凭据（token / headers）在输出里一律掩码。
package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

func cmdMCP(env []string, args []string, stdout, stderr *os.File) int {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list", "ls":
		return mcpList(env, stdout, stderr)
	case "tools":
		server := ""
		if len(args) > 1 {
			server = args[1]
		}
		return mcpTools(env, server, stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, `gocloudagent mcp <subcommand>

  list                 show configured servers and whether they are enabled
  tools [server]       connect and list tools (no credentials echoed)
  help                 show this message

Configure servers in ~/.gocloudagent/config.json under "mcp_servers":
  {"mcp_servers": {"github": {"url": "https://example.com/mcp",
                              "headers": {"Authorization": "Bearer ${GITHUB_TOKEN}"}}}}
`)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown mcp subcommand %q (try `gocloudagent mcp help`)\n", sub)
		return exitUsage
	}
}

// mcpList 列出配置里的 Server 与启用状态（不回显凭据）。
func mcpList(env []string, stdout, stderr *os.File) int {
	cfg, err := LoadConfig(env)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitConfig
	}
	if len(cfg.MCPServers) == 0 {
		fmt.Fprintln(stdout, "no mcp servers configured")
		return exitOK
	}
	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		srv := cfg.MCPServers[name]
		transport := "http"
		target := maskURL(srv.URL)
		if srv.IsStdio() {
			transport = "stdio"
			target = srv.Command
		}
		state := "enabled"
		if srv.Disabled {
			state = "disabled"
		}
		// 认证只报「配了几条 Header」，不回显值。
		auth := ""
		if len(srv.Headers) > 0 {
			auth = fmt.Sprintf("  headers=%d", len(srv.Headers))
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s%s\n", name, transport, state, target, auth)
	}
	return exitOK
}

// mcpTools 连接指定 Server（或全部）并列出工具。
func mcpTools(env []string, only string, stdout, stderr *os.File) int {
	cfg, err := LoadConfig(env)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitConfig
	}
	if only != "" {
		srv, ok := cfg.MCPServers[only]
		if !ok {
			fmt.Fprintf(stderr, "error: unknown mcp server %q\n", only)
			return exitUsage
		}
		cfg.MCPServers = map[string]MCPServerConfig{only: srv}
	}
	if len(cfg.MCPServers) == 0 {
		fmt.Fprintln(stdout, "no mcp servers configured")
		return exitOK
	}
	// 非交互：不提供 Confirm（只列工具，不执行）。
	// 隔离策略与交互 run 同源：开启时 stdio 一律拒绝。
	iso, perr := cfg.ShellIsolation.Policy(".", HomeDir(env))
	if perr != nil {
		fmt.Fprintf(stderr, "error: shell_isolation: %v\n", perr)
		return exitConfig
	}
	mcpEnv := mcpServerEnv{Env: env, Cwd: ".", IsolationEnabled: iso.Enabled}
	ctx, cancel := context.WithTimeout(context.Background(), mcpStartupTimeout)
	defer cancel()
	tools, closeAll, cerr := connectMCPServers(ctx, cfg, mcpEnv)
	defer closeAll()
	if cerr != nil {
		fmt.Fprintf(stderr, "error: %v\n", cerr)
		return exitConfig
	}
	if len(tools) == 0 {
		fmt.Fprintln(stdout, "no tools available (servers unreachable?)")
		return exitConfig
	}
	for _, name := range mcpToolNames(tools) {
		fmt.Fprintln(stdout, name)
	}
	return exitOK
}

// maskURL 隐藏 URL 里可能含凭据的部分：userinfo、query 与 fragment，
// 只保留 scheme://host/path（存在 query 时以 ?*** 标记）。
func maskURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.IndexByte(raw, '#'); idx >= 0 {
		raw = raw[:idx]
	}
	masked := ""
	if idx := strings.IndexByte(raw, '?'); idx >= 0 {
		raw, masked = raw[:idx], "?***"
	}
	if scheme := strings.Index(raw, "://"); scheme >= 0 {
		rest := raw[scheme+3:]
		authority, path := rest, ""
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			authority, path = rest[:slash], rest[slash:]
		}
		if at := strings.LastIndexByte(authority, '@'); at >= 0 {
			authority = "***@" + authority[at+1:]
		}
		raw = raw[:scheme+3] + authority + path
	}
	return raw + masked
}
