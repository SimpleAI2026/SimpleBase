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
	// 云沙盒领域错误（planv4.0 cloud-sandbox-plan §7.3）；不透传 SDK 内部路径。
	for _, mapping := range []struct {
		target        error
		status        int
		code, message string
	}{
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
	} {
		if errors.Is(err, mapping.target) {
			return NewAPIError(mapping.status, mapping.code, mapping.message, requestID)
		}
	}
	// auth 错误
	if errors.Is(err, auth.ErrMissingCredentials) {
		return NewAPIError(http.StatusUnauthorized, "unauthenticated", "missing or malformed authorization header", requestID)
	}
	if errors.Is(err, auth.ErrInvalidCredentials) {
		return NewAPIError(http.StatusUnauthorized, "invalid_api_key", "invalid api key", requestID)
	}
	if errors.Is(err, auth.ErrKeyRevoked) {
		return NewAPIError(http.StatusUnauthorized, "api_key_revoked", "api key revoked", requestID)
	}
	if errors.Is(err, auth.ErrForbidden) {
		return NewAPIError(http.StatusForbidden, "forbidden", "permission denied", requestID)
	}
	if errors.Is(err, auth.ErrUserNotFound) {
		return NewAPIError(http.StatusNotFound, "user_not_found", "user not found", requestID)
	}
	if errors.Is(err, auth.ErrUsernameTaken) {
		return NewAPIError(http.StatusConflict, "username_taken", "username already exists", requestID)
	}
	if errors.Is(err, auth.ErrUserDisabled) {
		return NewAPIError(http.StatusForbidden, "user_disabled", "user disabled", requestID)
	}
	if errors.Is(err, auth.ErrUserProtected) {
		return NewAPIError(http.StatusBadRequest, "user_protected", "user is protected", requestID)
	}
	if errors.Is(err, auth.ErrInvalidRole) {
		return NewAPIError(http.StatusBadRequest, "invalid_role", "invalid role", requestID)
	}
	if errors.Is(err, auth.ErrWeakPassword) {
		return NewAPIError(http.StatusBadRequest, "weak_password", "password too weak", requestID)
	}
	if errors.Is(err, auth.ErrMustChangePasswd) {
		return NewAPIError(http.StatusLocked, "must_change_password", "password change required", requestID)
	}
	if errors.Is(err, auth.ErrInvalidRefreshToken) {
		return NewAPIError(http.StatusUnauthorized, "invalid_refresh_token", "invalid refresh token", requestID)
	}
	if errors.Is(err, auth.ErrSessionRevoked) {
		return NewAPIError(http.StatusUnauthorized, "session_revoked", "session revoked", requestID)
	}
	if errors.Is(err, auth.ErrInvalidToken) {
		return NewAPIError(http.StatusUnauthorized, "invalid_or_expired_token", "invalid or expired token", requestID)
	}
	if errors.Is(err, auth.ErrExpiredToken) {
		return NewAPIError(http.StatusUnauthorized, "invalid_or_expired_token", "invalid or expired token", requestID)
	}
	// catalog 错误
	if errors.Is(err, catalog.ErrNotFound) {
		return NewAPIError(http.StatusNotFound, "database_not_found", "database not found", requestID)
	}
	if errors.Is(err, catalog.ErrAlreadyExists) {
		return NewAPIError(http.StatusConflict, "database_already_exists", "database already exists", requestID)
	}
	if errors.Is(err, catalog.ErrInvalidName) {
		return NewAPIError(http.StatusBadRequest, "invalid_database_name", "invalid database name", requestID)
	}
	if errors.Is(err, catalog.ErrInvalidDatabasePath) {
		return NewAPIError(http.StatusInternalServerError, "invalid_database_path", "project database storage is invalid", requestID)
	}
	if errors.Is(err, catalog.ErrInvalidCursor) {
		return NewAPIError(http.StatusBadRequest, "invalid_cursor", "invalid database list cursor", requestID)
	}
	if errors.Is(err, catalog.ErrInvalidState) {
		return NewAPIError(http.StatusConflict, "invalid_state", "invalid database state transition", requestID)
	}
	if errors.Is(err, catalog.ErrCrossProject) {
		return NewAPIError(http.StatusForbidden, "cross_project_denied", "cross project access denied", requestID)
	}
	if errors.Is(err, catalog.ErrDescriptorWrite) {
		return NewAPIError(http.StatusServiceUnavailable, "descriptor_write_failed", "failed to persist database descriptor", requestID)
	}
	if errors.Is(err, catalog.ErrMigrationFailed) {
		return NewAPIError(http.StatusServiceUnavailable, "migration_failed", "catalog migration failed", requestID)
	}
	if errors.Is(err, catalog.ErrSystemProtected) {
		return NewAPIError(http.StatusForbidden, "system_database_protected", "system database cannot be modified or deleted", requestID)
	}
	if errors.Is(err, catalog.ErrInvalidDataModel) {
		return NewAPIError(http.StatusBadRequest, "data_model_invalid", "data_model must be collection or sql", requestID)
	}
	if errors.Is(err, systemdb.ErrUnavailable) {
		return NewAPIError(http.StatusServiceUnavailable, "system_store_unavailable", "system database unavailable", requestID)
	}
	// database 运行时错误
	if errors.Is(err, godatabase.ErrDatabaseDeleting) {
		return NewAPIError(http.StatusConflict, "database_deleting", "database is being deleted", requestID)
	}
	if errors.Is(err, godatabase.ErrDatabaseNotReady) {
		return NewAPIError(http.StatusConflict, "database_not_ready", "database is not ready", requestID)
	}
	if errors.Is(err, godatabase.ErrWriterUnavailable) {
		return NewAPIError(http.StatusServiceUnavailable, "writer_unavailable", "this instance is not writable", requestID)
	}
	// 写租约获取在途（perf §1.5）：503 + Retry-After，客户端应稍后重试。
	if errors.Is(err, lease.ErrAcquiring) {
		return NewAPIError(http.StatusServiceUnavailable, "lease_acquiring", "write lease acquisition in progress; retry shortly", requestID)
	}
	if errors.Is(err, lease.ErrLeaseHeld) {
		return NewAPIError(http.StatusServiceUnavailable, "lease_held", "write lease held by another instance; retry shortly", requestID)
	}
	if errors.Is(err, godatabase.ErrRowLimitExceeded) {
		return NewAPIError(422, "row_limit_exceeded", "query row limit exceeded", requestID)
	}
	if errors.Is(err, godatabase.ErrRegistryClosed) {
		return NewAPIError(http.StatusServiceUnavailable, "registry_closed", "database registry is closed", requestID)
	}
	if errors.Is(err, godatabase.ErrUnsupportedValue) {
		return NewAPIError(http.StatusInternalServerError, "unsupported_value_type", "unsupported column value type", requestID)
	}
	if errors.Is(err, godatabase.ErrQueryConcurrency) {
		return NewAPIError(http.StatusTooManyRequests, "query_concurrency_exceeded", "query concurrency limit exceeded", requestID)
	}
	// sqlguard 错误
	if errors.Is(err, sqlguard.ErrEmptySQL) {
		return NewAPIError(http.StatusBadRequest, "empty_sql", "sql statement is empty", requestID)
	}
	// objectstore 文件 key 校验错误
	if errors.Is(err, objectstore.ErrInvalidKey) {
		return NewAPIError(http.StatusBadRequest, "invalid_file_key", "invalid file key", requestID)
	}
	if errors.Is(err, sqlguard.ErrMultipleStatements) {
		return NewAPIError(http.StatusBadRequest, "multiple_statements", "multiple statements are not allowed", requestID)
	}
	if errors.Is(err, sqlguard.ErrNulChar) {
		return NewAPIError(http.StatusBadRequest, "invalid_sql", "nul character not allowed in sql", requestID)
	}
	if errors.Is(err, sqlguard.ErrSQLNotAllowed) {
		return NewAPIError(http.StatusBadRequest, "sql_not_allowed", "sql statement not allowed", requestID)
	}
	if errors.Is(err, sqlguard.ErrWriteInReadOnly) {
		return NewAPIError(http.StatusBadRequest, "write_in_read_only", "write statement not allowed in read-only intent", requestID)
	}
	// kv 错误（key-value-ducklake-plan §5）
	if errors.Is(err, kv.ErrNotFound) {
		return NewAPIError(http.StatusNotFound, "kv_not_found", "key not found", requestID)
	}
	if errors.Is(err, kv.ErrKeyType) {
		return NewAPIError(http.StatusConflict, "kv_type_mismatch", "key holds a different type", requestID)
	}
	if errors.Is(err, kv.ErrKeyExists) {
		return NewAPIError(http.StatusConflict, "kv_key_exists", "key already exists", requestID)
	}
	if errors.Is(err, kv.ErrValueType) {
		return NewAPIError(http.StatusBadRequest, "kv_invalid_value", "value is not a valid number", requestID)
	}
	if errors.Is(err, kv.ErrArgument) {
		return NewAPIError(http.StatusBadRequest, "kv_invalid_argument", "invalid argument", requestID)
	}
	return nil
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
