// Package api 提供 SimpleBase 的 v1 HTTP API。
// 路由、中间件与错误协议在本包内组装；业务 handler 由各 plan 逐步挂载。
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/cloudagent"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/cronjob"
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
	SQLHandler  *SQLHandler
	DataHandler *DataHandler
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
	CloudAgent *cloudagent.Runtime
	// AgentScheduler 是 Cloud Agent 定时执行调度器。
	AgentScheduler *cloudagent.Scheduler
	// CronScheduler 是云函数定时任务调度器；nil 时 trigger 返回 503。
	CronScheduler *cronjob.Scheduler
	// Sandbox 是项目级云沙盒服务；未配置时仅 capabilities 路由可用。
	Sandbox      SandboxService
	SandboxUsage interface {
		RecordSandbox(context.Context, string, string, int64) error
	}
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
	// Tools 非空时走原生 function calling（云 Agent 使用）。
	Tools []LLMTool
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
	// 以下字段仅供内部（云 Agent）使用，不出现在 /llm/stream 的 JSON 中。
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

	e.Use(requestIDMiddleware())
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
	}

	// 管理端静态资源：所有未匹配 API 路由的 GET 请求回退到前端 SPA。
	// 必须在 /v1、/health、/metrics 等路由注册后调用，以免拦截 API 请求。
	web.Register(e)

	return e
}

// mountV1Routes 挂载 /v1 路由。
// 中间件顺序：AuthMiddleware（JWT/API Key 双通道）→ projectContext。
// 权限校验通过 auth.Require 在每个路由单独配置。
func mountV1Routes(e *echo.Echo, deps Dependencies) {
	authMW := timedAuthMiddleware(auth.AuthMiddlewareWithDelegation(deps.Sessions, deps.Auth, WithPrincipal, deps.Config.Auth.APIKeyHashSecret, deps.Delegations))
	require := func(perm auth.Permission) echo.MiddlewareFunc {
		return auth.Require(perm, PrincipalFromContext)
	}

	v1 := e.Group("/v1", authMW)

	// 登录态自身路由（需认证）。
	if deps.Sessions != nil && deps.Users != nil {
		ah := NewAuthHandler(deps.Sessions, deps.Users)
		v1.POST("/auth/logout", ah.Logout)
		v1.GET("/auth/me", ah.Me)
		v1.PUT("/auth/password", ah.ChangePassword)

		uh := NewUsersHandler(deps.Users, deps.Sessions)
		v1.GET("/users", uh.List)
		v1.POST("/users", uh.Create)
		v1.GET("/users/:id", uh.Get)
		v1.PATCH("/users/:id", uh.Patch)
		v1.DELETE("/users/:id", uh.Delete)
	}

	// 项目枚举 / 创建（不挂 :projectID，供全局切换器）
	ph := NewProjectsHandler(deps.Catalog, deps.Users)
	v1.GET("/projects", ph.ListProjects, require(auth.DatabaseRead))
	v1.POST("/projects", ph.CreateProject, require(auth.ProjectAdmin))
	p := v1.Group("/projects/:projectID", projectContextMiddlewareEcho(deps))
	h := deps.DatabaseHandler
	// 项目 KV（key-value-ducklake-plan §2）：新建项目时同步建 kind=kv 行。
	ph.KV = deps.KVService
	ph.KVInit = deps.KVInit
	ph.Logger = deps.Logger
	p.POST("/databases", h.CreateDatabase, require(auth.DatabaseAdmin))
	p.GET("/databases", h.ListDatabases, require(auth.DatabaseRead))
	p.GET("/databases/:databaseID", h.GetDatabase, require(auth.DatabaseRead))
	p.DELETE("/databases/:databaseID", h.DeleteDatabase, require(auth.DatabaseAdmin))

	// API Key 管理（签发 / 列表 / 吊销）。登录态角色与 ProjectAdmin Key 均可；
	// 权限位校验在 handler 内（canIssue），此处仅要求基础读权限通过认证链路。
	if deps.Auth != nil && deps.System != nil {
		kh := NewAPIKeysHandler(deps.Auth, deps.System.DB())
		p.POST("/api-keys", kh.Create, require(auth.DatabaseRead))
		p.GET("/api-keys", kh.List, require(auth.DatabaseRead))
		p.DELETE("/api-keys/:keyID", kh.Delete, require(auth.DatabaseRead))
	}

	// Plan 6：SQL 执行路由。SQLHandler 为 nil 时不挂载（readonly 实例可仅挂 query）。
	if deps.SQLHandler != nil {
		sh := deps.SQLHandler
		p.POST("/databases/:databaseID/query", sh.Query, require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/execute", sh.Execute, require(auth.DatabaseWrite))
		p.POST("/databases/:databaseID/batch", sh.Batch, require(auth.DatabaseWrite))
	}

	if deps.SchemaHandler != nil {
		sh := deps.SchemaHandler
		p.GET("/databases/:databaseID/schema", sh.ListSchema, require(auth.DatabaseRead))
		p.GET("/databases/:databaseID/schema/tables/:table/rows", sh.ListTableRows, require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/schema/tables", sh.CreateTable, require(auth.DatabaseWrite))
		p.POST("/databases/:databaseID/schema/columns", sh.AddColumn, require(auth.DatabaseWrite))
	}

	if deps.DataHandler != nil {
		dh := deps.DataHandler
		// Database-scoped routes: must select the given databaseID.
		p.GET("/databases/:databaseID/data/collections", dh.ListCollections, require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/data/collections", dh.CreateCollection, require(auth.DatabaseWrite))
		p.GET("/databases/:databaseID/data/collections/:collection", dh.ListDocuments, require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/data/collections/:collection/documents", dh.CreateDocument, require(auth.DatabaseWrite))
		p.PUT("/databases/:databaseID/data/collections/:collection/documents/:id", dh.UpdateDocument, require(auth.DatabaseWrite))
		p.DELETE("/databases/:databaseID/data/collections/:collection/documents/:id", dh.DeleteDocument, require(auth.DatabaseWrite))
	}

	// Key-Value 数据服务（key-value-ducklake-plan §3）：项目级单端点，
	// 读命令要 database:read，写命令要 database:write（handler 内按命令分类拒绝）。
	// KVHandler 为 nil 时不挂载。
	if deps.KVHandler != nil {
		kvh := deps.KVHandler
		p.POST("/kv", kvh.Execute, require(auth.DatabaseRead))
	}

	// Plan 8：LLM Gateway 路由。deps.LLM 为 nil 时不挂载。
	if deps.LLM != nil {
		lh := &LLMHandler{svc: deps.LLM, usage: deps.Usage, audit: deps.Audit, store: deps.System}
		p.POST("/llm/chat", lh.Chat, require(auth.DatabaseRead))
		p.POST("/llm/stream", lh.Stream, require(auth.DatabaseRead))
		p.GET("/llm/providers", lh.ListProviders, require(auth.DatabaseRead))
	}

	// Plan 9：配额与审计查询路由。
	if deps.Usage != nil {
		qh := &QuotaHandler{svc: deps.Usage}
		p.GET("/quota", qh.GetQuota, require(auth.DatabaseRead))
	}
	if deps.Audit != nil {
		ah := &AuditHandler{svc: deps.Audit}
		p.GET("/audit", ah.ListOperations, require(auth.DatabaseRead))
	}

	// S3 用户文件存储路由。deps.S3FileStore 为 nil 时不挂载。
	if deps.S3FileStore != nil {
		sh := &S3Handler{store: deps.S3FileStore, index: deps.System, writable: &deps.Config.Instance.Writable}
		p.GET("/s3/objects", sh.ListObjects, require(auth.DatabaseRead))
		p.POST("/s3/objects", sh.UploadObject, require(auth.DatabaseWrite), middleware.BodyLimit(bodyLimit(s3UploadBodyLimit)))
		p.DELETE("/s3/objects", sh.DeleteObject, require(auth.DatabaseWrite))
		p.GET("/s3/presign", sh.PresignObject, require(auth.DatabaseRead))
	}

	if deps.System != nil {
		mh := &metricsHandler{store: deps.System}
		p.GET("/metrics/summary", mh.Summary, require(auth.DatabaseRead))
		p.GET("/metrics/trend", mh.Trend, require(auth.DatabaseRead))

		lh := &logsHTTPHandler{store: deps.System}
		p.GET("/logs", lh.List, require(auth.DatabaseRead))
		p.GET("/logs/retention", lh.GetRetention, require(auth.DatabaseRead))
		p.PUT("/logs/retention", lh.PutRetention, require(auth.ProjectAdmin))

		seth := &settingsHandler{store: deps.System}
		p.GET("/settings", seth.GetProject, require(auth.DatabaseRead))
		p.PUT("/settings", seth.PutProject, require(auth.ProjectAdmin))
		v1.GET("/settings", seth.GetGlobal, require(auth.ProjectAdmin))
		v1.PUT("/settings", seth.PutGlobal, require(auth.ProjectAdmin))

		sess := &llmSessionHandler{store: deps.System}
		p.GET("/llm/settings", sess.GetSettings, require(auth.DatabaseRead))
		p.PUT("/llm/settings", sess.PutSettings, require(auth.ProjectAdmin))

		credh := &llmProviderCredHandler{store: deps.System}
		p.GET("/llm/providers", credh.ListProviderCreds, require(auth.DatabaseRead))
		p.PUT("/llm/providers/:provider", credh.PutProviderCred, require(auth.ProjectAdmin))
		p.DELETE("/llm/providers/:provider", credh.DeleteProviderCred, require(auth.ProjectAdmin))

		events := newRunEventHub()
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
		p.POST("/agents", ah.CreateAgent, require(auth.DatabaseWrite))
		p.GET("/agents/:agentID", ah.GetAgent, require(auth.DatabaseRead))
		p.PATCH("/agents/:agentID", ah.PatchAgent, require(auth.DatabaseWrite))
		p.DELETE("/agents/:agentID", ah.DeleteAgent, require(auth.DatabaseWrite))
		p.GET("/agent-threads", ah.ListThreads, require(auth.DatabaseRead))
		p.POST("/agent-threads", ah.CreateThread, require(auth.DatabaseWrite))
		p.GET("/agent-threads/:threadID", ah.GetThread, require(auth.DatabaseRead))
		p.PATCH("/agent-threads/:threadID", ah.PatchThread, require(auth.DatabaseWrite))
		p.DELETE("/agent-threads/:threadID", ah.DeleteThread, require(auth.DatabaseWrite))
		p.GET("/agent-threads/:threadID/messages", ah.ListMessages, require(auth.DatabaseRead))
		p.GET("/agent-threads/:threadID/runs", ah.ListThreadRuns, require(auth.DatabaseRead))
		p.POST("/agent-threads/:threadID/runs", ah.CreateRun, require(auth.DatabaseRead))
		p.POST("/agent-runs/:runID/cancel", ah.CancelRun, require(auth.DatabaseRead))
		p.POST("/agent-runs/:runID/confirmations/:callID", ah.ConfirmRun, require(auth.DatabaseRead))
		p.GET("/agent-runs/:runID/events", ah.ReplayEvents, require(auth.DatabaseRead))

		// Cloud Agent 定时执行路由。
		sch := &agentScheduleHandler{store: deps.System, scheduler: deps.AgentScheduler, usage: deps.Usage, writable: &deps.Config.Instance.Writable}
		p.GET("/agent-schedules", sch.ListSchedules, require(auth.DatabaseRead))
		p.POST("/agent-schedules", sch.CreateSchedule, require(auth.DatabaseWrite))
		p.GET("/agent-schedules/:scheduleID", sch.GetSchedule, require(auth.DatabaseRead))
		p.PATCH("/agent-schedules/:scheduleID", sch.PatchSchedule, require(auth.DatabaseWrite))
		p.DELETE("/agent-schedules/:scheduleID", sch.DeleteSchedule, require(auth.DatabaseWrite))
		p.GET("/agent-schedules/:scheduleID/runs", sch.ListScheduleRuns, require(auth.DatabaseRead))
		if deps.AgentScheduler != nil {
			p.POST("/agent-schedules/:scheduleID/run", sch.TriggerScheduleRun, require(auth.DatabaseRead))
		}

		// 云函数管理面（ui-gofunction-plan §7.1）。writable=false 时写操作返回 503。
		gh := NewGoFunctionHandler(deps.System, deps.Config.Instance.Writable, deps.Audit)
		p.GET("/gofunctions", gh.List, require(auth.DatabaseRead))
		p.POST("/gofunctions", gh.Create, require(auth.DatabaseWrite))
		p.GET("/gofunctions/:name", gh.Get, require(auth.DatabaseRead))
		p.PATCH("/gofunctions/:name", gh.Patch, require(auth.DatabaseWrite))
		p.DELETE("/gofunctions/:name", gh.Delete, require(auth.DatabaseWrite))
		p.GET("/gofunctions/:name/versions", gh.ListVersions, require(auth.DatabaseRead))
		p.POST("/gofunctions/:name/versions", gh.CreateVersion, require(auth.DatabaseWrite))
		p.GET("/gofunctions/:name/versions/:ver", gh.GetVersion, require(auth.DatabaseRead))
		p.POST("/gofunctions/:name/versions/:ver/activate", gh.ActivateVersion, require(auth.DatabaseWrite))
		p.POST("/gofunctions/:name/versions/:ver/test", gh.TestVersion, require(auth.DatabaseWrite))

		// 定时任务管理面（ui-cronjob-plan §6）。writable=false 时写操作返回 503。
		cj := NewCronJobHandler(deps.System, deps.Config.Instance.Writable, deps.CronScheduler, deps.Audit)
		p.GET("/cron-jobs", cj.List, require(auth.DatabaseRead))
		p.POST("/cron-jobs", cj.Create, require(auth.DatabaseWrite))
		p.GET("/cron-jobs/:jobID", cj.Get, require(auth.DatabaseRead))
		p.PATCH("/cron-jobs/:jobID", cj.Update, require(auth.DatabaseWrite))
		p.DELETE("/cron-jobs/:jobID", cj.Delete, require(auth.DatabaseWrite))
		p.GET("/cron-jobs/:jobID/runs", cj.ListRuns, require(auth.DatabaseRead))
		p.POST("/cron-jobs/:jobID/trigger", cj.Trigger, require(auth.DatabaseWrite))
	}

	// 云沙盒项目接口。未启用时保留 capabilities，供控制台展示配置空态。
	sh := NewSandboxHandler(deps.Sandbox, deps.Config.Instance.Writable, deps.Audit, deps.System, deps.SandboxUsage)
	p.GET("/sandboxes/capabilities", sh.Capabilities, require(auth.DatabaseRead))
	if deps.Sandbox != nil && deps.Sandbox.Available() {
		p.GET("/sandboxes", sh.List, require(auth.DatabaseRead))
		p.POST("/sandboxes", sh.Create, require(auth.DatabaseWrite))
		p.POST("/sandboxes/run", sh.RunOnce, require(auth.DatabaseWrite), middleware.BodyLimit(bodyLimit(int64(deps.Config.Sandbox.MaxFileBytes+64<<10))))
		p.GET("/sandboxes/:sandboxID", sh.Get, require(auth.DatabaseRead))
		p.PATCH("/sandboxes/:sandboxID", sh.Update, require(auth.DatabaseWrite))
		p.DELETE("/sandboxes/:sandboxID", sh.Delete, require(auth.DatabaseWrite))
		p.POST("/sandboxes/:sandboxID/start", sh.Start, require(auth.DatabaseWrite))
		p.POST("/sandboxes/:sandboxID/stop", sh.Stop, require(auth.DatabaseWrite))
		p.POST("/sandboxes/:sandboxID/exec", sh.Exec, require(auth.DatabaseWrite))
		p.GET("/sandboxes/:sandboxID/files", sh.ListDir, require(auth.DatabaseRead))
		p.GET("/sandboxes/:sandboxID/files/content", sh.ReadFile, require(auth.DatabaseRead))
		p.PUT("/sandboxes/:sandboxID/files/content", sh.WriteFile, require(auth.DatabaseWrite), middleware.BodyLimit(bodyLimit(int64(deps.Config.Sandbox.MaxFileBytes+64<<10))))
		p.DELETE("/sandboxes/:sandboxID/files/content", sh.RemoveFile, require(auth.DatabaseWrite))
	}
}

// mountGoRoutes 挂载对外调用面 /go/:projectID/:name/:functionName（§7.2）。
// 必须在 web.Register（SPA GET fallback）之前执行——由 NewRouter 调用顺序保证。
// 与 mountV1Routes 同一依赖守卫，复用认证与 project context 中间件。
func mountGoRoutes(e *echo.Echo, deps Dependencies) {
	if deps.System == nil {
		return
	}
	authMW := auth.AuthMiddlewareWithDelegation(deps.Sessions, deps.Auth, WithPrincipal, deps.Config.Auth.APIKeyHashSecret, deps.Delegations)
	require := func(perm auth.Permission) echo.MiddlewareFunc {
		return auth.Require(perm, PrincipalFromContext)
	}
	h := NewGoFunctionHandler(deps.System, deps.Config.Instance.Writable, deps.Audit)
	goGrp := e.Group("/go/:projectID", authMW, projectContextMiddlewareEcho(deps))
	goGrp.POST("/:name/:functionName", h.Invoke, require(auth.DatabaseRead))
	// 405 JSON：防止浏览器 GET 掉进 SPA index.html（验收 G11）
	goGrp.GET("/:name/:functionName", h.MethodNotAllowed, require(auth.DatabaseRead))
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

// skipLargeUploadBodyLimit 允许沙盒文件与一次性运行请求使用各自独立的上限。
func skipLargeUploadBodyLimit(s3Enabled, sandboxEnabled bool) middleware.Skipper {
	s3 := skipS3UploadBodyLimit(s3Enabled)
	return func(c echo.Context) bool {
		if s3(c) {
			return true
		}
		if !sandboxEnabled || c.Request().URL == nil {
			return false
		}
		p := c.Request().URL.Path
		return strings.Contains(p, "/sandboxes/") && ((c.Request().Method == http.MethodPut && strings.HasSuffix(p, "/files/content")) ||
			(c.Request().Method == http.MethodPost && strings.HasSuffix(p, "/sandboxes/run")))
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
		return itoa(int(bytes/m)) + "M"
	}
	if bytes%k == 0 {
		return itoa(int(bytes/k)) + "K"
	}
	// 向上取整到 KB，确保不超出限制语义。
	kb := (bytes + k - 1) / k
	return itoa(int(kb)) + "K"
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
					fieldString("status", itoa(status)),
					fieldDuration("duration", latency),
				)
			}
			if deps.Metrics != nil {
				deps.Metrics.HTTPRequests.WithLabelValues(c.Path(), c.Request().Method, itoa(status)).Inc()
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
						FieldsJSON: `{"status":` + itoa(status) + `,"duration_ms":` + itoa(int(latency.Milliseconds())) + `}`,
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
