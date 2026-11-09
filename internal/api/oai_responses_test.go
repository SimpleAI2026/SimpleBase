package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/testutil/fakellm"
)

func postResponses(t *testing.T, e *echo.Echo, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func setupResponsesRouter(t *testing.T, h *LLMHandler) *echo.Echo {
	t.Helper()
	e := setupLLMRouter(t, h, true, true)
	e.POST("/v1/responses", h.Responses)
	return e
}

// sseEvents 把 Responses 的 SSE 拆成 (event 名, data) 列表。
func sseEvents(t *testing.T, body string) []struct{ Name, Data string } {
	t.Helper()
	var out []struct{ Name, Data string }
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		var name, data string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		if name != "" {
			out = append(out, struct{ Name, Data string }{name, data})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 请求转换
// ---------------------------------------------------------------------------

func TestRespInputString(t *testing.T) {
	svc := &fakeLLMSvc{}
	e := setupResponsesRouter(t, &LLMHandler{svc: svc})
	rec := postResponses(t, e, `{"model":"m","instructions":"be brief","input":"hi","max_output_tokens":7,"temperature":0.2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	got := svc.lastReq
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[0].Content != "be brief" ||
		got.Messages[1].Role != "user" || got.Messages[1].Content != "hi" {
		t.Fatalf("messages=%+v", got.Messages)
	}
	if got.Model != "m" || *got.MaxTokens != 7 || *got.Temperature != 0.2 {
		t.Fatalf("scalars=%+v", got)
	}
}

// 数组形态：简写消息、内容块、developer 角色、reasoning 被忽略、工具往返。
func TestRespInputItemsToMessages(t *testing.T) {
	svc := &fakeLLMSvc{}
	e := setupResponsesRouter(t, &LLMHandler{svc: svc})
	rec := postResponses(t, e, `{"input":[
	  {"role":"developer","content":"rules"},
	  {"type":"message","role":"user","content":[{"type":"input_text","text":"run "},{"type":"input_text","text":"pwd"}]},
	  {"type":"reasoning","id":"rs_1","summary":[]},
	  {"type":"function_call","call_id":"c1","name":"shell","arguments":"{\"command\":\"pwd\"}"},
	  {"type":"function_call","call_id":"c2","name":"shell","arguments":""},
	  {"type":"function_call_output","call_id":"c1","output":"/tmp"},
	  {"type":"function_call_output","call_id":"c2","output":[{"type":"input_text","text":"done"}]},
	  {"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}
	]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	m := svc.lastReq.Messages
	if len(m) != 6 {
		t.Fatalf("messages(%d)=%+v", len(m), m)
	}
	if m[0].Role != "system" || m[0].Content != "rules" {
		t.Fatalf("developer must map to system: %+v", m[0])
	}
	if m[1].Role != "user" || m[1].Content != "run pwd" {
		t.Fatalf("content parts must be joined: %+v", m[1])
	}
	// 两个紧邻的 function_call 合并进同一条 assistant 消息，arguments 为空补 {}。
	if m[2].Role != "assistant" || len(m[2].ToolCalls) != 2 || m[2].ToolCalls[0].ID != "c1" ||
		m[2].ToolCalls[0].Arguments != `{"command":"pwd"}` || m[2].ToolCalls[1].Arguments != "{}" {
		t.Fatalf("parallel calls must merge into one assistant message: %+v", m[2])
	}
	if m[3].Role != "tool" || m[3].ToolCallID != "c1" || m[3].Content != "/tmp" {
		t.Fatalf("tool output 1: %+v", m[3])
	}
	if m[4].Role != "tool" || m[4].ToolCallID != "c2" || m[4].Content != "done" {
		t.Fatalf("tool output 2 (content parts): %+v", m[4])
	}
	if m[5].Role != "assistant" || m[5].Content != "ok" {
		t.Fatalf("assistant message: %+v", m[5])
	}
}

// Responses 的工具声明是平铺的，tool_choice 的函数选择也是平铺的；都要转成 chat 形状。
func TestRespToolsAndToolChoice(t *testing.T) {
	svc := &fakeLLMSvc{}
	e := setupResponsesRouter(t, &LLMHandler{svc: svc})
	rec := postResponses(t, e, `{"input":"x",
	  "tools":[{"type":"function","name":"shell","description":"run","parameters":{"type":"object","properties":{"command":{"type":"string"}}}}],
	  "tool_choice":{"type":"function","name":"shell"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	got := svc.lastReq
	if len(got.Tools) != 1 || got.Tools[0].Name != "shell" || got.Tools[0].Description != "run" ||
		!strings.Contains(string(got.Tools[0].Parameters), `"command"`) {
		t.Fatalf("tools=%+v", got.Tools)
	}
	var choice struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(got.ToolChoice, &choice); err != nil || choice.Type != "function" || choice.Function.Name != "shell" {
		t.Fatalf("tool_choice must be converted to the chat shape: %s err=%v", got.ToolChoice, err)
	}

	for _, c := range []string{`"auto"`, `"none"`, `"required"`} {
		svc := &fakeLLMSvc{}
		e := setupResponsesRouter(t, &LLMHandler{svc: svc})
		rec := postResponses(t, e, `{"input":"x","tools":[{"type":"function","name":"t"}],"tool_choice":`+c+`}`)
		if rec.Code != http.StatusOK || string(svc.lastReq.ToolChoice) != c {
			t.Fatalf("tool_choice %s: status=%d got=%s", c, rec.Code, svc.lastReq.ToolChoice)
		}
	}
}

// 不支持的用法一律 400 + OpenAI 错误信封，且不会触达上游。
func TestRespRejectsUnsupported(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"previous_response_id", `{"input":"x","previous_response_id":"resp_1"}`, "previous_response_id is not supported"},
		{"conversation", `{"input":"x","conversation":"conv_1"}`, "conversation is not supported"},
		{"background", `{"input":"x","background":true}`, "background mode is not supported"},
		{"structured_output", `{"input":"x","text":{"format":{"type":"json_schema"}}}`, "structured outputs are unavailable"},
		{"builtin_tool", `{"input":"x","tools":[{"type":"web_search"}]}`, "tools[0].type must be"},
		{"empty_tool_name", `{"input":"x","tools":[{"type":"function","name":" "}]}`, "name must not be empty"},
		{"missing_input", `{"model":"m"}`, "input is required"},
		{"null_input", `{"input":null}`, "input is required"},
		{"bad_input_type", `{"input":42}`, "input must be a string or an array"},
		{"empty_input_array", `{"input":[]}`, "input must not be empty"},
		{"image_part", `{"input":[{"role":"user","content":[{"type":"input_image","image_url":"http://x/a.png"}]}]}`, "text only"},
		{"unknown_item", `{"input":[{"type":"file_search_call"}]}`, "is not supported"},
		{"bad_role", `{"input":[{"role":"tool","content":"x"}]}`, "is not supported"},
		{"call_without_id", `{"input":[{"type":"function_call","name":"t"}]}`, "requires call_id and name"},
		{"output_without_id", `{"input":[{"type":"function_call_output","output":"x"}]}`, "requires call_id"},
		{"bad_choice", `{"input":"x","tools":[{"type":"function","name":"t"}],"tool_choice":"sometimes"}`, "tool_choice must be"},
		{"choice_without_tools", `{"input":"x","tool_choice":"required"}`, "requires tools"},
		{"allowed_tools_choice", `{"input":"x","tools":[{"type":"function","name":"t"}],"tool_choice":{"type":"allowed_tools"}}`, "tool_choice object must be"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeLLMSvc{}
			e := setupResponsesRouter(t, &LLMHandler{svc: svc})
			rec := postResponses(t, e, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
			}
			assertOAIError(t, rec.Body.String(), "invalid_request_error", "invalid_request")
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("message should mention %q: %s", tc.want, rec.Body.String())
			}
			if len(svc.lastReq.Messages) != 0 {
				t.Fatalf("an invalid request must not reach the model: %+v", svc.lastReq)
			}
		})
	}
	t.Run("bad_json", func(t *testing.T) {
		e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{}})
		rec := postResponses(t, e, `{`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d", rec.Code)
		}
		assertOAIError(t, rec.Body.String(), "invalid_request_error", "invalid_request")
	})
	t.Run("missing_project", func(t *testing.T) {
		h := &LLMHandler{svc: &fakeLLMSvc{}}
		e := setupLLMRouter(t, h, false, true)
		e.POST("/v1/responses", h.Responses)
		if rec := postResponses(t, e, `{"input":"x"}`); rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// 非流式响应
// ---------------------------------------------------------------------------

func TestRespNonStreamShape(t *testing.T) {
	svc := &fakeLLMSvc{resp: LLMResponse{
		Content: "hello", Model: "up-model", FinishReason: "stop",
		Usage: LLMTokenUsage{PromptTokens: 3, CompletionTokens: 4, ReasoningTokens: 1},
	}}
	e := setupResponsesRouter(t, &LLMHandler{svc: svc})
	rec := postResponses(t, e, `{"model":"req-model","instructions":"sys","input":"hi","temperature":0.5,"metadata":{"k":"v"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["object"] != "response" || raw["status"] != "completed" || !strings.HasPrefix(raw["id"].(string), "resp_") {
		t.Fatalf("envelope=%s", rec.Body.String())
	}
	// 官方 SDK 的响应模型把这些字段当必填：缺失会解析失败。
	for _, k := range []string{"created_at", "model", "output", "parallel_tool_calls", "tool_choice", "tools", "usage", "metadata", "error", "incomplete_details", "instructions", "temperature", "top_p"} {
		if _, ok := raw[k]; !ok {
			t.Fatalf("response object must carry %q: %s", k, rec.Body.String())
		}
	}
	if raw["model"] != "up-model" {
		t.Fatalf("model should report what actually ran, got %v", raw["model"])
	}
	if raw["instructions"] != "sys" || raw["tool_choice"] != "auto" || raw["error"] != nil || raw["incomplete_details"] != nil {
		t.Fatalf("echoed fields wrong: %s", rec.Body.String())
	}
	out := raw["output"].([]any)
	if len(out) != 1 {
		t.Fatalf("output=%v", out)
	}
	msg := out[0].(map[string]any)
	part := msg["content"].([]any)[0].(map[string]any)
	if msg["type"] != "message" || msg["role"] != "assistant" || msg["status"] != "completed" ||
		part["type"] != "output_text" || part["text"] != "hello" {
		t.Fatalf("message item=%v", msg)
	}
	if _, ok := part["annotations"].([]any); !ok {
		t.Fatalf("annotations must be [] not null: %v", part)
	}
	usage := raw["usage"].(map[string]any)
	if usage["input_tokens"] != float64(3) || usage["output_tokens"] != float64(4) || usage["total_tokens"] != float64(7) {
		t.Fatalf("usage=%v", usage)
	}
	if usage["output_tokens_details"].(map[string]any)["reasoning_tokens"] != float64(1) {
		t.Fatalf("reasoning tokens lost: %v", usage)
	}
}

func TestRespNonStreamToolCalls(t *testing.T) {
	svc := &fakeLLMSvc{resp: LLMResponse{
		FinishReason: "tool_calls",
		ToolCalls: []LLMToolCall{
			{ID: "call_a", Name: "shell", Arguments: `{"command":"pwd"}`},
			{Name: "glob", Arguments: ""},
		},
	}}
	e := setupResponsesRouter(t, &LLMHandler{svc: svc})
	rec := postResponses(t, e, `{"input":"x","tools":[{"type":"function","name":"shell"},{"type":"function","name":"glob"}]}`)
	var raw struct {
		Status string           `json:"status"`
		Output []map[string]any `json:"output"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	// 只有工具调用、没有正文：不应凭空多出一条空 message。
	if raw.Status != "completed" || len(raw.Output) != 2 {
		t.Fatalf("output=%s", rec.Body.String())
	}
	a, b := raw.Output[0], raw.Output[1]
	if a["type"] != "function_call" || a["call_id"] != "call_a" || a["name"] != "shell" || a["arguments"] != `{"command":"pwd"}` {
		t.Fatalf("call a=%v", a)
	}
	if b["type"] != "function_call" || b["name"] != "glob" || b["arguments"] != "" {
		t.Fatalf("call b=%v", b)
	}
	// 上游没给 id 时合成一个，客户端回灌 function_call_output 需要它。
	if id, _ := b["call_id"].(string); !strings.HasPrefix(id, "call_") {
		t.Fatalf("missing call_id must be synthesized: %v", b)
	}
	// arguments 即使为空串也必须出现（SDK 视为必填字符串）。
	if _, ok := b["arguments"]; !ok {
		t.Fatalf("arguments key dropped: %v", b)
	}
}

func TestRespNonStreamEmptyAndIncomplete(t *testing.T) {
	t.Run("empty_reply_still_has_a_message", func(t *testing.T) {
		svc := &fakeLLMSvc{resp: LLMResponse{ToolCalls: nil, Content: "", FinishReason: "stop", Model: "m"}}
		// Content 与 ToolCalls 都空会让 fakeLLMSvc 退回默认应答，改用显式空白正文以外的方式：直接测转换函数。
		_ = svc
		out := respOutputFromLLM(LLMResponse{})
		if len(out) != 1 {
			t.Fatalf("output=%v", out)
		}
		if m, ok := out[0].(oaiRespMessageItem); !ok || m.Content[0].Text != "" {
			t.Fatalf("item=%+v", out[0])
		}
	})
	t.Run("length_is_incomplete", func(t *testing.T) {
		svc := &fakeLLMSvc{resp: LLMResponse{Content: "cut", FinishReason: "length"}}
		e := setupResponsesRouter(t, &LLMHandler{svc: svc})
		rec := postResponses(t, e, `{"input":"x"}`)
		var raw struct {
			Status            string `json:"status"`
			IncompleteDetails struct {
				Reason string `json:"reason"`
			} `json:"incomplete_details"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		if raw.Status != "incomplete" || raw.IncompleteDetails.Reason != "max_output_tokens" {
			t.Fatalf("body=%s", rec.Body.String())
		}
	})
	t.Run("svc_error_is_openai_envelope", func(t *testing.T) {
		e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{chatErr: errors.New("boom sk-leak")}})
		rec := postResponses(t, e, `{"input":"x"}`)
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "sk-leak") {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
		assertOAIError(t, rec.Body.String(), "api_error", "internal_error")
	})
	t.Run("quota_denied", func(t *testing.T) {
		e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, usage: &fakeUsage{errLLM: errors.New("q")}})
		if rec := postResponses(t, e, `{"input":"x"}`); rec.Code == http.StatusOK {
			t.Fatalf("quota must reject: %s", rec.Body.String())
		}
	})
}

// ---------------------------------------------------------------------------
// 流式事件
// ---------------------------------------------------------------------------

func TestRespStreamTextEventSequence(t *testing.T) {
	reader := &seqStream{chunks: []*LLMStreamChunk{
		{Type: "content", Content: "he"},
		{Type: "content", Content: "llo"},
		{FinishReason: "stop"},
		{Done: true, Usage: &LLMTokenUsage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}},
	}}
	e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}})
	rec := postResponses(t, e, `{"model":"m","input":"hi","stream":true}`)
	if rec.Code != http.StatusOK || rec.Header().Get(echo.HeaderContentType) != "text/event-stream" {
		t.Fatalf("status=%d ct=%s", rec.Code, rec.Header().Get(echo.HeaderContentType))
	}
	evs := sseEvents(t, rec.Body.String())
	var names []string
	for _, ev := range evs {
		names = append(names, ev.Name)
	}
	want := []string{
		"response.created", "response.in_progress",
		"response.output_item.added", "response.content_part.added",
		"response.output_text.delta", "response.output_text.delta",
		"response.output_text.done", "response.content_part.done", "response.output_item.done",
		"response.completed",
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("events:\n got %v\nwant %v", names, want)
	}
	// 不得有 chat 流的 [DONE] 哨兵：Responses 流以终态事件收尾。
	if strings.Contains(rec.Body.String(), "[DONE]") {
		t.Fatalf("responses stream must not use the chat [DONE] sentinel")
	}
	// 每个事件的 data.type 与 event 名一致，sequence_number 从 0 单调递增。
	for i, ev := range evs {
		var d struct {
			Type string `json:"type"`
			Seq  int    `json:"sequence_number"`
		}
		if err := json.Unmarshal([]byte(ev.Data), &d); err != nil {
			t.Fatalf("event %d bad json: %q", i, ev.Data)
		}
		if d.Type != ev.Name || d.Seq != i {
			t.Fatalf("event %d: type=%q (event %q) seq=%d", i, d.Type, ev.Name, d.Seq)
		}
	}
	// delta 内容与最终聚合文本。
	var d1 struct {
		Delta string `json:"delta"`
	}
	_ = json.Unmarshal([]byte(evs[4].Data), &d1)
	if d1.Delta != "he" {
		t.Fatalf("first delta=%q", d1.Delta)
	}
	var done struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(evs[6].Data), &done)
	if done.Text != "hello" {
		t.Fatalf("output_text.done text=%q", done.Text)
	}
	// created 的响应骨架：in_progress、output 为 []、usage 为 null。
	var created struct {
		Response map[string]any `json:"response"`
	}
	_ = json.Unmarshal([]byte(evs[0].Data), &created)
	if created.Response["status"] != "in_progress" || created.Response["usage"] != nil ||
		len(created.Response["output"].([]any)) != 0 {
		t.Fatalf("created skeleton=%v", created.Response)
	}
	// completed 带完整 output 与 usage。
	var final struct {
		Response struct {
			Status string `json:"status"`
			Output []struct {
				Type    string `json:"type"`
				Status  string `json:"status"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
				TotalTokens  int `json:"total_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(evs[len(evs)-1].Data), &final); err != nil {
		t.Fatal(err)
	}
	r := final.Response
	if r.Status != "completed" || len(r.Output) != 1 || r.Output[0].Status != "completed" ||
		r.Output[0].Content[0].Text != "hello" || r.Usage.InputTokens != 3 || r.Usage.TotalTokens != 5 {
		t.Fatalf("completed=%+v", r)
	}
}

// 文本在前、工具调用在后：先收尾 message，再依次打开 function_call，output_index 递增。
func TestRespStreamToolCalls(t *testing.T) {
	reader := &seqStream{chunks: []*LLMStreamChunk{
		{Type: "content", Content: "let me check"},
		{Type: "tool_call_delta", ToolCallIndex: 0, ToolCallID: "call_1", ToolCallName: "shell"},
		{Type: "tool_call_delta", ToolCallIndex: 0, ToolCallArgs: `{"command":`},
		{Type: "tool_call_delta", ToolCallIndex: 0, ToolCallArgs: `"pwd"}`},
		{Type: "tool_call_delta", ToolCallIndex: 1, ToolCallID: "call_2", ToolCallName: "glob"},
		{Type: "tool_call_delta", ToolCallIndex: 1, ToolCallArgs: `{}`},
		{FinishReason: "tool_calls"},
		{Done: true},
	}}
	e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}})
	rec := postResponses(t, e, `{"input":"x","stream":true,"tools":[{"type":"function","name":"shell"},{"type":"function","name":"glob"}]}`)
	evs := sseEvents(t, rec.Body.String())

	// message 的收尾事件必须出现在第一个 function_call 打开之前。
	idxMsgDone, idxFirstFC := -1, -1
	for i, ev := range evs {
		if ev.Name == "response.output_item.done" && idxMsgDone < 0 {
			idxMsgDone = i
		}
		if ev.Name == "response.output_item.added" && strings.Contains(ev.Data, `"function_call"`) && idxFirstFC < 0 {
			idxFirstFC = i
		}
	}
	if idxMsgDone < 0 || idxFirstFC < 0 || idxMsgDone > idxFirstFC {
		t.Fatalf("message must be closed before tool calls open (done=%d firstFC=%d)", idxMsgDone, idxFirstFC)
	}

	// 按 output_index 聚合 arguments 增量，应还原出完整参数。
	args := map[int]string{}
	var argDone []struct {
		Idx  int
		Name string
		Args string
	}
	for _, ev := range evs {
		switch ev.Name {
		case "response.function_call_arguments.delta":
			var d struct {
				OutputIndex int    `json:"output_index"`
				Delta       string `json:"delta"`
			}
			_ = json.Unmarshal([]byte(ev.Data), &d)
			args[d.OutputIndex] += d.Delta
		case "response.function_call_arguments.done":
			var d struct {
				OutputIndex int    `json:"output_index"`
				Name        string `json:"name"`
				Arguments   string `json:"arguments"`
			}
			_ = json.Unmarshal([]byte(ev.Data), &d)
			argDone = append(argDone, struct {
				Idx  int
				Name string
				Args string
			}{d.OutputIndex, d.Name, d.Arguments})
		}
	}
	if args[1] != `{"command":"pwd"}` || args[2] != `{}` {
		t.Fatalf("aggregated deltas=%v", args)
	}
	if len(argDone) != 2 || argDone[0].Idx != 1 || argDone[0].Name != "shell" || argDone[0].Args != `{"command":"pwd"}` ||
		argDone[1].Idx != 2 || argDone[1].Name != "glob" {
		t.Fatalf("arguments.done=%+v", argDone)
	}

	var final struct {
		Response struct {
			Status string           `json:"status"`
			Output []map[string]any `json:"output"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(evs[len(evs)-1].Data), &final); err != nil {
		t.Fatal(err)
	}
	out := final.Response.Output
	if evs[len(evs)-1].Name != "response.completed" || len(out) != 3 ||
		out[0]["type"] != "message" || out[1]["type"] != "function_call" || out[2]["type"] != "function_call" {
		t.Fatalf("final output=%v last=%s", out, evs[len(evs)-1].Name)
	}
	if out[1]["call_id"] != "call_1" || out[1]["arguments"] != `{"command":"pwd"}` || out[2]["call_id"] != "call_2" {
		t.Fatalf("function_call items=%v", out)
	}
}

// 只有工具调用的流不应凭空多出一条空 message。
func TestRespStreamToolOnlyHasNoEmptyMessage(t *testing.T) {
	reader := &seqStream{chunks: []*LLMStreamChunk{
		{Type: "tool_call_delta", ToolCallIndex: 0, ToolCallID: "c", ToolCallName: "t", ToolCallArgs: "{}"},
		{Done: true},
	}}
	e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}})
	rec := postResponses(t, e, `{"input":"x","stream":true,"tools":[{"type":"function","name":"t"}]}`)
	if strings.Contains(rec.Body.String(), `"type":"message"`) {
		t.Fatalf("no message item expected: %s", rec.Body.String())
	}
	evs := sseEvents(t, rec.Body.String())
	if evs[len(evs)-1].Name != "response.completed" {
		t.Fatalf("last=%s", evs[len(evs)-1].Name)
	}
}

func TestRespStreamEmptyAndIncomplete(t *testing.T) {
	t.Run("empty_stream_gets_an_empty_message", func(t *testing.T) {
		e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: &seqStream{chunks: []*LLMStreamChunk{{Done: true}}}}})
		rec := postResponses(t, e, `{"input":"x","stream":true}`)
		evs := sseEvents(t, rec.Body.String())
		if evs[len(evs)-1].Name != "response.completed" || !strings.Contains(evs[len(evs)-1].Data, `"type":"message"`) {
			t.Fatalf("an empty stream must still produce one message: %s", rec.Body.String())
		}
	})
	t.Run("length_ends_incomplete", func(t *testing.T) {
		reader := &seqStream{chunks: []*LLMStreamChunk{{Type: "content", Content: "x"}, {FinishReason: "length"}, {Done: true}}}
		e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}})
		rec := postResponses(t, e, `{"input":"x","stream":true}`)
		evs := sseEvents(t, rec.Body.String())
		last := evs[len(evs)-1]
		if last.Name != "response.incomplete" || !strings.Contains(last.Data, `"max_output_tokens"`) {
			t.Fatalf("last=%s %s", last.Name, last.Data)
		}
	})
}

// 流已开始后上游出错：发标准的 response.failed 事件（chat 流做不到这一点），
// 且只用固定文案，不泄漏上游原文。
func TestRespStreamMidErrorEmitsFailedEvent(t *testing.T) {
	reader := &errAfterStream{first: &LLMStreamChunk{Type: "content", Content: "part"}, err: errors.New("upstream said sk-leak")}
	e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}})
	rec := postResponses(t, e, `{"input":"x","stream":true}`)
	evs := sseEvents(t, rec.Body.String())
	last := evs[len(evs)-1]
	if last.Name != "response.failed" {
		t.Fatalf("last=%s", last.Name)
	}
	if strings.Contains(rec.Body.String(), "sk-leak") {
		t.Fatalf("upstream error text leaked: %s", rec.Body.String())
	}
	var d struct {
		Response struct {
			Status string `json:"status"`
			Error  struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(last.Data), &d); err != nil {
		t.Fatal(err)
	}
	if d.Response.Status != "failed" || d.Response.Error.Code == "" || d.Response.Error.Message == "" {
		t.Fatalf("failed event=%+v", d.Response)
	}
	for _, ev := range evs {
		if ev.Name == "response.completed" {
			t.Fatalf("a failed stream must not also complete")
		}
	}
}

func TestRespStreamSetupErrorIsHTTPError(t *testing.T) {
	e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{streamErr: errors.New("no stream")}})
	rec := postResponses(t, e, `{"input":"x","stream":true}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "event:") {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	assertOAIError(t, rec.Body.String(), "api_error", "internal_error")
}

// 上游补发真实 id：合成的 call_id 被替换，已是上游给的 id 则不被覆盖。
func TestRespStreamCallIDHandling(t *testing.T) {
	reader := &seqStream{chunks: []*LLMStreamChunk{
		{Type: "tool_call_delta", ToolCallIndex: 0, ToolCallName: "a"},                         // 无 id：合成
		{Type: "tool_call_delta", ToolCallIndex: 0, ToolCallID: "late_id", ToolCallArgs: "{}"}, // 补发真实 id
		{Type: "tool_call_delta", ToolCallIndex: 1, ToolCallID: "real_1", ToolCallName: "b"},
		{Type: "tool_call_delta", ToolCallIndex: 1, ToolCallID: "other", ToolCallArgs: "{}"}, // 不得覆盖
		{Done: true},
	}}
	e := setupResponsesRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}})
	rec := postResponses(t, e, `{"input":"x","stream":true,"tools":[{"type":"function","name":"a"},{"type":"function","name":"b"}]}`)
	evs := sseEvents(t, rec.Body.String())
	var final struct {
		Response struct {
			Output []map[string]any `json:"output"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(evs[len(evs)-1].Data), &final); err != nil {
		t.Fatal(err)
	}
	out := final.Response.Output
	if len(out) != 2 || out[0]["call_id"] != "late_id" || out[1]["call_id"] != "real_1" {
		t.Fatalf("call ids=%v", out)
	}
}

// ---------------------------------------------------------------------------
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
