// 文件说明：mcp_config.go — gocloudagent/cli 的内部实现（mcpEnvPrefix、mcpEnvSuffix 等）
//
// 用法
//
//	同包调用：expandEnvValue() → mcp_config_test.go；mcpChildEnv() → mcp_config_test.go；mcpServerEnv → mcp_adapter.go, mcp_adapter_test.go
//
// 调用架构
//
//	[上游] gocloudagent
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cli/mcp_config.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_loop.go, agent_loop_test.go, budget_test.go, client.go, client_conn_reuse_test.go 等 39 个
//	[关键引用] expandEnvValue ← mcp_config_test.go；mcpChildEnv ← mcp_config_test.go；mcpServerEnv ← mcp_adapter.go, mcp_adapter_test.go
//
// 主要符号
//
//	const mcpEnvPrefix
//	const mcpEnvSuffix
//	expandEnvValue(v string, env []string) string
//	mcpChildEnv(configured map[string]string, env []string) []string
//	isSensitiveEnvKey(k string) bool
//	type mcpServerEnv struct{…}
//	(e mcpServerEnv) warn(msg string, fields ...zap.Field)
//	const mcpStartupTimeout
//	... 另有 4 个顶层符号

// mcp_config.go — CLI 侧 MCP 集成（planv6.2 §5）。
//
// 与云端的分工差异：
//   - CLI 没有 Agent.Writable 概念，权限模型是「首次调用需确认」；
//   - stdio 传输**仅 CLI 支持**，云端一律拒绝（起任意子进程等于任意命令执行）；
//   - 子进程环境不继承父进程，只传显式 env + 最小 PATH/HOME，
//     并过滤 GOCLOUDAGENT_* 与凭据类变量；
//   - 启用 shell_isolation 时 stdio 子进程同样要被隔离，隔离不可用则 fail-closed。
package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
	"github.com/linkxzhou/SimpleBase/gocloudagent/tools"
)

// mcpEnvVarPattern 匹配 ${VAR} 占位符。
const mcpEnvPrefix = "${"
const mcpEnvSuffix = "}"

// expandEnvValue 展开 ${VAR}；未定义的变量替换为空串（不报错，
// 避免把未使用的占位符变成启动失败）。
func expandEnvValue(v string, env []string) string {
	if !strings.Contains(v, mcpEnvPrefix) {
		return v
	}
	var b strings.Builder
	rest := v
	for {
		start := strings.Index(rest, mcpEnvPrefix)
		if start < 0 {
			b.WriteString(rest)
			return b.String()
		}
		end := strings.Index(rest[start:], mcpEnvSuffix)
		if end < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:start])
		name := rest[start+len(mcpEnvPrefix) : start+end]
		if val, ok := envGet(env, strings.TrimSpace(name)); ok {
			b.WriteString(val)
		}
		rest = rest[start+end+len(mcpEnvSuffix):]
	}
}

// mcpChildEnv 构造 stdio 子进程的环境变量：不继承父进程，只保留显式配置 +
// 最小 PATH/HOME，并过滤掉凭据类变量（planv6.2 §5.1）。
//
// 顺序：显式 env 优先；过滤只作用于继承来的值，用户显式写入的保留。
func mcpChildEnv(configured map[string]string, env []string) []string {
	out := make([]string, 0, len(configured)+3)
	// PATH/HOME 是子进程正常工作的最小依赖。
	for _, key := range []string{"PATH", "HOME"} {
		if v, ok := envGet(env, key); ok && strings.TrimSpace(v) != "" {
			out = append(out, key+"="+v)
			continue
		}
		if key == "PATH" {
			out = append(out, "PATH=/usr/bin:/bin")
		}
	}
	keys := make([]string, 0, len(configured))
	for k := range configured {
		keys = append(keys, k)
	}
	// 排序保证环境数组稳定，便于测试与复现。
	sort.Strings(keys)
	for _, k := range keys {
		if isSensitiveEnvKey(k) || strings.HasPrefix(k, "GOCLOUDAGENT_") {
			// 显式写入的凭据类变量仍被过滤：MCP Server 不应继承调用方令牌。
			continue
		}
		out = append(out, k+"="+expandEnvValue(configured[k], env))
	}
	return out
}

// isSensitiveEnvKey 报告变量名是否像凭据。
func isSensitiveEnvKey(k string) bool {
	upper := strings.ToUpper(k)
	for _, bad := range []string{"TOKEN", "SECRET", "PASSWORD", "API_KEY", "APIKEY", "CREDENTIAL", "PRIVATE_KEY"} {
		if strings.Contains(upper, bad) {
			return true
		}
	}
	return false
}

// mcpServerEnv 是建立连接所需的最小上下文。
type mcpServerEnv struct {
	// Workspace 提供确认（nil 表示非交互，一律拒绝破坏性调用）。
	Workspace *tools.Workspace
	// IsolationEnabled 为真时拒绝 stdio 传输（fail-closed）。独立于 Workspace：
	// 非交互入口（如 `mcp tools`）没有 Workspace，也必须遵守隔离配置。
	IsolationEnabled bool
	// Env 是进程环境（用于 ${VAR} 展开与子进程环境构造）。
	Env []string
	// Cwd 是 stdio 子进程的工作目录。
	Cwd string
	// Warn 记录降级路径。
	Warn func(msg string, fields ...zap.Field)
}

func (e mcpServerEnv) warn(msg string, fields ...zap.Field) {
	if e.Warn != nil {
		e.Warn(msg, fields...)
	}
}

// mcpStartupTimeout 是 CLI 启动期连接全部 MCP Server 的总时限。
const mcpStartupTimeout = 30 * time.Second

// connectMCPServers 按配置建立会话，返回可用工具与关闭钩子。
//
// 单个 Server 失败只 warn 并从列表里剔除，不阻止 CLI 启动（§5.2）。
// 返回的 close 函数必须在会话结束时调用（REPL 退出 / 单次 run 结束）。
func connectMCPServers(ctx context.Context, cfg Config, env mcpServerEnv) ([]tool.BaseTool, func(), error) {
	if len(cfg.MCPServers) == 0 {
		return nil, func() {}, nil
	}
	// 名称排序：连接顺序稳定，便于复现与测试。
	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []tool.BaseTool
	sessions := make([]mcp.Session, 0, len(names))
	closeAll := func() {
		for _, s := range sessions {
			_ = s.Close()
		}
	}
	for _, name := range names {
		srv := cfg.MCPServers[name]
		if srv.Disabled {
			continue
		}
		sess, err := connectOne(ctx, name, srv, env)
		if err != nil {
			// 错误只报分类，不回显 URL/命令（可能含凭据）。
			env.warn("mcp: server unavailable", zap.String("server", name), zap.String("err", err.Error()))
			continue
		}
		infos, err := sess.ListTools(ctx)
		if err != nil {
			_ = sess.Close()
			env.warn("mcp: list tools failed", zap.String("server", name))
			continue
		}
		sessions = append(sessions, sess)
		for _, info := range infos {
			if len(out) >= mcp.MaxToolsPerRun {
				env.warn("mcp: tool limit reached, remaining tools skipped")
				break
			}
			out = append(out, newMCPLocalTool(name, srv, sess, info, env))
		}
	}
	return out, closeAll, nil
}

// connectOne 建立一个 Server 的会话。
func connectOne(ctx context.Context, name string, srv MCPServerConfig, env mcpServerEnv) (mcp.Session, error) {
	if srv.IsStdio() {
		return connectStdioSession(ctx, name, srv, env)
	}
	client, err := mcpHTTPClient(name, srv, env.Env)
	if err != nil {
		return nil, err
	}
	// 连接超时固定：Server 的 timeout 是调用超时，不是连接超时。
	return mcp.ConnectHTTP(ctx, name, mcp.HTTPOptions{
		Endpoint:       strings.TrimSpace(srv.URL),
		HTTPClient:     client,
		ConnectTimeout: mcp.DefaultConnectTimeout,
	})
}

// connectStdioSession 建立 stdio 会话（仅 CLI）。
//
// 隔离不可用时 fail-closed：不退回裸执行（§5.1）。
func connectStdioSession(ctx context.Context, name string, srv MCPServerConfig, env mcpServerEnv) (mcp.Session, error) {
	if env.IsolationEnabled {
		// 启用了隔离：当前 SDK 的 CommandTransport 自己 Start 子进程，
		// 无法把已包装的 *exec.Cmd 交给它（planv6.2 §5.1 路径 1 尚未落地），
		// 因此对 stdio 一律拒绝，只放行 HTTP Server。
		return nil, errors.New("stdio mcp servers are not supported while shell_isolation is enabled")
	}
	cmd := exec.Command(srv.Command, srv.Args...)
	cmd.Dir = env.Cwd
	cmd.Env = mcpChildEnv(srv.Env, env.Env)
	return mcp.ConnectStdio(ctx, name, mcp.StdioOptions{Command: cmd})
}

// mcpHTTPClient 构造 CLI 侧 HTTP 客户端。
//
// CLI 是本机工具，用户自己的网络环境，不做云端的私网封锁（否则无法连
// 局域网内的自建 Server）；但仍要求传入非 nil 客户端（mcp 包的硬约束）。
func mcpHTTPClient(name string, srv MCPServerConfig, env []string) (*http.Client, error) {
	if strings.TrimSpace(srv.URL) == "" {
		return nil, fmt.Errorf("mcp: %s has no url", name)
	}
	headers := make(http.Header, len(srv.Headers))
	for k, v := range srv.Headers {
		// Header 值支持 ${VAR} 展开（凭据不落配置文件明文）；展开结果只存在于传输层。
		headers.Set(k, expandEnvValue(v, env))
	}
	return &http.Client{
		Timeout:   mcpCallTimeout(srv),
		Transport: &mcp.HeaderTransport{Base: http.DefaultTransport, Headers: headers},
	}, nil
}
