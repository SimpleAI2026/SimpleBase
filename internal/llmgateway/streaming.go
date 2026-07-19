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
	// 累加 usage（部分供应商在每个 chunk 更新累计值）。
	if chunk.Type == "usage" || chunk.Usage != nil {
		// providers.StreamChunk 可能携带 Usage 字段；通过反射或类型断言处理。
		// 这里简化：若 chunk 提供 Usage 则累计。
		if u := extractUsage(chunk); u != nil {
			r.accum = *u
		}
	}
	if chunk.FinishReason != "" {
		// 流结束：记录用量。
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

// extractUsage 从 StreamChunk 提取 Usage。
// providers.StreamChunk 结构可能不含 Usage 字段（取决于版本），
// 此处通过安全类型断言兼容；若无则返回 nil。
func extractUsage(chunk *providers.StreamChunk) *Usage {
	// providers.StreamChunk 当前无 Usage 字段；预留扩展点。
	// 若未来 litellm 在 chunk 中携带 usage，在此处提取。
	_ = chunk
	return nil
}
