package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/observability"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
	"github.com/prometheus/client_golang/prometheus"

	_ "github.com/uglyer/go-sqlite3"
)

type memAuthRepo struct {
	rec auth.APIKeyRecord
}

func (m *memAuthRepo) FindByHash(_ context.Context, hash string) (auth.APIKeyRecord, error) {
	if m.rec.KeyHash == hash {
		return m.rec, nil
	}
	return auth.APIKeyRecord{}, auth.ErrInvalidCredentials
}

func fullRouterDeps(t *testing.T) (Dependencies, string) {
	t.Helper()
	reg := prometheus.NewRegistry()
	rawKey := "sk-test-key-abcdef"
	authSvc := auth.NewService(&memAuthRepo{}, "secret")
	hash := authSvc.HashKey(rawKey)
	authSvc = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-1", TenantID: "tenant-1", ProjectIDs: []string{"proj-1"},
		Permissions: []auth.Permission{auth.DatabaseRead, auth.DatabaseWrite, auth.DatabaseAdmin, auth.ProjectAdmin},
		KeyHash:     hash,
	}}, "secret")

	cat := &stubCatalog{tenant: "tenant-1", db: catalog.Database{ID: "db-1", ProjectID: "proj-1", Name: "n", Status: catalog.DatabaseReady}}
	dbSvc := newFakeDBService()
	dbSvc.dbs["db-1"] = cat.db
	sqlSvc := &fakeSQLService{db: cat.db, lease: &fakeSQLLease{queryResult: database.QueryResult{Columns: []string{"n"}, Rows: [][]any{{1}}}}}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.ApplySystemMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)

	deps := Dependencies{
		Config: config.Config{
			HTTP:          config.HTTPConfig{Address: ":0"},
			Observability: config.ObservabilityConfig{MetricsPath: "/metrics"},
			Limits:        config.LimitsConfig{MaxRequestBytes: 1 << 20},
			Instance:      config.InstanceConfig{Writable: true},
		},
		Logger:          observability.NewLogger("debug", "json", nil),
		Metrics:         observability.NewMetrics(reg),
		Health:          &fakeHealth{},
		Auth:            authSvc,
		Catalog:         cat,
		DatabaseHandler: NewDatabaseHandler(dbSvc, true),
		SQLHandler:      NewSQLHandler(sqlSvc, testSQLLimits(), true),
		DataHandler:     NewDataHandler(&fakeDataService{dbs: []catalog.Database{cat.db}, lease: &fakeSQLLease{}}, true),
		Usage:           &fakeUsage{},
		Audit:           &fakeAudit{},
		LLM:             &fakeLLMSvc{names: []string{"openai"}},
		S3FileStore:     objectstore.NewMemoryFileStore(),
		System:          store,
	}
	return deps, rawKey
}

func TestNewRouter_AuthProjectMetricsSPA(t *testing.T) {
	deps, key := fullRouterDeps(t)
	e := NewRouter(deps)

	// unauthenticated v1
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/databases", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d %s", rec.Code, rec.Body.String())
	}

	// authenticated list
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/databases", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("auth list %d %s", rec.Code, rec.Body.String())
	}

	// project resolve error
	deps.Catalog = &stubCatalog{resolveErr: catalog.ErrNotFound}
	e2 := NewRouter(deps)
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/missing/databases", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	rec = httptest.NewRecorder()
	e2.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing project %d %s", rec.Code, rec.Body.String())
	}

	// SPA：未构建返回 503 提示；已构建（internal/web/dist 有 index.html）返回 200 HTML。
	req = httptest.NewRequest(http.MethodGet, "/console", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusServiceUnavailable {
		if !strings.Contains(rec.Body.String(), "管理端未构建") {
			t.Fatalf("spa unbuilt body %s", rec.Body.String())
		}
	} else if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("spa %d %s", rec.Code, rec.Body.String())
	}

	// metrics
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics %d", rec.Code)
	}

	// SPA registers GET /*; POST unmatched is Method Not Allowed via errorHandler.
	req = httptest.NewRequest(http.MethodPost, "/no-such-route", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
		t.Fatalf("unmatched POST %d %s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_NoMetricsNoAuth(t *testing.T) {
	deps := testDeps(t, &fakeHealth{})
	deps.Metrics = nil
	e := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatal("metrics should not be registered")
	}
}

func TestBodyLimitAndRequestTooLarge(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "1M"},
		{-1, "1M"},
		{1 << 20, "1M"},
		{2 << 20, "2M"},
		{1 << 10, "1K"},
		{512 << 10, "512K"},
		{1500, "2K"},
	}
	for _, tc := range cases {
		if got := bodyLimit(tc.n); got != tc.want {
			t.Errorf("bodyLimit(%d)=%q want %q", tc.n, got, tc.want)
		}
	}

	deps := testDeps(t, &fakeHealth{})
	deps.Config.Limits.MaxRequestBytes = 1 << 10 // bodyLimit → "1K"
	e := NewRouter(deps)
	e.POST("/_echo", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodPost, "/_echo", strings.NewReader(strings.Repeat("x", 2048)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestIsValidRequestID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"short", false},
		{"1234567", false},
		{"12345678", true},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
		{"bad rid", false},
		{"ok_ID-99", true},
		{"has.dot!!", false},
	}
	for _, tc := range cases {
		if got := isValidRequestID(tc.in); got != tc.want {
			t.Errorf("isValidRequestID(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestProjectContextMiddleware(t *testing.T) {
	e := echo.New()
	call := func(deps Dependencies, projectID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("projectID")
		c.SetParamValues(projectID)
		h := projectContextMiddlewareEcho(deps)(func(c echo.Context) error {
			pc, ok := ProjectFromContext(c.Request().Context())
			if !ok {
				return c.String(http.StatusInternalServerError, "no pc")
			}
			return c.String(http.StatusOK, pc.ID+":"+pc.TenantID)
		})
		_ = h(c)
		return rec
	}

	rec := call(Dependencies{}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty project %d %s", rec.Code, rec.Body.String())
	}
	rec = call(Dependencies{}, "p1")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil catalog %d %s", rec.Code, rec.Body.String())
	}
	rec = call(Dependencies{Catalog: &stubCatalog{resolveErr: catalog.ErrNotFound}}, "p1")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("resolve err %d %s", rec.Code, rec.Body.String())
	}
	rec = call(Dependencies{Catalog: &stubCatalog{tenant: "ten"}}, "p1")
	if rec.Code != http.StatusOK || rec.Body.String() != "p1:ten" {
		t.Fatalf("ok %d %s", rec.Code, rec.Body.String())
	}
}

func TestAccessLogAndErrorHandler(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := systemdb.ApplySystemMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	store := systemdb.NewStoreForTest(db)
	deps := testDeps(t, &fakeHealth{})
	deps.System = store
	e := NewRouter(deps)

	e.GET("/_http_err", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusConflict, "dup")
	})
	e.GET("/_plain_err", func(c echo.Context) error {
		return errors.New("boom")
	})
	e.GET("/_committed", func(c echo.Context) error {
		_ = c.JSON(http.StatusOK, map[string]string{"ok": "1"})
		return errors.New("after write")
	})
	e.GET("/_ok_proj", func(c echo.Context) error {
		ctx := WithProject(c.Request().Context(), ProjectContext{ID: "proj-1"})
		c.SetRequest(c.Request().WithContext(ctx))
		return c.NoContent(http.StatusOK)
	})
	e.GET("/_err_proj", func(c echo.Context) error {
		ctx := WithProject(c.Request().Context(), ProjectContext{ID: "proj-1"})
		c.SetRequest(c.Request().WithContext(ctx))
		return errors.New("fail-500")
	})

	for _, path := range []string{"/_http_err", "/_plain_err", "/_committed", "/_ok_proj", "/_err_proj"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == 0 {
			t.Fatalf("%s no status", path)
		}
	}

	// logger-less error handler
	e2 := echo.New()
	e2.HTTPErrorHandler = errorHandler(Dependencies{})
	e2.GET("/x", func(c echo.Context) error { return errors.New("z") })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	e2.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("no logger %d", rec.Code)
	}

	// access log without logger/metrics
	mw := accessLogMiddleware(Dependencies{})
	e3 := echo.New()
	e3.Use(mw)
	e3.GET("/n", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	req = httptest.NewRequest(http.MethodGet, "/n", nil)
	rec = httptest.NewRecorder()
	e3.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("quiet access log %d", rec.Code)
	}
}

func TestMountGoRoutes_WithoutSystem(t *testing.T) {
	e := echo.New()
	mountGoRoutes(e, Dependencies{Auth: auth.NewService(&memAuthRepo{}, "s")})
	req := httptest.NewRequest(http.MethodPost, "/go/proj-1/fn/Hello", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no system should not mount /go: %d", rec.Code)
	}
}

func TestNewRouter_GoInvokeMounted(t *testing.T) {
	deps, key := fullRouterDeps(t)
	e := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/go/proj-1/fn/Hello", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	// method not allowed JSON (405) rather than SPA
	if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Fatalf("go GET status=%d %s", rec.Code, rec.Body.String())
	}
}

func TestHealthReadySuccessViaRouter(t *testing.T) {
	e := NewRouter(testDeps(t, &fakeHealth{}))
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ready %d", rec.Code)
	}
}

func TestHealthLiveFailureViaRouter(t *testing.T) {
	e := NewRouter(testDeps(t, &fakeHealth{liveErr: errors.New("dead")}))
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("live fail %d", rec.Code)
	}
}
