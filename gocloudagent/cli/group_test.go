// 文件说明：group_test.go — 本地群模式（planv6.8 §13.8 CLI 落地）：群定义、文件存储、引擎端到端
//
// 覆盖：
//   - 群定义：脚手架不覆盖、校验（成员 id / 指令 / 兜底成员 / 策略）、列表；
//   - 文件存储：幂等键复用、非法迁移报错、任务 CAS、事件 seq 单调、占用位、重开恢复、未终态列表；
//   - 端到端：单成员规则短路（不调模型）→ 成员会话执行 → 轮次完成；双成员经 Planner 的 plan 工具分工 →
//     按依赖顺序执行 → 计划文件落盘；confirm_policy=always 在 REPL 里 [a]pprove；人工任务就地回复；
//     headless `group run` 的 --yes / 默认拒绝；成员会话带 group / task 元数据。

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/gocloudagent/tools"
)

func sampleGroup(name string, members ...GroupMember) GroupConfig {
	if len(members) == 0 {
		members = []GroupMember{{ID: "assistant", Instruction: "You are a careful assistant."}}
	}
	return GroupConfig{Name: name, FallbackMember: members[0].ID, Members: members}
}

func TestGroupConfigScaffoldLoadValidate(t *testing.T) {
	env := testEnv(t)
	path, created, err := ScaffoldGroup(env, "review")
	if err != nil || !created {
		t.Fatalf("scaffold: created=%v err=%v", created, err)
	}
	if _, createdAgain, err := ScaffoldGroup(env, "review"); err != nil || createdAgain {
		t.Fatalf("scaffold must not overwrite: created=%v err=%v", createdAgain, err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("group file mode: %v %v", st.Mode().Perm(), err)
	}
	def, err := LoadGroup(env, "review")
	if err != nil || def.Name != "review" || len(def.Members) != 1 || !def.HumanEnabled() {
		t.Fatalf("load: %+v err=%v", def, err)
	}
	names, err := ListGroups(env)
	if err != nil || len(names) != 1 || names[0] != "review" {
		t.Fatalf("list: %v %v", names, err)
	}
	if _, err := LoadGroup(env, "missing"); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("missing group: %v", err)
	}
	if _, err := LoadGroup(env, "Bad Name"); !errors.Is(err, ErrGroupInvalid) {
		t.Fatalf("bad name: %v", err)
	}
	for name, cfg := range map[string]GroupConfig{
		"no members":     {Name: "g"},
		"dup id":         sampleGroup("g", GroupMember{ID: "a", Instruction: "x"}, GroupMember{ID: "a", Instruction: "y"}),
		"no instruction": sampleGroup("g", GroupMember{ID: "a"}),
		"bad policy":     {Name: "g", ConfirmPolicy: "sometimes", Members: []GroupMember{{ID: "a", Instruction: "x"}}},
		"bad fallback":   {Name: "g", FallbackMember: "ghost", Members: []GroupMember{{ID: "a", Instruction: "x"}}},
	} {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestGroupStoreTransitionsAndPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := openGroupStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	turn, reused, err := s.CreateTurn(ctx, group.Turn{ID: "t1", ThreadID: "local", Status: group.TurnPlanning, IdempotencyKey: "k1"})
	if err != nil || reused || turn.Version != 1 {
		t.Fatalf("create: reused=%v %+v err=%v", reused, turn, err)
	}
	if _, reused, err = s.CreateTurn(ctx, group.Turn{ID: "t2", ThreadID: "local", IdempotencyKey: "k1"}); err != nil || !reused {
		t.Fatalf("idempotency: reused=%v err=%v", reused, err)
	}
	if err := s.TransitionTurn(ctx, "t1", group.TurnRunning, group.TurnCompleted, ""); !errors.Is(err, group.ErrIllegalTransition) {
		t.Fatalf("illegal turn transition must fail: %v", err)
	}
	if err := s.TransitionTurn(ctx, "t1", group.TurnPlanning, group.TurnRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTasks(ctx, "t1", []group.Task{{ID: "t1-d1", Ref: "d1", Status: group.TaskPending}, {ID: "t1-d2", Ref: "d2", Status: group.TaskPending}}); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionTask(ctx, "t1-d1", group.TaskPending, group.TaskRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.TransitionTask(ctx, "t1-d1", group.TaskPending, group.TaskCompleted, "", ""); !errors.Is(err, group.ErrIllegalTransition) {
		t.Fatal("stale task transition must fail")
	}
	if err := s.TransitionTask(ctx, "t1-d1", group.TaskRunning, group.TaskCompleted, "result", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTaskRunID(ctx, "t1-d1", "run-1"); err != nil {
		t.Fatal(err)
	}
	e1, _ := s.AppendEvent(ctx, "t1", group.Event{Type: group.EventTask, TaskID: "t1-d1"})
	e2, _ := s.AppendEvent(ctx, "t1", group.Event{Type: group.EventEnd})
	if e1.Seq != 1 || e2.Seq != 2 {
		t.Fatalf("seq: %d %d", e1.Seq, e2.Seq)
	}
	if err := s.Claim(ctx, "local", "t1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim(ctx, "local", "other"); !errors.Is(err, group.ErrTurnBusy) {
		t.Fatalf("claim must be exclusive: %v", err)
	}
	if err := s.SaveDecision(ctx, "t1", plan.Decision{Goal: "g"}, ""); err != nil {
		t.Fatal(err)
	}
	// 重开：状态来自文件；占用位随进程重启失效。
	s2, err := openGroupStore(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok, _ := s2.LoadTurn(ctx, "t1")
	if !ok || loaded.Status != group.TurnRunning || loaded.Decision.Goal != "g" || loaded.Version != 2 {
		t.Fatalf("reloaded turn: ok=%v %+v", ok, loaded)
	}
	tasks, _ := s2.LoadTasks(ctx, "t1")
	if len(tasks) != 2 || tasks[0].Status != group.TaskCompleted || tasks[0].Result != "result" || tasks[0].RunID != "run-1" || tasks[1].Status != group.TaskPending {
		t.Fatalf("reloaded tasks: %+v", tasks)
	}
	events, _ := s2.ListEvents(ctx, "t1", 1)
	if len(events) != 1 || events[0].Type != group.EventEnd {
		t.Fatalf("events after=1: %+v", events)
	}
	if err := s2.Claim(ctx, "local", "t2"); err != nil {
		t.Fatalf("claims must not survive a restart: %v", err)
	}
	unfinished, _ := s2.ListUnfinishedTurns(ctx)
	if len(unfinished) != 1 || unfinished[0].ID != "t1" {
		t.Fatalf("unfinished: %+v", unfinished)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode: %v %v", st.Mode().Perm(), err)
	}
}

// plannerReply 是栅栏的 plan 工具调用（native tool call）。
func plannerReply(goal string, dispatches ...plan.Dispatch) agentcore.ChatResponse {
	args, _ := json.Marshal(map[string]any{"action": "create", "goal": goal, "dispatches": dispatches})
	return agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{ID: "plan-1", Name: plan.ToolPlan, Arguments: string(args)}}}
}

func newTestGroupRuntime(t *testing.T, env []string, def GroupConfig, client agentcore.ChatClient, stdin string) (*groupRuntime, *strings.Builder) {
	t.Helper()
	out := &strings.Builder{}
	// 引擎的后台执行循环在 end 帧之后还会写一次状态（释放占用位）：先等它落定，再让 TempDir 清理。
	// Cleanup 按 LIFO 执行，这个先于 testEnv 注册的 TempDir 清理。
	t.Cleanup(func() { time.Sleep(200 * time.Millisecond) })
	rt, err := newGroupRuntime(env, Config{}, def, client, nil, groupRuntimeOptions{Out: out, In: strings.NewReader(stdin), MaxIterations: 4})
	if err != nil {
		t.Fatal(err)
	}
	return rt, out
}

// 单成员群：规则短路不调模型；成员任务用自己的会话执行；计划文件落在 groups/<name>/plan/。
func TestGroupRuntimeSingleMemberRuleShortcut(t *testing.T) {
	env := testEnv(t)
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "成员回复"}}}
	rt, out := newTestGroupRuntime(t, env, sampleGroup("solo"), client, "")
	code := rt.submit(context.Background(), "整理一下 README", true, false)
	if code != exitOK {
		t.Fatalf("code = %d\n%s", code, out.String())
	}
	if len(client.requests) != 1 {
		t.Fatalf("single member must short-circuit the planner; requests = %d", len(client.requests))
	}
	if req := client.requests[0]; !strings.Contains(req.Messages[len(req.Messages)-1].Content, "整理一下 README") ||
		!strings.Contains(req.Messages[0].Content, "member \"assistant\"") {
		t.Fatalf("member request: %+v", req.Messages)
	}
	for _, want := range []string{"目标：整理一下 README", "任务 d1 → @assistant", "[@assistant]", "成员回复", "结束（done）"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	turns, _ := rt.Store.ListUnfinishedTurns(context.Background())
	if len(turns) != 0 {
		t.Fatalf("turn must be completed: %+v", turns)
	}
	if _, err := os.Stat(filepath.Join(GroupStateDir(env, "solo"), "plan", "plan.md")); err != nil {
		t.Fatalf("plan.md must be persisted: %v", err)
	}
	// 成员会话带 group / task 元数据，落在 sessions/ 里可单独 resume。
	sessions, err := ListSessions(env)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions: %v %v", sessions, err)
	}
	sess, err := OpenSession(filepath.Join(HomeDir(env), "sessions", sessions[0].ID+".jsonl"))
	if err != nil || sess.Meta.Group != "solo" || sess.Meta.Task != "d1" {
		t.Fatalf("member session meta: %+v err=%v", sess.Meta, err)
	}
	// 历史进入下一轮的栅栏上下文。
	if len(rt.history) != 2 || rt.history[0].Role != "user" || rt.history[1].Speaker != "assistant" {
		t.Fatalf("history: %+v", rt.history)
	}
}

// 双成员：Planner 以 plan 工具分工，按依赖顺序执行；每个成员各自的会话与指令。
func TestGroupRuntimePlannedTwoMembersInOrder(t *testing.T) {
	env := testEnv(t)
	def := sampleGroup("duo",
		GroupMember{ID: "explorer", Instruction: "You explore."},
		GroupMember{ID: "writer", Instruction: "You write.", Tools: []string{"read"}},
	)
	client := &scriptedClient{replies: []agentcore.ChatResponse{
		plannerReply("ship it",
			plan.Dispatch{Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "explorer", Message: "先摸清结构"},
			plan.Dispatch{Ref: "d2", TargetKind: plan.TargetAgent, TargetID: "writer", Message: "再写文档", DependsOn: []string{"d1"}, ContextFrom: []string{"d1"}},
		),
		{Content: "结构已摸清"},
		{Content: "文档写好了"},
	}}
	// 依赖深度 ≥ 2 会触发整份计划确认：终端输入 a 批准。
	rt, out := newTestGroupRuntime(t, env, def, client, "a\n")
	code := rt.submit(context.Background(), "把这个项目整理成文档", true, false)
	if code != exitOK {
		t.Fatalf("code = %d\n%s", code, out.String())
	}
	if len(client.requests) != 3 {
		t.Fatalf("requests = %d (planner + 2 members)", len(client.requests))
	}
	first := strings.Join(msgTexts(client.requests[1].Messages), "\n")
	second := strings.Join(msgTexts(client.requests[2].Messages), "\n")
	if !strings.Contains(first, "member \"explorer\"") || !strings.Contains(first, "先摸清结构") {
		t.Fatalf("explorer prompt: %s", first)
	}
	if !strings.Contains(second, "member \"writer\"") || !strings.Contains(second, "再写文档") || !strings.Contains(second, "结构已摸清") {
		t.Fatalf("writer prompt must carry its instruction and upstream context: %s", second)
	}
	// 本运行时没有本机工具：两个成员都只拿到规划工具组（工具子集过滤见 TestGroupRunnerMemberTools）。
	for i, req := range client.requests[1:] {
		if names := toolNames(req.Tools); !contains(names, "plan") || contains(names, "read") {
			t.Fatalf("member %d tools: %v", i, names)
		}
	}
	for _, want := range []string{"任务 d1 → @explorer：先摸清结构", "任务 d2 → @writer：再写文档（依赖 d1）", "[@explorer]", "结构已摸清", "[@writer]", "文档写好了"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Index(out.String(), "结构已摸清") > strings.Index(out.String(), "文档写好了") {
		t.Fatal("dependent task must run after its upstream")
	}
	planMD, err := os.ReadFile(filepath.Join(GroupStateDir(env, "duo"), "plan", "plan.md"))
	if err != nil || !strings.Contains(string(planMD), "ship it") || !strings.Contains(string(planMD), "@writer") {
		t.Fatalf("plan.md: %v %s", err, planMD)
	}
}

func msgTexts(msgs []agentcore.ChatMessage) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Content)
	}
	return out
}

func toolNames(specs []agentcore.ToolSpec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Name)
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// confirm_policy=always：REPL 里整份计划要先 [a]pprove 才执行；拒绝则不执行任何任务。
func TestGroupRuntimePlanConfirmInteractive(t *testing.T) {
	env := testEnv(t)
	def := sampleGroup("gated",
		GroupMember{ID: "a", Instruction: "A."},
		GroupMember{ID: "b", Instruction: "B."},
	)
	def.ConfirmPolicy = plan.ConfirmAlways
	t.Run("approve", func(t *testing.T) {
		client := &scriptedClient{replies: []agentcore.ChatResponse{
			plannerReply("g", plan.Dispatch{Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Message: "mA"}),
			{Content: "done a"},
		}}
		rt, out := newTestGroupRuntime(t, env, def, client, "a\n")
		if code := rt.submit(context.Background(), "go", true, false); code != exitOK {
			t.Fatalf("code = %d\n%s", code, out.String())
		}
		for _, want := range []string{"需要你确认", "[a]pprove / [e]dit / [r]eject:", "计划确认：approved", "[@a]", "done a"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("output missing %q:\n%s", want, out.String())
			}
		}
	})
	t.Run("reject", func(t *testing.T) {
		client := &scriptedClient{replies: []agentcore.ChatResponse{
			plannerReply("g", plan.Dispatch{Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Message: "mA"}),
		}}
		rt, out := newTestGroupRuntime(t, env, def, client, "r\n")
		if code := rt.submit(context.Background(), "go", true, false); code != exitUsage {
			t.Fatalf("rejected turn code = %d", code)
		}
		if strings.Contains(out.String(), "[@a]") || len(client.requests) != 1 {
			t.Fatalf("no member may run after rejection:\n%s", out.String())
		}
		turns, _ := rt.recentTurns(1)
		if len(turns) != 1 || turns[0].Status != group.TurnRejected {
			t.Fatalf("turn: %+v", turns)
		}
	})
	t.Run("edit then approve", func(t *testing.T) {
		client := &scriptedClient{replies: []agentcore.ChatResponse{
			plannerReply("g",
				plan.Dispatch{Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Message: "mA"},
				plan.Dispatch{Ref: "d2", TargetKind: plan.TargetAgent, TargetID: "b", Message: "mB", DependsOn: []string{"d1"}},
			),
			{Content: "done a"},
		}}
		// e → 改 d1 的指令 → 移除 d2 → 批准。
		rt, out := newTestGroupRuntime(t, env, def, client, "e\nd1\n rewritten for a\nd2\n")
		if code := rt.submit(context.Background(), "go", true, false); code != exitOK {
			t.Fatalf("code = %d\n%s", code, out.String())
		}
		if len(client.requests) != 2 || !strings.Contains(strings.Join(msgTexts(client.requests[1].Messages), "\n"), "rewritten for a") {
			t.Fatalf("edited instruction must reach the member: %d requests", len(client.requests))
		}
		if strings.Contains(out.String(), "[@b]") || strings.Contains(out.String(), "任务 d1 → @b") {
			t.Fatalf("removed dispatch must not run:\n%s", out.String())
		}
	})
}

// 人工任务：轮次停下等人，REPL 就地回复后继续执行下游任务；/unable 则把任务标记失败。
func TestGroupRuntimeHumanTaskReply(t *testing.T) {
	env := testEnv(t)
	def := sampleGroup("human",
		GroupMember{ID: "a", Instruction: "A."},
		GroupMember{ID: "b", Instruction: "B."},
	)
	plans := []plan.Dispatch{
		{Ref: "d1", TargetKind: plan.TargetHuman, TargetID: "me", Message: "确认要删除哪张表"},
		{Ref: "d2", TargetKind: plan.TargetAgent, TargetID: "a", Message: "按确认结果执行", DependsOn: []string{"d1"}, ContextFrom: []string{"d1"}},
	}
	t.Run("continue inline", func(t *testing.T) {
		client := &scriptedClient{replies: []agentcore.ChatResponse{plannerReply("g", plans...), {Content: "done"}}}
		rt, out := newTestGroupRuntime(t, env, def, client, "a\n删除 orders\n")
		if code := rt.submit(context.Background(), "清理数据", true, false); code != exitOK {
			t.Fatalf("code = %d\n%s", code, out.String())
		}
		for _, want := range []string{"需要你处理：确认要删除哪张表", "回复并继续请直接输入", "[@a]", "done"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("output missing %q:\n%s", want, out.String())
			}
		}
		if !strings.Contains(strings.Join(msgTexts(client.requests[1].Messages), "\n"), "删除 orders") {
			t.Fatalf("human reply must flow into the dependent task: %v", msgTexts(client.requests[1].Messages))
		}
	})
	t.Run("later then /reply", func(t *testing.T) {
		client := &scriptedClient{replies: []agentcore.ChatResponse{plannerReply("g", plans...), {Content: "done"}}}
		rt, out := newTestGroupRuntime(t, env, def, client, "a\n/later\n")
		if code := rt.submit(context.Background(), "清理数据", true, false); code != exitOK {
			t.Fatalf("code = %d\n%s", code, out.String())
		}
		turns, _ := rt.recentTurns(1)
		if turns[0].Status != group.TurnWaitingHuman || len(client.requests) != 1 {
			t.Fatalf("turn must wait: %+v requests=%d", turns[0], len(client.requests))
		}
		if _, taskID := rt.waitingHumanTask(); taskID == "" {
			t.Fatal("waiting task must be discoverable")
		}
		if err := rt.reply(context.Background(), group.ReplyContinue, "删除 orders"); err != nil {
			t.Fatal(err)
		}
		if len(client.requests) != 2 || !strings.Contains(out.String(), "done") {
			t.Fatalf("reply must resume the turn:\n%s", out.String())
		}
	})
	t.Run("unable fails the task", func(t *testing.T) {
		client := &scriptedClient{replies: []agentcore.ChatResponse{plannerReply("g", plans...)}}
		rt, _ := newTestGroupRuntime(t, env, def, client, "a\n/unable 不确定\n")
		code := rt.submit(context.Background(), "清理数据", true, false)
		turns, _ := rt.recentTurns(1)
		if turns[0].Status != group.TurnFailed || code != exitAPI {
			t.Fatalf("unable must fail the turn: %+v code=%d", turns[0], code)
		}
		if len(client.requests) != 1 {
			t.Fatal("downstream task must stay blocked")
		}
	})
}

// headless：`group run` 需要确认的计划不带 --yes 时按策略拒绝并记录；--yes 自动批准；人工任务只提示不等待。
func TestGroupRunHeadless(t *testing.T) {
	env := testEnv(t)
	def := sampleGroup("headless",
		GroupMember{ID: "a", Instruction: "A."},
		GroupMember{ID: "b", Instruction: "B."},
	)
	def.ConfirmPolicy = plan.ConfirmAlways
	if _, err := SaveGroup(env, def); err != nil {
		t.Fatal(err)
	}
	run := func(client *scriptedClient, message string, yes bool) (int, string, string) {
		read, out, errw := captureStdio(t)
		_ = read
		opts := Options{Env: env, Headless: true, Stdout: out, Stderr: errw}
		code := runGroupHeadless(opts, Config{}, def, client, nil, message, yes)
		outb, _ := os.ReadFile(out.Name())
		errb, _ := os.ReadFile(errw.Name())
		return code, string(outb), string(errb)
	}
	plans := []plan.Dispatch{{Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a", Message: "mA"}}
	code, out, _ := run(&scriptedClient{replies: []agentcore.ChatResponse{plannerReply("g", plans...)}}, "go", false)
	if code != exitUsage || !strings.Contains(out, "headless 下按策略拒绝") {
		t.Fatalf("headless without --yes must reject: code=%d\n%s", code, out)
	}
	code, out, _ = run(&scriptedClient{replies: []agentcore.ChatResponse{plannerReply("g", plans...), {Content: "done a"}}}, "go", true)
	if code != exitOK || !strings.Contains(out, "--yes") || !strings.Contains(out, "done a") {
		t.Fatalf("--yes must approve and run: code=%d\n%s", code, out)
	}
	human := []plan.Dispatch{{Ref: "d1", TargetKind: plan.TargetHuman, TargetID: "me", Message: "请确认"}}
	code, out, _ = run(&scriptedClient{replies: []agentcore.ChatResponse{plannerReply("g", human...)}}, "go", true)
	if code != exitOK || !strings.Contains(out, "等待你处理：请确认") {
		t.Fatalf("human task must be reported, not waited on: code=%d\n%s", code, out)
	}
	// 命令面：缺少消息是用法错误；群不存在也是用法错误（提示去 new）。
	_, out2, errw2 := captureStdio(t)
	if code := cmdGroup(env, []string{"run", "headless"}, out2, errw2); code != exitUsage {
		t.Fatalf("run without message: %d", code)
	}
	if code := cmdGroup(env, []string{"run", "ghost", "hi"}, out2, errw2); code != exitUsage {
		t.Fatalf("unknown group: %d", code)
	}
	showb, _ := os.ReadFile(out2.Name())
	if code := cmdGroup(env, []string{"show", "headless"}, out2, errw2); code != exitOK {
		errb, _ := os.ReadFile(errw2.Name())
		t.Fatalf("show: %d %s", code, errb)
	}
	if code := cmdGroup(env, []string{"list"}, out2, errw2); code != exitOK || !strings.Contains(string(showb)+func() string { b, _ := os.ReadFile(out2.Name()); return string(b) }(), "headless") {
		t.Fatalf("list: %d", code)
	}
}

// 进程重启后的收敛：残留的 running 轮次在重新装配时标记为 interrupted，可 resume。
func TestGroupRuntimeConvergeAndResume(t *testing.T) {
	env := testEnv(t)
	def := sampleGroup("restart")
	// 先造一个停在 running 的轮次。
	if err := os.MkdirAll(GroupStateDir(env, def.Name), 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := openGroupStore(filepath.Join(GroupStateDir(env, def.Name), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, err := s.CreateTurn(ctx, group.Turn{ID: "t-old", ThreadID: groupLocalThread, Status: group.TurnRunning, UserText: "旧任务"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTasks(ctx, "t-old", []group.Task{{ID: "t-old-d1", Ref: "d1", TargetKind: "agent", TargetID: "assistant", Message: "旧指令", Status: group.TaskRunning}}); err != nil {
		t.Fatal(err)
	}
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "补跑完成"}}}
	rt, out := newTestGroupRuntime(t, env, def, client, "")
	old, _, _ := rt.Store.LoadTurn(ctx, "t-old")
	if old.Status != group.TurnInterrupted {
		t.Fatalf("stale running turn must converge to interrupted: %+v", old)
	}
	if !rt.resume(ctx, true) {
		t.Fatal("resume must pick the interrupted turn")
	}
	if !strings.Contains(out.String(), "补跑完成") || len(client.requests) != 1 {
		t.Fatalf("resumed task must run:\n%s", out.String())
	}
}

func TestMentionTokensAndStrip(t *testing.T) {
	got := mentionTokens("请 @explorer 和 @Writer 一起看，@me 确认 @not-a-member")
	if strings.Join(got, ",") != "explorer,writer,me,not-a-member" {
		t.Fatalf("tokens = %v", got)
	}
	if stripMentions("请 @explorer 看一下") != "请  看一下" && strings.TrimSpace(stripMentions("请 @explorer 看一下")) != "请 看一下" {
		t.Fatalf("strip = %q", stripMentions("请 @explorer 看一下"))
	}
}

// 成员工具子集：tools 为空拿全部；非空只拿点名的；规划工具组总是追加。
func TestGroupRunnerMemberTools(t *testing.T) {
	mk := func(name string) tool.BaseTool {
		tl, err := tools.Exec(name, name+" tool", func(context.Context, map[string]any) (any, error) { return "ok", nil })
		if err != nil {
			t.Fatal(err)
		}
		return tl
	}
	def := sampleGroup("tools",
		GroupMember{ID: "all", Instruction: "x"},
		GroupMember{ID: "ro", Instruction: "y", Tools: []string{"read", "glob"}},
	)
	r := &groupRunner{def: def, tools: []tool.BaseTool{mk("read"), mk("write"), mk("glob")}, planDir: t.TempDir()}
	all, err := r.memberTools(def.Members[0])
	if err != nil {
		t.Fatal(err)
	}
	if names := toolInfos(all); strings.Join(names, ",") != "read,write,glob,plan,explore,reflect" {
		t.Fatalf("all tools: %v", names)
	}
	ro, err := r.memberTools(def.Members[1])
	if err != nil {
		t.Fatal(err)
	}
	if names := toolInfos(ro); strings.Join(names, ",") != "read,glob,plan,explore,reflect" {
		t.Fatalf("subset tools: %v", names)
	}
}

func toolInfos(list []tool.BaseTool) []string {
	out := make([]string, 0, len(list))
	for _, t := range list {
		info, err := t.Info(context.Background())
		if err == nil && info != nil {
			out = append(out, info.Name)
		}
	}
	return out
}

// REPL 集成：/group 列出 / 进入 / 发消息 / status / off；退出群模式后回到单助手会话。
func TestREPLGroupMode(t *testing.T) {
	env := testEnv(t)
	if _, err := SaveGroup(env, sampleGroup("solo")); err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(env, Config{}, "")
	if err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	opts := Options{
		Env: env, Stdout: &out, Stderr: &errb,
		Stdin: strings.NewReader("/group\n/group solo\n看看 README\n/group status\n/group off\n再问我一次\n/exit\n"),
	}
	client := &scriptedClient{replies: []agentcore.ChatResponse{{Content: "成员回复"}, {Content: "单助手回复"}}}
	runtime := &SessionRunner{Session: s, Client: client, MaxIterations: 4}
	if code := runREPL(opts, Config{}, runtime, s); code != exitOK {
		t.Fatalf("code = %d\n%s\n%s", code, out.String(), errb.String())
	}
	for _, want := range []string{"可用的群（/group <name> 进入）：solo", "已进入群 solo（成员：@assistant）", "[@assistant]", "成员回复", "群 solo · 最近一轮", "已退出群 solo", "单助手回复"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	// 第一条走群（成员会话），第二条走单助手会话（本会话）。
	if len(client.requests) != 2 || !strings.Contains(strings.Join(msgTexts(client.requests[0].Messages), "\n"), "member \"assistant\"") ||
		strings.Contains(strings.Join(msgTexts(client.requests[1].Messages), "\n"), "member \"") {
		t.Fatalf("requests: %v", client.requests)
	}
	if !strings.Contains(replHelp(nil), "/group") {
		t.Fatal("help must mention /group")
	}
}
