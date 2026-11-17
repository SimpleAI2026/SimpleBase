// 文件说明：run_types.go — Run 请求 / 结果 / 流事件类型

package cloudagent

import ()

// Event is a streaming run event for SSE/NDJSON.
type Event struct {
	Type             string   `json:"type"`
	Content          string   `json:"content,omitempty"`
	Name             string   `json:"name,omitempty"`
	Arguments        string   `json:"arguments,omitempty"`
	CallID           string   `json:"call_id,omitempty"`
	RunID            string   `json:"run_id,omitempty"`
	Message          string   `json:"message,omitempty"`
	Code             string   `json:"code,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	ElapsedMS        int64    `json:"elapsed_ms,omitempty"`
	DurationMS       int64    `json:"duration_ms,omitempty"`
	PromptTokens     int      `json:"prompt_tokens,omitempty"`
	CompletionTokens int      `json:"completion_tokens,omitempty"`
	ReasoningTokens  int      `json:"reasoning_tokens,omitempty"`
	ToolCalls        int      `json:"tool_calls,omitempty"`
	IsError          bool     `json:"is_error,omitempty"`
	Truncated        bool     `json:"truncated,omitempty"`
	Argv             []string `json:"argv,omitempty"`
	Approve          *bool    `json:"approve,omitempty"`
}

// RunRequest starts a single-agent turn.
type RunRequest struct {
	ProjectID string
	Principal Principal
	Agent     Agent
	ThreadID  string
	RunID     string
	UserText  string
	History   []Message
	Stream    bool
	// Skills 是本轮已校验的 skill id。Headless 为定时任务，不能确认破坏性命令。
	Skills   []string
	Headless bool
	// ReadOnlySkills 是 Skills 中只允许只读命令的子集（搭建会话的阶段限权）。
	// 由宿主按阶段白名单换算，不接受客户端输入。
	ReadOnlySkills []string
	// StarterStage 非空表示首页搭建会话的某个阶段（宿主已校验）：追加搭建提示词。
	StarterStage string
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
