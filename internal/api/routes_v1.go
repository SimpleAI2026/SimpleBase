// 文件说明：routes_v1.go — /v1 业务路由挂载（按资源拆分）
//
// 用法
//
//	mountV1Routes(e, deps)  // 仅在 deps.Auth 与 deps.DatabaseHandler 非 nil 时由 NewRouter 调用
//
// 调用架构
//
//	NewRouter（router.go）
//	  → mountV1Routes：建 routeGuards 与 /v1、/v1/projects/:projectID 两个 group，按序调用
//	      mountAuthRoutes → mountProjectRoutes → mountDatabaseRoutes → mountBackupRealtimeRoutes
//	      → mountKVRoutes → mountLLMRoutes → mountUsageAuditRoutes → mountS3Routes → mountSystemRoutes
//	  mountSystemRoutes 内再调 mountAgentRoutes / mountGoFunctionAndCronRoutes（见 routes_agent.go、routes_function.go）
//
//	注册顺序即 echo 路由匹配与 buildOpenAPI(e.Routes()) 的输出顺序，调整前先跑路由清单对比。
//
// 主要符号
//
//	func mountV1Routes(e *echo.Echo, deps Dependencies)
//	func mountAuthRoutes / mountProjectRoutes / mountDatabaseRoutes / mountBackupRealtimeRoutes
//	func mountKVRoutes / mountLLMRoutes / mountUsageAuditRoutes / mountS3Routes / mountSystemRoutes

package api

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/linkxzhou/SimpleBase/internal/api/filehttp"
	"github.com/linkxzhou/SimpleBase/internal/api/iamhttp"
	"github.com/linkxzhou/SimpleBase/internal/api/llmhttp"
	"github.com/linkxzhou/SimpleBase/internal/api/opshttp"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// mountV1Routes 挂载 /v1 路由。
// 中间件顺序：AuthMiddleware（JWT/API Key 双通道）→ projectContext。
// 权限校验通过 routeGuards 在每个路由单独配置。
func mountV1Routes(e *echo.Echo, deps Dependencies) {
	g := newRouteGuards(deps)
	authMW := timedAuthMiddleware(deps.authMW())
	// §8.1.6：post_auth 限流（authMW 之后才能拿到 principal）。
	v1 := e.Group("/v1", authMW, newRateLimiter(deps.Config.Limits.RateLimit, deps.Metrics).middleware(postAuthStage))

	mountAuthRoutes(v1, deps, g)
	ph := NewProjectsHandler(deps.Catalog, deps.Users)
	mountProjectListRoutes(v1, deps, g, ph)
	p := v1.Group("/projects/:projectID", projectContextMiddlewareEcho(deps.Catalog))
	mountDatabaseRoutes(p, deps, g, ph)
	mountBackupRealtimeRoutes(p, deps, g)
	mountKVRoutes(p, deps, g)
	mountLLMRoutes(p, deps, g)
	mountUsageAuditRoutes(p, deps, g)
	mountS3Routes(e, p, deps, g)
	mountSystemRoutes(e, v1, p, deps, g)
}

// mountAuthRoutes 挂载登录态自身与用户管理路由（需认证）。
func mountAuthRoutes(v1 *echo.Group, deps Dependencies, g routeGuards) {
	if deps.Sessions == nil || deps.Users == nil {
		return
	}
	ah := iamhttp.NewAuthHandler(deps.Sessions, deps.Users).WithAudit(deps.Audit)
	v1.POST("/auth/logout", ah.Logout)
	v1.GET("/auth/me", ah.Me)
	v1.PUT("/auth/password", ah.ChangePassword, g.writeGate)

	uh := iamhttp.NewUsersHandler(deps.Users, deps.Sessions)
	v1.GET("/users", uh.List)
	v1.POST("/users", uh.Create, g.writeGate)
	v1.GET("/users/:id", uh.Get)
	v1.PATCH("/users/:id", uh.Patch, g.writeGate)
	v1.DELETE("/users/:id", uh.Delete, g.writeGate)
}

// mountProjectListRoutes 挂载项目枚举 / 创建（不挂 :projectID，供全局切换器）与存储设置。
func mountProjectListRoutes(v1 *echo.Group, deps Dependencies, g routeGuards, ph *ProjectsHandler) {
	// 存储设置：以 config.yaml 为准，仅 superadminl1 可改（planv6.0 阶段 E）。
	// 修改后需要停服 → ./build.sh reset -y → 重启，不迁移数据。
	if deps.System != nil {
		ssh := opshttp.NewStorageSettingsHandler(deps.Config, config.ConfigFilePath(), deps.Logger)
		v1.GET("/system/storage", ssh.GetStorage, g.require(auth.DatabaseRead))
		v1.PUT("/system/storage", ssh.PutStorage, g.writeGate)
	}

	v1.GET("/projects", ph.ListProjects, g.require(auth.DatabaseRead))
	v1.POST("/projects", ph.CreateProject, g.require(auth.ProjectAdmin), g.writeGate)
}

// mountDatabaseRoutes 挂载库 CRUD、API Key、SQL、schema、集合数据与集合元数据路由。
func mountDatabaseRoutes(p *echo.Group, deps Dependencies, g routeGuards, ph *ProjectsHandler) {
	h := deps.DatabaseHandler
	// 项目 KV（key-value-ducklake-plan §2）：新建项目时同步建 kind=kv 行。
	ph.KV = deps.KVService
	ph.KVInit = deps.KVInit
	ph.Logger = deps.Logger
	p.POST("/databases", h.CreateDatabase, g.delegable(auth.DatabaseAdmin))
	p.GET("/databases", h.ListDatabases, g.delegable(auth.DatabaseRead))
	p.GET("/databases/:databaseID", h.GetDatabase, g.delegable(auth.DatabaseRead))
	p.DELETE("/databases/:databaseID", h.DeleteDatabase, g.delegable(auth.DatabaseAdmin), g.writeGate)

	// API Key 管理（签发 / 列表 / 吊销）。登录态角色与 ProjectAdmin Key 均可；
	// 权限位校验在 handler 内（canIssue），此处仅要求基础读权限通过认证链路。
	if deps.Auth != nil && deps.System != nil {
		// SYNC-01：auth 仓储直接写 sys_api_keys，绕过 systemdb 的 notifyWrite；
		// 写成功后显式触发 catalog 同步，防止“吊销是唯一一次写入”时崩溃恢复
		// 后 Key 仍然有效。
		kh := iamhttp.NewAPIKeysHandler(deps.Auth, deps.System.DB(), deps.System.NotifyWrite)
		p.POST("/api-keys", kh.Create, g.require(auth.DatabaseRead), g.writeGate)
		p.GET("/api-keys", kh.List, g.require(auth.DatabaseRead))
		p.DELETE("/api-keys/:keyID", kh.Delete, g.require(auth.DatabaseRead), g.writeGate)
	}

	// Plan 6：SQL 执行路由。SQLHandler 为 nil 时不挂载（readonly 实例可仅挂 query）。
	if deps.SQLHandler != nil {
		sh := deps.SQLHandler
		p.POST("/databases/:databaseID/query", sh.Query, g.delegable(auth.DatabaseRead))
		p.POST("/databases/:databaseID/execute", sh.Execute, g.delegable(auth.DatabaseWrite), g.writeGate)
		p.POST("/databases/:databaseID/batch", sh.Batch, g.delegable(auth.DatabaseWrite), g.writeGate)
	}

	if deps.SchemaHandler != nil {
		sh := deps.SchemaHandler
		p.GET("/databases/:databaseID/schema", sh.ListSchema, g.delegable(auth.DatabaseRead))
		p.GET("/databases/:databaseID/schema/tables/:table/rows", sh.ListTableRows, g.delegable(auth.DatabaseRead))
		p.GET("/databases/:databaseID/schema/tables/:table/aggregate", sh.AggregateTable, g.require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/schema/tables", sh.CreateTable, g.delegable(auth.DatabaseWrite), g.writeGate)
		p.POST("/databases/:databaseID/schema/columns", sh.AddColumn, g.delegable(auth.DatabaseWrite), g.writeGate)
	}

	if deps.DataHandler != nil {
		dh := deps.DataHandler
		// Database-scoped routes: must select the given databaseID.
		p.GET("/databases/:databaseID/data/collections", dh.ListCollections, g.delegable(auth.DatabaseRead))
		p.POST("/databases/:databaseID/data/collections", dh.CreateCollection, g.delegable(auth.DatabaseWrite), g.writeGate)
		p.GET("/databases/:databaseID/data/collections/:collection", dh.ListDocuments, g.delegable(auth.DatabaseRead))
		p.GET("/databases/:databaseID/data/collections/:collection/export", dh.Export, g.require(auth.DatabaseRead))
		// 聚合只读：require 而非 delegable，不扩大云助手可委托路由集合（planv6.7 §2.1）。
		p.GET("/databases/:databaseID/data/collections/:collection/aggregate", dh.Aggregate, g.require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/data/collections/:collection/import", dh.Import, g.delegable(auth.DatabaseWrite), g.writeGate, middleware.BodyLimit(bodyLimit(deps.Config.Limits.MaxImportBytes)))
		p.POST("/databases/:databaseID/data/collections/:collection/documents", dh.CreateDocument, g.delegable(auth.DatabaseWrite), g.writeGate)
		p.POST("/databases/:databaseID/data/collections/:collection/documents/batch", dh.BatchDocuments, g.delegable(auth.DatabaseWrite), g.writeGate)
		p.GET("/databases/:databaseID/data/collections/:collection/documents/:id", dh.GetDocument, g.delegable(auth.DatabaseRead))
		p.PATCH("/databases/:databaseID/data/collections/:collection/documents/:id", dh.PatchDocument, g.delegable(auth.DatabaseWrite), g.writeGate)
		p.PUT("/databases/:databaseID/data/collections/:collection/documents/:id", dh.UpdateDocument, g.delegable(auth.DatabaseWrite), g.writeGate)
		p.DELETE("/databases/:databaseID/data/collections/:collection/documents/:id", dh.DeleteDocument, g.delegable(auth.DatabaseWrite), g.writeGate)
		p.DELETE("/databases/:databaseID/data/collections/:collection", dh.TruncateCollection, g.delegable(auth.ProjectAdmin), g.writeGate)
		// 行级访问规则：读规则沿用数据库读权限；变更只允许项目管理员（不外委）。
		p.GET("/databases/:databaseID/data/collections/:collection/access", dh.GetAccessRule, g.delegable(auth.DatabaseRead))
		p.PUT("/databases/:databaseID/data/collections/:collection/access", dh.PutAccessRule, g.require(auth.ProjectAdmin), g.writeGate)
		p.DELETE("/databases/:databaseID/data/collections/:collection/access", dh.DeleteAccessRule, g.require(auth.ProjectAdmin), g.writeGate)
	}
	if deps.CollectionMetaHandler != nil {
		mh := deps.CollectionMetaHandler
		p.POST("/databases/:databaseID/data/collections/meta/dry-run-view", mh.DryRunView, g.require(auth.ProjectAdmin))
		p.GET("/databases/:databaseID/data/collections/:collection/meta", mh.Get, g.delegable(auth.DatabaseRead))
		p.PUT("/databases/:databaseID/data/collections/:collection/meta", mh.Put, g.delegable(auth.DatabaseWrite), g.writeGate)
		p.DELETE("/databases/:databaseID/data/collections/:collection/meta", mh.Delete, g.delegable(auth.DatabaseWrite), g.writeGate)
	}
}

// mountBackupRealtimeRoutes 挂载备份与实时订阅路由。
func mountBackupRealtimeRoutes(p *echo.Group, deps Dependencies, g routeGuards) {
	if deps.BackupHandler != nil {
		bh := deps.BackupHandler
		p.GET("/backups", bh.List, g.require(auth.ProjectAdmin))
		p.POST("/backups", bh.Create, g.require(auth.ProjectAdmin), g.writeGate)
		p.GET("/backups/:id/download", bh.Download, g.require(auth.ProjectAdmin))
		p.DELETE("/backups/:id", bh.Delete, g.require(auth.ProjectAdmin), g.writeGate)
		p.POST("/backups/:id/restore", bh.Restore, g.require(auth.ProjectAdmin), g.writeGate)
	}
	if deps.RealtimeHandler != nil {
		rh := deps.RealtimeHandler
		p.GET("/realtime", rh.Subscribe, g.require(auth.DatabaseRead))
		p.POST("/realtime/subscriptions", rh.UpdateSubscription, g.require(auth.DatabaseRead))
	}
}

// mountKVRoutes 挂载 Key-Value 数据服务（key-value-ducklake-plan §3）：项目级单端点，
// 读命令要 database:read，写命令要 database:write（handler 内按命令分类拒绝）。
// KVHandler 为 nil 时不挂载。
func mountKVRoutes(p *echo.Group, deps Dependencies, g routeGuards) {
	if deps.KVHandler != nil {
		kvh := deps.KVHandler
		p.POST("/kv", kvh.Execute, g.delegable(auth.DatabaseRead))
	}
}

// mountLLMRoutes 挂载 LLM Gateway 的 OpenAI 兼容出口（planv6.0 openai-compatible-llm-api-plan）。
// deps.LLM 为 nil 时不挂载。旧的 /llm/chat、/llm/stream、/llm/providers 已删除。
//
// 路径里的两段 v1 都是刻意的：前一段是 SimpleBase 的 API 版本前缀，后一段是
// OpenAI SDK 约定的资源前缀。SDK 的 base_url 指向 ".../projects/<id>/v1"，
// 再拼 /chat/completions。请勿合并成单段（TestOpenAIRoutePaths 锁定）。
//
// 权限用 llm:invoke 而非 database:read：这是该接口的原始设计意图（plan8.md）。
// 三个内置角色与 API Key 签发白名单均已包含它，因此是语义对齐而非权限收紧。
func mountLLMRoutes(p *echo.Group, deps Dependencies, g routeGuards) {
	if deps.LLM != nil {
		lh := llmhttp.NewLLMHandler(deps.LLM, deps.Usage, deps.Audit, deps.System)
		oa := p.Group("/v1")
		oa.POST("/chat/completions", lh.ChatCompletions, g.require(auth.LLMInvoke))
		oa.POST("/responses", lh.Responses, g.require(auth.LLMInvoke))
		oa.GET("/models", lh.ListModels, g.require(auth.LLMInvoke))
	}
}

// mountUsageAuditRoutes 挂载配额与审计查询路由（Plan 9）。
func mountUsageAuditRoutes(p *echo.Group, deps Dependencies, g routeGuards) {
	if deps.Usage != nil {
		qh := opshttp.NewQuotaHandler(deps.Usage)
		p.GET("/quota", qh.GetQuota, g.delegable(auth.DatabaseRead))
	}
	if deps.Audit != nil {
		ah := opshttp.NewAuditHandler(deps.Audit)
		p.GET("/audit", ah.ListOperations, g.delegable(auth.DatabaseRead))
	}
}

// mountS3Routes 挂载 S3 用户文件存储路由。deps.S3FileStore 为 nil 时不挂载。
// /files/:token 挂在根 echo 上（匿名下载链接）。
func mountS3Routes(e *echo.Echo, p *echo.Group, deps Dependencies, g routeGuards) {
	if deps.S3FileStore == nil {
		return
	}
	sh := filehttp.NewS3Handler(filehttp.S3HandlerDeps{Store: deps.S3FileStore, Index: deps.System, Writable: deps.Config.Instance.Writable, TokenSecret: deps.Config.Files.TokenSecret, AllowedMIME: deps.Config.Files.AllowedMIME, Events: deps.TriggerEvents, Sites: deps.Sites, SitesCfg: deps.Config.Sites, SiteTraffic: deps.SiteTraffic, PublicURL: deps.Config.HTTP.PublicURL})
	p.GET("/s3/objects", sh.ListObjects, g.delegable(auth.DatabaseRead))
	p.GET("/s3/objects/content", sh.DownloadObject, g.delegable(auth.DatabaseRead))
	p.POST("/s3/objects", sh.UploadObject, g.delegable(auth.DatabaseWrite), g.writeGate, middleware.BodyLimit(bodyLimit(s3UploadBodyLimit)))
	p.DELETE("/s3/objects", sh.DeleteObject, g.delegable(auth.DatabaseWrite), g.writeGate)
	p.GET("/s3/presign", sh.PresignObject, g.delegable(auth.DatabaseRead))
	p.POST("/s3/link", sh.CreateLink, g.delegable(auth.DatabaseRead))
	// planv6.5 §5.4：发布网站 / 外部访问。写入要求 database:admin。
	// 站点没有实例级开关：配置了 sites.base_domain 即可发布，对象默认私有。
	// 委托主体对 PUT /s3/public 只能操作 web/ 文件夹（见 S3Handler.PutPublic），
	// 其余路由（含公开路径列表）仍不可委托。
	p.GET("/s3/site", sh.GetSite, g.delegable(auth.DatabaseRead))
	// 系统管理员下线任意项目站点（关闭全部公开路径并写审计）。
	e.POST("/v1/system/projects/:projectID/site/takedown", sh.TakedownSite, g.writeGate)
	e.PUT("/v1/system/projects/:projectID/site/quota", sh.PutSiteQuota, g.writeGate)
	p.GET("/s3/public", sh.ListPublic, g.require(auth.DatabaseRead))
	p.PUT("/s3/public", sh.PutPublic, g.delegable(auth.DatabaseAdmin), g.writeGate)
	e.GET("/files/:token", sh.DownloadLinkedObject)
}

// mountSystemRoutes 挂载依赖系统库的路由：观测（metrics / logs / settings / llm traces）、
// 云 Agent、迁移、云函数与定时任务、内置 MCP Server。deps.System 为 nil 时不挂载。
func mountSystemRoutes(e *echo.Echo, v1, p *echo.Group, deps Dependencies, g routeGuards) {
	if deps.System == nil {
		return
	}
	mountObservabilityRoutesV1(v1, p, deps, g)

	// 云 Agent 路由块（含定时执行路由，见 mountAgentRoutes）。
	var bm *builtinMCPHandler
	if deps.Config.Agent.MCP.Builtin {
		bm = newBuiltinMCPHandler(e, deps)
		attachBuiltinMCP(deps.CloudAgent, bm)
	}
	mountAgentRoutes(p, deps, g, bm)

	migrationH := opshttp.NewMigrationHandler(deps.System)
	p.GET("/migrations/status", migrationH.Status, g.require(auth.ProjectAdmin))
	p.GET("/migrations/plan", migrationH.Plan, g.require(auth.ProjectAdmin))

	// 云函数 + 定时任务 + 函数触发器管理面。
	mountGoFunctionAndCronRoutes(p, deps, g)

	// 内置 MCP Server（planv6.7 §3）：对外暴露项目 API。工具调用以调用方自己的
	// Authorization 头对本路由树发子请求，权限与审计沿用各路由原有逻辑。
	// 不挂 delegable：委托令牌（云助手）走进程内会话，不经这个 HTTP 面。
	if bm != nil {
		p.POST("/mcp", bm.Serve, g.require(auth.DatabaseRead))
		p.POST("/mcp/builtin/:name", bm.Serve, g.require(auth.DatabaseRead))
		p.GET("/mcp", bm.MethodNotAllowed, g.require(auth.DatabaseRead))
		p.DELETE("/mcp", bm.MethodNotAllowed, g.require(auth.DatabaseRead))
	}
}

// mountObservabilityRoutesV1 挂载 /v1 下的指标、日志、设置与 LLM trace 查询路由。
// 与根级 mountObservabilityRoutes（/metrics、pprof，router 入口）无关。
func mountObservabilityRoutesV1(v1, p *echo.Group, deps Dependencies, g routeGuards) {
	mh := opshttp.NewMetricsHandler(deps.System)
	p.GET("/metrics/summary", mh.Summary, g.require(auth.DatabaseRead))
	p.GET("/metrics/trend", mh.Trend, g.require(auth.DatabaseRead))

	lh := opshttp.NewLogsHandler(deps.System)
	// planv6.0 §3.1：logs 只读可委托；retention 管理（GET/PUT）不可委托。
	p.GET("/logs", lh.List, g.delegable(auth.DatabaseRead))
	p.GET("/logs/stats", lh.Stats, g.delegable(auth.DatabaseRead))
	p.GET("/logs/export", lh.Export, g.delegable(auth.DatabaseRead))
	p.GET("/logs/retention", lh.GetRetention, g.require(auth.DatabaseRead))
	p.PUT("/logs/retention", lh.PutRetention, g.require(auth.ProjectAdmin), g.writeGate)

	seth := opshttp.NewProjectSettingsHandler(deps.System)
	p.GET("/settings", seth.GetProject, g.require(auth.DatabaseRead))
	p.PUT("/settings", seth.PutProject, g.require(auth.ProjectAdmin), g.writeGate)
	v1.GET("/settings", seth.GetGlobal, g.require(auth.ProjectAdmin))
	v1.PUT("/settings", seth.PutGlobal, g.require(auth.ProjectAdmin), g.writeGate)

	sess := opshttp.NewLLMSessionHandler(deps.System)
	p.GET("/llm/settings", sess.GetSettings, g.require(auth.DatabaseRead))
	p.PUT("/llm/settings", sess.PutSettings, g.require(auth.ProjectAdmin), g.writeGate)

	// 模型调用 trace 查询（planv6.5）：只读、可委托，与 /logs 同权限级。
	// 二期（§9.3）：分组列表 + 详情 span 时间线。
	lth := llmhttp.NewTracesHandler(deps.System)
	p.GET("/llm/traces", lth.List, g.delegable(auth.DatabaseRead))
	p.GET("/llm/trace-groups", lth.Groups, g.delegable(auth.DatabaseRead))
	p.GET("/llm/trace-groups/:traceId", lth.Detail, g.delegable(auth.DatabaseRead))
}
