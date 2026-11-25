// 文件说明：agent_plan_tools_test.go — 宿主规划工具组的装配、历史重放与 plan 帧下发（planv6.8 B14）

package agentrun

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// planArgs 是测试里构造 plan 工具卡片用的 arguments。
func planArgs(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func planCards(t *testing.T, calls ...string) string {
	t.Helper()
	cards := make([]map[string]any, 0, len(calls))
	for _, args := range calls {
		cards = append(cards, map[string]any{"name": plan.ToolPlan, "arguments": args, "content": `{"ok":true}`})
	}
	raw, err := json.Marshal(cards)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func invoke(t *testing.T, tools []tool.BaseTool, name, args string, ctx context.Context) string {
	t.Helper()
	for _, tl := range tools {
		info, err := tl.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if info.Name != name {
			continue
		}
		inv, ok := tl.(interface {
			InvokableRun(context.Context, string, ...tool.Option) (string, error)
		})
		if !ok {
			t.Fatalf("tool %s not invokable", name)
		}
		out, err := inv.InvokableRun(ctx, args)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	t.Fatalf("tool %s not registered", name)
	return ""
}

// 工具卡片解析：只取 plan 调用并保持顺序；坏 JSON 返回 nil。
func TestPlanCallArguments(t *testing.T) {
	cards := `[{"name":"read","arguments":"{}"},{"name":"plan","arguments":"{\"action\":\"create\"}"},
		{"name":"plan","arguments":"{\"action\":\"update\"}"}]`
	got := planCallArguments(cards)
	if len(got) != 2 || !strings.Contains(got[0], "create") || !strings.Contains(got[1], "update") {
		t.Fatalf("plan args = %v", got)
	}
	if planCallArguments("not json") != nil || planCallArguments("") != nil {
		t.Fatal("malformed cards must return nil")
	}
}

// requestsReplan 只认 action=replan 的 plan 调用。
func TestRequestsReplan(t *testing.T) {
	if requestsReplan(planCards(t, planArgs(t, map[string]any{"action": "update"}))) {
		t.Fatal("update is not a replan")
	}
	if requestsReplan(planCards(t, planArgs(t, map[string]any{"action": "replan"}))) != true {
		t.Fatal("replan must be detected")
	}
	if requestsReplan(`[{"name":"simplebase","arguments":"{\"action\":\"replan\"}"}]`) {
		t.Fatal("non-plan tools must be ignored")
	}
	if requestsReplan("garbage") {
		t.Fatal("garbage must not request replan")
	}
}

// 历史重放：按时间顺序叠加 create → update，得到会话的当前计划；非 assistant 消息与坏卡片跳过。
func TestReplayPlan(t *testing.T) {
	hist := []systemdb.AgentMessage{
		{Role: "user", Content: "build", ToolCallsJSON: planCards(t, planArgs(t, map[string]any{"action": "create", "goal": "ignored"}))},
		{Role: "assistant", ToolCallsJSON: `[{"name":"plan","arguments":"not json"}]`},
		{Role: "assistant", ToolCallsJSON: planCards(t, planArgs(t, map[string]any{
			"action": "create", "goal": "build app", "complexity": "complex",
			"steps": []map[string]any{{"id": "s1", "title": "schema", "status": "in_progress"}, {"id": "s2", "title": "api", "status": "pending"}},
		}))},
		{Role: "assistant", ToolCallsJSON: planCards(t, planArgs(t, map[string]any{
			"action":   "update",
			"steps":    []map[string]any{{"id": "s1", "status": "done", "evidence": "sql exec ok"}},
			"progress": map[string]any{"percent": 50, "summary": "half"},
		}))},
	}
	d, ok := replayPlan(hist)
	if !ok || d.Goal != "build app" || len(d.Steps) != 2 {
		t.Fatalf("replayed: ok=%v %+v", ok, d)
	}
	if d.Steps[0].Status != plan.StepDone || d.Steps[0].Evidence != "sql exec ok" || d.Steps[1].Status != plan.StepPending || d.Progress.Percent != 50 {
		t.Fatalf("update not merged: %+v", d)
	}
	if _, ok := replayPlan([]systemdb.AgentMessage{{Role: "assistant", ToolCallsJSON: `[]`}}); ok {
		t.Fatal("empty history has no plan")
	}
}

// 固定助手单聊：带 Emit 的 run 上下文里 plan 记录决策后下发 v3 `plan` 帧（decision 为完整 Decision）；
// 历史里的计划可被 update；没有 Emit（非流式）时静默。
func TestRunPlanningToolsEmitsPlanFrame(t *testing.T) {
	s := &Service{}
	agent := systemdb.CloudAgent{ID: "a1", Name: "通用", Module: "general"}
	hist := []systemdb.AgentMessage{{Role: "assistant", ToolCallsJSON: planCards(t, planArgs(t, map[string]any{
		"action": "create", "goal": "build app",
		"steps": []map[string]any{{"id": "s1", "title": "schema", "status": "pending"}},
	}))}}

	var frames []cloudagent.Event
	ctx := cloudagent.WithRunContext(context.Background(), cloudagent.RunContext{RunID: "run-1", Emit: func(ev cloudagent.Event) { frames = append(frames, ev) }})
	tools := s.runPlanningTools(ctx, "testprj1", "run-1", agent, hist)
	if len(tools) != 3 {
		t.Fatalf("want plan/explore/reflect, got %d", len(tools))
	}
	out := invoke(t, tools, plan.ToolPlan, `{"action":"update","steps":[{"id":"s1","status":"in_progress"}],"progress":{"percent":10}}`, ctx)
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("update over replayed plan: %s", out)
	}
	if len(frames) != 1 || frames[0].Type != group.EventPlan || frames[0].RunID != "run-1" || frames[0].AgentID != "a1" {
		t.Fatalf("plan frame: %+v", frames)
	}
	var decision plan.Decision
	if err := json.Unmarshal(frames[0].Decision, &decision); err != nil {
		t.Fatalf("decision payload: %v", err)
	}
	if decision.Goal != "build app" || len(decision.Steps) != 1 || decision.Steps[0].Status != plan.StepInProgress {
		t.Fatalf("frame decision: %+v", decision)
	}
	// SSE 形状：type / run_id / decision 同级，不带 confirm_required（单聊没有整份计划确认）。
	raw, _ := json.Marshal(frames[0])
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	if _, nested := wire["decision"].(map[string]any)["decision"]; nested {
		t.Fatalf("decision must not be nested: %s", raw)
	}
	if _, has := wire["confirm_required"]; has {
		t.Fatalf("single-agent plan frame must not carry confirm_required: %s", raw)
	}

	// 非流式（无 Emit）：工具照常可用，不下发。
	plain := s.runPlanningTools(context.Background(), "testprj1", "run-2", agent, nil)
	if out := invoke(t, plain, plan.ToolPlan, `{"action":"create","goal":"fresh","steps":[{"id":"s1","title":"t","status":"pending"}]}`, context.Background()); !strings.Contains(out, `"ok":true`) {
		t.Fatalf("create without emit: %s", out)
	}
	// 没有 Emit 的上下文：update 无计划时按工具语义返回 no_plan，而不是 panic。
	if out := invoke(t, s.runPlanningTools(context.Background(), "testprj1", "run-3", agent, nil), plan.ToolPlan, `{"action":"update"}`, context.Background()); !strings.Contains(out, "no_plan") {
		t.Fatalf("update without plan: %s", out)
	}
}

// 群任务：栅栏决策打底并按群约束校验（群外成员被拒），决策写入轮次事件。
func TestTurnPlanningTools(t *testing.T) {
	store := openAgentStore(t)
	s := NewServiceWithStore(Config{}, store, nil, nil)
	if _, err := store.CreateCloudAgent(context.Background(), systemdb.CloudAgent{
		ID: "a1", ProjectID: "testprj1", Name: "通用", Module: "general",
	}); err != nil {
		t.Fatal(err)
	}
	// 空群 = 通用群：成员是项目内全部助手。
	turn := systemdb.AgentTurn{ID: "turn-1", ProjectID: "testprj1", ThreadID: "th1", Status: group.TurnRunning,
		RouteJSON: marshalGroupRoute(groupRouteEnvelope{Decision: plan.Decision{Action: plan.ActionCreate, Goal: "ship it",
			Dispatches: []plan.Dispatch{{Ref: "d1", TargetKind: plan.TargetAgent, TargetID: "a1", Message: "m"}}}})}
	var events []group.Event
	tools := s.turnPlanningTools(context.Background(), "testprj1", turn, "turn-1-d1", systemdb.CloudAgent{ID: "a1"}, func(ev group.Event) { events = append(events, ev) })
	out := invoke(t, tools, plan.ToolPlan, `{"action":"update","progress":{"percent":40}}`, context.Background())
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("turn update: %s", out)
	}
	out = invoke(t, tools, plan.ToolPlan, `{"action":"replan","goal":"g","dispatches":[{"ref":"d1","target_kind":"agent","target_id":"ghost","message":"m"}]}`, context.Background())
	if !strings.Contains(out, "agent_not_in_tag") {
		t.Fatalf("non-member must be rejected: %s", out)
	}
	if len(events) != 1 || events[0].Type != group.EventPlan || events[0].TaskID != "turn-1-d1" || events[0].AgentID != "plan" {
		t.Fatalf("turn plan events: %+v", events)
	}
	var payload struct {
		Decision       plan.Decision `json:"decision"`
		ConfirmRequire bool          `json:"confirm_required"`
	}
	if err := json.Unmarshal(events[0].Data, &payload); err != nil || payload.Decision.Progress.Percent != 40 || payload.Decision.Goal != "ship it" || len(payload.Decision.Dispatches) != 1 {
		t.Fatalf("turn plan payload: %v %+v", err, payload)
	}
}
