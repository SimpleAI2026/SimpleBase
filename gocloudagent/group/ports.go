// 文件说明：ports.go — 群引擎的宿主端口（planv6.8 §13.8）
//
// SimpleBase 实现这些端口（systemdb 持久化、cloudagent.Runtime 执行、
// 工作区成员解析）；CLI 用内存实现 + 文件存储。group 包自身不 import 上层。

package group

import (
	"context"

	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// Store 是轮次/任务/决策/事件的持久化端口。实现必须并发安全；
// 迁移方法（Transition*）以 from 为前置条件，非法迁移返回 ErrIllegalTransition
// （修复 CB-BE-03：终态不被覆盖；CB-BE-11：不再静默吞错）。
type Store interface {
	// CreateTurn 建轮次并落首条 turn 事件；幂等键命中时返回已有轮次与 reused=true。
	CreateTurn(ctx context.Context, t Turn) (Turn, bool, error)
	// TransitionTurn 状态迁移；from 不匹配（含版本冲突）返回 ErrIllegalTransition。
	TransitionTurn(ctx context.Context, turnID, from, to, errorCode string) error
	// SaveDecision 持久化决策（route_json v3：Decision + confirm 状态）。
	SaveDecision(ctx context.Context, turnID string, d plan.Decision, confirmState string) error
	// SaveReplans 持久化已消耗的再规划次数，保证重启后预算仍有效。
	SaveReplans(ctx context.Context, turnID string, replans int) error
	// LoadTurn 取轮次快照。
	LoadTurn(ctx context.Context, turnID string) (Turn, bool, error)
	// CreateTasks 按决策全量重建该轮任务（replan 复用：旧任务整组替换，
	// 已完成任务的 ID/结果由调用方保留在重建集内）。
	CreateTasks(ctx context.Context, turnID string, tasks []Task) error
	// TransitionTask 任务迁移；from 不匹配返回 ErrIllegalTransition。
	TransitionTask(ctx context.Context, taskID, from, to, result, errorCode string) error
	// SetTaskRunID 记录子 run 关联（前端内容二级续接，§13.10）。
	SetTaskRunID(ctx context.Context, taskID, runID string) error
	// LoadTasks 按轮次取全部任务。
	LoadTasks(ctx context.Context, turnID string) ([]Task, error)
	// AppendEvent 落一条结构帧并原子分配单调 seq（修复 CB-BE-04）。
	AppendEvent(ctx context.Context, turnID string, e Event) (Event, error)
	// ListEvents 返回 after 之后的事件（续接）。
	ListEvents(ctx context.Context, turnID string, after int) ([]Event, error)
	// Claim/Release 是会话级单轮占用（进程内部分见 registry）。
	Claim(ctx context.Context, threadID, turnID string) error
	Release(ctx context.Context, threadID, turnID string) error
	// ListUnfinishedTurns 返回非终态轮次（Converge 重启收敛用）。
	ListUnfinishedTurns(ctx context.Context) ([]Turn, error)
}

// Runner 执行单个助手任务（宿主桥到 cloudagent.Runtime.StartRun）。
type Runner interface {
	// RunTask 执行一个 dispatch；emit 把结构帧（如 confirmation_required）
	// 送进轮次流（引擎负责落库与广播）。ctx 取消即中止子 run。
	RunTask(ctx context.Context, in TaskInput, emit func(Event)) (TaskResult, error)
}

// TaskInput 是 Runner 的输入。
type TaskInput struct {
	TurnID  string
	TaskID  string
	Ref     string
	AgentID string
	// Message 是栅栏给该助手的独立指令（自包含）。
	Message string
	// Context 是 context_from 引用的上游结果（已截断、带 [来自 @X 的结果] 前缀）。
	Context string
	// History 是群聊历史投影。
	History []plan.Turn
	// Goal 是本轮目标（决策 goal）。
	Goal string
	// OnRunID 在子 run 建立后回调（落 task.run_id，供前端二级续接）。
	OnRunID func(runID string)
}

// TaskResult 是 Runner 的产物。
type TaskResult struct {
	Content string // 助手回复（宿主截断后）
	RunID   string
	// Failed 为真表示任务失败（能力不可用、运行错误）。
	Failed bool
	// ErrorCode 失败码（cli_skills_unavailable / sandbox_unavailable / run_failed…）。
	ErrorCode string
	// Canceled 表示子 run 被取消（不算失败）。
	Canceled bool
	// NeedReplan 为真表示助手主动请求再规划（plan action=replan）。
	NeedReplan bool
}

// Members 提供群配置与成员可用性。
type Members interface {
	// Group 返回群描述（成员、人、策略）。
	Group(ctx context.Context, groupID string) (Group, error)
	// Candidates 返回可 @ 的助手（Planner 可见的候选）。
	Candidates(ctx context.Context, groupID string) ([]plan.Candidate, error)
	// History 返回近期群聊历史投影（Submit 时未显式传入才调用）。
	History(ctx context.Context, threadID string, limit int) ([]plan.Turn, error)
}
