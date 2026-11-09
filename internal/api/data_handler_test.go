package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

type fakeDataService struct {
	dbs          []catalog.Database
	listErr      error
	getErr       error
	acquireErr   error
	lastGetID    string
	listCalled   bool
	acquiredMode godatabase.AccessMode
	lease        *fakeSQLLease
}

func (f *fakeDataService) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	f.lastGetID = databaseID
	if f.getErr != nil {
		return catalog.Database{}, f.getErr
	}
	for _, db := range f.dbs {
		if db.ID == databaseID {
			return db, nil
		}
	}
	return catalog.Database{}, catalog.ErrNotFound
}

func (f *fakeDataService) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error) {
	f.listCalled = true
	if f.listErr != nil {
		return nil, "", f.listErr
	}
	if page.Limit > 0 && page.Limit < len(f.dbs) {
		return f.dbs[:page.Limit], "", nil
	}
	return f.dbs, "", nil
}

func (f *fakeDataService) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error) {
	if f.acquireErr != nil {
		return nil, f.acquireErr
	}
	f.acquiredMode = mode
	if f.lease == nil {
		f.lease = &fakeSQLLease{}
	}
	return f.lease, nil
}

func setupDataTestRouter(t *testing.T, svc *fakeDataService, writable bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	h := NewDataHandler(svc, writable)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{
				APIKeyID:   "key-test",
				TenantID:   "tenant-1",
				ProjectIDs: map[string]struct{}{"proj-1": {}},
				Permissions: map[auth.Permission]struct{}{
					auth.DatabaseRead:  {},
					auth.DatabaseWrite: {},
				},
			}
			ctx := WithPrincipal(c.Request().Context(), p)
			ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	p := e.Group("/v1/projects/:projectID")
	p.GET("/databases/:databaseID/data/collections", h.ListCollections)
	p.POST("/databases/:databaseID/data/collections", h.CreateCollection)
	p.GET("/databases/:databaseID/data/collections/:collection", h.ListDocuments)
	p.POST("/databases/:databaseID/data/collections/:collection/documents", h.CreateDocument)
	return e
}

func TestDataListCollections_UsesPathDatabaseID(t *testing.T) {
	svc := &fakeDataService{
		dbs: []catalog.Database{
			{ID: "db-first", Name: "first", ProjectID: "proj-1"},
			{ID: "db-target", Name: "target", ProjectID: "proj-1"},
		},
		lease: &fakeSQLLease{
			// 第一次查询返回「表 + 四列是否存在」，之后每张表各一次 COUNT(*)。
			queryQueue: []godatabase.QueryResult{
				{
					Columns: []string{"table_name", "has_id", "has_data", "has_created_at", "has_updated_at"},
					Rows: [][]any{
						{"users", int64(1), int64(1), int64(1), int64(1)},
						{"orders", int64(1), int64(1), int64(1), int64(1)},
					},
				},
				{Rows: [][]any{{int64(0)}}},
				{Rows: [][]any{{int64(0)}}},
			},
		},
	}
	e := setupDataTestRouter(t, svc, true)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/databases/db-target/data/collections", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if svc.listCalled {
		t.Fatal("database-scoped list must not call ListDatabases (first-DB fallback)")
	}
	if svc.lastGetID != "db-target" {
		t.Fatalf("GetDatabase id = %q, want db-target", svc.lastGetID)
	}
	var body collectionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Collections) != 2 || body.Collections[0] != "users" {
		t.Fatalf("collections = %#v", body.Collections)
	}
}

func TestDataListCollections_HidesInternalAndNonDocumentTables(t *testing.T) {
	svc := &fakeDataService{
		dbs: []catalog.Database{{ID: "db-1", ProjectID: "proj-1", DataModel: catalog.DataModelCollection}},
		lease: &fakeSQLLease{queryQueue: []godatabase.QueryResult{
			{
				Columns: []string{"table_name", "has_id", "has_data", "has_created_at", "has_updated_at"},
				Rows: [][]any{
					// dev apply 的记账表：名字不合法且不是文档形，两条规则都应挡住。
					{"_migrations", int64(0), int64(0), int64(0), int64(0)},
					// __ 前缀内部表（KV）不向用户暴露。
					{"__sb_kv_strings", int64(1), int64(1), int64(1), int64(1)},
					// 原生 SQL 迁移建的业务表：缺 data 列，文档查询会 500。
					{"audit_log", int64(1), int64(0), int64(1), int64(1)},
					// 缺 updated_at 同样会让固定四列查询失败。
					{"legacy", int64(1), int64(1), int64(1), int64(0)},
					// 合法文档形表：保留。
					{"todos", int64(1), int64(1), int64(1), int64(1)},
				},
			},
			{Rows: [][]any{{int64(7)}}},
		}},
	}
	e := setupDataTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body collectionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Collections) != 1 || body.Collections[0] != "todos" {
		t.Fatalf("collections = %#v, want [todos]", body.Collections)
	}
	if len(body.Items) != 1 || body.Items[0].DocumentCount != 7 {
		t.Fatalf("items = %#v", body.Items)
	}
}

func TestDataListDocuments_InternalTableNameRejected(t *testing.T) {
	svc := &fakeDataService{
		dbs:   []catalog.Database{{ID: "db-1", ProjectID: "proj-1", DataModel: catalog.DataModelCollection}},
		lease: &fakeSQLLease{},
	}
	e := setupDataTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections/_migrations", nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(svc.lease.queryStmts) != 0 {
		t.Fatalf("invalid collection name must not reach the engine: %#v", svc.lease.queryStmts)
	}
}

func TestDataListCollections_SQLDatabaseRejected(t *testing.T) {
	svc := &fakeDataService{
		dbs: []catalog.Database{{ID: "db-sql", ProjectID: "proj-1", DataModel: catalog.DataModelSQL}},
	}
	e := setupDataTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sql/data/collections", nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "data_model_mismatch") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.acquiredMode != 0 {
		t.Fatal("sql database must not acquire a collection lease")
	}
}

func TestDataCreateCollection_CollectionDatabaseStillWorks(t *testing.T) {
	svc := &fakeDataService{
		dbs:   []catalog.Database{{ID: "db-1", ProjectID: "proj-1", DataModel: catalog.DataModelCollection}},
		lease: &fakeSQLLease{},
	}
	e := setupDataTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/data/collections", map[string]any{"name": "users"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.acquiredMode != godatabase.ReadWrite {
		t.Fatalf("mode=%v", svc.acquiredMode)
	}
}

func TestDataCreateCollection_UnknownDatabaseID(t *testing.T) {
	svc := &fakeDataService{
		dbs: []catalog.Database{{ID: "db-first", Name: "first", ProjectID: "proj-1"}},
	}
	e := setupDataTestRouter(t, svc, true)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/databases/db-missing/data/collections", strings.NewReader(`{"name":"users"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
	if svc.listCalled {
		t.Fatal("missing databaseID must not fall back to first DB")
	}
	if svc.lastGetID != "db-missing" {
		t.Fatalf("GetDatabase id = %q, want db-missing", svc.lastGetID)
	}
}

func TestDataCreateDocument_ScopedToPathDatabase(t *testing.T) {
	svc := &fakeDataService{
		dbs: []catalog.Database{
			{ID: "db-first", Name: "first", ProjectID: "proj-1"},
			{ID: "db-target", Name: "target", ProjectID: "proj-1"},
		},
		lease: &fakeSQLLease{
			executeResult: godatabase.QueryResult{RowsAffected: 1},
		},
	}
	e := setupDataTestRouter(t, svc, true)
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/projects/proj-1/databases/db-target/data/collections/users/documents",
		strings.NewReader(`{"name":"Ada"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	if svc.listCalled {
		t.Fatal("create document with databaseID must not use first-DB fallback")
	}
	if svc.lastGetID != "db-target" {
		t.Fatalf("GetDatabase id = %q, want db-target", svc.lastGetID)
	}
	if svc.acquiredMode != godatabase.ReadWrite {
		t.Fatalf("mode = %v, want ReadWrite", svc.acquiredMode)
	}
}

func TestDataAcquireGetError(t *testing.T) {
	svc := &fakeDataService{getErr: catalog.ErrNotFound, dbs: []catalog.Database{{ID: "db-1", ProjectID: "proj-1"}}}
	e := setupDataTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
	svc.getErr = nil
	svc.acquireErr = godatabase.ErrWriterUnavailable
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("acquire %d", rec.Code)
	}

	svc.acquireErr = catalog.ErrInvalidDatabasePath
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections", nil)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "invalid_database_path") || strings.Contains(rec.Body.String(), "invalid_database_name") {
		t.Fatalf("invalid storage metadata %d %s", rec.Code, rec.Body.String())
	}

	svc.acquireErr = nil
	svc.getErr = catalog.ErrNotFound
}

func TestCreateCollectionExecuteError(t *testing.T) {
	svc := &fakeDataService{
		dbs:   []catalog.Database{{ID: "db-1", ProjectID: "proj-1"}},
		lease: &fakeSQLLease{executeErr: errorsNew("exec")},
	}
	e := setupDataTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/data/collections", map[string]any{"name": "Users"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("exec err %d %s", rec.Code, rec.Body.String())
	}
}

func TestDataListCollections_SystemDatabaseRejected(t *testing.T) {
	svc := &fakeDataService{dbs: []catalog.Database{{
		ID: "db-sys", ProjectID: "proj-1", Kind: catalog.DatabaseKindSystem, DataModel: catalog.DataModelCollection,
	}}}
	e := setupDataTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-sys/data/collections", nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "data_model_mismatch") {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-sys/data/collections/sys_users/documents", map[string]any{"data": "{}"})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "system_database_protected") {
		t.Fatalf("write %d %s", rec.Code, rec.Body.String())
	}
}

func errorsNew(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
