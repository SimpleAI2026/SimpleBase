// 文件说明：group_runtime.go — 本地群运行时：群定义 + 模型 + 工具 + 文件存储 → group.Engine（planv6.8 §13.8 CLI 落地）
//
// 用法
//
//	newGroupRuntime(env, cfg, def, client, tools, opts) (*groupRuntime, error)
//	(rt *groupRuntime) submit(ctx, text, interactive) int        // 发一条消息：栅栏 → （确认）→ 跟随执行直到结束 / 等人
//	(rt *groupRuntime) reply(ctx, taskID, outcome, text) error   // 回复人工待办
//	(rt *groupRuntime) status(w)                                  // 最近一轮的计划与任务
//
// 调用架构
//
//	[上游] repl.go（/group）与 group_cmd.go（group run）
//	         ▼
//	[本文件] group_runtime.go ──► gocloudagent/group.Engine（栅栏 / 调度 / 事件）
//	         │                        │ Runner.RunTask
//	         │                        ▼
//	         │              groupRunner：每个任务一个成员会话（SessionRunner，工具子集 + 规划工具）
//	         ▼
//	  fileGroupStore（groups/<name>/state.json）、plan.FileStore（groups/<name>/plan/）
//
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
	"sync"
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

// promptPlanConfirm 在终端就整份计划询问 [a]pprove/[e]dit/[r]eject；edit 只支持改某个 ref 的指令或移除某个 ref。
func (rt *groupRuntime) promptPlanConfirm(turn group.Turn) (bool, []plan.Edit) {
	d := turn.Decision
	rt.println("\n⚠ 这份计划需要你确认后才会执行：")
	if d.Confirm.Reason != "" {
		rt.printf("  原因：%s\n", d.Confirm.Reason)
	}
	if d.Confirm.Question != "" {
		rt.printf("  问题：%s\n", d.Confirm.Question)
	}
	if len(d.Confirm.Options) > 0 {
		rt.printf("  可选：%s\n", strings.Join(d.Confirm.Options, " / "))
	}
	for {
		rt.printf("[a]pprove / [e]dit / [r]eject: ")
		raw, ok := rt.readLine()
		if !ok {
			// 没有交互输入（stdin 已结束）：按拒绝处理，绝不空转。
			rt.println("\n没有收到决定，计划已拒绝。")
			return false, nil
		}
		choice := strings.ToLower(strings.TrimSpace(raw))
		switch choice {
		case "a", "approve", "y", "yes":
			return true, nil
		case "r", "reject", "n", "no":
			return false, nil
		case "e", "edit":
			if !d.Confirm.CanEdit() {
				rt.println("这份计划不允许编辑（editable=false）：只能批准或拒绝。")
				continue
			}
			return true, rt.promptEdits(d)
		default:
			rt.println("请输入 a（批准）、e（编辑后批准）或 r（拒绝）。")
		}
	}
}

// promptEdits 逐条询问要改的指令与要移除的 ref。
func (rt *groupRuntime) promptEdits(d plan.Decision) []plan.Edit {
	var edits []plan.Edit
	for _, dp := range d.Dispatches {
		rt.printf("  %s → @%s：%s\n", dp.Ref, dp.TargetID, firstLine(dp.Message, 120))
	}
	rt.printf("修改哪个 ref 的指令（回车跳过）: ")
	if raw, ok := rt.readLine(); ok {
		if ref := strings.TrimSpace(raw); ref != "" {
			rt.printf("新指令: ")
			if rawMsg, ok := rt.readLine(); ok {
				if msg := strings.TrimSpace(rawMsg); msg != "" {
					edits = append(edits, plan.Edit{Ref: ref, Message: &msg})
				}
			}
		}
	}
	rt.printf("移除哪个 ref（回车跳过）: ")
	if raw, ok := rt.readLine(); ok {
		if ref := strings.TrimSpace(raw); ref != "" {
			edits = append(edits, plan.Edit{Ref: ref, Remove: true})
		}
	}
	return edits
}

// promptHumanReply 就人工待办询问回复；返回 false 表示用户选择稍后处理（/reply）。
func (rt *groupRuntime) promptHumanReply(ctx context.Context, turnID string) bool {
	rt.printWaitingHuman(turnID)
	rt.printf("回复并继续请直接输入；无法处理输入 /unable [说明]；稍后处理输入 /later: ")
	raw, ok := rt.readLine()
	if !ok {
		rt.println("\n没有收到回复，待办已保留。")
		return false
	}
	line := strings.TrimSpace(raw)
	switch {
	case line == "" || line == "/later":
		rt.println("已保留待办：稍后用 /reply <说明> 或 /unable [说明] 处理。")
		return false
	case strings.HasPrefix(line, "/unable"):
		text := strings.TrimSpace(strings.TrimPrefix(line, "/unable"))
		_, taskID := rt.waitingHumanTask()
		if _, err := rt.Engine.ReplyHuman(ctx, taskID, group.Reply{Text: text, Outcome: group.ReplyUnable}); err != nil {
			rt.printf("error: %v\n", err)
		}
		return true
	default:
		_, taskID := rt.waitingHumanTask()
		if _, err := rt.Engine.ReplyHuman(ctx, taskID, group.Reply{Text: line, Outcome: group.ReplyContinue}); err != nil {
			rt.printf("error: %v\n", err)
		}
		return true
	}
}

func (rt *groupRuntime) printWaitingHuman(turnID string) {
	tasks, _ := rt.Store.LoadTasks(context.Background(), turnID)
	for _, t := range tasks {
		if t.Status == group.TaskWaitingHuman {
			rt.printf("等待你处理：%s（用 /reply <说明> 回复并继续，/unable [说明] 标记无法处理）\n", t.Message)
		}
	}
}

// readLine 从交互输入读一行；非交互（In 为 nil）或输入已结束时 ok=false。
func (rt *groupRuntime) readLine() (string, bool) {
	if rt.Runner.lineReader == nil {
		return "", false
	}
	line, err := rt.Runner.lineReader.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimRight(line, "\r\n"), true
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

// ---- 成员与候选 ----

// groupMembers 实现 group.Members：群配置、候选与近期历史。
type groupMembers struct {
	cfg     GroupConfig
	history func() []plan.Turn
}

var _ group.Members = (*groupMembers)(nil)

func (m *groupMembers) Group(context.Context, string) (group.Group, error) {
	g := group.Group{
		ID: m.cfg.Name, Policy: group.Policy{
			Confirm:       plan.ConfirmPolicy{Mode: strings.ToLower(strings.TrimSpace(m.cfg.ConfirmPolicy))},
			FallbackAgent: strings.TrimSpace(m.cfg.FallbackMember),
			MaxReplans:    m.cfg.MaxReplans,
		},
	}
	for _, member := range m.cfg.Members {
		g.MemberIDs = append(g.MemberIDs, member.ID)
	}
	if m.cfg.HumanEnabled() {
		g.HumanID = "me"
		g.HumanName = osUser()
	} else {
		g.HumanDisabled = true
	}
	return g, nil
}

func (m *groupMembers) Candidates(context.Context, string) ([]plan.Candidate, error) {
	out := make([]plan.Candidate, 0, len(m.cfg.Members))
	for _, member := range m.cfg.Members {
		out = append(out, plan.Candidate{
			ID: member.ID, Name: member.ID, Module: "member",
			Description: firstLine(member.Instruction, 200), Skills: append([]string(nil), member.Tools...),
		})
	}
	return out, nil
}

func (m *groupMembers) History(_ context.Context, _ string, limit int) ([]plan.Turn, error) {
	all := m.history()
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return append([]plan.Turn(nil), all...), nil
}

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

// ---- 任务执行 ----

// lockedWriter 串行化对底层输出的写入（结构帧与成员正文可能来自不同协程）。
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// groupRunner 实现 group.Runner：每个任务用成员自己的模型 / 指令 / 工具子集跑一个会话。
type groupRunner struct {
	env           []string
	cfg           Config
	def           GroupConfig
	client        agentcore.ChatClient
	tools         []tool.BaseTool
	planDir       string
	out           io.Writer
	in            io.Reader
	lineReader    *bufio.Reader
	maxIterations int
	warn          func(msg string, fields ...zap.Field)
	redact        func(string) string
}

var _ group.Runner = (*groupRunner)(nil)

// RunTask 执行一个 dispatch：成员会话的流式输出带 [成员] 前缀写到终端；
// 助手调用 plan(action=replan) 时上报 NeedReplan，由引擎回到栅栏。
func (r *groupRunner) RunTask(ctx context.Context, in group.TaskInput, emit func(group.Event)) (group.TaskResult, error) {
	member, ok := r.def.Member(in.AgentID)
	if !ok {
		return group.TaskResult{Failed: true, ErrorCode: "group_member_unavailable"}, nil
	}
	if r.client == nil {
		return group.TaskResult{Failed: true, ErrorCode: "model_not_configured"}, nil
	}
	memberTools, err := r.memberTools(member)
	if err != nil {
		return group.TaskResult{}, err
	}
	session, err := NewSessionWithMeta(r.env, r.cfg, "", func(e *Entry) {
		e.Group = r.def.Name
		e.Task = in.Ref
	})
	if err != nil {
		return group.TaskResult{}, err
	}
	model := strings.TrimSpace(member.Model)
	if model == "" {
		model = r.cfg.Model
	}
	// 成员前缀单独成行：结构帧（任务状态行）从跟随协程渲染，前缀与正文不能拼在同一行上。
	fmt.Fprintf(r.out, "\n[@%s]\n", member.ID)
	runner := &SessionRunner{
		Session: session, Client: r.client, Tools: memberTools, Model: model,
		MaxIterations: r.maxIterations, SystemPrompt: r.instruction(member, in),
		Warn: r.warn, Redact: r.redact,
	}
	result, runErr := runner.Run(ctx, in.Message, func(e CLIEvent) {
		switch e.Type {
		case "token":
			fmt.Fprint(r.out, e.Content)
		case "tool_call":
			fmt.Fprintf(r.out, "\n  [@%s ▶ %s(%s)] ", member.ID, e.Name, firstLine(e.Arguments, 120))
		}
	})
	fmt.Fprintln(r.out)
	out := group.TaskResult{Content: result.Content, NeedReplan: sessionRequestsReplan(session)}
	switch {
	case errors.Is(runErr, context.Canceled) || ctx.Err() != nil:
		out.Canceled = true
	case runErr != nil:
		out.Failed = true
		out.ErrorCode = "run_failed"
		out.Content = runErr.Error()
	}
	return out, nil
}

// memberTools 返回成员的工具子集（空 = 全部）加上绑定群约束的规划工具组。
func (r *groupRunner) memberTools(member GroupMember) ([]tool.BaseTool, error) {
	var base []tool.BaseTool
	if len(member.Tools) == 0 {
		base = append(base, r.tools...)
	} else {
		want := make(map[string]bool, len(member.Tools))
		for _, n := range member.Tools {
			want[n] = true
		}
		for _, t := range r.tools {
			info, err := t.Info(context.Background())
			if err == nil && info != nil && want[info.Name] {
				base = append(base, t)
			}
		}
	}
	members, _ := (&groupMembers{cfg: r.def}).Group(context.Background(), r.def.Name)
	planning := &plan.Tools{
		Store: &plan.FileStore{Dir: r.planDir},
		Constraints: func() (plan.Constraints, bool) {
			return plan.Constraints{Members: members.MemberIDs, HumanID: members.HumanID, HumanDisabled: members.HumanDisabled, Limits: plan.DefaultLimits()}, true
		},
	}
	// FileStore 首次写入时自己 MkdirAll。
	return append(base, planning.All()...), nil
}

// instruction 是成员任务的系统提示词：成员职责 + 本轮目标 + Plan 下发的指令 + 上游结果 + 近期群聊历史。
func (r *groupRunner) instruction(member GroupMember, in group.TaskInput) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(member.Instruction))
	b.WriteString("\n\nYou are member \"" + member.ID + "\" of the local group \"" + r.def.Name + "\". ")
	b.WriteString("The group's plan coordinator assigned you exactly one task; do only this task and report the result as your final answer.")
	if in.Goal != "" {
		b.WriteString("\n\nTurn goal: " + in.Goal)
	}
	b.WriteString("\n\nYour assignment from the plan:\n" + strings.TrimSpace(in.Message) + "\n")
	if ctxText := strings.TrimSpace(in.Context); ctxText != "" {
		b.WriteString("\nResults from upstream members (context):\n" + agentcore.Truncate(ctxText, 6000) + "\n")
	}
	if len(in.History) > 0 {
		var lines []string
		for _, t := range in.History {
			content := agentcore.Truncate(strings.ReplaceAll(t.Content, "\n", " "), 300)
			if t.Speaker != "" {
				lines = append(lines, fmt.Sprintf("%s(@%s): %s", t.Role, t.Speaker, content))
			} else {
				lines = append(lines, fmt.Sprintf("%s: %s", t.Role, content))
			}
		}
		b.WriteString("\nRecent group conversation:\n- " + strings.Join(lines, "\n- ") + "\n")
	}
	b.WriteString("\nIf the overall plan should change (a step cannot be done as planned, or new information changes the split), call the `plan` tool with action=replan and the new dispatches; use action=update only to record progress of your own step.")
	return b.String()
}

// sessionRequestsReplan 报告成员会话里是否有 plan(action=replan) 的工具调用。
func sessionRequestsReplan(session *Session) bool {
	for _, e := range session.Messages() {
		for _, tc := range e.ToolCalls {
			if tc.Name != plan.ToolPlan {
				continue
			}
			var input struct {
				Action string `json:"action"`
			}
			if json.Unmarshal([]byte(tc.Arguments), &input) == nil && strings.EqualFold(strings.TrimSpace(input.Action), plan.ActionReplan) {
				return true
			}
		}
	}
	return false
}
