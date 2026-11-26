// 文件说明：errors.go — 统一错误协议：APIError 载荷、领域错误 → 状态码 / code 的映射、WriteError
//
// 用法
//
//	NewAPIError(status, code, message, RequestID) *APIError      // 构造协议载荷
//	Error(err, RequestID) *APIError                              // 已知领域错误 → APIError（未知返回 nil）
//	WriteError(c echo.Context, err error) error                  // handler 把任意 err 写成统一错误协议
//	BadRequest(c, msg) / BadRequestWithID(msg, rid)              // 400 invalid_request 的两种构造
//	MapEchoError / HTTPStatusToCode / SafeMessage                // Echo 错误与兜底状态码的翻译（api 的 errorHandler 用）
//	SetLogger(l)                                                // 注入「未知错误」日志的 logger
//
// 调用架构
//
//	[上游] internal/api、internal/api/<context>http（全部 handler）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/httpkit/errors.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent/{cloudagent,group,plan}, godatabase/{kv,lease,sqlguard}, gosandbox/vm,
//	       internal/agentbridge, internal/agentrun, internal/auth, internal/catalog, internal/objectstore,
//	       internal/observability, internal/sandbox, internal/systemdb
//
// 主要符号
//
//	type APIError struct {…}
//	type APIErrorBody struct {…}
//	type APIErrorDetail struct {…}
//	NewAPIError(status int, code, message, RequestID string) *APIError
//	Error(err error, RequestID string) *APIError
//	WriteError(c echo.Context, err error) error
//	var SentinelErrorMappings
//	... 另有 9 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §6.1、D3-a）：错误协议是入口内核，被每个业务域的
// handler 使用；它是叶子包，不 import api / 子包 / app。映射顺序即优先级，新增条目须插在语义正确的位置。

package httpkit

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"
	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/kv"
	"github.com/linkxzhou/SimpleBase/godatabase/lease"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// 入口层守卫的哨兵错误（迁移自 api 的 MCP 配置与站点发布 handler；映射表在上面）。
var (
	// ErrMCPNameReserved：内置 MCP Server 名称不允许用户占用。
	ErrMCPNameReserved = errors.New("mcp server name is reserved")
	// ErrSiteNotPublishable：项目不可发布网站（sites 未开启 / 系统项目 / ID 不可作 DNS 标签）。
	ErrSiteNotPublishable = errors.New("site is not publishable for this project")
)

// APIError 是统一错误协议的载荷。
// {"error":{"code":"...","message":"...","request_id":"..."}}
type APIError struct {
	HTTPStatus int
	Body       APIErrorBody
}

type APIErrorBody struct {
	Error APIErrorDetail `json:"error"`
}

type APIErrorDetail struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	RequestID   string `json:"request_id"`
	Field       string `json:"field,omitempty"`
	FailedIndex *int   `json:"failed_index,omitempty"`
}

func (e *APIError) Error() string { return e.Body.Error.Code + ": " + e.Body.Error.Message }

// NewAPIError 构造带 request ID 的错误。
func NewAPIError(status int, code, message, RequestID string) *APIError {
	return &APIError{
		HTTPStatus: status,
		Body: APIErrorBody{Error: APIErrorDetail{
			Code: code, Message: message, RequestID: RequestID,
		}},
	}
}

// badRequest 构造 400 invalid_request；只封装这一种 code，其它 code 仍直接 NewAPIError。
func BadRequest(c echo.Context, msg string) *APIError {
	return BadRequestWithID(msg, RequestID(c))
}

// BadRequestWithID 供没有 Echo 上下文的纯校验函数构造同一种错误。
func BadRequestWithID(msg, rid string) *APIError {
	return NewAPIError(http.StatusBadRequest, "invalid_request", msg, rid)
}

// Error 编码已知的领域错误类型到 APIError。未知错误返回 nil 由调用方兜底。
// 各 plan 在此追加新错误类型的映射。
func Error(err error, RequestID string) *APIError {
	if err == nil {
		return nil
	}
	// 已是 APIError 直接返回
	var ae *APIError
	if errors.As(err, &ae) {
		if ae.Body.Error.RequestID == "" {
			ae.Body.Error.RequestID = RequestID
		}
		return ae
	}
	// echo.HTTPError（handler 经 WriteError 透传的 4xx/5xx）
	if apiErr := MapEchoError(err, RequestID); apiErr != nil {
		return apiErr
	}
	// 云 Agent 错误码只使用固定文案，不将上游原文传给前端。
	var agentError *cloudagent.RunError
	if errors.Is(err, cloudagent.ErrThreadBusy) || errors.As(err, &agentError) {
		re := agentbridge.ClassifyError(err)
		known := map[string]int{
			"agent_thread_busy":     http.StatusConflict,
			"llm_not_configured":    http.StatusServiceUnavailable,
			"llm_auth_failed":       http.StatusBadGateway,
			"llm_rate_limited":      http.StatusTooManyRequests,
			"llm_model_not_allowed": http.StatusBadRequest,
			"llm_timeout":           http.StatusGatewayTimeout,
		}
		if status, ok := known[re.Code]; ok {
			return NewAPIError(status, re.Code, re.Message, RequestID)
		}
	}
	// context 语义
	if errors.Is(err, context.Canceled) {
		return NewAPIError(http.StatusServiceUnavailable, "request_canceled", "request canceled", RequestID)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewAPIError(http.StatusGatewayTimeout, "request_timeout", "request timed out", RequestID)
	}
	// KV 命令表的参数错误（命令表不依赖 Echo，由 kvInvalidArgumentRaw 构造）。
	var kvArg *KVArgumentError
	if errors.As(err, &kvArg) {
		return NewAPIError(http.StatusBadRequest, "kv_invalid_argument", kvArg.Message, RequestID)
	}
	// 应用服务的「请求不合法且文案由底层错误决定」（迁移前写作 echo.NewHTTPError(400, err.Error())）。
	var invalid *agentrun.InvalidError
	if errors.As(err, &invalid) {
		return NewAPIError(http.StatusBadRequest, "invalid_request", invalid.Message, RequestID)
	}
	// 哨兵错误按 SentinelErrorMappings 的顺序匹配（顺序即优先级）。
	for _, m := range SentinelErrorMappings {
		if errors.Is(err, m.target) {
			return NewAPIError(m.status, m.code, m.message, RequestID)
		}
	}
	return nil
}

// SentinelErrorMappings 是 Error 的哨兵错误映射表；表内顺序即匹配优先级，
// 新增条目须插在语义正确的位置，不得随意重排。
var SentinelErrorMappings = []struct {
	target        error
	status        int
	code, message string
}{
	// 云沙盒领域错误（planv4.0 cloud-sandbox-plan §7.3）；不透传 SDK 内部路径。
	{vm.ErrUnavailable, http.StatusServiceUnavailable, "sandbox_unavailable", "sandbox is unavailable"},
	{sandbox.ErrNotFound, http.StatusNotFound, "sandbox_not_found", "sandbox not found"},
	{vm.ErrFileNotFound, http.StatusNotFound, "sandbox_file_not_found", "file not found"},
	{sandbox.ErrNameConflict, http.StatusConflict, "sandbox_name_conflict", "sandbox name already exists"},
	{sandbox.ErrLimitExceeded, http.StatusTooManyRequests, "sandbox_limit_exceeded", "sandbox limit exceeded"},
	{sandbox.ErrInvalidSpec, http.StatusBadRequest, "sandbox_invalid_spec", "invalid sandbox specification"},
	{sandbox.ErrInvalidPath, http.StatusBadRequest, "sandbox_invalid_path", "invalid sandbox path"},
	{sandbox.ErrFileTooLarge, http.StatusRequestEntityTooLarge, "sandbox_file_too_large", "sandbox file too large"},
	{sandbox.ErrBusy, http.StatusConflict, "sandbox_busy", "sandbox is busy"},
	{sandbox.ErrGone, http.StatusGone, "sandbox_gone", "sandbox has expired"},
	{vm.ErrBackend, http.StatusBadGateway, "sandbox_backend_error", "sandbox backend error"},
	// handler 前置：拿不到中间件注入的 project 边界（正常不可达，见 context.go）。
	{ErrProjectMissing, http.StatusBadRequest, "invalid_request", ErrProjectMissing.Error()},
	{ErrMCPNameReserved, http.StatusBadRequest, "mcp_name_reserved", "mcp server name is reserved"},
	// planv6.5：项目不可发布网站（sites 未开启 / 系统项目 / ID 不可作 DNS 标签）。
	{ErrSiteNotPublishable, http.StatusBadRequest, "site_not_publishable", ErrSiteNotPublishable.Error()},
	// planv6.7 数据看板：Agent 生成图表的数据源上下文校验失败。
	// planv6.7 AGC-NAV / v2：标签与群轮次领域错误。
	{systemdb.ErrAgentTagNotFound, http.StatusNotFound, "agent_tag_not_found", "agent tag not found"},
	{systemdb.ErrAgentTagConflict, http.StatusConflict, "agent_tag_conflict", "agent tag was modified, reload and retry"},
	{systemdb.ErrAgentTagDuplicate, http.StatusConflict, "agent_tag_duplicate", "agent tag name already exists"},
	{systemdb.ErrAgentTagBuiltin, http.StatusBadRequest, "agent_tag_builtin", "builtin agent tag cannot be deleted"},
	{systemdb.ErrAgentTagLimit, http.StatusTooManyRequests, "agent_tag_limit_exceeded", "too many agent tags"},
	{agentrun.ErrTurnsRequired, http.StatusBadRequest, "turns_required", agentrun.ErrTurnsRequired.Error()},
	// Agent 协作用例的请求类错误（internal/agentrun 的领域错误；文案与迁移前逐字一致）。
	{agentrun.ErrStopping, http.StatusServiceUnavailable, "service_stopping", "service is shutting down"},
	{agentrun.ErrRunContentRequired, http.StatusBadRequest, "invalid_request", "content is required"},
	{agentrun.ErrInvalidRetry, http.StatusBadRequest, "invalid_request", "invalid_retry"},
	{agentrun.ErrMultipleAgents, http.StatusBadRequest, "multiple_agents_not_supported", "mention only one agent per run"},
	{agentrun.ErrAgentSwitchRequired, http.StatusConflict, "agent_switch_required", "switch to the mentioned agent before sending"},
	{agentrun.ErrCLISkillsUnavailable, http.StatusServiceUnavailable, "cli_skills_unavailable", "this agent requires CLI skills"},
	{agentrun.ErrAgentSandboxUnavailable, http.StatusServiceUnavailable, "sandbox_unavailable", "sandbox is unavailable"},
	{agentrun.ErrThreadNotFound, http.StatusNotFound, "not_found", "thread not found"},
	{agentrun.ErrRunNotFound, http.StatusNotFound, "not_found", "run not found"},
	{agentrun.ErrConfirmationNotFound, http.StatusNotFound, "not_found", "confirmation not found"},
	{agentrun.ErrQuotaExceeded, http.StatusTooManyRequests, "quota_exceeded", "项目模型调用配额已用完"},
	// planv6.7 v2：协作群领域错误。顺序按 HTTP 语义分组，具体到泛化。
	{systemdb.ErrCollaborationUnavailable, http.StatusServiceUnavailable, "workspace_upgrade_required", "collaboration schema is not ready; run the schema bootstrap first"},
	{agentrun.ErrWorkspaceUpgradeRequired, http.StatusServiceUnavailable, "workspace_upgrade_required", "collaboration schema is not ready; run the schema bootstrap first"},
	{agentrun.ErrGroupArchived, http.StatusConflict, "group_archived", "this group has been archived"},
	{agentrun.ErrGroupMemberUnavailable, http.StatusBadRequest, "group_member_unavailable", "the target agent is not an available member of this group"},
	{systemdb.ErrTurnBusy, http.StatusConflict, "turn_busy", "this thread already has an active turn"},
	{agentrun.ErrTurnBusy, http.StatusConflict, "turn_busy", "this thread already has an active turn"},
	{systemdb.ErrTurnConflict, http.StatusConflict, "turn_conflict", "agent turn was modified, reload and retry"},
	{agentrun.ErrTurnConflict, http.StatusConflict, "turn_conflict", "agent turn was modified, reload and retry"},
	{group.ErrTurnBusy, http.StatusConflict, "turn_busy", "this thread already has an active turn"},
	{group.ErrIllegalTransition, http.StatusConflict, "turn_conflict", "agent turn was modified, reload and retry"},
	{systemdb.ErrTurnNotFound, http.StatusNotFound, "turn_not_found", "agent turn not found"},
	{systemdb.ErrTaskNotFound, http.StatusNotFound, "task_not_found", "agent task not found"},
	{agentrun.ErrRoutingFailed, http.StatusBadGateway, "routing_failed", "routing failed"},
	{agentrun.ErrInvalidDispatchGraph, http.StatusBadRequest, "invalid_dispatch_graph", "invalid dispatch graph"},
	{agentrun.ErrHumanAssigneeRequired, http.StatusConflict, "human_assignee_required", "a human assignee is required"},
	{plan.ErrMemberMissing, http.StatusBadRequest, "agent_not_in_tag", "mentioned agent is not in this tag"},
	{plan.ErrMentionMissing, http.StatusBadRequest, "invalid_dispatch_graph", "invalid dispatch graph"},
	{plan.ErrHumanUnavailable, http.StatusConflict, "human_assignee_required", "a human assignee is required"},
	{plan.ErrHumanDisabled, http.StatusConflict, "human_policy_disabled", "human tasks are disabled for this group"},
	{plan.ErrInvalid, http.StatusBadRequest, "invalid_dispatch_graph", "invalid dispatch graph"},
	{plan.ErrEmpty, http.StatusBadRequest, "invalid_dispatch_graph", "invalid dispatch graph"},
	{plan.ErrTooLarge, http.StatusBadRequest, "invalid_dispatch_graph", "invalid dispatch graph"},
	{plan.ErrTooDeep, http.StatusBadRequest, "invalid_dispatch_graph", "invalid dispatch graph"},
	{group.ErrRoutingFailed, http.StatusBadGateway, "routing_failed", "routing failed"},
	{agentrun.ErrHumanTaskForbidden, http.StatusForbidden, "human_task_forbidden", "only the assignee or a project admin can resolve this task"},
	{agentrun.ErrTaskNotRetryable, http.StatusConflict, "task_not_retryable", "this task cannot be retried"},
	{agentrun.ErrCollaborationBudgetExceeded, http.StatusTooManyRequests, "collaboration_budget_exceeded", "collaboration budget exceeded"},
	// auth 错误
	{auth.ErrMissingCredentials, http.StatusUnauthorized, "unauthenticated", "missing or malformed authorization header"},
	{auth.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_api_key", "invalid api key"},
	{auth.ErrKeyRevoked, http.StatusUnauthorized, "api_key_revoked", "api key revoked"},
	{auth.ErrForbidden, http.StatusForbidden, "forbidden", "permission denied"},
	{auth.ErrUserNotFound, http.StatusNotFound, "user_not_found", "user not found"},
	{auth.ErrUsernameTaken, http.StatusConflict, "username_taken", "username already exists"},
	{auth.ErrUserDisabled, http.StatusForbidden, "user_disabled", "user disabled"},
	{auth.ErrUserProtected, http.StatusBadRequest, "user_protected", "user is protected"},
	{auth.ErrInvalidRole, http.StatusBadRequest, "invalid_role", "invalid role"},
	{auth.ErrWeakPassword, http.StatusBadRequest, "weak_password", "password too weak"},
	{auth.ErrMustChangePasswd, http.StatusLocked, "must_change_password", "password change required"},
	{auth.ErrInvalidRefreshToken, http.StatusUnauthorized, "invalid_refresh_token", "invalid refresh token"},
	{auth.ErrSessionRevoked, http.StatusUnauthorized, "session_revoked", "session revoked"},
	{auth.ErrInvalidToken, http.StatusUnauthorized, "invalid_or_expired_token", "invalid or expired token"},
	{auth.ErrExpiredToken, http.StatusUnauthorized, "invalid_or_expired_token", "invalid or expired token"},
	// catalog 错误
	{catalog.ErrNotFound, http.StatusNotFound, "database_not_found", "database not found"},
	{catalog.ErrAlreadyExists, http.StatusConflict, "database_already_exists", "database already exists"},
	{catalog.ErrInvalidName, http.StatusBadRequest, "invalid_database_name", "invalid database name"},
	{catalog.ErrInvalidDatabasePath, http.StatusInternalServerError, "invalid_database_path", "project database storage is invalid"},
	{catalog.ErrInvalidCursor, http.StatusBadRequest, "invalid_cursor", "invalid database list cursor"},
	{catalog.ErrInvalidState, http.StatusConflict, "invalid_state", "invalid database state transition"},
	{catalog.ErrCrossProject, http.StatusForbidden, "cross_project_denied", "cross project access denied"},
	{catalog.ErrDescriptorWrite, http.StatusServiceUnavailable, "descriptor_write_failed", "failed to persist database descriptor"},
	{catalog.ErrMigrationFailed, http.StatusServiceUnavailable, "migration_failed", "catalog migration failed"},
	{catalog.ErrSystemProtected, http.StatusForbidden, "system_database_protected", "system database cannot be modified or deleted"},
	{catalog.ErrInvalidDataModel, http.StatusBadRequest, "data_model_invalid", "data_model must be collection or sql"},
	{systemdb.ErrUnavailable, http.StatusServiceUnavailable, "system_store_unavailable", "system database unavailable"},
	// database 运行时错误
	{godatabase.ErrDatabaseDeleting, http.StatusConflict, "database_deleting", "database is being deleted"},
	{godatabase.ErrDatabaseNotReady, http.StatusConflict, "database_not_ready", "database is not ready"},
	{godatabase.ErrWriterUnavailable, http.StatusServiceUnavailable, "writer_unavailable", "this instance is not writable"},
	// 写租约获取在途（perf §1.5）：503 + Retry-After（WriteError 按 code 附加），客户端应稍后重试。
	{lease.ErrAcquiring, http.StatusServiceUnavailable, "lease_acquiring", "write lease acquisition in progress; retry shortly"},
	{lease.ErrLeaseHeld, http.StatusServiceUnavailable, "lease_held", "write lease held by another instance; retry shortly"},
	{godatabase.ErrRowLimitExceeded, 422, "row_limit_exceeded", "query row limit exceeded"},
	{godatabase.ErrRegistryClosed, http.StatusServiceUnavailable, "registry_closed", "database registry is closed"},
	{godatabase.ErrUnsupportedValue, http.StatusInternalServerError, "unsupported_value_type", "unsupported column value type"},
	{godatabase.ErrQueryConcurrency, http.StatusTooManyRequests, "query_concurrency_exceeded", "query concurrency limit exceeded"},
	// sqlguard 错误
	{sqlguard.ErrEmptySQL, http.StatusBadRequest, "empty_sql", "sql statement is empty"},
	// objectstore 文件 key 校验错误
	{objectstore.ErrInvalidKey, http.StatusBadRequest, "invalid_file_key", "invalid file key"},
	// sqlguard 错误（续）
	{sqlguard.ErrMultipleStatements, http.StatusBadRequest, "multiple_statements", "multiple statements are not allowed"},
	{sqlguard.ErrNulChar, http.StatusBadRequest, "invalid_sql", "nul character not allowed in sql"},
	{sqlguard.ErrSQLNotAllowed, http.StatusBadRequest, "sql_not_allowed", "sql statement not allowed"},
	{sqlguard.ErrWriteInReadOnly, http.StatusBadRequest, "write_in_read_only", "write statement not allowed in read-only intent"},
	// kv 错误（key-value-ducklake-plan §5）
	{kv.ErrNotFound, http.StatusNotFound, "kv_not_found", "key not found"},
	{kv.ErrKeyType, http.StatusConflict, "kv_type_mismatch", "key holds a different type"},
	{kv.ErrKeyExists, http.StatusConflict, "kv_key_exists", "key already exists"},
	{kv.ErrValueType, http.StatusBadRequest, "kv_invalid_value", "value is not a valid number"},
	{kv.ErrArgument, http.StatusBadRequest, "kv_invalid_argument", "invalid argument"},
}

// WriteError 是 handler 把任意 err 写成统一错误协议的入口。
func WriteError(c echo.Context, err error) error {
	if err == nil {
		return nil
	}
	rid := RequestID(c)
	if ae := Error(err, rid); ae != nil {
		// 写租约获取在途：附带 Retry-After，客户端据此退避（perf §1.5）。
		if ae.Body.Error.Code == "lease_acquiring" {
			c.Response().Header().Set("Retry-After", "5")
		}
		return c.JSON(ae.HTTPStatus, ae.Body)
	}
	// 未知错误：记录但不泄露细节（§7 排障：静默 500 不可追踪）。
	LogUnhandledError(c, err, rid)
	return c.JSON(http.StatusInternalServerError, APIErrorBody{Error: APIErrorDetail{
		Code: "internal_error", Message: "internal error", RequestID: rid,
	}})
}

// requestLogger 是当前 echo 实例注入的 logger（New 装配时设置）。
var requestLogger observability.Logger

// LogUnhandledError 输出未映射错误的原始内容，便于定位静默 500。
func LogUnhandledError(c echo.Context, err error, rid string) {
	if requestLogger == nil {
		return
	}
	requestLogger.Error("unhandled api error",
		FieldString("request_id", rid),
		FieldString("route", c.Path()),
		FieldString("err", err.Error()),
	)
}

// MapEchoError 将 echo.HTTPError 转换为 APIError。其他错误返回 nil。
func MapEchoError(err error, rid string) *APIError {
	if err == nil {
		return nil
	}
	var he *echo.HTTPError
	if errors.As(err, &he) {
		code := HTTPStatusToCode(he.Code)
		return NewAPIError(he.Code, code, SafeMessage(he), rid)
	}
	return nil
}

func HTTPStatusToCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	case http.StatusGatewayTimeout:
		return "gateway_timeout"
	default:
		return "http_error"
	}
}

func SafeMessage(he *echo.HTTPError) string {
	if he.Message == nil {
		return "error"
	}
	if s, ok := he.Message.(string); ok {
		return s
	}
	return "error"
}

// SetLogger 注入记录「未知错误」的 logger；nil 表示不记录。
func SetLogger(l observability.Logger) { requestLogger = l }

// KVArgumentError 是 KV 命令表的参数错误（命令表不依赖 Echo），映射为 400 kv_invalid_argument。
type KVArgumentError struct{ Message string }

func (e *KVArgumentError) Error() string { return "kv_invalid_argument: " + e.Message }

// NewKVArgumentError 构造 KV 参数错误。
func NewKVArgumentError(msg string) error { return &KVArgumentError{Message: msg} }

// ErrorHandler 是 Echo 的全局错误处理器（与迁移前 api.errorHandler 的非 E2B 部分逐字一致）：
// handler 未处理的知识错误经 MapEchoError 写出，其余 echo.HTTPError 以 http_error 兜底，未知错误只记日志、
// 对外 500 internal_error。E2B 兼容面由 api 在调用本处理器之前自行分流。
func ErrorHandler(logger observability.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		rid := RequestID(c)
		if apiErr := MapEchoError(err, rid); apiErr != nil {
			if cErr := c.JSON(apiErr.HTTPStatus, apiErr.Body); cErr != nil && logger != nil {
				logger.Error("write error", FieldString("err", cErr.Error()))
			}
			return
		}
		if he, ok := err.(*echo.HTTPError); ok {
			if cErr := c.JSON(he.Code, map[string]any{
				"error": map[string]any{
					"code":       "http_error",
					"message":    SafeMessage(he),
					"request_id": rid,
				},
			}); cErr != nil && logger != nil {
				logger.Error("write http error", FieldString("err", cErr.Error()))
			}
			return
		}
		if logger != nil {
			logger.Error("unhandled error", FieldString("request_id", rid), FieldString("err", err.Error()))
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
