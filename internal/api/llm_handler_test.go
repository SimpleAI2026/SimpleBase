package api

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

// fakeLLMSvc 被本文件与 router_more / gocloudagent_access / sse_write_timeout 等测试共用，
// 并记录最近一次请求以便断言协议映射（工具、tool_choice、工具往返消息）。
type fakeLLMSvc struct {
	chatErr   error
	streamErr error
	listErr   error
	resp      LLMResponse
	reader    LLMStreamReader
	names     []string
	models    map[string][]string

	lastReq LLMRequest
}

func (f *fakeLLMSvc) Chat(_ context.Context, _ string, req LLMRequest) (LLMResponse, error) {
	f.lastReq = req
	if f.chatErr != nil {
		return LLMResponse{}, f.chatErr
	}
	if f.resp.Content == "" && len(f.resp.ToolCalls) == 0 {
		return LLMResponse{Content: "ok", Model: "m", Provider: "p", FinishReason: "stop", Usage: LLMTokenUsage{PromptTokens: 2, CompletionTokens: 3}}, nil
	}
	return f.resp, nil
}

func (f *fakeLLMSvc) Stream(_ context.Context, _ string, req LLMRequest) (LLMStreamReader, error) {
	f.lastReq = req
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	if f.reader != nil {
		return f.reader, nil
	}
	return &seqStream{chunks: []*LLMStreamChunk{{Type: "delta", Content: "hel"}, {Type: "delta", Content: "lo", FinishReason: "stop"}}}, nil
}

func (f *fakeLLMSvc) ListProviders(context.Context, string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.names, nil
}

func (f *fakeLLMSvc) ProviderModels(context.Context, string) (map[string][]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.models != nil {
		return f.models, nil
	}
	return map[string][]string{}, nil
}

type seqStream struct {
	chunks []*LLMStreamChunk
	i      int
}

func (s *seqStream) Next() (*LLMStreamChunk, error) {
	if s.i >= len(s.chunks) {
		return nil, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	return c, nil
}
func (s *seqStream) Close() error { return nil }

func setupLLMRouter(t *testing.T, h *LLMHandler, withProject, withPrincipal bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			if withPrincipal {
				ctx = WithPrincipal(ctx, auth.Principal{APIKeyID: "key-llm"})
			}
			if withProject {
				ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "t"})
			}
			ctx = WithRequestID(ctx, "rid-llm")
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.POST("/v1/chat/completions", h.ChatCompletions)
	e.GET("/v1/models", h.ListModels)
	return e
}

func userMsg(content string) []oaiMessage { return []oaiMessage{{Role: "user", Content: content}} }

// sseFrames 把 SSE 响应体拆成 data 负载列表（去掉 "data: " 前缀）。
func sseFrames(t *testing.T, body string) []string {
	t.Helper()
	var out []string
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data:") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return out
}

// decodeChunk 解析一个 chunk 帧。
func decodeChunk(t *testing.T, frame string) oaiChunk {
	t.Helper()
	var ch oaiChunk
	if err := json.Unmarshal([]byte(frame), &ch); err != nil {
		t.Fatalf("frame is not a chunk: %q: %v", frame, err)
	}
	return ch
}

// ---------------------------------------------------------------------------
// 非流式
// ---------------------------------------------------------------------------

func TestChatCompletions_NonStream(t *testing.T) {
	t.Run("missing_project", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, false, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hi")})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("bad_json_is_openai_error", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, true, true)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{"))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
		assertOAIError(t, rec.Body.String(), "invalid_request_error", "invalid_request")
	})
	t.Run("empty_messages", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, true, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
		assertOAIError(t, rec.Body.String(), "invalid_request_error", "invalid_request")
	})
	t.Run("quota_denied", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, usage: &fakeUsage{errLLM: errors.New("quota")}}, true, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hi")})
		if rec.Code == http.StatusOK {
			t.Fatalf("quota must reject: %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"type"`) {
			t.Fatalf("quota error must use the OpenAI envelope: %s", rec.Body.String())
		}
	})
	t.Run("svc_error_is_sanitized", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{chatErr: errors.New("upstream said sk-secret-123")}}, true, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hi")})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "sk-secret-123") {
			t.Fatalf("unknown upstream error text must not leak: %s", rec.Body.String())
		}
		assertOAIError(t, rec.Body.String(), "api_error", "internal_error")
	})
	t.Run("success_shape_audit_persist", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		store := systemdb.NewStoreForTest(db)
		aud := &fakeAudit{}
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, usage: &fakeUsage{}, audit: aud, store: store}, true, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hello world")})
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
		}
		var out oaiChatResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Object != "chat.completion" || !strings.HasPrefix(out.ID, "chatcmpl-") || out.Created == 0 {
			t.Fatalf("envelope=%+v", out)
		}
		if len(out.Choices) != 1 || out.Choices[0].Message.Role != "assistant" || out.Choices[0].Message.Content != "ok" {
			t.Fatalf("choices=%+v", out.Choices)
		}
		if out.Choices[0].FinishReason != "stop" {
			t.Fatalf("finish_reason=%q", out.Choices[0].FinishReason)
		}
		// 上游只回填分项、未回填 total 时按求和补齐。
		if out.Usage.PromptTokens != 2 || out.Usage.CompletionTokens != 3 || out.Usage.TotalTokens != 5 {
			t.Fatalf("usage=%+v", out.Usage)
		}
		if aud.last.Kind != "llm_chat_completions" || aud.last.Status != "ok" || aud.last.PrincipalID != "key-llm" {
			t.Fatalf("audit=%+v", aud.last)
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sys_llm_sessions WHERE project_id = ?`, "proj-1").Scan(&n); err != nil || n != 1 {
			t.Fatalf("sessions=%d err=%v", n, err)
		}
	})
	t.Run("persist_failure_does_not_fail_request", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		// 没有 schema：CreateLLMSession 会失败，但对话本身必须成功。
		store := systemdb.NewStoreForTest(db)
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, store: store}, true, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("x")})
		if rec.Code != http.StatusOK {
			t.Fatalf("chat should still succeed: %d %s", rec.Code, rec.Body.String())
		}
	})
}

// 工具调用：请求里的 tools / 工具往返消息 / tool_choice 必须原样进入 LLMRequest，
// 响应里的 tool_calls 必须以 OpenAI 形状返回，finish_reason 为 tool_calls。
func TestChatCompletions_ToolsRoundTrip(t *testing.T) {
	svc := &fakeLLMSvc{resp: LLMResponse{
		Model: "m", Provider: "p", FinishReason: "tool_calls",
		ToolCalls: []LLMToolCall{{ID: "call_1", Name: "shell", Arguments: `{"command":"pwd"}`}},
		Usage:     LLMTokenUsage{PromptTokens: 5, CompletionTokens: 6, TotalTokens: 11, ReasoningTokens: 2},
	}}
	e := setupLLMRouter(t, &LLMHandler{svc: svc}, true, true)

	body := `{
	  "model": "m",
	  "messages": [
	    {"role":"user","content":"run pwd"},
	    {"role":"assistant","content":"","tool_calls":[{"id":"call_0","type":"function","function":{"name":"shell","arguments":"{\"command\":\"ls\"}"}}]},
	    {"role":"tool","tool_call_id":"call_0","content":"a.go"}
	  ],
	  "tools": [{"type":"function","function":{"name":"shell","description":"run","parameters":{"type":"object","properties":{"command":{"type":"string"}}}}}],
	  "tool_choice": {"type":"function","function":{"name":"shell"}}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}

	// 请求侧映射。
	got := svc.lastReq
	if len(got.Tools) != 1 || got.Tools[0].Name != "shell" || !strings.Contains(string(got.Tools[0].Parameters), `"command"`) {
		t.Fatalf("tools=%+v", got.Tools)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("messages=%+v", got.Messages)
	}
	if tc := got.Messages[1].ToolCalls; len(tc) != 1 || tc[0].ID != "call_0" || tc[0].Arguments != `{"command":"ls"}` {
		t.Fatalf("assistant tool_calls lost: %+v", got.Messages[1])
	}
	if got.Messages[2].ToolCallID != "call_0" || got.Messages[2].Role != "tool" {
		t.Fatalf("tool result lost: %+v", got.Messages[2])
	}
	if !strings.Contains(string(got.ToolChoice), `"name":"shell"`) {
		t.Fatalf("tool_choice not forwarded: %s", got.ToolChoice)
	}

	// 响应侧形状。
	var out oaiChatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	c := out.Choices[0]
	if c.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason=%q", c.FinishReason)
	}
	if len(c.Message.ToolCalls) != 1 {
		t.Fatalf("tool_calls=%+v", c.Message.ToolCalls)
	}
	tc := c.Message.ToolCalls[0]
	if tc.ID != "call_1" || tc.Type != "function" || tc.Function.Name != "shell" || tc.Function.Arguments != `{"command":"pwd"}` {
		t.Fatalf("tool call=%+v", tc)
	}
	if out.Usage.CompletionTokensDetails == nil || out.Usage.CompletionTokensDetails.ReasoningTokens != 2 {
		t.Fatalf("reasoning tokens lost: %+v", out.Usage)
	}
}

// Arguments 必须以 JSON 字符串输出而不是对象——OpenAI 的既有约定。
func TestChatCompletions_ToolArgumentsAreAString(t *testing.T) {
	svc := &fakeLLMSvc{resp: LLMResponse{
		ToolCalls: []LLMToolCall{{ID: "c", Name: "t", Arguments: `{"a":1}`}},
	}}
	e := setupLLMRouter(t, &LLMHandler{svc: svc}, true, true)
	rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("x")})
	if !strings.Contains(rec.Body.String(), `"arguments":"{\"a\":1}"`) {
		t.Fatalf("arguments must be a JSON string: %s", rec.Body.String())
	}
}

// 空 finish_reason 且带工具调用时，应推断为 tool_calls，避免客户端把它当作「已答完」。
func TestChatCompletions_FinishReasonInferredFromToolCalls(t *testing.T) {
	svc := &fakeLLMSvc{resp: LLMResponse{ToolCalls: []LLMToolCall{{ID: "c", Name: "t", Arguments: "{}"}}}}
	e := setupLLMRouter(t, &LLMHandler{svc: svc}, true, true)
	rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("x")})
	var out oaiChatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish_reason=%q", out.Choices[0].FinishReason)
	}
}

// 请求校验：非 function 工具、空工具名、非法 tool_choice 都以 OpenAI 错误信封返回 400。
func TestChatCompletions_RequestValidation(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"non_function_tool", `{"messages":[{"role":"user","content":"x"}],"tools":[{"type":"code_interpreter","function":{"name":"x"}}]}`, "tools[0].type must be"},
		{"empty_tool_name", `{"messages":[{"role":"user","content":"x"}],"tools":[{"type":"function","function":{"name":" "}}]}`, "must not be empty"},
		{"bad_choice_string", `{"messages":[{"role":"user","content":"x"}],"tools":[{"type":"function","function":{"name":"t"}}],"tool_choice":"sometimes"}`, "tool_choice must be"},
		{"bad_choice_object", `{"messages":[{"role":"user","content":"x"}],"tools":[{"type":"function","function":{"name":"t"}}],"tool_choice":{"type":"function"}}`, "tool_choice object must be"},
		{"choice_without_tools", `{"messages":[{"role":"user","content":"x"}],"tool_choice":"required"}`, "requires tools"},
		{"choice_wrong_type", `{"messages":[{"role":"user","content":"x"}],"tools":[{"type":"function","function":{"name":"t"}}],"tool_choice":42}`, "tool_choice must be a string or an object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, true, true)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
			}
			assertOAIError(t, rec.Body.String(), "invalid_request_error", "invalid_request")
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("message should mention %q: %s", tc.want, rec.Body.String())
			}
		})
	}
	// 合法的 tool_choice 取值必须放行。
	for _, choice := range []string{`"none"`, `"auto"`, `"required"`, `{"type":"function","function":{"name":"t"}}`} {
		svc := &fakeLLMSvc{}
		e := setupLLMRouter(t, &LLMHandler{svc: svc}, true, true)
		body := `{"messages":[{"role":"user","content":"x"}],"tools":[{"type":"function","function":{"name":"t"}}],"tool_choice":` + choice + `}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("tool_choice %s rejected: %d %s", choice, rec.Code, rec.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// 流式
// ---------------------------------------------------------------------------

func TestChatCompletions_StreamFrameSequence(t *testing.T) {
	reader := &seqStream{chunks: []*LLMStreamChunk{
		{Type: "content", Content: "hel"},
		{Type: "content", Content: "lo"},
		{FinishReason: "stop"},
		{Done: true},
	}}
	e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}}, true, true)
	rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hi"), Stream: true, Model: "m"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get(echo.HeaderContentType); ct != "text/event-stream" {
		t.Fatalf("content-type=%q", ct)
	}
	frames := sseFrames(t, rec.Body.String())
	// role → hel → lo → finish → [DONE]
	if len(frames) != 5 {
		t.Fatalf("frames=%d: %v", len(frames), frames)
	}
	if frames[len(frames)-1] != oaiSSEDone {
		t.Fatalf("last frame=%q want [DONE]", frames[len(frames)-1])
	}
	first := decodeChunk(t, frames[0])
	if first.Object != "chat.completion.chunk" || first.Choices[0].Delta.Role != "assistant" {
		t.Fatalf("first frame must establish the assistant role: %+v", first)
	}
	if first.Model != "m" {
		t.Fatalf("model=%q", first.Model)
	}
	if got := decodeChunk(t, frames[1]).Choices[0].Delta.Content; got != "hel" {
		t.Fatalf("frame1 content=%q", got)
	}
	if got := decodeChunk(t, frames[2]).Choices[0].Delta.Content; got != "lo" {
		t.Fatalf("frame2 content=%q", got)
	}
	fin := decodeChunk(t, frames[3]).Choices[0]
	if fin.FinishReason == nil || *fin.FinishReason != "stop" {
		t.Fatalf("finish frame=%+v", fin)
	}
	// 同一次流的所有帧共用同一个 id。
	id := first.ID
	for _, f := range frames[:len(frames)-1] {
		if decodeChunk(t, f).ID != id {
			t.Fatalf("chunk ids must be stable within a stream")
		}
	}
	// 未结束的帧 finish_reason 必须是 null（OpenAI 形状），不是缺失。
	if !strings.Contains(frames[1], `"finish_reason":null`) {
		t.Fatalf("in-flight frames must carry finish_reason:null: %s", frames[1])
	}
	// 不得出现旧协议的 {"type":"end"} 帧。
	if strings.Contains(rec.Body.String(), `"type":"end"`) {
		t.Fatalf("legacy end frame leaked into the OpenAI stream: %s", rec.Body.String())
	}
}

// 工具调用增量：id/name 只在首个分片出现，后续分片只带 arguments，并按 index 聚合。
func TestChatCompletions_StreamToolCallDeltas(t *testing.T) {
	reader := &seqStream{chunks: []*LLMStreamChunk{
		{Type: "tool_call_delta", ToolCallIndex: 0, ToolCallID: "call_1", ToolCallName: "shell"},
		{Type: "tool_call_delta", ToolCallIndex: 0, ToolCallArgs: `{"command":`},
		{Type: "tool_call_delta", ToolCallIndex: 0, ToolCallArgs: `"pwd"}`},
		{Type: "tool_call_delta", ToolCallIndex: 1, ToolCallID: "call_2", ToolCallName: "glob"},
		{Type: "tool_call_delta", ToolCallIndex: 1, ToolCallArgs: `{}`},
		{FinishReason: ""}, // 上游没给 finish_reason：应推断为 tool_calls
		{Done: true},
	}}
	e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}}, true, true)
	rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("go"), Stream: true})
	frames := sseFrames(t, rec.Body.String())

	// 用客户端的方式把分片拼回完整调用：按 index 聚合。
	type call struct{ id, name, args string }
	calls := map[int]*call{}
	for _, f := range frames {
		if f == oaiSSEDone {
			continue
		}
		for _, ch := range decodeChunk(t, f).Choices {
			for _, tc := range ch.Delta.ToolCalls {
				c := calls[tc.Index]
				if c == nil {
					c = &call{}
					calls[tc.Index] = c
				}
				if tc.ID != "" {
					c.id = tc.ID
				}
				c.name += tc.Function.Name
				c.args += tc.Function.Arguments
			}
		}
	}
	if len(calls) != 2 {
		t.Fatalf("calls=%d want 2: %v", len(calls), frames)
	}
	if c := calls[0]; c.id != "call_1" || c.name != "shell" || c.args != `{"command":"pwd"}` {
		t.Fatalf("call0=%+v", *c)
	}
	if c := calls[1]; c.id != "call_2" || c.name != "glob" || c.args != `{}` {
		t.Fatalf("call1=%+v", *c)
	}

	// 只有首个分片携带 id/type；后续分片不得重复，否则客户端会把它当成新调用。
	var withID int
	for _, f := range frames {
		if f == oaiSSEDone {
			continue
		}
		for _, ch := range decodeChunk(t, f).Choices {
			for _, tc := range ch.Delta.ToolCalls {
				if tc.ID != "" {
					withID++
					if tc.Type != "function" {
						t.Fatalf("first fragment must carry type=function: %+v", tc)
					}
				}
			}
		}
	}
	if withID != 2 {
		t.Fatalf("id appeared on %d fragments, want exactly 2 (one per call)", withID)
	}

	// 结束原因应推断为 tool_calls。
	var fin *string
	for _, f := range frames {
		if f == oaiSSEDone {
			continue
		}
		for _, ch := range decodeChunk(t, f).Choices {
			if ch.FinishReason != nil {
				fin = ch.FinishReason
			}
		}
	}
	if fin == nil || *fin != "tool_calls" {
		t.Fatalf("finish_reason=%v want tool_calls", fin)
	}
}

// usage 尾帧只在 stream_options.include_usage=true 时发送，且 choices 为空数组。
func TestChatCompletions_StreamUsageFrame(t *testing.T) {
	mk := func() LLMStreamReader {
		return &seqStream{chunks: []*LLMStreamChunk{
			{Type: "content", Content: "a"},
			{FinishReason: "stop"},
			{Done: true, Usage: &LLMTokenUsage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7, ReasoningTokens: 1}},
		}}
	}
	t.Run("included_when_requested", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: mk()}}, true, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{
			Messages: userMsg("hi"), Stream: true, StreamOptions: &oaiStreamOptions{IncludeUsage: true},
		})
		frames := sseFrames(t, rec.Body.String())
		if frames[len(frames)-1] != oaiSSEDone {
			t.Fatalf("last frame=%q", frames[len(frames)-1])
		}
		usage := decodeChunk(t, frames[len(frames)-2])
		if len(usage.Choices) != 0 {
			t.Fatalf("usage frame must have empty choices: %+v", usage)
		}
		if usage.Usage == nil || usage.Usage.PromptTokens != 3 || usage.Usage.TotalTokens != 7 {
			t.Fatalf("usage=%+v", usage.Usage)
		}
		if usage.Usage.CompletionTokensDetails == nil || usage.Usage.CompletionTokensDetails.ReasoningTokens != 1 {
			t.Fatalf("reasoning tokens lost: %+v", usage.Usage)
		}
		// usage 帧必须在 finish_reason 帧之后。
		fin := decodeChunk(t, frames[len(frames)-3]).Choices[0]
		if fin.FinishReason == nil {
			t.Fatalf("frame before usage must be the finish frame")
		}
	})
	t.Run("omitted_by_default", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: mk()}}, true, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hi"), Stream: true})
		if strings.Contains(rec.Body.String(), `"usage"`) {
			t.Fatalf("usage frame must not be sent unless requested: %s", rec.Body.String())
		}
	})
}

// reasoning 等非标准块不得泄漏进 OpenAI 流。
func TestChatCompletions_StreamDropsNonStandardChunks(t *testing.T) {
	reader := &seqStream{chunks: []*LLMStreamChunk{
		{Type: "reasoning", Content: "THINKING"},
		{Type: "content", Content: "answer"},
		{FinishReason: "stop"},
		{Done: true},
	}}
	e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}}, true, true)
	rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hi"), Stream: true})
	if strings.Contains(rec.Body.String(), "THINKING") {
		t.Fatalf("reasoning chunk leaked into the stream: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"content":"answer"`) {
		t.Fatalf("content lost: %s", rec.Body.String())
	}
}

// 流在服务端出错（响应头已发出）时，不得发明错误帧：只终止流，客户端凭缺失 [DONE] 判定异常。
func TestChatCompletions_StreamMidErrorTerminatesWithoutDone(t *testing.T) {
	reader := &errAfterStream{first: &LLMStreamChunk{Type: "content", Content: "part"}, err: errors.New("boom")}
	e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{reader: reader}}, true, true)
	rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hi"), Stream: true})
	body := rec.Body.String()
	if strings.Contains(body, oaiSSEDone) {
		t.Fatalf("an aborted stream must not claim a clean finish: %s", body)
	}
	if strings.Contains(body, "boom") || strings.Contains(body, `"error"`) {
		t.Fatalf("no error frame may be invented mid-stream: %s", body)
	}
}

type errAfterStream struct {
	first *LLMStreamChunk
	err   error
	sent  bool
}

func (s *errAfterStream) Next() (*LLMStreamChunk, error) {
	if !s.sent {
		s.sent = true
		return s.first, nil
	}
	return nil, s.err
}
func (s *errAfterStream) Close() error { return nil }

func TestChatCompletions_StreamSetupErrors(t *testing.T) {
	t.Run("svc_error_before_first_byte_is_http_error", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{streamErr: errors.New("no stream")}}, true, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hi"), Stream: true})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
		assertOAIError(t, rec.Body.String(), "api_error", "internal_error")
	})
	t.Run("quota_rejects_before_streaming", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}, usage: &fakeUsage{errLLM: errors.New("q")}}, true, true)
		rec := doRequest(e, http.MethodPost, "/v1/chat/completions", oaiChatRequest{Messages: userMsg("hi"), Stream: true})
		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "data:") {
			t.Fatal("quota should fail before the stream starts")
		}
	})
}

// ---------------------------------------------------------------------------
// 模型列表
// ---------------------------------------------------------------------------

func TestListModels(t *testing.T) {
	t.Run("missing_project", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, false, true)
		rec := doRequest(e, http.MethodGet, "/v1/models", nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})
	t.Run("error", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{listErr: errors.New("x")}}, true, true)
		rec := doRequest(e, http.MethodGet, "/v1/models", nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
		assertOAIError(t, rec.Body.String(), "api_error", "internal_error")
	})
	t.Run("empty_is_an_empty_list_not_null", func(t *testing.T) {
		e := setupLLMRouter(t, &LLMHandler{svc: &fakeLLMSvc{}}, true, true)
		rec := doRequest(e, http.MethodGet, "/v1/models", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		// SDK 会遍历 data；null 会让部分客户端崩溃。
		if !strings.Contains(rec.Body.String(), `"data":[]`) {
			t.Fatalf("data must be [] not null: %s", rec.Body.String())
		}
	})
	t.Run("strict_openai_list_sorted", func(t *testing.T) {
		svc := &fakeLLMSvc{models: map[string][]string{
			"openai":    {"gpt-b", "gpt-a"},
			"anthropic": {"claude-x"},
		}}
		e := setupLLMRouter(t, &LLMHandler{svc: svc}, true, true)
		rec := doRequest(e, http.MethodGet, "/v1/models", nil)
		var out oaiModelList
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Object != "list" {
			t.Fatalf("object=%q", out.Object)
		}
		var ids []string
		for _, m := range out.Data {
			if m.Object != "model" || m.Created == 0 {
				t.Fatalf("model=%+v", m)
			}
			ids = append(ids, m.OwnedBy+"/"+m.ID)
		}
		// 先按供应商名、再按模型名排序，顺序稳定可复现。
		want := "anthropic/claude-x,openai/gpt-a,openai/gpt-b"
		if strings.Join(ids, ",") != want {
			t.Fatalf("ids=%v want %s", ids, want)
		}
		// 纯 OpenAI 形状：不得夹带私有字段。
		var raw map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &raw)
		for _, m := range raw["data"].([]any) {
			for k := range m.(map[string]any) {
				switch k {
				case "id", "object", "created", "owned_by":
				default:
					t.Fatalf("unexpected non-OpenAI field %q in model object", k)
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// 辅助
// ---------------------------------------------------------------------------

// assertOAIError 断言响应体是 OpenAI 错误信封，且 type/code 符合预期。
func assertOAIError(t *testing.T, body, wantType, wantCode string) {
	t.Helper()
	var env oaiErrorBody
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("not an error envelope: %q: %v", body, err)
	}
	if env.Error.Type != wantType || env.Error.Code != wantCode {
		t.Fatalf("error = type %q code %q, want type %q code %q (body %s)",
			env.Error.Type, env.Error.Code, wantType, wantCode, body)
	}
	if env.Error.Message == "" {
		t.Fatalf("error.message must not be empty: %s", body)
	}
	// 不得出现既有信封的 request_id 字段——那会让严格 SDK 误判形状。
	if strings.Contains(body, `"request_id"`) {
		t.Fatalf("legacy envelope field leaked: %s", body)
	}
}

func TestLLMHandler_recordAuditNil(t *testing.T) {
	h := &LLMHandler{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h.recordAudit(c, "p", "k", "ok")                       // no panic when audit is nil
	h.persistChat(c, "p", oaiChatRequest{}, LLMResponse{}) // store nil
}
