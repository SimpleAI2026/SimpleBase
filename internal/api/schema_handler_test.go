package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
)

func setupSchemaRouter(t *testing.T, svc *fakeDataService, writable bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	h := NewSchemaHandler(svc, writable, testSQLLimits())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{
				APIKeyID: "key-test", TenantID: "tenant-1",
				ProjectIDs: map[string]struct{}{"proj-1": {}},
			}
			ctx := WithPrincipal(c.Request().Context(), p)
			ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	p := e.Group("/v1/projects/:projectID")
	p.GET("/databases/:databaseID/schema", h.ListSchema)
	p.GET("/databases/:databaseID/schema/tables/:table/rows", h.ListTableRows)
	p.POST("/databases/:databaseID/schema/tables", h.CreateTable)
	p.POST("/databases/:databaseID/schema/columns", h.AddColumn)
	return e
}

func TestSchemaListAndMutateSQLDatabase(t *testing.T) {
	lease := &fakeSQLLease{
		queryResult: database.QueryResult{
			Rows: [][]any{
				{"users", "id", "INTEGER", "YES"},
				{"users", "email", "VARCHAR", "NO"},
				{"users", "password_hash", "VARCHAR", "NO"},
				{"bad"},
			},
		},
	}
	svc := &fakeDataService{
		dbs:   []catalog.Database{{ID: "db-sql", ProjectID: "proj-1", Kind: catalog.DatabaseKindUser, DataModel: catalog.DataModelSQL}},
		lease: lease,
	}
	e := setupSchemaRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sql/schema", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"email"`) || !strings.Contains(rec.Body.String(), `"nullable":false`) {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"sensitive"`) {
		t.Fatalf("user schema must not mark credential columns: %s", rec.Body.String())
	}
	if svc.acquiredMode != database.ReadOnly {
		t.Fatalf("list mode %v", svc.acquiredMode)
	}

	rec = doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-sql/schema/tables", map[string]any{
		"name": "orders",
		"columns": []map[string]any{
			{"name": "id", "type": "integer", "nullable": false},
			{"name": "note", "type": "varchar"},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(lease.lastExecStmt.SQL, `CREATE TABLE "orders"`) || !strings.Contains(lease.lastExecStmt.SQL, `"id" INTEGER NOT NULL`) {
		t.Fatalf("create sql %s", lease.lastExecStmt.SQL)
	}

	rec = doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-sql/schema/columns", map[string]any{
		"table": "orders", "name": "total", "type": "DOUBLE",
	})
	if rec.Code != http.StatusCreated || !strings.Contains(lease.lastExecStmt.SQL, `ALTER TABLE "orders" ADD COLUMN "total" DOUBLE`) {
		t.Fatalf("add %d %s sql=%s", rec.Code, rec.Body.String(), lease.lastExecStmt.SQL)
	}
}

func TestSchemaRejectsCollectionSystemAndReadOnly(t *testing.T) {
	svc := &fakeDataService{dbs: []catalog.Database{
		{ID: "db-col", ProjectID: "proj-1", DataModel: catalog.DataModelCollection},
		{ID: "db-sys", ProjectID: "proj-1", Kind: catalog.DatabaseKindSystem, DataModel: catalog.DataModelSQL},
	}}
	e := setupSchemaRouter(t, svc, true)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-col/schema/tables", map[string]any{
		"name": "t", "columns": []map[string]any{{"name": "id", "type": "INTEGER"}},
	})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "data_model_mismatch") {
		t.Fatalf("collection %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-sys/schema/columns", map[string]any{
		"table": "t", "name": "id", "type": "INTEGER",
	})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "system_database_protected") {
		t.Fatalf("system %d %s", rec.Code, rec.Body.String())
	}

	ro := setupSchemaRouter(t, svc, false)
	rec = doRequest(ro, http.MethodPost, "/v1/projects/proj-1/databases/db-sql/schema/tables", map[string]any{
		"name": "t", "columns": []map[string]any{{"name": "id", "type": "INTEGER"}},
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readonly %d %s", rec.Code, rec.Body.String())
	}
}

func TestBuildSchemaSQLValidation(t *testing.T) {
	sqlText, err := buildCreateTableSQL("Users", []schemaColumnSpec{{Name: "id", Type: "integer", Nullable: true}})
	if err != nil || sqlText != `CREATE TABLE "Users" ("id" INTEGER)` {
		t.Fatalf("%s %v", sqlText, err)
	}
	if _, err := buildCreateTableSQL("1bad", []schemaColumnSpec{{Name: "id", Type: "INTEGER", Nullable: true}}); err == nil {
		t.Fatal("bad table name")
	}
	if _, err := buildAddColumnSQL("users", schemaColumnSpec{Name: "id", Type: "NOPE", Nullable: true}); err == nil {
		t.Fatal("bad type")
	}
	if _, err := buildCreateTableSQL("users", []schemaColumnSpec{
		{Name: "id", Type: "INTEGER", Nullable: true},
		{Name: "ID", Type: "INTEGER", Nullable: true},
	}); err == nil {
		t.Fatal("duplicate column")
	}
}

func TestSchemaListSystemDatabase(t *testing.T) {
	lease := &fakeSQLLease{queryResult: database.QueryResult{Rows: [][]any{
		{"sys_users", "id", "VARCHAR", "NO"},
		{"sys_users", "password_hash", "VARCHAR", "NO"},
		{"sys_users", "token_input", "BIGINT", "YES"},
		{"__internal", "secret", "VARCHAR", "YES"},
	}}}
	svc := &fakeDataService{dbs: []catalog.Database{{
		ID: "db-sys", ProjectID: "proj-1", Kind: catalog.DatabaseKindSystem, DataModel: catalog.DataModelCollection,
	}}, lease: lease}
	e := setupSchemaRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sys/schema", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"password_hash"`) || !strings.Contains(body, `"sensitive":true`) {
		t.Fatalf("sensitive column missing: %s", body)
	}
	if strings.Contains(body, "__internal") {
		t.Fatalf("internal table leaked: %s", body)
	}
	if strings.Contains(body, `"name":"token_input"`) && strings.Contains(body, `"name":"token_input","type":"BIGINT","nullable":true,"sensitive"`) {
		t.Fatalf("token_input must stay visible: %s", body)
	}
	if svc.acquiredMode != database.ReadOnly {
		t.Fatalf("mode %v", svc.acquiredMode)
	}
}

func TestSchemaTableRowsSystemAndUser(t *testing.T) {
	sysLease := &fakeSQLLease{queryQueue: []database.QueryResult{
		{Rows: [][]any{
			{"id", "VARCHAR", "NO"},
			{"password_hash", "VARCHAR", "NO"},
			{"created_at", "TIMESTAMP", "YES"},
		}},
		{Rows: [][]any{{int64(2)}}},
		{Rows: [][]any{{"u1", nil, "t"}}},
	}}
	sys := &fakeDataService{dbs: []catalog.Database{{
		ID: "db-sys", ProjectID: "proj-1", Kind: catalog.DatabaseKindSystem,
	}}, lease: sysLease}
	e := setupSchemaRouter(t, sys, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sys/schema/tables/sys_users/rows", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sensitive":true`) || !strings.Contains(rec.Body.String(), `"total":2`) {
		t.Fatalf("system rows %d %s", rec.Code, rec.Body.String())
	}
	if sys.acquiredMode != database.ReadOnly {
		t.Fatalf("mode %v", sys.acquiredMode)
	}
	var pageSQL string
	for _, stmt := range sysLease.queryStmts {
		if strings.Contains(stmt.SQL, "SELECT ") && strings.Contains(stmt.SQL, "LIMIT") {
			pageSQL = stmt.SQL
			if len(stmt.Args) != 2 || stmt.Args[0] != schemaRowsDefaultLimit || stmt.Args[1] != 0 {
				t.Fatalf("page args %#v", stmt.Args)
			}
		}
	}
	if !strings.Contains(pageSQL, `NULL AS "password_hash"`) || strings.Count(pageSQL, `"password_hash"`) != 1 {
		t.Fatalf("projection %s", pageSQL)
	}
	if !strings.Contains(pageSQL, `ORDER BY "created_at" DESC, "id" ASC`) {
		t.Fatalf("order %s", pageSQL)
	}

	userLease := &fakeSQLLease{queryQueue: []database.QueryResult{
		{Rows: [][]any{{"id", "INTEGER", "NO"}, {"password_hash", "VARCHAR", "YES"}}},
		{Rows: [][]any{{int64(1)}}},
		{Rows: [][]any{{int64(1), "hash"}}},
	}}
	user := &fakeDataService{dbs: []catalog.Database{{
		ID: "db-sql", ProjectID: "proj-1", Kind: catalog.DatabaseKindUser, DataModel: catalog.DataModelSQL,
	}}, lease: userLease}
	e = setupSchemaRouter(t, user, true)
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sql/schema/tables/users/rows?limit=10&offset=20", nil)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"sensitive"`) {
		t.Fatalf("user rows %d %s", rec.Code, rec.Body.String())
	}
	found := false
	for _, stmt := range userLease.queryStmts {
		if strings.Contains(stmt.SQL, "LIMIT") {
			found = true
			if strings.Contains(stmt.SQL, "NULL AS") {
				t.Fatalf("user projection redacted: %s", stmt.SQL)
			}
			if len(stmt.Args) != 2 || stmt.Args[0] != 10 || stmt.Args[1] != 20 {
				t.Fatalf("args %#v", stmt.Args)
			}
			if !strings.Contains(stmt.SQL, `ORDER BY "id" ASC`) {
				t.Fatalf("order %s", stmt.SQL)
			}
		}
	}
	if !found {
		t.Fatal("missing page sql")
	}
}

func TestSchemaTableRowsValidation(t *testing.T) {
	svc := &fakeDataService{dbs: []catalog.Database{
		{ID: "db-sql", ProjectID: "proj-1", DataModel: catalog.DataModelSQL},
		{ID: "db-col", ProjectID: "proj-1", DataModel: catalog.DataModelCollection},
	}, lease: &fakeSQLLease{queryResult: database.QueryResult{}}}
	e := setupSchemaRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-col/schema/tables/users/rows", nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "data_model_mismatch") {
		t.Fatalf("collection %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sql/schema/tables/1bad/rows", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad name %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sql/schema/tables/__secret/rows", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "table_not_found") {
		t.Fatalf("internal %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sql/schema/tables/users/rows?limit=0", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("limit %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sql/schema/tables/users/rows?offset=-1", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("offset %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sql/schema/tables/users/rows?offset=100001", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("offset cap %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sql/schema/tables/missing/rows", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "table_not_found") {
		t.Fatalf("missing %d %s", rec.Code, rec.Body.String())
	}
	if len(svc.lease.queryStmts) == 0 {
		t.Fatal("missing table should query information_schema")
	}
}

func TestSchemaRowsOrderFallback(t *testing.T) {
	sqlText, err := schemaRowsSelectSQL("events", []schemaColumnJSON{
		{Name: "name", Type: "VARCHAR"},
		{Name: "occurred_at", Type: "TIMESTAMP"},
		{Name: "id", Type: "VARCHAR"},
	}, false)
	if err != nil || !strings.Contains(sqlText, `ORDER BY "occurred_at" DESC, "id" ASC`) {
		t.Fatalf("%s %v", sqlText, err)
	}
	sqlText, err = schemaRowsSelectSQL("notes", []schemaColumnJSON{{Name: "body", Type: "VARCHAR"}}, false)
	if err != nil || !strings.Contains(sqlText, `ORDER BY "body" ASC`) {
		t.Fatalf("%s %v", sqlText, err)
	}
	if _, err := schemaRowsSelectSQL("t", nil, false); err == nil {
		t.Fatal("expected empty columns error")
	}
}
