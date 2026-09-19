package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func TestAPIError_Error(t *testing.T) {
	e := NewAPIError(http.StatusBadRequest, "invalid_request", "bad body", "rid-1")
	if got := e.Error(); got != "invalid_request: bad body" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestError_Nil(t *testing.T) {
	if Error(nil, "rid") != nil {
		t.Fatal("expected nil for nil error")
	}
}

func TestError_DomainMapping(t *testing.T) {
	rid := "req-map"
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"canceled", context.Canceled, http.StatusServiceUnavailable, "request_canceled"},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout, "request_timeout"},
		{"wrapped_canceled", fmt.Errorf("wrap: %w", context.Canceled), http.StatusServiceUnavailable, "request_canceled"},
		{"missing_creds", auth.ErrMissingCredentials, http.StatusUnauthorized, "unauthenticated"},
		{"invalid_key", auth.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_api_key"},
		{"revoked", auth.ErrKeyRevoked, http.StatusUnauthorized, "api_key_revoked"},
		{"forbidden", auth.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"catalog_not_found", catalog.ErrNotFound, http.StatusNotFound, "database_not_found"},
		{"already_exists", catalog.ErrAlreadyExists, http.StatusConflict, "database_already_exists"},
		{"invalid_name", catalog.ErrInvalidName, http.StatusBadRequest, "invalid_database_name"},
		{"invalid_state", catalog.ErrInvalidState, http.StatusConflict, "invalid_state"},
		{"cross_project", catalog.ErrCrossProject, http.StatusForbidden, "cross_project_denied"},
		{"descriptor", catalog.ErrDescriptorWrite, http.StatusServiceUnavailable, "descriptor_write_failed"},
		{"migration", catalog.ErrMigrationFailed, http.StatusServiceUnavailable, "migration_failed"},
		{"system_protected", catalog.ErrSystemProtected, http.StatusForbidden, "system_database_protected"},
		{"system_unavailable", systemdb.ErrUnavailable, http.StatusServiceUnavailable, "system_store_unavailable"},
		{"db_deleting", database.ErrDatabaseDeleting, http.StatusConflict, "database_deleting"},
		{"db_not_ready", database.ErrDatabaseNotReady, http.StatusConflict, "database_not_ready"},
		{"writer", database.ErrWriterUnavailable, http.StatusServiceUnavailable, "writer_unavailable"},
		{"row_limit", database.ErrRowLimitExceeded, 422, "row_limit_exceeded"},
		{"registry_closed", database.ErrRegistryClosed, http.StatusServiceUnavailable, "registry_closed"},
		{"unsupported", database.ErrUnsupportedValue, http.StatusInternalServerError, "unsupported_value_type"},
		{"concurrency", database.ErrQueryConcurrency, http.StatusTooManyRequests, "query_concurrency_exceeded"},
		{"empty_sql", sqlguard.ErrEmptySQL, http.StatusBadRequest, "empty_sql"},
		{"invalid_key", objectstore.ErrInvalidKey, http.StatusBadRequest, "invalid_file_key"},
		{"multi_stmt", sqlguard.ErrMultipleStatements, http.StatusBadRequest, "multiple_statements"},
		{"nul", sqlguard.ErrNulChar, http.StatusBadRequest, "invalid_sql"},
		{"sql_denied", sqlguard.ErrSQLNotAllowed, http.StatusBadRequest, "sql_not_allowed"},
		{"write_ro", sqlguard.ErrWriteInReadOnly, http.StatusBadRequest, "write_in_read_only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ae := Error(tc.err, rid)
			if ae == nil {
				t.Fatalf("expected mapping for %v", tc.err)
			}
			if ae.HTTPStatus != tc.status {
				t.Fatalf("status=%d want %d", ae.HTTPStatus, tc.status)
			}
			if ae.Body.Error.Code != tc.code {
				t.Fatalf("code=%q want %q", ae.Body.Error.Code, tc.code)
			}
			if ae.Body.Error.RequestID != rid {
				t.Fatalf("request_id=%q", ae.Body.Error.RequestID)
			}
		})
	}
}

func TestError_UnknownReturnsNil(t *testing.T) {
	if ae := Error(errors.New("mystery"), "rid"); ae != nil {
		t.Fatalf("expected nil, got %+v", ae)
	}
}

func TestError_APIErrorReuse(t *testing.T) {
	existing := NewAPIError(http.StatusTeapot, "teapot", "short and stout", "")
	got := Error(existing, "filled")
	if got != existing {
		t.Fatal("expected same *APIError pointer")
	}
	if got.Body.Error.RequestID != "filled" {
		t.Fatalf("request_id filled = %q", got.Body.Error.RequestID)
	}
	// already has request id: keep it
	got2 := Error(existing, "other")
	if got2.Body.Error.RequestID != "filled" {
		t.Fatalf("should keep existing request id, got %q", got2.Body.Error.RequestID)
	}
}

func TestError_EchoHTTPError(t *testing.T) {
	he := echo.NewHTTPError(http.StatusNotFound, "nope")
	ae := Error(he, "rid-e")
	if ae == nil || ae.HTTPStatus != http.StatusNotFound || ae.Body.Error.Code != "not_found" {
		t.Fatalf("unexpected %+v", ae)
	}
	if ae.Body.Error.Message != "nope" {
		t.Fatalf("message=%q", ae.Body.Error.Message)
	}
}

func TestMapEchoError(t *testing.T) {
	if mapEchoError(nil, "r") != nil {
		t.Fatal("nil should map to nil")
	}
	if mapEchoError(errors.New("x"), "r") != nil {
		t.Fatal("non-http error should be nil")
	}
	wrapped := fmt.Errorf("wrap: %w", echo.NewHTTPError(http.StatusConflict, "dup"))
	ae := mapEchoError(wrapped, "rid")
	if ae == nil || ae.HTTPStatus != http.StatusConflict || ae.Body.Error.Code != "conflict" {
		t.Fatalf("wrapped http error: %+v", ae)
	}
}

func TestHTTPStatusToCode(t *testing.T) {
	cases := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, "invalid_request"},
		{http.StatusUnauthorized, "unauthenticated"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "not_found"},
		{http.StatusConflict, "conflict"},
		{http.StatusRequestEntityTooLarge, "request_too_large"},
		{http.StatusTooManyRequests, "rate_limited"},
		{http.StatusServiceUnavailable, "service_unavailable"},
		{http.StatusGatewayTimeout, "gateway_timeout"},
		{http.StatusTeapot, "http_error"},
		{599, "http_error"},
	}
	for _, tc := range cases {
		if got := httpStatusToCode(tc.status); got != tc.code {
			t.Errorf("status %d: got %q want %q", tc.status, got, tc.code)
		}
	}
}

func TestWriteError_NilKnownUnknown(t *testing.T) {
	e := echo.New()
	e.HideBanner = true

	t.Run("nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := WriteError(c, nil); err != nil {
			t.Fatalf("nil err: %v", err)
		}
		if rec.Code != http.StatusOK && rec.Body.Len() != 0 {
			t.Fatalf("nil write should not emit body: %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("known", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(WithRequestID(req.Context(), "rid-known"))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := WriteError(c, auth.ErrForbidden); err != nil {
			t.Fatalf("write: %v", err)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"forbidden"`) {
			t.Fatalf("body=%s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "rid-known") {
			t.Fatalf("missing request id: %s", rec.Body.String())
		}
	})

	t.Run("unknown", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(WithRequestID(req.Context(), "rid-unk"))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := WriteError(c, errors.New("dsn=secret")); err != nil {
			t.Fatalf("write: %v", err)
		}
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "secret") {
			t.Fatalf("leaked: %s", rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "internal_error") {
			t.Fatalf("body=%s", rec.Body.String())
		}
	})
}

func TestSafeMessage(t *testing.T) {
	if got := safeMessage(&echo.HTTPError{}); got != "error" {
		t.Fatalf("nil message: %q", got)
	}
	if got := safeMessage(&echo.HTTPError{Message: "hello"}); got != "hello" {
		t.Fatalf("string message: %q", got)
	}
	if got := safeMessage(&echo.HTTPError{Message: 42}); got != "error" {
		t.Fatalf("non-string: %q", got)
	}
}
