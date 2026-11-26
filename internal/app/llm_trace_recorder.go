// 文件说明：llm_trace_recorder.go — llmgateway.TraceRecorder 的 systemdb 适配（app 私有）
//
// 用法
//
//	newLLMTraceRecorder(store *systemdb.Store) llmgateway.TraceRecorder   // 装配进 llmgateway.NewService
//	(r *llmTraceRecorder) RecordTrace(ctx context.Context, t llmgateway.LLMTrace) error
//
// 调用架构
//
//	[上游] internal/app（assembleLLMService，仅此一处使用）
//	         │ 同包调用
//	         ▼
//	[本文件] internal/app/llm_trace_recorder.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api（request_id 的 context key）, internal/llmgateway, internal/systemdb
//
// 实现属于组合根（internal-ddd-hexagonal-implementation-plan §4）：它不是 HTTP 适配，
// 只在装配处把网关 trace 落系统库；request_id 由入口内核的 context key 解出，
// llmgateway 不能反向依赖 api（会循环导入），所以放在唯一能同时看到两者的 app。

package app

import (
	"context"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"

	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// llmTraceRecorder 把网关侧的 LLMTrace 转成 systemdb 行。
type llmTraceRecorder struct{ store *systemdb.Store }

// newLLMTraceRecorder 构造适配器；store 为 nil 时返回 nil（网关侧静默跳过）。
func newLLMTraceRecorder(store *systemdb.Store) llmgateway.TraceRecorder {
	if store == nil {
		return nil
	}
	return &llmTraceRecorder{store: store}
}

// RecordTrace 实现 llmgateway.TraceRecorder：从 ctx 解 request_id 后入缓冲。
// 二期（§10.1）回退规则：trace_id 为空 → request_id；caller 为空 → "api"。
func (r *llmTraceRecorder) RecordTrace(ctx context.Context, t llmgateway.LLMTrace) error {
	if r == nil || r.store == nil {
		return nil
	}
	// Stream 场景 ctx 可能已随请求结束取消，request_id 此刻仍在 ctx 值里可读。
	requestID := httpkit.RequestIDFromContext(ctx)
	traceID := t.TraceID
	if traceID == "" {
		traceID = requestID
	}
	caller := t.Caller
	if caller == "" {
		caller = "api"
	}
	r.store.RecordLLMTrace(systemdb.LLMTrace{
		ProjectID:    t.ProjectID,
		RequestID:    requestID,
		Source:       "llm",
		Provider:     t.Provider,
		Model:        t.Model,
		Status:       t.Status,
		Stream:       t.Stream,
		DurationMs:   t.DurationMs,
		InputTokens:  int64(t.InputTokens),
		OutputTokens: int64(t.OutputTokens),
		ErrorCode:    t.ErrorCode,
		ErrorMessage: t.ErrorMessage,
		TraceID:      traceID,
		Caller:       caller,
		SpanKind:     "llm",
		Name:         t.Name,
		InputJSON:    t.InputJSON,
		OutputJSON:   t.OutputJSON,
		ToolsJSON:    t.ToolsJSON,
	})
	return nil
}
