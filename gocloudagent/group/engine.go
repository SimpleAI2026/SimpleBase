// 文件说明：engine.go — 群聊引擎入口：构造、Submit 与后台执行启动
//
// 执行语义：Submit 在请求上下文内完成栅栏；执行循环脱离请求取消，
// 由轮次级 cancel 与 Store.Transition* 保证收尾和状态迁移。
// 同包实现按职责分在 engine_exec.go / engine_control.go / engine_state.go / engine_frames.go。
// 依赖：group → plan / agentcore；不 import cloudagent、cli、宿主包。

package group

import (
	"context"
	"errors"
	"sync"
	"time"

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
