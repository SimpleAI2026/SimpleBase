// 文件说明：llm_trace_recorder_test.go — internal/app 的单元测试（TestLLMTraceRecorder、TestLLMTraceRecorderFallback 等）
//
// 用法
//
//	go test ./internal/app -run 'TestLLMTraceRecorder|TestLLMTraceRecorderFallback'
//
// 调用架构
//
//	[上游] cmd/simplebase, internal/api/perfbench
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/app/llm_trace_recorder_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] agent_shutdown_test.go, app.go, app_alerts.go, app_lease.go, app_test.go 等 41 个
//
// 主要符号
//
//	TestLLMTraceRecorder(t *testing.T)
//	TestLLMTraceRecorderFallback(t *testing.T)
//	gatewayTraceForTest() llmgateway.LLMTrace
//	openTraceStore(t *testing.T) *systemdb.Store

package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/api"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

// recorder 适配：ctx 带 request_id 落库；nil store 返回 nil。（自 internal/api/llm_traces_test.go 随实现迁入）
func TestLLMTraceRecorder(t *testing.T) {
	if newLLMTraceRecorder(nil) != nil {
		t.Fatal("nil store should return nil")
	}
	store := openTraceStore(t)
	rec := newLLMTraceRecorder(store)
	ctx := api.WithRequestID(context.Background(), "rid-trace-1")
	if err := rec.RecordTrace(ctx, gatewayTraceForTest()); err != nil {
		t.Fatal(err)
	}
	if err := store.FlushTraces(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := store.QueryLLMTraces(context.Background(), systemdb.LLMTraceQuery{ProjectID: "proj-1"})
	if err != nil || len(got) != 1 {
		t.Fatalf("traces=%d err=%v", len(got), err)
	}
	if got[0].RequestID != "rid-trace-1" || !got[0].Stream || got[0].Status != "ok" {
		t.Fatalf("row=%+v", got[0])
	}
	if got[0].ID == "" {
		t.Fatal("id should be generated")
	}
}

// recorder 回退：无 meta 时 trace_id=request_id、caller=api；有 meta 时透传。
func TestLLMTraceRecorderFallback(t *testing.T) {
	store := openTraceStore(t)

	// 无 meta：回退 request_id / api
	ctx := api.WithRequestID(context.Background(), "req-fb")
	tr := gatewayTraceForTest()
	tr.InputJSON = `[{"role":"user","content":"x"}]`
	if err := newLLMTraceRecorder(store).RecordTrace(ctx, tr); err != nil {
		t.Fatal(err)
	}
	// 有 meta：透传（caller=agent, trace_id=run-x）
	ctx2 := llmgateway.WithTraceMeta(api.WithRequestID(context.Background(), "req-2"), "run-x", "agent")
	tr2 := gatewayTraceForTest()
	tr2.TraceID = "run-x"
	tr2.Caller = "agent"
	if err := newLLMTraceRecorder(store).RecordTrace(ctx2, tr2); err != nil {
		t.Fatal(err)
	}
	if err := store.FlushTraces(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := store.QueryLLMTraces(context.Background(), systemdb.LLMTraceQuery{ProjectID: "proj-1"})
	if err != nil || len(got) != 2 {
		t.Fatalf("traces=%d err=%v", len(got), err)
	}
	byTrace := map[string]systemdb.LLMTrace{}
	for _, r := range got {
		byTrace[r.TraceID] = r
	}
	fb := byTrace["req-fb"]
	if fb.Caller != "api" || fb.SpanKind != "llm" || fb.InputJSON == "" {
		t.Fatalf("fallback row=%+v", fb)
	}
	agent := byTrace["run-x"]
	if agent.Caller != "agent" || agent.RequestID != "req-2" {
		t.Fatalf("agent row=%+v", agent)
	}
}

// gatewayTraceForTest 构造一条网关侧 trace（等价于 llmgateway.Chat 成功后的记录）。
func gatewayTraceForTest() llmgateway.LLMTrace {
	return llmgateway.LLMTrace{
		ProjectID: "proj-1", Provider: "openai", Model: "gpt-4o",
		Status: "ok", Stream: true, DurationMs: 1234,
		InputTokens: 10, OutputTokens: 5,
	}
}

// openTraceStore 打开一个内存系统库（测试用）。
func openTraceStore(t *testing.T) *systemdb.Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return systemdb.NewStoreForTest(db)
}
