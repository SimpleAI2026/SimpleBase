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
