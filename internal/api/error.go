// 文件说明：error.go — internal/api 中 APIError、APIErrorBody 等符号的实现
//
// 用法
//
//	NewAPIError(status int, code, message, requestID string) *APIError
//	(e *APIError) Error() string
//	Error(err error, requestID string) *APIError  // Error 编码已知的领域错误类型到 APIError
//	同包调用：Error() → agent_handler.go, agent_handler.go 等；WriteError() → agent_handler.go, agent_schedule_handler.go 等；NewAPIError() → agent_handler.go, api_keys_handler.go 等
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/error.go
//	         │ 依赖
//	         ▼
//	[下游] gocloudagent, godatabase, godatabase/kv, godatabase/lease 等
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] Error() ← agent_handler.go, agent_schedule_handler.go
//
// 主要符号
//
//	type APIError struct {…}
//	type APIErrorBody struct {…}
//	type APIErrorDetail struct {…}
//	(e *APIError) Error() string
//	NewAPIError(status int, code, message, requestID string) *APIError
//	Error(err error, requestID string) *APIError
//	... 另有 5 个顶层符号

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/kv"
	"github.com/linkxzhou/SimpleBase/godatabase/lease"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/agentbridge"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
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
func NewAPIError(status int, code, message, requestID string) *APIError {
	return &APIError{
		HTTPStatus: status,
		Body: APIErrorBody{Error: APIErrorDetail{
			Code: code, Message: message, RequestID: requestID,
		}},
	}
}

// Error 编码已知的领域错误类型到 APIError。未知错误返回 nil 由调用方兜底。
// 各 plan 在此追加新错误类型的映射。
func Error(err error, requestID string) *APIError {
	if err == nil {
		return nil
	}
	// 已是 APIError 直接返回
	var ae *APIError
	if errors.As(err, &ae) {
		if ae.Body.Error.RequestID == "" {
			ae.Body.Error.RequestID = requestID
		}
		return ae
	}
	// echo.HTTPError（handler 经 WriteError 透传的 4xx/5xx）
	if apiErr := mapEchoError(err, requestID); apiErr != nil {
		return apiErr
	}
	// 云 Agent 错误码只使用固定文案，不将上游原文传给前端。
	var agentError *gocloudagent.RunError
	if errors.Is(err, gocloudagent.ErrThreadBusy) || errors.As(err, &agentError) {
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
			return NewAPIError(status, re.Code, re.Message, requestID)
		}
	}
	// context 语义
	if errors.Is(err, context.Canceled) {
		return NewAPIError(http.StatusServiceUnavailable, "request_canceled", "request canceled", requestID)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewAPIError(http.StatusGatewayTimeout, "request_timeout", "request timed out", requestID)
	}
	// 哨兵错误按 sentinelErrorMappings 的顺序匹配（顺序即优先级）。
	for _, m := range sentinelErrorMappings {
		if errors.Is(err, m.target) {
			return NewAPIError(m.status, m.code, m.message, requestID)
		}
	}
	return nil
}

// sentinelErrorMappings 是 Error 的哨兵错误映射表；表内顺序即匹配优先级，
// 新增条目须插在语义正确的位置，不得随意重排。
var sentinelErrorMappings = []struct {
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
	// planv6.5：项目不可发布网站（sites 未开启 / 系统项目 / ID 不可作 DNS 标签）。
	{ErrSiteNotPublishable, http.StatusBadRequest, "site_not_publishable", ErrSiteNotPublishable.Error()},
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
	rid := RequestIDFromContext(c.Request().Context())
	if ae := Error(err, rid); ae != nil {
		// 写租约获取在途：附带 Retry-After，客户端据此退避（perf §1.5）。
		if ae.Body.Error.Code == "lease_acquiring" {
			c.Response().Header().Set("Retry-After", "5")
		}
		return c.JSON(ae.HTTPStatus, ae.Body)
	}
	// 未知错误：记录但不泄露细节（§7 排障：静默 500 不可追踪）。
	logUnhandledError(c, err, rid)
	return c.JSON(http.StatusInternalServerError, APIErrorBody{Error: APIErrorDetail{
		Code: "internal_error", Message: "internal error", RequestID: rid,
	}})
}

// requestLogger 是当前 echo 实例注入的 logger（New 装配时设置）。
var requestLogger observability.Logger

// logUnhandledError 输出未映射错误的原始内容，便于定位静默 500。
func logUnhandledError(c echo.Context, err error, rid string) {
	if requestLogger == nil {
		return
	}
	requestLogger.Error("unhandled api error",
		fieldString("request_id", rid),
		fieldString("route", c.Path()),
		fieldString("err", err.Error()),
	)
}

// mapEchoError 将 echo.HTTPError 转换为 APIError。其他错误返回 nil。
func mapEchoError(err error, rid string) *APIError {
	if err == nil {
		return nil
	}
	var he *echo.HTTPError
	if errors.As(err, &he) {
		code := httpStatusToCode(he.Code)
		return NewAPIError(he.Code, code, safeMessage(he), rid)
	}
	return nil
}

func httpStatusToCode(status int) string {
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
