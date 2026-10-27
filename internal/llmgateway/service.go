// 文件说明：service.go — 在 SimpleBase 内部封装 litellm 多供应商客户端，
//
// 用法
//
//	NewService(resolver ProviderResolver, recorder UsageRecorder, logger ...
//	type ProviderConfig（导出类型，同包与上层均可使用）
//	type ProjectProviders（导出类型，同包与上层均可使用）
//	同包调用：service → catalog_resolver.go, fallback_resolver.go 等；ProviderConfig → catalog_resolver.go, fallback_resolver.go 等；ProjectProviders → catalog_resolver.go, fallback_resolver.go 等
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/llmgateway/service.go
//	         │ 依赖
//	         ▼
//	[下游] internal/observability
//	[同包] catalog_resolver.go, fallback_resolver.go, project_cred_resolver.go, provider_driver.go, streaming.go
//	[关键引用] service ← catalog_resolver.go, fallback_resolver.go, project_cred_resolver.go
//
// 主要符号
//
//	type ProviderConfig struct {…}
//	type ProjectProviders struct {…}
//	type Request struct {…}
//	type Response struct {…}
//	type Usage struct {…}
//	type StreamReader interface {…}
//	... 另有 20 个顶层符号

package llmgateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

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
	AllowedModels []string          `json:"allowed_models,omitempty"`
	Timeout       time.Duration     `json:"timeout,omitempty"` // 整体请求超时；0=不限（planv6.1 O2）
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
	// ToolChoice 对应 OpenAI 的 tool_choice（字符串或 {"type":"function",...} 对象）；
	// nil 表示不指定。仅在 Tools 非空时有意义，由 litellm 驱动按供应商转换。
	ToolChoice any
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

// ProviderModels 解析各 provider 的 allowed_models 和默认模型（planv4.1 BUG-07）。
// 解析失败时返回空 map 而非错误（模型列表缺失不应阻塞 Agent 功能）。
func (s *service) ProviderModels(ctx context.Context, projectID string) (map[string][]string, error) {
	pp, err := s.resolver.Resolve(ctx, projectID)
	if err != nil {
		return map[string][]string{}, nil
	}
	out := make(map[string][]string, len(pp.Providers))
	for _, p := range pp.Providers {
		models := append([]string(nil), p.AllowedModels...)
		if p.Model != "" && (len(models) == 0 || checkModelAllowed(p, p.Model) == nil) {
			found := false
			for _, m := range models {
				if m == p.Model {
					found = true
					break
				}
			}
			if !found {
				models = append(models, p.Model)
			}
		}
		if len(models) > 0 {
			out[p.Name] = models
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
	// planv6.1 O2: 重试 + 超时分层。MaxRetries=3（5xx/429/网络），ConnectTimeout=10s，
	// RequestTimeout 来自 cfg.Timeout（若非零）或上层 ctx deadline。
	resilience := litellm.DefaultResilienceConfig()
	resilience.MaxRetries = 3
	resilience.ConnectTimeout = 10 * time.Second
	resilience.RequestTimeout = cfg.Timeout // 0=不限（由 ctx 控）
	c, err := litellm.NewWithProvider(driver, litellm.ProviderConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Timeout:    cfg.Timeout,
		Resilience: resilience,
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

// resolveModel 决定本次调用实际发给上游的模型名，并把结果写进日志。
//
// 语义（planv6.0 v2 §11）：reqModel 为空表示调用方没有显式指定模型，
// 此时回填供应商的 default_model。两边都为空说明这个项目实际没有可用模型，
// 宁可在这里给出 llm_model_not_configured，也不要把空模型名发给上游
// ——那会得到供应商侧的模糊报错，且日志里只看到 model=""，难以定位。
//
// 该函数不记录 api_key，只记录解析结果。
func (s *service) resolveModel(projectID string, cfg ProviderConfig, reqModel string) (string, error) {
	model := strings.TrimSpace(reqModel)
	source := "request"
	if model == "" {
		model = strings.TrimSpace(cfg.Model)
		source = "provider_default"
	}
	if model == "" {
		return "", fmt.Errorf("%w: provider %q has no default model", ErrModelNotConfigured, cfg.Name)
	}
	if err := checkModelAllowed(cfg, model); err != nil {
		return "", err
	}
	if s.logger != nil {
		s.logger.Debug("llmgateway: model resolved",
			zap.String("project", projectID),
			zap.String("provider", cfg.Name),
			zap.String("model", model),
			zap.String("source", source))
	}
	return model, nil
}

// prepareRequest 完成 Chat/Stream 共用的请求准备：取默认 provider 的 client、
// 解析实际模型（回填 default_model + allowlist 校验）、构造 providers.Request。
// 两种执行与用量记录流程不在其中（planv6.0 §7 A2）。
func (s *service) prepareRequest(ctx context.Context, projectID string, req Request) (*litellm.Client, ProviderConfig, *providers.Request, error) {
	// 若请求指定 model，仍使用默认 provider（litellm client 绑定 provider）。
	c, cfg, err := s.getClient(ctx, projectID, "")
	if err != nil {
		return nil, ProviderConfig{}, nil, err
	}
	model, err := s.resolveModel(projectID, cfg, req.Model)
	if err != nil {
		return nil, ProviderConfig{}, nil, err
	}
	lreq := &providers.Request{
		Model:       model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Tools:       req.Tools,
	}
	// 没有工具时 tool_choice 没有意义，部分上游会直接 400，因此不透传。
	if len(req.Tools) > 0 {
		lreq.ToolChoice = req.ToolChoice
	}
	return c, cfg, lreq, nil
}

// Chat 执行非流式对话。
func (s *service) Chat(ctx context.Context, projectID string, req Request) (Response, error) {
	c, _, lreq, err := s.prepareRequest(ctx, projectID, req)
	if err != nil {
		return Response{}, err
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
	c, cfg, lreq, err := s.prepareRequest(ctx, projectID, req)
	if err != nil {
		return nil, err
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
			model:    lreq.Model,
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
	// ErrModelNotConfigured：请求未指定模型，供应商也没有 default_model。
	// 调用方（云助手）把它映射为 llm_model_not_configured，提示用户去设置页配置。
	ErrModelNotConfigured = errors.New("llmgateway: no model configured")
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
