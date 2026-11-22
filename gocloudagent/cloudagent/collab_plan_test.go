// planv6.7 v2：collab_plan.go 的纯函数测试。
// 覆盖：解析（标准/简写/代码块/越界归一/人工指代）、任务图校验（数量/越界/空指令/
// 依赖未知/依赖倒序/重复 ref/深度）、提示词拼装（群提示词注入与截断、无人工时的约束）、
// 状态归约、下一个可执行任务、阻塞传播。
package cloudagent

import (
	"errors"
	"strings"
	"testing"
)

func TestParseDispatchPlanStandard(t *testing.T) {
	raw := `{"tasks":[
		{"ref":"t1","target_kind":"agent","target_id":"a1","instruction":"查订单"},
		{"ref":"t2","target_id":"a2","instruction":"核对文件","depends_on":["t1"]},
		{"ref":"t3","target_kind":"human","target_id":"@me","instruction":"确认异常","depends_on":["t2"]}
	],"need_summary":true,"reason":"跨库与文件"}`
	plan, err := ParseDispatchPlan(raw, []string{"a1", "a2"}, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 3 || !plan.NeedSummary || plan.Reason != "跨库与文件" {
		t.Fatalf("plan: %+v", plan)
	}
	if plan.Tasks[0].TargetKind != CollabTargetAgent || plan.Tasks[0].TargetID != "a1" || plan.Tasks[0].Instruction != "查订单" {
		t.Fatalf("task1: %+v", plan.Tasks[0])
	}
	// 缺 target_kind 时按成员推断为助手
	if plan.Tasks[1].TargetKind != CollabTargetAgent {
		t.Fatalf("task2 kind: %+v", plan.Tasks[1])
	}
	if len(plan.Tasks[1].DependsOn) != 1 || plan.Tasks[1].DependsOn[0] != "t1" {
		t.Fatalf("task2 deps: %+v", plan.Tasks[1])
	}
	// "@me" 归一为当前 UserID
	if plan.Tasks[2].TargetKind != CollabTargetHuman || plan.Tasks[2].TargetID != "u1" {
		t.Fatalf("task3 human: %+v", plan.Tasks[2])
	}
}

func TestParseDispatchPlanShorthandAndCodeBlock(t *testing.T) {
	// 代码块包裹 + agent_ids 简写 → 顺序任务
	plan, err := ParseDispatchPlan("```json\n{\"agent_ids\":[\"@a1\",\"a2\"]}\n```", []string{"a1", "a2"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 2 || plan.Tasks[0].TargetID != "a1" || plan.Tasks[1].TargetID != "a2" {
		t.Fatalf("agent_ids shorthand: %+v", plan.Tasks)
	}
	if plan.Tasks[0].Ref != "t1" || plan.Tasks[1].Ref != "t2" {
		t.Fatalf("auto refs: %+v", plan.Tasks)
	}
	if plan.Tasks[0].RouteKind != CollabRouteAuto {
		t.Fatalf("shorthand route kind: %+v", plan.Tasks[0])
	}
	// 单 agent_id 简写
	single, err := ParseDispatchPlan(`{"agent_id":"a2"}`, []string{"a1", "a2"}, "")
	if err != nil || len(single.Tasks) != 1 || single.Tasks[0].TargetID != "a2" {
		t.Fatalf("agent_id shorthand: %v %+v", err, single)
	}
	// task 字段当 instruction、id 当 ref
	alt, err := ParseDispatchPlan(`{"tasks":[{"id":"x1","agent_id":"a1","task":"做事"}]}`, []string{"a1"}, "")
	if err != nil || alt.Tasks[0].Ref != "x1" || alt.Tasks[0].Instruction != "做事" {
		t.Fatalf("alt fields: %v %+v", err, alt)
	}
}

func TestParseDispatchPlanErrors(t *testing.T) {
	if _, err := ParseDispatchPlan("", nil, ""); !errors.Is(err, ErrDispatchPlanEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := ParseDispatchPlan("我认为应该选数据库助手", nil, ""); !errors.Is(err, ErrDispatchPlanInvalid) {
		t.Fatalf("garbage: %v", err)
	}
	// 纯 JSON 但没有任务
	if _, err := ParseDispatchPlan(`{"agent_id":"none"}`, []string{"a1"}, ""); !errors.Is(err, ErrDispatchPlanEmpty) {
		t.Fatalf("none: %v", err)
	}
	// 任务列表为空
	if _, err := ParseDispatchPlan(`{"tasks":[]}`, nil, ""); !errors.Is(err, ErrDispatchPlanEmpty) {
		t.Fatalf("empty tasks: %v", err)
	}
}

func TestValidateDispatchPlan(t *testing.T) {
	limits := DefaultPlanLimits()
	members := []string{"a1", "a2"}

	ok := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "一"},
		{Ref: "t2", TargetKind: CollabTargetAgent, TargetID: "a2", Instruction: "二", DependsOn: []string{"t1"}},
	}}
	if err := ValidateDispatchPlan(ok, limits, members, "", false); err != nil {
		t.Fatalf("valid plan: %v", err)
	}
	// 空任务
	if err := ValidateDispatchPlan(DispatchPlan{}, limits, members, "", false); !errors.Is(err, ErrDispatchPlanEmpty) {
		t.Fatalf("no tasks: %v", err)
	}
	// 超过任务上限
	many := DispatchPlan{}
	for i := 0; i < limits.MaxTasks+1; i++ {
		many.Tasks = append(many.Tasks, DispatchTask{
			Ref: "r" + string(rune('a'+i)), TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "x",
		})
	}
	if err := ValidateDispatchPlan(many, limits, members, "", false); !errors.Is(err, ErrDispatchPlanTooLarge) {
		t.Fatalf("too large: %v", err)
	}
	// 非成员
	nonMember := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: CollabTargetAgent, TargetID: "zz", Instruction: "x"},
	}}
	if err := ValidateDispatchPlan(nonMember, limits, members, "", false); !errors.Is(err, ErrDispatchPlanMemberMissing) {
		t.Fatalf("non member: %v", err)
	}
	// 空指令
	empty := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "   "},
	}}
	if err := ValidateDispatchPlan(empty, limits, members, "", false); !errors.Is(err, ErrDispatchPlanInvalid) {
		t.Fatalf("empty instruction: %v", err)
	}
	// 目标类型未知
	badKind := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: "robot", TargetID: "a1", Instruction: "x"},
	}}
	if err := ValidateDispatchPlan(badKind, limits, members, "", false); !errors.Is(err, ErrDispatchPlanInvalid) {
		t.Fatalf("bad kind: %v", err)
	}
	// 依赖未知
	unknownDep := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "x", DependsOn: []string{"tz"}},
	}}
	if err := ValidateDispatchPlan(unknownDep, limits, members, "", false); !errors.Is(err, ErrDispatchPlanInvalid) {
		t.Fatalf("unknown dep: %v", err)
	}
	// 依赖倒序（前向引用）
	forward := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "x", DependsOn: []string{"t2"}},
		{Ref: "t2", TargetKind: CollabTargetAgent, TargetID: "a2", Instruction: "y"},
	}}
	if err := ValidateDispatchPlan(forward, limits, members, "", false); !errors.Is(err, ErrDispatchPlanInvalid) {
		t.Fatalf("forward dep: %v", err)
	}
	// 重复 ref
	dupRef := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "x"},
		{Ref: "t1", TargetKind: CollabTargetAgent, TargetID: "a2", Instruction: "y"},
	}}
	if err := ValidateDispatchPlan(dupRef, limits, members, "", false); !errors.Is(err, ErrDispatchPlanInvalid) {
		t.Fatalf("dup ref: %v", err)
	}
	// 人工任务：未允许 / 无当前人 / 目标不是当前人
	humanTask := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: CollabTargetHuman, TargetID: "u1", Instruction: "确认"},
	}}
	if err := ValidateDispatchPlan(humanTask, limits, members, "", false); !errors.Is(err, ErrDispatchPlanHumanUnavailable) {
		t.Fatalf("human not allowed: %v", err)
	}
	if err := ValidateDispatchPlan(humanTask, limits, members, "u1", false); !errors.Is(err, ErrDispatchPlanHumanUnavailable) {
		t.Fatalf("human policy off: %v", err)
	}
	if err := ValidateDispatchPlan(humanTask, limits, members, "u2", true); !errors.Is(err, ErrDispatchPlanHumanUnavailable) {
		t.Fatalf("unknown human: %v", err)
	}
	if err := ValidateDispatchPlan(humanTask, limits, members, "u1", true); err != nil {
		t.Fatalf("valid human task: %v", err)
	}
}

func TestValidateDispatchPlanDepth(t *testing.T) {
	limits := PlanLimits{MaxDepth: 2}
	members := []string{"a1"}
	deep := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "1"},
		{Ref: "t2", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "2", DependsOn: []string{"t1"}},
		{Ref: "t3", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "3", DependsOn: []string{"t2"}},
	}}
	if err := ValidateDispatchPlan(deep, limits, members, "", false); !errors.Is(err, ErrDispatchPlanTooDeep) {
		t.Fatalf("too deep: %v", err)
	}
	// 同深度但更宽不受限
	wide := DispatchPlan{Tasks: []DispatchTask{
		{Ref: "t1", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "1"},
		{Ref: "t2", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "2", DependsOn: []string{"t1"}},
		{Ref: "t3", TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "3", DependsOn: []string{"t1"}},
	}}
	if err := ValidateDispatchPlan(wide, limits, members, "", false); err != nil {
		t.Fatalf("wide plan: %v", err)
	}
}

func TestBuildCollabDispatchPrompt(t *testing.T) {
	in := CollabDispatchInput{
		RoutingPrompt: "数据库问题先问数据库助手，再让对象存储核对文件。",
		Candidates: []CollabCandidate{
			{ID: "a1", Name: "数据库助手", Module: "database", Description: strings.Repeat("很长的描述", 80)},
			{ID: "a2", Name: "存储助手", Module: "s3", SkillHints: []string{"list_objects"}},
		},
		HumanID:   "u1",
		HumanName: "小王",
		UserText:  "订单和文件对不上",
		Mentioned: []string{"a1"},
		Recent: []CollabTurn{
			{Role: "user", Content: "上一轮问题"},
			{Role: "assistant", Speaker: "数据库助手", Content: "上一轮结论", RouteKind: CollabRouteAuto},
		},
	}
	msgs := BuildCollabDispatchPrompt(in)
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("shape: %+v", msgs)
	}
	sys := msgs[0].Content
	if !strings.Contains(sys, `"agent_id":"a1"`) || !strings.Contains(sys, `"module":"database"`) {
		t.Fatalf("system missing candidates: %s", sys)
	}
	// 描述被截断
	if strings.Contains(sys, strings.Repeat("很长的描述", 80)) {
		t.Fatal("description not truncated")
	}
	if !strings.Contains(sys, "不能改变输出格式") {
		t.Fatal("system should pin output rules against group prompt override")
	}
	user := msgs[1].Content
	if !strings.Contains(user, "群路由提示词") || !strings.Contains(user, "数据库问题先问数据库助手") {
		t.Fatalf("group prompt missing: %s", user)
	}
	if !strings.Contains(user, "小王") || !strings.Contains(user, "u1") {
		t.Fatalf("human hint missing: %s", user)
	}
	if !strings.Contains(user, "用户本轮显式点名") || !strings.Contains(user, "a1") {
		t.Fatalf("mention hint missing: %s", user)
	}
	if !strings.Contains(user, "assistant(数据库助手)") {
		t.Fatalf("recent speaker missing: %s", user)
	}
	if !strings.Contains(user, "订单和文件对不上") {
		t.Fatalf("user text missing: %s", user)
	}
	// 无当前人时明确禁止人工任务
	noHuman := BuildCollabDispatchPrompt(CollabDispatchInput{
		Candidates: []CollabCandidate{{ID: "a1", Name: "x"}},
		UserText:   "hi",
	})
	if !strings.Contains(noHuman[1].Content, "禁止生成 target_kind=human") {
		t.Fatalf("no-human constraint missing: %s", noHuman[1].Content)
	}
	// 群提示词与用户正文超长时被截断
	long := BuildCollabDispatchPrompt(CollabDispatchInput{
		RoutingPrompt: strings.Repeat("规", 9000),
		UserText:      strings.Repeat("问", 9000),
	})
	if n := len([]rune(long[1].Content)); n > 8000 {
		t.Fatalf("prompt not truncated: %d", n)
	}
}

func TestBuildMentionDispatchPlan(t *testing.T) {
	plan := BuildMentionDispatchPlan("看看", []string{"a1", "a2"}, "")
	if len(plan.Tasks) != 2 || plan.Tasks[0].DependsOn != nil {
		t.Fatalf("mention plan: %+v", plan.Tasks)
	}
	if plan.Tasks[0].RouteKind != CollabRouteMention || plan.Tasks[1].RouteKind != CollabRouteMention {
		t.Fatalf("mention route kind: %+v", plan.Tasks)
	}
	if plan.Tasks[0].Instruction != "看看" {
		t.Fatalf("instruction: %+v", plan.Tasks[0])
	}
	// 带人：追加一个人工任务
	withHuman := BuildMentionDispatchPlan("确认", []string{"a1"}, "u1")
	if len(withHuman.Tasks) != 2 || withHuman.Tasks[1].TargetKind != CollabTargetHuman || withHuman.Tasks[1].TargetID != "u1" {
		t.Fatalf("mention with human: %+v", withHuman.Tasks)
	}
	// 空输入不产生任务
	if empty := BuildMentionDispatchPlan("", nil, ""); len(empty.Tasks) != 0 {
		t.Fatalf("empty mention: %+v", empty.Tasks)
	}
}

func TestReduceTurnState(t *testing.T) {
	cases := []struct {
		name      string
		statuses  []string
		want      string
		completed int
	}{
		{"all done", []string{CollabTaskCompleted, CollabTaskCompleted}, CollabTurnCompleted, 2},
		{"waiting human wins", []string{CollabTaskCompleted, CollabTaskWaitingHuman, CollabTaskRunning}, CollabTurnWaitingHuman, 1},
		{"cancel wins", []string{CollabTaskCanceled, CollabTaskWaitingHuman}, CollabTurnCanceled, 0},
		{"running", []string{CollabTaskCompleted, CollabTaskRunning}, CollabTurnRunning, 1},
		{"pending counts as running", []string{CollabTaskPending}, CollabTurnRunning, 0},
		{"failed", []string{CollabTaskCompleted, CollabTaskFailed}, CollabTurnFailed, 1},
		{"blocked is failure", []string{CollabTaskBlocked}, CollabTurnFailed, 0},
		{"interrupted is failure", []string{CollabTaskInterrupted}, CollabTurnFailed, 0},
		{"empty", nil, CollabTurnFailed, 0},
	}
	for _, tc := range cases {
		got := ReduceTurnState(tc.statuses)
		if got.Status != tc.want || got.Completed != tc.completed || got.Total != len(tc.statuses) {
			t.Fatalf("%s: %+v", tc.name, got)
		}
		if got.Status == CollabTurnFailed && got.ErrorCode != CollabErrRoutingFailed {
			t.Fatalf("%s: error code %q", tc.name, got.ErrorCode)
		}
	}
}

func TestNextTaskIndexAndBlocked(t *testing.T) {
	depends := [][]int{{}, {0}, {0}, {1}}
	// 第一个就绪
	if idx := NextTaskIndex([]string{CollabTaskPending, CollabTaskPending, CollabTaskPending, CollabTaskPending}, depends); idx != 0 {
		t.Fatalf("first ready: %d", idx)
	}
	// 完成 t0 后，t1/t2 都就绪（取最小索引）
	statuses := []string{CollabTaskCompleted, CollabTaskPending, CollabTaskPending, CollabTaskPending}
	if idx := NextTaskIndex(statuses, depends); idx != 1 {
		t.Fatalf("after t0: %d", idx)
	}
	// t3 需要 t1 完成
	statuses = []string{CollabTaskCompleted, CollabTaskCompleted, CollabTaskPending, CollabTaskPending}
	if idx := NextTaskIndex(statuses, depends); idx != 2 {
		t.Fatalf("pick t2: %d", idx)
	}
	statuses = []string{CollabTaskCompleted, CollabTaskCompleted, CollabTaskCompleted, CollabTaskPending}
	if idx := NextTaskIndex(statuses, depends); idx != 3 {
		t.Fatalf("pick t3: %d", idx)
	}
	// 全部非 pending → -1
	if idx := NextTaskIndex([]string{CollabTaskCompleted, CollabTaskCompleted, CollabTaskCompleted, CollabTaskCompleted}, depends); idx != -1 {
		t.Fatalf("no pending: %d", idx)
	}
	// 上游失败 → 下游被阻塞
	statuses = []string{CollabTaskFailed, CollabTaskPending, CollabTaskPending, CollabTaskPending}
	blocked := BlockedTasks(statuses, depends)
	if len(blocked) != 2 || blocked[0] != 1 || blocked[1] != 2 {
		t.Fatalf("blocked: %+v", blocked)
	}
	// 无上游失败则没有阻塞
	if got := BlockedTasks([]string{CollabTaskCompleted, CollabTaskPending, CollabTaskPending, CollabTaskPending}, depends); len(got) != 0 {
		t.Fatalf("no blocked: %+v", got)
	}
}

func TestHumanizeTask(t *testing.T) {
	agent := HumanizeTask(DispatchTask{TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: "查订单"})
	if agent != "@a1：查订单" {
		t.Fatalf("agent: %s", agent)
	}
	human := HumanizeTask(DispatchTask{TargetKind: CollabTargetHuman, TargetID: "u1", Instruction: "确认"})
	if human != "请你确认：确认" {
		t.Fatalf("human: %s", human)
	}
	// 超长指令被截断
	long := HumanizeTask(DispatchTask{TargetKind: CollabTargetAgent, TargetID: "a1", Instruction: strings.Repeat("长", 200)})
	if len([]rune(long)) > 90 {
		t.Fatalf("not truncated: %d", len([]rune(long)))
	}
}

func TestPlanLimitsNormalized(t *testing.T) {
	got := PlanLimits{}.normalized()
	def := DefaultPlanLimits()
	if got.MaxTasks != def.MaxTasks || got.MaxDepth != def.MaxDepth || got.MaxInstruction != def.MaxInstruction {
		t.Fatalf("defaults: %+v", got)
	}
	// 超上限被收回默认值，避免配置写错后无限任务
	if n := (PlanLimits{MaxTasks: 9999}).normalized(); n.MaxTasks != def.MaxTasks {
		t.Fatalf("clamp max tasks: %+v", n)
	}
	if n := (PlanLimits{MaxTasks: 3, MaxDepth: 2}).normalized(); n.MaxTasks != 3 || n.MaxDepth != 2 {
		t.Fatalf("keep explicit: %+v", n)
	}
}
