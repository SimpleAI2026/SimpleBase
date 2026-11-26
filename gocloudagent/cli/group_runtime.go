// 文件说明：group_runtime.go — 本地群运行时：群定义 + 模型 + 工具 + 文件存储 → group.Engine（planv6.8 §13.8 CLI 落地）
//
// 用法
//
//	newGroupRuntime(env, cfg, def, client, tools, opts) (*groupRuntime, error)
//	(rt *groupRuntime) submit(ctx, text, interactive, yes) int   // 发一条消息：栅栏 → （确认）→ 跟随执行直到结束 / 等人
//	(rt *groupRuntime) reply(ctx, outcome, text) error           // 回复人工待办
//	(rt *groupRuntime) status()                                  // 最近一轮的计划与任务
//
// 调用架构
//
//	[上游] repl.go（/group）与 group_cmd.go（group run）
//	         ▼
//	[本文件] group_runtime.go ──► gocloudagent/group.Engine（栅栏 / 调度 / 事件）
//	         │                        │ Runner.RunTask
//	         │                        ▼
//	         │              groupRunner（group_runner.go）：每个任务一个成员会话（SessionRunner，工具子集 + 规划工具）
//	         ▼
//	  fileGroupStore（groups/<name>/state.json）、plan.FileStore（groups/<name>/plan/）
//
// 同包实现按职责分在三个文件：group_runtime.go（本文件：装配、提交 / 跟随 / 回复 / 恢复、终端渲染）、
// group_prompt.go（终端交互提示）、group_runner.go（成员视图 groupMembers 与任务执行 groupRunner）。
// 终端交互：整份计划需要确认时提示 [a]pprove/[e]dit/[r]eject；人工待办在轮次停下等人时提示回复。
// headless（非交互）下需要确认按 --yes 决定批准或拒绝（默认拒绝并记录），人工任务只提示不等待。

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// groupLocalThread 是本地 REPL 的会话标识：一个群在同一时刻只有一轮。
const groupLocalThread = "local"

// groupRuntime 是一个已装配的本地群。
type groupRuntime struct {
	Name    string
	Config  GroupConfig
	Engine  *group.Engine
	Store   *fileGroupStore
	Members *groupMembers
	Runner  *groupRunner
	history []plan.Turn
}

// groupRuntimeOptions 是装配参数。
type groupRuntimeOptions struct {
	// Out 是终端输出（流式正文、计划卡、状态行）。
	Out io.Writer
	// In 是交互输入（确认 / 人工回复）；nil 表示 headless。
	In io.Reader
	// JSON 为真时事件按 JSON 行输出（与 gocloudagent --json 同口径）。
	JSON bool
	// MaxIterations 是单个成员任务的工具轮数上限；0 用默认。
	MaxIterations int
	// Warn 记录降级路径（可为 nil）。
	Warn func(msg string, fields ...zap.Field)
	// Redact 用于成员会话落盘前脱敏（可为 nil）。
	Redact func(string) string
}

// newGroupRuntime 装配引擎并做重启收敛（本进程没有活动轮次，残留的 running 轮次标记为 interrupted）。
func newGroupRuntime(env []string, cfg Config, def GroupConfig, client agentcore.ChatClient, tools []tool.BaseTool, opts groupRuntimeOptions) (*groupRuntime, error) {
	stateDir := GroupStateDir(env, def.Name)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	store, err := openGroupStore(filepath.Join(stateDir, "state.json"))
	if err != nil {
		return nil, err
	}
	rt := &groupRuntime{Name: def.Name, Config: def, Store: store}
	rt.Members = &groupMembers{cfg: def, history: func() []plan.Turn { return rt.history }}
	// 终端输出会被两处写：跟随循环渲染结构帧、成员任务流式写正文（在引擎的后台协程里）。加锁串行化。
	var out io.Writer = opts.Out
	if opts.Out != nil {
		out = &lockedWriter{w: opts.Out}
	}
	rt.Runner = &groupRunner{
		env: env, cfg: cfg, def: def, client: client, tools: tools,
		planDir: filepath.Join(stateDir, "plan"),
		out:     out, maxIterations: opts.MaxIterations, warn: opts.Warn, redact: opts.Redact,
	}
	if opts.In != nil {
		rt.Runner.in = opts.In
		rt.Runner.lineReader = bufio.NewReader(opts.In)
	}
	var planner *plan.Planner
	if client != nil {
		planner = &plan.Planner{LLM: client, Model: strings.TrimSpace(cfg.Model)}
	}
	engine, err := group.NewEngine(planner, rt.Runner, store, rt.Members)
	if err != nil {
		return nil, err
	}
	rt.Engine = engine
	rt.loadHistory()
	if err := engine.Converge(context.Background()); err != nil {
		return nil, err
	}
	return rt, nil
}

// loadHistory 用最近 6 轮的「用户消息 + 各成员结果」重建栅栏可见的群聊历史。
func (rt *groupRuntime) loadHistory() {
	all, err := rt.recentTurns(6)
	if err != nil {
		return
	}
	rt.history = nil
	for _, t := range all {
		rt.history = append(rt.history, plan.Turn{Role: "user", Content: t.UserText})
		tasks, _ := rt.Store.LoadTasks(context.Background(), t.ID)
		for _, task := range tasks {
			if task.Status == group.TaskCompleted && task.Result != "" {
				rt.history = append(rt.history, plan.Turn{Role: "assistant", Speaker: task.TargetID, Content: agentcore.Truncate(task.Result, 600)})
			}
		}
	}
}

// recentTurns 返回最近 limit 轮（时间升序）。
func (rt *groupRuntime) recentTurns(limit int) ([]group.Turn, error) {
	rt.Store.mu.Lock()
	turns := make([]group.Turn, 0, len(rt.Store.st.Turns))
	for _, st := range rt.Store.st.Turns {
		turns = append(turns, st.Turn)
	}
	rt.Store.mu.Unlock()
	sort.Slice(turns, func(i, j int) bool { return turns[i].CreatedAt.Before(turns[j].CreatedAt) })
	if len(turns) > limit {
		turns = turns[len(turns)-limit:]
	}
	return turns, nil
}

// submit 处理一条群消息：解析 @ 点名 → Submit → 需要确认时按交互 / --yes 处理 → 跟随执行。
// 返回进程退出码口径（0 完成 / 等人，2 失败，1 取消或拒绝）。
func (rt *groupRuntime) submit(ctx context.Context, text string, interactive, yes bool) int {
	in := rt.turnInput(text)
	turn, err := rt.Engine.Submit(ctx, in)
	if err != nil {
		rt.printf("error: %s: %v\n", plan.ErrorCode(err), err)
		return exitAPI
	}
	rt.persistPlan(ctx, turn.Decision)
	if turn.Status == group.TurnAwaitingPlanConfirm {
		approve := false
		var edits []plan.Edit
		if interactive {
			approve, edits = rt.promptPlanConfirm(turn)
		} else if yes {
			approve = true
			rt.printf("plan: --yes，自动批准整份计划\n")
		} else {
			rt.printf("plan: 需要人工确认，headless 下按策略拒绝（加 --yes 自动批准，或在 REPL 里用 /group %s）\n", rt.Name)
		}
		turn, err = rt.Engine.ConfirmPlan(ctx, turn.ID, group.Approval{Approve: approve, Edits: edits})
		if err != nil {
			rt.printf("error: %v\n", err)
			return exitAPI
		}
		if !approve {
			return exitUsage
		}
	}
	rt.follow(ctx, turn.ID, 0, interactive)
	rt.loadHistory()
	final, _, _ := rt.Store.LoadTurn(ctx, turn.ID)
	return turnExitCode(final)
}

// turnInput 解析 @ 点名（成员 id / me）并构造 Submit 输入。
func (rt *groupRuntime) turnInput(text string) group.TurnInput {
	in := group.TurnInput{
		ThreadID: groupLocalThread, GroupID: rt.Name, Initiator: "user:" + osUser(),
		UserText: strings.TrimSpace(text), History: append([]plan.Turn(nil), rt.history...),
		RoutingPrompt: rt.Config.RoutingPrompt,
	}
	for _, token := range mentionTokens(text) {
		if _, ok := rt.Config.Member(token); ok {
			in.Mentions = append(in.Mentions, token)
		}
		if token == "me" || token == "human" {
			in.MentionedHuman = true
		}
	}
	in.UserText = strings.TrimSpace(stripMentions(in.UserText))
	return in
}

// follow 订阅轮次事件并渲染，直到轮次终态或停下等人（人工任务 / 需要确认）。
// 交互模式下遇到需要确认或等人会就地提示并继续跟随。
func (rt *groupRuntime) follow(ctx context.Context, turnID string, after int, interactive bool) {
	idleSince := time.Now()
	for {
		turn, ok, err := rt.Store.LoadTurn(ctx, turnID)
		if err != nil || !ok {
			return
		}
		if group.TerminalTurn(turn.Status) {
			return
		}
		if turn.Status == group.TurnAwaitingPlanConfirm {
			if !interactive {
				rt.printf("plan: 等待确认（在 REPL 里用 /group %s 打开）\n", rt.Name)
				return
			}
			approve, edits := rt.promptPlanConfirm(turn)
			if _, err := rt.Engine.ConfirmPlan(ctx, turn.ID, group.Approval{Approve: approve, Edits: edits}); err != nil {
				rt.printf("error: %v\n", err)
				return
			}
			if !approve {
				return
			}
		}
		if turn.Status == group.TurnWaitingHuman {
			if !interactive {
				rt.printWaitingHuman(turnID)
				return
			}
			if !rt.promptHumanReply(ctx, turnID) {
				return
			}
		}
		sub, err := rt.Engine.Subscribe(ctx, turnID, after)
		if err != nil {
			rt.printf("error: subscribe: %v\n", err)
			return
		}
		for {
			ev, ok := sub.Next()
			if !ok {
				break
			}
			if ev.Seq > after {
				after = ev.Seq
				idleSince = time.Now()
			}
			rt.render(ev)
		}
		sub.Close()
		// 订阅结束但没有 end 帧：轮次可能停在了等人 / 待确认，回到循环头判断；否则短暂等待后重连。
		current, ok, _ := rt.Store.LoadTurn(ctx, turnID)
		if !ok || group.TerminalTurn(current.Status) || current.Status == group.TurnWaitingHuman || current.Status == group.TurnAwaitingPlanConfirm {
			continue
		}
		// 轮次仍是 running 却长时间没有任何事件：执行循环可能已经不在（进程重启 / 异常退出），不空转。
		if time.Since(idleSince) > 15*time.Second {
			rt.printf("— 轮次 %s 长时间无进展，停止跟随（/group status 查看，/group resume 恢复）\n", current.Status)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// reply 回复当前轮次里等待中的人工任务。
func (rt *groupRuntime) reply(ctx context.Context, outcome, text string) error {
	turnID, taskID := rt.waitingHumanTask()
	if taskID == "" {
		return errors.New("没有等待你处理的任务")
	}
	if _, err := rt.Engine.ReplyHuman(ctx, taskID, group.Reply{Text: text, Outcome: outcome}); err != nil {
		return err
	}
	rt.follow(ctx, turnID, rt.lastSeq(ctx, turnID), true)
	return nil
}

// lastSeq 返回轮次已落库的最大事件序号（续接用，避免回放已看过的帧）。
func (rt *groupRuntime) lastSeq(ctx context.Context, turnID string) int {
	events, err := rt.Store.ListEvents(ctx, turnID, 0)
	if err != nil {
		return 0
	}
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Seq
}

// waitingHumanTask 返回最近一轮里第一个等待中的人工任务。
func (rt *groupRuntime) waitingHumanTask() (turnID, taskID string) {
	unfinished, _ := rt.Store.ListUnfinishedTurns(context.Background())
	for i := len(unfinished) - 1; i >= 0; i-- {
		if unfinished[i].Status != group.TurnWaitingHuman {
			continue
		}
		tasks, _ := rt.Store.LoadTasks(context.Background(), unfinished[i].ID)
		for _, t := range tasks {
			if t.Status == group.TaskWaitingHuman {
				return unfinished[i].ID, t.ID
			}
		}
	}
	return "", ""
}

// resume 恢复中断 / 失败 / 等人的轮次（进程重启后或上一次 Ctrl+C 之后）。
func (rt *groupRuntime) resume(ctx context.Context, interactive bool) bool {
	candidates, _ := rt.Store.ListResumableTurns(context.Background())
	for i := len(candidates) - 1; i >= 0; i-- {
		switch candidates[i].Status {
		case group.TurnInterrupted, group.TurnWaitingHuman, group.TurnAwaitingPlanConfirm:
			// 失败的轮次由引擎的 RetryTask 复活（按任务），不在这里整体重跑。
			turn, err := rt.Engine.Resume(ctx, candidates[i].ID)
			if err != nil {
				rt.printf("error: resume: %v\n", err)
				return false
			}
			rt.follow(ctx, turn.ID, 0, interactive)
			return true
		}
	}
	return false
}

// status 打印最近一轮的计划、任务与确认状态。
func (rt *groupRuntime) status() {
	turns, err := rt.recentTurns(1)
	if err != nil || len(turns) == 0 {
		rt.printf("群 %s：还没有轮次。\n", rt.Name)
		return
	}
	t := turns[0]
	rt.printf("群 %s · 最近一轮 %s · %s\n", rt.Name, shortID(t.ID), t.Status)
	if t.Decision.Goal != "" {
		rt.printf("  目标：%s\n", t.Decision.Goal)
	}
	for _, s := range t.Decision.Steps {
		rt.printf("  步骤 [%s] %s %s\n", s.Status, s.ID, s.Title)
	}
	tasks, _ := rt.Store.LoadTasks(context.Background(), t.ID)
	for _, task := range tasks {
		line := fmt.Sprintf("  任务 %s → %s：%s（%s）", task.Ref, task.TargetID, firstLine(task.Message, 80), task.Status)
		if task.ErrorCode != "" {
			line += " " + task.ErrorCode
		}
		rt.println(line)
	}
}

// ---- 终端渲染 ----

func (rt *groupRuntime) printf(format string, args ...any) {
	fmt.Fprintf(rt.Runner.out, format, args...)
}

func (rt *groupRuntime) println(line string) { fmt.Fprintln(rt.Runner.out, line) }

// render 把一帧轮次事件画到终端。
func (rt *groupRuntime) render(ev group.Event) {
	var data map[string]any
	_ = json.Unmarshal(ev.Data, &data)
	switch ev.Type {
	case group.EventPlan:
		d := plan.Decision{}
		if raw, ok := data["decision"]; ok {
			if b, err := json.Marshal(raw); err == nil {
				_ = json.Unmarshal(b, &d)
			}
		}
		rt.println("\n┌─ 计划")
		rt.printf("│ 目标：%s\n", d.Goal)
		if d.Complexity == plan.ComplexityComplex {
			rt.printf("│ 复杂度：%s\n", d.Complexity)
		}
		for _, s := range d.Steps {
			rt.printf("│ 步骤 [%s] %s %s\n", s.Status, s.ID, s.Title)
		}
		rt.persistPlan(context.Background(), d)
		for _, dp := range d.Dispatches {
			line := fmt.Sprintf("│ 任务 %s → @%s：%s", dp.Ref, dp.TargetID, firstLine(dp.Message, 100))
			if len(dp.DependsOn) > 0 {
				line += fmt.Sprintf("（依赖 %s）", strings.Join(dp.DependsOn, ", "))
			}
			if dp.Risk != "" {
				line += " [" + dp.Risk + "]"
			}
			rt.println(line)
		}
		if len(d.Findings) > 0 {
			rt.printf("│ 发现：%s\n", strings.Join(d.Findings, "；"))
		}
		rt.println("└─")
	case group.EventPlanConfirmation:
		rt.printf("计划确认：%s\n", data["state"])
	case group.EventTask:
		line := fmt.Sprintf("· 任务 %v → @%s：%v", data["ref"], ev.AgentID, data["status"])
		if code, _ := data["error_code"].(string); code != "" {
			line += "（" + code + "）"
		}
		rt.println(line)
	case group.EventProgress:
		rt.printf("进度 %v%% %v\n", data["percent"], data["summary"])
	case group.EventHuman:
		rt.printf("\n需要你处理：%v\n", data["message"])
	case group.EventTurn:
		if st, _ := data["status"].(string); st != "" && st != group.TurnPlanning {
			rt.printf("— 轮次 %s\n", st)
		}
	case group.EventEnd:
		rt.printf("— 结束（%v）\n", data["reason"])
	}
}

// persistPlan 把当前决策写成计划文件（plan.md / decision.json，planning-with-files 布局）；
// 写失败只提示，不影响轮次执行。
func (rt *groupRuntime) persistPlan(ctx context.Context, d plan.Decision) {
	if strings.TrimSpace(d.Goal) == "" && len(d.Dispatches) == 0 {
		return
	}
	if err := (&plan.FileStore{Dir: rt.Runner.planDir}).SaveDecision(ctx, d); err != nil {
		rt.printf("warn: plan files: %v\n", err)
	}
}

// turnExitCode 把轮次终态映射为退出码。
func turnExitCode(t group.Turn) int {
	switch t.Status {
	case group.TurnCompleted, group.TurnWaitingHuman:
		return exitOK
	case group.TurnCanceled, group.TurnRejected:
		return exitUsage
	case group.TurnFailed, group.TurnInterrupted:
		return exitAPI
	}
	return exitOK
}

// mentionTokens 提取文本里的 @token（不含 @ 本身）。
func mentionTokens(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range mentionPattern.FindAllStringSubmatch(text, -1) {
		tok := strings.ToLower(m[1])
		if tok == "" || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

// stripMentions 去掉 @token（成员 / 人由栅栏按 Mentions 处理，正文里不再重复）。
func stripMentions(text string) string {
	return strings.TrimSpace(mentionPattern.ReplaceAllString(text, ""))
}

var mentionPattern = regexp.MustCompile(`@([A-Za-z0-9_-]+)`)

// osUser 返回当前系统用户名（拿不到时用 "you"）。
func osUser() string {
	for _, key := range []string{"USER", "USERNAME", "LOGNAME"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return "you"
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
