package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// B2-durability 表征（planv6.0 §8 B2）：durability 字段在响应路径上的选择语义
// ——nil 回退 committed_local、callback 值原样返回、空串保持空串、
// 每次响应恰好调用一次、失败路径不调用——作为抽取 helper 的等价证据。

// runSQL 便捷封装：注册带 :databaseID 路径参数的路由并发一次请求，返回 code 与 body。
func runSQL(t *testing.T, h *SQLHandler, method, path string, body any) (int, string) {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = WithPrincipal(ctx, auth.Principal{
				APIKeyID: "key-test", TenantID: "tenant-1",
				ProjectIDs:  map[string]struct{}{"proj-1": {}},
				Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}, auth.DatabaseWrite: {}, auth.DatabaseAdmin: {}},
			})
			ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.POST("/databases/:databaseID/e", h.Execute)
	e.POST("/databases/:databaseID/b", h.Batch)
	rec := doRequest(e, method, path, body)
	return rec.Code, rec.Body.String()
}

func TestSQLHandler_DurabilitySemantics(t *testing.T) {
	newSvc := func() *fakeSQLService {
		svc := &fakeSQLService{}
		svc.db = catalog.Database{ID: "u", ProjectID: "proj-1", Status: catalog.DatabaseReady}
		svc.lease = &fakeSQLLease{
			executeResult: godatabase.QueryResult{RowsAffected: 1},
			batchResults:  []godatabase.QueryResult{{RowsAffected: 1}},
		}
		return svc
	}
	execReq := func(h *SQLHandler) (int, string) {
		return runSQL(t, h, http.MethodPost, "/databases/u/e", ExecuteRequest{SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"}})
	}
	txReq := func(h *SQLHandler) (int, string) {
		return runSQL(t, h, http.MethodPost, "/databases/u/b", BatchRequest{Transactional: true, Statements: []SQLStatementRequest{{SQL: "INSERT INTO t VALUES(1)"}}})
	}

	// 1. callback 为 nil：响应默认 committed_local。
	{
		h := NewSQLHandler(newSvc(), testSQLLimits(), true)
		for name, fn := range map[string]func(*SQLHandler) (int, string){"exec": execReq, "tx": txReq} {
			code, body := fn(h)
			if code != http.StatusOK || !strings.Contains(body, `"durability":"committed_local"`) {
				t.Fatalf("%s nil callback: %d %s", name, code, body)
			}
		}
	}

	// 2. callback 返回自定义值：每次响应恰好调用一次，收到原 databaseID。
	{
		svc := newSvc()
		h := NewSQLHandler(svc, testSQLLimits(), true)
		calls := 0
		h.DurabilityFor = func(dbID string) string {
			calls++
			if dbID != "u" {
				t.Errorf("callback dbID=%q want u", dbID)
			}
			return "synced_s3"
		}
		for name, fn := range map[string]func(*SQLHandler) (int, string){"exec": execReq, "tx": txReq} {
			before := calls
			code, body := fn(h)
			if code != http.StatusOK || !strings.Contains(body, `"durability":"synced_s3"`) {
				t.Fatalf("%s custom callback: %d %s", name, code, body)
			}
			if calls != before+1 {
				t.Fatalf("%s durability calls=%d want exactly one more", name, calls-before)
			}
		}
	}

	// 3. callback 返回空串：响应保持空串，不做 committed_local 兜底。
	{
		h := NewSQLHandler(newSvc(), testSQLLimits(), true)
		h.DurabilityFor = func(string) string { return "" }
		code, body := execReq(h)
		if code != http.StatusOK {
			t.Fatalf("empty callback exec: %d %s", code, body)
		}
		var resp struct {
			Durability string `json:"durability"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("json: %v", err)
		}
		if resp.Durability != "" {
			t.Fatalf("empty callback durability=%q want empty", resp.Durability)
		}
	}

	// 4. 失败路径（GetDatabase 出错）不调用 callback。
	{
		svc := newSvc()
		svc.getErr = catalog.ErrNotFound
		h := NewSQLHandler(svc, testSQLLimits(), true)
		calls := 0
		h.DurabilityFor = func(string) string {
			calls++
			return "should-not-appear"
		}
		code, _ := execReq(h)
		if code != http.StatusNotFound {
			t.Fatalf("get err status=%d", code)
		}
		if calls != 0 {
			t.Fatalf("durability calls=%d want 0 on failed path", calls)
		}
	}
}
