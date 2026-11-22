package cloudagent

// Event is a streaming run event for SSE/NDJSON.
type Event struct {
	Type             string      `json:"type"`
	Content          string      `json:"content,omitempty"`
	Name             string      `json:"name,omitempty"`
	Arguments        string      `json:"arguments,omitempty"`
	CallID           string      `json:"call_id,omitempty"`
	RunID            string      `json:"run_id,omitempty"`
	Message          string      `json:"message,omitempty"`
	Code             string      `json:"code,omitempty"`
	Reason           string      `json:"reason,omitempty"`
	ElapsedMS        int64       `json:"elapsed_ms,omitempty"`
	DurationMS       int64       `json:"duration_ms,omitempty"`
	PromptTokens     int         `json:"prompt_tokens,omitempty"`
	CompletionTokens int         `json:"completion_tokens,omitempty"`
	ReasoningTokens  int         `json:"reasoning_tokens,omitempty"`
	ToolCalls        int         `json:"tool_calls,omitempty"`
	IsError          bool        `json:"is_error,omitempty"`
	Truncated        bool        `json:"truncated,omitempty"`
	Argv             []string    `json:"argv,omitempty"`
	Approve          *bool       `json:"approve,omitempty"`
	AgentID          string      `json:"agent_id,omitempty"`
	RouteKind        string      `json:"route_kind,omitempty"`
	Status           string      `json:"status,omitempty"`
	Steps            []EventStep `json:"steps,omitempty"`
	Index            int         `json:"index,omitempty"`
	Total            int         `json:"total,omitempty"`
	HumanID          string      `json:"human_id,omitempty"`
	HumanName        string      `json:"human_name,omitempty"`
	TaskID           string      `json:"task_id,omitempty"`
}

// EventStep is one agent or human step in a collaboration route.
type EventStep struct {
	Kind        string `json:"kind"`
	AgentID     string `json:"agent_id,omitempty"`
	Instruction string `json:"instruction,omitempty"`
	RouteKind   string `json:"route_kind,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
}

// RunRequest starts a single-agent turn. Context is reserved for the configured
// RunProvider and MCP connector; the core runtime treats it as opaque.
type RunRequest struct {
	Agent       Agent
	ThreadID    string
	RunID       string
	UserText    string
	History     []Message
	Stream      bool
	Context     any
	Headless    bool
	Subject     string
	Instruction string
}

// RunResult is the completed assistant turn.
type RunResult struct {
	Content          string
	ToolCallsJSON    string
	DurationMS       int64
	PromptTokens     int
	CompletionTokens int
	ReasoningTokens  int
	ToolCalls        int
	Reason           string
}
