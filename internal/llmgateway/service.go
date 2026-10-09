// Package llmgateway 在 SimpleBase 内部封装 litellm 多供应商客户端，
// 暴露受项目隔离的 LLM 服务端 API（plan8.md）。
//
// 关键约束：
//   - 禁止从环境变量自动发现或记录供应商密钥。密钥来源只能是 catalog 中的 provider 配置。
//   - handler 不直接依赖 litellm.Client；通过 Service 接口隔离。
//   - 用量按 project 维度统计，与数据库用量共用 usage 基础设施（plan9.md）。
//   - 流式响应使用 SSE/分块转发，禁止缓冲完整响应。
//
// litellm v1.5.8 的 Client 绑定单个 Provider；本包为每个 project+provider
// 维护独立 client（按需创建、缓存）。
package llmgateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/voocel/litellm"
	"github.com/voocel/litellm/providers"
	"go.uber.org/zap"
)

// ProviderConfig 描述单个供应商在 catalog 中的配置（已解密）。
// 密钥仅在此结构中短暂存在，禁止日志/审计回显。
type ProviderConfig struct {
	Name    string `json:"name"`    // 供应商标识：openai/anthropic/gemini/...
	APIKey  string `json:"api_key"` // 解密后的密钥；不写入日志
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	// AllowedModels 非空时，请求的 model 必须在列表内（planv4.0 cloud-agent-optimization-plan §4.7）。
	AllowedModels []string `json:"allowed_models,omitempty"`
}

// ProjectProviders 描述一个 project 可用的供应商集合。
type ProjectProviders struct {
	ProjectID string
	Default   string
	Providers []ProviderConfig
}

// Request 是 LLM Gateway 对外的请求抽象，映射到 providers.Request。
type Request struct {
	Model       string
	Messages    []providers.Message
	MaxTokens   *int
	Temperature *float64
	// Tools 非空时走原生 function calling。
	Tools []providers.Tool
}

// Response 映射 providers.Response。
type Response struct {
	Content      string
	Usage        Usage
	Model        string
	Provider     string
	FinishReason string
	ToolCalls    []providers.ToolCall
}

// Usage 是 LLM token 用量。
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int
}

// StreamReader 抽象流式读取。
type StreamReader interface {
	Next() (*providers.StreamChunk, error)
	Close() error
}

// UsageRecorder 由 plan9 的 usage.Service 实现。
type UsageRecorder interface {
	RecordLLM(ctx context.Context, projectID, provider, model string, u Usage) error
}

// Service 是 LLM Gateway 的核心接口。handler 仅依赖此接口。
type Service interface {
	Chat(ctx context.Context, projectID string, req Request) (Response, error)
	Stream(ctx context.Context, projectID string, req Request) (StreamReader, error)
	ListProviders(ctx context.Context, projectID string) ([]string, error)
	// ProviderModels 返回各 provider 的可用模型名列表（无密钥/URL），
	// 供云 Agent 模型选择（planv4.1 BUG-07）。
	ProviderModels(ctx context.Context, projectID string) (map[string][]string, error)
}

// ProviderResolver 从 catalog 解析 project 的供应商配置（含解密密钥）。
type ProviderResolver interface {
	Resolve(ctx context.Context, projectID string) (ProjectProviders, error)
}

// service 实现。
type service struct {
	resolver ProviderResolver
	recorder UsageRecorder
	logger   observability.Logger

	mu      sync.Mutex
	clients map[string]cachedClient // key: projectID|providerName
}

// cachedClient 按凭证指纹缓存。设置里改了 key 或 base_url 后，下一次请求换新 client，无需重启。
type cachedClient struct {
	client *litellm.Client
	fp     string
}

// NewService 构造 Service。
func NewService(resolver ProviderResolver, recorder UsageRecorder, logger observability.Logger) Service {
	return &service{
		resolver: resolver,
		recorder: recorder,
		logger:   logger,
		clients:  make(map[string]cachedClient),
	}
}

// clientKey 构造缓存 key。
func clientKey(projectID, providerName string) string {
	return projectID + "|" + providerName
}

// ProviderModels 解析各 provider 的 allowed_models（planv4.1 BUG-07）。
// 解析失败时返回空 map 而非错误（模型列表缺失不应阻塞 Agent 功能）。
func (s *service) ProviderModels(ctx context.Context, projectID string) (map[string][]string, error) {
	pp, err := s.resolver.Resolve(ctx, projectID)
	if err != nil {
		return map[string][]string{}, nil
	}
	out := make(map[string][]string, len(pp.Providers))
	for _, p := range pp.Providers {
		if len(p.AllowedModels) > 0 {
			out[p.Name] = p.AllowedModels
		}
	}
	return out, nil
}

// getClient 获取或创建指定 project+provider 的 litellm client。
func (s *service) getClient(ctx context.Context, projectID, providerName string) (*litellm.Client, ProviderConfig, error) {
	pp, err := s.resolver.Resolve(ctx, projectID)
	if err != nil {
		return nil, ProviderConfig{}, fmt.Errorf("llmgateway: resolve providers: %w", err)
	}
	var cfg ProviderConfig
	found := false
	if providerName == "" {
		// 使用默认供应商。
		if pp.Default == "" && len(pp.Providers) > 0 {
			providerName = pp.Providers[0].Name
		} else {
			providerName = pp.Default
		}
	}
	for _, p := range pp.Providers {
		if p.Name == providerName {
			cfg = p
			found = true
			break
		}
	}
	if len(pp.Providers) == 0 {
		return nil, ProviderConfig{}, ErrNoProviders
	}
	if !found {
		return nil, cfg, fmt.Errorf("%w: %s", ErrProviderNotFound, providerName)
	}

	fp := credFingerprint(cfg)
	key := clientKey(projectID, providerName)
	s.mu.Lock()
	if hit, ok := s.clients[key]; ok && hit.fp == fp {
		s.mu.Unlock()
		return hit.client, cfg, nil
	}
	s.mu.Unlock()
	// 创建单供应商 client。禁止自动发现；逻辑名（如 custom_openai）映射到 litellm 驱动。
	driver := cfg.Name
	if mapped, ok := litellmDriver(cfg.Name); ok {
		driver = mapped
	}
	c, err := litellm.NewWithProvider(driver, litellm.ProviderConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
	})
	if err != nil {
		return nil, cfg, fmt.Errorf("llmgateway: create client for %s: %w", providerName, err)
	}
	s.mu.Lock()
	s.clients[key] = cachedClient{client: c, fp: fp}
	s.mu.Unlock()
	return c, cfg, nil
}

// credFingerprint 只用于判断缓存是否过期，不进入日志。
func credFingerprint(cfg ProviderConfig) string {
	h := sha256.New()
	_, _ = io.WriteString(h, cfg.APIKey)
	_, _ = h.Write([]byte{0})
	_, _ = io.WriteString(h, cfg.BaseURL)
	_, _ = h.Write([]byte{0})
	_, _ = io.WriteString(h, cfg.Name)
	return hex.EncodeToString(h.Sum(nil))
}

// Chat 执行非流式对话。
func (s *service) Chat(ctx context.Context, projectID string, req Request) (Response, error) {
	providerName := ""
	// 若请求指定 model，仍使用默认 provider（litellm client 绑定 provider）。
	c, cfg, err := s.getClient(ctx, projectID, providerName)
	if err != nil {
		return Response{}, err
	}
	model := req.Model
	if model == "" {
		model = cfg.Model
	}
	if err := checkModelAllowed(cfg, model); err != nil {
		return Response{}, err
	}
	lreq := &providers.Request{
		Model:       model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Tools:       req.Tools,
	}
	resp, err := c.Chat(ctx, lreq)
	if err != nil {
		return Response{}, fmt.Errorf("llmgateway: chat: %w", err)
	}
	out := Response{
		Content:      resp.Content,
		Model:        resp.Model,
		Provider:     resp.Provider,
		FinishReason: resp.FinishReason,
		ToolCalls:    resp.ToolCalls,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			ReasoningTokens:  resp.Usage.ReasoningTokens,
		},
	}
	if s.recorder != nil {
		if rerr := s.recorder.RecordLLM(ctx, projectID, resp.Provider, resp.Model, out.Usage); rerr != nil {
			if s.logger != nil {
				s.logger.Warn("llmgateway: record usage failed",
					zap.String("project", projectID),
					zap.String("err", rerr.Error()))
			}
		}
	}
	return out, nil
}

// Stream 执行流式对话。
func (s *service) Stream(ctx context.Context, projectID string, req Request) (StreamReader, error) {
	c, cfg, err := s.getClient(ctx, projectID, "")
	if err != nil {
		return nil, err
	}
	model := req.Model
	if model == "" {
		model = cfg.Model
	}
	if err := checkModelAllowed(cfg, model); err != nil {
		return nil, err
	}
	lreq := &providers.Request{
		Model:       model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Tools:       req.Tools,
	}
	reader, err := c.Stream(ctx, lreq)
	if err != nil {
		return nil, fmt.Errorf("llmgateway: stream: %w", err)
	}
	if s.recorder != nil {
		return &usageRecordingReader{
			inner:    reader,
			recorder: s.recorder,
			project:  projectID,
			provider: cfg.Name,
			model:    model,
			logger:   s.logger,
		}, nil
	}
	return reader, nil
}

// ListProviders 返回 project 可用供应商名（不含密钥）。
func (s *service) ListProviders(ctx context.Context, projectID string) ([]string, error) {
	pp, err := s.resolver.Resolve(ctx, projectID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(pp.Providers))
	for _, p := range pp.Providers {
		names = append(names, p.Name)
	}
	return names, nil
}

// 错误定义。
var (
	ErrNoProviders      = errors.New("llmgateway: no providers configured for project")
	ErrProviderNotFound = errors.New("llmgateway: provider not found")
	ErrModelNotAllowed  = errors.New("llmgateway: model not allowed")
)

// checkModelAllowed 在发上游前校验白名单；列表为空表示不限制。
func checkModelAllowed(cfg ProviderConfig, model string) error {
	if len(cfg.AllowedModels) == 0 || model == "" {
		return nil
	}
	for _, m := range cfg.AllowedModels {
		if m == model {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrModelNotAllowed, model)
}
