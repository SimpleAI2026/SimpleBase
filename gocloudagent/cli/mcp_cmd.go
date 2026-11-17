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
	"time"
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
	mcpEnv := mcpServerEnv{Env: env, Cwd: "."}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

// maskURL 隐藏 URL 里的 query（可能含 token），只保留 scheme://host/path。
func maskURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.IndexByte(raw, '?'); idx >= 0 {
		return raw[:idx] + "?***"
	}
	return raw
}
