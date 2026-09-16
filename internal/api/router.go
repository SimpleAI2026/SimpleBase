// Package api 提供 SimpleBase 的 v1 HTTP API。
// 路由、中间件与错误协议在本包内组装；业务 handler 由各 plan 逐步挂载。
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/linkxzhou/SimpleBase/internal/web"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Dependencies 是 NewRouter 注入的全部运行期依赖。各字段可由后续 plan 逐步填充。
type Dependencies struct {
	Config  config.Config
	Logger  observability.Logger
	Metrics *observability.Metrics
	Health  HealthChecker
	// 业务依赖（Plan 5 起填充）
	Auth            *auth.Service
	Catalog         CatalogService
	Registry        RegistryService
	DatabaseHandler *DatabaseHandler
	// Plan 6：SQL 执行 handler。writable=false 时仅 query 可用。
	SQLHandler  *SQLHandler
	DataHandler *DataHandler
	// Plan 7-9：缓存、用量、审计、LLM、后台任务
	Cache       CacheService
	Usage       UsageService
	Audit       AuditService
	LLM         LLMService
	JobEnqueuer JobEnqueuer
	// S3FileStore：用户文件存储（objectstore.FileStore）。
	S3FileStore objectstore.FileStore
	// System 是实例系统 DuckLake（元数据 / 指标 / 日志 / S3 索引 / LLM 会话）。
	System *systemdb.Store
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
}

// LLMRequest 是对外请求抽象。
type LLMRequest struct {
	Model       string
	Messages    []LLMMessage
	MaxTokens   *int
	Temperature *float64
}

// LLMMessage 是对话消息。
type LLMMessage struct {
	Role    string
	Content string
}

// LLMResponse 映射 LLM 响应。
type LLMResponse struct {
	Content      string
	Usage        LLMTokenUsage
	Model        string
	Provider     string
	FinishReason string
}

// LLMTokenUsage 是 token 用量。
type LLMTokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
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
}

// JobEnqueuer 抽象后台任务提交（plan7.md）。
type JobEnqueuer interface {
	Enqueue(ctx context.Context, in JobInput) (string, error)
}

// JobInput 是提交任务的输入。
type JobInput struct {
	OperationID string
	DatabaseID  string
	ProjectID   string
	Type        string
	PayloadJSON string
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

	e.Use(requestIDMiddleware())
	e.Use(middleware.Recover())
	e.Use(accessLogMiddleware(deps))
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		Limit: bodyLimit(deps.Config.Limits.MaxRequestBytes),
	}))

	// /health/live 与 /health/ready 不经认证
	health := &HealthHandler{checker: deps.Health}
	e.GET("/health/live", health.Live)
	e.GET("/health/ready", health.Ready)

	if deps.Metrics != nil {
		e.GET(deps.Config.Observability.MetricsPath, echo.WrapHandler(promhttp.Handler()))
	}

	// v1 业务路由组：认证 → project context → 各 handler
	if deps.Auth != nil && deps.DatabaseHandler != nil {
		mountV1Routes(e, deps)
	}

	// 管理端静态资源：所有未匹配 API 路由的 GET 请求回退到前端 SPA。
	// 必须在 /v1、/health、/metrics 等路由注册后调用，以免拦截 API 请求。
	web.Register(e)

	return e
}

// mountV1Routes 挂载 /v1/projects/:projectID/databases/* 路由。
// 中间件顺序：APIKeyMiddleware（认证+注入 Principal）→ projectContext（解析+注入 ProjectContext）。
// 权限校验通过 auth.Require 在每个路由单独配置。
func mountV1Routes(e *echo.Echo, deps Dependencies) {
	authMW := auth.APIKeyMiddleware(deps.Auth, WithPrincipal)
	require := func(perm auth.Permission) echo.MiddlewareFunc {
		return auth.Require(perm, PrincipalFromContext)
	}

	v1 := e.Group("/v1", authMW)
	// 项目枚举（不挂 :projectID，供前端下拉）
	v1.GET("/projects", NewProjectsHandler(deps.Catalog).ListProjects, require(auth.DatabaseRead))
	p := v1.Group("/projects/:projectID", projectContextMiddlewareEcho(deps))
	h := deps.DatabaseHandler
	p.POST("/databases", h.CreateDatabase, require(auth.DatabaseAdmin))
	p.GET("/databases", h.ListDatabases, require(auth.DatabaseRead))
	p.GET("/databases/:databaseID", h.GetDatabase, require(auth.DatabaseRead))
	p.POST("/databases/:databaseID/open", h.OpenDatabase, require(auth.DatabaseAdmin))
	p.POST("/databases/:databaseID/close", h.CloseDatabase, require(auth.DatabaseAdmin))
	p.POST("/databases/:databaseID/backups", h.CreateBackup, require(auth.DatabaseAdmin))
	p.POST("/databases/:databaseID/restore", h.RestoreDatabase, require(auth.DatabaseAdmin))
	p.DELETE("/databases/:databaseID", h.DeleteDatabase, require(auth.DatabaseAdmin))

	// Plan 6：SQL 执行路由。SQLHandler 为 nil 时不挂载（readonly 实例可仅挂 query）。
	if deps.SQLHandler != nil {
		sh := deps.SQLHandler
		p.POST("/databases/:databaseID/query", sh.Query, require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/execute", sh.Execute, require(auth.DatabaseWrite))
		p.POST("/databases/:databaseID/batch", sh.Batch, require(auth.DatabaseWrite))
	}

	if deps.DataHandler != nil {
		dh := deps.DataHandler
		// Legacy project-scoped routes: implicit first database.
		p.GET("/data/collections", dh.ListCollections, require(auth.DatabaseRead))
		p.POST("/data/collections", dh.CreateCollection, require(auth.DatabaseWrite))
		p.GET("/data/collections/:collection", dh.ListDocuments, require(auth.DatabaseRead))
		p.POST("/data/collections/:collection/documents", dh.CreateDocument, require(auth.DatabaseWrite))
		p.PUT("/data/collections/:collection/documents/:id", dh.UpdateDocument, require(auth.DatabaseWrite))
		p.DELETE("/data/collections/:collection/documents/:id", dh.DeleteDocument, require(auth.DatabaseWrite))
		// Database-scoped routes: must select the given databaseID.
		p.GET("/databases/:databaseID/data/collections", dh.ListCollections, require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/data/collections", dh.CreateCollection, require(auth.DatabaseWrite))
		p.GET("/databases/:databaseID/data/collections/:collection", dh.ListDocuments, require(auth.DatabaseRead))
		p.POST("/databases/:databaseID/data/collections/:collection/documents", dh.CreateDocument, require(auth.DatabaseWrite))
		p.PUT("/databases/:databaseID/data/collections/:collection/documents/:id", dh.UpdateDocument, require(auth.DatabaseWrite))
		p.DELETE("/databases/:databaseID/data/collections/:collection/documents/:id", dh.DeleteDocument, require(auth.DatabaseWrite))
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
		sh := &S3Handler{store: deps.S3FileStore, index: deps.System}
		p.GET("/s3/objects", sh.ListObjects, require(auth.DatabaseRead))
		p.POST("/s3/objects", sh.UploadObject, require(auth.DatabaseWrite))
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
		p.GET("/llm/sessions", sess.List, require(auth.DatabaseRead))
		p.POST("/llm/sessions", sess.Create, require(auth.DatabaseWrite))
		p.GET("/llm/sessions/:sessionID", sess.Get, require(auth.DatabaseRead))
		p.DELETE("/llm/sessions/:sessionID", sess.Delete, require(auth.DatabaseWrite))
		p.GET("/llm/sessions/:sessionID/messages", sess.ListMessages, require(auth.DatabaseRead))
		p.POST("/llm/sessions/:sessionID/messages", sess.PostMessage, require(auth.DatabaseWrite))
		p.GET("/llm/settings", sess.GetSettings, require(auth.DatabaseRead))
		p.PUT("/llm/settings", sess.PutSettings, require(auth.ProjectAdmin))
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

func accessLogMiddleware(deps Dependencies) echo.MiddlewareFunc {
	logger := deps.Logger
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
				deps.System.RecordLog(systemdb.LogEvent{
					ProjectID:  projectID,
					Level:      "info",
					Logger:     "http",
					Message:    c.Request().Method + " " + c.Path(),
					FieldsJSON: `{"status":` + itoa(status) + `,"duration_ms":` + itoa(int(latency.Milliseconds())) + `}`,
					RequestID:  rid,
				})
				if projectID != "" {
					deps.System.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_requests", Value: 1})
					deps.System.RecordMetric(systemdb.MetricSample{ProjectID: projectID, Name: "http_latency_ms", Value: float64(latency.Milliseconds())})
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
			if cErr := c.JSON(apiErr.HTTPStatus, apiErr); cErr != nil && deps.Logger != nil {
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
