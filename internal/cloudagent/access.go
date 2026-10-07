package cloudagent

import (
	"context"
	"encoding/json"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// ChatMessage is a gateway-shaped chat turn (no secrets).
// ToolCalls / ToolCallID 仅在原生 function calling 协议下使用（planv4.0 cloud-agent-optimization-plan §4.3）。
type ChatMessage struct {
	Role       string
	Content    string
	ToolCalls  []ChatToolCall
	ToolCallID string
}

// ChatToolCall 是一次原生工具调用。
type ChatToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolSpec 描述一个原生工具（OpenAI function 形状）。Parameters 为 JSON Schema。
type ToolSpec struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ChatRequest is sent to the existing LLM gateway.
type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	MaxTokens   *int
	Temperature *float64
	// Tools 非空表示走原生 function calling。
	Tools []ToolSpec
}

// ChatUsage 是一次调用的 token 用量。
type ChatUsage struct {
	PromptTokens     int
	CompletionTokens int
	ReasoningTokens  int
}

// ChatResponse is a non-stream completion.
type ChatResponse struct {
	Content      string
	Model        string
	Provider     string
	FinishReason string
	ToolCalls    []ChatToolCall
	Usage        ChatUsage
}

// TokenStream is a streaming completion reader.
type TokenStream interface {
	Next() (content string, finish bool, err error)
	Close() error
}

// StreamDelta 是一个增量帧；Content 与 ToolCall 互斥出现。
type StreamDelta struct {
	Content  string
	ToolCall *ToolCallDelta
	Usage    *ChatUsage
	Finish   bool
}

// ToolCallDelta 是原生工具调用的增量片段，按 Index 聚合。
type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	ArgsDelta string
}

// DeltaStream 是 TokenStream 的增强形态：可携带工具调用增量与用量。
// 实现方可选；未实现时退化为 TokenStream.Next。
type DeltaStream interface {
	NextDelta() (StreamDelta, error)
}

// ChatClient is the LLM gateway surface used by the eino ChatModel adapter.
type ChatClient interface {
	Chat(ctx context.Context, projectID string, req ChatRequest) (ChatResponse, error)
	Stream(ctx context.Context, projectID string, req ChatRequest) (TokenStream, error)
}

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
type DatabaseAccess interface {
	ListDatabases(ctx context.Context, principal auth.Principal, projectID string) ([]DatabaseInfo, error)
	ListCollections(ctx context.Context, principal auth.Principal, projectID, databaseID string) ([]string, error)
	ReadOnlyQuery(ctx context.Context, principal auth.Principal, projectID, databaseID, sqlText string, maxRows int) (SQLResult, error)
}

// ObjectAccess is the readonly S3 surface for tools.
type ObjectAccess interface {
	ListObjects(ctx context.Context, projectID, prefix string, limit int) ([]ObjectInfo, error)
	HeadObject(ctx context.Context, projectID, key string) (ObjectInfo, error)
}

// LogAccess is the readonly logs surface for tools.
type LogAccess interface {
	SearchLogs(ctx context.Context, projectID, level, q string, limit int) ([]systemdb.LogEvent, error)
	LevelStats(ctx context.Context, projectID string) ([]systemdb.LogLevelCount, error)
}

// SettingsAccess reads project LLM defaults (no keys).
type SettingsAccess interface {
	LLMSettings(ctx context.Context, projectID string) (systemdb.LLMSettings, error)
}

// SandboxOutput 是一次沙盒命令的结果（stdout/stderr 已截断）。
type SandboxOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Sandbox 是云沙盒的工具面（cloud-agent-sandbox-plan §4）。
// 实现方为 internal/sandbox.Client；本包不 import microsandbox SDK。
type Sandbox interface {
	Available() bool
	Exec(ctx context.Context, projectID, threadID, cmd string, args []string) (SandboxOutput, error)
	Shell(ctx context.Context, projectID, threadID, command string) (SandboxOutput, error)
	ReadFile(ctx context.Context, projectID, threadID, path string) (string, error)
	WriteFile(ctx context.Context, projectID, threadID, path, content string) error
	ReleaseThread(ctx context.Context, projectID, threadID string) error
}
