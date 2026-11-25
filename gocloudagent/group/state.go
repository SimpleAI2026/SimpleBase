// 文件说明：state.go — Turn/Task 状态机与合法迁移表（planv6.8 §13.8，修复 CB-BE-03/11）
//
// 迁移集中在这里判定：所有状态变更必须经 CanTransition / Transition 校验，
// 非法迁移返回 ErrIllegalTransition 而不是静默覆盖（取消态不被失败回写覆盖、
// 并发动作不互相踩踏）。

package group

import "fmt"

// ErrIllegalTransition 表示一次非法状态迁移（前置状态不匹配或目标非法）。
var ErrIllegalTransition = fmt.Errorf("group: illegal state transition")

// turnTransitions 是轮次合法迁移表（终态无出边）。
var turnTransitions = map[string][]string{
	TurnPlanning:            {TurnAwaitingPlanConfirm, TurnRunning, TurnFailed, TurnCanceled, TurnInterrupted},
	TurnAwaitingPlanConfirm: {TurnRunning, TurnRejected, TurnFailed, TurnCanceled, TurnInterrupted},
	TurnRunning:             {TurnAwaitingPlanConfirm, TurnWaitingHuman, TurnCompleted, TurnFailed, TurnCanceled, TurnInterrupted},
	TurnWaitingHuman:        {TurnAwaitingPlanConfirm, TurnRunning, TurnCompleted, TurnFailed, TurnCanceled, TurnInterrupted},
}

// taskTransitions 是任务合法迁移表。
var taskTransitions = map[string][]string{
	TaskPending:      {TaskRunning, TaskWaitingHuman, TaskBlocked, TaskCanceled, TaskInterrupted},
	TaskBlocked:      {TaskPending, TaskCanceled, TaskInterrupted},
	TaskRunning:      {TaskCompleted, TaskFailed, TaskCanceled, TaskInterrupted},
	TaskWaitingHuman: {TaskCompleted, TaskFailed, TaskCanceled, TaskInterrupted},
}

// revive 迁移：仅限用户显式动作（RetryTask / Resume）使用，与引擎内部推进区分；
// 引擎自身的终局路径（settle/cancel）不会触发这些边（CB-BE-03 的取消不被覆盖）。
var turnReviveTransitions = map[string][]string{
	TurnFailed:       {TurnRunning},
	TurnCanceled:     {TurnRunning},
	TurnInterrupted:  {TurnRunning},
	TurnWaitingHuman: {TurnRunning},
}

var taskReviveTransitions = map[string][]string{
	TaskFailed:      {TaskPending},
	TaskInterrupted: {TaskPending},
	TaskBlocked:     {TaskPending},
	TaskCanceled:    {TaskPending},
}

// TerminalTurn 报告轮次是否终态。
func TerminalTurn(status string) bool {
	switch status {
	case TurnCompleted, TurnFailed, TurnCanceled, TurnRejected, TurnInterrupted:
		return true
	}
	return false
}

// TerminalTask 报告任务是否终态。
func TerminalTask(status string) bool {
	switch status {
	case TaskCompleted, TaskFailed, TaskBlocked, TaskCanceled, TaskInterrupted:
		return true
	}
	return false
}

// CanTurnTransition 报告 from→to 是否合法。
func CanTurnTransition(from, to string) bool {
	return contains(turnTransitions[from], to)
}

// CanTaskTransition 报告 from→to 是否合法。
func CanTaskTransition(from, to string) bool {
	return contains(taskTransitions[from], to)
}

// CanTurnRevive 报告显式复活（RetryTask / Resume）是否合法。
func CanTurnRevive(from, to string) bool {
	return contains(turnReviveTransitions[from], to)
}

// CanTaskRevive 报告任务重置（retry/resume 子树）是否合法。
func CanTaskRevive(from, to string) bool {
	return contains(taskReviveTransitions[from], to)
}

// CheckTurnTransition 校验并描述一次轮次迁移。
func CheckTurnTransition(from, to string) error {
	if TerminalTurn(from) {
		return fmt.Errorf("%w: turn is %s (terminal)", ErrIllegalTransition, from)
	}
	if !CanTurnTransition(from, to) {
		return fmt.Errorf("%w: turn %s -> %s", ErrIllegalTransition, from, to)
	}
	return nil
}

// CheckTaskTransition 校验并描述一次任务迁移。
func CheckTaskTransition(from, to string) error {
	if TerminalTask(from) {
		return fmt.Errorf("%w: task is %s (terminal)", ErrIllegalTransition, from)
	}
	if !CanTaskTransition(from, to) {
		return fmt.Errorf("%w: task %s -> %s", ErrIllegalTransition, from, to)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
