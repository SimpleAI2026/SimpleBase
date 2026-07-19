package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
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
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
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
	// context 语义
	if errors.Is(err, context.Canceled) {
		return NewAPIError(http.StatusServiceUnavailable, "request_canceled", "request canceled", requestID)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewAPIError(http.StatusGatewayTimeout, "request_timeout", "request timed out", requestID)
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
	// database 运行时错误
	if errors.Is(err, database.ErrDatabaseDeleting) {
		return NewAPIError(http.StatusConflict, "database_deleting", "database is being deleted", requestID)
	}
	if errors.Is(err, database.ErrDatabaseNotReady) {
		return NewAPIError(http.StatusConflict, "database_not_ready", "database is not ready", requestID)
	}
	if errors.Is(err, database.ErrWriterUnavailable) {
		return NewAPIError(http.StatusServiceUnavailable, "writer_unavailable", "this instance is not writable", requestID)
	}
	if errors.Is(err, database.ErrRowLimitExceeded) {
		return NewAPIError(422, "row_limit_exceeded", "query row limit exceeded", requestID)
	}
	if errors.Is(err, database.ErrRegistryClosed) {
		return NewAPIError(http.StatusServiceUnavailable, "registry_closed", "database registry is closed", requestID)
	}
	if errors.Is(err, database.ErrUnsupportedValue) {
		return NewAPIError(http.StatusInternalServerError, "unsupported_value_type", "unsupported column value type", requestID)
	}
	if errors.Is(err, database.ErrQueryConcurrency) {
		return NewAPIError(http.StatusTooManyRequests, "query_concurrency_exceeded", "query concurrency limit exceeded", requestID)
	}
	// sqlguard 错误
	if errors.Is(err, sqlguard.ErrEmptySQL) {
		return NewAPIError(http.StatusBadRequest, "empty_sql", "sql statement is empty", requestID)
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
	return nil
}

// WriteError 是 handler 把任意 err 写成统一错误协议的入口。
func WriteError(c echo.Context, err error) error {
	if err == nil {
		return nil
	}
	rid := RequestIDFromContext(c.Request().Context())
	if ae := Error(err, rid); ae != nil {
		return c.JSON(ae.HTTPStatus, ae.Body)
	}
	// 未知错误：记录但不泄露细节
	return c.JSON(http.StatusInternalServerError, APIErrorBody{Error: APIErrorDetail{
		Code: "internal_error", Message: "internal error", RequestID: rid,
	}})
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
