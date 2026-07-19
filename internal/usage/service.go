// Package usage 实现项目维度的用量记录与配额检查（plan9.md）。
//
// 数据库与 LLM 共用 project 维度的用量基础设施，但资源池、超时、失败策略完全隔离。
// 配额检查在请求入口（middleware）执行；用量记录在请求完成后异步追加。
package usage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"go.uber.org/zap"
)

// Service 提用量记录与配额检查。
type Service struct {
	repo   catalog.Repository
	logger observability.Logger
	now    func() time.Time

	// buffer 批量追加以减少写放大。
	mu      sync.Mutex
	buffer  []catalog.UsageEvent
	flushAt time.Time
}

// NewService 构造 Service。
func NewService(repo catalog.Repository, logger observability.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
		now:    time.Now,
	}
}

// RecordDatabase 记录数据库用量事件。
func (s *Service) RecordDatabase(ctx context.Context, projectID, requestID string, bytes int64) error {
	ev := catalog.UsageEvent{
		ID:         uuid.NewString(),
		ProjectID:  projectID,
		Kind:       "database",
		RequestID:  requestID,
		OccurredAt: s.now(),
	}
	return s.append(ctx, ev)
}

// RecordLLM 记录 LLM 用量。满足 llmgateway.UsageRecorder 接口。
func (s *Service) RecordLLM(ctx context.Context, projectID, provider, model string, u LLMUsage) error {
	ev := catalog.UsageEvent{
		ID:           uuid.NewString(),
		ProjectID:    projectID,
		Kind:         "llm",
		Provider:     provider,
		Model:        model,
		InputTokens:  int64(u.PromptTokens),
		OutputTokens: int64(u.CompletionTokens),
		OccurredAt:   s.now(),
	}
	return s.append(ctx, ev)
}

// LLMUsage 是 LLM token 用量（与 llmgateway.Usage 一致，避免循环依赖）。
type LLMUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// CheckQuota 检查 project 在当前周期内是否超出配额。
// 返回 nil 表示允许；ErrQuotaExceeded 表示拒绝。
func (s *Service) CheckQuota(ctx context.Context, projectID string, kind string) error {
	quota, err := s.repo.GetQuota(ctx, projectID)
	if err != nil {
		return fmt.Errorf("usage: get quota: %w", err)
	}
	period := time.Duration(quota.PeriodSeconds) * time.Second
	if period <= 0 {
		period = time.Hour
	}
	since := s.now().Add(-period)
	sum, err := s.repo.SumUsageSince(ctx, projectID, since)
	if err != nil {
		return fmt.Errorf("usage: sum usage: %w", err)
	}
	switch kind {
	case "llm":
		if quota.MaxLLMRequests > 0 && sum.LLMRequests >= quota.MaxLLMRequests {
			return fmt.Errorf("%w: llm requests %d >= %d", ErrQuotaExceeded, sum.LLMRequests, quota.MaxLLMRequests)
		}
		if quota.MaxLLMTokens > 0 && sum.LLMTokens >= quota.MaxLLMTokens {
			return fmt.Errorf("%w: llm tokens %d >= %d", ErrQuotaExceeded, sum.LLMTokens, quota.MaxLLMTokens)
		}
	case "database":
		if quota.MaxDatabases > 0 {
			// 数据库数量由 catalog 层在 CreateDatabase 时校验。
		}
		if quota.MaxStorageBytes > 0 && sum.DatabaseStorage >= quota.MaxStorageBytes {
			return fmt.Errorf("%w: storage %d >= %d", ErrQuotaExceeded, sum.DatabaseStorage, quota.MaxStorageBytes)
		}
	}
	return nil
}

// SetQuota 设置 project 配额。
func (s *Service) SetQuota(ctx context.Context, quota catalog.ProjectQuota) error {
	return s.repo.UpsertQuota(ctx, quota)
}

// GetQuota 返回 project 配额。
func (s *Service) GetQuota(ctx context.Context, projectID string) (catalog.ProjectQuota, error) {
	return s.repo.GetQuota(ctx, projectID)
}

// append 缓冲事件并批量刷新。
func (s *Service) append(ctx context.Context, ev catalog.UsageEvent) error {
	s.mu.Lock()
	s.buffer = append(s.buffer, ev)
	shouldFlush := len(s.buffer) >= 64 || s.flushAt.IsZero() || s.now().Sub(s.flushAt) >= 5*time.Second
	if s.flushAt.IsZero() {
		s.flushAt = s.now()
	}
	s.mu.Unlock()
	if !shouldFlush {
		return nil
	}
	return s.Flush(ctx)
}

// Flush 将缓冲区事件批量写入。
func (s *Service) Flush(ctx context.Context) error {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return nil
	}
	batch := s.buffer
	s.buffer = nil
	s.flushAt = time.Time{}
	s.mu.Unlock()
	if err := s.repo.AppendUsage(ctx, batch); err != nil {
		if s.logger != nil {
			s.logger.Warn("usage: flush failed",
				zap.Int("events", len(batch)),
				zap.String("err", err.Error()))
		}
		return err
	}
	return nil
}

// 错误定义。
var (
	ErrQuotaExceeded = errors.New("usage: quota exceeded")
)
