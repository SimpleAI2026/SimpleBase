package agentcore

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrMaxTokensExceeded 表示模型输出被 max_tokens 截断（finish_reason='length'），
// 内容不完整但不是调用失败。上层检测到这个错误后应追加提示而非终止整轮（planv6.1 O2）。
var ErrMaxTokensExceeded = errors.New("output truncated: max_tokens exceeded")

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
