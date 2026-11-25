// 文件说明：client.go — gocloudagent/mcp 中 ToolInfo、CallResult 等符号的实现
//
// 用法
//
//	type ToolInfo struct{…}  // ToolInfo 是暴露给调用方的工具元数据（不含 SDK 类型）
//	type CallResult struct{…}  // CallResult 是一次 tools/call 的结果（不含 SDK 类型）
//	type Session interface{…}  // Session 是一个已连接的 MCP 会话
//	type SessionSet interface{…}  // SessionSet 是一轮 run 内可用的会话集合
//	type HTTPOptions struct{…}  // HTTPOptions 是 Streamable HTTP 传输的构造参数
//	type StdioOptions struct{…}  // StdioOptions 是 stdio 传输的构造参数（仅 CLI）
//	同包调用：connect() → server.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/cloudagent, gocloudagent/tools, internal/agentbridge 等 5 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/mcp/client.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapter_test.go, http_transport.go, http_transport_test.go, limits.go 等 10 个
//	[关键引用] ToolInfo ← adapter.go, server_test.go；CallResult ← adapter.go, server.go；Session ← server.go, server_test.go
//
// 主要符号
//
//	type ToolInfo struct{…}
//	type CallResult struct{…}
//	type Session interface{…}
//	type SessionSet interface{…}
//	type HTTPOptions struct{…}
//	type StdioOptions struct{…}
//	ConnectHTTP(ctx context.Context, serverID string, opts HTTPOptions) (Session, error)
//	ConnectStdio(ctx context.Context, serverID string, opts StdioOptions) (Session, error)
//	... 另有 11 个顶层符号

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolInfo 是暴露给调用方的工具元数据（不含 SDK 类型）。
type ToolInfo struct {
	// Name 是 Server 侧原始工具名（未加 mcp__ 前缀）。
	Name        string
	Description string
	// InputSchema 是 Server 返回的 JSON Schema 原文（已序列化）。
	InputSchema []byte
	// ReadOnlyHint 是 Server 自报的只读提示，仅作配置向导的默认建议，
	// 不作为权限判定依据（§4.2-1）。
	ReadOnlyHint bool
	// DestructiveHint 是 Server 自报的破坏性提示，同样只作建议。
	DestructiveHint bool
}

// CallResult 是一次 tools/call 的结果（不含 SDK 类型）。
type CallResult struct {
	// Text 是拼接后的文本结果，已按 MaxResultBytes 截断。
	Text string
	// IsError 对应 MCP 的 isError 字段。
	IsError bool
	// Truncated 表示结果被截断。
	Truncated bool
}

// Session 是一个已连接的 MCP 会话。实现必须并发安全（§4.3）。
type Session interface {
	// ServerID 返回配置里的 Server 标识。
	ServerID() string
	// ListTools 列出工具，最多 MaxToolsPerServer 个。
	ListTools(ctx context.Context) ([]ToolInfo, error)
	// CallTool 调用一个工具。arguments 是模型给的原始 JSON。
	CallTool(ctx context.Context, tool string, arguments []byte) (CallResult, error)
	// Close 关闭会话并回收子进程（stdio）。
	Close() error
}

// SessionSet 是一轮 run 内可用的会话集合。
type SessionSet interface {
	Sessions() []Session
	Close() error
}

// HTTPOptions 是 Streamable HTTP 传输的构造参数。
type HTTPOptions struct {
	// Endpoint 是 MCP Server 的 URL。
	Endpoint string
	// HTTPClient 必须由宿主注入（SSRF 防护、Header 注入都在其 RoundTripper 里）。
	// 传 nil 直接报错：SDK 不做任何 SSRF 防护，禁止回落到 http.DefaultClient（§4.2-5）。
	HTTPClient *http.Client
	// ConnectTimeout 为零时用 DefaultConnectTimeout。
	ConnectTimeout time.Duration
}

// StdioOptions 是 stdio 传输的构造参数（仅 CLI）。
type StdioOptions struct {
	// Command 是已构造但未启动的子进程。环境变量必须由调用方显式构造，
	// 不继承父进程（§5.1）。
	Command *exec.Cmd
	// ConnectTimeout 为零时用 DefaultConnectTimeout。
	ConnectTimeout time.Duration
}

// ConnectHTTP 建立 Streamable HTTP 会话。
func ConnectHTTP(ctx context.Context, serverID string, opts HTTPOptions) (Session, error) {
	if strings.TrimSpace(opts.Endpoint) == "" {
		return nil, errors.New("mcp: endpoint is required")
	}
	if opts.HTTPClient == nil {
		// fail-closed：SDK 默认 client 会直连任意地址，等于放弃 SSRF 防护。
		return nil, errors.New("mcp: http client is required (SSRF protection must be injected by the host)")
	}
	transport := &sdkmcp.StreamableClientTransport{
		Endpoint:   opts.Endpoint,
		HTTPClient: opts.HTTPClient,
		// per-run 连接不需要常驻 GET 流（§3.1.1）。
		DisableStandaloneSSE: true,
		// 重试交给上层：per-run 连接失败即该 Server 不可用。
		MaxRetries: -1,
		// 显式设上限，不用 0（默认）或负数（不限）。
		MaxEventSize: MaxEventBytes,
	}
	return connect(ctx, serverID, transport, opts.ConnectTimeout)
}

// ConnectStdio 建立 stdio 会话（仅 CLI）。
func ConnectStdio(ctx context.Context, serverID string, opts StdioOptions) (Session, error) {
	if opts.Command == nil {
		return nil, errors.New("mcp: command is required")
	}
	transport := &sdkmcp.CommandTransport{
		Command:           opts.Command,
		TerminateDuration: DefaultTerminateDuration,
	}
	return connect(ctx, serverID, transport, opts.ConnectTimeout)
}

func connect(ctx context.Context, serverID string, transport sdkmcp.Transport, timeout time.Duration) (Session, error) {
	if timeout <= 0 {
		timeout = DefaultConnectTimeout
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "gocloudagent",
		Version: "1",
	}, nil)
	cs, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: connect %s: %w", serverID, err)
	}
	return &sdkSession{id: serverID, cs: cs}, nil
}

// ErrTooManyPages 表示 tools/list 的页数超出预算。
var ErrTooManyPages = errors.New("mcp: tools/list exceeded page budget")

// sdkSession 是 Session 的 go-sdk 实现。SDK 类型只出现在本文件。
type sdkSession struct {
	id string
	cs *sdkmcp.ClientSession
}

func (s *sdkSession) ServerID() string { return s.id }

func (s *sdkSession) ListTools(ctx context.Context) ([]ToolInfo, error) {
	if s == nil || s.cs == nil {
		return nil, errors.New("mcp: session is closed")
	}
	out := make([]ToolInfo, 0, 16)
	// 自己翻页而不用 SDK 的 Tools 迭代器：有效工具累计到 MaxToolsPerServer 即停，
	// 同时对页数设总预算——否则恶意 Server 可以用「空页 / 全无效工具」的无限分页
	// 让累计数永远到不了上限，一直占用连接直到调用方超时。
	params := &sdkmcp.ListToolsParams{}
	for page := 0; ; page++ {
		if page >= MaxListPages {
			return nil, fmt.Errorf("mcp: list tools: %w (%d pages)", ErrTooManyPages, MaxListPages)
		}
		res, err := s.cs.ListTools(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("mcp: list tools: %w", err)
		}
		for _, t := range res.Tools {
			if t == nil {
				continue
			}
			info, ok := toToolInfo(t)
			if !ok {
				continue
			}
			out = append(out, info)
			if len(out) >= MaxToolsPerServer {
				return out, nil
			}
		}
		if res.NextCursor == "" {
			return out, nil
		}
		params = &sdkmcp.ListToolsParams{Cursor: res.NextCursor}
	}
}

func (s *sdkSession) CallTool(ctx context.Context, tool string, arguments []byte) (CallResult, error) {
	if s == nil || s.cs == nil {
		return CallResult{}, errors.New("mcp: session is closed")
	}
	params := &sdkmcp.CallToolParams{Name: tool}
	if len(arguments) > 0 {
		// 原样透传模型给的 JSON，不在客户端重新校验 schema（校验权在 Server）。
		params.Arguments = json.RawMessage(arguments)
	}
	res, err := s.cs.CallTool(ctx, params)
	if err != nil {
		return CallResult{}, fmt.Errorf("mcp: call %s: %w", tool, err)
	}
	if res == nil {
		return CallResult{}, errors.New("mcp: empty result")
	}
	// input-required（多轮往返）没有可满足的 handler，统一报错，绝不挂起（§3.1.1）。
	if res.NeedsInput() {
		return CallResult{}, fmt.Errorf("mcp: %s", CodeInputRequired)
	}
	return toCallResult(res), nil
}

func (s *sdkSession) Close() error {
	if s == nil || s.cs == nil {
		return nil
	}
	return s.cs.Close()
}

// sessionSet 是 SessionSet 的简单实现。
type sessionSet struct {
	sessions []Session
}

func (s *sessionSet) Sessions() []Session { return s.sessions }

func (s *sessionSet) Close() error {
	var first error
	for _, sess := range s.sessions {
		if err := sess.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// NewSessionSet 构造会话集合。
func NewSessionSet(sessions []Session) SessionSet {
	return &sessionSet{sessions: sessions}
}
