package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/uglyer/go-sqlite3"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func openAgentStore(t *testing.T) *systemdb.Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.ApplySystemMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return systemdb.NewStoreForTest(db)
}

func agentEcho(store *systemdb.Store) *echo.Echo {
	e := echo.New()
	h := &cloudAgentHandler{store: store}
	e.GET("/agents/modules", h.ListModules)
	e.GET("/agents", h.ListAgents)
	e.POST("/agents", h.CreateAgent)
	e.GET("/agents/:agentID", h.GetAgent)
	e.PATCH("/agents/:agentID", h.PatchAgent)
	e.DELETE("/agents/:agentID", h.DeleteAgent)
	e.POST("/agent-threads", h.CreateThread)
	e.GET("/agent-threads/:threadID/messages", h.ListMessages)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return next
	})
	return e
}

func agentReq(e *echo.Echo, method, path string, body any) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	ctx := WithProject(r.Context(), ProjectContext{ID: catalog.DevProjectID, TenantID: catalog.ReservedTenantID})
	ctx = WithPrincipal(ctx, auth.Principal{APIKeyID: "k1", TenantID: catalog.ReservedTenantID})
	r = r.WithContext(ctx)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	return rec
}

func TestAgentCRUDAndSeed(t *testing.T) {
	store := openAgentStore(t)
	e := agentEcho(store)

	rec := agentReq(e, http.MethodGet, "/agents/modules", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("modules status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = agentReq(e, http.MethodGet, "/agents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Agents []agentDTO `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Agents) != 3 {
		t.Fatalf("seeded agents=%d", len(listed.Agents))
	}

	rec = agentReq(e, http.MethodPost, "/agents", map[string]any{
		"name": "Custom", "module": "general", "system_prompt": "Be brief", "tool_ids": []string{},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created agentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rec = agentReq(e, http.MethodPatch, "/agents/"+created.ID, map[string]any{"name": "Custom2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = agentReq(e, http.MethodDelete, "/agents/"+created.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", rec.Code)
	}

	rec = agentReq(e, http.MethodPost, "/agent-threads", map[string]any{"title": "chat"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("thread status=%d body=%s", rec.Code, rec.Body.String())
	}
}
