// 文件说明：engine_control.go — 群聊轮次的确认、取消、重试、恢复与订阅控制
//
// 用法：宿主调用 Engine 的导出控制方法推进轮次，执行由同包后台循环承接。
// 调用架构：控制动作 → Store 状态迁移 / registry → Engine.execute。
// 主要符号：ConfirmPlan、Cancel、RetryTask、ReplyHuman、Resume、RunHeadless、Subscribe。

package group

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// ---------------------------------------------------------------------------
// 动作：确认 / 取消 / 重试 / 人工回复 / 恢复
// ---------------------------------------------------------------------------

// ConfirmPlan 处理人工对计划的决定（批准 / 编辑后批准 / 拒绝）。
func (e *Engine) ConfirmPlan(ctx context.Context, turnID string, ap Approval) (Turn, error) {
	if e.reg.isClosed() {
		return Turn{}, ErrEngineClosed
	}
	turn, ok, err := e.Store.LoadTurn(ctx, turnID)
	if err != nil || !ok {
		return Turn{}, fmt.Errorf("group: turn %s not found", turnID)
	}
	if turn.Status != TurnAwaitingPlanConfirm {
		return turn, fmt.Errorf("%w: turn is %s, not awaiting_plan_confirm", ErrIllegalTransition, turn.Status)
	}
	if ap.ExpectedVersion != 0 && ap.ExpectedVersion != turn.Version {
		return turn, fmt.Errorf("%w: expected version %d, got %d", ErrIllegalTransition, ap.ExpectedVersion, turn.Version)
	}
	b := e.brokerOf(turnID)

	if !ap.Approve {
		if err := e.transitionTurn(ctx, turnID, TurnAwaitingPlanConfirm, TurnRejected, "plan_rejected"); err != nil {
			return turn, err
		}
		_, _ = b.publish(ctx, turnID, Event{Type: EventPlanConfirmation, Data: planConfirmJSON(ConfirmRejected, false)})
		_ = e.Store.SaveDecision(ctx, turnID, turn.Decision, ConfirmRejected)
		e.Store.Release(ctx, turn.ThreadID, turnID)
		b.finish(EndRejected)
		turn.Status = TurnRejected
		return turn, nil
	}

	d := turn.Decision
	edited := false
	if len(ap.Edits) > 0 {
		if d.Confirm.CanEdit() {
			if nd, derr := plan.ApplyEdits(d, ap.Edits); derr == nil {
				// 编辑后重新校验（不调模型，§13.3-6）。
				if verr := plan.Validate(nd, constraintsOf(turn)); verr != nil {
					return turn, fmt.Errorf("invalid_plan: %v", verr)
				}
				d = nd
				edited = true
			} else {
				return turn, fmt.Errorf("invalid_plan: %v", derr)
			}
		} else {
			return turn, fmt.Errorf("invalid_plan: plan is not editable")
		}
	}
	if err := e.transitionTurn(ctx, turnID, TurnAwaitingPlanConfirm, TurnRunning, ""); err != nil {
		return turn, err
	}
	turn.Status = TurnRunning
	if _, err := b.publish(ctx, turnID, Event{Type: EventPlanConfirmation, Data: planConfirmJSON(ConfirmApproved, edited)}); err != nil {
		return turn, err
	}
	if err := e.Store.SaveDecision(ctx, turnID, d, ConfirmApproved); err != nil {
		return turn, err
	}
	turn.Decision = d
	turn.ConfirmState = ConfirmApproved
	if err := e.launch(ctx, turn, d); err != nil {
		return turn, err
	}
	return turn, nil
}

// Cancel 取消轮次：本进程活动轮次经 registry cancel（执行循环收尾），
// 非本进程轮次直接落库收尾。
func (e *Engine) Cancel(ctx context.Context, turnID string) (Turn, error) {
	turn, ok, err := e.Store.LoadTurn(ctx, turnID)
	if err != nil || !ok {
		return Turn{}, fmt.Errorf("group: turn %s not found", turnID)
	}
	if TerminalTurn(turn.Status) {
		return turn, nil
	}
	if at, active := e.reg.get(turnID); active {
		at.cancel()
		// 等执行循环收尾（有界等待，避免卡死）。
		waitDone(at, cancelWait)
		turn, _, _ = e.Store.LoadTurn(ctx, turnID)
		return turn, nil
	}
	// 非活动轮次（重启后的僵尸）：直接终态化。
	b := e.brokerOf(turnID)
	e.cancelFromWithin(ctx, turn, b)
	turn, _, _ = e.Store.LoadTurn(ctx, turnID)
	return turn, nil
}

// RetryTask 重试失败任务：重置目标任务与未完成下游，轮次复活为 running。
func (e *Engine) RetryTask(ctx context.Context, taskID string) (Turn, error) {
	if e.reg.isClosed() {
		return Turn{}, ErrEngineClosed
	}
	task, err := e.loadTask(ctx, taskID)
	if err != nil {
		return Turn{}, err
	}
	turn, ok, err := e.Store.LoadTurn(ctx, task.TurnID)
	if err != nil || !ok {
		return Turn{}, fmt.Errorf("group: turn %s not found", task.TurnID)
	}
	if task.TargetKind == "human" {
		return turn, fmt.Errorf("group: human task is not retryable")
	}
	if !CanTaskRevive(task.Status, TaskPending) {
		return turn, fmt.Errorf("group: task %s is %s, not retryable", taskID, task.Status)
	}
	if TerminalTurn(turn.Status) && !CanTurnRevive(turn.Status, TurnRunning) {
		return turn, fmt.Errorf("%w: turn %s cannot resume", ErrIllegalTransition, turn.Status)
	}
	tasks, err := e.Store.LoadTasks(ctx, turn.ID)
	if err != nil {
		return Turn{}, err
	}
	// 重置子树（目标 + 依赖它的未完成下游；人工任务与已完成任务不动）。
	reset := map[string]bool{taskID: true}
	for changed := true; changed; {
		changed = false
		for _, t := range tasks {
			if reset[t.ID] || t.TargetKind == "human" || t.Status == TaskCompleted {
				continue
			}
			for _, dep := range t.DependsOn {
				if reset[depRef(tasks, dep)] {
					reset[t.ID] = true
					changed = true
					break
				}
			}
		}
	}
	for _, t := range tasks {
		if reset[t.ID] && t.Status != TaskPending && !CanTaskRevive(t.Status, TaskPending) {
			return turn, fmt.Errorf("%w: task %s is %s", ErrIllegalTransition, t.ID, t.Status)
		}
	}
	b := e.brokerOf(turn.ID)
	for _, t := range tasks {
		if !reset[t.ID] || t.Status == TaskPending {
			continue
		}
		if err := e.reviveTask(ctx, t.ID, t.Status); err != nil {
			return turn, err
		}
		t.Status = TaskPending
		if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: t.ID, AgentID: t.TargetID,
			Data: taskJSON(t, indexTask(tasks, t), len(tasks))}); err != nil {
			return turn, err
		}
	}
	// 轮次复活（显式动作允许终态 → running）。
	if TerminalTurn(turn.Status) {
		if err := e.reviveTurn(ctx, turn.ID, turn.Status, TurnRunning); err != nil {
			return turn, err
		}
		turn.Status = TurnRunning
	} else if turn.Status == TurnWaitingHuman {
		if err := e.transitionTurn(ctx, turn.ID, turn.Status, TurnRunning, ""); err != nil {
			return turn, err
		}
		turn.Status = TurnRunning
	}
	if err := e.ensureExecuting(ctx, turn); err != nil {
		return turn, err
	}
	return turn, nil
}

// ReplyHuman 回复人工待办：continue 继续推进，unable 将任务与轮次标记失败。
func (e *Engine) ReplyHuman(ctx context.Context, taskID string, reply Reply) (Turn, error) {
	if e.reg.isClosed() {
		return Turn{}, ErrEngineClosed
	}
	task, err := e.loadTask(ctx, taskID)
	if err != nil {
		return Turn{}, err
	}
	if task.Status != TaskWaitingHuman {
		return Turn{}, fmt.Errorf("group: task %s is %s, not waiting_human", taskID, task.Status)
	}
	turn, ok, err := e.Store.LoadTurn(ctx, task.TurnID)
	if err != nil || !ok {
		return Turn{}, fmt.Errorf("group: turn %s not found", task.TurnID)
	}
	outcome := strings.TrimSpace(reply.Outcome)
	if outcome == "" {
		outcome = ReplyContinue
	}
	if outcome != ReplyContinue && outcome != ReplyUnable {
		return turn, fmt.Errorf("group: invalid human reply outcome %q", outcome)
	}
	to, errorCode := TaskCompleted, ""
	if outcome == ReplyUnable {
		to, errorCode = TaskFailed, "human_unable"
	}
	if err := e.transitionTask(ctx, taskID, TaskWaitingHuman, to, agentcore.Truncate(reply.Text, maxGroupContentChars), errorCode); err != nil {
		return turn, err
	}
	b := e.brokerOf(turn.ID)
	task.Status = to
	task.Result = agentcore.Truncate(reply.Text, maxGroupContentChars)
	task.ErrorCode = errorCode
	if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: taskID,
		Data: taskJSON(task, indexTask([]Task{task}, task), 1)}); err != nil {
		return turn, err
	}
	if _, err := b.publish(ctx, turn.ID, Event{Type: EventHuman, TaskID: taskID,
		Data: humanJSON("", agentcore.Truncate(reply.Text, maxGroupContentChars))}); err != nil {
		return turn, err
	}
	if turn.Status == TurnWaitingHuman {
		nextTurn := TurnRunning
		if outcome == ReplyUnable {
			nextTurn = TurnFailed
		}
		if err := e.transitionTurn(ctx, turn.ID, TurnWaitingHuman, nextTurn, errorCode); err != nil {
			return turn, err
		}
		turn.Status = nextTurn
	}
	if err := e.ensureExecuting(ctx, turn); err != nil {
		return turn, err
	}
	return turn, nil
}

// Resume 恢复中断轮次（interrupted/blocked 任务重新排队）。
func (e *Engine) Resume(ctx context.Context, turnID string) (Turn, error) {
	if e.reg.isClosed() {
		return Turn{}, ErrEngineClosed
	}
	turn, ok, err := e.Store.LoadTurn(ctx, turnID)
	if err != nil || !ok {
		return Turn{}, fmt.Errorf("group: turn %s not found", turnID)
	}
	if TerminalTurn(turn.Status) && turn.Status != TurnInterrupted {
		return turn, nil
	}
	tasks, err := e.Store.LoadTasks(ctx, turnID)
	if err != nil {
		return Turn{}, err
	}
	b := e.brokerOf(turnID)
	for _, t := range tasks {
		if t.TargetKind == "human" {
			continue
		}
		if t.Status == TaskInterrupted || t.Status == TaskBlocked {
			if err := e.reviveTask(ctx, t.ID, t.Status); err == nil {
				_, _ = b.publish(ctx, turnID, Event{Type: EventTask, TaskID: t.ID, AgentID: t.TargetID,
					Data: taskJSON(Task{ID: t.ID, Status: TaskPending, TargetID: t.TargetID}, 0, len(tasks))})
			}
		}
	}
	if turn.Status == TurnInterrupted {
		if err := e.reviveTurn(ctx, turnID, TurnInterrupted, TurnRunning); err == nil {
			turn.Status = TurnRunning
		}
	}
	e.ensureExecuting(ctx, turn)
	return turn, nil
}

// RunHeadless 同步执行一轮（cron / 告警）：不弹确认（需要确认按策略拒绝并记录），
// 无人时人类任务转为待办后返回。
func (e *Engine) RunHeadless(ctx context.Context, in TurnInput) (TurnResult, error) {
	turn, err := e.Submit(ctx, in)
	if err != nil {
		return TurnResult{}, err
	}
	// headless 下需要确认：拒绝并记录（§13.14 已拍板）。
	if turn.Status == TurnAwaitingPlanConfirm {
		turn, _ = e.ConfirmPlan(ctx, turn.ID, Approval{Approve: false})
		return TurnResult{TurnID: turn.ID, Status: TurnRejected, Content: "headless: plan confirmation denied"}, nil
	}
	for {
		final, _, loadErr := e.Store.LoadTurn(context.WithoutCancel(ctx), turn.ID)
		if loadErr == nil && (TerminalTurn(final.Status) || final.Status == TurnWaitingHuman) {
			return e.headlessResult(context.WithoutCancel(ctx), final), nil
		}
		at, active := e.reg.get(turn.ID)
		if !active {
			return e.headlessResult(context.WithoutCancel(ctx), final), nil
		}
		select {
		case <-at.done:
		case <-ctx.Done():
			e.reg.cancelTurn(turn.ID)
			// 等执行循环收尾落库（有界）：调用方（如 cron 调度器）拿到 ctx.Err() 返回时，
			// 轮次已是终态，不会在宿主关闭资源之后仍有写入。
			waitDone(at, cancelWait)
			return TurnResult{TurnID: turn.ID, Status: TurnCanceled}, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// cancelWait 是取消后等待执行循环收尾的上限（与用户 Cancel 同口径）。
const cancelWait = 5 * time.Second

// waitDone 等执行循环结束，最多等 timeout。
func waitDone(at *activeTurn, timeout time.Duration) {
	select {
	case <-at.done:
	case <-time.After(timeout):
	}
}

// BeginShutdown 进入停机（幂等、非阻塞）：之后的 Submit 返回 ErrEngineClosed，动作
// （ConfirmPlan / RetryTask / ReplyHuman / Resume）也不再重启执行循环；已登记的轮次被取消，
// 由各自的执行循环按取消语义收尾（轮次 canceled、running 任务 interrupted、其余任务 canceled），
// 与用户 Cancel 相同。停在 waiting_human / awaiting_plan_confirm 的轮次没有执行循环，不受影响，
// 重启后仍由 Converge 收敛。宿主随后用 WaitIdle 等全部收尾落库。
func (e *Engine) BeginShutdown() { e.reg.close() }

// WaitIdle 等待全部执行循环退出（轮次表清空即收尾写库已完成）；没有活动轮次时立即返回。
// 超时返回 ctx.Err()，调用方可用 ActiveTurns 列出残留轮次。
func (e *Engine) WaitIdle(ctx context.Context) error {
	for {
		ch := e.reg.idleCh()
		if ch == nil {
			return nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ActiveTurns 返回仍有执行循环的轮次 ID（停机超时时的诊断用）。
func (e *Engine) ActiveTurns() []string { return e.reg.active() }

func (e *Engine) headlessResult(ctx context.Context, turn Turn) TurnResult {
	content := turn.Status
	tasks, _ := e.Store.LoadTasks(ctx, turn.ID)
	for _, t := range tasks {
		if t.Status == TaskCompleted && t.Result != "" {
			content = t.Result
		}
	}
	return TurnResult{TurnID: turn.ID, Status: turn.Status, Content: agentcore.Truncate(content, maxGroupContentChars)}
}

// Converge 把本进程不再活动但库内未终态的轮次收敛为 interrupted（重启恢复）。
func (e *Engine) Converge(ctx context.Context) error {
	turns, err := e.Store.ListUnfinishedTurns(ctx)
	if err != nil {
		return err
	}
	for _, turn := range turns {
		if _, active := e.reg.get(turn.ID); active {
			continue
		}
		tasks, err := e.Store.LoadTasks(ctx, turn.ID)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			if TerminalTask(t.Status) || t.Status == TaskWaitingHuman {
				continue
			}
			if err := e.transitionTask(ctx, t.ID, t.Status, TaskInterrupted, "", "process_restarted"); err != nil {
				return err
			}
		}
		if err := e.transitionTurn(ctx, turn.ID, turn.Status, TurnInterrupted, "process_restarted"); err != nil {
			return err
		}
		if err := e.Store.Release(ctx, turn.ThreadID, turn.ID); err != nil {
			return err
		}
	}
	return nil
}

// Subscribe 订阅轮次事件流（SSE 只是订阅者；after 为已收到的最大 seq）。
func (e *Engine) Subscribe(ctx context.Context, turnID string, after int) (*Subscription, error) {
	b := e.brokerOf(turnID)
	sub, err := b.subscribe(ctx, turnID, after)
	if err != nil {
		return nil, err
	}
	return &Subscription{sub: sub, broker: b}, nil
}

// Subscription 是一个轮次事件订阅。
type Subscription struct {
	sub    *subscription
	broker *broker
}

// Next 返回下一帧；ok=false 表示订阅关闭（已收 end 或流终止）。
func (s *Subscription) Next() (Event, bool) { return s.sub.next() }

// Close 关闭订阅（不取消轮次）。
func (s *Subscription) Close() { s.broker.unsubscribe(s.sub) }
