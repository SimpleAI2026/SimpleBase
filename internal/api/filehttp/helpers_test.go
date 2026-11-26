package filehttp

// helpers_test.go：子包内测试的最小 HTTP 夹具（与 api 同包测试的 doRequest 同形）。

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func doRequest(e *echo.Echo, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// fakeUsage 是配额服务的最小假实现（与 api 同包测试同形）。
type fakeUsage struct {
	errLLM error
	errDB  error
	calls  []string
}

func (f *fakeUsage) CheckQuota(_ context.Context, _ string, kind string) error {
	f.calls = append(f.calls, kind)
	if kind == "llm" {
		return f.errLLM
	}
	return f.errDB
}

// fakeAudit 是审计服务的最小假实现（与 api 同包测试同形）。
type fakeAudit struct {
	ops    []catalog.Operation
	err    error
	last   httpkit.AuditEvent
	recErr error
}

func (f *fakeAudit) Record(_ context.Context, e httpkit.AuditEvent) error {
	f.last = e
	return f.recErr
}

func (f *fakeAudit) ListOperations(context.Context, string, string, int) ([]catalog.Operation, error) {
	return f.ops, f.err
}

// openSystemAPI 打开一个内存系统库（测试用；与 api 同包测试同形）。
func openSystemAPI(t *testing.T) *systemdb.Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return systemdb.NewStoreForTest(db)
}
