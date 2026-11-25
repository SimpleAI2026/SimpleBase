// 文件说明：errors.go — Agent 协作上下文的领域错误（入口经 internal/api 的错误协议映射）
//
// 用法
//
//	var ErrStopping                      // 服务已进入停机：拒绝新建 run / 轮次 / 群定时（503 service_stopping）
//	var ErrRunContentRequired …          // 固定助手会话 run 的请求类错误（400 / 404 / 409 / 503，见映射表）
//	var ErrGroupArchived …               // 协作群领域错误（自 internal/api 迁入）
//	type InvalidError struct {…}         // 文案由底层错误决定的 400 invalid_request
//
// 调用架构
//
//	[上游] internal/api（哨兵映射）、internal/agentrun 各用例
//	         │ 使用本包导出符号
//	         ▼
//	[本文件] internal/agentrun/errors.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] run.go, turn.go, cron.go
//
// 主要符号
//
//	var ErrStopping
//	var ErrRunContentRequired
//	var ErrQuotaExceeded
//	type InvalidError struct {…}
//	... 另有 15 个顶层符号
//
// 错误码与文案与迁移前逐字一致（internal-ddd-hexagonal-implementation-plan §2 行为契约）；
// 应用服务只返回领域错误，不认识 HTTP。

package agentrun

import (
	"errors"
	"fmt"
)

// ErrStopping：服务已进入停机（BeginShutdown 之后），不再接受新的 run / 轮次 / 群定时。
var ErrStopping = errors.New("agent runs are shutting down")

// 固定助手会话 run 的请求类错误。
var (
	ErrRunContentRequired      = errors.New("content is required")
	ErrInvalidRetry            = errors.New("invalid_retry")
	ErrMultipleAgents          = errors.New("mention only one agent per run")
	ErrAgentSwitchRequired     = errors.New("switch to the mentioned agent before sending")
	ErrCLISkillsUnavailable    = errors.New("this agent requires CLI skills")
	ErrAgentSandboxUnavailable = errors.New("sandbox is unavailable")
	ErrThreadNotFound          = errors.New("thread not found")
	ErrRunNotFound             = errors.New("run not found")
	ErrConfirmationNotFound    = errors.New("confirmation not found")
	ErrQuotaExceeded           = errors.New("agent run quota exceeded")
)

// ErrTurnsRequired：群会话的消息必须走 POST /agent-threads/:id/turns（Plan 栅栏），
// /runs 只服务固定助手会话（planv6.8 §13.15 CB-BE-13）。
var ErrTurnsRequired = errors.New("group threads accept messages through /turns only")

// 协作群的领域错误（planv6.7 v2；统一进入口的哨兵映射）。
var (
	// ErrWorkspaceUpgradeRequired：协作表尚未建好（未升级环境），需要用户执行建表。
	ErrWorkspaceUpgradeRequired = errors.New("collaboration schema is not ready")
	// ErrGroupArchived：群已归档，拒绝新发送与新调度。
	ErrGroupArchived = errors.New("this group has been archived")
	// ErrGroupMemberUnavailable：目标助手已归档或不在群成员中。
	ErrGroupMemberUnavailable = errors.New("the target agent is not an available member of this group")
	// ErrTurnBusy：同一会话已有活动轮次。
	ErrTurnBusy = errors.New("this thread already has an active turn")
	// ErrTurnConflict：轮次/任务版本冲突（并发推进）。
	ErrTurnConflict = errors.New("agent turn was modified, reload and retry")
	// ErrRoutingFailed：路由器超时、输出越界或无法给出可用分工。
	ErrRoutingFailed = errors.New("routing failed")
	// ErrInvalidDispatchGraph：任务图不合法（越界、环、深度、数量）。
	ErrInvalidDispatchGraph = errors.New("invalid dispatch graph")
	// ErrHumanAssigneeRequired：需要人但没有可指派的人（API Key 主体或无人负责人）。
	ErrHumanAssigneeRequired = errors.New("a human assignee is required")
	// ErrHumanTaskForbidden：非接收人（且非项目管理员）处理他人待办。
	ErrHumanTaskForbidden = errors.New("only the assignee or a project admin can resolve this task")
	// ErrTaskNotRetryable：任务状态不允许重试。
	ErrTaskNotRetryable = errors.New("this task cannot be retried")
	// ErrCollaborationBudgetExceeded：超出协作预算（任务/交接次数）。
	ErrCollaborationBudgetExceeded = errors.New("collaboration budget exceeded")
)

// InvalidError 是「请求不合法」且文案由底层错误决定的情形：
// 迁移前写作 echo.NewHTTPError(400, err.Error())，入口把它映射为 400 invalid_request + 原文案。
type InvalidError struct{ Message string }

func (e *InvalidError) Error() string { return e.Message }

func invalidf(format string, args ...any) error {
	return &InvalidError{Message: fmt.Sprintf(format, args...)}
}
