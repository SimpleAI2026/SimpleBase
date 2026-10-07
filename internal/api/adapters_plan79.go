// adapters_plan79.go 提供具体服务到 api.Dependencies 接口的适配器。
package api

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database/cache"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/usage"
	"github.com/voocel/litellm/providers"
)

// cacheServiceAdapter 适配 *cache.Manager。
type cacheServiceAdapter struct{ m *cache.Manager }

func (a *cacheServiceAdapter) Usage(ctx context.Context) (CacheUsage, error) {
	u, err := a.m.Usage(ctx)
	if err != nil {
		return CacheUsage{}, err
	}
	return CacheUsage{TotalBytes: u.TotalBytes, DatabaseDirs: u.DatabaseDirs}, nil
}

// NewCacheService 构造 CacheService 适配器。
func NewCacheService(m *cache.Manager) CacheService {
	if m == nil {
		return nil
	}
	return &cacheServiceAdapter{m: m}
}

// usageServiceAdapter 适配 *usage.Service。
type usageServiceAdapter struct{ s *usage.Service }

func (a *usageServiceAdapter) CheckQuota(ctx context.Context, projectID string, kind string) error {
	return a.s.CheckQuota(ctx, projectID, kind)
}

// NewUsageService 构造 UsageService 适配器。
func NewUsageService(s *usage.Service) UsageService {
	if s == nil {
		return nil
	}
	return &usageServiceAdapter{s: s}
}

// auditServiceAdapter 适配 *audit.Service。
type auditServiceAdapter struct{ s *audit.Service }

func (a *auditServiceAdapter) Record(ctx context.Context, e AuditEvent) error {
	return a.s.Record(ctx, audit.Event{
		DatabaseID:  e.DatabaseID,
		ProjectID:   e.ProjectID,
		PrincipalID: e.PrincipalID,
		Kind:        e.Kind,
		RequestID:   e.RequestID,
		Status:      e.Status,
		Detail:      e.Detail,
	})
}

func (a *auditServiceAdapter) ListOperations(ctx context.Context, projectID, databaseID string, limit int) ([]catalog.Operation, error) {
	return a.s.ListOperations(ctx, audit.Query{ProjectID: projectID, DatabaseID: databaseID, Limit: limit})
}

// NewAuditService 构造 AuditService 适配器。
func NewAuditService(s *audit.Service) AuditService {
	if s == nil {
		return nil
	}
	return &auditServiceAdapter{s: s}
}

// llmServiceAdapter 适配 llmgateway.Service。
type llmServiceAdapter struct{ s llmgateway.Service }

func (a *llmServiceAdapter) Chat(ctx context.Context, projectID string, req LLMRequest) (LLMResponse, error) {
	resp, err := a.s.Chat(ctx, projectID, toGatewayRequest(req))
	if err != nil {
		return LLMResponse{}, err
	}
	out := LLMResponse{
		Content: resp.Content,
		Usage: LLMTokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			ReasoningTokens:  resp.Usage.ReasoningTokens,
		},
		Model:        resp.Model,
		Provider:     resp.Provider,
		FinishReason: resp.FinishReason,
	}
	for _, tc := range resp.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, LLMToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return out, nil
}

func (a *llmServiceAdapter) Stream(ctx context.Context, projectID string, req LLMRequest) (LLMStreamReader, error) {
	reader, err := a.s.Stream(ctx, projectID, toGatewayRequest(req))
	if err != nil {
		return nil, err
	}
	return &llmStreamReaderAdapter{inner: reader}, nil
}

// toGatewayRequest 把 api 层请求映射为 litellm 形状；工具与工具往返消息原样透传。
func toGatewayRequest(req LLMRequest) llmgateway.Request {
	msgs := make([]providers.Message, len(req.Messages))
	for i, m := range req.Messages {
		pm := providers.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			pm.ToolCalls = append(pm.ToolCalls, providers.ToolCall{
				ID: tc.ID, Type: "function",
				Function: providers.FunctionCall{Name: tc.Name, Arguments: tc.Arguments},
			})
		}
		msgs[i] = pm
	}
	out := llmgateway.Request{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	for _, t := range req.Tools {
		var params any = map[string]any{"type": "object", "properties": map[string]any{}}
		if len(t.Parameters) > 0 {
			params = t.Parameters
		}
		out.Tools = append(out.Tools, providers.Tool{
			Type:     "function",
			Function: providers.FunctionDef{Name: t.Name, Description: t.Description, Parameters: params},
		})
	}
	return out
}

func (a *llmServiceAdapter) ListProviders(ctx context.Context, projectID string) ([]string, error) {
	return a.s.ListProviders(ctx, projectID)
}

// NewLLMService 构造 LLMService 适配器。
func NewLLMService(s llmgateway.Service) LLMService {
	if s == nil {
		return nil
	}
	return &llmServiceAdapter{s: s}
}

// llmStreamReaderAdapter 适配 llmgateway.StreamReader。
type llmStreamReaderAdapter struct{ inner llmgateway.StreamReader }

func (a *llmStreamReaderAdapter) Next() (*LLMStreamChunk, error) {
	chunk, err := a.inner.Next()
	if err != nil {
		return nil, err
	}
	out := &LLMStreamChunk{
		Type:         chunk.Type,
		Content:      chunk.Content,
		FinishReason: chunk.FinishReason,
		Done:         chunk.Done,
	}
	if d := chunk.ToolCallDelta; d != nil {
		out.ToolCallIndex = d.Index
		out.ToolCallID = d.ID
		out.ToolCallName = d.FunctionName
		out.ToolCallArgs = d.ArgumentsDelta
	}
	if u := chunk.Usage; u != nil {
		out.Usage = &LLMTokenUsage{
			PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens,
			TotalTokens: u.TotalTokens, ReasoningTokens: u.ReasoningTokens,
		}
	}
	return out, nil
}

func (a *llmStreamReaderAdapter) Close() error { return a.inner.Close() }
