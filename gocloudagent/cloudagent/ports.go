// 文件说明：ports.go — gocloudagent/cloudagent 中 ChatMessage、ChatToolCall 等符号的实现
//
// 用法
//
//	type ChatMessage = agentcore.ChatMessage
//	type ChatToolCall = agentcore.ChatToolCall
//	type ChatRequest = agentcore.ChatRequest
//	type ChatUsage = agentcore.ChatUsage
//	type ChatResponse = agentcore.ChatResponse
//	type TokenStream = agentcore.TokenStream
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/ports.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] ChatToolCall ← model_native_test.go；ChatRequest ← model_native_test.go, runtime_limits_test.go；ChatUsage ← model_native_test.go, runtime.go
//
// 主要符号
//
//	type ChatMessage = agentcore.ChatMessage
//	type ChatToolCall = agentcore.ChatToolCall
//	type ChatRequest = agentcore.ChatRequest
//	type ChatUsage = agentcore.ChatUsage
//	type ChatResponse = agentcore.ChatResponse
//	type TokenStream = agentcore.TokenStream
//	type StreamDelta = agentcore.StreamDelta
//	type ToolCallDelta = agentcore.ToolCallDelta
//	... 另有 9 个顶层符号

package cloudagent

import (
	"context"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

type (
	ChatMessage   = agentcore.ChatMessage
	ChatToolCall  = agentcore.ChatToolCall
	ChatRequest   = agentcore.ChatRequest
	ChatUsage     = agentcore.ChatUsage
	ChatResponse  = agentcore.ChatResponse
	TokenStream   = agentcore.TokenStream
	StreamDelta   = agentcore.StreamDelta
	ToolCallDelta = agentcore.ToolCallDelta
	ChatClient    = agentcore.ChatClient
)

// Logger is the structured logging surface used by the runtime.
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

// ToolDecision describes the safety classification of a tool invocation.
type ToolDecision struct {
	ReadOnly    bool
	Destructive bool
}

// ConfirmationWaiter waits for a decision about a destructive tool call.
type ConfirmationWaiter interface {
	Wait(ctx context.Context, runID, callID, initiator string, timeout time.Duration) (approved bool, err error)
}

// ConfirmationPreparer can reserve a confirmation before its event is emitted.
type ConfirmationPreparer interface {
	Prepare(runID, callID, initiator string)
}

// RunPreparation contains everything needed to execute one agent turn.
type RunPreparation struct {
	Model       string
	ModelScope  string
	Instruction string
	Tools       []tool.BaseTool
	Notes       []string
	Cleanup     func()
	OnToolCall  func(callID string)
}

// RunProvider resolves model, instruction and host-specific tools for a run.
// RunRequest.Context is an opaque, provider-owned value.
type RunProvider interface {
	Prepare(ctx context.Context, req RunRequest, emit func(Event)) (RunPreparation, error)
}

// RunProviderFunc adapts a function to RunProvider.
type RunProviderFunc func(context.Context, RunRequest, func(Event)) (RunPreparation, error)

func (f RunProviderFunc) Prepare(ctx context.Context, req RunRequest, emit func(Event)) (RunPreparation, error) {
	return f(ctx, req, emit)
}
