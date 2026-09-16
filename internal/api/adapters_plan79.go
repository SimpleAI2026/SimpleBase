// adapters_plan79.go 提供具体服务到 api.Dependencies 接口的适配器。
package api

import (
	"context"

	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database/cache"
	"github.com/linkxzhou/SimpleBase/internal/jobs"
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
	msgs := make([]providers.Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = providers.Message{Role: m.Role, Content: m.Content}
	}
	resp, err := a.s.Chat(ctx, projectID, llmgateway.Request{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	if err != nil {
		return LLMResponse{}, err
	}
	return LLMResponse{
		Content: resp.Content,
		Usage: LLMTokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		Model:        resp.Model,
		Provider:     resp.Provider,
		FinishReason: resp.FinishReason,
	}, nil
}

func (a *llmServiceAdapter) Stream(ctx context.Context, projectID string, req LLMRequest) (LLMStreamReader, error) {
	msgs := make([]providers.Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = providers.Message{Role: m.Role, Content: m.Content}
	}
	reader, err := a.s.Stream(ctx, projectID, llmgateway.Request{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	if err != nil {
		return nil, err
	}
	return &llmStreamReaderAdapter{inner: reader}, nil
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
	return &LLMStreamChunk{
		Type:         chunk.Type,
		Content:      chunk.Content,
		FinishReason: chunk.FinishReason,
	}, nil
}

func (a *llmStreamReaderAdapter) Close() error { return a.inner.Close() }

// jobEnqueuerAdapter 适配 *jobs.Enqueuer。
type jobEnqueuerAdapter struct{ e *jobs.Enqueuer }

func (a *jobEnqueuerAdapter) Enqueue(ctx context.Context, in JobInput) (string, error) {
	return a.e.Enqueue(ctx, jobs.EnqueueInput{
		OperationID: in.OperationID,
		DatabaseID:  in.DatabaseID,
		ProjectID:   in.ProjectID,
		Type:        catalog.JobType(in.Type),
		PayloadJSON: in.PayloadJSON,
	})
}

// NewJobEnqueuer 构造 JobEnqueuer 适配器。
func NewJobEnqueuer(e *jobs.Enqueuer) JobEnqueuer {
	if e == nil {
		return nil
	}
	return &jobEnqueuerAdapter{e: e}
}
