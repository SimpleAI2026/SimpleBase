// 文件说明：adapter.go — usage.Service 到 llmgateway.UsageRecorder 的适配
//
// 用法
//
//	NewLLMRecorder(s *Service) llmgateway.UsageRecorder
//	(r *LLMRecorder) RecordLLM(ctx context.Context, projectID, provider, model string ...  // RecordLLM 实现 llmgateway.UsageRecorder
//	同包调用：RecordLLM() → service.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/usage/adapter.go
//	         │ 依赖
//	         ▼
//	[下游] internal/llmgateway
//	[同包] service.go
//	[关键引用] RecordLLM() ← service.go
//
// 主要符号
//
//	type LLMRecorder struct {…}
//	NewLLMRecorder(s *Service) llmgateway.UsageRecorder
//	(r *LLMRecorder) RecordLLM(ctx context.Context, projectID, provider, model string ...
//	adapter.go 提供 usage.Service 到 llmgateway.UsageRecorder 的适配。

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
