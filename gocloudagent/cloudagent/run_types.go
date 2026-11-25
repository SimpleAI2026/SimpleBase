// 文件说明：run_types.go — gocloudagent/cloudagent 中 Event、EventStep 等符号的实现
//
// 用法
//
//	type Event struct{…}  // Event is a streaming run event for SSE/NDJSON.
//	type EventStep struct{…}  // EventStep is one agent or human step in a collaboration route.
//	type RunRequest struct{…}  // RunRequest starts a single-agent turn. Context is reserved for the configured Ru...
//	type RunResult struct{…}  // RunResult is the completed assistant turn.
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/cloudagent/run_types.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] callgate.go, callgate_test.go, confirmation.go, confirmation_test.go, errors.go 等 32 个
//	[关键引用] Event ← confirmation.go, confirmation_test.go；RunRequest ← mcpaccess.go, mcptool.go；RunResult ← runtime.go, trace.go
//
// 主要符号
//
//	type Event struct{…}
//	type EventStep struct{…}
//	type RunRequest struct{…}
//	type RunResult struct{…}

package cloudagent

import "encoding/json"

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
	// Decision 是 plan 工具记录的结构化计划（v3 `plan` 帧的 decision 字段）：宿主的规划工具
	// 经 Emit 下发，Runtime 不解释其内容。群轮次由 group 的 broker 承载，不走这里。
	Decision json.RawMessage `json:"decision,omitempty"`
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
