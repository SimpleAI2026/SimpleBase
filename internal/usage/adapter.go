// adapter.go 提供 usage.Service 到 llmgateway.UsageRecorder 的适配。
package usage

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
)

// LLMRecorder 适配 usage.Service 为 llmgateway.UsageRecorder。
type LLMRecorder struct{ svc *Service }

// NewLLMRecorder 构造适配器。
func NewLLMRecorder(s *Service) llmgateway.UsageRecorder {
	return &LLMRecorder{svc: s}
}

// RecordLLM 实现 llmgateway.UsageRecorder。
func (r *LLMRecorder) RecordLLM(ctx context.Context, projectID, provider, model string, u llmgateway.Usage) error {
	return r.svc.RecordLLM(ctx, projectID, provider, model, LLMUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	})
}
