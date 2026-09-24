package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// fakeDBService 是 DatabaseService 的内存假实现，用于 handler 测试。
type fakeDBService struct {
	dbs map[string]catalog.Database // key = databaseID

	createErr error
	getErr    error
	listErr   error
	deleteErr error
}

func newFakeDBService() *fakeDBService {
	return &fakeDBService{dbs: make(map[string]catalog.Database)}
}

func (f *fakeDBService) CreateDatabase(ctx context.Context, in catalog.CreateDatabaseInput) (catalog.Database, error) {
	if f.createErr != nil {
		return catalog.Database{}, f.createErr
	}
	db := catalog.Database{
		ID:        "db-" + in.Name,
		TenantID:  in.TenantID,
		ProjectID: in.ProjectID,
		Name:      in.Name,
		Status:    catalog.DatabaseReady,
	}
	f.dbs[db.ID] = db
	return db, nil
}

func (f *fakeDBService) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	if f.getErr != nil {
		return catalog.Database{}, f.getErr
	}
	db, ok := f.dbs[databaseID]
	if !ok {
		return catalog.Database{}, catalog.ErrNotFound
	}
	return db, nil
}

func (f *fakeDBService) ListDatabases(ctx context.Context, principal auth.Principal, projectID string, page catalog.Page) ([]catalog.Database, string, error) {
	if f.listErr != nil {
		return nil, "", f.listErr
	}
	var out []catalog.Database
	for _, db := range f.dbs {
		if db.ProjectID == projectID {
			out = append(out, db)
		}
	}
	return out, "", nil
}

func (f *fakeDBService) BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	if f.deleteErr != nil {
		return catalog.Database{}, f.deleteErr
	}
	db, ok := f.dbs[databaseID]
	if !ok {
		return catalog.Database{}, catalog.ErrNotFound
	}
	db.Status = catalog.DatabaseDeleting
	f.dbs[databaseID] = db
	return db, nil
}

func (f *fakeDBService) DeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	db, err := f.BeginDeleteDatabase(ctx, principal, projectID, databaseID)
	if err != nil {
		return catalog.Database{}, err
	}
	db.Status = catalog.DatabaseDeleted
	if f.dbs != nil {
		f.dbs[databaseID] = db
	}
	return db, nil
}

// setupTestRouter 构造一个带 auth + project context 中间件的测试路由。
// 使用固定 principal 注入，跳过真实 API key 认证。
func setupTestRouter(t *testing.T, svc *fakeDBService, writable bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true

	h := NewDatabaseHandler(svc, writable)

	// 注入固定 principal 和 project context 的中间件
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{
				APIKeyID:   "key-test",
				TenantID:   "tenant-1",
				ProjectIDs: map[string]struct{}{"proj-1": {}},
				Permissions: map[auth.Permission]struct{}{
					auth.DatabaseRead:  {},
					auth.DatabaseAdmin: {},
				},
			}
			ctx := WithPrincipal(c.Request().Context(), p)
			ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	v1 := e.Group("/v1")
	p := v1.Group("/projects/:projectID")
	p.POST("/databases", h.CreateDatabase)
	p.GET("/databases", h.ListDatabases)
	p.GET("/databases/:databaseID", h.GetDatabase)
	p.POST("/databases/:databaseID/open", removedDatabaseAction)
	p.POST("/databases/:databaseID/close", removedDatabaseAction)
	p.DELETE("/databases/:databaseID", h.DeleteDatabase)

	return e
}

// doRequest 执行 HTTP 请求并返回 recorder。
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

func TestCreateDatabase_Success(t *testing.T) {
	svc := newFakeDBService()
	e := setupTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases", CreateDatabaseRequest{Name: "mydb"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatabaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Name != "mydb" {
		t.Errorf("expected name mydb, got %s", resp.Name)
	}
	// 响应不得包含 StoragePrefix
	if bytes.Contains(rec.Body.Bytes(), []byte("storage_prefix")) {
		t.Error("response leaked storage_prefix")
	}
}

func TestCreateDatabase_EmptyName(t *testing.T) {
	svc := newFakeDBService()
	e := setupTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases", CreateDatabaseRequest{Name: ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateDatabase_NotWritable(t *testing.T) {
	svc := newFakeDBService()
	e := setupTestRouter(t, svc, false)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases", CreateDatabaseRequest{Name: "mydb"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestCreateDatabase_UnknownField(t *testing.T) {
	e := setupTestRouter(t, newFakeDBService(), true)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/databases",
		bytes.NewBufferString(`{"name":"x","evil":"leak"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDatabase_NotFound(t *testing.T) {
	e := setupTestRouter(t, newFakeDBService(), true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/nonexistent", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	var body APIErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "database_not_found" {
		t.Errorf("expected code database_not_found, got %s", body.Error.Code)
	}
}

func TestDeleteDatabase_Returns202(t *testing.T) {
	svc := newFakeDBService()
	svc.dbs["db-1"] = catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady}
	e := setupTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodDelete, "/v1/projects/proj-1/databases/db-1", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteDatabase_NotFound(t *testing.T) {
	e := setupTestRouter(t, newFakeDBService(), true)
	rec := doRequest(e, http.MethodDelete, "/v1/projects/proj-1/databases/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteDatabase_StateConflict(t *testing.T) {
	svc := newFakeDBService()
	svc.dbs["db-1"] = catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseDeleting}
	svc.deleteErr = catalog.ErrInvalidState
	e := setupTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodDelete, "/v1/projects/proj-1/databases/db-1", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListDatabases_Success(t *testing.T) {
	svc := newFakeDBService()
	svc.dbs["db-1"] = catalog.Database{ID: "db-1", ProjectID: "proj-1", Name: "a", Status: catalog.DatabaseReady}
	svc.dbs["db-2"] = catalog.Database{ID: "db-2", ProjectID: "proj-1", Name: "b", Status: catalog.DatabaseReady}
	e := setupTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatabaseListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Databases) != 2 {
		t.Errorf("expected 2 databases, got %d", len(resp.Databases))
	}
}

func TestErrorJSONFormat(t *testing.T) {
	e := setupTestRouter(t, newFakeDBService(), true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/missing", nil)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatal("missing error object in response")
	}
	for _, k := range []string{"code", "message", "request_id"} {
		if _, ok := errObj[k]; !ok {
			t.Errorf("error object missing field %s", k)
		}
	}
}

// 确保错误不泄露内部细节
func TestErrorNoInternalLeak(t *testing.T) {
	svc := newFakeDBService()
	svc.getErr = errors.New("internal: dsn=libsql://s3.amazonaws.com/bucket/key token=secret123")
	e := setupTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("secret123")) {
		t.Error("internal error leaked secret in response")
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("dsn=")) {
		t.Error("internal error leaked dsn in response")
	}
}

func TestPlan_CreateDatabaseHTTPReturnsReady(t *testing.T) {
	svc := newFakeDBService()
	e := setupTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases", map[string]any{"name": "plan_db"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body DatabaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ready" {
		t.Fatalf("plan: create response status=%q want ready", body.Status)
	}
}

func TestPlan_DeleteDatabaseHTTPReturnsDeleted(t *testing.T) {
	svc := newFakeDBService()
	svc.dbs["db-1"] = catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady}
	e := setupTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodDelete, "/v1/projects/proj-1/databases/db-1", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var body DeleteDatabaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "deleted" {
		t.Fatalf("plan: delete response status=%q want deleted", body.Status)
	}
	if svc.dbs["db-1"].Status != catalog.DatabaseDeleted {
		t.Fatalf("fake store status=%s want deleted", svc.dbs["db-1"].Status)
	}
}

func TestPlan_OpenCloseRoutesNotFound(t *testing.T) {
	e := setupTestRouter(t, newFakeDBService(), true)
	e.HTTPErrorHandler = errorHandler(Dependencies{})
	for _, path := range []string{
		"/v1/projects/proj-1/databases/db-1/open",
		"/v1/projects/proj-1/databases/db-1/close",
	} {
		rec := doRequest(e, http.MethodPost, path, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d: %s", path, rec.Code, rec.Body.String())
		}
		var body APIErrorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v body=%s", path, err, rec.Body.String())
		}
		if body.Error.Code != "not_found" {
			t.Fatalf("%s: code=%q body=%s", path, body.Error.Code, rec.Body.String())
		}
	}
}
