package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/linkxzhou/SimpleBase/internal/dbbridge"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

type staticSchemas struct{ schema *collection.Collection }

func (s staticSchemas) Get(context.Context, string, string) (*collection.Collection, error) {
	return s.schema, nil
}
func (s staticSchemas) List(context.Context, string) ([]collection.Collection, error) {
	if s.schema == nil {
		return nil, nil
	}
	return []collection.Collection{*s.schema}, nil
}

func TestCollectionFilterTypesAndInjection(t *testing.T) {
	schema := &collection.Collection{Fields: []collection.Field{
		{Name: "title", Type: "text"}, {Name: "priority", Type: "number"},
		{Name: "active", Type: "bool"}, {Name: "published", Type: "date"},
		{Name: "state", Type: "select"}, {Name: "payload", Type: "json"},
	}}
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		dialect, _ := godatabase.DialectFor(engine)
		for _, test := range []struct {
			filter string
			args   int
		}{
			{`title~"a%b" && priority>=2`, 2}, {`active=true`, 1},
			{`published>="2026-01-01T00:00:00Z"`, 1}, {`state in ["open","closed"]`, 2},
			{`payload is null`, 0}, {`(title!="x" || priority<3) && active=false`, 3},
		} {
			_, args, err := compileFilter(test.filter, schema, dialect)
			if err != nil || len(args) != test.args {
				t.Fatalf("%s %s: args=%v err=%v", engine, test.filter, args, err)
			}
		}
		for _, invalid := range []string{`unknown="x"`, `title="x"; DROP TABLE t`, `priority~"2"`, `payload={}`, `title in []`} {
			if _, _, err := compileFilter(invalid, schema, dialect); err == nil {
				t.Fatalf("%s accepted %q", engine, invalid)
			}
		}
	}
}

func TestCollectionFieldsNested(t *testing.T) {
	schema := &collection.Collection{Fields: []collection.Field{{Name: "title", Type: "text"}, {Name: "payload", Type: "json"}}}
	fields, err := parseFields("title,payload.owner.name", schema)
	if err != nil {
		t.Fatal(err)
	}
	got := pickDocument(map[string]any{"title": "x", "payload": map[string]any{"owner": map[string]any{"name": "Ada", "secret": true}}}, fields)
	if len(got) != 2 || got["payload"].(map[string]any)["owner"].(map[string]any)["name"] != "Ada" {
		t.Fatalf("unexpected projection: %#v", got)
	}
	if _, exists := got["payload"].(map[string]any)["owner"].(map[string]any)["secret"]; exists {
		t.Fatal("unselected nested field returned")
	}
}

type realDataService struct {
	db      catalog.Database
	factory *filedb.Factory
}

func (s realDataService) GetDatabase(context.Context, auth.Principal, string, string) (catalog.Database, error) {
	return s.db, nil
}
func (s realDataService) ListDatabases(context.Context, auth.Principal, string, catalog.Page) ([]catalog.Database, string, error) {
	return []catalog.Database{s.db}, "", nil
}
func (s realDataService) Acquire(ctx context.Context, db catalog.Database, mode godatabase.AccessMode) (SQLLease, error) {
	conn, err := s.factory.Open(ctx, dbbridge.SpecOf(db), mode)
	if err != nil {
		return nil, err
	}
	return &realDataLease{db: conn}, nil
}

type realDataLease struct{ db *sql.DB }

func (l *realDataLease) Release() { _ = l.db.Close() }
func (l *realDataLease) Query(ctx context.Context, stmt godatabase.Statement, max int) (godatabase.QueryResult, error) {
	return godatabase.Query(ctx, l.db, stmt, max)
}
func (l *realDataLease) Execute(ctx context.Context, stmt godatabase.Statement) (godatabase.QueryResult, error) {
	return godatabase.Execute(ctx, l.db, stmt)
}
func (l *realDataLease) Batch(ctx context.Context, statements []godatabase.Statement, transactional bool) ([]godatabase.QueryResult, error) {
	return godatabase.Batch(ctx, l.db, statements, transactional)
}
func (l *realDataLease) Raw() *sql.DB                { return l.db }
func (l *realDataLease) NotifyWrite(context.Context) {}

func TestCollectionDocumentAPIRealEngines(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			dir := t.TempDir()
			path, err := catalog.DatabasePathForProject("prj00001", engine)
			if err != nil {
				t.Fatal(err)
			}
			db := catalog.Database{ID: "db-1", ProjectID: "prj00001", DatabasePath: path, DataModel: catalog.DataModelCollection}
			svc := realDataService{db: db, factory: &filedb.Factory{DataRoot: dir, CacheDir: dir, Syncer: filedb.NewNopSyncer()}}
			schema := &collection.Collection{Name: "tasks", Fields: []collection.Field{
				{Name: "title", Type: "text", Required: true, Unique: true},
				{Name: "priority", Type: "number"}, {Name: "active", Type: "bool"},
				{Name: "payload", Type: "json"}, {Name: "secret", Type: "text", Hidden: true},
			}}
			h := NewDataHandler(svc, true).WithCollections(staticSchemas{schema}, 50000)
			e := echo.New()
			e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error {
					ctx := WithPrincipal(c.Request().Context(), auth.Principal{APIKeyID: "test"})
					ctx = WithProject(ctx, ProjectContext{ID: "prj00001"})
					c.SetRequest(c.Request().WithContext(ctx))
					return next(c)
				}
			})
			base := "/databases/:databaseID/data/collections/:collection"
			e.POST(base+"/documents", h.CreateDocument)
			e.POST(base+"/documents/batch", h.BatchDocuments)
			e.GET(base, h.ListDocuments)
			e.GET(base+"/documents/:id", h.GetDocument)
			e.PATCH(base+"/documents/:id", h.PatchDocument)

			request := func(method, target, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, target, strings.NewReader(body))
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				return rec
			}
			rec := request(http.MethodPost, "/databases/db-1/data/collections/tasks/documents", `{"title":"first","priority":2,"active":true,"payload":{"owner":{"name":"Ada","private":1}},"secret":"x"}`)
			if rec.Code != http.StatusCreated {
				t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
			}
			var created documentDTO
			if json.Unmarshal(rec.Body.Bytes(), &created) != nil {
				created.Data = map[string]any{}
			}
			var flat map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &flat); err != nil || flat["secret"] != nil {
				t.Fatalf("hidden create response: %s %v", rec.Body.String(), err)
			}
			id, _ := flat["id"].(string)
			rec = request(http.MethodPost, "/databases/db-1/data/collections/tasks/documents", `{"title":"first"}`)
			if rec.Code != http.StatusConflict {
				t.Fatalf("unique: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodPost, "/databases/db-1/data/collections/tasks/documents/batch", `{"documents":[{"title":"same"},{"title":"same"}]}`)
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"failed_index":1`) {
				t.Fatalf("batch duplicate: %d %s", rec.Code, rec.Body.String())
			}
			for _, title := range []string{"second", "third", "fourth"} {
				rec = request(http.MethodPost, "/databases/db-1/data/collections/tasks/documents", `{"title":"`+title+`","priority":3,"active":false}`)
				if rec.Code != http.StatusCreated {
					t.Fatalf("seed: %d %s", rec.Code, rec.Body.String())
				}
			}
			target := "/databases/db-1/data/collections/tasks?limit=2&count=1&filter=" + url.QueryEscape(`priority>=2 && active=true || priority=3`) + `&fields=id,title,payload.owner.name`
			rec = request(http.MethodGet, target, "")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":4`) || !strings.Contains(rec.Body.String(), `"next_cursor"`) {
				t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
			}
			var page rowsResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Items) != 2 || page.Items[0].Data["secret"] != nil {
				t.Fatalf("page: %#v %v", page, err)
			}
			rec = request(http.MethodGet, "/databases/db-1/data/collections/tasks?limit=2&cursor="+url.QueryEscape(page.NextCursor), "")
			if rec.Code != http.StatusOK {
				t.Fatalf("cursor: %d %s", rec.Code, rec.Body.String())
			}
			var next rowsResponse
			_ = json.Unmarshal(rec.Body.Bytes(), &next)
			if len(next.Items) != 2 || next.Items[0].ID == page.Items[0].ID {
				t.Fatalf("cursor overlap: first=%v next=%v", page.Items, next.Items)
			}
			rec = request(http.MethodPatch, "/databases/db-1/data/collections/tasks/documents/"+id, `{"payload":{"owner":{"name":"Grace"}},"priority":4}`)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"updated_at"`) {
				t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodGet, "/databases/db-1/data/collections/tasks/documents/"+id, "")
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"secret"`) {
				t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
