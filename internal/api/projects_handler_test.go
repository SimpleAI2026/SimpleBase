package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

type fakeProjectCatalog struct {
	list    []catalog.Project
	created catalog.Project
	err     error
}

func (f *fakeProjectCatalog) CreateDatabase(context.Context, catalog.CreateDatabaseInput) (catalog.Database, error) {
	return catalog.Database{}, errors.New("unused")
}
func (f *fakeProjectCatalog) GetDatabase(context.Context, auth.Principal, string, string) (catalog.Database, error) {
	return catalog.Database{}, errors.New("unused")
}
func (f *fakeProjectCatalog) ListDatabases(context.Context, auth.Principal, string, catalog.Page) ([]catalog.Database, string, error) {
	return nil, "", errors.New("unused")
}
func (f *fakeProjectCatalog) BeginDeleteDatabase(context.Context, auth.Principal, string, string) (catalog.Database, error) {
	return catalog.Database{}, errors.New("unused")
}
func (f *fakeProjectCatalog) DeleteDatabaseSync(context.Context, auth.Principal, string, string, func(context.Context, string) error, catalog.StoragePurger) (catalog.Database, error) {
	return catalog.Database{}, errors.New("unused")
}
func (f *fakeProjectCatalog) SetDatabaseReady(context.Context, string) error {
	return errors.New("unused")
}
func (f *fakeProjectCatalog) ResolveProjectTenant(context.Context, string) (string, error) {
	return "", errors.New("unused")
}
func (f *fakeProjectCatalog) ListProjects(context.Context, auth.Principal) ([]catalog.Project, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}
func (f *fakeProjectCatalog) CreateProject(_ context.Context, _ auth.Principal, in catalog.CreateProjectInput) (catalog.Project, error) {
	if f.err != nil {
		return catalog.Project{}, f.err
	}
	id := in.ID
	if id == "" {
		id = "11111111-1111-1111-1111-111111111111"
	}
	f.created = catalog.Project{ID: id, Name: in.Name, CreatedAt: time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)}
	return f.created, nil
}

func setupProjectsRouter(t *testing.T, cat CatalogService, perms ...auth.Permission) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	if len(perms) == 0 {
		perms = []auth.Permission{auth.DatabaseRead, auth.ProjectAdmin}
	}
	pm := map[auth.Permission]struct{}{}
	for _, p := range perms {
		pm[p] = struct{}{}
	}
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithPrincipal(c.Request().Context(), auth.Principal{
				APIKeyID:    "key-test",
				TenantID:    catalog.ReservedTenantID,
				ProjectIDs:  map[string]struct{}{catalog.DevProjectID: {}},
				Permissions: pm,
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	h := NewProjectsHandler(cat, nil)
	e.GET("/v1/projects", h.ListProjects)
	e.POST("/v1/projects", h.CreateProject)
	return e
}

func TestListProjects_OK(t *testing.T) {
	cat := &fakeProjectCatalog{list: []catalog.Project{{
		ID: catalog.DevProjectID, Name: "商城后台", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}}}
	e := setupProjectsRouter(t, cat)
	rec := doRequest(e, http.MethodGet, "/v1/projects", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out ProjectListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Projects) != 1 || out.Projects[0].ID != catalog.DevProjectID || out.Projects[0].Name != "商城后台" {
		t.Fatalf("unexpected body: %+v", out)
	}
}

func TestCreateProject_Created(t *testing.T) {
	cat := &fakeProjectCatalog{}
	e := setupProjectsRouter(t, cat)
	rec := doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "演示项目"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out ProjectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "演示项目" || out.ID == "" {
		t.Fatalf("unexpected body: %+v", out)
	}
}

func TestCreateProject_Conflict(t *testing.T) {
	cat := &fakeProjectCatalog{err: catalog.ErrAlreadyExists}
	e := setupProjectsRouter(t, cat)
	rec := doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "商城后台"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("project_already_exists")) {
		t.Fatalf("expected project_already_exists: %s", rec.Body.String())
	}
}

func TestCreateProject_EmptyName(t *testing.T) {
	e := setupProjectsRouter(t, &fakeProjectCatalog{})
	rec := doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "  "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateProject_UnknownField(t *testing.T) {
	e := setupProjectsRouter(t, &fakeProjectCatalog{})
	req := httptest.NewRequest(http.MethodPost, "/v1/projects", bytes.NewReader([]byte(`{"name":"x","extra":1}`)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
