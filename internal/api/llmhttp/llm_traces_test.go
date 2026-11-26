// 文件说明：llm_traces_test.go — internal/api 的单元测试（TestLLMTracesHandler_MissingProject、TestLLMTracesHandler_NilStore 等）
//
// 用法
//
//	go test ./internal/api -run 'TestLLMTracesHandler_MissingProject|TestLLMTracesHandler_NilStore|TestLLMTracesHandler_List|TestLLMTraceGroupsHandler'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/llm_traces_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	setupTracesRouter(t *testing.T, store *systemdb.Store, withProject bool) *echo.Echo
//	TestLLMTracesHandler_MissingProject(t *testing.T)
//	TestLLMTracesHandler_NilStore(t *testing.T)
//	TestLLMTracesHandler_List(t *testing.T)
//	setupTraceGroupsRouter(t *testing.T, store *systemdb.Store) *echo.Echo
//	seedGroupedTraces(t *testing.T, store *systemdb.Store) time.Time
//	TestLLMTraceGroupsHandler(t *testing.T)
//	TestLLMTraceDetailHandler(t *testing.T)

package llmhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

func setupTracesRouter(t *testing.T, store *systemdb.Store, withProject bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = httpkit.ErrorHandler(nil)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			if withProject {
				ctx = httpkit.WithProject(ctx, httpkit.ProjectContext{ID: "proj-1", TenantID: "t"})
			}
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	th := &TracesHandler{store: store}
	e.GET("/llm/traces", th.List)
	return e
}

// 无项目上下文 → 500。
func TestLLMTracesHandler_MissingProject(t *testing.T) {
	store := openSystemAPI(t)
	e := setupTracesRouter(t, store, false)
	rec := doRequest(e, http.MethodGet, "/llm/traces", nil)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected error, got %d %s", rec.Code, rec.Body.String())
	}
}

// store 为 nil → 503。
func TestLLMTracesHandler_NilStore(t *testing.T) {
	e := setupTracesRouter(t, nil, true)
	rec := doRequest(e, http.MethodGet, "/llm/traces", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
}

// 正常查询：写入 → 刷盘 → 查询过滤（status / 分页 total）。
func TestLLMTracesHandler_List(t *testing.T) {
	store := openSystemAPI(t)
	ctx := context.Background()
	now := time.Now().UTC()
	rows := []systemdb.LLMTrace{
		{ProjectID: "proj-1", RequestID: "r1", Provider: "openai", Model: "gpt-4o", Status: "ok",
			InputTokens: 10, OutputTokens: 5, DurationMs: 100, CreatedAt: now.Add(-2 * time.Hour)},
		{ProjectID: "proj-1", RequestID: "r2", Provider: "openai", Model: "gpt-4o", Status: "error",
			ErrorCode: "upstream_error", ErrorMessage: "boom", DurationMs: 50, CreatedAt: now.Add(-time.Hour)},
	}
	for _, r := range rows {
		store.RecordLLMTrace(r)
	}
	if err := store.FlushTraces(ctx); err != nil {
		t.Fatal(err)
	}

	e := setupTracesRouter(t, store, true)
	rec := doRequest(e, http.MethodGet, "/llm/traces?from="+now.Add(-3*time.Hour).Format(time.RFC3339), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Traces []systemdb.LLMTrace `json:"traces"`
		Total  int64               `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Traces) != 2 || body.Total != 2 {
		t.Fatalf("traces=%d total=%d", len(body.Traces), body.Total)
	}

	// status 过滤
	rec = doRequest(e, http.MethodGet, "/llm/traces?status=error&from="+now.Add(-3*time.Hour).Format(time.RFC3339), nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Traces) != 1 || body.Traces[0].RequestID != "r2" {
		t.Fatalf("error filter=%+v", body.Traces)
	}
	// 非法 status 被忽略（不过滤）
	rec = doRequest(e, http.MethodGet, "/llm/traces?status=weird&from="+now.Add(-3*time.Hour).Format(time.RFC3339), nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Traces) != 2 {
		t.Fatalf("weird status should be ignored, got %d", len(body.Traces))
	}
}

// —— 二期（planv6.5 §9.3）：分组列表 + 详情 span API ——

func setupTraceGroupsRouter(t *testing.T, store *systemdb.Store) *echo.Echo {
	t.Helper()
	e := setupTracesRouter(t, store, true)
	th := &TracesHandler{store: store}
	e.GET("/llm/trace-groups", th.Groups)
	e.GET("/llm/trace-groups/:traceId", th.Detail)
	return e
}

// seedGroupedTraces 写入：agent run（2 llm + 1 tool）与两条 api 直调。
func seedGroupedTraces(t *testing.T, store *systemdb.Store) time.Time {
	t.Helper()
	base := time.Now().UTC().Add(-time.Hour)
	spans := []systemdb.LLMTrace{
		{ProjectID: "proj-1", RequestID: "rid-a", Provider: "openai", Model: "gpt-4o", Status: "ok",
			TraceID: "run-1", Caller: "agent", SpanKind: "llm", Name: "gpt-4o",
			InputTokens: 10, OutputTokens: 5, DurationMs: 800,
			InputJSON: `[{"role":"user","content":"hi"}]`, CreatedAt: base},
		{ProjectID: "proj-1", TraceID: "run-1", Caller: "agent", SpanKind: "tool", Name: "sql_query",
			Status: "ok", DurationMs: 100, OutputJSON: "rows", CreatedAt: base.Add(time.Second)},
		{ProjectID: "proj-1", RequestID: "rid-a", Provider: "openai", Model: "gpt-4o", Status: "ok",
			TraceID: "run-1", Caller: "agent", SpanKind: "llm", Name: "gpt-4o", Stream: true,
			InputTokens: 20, OutputTokens: 8, DurationMs: 900, CreatedAt: base.Add(2 * time.Second)},
		{ProjectID: "proj-1", RequestID: "rid-b", Provider: "openai", Model: "gpt-4o", Status: "error",
			TraceID: "rid-b", Caller: "api", SpanKind: "llm", Name: "gpt-4o",
			ErrorCode: "upstream_error", ErrorMessage: "boom", DurationMs: 40, CreatedAt: base.Add(3 * time.Minute)},
	}
	for _, sp := range spans {
		store.RecordLLMTrace(sp)
	}
	if err := store.FlushTraces(context.Background()); err != nil {
		t.Fatal(err)
	}
	return base
}

// 分组列表：聚合行 + caller/status 过滤 + 分页 total。
func TestLLMTraceGroupsHandler(t *testing.T) {
	store := openSystemAPI(t)
	base := seedGroupedTraces(t, store)
	e := setupTraceGroupsRouter(t, store)

	rec := doRequest(e, http.MethodGet, "/llm/trace-groups?from="+base.Add(-time.Minute).Format(time.RFC3339), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("groups %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Groups []systemdb.LLMTraceGroup `json:"groups"`
		Total  int64                    `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Groups) != 2 || body.Total != 2 {
		t.Fatalf("groups=%d total=%d", len(body.Groups), body.Total)
	}
	// 倒序：rid-b（error）在前
	if body.Groups[0].TraceID != "rid-b" || body.Groups[0].Status != "error" {
		t.Fatalf("first=%+v", body.Groups[0])
	}
	g := body.Groups[1]
	if g.TraceID != "run-1" || g.Caller != "agent" || g.SpanCount != 3 || g.ToolCount != 1 || g.LLMCount != 2 {
		t.Fatalf("agent group=%+v", g)
	}
	if g.InputTokens != 30 || !g.Stream {
		t.Fatalf("aggregate=%+v", g)
	}

	// caller 过滤
	rec = doRequest(e, http.MethodGet, "/llm/trace-groups?caller=agent&from="+base.Add(-time.Minute).Format(time.RFC3339), nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Groups) != 1 || body.Groups[0].TraceID != "run-1" {
		t.Fatalf("caller filter=%+v", body.Groups)
	}
	// 非法 caller 忽略
	rec = doRequest(e, http.MethodGet, "/llm/trace-groups?caller=weird&from="+base.Add(-time.Minute).Format(time.RFC3339), nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Groups) != 2 {
		t.Fatalf("weird caller ignored=%d", len(body.Groups))
	}
}

// 详情：返回该 trace 全部 span；跨项目/不存在为空数组；缺 traceId 400。
func TestLLMTraceDetailHandler(t *testing.T) {
	store := openSystemAPI(t)
	seedGroupedTraces(t, store)
	e := setupTraceGroupsRouter(t, store)

	rec := doRequest(e, http.MethodGet, "/llm/trace-groups/run-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Spans []systemdb.LLMTrace `json:"spans"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Spans) != 3 {
		t.Fatalf("spans=%d", len(body.Spans))
	}
	if body.Spans[1].SpanKind != "tool" || body.Spans[1].Name != "sql_query" {
		t.Fatalf("tool span=%+v", body.Spans[1])
	}

	rec = doRequest(e, http.MethodGet, "/llm/trace-groups/nope", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("missing trace should be 200 empty, got %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Spans) != 0 {
		t.Fatalf("missing spans=%d", len(body.Spans))
	}
}
