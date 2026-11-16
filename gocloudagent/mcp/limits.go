// Package mcp 是 gocloudagent 的 MCP 客户端封装（planv6.2 §3.1）。
//
// 边界：本包只依赖 eino 与官方 go-sdk，**不 import gocloudagent 根包**，
// 也不把 SDK 类型（*mcp.ClientSession / *mcp.Tool 等）暴露给调用方——
// 对外只有 Session / ToolInfo / CallResult 三个自有类型，换 SDK 的影响面
// 限定在本包内。
package mcp

import "time"

const (
	// MaxToolsPerServer 是单个 Server 暴露给模型的工具上限（§3.3）。
	// 超出部分丢弃并告警，防止恶意 Server 用无限分页撑爆上下文。
	MaxToolsPerServer = 64
	// MaxToolsPerRun 是一次 run 合计的工具上限（§3.3）。
	MaxToolsPerRun = 128
	// MaxSchemaBytes 是单个工具 inputSchema 序列化后的上限（§3.3）。
	MaxSchemaBytes = 16 << 10
	// MaxDescriptionRunes 是工具描述截断长度（§3.3）。
	MaxDescriptionRunes = 1024
	// MaxResultBytes 是单次调用结果回给模型的上限（§3.4）。
	MaxResultBytes = 32 << 10
	// MaxEventBytes 是 Streamable HTTP 单条 SSE 事件的上限（§3.1.1）。
	// 显式设置，不用 SDK 的 0（默认值）或负数（不限）。
	MaxEventBytes = 4 << 20

	// DefaultConnectTimeout 是建立会话的超时（§3.1.1）。
	DefaultConnectTimeout = 15 * time.Second
	// DefaultCallTimeout 是单次 tools/call 的默认超时（§3.4）。
	DefaultCallTimeout = 60 * time.Second
	// MaxCallTimeout 是 Server 级可配超时的上限（§3.4）。
	MaxCallTimeout = 300 * time.Second
	// DefaultTerminateDuration 是 stdio 子进程的优雅退出窗口（§3.1.1）。
	DefaultTerminateDuration = 3 * time.Second
)

// 稳定错误码：与 skillFail 的信封同形，前端与模型都按 code 分支。
const (
	CodeUnavailable             = "mcp_unavailable"
	CodeTimeout                 = "mcp_timeout"
	CodeToolError               = "mcp_tool_error"
	CodeInputRequired           = "mcp_input_required_unsupported"
	CodePolicyUnavailable       = "mcp_policy_unavailable"
	CodeWriteNotAllowed         = "write_not_allowed"
	CodeConfirmationDenied      = "confirmation_denied"
	CodeConfirmationUnavailable = "confirmation_unavailable"
)
