package systemdb

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/utils/limit"
	_ "github.com/uglyer/go-sqlite3"
)

func newTraceTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return &Store{db: db}
}

// 插入 → 刷写 → 查询往返；默认值补全（ID/时间/source/status）。
func TestRecordAndQueryLLMTraces(t *testing.T) {
	store := newTraceTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)

	store.RecordLLMTrace(LLMTrace{
		ProjectID: "p1", RequestID: "rid-1", Provider: "openai", Model: "gpt-4o-mini",
		Status: "ok", Stream: true, DurationMs: 1234, InputTokens: 100, OutputTokens: 50,
		CreatedAt: base,
	})
	store.RecordLLMTrace(LLMTrace{
		ProjectID: "p1", RequestID: "rid-2", Provider: "openai", Model: "gpt-4o",
		Status: "error", ErrorCode: "upstream_error", ErrorMessage: "boom",
		CreatedAt: base.Add(time.Minute),
	})

	if err := store.FlushTraces(ctx); err != nil {
		t.Fatal(err)
	}
	// 缓冲已排空
	if got := len(store.drainTraces()); got != 0 {
		t.Fatalf("trace buffer=%d want 0", got)
	}

	got, err := store.QueryLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1", Limit: 10, From: base.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("traces=%d want 2", len(got))
	}
	// 按时间倒序：error 行在后插入，应排前
	if got[0].RequestID != "rid-2" || got[0].Status != "error" {
		t.Fatalf("first=%+v", got[0])
	}
	if !got[1].Stream || got[1].Source != "llm" || got[1].ID == "" {
		t.Fatalf("second=%+v", got[1])
	}

	total, err := store.CountLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1"})
	if err != nil || total != 2 {
		t.Fatalf("total=%d err=%v want 2", total, err)
	}
}

// 过滤条件：status / model 模糊 / request_id 精确 / 时间范围；offset 分页。
func TestQueryLLMTracesFilters(t *testing.T) {
	store := newTraceTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)

	rows := []LLMTrace{
		{ProjectID: "p1", RequestID: "r1", Model: "gpt-4o", Status: "ok", CreatedAt: base},
		{ProjectID: "p1", RequestID: "r2", Model: "gpt-4o-mini", Status: "error", CreatedAt: base.Add(2 * time.Minute)},
		{ProjectID: "p2", RequestID: "r3", Model: "gpt-4o", Status: "ok", CreatedAt: base.Add(4 * time.Minute)},
	}
	for _, r := range rows {
		store.RecordLLMTrace(r)
	}
	if err := store.FlushTraces(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := store.QueryLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1", Status: "error"})
	if err != nil || len(got) != 1 || got[0].RequestID != "r2" {
		t.Fatalf("status filter=%v err=%v", got, err)
	}
	got, err = store.QueryLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1", Model: "mini"})
	if err != nil || len(got) != 1 || got[0].RequestID != "r2" {
		t.Fatalf("model filter=%v err=%v", got, err)
	}
	got, err = store.QueryLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1", RequestID: "r1"})
	if err != nil || len(got) != 1 || got[0].RequestID != "r1" {
		t.Fatalf("request filter=%v err=%v", got, err)
	}
	// 时间范围
	got, err = store.QueryLLMTraces(ctx, LLMTraceQuery{
		ProjectID: "p1", From: base.Add(90 * time.Second), To: base.Add(3 * time.Minute),
	})
	if err != nil || len(got) != 1 || got[0].RequestID != "r2" {
		t.Fatalf("range filter=%v err=%v", got, err)
	}
	// From 为空时默认 24h 窗口，一小时前的行也能查到
	got, err = store.QueryLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1"})
	if err != nil || len(got) != 2 {
		t.Fatalf("default window=%d err=%v", len(got), err)
	}
	// offset 分页
	got, err = store.QueryLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1", Limit: 1, Offset: 1})
	if err != nil || len(got) != 1 {
		t.Fatalf("offset page=%v err=%v", got, err)
	}
}

// error_message 超 512 截断；limit 规范化（>200 收敛）。
func TestTraceNormalize(t *testing.T) {
	store := newTraceTestStore(t)
	ctx := context.Background()

	long := strings.Repeat("x", 800)
	store.RecordLLMTrace(LLMTrace{ProjectID: "p1", RequestID: "r1", Status: "error", ErrorMessage: long})
	if err := store.FlushTraces(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.QueryLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1"})
	if err != nil || len(got) != 1 {
		t.Fatalf("traces=%d err=%v", len(got), err)
	}
	if len(got[0].ErrorMessage) != 512 {
		t.Fatalf("message len=%d want 512", len(got[0].ErrorMessage))
	}

	// limit 异常值收敛（limit.Normalize：非正或超限回落默认值）
	if normalized := limit.Normalize(99999, 50, 200); normalized != 50 {
		t.Fatalf("limit normalized=%d want 50", normalized)
	}
	if normalized := limit.Normalize(0, 50, 200); normalized != 50 {
		t.Fatalf("limit normalized=%d want 50", normalized)
	}
}

// 合批入口：trace 与日志同事务落盘；空批 no-op。
func TestFlushAllAsyncWithTraces(t *testing.T) {
	store := newTraceTestStore(t)
	ctx := context.Background()

	store.RecordLLMTrace(LLMTrace{ProjectID: "p1", RequestID: "r1", Model: "gpt-4o", Status: "ok"})
	if err := store.flushAllAsync(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.QueryLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1"})
	if err != nil || len(got) != 1 {
		t.Fatalf("traces=%d err=%v", len(got), err)
	}
	// 空批 no-op
	if err := store.flushAllAsync(ctx); err != nil {
		t.Fatal(err)
	}
}

// db 为 nil：只入缓冲不 panic，FlushTraces no-op。
func TestTraceNilStore(t *testing.T) {
	store := &Store{}
	store.RecordLLMTrace(LLMTrace{ProjectID: "p1", RequestID: "r1"})
	if err := store.FlushTraces(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(store.drainTraces()); got != 1 {
		t.Fatalf("buffer=%d want 1", got)
	}
}

// —— 二期（planv6.5 §9）：分组聚合 / span 详情 / 工具卡片写入 / 内容截断 ——

// seedAgentTrace 写一个「agent run」形状的 trace：2 条 llm span + 1 条 tool span。
func seedAgentTrace(t *testing.T, store *Store, projectID, traceID string, base time.Time) {
	t.Helper()
	spans := []LLMTrace{
		{ProjectID: projectID, RequestID: "rid-a", Provider: "openai", Model: "m1", Status: "ok",
			TraceID: traceID, Caller: "agent", SpanKind: "llm", Name: "m1",
			InputTokens: 100, OutputTokens: 20, DurationMs: 800,
			InputJSON: `[{"role":"system","content":"sys prompt"}]`, OutputJSON: `{"content":"call tool"}`,
			ToolsJSON: `[{"name":"sql_query","description":"query"}]`, CreatedAt: base},
		{ProjectID: projectID, TraceID: traceID, Caller: "agent", SpanKind: "tool", Name: "sql_query",
			Status: "ok", DurationMs: 300, InputJSON: `{"q":"select 1"}`, OutputJSON: "rows",
			CreatedAt: base.Add(time.Second)},
		{ProjectID: projectID, RequestID: "rid-a", Provider: "openai", Model: "m1", Status: "ok",
			TraceID: traceID, Caller: "agent", SpanKind: "llm", Name: "m1",
			InputTokens: 200, OutputTokens: 40, DurationMs: 900, Stream: true,
			OutputJSON: `{"content":"final"}`, CreatedAt: base.Add(2 * time.Second)},
	}
	for _, sp := range spans {
		store.RecordLLMTrace(sp)
	}
	if err := store.FlushTraces(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// 分组聚合：一次 agent run 聚成一行；tokens 合计、span 计数、状态、流式、耗时。
func TestQueryLLMTraceGroups(t *testing.T) {
	store := newTraceTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)

	seedAgentTrace(t, store, "p1", "run-1", base)
	// API 直调：单 span，caller=api
	store.RecordLLMTrace(LLMTrace{ProjectID: "p1", RequestID: "req-2", Model: "m2", Status: "ok",
		TraceID: "req-2", Caller: "api", SpanKind: "llm", Name: "m2", CreatedAt: base.Add(3 * time.Minute)})
	// 错误调用：独立 trace
	store.RecordLLMTrace(LLMTrace{ProjectID: "p1", RequestID: "req-3", Model: "m2", Status: "error",
		TraceID: "req-3", Caller: "api", SpanKind: "llm", ErrorCode: "upstream", CreatedAt: base.Add(4 * time.Minute)})
	// 其他项目
	store.RecordLLMTrace(LLMTrace{ProjectID: "pX", RequestID: "r", TraceID: "r", Caller: "api", CreatedAt: base})
	if err := store.FlushTraces(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := store.QueryLLMTraceGroups(ctx, LLMTraceGroupQuery{ProjectID: "p1", From: base.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("groups=%d want 3: %+v", len(got), got)
	}
	// 倒序：req-3（最新）→ req-2 → run-1
	if got[0].TraceID != "req-3" || got[0].Status != "error" {
		t.Fatalf("first=%+v", got[0])
	}
	if got[1].TraceID != "req-2" || got[1].Caller != "api" || got[1].LLMCount != 1 || got[1].ToolCount != 0 {
		t.Fatalf("second=%+v", got[1])
	}
	g := got[2]
	if g.TraceID != "run-1" || g.Caller != "agent" || g.Status != "ok" {
		t.Fatalf("agent group=%+v", g)
	}
	if g.SpanCount != 3 || g.LLMCount != 2 || g.ToolCount != 1 {
		t.Fatalf("counts=%+v", g)
	}
	if g.InputTokens != 300 || g.OutputTokens != 60 {
		t.Fatalf("tokens=%+v", g)
	}
	if !g.Stream {
		t.Fatalf("stream should aggregate true: %+v", g)
	}
	if g.DurationMs != 2000 {
		t.Fatalf("duration=%d want 2000", g.DurationMs)
	}
	if g.Model != "m1" || g.Provider != "openai" {
		t.Fatalf("model/provider=%+v", g)
	}

	// 过滤：caller=agent 只剩 run-1
	got, err = store.QueryLLMTraceGroups(ctx, LLMTraceGroupQuery{ProjectID: "p1", Caller: "agent", From: base.Add(-time.Minute)})
	if err != nil || len(got) != 1 || got[0].TraceID != "run-1" {
		t.Fatalf("caller filter=%v err=%v", got, err)
	}
	// 过滤：status=error 只剩 req-3
	got, err = store.QueryLLMTraceGroups(ctx, LLMTraceGroupQuery{ProjectID: "p1", Status: "error", From: base.Add(-time.Minute)})
	if err != nil || len(got) != 1 || got[0].TraceID != "req-3" {
		t.Fatalf("status filter=%v err=%v", got, err)
	}
	// 过滤：model 关键词命中 llm span name
	got, err = store.QueryLLMTraceGroups(ctx, LLMTraceGroupQuery{ProjectID: "p1", Model: "m2", From: base.Add(-time.Minute)})
	if err != nil || len(got) != 2 {
		t.Fatalf("model filter=%d err=%v", len(got), err)
	}

	total, err := store.CountLLMTraceGroups(ctx, LLMTraceGroupQuery{ProjectID: "p1", From: base.Add(-time.Minute)})
	if err != nil || total != 3 {
		t.Fatalf("total=%d err=%v want 3", total, err)
	}

	// 分页
	got, err = store.QueryLLMTraceGroups(ctx, LLMTraceGroupQuery{ProjectID: "p1", Limit: 2, Offset: 2, From: base.Add(-time.Minute)})
	if err != nil || len(got) != 1 || got[0].TraceID != "run-1" {
		t.Fatalf("page=%v err=%v", got, err)
	}
}

// span 详情：按时间升序返回该 trace 全部 span，内容字段完整往返。
func TestQueryTraceSpans(t *testing.T) {
	store := newTraceTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)

	seedAgentTrace(t, store, "p1", "run-9", base)

	spans, err := store.QueryTraceSpans(ctx, "p1", "run-9")
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 3 {
		t.Fatalf("spans=%d want 3", len(spans))
	}
	if spans[0].SpanKind != "llm" || spans[0].Name != "m1" {
		t.Fatalf("first=%+v", spans[0])
	}
	if spans[1].SpanKind != "tool" || spans[1].Name != "sql_query" || spans[1].InputJSON != `{"q":"select 1"}` {
		t.Fatalf("tool span=%+v", spans[1])
	}
	if spans[2].SpanKind != "llm" || !spans[2].Stream {
		t.Fatalf("third=%+v", spans[2])
	}
	if spans[0].ToolsJSON == "" || spans[0].InputJSON == "" {
		t.Fatalf("content lost: %+v", spans[0])
	}

	// 越权 / 不存在 / 空 traceID 都返回空
	spans, err = store.QueryTraceSpans(ctx, "pX", "run-9")
	if err != nil || len(spans) != 0 {
		t.Fatalf("cross project=%v err=%v", spans, err)
	}
	spans, err = store.QueryTraceSpans(ctx, "p1", "nope")
	if err != nil || len(spans) != 0 {
		t.Fatalf("missing=%v err=%v", spans, err)
	}
	if _, err = store.QueryTraceSpans(ctx, "p1", ""); err != nil {
		t.Fatalf("empty traceID err=%v", err)
	}
}

// 工具卡片 JSON → tool span 入队；空/非法输入容错。
func TestRecordAgentToolSpans(t *testing.T) {
	store := newTraceTestStore(t)
	ctx := context.Background()

	cards := `[{"call_id":"c1","name":"sql_query","arguments":"{\"q\":\"select 1\"}","content":"ok rows","duration_ms":120,"truncated":false,"is_error":false},
	           {"call_id":"c2","name":"exec_cmd","arguments":"{}","content":"boom","duration_ms":50,"truncated":true,"is_error":true}]`
	if err := store.RecordAgentToolSpans("p1", "run-1", cards); err != nil {
		t.Fatal(err)
	}
	if err := store.FlushTraces(ctx); err != nil {
		t.Fatal(err)
	}
	spans, err := store.QueryTraceSpans(ctx, "p1", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 {
		t.Fatalf("spans=%d want 2", len(spans))
	}
	// 同一 now 写入的两条 span 排序不稳定，按 Name 定位断言
	byName := map[string]LLMTrace{}
	for _, sp := range spans {
		byName[sp.Name] = sp
	}
	first := byName["sql_query"]
	if first.SpanKind != "tool" || first.Status != "ok" || first.DurationMs != 120 {
		t.Fatalf("first=%+v", first)
	}
	if first.Caller != "agent" || first.TraceID != "run-1" {
		t.Fatalf("meta=%+v", first)
	}
	second := byName["exec_cmd"]
	if second.Status != "error" || second.ErrorCode != "tool_error" || second.ErrorMessage != "boom" {
		t.Fatalf("error span=%+v", second)
	}

	// 空 / 空数组 / 非法 JSON
	if err := store.RecordAgentToolSpans("p1", "run-2", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAgentToolSpans("p1", "run-2", "[]"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAgentToolSpans("p1", "", cards); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAgentToolSpans("p1", "run-3", "{bad"); err == nil {
		t.Fatal("bad json should error")
	}
}

// 内容超限截断：input/output/tools JSON 超上限时替换为 truncated 标记。
func TestTraceContentTruncation(t *testing.T) {
	store := newTraceTestStore(t)
	ctx := context.Background()

	huge := `{"content":"` + strings.Repeat("x", 100*1024) + `"}`
	store.RecordLLMTrace(LLMTrace{ProjectID: "p1", RequestID: "r1", Model: "m", Status: "ok",
		InputJSON: huge, OutputJSON: huge, ToolsJSON: huge})
	if err := store.FlushTraces(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.QueryLLMTraces(ctx, LLMTraceQuery{ProjectID: "p1"})
	if err != nil || len(got) != 1 {
		t.Fatalf("traces=%d err=%v", len(got), err)
	}
	want := `{"truncated":true}`
	if got[0].InputJSON != want || got[0].OutputJSON != want || got[0].ToolsJSON != want {
		t.Fatalf("truncation lost: in=%d out=%d tools=%d", len(got[0].InputJSON), len(got[0].OutputJSON), len(got[0].ToolsJSON))
	}
}

// 二期扩列兜底：一期形状的旧表（缺新列）经 ensureTraceColumns 补列后可写可查。
func TestEnsureTraceColumnsUpgrade(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// 模拟一期旧表：无二期 7 列
	if _, err := db.Exec(`CREATE TABLE sys_llm_traces (
		id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, request_id VARCHAR NOT NULL,
		source VARCHAR NOT NULL, provider VARCHAR NOT NULL, model VARCHAR NOT NULL,
		status VARCHAR NOT NULL, stream BIGINT NOT NULL, duration_ms BIGINT DEFAULT 0,
		input_tokens BIGINT DEFAULT 0, output_tokens BIGINT DEFAULT 0,
		error_code VARCHAR DEFAULT '', error_message VARCHAR DEFAULT '', created_at TIMESTAMP NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	// 预置一行一期数据
	if _, err := db.Exec(`INSERT INTO sys_llm_traces(id, project_id, request_id, source, provider, model, status, stream, created_at)
		VALUES('old1','p1','r1','llm','openai','gpt','ok',0,'2026-11-12T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := &Store{db: db}
	// 旧数据可查（caller 回退 source 空串、span_kind 默认 llm）
	got, err := store.QueryLLMTraces(context.Background(), LLMTraceQuery{ProjectID: "p1", From: time.Now().UTC().Add(-48 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SpanKind != "llm" || got[0].Name != "gpt" {
		t.Fatalf("legacy row=%+v", got)
	}
	// 新数据可写
	store.RecordLLMTrace(LLMTrace{ProjectID: "p1", RequestID: "r2", TraceID: "r2", Caller: "api",
		SpanKind: "llm", Model: "m2", Name: "m2", InputJSON: `[{"role":"user","content":"hi"}]`})
	if err := store.FlushTraces(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = store.QueryLLMTraces(context.Background(), LLMTraceQuery{ProjectID: "p1", From: time.Now().UTC().Add(-48 * time.Hour)})
	if err != nil || len(got) != 2 {
		t.Fatalf("after upgrade=%d err=%v", len(got), err)
	}
	// 幂等：重复执行不报错
	if err := EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}
