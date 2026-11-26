package datahttp

// helpers_test.go：子包内测试的最小 HTTP 夹具（与 api 同包测试的 doRequest 同形）。

import (
	"context"
	"database/sql"
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

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

// setupBareEcho 构造bare Echo 并按需注入主体 / 项目上下文（与 api 同包测试同形）。
func setupBareEcho(injectPrincipal, injectProject bool, register func(*echo.Echo)) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			if injectPrincipal {
				ctx = httpkit.WithPrincipal(ctx, auth.Principal{
					APIKeyID: "key-test", TenantID: "tenant-1",
					ProjectIDs:  map[string]struct{}{"proj-1": {}},
					Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}, auth.DatabaseWrite: {}, auth.DatabaseAdmin: {}},
				})
			}
			if injectProject {
				ctx = httpkit.WithProject(ctx, httpkit.ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			}
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	register(e)
	return e
}
