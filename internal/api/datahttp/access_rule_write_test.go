// 文件说明：access_rule_write_test.go — 行级规则在创建 / 更新 / 删除路径上的强制测试。
//
// 用法
//
//	go test ./internal/api/datahttp -run 'TestCreateDocumentStampsOwner|TestUpdateAndDeleteRejectOtherOwner'
//
// 调用架构
//
//	[上游] 无（同包测试）
//	[本文件] internal/api/datahttp/access_rule_write_test.go
//	[下游] data_handler.go、access_rule.go
//
// 主要符号
//
//	ruleWriteRouter
//	TestCreateDocumentStampsOwner
//	TestUpdateAndDeleteRejectOtherOwner

package datahttp

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

func ruleWriteRouter(t *testing.T, svc *fakeDataService, rule string) *echo.Echo {
	t.Helper()
	h := NewDataHandler(svc, true).WithAccessRules(func(ctx context.Context, projectID, key string) (string, error) {
		return rule, nil
	}, nil)
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{
				APIKeyID:    "key-test",
				TenantID:    "tenant-1",
				ProjectIDs:  map[string]struct{}{"proj-1": {}},
				Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}, auth.DatabaseWrite: {}},
			}
			ctx := httpkit.WithPrincipal(c.Request().Context(), p)
			ctx = httpkit.WithProject(ctx, httpkit.ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.POST("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents", h.CreateDocument)
	e.POST("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents/batch", h.BatchDocuments)
	e.GET("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection", h.ListDocuments)
	e.POST("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection/import", h.Import)
	e.PUT("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents/:id", h.UpdateDocument)
	e.DELETE("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection/documents/:id", h.DeleteDocument)
	return e
}

func newRuleWriteService() *fakeDataService {
	return &fakeDataService{
		dbs:   []catalog.Database{{ID: "db-1", Name: "main", ProjectID: "proj-1", DatabasePath: "test.duckdb"}},
		lease: &fakeSQLLease{},
	}
}

func TestCreateDocumentStampsOwner(t *testing.T) {
	svc := newRuleWriteService()
	e := ruleWriteRouter(t, svc, `{"mode":"owner_write"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/databases/db-1/data/collections/todos/documents", strings.NewReader(`{"name":"a"}`))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"owner_id":"key:key-test"`) {
		t.Fatalf("owner not stamped: %s", rec.Body.String())
	}
}

func TestUpdateAndDeleteRejectOtherOwner(t *testing.T) {
	row := []any{"d1", `{"name":"a","owner_id":"key:other"}`, "2026-11-26T00:00:00Z", "2026-11-26T00:00:00Z"}

	t.Run("update", func(t *testing.T) {
		svc := newRuleWriteService()
		svc.lease.queryResult = mustQueryResult(t, row)
		e := ruleWriteRouter(t, svc, `{"mode":"owner_write"}`)
		req := httptest.NewRequest(http.MethodPut, "/v1/projects/proj-1/databases/db-1/data/collections/todos/documents/d1", strings.NewReader(`{"name":"b"}`))
		req.Header.Set(echo.HeaderContentType, "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "forbidden_row") {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("delete", func(t *testing.T) {
		svc := newRuleWriteService()
		svc.lease.queryResult = mustQueryResult(t, row)
		e := ruleWriteRouter(t, svc, `{"mode":"owner_write"}`)
		req := httptest.NewRequest(http.MethodDelete, "/v1/projects/proj-1/databases/db-1/data/collections/todos/documents/d1", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "forbidden_row") {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestListDocumentsFailsClosedOnInvalidRule(t *testing.T) {
	svc := newRuleWriteService()
	e := ruleWriteRouter(t, svc, `{"mode":"nope"}`)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections/todos", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "access_rule_invalid") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOffRuleDoesNotFilter(t *testing.T) {
	svc := newRuleWriteService()
	e := ruleWriteRouter(t, svc, "")
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections/todos", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	selectStmt := svc.lease.queryStmts[len(svc.lease.queryStmts)-1]
	if strings.Contains(selectStmt.SQL, "json_extract_string") {
		t.Fatalf("off rule must not filter: %s", selectStmt.SQL)
	}
}

func TestBatchDocumentsStampsOwner(t *testing.T) {
	svc := newRuleWriteService()
	e := ruleWriteRouter(t, svc, `{"mode":"owner_read"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/databases/db-1/data/collections/todos/documents/batch", strings.NewReader(`{"documents":[{"name":"a"},{"name":"b"}]}`))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(svc.lease.lastBatchStmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(svc.lease.lastBatchStmts))
	}
	for _, stmt := range svc.lease.lastBatchStmts {
		body, _ := stmt.Args[1].(string)
		if !strings.Contains(body, `"owner_id":"key:key-test"`) {
			t.Fatalf("owner not stamped: %s", body)
		}
	}
}

func TestImportStampsOwner(t *testing.T) {
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`CREATE TABLE todos (id TEXT PRIMARY KEY, data TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	svc := newRuleWriteService()
	svc.lease.raw = raw
	e := ruleWriteRouter(t, svc, `{"mode":"owner_write"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/databases/db-1/data/collections/todos/import", strings.NewReader(`{"name":"a","owner_id":"key:other"}`+"\n"))
	req.Header.Set(echo.HeaderContentType, "application/x-ndjson")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var data string
	if err := raw.QueryRow(`SELECT data FROM todos`).Scan(&data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, `"owner_id":"key:key-test"`) {
		t.Fatalf("import must stamp owner: %s", data)
	}
}

func mustQueryResult(t *testing.T, row []any) godatabase.QueryResult {
	t.Helper()
	return godatabase.QueryResult{Rows: [][]any{row}}
}
