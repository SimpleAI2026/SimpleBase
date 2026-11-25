// 文件说明：types.go — gocloudagent/cloudagent 中 Agent、Message 等符号的实现
//
// 用法
//
//	type Agent struct{…}  // Agent describes the agent used for one run.
//	type Message struct{…}  // Message is one persisted conversation turn.
//	type RunContext struct{…}  // RunContext is available to tools while a run is executing. Value is owned by the...
//	WithRunContext(ctx context.Context, rc RunContext) context.Context  // WithRunContext injects a run context, primarily for custom tool runners.
//	RunContextFrom(ctx context.Context) (RunContext, bool)  // RunContextFrom returns the current run context visible to a tool.
//	同包调用：withRunContext() → runtime.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/types.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] Agent ← mcptool_test.go, run_types.go；Message ← history.go, model_native_test.go；RunContext ← mcptool.go, mcptool_test.go
//
// 主要符号
//
//	type Agent struct{…}
//	type Message struct{…}
//	type runContextKey struct{…}
//	type RunContext struct{…}
//	withRunContext(ctx context.Context, rc RunContext) context.Context
//	WithRunContext(ctx context.Context, rc RunContext) context.Context
//	RunContextFrom(ctx context.Context) (RunContext, bool)

package cloudagent

import "context"

// Agent describes the agent used for one run.
type Agent struct {
	ID            string
	Name          string
	Module        string
	Description   string
	SystemPrompt  string
	ToolIDs       []string
	ModelOverride string
	Writable      bool
}

// Message is one persisted conversation turn.
type Message struct {
	Role          string
	Content       string
	ToolCallsJSON string
}

type runContextKey struct{}

// RunContext is available to tools while a run is executing. Value is owned by
// the configured RunProvider and is never inspected by the runtime.
type RunContext struct {
	RunID    string
	ThreadID string
	Value    any
	Emit     func(Event)
}

func withRunContext(ctx context.Context, rc RunContext) context.Context {
	return context.WithValue(ctx, runContextKey{}, rc)
}

// WithRunContext injects a run context, primarily for custom tool runners.
func WithRunContext(ctx context.Context, rc RunContext) context.Context {
	return withRunContext(ctx, rc)
}

// RunContextFrom returns the current run context visible to a tool.
func RunContextFrom(ctx context.Context) (RunContext, bool) {
	rc, ok := ctx.Value(runContextKey{}).(RunContext)
	return rc, ok
}
