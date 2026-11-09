<!-- status: completed -->
<!-- progress: 2026-07-19 完成：llmgateway.Service（项目隔离、单供应商 client 缓存、用量记录）、CatalogResolver（CredentialRef 模式，禁止环境变量发现）、SSE 流式转发、usage 适配器。适配 litellm v1.5.8 API。全部测试通过。 -->

# Plan 8：LLM Gateway、流式转发、密钥与用量

## 目标
将现有 `/litellm`（当前包声明为 `github.com/voocel/litellm`）作为纯 Go 多供应商客户端，通过新 `internal/llmgateway` 暴露受项目隔离的服务端 API。禁止在 handler 中从环境变量自动发现或记录供应商密钥。

## 当前调用点
- `litellm/client.go` 提供 `New`、`WithProviderConfig`、`Chat`、`Stream`、`Models`。
- `Client.Chat`/`Stream` 已做基础请求验证、provider 解析、默认参数和 provider error wrap。
- 新 gateway 必须在调用前完成 project 策略、预算、密钥解析和请求限制；不改坏现有客户端对外 API。

## 新目录
```text
internal/llmgateway/service.go
internal/llmgateway/policy.go
internal/llmgateway/client_factory.go
internal/llmgateway/handler.go
internal/llmgateway/openai_types.go
internal/llmgateway/sse.go
internal/llmgateway/usage.go
internal/llmgateway/service_test.go
```

## 核心接口
```go
type CredentialResolver interface { Resolve(ctx context.Context, ref string) (litellm.ProviderConfig, error) }
type Policy struct { ProjectID string; AllowedProviders, AllowedModels []string; DefaultModel string; MaxTokens int; RequestTimeout time.Duration; MonthlyBudgetMicros int64 }
type Service struct { catalog *catalog.Service; credentials CredentialResolver; usage *usage.Service; clients sync.Map }
func (s *Service) Chat(ctx context.Context, principal auth.Principal, projectID string, req ChatRequest) (ChatResponse, error)
func (s *Service) Stream(ctx context.Context, principal auth.Principal, projectID string, req ChatRequest, emit func(StreamEvent) error) error
func (s *Service) Models(ctx context.Context, principal auth.Principal, projectID string) ([]Model, error)
```
client cache key 必须是 provider config version/credential reference 的 hash，不是 API key 原文。凭据变更后使旧 key 失效/驱逐。provider client 仅能用于对应 project，不能用全局默认 client 混用 BYOK。

## API 与兼容
```go
v1.POST("/llm/chat/completions", h.ChatCompletions, Require(auth.LLMInvoke))
v1.POST("/llm/responses", h.Responses, Require(auth.LLMInvoke))
v1.GET("/llm/models", h.Models, Require(auth.LLMInvoke))
p.GET("/usage", h.ProjectUsage, Require(auth.ProjectAdmin))
```
首期可先实现与 `litellm.Request` 一一映射的 `chat/completions` DTO；`responses` 如果现有 request 模型不能完整支持，返回 501 `feature_not_implemented`，不要伪造 OpenAI 完整兼容。每个请求必须显式 project（header 或 path），并由 principal 授权验证。

## `Chat` 流程
1. 校验请求大小、model、messages、tools 数量、max tokens；生成 request ID。
2. 获取 project policy 与启用 provider 配置；model 不在 allowlist 返回 403。
3. 在调用上游前执行 `usage.Reserve`，预算不足返回 429/402（选择并固定一种 code）。
4. `CredentialResolver.Resolve` 取得内存中的 provider config；构建/复用 scoped `litellm.Client`。
5. 使用 `context.WithTimeout` 调 `Client.Chat`；错误映射为 validation、rate_limit、upstream_timeout、upstream_unavailable。
6. 成功/失败都通过 `usage.Settle` 记录 request ID、provider/model、token、时延和成本；不持久化 prompt/response 正文。

## SSE `Stream`
```go
func (h *Handler) ChatCompletions(c echo.Context) error {
  if !request.Stream { return h.nonStreaming(c) }
  c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
  c.Response().Header().Set("Cache-Control", "no-cache")
  c.Response().WriteHeader(http.StatusOK)
  return h.service.Stream(ctx, principal, projectID, req, func(ev StreamEvent) error {
      return writeSSE(c.Response(), ev) // 每个 event 后 Flush
  })
}
```
- 在写 header 前完成认证、策略、预算预留与 provider client 创建。
- 监听 `c.Request().Context().Done()`；客户端断开即 cancel 上游 stream 并结算已知 usage。
- SSE event 只输出规范 JSON，禁止把上游原始错误/密钥透传；结束发送 `[DONE]` 或确定结束事件。
- SSE 连接数单独 semaphore，不占用数据库查询 semaphore。

## 密钥与 BYOK
- Catalog 保存 `CredentialRef`，不保存明文；首期可实现 `EncryptedCredentialStore`（AES-GCM/KMS 包装）或对接已有秘密服务。
- BYOK 从 request/header 接收时只用于本次调用，不持久化、不写审计日志、不进入 client cache key 明文。
- 任何 config/log/error/metrics 均不能出现 API key、Authorization、prompt 全文。

## 测试与验收
- fake `litellm.Provider` 验证 allowlist、timeout、budget reserve/settle、错误映射。
- 流式测试验证 content 顺序、flush、客户端 cancel 上游、结束事件和 usage 写入。
- 一个项目的 credential/model 不可被另一项目调用；日志断言不含 secret/prompt。
- 数据库 registry 故障时 LLM 路由仍可调用；LLM 上游故障不影响数据库 API。
