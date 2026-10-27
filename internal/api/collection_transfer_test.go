package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

func TestCollectionTransferRealEngines(t *testing.T) {
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
			e.POST(base+"/import", h.Import)
			e.GET(base+"/export", h.Export)
			e.GET(base, h.ListDocuments)

			request := func(method, target, contentType, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, target, strings.NewReader(body))
				req.Header.Set(echo.HeaderContentType, contentType)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				return rec
			}
			root := "/databases/db-1/data/collections/tasks"

			var input strings.Builder
			for i := 0; i <= 100; i++ {
				title := fmt.Sprintf("task-%d", i)
				if i == 100 {
					title = "task-0"
				}
				fmt.Fprintf(&input, "{\"title\":%q,\"priority\":%d}\n", title, i)
			}
			rec := request(http.MethodPost, root+"/import", "application/x-ndjson", input.String())
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"failed_index":100`) {
				t.Fatalf("cross-batch duplicate: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodGet, root+"?count=1", echo.MIMEApplicationJSON, "")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":0`) {
				t.Fatalf("rollback left documents: %d %s", rec.Code, rec.Body.String())
			}

			rec = request(http.MethodPost, root+"/import?dry_run=1", "application/x-ndjson", "{\"title\":\"dry\"}\n")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"imported":0`) {
				t.Fatalf("dry run: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodPost, root+"/import", "text/csv", "title,priority,active,payload,secret\nfirst,2,true,\"{\"\"owner\"\":\"\"Ada\"\"}\",classified\nsecond,3,false,{},x\n")
			if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"imported":2`) {
				t.Fatalf("csv import: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodGet, root+"/export?format=ndjson&sort=priority", echo.MIMEApplicationJSON, "")
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "classified") || rec.Header().Get("Content-Disposition") != `attachment; filename="tasks.ndjson"` {
				t.Fatalf("ndjson export: %d %s headers=%v", rec.Code, rec.Body.String(), rec.Header())
			}
			var exported []map[string]any
			for _, line := range strings.Split(strings.TrimSpace(rec.Body.String()), "\n") {
				var doc map[string]any
				if err := json.Unmarshal([]byte(line), &doc); err != nil {
					t.Fatal(err)
				}
				exported = append(exported, doc)
			}
			if len(exported) != 2 || exported[0]["priority"].(float64) != 2 {
				t.Fatalf("exported rows: %#v", exported)
			}
			rec = request(http.MethodGet, root+"/export?format=json&sort=priority&fields=id,title,priority", echo.MIMEApplicationJSON, "")
			if rec.Code != http.StatusOK || rec.Header().Get(echo.HeaderContentType) != "application/json; charset=utf-8" || rec.Header().Get("Content-Disposition") != `attachment; filename="tasks.json"` {
				t.Fatalf("json export: %d %s headers=%v", rec.Code, rec.Body.String(), rec.Header())
			}
			var jsonDocs []map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &jsonDocs); err != nil {
				t.Fatalf("json export is not an array: %v body=%s", err, rec.Body.String())
			}
			if len(jsonDocs) != 2 || jsonDocs[0]["title"] != "first" || jsonDocs[0]["priority"].(float64) != 2 || strings.Contains(rec.Body.String(), "classified") {
				t.Fatalf("json export docs: %#v", jsonDocs)
			}
			rec = request(http.MethodGet, root+"/export?format=json&filter=priority%3E100", echo.MIMEApplicationJSON, "")
			if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
				t.Fatalf("empty json export: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodGet, root+"/export?format=csv&sort=priority&fields=id,title,priority,active,payload", echo.MIMEApplicationJSON, "")
			rows, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
			if err != nil || len(rows) != 3 || rows[1][1] != "first" || rows[1][4] != `{"owner":"Ada"}` {
				t.Fatalf("csv export: rows=%#v err=%v body=%s", rows, err, rec.Body.String())
			}
			rec = request(http.MethodPost, root+"/import", "application/json", `{}`)
			if rec.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("unsupported media: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCollectionImportRejectsViewAndSystemDatabase(t *testing.T) {
	view := &collection.Collection{Name: "tasks", SchemaMode: "view"}
	for _, test := range []struct {
		name string
		db   catalog.Database
		code int
	}{
		{name: "view", db: catalog.Database{ID: "db-1", ProjectID: "proj-1", DataModel: catalog.DataModelCollection}, code: http.StatusConflict},
		{name: "system", db: catalog.Database{ID: "db-1", ProjectID: "proj-1", Kind: catalog.DatabaseKindSystem, DataModel: catalog.DataModelCollection}, code: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &fakeDataService{dbs: []catalog.Database{test.db}}
			h := NewDataHandler(svc, true).WithCollections(staticSchemas{view}, 100)
			e := setupDataTestRouter(t, svc, true)
			e.POST("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection/import", h.Import)
			req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/databases/db-1/data/collections/tasks/import", strings.NewReader("{}\n"))
			req.Header.Set(echo.HeaderContentType, "application/x-ndjson")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != test.code {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
