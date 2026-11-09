package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// 捕获服务端收到的请求，供断言 wire 形状。
type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   []byte
}

func newCaptureServer(t *testing.T, cap *capturedRequest, handler http.HandlerFunc) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.Method, cap.Path, cap.Auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		cap.Body, _ = io.ReadAll(r.Body)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return &HTTPClient{Endpoint: srv.URL, Token: "sk-test", ProjectID: "proj1"}
}

func sampleToolRequest() agentcore.ChatRequest {
	return agentcore.ChatRequest{
		Model: "m",
		Messages: []agentcore.ChatMessage{
			{Role: "user", Content: "run pwd"},
			{Role: "assistant", ToolCalls: []agentcore.ChatToolCall{{ID: "c0", Name: "shell", Arguments: `{"command":"ls"}`}}},
			{Role: "tool", ToolCallID: "c0", Content: "a.go"},
		},
		Tools: []agentcore.ToolSpec{{
			Name: "shell", Description: "run a command",
			Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		}},
	}
}

// 路径必须是 {endpoint}/v1/projects/{project}/v1/chat/completions（双 v1）。
func TestClientURLHasDoubleV1(t *testing.T) {
	c := &HTTPClient{Endpoint: "http://127.0.0.1:8080/", ProjectID: "faayefdq"}
	if got := c.url("/chat/completions"); got != "http://127.0.0.1:8080/v1/projects/faayefdq/v1/chat/completions" {
		t.Fatalf("url=%s", got)
	}
	if got := c.url("/models"); got != "http://127.0.0.1:8080/v1/projects/faayefdq/v1/models" {
		t.Fatalf("url=%s", got)
	}
}

// 请求体必须是标准 OpenAI 嵌套：tools[].function、tool_calls[].function，
// 而不是旧协议的平铺 {name, description, parameters}。
func TestChatRequestIsOpenAIShaped(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"model":"m","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`)
	})
	if _, err := c.Chat(context.Background(), "", sampleToolRequest()); err != nil {
		t.Fatal(err)
	}
	if cap.Method != http.MethodPost || cap.Path != "/v1/projects/proj1/v1/chat/completions" {
		t.Fatalf("request = %s %s", cap.Method, cap.Path)
	}
	if cap.Auth != "Bearer sk-test" {
		t.Fatalf("auth=%q", cap.Auth)
	}

	var body struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name       string          `json:"name"`
				Parameters json.RawMessage `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(cap.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Stream {
		t.Fatal("non-stream request must not set stream")
	}
	if len(body.Tools) != 1 || body.Tools[0].Type != "function" || body.Tools[0].Function.Name != "shell" {
		t.Fatalf("tools must be nested under function: %s", cap.Body)
	}
	if !strings.Contains(string(body.Tools[0].Function.Parameters), `"command"`) {
		t.Fatalf("parameters lost: %s", cap.Body)
	}
	if len(body.Messages) != 3 {
		t.Fatalf("messages=%s", cap.Body)
	}
	tc := body.Messages[1].ToolCalls
	if len(tc) != 1 || tc[0].Type != "function" || tc[0].Function.Name != "shell" || tc[0].Function.Arguments != `{"command":"ls"}` {
		t.Fatalf("assistant tool_calls must be nested under function: %s", cap.Body)
	}
	if body.Messages[2].ToolCallID != "c0" {
		t.Fatalf("tool result id lost: %s", cap.Body)
	}
	// 旧协议的平铺字段不得残留。
	if strings.Contains(string(cap.Body), `"description":""`) {
		t.Fatalf("empty fields should be omitted: %s", cap.Body)
	}
}

// 响应从 choices[0].message 取内容与工具调用，finish_reason 取自 choices[0]。
func TestChatParsesChoices(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"model":"gpt","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_9","type":"function","function":{"name":"shell","arguments":"{\"command\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":6,"total_tokens":11,"completion_tokens_details":{"reasoning_tokens":2}}}`)
	})
	got, err := c.Chat(context.Background(), "", sampleToolRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.FinishReason != "tool_calls" || got.Model != "gpt" {
		t.Fatalf("resp=%+v", got)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].ID != "call_9" || got.ToolCalls[0].Name != "shell" || got.ToolCalls[0].Arguments != `{"command":"pwd"}` {
		t.Fatalf("tool calls=%+v", got.ToolCalls)
	}
	if got.Usage.PromptTokens != 5 || got.Usage.CompletionTokens != 6 || got.Usage.ReasoningTokens != 2 {
		t.Fatalf("usage=%+v", got.Usage)
	}
}

func TestChatEmptyChoicesIsNotAPanic(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"model":"m","choices":[]}`)
	})
	got, err := c.Chat(context.Background(), "", agentcore.ChatRequest{Messages: []agentcore.ChatMessage{{Role: "user", Content: "x"}}})
	if err != nil || got.Content != "" || len(got.ToolCalls) != 0 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

// 流式：请求带 stream 与 include_usage；帧序列按增量产出；[DONE] 收尾。
func TestStreamParsesDeltasAndDone(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, f := range []string{
			`{"choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"choices":[{"index":0,"delta":{"content":"he"},"finish_reason":null}]}`,
			`{"choices":[{"index":0,"delta":{"content":"llo"},"finish_reason":null}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"shell","arguments":""}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"pwd\"}"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"completion_tokens_details":{"reasoning_tokens":1}}}`,
		} {
			_, _ = io.WriteString(w, ": ping\n\ndata: "+f+"\n\n")
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	stream, err := c.Stream(context.Background(), "", sampleToolRequest())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var sent struct {
		Stream        bool `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	if err := json.Unmarshal(cap.Body, &sent); err != nil {
		t.Fatal(err)
	}
	if !sent.Stream || !sent.StreamOptions.IncludeUsage {
		t.Fatalf("stream request must set stream and include_usage: %s", cap.Body)
	}

	ds, ok := stream.(agentcore.DeltaStream)
	if !ok {
		t.Fatal("stream must implement DeltaStream, otherwise agentcore drops tool call deltas")
	}
	var text strings.Builder
	var name, id, args string
	var usage *agentcore.ChatUsage
	for {
		d, err := ds.NextDelta()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		text.WriteString(d.Content)
		if tc := d.ToolCall; tc != nil {
			if tc.ID != "" {
				id = tc.ID
			}
			name += tc.Name
			args += tc.ArgsDelta
		}
		if d.Usage != nil {
			usage = d.Usage
		}
	}
	if text.String() != "hello" {
		t.Fatalf("text=%q", text.String())
	}
	if id != "c1" || name != "shell" || args != `{"command":"pwd"}` {
		t.Fatalf("tool call id=%q name=%q args=%q", id, name, args)
	}
	if usage == nil || usage.PromptTokens != 3 || usage.CompletionTokens != 4 || usage.ReasoningTokens != 1 {
		t.Fatalf("usage=%+v", usage)
	}
}

// [DONE] 之后不能再读：重复调用仍返回 EOF，而不是卡在已关闭的连接上。
func TestStreamStopsAtDone(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\ndata: [DONE]\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"AFTER\"}}]}\n\n")
	})
	stream, err := c.Stream(context.Background(), "", agentcore.ChatRequest{Messages: []agentcore.ChatMessage{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	ds := stream.(agentcore.DeltaStream)
	var got strings.Builder
	for {
		d, err := ds.NextDelta()
		if err != nil {
			break
		}
		got.WriteString(d.Content)
	}
	if got.String() != "a" {
		t.Fatalf("content after [DONE] must be ignored, got %q", got.String())
	}
	if _, err := ds.NextDelta(); !errors.Is(err, io.EOF) {
		t.Fatalf("NextDelta after done: %v", err)
	}
}

// Next（兼容 TokenStream 的形态）只返回正文。
func TestStreamNextReturnsContentOnly(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c\",\"function\":{\"name\":\"t\"}}]}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\ndata: [DONE]\n\n")
	})
	stream, _ := c.Stream(context.Background(), "", agentcore.ChatRequest{Messages: []agentcore.ChatMessage{{Role: "user", Content: "q"}}})
	defer stream.Close()
	text, _, err := stream.Next()
	if err != nil || text != "x" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	if _, _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("err=%v", err)
	}
}

// 兼容错误信封：新端点是 OpenAI 形状，其余路由是既有形状，同一份解析两者都能读。
func TestAPIErrorBothEnvelopes(t *testing.T) {
	openai := `{"error":{"message":"invalid api key","type":"authentication_error","param":null,"code":"invalid_api_key"}}`
	if err := apiError(401, []byte(openai)); err == nil || !strings.Contains(err.Error(), "invalid_api_key (http 401): invalid api key") {
		t.Fatalf("openai envelope: %v", err)
	}
	legacy := `{"error":{"code":"forbidden","message":"permission denied","request_id":"r"}}`
	if err := apiError(403, []byte(legacy)); err == nil || !strings.Contains(err.Error(), "forbidden (http 403): permission denied") {
		t.Fatalf("legacy envelope: %v", err)
	}
	// 无 code 时落到 api_error。
	if err := apiError(500, []byte(`{"error":{"message":"boom"}}`)); err == nil || !strings.Contains(err.Error(), "api_error (http 500): boom") {
		t.Fatalf("no code: %v", err)
	}
	// 非 JSON（如 SPA fallback 的 HTML）按原文截断。
	if err := apiError(502, []byte("<html>bad gateway</html>")); err == nil || !strings.Contains(err.Error(), "bad gateway") {
		t.Fatalf("non-json: %v", err)
	}
}

// 服务端错误经 Chat / Stream 都要变成带 code 的错误，且流式不会在非 2xx 时返回读取器。
func TestClientSurfacesServerErrors(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"message":"permission denied","type":"permission_error","param":null,"code":"forbidden"}}`)
	})
	req := agentcore.ChatRequest{Messages: []agentcore.ChatMessage{{Role: "user", Content: "x"}}}
	if _, err := c.Chat(context.Background(), "", req); err == nil || !strings.Contains(err.Error(), "forbidden (http 403)") {
		t.Fatalf("chat err=%v", err)
	}
	if s, err := c.Stream(context.Background(), "", req); err == nil || s != nil || !strings.Contains(err.Error(), "forbidden (http 403)") {
		t.Fatalf("stream s=%v err=%v", s, err)
	}
}

// Probe 打 GET .../v1/models：权限与对话共用 llm:invoke。
func TestProbeHitsModels(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"object":"list","data":[]}`)
	})
	if err := c.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cap.Method != http.MethodGet || cap.Path != "/v1/projects/proj1/v1/models" {
		t.Fatalf("probe = %s %s", cap.Method, cap.Path)
	}
}

func TestProbeSurfacesAuthFailure(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid api key","type":"authentication_error","param":null,"code":"invalid_api_key"}}`)
	})
	if err := c.Probe(context.Background()); err == nil || !strings.Contains(err.Error(), "http 401") {
		t.Fatalf("probe err=%v", err)
	}
}

// runner 现在默认请求原生工具协议：带工具时，真实客户端发出的请求里必须有 tools 声明，
// 且不得出现文本协议的 TOOL_CALL 提示。这是本次改造的核心收益——此前 CLI 恒为文本协议，
// 要靠模型手写 TOOL_CALL，格式稍有偏差就整轮失败。
func TestRunnerRequestsNativeTools(t *testing.T) {
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\ndata: [DONE]\n\n")
	})
	s := newTestSession(t)
	var echoCalls int
	r := &SessionRunner{Session: s, Client: c, Tools: []tool.BaseTool{echoTool(t, &echoCalls)}}
	if _, err := r.Run(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(cap.Body, &sent); err != nil {
		t.Fatalf("request body: %v: %s", err, cap.Body)
	}
	if len(sent.Tools) != 1 || sent.Tools[0].Type != "function" || sent.Tools[0].Function.Name != "echo" {
		t.Fatalf("native tools must be declared in the request: %s", cap.Body)
	}
	for _, m := range sent.Messages {
		if strings.Contains(m.Content, "TOOL_CALL") {
			t.Fatalf("native protocol must not inject the text tool prompt: %s", cap.Body)
		}
	}
}

// 端到端：服务端以标准 tool_calls 增量返回工具调用 → runner 执行真实工具 → 结果回灌。
// 这正是用户此前遇到的「调用 tool 就结束了」的反例。
func TestRunnerExecutesNativeToolCallEndToEnd(t *testing.T) {
	var round int
	var secondBody []byte
	var cap capturedRequest
	c := newCaptureServer(t, &cap, func(w http.ResponseWriter, _ *http.Request) {
		round++
		w.Header().Set("Content-Type", "text/event-stream")
		if round == 1 {
			for _, f := range []string{
				`{"choices":[{"delta":{"role":"assistant"}}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":""}}]}}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"text\":\"ping\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			} {
				_, _ = io.WriteString(w, "data: "+f+"\n\n")
			}
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		secondBody = append([]byte(nil), cap.Body...)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"final answer\"}}]}\n\ndata: [DONE]\n\n")
	})
	s := newTestSession(t)
	var echoCalls int
	r := &SessionRunner{Session: s, Client: c, Tools: []tool.BaseTool{echoTool(t, &echoCalls)}}
	var events []CLIEvent
	res, err := r.Run(context.Background(), "please echo", func(e CLIEvent) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if echoCalls != 1 {
		t.Fatalf("the tool must really run exactly once, ran %d times (round=%d)", echoCalls, round)
	}
	if res.Content != "final answer" {
		t.Fatalf("content=%q", res.Content)
	}
	var sawCall, sawResult bool
	for _, e := range events {
		switch e.Type {
		case "tool_call":
			sawCall = e.CallID == "call_1" && e.Name == "echo"
		case "tool_result":
			sawResult = e.CallID == "call_1" && !e.IsError
		}
	}
	if !sawCall || !sawResult {
		t.Fatalf("missing tool events: %+v", events)
	}
	// 第二轮请求必须把工具结果回灌，且以标准形状携带 tool_call_id。
	var follow struct {
		Messages []struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID string `json:"id"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(secondBody, &follow); err != nil {
		t.Fatalf("second request: %v: %s", err, secondBody)
	}
	var sawAssistantCall, sawToolResult bool
	for _, m := range follow.Messages {
		if m.Role == "assistant" && len(m.ToolCalls) == 1 && m.ToolCalls[0].ID == "call_1" {
			sawAssistantCall = true
		}
		if m.Role == "tool" && m.ToolCallID == "call_1" {
			sawToolResult = true
		}
	}
	if !sawAssistantCall || !sawToolResult {
		t.Fatalf("tool round trip not sent back to the model: %s", secondBody)
	}
}

// 回归：响应头超时不能短于推理模型的正常首字节。30s 会在上游还没写响应头时掐掉整轮。
func TestDefaultClientWaitsForSlowResponseHeaders(t *testing.T) {
	c := &HTTPClient{}
	hc := c.httpClient()
	tr, ok := hc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type %T", hc.Transport)
	}
	if tr.ResponseHeaderTimeout < 3*time.Minute {
		t.Fatalf("header timeout %s is shorter than a reasoning model's first byte", tr.ResponseHeaderTimeout)
	}
	if hc.Timeout < tr.ResponseHeaderTimeout {
		t.Fatalf("overall timeout %s is shorter than header timeout %s", hc.Timeout, tr.ResponseHeaderTimeout)
	}
}
