// 文件说明：access.go — gocloudagent 中 ResetUnsupportedNativeTools、DatabaseInfo 等符号的实现
//
// 用法
//
//	type DatabaseInfo（导出类型，同包与上层均可使用）
//	type SQLResult（导出类型，同包与上层均可使用）
//	同包调用：Sandbox → cli_tool.go, modules.go 等；DatabaseAccess → runtime.go, tools.go；ObjectAccess → runtime.go, tools.go
//
// 调用架构
//
//	[上游] gocloudagent/cmd/gocloudagent, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/access.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/agentcore
//	[同包] cli_entry.go, cli_runner.go, cli_tool.go, errors.go, identity.go 等 11 个
//	[关键引用] Sandbox ← cli_tool.go, modules.go, runtime.go
//
// 主要符号
//
//	var ResetUnsupportedNativeTools
//	type DatabaseInfo struct {…}
//	type SQLResult struct {…}
//	type ObjectInfo struct {…}
//	type DatabaseAccess interface {…}
//	type ObjectAccess interface {…}
//	... 另有 3 个顶层符号

package gocloudagent

import (
	"context"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// 网关类型已迁至 agentcore（云端 Runtime 与本地 CLI 共用）；此处保留别名，
// 使 api 适配器与既有调用方的符号不变。
type (
	ChatMessage   = agentcore.ChatMessage
	ChatToolCall  = agentcore.ChatToolCall
	ToolSpec      = agentcore.ToolSpec
	ChatRequest   = agentcore.ChatRequest
	ChatUsage     = agentcore.ChatUsage
	ChatResponse  = agentcore.ChatResponse
	TokenStream   = agentcore.TokenStream
	StreamDelta   = agentcore.StreamDelta
	ToolCallDelta = agentcore.ToolCallDelta
	DeltaStream   = agentcore.DeltaStream
	ChatClient    = agentcore.ChatClient
)

// ResetUnsupportedNativeTools 是 agentcore 的别名（测试与适配器沿用旧符号）。
var ResetUnsupportedNativeTools = agentcore.ResetUnsupportedNativeTools

// DatabaseInfo is a non-secret catalog row for tools/snapshots.
type DatabaseInfo struct {
	ID     string
	Name   string
	Status string
}

// SQLResult is a truncated readonly query result.
type SQLResult struct {
	Columns  []string
	Rows     [][]any
	RowCount int
}

// ObjectInfo is a project-relative object (no physical prefix, no credentials).
type ObjectInfo struct {
	Key          string
	Size         int64
	ETag         string
	ContentType  string
	LastModified string
}

// DatabaseAccess is the readonly database surface for tools.
//
// 实现方是宿主的 api.cloudAgentDB：它把 Principal 还原成 Principal 后
// 继续走 catalog 的项目授权，模块不参与鉴权。
type DatabaseAccess interface {
	ListDatabases(ctx context.Context, principal Principal, projectID string) ([]DatabaseInfo, error)
	ListCollections(ctx context.Context, principal Principal, projectID, databaseID string) ([]string, error)
	ReadOnlyQuery(ctx context.Context, principal Principal, projectID, databaseID, sqlText string, maxRows int) (SQLResult, error)
}

// ObjectAccess is the readonly S3 surface for tools.
type ObjectAccess interface {
	ListObjects(ctx context.Context, projectID, prefix string, limit int) ([]ObjectInfo, error)
	HeadObject(ctx context.Context, projectID, key string) (ObjectInfo, error)
}

// LogAccess is the readonly logs surface for tools.
type LogAccess interface {
	SearchLogs(ctx context.Context, projectID, level, q string, limit int) ([]LogEvent, error)
	LevelStats(ctx context.Context, projectID string) ([]LogLevelCount, error)
}

// SandboxOutput 是一次沙盒命令的结果（stdout/stderr 已截断）。
type SandboxOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Sandbox 是云沙盒的工具面（cloud-agent-sandbox-plan §4）。
// 实现方为 app.sandboxAdapter（包装 sandbox.Manager）；本包不 import gosandbox。
type Sandbox interface {
	Available() bool
	Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (SandboxOutput, error)
	Shell(ctx context.Context, projectID, threadID, command string) (SandboxOutput, error)
	ReadFile(ctx context.Context, projectID, threadID, path string) (string, error)
	WriteFile(ctx context.Context, projectID, threadID, path, content string) error
	ReleaseThread(ctx context.Context, projectID, threadID string) error
}
