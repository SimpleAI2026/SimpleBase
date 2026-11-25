// 文件说明：planner_test.go — Planner 决策链测试（planv6.8 §13.11.1-4）
//
// fake ChatClient 队列化响应，覆盖：native 工具调用、tools 不支持时回退文本协议、
// 校验失败的修复重试、重试耗尽返回 ErrNoPlan（调用方走规则回退）、普通文本不解析。

package plan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/voocel/litellm/providers"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// scriptedLLM 按脚本回放 Chat 调用；记录全部请求供断言。
type scriptedLLM struct {
	steps []scriptStep
	calls []agentcore.ChatRequest
}

type scriptStep struct {
	resp agentcore.ChatResponse
	err  error
}

func (s *scriptedLLM) Chat(_ context.Context, _ string, req agentcore.ChatRequest) (agentcore.ChatResponse, error) {
	s.calls = append(s.calls, req)
	if len(s.steps) == 0 {
		return agentcore.ChatResponse{}, errors.New("scriptedLLM: no steps left")
	}
	step := s.steps[0]
	s.steps = s.steps[1:]
	return step.resp, step.err
}

func (s *scriptedLLM) Stream(context.Context, string, agentcore.ChatRequest) (agentcore.TokenStream, error) {
	return nil, errors.New("stream not supported in scripted LLM")
}

func plannerRequest() Request {
	return Request{
		UserText: "帮我调研并总结",
		Candidates: []Candidate{
			{ID: "a1", Name: "研究员", Module: "research"},
			{ID: "a2", Name: "写手", Module: "writer"},
		},
		Constraints: baseConstraints(),
	}
}

func validPlanArgs() string {
	return `{"action":"create","goal":"调研并总结","dispatches":[
		{"ref":"d1","target_kind":"agent","target_id":"a1","message":"先调研"},
		{"ref":"d2","target_kind":"agent","target_id":"a2","message":"再总结","depends_on":["d1"]}]}` + ""
}

// native：模型以 plan 工具调用输出，请求带 Tools。
func TestPlannerNativeToolCall(t *testing.T) {
	llm := &scriptedLLM{steps: []scriptStep{
		{resp: agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{
			{Name: ToolPlan, Arguments: validPlanArgs()},
		}}},
	}}
	p := &Planner{LLM: llm, Scope: "test", Model: "m1"}
	d, err := p.Decide(context.Background(), plannerRequest())
	if err != nil {
		t.Fatal(err)
	}
	if d.Goal != "调研并总结" || len(d.Dispatches) != 2 || d.Dispatches[1].DependsOn[0] != "d1" {
		t.Fatalf("decision: %+v", d)
	}
	if len(llm.calls) != 1 {
		t.Fatalf("calls = %d", len(llm.calls))
	}
	req := llm.calls[0]
	if len(req.Tools) != 1 || req.Tools[0].Name != ToolPlan {
		t.Fatalf("request must carry the plan tool spec: %+v", req.Tools)
	}
	if req.Model != "m1" || req.MaxTokens == nil || *req.MaxTokens != plannerDefaultMaxTokens {
		t.Fatalf("request defaults: model=%q maxTokens=%v", req.Model, req.MaxTokens)
	}
	// 提示词：system 规则 + user 数据（本轮请求）。
	if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
		t.Fatalf("messages: %+v", req.Messages)
	}
	if !strings.Contains(req.Messages[0].Content, "规划器") || !strings.Contains(req.Messages[0].Content, "a1") {
		t.Fatalf("system prompt missing rules or candidates: %q", req.Messages[0].Content)
	}
	if !strings.Contains(req.Messages[1].Content, "帮我调研并总结") {
		t.Fatalf("user prompt missing request text: %q", req.Messages[1].Content)
	}
}

// text 回退：第一次 tools 不支持（400 + tool 字样），第二次走 TOOL_CALL 文本协议。
func TestPlannerFallsBackToTextProtocol(t *testing.T) {
	llm := &scriptedLLM{steps: []scriptStep{
		{err: &providers.LiteLLMError{Type: providers.ErrorTypeValidation, Message: "tools are not supported for this model", StatusCode: 400}},
		{resp: agentcore.ChatResponse{Content: `TOOL_CALL {"name":"plan","arguments":` + validPlanArgs() + `}`}},
	}}
	p := &Planner{LLM: llm}
	d, err := p.Decide(context.Background(), plannerRequest())
	if err != nil {
		t.Fatal(err)
	}
	if d.Goal != "调研并总结" {
		t.Fatalf("decision: %+v", d)
	}
	if len(llm.calls) != 2 {
		t.Fatalf("calls = %d", len(llm.calls))
	}
	// 第一次带 tools，第二次不带，且 system 末尾有文本协议说明。
	if len(llm.calls[0].Tools) != 1 {
		t.Fatalf("first call must carry tools")
	}
	if len(llm.calls[1].Tools) != 0 {
		t.Fatalf("second call must drop tools")
	}
	if !strings.Contains(llm.calls[1].Messages[0].Content, "TOOL_CALL") {
		t.Fatalf("text protocol instruction missing: %q", llm.calls[1].Messages[0].Content)
	}
}

// 非 tools 错误不触发回退（如 401）。
func TestPlannerAuthErrorDoesNotFallback(t *testing.T) {
	llm := &scriptedLLM{steps: []scriptStep{
		{err: &providers.LiteLLMError{Type: providers.ErrorTypeAuth, Message: "invalid api key", StatusCode: 401}},
	}}
	p := &Planner{LLM: llm}
	if _, err := p.Decide(context.Background(), plannerRequest()); err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("err = %v", err)
	}
	if len(llm.calls) != 1 {
		t.Fatalf("auth error must not retry: calls = %d", len(llm.calls))
	}
}

// 修复重试：第一份缺 goal，带错误反馈重试后成功。
func TestPlannerRepairRetry(t *testing.T) {
	bad := `{"action":"create","dispatches":[{"ref":"d1","target_kind":"agent","target_id":"a1","message":"m"}]}`
	llm := &scriptedLLM{steps: []scriptStep{
		{resp: agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{Name: ToolPlan, Arguments: bad}}}},
		{resp: agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{Name: ToolPlan, Arguments: validPlanArgs()}}}},
	}}
	p := &Planner{LLM: llm}
	d, err := p.Decide(context.Background(), plannerRequest())
	if err != nil {
		t.Fatal(err)
	}
	if d.Goal != "调研并总结" || len(llm.calls) != 2 {
		t.Fatalf("decision=%+v calls=%d", d, len(llm.calls))
	}
	// 第二次请求包含失败输出与错误说明。
	last := llm.calls[1]
	joined := last.Messages[len(last.Messages)-1].Content
	if !strings.Contains(joined, "goal is required") || !strings.Contains(joined, ToolPlan) {
		t.Fatalf("repair feedback: %q", joined)
	}
}

// 重试耗尽：ErrNoPlan（errors.Is 可判），调用方走规则回退。
func TestPlannerRepairExhausted(t *testing.T) {
	bad := `{"action":"create","goal":"g","dispatches":[{"ref":"d1","target_kind":"agent","target_id":"ghost","message":"m"}]}`
	steps := make([]scriptStep, 0, 2)
	for i := 0; i < 2; i++ {
		steps = append(steps, scriptStep{resp: agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{Name: ToolPlan, Arguments: bad}}}})
	}
	llm := &scriptedLLM{steps: steps}
	p := &Planner{LLM: llm}
	_, err := p.Decide(context.Background(), plannerRequest())
	if !errors.Is(err, ErrNoPlan) {
		t.Fatalf("err = %v, want ErrNoPlan", err)
	}
	if !strings.Contains(err.Error(), "non-member") {
		t.Fatalf("err should carry the last validation failure: %v", err)
	}
}

// 普通文本（无 TOOL_CALL、无工具调用）不解析，ErrNoPlan。
func TestPlannerPlainTextRejected(t *testing.T) {
	llm := &scriptedLLM{steps: []scriptStep{
		{resp: agentcore.ChatResponse{Content: "好的，我来分派：a1 去调研。"}},
		{resp: agentcore.ChatResponse{Content: "还是不想用工具。"}},
	}}
	p := &Planner{LLM: llm}
	_, err := p.Decide(context.Background(), plannerRequest())
	if !errors.Is(err, ErrNoPlan) {
		t.Fatalf("err = %v, want ErrNoPlan", err)
	}
}

// 提取了别的工具名（非 plan）不算决策。
func TestPlannerIgnoresOtherToolNames(t *testing.T) {
	llm := &scriptedLLM{steps: []scriptStep{
		{resp: agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{Name: "shell", Arguments: `{"command":"ls"}`}}}},
		{resp: agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{Name: "shell", Arguments: `{"command":"ls"}`}}}},
	}}
	p := &Planner{LLM: llm}
	if _, err := p.Decide(context.Background(), plannerRequest()); !errors.Is(err, ErrNoPlan) {
		t.Fatalf("err = %v, want ErrNoPlan", err)
	}
}

// @ 硬约束：模型遗漏被点名成员 → 修复重试 → 成功。
func TestPlannerEnforcesMentionOnRetry(t *testing.T) {
	req := plannerRequest()
	req.Constraints.Mentioned = []string{"a2"}
	missing := `{"action":"create","goal":"g","dispatches":[
		{"ref":"d1","target_kind":"agent","target_id":"a1","message":"m"}]}`
	llm := &scriptedLLM{steps: []scriptStep{
		{resp: agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{Name: ToolPlan, Arguments: missing}}}},
		{resp: agentcore.ChatResponse{ToolCalls: []agentcore.ChatToolCall{{Name: ToolPlan, Arguments: validPlanArgs()}}}},
	}}
	p := &Planner{LLM: llm}
	d, err := p.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("retry should fix mention gap: %v", err)
	}
	if d.Dispatches[1].TargetID != "a2" {
		t.Fatalf("repaired decision: %+v", d)
	}
}

// 提示词：群路由提示词进 user 消息且标注不授予权限；@ 点名进 system 硬约束。
func TestBuildPromptRoutingPromptIsUserData(t *testing.T) {
	req := plannerRequest()
	req.RoutingPrompt = "优先让写手做总结"
	req.Constraints.Mentioned = []string{"a1"}
	req.History = []Turn{{Role: "user", Content: "历史一轮"}}
	msgs := BuildPrompt(req, false)
	if len(msgs) != 2 {
		t.Fatalf("messages = %d", len(msgs))
	}
	sys, user := msgs[0].Content, msgs[1].Content
	if !strings.Contains(sys, "显式点名") || !strings.Contains(sys, "a1") {
		t.Fatalf("system must state the mention hard constraint: %q", sys)
	}
	if !strings.Contains(user, "优先让写手做总结") || !strings.Contains(user, "不授予权限") {
		t.Fatalf("routing prompt placement: %q", user)
	}
	if !strings.Contains(user, "历史一轮") {
		t.Fatalf("history missing: %q", user)
	}
	// 人类说明：无人类时明示禁止。
	noHuman := plannerRequest()
	noHuman.Constraints.HumanID = ""
	msgsWithoutHuman := BuildPrompt(noHuman, false)
	if !strings.Contains(msgsWithoutHuman[1].Content, "没有可指派的人") {
		t.Fatalf("human availability: %q", msgsWithoutHuman[1].Content)
	}
	req.Constraints.HumanID = "u9"
	req.Constraints.MentionedHuman = true
	msgs = BuildPrompt(req, false)
	if !strings.Contains(msgs[0].Content, "u9") || !strings.Contains(msgs[1].Content, "u9") {
		t.Fatalf("human id must appear in both messages: %q %q", msgs[0].Content, msgs[1].Content)
	}
	// 文本协议模式：system 末尾附 TOOL_CALL 格式说明。
	msgs = BuildPrompt(req, true)
	if !strings.Contains(msgs[0].Content, "TOOL_CALL") {
		t.Fatalf("text protocol instruction: %q", msgs[0].Content)
	}
}
