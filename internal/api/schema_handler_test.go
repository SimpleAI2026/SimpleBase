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
	h := NewSchemaHandler(svc, writable)
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
