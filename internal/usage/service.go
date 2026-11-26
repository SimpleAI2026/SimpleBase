// 文件说明：service.go — 项目维度的用量记录与配额检查（plan9.md）
//
// 用法
//
//	NewService(repo catalog.Repository, logger observability.Logger) *Service
//	(s *Service) RecordDatabase(ctx context.Context, projectID, requestID string, bytes ...  // RecordDatabase 记录数据库用量事件
//	(s *Service) RecordLLM(ctx context.Context, projectID, provider, model string, u ...  // RecordLLM 记录 LLM 用量
//	同包调用：Service → adapter.go；RecordLLM() → adapter.go；LLMUsage → adapter.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/usage/service.go
//	         │ 依赖
//	         ▼
//	[下游] internal/catalog, internal/observability
//	[同包] adapter.go
//	[关键引用] Service ← adapter.go
//
// 主要符号
//
//	type Service struct {…}
//	NewService(repo catalog.Repository, logger observability.Logger) *Service
//	(s *Service) RecordDatabase(ctx context.Context, projectID, requestID string, bytes ...
//	(s *Service) RecordLLM(ctx context.Context, projectID, provider, model string, u ...
//	(s *Service) RecordSandbox(ctx context.Context, projectID, requestID string ...
//	type LLMUsage struct {…}
//	... 另有 6 个顶层符号

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

	// 站点流量按 UTC 日聚合，内存累计后周期写 sys_site_traffic。
	siteMu       sync.Mutex
	siteDay      string
	siteCounters map[string]*siteCounter
	siteQuota    map[string]siteQuotaEntry
	sitePending  int64
	siteFlushAt  time.Time
}

type siteCounter struct {
	day      string
	requests int64
	bytes    int64
}

type siteQuotaEntry struct {
	quota   catalog.ProjectQuota
	expires time.Time
}

// NewService 构造 Service。
func NewService(repo catalog.Repository, logger observability.Logger) *Service {
	return &Service{
		repo:         repo,
		logger:       logger,
		now:          time.Now,
		siteCounters: map[string]*siteCounter{},
		siteQuota:    map[string]siteQuotaEntry{},
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

// RecordSandbox 记录一次沙盒 exec/run，不做配额拦截。
func (s *Service) RecordSandbox(ctx context.Context, projectID, requestID string, durationMs int64) error {
	return s.append(ctx, catalog.UsageEvent{
		ID: uuid.NewString(), ProjectID: projectID, Kind: "sandbox", RequestID: requestID,
		DurationMs: durationMs, OccurredAt: s.now(),
	})
}

// AllowSite 检查站点配额并记一次请求；配额读取失败按可用性优先放行。
func (s *Service) AllowSite(ctx context.Context, projectID string) error {
	day := s.now().UTC().Format("2006-01-02")
	counter, err := s.siteCounterFor(ctx, projectID, day)
	if err != nil {
		s.warnSite("site quota counter", err)
		return nil
	}
	quota, err := s.siteQuotaFor(ctx, projectID)
	if err != nil {
		s.warnSite("site quota", err)
		return nil
	}
	s.siteMu.Lock()
	exceeded := (quota.MaxSiteRequests > 0 && counter.requests >= quota.MaxSiteRequests) ||
		(quota.MaxSiteEgressBytes > 0 && counter.bytes >= quota.MaxSiteEgressBytes)
	if !exceeded {
		counter.requests++
		s.sitePending++
		if s.siteFlushAt.IsZero() {
			s.siteFlushAt = s.now()
		}
	}
	requests, bytes := counter.requests, counter.bytes
	s.siteMu.Unlock()
	if exceeded {
		return fmt.Errorf("%w: site requests=%d bytes=%d", ErrSiteQuotaExceeded, requests, bytes)
	}
	return s.maybeFlushSite(ctx)
}

// SetSiteQuota 更新项目的站点每日请求 / 出口配额，并让缓存立即失效。
func (s *Service) SetSiteQuota(ctx context.Context, projectID string, maxRequests, maxEgressBytes int64) error {
	quota, err := s.repo.GetQuota(ctx, projectID)
	if err != nil {
		return err
	}
	quota.ProjectID = projectID
	quota.MaxSiteRequests = maxRequests
	quota.MaxSiteEgressBytes = maxEgressBytes
	if err := s.repo.UpsertQuota(ctx, quota); err != nil {
		return err
	}
	s.siteMu.Lock()
	delete(s.siteQuota, projectID)
	s.siteMu.Unlock()
	return nil
}

// RecordSite 记录一次站点响应的字节数。
func (s *Service) RecordSite(ctx context.Context, projectID string, bytes int64) error {
	if bytes <= 0 {
		return nil
	}
	day := s.now().UTC().Format("2006-01-02")
	counter, err := s.siteCounterFor(ctx, projectID, day)
	if err != nil {
		s.warnSite("site traffic counter", err)
		return nil
	}
	s.siteMu.Lock()
	counter.bytes += bytes
	s.siteMu.Unlock()
	return s.maybeFlushSite(ctx)
}

// FlushSite 把内存中的站点流量写入 sys_site_traffic；失败时把计数放回内存。
func (s *Service) FlushSite(ctx context.Context) error {
	store, ok := s.repo.(catalog.SiteTrafficStore)
	if !ok {
		return nil
	}
	s.siteMu.Lock()
	snapshot := s.siteCounters
	s.siteCounters = map[string]*siteCounter{}
	s.sitePending = 0
	s.siteFlushAt = s.now()
	s.siteMu.Unlock()

	var firstErr error
	for projectID, counter := range snapshot {
		if counter.requests == 0 && counter.bytes == 0 {
			continue
		}
		if err := store.IncrementSiteTraffic(ctx, projectID, counter.day, counter.requests, counter.bytes); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			s.warnSite("site traffic flush", err)
			s.siteMu.Lock()
			if s.siteDay == counter.day {
				current := s.siteCounters[projectID]
				if current == nil {
					current = &siteCounter{day: counter.day}
					s.siteCounters[projectID] = current
				}
				current.requests += counter.requests
				current.bytes += counter.bytes
				s.sitePending += counter.requests
			}
			s.siteMu.Unlock()
		}
	}
	return firstErr
}

func (s *Service) siteCounterFor(ctx context.Context, projectID, day string) (*siteCounter, error) {
	s.siteMu.Lock()
	if s.siteDay != day {
		s.siteDay = day
		s.siteCounters = map[string]*siteCounter{}
		s.sitePending = 0
		s.siteFlushAt = s.now()
	}
	counter := s.siteCounters[projectID]
	s.siteMu.Unlock()
	if counter != nil {
		return counter, nil
	}

	var base catalog.SiteTraffic
	if store, ok := s.repo.(catalog.SiteTrafficStore); ok {
		var err error
		base, err = store.GetSiteTraffic(ctx, projectID, day)
		if err != nil {
			return nil, err
		}
	}

	s.siteMu.Lock()
	defer s.siteMu.Unlock()
	if s.siteDay != day {
		s.siteDay = day
		s.siteCounters = map[string]*siteCounter{}
		s.sitePending = 0
		s.siteFlushAt = s.now()
	}
	if counter = s.siteCounters[projectID]; counter == nil {
		counter = &siteCounter{day: day, requests: base.Requests, bytes: base.Bytes}
		s.siteCounters[projectID] = counter
	}
	return counter, nil
}

func (s *Service) siteQuotaFor(ctx context.Context, projectID string) (catalog.ProjectQuota, error) {
	s.siteMu.Lock()
	entry, ok := s.siteQuota[projectID]
	s.siteMu.Unlock()
	if ok && s.now().Before(entry.expires) {
		return entry.quota, nil
	}
	quota, err := s.repo.GetQuota(ctx, projectID)
	if err != nil {
		return catalog.ProjectQuota{}, err
	}
	s.siteMu.Lock()
	s.siteQuota[projectID] = siteQuotaEntry{quota: quota, expires: s.now().Add(15 * time.Second)}
	s.siteMu.Unlock()
	return quota, nil
}

func (s *Service) maybeFlushSite(ctx context.Context) error {
	s.siteMu.Lock()
	due := s.sitePending >= 64 || (!s.siteFlushAt.IsZero() && s.now().Sub(s.siteFlushAt) >= 5*time.Second)
	s.siteMu.Unlock()
	if !due {
		return nil
	}
	return s.FlushSite(ctx)
}

func (s *Service) warnSite(scope string, err error) {
	if s.logger == nil {
		return
	}
	s.logger.Warn("usage: "+scope+" failed", zap.String("err", err.Error()))
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
		// 数据库数量由 catalog 层在 CreateDatabase 时校验。
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
	ErrQuotaExceeded     = errors.New("usage: quota exceeded")
	ErrSiteQuotaExceeded = errors.New("usage: site quota exceeded")
)
