// 文件说明：access_rule_test.go — 集合行级规则的读取、SQL 注入与管理员绕过测试。
//
// 用法
//
//	go test ./internal/api/datahttp -run 'TestRuleScope|TestParseCollectionQueryInjectsOwner|TestListDocumentsAppliesOwnerFilter'
//
// 调用架构
//
//	[上游] 无（同包测试）
//	[本文件] internal/api/datahttp/access_rule_test.go
//	[下游] access_rule.go、collection_query.go、data_handler.go
//
// 主要符号
//
//	TestRuleScopeReadsProjectSetting
//	TestParseCollectionQueryInjectsOwner
//	TestListDocumentsAppliesOwnerFilter

package datahttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

func ruleContext(t *testing.T, actor string, admin bool, target string) echo.Context {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	perms := map[auth.Permission]struct{}{auth.DatabaseRead: {}, auth.DatabaseWrite: {}}
	if admin {
		perms[auth.DatabaseAdmin] = struct{}{}
	}
	p := auth.Principal{
		APIKeyID:    actor,
		TenantID:    "tenant-1",
		ProjectIDs:  map[string]struct{}{"proj-1": {}},
		Permissions: perms,
	}
	ctx := httpkit.WithPrincipal(req.Context(), p)
	ctx = httpkit.WithProject(ctx, httpkit.ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
	c := e.NewContext(req.WithContext(ctx), httptest.NewRecorder())
	c.SetParamNames("projectID", "databaseID", "collection")
	c.SetParamValues("proj-1", "db-1", "todos")
	return c
}

func TestRuleScopeReadsProjectSetting(t *testing.T) {
	h := NewDataHandler(nil, true).WithAccessRules(func(ctx context.Context, projectID, key string) (string, error) {
		if projectID != "proj-1" || key != "collection.access.todos" {
			t.Fatalf("unexpected setting lookup: %s %s", projectID, key)
		}
		return `{"mode":"owner_read","owner_field":"user_id"}`, nil
	}, nil)

	rule, actor, admin, err := h.ruleScope(ruleContext(t, "key-test", false, "/v1/projects/proj-1/databases/db-1/data/collections/todos"), "todos")
	if err != nil {
		t.Fatalf("ruleScope: %v", err)
	}
	if rule.Mode != collection.AccessOwnerRead || rule.OwnerField != "user_id" {
		t.Fatalf("unexpected rule: %+v", rule)
	}
	if actor != "key:key-test" || admin {
		t.Fatalf("unexpected actor/admin: %s %v", actor, admin)
	}
}

func TestParseCollectionQueryInjectsOwner(t *testing.T) {
	c := ruleContext(t, "key-test", false, "/v1/projects/proj-1/databases/db-1/data/collections/todos?count=1")
	dialect, err := godatabase.DialectFor(catalog.EngineDuckDB)
	if err != nil {
		t.Fatal(err)
	}
	q, err := parseCollectionQuery(c, "todos", nil, dialect, 50000, "json_extract_string(data, '$.owner_id') = ?", []any{"key:key-test"})
	if err != nil {
		t.Fatalf("parseCollectionQuery: %v", err)
	}
	if !strings.Contains(q.SQL, "json_extract_string(data, '$.owner_id') = ?") {
		t.Fatalf("owner condition missing: %s", q.SQL)
	}
	if len(q.Args) != 1 || q.Args[0] != "key:key-test" {
		t.Fatalf("unexpected args: %+v", q.Args)
	}
	if q.CountSQL == "" || len(q.CountArgs) != 1 || q.CountArgs[0] != "key:key-test" {
		t.Fatalf("count must include owner filter: %s %+v", q.CountSQL, q.CountArgs)
	}
}

func TestListDocumentsAppliesOwnerFilter(t *testing.T) {
	svc := &fakeDataService{
		dbs:   []catalog.Database{{ID: "db-1", Name: "main", ProjectID: "proj-1", DatabasePath: "test.duckdb"}},
		lease: &fakeSQLLease{},
	}
	h := NewDataHandler(svc, true).WithAccessRules(func(ctx context.Context, projectID, key string) (string, error) {
		return `{"mode":"owner_write"}`, nil
	}, nil)
	e := echo.New()
	e.HideBanner = true
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
	e.GET("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection", h.ListDocuments)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections/todos", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(svc.lease.queryStmts) < 2 {
		t.Fatalf("expected scan + select queries, got %d", len(svc.lease.queryStmts))
	}
	selectStmt := svc.lease.queryStmts[len(svc.lease.queryStmts)-1]
	if !strings.Contains(selectStmt.SQL, "json_extract_string(data, '$.owner_id') = ?") {
		t.Fatalf("owner filter missing: %s", selectStmt.SQL)
	}
	if len(selectStmt.Args) != 1 || selectStmt.Args[0] != "key:key-test" {
		t.Fatalf("unexpected args: %+v", selectStmt.Args)
	}
}

func TestListDocumentsAdminBypassesOwnerFilter(t *testing.T) {
	svc := &fakeDataService{
		dbs:   []catalog.Database{{ID: "db-1", Name: "main", ProjectID: "proj-1", DatabasePath: "test.duckdb"}},
		lease: &fakeSQLLease{},
	}
	h := NewDataHandler(svc, true).WithAccessRules(func(ctx context.Context, projectID, key string) (string, error) {
		return `{"mode":"owner_write"}`, nil
	}, nil)
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{
				APIKeyID:    "admin-key",
				TenantID:    "tenant-1",
				ProjectIDs:  map[string]struct{}{"proj-1": {}},
				Permissions: map[auth.Permission]struct{}{auth.DatabaseAdmin: {}},
			}
			ctx := httpkit.WithPrincipal(c.Request().Context(), p)
			ctx = httpkit.WithProject(ctx, httpkit.ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.GET("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection", h.ListDocuments)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections/todos", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	selectStmt := svc.lease.queryStmts[len(svc.lease.queryStmts)-1]
	if strings.Contains(selectStmt.SQL, "json_extract_string") {
		t.Fatalf("admin must bypass owner filter: %s", selectStmt.SQL)
	}
}
