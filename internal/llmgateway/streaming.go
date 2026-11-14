// 文件说明：streaming.go — 流式响应的用量记录 + trace 包装器（plan8.md；trace 见 planv6.5）
//
// 用法
//
//	type usageRecordingReader（包内使用）
//	同包调用：usageRecordingReader → service.go；Next() → service.go；Close() → service.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/llmgateway/streaming.go
//	         │ 依赖
//	         ▼
//	[下游] internal/observability
//	[同包] catalog_resolver.go, fallback_resolver.go, project_cred_resolver.go, provider_driver.go, service.go
//	[关键引用] usageRecordingReader ← service.go

package llmgateway

import (
	"context"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/voocel/litellm/providers"
	"go.uber.org/zap"
)

// streamToolCall 聚合中的工具调用分片（按 chunk.ToolCallIndex 归并）。
type streamToolCall struct {
	ID   string
	Name string
	Args strings.Builder
}

// usageRecordingReader 包装 StreamReader，在流结束时记录用量与 trace。
type usageRecordingReader struct {
	inner    StreamReader
	recorder UsageRecorder
	tracer   TraceRecorder
	traceCtx context.Context // 透传原始请求 ctx（含 request_id / trace meta），落 trace 用
	project  string
	provider string
	model    string
	logger   observability.Logger
	start    time.Time

	// 二期 §10.1：输入消息快照与工具声明在构造时固化（请求体不再变化）。
	inputJSON string
	toolsJSON string

	recorded bool
	accum    Usage // 累计用量

	content   strings.Builder         // 流式正文聚合
	toolCalls map[int]*streamToolCall // index → 聚合中的工具调用
}

func (r *usageRecordingReader) Next() (*providers.StreamChunk, error) {
	chunk, err := r.inner.Next()
	if err != nil {
		// 流错误或 EOF：尝试兜底记录已观察到的用量与 trace。
		r.maybeRecord()
		return nil, err
	}
	// 部分供应商仅在最后一个 chunk 返回完整用量，其他供应商逐帧更新累计值。
	if u := extractUsage(chunk); u != nil {
		r.accum = *u
	}
	r.collectContent(chunk)
	if chunk.Done {
		// 真正的流结束（可能在 finish_reason 后还有 usage 帧）。
		r.maybeRecord()
	}
	return chunk, nil
}

func (r *usageRecordingReader) Close() error {
	r.maybeRecord()
	return r.inner.Close()
}

// collectContent 聚合流式输出（二期 §10.1）：content 文本与 tool_call_delta 分片。
func (r *usageRecordingReader) collectContent(chunk *providers.StreamChunk) {
	if chunk == nil {
		return
	}
	if chunk.Content != "" && chunk.ToolCallDelta == nil {
		r.content.WriteString(chunk.Content)
	}
	if d := chunk.ToolCallDelta; d != nil {
		if r.toolCalls == nil {
			r.toolCalls = make(map[int]*streamToolCall)
		}
		tc := r.toolCalls[d.Index]
		if tc == nil {
			tc = &streamToolCall{ID: d.ID, Name: d.FunctionName}
			r.toolCalls[d.Index] = tc
		}
		if d.ID != "" {
			tc.ID = d.ID
		}
		if d.FunctionName != "" {
			tc.Name = d.FunctionName
		}
		tc.Args.WriteString(d.ArgumentsDelta)
	}
}

// collectedToolCalls 按序返回聚合后的工具调用。
func (r *usageRecordingReader) collectedToolCalls() []providers.ToolCall {
	if len(r.toolCalls) == 0 {
		return nil
	}
	// map 无序：按 index 排序输出。
	maxIdx := -1
	for i := range r.toolCalls {
		if i > maxIdx {
			maxIdx = i
		}
	}
	out := make([]providers.ToolCall, 0, len(r.toolCalls))
	for i := 0; i <= maxIdx; i++ {
		tc := r.toolCalls[i]
		if tc == nil {
			continue
		}
		out = append(out, providers.ToolCall{
			ID:       tc.ID,
			Function: providers.FunctionCall{Name: tc.Name, Arguments: tc.Args.String()},
		})
	}
	return out
}

func (r *usageRecordingReader) maybeRecord() {
	if r.recorded {
		return
	}
	r.recorded = true
	if r.recorder != nil && r.accum.TotalTokens > 0 {
		if err := r.recorder.RecordLLM(context.Background(), r.project, r.provider, r.model, r.accum); err != nil {
			if r.logger != nil {
				r.logger.Warn("llmgateway: record stream usage failed",
					zap.String("project", r.project),
					zap.String("err", err.Error()))
			}
		}
	}
	// trace 无论有无 usage 都记：流式调用的存在性本身就是要观测的对象。
	if r.tracer != nil {
		t := LLMTrace{
			ProjectID:    r.project,
			Provider:     r.provider,
			Model:        r.model,
			Status:       "ok",
			Stream:       true,
			DurationMs:   time.Since(r.start).Milliseconds(),
			InputTokens:  r.accum.PromptTokens,
			OutputTokens: r.accum.CompletionTokens,
			InputJSON:    r.inputJSON,
			ToolsJSON:    r.toolsJSON,
			OutputJSON:   traceSpanOutputJSON(r.content.String(), r.collectedToolCalls(), nil),
		}
		fillTraceMeta(r.traceCtx, &t)
		err := r.tracer.RecordTrace(r.traceCtx, t)
		if err != nil && r.logger != nil {
			r.logger.Warn("llmgateway: record stream trace failed",
				zap.String("project", r.project),
				zap.String("err", err.Error()))
		}
	}
}

// extractUsage 从 StreamChunk 提取 Usage（litellm 在末尾 usage 帧携带累计值）。
func extractUsage(chunk *providers.StreamChunk) *Usage {
	if chunk == nil || chunk.Usage == nil {
		return nil
	}
	return &Usage{
		PromptTokens:     chunk.Usage.PromptTokens,
		CompletionTokens: chunk.Usage.CompletionTokens,
		TotalTokens:      chunk.Usage.TotalTokens,
		ReasoningTokens:  chunk.Usage.ReasoningTokens,
	}
}
