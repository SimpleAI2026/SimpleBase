package api

import (
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
	if err := systemdb.EnsureSystemSchema(context.Background(), db); err != nil {
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
	// 无直方图样本时分位数为 null（不能误报 0ms），并带超量程阈值。
	for _, want := range []string{`"latency_p50_ms":null`, `"latency_p99_ms":null`, `"latency_sample_count":0`, `"latency_overflow_ms":30000`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("summary body missing %s: %s", want, rec.Body.String())
		}
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
	rec = doRequest(e, http.MethodGet, "/settings", nil)
	var got struct {
		Settings []map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Settings) != 1 {
		t.Fatalf("settings: %s", rec.Body.String())
	}
	for _, k := range []string{"key", "value_json", "updated_at"} {
		if _, ok := got.Settings[0][k]; !ok {
			t.Fatalf("missing snake_case field %q: %s", k, rec.Body.String())
		}
	}
	for _, k := range []string{"Key", "ValueJSON", "UpdatedAt"} {
		if _, ok := got.Settings[0][k]; ok {
			t.Fatalf("legacy field %q leaked: %s", k, rec.Body.String())
		}
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

func TestLLMSettingsHandlers(t *testing.T) {
	store := openSystemAPI(t)
	e := setupSystemRouter(t, store, true, true)

	rec := doRequest(e, http.MethodGet, "/llm/settings", nil)
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
	// 连接信息随设置一起保存；响应只回掩码，不回 key 原文。
	rec = doRequest(e, http.MethodPut, "/llm/settings", map[string]any{
		"default_provider": "custom_openai", "default_model": "fake-model",
		"api_key": "sk-live-1234567890", "base_url": "http://127.0.0.1:9/v1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("put connection %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-live-1234567890") {
		t.Fatalf("api key leaked: %s", rec.Body.String())
	}
	var saved struct {
		APIKeyMasked string `json:"api_key_masked"`
		HasAPIKey    bool   `json:"has_api_key"`
		BaseURL      string `json:"base_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.HasAPIKey || saved.APIKeyMasked != systemdb.MaskSecretValue("sk-live-1234567890") {
		t.Fatalf("masked view: %+v", saved)
	}
	if saved.BaseURL != "http://127.0.0.1:9/v1" {
		t.Fatalf("base url: %+v", saved)
	}
	// 留空 api_key 沿用既有值，base_url 正常覆盖。
	rec = doRequest(e, http.MethodPut, "/llm/settings", map[string]any{
		"default_provider": "custom_openai", "default_model": "fake-model",
		"base_url": "http://127.0.0.1:8/v1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("put without key %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.HasAPIKey || saved.BaseURL != "http://127.0.0.1:8/v1" {
		t.Fatalf("key should be preserved: %+v", saved)
	}
	req := httptest.NewRequest(http.MethodPut, "/llm/settings", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("bad llm settings body")
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
	for _, path := range []string{"/metrics/trend", "/logs", "/logs/retention", "/settings", "/settings/global", "/llm/settings"} {
		rec := doRequest(e, http.MethodGet, path, nil)
		if rec.Code == http.StatusOK {
			// some getters return zero values on missing tables? fail if unexpectedly OK with empty schema
			continue
		}
	}
	// MetricsSummary 不再吞查询错误：空 schema 必须返回非 200。
	if rec := doRequest(e, http.MethodGet, "/metrics/summary", nil); rec.Code == http.StatusOK {
		t.Fatalf("summary on empty schema should fail, got %s", rec.Body.String())
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
	rec = doRequest(e, http.MethodPut, "/llm/settings", map[string]any{"default_model": "m"})
	if rec.Code == http.StatusOK {
		t.Fatal("put llm settings on empty schema should fail")
	}
}
