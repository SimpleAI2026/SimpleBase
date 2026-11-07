package api

// r2_error_mapping_test.go：planv6.0 R2-B3 表征测试（提取前先行）。
// 从旧实现逐条快照 Error() 的哨兵错误映射（59 条：sandbox 11 + 其余 48），
// 并锁定「匹配顺序 = 优先级」：同时命中多个哨兵时以源码靠前者为准。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/kv"
	"github.com/linkxzhou/SimpleBase/godatabase/lease"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
	"github.com/linkxzhou/SimpleBase/gosandbox/vm"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/sandbox"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

var r2ErrorSnapshot = []struct {
	err     error
	status  int
	code    string
	message string
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
	{ErrProjectMissing, http.StatusBadRequest, "invalid_request", "project context missing"},
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
	{godatabase.ErrDatabaseDeleting, http.StatusConflict, "database_deleting", "database is being deleted"},
	{godatabase.ErrDatabaseNotReady, http.StatusConflict, "database_not_ready", "database is not ready"},
	{godatabase.ErrWriterUnavailable, http.StatusServiceUnavailable, "writer_unavailable", "this instance is not writable"},
	{lease.ErrAcquiring, http.StatusServiceUnavailable, "lease_acquiring", "write lease acquisition in progress; retry shortly"},
	{lease.ErrLeaseHeld, http.StatusServiceUnavailable, "lease_held", "write lease held by another instance; retry shortly"},
	{godatabase.ErrRowLimitExceeded, 422, "row_limit_exceeded", "query row limit exceeded"},
	{godatabase.ErrRegistryClosed, http.StatusServiceUnavailable, "registry_closed", "database registry is closed"},
	{godatabase.ErrUnsupportedValue, http.StatusInternalServerError, "unsupported_value_type", "unsupported column value type"},
	{godatabase.ErrQueryConcurrency, http.StatusTooManyRequests, "query_concurrency_exceeded", "query concurrency limit exceeded"},
	{sqlguard.ErrEmptySQL, http.StatusBadRequest, "empty_sql", "sql statement is empty"},
	{objectstore.ErrInvalidKey, http.StatusBadRequest, "invalid_file_key", "invalid file key"},
	{sqlguard.ErrMultipleStatements, http.StatusBadRequest, "multiple_statements", "multiple statements are not allowed"},
	{sqlguard.ErrNulChar, http.StatusBadRequest, "invalid_sql", "nul character not allowed in sql"},
	{sqlguard.ErrSQLNotAllowed, http.StatusBadRequest, "sql_not_allowed", "sql statement not allowed"},
	{sqlguard.ErrWriteInReadOnly, http.StatusBadRequest, "write_in_read_only", "write statement not allowed in read-only intent"},
	{kv.ErrNotFound, http.StatusNotFound, "kv_not_found", "key not found"},
	{kv.ErrKeyType, http.StatusConflict, "kv_type_mismatch", "key holds a different type"},
	{kv.ErrKeyExists, http.StatusConflict, "kv_key_exists", "key already exists"},
	{kv.ErrValueType, http.StatusBadRequest, "kv_invalid_value", "value is not a valid number"},
	{kv.ErrArgument, http.StatusBadRequest, "kv_invalid_argument", "invalid argument"},
}

func TestR2_ErrorSentinelMappingSnapshot(t *testing.T) {
	if len(r2ErrorSnapshot) != 59 {
		t.Fatalf("snapshot size=%d", len(r2ErrorSnapshot))
	}
	for _, tc := range r2ErrorSnapshot {
		for _, err := range []error{tc.err, fmt.Errorf("wrap: %w", tc.err)} {
			got := Error(err, "rid")
			if got == nil {
				t.Fatalf("%v: unmapped", err)
			}
			if got.HTTPStatus != tc.status || got.Body.Error.Code != tc.code || got.Body.Error.Message != tc.message || got.Body.Error.RequestID != "rid" {
				t.Errorf("%v: got %d %+v, want %d %s %q", err, got.HTTPStatus, got.Body.Error, tc.status, tc.code, tc.message)
			}
		}
	}
}

// 多哨兵同时命中时，源码顺序靠前者胜出（表驱动改造不得改变）。
func TestR2_ErrorMappingPriority(t *testing.T) {
	for i := 0; i < len(r2ErrorSnapshot); i++ {
		for j := i + 1; j < len(r2ErrorSnapshot); j++ {
			a, b := r2ErrorSnapshot[i], r2ErrorSnapshot[j]
			if errors.Is(a.err, b.err) || errors.Is(b.err, a.err) {
				continue
			}
			got := Error(errors.Join(b.err, a.err), "")
			if got == nil || got.Body.Error.Code != a.code || got.HTTPStatus != a.status {
				t.Fatalf("join(%v,%v): got %+v, want earlier %s", b.err, a.err, got, a.code)
			}
		}
	}
	// 前置分支优先于表：context 与 ErrProjectMissing 的相对位置。
	if got := Error(errors.Join(catalog.ErrNotFound, context.Canceled), ""); got.Body.Error.Code != "request_canceled" {
		t.Fatalf("context must win: %+v", got)
	}
	if got := Error(errors.Join(auth.ErrForbidden, ErrProjectMissing), ""); got.Body.Error.Code != "invalid_request" {
		t.Fatalf("project missing must precede auth: %+v", got)
	}
	if got := Error(errors.New("unknown"), ""); got != nil {
		t.Fatalf("unknown must be nil: %+v", got)
	}
}
