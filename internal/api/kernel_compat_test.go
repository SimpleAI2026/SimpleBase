// 文件说明：kernel_compat.go — 入口内核的过渡转发面（internal-ddd-hexagonal-implementation-plan §6.1）
//
// 用法
//
//	同包代码继续使用 requestID / requireProject / WriteError / newSSEWriter 等私有名；
//	实现已移到 internal/api/httpkit（协议内核叶子包），这里以类型别名 / 函数变量 / 薄包装保留旧名。
//
// 调用架构
//
//	[上游] internal/api 各 handler / 中间件 / 测试
//	         │ 同包调用
//	         ▼
//	[本文件] internal/api/kernel_compat.go
//	         │ 依赖
//	         ▼
//	[下游] internal/api/httpkit
//
// 主要符号
//
//	type APIError = httpkit.APIError（及 APIErrorBody / APIErrorDetail / AuditEvent / LLM* / ProjectContext / Stage / StageTimer / SSEWriter / NullWriter 别名）
//	var WriteError / NewAPIError / Error / New*Service / ErrProjectMissing …
//	func requestID / requireProject / projectOf / principalOf / decodeJSONBody / queryLimit / recordAudit / newSSEWriter / writeSSEFrame / beginSSE …
//
// 各业务域子包（P5b–P5j）直接 import httpkit，不经本文件；全部迁出后（P6）删除本文件并让 api 内核也改用 httpkit 的导出名。

package api

// 内核转发面（仅测试）：生产代码已直接使用 internal/api/httpkit 的导出名；这里保留同包测试仍在用的旧名。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/llmgateway"
)

// 协议载荷、跨域共享端口与上下文类型：类型别名保证同包的字面量与签名零改动。
type (
	APIError               = httpkit.APIError
	APIErrorBody           = httpkit.APIErrorBody
	APIErrorDetail         = httpkit.APIErrorDetail
	AuditEvent             = httpkit.AuditEvent
	AuditService           = httpkit.AuditService
	UsageService           = httpkit.UsageService
	CacheService           = httpkit.CacheService
	CacheUsage             = httpkit.CacheUsage
	LLMService             = httpkit.LLMService
	LLMRequest             = httpkit.LLMRequest
	LLMMessage             = httpkit.LLMMessage
	LLMTool                = httpkit.LLMTool
	LLMToolCall            = httpkit.LLMToolCall
	LLMResponse            = httpkit.LLMResponse
	LLMTokenUsage          = httpkit.LLMTokenUsage
	LLMStreamReader        = httpkit.LLMStreamReader
	LLMStreamChunk         = httpkit.LLMStreamChunk
	ProjectContext         = httpkit.ProjectContext
	Stage                  = httpkit.Stage
	StageTimer             = httpkit.StageTimer
	StageScope             = httpkit.StageScope
	sseWriter              = httpkit.SSEWriter
	nullWriter             = httpkit.NullWriter
	kvInvalidArgError      = httpkit.KVArgumentError
	llmStreamReaderAdapter = httpkit.LLMStreamReaderAdapter
)

// 请求分段计时的阶段常量与 context key 常量（类型别名不带常量，逐个别名；key 只用于同包测试）。
const (
	requestIDKey    = httpkit.RequestIDKey
	projectKey      = httpkit.ProjectKey
	principalCtxKey = httpkit.PrincipalCtxKey

	StageTotal             = httpkit.StageTotal
	StageAuth              = httpkit.StageAuth
	StageProject           = httpkit.StageProject
	StageDecode            = httpkit.StageDecode
	StageCatalog           = httpkit.StageCatalog
	StageSemaphore         = httpkit.StageSemaphore
	StageAcquire           = httpkit.StageAcquire
	StageDBQueue           = httpkit.StageDBQueue
	StageDBExec            = httpkit.StageDBExec
	StageDBCommit          = httpkit.StageDBCommit
	StageSerialize         = httpkit.StageSerialize
	StageOnWrite           = httpkit.StageOnWrite
	StageUnattributed      = httpkit.StageUnattributed
	StageAuthLock          = httpkit.StageAuthLock
	StageAuthUser          = httpkit.StageAuthUser
	StageAuthUserConn      = httpkit.StageAuthUserConn
	StageAuthUserQuery     = httpkit.StageAuthUserQuery
	StageAuthUserScan      = httpkit.StageAuthUserScan
	StageAuthFlightWait    = httpkit.StageAuthFlightWait
	StageAuthFlightLoad    = httpkit.StageAuthFlightLoad
	StageAuthProject       = httpkit.StageAuthProject
	StageAuthSession       = httpkit.StageAuthSession
	StageAuthKey           = httpkit.StageAuthKey
	StageAuthSessionUpdate = httpkit.StageAuthSessionUpdate
	StageAuthSessionIssue  = httpkit.StageAuthSessionIssue
	StageSystemDB          = httpkit.StageSystemDB
)

var (
	ErrProjectMissing     = httpkit.ErrProjectMissing
	ErrMCPNameReserved    = httpkit.ErrMCPNameReserved
	ErrSiteNotPublishable = httpkit.ErrSiteNotPublishable
	PerfStages            = httpkit.PerfStages

	NewAPIError          = httpkit.NewAPIError
	Error                = httpkit.Error
	WriteError           = httpkit.WriteError
	NewCacheService      = httpkit.NewCacheService
	NewUsageService      = httpkit.NewUsageService
	NewAuditService      = httpkit.NewAuditService
	NewLLMService        = httpkit.NewLLMService
	WithRequestID        = httpkit.WithRequestID
	RequestIDFromContext = httpkit.RequestIDFromContext
	WithProject          = httpkit.WithProject
	ProjectFromContext   = httpkit.ProjectFromContext
	WithPrincipal        = httpkit.WithPrincipal
	PrincipalFromContext = httpkit.PrincipalFromContext
	StageTimerFrom       = httpkit.StageTimerFrom
	HTTPStatusToCode     = httpkit.HTTPStatusToCode
	SetRequestLogger     = httpkit.SetLogger
)

func requestID(c echo.Context) string { return httpkit.RequestID(c) }

func badRequest(c echo.Context, msg string) *APIError { return httpkit.BadRequest(c, msg) }

func badRequestWithID(msg, rid string) *APIError { return httpkit.BadRequestWithID(msg, rid) }

func requireProject(c echo.Context) (ProjectContext, error) { return httpkit.RequireProject(c) }
func projectOf(c echo.Context) ProjectContext               { return httpkit.ProjectOf(c) }
func projectID(c echo.Context) string                       { return httpkit.ProjectID(c) }
func projectIDFrom(ctx context.Context) string              { return httpkit.ProjectIDFrom(ctx) }
func principalOf(c echo.Context) auth.Principal             { return httpkit.PrincipalOf(c) }

func decodeJSONBody(c echo.Context, dst any, strict bool) error {
	return httpkit.DecodeJSONBody(c, dst, strict)
}
func queryLimit(c echo.Context, def, max int) int { return httpkit.QueryLimit(c, def, max) }
func parseQueryLimit(c echo.Context, def, max int) (int, bool) {
	return httpkit.ParseQueryLimit(c, def, max)
}

func fieldString(k, v string) zap.Field                 { return httpkit.FieldString(k, v) }
func fieldDuration(k string, d time.Duration) zap.Field { return httpkit.FieldDuration(k, d) }

func recordAudit(c echo.Context, svc AuditService, kind, projectID, status, detail string) {
	httpkit.RecordAudit(c, svc, kind, projectID, status, detail)
}

func newSSEWriter(c echo.Context, idHeader, idValue string) *sseWriter {
	return httpkit.NewSSEWriter(c, idHeader, idValue)
}
func writeSSEFrame(dst io.Writer, flusher http.Flusher, id int64, payload any) {
	httpkit.WriteSSEFrame(dst, flusher, id, payload)
}
func beginSSE(c echo.Context) { httpkit.BeginSSE(c) }

func publicBaseURL(c echo.Context, publicURL, override string) string {
	return httpkit.PublicBaseURL(c, publicURL, override)
}

func newStageTimer(ctx context.Context) *StageTimer { return httpkit.NewStageTimer(ctx) }
func stageDurationsJSON(spent map[Stage]time.Duration) map[string]float64 {
	return httpkit.StageDurationsJSON(spent)
}

func mapEchoError(err error, rid string) *APIError { return httpkit.MapEchoError(err, rid) }
func safeMessage(he *echo.HTTPError) string        { return httpkit.SafeMessage(he) }

// kvInvalidArgumentRaw 是 KV 命令表 / 事务内的参数错误（Error 统一映射为 400 kv_invalid_argument）。
func kvInvalidArgumentRaw(msg string) error { return httpkit.NewKVArgumentError(msg) }

func logUnhandledError(c echo.Context, err error, rid string) { httpkit.LogUnhandledError(c, err, rid) }

func httpStatusToCode(status int) string { return httpkit.HTTPStatusToCode(status) }

// 仅供同包测试读取映射表与阶段顺序。
var (
	sentinelErrorMappings = httpkit.SentinelErrorMappings
	perfStages            = httpkit.PerfStages
)

func decodeToolChoice(raw json.RawMessage) any           { return httpkit.DecodeToolChoice(raw) }
func toGatewayRequest(req LLMRequest) llmgateway.Request { return httpkit.ToGatewayRequest(req) }

func perfStageMiddleware(recorder httpkit.StageTimingRecorder, logger httpkit.LoggerLike) echo.MiddlewareFunc {
	return httpkit.PerfStageMiddleware(recorder, logger)
}
