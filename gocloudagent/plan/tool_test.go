// 文件说明：tool_test.go — plan / explore / reflect 工具参数解码、mergeUpdate 与往返（planv6.8 §13.11.1-4）

package plan

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

func runTool(t *testing.T, tl tool.BaseTool, args string) string {
	t.Helper()
	inv, ok := tl.(interface {
		InvokableRun(context.Context, string, ...tool.Option) (string, error)
	})
	if !ok {
		t.Fatalf("tool %T not invokable", tl)
	}
	out, err := inv.InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	return out
}

// plan 工具：严格模式（带 Constraints）落库并回调；校验失败返回错误信封。
func TestPlanToolStrictRoundtrip(t *testing.T) {
	var gotOn Decision
	store := &MemoryStore{}
	tools := &Tools{
		Store: store,
		Constraints: func() (Constraints, bool) {
			return baseConstraints(), true
		},
		OnDecision: func(_ context.Context, d Decision) { gotOn = d },
	}
	pt := &planTool{tools}
	out := runTool(t, pt, `{
		"action":"create","goal":"goal g","complexity":"simple",
		"dispatches":[{"ref":"d1","target_kind":"agent","target_id":"a1","message":"m"}],
		"progress":{"percent":10,"summary":"started"}
	}`)
	if !strings.Contains(out, `"ok":true`) || !strings.Contains(out, `"dispatches":1`) {
		t.Fatalf("out = %s", out)
	}
	saved, ok, _ := store.LoadDecision(context.Background())
	if !ok || saved.Goal != "goal g" || saved.Source != SourceModel {
		t.Fatalf("saved: ok=%v %+v", ok, saved)
	}
	if gotOn.Goal != "goal g" {
		t.Fatalf("OnDecision not called: %+v", gotOn)
	}
	// progress 时间线落一行。
	if lines := store.Progress(); len(lines) != 1 || !strings.Contains(lines[0], "started") {
		t.Fatalf("progress = %v", lines)
	}

	// 校验失败（非成员）→ 错误信封，不落库。
	out = runTool(t, pt, `{"action":"create","goal":"g",
		"dispatches":[{"target_kind":"agent","target_id":"ghost","message":"m"}]}`)
	if !strings.Contains(out, "agent_not_in_tag") {
		t.Fatalf("non-member envelope: %s", out)
	}
	if _, ok, _ := store.LoadDecision(context.Background()); !ok {
		t.Fatal("failed decision must not overwrite the stored one")
	}
}

// 非严格模式（无 Constraints）：只做结构校验。
func TestPlanToolStructureOnly(t *testing.T) {
	store := &MemoryStore{}
	pt := &planTool{&Tools{Store: store}}
	out := runTool(t, pt, `{"action":"create","goal":"g",
		"dispatches":[{"target_kind":"agent","target_id":"anything","message":"m"}]}`)
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("structure-only accepts arbitrary ids: %s", out)
	}
	out = runTool(t, pt, `{"action":"create","goal":""}`)
	if !strings.Contains(out, "invalid_plan") {
		t.Fatalf("missing goal: %s", out)
	}
}

// update 叠加到上一份决策：分工不变；complete 收尾。
func TestPlanToolMergeUpdate(t *testing.T) {
	store := &MemoryStore{}
	pt := &planTool{&Tools{Store: store}}
	if out := runTool(t, pt, `{"action":"update","progress":{"percent":50}}`); !strings.Contains(out, "no_plan") {
		t.Fatalf("update without create: %s", out)
	}
	runTool(t, pt, `{"action":"create","goal":"g",
		"dispatches":[{"ref":"d1","target_kind":"agent","target_id":"a1","message":"m"}],
		"steps":[{"id":"s1","title":"step","status":"pending"}]}`)
	out := runTool(t, pt, `{"action":"update",
		"steps":[{"id":"s1","status":"done","evidence":"done it"}],
		"progress":{"percent":50,"summary":"half"},"findings":["learned"]}`)
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("update: %s", out)
	}
	d, _, _ := store.LoadDecision(context.Background())
	if len(d.Dispatches) != 1 || d.Steps[0].Status != StepDone || d.Steps[0].Evidence != "done it" {
		t.Fatalf("merged: %+v", d)
	}
	if d.Progress.Percent != 50 || len(d.Findings) != 1 {
		t.Fatalf("progress/findings: %+v", d)
	}
	// update 试图改分工 → 必须用 replan。
	out = runTool(t, pt, `{"action":"update","dispatches":[{"ref":"d1","target_kind":"agent","target_id":"a1","message":"changed"}]}`)
	if !strings.Contains(out, "invalid_plan") {
		t.Fatalf("update must not change dispatches: %s", out)
	}
	// complete：进度归 100，未完成步骤收尾。
	out = runTool(t, pt, `{"action":"complete"}`)
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("complete: %s", out)
	}
	d, _, _ = store.LoadDecision(context.Background())
	if d.Action != ActionComplete || d.Progress.Percent != 100 || d.Steps[0].Status != StepDone {
		t.Fatalf("completed: %+v", d)
	}
}

func TestPlanToolStrictUpdateAndComplete(t *testing.T) {
	store := &MemoryStore{}
	pt := &planTool{&Tools{Store: store, Constraints: func() (Constraints, bool) {
		return baseConstraints(), true
	}}}
	runTool(t, pt, `{"action":"create","goal":"g","dispatches":[{"ref":"d1","target_kind":"agent","target_id":"a1","message":"m"}]}`)
	if out := runTool(t, pt, `{"action":"update","progress":{"percent":50}}`); !strings.Contains(out, `"ok":true`) {
		t.Fatalf("strict update: %s", out)
	}
	if out := runTool(t, pt, `{"action":"complete"}`); !strings.Contains(out, `"ok":true`) {
		t.Fatalf("strict complete: %s", out)
	}
	d, ok, err := store.LoadDecision(context.Background())
	if err != nil || !ok || d.Action != ActionComplete || len(d.Dispatches) != 1 || d.Progress.Percent != 100 {
		t.Fatalf("strict merged decision: ok=%v err=%v %+v", ok, err, d)
	}
}

// ReplayCall 从工具卡片参数恢复当前计划：create → update → complete 与工具实现同一套合并规则；
// 非法参数与无计划的 update 被跳过，不覆盖已有决策。
func TestReplayCall(t *testing.T) {
	var d Decision
	var ok bool
	// create 之前的 update：忽略。
	if d, ok = ReplayCall(d, ok, `{"action":"update","progress":{"percent":50}}`); ok {
		t.Fatalf("update without plan must be ignored: %+v", d)
	}
	if d, ok = ReplayCall(d, ok, `{"action":"create","goal":"build app","complexity":"complex",
		"steps":[{"id":"s1","title":"schema","status":"in_progress"},{"id":"s2","title":"api","status":"pending"}],
		"progress":{"percent":10,"summary":"started"}}`); !ok || d.Goal != "build app" || len(d.Steps) != 2 {
		t.Fatalf("create: ok=%v %+v", ok, d)
	}
	if d.Source != SourceModel {
		t.Fatalf("replayed source = %q", d.Source)
	}
	// 坏 JSON 与缺 goal 的 create：跳过，不覆盖。
	before := d
	if d, ok = ReplayCall(d, ok, `not json`); !ok || d.Goal != before.Goal {
		t.Fatalf("bad json must keep previous: ok=%v %+v", ok, d)
	}
	if d, ok = ReplayCall(d, ok, `{"action":"create"}`); !ok || d.Goal != before.Goal {
		t.Fatalf("create without goal must keep previous: ok=%v %+v", ok, d)
	}
	// update：步骤状态与进度合并，分工不变。
	if d, ok = ReplayCall(d, ok, `{"action":"update","steps":[{"id":"s1","status":"done","evidence":"sql exec ok"}],
		"progress":{"percent":50,"summary":"half"},"findings":["f1"]}`); !ok {
		t.Fatal("update lost plan")
	}
	if d.Steps[0].Status != StepDone || d.Steps[0].Evidence != "sql exec ok" || d.Steps[1].Status != StepPending ||
		d.Progress.Percent != 50 || len(d.Findings) != 1 || d.Action != ActionUpdate {
		t.Fatalf("merged update: %+v", d)
	}
	// update 试图改分工：跳过。
	before = d
	if d, ok = ReplayCall(d, ok, `{"action":"update","dispatches":[{"ref":"d1","message":"x"}]}`); !ok || len(d.Dispatches) != 0 {
		t.Fatalf("update with dispatches must be ignored: %+v", d)
	}
	// complete：进度归 100，未完成步骤收尾。
	if d, ok = ReplayCall(d, ok, `{"action":"complete"}`); !ok || d.Action != ActionComplete ||
		d.Progress.Percent != 100 || d.Steps[1].Status != StepDone {
		t.Fatalf("complete: ok=%v %+v", ok, d)
	}
	// 未知动作：跳过。
	if _, ok = ReplayCall(Decision{}, false, `{"action":"nope"}`); ok {
		t.Fatal("unknown action must not create a plan")
	}
}

// explore：追加 jsonl、汇总 reads/findings/unknowns；缺 goal 拒绝。
func TestExploreTool(t *testing.T) {
	store := &MemoryStore{}
	et := &exploreTool{&Tools{Store: store}}
	out := runTool(t, et, `{"goal":"understand","questions":["why"],
		"reads":[{"path":"a.go","symbol":"F","why":"entry"}],
		"findings":["f1"],"unknowns":["u1"]}`)
	if !strings.Contains(out, `"notes":1`) || !strings.Contains(out, `"files":1`) {
		t.Fatalf("out = %s", out)
	}
	runTool(t, et, `{"goal":"more","reads":[{"path":"b.go"}],"findings":["f1","f2"]}`)
	out = runTool(t, et, `{"goal":"check"}`)
	if !strings.Contains(out, `"notes":3`) || !strings.Contains(out, `"files":2`) {
		t.Fatalf("aggregation: %s", out)
	}
	if !strings.Contains(out, `"findings":["f1","f2"]`) {
		t.Fatalf("dedup findings: %s", out)
	}
	if out := runTool(t, et, `{"reads":[{"path":"a"}]}`); !strings.Contains(out, "bad_arguments") {
		t.Fatalf("missing goal: %s", out)
	}
	if out := runTool(t, et, `{"goal":"x","reads":[{"symbol":"s"}]}`); !strings.Contains(out, "bad_arguments") {
		t.Fatalf("read without path: %s", out)
	}
}

// reflect：无模型走确定性清单；facts 与 progress 落库。
func TestReflectToolChecklist(t *testing.T) {
	store := &MemoryStore{}
	rt := &reflectTool{&Tools{Store: store}}
	// 无计划 → 建议先记录计划。
	out := runTool(t, rt, `{"reason":"risky step"}`)
	if !strings.Contains(out, `"aligned":false`) || !strings.Contains(out, "no explicit goal") {
		t.Fatalf("checklist without plan: %s", out)
	}
	// 有计划 → aligned 且含进度评估；destructive 风险入 risks。
	store.SaveDecision(context.Background(), Decision{
		Goal: "g", Action: ActionCreate, Progress: Progress{Percent: 30},
		Dispatches: []Dispatch{{Ref: "d1", Risk: RiskDestructive, Message: "m"}},
	})
	out = runTool(t, rt, `{"reason":"before destructive"}`)
	if !strings.Contains(out, `"aligned":true`) || !strings.Contains(out, "destructive") {
		t.Fatalf("checklist with plan: %s", out)
	}
	facts, _ := store.Facts(context.Background())
	if len(facts) != 0 {
		t.Fatalf("checklist must not fabricate facts: %v", facts)
	}
	if lines := store.Progress(); len(lines) == 0 || !strings.Contains(lines[0], "reflect") {
		t.Fatalf("reflect progress line: %v", lines)
	}
}

// fakeChat 只支持 reflect / planner 测试的 Chat。
type fakeChat struct {
	content string
	err     error
	calls   int
}

func (f *fakeChat) Chat(_ context.Context, _ string, _ agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	f.calls++
	if f.err != nil {
		return agentcore.ChatResponse{}, f.err
	}
	return agentcore.ChatResponse{Content: f.content}, nil
}

func (f *fakeChat) Stream(context.Context, string, agentcore.ChatRequest) (agentcore.TokenStream, error) {
	return nil, errors.New("stream not supported in fake")
}

// reflect：有模型时解析结构化输出并落 facts；非法 JSON 走清单回退。
func TestReflectToolWithModel(t *testing.T) {
	store := &MemoryStore{}
	store.SaveDecision(context.Background(), Decision{Goal: "g", Action: ActionCreate})
	fc := &fakeChat{content: `{"aligned":true,"assessment":"ok","risks":["r1"],
		"counterpoints":["c1"],"next_actions":["n1"],"facts":["fact1","fact2"]}`}
	rt := &reflectTool{&Tools{Store: store, LLM: fc}}
	out := runTool(t, rt, `{"reason":"decide"}`)
	if !strings.Contains(out, `"aligned":true`) || !strings.Contains(out, "fact1") {
		t.Fatalf("model reflection: %s", out)
	}
	if fc.calls != 1 {
		t.Fatalf("model calls = %d", fc.calls)
	}
	facts, _ := store.Facts(context.Background())
	if strings.Join(facts, ",") != "fact1,fact2" {
		t.Fatalf("facts = %v", facts)
	}

	// 非法 JSON → 确定性清单 + 失败说明。
	fc2 := &fakeChat{content: "not json at all"}
	rt2 := &reflectTool{&Tools{Store: store, LLM: fc2}}
	out = runTool(t, rt2, `{"reason":"x"}`)
	if !strings.Contains(out, "deterministic checklist used") {
		t.Fatalf("fallback to checklist: %s", out)
	}
}

// 工具组装配：三个工具名与 Info。
func TestToolsAll(t *testing.T) {
	list := (&Tools{Store: &MemoryStore{}}).All()
	if len(list) != 3 {
		t.Fatalf("want 3 tools, got %d", len(list))
	}
	for i, want := range []string{ToolPlan, ToolExplore, ToolReflect} {
		info, err := list[i].Info(context.Background())
		if err != nil || info.Name != want {
			t.Fatalf("tool[%d] = %v err=%v", i, info, err)
		}
	}
	// schema 可解析且 required 含 action。
	var js map[string]any
	if err := json.Unmarshal(PlanSchemaJSON(), &js); err != nil {
		t.Fatalf("plan schema: %v", err)
	}
	if req, ok := js["required"].([]any); !ok || len(req) == 0 {
		t.Fatalf("plan schema required: %v", js["required"])
	}
}

// ParseDecision 容忍代码块包裹与首尾杂文。
func TestParseDecisionTolerant(t *testing.T) {
	raw := "sure, here is the plan:\n```json\n{\"action\":\"create\",\"goal\":\"g\"}\n```\nthanks"
	d, err := ParseDecision(raw)
	if err != nil || d.Goal != "g" {
		t.Fatalf("tolerant parse: %v %+v", err, d)
	}
	if _, err := ParseDecision("no json here"); err == nil {
		t.Fatal("plain text must fail")
	}
}
