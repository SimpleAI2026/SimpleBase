package llmgateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/voocel/litellm/providers"
)

// fakeTracer 收集 RecordTrace 调用（planv6.5 §3.2 单测桩）。
type fakeTracer struct {
	mu     sync.Mutex
	traces []LLMTrace
	err    error
}

func (f *fakeTracer) RecordTrace(ctx context.Context, t LLMTrace) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	f.traces = append(f.traces, t)
	f.mu.Unlock()
	return nil
}

func (f *fakeTracer) snapshot() []LLMTrace {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]LLMTrace(nil), f.traces...)
}

// Chat 成功：记一条 status=ok、带 tokens 与耗时的 trace。
func TestChatRecordsTrace(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	srv := openaiCompatServer(t, false)
	defer srv.Close()

	tr := &fakeTracer{}
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, tr, nil)
	if _, err := svc.Chat(context.Background(), "proj-a", Request{
		Messages: []providers.Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	got := tr.snapshot()
	if len(got) != 1 {
		t.Fatalf("traces=%d want 1", len(got))
	}
	trc := got[0]
	if trc.ProjectID != "proj-a" || trc.Status != "ok" || trc.Stream {
		t.Fatalf("trace=%+v", trc)
	}
	if trc.InputTokens == 0 || trc.OutputTokens == 0 {
		t.Fatalf("tokens=%+v", trc)
	}
	if trc.DurationMs < 0 {
		t.Fatalf("duration=%d", trc.DurationMs)
	}
}

// Chat 上游错误：记一条 status=error、带错误信息的 trace。
func TestChatRecordsErrorTrace(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	// 返回 500 的假上游。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := &fakeTracer{}
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, tr, nil)
	_, err := svc.Chat(context.Background(), "proj-a", Request{
		Messages: []providers.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("want upstream error")
	}
	got := tr.snapshot()
	if len(got) != 1 {
		t.Fatalf("traces=%d want 1", len(got))
	}
	if got[0].Status != "error" || got[0].ErrorCode == "" || got[0].ErrorMessage == "" {
		t.Fatalf("trace=%+v", got[0])
	}
}

// Stream 成功：reader 读到 Done 后记一条 stream=true 的 trace。
func TestStreamRecordsTrace(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	srv := openaiCompatServer(t, false)
	defer srv.Close()

	tr := &fakeTracer{}
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, tr, nil)
	reader, err := svc.Stream(context.Background(), "proj-a", Request{
		Messages: []providers.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for {
		chunk, rerr := reader.Next()
		if rerr != nil {
			t.Fatal(rerr)
		}
		if chunk.Done {
			break
		}
	}
	got := tr.snapshot()
	if len(got) != 1 {
		t.Fatalf("traces=%d want 1", len(got))
	}
	if !got[0].Stream || got[0].Status != "ok" {
		t.Fatalf("trace=%+v", got[0])
	}
}

// tracer 报错不影响主流程；无 tracer（nil）静默跳过。
func TestTraceRecorderErrorsIgnored(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	srv := openaiCompatServer(t, false)
	defer srv.Close()

	tr := &fakeTracer{err: errors.New("trace down")}
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, tr, nil)
	if _, err := svc.Chat(context.Background(), "proj-a", Request{
		Messages: []providers.Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatal(err)
	}

	svcNil := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, nil, nil)
	if _, err := svcNil.Chat(context.Background(), "proj-b", Request{
		Messages: []providers.Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatal(err)
	}
}

// traceError 的截断由存储层兜底；这里只验证非空错误产生非空 code/message。
func TestTraceErrorHelper(t *testing.T) {
	code, msg := traceError(errors.New("boom"))
	if code == "" || msg != "boom" {
		t.Fatalf("code=%q msg=%q", code, msg)
	}
	if code, msg := traceError(nil); code != "" || msg != "" {
		t.Fatalf("nil err → %q %q", code, msg)
	}
}

// —— 二期（planv6.5 §9）：内容观测埋点 + ctx 元数据穿透 ——

// ctx 元数据注入/读取往返；未注入返回零值。
func TestTraceMetaContext(t *testing.T) {
	if m := TraceMetaFrom(context.Background()); m.TraceID != "" || m.Caller != "" {
		t.Fatalf("empty ctx=%+v", m)
	}
	ctx := WithTraceMeta(context.Background(), "run-1", "agent")
	m := TraceMetaFrom(ctx)
	if m.TraceID != "run-1" || m.Caller != "agent" {
		t.Fatalf("meta=%+v", m)
	}
	// 覆盖写
	ctx = WithTraceMeta(ctx, "run-2", "cron")
	if m := TraceMetaFrom(ctx); m.TraceID != "run-2" || m.Caller != "cron" {
		t.Fatalf("overwrite=%+v", m)
	}
}

// Chat 成功：input/output/tools JSON 快照 + ctx 元数据穿透到 trace。
func TestChatTraceContent(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	srv := openaiCompatServer(t, false)
	defer srv.Close()

	tr := &fakeTracer{}
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, tr, nil)
	ctx := WithTraceMeta(context.Background(), "run-42", "agent")
	_, err := svc.Chat(ctx, "proj-a", Request{
		Messages: []providers.Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}},
		Tools:    []providers.Tool{{Function: providers.FunctionDef{Name: "sql_query", Description: "run sql"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := tr.snapshot()
	if len(got) != 1 {
		t.Fatalf("traces=%d want 1", len(got))
	}
	trc := got[0]
	if trc.TraceID != "run-42" || trc.Caller != "agent" {
		t.Fatalf("meta=%+v", trc)
	}
	if trc.Name != trc.Model {
		t.Fatalf("name fallback=%+v", trc)
	}
	// input_json：消息数组快照（system prompt 可见）
	if !strings.Contains(trc.InputJSON, `"role":"system"`) || !strings.Contains(trc.InputJSON, "sys") {
		t.Fatalf("input=%s", trc.InputJSON)
	}
	// output_json：正文
	if !strings.Contains(trc.OutputJSON, `"content"`) {
		t.Fatalf("output=%s", trc.OutputJSON)
	}
	// tools_json：name + description
	if !strings.Contains(trc.ToolsJSON, "sql_query") || !strings.Contains(trc.ToolsJSON, "run sql") {
		t.Fatalf("tools=%s", trc.ToolsJSON)
	}
}

// Chat 错误：input 快照照记，output 记 {"error":...}；meta 穿透。
func TestChatErrorTraceContent(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := &fakeTracer{}
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, tr, nil)
	ctx := WithTraceMeta(context.Background(), "req-x", "api")
	_, err := svc.Chat(ctx, "proj-a", Request{
		Messages: []providers.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("want error")
	}
	got := tr.snapshot()
	if len(got) != 1 {
		t.Fatalf("traces=%d", len(got))
	}
	if got[0].TraceID != "req-x" || got[0].Caller != "api" {
		t.Fatalf("meta=%+v", got[0])
	}
	if !strings.Contains(got[0].InputJSON, "hi") {
		t.Fatalf("input=%s", got[0].InputJSON)
	}
	if !strings.Contains(got[0].OutputJSON, "error") {
		t.Fatalf("output=%s", got[0].OutputJSON)
	}
}

// Stream 成功：流式 content 聚合进 output_json。
func TestStreamTraceContent(t *testing.T) {
	if testing.Short() {
		t.Skip("local provider HTTP")
	}
	srv := openaiCompatServer(t, false)
	defer srv.Close()

	tr := &fakeTracer{}
	svc := NewService(&fakeResolver{providers: testProviders(srv.URL)}, nil, tr, nil)
	ctx := WithTraceMeta(context.Background(), "run-s", "cron")
	reader, err := svc.Stream(ctx, "proj-a", Request{
		Messages: []providers.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for {
		chunk, rerr := reader.Next()
		if rerr != nil {
			t.Fatal(rerr)
		}
		if chunk.Done {
			break
		}
	}
	got := tr.snapshot()
	if len(got) != 1 {
		t.Fatalf("traces=%d want 1", len(got))
	}
	if got[0].TraceID != "run-s" || got[0].Caller != "cron" {
		t.Fatalf("meta=%+v", got[0])
	}
	if !strings.Contains(got[0].OutputJSON, "hello") {
		t.Fatalf("stream content=%s", got[0].OutputJSON)
	}
	if !strings.Contains(got[0].InputJSON, "hi") {
		t.Fatalf("input=%s", got[0].InputJSON)
	}
}

// 序列化辅助：空输入返回空串；工具调用展开到 output。
func TestTraceContentHelpers(t *testing.T) {
	if s := traceSpanInputJSON(nil); s != "" {
		t.Fatalf("nil input=%s", s)
	}
	req := &providers.Request{}
	if s := traceSpanInputJSON(req); s != "" {
		t.Fatalf("empty messages=%s", s)
	}
	out := traceSpanOutputJSON("text", []providers.ToolCall{{
		ID:       "c1",
		Function: providers.FunctionCall{Name: "sql", Arguments: `{"q":1}`},
	}}, nil)
	if !strings.Contains(out, `"tool_calls"`) || !strings.Contains(out, "sql") {
		t.Fatalf("out=%s", out)
	}
	if s := traceSpanToolsJSON(nil); s != "" {
		t.Fatalf("nil tools=%s", s)
	}
	// fillTraceMeta：无 ctx 元数据时 TraceID/Caller 留空（recorder 侧回退），name 回退 model
	tt := LLMTrace{Model: "m1"}
	fillTraceMeta(context.Background(), &tt)
	if tt.TraceID != "" || tt.Caller != "" || tt.Name != "m1" {
		t.Fatalf("meta=%+v", tt)
	}
}
