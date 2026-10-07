// streaming.go 实现流式响应的用量记录包装器（plan8.md）。
//
// litellm 的 StreamReader 在最后一个 chunk 携带累计 Usage；
// 本包装器在读取到 finish/usage chunk 时记录用量，在 Close 时兜底记录。
package llmgateway

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/voocel/litellm/providers"
	"go.uber.org/zap"
)

// usageRecordingReader 包装 StreamReader，在流结束时记录用量。
type usageRecordingReader struct {
	inner    StreamReader
	recorder UsageRecorder
	project  string
	provider string
	model    string
	logger   observability.Logger

	recorded bool
	accum    Usage // 累计用量
}

func (r *usageRecordingReader) Next() (*providers.StreamChunk, error) {
	chunk, err := r.inner.Next()
	if err != nil {
		// 流错误或 EOF：尝试兜底记录已观察到的用量。
		r.maybeRecord()
		return nil, err
	}
	// 部分供应商仅在最后一个 chunk 返回完整用量，其他供应商逐帧更新累计值。
	if u := extractUsage(chunk); u != nil {
		r.accum = *u
	}
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

func (r *usageRecordingReader) maybeRecord() {
	if r.recorded || r.recorder == nil {
		return
	}
	if r.accum.TotalTokens == 0 {
		// 无用量信息：不记录，避免污染数据。
		return
	}
	r.recorded = true
	if err := r.recorder.RecordLLM(context.Background(), r.project, r.provider, r.model, r.accum); err != nil {
		if r.logger != nil {
			r.logger.Warn("llmgateway: record stream usage failed",
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
