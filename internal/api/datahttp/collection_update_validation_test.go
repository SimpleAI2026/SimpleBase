// 文件说明：collection_update_validation_test.go — internal/api 的单元测试（TestReadonlyAndRemovedFieldsRealEngines 等）
//
// 用法
//
//	go test ./internal/api -run 'TestReadonlyAndRemovedFieldsRealEngines'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/collection_update_validation_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestReadonlyAndRemovedFieldsRealEngines(t *testing.T)

package datahttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/collection"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

func TestReadonlyAndRemovedFieldsRealEngines(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			dir := t.TempDir()
			path, _ := catalog.DatabasePathForProject("prj00001", engine)
			db := catalog.Database{ID: "db-1", ProjectID: "prj00001", DatabasePath: path, DataModel: catalog.DataModelCollection}
			svc := realDataService{db: db, factory: &filedb.Factory{DataRoot: dir, CacheDir: dir, Syncer: filedb.NewLocalSyncer()}}
			schema := &collection.Collection{Name: "tasks", SchemaMode: "json", Fields: []collection.Field{{Name: "title", Type: "text"}, {Name: "owner", Type: "text"}, {Name: "legacy", Type: "text"}}}
			schemas := mapCollectionSchemas{"tasks": schema}
			h := NewDataHandler(svc, true).WithCollections(schemas, 100)
			e := echo.New()
			e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error {
					c.SetRequest(c.Request().WithContext(httpkit.WithProject(httpkit.WithPrincipal(c.Request().Context(), auth.Principal{APIKeyID: "test"}), httpkit.ProjectContext{ID: "prj00001"})))
					return next(c)
				}
			})
			base := "/databases/:databaseID/data/collections/:collection"
			e.POST(base+"/documents", h.CreateDocument)
			e.POST(base+"/documents/batch", h.BatchDocuments)
			e.POST(base+"/import", h.Import)
			e.PATCH(base+"/documents/:id", h.PatchDocument)
			request := func(method, target, contentType, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, target, strings.NewReader(body))
				req.Header.Set(echo.HeaderContentType, contentType)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				return rec
			}
			root := "/databases/db-1/data/collections/tasks"
			rec := request(http.MethodPost, root+"/documents", echo.MIMEApplicationJSON, `{"title":"one","owner":"server","legacy":"keep"}`)
			if rec.Code != http.StatusCreated {
				t.Fatalf("seed: %d %s", rec.Code, rec.Body.String())
			}
			var created map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &created)
			id := created["id"].(string)
			schema.Fields = []collection.Field{{Name: "title", Type: "text"}, {Name: "owner", Type: "text", Readonly: true}}
			rec = request(http.MethodPatch, root+"/documents/"+id, echo.MIMEApplicationJSON, `{"owner":"changed"}`)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "readonly_field") {
				t.Fatalf("readonly patch: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodPatch, root+"/documents/"+id, echo.MIMEApplicationJSON, `{"title":"two"}`)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"legacy":"keep"`) || !strings.Contains(rec.Body.String(), `"owner":"server"`) {
				t.Fatalf("stored patch: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodPost, root+"/documents/batch", echo.MIMEApplicationJSON, `{"documents":[{"title":"x","owner":"client"}]}`)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "readonly_field") {
				t.Fatalf("readonly batch: %d %s", rec.Code, rec.Body.String())
			}
			rec = request(http.MethodPost, root+"/import", "application/x-ndjson", "{\"title\":\"x\",\"owner\":\"client\"}\n")
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "readonly_field") {
				t.Fatalf("readonly import: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
