// 文件说明：engine_state.go — 群聊引擎的状态迁移、恢复、失败收尾与内部辅助
//
// 用法：仅供同包入口、执行与控制路径调用。
// 调用架构：Engine → Store.Transition* → broker 状态帧。
// 主要符号：ensureExecuting、transitionTurn、transitionTask、failTurn、loadTask。

package group

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// ---------------------------------------------------------------------------
// 内部辅助
// ---------------------------------------------------------------------------

var (
	errNeedReplan  = errors.New("group: assistant requested replan")
	errReplanLimit = errors.New("group: replan disabled or budget exhausted")
)

// ensureExecuting 保证轮次有执行循环在跑（动作推进后）。
func (e *Engine) ensureExecuting(ctx context.Context, turn Turn) error {
	if at, active := e.reg.get(turn.ID); active {
		// 已在跑：等待中的人回复经 humanCh 唤醒。
		at.wake()
		return nil
	}
	g, err := e.Members.Group(ctx, turn.GroupID)
	if err != nil {
		e.failTurn(ctx, turn, "group_unavailable", err)
		return err
	}
	if err := e.Store.Claim(ctx, turn.ThreadID, turn.ID); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	at, err := e.reg.register(turn.ID, turn.ThreadID, cancel)
	if err != nil {
		cancel()
		_ = e.Store.Release(ctx, turn.ThreadID, turn.ID)
		return err
	}
	go e.execute(at, runCtx, turn, g)
	return nil
}

func (a *activeTurn) wake() {
	select {
	case a.humanCh <- struct{}{}:
	default:
	}
}

// transitionTurn 带前置校验的轮次迁移（迁移失败返回错误，不静默）。
func (e *Engine) transitionTurn(ctx context.Context, turnID, from, to, errorCode string) error {
	if err := CheckTurnTransition(from, to); err != nil {
		return err
	}
	if err := e.Store.TransitionTurn(ctx, turnID, from, to, errorCode); err != nil {
		return err
	}
	return e.publishTurnStatus(ctx, turnID, to)
}

func (e *Engine) reviveTurn(ctx context.Context, turnID, from, to string) error {
	if !CanTurnRevive(from, to) {
		return fmt.Errorf("%w: turn %s -> %s", ErrIllegalTransition, from, to)
	}
	if err := e.Store.TransitionTurn(ctx, turnID, from, to, ""); err != nil {
		return err
	}
	return e.publishTurnStatus(ctx, turnID, to)
}

func (e *Engine) publishTurnStatus(ctx context.Context, turnID, status string) error {
	_, err := e.brokerOf(turnID).publish(ctx, turnID, Event{Type: EventTurn, Data: turnJSON(status, 0)})
	return err
}

// transitionTask 带前置校验的任务迁移。
func (e *Engine) transitionTask(ctx context.Context, taskID, from, to, result, errorCode string) error {
	if err := CheckTaskTransition(from, to); err != nil {
		return err
	}
	return e.Store.TransitionTask(ctx, taskID, from, to, result, errorCode)
}

func (e *Engine) reviveTask(ctx context.Context, taskID, from string) error {
	if !CanTaskRevive(from, TaskPending) {
		return fmt.Errorf("%w: task %s -> %s", ErrIllegalTransition, from, TaskPending)
	}
	return e.Store.TransitionTask(ctx, taskID, from, TaskPending, "", "")
}

// failTurn 统一失败收尾（不覆盖终态，CB-BE-03）。
func (e *Engine) failTurn(ctx context.Context, turn Turn, code string, cause error) {
	if TerminalTurn(turn.Status) {
		return
	}
	from := turn.Status
	if from == "" {
		from = TurnPlanning
	}
	msg := code
	if cause != nil {
		msg = code + ": " + cause.Error()
	}
	if err := e.transitionTurn(ctx, turn.ID, from, TurnFailed, msg); err != nil {
		// 迁移失败（终态或冲突）：不再重试，交由调用方决定。
		return
	}
	_ = e.Store.Release(ctx, turn.ThreadID, turn.ID)
}

// loadTask 取单个任务（经轮次任务表）。
func (e *Engine) loadTask(ctx context.Context, taskID string) (Task, error) {
	// 任务 ID 形如 <turnID>-<ref>：按前缀取轮次再定位。
	turnID := taskID
	if i := strings.LastIndex(taskID, "-"); i > 0 {
		turnID = taskID[:i]
	}
	tasks, err := e.Store.LoadTasks(ctx, turnID)
	if err != nil {
		return Task{}, err
	}
	for _, t := range tasks {
		if t.ID == taskID {
			return t, nil
		}
	}
	return Task{}, fmt.Errorf("group: task %s not found", taskID)
}

// currentTurnStatus 推断轮次当前状态（执行循环内库即真相）。
func currentTurnStatus(turn Turn, tasks []Task) string {
	if turn.Status != "" {
		return turn.Status
	}
	return TurnRunning
}

func constraintsOf(turn Turn) plan.Constraints {
	return plan.Constraints{
		Members:        memberIDsOf(turn.Decision),
		HumanID:        humanIDOf(turn.Decision),
		Mentioned:      turn.Mentions,
		MentionedHuman: turn.MentionedHuman,
		Limits:         plan.DefaultLimits(),
	}
}

func memberIDsOf(d plan.Decision) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range d.Dispatches {
		if t.TargetKind == plan.TargetAgent && !seen[t.TargetID] {
			seen[t.TargetID] = true
			out = append(out, t.TargetID)
		}
	}
	return out
}

func humanIDOf(d plan.Decision) string {
	for _, t := range d.Dispatches {
		if t.TargetKind == plan.TargetHuman {
			return t.TargetID
		}
	}
	return ""
}

func maxReplans(g Group) int {
	if g.Policy.MaxReplans == 0 {
		return 2
	}
	return g.Policy.MaxReplans
}

func canReplan(turn Turn, g Group) bool {
	limit := maxReplans(g)
	return limit >= 0 && turn.Replans < limit
}

func indexTask(all []Task, t Task) int {
	for i, o := range all {
		if o.ID == t.ID {
			return i + 1
		}
	}
	return 0
}

func depRef(tasks []Task, ref string) string {
	// ref → 任务 ID（DependsOn 存 ref）。
	for _, t := range tasks {
		if t.Ref == ref {
			return t.ID
		}
	}
	return ref
}

func stepsOf(d plan.Decision, tasks []Task) []plan.Step {
	if len(d.Steps) == 0 {
		return nil
	}
	status := taskStatusIndex(tasks)
	out := append([]plan.Step(nil), d.Steps...)
	for i, s := range out {
		done, total := 0, 0
		for _, t := range d.Dispatches {
			if t.StepID != s.ID {
				continue
			}
			total++
			if status[t.Ref] == TaskCompleted {
				done++
			}
		}
		if total > 0 {
			switch {
			case done == total:
				out[i].Status = plan.StepDone
			case done > 0:
				out[i].Status = plan.StepInProgress
			}
		}
	}
	return out
}

func truncate(s string) string {
	return agentcore.Truncate(s, 2000)
}

// HistoryOf 返回轮次历史的副本（replan 上下文用）。
func (t Turn) HistoryOf() []plan.Turn { return append([]plan.Turn(nil), t.History...) }
