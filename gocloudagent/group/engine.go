// 文件说明：engine.go — 群聊引擎：栅栏 → 决策 → 后台执行（planv6.8 §13.8/§13.10）
//
// 执行语义（修复 CB-BE-01/02/03）：
//   - Submit 在请求 ctx 内完成栅栏（限时 FenceTimeout）；执行循环在后台
//     goroutine 以 WithoutCancel + 轮次级 cancel 运行，SSE 只是订阅者；
//   - 动作（ConfirmPlan / Cancel / RetryTask / ReplyHuman / Resume）返回快照，
//     推进交给后台执行器；
//   - 所有状态变更经 Store.Transition*（乐观锁 + 非法迁移报错，不静默覆盖）；
//   - 结构帧全部落库（单事件表，seq 原子单调），内容帧走 Runner 自己的 run 事件
//     缓存，按 task.run_id 二级续接；
//   - 停机：BeginShutdown 取消全部有执行循环的轮次（非阻塞），WaitIdle 等它们收尾落库；
//     停在 waiting_human / awaiting_plan_confirm 的轮次没有执行循环，不受影响。
//
// 依赖：group → plan / agentcore；不 import cloudagent、cli、宿主包。

package group

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// Engine 是群聊运行时。零值不可用；经 NewEngine 构造。
type Engine struct {
	Planner *plan.Planner // 可空：只走规则栅栏（无模型群）
	Runner  Runner        // 必填
	Store   Store         // 必填
	Members Members       // 必填

	// FenceTimeout 是栅栏（含再规划）的单次模型决策限时；0 取 20 秒。
	FenceTimeout time.Duration

	reg     *registry
	brokers sync.Map // turnID → *broker
	// summarize 独立注入便于测试；空时用 Planner.LLM 或确定性拼接。
	Summarizer func(ctx context.Context, goal string, results []string) (string, error)
}

// NewEngine 构造引擎并校验必填端口。
func NewEngine(planner *plan.Planner, runner Runner, store Store, members Members) (*Engine, error) {
	if runner == nil || store == nil || members == nil {
		return nil, errors.New("group: Runner, Store and Members are required")
	}
	return &Engine{
		Planner: planner, Runner: runner, Store: store, Members: members,
		reg: newRegistry(),
	}, nil
}

const defaultFenceTimeout = 20 * time.Second

func (e *Engine) fenceTimeout() time.Duration {
	if e.FenceTimeout > 0 {
		return e.FenceTimeout
	}
	return defaultFenceTimeout
}

func (e *Engine) fence() *fence { return &fence{planner: e.Planner} }

// brokerOf 返回（或惰性创建）轮次的 broker。幂等：竞态时后建者被丢弃。
func (e *Engine) brokerOf(turnID string) *broker {
	if b, ok := e.brokers.Load(turnID); ok {
		return b.(*broker)
	}
	b := newBroker(e.Store, turnID)
	actual, loaded := e.brokers.LoadOrStore(turnID, b)
	if loaded {
		return actual.(*broker)
	}
	return b
}

// ---------------------------------------------------------------------------
// Submit：栅栏入口
// ---------------------------------------------------------------------------

// Submit 处理一条群消息：建轮次 → 栅栏 → 需要确认则停在 awaiting_plan_confirm，
// 否则启动后台执行。返回 turnID 与当前状态（供宿主选择 JSON 快照或 SSE 订阅）。
func (e *Engine) Submit(ctx context.Context, in TurnInput) (Turn, error) {
	if e.reg.isClosed() {
		return Turn{}, ErrEngineClosed
	}
	g, err := e.Members.Group(ctx, in.GroupID)
	if err != nil {
		return Turn{}, err
	}
	var cands []plan.Candidate
	if e.Planner != nil {
		cands, err = e.Members.Candidates(ctx, in.GroupID)
		if err != nil {
			return Turn{}, err
		}
	}
	if in.History == nil {
		in.History, _ = e.Members.History(ctx, in.ThreadID, 6)
	}

	turn, reused, err := e.Store.CreateTurn(ctx, Turn{
		ThreadID: in.ThreadID, GroupID: in.GroupID, Status: TurnPlanning,
		Initiator: in.Initiator, UserText: in.UserText, Mentions: in.Mentions,
		MentionedHuman: in.MentionedHuman, IdempotencyKey: in.IdempotencyKey,
		History: in.History, RoutingPrompt: in.RoutingPrompt,
	})
	if err != nil {
		return Turn{}, err
	}
	if reused {
		return turn, nil
	}
	if in.BeforeFence != nil {
		if err := in.BeforeFence(ctx, turn); err != nil {
			e.failTurn(ctx, turn, "turn_store_failed", err)
			return Turn{}, err
		}
	}
	b := e.brokerOf(turn.ID)
	// 首帧 turn（携带 turn_id，修复 CB-FE-02）：前端停止/续接都以它为准。
	if _, err := b.publish(ctx, turn.ID, Event{Type: EventTurn, Data: turnJSON(TurnPlanning, 0)}); err != nil {
		e.failTurn(ctx, turn, "turn_store_failed", err)
		return Turn{}, err
	}
	if err := e.Store.Claim(ctx, in.ThreadID, turn.ID); err != nil {
		e.failTurn(ctx, turn, "turn_busy", err)
		return Turn{}, err
	}

	// 栅栏（请求 ctx + 限时）。
	fenceCtx, cancel := context.WithTimeout(ctx, e.fenceTimeout())
	fr, derr := e.fence().decide(fenceCtx, in, g, cands)
	cancel()
	if derr != nil {
		e.failTurn(ctx, turn, plan.ErrorCode(derr), derr)
		return Turn{}, derr
	}
	d := fr.decision
	d.Confirm = plan.ApplyConfirmPolicy(d, g.Policy.Confirm)

	if err := e.Store.SaveDecision(ctx, turn.ID, d, ""); err != nil {
		e.failTurn(ctx, turn, "turn_store_failed", err)
		return Turn{}, err
	}
	turn.Decision = d
	if _, err := b.publish(ctx, turn.ID, Event{Type: EventPlan, AgentID: "plan",
		Data: planJSON(d, d.Confirm.Required)}); err != nil {
		e.failTurn(ctx, turn, "turn_store_failed", err)
		return Turn{}, err
	}

	if d.Confirm.Required {
		// 需要人工批准整份计划：轮次停在 awaiting_plan_confirm（claim 保持，
		// 防止确认期间又开新轮次；Cancel 可释放）。
		if err := e.transitionTurn(ctx, turn.ID, TurnPlanning, TurnAwaitingPlanConfirm, ""); err != nil {
			e.failTurn(ctx, turn, "turn_store_failed", err)
			return Turn{}, err
		}
		if _, err := b.publish(ctx, turn.ID, Event{Type: EventPlanConfirmation,
			Data: planConfirmJSON(ConfirmRequired, false)}); err != nil {
			return Turn{}, err
		}
		turn.Status = TurnAwaitingPlanConfirm
		turn.ConfirmState = ConfirmRequired
		_ = e.Store.SaveDecision(ctx, turn.ID, d, ConfirmRequired)
		return turn, nil
	}

	if err := e.launch(ctx, turn, d); err != nil {
		return Turn{}, err
	}
	turn.Status = TurnRunning
	return turn, nil
}

// launch 持久化任务、迁移到 running 并启动后台执行循环。
func (e *Engine) launch(ctx context.Context, turn Turn, d plan.Decision) error {
	g, err := e.Members.Group(ctx, turn.GroupID)
	if err != nil {
		e.failTurn(ctx, turn, "group_unavailable", err)
		return err
	}
	old, err := e.Store.LoadTasks(ctx, turn.ID)
	if err != nil {
		e.failTurn(ctx, turn, "turn_store_failed", err)
		return err
	}
	tasks := mergeCompletedTasks(turn.ID, d, old)
	if err := e.Store.CreateTasks(ctx, turn.ID, tasks); err != nil {
		e.failTurn(ctx, turn, "turn_store_failed", err)
		return err
	}
	b := e.brokerOf(turn.ID)
	for i, t := range tasks {
		if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: t.ID, AgentID: t.TargetID,
			Data: taskJSON(t, i+1, len(tasks))}); err != nil {
			e.failTurn(ctx, turn, "turn_store_failed", err)
			return err
		}
	}
	from := turn.Status
	if from == TurnPlanning || from == TurnAwaitingPlanConfirm || from == TurnWaitingHuman {
		if err := e.transitionTurn(ctx, turn.ID, from, TurnRunning, ""); err != nil {
			return err
		}
		turn.Status = TurnRunning
	}
	if err := e.Store.Claim(ctx, turn.ThreadID, turn.ID); err != nil {
		e.failTurn(ctx, turn, "turn_busy", err)
		return err
	}
	// 后台执行：脱离请求取消（CB-BE-02），轮次级 cancel 在 registry。
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	at, err := e.reg.register(turn.ID, turn.ThreadID, cancel)
	if err != nil {
		cancel()
		_ = e.Store.Release(ctx, turn.ThreadID, turn.ID)
		if errors.Is(err, ErrEngineClosed) {
			// 停机窗口内走到这里的轮次：按引擎关闭收尾，不借用 turn_busy 的错误码。
			e.failTurn(ctx, turn, "engine_closed", err)
			return err
		}
		e.failTurn(ctx, turn, "turn_busy", err)
		return err
	}
	go e.execute(at, runCtx, turn, g)
	return nil
}

// tasksOf 把决策 dispatch 转成任务。
func tasksOf(turnID string, d plan.Decision) []Task {
	out := make([]Task, 0, len(d.Dispatches))
	for _, dp := range d.Dispatches {
		out = append(out, Task{
			ID: turnID + "-" + dp.Ref, TurnID: turnID, Ref: dp.Ref,
			TargetKind: dp.TargetKind, TargetID: dp.TargetID, Message: dp.Message,
			Status: TaskPending, DependsOn: dp.DependsOn, ContextFrom: dp.ContextFrom,
			Risk: dp.Risk, StepID: dp.StepID, ExpectedOutput: dp.ExpectedOutput,
		})
	}
	return out
}

func mergeCompletedTasks(turnID string, d plan.Decision, old []Task) []Task {
	tasks := tasksOf(turnID, d)
	previous := make(map[string]Task, len(old))
	for _, task := range old {
		previous[task.Ref] = task
	}
	for i := range tasks {
		if oldTask, ok := previous[tasks[i].Ref]; ok && oldTask.Status == TaskCompleted {
			tasks[i] = oldTask
		}
	}
	return tasks
}

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
	for {
		if ctx.Err() != nil {
			e.cancelFromWithin(ctx, turn, b)
			return
		}
		current, ok, err := e.Store.LoadTurn(ctx, turn.ID)
		if err != nil {
			e.failTurn(ctx, turn, "turn_store_failed", err)
			b.finish(EndFailed)
			return
		}
		if ok {
			turn = current
		}
		if TerminalTurn(turn.Status) {
			b.finish(endReasonOf(turn.Status))
			return
		}
		if turn.Status == TurnAwaitingPlanConfirm {
			b.pause()
			return
		}
		tasks, err := e.Store.LoadTasks(ctx, turn.ID)
		if err != nil {
			e.failTurn(ctx, turn, "turn_store_failed", err)
			b.finish(EndFailed)
			return
		}

		// 人工任务就绪 → waiting_human（等人不是错误，CB-BE 旧语义保留）。
		if ht, ok := humanReady(tasks); ok {
			if err := e.transitionTask(ctx, ht.ID, TaskPending, TaskWaitingHuman, "", ""); err != nil {
				e.failTurn(ctx, turn, "turn_store_failed", err)
				b.finish(EndFailed)
				return
			}
			ht.Status = TaskWaitingHuman
			if err := e.transitionTurn(ctx, turn.ID, turn.Status, TurnWaitingHuman, ""); err != nil {
				e.failTurn(ctx, turn, "turn_store_failed", err)
				b.finish(EndFailed)
				return
			}
			turn.Status = TurnWaitingHuman
			if _, err := b.publish(ctx, turn.ID, Event{Type: EventHuman, TaskID: ht.ID,
				Data: humanJSON(g.HumanName, ht.Message, g.HumanID)}); err != nil {
				e.failTurn(ctx, turn, "turn_store_failed", err)
				return
			}
			tasks, err = e.Store.LoadTasks(ctx, turn.ID)
			if err != nil {
				e.failTurn(ctx, turn, "turn_store_failed", err)
				b.finish(EndFailed)
				return
			}
		}

		// 人工待办释放会话与执行循环；回复动作会重新启动。先处理回复/取消竞态。
		if HasWaitingHuman(tasks) {
			if ctx.Err() != nil {
				e.cancelFromWithin(ctx, turn, b)
				return
			}
			current, _, err := e.Store.LoadTurn(ctx, turn.ID)
			if err != nil {
				e.failTurn(ctx, turn, "turn_store_failed", err)
				b.finish(EndFailed)
				return
			}
			if current.Status == TurnWaitingHuman {
				b.pause()
				return
			}
			turn = current
			continue
		}

		// 串行执行就绪任务（§13.13：并发默认 1）。
		if t, ok := readyTask(tasks); ok {
			if err := e.runOne(ctx, at, b, turn, g, t, tasks); err != nil {
				if ctx.Err() != nil || errors.Is(err, context.Canceled) {
					e.cancelFromWithin(ctx, turn, b)
					return
				}
				if errors.Is(err, errNeedReplan) {
					// 助手请求再规划：回到栅栏（有上限）。
					if rerr := e.replan(ctx, at, b, &turn, g); rerr != nil {
						e.failTurn(ctx, turn, "replan_failed", rerr)
						b.finish(EndFailed)
						return
					}
					continue
				}
				e.failTurn(ctx, turn, "run_failed", err)
				b.finish(EndFailed)
				return
			}
			continue
		}

		// 无就绪任务：终局判定。
		done := e.settle(ctx, at, b, turn, g, tasks)
		if done {
			return
		}
		// settle 未终局（如等待中的轮次被外部推进）：下一轮再查。
		select {
		case <-ctx.Done():
			e.cancelFromWithin(ctx, turn, b)
			return
		case <-at.humanCh:
		case <-time.After(200 * time.Millisecond):
		}
	}
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
		if err := e.transitionTask(ctx, t.ID, TaskRunning, TaskFailed, truncate(res.Content), res.ErrorCode); err != nil {
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
		if err := e.transitionTask(ctx, t.ID, TaskRunning, TaskCompleted, truncate(res.Content), ""); err != nil {
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
			results = append(results, truncate(t.Result))
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
		d.Progress = plan.Progress{Percent: 100, Summary: truncate(summary)}
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
		Message: "自动执行失败且再规划次数已用完，请人工接管：" + truncate(turn.Decision.Goal),
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
			hist = append(hist, plan.Turn{Role: "assistant", Speaker: t.TargetID, Content: truncate(t.Result)})
		} else if t.Status == TaskFailed && t.Result != "" {
			hist = append(hist, plan.Turn{Role: "assistant", Speaker: t.TargetID, Content: "任务失败：" + truncate(t.Result)})
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
	if err := e.transitionTask(ctx, taskID, TaskWaitingHuman, to, truncate(reply.Text), errorCode); err != nil {
		return turn, err
	}
	b := e.brokerOf(turn.ID)
	task.Status = to
	task.Result = truncate(reply.Text)
	task.ErrorCode = errorCode
	if _, err := b.publish(ctx, turn.ID, Event{Type: EventTask, TaskID: taskID,
		Data: taskJSON(task, indexTask([]Task{task}, task), 1)}); err != nil {
		return turn, err
	}
	if _, err := b.publish(ctx, turn.ID, Event{Type: EventHuman, TaskID: taskID,
		Data: humanJSON("", truncate(reply.Text))}); err != nil {
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
	return TurnResult{TurnID: turn.ID, Status: turn.Status, Content: truncate(content)}
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

// ---------------------------------------------------------------------------
// 帧载荷
// ---------------------------------------------------------------------------

func turnJSON(status string, _ int) []byte {
	b, _ := json.Marshal(map[string]any{"status": status})
	return b
}

func planJSON(d plan.Decision, confirmRequired bool) []byte {
	b, _ := json.Marshal(map[string]any{
		"decision": d, "confirm_required": confirmRequired,
	})
	return b
}

func planConfirmJSON(status string, edited bool) []byte {
	b, _ := json.Marshal(map[string]any{"state": status, "status": status, "edited": edited})
	return b
}

func taskJSON(t Task, index, total int) []byte {
	b, _ := json.Marshal(map[string]any{
		"status": t.Status, "index": index, "total": total,
		"ref": t.Ref, "target_kind": t.TargetKind, "target_id": t.TargetID,
		"message": truncate(t.Message), "depends_on": t.DependsOn, "context_from": t.ContextFrom,
		"step_id": t.StepID, "risk": t.Risk, "expected_output": t.ExpectedOutput,
		"result": t.Result, "error_code": t.ErrorCode, "run_id": t.RunID,
		"attempt": t.Attempt, "version": t.Version,
	})
	return b
}

func humanJSON(name, message string, humanIDs ...string) []byte {
	if name == "" {
		name = "当前用户"
	}
	payload := map[string]any{"human_name": name, "message": truncate(message)}
	if len(humanIDs) > 0 {
		payload["human_id"] = humanIDs[0]
	}
	b, _ := json.Marshal(payload)
	return b
}

func progressJSON(percent int, summary string, steps []plan.Step) []byte {
	b, _ := json.Marshal(map[string]any{
		"percent": percent, "summary": summary, "steps": steps,
	})
	return b
}
