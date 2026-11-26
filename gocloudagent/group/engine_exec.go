// 文件说明：engine_exec.go — 群聊轮次的后台执行、单任务运行、再规划与总结
//
// 用法：由 engine.go 的 launch 或 engine_state.go 的 ensureExecuting 启动。
// 调用架构：Engine.execute → runOne / settle / replan → Store / Runner / broker。
// 主要符号：execute、step、runOne、settle、replan、summarize、cancelFromWithin。

package group

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// ---------------------------------------------------------------------------
// 后台执行循环
// ---------------------------------------------------------------------------

// execute 是单轮次的执行循环：串行推进就绪任务，直到终局。
func (e *Engine) execute(at *activeTurn, ctx context.Context, turn Turn, g Group) {
	b := e.brokerOf(turn.ID)
	defer func() {
		_ = e.Store.Release(context.WithoutCancel(ctx), turn.ThreadID, turn.ID)
		e.reg.release(turn.ID)
	}()
	for !e.step(at, ctx, b, &turn, g) {
	}
}

// step 推进一次执行循环；done=true 表示执行循环应退出。
func (e *Engine) step(at *activeTurn, ctx context.Context, b *broker, turn *Turn, g Group) (done bool) {
	if ctx.Err() != nil {
		e.cancelFromWithin(ctx, *turn, b)
		return true
	}
	current, ok, err := e.Store.LoadTurn(ctx, turn.ID)
	if err != nil {
		e.failTurn(ctx, *turn, "turn_store_failed", err)
		b.finish(EndFailed)
		return true
	}
	if ok {
		*turn = current
	}
	if TerminalTurn(turn.Status) {
		b.finish(endReasonOf(turn.Status))
		return true
	}
	if turn.Status == TurnAwaitingPlanConfirm {
		b.pause()
		return true
	}
	tasks, err := e.Store.LoadTasks(ctx, turn.ID)
	if err != nil {
		e.failTurn(ctx, *turn, "turn_store_failed", err)
		b.finish(EndFailed)
		return true
	}

	// 人工任务就绪 → waiting_human（等人不是错误，CB-BE 旧语义保留）。
	if ht, ok := humanReady(tasks); ok {
		if err := e.transitionTask(ctx, ht.ID, TaskPending, TaskWaitingHuman, "", ""); err != nil {
			e.failTurn(ctx, *turn, "turn_store_failed", err)
			b.finish(EndFailed)
			return true
		}
		ht.Status = TaskWaitingHuman
		if err := e.transitionTurn(ctx, turn.ID, turn.Status, TurnWaitingHuman, ""); err != nil {
			e.failTurn(ctx, *turn, "turn_store_failed", err)
			b.finish(EndFailed)
			return true
		}
		turn.Status = TurnWaitingHuman
		if _, err := b.publish(ctx, turn.ID, Event{Type: EventHuman, TaskID: ht.ID,
			Data: humanJSON(g.HumanName, ht.Message, g.HumanID)}); err != nil {
			e.failTurn(ctx, *turn, "turn_store_failed", err)
			return true
		}
		tasks, err = e.Store.LoadTasks(ctx, turn.ID)
		if err != nil {
			e.failTurn(ctx, *turn, "turn_store_failed", err)
			b.finish(EndFailed)
			return true
		}
	}

	// 人工待办释放会话与执行循环；回复动作会重新启动。先处理回复/取消竞态。
	if HasWaitingHuman(tasks) {
		if ctx.Err() != nil {
			e.cancelFromWithin(ctx, *turn, b)
			return true
		}
		current, _, err := e.Store.LoadTurn(ctx, turn.ID)
		if err != nil {
			e.failTurn(ctx, *turn, "turn_store_failed", err)
			b.finish(EndFailed)
			return true
		}
		if current.Status == TurnWaitingHuman {
			b.pause()
			return true
		}
		*turn = current
		return false
	}

	// 串行执行就绪任务（§13.13：并发默认 1）。
	if t, ok := readyTask(tasks); ok {
		if err := e.runOne(ctx, at, b, *turn, g, t, tasks); err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				e.cancelFromWithin(ctx, *turn, b)
				return true
			}
			if errors.Is(err, errNeedReplan) {
				// 助手请求再规划：回到栅栏（有上限）。
				if rerr := e.replan(ctx, at, b, turn, g); rerr != nil {
					e.failTurn(ctx, *turn, "replan_failed", rerr)
					b.finish(EndFailed)
					return true
				}
				return false
			}
			e.failTurn(ctx, *turn, "run_failed", err)
			b.finish(EndFailed)
			return true
		}
		return false
	}

	// 无就绪任务：终局判定。
	if e.settle(ctx, at, b, *turn, g, tasks) {
		return true
	}
	// settle 未终局（如等待中的轮次被外部推进）：下一轮再查。
	select {
	case <-ctx.Done():
		e.cancelFromWithin(ctx, *turn, b)
		return true
	case <-at.humanCh:
	case <-time.After(200 * time.Millisecond):
	}
	return false
}

// runOne 执行一个任务并更新进度。
func (e *Engine) runOne(ctx context.Context, at *activeTurn, b *broker, turn Turn, g Group, t Task, all []Task) error {
	if t.TargetKind == "human" {
		return nil
	}
	if err := e.transitionTask(ctx, t.ID, TaskPending, TaskRunning, "", ""); err != nil {
		return err
	}
	t.Status = TaskRunning
	if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: t.ID, AgentID: t.TargetID,
		Data: taskJSON(t, indexTask(all, t), len(all))}); err != nil {
		return err
	}

	// context_from 拼接上游结果。
	results := map[string]taskOutcome{}
	for _, o := range all {
		if o.Status == TaskCompleted {
			results[o.Ref] = taskOutcome{agentID: o.TargetID, content: o.Result}
		}
	}
	emit := func(ev Event) { _, _ = b.publish(ctx, turn.ID, ev) }
	var runIDErr error
	res, err := e.Runner.RunTask(ctx, TaskInput{
		TurnID: turn.ID, TaskID: t.ID, Ref: t.Ref, AgentID: t.TargetID,
		Message: t.Message, Context: contextPayload(t, results),
		History: turn.HistoryOf(), Goal: turn.Decision.Goal,
		OnRunID: func(runID string) {
			t.RunID = runID
			if err := e.Store.SetTaskRunID(ctx, t.ID, runID); err != nil {
				runIDErr = err
				return
			}
			if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: t.ID, RunID: runID, AgentID: t.TargetID,
				Data: taskJSON(t, indexTask(all, t), len(all))}); err != nil {
				runIDErr = err
			}
		},
	}, emit)
	if runIDErr != nil {
		return runIDErr
	}
	if err != nil && !res.Failed && !res.Canceled {
		return err
	}
	switch {
	case res.Canceled:
		return context.Canceled
	case res.Failed:
		if err := e.transitionTask(ctx, t.ID, TaskRunning, TaskFailed, agentcore.Truncate(res.Content, maxGroupContentChars), res.ErrorCode); err != nil {
			return err
		}
		t.Status = TaskFailed
		if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: t.ID, AgentID: t.TargetID,
			Data: taskJSON(t, indexTask(all, t), len(all))}); err != nil {
			return err
		}
		if err := e.markBlocked(ctx, b, turn, all, t); err != nil {
			return err
		}
		if res.NeedReplan {
			return errNeedReplan
		}
		return nil
	default:
		if err := e.transitionTask(ctx, t.ID, TaskRunning, TaskCompleted, agentcore.Truncate(res.Content, maxGroupContentChars), ""); err != nil {
			return err
		}
		t.Status = TaskCompleted
		if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: t.ID, AgentID: t.TargetID,
			Data: taskJSON(t, indexTask(all, t), len(all))}); err != nil {
			return err
		}
		e.publishProgress(ctx, b, turn, all, t)
		if res.NeedReplan {
			return errNeedReplan
		}
		return nil
	}
}

// markBlocked 把依赖失败任务的下游逐层置 blocked（旧 resetTaskSubtree 的前置判定）。
func (e *Engine) markBlocked(ctx context.Context, b *broker, turn Turn, all []Task, failed Task) error {
	status := taskStatusIndex(all)
	status[failed.Ref] = TaskFailed
	for _, t := range all {
		if t.Status != TaskPending {
			continue
		}
		blocked := false
		for _, dep := range t.DependsOn {
			switch status[dep] {
			case TaskFailed, TaskCanceled, TaskInterrupted, TaskBlocked:
				blocked = true
			}
		}
		if !blocked {
			continue
		}
		if err := e.transitionTask(ctx, t.ID, TaskPending, TaskBlocked, "", "dependency_failed"); err != nil {
			return err
		}
		t.Status = TaskBlocked
		status[t.Ref] = TaskBlocked
		if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: t.ID, AgentID: t.TargetID,
			Data: taskJSON(t, indexTask(all, t), len(all))}); err != nil {
			return err
		}
	}
	return nil
}

// publishProgress 发布一次进度帧（任务完成后）。
func (e *Engine) publishProgress(ctx context.Context, b *broker, turn Turn, all []Task, just Task) {
	tasks, err := e.Store.LoadTasks(ctx, turn.ID)
	if err != nil {
		tasks = all
	}
	percent, summary := Progress(tasks)
	_, _ = b.publish(ctx, turn.ID, Event{Type: EventProgress,
		Data: progressJSON(percent, summary, stepsOf(turn.Decision, tasks))})
}

// settle 终局判定：全部任务终态 → 总结 + completed；有失败/阻塞 → 再规划或失败。
// 返回 true 表示轮次已终局。
func (e *Engine) settle(ctx context.Context, at *activeTurn, b *broker, turn Turn, g Group, tasks []Task) bool {
	if !AllSettled(tasks) {
		// 还有 pending/blocked 但无法推进：失败依赖的下游已 blocked，
		// 剩余 pending 一定是依赖未满足（理论不可达：DAG 前序）。
		return false
	}
	anyFailed := false
	var results []string
	for _, t := range tasks {
		if t.Status == TaskCompleted {
			results = append(results, agentcore.Truncate(t.Result, maxGroupContentChars))
		}
		if t.Status == TaskFailed || t.Status == TaskBlocked {
			anyFailed = true
		}
	}
	if anyFailed {
		// 失败收尾：有再规划预算 → 回栅栏；超出 → 转人工待办或失败（§13.3-7）。
		if err := e.replan(ctx, at, b, &turn, g); err != nil {
			e.failTurn(ctx, turn, "replan_failed", err)
			b.finish(EndFailed)
			return true
		}
		return false
	}
	// 全部完成：可选总结。
	if turn.Decision.NeedSummary && !turn.Summarized {
		summary := e.summarize(ctx, turn, results)
		_, _ = b.publish(ctx, turn.ID, Event{Type: EventProgress,
			Data: progressJSON(100, summary, stepsOf(turn.Decision, tasks))})
		d := turn.Decision
		d.Action = plan.ActionComplete
		d.Progress = plan.Progress{Percent: 100, Summary: agentcore.Truncate(summary, maxGroupContentChars)}
		_ = e.Store.SaveDecision(ctx, turn.ID, d, turn.ConfirmState)
	}
	if err := e.transitionTurn(ctx, turn.ID, currentTurnStatus(turn, tasks), TurnCompleted, ""); err != nil {
		e.failTurn(ctx, turn, "turn_store_failed", err)
	}
	b.finish(EndDone)
	return true
}

// handoverHuman 把失败轮次转人工待办（§13.3-7：超出再规划上限）。
func (e *Engine) handoverHuman(ctx context.Context, b *broker, turn Turn, g Group) {
	ht := Task{
		ID: turn.ID + "-h1", TurnID: turn.ID, Ref: "h1", TargetKind: "human", TargetID: g.HumanID,
		Message: "自动执行失败且再规划次数已用完，请人工接管：" + agentcore.Truncate(turn.Decision.Goal, maxGroupContentChars),
		Status:  TaskWaitingHuman,
	}
	if err := e.Store.CreateTasks(ctx, turn.ID, []Task{ht}); err != nil {
		e.failTurn(ctx, turn, "turn_store_failed", err)
		b.finish(EndFailed)
		return
	}
	_ = e.transitionTurn(ctx, turn.ID, currentTurnStatus(turn, nil), TurnWaitingHuman, "")
	_, _ = b.publish(ctx, turn.ID, Event{Type: EventHuman, TaskID: ht.ID, Data: humanJSON(g.HumanName, ht.Message)})
	_, _ = b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: ht.ID, Data: taskJSON(ht, 0, 1)})
}

// replan 回到栅栏：携带已完成结果作为历史，重建任务。
func (e *Engine) replan(ctx context.Context, at *activeTurn, b *broker, turn *Turn, g Group) error {
	if !canReplan(*turn, g) {
		if g.HumanID != "" && !g.HumanDisabled {
			e.handoverHuman(ctx, b, *turn, g)
			turn.Status = TurnWaitingHuman
			return nil
		}
		return errReplanLimit
	}
	tasks, err := e.Store.LoadTasks(ctx, turn.ID)
	if err != nil {
		return err
	}
	hist := append([]plan.Turn(nil), turn.HistoryOf()...)
	for _, t := range tasks {
		if t.Status == TaskCompleted && t.Result != "" {
			hist = append(hist, plan.Turn{Role: "assistant", Speaker: t.TargetID, Content: agentcore.Truncate(t.Result, maxGroupContentChars)})
		} else if t.Status == TaskFailed && t.Result != "" {
			hist = append(hist, plan.Turn{Role: "assistant", Speaker: t.TargetID, Content: "任务失败：" + agentcore.Truncate(t.Result, maxGroupContentChars)})
		}
	}
	in := TurnInput{
		ThreadID: turn.ThreadID, GroupID: turn.GroupID, Initiator: turn.Initiator,
		UserText: turn.UserText, Mentions: turn.Mentions, MentionedHuman: turn.MentionedHuman,
		History: hist, RoutingPrompt: turn.RoutingPrompt,
	}
	var cands []plan.Candidate
	if e.Planner != nil {
		cands, err = e.Members.Candidates(ctx, turn.GroupID)
		if err != nil {
			return err
		}
	}
	fenceCtx, cancel := context.WithTimeout(ctx, e.fenceTimeout())
	fr, derr := e.fence().decide(fenceCtx, in, g, cands)
	cancel()
	if derr != nil {
		return derr
	}
	d := fr.decision
	d.Action = plan.ActionReplan
	d.Confirm = plan.ApplyConfirmPolicy(d, g.Policy.Confirm)
	turn.Replans++
	turn.Decision = d
	if err := e.Store.SaveReplans(ctx, turn.ID, turn.Replans); err != nil {
		return err
	}
	confirmState := ""
	if d.Confirm.Required {
		confirmState = ConfirmRequired
	}
	if err := e.Store.SaveDecision(ctx, turn.ID, d, confirmState); err != nil {
		return err
	}
	tasks2 := mergeCompletedTasks(turn.ID, d, tasks)
	if err := e.Store.CreateTasks(ctx, turn.ID, tasks2); err != nil {
		return err
	}
	if _, err := b.publish(ctx, turn.ID, Event{Type: EventPlan, AgentID: "plan",
		Data: planJSON(d, d.Confirm.Required)}); err != nil {
		return err
	}
	for i, t := range tasks2 {
		if t.Status != TaskPending {
			continue
		}
		if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: t.ID, AgentID: t.TargetID,
			Data: taskJSON(t, i+1, len(tasks2))}); err != nil {
			return err
		}
	}
	if d.Confirm.Required {
		if _, err := b.publish(ctx, turn.ID, Event{Type: EventPlanConfirmation,
			Data: planConfirmJSON(ConfirmRequired, false)}); err != nil {
			return err
		}
		if err := e.transitionTurn(ctx, turn.ID, turn.Status, TurnAwaitingPlanConfirm, ""); err != nil {
			return err
		}
		turn.Status = TurnAwaitingPlanConfirm
		turn.ConfirmState = ConfirmRequired
		return nil
	}
	if turn.Status != TurnRunning {
		if err := e.transitionTurn(ctx, turn.ID, turn.Status, TurnRunning, ""); err != nil {
			return err
		}
	}
	turn.Status = TurnRunning
	turn.ConfirmState = ""
	return nil
}

// summarize 产出总结（Planner.LLM 或确定性拼接）。
func (e *Engine) summarize(ctx context.Context, turn Turn, results []string) string {
	if e.Summarizer != nil {
		if s, err := e.Summarizer(ctx, turn.Decision.Goal, results); err == nil && strings.TrimSpace(s) != "" {
			return s
		}
	}
	if e.Planner != nil && e.Planner.LLM != nil {
		var b strings.Builder
		fmt.Fprintf(&b, "Goal: %s\n\nTask results:\n", turn.Decision.Goal)
		for _, r := range results {
			b.WriteString("- " + r + "\n")
		}
		maxTokens := 400
		temp := 0.3
		resp, err := e.Planner.LLM.Chat(ctx, e.Planner.Scope, agentcore.ChatRequest{
			Model: e.Planner.Model, MaxTokens: &maxTokens, Temperature: &temp,
			Messages: []agentcore.ChatMessage{
				{Role: "system", Content: "Summarize the group's work for the user in one short paragraph (中文). Plain text only, no markdown headers."},
				{Role: "user", Content: agentcore.Truncate(b.String(), 8000)},
			},
		})
		if err == nil && strings.TrimSpace(resp.Content) != "" {
			return agentcore.Truncate(resp.Content, 2000)
		}
	}
	// 确定性回退：拼接结果。
	return agentcore.Truncate(strings.Join(results, "\n"), 2000)
}

// cancelFromWithin 是执行循环内的取消收尾：running → interrupted、其余未终态 → canceled。
func (e *Engine) cancelFromWithin(ctx context.Context, turn Turn, b *broker) {
	cleanupCtx := context.WithoutCancel(ctx)
	if current, ok, err := e.Store.LoadTurn(cleanupCtx, turn.ID); err == nil && ok {
		turn = current
	}
	tasks, err := e.Store.LoadTasks(cleanupCtx, turn.ID)
	if err != nil {
		e.failTurn(cleanupCtx, turn, "turn_store_failed", err)
		b.finish(EndCanceled)
		return
	}
	for _, t := range tasks {
		switch t.Status {
		case TaskRunning:
			if err := e.transitionTask(cleanupCtx, t.ID, TaskRunning, TaskInterrupted, "", "canceled"); err == nil {
				_, _ = b.publish(cleanupCtx, turn.ID, Event{Type: EventTask, TaskID: t.ID, AgentID: t.TargetID,
					Data: taskJSON(Task{ID: t.ID, Status: TaskInterrupted, TargetID: t.TargetID}, 0, len(tasks))})
			}
		case TaskPending, TaskBlocked, TaskWaitingHuman:
			_ = e.transitionTask(cleanupCtx, t.ID, t.Status, TaskCanceled, "", "canceled")
		}
	}
	_ = e.transitionTurn(cleanupCtx, turn.ID, turn.Status, TurnCanceled, "canceled")
	b.finish(EndCanceled)
}
