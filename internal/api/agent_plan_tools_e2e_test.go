// 文件说明：agent_plan_tools_e2e_test.go — 固定助手单聊的规划工具组全链路（planv6.8 B14）
//
// 真实链路：router → cloudagent.Runtime → llmgateway → fake 上游（原生 tool_calls）。
// 覆盖：请求体携带 plan / explore / reflect；plan 记录决策后 run 流出现 v3 `plan` 帧；
// 下一条 run 从会话历史里的 plan 卡片重放当前计划，因此 update 不必先 create。

package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

func upstreamToolNames(t *testing.T, req fakellm.Request) []string {
	t.Helper()
	var out []string
	for _, tool := range req.Tools {
		fn, _ := tool["function"].(map[string]any)
		if name, _ := fn["name"].(string); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func planFrameDecision(t *testing.T, ev sseEvent) plan.Decision {
	t.Helper()
	var d plan.Decision
	if err := json.Unmarshal(ev.Decision, &d); err != nil {
		t.Fatalf("plan frame decision %s: %v", ev.Decision, err)
	}
	return d
}

// planCallArguments 返回工具卡片里 plan 调用的 arguments（保持调用顺序）；卡片不是 JSON 数组时返回 nil。
// 与 agentrun.plan_tools.go 的同名内部函数同形（这里只验证落库的工具卡片）。
func planCallArguments(toolCallsJSON string) []string {
	var cards []struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if json.Unmarshal([]byte(toolCallsJSON), &cards) != nil {
		return nil
	}
	var out []string
	for _, card := range cards {
		if card.Name == plan.ToolPlan {
			out = append(out, card.Arguments)
		}
	}
	return out
}

func TestAgentE2EPlanToolFrameAndReplay(t *testing.T) {
	create := fakellm.Script{Seq: 1, Steps: []fakellm.Step{
		{ToolID: "p1", ToolName: plan.ToolPlan, ToolIndex: 0, ToolArgs: `{"action":"create","goal":"build the orders dashboard","complexity":"complex",
			"steps":[{"id":"s1","title":"schema","status":"in_progress"},{"id":"s2","title":"page","status":"pending"}],
			"progress":{"percent":10,"summary":"started"}}`},
		{FinishReason: "tool_calls"},
	}}
	createDone := fakellm.Script{Seq: 2, Steps: []fakellm.Step{{Content: "已记录计划"}, {FinishReason: "stop"}}}
	update := fakellm.Script{Seq: 3, Steps: []fakellm.Step{
		{ToolID: "p2", ToolName: plan.ToolPlan, ToolIndex: 0,
			ToolArgs: `{"action":"update","steps":[{"id":"s1","status":"done","evidence":"sql exec ok"}],"progress":{"percent":50}}`},
		{FinishReason: "tool_calls"},
	}}
	updateDone := fakellm.Script{Seq: 4, Steps: []fakellm.Step{{Content: "继续中"}, {FinishReason: "stop"}}}
	hs := newAgentHarness(t, []fakellm.Script{create, createDone, update, updateDone}, createDone)
	hs.runtime.ToolProtocol = "native"
	threadID := hs.createThread(t)

	// 第一条 run：模型先记录计划再回答。
	events := hs.createRun(t, threadID, "做一个订单看板", nil)
	frames := eventsOf(events, plan.ToolPlan)
	if len(frames) != 1 {
		t.Fatalf("plan frames = %+v", events)
	}
	d := planFrameDecision(t, frames[0])
	if d.Goal != "build the orders dashboard" || d.Complexity != plan.ComplexityComplex || len(d.Steps) != 2 || d.Progress.Percent != 10 {
		t.Fatalf("plan frame decision: %+v", d)
	}
	if frames[0].RunID == "" {
		t.Fatal("plan frame must carry run_id")
	}
	if len(eventsOf(events, "tool_result")) != 1 {
		t.Fatalf("plan tool result missing: %+v", events)
	}
	reqs := hs.fake.Requests()
	if len(reqs) != 2 {
		t.Fatalf("upstream requests = %d", len(reqs))
	}
	names := strings.Join(upstreamToolNames(t, reqs[0]), ",")
	for _, want := range []string{plan.ToolPlan, plan.ToolExplore, plan.ToolReflect, "list_databases"} {
		if !strings.Contains(names, want) {
			t.Fatalf("first request tools %q missing %s", names, want)
		}
	}
	// 决策落在 assistant 消息的工具卡片里（后续 run 重放的来源）。
	msgs, err := hs.store.ListAgentMessages(context.Background(), "testprj1", threadID, 20)
	if err != nil {
		t.Fatal(err)
	}
	var last systemdb.AgentMessage
	for _, m := range msgs {
		if m.Role == "assistant" {
			last = m
		}
	}
	if args := planCallArguments(last.ToolCallsJSON); len(args) != 1 || !strings.Contains(args[0], "orders dashboard") {
		t.Fatalf("plan card not persisted: %s", last.ToolCallsJSON)
	}

	// 第二条 run：只发 update——当前计划已从历史重放，不需要先 create。
	events = hs.createRun(t, threadID, "继续", nil)
	frames = eventsOf(events, plan.ToolPlan)
	if len(frames) != 1 {
		t.Fatalf("second run plan frames = %+v", events)
	}
	d = planFrameDecision(t, frames[0])
	if d.Action != plan.ActionUpdate || d.Goal != "build the orders dashboard" || len(d.Steps) != 2 ||
		d.Steps[0].Status != plan.StepDone || d.Steps[0].Evidence != "sql exec ok" || d.Progress.Percent != 50 {
		t.Fatalf("replayed update decision: %+v", d)
	}
	for _, ev := range eventsOf(events, "tool_result") {
		if strings.Contains(ev.Content, "no_plan") {
			t.Fatalf("update must replay the stored plan, got %s", ev.Content)
		}
	}
}
