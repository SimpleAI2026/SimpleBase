// 文件说明：audit.go — handler 侧的跨域共享端口：审计 / 配额 / 缓存 / LLM 网关，及其适配器与统一审计入口
//
// 用法
//
//	NewAuditService(*audit.Service) AuditService             // 审计：Record 与操作列表
//	RecordAudit(c, svc, kind, projectID, status, detail)     // handler 写审计的统一入口（svc 为 nil 直接返回）
//	NewUsageService(*usage.Service) UsageService             // 配额检查
//	NewCacheService(*cache.Manager) CacheService             // 缓存用量
//	NewLLMService(llmgateway.Service) LLMService             // OpenAI 兼容端点与模型列表用的网关端口（DTO 在本包）
//
// 调用架构
//
//	[上游] internal/app（构造并经 api.Dependencies 注入）、internal/api 与各业务域 handler
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/httpkit/audit.go
//	         │ 依赖
//	         ▼
//	[下游] internal/audit, internal/usage, godatabase/cache, internal/llmgateway, litellm/providers, internal/catalog
//	[同包] context.go
//
// 主要符号
//
//	type AuditEvent struct {…}
//	type AuditService interface {…}
//	type UsageService interface {…}
//	type CacheService interface {…}
//	type LLMService interface {…}
//	RecordAudit(c echo.Context, svc AuditService, kind, projectID, status, detail string)
//	NewAuditService / NewUsageService / NewCacheService / NewLLMService
//	... 另有 12 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §6.1）：这些端口被 ≥ 3 个业务域的 handler 使用，所以随协议内核
// 落在叶子包；域专属端口（DataService / SQLService / KVService …）随所属业务域迁移，不进本包。

package httpkit

import (
	"context"
	"encoding/json"

	"github.com/labstack/echo/v4"
	"github.com/voocel/litellm/providers"

	"github.com/linkxzhou/SimpleBase/godatabase/cache"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
	"github.com/linkxzhou/SimpleBase/internal/usage"
)

func RecordAudit(c echo.Context, svc AuditService, kind, projectID, status, detail string) {
	if svc == nil {
		return
	}
	ctx := c.Request().Context()
	_ = svc.Record(context.WithoutCancel(ctx), AuditEvent{
		ProjectID:   projectID,
		PrincipalID: PrincipalOf(c).Actor(),
		Kind:        kind,
		RequestID:   RequestIDFromContext(ctx),
		Status:      status,
		Detail:      detail,
	})
}

// CacheService 抽象缓存管理（plan7.md）。
type CacheService interface {
	Usage(ctx context.Context) (CacheUsage, error)
}

// CacheUsage 是缓存用量快照。
type CacheUsage struct {
	TotalBytes   int64
	DatabaseDirs int
}

// UsageService 抽象用量配额（plan9.md）。
type UsageService interface {
	CheckQuota(ctx context.Context, projectID string, kind string) error
}

// AuditService 抽象审计记录（plan9.md）。
type AuditService interface {
	Record(ctx context.Context, e AuditEvent) error
	ListOperations(ctx context.Context, projectID, databaseID string, limit int) ([]catalog.Operation, error)
}

// AuditEvent 是审计事件输入。
type AuditEvent struct {
	DatabaseID  string
	ProjectID   string
	PrincipalID string
	Kind        string
	RequestID   string
	Status      string
	Detail      string
}

// LLMService 抽象 LLM Gateway（plan8.md）。
type LLMService interface {
	Chat(ctx context.Context, projectID string, req LLMRequest) (LLMResponse, error)
	Stream(ctx context.Context, projectID string, req LLMRequest) (LLMStreamReader, error)
	ListProviders(ctx context.Context, projectID string) ([]string, error)
	// ProviderModels 返回各 provider 可用模型名（planv4.1 BUG-07）。
	ProviderModels(ctx context.Context, projectID string) (map[string][]string, error)
}

// LLMRequest 是对外请求抽象。
type LLMRequest struct {
	Model       string
	Messages    []LLMMessage
	MaxTokens   *int
	Temperature *float64
	// Tools 非空时走原生 function calling（云 Agent 与 OpenAI 兼容端点使用）。
	Tools []LLMTool
	// ToolChoice 是调用方指定的工具选择策略（"auto"/"none"/"required" 或指定函数的对象），
	// 原样保存 HTTP 请求里的 JSON 值，由 adapter 反序列化后透传给上游。
	// 空表示不指定；云 Agent 的循环从不设置它（强制选择工具对 agent 循环有害）。
	ToolChoice json.RawMessage
}

// LLMMessage 是对话消息。
type LLMMessage struct {
	Role       string
	Content    string
	ToolCalls  []LLMToolCall
	ToolCallID string
}

// LLMTool 是原生工具定义；Parameters 为 JSON Schema。
type LLMTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// LLMToolCall 是一次原生工具调用。
type LLMToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// LLMResponse 映射 LLM 响应。
type LLMResponse struct {
	Content      string
	Usage        LLMTokenUsage
	Model        string
	Provider     string
	FinishReason string
	ToolCalls    []LLMToolCall
}

// LLMTokenUsage 是 token 用量。
type LLMTokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens,omitempty"`
}

// LLMStreamReader 抽象流式读取。
type LLMStreamReader interface {
	Next() (*LLMStreamChunk, error)
	Close() error
}

// LLMStreamChunk 是流式分块。
type LLMStreamChunk struct {
	Type         string `json:"type"`
	Content      string `json:"content,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
	Done         bool   `json:"-"`
	// 以下字段由 api 层内部消费；对外的 OpenAI 兼容流式帧由 oaiStreamWriter 编码，不直接序列化本结构。
	ToolCallIndex int            `json:"-"`
	ToolCallID    string         `json:"-"`
	ToolCallName  string         `json:"-"`
	ToolCallArgs  string         `json:"-"`
	Usage         *LLMTokenUsage `json:"-"`
}

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
	resp, err := a.s.Chat(ctx, projectID, ToGatewayRequest(req))
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
	reader, err := a.s.Stream(ctx, projectID, ToGatewayRequest(req))
	if err != nil {
		return nil, err
	}
	return &LLMStreamReaderAdapter{Inner: reader}, nil
}

// ToGatewayRequest 把 api 层请求映射为 litellm 形状；工具与工具往返消息原样透传。
func ToGatewayRequest(req LLMRequest) llmgateway.Request {
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
		ToolChoice:  DecodeToolChoice(req.ToolChoice),
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

// DecodeToolChoice 把 HTTP 层保存的 tool_choice JSON 还原成 litellm 接受的 any。
//
// tool_choice 要么是字符串（"auto"/"none"/"required"），要么是
// {"type":"function","function":{"name":"..."}} 对象，json 解码到 any 即可保持原形。
// 空值或非法 JSON 都返回 nil（不指定）：非法值应由 handler 入口校验并以 400 拒绝，
// 这里不再重复报错，避免改动 ToGatewayRequest 的签名而波及所有调用方与测试桩。
func DecodeToolChoice(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

func (a *llmServiceAdapter) ListProviders(ctx context.Context, projectID string) ([]string, error) {
	return a.s.ListProviders(ctx, projectID)
}

// ProviderModels 暴露各 provider 可用模型名（无 base_url/api_key），
// 供云 Agent 模型列表（planv4.1 BUG-07）。
func (a *llmServiceAdapter) ProviderModels(ctx context.Context, projectID string) (map[string][]string, error) {
	return a.s.ProviderModels(ctx, projectID)
}

// NewLLMService 构造 LLMService 适配器。
func NewLLMService(s llmgateway.Service) LLMService {
	if s == nil {
		return nil
	}
	return &llmServiceAdapter{s: s}
}

// LLMStreamReaderAdapter 适配 llmgateway.StreamReader。
type LLMStreamReaderAdapter struct{ Inner llmgateway.StreamReader }

func (a *LLMStreamReaderAdapter) Next() (*LLMStreamChunk, error) {
	chunk, err := a.Inner.Next()
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

func (a *LLMStreamReaderAdapter) Close() error { return a.Inner.Close() }
