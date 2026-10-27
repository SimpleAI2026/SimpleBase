// 文件说明：router.go — SimpleBase 的 v1 HTTP API
//
// 用法
//
//	NewRouter(deps Dependencies) *echo.Echo
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
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/linkxzhou/SimpleBase/gocloudagent"
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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	// pprof 仅在 PerfStageTiming 诊断模式挂载（planv5.0 §4 P0.1）。
	"net/http/pprof"
)

// Dependencies 是 NewRouter 注入的全部运行期依赖。各字段可由后续 plan 逐步填充。
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
	LLM   LLMService
	// S3FileStore：用户文件存储（objectstore.FileStore）。
	S3FileStore objectstore.FileStore
	// System 是实例系统 DuckLake（元数据 / 指标 / 日志 / S3 索引 / LLM 会话 / Cloud Agent）。
	System *systemdb.Store
	// CloudAgent 是 Phase 1 单 agent + 只读工具运行时。
	CloudAgent *gocloudagent.Runtime
	// CronScheduler 是统一定时任务调度器（云函数 + Agent）；nil 时 trigger 返回 503。
	CronScheduler *cronjob.Scheduler
	// Sandbox 是项目级云沙盒服务；未配置时仅 capabilities 路由可用。
	Sandbox      SandboxService
	SandboxUsage interface {
		RecordSandbox(context.Context, string, string, int64) error
	}
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

// NewRouter 按 plan1 规定的中间件顺序装配路由：
// request ID → recover → access log → body limit → auth（业务组）→
// project context → error mapper。
// 业务路由由后续 plan 在此函数内挂载。
func NewRouter(deps Dependencies) *echo.Echo {
	e := echo.New()
	e.Logger.SetOutput(nullWriter{})
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = errorHandler(deps)
	requestLogger = deps.Logger
	e.IPExtractor = trustedIPExtractor(deps.Config.HTTP.TrustedProxies)

	e.Use(requestIDMiddleware())
	if deps.Config.HTTP.CORS.Enabled {
		e.Use(corsMiddleware(deps.Config.HTTP.CORS))
	}
	e.Use(securityHeadersMiddleware())
	// 分段计时（api-db-perf-validation-plan §2.1）：默认关闭，开启后输出
	// simplebase_api_stage_seconds 与 debug 日志行。必须在 requestID 之后。
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
		pprofRoute := func(path string, h http.Handler) {
			e.GET(path, echo.WrapHandler(h))
		}
		pprofRoute("/debug/pprof/", http.HandlerFunc(pprof.Index))
		pprofRoute("/debug/pprof/heap", pprof.Handler("heap"))
		pprofRoute("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		pprofRoute("/debug/pprof/mutex", pprof.Handler("mutex"))
		pprofRoute("/debug/pprof/block", pprof.Handler("block"))
	}
	e.Use(middleware.Recover())
	e.Use(accessLogMiddleware(deps))
	if deps.Config.HTTP.Gzip {
		e.Use(gzipMiddleware())
	}
	e.Use(newRateLimiter(deps.Config.Limits.RateLimit, deps.Metrics).middleware())
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		// S3 上传单独放宽，见 s3UploadBodyLimit。全局 1MB 会让控制台允许的文件先 413，
		// 开发代理再把未读完的请求体报成 500。
		Skipper: skipLargeUploadBodyLimit(deps.S3FileStore != nil, deps.Sandbox != nil),
		Limit:   bodyLimit(deps.Config.Limits.MaxRequestBytes),
	}))

	// /health/live 与 /health/ready 不经认证
	health := &HealthHandler{checker: deps.Health}
	e.GET("/health/live", health.Live)
	e.GET("/health/ready", health.Ready)

	if deps.Metrics != nil {
		reg := deps.MetricsRegistry
		if reg == nil {
			reg = prometheus.DefaultGatherer
		}
		e.GET(deps.Config.Observability.MetricsPath, echo.WrapHandler(promhttp.HandlerFor(reg, promhttp.HandlerOpts{})))
	}

	// login-auth-plan：免认证登录/刷新端点。
	if deps.Sessions != nil && deps.Users != nil {
		ah := NewAuthHandler(deps.Sessions, deps.Users)
		e.POST("/v1/auth/login", ah.Login)
		e.POST("/v1/auth/refresh", ah.Refresh)
	}

	// v1 业务路由组：认证 → project context → 各 handler
	if deps.Auth != nil && deps.DatabaseHandler != nil {
		mountV1Routes(e, deps)
		mountGoRoutes(e, deps)
		mountE2BRoutes(e, deps)
	}

	e.GET("/v1/openapi.json", func(c echo.Context) error {
		return c.JSON(http.StatusOK, buildOpenAPI(e.Routes()))
	})

	// 管理端静态资源：所有未匹配 API 路由的 GET 请求回退到前端 SPA。
	// 必须在 /v1、/health、/metrics 等路由注册后调用，以免拦截 API 请求。
	web.Register(e)

	return e
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
	return auth.AuthMiddlewareWithDelegation(sessions, deps.Auth, WithPrincipal, deps.Config.Auth.APIKeyHashSecret, deps.Delegations)
}

// mountV1Routes 挂载 /v1 路由。
// 中间件顺序：AuthMiddleware（JWT/API Key 双通道）→ projectContext。
// 权限校验通过 auth.Require 在每个路由单独配置。
func mountV1Routes(e *echo.Echo, deps Dependencies) {
	authMW := timedAuthMiddleware(deps.authMW())
	// require 不带 delegable：路由对委托令牌默认拒绝（planv6.0 D1）。
	require := func(perm auth.Permission) echo.MiddlewareFunc {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return policyGate(auth.Require(perm, PrincipalFromContext)(next))
		}
	}
	// delegable 在 require 基础上标记路由可被委托令牌访问（planv6.0 §3.1 表）。
	delegable := func(perm auth.Permission) echo.MiddlewareFunc {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return policyGate(auth.RequireDelegable(perm, PrincipalFromContext, auth.DelegationMark())(next))
		}
	}
	// WR-01：统一写门禁（纵深防御）。只读实例当前不启动系统库、不挂这些
	// 路由；但若未来只读实例也打开系统库，缺少这层门禁的写接口会立即变成
	// 可写漏洞。挂载方式见 writeGateRoutes 驱动测试（防新增写路由漏挂）。
	writeGate := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !deps.Config.Instance.Writable {
				return c.JSON(http.StatusServiceUnavailable, APIErrorBody{Error: APIErrorDetail{
					Code: "readonly_instance", Message: "instance is read-only",
					RequestID: RequestIDFromContext(c.Request().Context()),
				}})
			}
			return next(c)
		}
	}

	v1 := e.Group("/v1", authMW)

	// 登录态自身路由（需认证）。
	if deps.Sessions != nil && deps.Users != nil {
		ah := NewAuthHandler(deps.Sessions, deps.Users)
		v1.POST("/auth/logout", ah.Logout)
		v1.GET("/auth/me", ah.Me)
		v1.PUT("/auth/password", ah.ChangePassword, writeGate)

		uh := NewUsersHandler(deps.Users, deps.Sessions)
		v1.GET("/users", uh.List)
		v1.POST("/users", uh.Create, writeGate)
		v1.GET("/users/:id", uh.Get)
		v1.PATCH("/users/:id", uh.Patch, writeGate)
		v1.DELETE("/users/:id", uh.Delete, writeGate)
	}

	// 项目枚举 / 创建（不挂 :projectID，供全局切换器）
	ph := NewProjectsHandler(deps.Catalog, deps.Users)

	// 存储设置：以 config.yaml 为准，仅 superadminl1 可改（planv6.0 阶段 E）。
	// 修改后需要停服 → ./build.sh reset -y → 重启，不迁移数据。
	if deps.System != nil {
		ssh := &settingsStorageHandler{cfg: deps.Config, path: config.ConfigFilePath(), logger: deps.Logger}
		v1.GET("/system/storage", ssh.GetStorage, require(auth.DatabaseRead))
		v1.PUT("/system/storage", ssh.PutStorage, writeGate)
	}

	v1.GET("/projects", ph.ListProjects, require(auth.DatabaseRead))
	v1.POST("/projects", ph.CreateProject, require(auth.ProjectAdmin), writeGate)
	p := v1.Group("/projects/:projectID", projectContextMiddlewareEcho(deps))
	h := deps.DatabaseHandler
	// 项目 KV（key-value-ducklake-plan §2）：新建项目时同步建 kind=kv 行。
	ph.KV = deps.KVService
	ph.KVInit = deps.KVInit
	ph.Logger = deps.Logger
	p.POST("/databases", h.CreateDatabase, delegable(auth.DatabaseAdmin))
	p.GET("/databases", h.ListDatabases, delegable(auth.DatabaseRead))
	p.GET("/databases/:databaseID", h.GetDatabase, delegable(auth.DatabaseRead))
	p.DELETE("/databases/:databaseID", h.DeleteDatabase, delegable(auth.DatabaseAdmin), writeGate)

	// API Key 管理（签发 / 列表 / 吊销）。登录态角色与 ProjectAdmin Key 均可；
	// 权限位校验在 handler 内（canIssue），此处仅要求基础读权限通过认证链路。
	if deps.Auth != nil && deps.System != nil {
		// SYNC-01：auth 仓储直接写 sys_api_keys，绕过 systemdb 的 notifyWrite；
		// 写成功后显式触发 catalog 同步，防止“吊销是唯一一次写入”时崩溃恢复
		// 后 Key 仍然有效。
		kh := NewAPIKeysHandler(deps.Auth, deps.System.DB(), deps.System.NotifyWrite)
		p.POST("/api-keys", kh.Create, require(auth.DatabaseRead), writeGate)
		p.GET("/api-keys", kh.List, require(auth.DatabaseRead))
		p.DELETE("/api-keys/:keyID", kh.Delete, require(auth.DatabaseRead), writeGate)
	}

	// Plan 6：SQL 执行路由。SQLHandler 为 nil 时不挂载（readonly 实例可仅挂 query）。
	if deps.SQLHandler != nil {
		sh := deps.SQLHandler
		p.POST("/databases/:databaseID/query", sh.Query, delegable(auth.DatabaseRead))
		p.POST("/databases/:databaseID/execute", sh.Execute, delegable(auth.DatabaseWrite), writeGate)
		p.POST("/databases/:databaseID/batch", sh.Batch, delegable(auth.DatabaseWrite), writeGate)
	}

	if deps.SchemaHandler != nil {
		sh := deps.SchemaHandler
		p.GET("/databases/:databaseID/schema", sh.ListSchema, delegable(auth.DatabaseRead))
		p.GET("/databases/:databaseID/schema/tables/:table/rows", sh.ListTableRows, delegable(auth.DatabaseRead))
		p.POST("/databases/:databaseID/schema/tables", sh.CreateTable, delegable(auth.DatabaseWrite), writeGate)
		p.POST("/databases/:databaseID/schema/columns", sh.AddColumn, delegable(auth.DatabaseWrite), writeGate)
	}

	if deps.DataHandler != nil {
		dh := deps.DataHandler
		// Database-scoped routes: must select the given databaseID.
		p.GET("/databases/:databaseID/data/collections", dh.ListCollections, delegable(auth.DatabaseRead))
		p.POST("/databases/:databaseID/data/collections", dh.CreateCollection, delegable(auth.DatabaseWrite), writeGate)
		p.GET("/databases/:databaseID/data/collections/:collection", dh.ListDocuments, delegable(auth.DatabaseRead))
		p.GET("/databases/:databaseID/data/collections/:collection/export", dh.Export, require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/data/collections/:collection/import", dh.Import, delegable(auth.DatabaseWrite), writeGate, middleware.BodyLimit(bodyLimit(deps.Config.Limits.MaxImportBytes)))
		p.POST("/databases/:databaseID/data/collections/:collection/documents", dh.CreateDocument, delegable(auth.DatabaseWrite), writeGate)
		p.POST("/databases/:databaseID/data/collections/:collection/documents/batch", dh.BatchDocuments, delegable(auth.DatabaseWrite), writeGate)
		p.GET("/databases/:databaseID/data/collections/:collection/documents/:id", dh.GetDocument, delegable(auth.DatabaseRead))
		p.PATCH("/databases/:databaseID/data/collections/:collection/documents/:id", dh.PatchDocument, delegable(auth.DatabaseWrite), writeGate)
		p.PUT("/databases/:databaseID/data/collections/:collection/documents/:id", dh.UpdateDocument, delegable(auth.DatabaseWrite), writeGate)
		p.DELETE("/databases/:databaseID/data/collections/:collection/documents/:id", dh.DeleteDocument, delegable(auth.DatabaseWrite), writeGate)
		p.DELETE("/databases/:databaseID/data/collections/:collection", dh.TruncateCollection, delegable(auth.ProjectAdmin), writeGate)
	}
	if deps.CollectionMetaHandler != nil {
		mh := deps.CollectionMetaHandler
		p.POST("/databases/:databaseID/data/collections/meta/dry-run-view", mh.DryRunView, require(auth.ProjectAdmin))
		p.GET("/databases/:databaseID/data/collections/:collection/meta", mh.Get, delegable(auth.DatabaseRead))
		p.PUT("/databases/:databaseID/data/collections/:collection/meta", mh.Put, delegable(auth.DatabaseWrite), writeGate)
		p.DELETE("/databases/:databaseID/data/collections/:collection/meta", mh.Delete, delegable(auth.DatabaseWrite), writeGate)
	}
	if deps.BackupHandler != nil {
		bh := deps.BackupHandler
		p.GET("/backups", bh.List, require(auth.ProjectAdmin))
		p.POST("/backups", bh.Create, require(auth.ProjectAdmin), writeGate)
		p.GET("/backups/:id/download", bh.Download, require(auth.ProjectAdmin))
		p.DELETE("/backups/:id", bh.Delete, require(auth.ProjectAdmin), writeGate)
		p.POST("/backups/:id/restore", bh.Restore, require(auth.ProjectAdmin), writeGate)
	}
	if deps.RealtimeHandler != nil {
		rh := deps.RealtimeHandler
		p.GET("/realtime", rh.Subscribe, require(auth.DatabaseRead))
		p.POST("/realtime/subscriptions", rh.UpdateSubscription, require(auth.DatabaseRead))
	}

	// Key-Value 数据服务（key-value-ducklake-plan §3）：项目级单端点，
	// 读命令要 database:read，写命令要 database:write（handler 内按命令分类拒绝）。
	// KVHandler 为 nil 时不挂载。
	if deps.KVHandler != nil {
		kvh := deps.KVHandler
		p.POST("/kv", kvh.Execute, delegable(auth.DatabaseRead))
	}

	// LLM Gateway：OpenAI 兼容出口（planv6.0 openai-compatible-llm-api-plan）。
	// deps.LLM 为 nil 时不挂载。旧的 /llm/chat、/llm/stream、/llm/providers 已删除。
	//
	// 路径里的两段 v1 都是刻意的：前一段是 SimpleBase 的 API 版本前缀，后一段是
	// OpenAI SDK 约定的资源前缀。SDK 的 base_url 指向 ".../projects/<id>/v1"，
	// 再拼 /chat/completions。请勿合并成单段（TestOpenAIRoutePaths 锁定）。
	//
	// 权限用 llm:invoke 而非 database:read：这是该接口的原始设计意图（plan8.md）。
	// 三个内置角色与 API Key 签发白名单均已包含它，因此是语义对齐而非权限收紧。
	if deps.LLM != nil {
		lh := &LLMHandler{svc: deps.LLM, usage: deps.Usage, audit: deps.Audit, store: deps.System}
		oa := p.Group("/v1")
		oa.POST("/chat/completions", lh.ChatCompletions, require(auth.LLMInvoke))
		oa.POST("/responses", lh.Responses, require(auth.LLMInvoke))
		oa.GET("/models", lh.ListModels, require(auth.LLMInvoke))
	}

	// Plan 9：配额与审计查询路由。
	if deps.Usage != nil {
		qh := &QuotaHandler{svc: deps.Usage}
		p.GET("/quota", qh.GetQuota, delegable(auth.DatabaseRead))
	}
	if deps.Audit != nil {
		ah := &AuditHandler{svc: deps.Audit}
		p.GET("/audit", ah.ListOperations, delegable(auth.DatabaseRead))
	}

	// S3 用户文件存储路由。deps.S3FileStore 为 nil 时不挂载。
	if deps.S3FileStore != nil {
		sh := &S3Handler{store: deps.S3FileStore, index: deps.System, writable: &deps.Config.Instance.Writable, tokenSecret: deps.Config.Files.TokenSecret, allowedMIME: deps.Config.Files.AllowedMIME, events: deps.TriggerEvents}
		p.GET("/s3/objects", sh.ListObjects, delegable(auth.DatabaseRead))
		p.GET("/s3/objects/content", sh.DownloadObject, delegable(auth.DatabaseRead))
		p.POST("/s3/objects", sh.UploadObject, delegable(auth.DatabaseWrite), writeGate, middleware.BodyLimit(bodyLimit(s3UploadBodyLimit)))
		p.DELETE("/s3/objects", sh.DeleteObject, delegable(auth.DatabaseWrite), writeGate)
		p.GET("/s3/presign", sh.PresignObject, delegable(auth.DatabaseRead))
		p.POST("/s3/link", sh.CreateLink, delegable(auth.DatabaseRead))
		e.GET("/files/:token", sh.DownloadLinkedObject)
	}

	if deps.System != nil {
		mh := &metricsHandler{store: deps.System}
		p.GET("/metrics/summary", mh.Summary, require(auth.DatabaseRead))
		p.GET("/metrics/trend", mh.Trend, require(auth.DatabaseRead))

		lh := &logsHTTPHandler{store: deps.System}
		// planv6.0 §3.1：logs 只读可委托；retention 管理（GET/PUT）不可委托。
		p.GET("/logs", lh.List, delegable(auth.DatabaseRead))
		p.GET("/logs/stats", lh.Stats, delegable(auth.DatabaseRead))
		p.GET("/logs/export", lh.Export, delegable(auth.DatabaseRead))
		p.GET("/logs/retention", lh.GetRetention, require(auth.DatabaseRead))
		p.PUT("/logs/retention", lh.PutRetention, require(auth.ProjectAdmin), writeGate)

		seth := &settingsHandler{store: deps.System}
		p.GET("/settings", seth.GetProject, require(auth.DatabaseRead))
		p.PUT("/settings", seth.PutProject, require(auth.ProjectAdmin), writeGate)
		v1.GET("/settings", seth.GetGlobal, require(auth.ProjectAdmin))
		v1.PUT("/settings", seth.PutGlobal, require(auth.ProjectAdmin), writeGate)

		sess := &llmSessionHandler{store: deps.System}
		p.GET("/llm/settings", sess.GetSettings, require(auth.DatabaseRead))
		p.PUT("/llm/settings", sess.PutSettings, require(auth.ProjectAdmin), writeGate)

		// 云 Agent 路由块（B3 提取；含定时执行路由，见 mountAgentRoutes）。
		mountAgentRoutes(p, deps, require, writeGate)

		migrationH := NewMigrationHandler(deps.System)
		p.GET("/migrations/status", migrationH.Status, require(auth.ProjectAdmin))
		p.GET("/migrations/plan", migrationH.Plan, require(auth.ProjectAdmin))

		// 云函数 + 定时任务管理面（B3 提取，见 mountGoFunctionAndCronRoutes）。
		mountGoFunctionAndCronRoutes(p, deps, delegable, writeGate)
	}
}

// mountGoFunctionAndCronRoutes 挂载云函数 + 定时任务管理面路由块
// （原 mountV1Routes System 守卫下尾部连续块，B3 提取；路由、权限、
// writeGate 与注册顺序保持原样）。两块均可委托（delegable）。
// 调用方保证 deps.System != nil。
func mountGoFunctionAndCronRoutes(p *echo.Group, deps Dependencies, delegable func(auth.Permission) echo.MiddlewareFunc, writeGate echo.MiddlewareFunc) {
	// 云函数管理面（ui-gofunction-plan §7.1）。writable=false 时写操作返回 503。
	gh := NewGoFunctionHandler(deps.System, deps.Config.Instance.Writable, deps.Audit, deps.Config.Limits.MaxConcurrentGoRuns).WithRunner(deps.FunctionRunner)
	p.GET("/gofunctions", gh.List, delegable(auth.DatabaseRead))
	p.POST("/gofunctions", gh.Create, delegable(auth.DatabaseWrite), writeGate)
	p.GET("/gofunctions/:name", gh.Get, delegable(auth.DatabaseRead))
	p.PATCH("/gofunctions/:name", gh.Patch, delegable(auth.DatabaseWrite), writeGate)
	p.DELETE("/gofunctions/:name", gh.Delete, delegable(auth.DatabaseWrite), writeGate)
	p.GET("/gofunctions/:name/versions", gh.ListVersions, delegable(auth.DatabaseRead))
	p.POST("/gofunctions/:name/versions", gh.CreateVersion, delegable(auth.DatabaseWrite), writeGate)
	p.GET("/gofunctions/:name/versions/:ver", gh.GetVersion, delegable(auth.DatabaseRead))
	p.POST("/gofunctions/:name/versions/:ver/activate", gh.ActivateVersion, delegable(auth.DatabaseWrite), writeGate)
	p.POST("/gofunctions/:name/versions/:ver/test", gh.TestVersion, delegable(auth.DatabaseWrite), writeGate)

	// 定时任务管理面（ui-cronjob-plan §6）。writable=false 时写操作返回 503。
	cj := NewCronJobHandler(deps.System, deps.Config.Instance.Writable, deps.CronScheduler, deps.Audit).WithUsage(deps.Usage)
	p.GET("/cron-jobs", cj.List, delegable(auth.DatabaseRead))
	p.POST("/cron-jobs", cj.Create, delegable(auth.DatabaseWrite), writeGate)
	p.GET("/cron-jobs/:jobID", cj.Get, delegable(auth.DatabaseRead))
	p.PATCH("/cron-jobs/:jobID", cj.Update, delegable(auth.DatabaseWrite), writeGate)
	p.DELETE("/cron-jobs/:jobID", cj.Delete, delegable(auth.DatabaseWrite), writeGate)
	p.GET("/cron-jobs/:jobID/runs", cj.ListRuns, delegable(auth.DatabaseRead))
	p.POST("/cron-jobs/:jobID/trigger", cj.Trigger, delegable(auth.DatabaseWrite), writeGate)
	if deps.FunctionTriggerHandler != nil {
		th := deps.FunctionTriggerHandler
		p.GET("/function-triggers", th.List, auth.Require(auth.ProjectAdmin, PrincipalFromContext))
		p.POST("/function-triggers", th.Create, auth.Require(auth.ProjectAdmin, PrincipalFromContext), writeGate)
		p.GET("/function-triggers/:id", th.Get, auth.Require(auth.ProjectAdmin, PrincipalFromContext))
		p.PATCH("/function-triggers/:id", th.Update, auth.Require(auth.ProjectAdmin, PrincipalFromContext), writeGate)
		p.DELETE("/function-triggers/:id", th.Delete, auth.Require(auth.ProjectAdmin, PrincipalFromContext), writeGate)
		p.POST("/function-triggers/:id/invoke", th.Invoke, auth.Require(auth.DatabaseWrite, PrincipalFromContext), writeGate)
	}
}

// mountAgentRoutes 挂载云 Agent + 定时执行路由块（原 mountV1Routes 内
// System 守卫下的连续块，B3 提取；路由、权限、writeGate 与注册顺序保持
// 原样）。全部 require（无 delegable）：委托令牌默认拒绝（planv6.0 D1）。
// POST /agent-threads/:threadID/runs 保持 DatabaseRead + writeGate，
// 不得改成 DatabaseWrite（创建 run 属于低门槛操作，SEC-04 只收紧
// cancel/confirm）。调用方保证 deps.System != nil。
func mountAgentRoutes(p *echo.Group, deps Dependencies, require func(auth.Permission) echo.MiddlewareFunc, writeGate echo.MiddlewareFunc) {
	events := newRunEventHub()
	// LIFE-05：hub 自带惰性清理（append 时节流触发 sweep + 总量上限），
	// 无 end 事件的 run 不会永久驻留，也不需要独立后台 goroutine。
	confirms := newConfirmHub()
	if deps.CloudAgent != nil && deps.CloudAgent.Confirm == nil {
		deps.CloudAgent.Confirm = confirms
	}
	ah := &cloudAgentHandler{store: deps.System, runtime: deps.CloudAgent, usage: deps.Usage, audit: deps.Audit, writable: &deps.Config.Instance.Writable, events: events, confirms: confirms}
	if deps.LLM != nil {
		ah.llm = deps.LLM
	}
	p.GET("/agents/modules", ah.ListModules, require(auth.DatabaseRead))
	p.GET("/agents", ah.ListAgents, require(auth.DatabaseRead))
	p.GET("/agents/models", ah.ListAgentModels, require(auth.DatabaseRead))
	p.POST("/agents", ah.CreateAgent, require(auth.DatabaseWrite), writeGate)
	p.GET("/agents/:agentID", ah.GetAgent, require(auth.DatabaseRead))
	p.PATCH("/agents/:agentID", ah.PatchAgent, require(auth.DatabaseWrite), writeGate)
	p.DELETE("/agents/:agentID", ah.DeleteAgent, require(auth.DatabaseWrite), writeGate)
	p.GET("/agent-threads", ah.ListThreads, require(auth.DatabaseRead))
	p.POST("/agent-threads", ah.CreateThread, require(auth.DatabaseWrite), writeGate)
	p.GET("/agent-threads/:threadID", ah.GetThread, require(auth.DatabaseRead))
	p.PATCH("/agent-threads/:threadID", ah.PatchThread, require(auth.DatabaseWrite), writeGate)
	p.DELETE("/agent-threads/:threadID", ah.DeleteThread, require(auth.DatabaseWrite), writeGate)
	p.GET("/agent-threads/:threadID/messages", ah.ListMessages, require(auth.DatabaseRead))
	p.GET("/agent-threads/:threadID/runs", ah.ListThreadRuns, require(auth.DatabaseRead))
	p.POST("/agent-threads/:threadID/runs", ah.CreateRun, require(auth.DatabaseRead), writeGate)
	// SEC-04：取消/确认是破坏性操作（终止他人 run / 放行工具执行），
	// 权限从 DatabaseRead 收紧为 DatabaseWrite；handler 内再校验主体。
	p.GET("/agent-runs/active", ah.ListActiveRuns, require(auth.DatabaseRead))
	p.POST("/agent-runs/:runID/cancel", ah.CancelRun, require(auth.DatabaseWrite), writeGate)
	p.POST("/agent-runs/:runID/confirmations/:callID", ah.ConfirmRun, require(auth.DatabaseWrite), writeGate)
	p.GET("/agent-runs/:runID/events", ah.ReplayEvents, require(auth.DatabaseRead))

	// 旧 Agent 定时执行路由：迁移期只读兼容层，写入与触发返回 410（统一到 /cron-jobs）。
	sch := &agentScheduleHandler{store: deps.System}
	p.GET("/agent-schedules", sch.ListSchedules, require(auth.DatabaseRead))
	p.POST("/agent-schedules", sch.Gone, require(auth.DatabaseWrite), writeGate)
	p.GET("/agent-schedules/:scheduleID", sch.GetSchedule, require(auth.DatabaseRead))
	p.PATCH("/agent-schedules/:scheduleID", sch.Gone, require(auth.DatabaseWrite), writeGate)
	p.DELETE("/agent-schedules/:scheduleID", sch.Gone, require(auth.DatabaseWrite), writeGate)
	p.GET("/agent-schedules/:scheduleID/runs", sch.ListScheduleRuns, require(auth.DatabaseRead))
	p.POST("/agent-schedules/:scheduleID/run", sch.Gone, require(auth.DatabaseWrite), writeGate)
}

// mountE2BRoutes 挂载云沙盒的 E2B 兼容 API。这是沙盒唯一的 HTTP 面，旧的
// /v1/projects/:projectID/sandboxes/** 已整体删除，不保留任何兼容层。
//
//   - 控制面：/e2b/sandboxes、/e2b/v2/sandboxes/**（X-API-Key 或 Bearer 鉴权）；
//   - envd 数据面：根路径（SDK 拼 /files 会丢掉基址里的路径前缀，所以必须在根）与
//     /e2b/envd/**（控制台用）两处挂同一组 handler；
//   - ext：/e2b/ext/**，E2B 没有对应物但控制台需要（能力查询、终端）。
//
// 必须在 web.Register（SPA GET fallback）之前调用。
func mountE2BRoutes(e *echo.Echo, deps Dependencies) {
	sh := NewSandboxHandler(deps.Sandbox, deps.Config.Instance.Writable, deps.Audit, deps.System, deps.SandboxUsage, deps)
	authMW := timedAuthMiddleware(deps.authMW())
	cred := e2bCredentialMiddleware()
	proj := e2bProjectMiddleware(deps)
	available := deps.Sandbox != nil && deps.Sandbox.Available()

	// ext：capabilities 无条件挂载，沙盒不可用时给出原因，控制台据此展示空态。
	ext := e.Group(e2bPrefix+"/ext", cred, authMW, proj)
	ext.GET("/capabilities", sh.Capabilities)
	if !available {
		// 其余 E2B 路径统一返回 503，而不是落进 SPA fallback。
		for _, prefix := range []string{e2bPrefix + "/*"} {
			e.Any(prefix, sh.unavailable)
		}
		return
	}

	// ---- 控制面 ----
	cp := e.Group(e2bPrefix, cred, authMW, proj)
	cp.POST("/sandboxes", sh.Create)
	cp.POST("/v2/sandboxes", sh.Create)
	cp.GET("/sandboxes", sh.List)
	cp.GET("/v2/sandboxes", sh.List)
	cp.GET("/sandboxes/:sandboxID", sh.Get)
	cp.DELETE("/sandboxes/:sandboxID", sh.Delete)
	cp.POST("/sandboxes/:sandboxID/timeout", sh.SetTimeout)
	cp.POST("/sandboxes/:sandboxID/pause", sh.Pause)
	cp.POST("/sandboxes/:sandboxID/resume", sh.Connect)
	cp.POST("/sandboxes/:sandboxID/connect", sh.Connect)
	cp.POST("/v2/sandboxes/:sandboxID/connect", sh.Connect)
	for _, u := range []string{"fork", "snapshots", "logs", "metrics", "network", "refreshes"} {
		cp.Any("/sandboxes/:sandboxID/"+u, sh.Unsupported)
		cp.Any("/v2/sandboxes/:sandboxID/"+u, sh.Unsupported)
	}

	// ---- ext：终端 ----
	if deps.Terminal != nil && deps.Sandbox.Interactive() {
		ext.POST("/sandboxes/:sandboxID/terminal-tickets", sh.TerminalTicket)
		// WebSocket 升级不带 Authorization，靠一次性票据 + 同源校验，所以不经过 authMW。
		e.GET(e2bPrefix+"/ext/sandboxes/:sandboxID/terminal", sh.Terminal)
	}

	// ---- envd 数据面：同一组 handler 挂两处 ----
	envdAuth := sh.envdAuth(func(next echo.HandlerFunc) echo.HandlerFunc { return cred(authMW(next)) })
	mountEnvd := func(g *echo.Group) {
		g.GET("/health", sh.Health)
		g.GET("/envs", sh.Envs)
		g.GET("/metrics", sh.EnvdMetrics)
		g.GET("/files", sh.FilesGet)
		g.POST("/files", sh.FilesPost)
		fs := "/filesystem.Filesystem/"
		g.POST(fs+"Stat", sh.FsStat)
		g.POST(fs+"ListDir", sh.FsListDir)
		g.POST(fs+"MakeDir", sh.FsMakeDir)
		g.POST(fs+"Move", sh.FsMove)
		g.POST(fs+"Remove", sh.FsRemove)
		for _, m := range []string{"WatchDir", "CreateWatcher", "GetWatcherEvents", "RemoveWatcher"} {
			g.POST(fs+m, sh.FsUnimplemented)
		}
		pr := "/process.Process/"
		g.POST(pr+"Start", sh.ProcStart)
		g.POST(pr+"Connect", sh.ProcConnect)
		g.POST(pr+"List", sh.ProcList)
		g.POST(pr+"SendInput", sh.ProcSendInput)
		g.POST(pr+"SendSignal", sh.ProcSendSignal)
		g.POST(pr+"CloseStdin", sh.ProcCloseStdin)
		g.POST(pr+"Update", sh.ProcUpdate)
		g.POST(pr+"StreamInput", sh.ProcStreamInput)
	}
	mountEnvd(e.Group("", envdAuth))
	mountEnvd(e.Group(e2bEnvdPrefix, envdAuth))
	// /e2b/** 下其余未注册的路径：E2B 形状的 404（而不是 SPA 的 index.html）。
	e.Any(e2bPrefix+"/*", sh.notFound)
}

// mountGoRoutes 挂载对外调用面 /go/:projectID/:name/:functionName（§7.2）。
// 必须在 web.Register（SPA GET fallback）之前执行——由 NewRouter 调用顺序保证。
// 与 mountV1Routes 同一依赖守卫，复用认证与 project context 中间件。
func mountGoRoutes(e *echo.Echo, deps Dependencies) {
	if deps.System == nil {
		return
	}
	authMW := deps.authMW()
	delegable := func(perm auth.Permission) echo.MiddlewareFunc {
		return auth.RequireDelegable(perm, PrincipalFromContext, auth.DelegationMark())
	}
	h := NewGoFunctionHandler(deps.System, deps.Config.Instance.Writable, deps.Audit, deps.Config.Limits.MaxConcurrentGoRuns).WithRunner(deps.FunctionRunner)
	goGrp := e.Group("/go/:projectID", authMW, projectContextMiddlewareEcho(deps))
	goGrp.POST("/:name/:functionName", h.Invoke, delegable(auth.DatabaseRead))
	// 405 JSON：防止浏览器 GET 掉进 SPA index.html（验收 G11）；GET 不可委托。
	goGrp.GET("/:name/:functionName", h.MethodNotAllowed, auth.Require(auth.DatabaseRead, PrincipalFromContext))
}

// s3UploadBodyLimit 是对象上传的请求体上限。
// 控制台单文件最大 50MiB（S3Manager MAX_UPLOAD_BYTES），另留 64KiB 给 multipart 帧。
const s3UploadBodyLimit = 50<<20 + 64<<10

// skipS3UploadBodyLimit 让 POST /s3/objects 跳过全局 max_request_bytes，改由路由上的更大上限约束。
func skipS3UploadBodyLimit(enabled bool) middleware.Skipper {
	if !enabled {
		return middleware.DefaultSkipper
	}
	const suffix = "/s3/objects"
	return func(c echo.Context) bool {
		r := c.Request()
		if r.Method != http.MethodPost || r.URL == nil {
			return false
		}
		p := r.URL.Path
		return len(p) >= len(suffix) && p[len(p)-len(suffix):] == suffix
	}
}

// skipLargeUploadBodyLimit 允许沙盒文件上传（envd POST /files）使用自己的上限，而不是全局 max_request_bytes。
func skipLargeUploadBodyLimit(s3Enabled, sandboxEnabled bool) middleware.Skipper {
	s3 := skipS3UploadBodyLimit(s3Enabled)
	return func(c echo.Context) bool {
		if s3(c) {
			return true
		}
		if c.Request().URL == nil || c.Request().Method != http.MethodPost {
			return false
		}
		p := c.Request().URL.Path
		if strings.HasSuffix(p, "/import") {
			return true
		}
		return sandboxEnabled && (p == "/files" || p == e2bEnvdPrefix+"/files")
	}
}

// bodyLimit 将字节数转换为 echo BodyLimit 字符串（K/M）。
// BodyLimit 解析支持 1024 形式与 "1M"/"512K" 形式；这里输出后者。
func bodyLimit(bytes int64) string {
	if bytes <= 0 {
		return "1M"
	}
	const k = 1 << 10
	const m = 1 << 20
	if bytes%m == 0 {
		return strconv.Itoa(int(bytes/m)) + "M"
	}
	if bytes%k == 0 {
		return strconv.Itoa(int(bytes/k)) + "K"
	}
	return strconv.FormatInt(bytes, 10)
}

func requestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			rid := req.Header.Get("X-Request-ID")
			if rid == "" || !isValidRequestID(rid) {
				rid = uuid.NewString()
			}
			c.Response().Header().Set("X-Request-ID", rid)
			ctx := WithRequestID(req.Context(), rid)
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}

// isValidRequestID 仅允许 UUID 或 [a-zA-Z0-9_-]{8,64}，避免日志注入。
func isValidRequestID(s string) bool {
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// shouldRecordRequestLog 访问日志选择性记录（perf §1 P1-A：日志页不再是全量 access log）：
//   - 跳过：健康检查、Prometheus 指标、SPA 静态回退、日志查询接口自身；
//   - 保留：status ≥ 400、耗时 ≥ 500ms 的慢请求、所有写方法（POST/PUT/PATCH/DELETE）。
//
// 指标（RecordMetric）口径不受影响，仍全量记录。
func shouldRecordRequestLog(method, route string, status int, latency time.Duration, metricsPath string) bool {
	if route == "" || route == "/*" || strings.HasPrefix(route, "/health/") {
		return false
	}
	if metricsPath == "" {
		metricsPath = "/metrics"
	}
	if route == metricsPath {
		return false
	}
	// 日志页自身（列表 / retention）不产生日志，避免自我放大。
	if strings.HasPrefix(route, "/v1/projects/:id/logs") {
		return false
	}
	if status >= 400 || latency >= 500*time.Millisecond {
		return true
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func accessLogMiddleware(deps Dependencies) echo.MiddlewareFunc {
	logger := deps.Logger
	metricsPath := deps.Config.Observability.MetricsPath
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			latency := time.Since(start)
			rid := RequestIDFromContext(c.Request().Context())
			status := c.Response().Status
			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					status = he.Code
				} else {
					status = http.StatusInternalServerError
				}
			}
			if logger != nil {
				logger.Info("http",
					fieldString("request_id", rid),
					fieldString("method", c.Request().Method),
					fieldString("route", c.Path()),
					fieldString("status", strconv.Itoa(status)),
					fieldDuration("duration", latency),
				)
			}
			if deps.Metrics != nil {
				deps.Metrics.HTTPRequests.WithLabelValues(c.Path(), c.Request().Method, strconv.Itoa(status)).Inc()
				deps.Metrics.HTTPRequestDuration.WithLabelValues(c.Path(), c.Request().Method).Observe(latency.Seconds())
			}
			if deps.System != nil {
				pc, _ := ProjectFromContext(c.Request().Context())
				projectID := ""
				if pc.ID != "" {
					projectID = pc.ID
				}
				if shouldRecordRequestLog(c.Request().Method, c.Path(), status, latency, metricsPath) {
					deps.System.RecordLog(systemdb.LogEvent{
						ProjectID:  projectID,
						Level:      "info",
						Logger:     "http",
						Message:    c.Request().Method + " " + c.Path(),
						FieldsJSON: `{"status":` + strconv.Itoa(status) + `,"duration_ms":` + strconv.Itoa(int(latency.Milliseconds())) + `}`,
						RequestID:  rid,
					})
				}
				if projectID != "" {
					deps.System.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_requests", Value: 1})
					deps.System.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_latency_ms", Value: float64(latency.Microseconds()) / 1000})
					if status >= 500 {
						deps.System.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_errors", Value: 1})
					}
				}
			}
			return err
		}
	}
}

func errorHandler(deps Dependencies) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		rid := RequestIDFromContext(c.Request().Context())
		// E2B 兼容面（含认证 / BodyLimit 等中间件产生的错误）统一用 E2B 的错误体。
		if isE2BPath(c.Request().URL.Path) {
			_ = writeE2BErr(c, err)
			return
		}
		// 业务错误统一通过 WriteError 写入；若 handler 未处理，此处兜底。
		if apiErr := mapEchoError(err, rid); apiErr != nil {
			if cErr := c.JSON(apiErr.HTTPStatus, apiErr.Body); cErr != nil && deps.Logger != nil {
				deps.Logger.Error("write error", fieldString("err", cErr.Error()))
			}
			return
		}
		if he, ok := err.(*echo.HTTPError); ok {
			if cErr := c.JSON(he.Code, map[string]any{
				"error": map[string]any{
					"code":       "http_error",
					"message":    safeMessage(he),
					"request_id": rid,
				},
			}); cErr != nil && deps.Logger != nil {
				deps.Logger.Error("write http error", fieldString("err", cErr.Error()))
			}
			return
		}
		if deps.Logger != nil {
			deps.Logger.Error("unhandled error",
				fieldString("request_id", rid),
				fieldString("err", err.Error()),
			)
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]any{
			"error": map[string]any{
				"code":       "internal_error",
				"message":    "internal error",
				"request_id": rid,
			},
		})
	}
}

func safeMessage(he *echo.HTTPError) string {
	if he.Message == nil {
		return "error"
	}
	if s, ok := he.Message.(string); ok {
		return s
	}
	return "error"
}
