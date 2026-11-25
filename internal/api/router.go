// 文件说明：router.go — SimpleBase 的 v1 HTTP API
//
// 用法
//
//	NewRouter(deps Dependencies) *echo.Echo
//	useGlobalMiddleware(e, &deps) bool
//	mountPublicAuthRoutes(e, deps)
//	type Dependencies（导出类型，同包与上层均可使用）
//	type CacheService（导出类型，同包与上层均可使用）
//	同包调用：AuditService → adapter.go, agent_handler.go 等；AuditEvent → adapter.go, agent_handler.go 等；UsageService → adapter.go, agent_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/router.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, internal/auth, internal/catalog, internal/config 等
//	[同包] adapter.go, agent_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] AuditService ← adapter.go, agent_handler.go, backup_handler.go
//
// 主要符号
//
//	type Dependencies struct {…}
//	type CacheService interface {…}
//	type CacheUsage struct {…}
//	type UsageService interface {…}
//	type AuditService interface {…}
//	type AuditEvent struct {…}
//	... 另有 27 个顶层符号

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gosandbox/image"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
	"github.com/linkxzhou/SimpleBase/internal/functionrun"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/web"
	"github.com/prometheus/client_golang/prometheus"
)

// Dependencies 是 NewRouter 注入的全部运行期依赖；零值字段表示对应能力未启用。
type Dependencies struct {
	Config  config.Config
	Logger  observability.Logger
	Metrics *observability.Metrics
	// MetricsRegistry 是 Metrics 所属的 Prometheus registry（/metrics 输出源）。
	// nil 时回退 prometheus 默认 registry（兼容旧装配与测试）。
	MetricsRegistry prometheus.Gatherer
	Health          HealthChecker
	// 业务依赖（Plan 5 起填充）
	Auth            *auth.Service
	Catalog         CatalogService
	Registry        RegistryService
	DatabaseHandler *DatabaseHandler
	// Plan 6：SQL 执行 handler。writable=false 时仅 query 可用。
	SQLHandler             *SQLHandler
	DataHandler            *DataHandler
	CollectionMetaHandler  *CollectionMetaHandler
	BackupHandler          *BackupHandler
	RealtimeHandler        *RealtimeHandler
	FunctionRunner         *functionrun.Runner
	FunctionTriggerHandler *FunctionTriggerHandler
	TriggerEvents          objectEventPublisher
	// SchemaHandler 管理 SQL 数据库的表结构。nil 时不挂载。
	SchemaHandler *SchemaHandler
	// KVHandler：项目级 Key-Value 数据服务（key-value-ducklake-plan）。
	KVHandler *KVHandler
	// KVService：项目 KV catalog 服务（建项目时建 kind=kv 行）。
	KVService KVService
	// KVInit：项目 KV catalog 行创建后初始化 kv schema。
	KVInit KVInitializer
	// Plan 7-9：缓存、用量、审计、LLM
	Cache CacheService
	Usage UsageService
	Audit AuditService
	// EndUsers 是终端用户服务（§8.2.6；nil = 不挂终端用户路由）。
	EndUsers *auth.EndUserService
	LLM      LLMService
	// S3FileStore：用户文件存储（objectstore.FileStore）。
	S3FileStore objectstore.FileStore
	// System 是实例系统 DuckLake（元数据 / 指标 / 日志 / S3 索引 / LLM 会话 / Cloud Agent）。
	System *systemdb.Store
	// Sites 是对象存储发布网站的站点开关 / 公开路径存储（planv6.5）。
	// nil 且 System 非 nil 时由 NewRouter 基于 System 自动构造（带 sites.cache_ttl 缓存）。
	Sites SiteStore
	// CloudAgent 是 Phase 1 单 agent + 只读工具运行时。
	CloudAgent *cloudagent.Runtime
	// CronScheduler 是统一定时任务调度器（云函数 + Agent）；nil 时 trigger 返回 503。
	CronScheduler *cronjob.Scheduler
	// AgentRun 是 Agent 协作应用服务（run / 群轮次 / 审批 / 事件环 / 生命周期）：
	// app 构造并注入，cron 的群定时执行器直接持有同一个 Service，不再经路由延迟绑定。
	AgentRun *agentrun.Service
	// Sandbox 是项目级云沙盒服务；未配置时仅 capabilities 路由可用。
	Sandbox      SandboxService
	SandboxUsage interface {
		RecordSandbox(context.Context, string, string, int64) error
	}
	// Images 是镜像 store；nil 表示本实例不分发镜像（ext/images 回答 501）。
	Images *image.Store
	// Terminal 是浏览器终端的 SSH 桥；nil 表示终端不可用（capabilities.terminal=false）。
	Terminal *TerminalBridge
	// login-auth-plan：登录态与用户管理。
	Sessions *auth.SessionService
	Users    *auth.UserService
	// Delegations 校验助手 CLI 委托 JWT。nil 时不接受委托令牌。
	Delegations *auth.DelegationRegistry
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

// HealthChecker 由 App 提供；live 不做 I/O，ready 检查 catalog/S3。
type HealthChecker interface {
	Live(ctx context.Context) error
	Ready(ctx context.Context) error
}

// NewRouter 装配全部路由与中间件。全局中间件顺序：
// site（可选，e.Pre）→ request ID → CORS（可选）→ security headers →
// perf stage（可选）→ recover → access log → gzip（可选）→
// pre_auth 限流 → body limit。
// 之后依次挂载 health / 观测 / 登录与终端用户路由、/v1 业务路由（auth → project
// context → handler）、OpenAPI、站点面与 SPA fallback；错误统一经 errorHandler 映射。
func NewRouter(deps Dependencies) *echo.Echo {
	e := echo.New()
	e.Logger.SetOutput(nullWriter{})
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = errorHandler(deps)
	requestLogger = deps.Logger
	e.IPExtractor = trustedIPExtractor(deps.Config.HTTP.TrustedProxies)

	sitesOn := useGlobalMiddleware(e, &deps)

	// /health/live 与 /health/ready 不经认证
	health := &HealthHandler{checker: deps.Health}
	e.GET("/health/live", health.Live)
	e.GET("/health/ready", health.Ready)

	// §8.1.7：观测端点收口（独立监听 / token / 404）。
	mountObservabilityRoutes(e, deps)

	mountPublicAuthRoutes(e, deps)

	// v1 业务路由组：认证 → project context → 各 handler
	if deps.Auth != nil && deps.DatabaseHandler != nil {
		mountV1Routes(e, deps)
		mountGoRoutes(e, deps)
		mountE2BRoutes(e, deps)
	}

	e.GET("/v1/openapi.json", func(c echo.Context) error {
		return c.JSON(http.StatusOK, buildOpenAPI(e.Routes()))
	})

	// planv6.5 §5.2 / planv6.7 §1.1：站点面（匿名只读）。handler 内再校验分流标记，控制台域直接访问 /__site 返回 404。
	if sitesOn {
		site := &SiteHandler{store: deps.S3FileStore, sites: deps.Sites, cfg: deps.Config.Sites}
		e.Any(sitePathPrefix+"/*", site.Serve)
	}

	// 管理端静态资源：所有未匹配 API 路由的 GET 请求回退到前端 SPA。
	// 必须在 /v1、/health、/metrics 等路由注册后调用，以免拦截 API 请求。
	web.Register(e)

	return e
}

// useGlobalMiddleware 按固定顺序挂载站点改写与全局中间件，返回站点面是否启用。
func useGlobalMiddleware(e *echo.Echo, deps *Dependencies) bool {
	// planv6.5 §5.1 / planv6.7 §1.2：对象存储发布网站。按模式在路由前改写到 /__site/*。
	if deps.Sites == nil && deps.System != nil {
		deps.Sites = NewSiteStore(deps.System, deps.Config.Sites.EffectiveCacheTTL())
	}
	mode := deps.Config.Sites.Mode()
	sitesOn := deps.Sites != nil && deps.S3FileStore != nil && (mode == config.SiteModePath || deps.Config.Sites.Configured())
	if sitesOn {
		if mode == config.SiteModeSubdomain {
			e.Pre(siteHostMiddleware(deps.Config.Sites))
		} else {
			e.Pre(sitePathMiddleware(deps.Config.Sites))
		}
	}

	e.Use(requestIDMiddleware())
	if deps.Config.HTTP.CORS.Enabled {
		e.Use(corsMiddleware(deps.Config.HTTP.CORS))
	}
	e.Use(securityHeadersMiddleware(deps.Config))
	// 分段计时（api-db-perf-validation-plan §2.1）：默认关闭，开启后输出
	// simplebase_api_stage_seconds 与 debug 日志行。必须在 requestID 之后。
	// planv5.0 §4 P0.1：pprof 挂载已与 perf_stage_timing 解耦（§8.1.7），见 mountObservabilityRoutes。
	if deps.Config.Observability.PerfStageTiming {
		var recorder stageTimingRecorder
		if deps.Metrics != nil {
			recorder = deps.Metrics
		}
		e.Use(perfStageMiddleware(recorder, deps.Logger))
		// planv5.0 §4 P0.1：认证子阶段（锁等待/缓存/查库/项目集）注入
		// 认证服务；观察者从请求 ctx 取 timer，关闭时零开销。
		if deps.Sessions != nil {
			deps.Sessions.WithAuthObserver(newStageAuthObserver())
		}
		if deps.Auth != nil {
			deps.Auth.WithAuthObserver(newStageAuthObserver())
		}
		// planv5.0 §4 P0.1：开启 mutex/block 事件采样，供 /debug/pprof
		// 取证 principalMu 锁 convoy；1/1000 采样率足够诊断且开销可忽略。
		runtime.SetMutexProfileFraction(1000)
		runtime.SetBlockProfileRate(1000)
	}
	e.Use(middleware.Recover())
	e.Use(accessLogMiddleware(*deps))
	if deps.Config.HTTP.Gzip {
		e.Use(gzipMiddleware())
	}
	// §8.1.6：pre_auth 限流只跑 key=ip 的规则（全局挂载，认证前）。
	e.Use(newRateLimiter(deps.Config.Limits.RateLimit, deps.Metrics).middleware(preAuthStage))
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		// S3 上传单独放宽，见 s3UploadBodyLimit。全局 1MB 会让控制台允许的文件先 413，
		// 开发代理再把未读完的请求体报成 500。
		Skipper: skipLargeUploadBodyLimit(deps.S3FileStore != nil, deps.Sandbox != nil),
		Limit:   bodyLimit(deps.Config.Limits.MaxRequestBytes),
	}))
	return sitesOn
}

// mountPublicAuthRoutes 挂载免认证登录、刷新、初始改密与终端用户路由。
func mountPublicAuthRoutes(e *echo.Echo, deps Dependencies) {
	if deps.Sessions != nil && deps.Users != nil {
		ah := NewAuthHandler(deps.Sessions, deps.Users).WithAudit(deps.Audit)
		e.POST("/v1/auth/login", ah.Login)
		e.POST("/v1/auth/refresh", ah.Refresh)
		// §8.2.2：强制改密闭环（免认证；复用 loginGuard）。
		e.POST("/v1/auth/password/initial", ah.InitialPassword)
	}

	// §8.2.6：终端用户路由（独立密钥域 + 独立守卫；signup/login 免认证，
	// me 走 endUserAuth；pre_auth 限流已覆盖 key=ip 规则）。
	if deps.EndUsers != nil {
		eh := NewEndUsersHandler(deps.EndUsers).WithAudit(deps.Audit)
		e.POST("/v1/projects/:projectID/end-users/signup", eh.Signup)
		e.POST("/v1/projects/:projectID/end-users/login", eh.Login)
		e.GET("/v1/projects/:projectID/end-users/me", eh.Me, endUserAuth(deps.Config.Auth.SessionSecret))
	}
}

// authMW 构造统一认证中间件。
// deps.Sessions 为具体指针类型，直接传入 auth.SessionAuthenticator 接口会形成
// typed-nil：接口值非 nil 但 receiver 为 nil，委托令牌回退登录态校验时 panic
// （agent_skills_router_e2e 样例17回归）。此处显式归一为 nil 接口。
func (deps Dependencies) authMW() echo.MiddlewareFunc {
	var sessions auth.SessionAuthenticator
	if deps.Sessions != nil {
		sessions = deps.Sessions
	}
	return withInternalPrincipalAuth(auth.AuthMiddlewareWithDelegation(sessions, deps.Auth, WithPrincipal, deps.Config.Auth.APIKeyHashSecret, deps.Delegations))
}
