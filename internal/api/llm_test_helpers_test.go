package api

// llm_test_helpers_test.go：留在 api 的路由级 / SSE 用例仍需要的 LLM 网关假实现
//（LLM handler 自身的测试已随实现迁到 internal/api/llmhttp）。

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/api/llmhttp"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"sync"
)

type fakeLLMSvc struct {
	chatErr   error
	streamErr error
	listErr   error
	resp      httpkit.LLMResponse
	reader    httpkit.LLMStreamReader
	names     []string
	models    map[string][]string

	lastReq httpkit.LLMRequest
}

func (f *fakeLLMSvc) Chat(_ context.Context, _ string, req httpkit.LLMRequest) (httpkit.LLMResponse, error) {
	f.lastReq = req
	if f.chatErr != nil {
		return httpkit.LLMResponse{}, f.chatErr
	}
	if f.resp.Content == "" && len(f.resp.ToolCalls) == 0 {
		return httpkit.LLMResponse{Content: "ok", Model: "m", Provider: "p", FinishReason: "stop", Usage: httpkit.LLMTokenUsage{PromptTokens: 2, CompletionTokens: 3}}, nil
	}
	return f.resp, nil
}

func (f *fakeLLMSvc) Stream(_ context.Context, _ string, req httpkit.LLMRequest) (httpkit.LLMStreamReader, error) {
	f.lastReq = req
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	if f.reader != nil {
		return f.reader, nil
	}
	return &seqStream{chunks: []*httpkit.LLMStreamChunk{{Type: "delta", Content: "hel"}, {Type: "delta", Content: "lo", FinishReason: "stop"}}}, nil
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
	chunks []*httpkit.LLMStreamChunk
	i      int
}

func (s *seqStream) Next() (*httpkit.LLMStreamChunk, error) {
	if s.i >= len(s.chunks) {
		return nil, io.EOF
	}
	c := s.chunks[s.i]
	s.i++
	return c, nil
}
func (s *seqStream) Close() error { return nil }

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

// decodeChunk 解析一个 chat/completions 增量帧（路由级用例用；线上形态经 llmhttp 的类型别名解码）。
func decodeChunk(t *testing.T, frame string) llmhttp.ChatChunk {
	t.Helper()
	var ch llmhttp.ChatChunk
	if err := json.Unmarshal([]byte(frame), &ch); err != nil {
		t.Fatalf("frame is not a chunk: %q: %v", frame, err)
	}
	return ch
}

// assertOAIError 断言响应体是 OpenAI 错误信封，且 type / code 符合预期。
func assertOAIError(t *testing.T, body, wantType, wantCode string) {
	t.Helper()
	var env llmhttp.ErrorEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("not an error envelope: %q: %v", body, err)
	}
	if env.Error.Type != wantType || env.Error.Code != wantCode {
		t.Fatalf("error = type %q code %q, want type %q code %q (body %s)", env.Error.Type, env.Error.Code, wantType, wantCode, body)
	}
	if env.Error.Message == "" {
		t.Fatalf("error.message must not be empty: %s", body)
	}
	if strings.Contains(body, `"request_id"`) {
		t.Fatalf("legacy envelope field leaked: %s", body)
	}
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

// fakeUsage 是配额服务的最小假实现（与 api 同包测试同形）。
type fakeUsage struct {
	errLLM error
	errDB  error
	calls  []string
}

func (f *fakeUsage) CheckQuota(_ context.Context, _ string, kind string) error {
	f.calls = append(f.calls, kind)
	if kind == "llm" {
		return f.errLLM
	}
	return f.errDB
}

// fakeAudit 是审计服务的最小假实现（与 opshttp 测试同形）。
type fakeAudit struct {
	ops    []catalog.Operation
	err    error
	last   httpkit.AuditEvent
	recErr error
}

func (f *fakeAudit) Record(_ context.Context, e httpkit.AuditEvent) error {
	f.last = e
	return f.recErr
}

func (f *fakeAudit) ListOperations(context.Context, string, string, int) ([]catalog.Operation, error) {
	return f.ops, f.err
}

// scriptLLM 是可脚本化的 ChatClient 假实现（路由级用例用；handler 单测的同名实现在 internal/api/agenthttp）。
type scriptLLM struct {
	mu    sync.Mutex
	steps [][]cloudagent.StreamDelta
	n     int
}

func (s *scriptLLM) reset(steps [][]cloudagent.StreamDelta) {
	s.mu.Lock()
	s.steps = steps
	s.n = 0
	s.mu.Unlock()
}
func (s *scriptLLM) Chat(context.Context, string, cloudagent.ChatRequest) (cloudagent.ChatResponse, error) {
	return cloudagent.ChatResponse{Content: "done"}, nil
}
func (s *scriptLLM) Stream(context.Context, string, cloudagent.ChatRequest) (cloudagent.TokenStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deltas []cloudagent.StreamDelta
	if s.n < len(s.steps) {
		deltas = s.steps[s.n]
		s.n++
	} else {
		deltas = []cloudagent.StreamDelta{{Content: "done", Finish: true}}
	}
	return &stepDelta{deltas: deltas}, nil
}
