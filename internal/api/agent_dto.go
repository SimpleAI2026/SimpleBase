// 文件说明：agent_dto.go — agent 相关 DTO 与领域对象到 DTO 的转换函数
//
// 用法
//
//	agentDTO、threadDTO、messageDTO（包内使用，由同包 handler / 路由装配调用）
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/agent_dto.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, godatabase, internal/agentbridge, internal/auth 等
//	[同包] adapter.go, api_key_policy.go, api_keys_handler.go, auth_handler.go 等 59 个
//
// 主要符号
//
//	type agentDTO struct {…}
//	type threadDTO struct {…}
//	type messageDTO struct {…}
//	type runDTO struct {…}
//	func toAgentDTO(a systemdb.CloudAgent) agentDTO
//	func toThreadDTO(t systemdb.AgentThread) threadDTO
//	func toThreadDTOWithWorkspace(t systemdb.AgentThread, ws systemdb.AgentThreadWorkspace, threadKind string) threadDTO
//	... 另有 6 个顶层符号

package api

import (
	"encoding/json"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

type agentDTO struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Module        string    `json:"module"`
	Description   string    `json:"description"`
	SystemPrompt  string    `json:"system_prompt"`
	ToolIDs       []string  `json:"tool_ids"`
	ModelOverride string    `json:"model_override,omitempty"`
	BuiltinKey    string    `json:"builtin_key,omitempty"`
	TeamEnabled   bool      `json:"team_enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type threadDTO struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	AgentID            string    `json:"agent_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	LastMessagePreview string    `json:"last_message_preview,omitempty"`
	// TagID 是工作台标签归属（planv6.7 AGC-NAV）；#对话 与旧会话为空。
	TagID string `json:"tag_id,omitempty"`
	// DispatchMode 为 "collaborative" 表示群会话（消息经 Plan 栅栏）；空为固定助手会话。
	DispatchMode string `json:"dispatch_mode,omitempty"`
	// ThreadKind 是会话类型（chat/alert/statistics/scheduled，v2 §5.3）；旧会话为空。
	ThreadKind string `json:"thread_kind,omitempty"`
}

type messageDTO struct {
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	AgentID   string          `json:"agent_id,omitempty"`
	Mentions  json.RawMessage `json:"mentions,omitempty"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	// RunStatus / ErrorCode 来自所属 run（BUG-06）：刷新后还原「已停止 / 失败」。
	RunStatus string    `json:"run_status,omitempty"`
	ErrorCode string    `json:"error_code,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type runDTO struct {
	ID               string     `json:"id"`
	ThreadID         string     `json:"thread_id"`
	AgentID          string     `json:"agent_id"`
	Status           string     `json:"status"`
	Error            string     `json:"error,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	DurationMS       int64      `json:"duration_ms"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	ReasoningTokens  int        `json:"reasoning_tokens"`
	ToolCalls        int        `json:"tool_calls"`
	ErrorCode        string     `json:"error_code,omitempty"`
}

func toAgentDTO(a systemdb.CloudAgent) agentDTO {
	ids := a.ToolIDs
	if ids == nil {
		ids = []string{}
	}
	return agentDTO{
		ID: a.ID, Name: a.Name, Module: a.Module, Description: a.Description,
		SystemPrompt: a.SystemPrompt, ToolIDs: ids, ModelOverride: a.ModelOverride,
		BuiltinKey: a.BuiltinKey, TeamEnabled: a.TeamEnabled,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func toThreadDTO(t systemdb.AgentThread) threadDTO {
	return threadDTO{ID: t.ID, Title: t.Title, AgentID: t.AgentID, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

// toThreadDTOWithWorkspace 在 threadDTO 基础上补工作台元数据（标签、调度模式、会话类型）。
// 一轮一助手的 "auto" 调度模式已下线：群会话只有 collaborative 一种写法，不再做读写折叠。
func toThreadDTOWithWorkspace(t systemdb.AgentThread, ws systemdb.AgentThreadWorkspace, threadKind string) threadDTO {
	dto := toThreadDTO(t)
	dto.TagID = ws.TagID
	dto.DispatchMode = ws.DispatchMode
	dto.ThreadKind = threadKind
	return dto
}

func toMessageDTO(m systemdb.AgentMessage) messageDTO {
	mentions := json.RawMessage(m.MentionsJSON)
	if len(mentions) == 0 {
		mentions = json.RawMessage("[]")
	}
	tools := json.RawMessage(m.ToolCallsJSON)
	if len(tools) == 0 {
		tools = json.RawMessage("[]")
	}
	return messageDTO{
		ID: m.ID, Role: m.Role, Content: m.Content, AgentID: m.AgentID,
		Mentions: mentions, ToolCalls: tools, RunID: m.RunID, CreatedAt: m.CreatedAt,
		RunStatus: m.RunStatus, ErrorCode: m.ErrorCode,
	}
}

// activeRunDTO 是「正在回复」指示所需的最小元数据，不含正文。
type activeRunDTO struct {
	RunID     string     `json:"run_id"`
	AgentID   string     `json:"agent_id"`
	ThreadID  string     `json:"thread_id"`
	StartedAt *time.Time `json:"started_at,omitempty"`
}

type approvalDTO struct {
	ID          string     `json:"id"`
	RunID       string     `json:"run_id"`
	CallID      string     `json:"call_id"`
	ThreadID    string     `json:"thread_id,omitempty"`
	AgentID     string     `json:"agent_id,omitempty"`
	ToolName    string     `json:"tool_name,omitempty"`
	Argv        []string   `json:"argv"`
	Message     string     `json:"message,omitempty"`
	Status      string     `json:"status"`
	InitiatorID string     `json:"initiator_id,omitempty"`
	ResolvedBy  string     `json:"resolved_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

func toApprovalDTO(a systemdb.AgentApproval) approvalDTO {
	argv := []string{}
	if a.ArgvJSON != "" {
		_ = json.Unmarshal([]byte(a.ArgvJSON), &argv)
	}
	d := approvalDTO{
		ID: a.ID, RunID: a.RunID, CallID: a.CallID, ThreadID: a.ThreadID, AgentID: a.AgentID,
		ToolName: a.ToolName, Argv: argv, Message: a.Message, Status: a.Status,
		InitiatorID: a.InitiatorID, ResolvedBy: a.ResolvedBy, CreatedAt: a.CreatedAt,
	}
	if !a.ExpiresAt.IsZero() {
		t := a.ExpiresAt
		d.ExpiresAt = &t
	}
	if !a.ResolvedAt.IsZero() {
		t := a.ResolvedAt
		d.ResolvedAt = &t
	}
	return d
}

// agentModelDTO 是 /agents/models 返回的单个模型（只含名字，无 base_url/key）。
type agentModelDTO struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

func toRunDTO(r systemdb.AgentRun) runDTO {
	dto := runDTO{ID: r.ID, ThreadID: r.ThreadID, AgentID: r.AgentID, Status: r.Status, Error: r.Error,
		DurationMS: r.DurationMS, PromptTokens: r.PromptTokens, CompletionTokens: r.CompletionTokens,
		ReasoningTokens: r.ReasoningTokens, ToolCalls: r.ToolCalls, ErrorCode: r.ErrorCode}
	if !r.StartedAt.IsZero() {
		t := r.StartedAt
		dto.StartedAt = &t
	}
	if !r.FinishedAt.IsZero() {
		t := r.FinishedAt
		dto.FinishedAt = &t
	}
	return dto
}
