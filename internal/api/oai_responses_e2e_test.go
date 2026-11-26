// 文件说明：oai_responses_e2e_test.go — OpenAI Responses API 的路由级端到端（完整 NewRouter + 真实 llmgateway + fakellm）；
// handler 单元测试在 internal/api/llmhttp。
//
// 用法
//
//	go test ./internal/api -run 'TestOAIE2E_Responses|TestResponsesRouteRegisteredAndGated'
//
// 调用架构
//
//	[上游] 本包测试
//	         │ 挂真实路由树
//	         ▼
//	[本文件] internal/api/oai_responses_e2e_test.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/llmhttp（经 NewRouter 挂载）
//
// 主要符号
//
//	TestOAIE2E_ResponsesNonStreamAndStream / TestOAIE2E_ResponsesToolsRoundTrip / TestOAIE2E_ResponsesUpstreamErrors
//	TestResponsesRouteRegisteredAndGated

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

// 端到端：完整 NewRouter + 真实 llmgateway + fakellm 上游
// ---------------------------------------------------------------------------

func TestOAIE2E_ResponsesNonStreamAndStream(t *testing.T) {
	e := newOAIE2E(t, nil, fakellm.Script{
		Steps: []fakellm.Step{{Content: "he"}, {Content: "llo"}, {FinishReason: "stop"}, {PromptTokens: 3, CompletionTokens: 2}},
	})
	resp, raw := e.do(t, http.MethodPost, oaiBase+"/responses", `{"instructions":"be brief","input":"hi"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
	var out struct {
		Object string `json:"object"`
		Status string `json:"status"`
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "response" || out.Status != "completed" || out.Output[0].Content[0].Text != "hello" {
		t.Fatalf("resp=%s", raw)
	}
	if strings.Contains(string(raw), "upstream-secret-key") {
		t.Fatalf("upstream key leaked: %s", raw)
	}
	// instructions 经真实网关作为 system 消息到达上游。
	up := e.fake.Requests()
	if len(up) != 1 || up[0].Messages[0].Role != "system" || up[0].Messages[0].Content.String() != "be brief" {
		t.Fatalf("upstream messages=%+v", up)
	}

	resp, raw = e.do(t, http.MethodPost, oaiBase+"/responses", `{"input":"hi","stream":true}`)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status=%d ct=%s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	evs := sseEvents(t, string(raw))
	if evs[0].Name != "response.created" || evs[len(evs)-1].Name != "response.completed" {
		t.Fatalf("events=%v", evs)
	}
	var text strings.Builder
	for _, ev := range evs {
		if ev.Name == "response.output_text.delta" {
			var d struct {
				Delta string `json:"delta"`
			}
			_ = json.Unmarshal([]byte(ev.Data), &d)
			text.WriteString(d.Delta)
		}
	}
	if text.String() != "hello" {
		t.Fatalf("streamed text=%q", text.String())
	}
}

// 工具往返：function_call / function_call_output 经真实链路到达上游，
// 上游返回的工具调用以 Responses 形状回到客户端。
func TestOAIE2E_ResponsesToolsRoundTrip(t *testing.T) {
	e := newOAIE2E(t, []fakellm.Script{
		{Seq: 1, NonStreamToolCalls: []fakellm.ToolCall{{ID: "call_9", Name: "shell", Arguments: `{"command":"ls"}`}}},
	}, fakellm.Script{Steps: []fakellm.Step{{Content: "x"}, {FinishReason: "stop"}}})
	body := `{"input":[
	  {"role":"user","content":"run"},
	  {"type":"function_call","call_id":"call_0","name":"shell","arguments":"{}"},
	  {"type":"function_call_output","call_id":"call_0","output":"ok"}],
	 "tools":[{"type":"function","name":"shell","parameters":{"type":"object","properties":{}}}],
	 "tool_choice":{"type":"function","name":"shell"}}`
	resp, raw := e.do(t, http.MethodPost, oaiBase+"/responses", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
	var out struct {
		Output []map[string]any `json:"output"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Output) != 1 || out.Output[0]["type"] != "function_call" ||
		out.Output[0]["call_id"] != "call_9" || out.Output[0]["arguments"] != `{"command":"ls"}` {
		t.Fatalf("output=%s", raw)
	}
	up := e.fake.Requests()
	if len(up) != 1 || len(up[0].Tools) != 1 {
		t.Fatalf("upstream=%+v", up)
	}
	var sawCall, sawResult bool
	for _, m := range up[0].Messages {
		if m.Role == "assistant" && len(m.ToolCalls) == 1 && m.ToolCalls[0].ID == "call_0" {
			sawCall = true
		}
		if m.Role == "tool" && m.ToolCallID == "call_0" {
			sawResult = true
		}
	}
	if !sawCall || !sawResult {
		t.Fatalf("tool round trip not forwarded upstream: %+v", up[0].Messages)
	}
}

// 上游错误沿用 chat 端点的分类与脱敏：错误信封是 OpenAI 形状，上游原文不外泄。
func TestOAIE2E_ResponsesUpstreamErrors(t *testing.T) {
	e := newOAIE2E(t, nil, fakellm.Script{HTTPStatus: http.StatusUnauthorized, Body: `{"error":{"message":"bad key sk-leaky"}}`})
	resp, raw := e.do(t, http.MethodPost, oaiBase+"/responses", `{"input":"x"}`)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status=%d %s", resp.StatusCode, raw)
	}
	assertOAIError(t, string(raw), "api_error", "llm_auth_failed")
	if strings.Contains(string(raw), "sk-leaky") {
		t.Fatalf("upstream error text leaked: %s", raw)
	}
}

// 路由与权限：与 chat 端点一致，需要 llm:invoke。
func TestResponsesRouteRegisteredAndGated(t *testing.T) {
	deps, key := fullRouterDeps(t)
	e := NewRouter(deps)
	registered := false
	for _, r := range e.Routes() {
		if r.Method == http.MethodPost && r.Path == "/v1/projects/:projectID/v1/responses" {
			registered = true
		}
	}
	if !registered {
		t.Fatalf("POST /v1/projects/:projectID/v1/responses is not registered")
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/v1/responses", strings.NewReader(`{"input":"x"}`))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", rec.Code)
	}
	_ = key
}
