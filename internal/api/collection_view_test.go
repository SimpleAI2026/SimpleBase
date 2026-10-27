package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

type mapCollectionSchemas map[string]*collection.Collection

func (m mapCollectionSchemas) Get(_ context.Context, _ string, name string) (*collection.Collection, error) {
	if schema := m[name]; schema != nil {
		copy := *schema
		return &copy, nil
	}
	return nil, collection.ErrNotFound
}
func (m mapCollectionSchemas) List(context.Context, string) ([]collection.Collection, error) {
	out := []collection.Collection{}
	for _, schema := range m {
		out = append(out, *schema)
	}
	return out, nil
}
func (m mapCollectionSchemas) Put(_ context.Context, schema collection.Collection) error {
	m[schema.Name] = &schema
	return nil
}
func (m mapCollectionSchemas) Delete(_ context.Context, _ string, name string) error {
	delete(m, name)
	return nil
}

func TestLogicalViewRealEngines(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			dir := t.TempDir()
			path, err := catalog.DatabasePathForProject("prj00001", engine)
			if err != nil {
				t.Fatal(err)
			}
			db := catalog.Database{ID: "db-1", ProjectID: "prj00001", DatabasePath: path, DataModel: catalog.DataModelCollection}
			svc := realDataService{db: db, factory: &filedb.Factory{DataRoot: dir, CacheDir: dir, Syncer: filedb.NewNopSyncer()}}
			schemas := mapCollectionSchemas{
				"tasks":        {Name: "tasks", SchemaMode: "json", Fields: []collection.Field{{Name: "title", Type: "text"}, {Name: "priority", Type: "number"}}},
				"active_tasks": {Name: "active_tasks", SchemaMode: "view", ViewQuery: "SELECT id, data, created_at, updated_at FROM tasks WHERE json_extract(data, '$.priority') >= 2", Fields: []collection.Field{{Name: "title", Type: "text"}, {Name: "priority", Type: "number"}}},
			}
			h := NewDataHandler(svc, true).WithCollections(schemas, 50000)
			mh := NewCollectionMetaHandler(svc, schemas, true, 50000)
			e := echo.New()
			e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error {
					ctx := WithPrincipal(c.Request().Context(), auth.Principal{APIKeyID: "test", Permissions: map[auth.Permission]struct{}{auth.ProjectAdmin: {}}})
					ctx = WithProject(ctx, ProjectContext{ID: "prj00001"})
					c.SetRequest(c.Request().WithContext(ctx))
					return next(c)
				}
			})
			base := "/databases/:databaseID/data/collections/:collection"
			e.GET("/databases/:databaseID/data/collections", h.ListCollections)
			e.GET(base, h.ListDocuments)
			e.GET(base+"/documents/:id", h.GetDocument)
			e.GET(base+"/export", h.Export)
			e.POST(base+"/documents", h.CreateDocument)
			e.POST(base+"/documents/batch", h.BatchDocuments)
			e.POST(base+"/import", h.Import)
			e.PATCH(base+"/documents/:id", h.PatchDocument)
			e.DELETE(base+"/documents/:id", h.DeleteDocument)
			e.DELETE(base, h.TruncateCollection)
			e.POST("/databases/:databaseID/data/collections/meta/dry-run-view", mh.DryRunView)

			request := func(method, target, contentType, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, target, strings.NewReader(body))
				req.Header.Set(echo.HeaderContentType, contentType)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				return rec
			}
			for _, body := range []string{`{"title":"one","priority":1}`, `{"title":"two","priority":2}`, `{"title":"three","priority":3}`} {
				rec := request(http.MethodPost, "/databases/db-1/data/collections/tasks/documents", echo.MIMEApplicationJSON, body)
				if rec.Code != http.StatusCreated {
					t.Fatalf("seed: %d %s", rec.Code, rec.Body.String())
				}
			}
			view := "/databases/db-1/data/collections/active_tasks"
			rec := request(http.MethodGet, view+"?limit=1&count=1", echo.MIMEApplicationJSON, "")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":2`) || !strings.Contains(rec.Body.String(), `"next_cursor"`) {
				t.Fatalf("view list: %d %s", rec.Code, rec.Body.String())
			}
			var page rowsResponse
			if err := jsonUnmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Items) != 1 {
				t.Fatalf("view page: %#v %v", page, err)
			}
			id := page.Items[0].ID
			rec = request(http.MethodGet, view+"?limit=1&cursor="+url.QueryEscape(page.NextCursor), echo.MIMEApplicationJSON, "")
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"next_cursor"`) {
				t.Fatalf("view cursor: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodGet, view+"/documents/"+id, echo.MIMEApplicationJSON, "")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), id) {
				t.Fatalf("view get: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodGet, view+"/export?format=ndjson", echo.MIMEApplicationJSON, "")
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "one") || !strings.Contains(rec.Body.String(), "three") {
				t.Fatalf("view export: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodGet, "/databases/db-1/data/collections", echo.MIMEApplicationJSON, "")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"active_tasks"`) || !strings.Contains(rec.Body.String(), `"schema_mode":"view"`) {
				t.Fatalf("collection list: %d %s", rec.Code, rec.Body.String())
			}
			for _, route := range []struct{ method, suffix, contentType, body string }{
				{http.MethodPost, "/documents", echo.MIMEApplicationJSON, `{"title":"bad"}`},
				{http.MethodPost, "/documents/batch", echo.MIMEApplicationJSON, `{"documents":[{"title":"bad"}]}`},
				{http.MethodPost, "/import", "application/x-ndjson", "{\"title\":\"bad\"}\n"},
				{http.MethodPatch, "/documents/" + id, echo.MIMEApplicationJSON, `{"title":"bad"}`},
				{http.MethodDelete, "/documents/" + id, echo.MIMEApplicationJSON, ""},
				{http.MethodDelete, "", echo.MIMEApplicationJSON, ""},
			} {
				rec = request(route.method, view+route.suffix, route.contentType, route.body)
				if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "collection_read_only") {
					t.Fatalf("view write %s %s: %d %s", route.method, route.suffix, rec.Code, rec.Body.String())
				}
			}
			preview := `{"name":"preview","query":"SELECT id, data, created_at, updated_at FROM tasks WHERE json_extract(data, '$.priority') >= 2"}`
			rec = request(http.MethodPost, "/databases/db-1/data/collections/meta/dry-run-view", echo.MIMEApplicationJSON, preview)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"scanned":2`) || !strings.Contains(rec.Body.String(), `"type":"number"`) {
				t.Fatalf("view dry run: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodPost, "/databases/db-1/data/collections/meta/dry-run-view", echo.MIMEApplicationJSON, `{"name":"preview","query":"DELETE FROM tasks"}`)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_view_query") {
				t.Fatalf("view guard: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func jsonUnmarshal(data []byte, target any) error {
	return json.Unmarshal(data, target)
}
