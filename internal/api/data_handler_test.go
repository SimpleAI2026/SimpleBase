package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
)

type fakeDataService struct {
	dbs          []catalog.Database
	listErr      error
	getErr       error
	acquireErr   error
	lastGetID    string
	listCalled   bool
	acquiredMode database.AccessMode
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

func (f *fakeDataService) Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (SQLLease, error) {
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
	p.GET("/data/collections", h.ListCollections)
	p.POST("/data/collections", h.CreateCollection)
	p.GET("/data/collections/:collection", h.ListDocuments)
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
			queryResult: database.QueryResult{
				Columns: []string{"table_name"},
				Rows:    [][]any{{"users"}, {"orders"}},
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

func TestDataListCollections_LegacyUsesFirstDatabase(t *testing.T) {
	svc := &fakeDataService{
		dbs: []catalog.Database{
			{ID: "db-first", Name: "first", ProjectID: "proj-1"},
			{ID: "db-second", Name: "second", ProjectID: "proj-1"},
		},
		lease: &fakeSQLLease{
			queryResult: database.QueryResult{
				Columns: []string{"table_name"},
				Rows:    [][]any{{"users"}},
			},
		},
	}
	e := setupDataTestRouter(t, svc, true)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/data/collections", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !svc.listCalled {
		t.Fatal("legacy route should ListDatabases")
	}
	if svc.lastGetID != "db-first" {
		t.Fatalf("legacy GetDatabase id = %q, want db-first", svc.lastGetID)
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
			executeResult: database.QueryResult{RowsAffected: 1},
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
	if svc.acquiredMode != database.ReadWrite {
		t.Fatalf("mode = %v, want ReadWrite", svc.acquiredMode)
	}
}
