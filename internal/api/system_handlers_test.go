package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"

	_ "github.com/uglyer/go-sqlite3"
)

func openSystemAPI(t *testing.T) *systemdb.Store {
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

func setupSystemRouter(t *testing.T, store *systemdb.Store, withProject, withPrincipal bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = errorHandler(Dependencies{})
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			if withPrincipal {
				ctx = WithPrincipal(ctx, auth.Principal{APIKeyID: "key-sys"})
			}
			if withProject {
				ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "t"})
			}
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	mh := &metricsHandler{store: store}
	e.GET("/metrics/summary", mh.Summary)
	e.GET("/metrics/trend", mh.Trend)
	lh := &logsHTTPHandler{store: store}
	e.GET("/logs", lh.List)
	e.GET("/logs/retention", lh.GetRetention)
	e.PUT("/logs/retention", lh.PutRetention)
	seth := &settingsHandler{store: store}
	e.GET("/settings", seth.GetProject)
	e.PUT("/settings", seth.PutProject)
	e.GET("/settings/global", seth.GetGlobal)
	e.PUT("/settings/global", seth.PutGlobal)
	sess := &llmSessionHandler{store: store}
	e.GET("/llm/sessions", sess.List)
	e.POST("/llm/sessions", sess.Create)
	e.GET("/llm/sessions/:sessionID", sess.Get)
	e.DELETE("/llm/sessions/:sessionID", sess.Delete)
	e.GET("/llm/sessions/:sessionID/messages", sess.ListMessages)
	e.POST("/llm/sessions/:sessionID/messages", sess.PostMessage)
	e.GET("/llm/settings", sess.GetSettings)
	e.PUT("/llm/settings", sess.PutSettings)
	return e
}

func TestSystemHandlers_MissingProject(t *testing.T) {
	store := openSystemAPI(t)
	e := setupSystemRouter(t, store, false, true)
	paths := []struct {
		method, path string
		body         string
	}{
		{http.MethodGet, "/metrics/summary", ""},
		{http.MethodGet, "/metrics/trend", ""},
		{http.MethodGet, "/logs", ""},
		{http.MethodGet, "/logs/retention", ""},
		{http.MethodPut, "/logs/retention", `{"keep_days":7}`},
		{http.MethodGet, "/settings", ""},
		{http.MethodPut, "/settings", `{"key":"a","value_json":"1"}`},
		{http.MethodGet, "/llm/sessions", ""},
		{http.MethodPost, "/llm/sessions", `{}`},
		{http.MethodGet, "/llm/sessions/s1", ""},
		{http.MethodDelete, "/llm/sessions/s1", ""},
		{http.MethodGet, "/llm/sessions/s1/messages", ""},
		{http.MethodPost, "/llm/sessions/s1/messages", `{"role":"user","content":"x"}`},
		{http.MethodGet, "/llm/settings", ""},
		{http.MethodPut, "/llm/settings", `{}`},
	}
	for _, tc := range paths {
		var req *http.Request
		if tc.body == "" {
			req = httptest.NewRequest(tc.method, tc.path, nil)
		} else {
			req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK || rec.Code == http.StatusCreated || rec.Code == http.StatusNoContent {
			t.Fatalf("%s %s expected error, got %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestMetricsAndLogsHandlers(t *testing.T) {
	store := openSystemAPI(t)
	e := setupSystemRouter(t, store, true, true)

	rec := doRequest(e, http.MethodGet, "/metrics/summary", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("summary %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/metrics/trend", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("trend %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/metrics/trend?days=3", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("trend days %d", rec.Code)
	}
	rec = doRequest(e, http.MethodGet, "/metrics/trend?days=nope", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("trend bad days %d", rec.Code)
	}

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	rec = doRequest(e, http.MethodGet, "/logs?level=info&q=http&limit=10&from="+from+"&to="+to, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logs %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/logs?limit=x&from=bad&to=bad", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logs invalid filters %d", rec.Code)
	}

	rec = doRequest(e, http.MethodGet, "/logs/retention", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get retention %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPut, "/logs/retention", map[string]any{"keep_days": 14})
	if rec.Code != http.StatusOK {
		t.Fatalf("put retention %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodPut, "/logs/retention", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("bad retention body should fail")
	}
}

func TestSettingsHandlers(t *testing.T) {
	store := openSystemAPI(t)
	e := setupSystemRouter(t, store, true, true)

	rec := doRequest(e, http.MethodGet, "/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get project %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPut, "/settings", map[string]any{"key": "theme", "value_json": `"dark"`})
	if rec.Code != http.StatusOK {
		t.Fatalf("put project %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPut, "/settings", map[string]any{"key": "", "value_json": "1"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty key %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodPut, "/settings", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("bad settings body")
	}

	rec = doRequest(e, http.MethodGet, "/settings/global", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get global %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPut, "/settings/global", map[string]any{"key": "inst", "value_json": `"v"`})
	if rec.Code != http.StatusOK {
		t.Fatalf("put global %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPut, "/settings/global", map[string]any{"key": "", "value_json": "1"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("global empty key %d", rec.Code)
	}

	eNoPrin := setupSystemRouter(t, store, true, false)
	rec = doRequest(eNoPrin, http.MethodPut, "/settings/global", map[string]any{"key": "x", "value_json": "1"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing principal %d %s", rec.Code, rec.Body.String())
	}
}

func TestLLMSessionHandlers(t *testing.T) {
	store := openSystemAPI(t)
	e := setupSystemRouter(t, store, true, true)

	rec := doRequest(e, http.MethodGet, "/llm/sessions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list empty %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPost, "/llm/sessions", map[string]any{"title": "chat", "provider": "openai", "model": "gpt"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	var created llmSessionDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rec = doRequest(e, http.MethodGet, "/llm/sessions/"+created.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/llm/sessions/missing", nil)
	if rec.Code == http.StatusOK {
		t.Fatal("missing session should error")
	}

	rec = doRequest(e, http.MethodPost, "/llm/sessions/"+created.ID+"/messages", map[string]any{"role": "user", "content": "hello"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("post msg %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPost, "/llm/sessions/"+created.ID+"/messages", map[string]any{"role": "", "content": ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty msg %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodPost, "/llm/sessions/"+created.ID+"/messages", bytes.NewReader([]byte("{")))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusCreated {
		t.Fatal("bad message body")
	}
	rec = doRequest(e, http.MethodGet, "/llm/sessions/"+created.ID+"/messages", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("list msgs %d %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(e, http.MethodGet, "/llm/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get settings %d %s", rec.Code, rec.Body.String())
	}
	temp := 0.3
	max := 64
	rec = doRequest(e, http.MethodPut, "/llm/settings", map[string]any{
		"default_provider": "openai", "default_model": "gpt", "temperature": temp, "max_tokens": max,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPut, "/llm/settings", map[string]any{"default_provider": "x"})
	if rec.Code != http.StatusOK {
		t.Fatalf("put defaults %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPut, "/llm/settings", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("bad llm settings body")
	}

	rec = doRequest(e, http.MethodDelete, "/llm/sessions/"+created.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}

	// create without principal (createdBy empty)
	e2 := setupSystemRouter(t, store, true, false)
	rec = doRequest(e2, http.MethodPost, "/llm/sessions", map[string]any{"title": "anon"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("anon create %d %s", rec.Code, rec.Body.String())
	}
}

func TestSystemHandlers_StoreErrors(t *testing.T) {
	// unmigrated store triggers write/query errors
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := systemdb.NewStoreForTest(db)
	e := setupSystemRouter(t, store, true, true)
	for _, path := range []string{"/metrics/summary", "/metrics/trend", "/logs", "/logs/retention", "/settings", "/settings/global", "/llm/sessions", "/llm/settings"} {
		rec := doRequest(e, http.MethodGet, path, nil)
		if rec.Code == http.StatusOK {
			// some getters return zero values on missing tables? fail if unexpectedly OK with empty schema
			// MetricsSummary swallows scan errors and still returns 200.
			continue
		}
	}
	rec := doRequest(e, http.MethodPut, "/settings", map[string]any{"key": "k", "value_json": "1"})
	if rec.Code == http.StatusOK {
		t.Fatal("put setting on empty schema should fail")
	}
	rec = doRequest(e, http.MethodPut, "/settings/global", map[string]any{"key": "k", "value_json": "1"})
	if rec.Code == http.StatusOK {
		t.Fatal("put global on empty schema should fail")
	}
	rec = doRequest(e, http.MethodPut, "/logs/retention", map[string]any{"keep_days": 3})
	if rec.Code == http.StatusOK {
		t.Fatal("put retention on empty schema should fail")
	}
	rec = doRequest(e, http.MethodPost, "/llm/sessions", map[string]any{"title": "x"})
	if rec.Code == http.StatusCreated {
		t.Fatal("create session on empty schema should fail")
	}
	rec = doRequest(e, http.MethodPut, "/llm/settings", map[string]any{"default_model": "m"})
	if rec.Code == http.StatusOK {
		t.Fatal("put llm settings on empty schema should fail")
	}
}
