// 文件说明：types.go — 群聊引擎的数据结构与状态常量（planv6.8 §13.8）
//
// group 包是 CLI 与云端共用的群聊运行时：栅栏（fence）产出 plan.Decision，
// 执行器按依赖调度，事件经 broker 广播与落库。SimpleBase 适配（systemdb 存储、
// SSE、身份）在宿主侧实现 ports.go 的端口。
//
// 状态机（§13.8）：
//
//	Turn:  planning → awaiting_plan_confirm → running ⇄ waiting_human
//	                                    → completed | failed | canceled | interrupted
//	       planning → rejected（拒绝计划，终态，不执行任何任务）
//	Task:  pending → running → completed | failed ; pending → blocked
//	       human: waiting_human → completed | failed(unable) ; * → canceled ; running → interrupted
//
// 依赖方向：group → plan / agentcore；不 import cloudagent、cli、宿主包。

package group

import (
	"context"
	"encoding/json"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// 轮次状态。
const (
	TurnPlanning            = "planning" // 取代旧 routing（§13.10，激进替换）
	TurnAwaitingPlanConfirm = "awaiting_plan_confirm"
	TurnRunning             = "running"
	TurnWaitingHuman        = "waiting_human"
	TurnCompleted           = "completed"
	TurnFailed              = "failed"
	TurnCanceled            = "canceled"
	TurnRejected            = "rejected"
	TurnInterrupted         = "interrupted"
)

// 任务状态。
const (
	TaskPending      = "pending"
	TaskRunning      = "running"
	TaskWaitingHuman = "waiting_human"
	TaskCompleted    = "completed"
	TaskFailed       = "failed"
	TaskBlocked      = "blocked"
	TaskCanceled     = "canceled"
	TaskInterrupted  = "interrupted"
)

// 事件帧类型（SSE v3，§13.10）。
const (
	EventTurn                 = "turn"
	EventPlan                 = "plan"
	EventPlanConfirmation     = "plan_confirmation"
	EventProgress             = "progress"
	EventTask                 = "task"
	EventHuman                = "human"
	EventConfirmationRequired = "confirmation_required"
	EventConfirmationResolved = "confirmation_resolved"
	EventEnd                  = "end"
)

// 确认状态（plan_confirmation 帧）。
const (
	ConfirmRequired = "required"
	ConfirmApproved = "approved"
	ConfirmRejected = "rejected"
)

// 结束原因（end 帧 reason）。
const (
	EndDone     = "done"
	EndFailed   = "failed"
	EndCanceled = "canceled"
	EndRejected = "rejected"
)

// Group 是一次群会话的静态描述（宿主从配置加载）。
type Group struct {
	ID        string
	MemberIDs []string
	// HumanID 非空表示可指派当前人；HumanDisabled 表示群策略禁用人工。
	HumanID       string
	HumanName     string
	HumanDisabled bool
	// Policy 是群策略。
	Policy Policy
}

// Policy 是群级策略。
type Policy struct {
	// Confirm 是确认策略（只能加严，不能放宽 destructive）。
	Confirm plan.ConfirmPolicy
	// FallbackAgent 是模型不可用时的兜底助手；空表示无兜底。
	FallbackAgent string
	// MaxReplans 是每轮最大再规划次数；0 取 2，负数关闭。
	MaxReplans int
	// Concurrency 是无依赖任务的并发数；0 取 1（§13.13：默认关闭并发）。
	Concurrency int
	// NeedSummary 默认值由决策 need_summary 决定；这里只做覆盖（空字符串跟随决策）。
}

// Turn 是一轮群聊。
type Turn struct {
	ID       string
	ThreadID string
	GroupID  string
	Status   string
	Version  int // 乐观锁版本号
	// Initiator 是发起者（user:<id> / cron:<name> 等）。
	Initiator      string
	IdempotencyKey string
	// UserText 是本轮用户输入（@ 前缀已剥离为 Mentions）。
	UserText       string
	Mentions       []string
	MentionedHuman bool
	History        []plan.Turn
	RoutingPrompt  string
	// Decision 是当前栅栏决策（route_json v3）。
	Decision plan.Decision
	// ConfirmState 是计划确认状态。
	ConfirmState string
	// Replans 是已用掉的再规划次数。
	Replans int
	// NeedSummary 是否已产出总结。
	Summarized bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Task 是决策里的一个 dispatch 的执行实例。
type Task struct {
	ID         string
	TurnID     string
	Ref        string
	TargetKind string // agent | human
	TargetID   string
	Message    string
	Status     string
	Version    int
	Attempt    int
	// DependsOn / ContextFrom 引用同轮更早的 ref。
	DependsOn      []string
	ContextFrom    []string
	Risk           string
	StepID         string
	ExpectedOutput string
	// RunID 关联的子 run（前端内容二级续接，§13.10）。
	RunID string
	// Result 是完成后的内容摘要（截断）。
	Result    string
	ErrorCode string
}

// Event 是一条轮次事件（结构帧；内容帧走 run 事件缓存）。
// 形状对齐 SSE v3：{type, seq, turn_id, task_id?, run_id?, agent_id?, ...}。
type Event struct {
	Type    string          `json:"type"`
	Seq     int             `json:"seq"`
	TurnID  string          `json:"turn_id"`
	TaskID  string          `json:"task_id,omitempty"`
	RunID   string          `json:"run_id,omitempty"`
	AgentID string          `json:"agent_id,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// TurnInput 是 Submit 的输入。
type TurnInput struct {
	ThreadID string
	GroupID  string
	// Initiator 发起者主体串。
	Initiator string
	// UserText 用户消息正文（@ 前缀已剥离）。
	UserText string
	// Mentions 显式 @ 的成员 ID（硬约束）。
	Mentions []string
	// MentionedHuman 显式 @ 了人。
	MentionedHuman bool
	// History 是近期群聊历史（投影后的通用形状）。
	History []plan.Turn
	// IdempotencyKey 幂等键（可选）。
	IdempotencyKey string
	// RoutingPrompt 群路由提示词（用户数据）。
	RoutingPrompt string
	// BeforeFence 在新建轮次后、栅栏前执行一次宿主落库钩子；幂等复用不调用。
	BeforeFence func(context.Context, Turn) error
}

// Approval 是人工对计划的决定。
type Approval struct {
	Approve bool
	// ExpectedVersion 非零时要求轮次乐观锁版本一致。
	ExpectedVersion int
	// Edits 编辑后批准：改 message / 删除 dispatch。
	Edits []plan.Edit
}

// Reply 是人工待办回复；Outcome 为 continue / unable。
type Reply struct {
	Text    string
	Outcome string
}

const (
	ReplyContinue = "continue"
	ReplyUnable   = "unable"
)

// TurnResult 是同步执行（RunHeadless）的产物。
type TurnResult struct {
	TurnID  string
	Status  string
	Content string // 总结或最后一条结果
}
