package api

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/observability"
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

func TestNewRouter_CrossProjectDeniedBeforeHandlers(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Auth = auth.NewService(&memAuthRepo{rec: auth.APIKeyRecord{
		ID: "key-1", KeyHash: deps.Auth.HashKey(key), TenantID: "tenant-1",
		ProjectIDs: []string{"proj-1"}, Permissions: []auth.Permission{auth.DatabaseRead, auth.DatabaseWrite},
	}}, "secret")
	e := NewRouter(deps)
	paths := []struct{ method, path, body string }{
		{http.MethodPost, "/v1/projects/proj-2/kv", `{"type":"cmd","argvs":["GET","k"]}`},
		{http.MethodGet, "/v1/projects/proj-2/s3/objects", ""},
		{http.MethodGet, "/v1/projects/proj-2/gofunctions", ""},
		{http.MethodGet, "/v1/projects/proj-2/cron-jobs", ""},
		{http.MethodPost, "/go/proj-2/pricing/Quote", `{}`},
		{http.MethodGet, "/v1/projects/proj-2/llm/providers", ""},
		{http.MethodGet, "/v1/projects/proj-2/agents", ""},
		{http.MethodGet, "/v1/projects/proj-2/logs", ""},
		{http.MethodGet, "/v1/projects/proj-2/databases", ""},
	}
	for _, tc := range paths {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "cross_project_denied") {
				t.Fatalf("%s %s: status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
			}
		})
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

func TestS3UploadUsesDedicatedBodyLimit(t *testing.T) {
	deps, key := fullRouterDeps(t)
	deps.Config.Limits.MaxRequestBytes = 1 << 10 // 全局 1KB；S3 上传不受此限
	e := NewRouter(deps)

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/databases", strings.NewReader(strings.Repeat("x", 2048)))
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("global limit: got %d %s", rec.Code, rec.Body.String())
	}

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	if err := mw.WriteField("key", "a.txt"); err != nil {
		t.Fatal(err)
	}
	fw, err := mw.CreateFormFile("file", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(bytes.Repeat([]byte("x"), 2048)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/s3/objects", body)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("s3 upload within dedicated limit: got %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/s3/objects", strings.NewReader("x"))
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+key)
	req.Header.Set(echo.HeaderContentType, "multipart/form-data; boundary=x")
	req.ContentLength = s3UploadBodyLimit + 1
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized s3 upload: got %d %s", rec.Code, rec.Body.String())
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
		ctx := WithPrincipal(c.Request().Context(), auth.Principal{TenantID: "ten", ProjectIDs: map[string]struct{}{projectID: {}}})
		c.SetRequest(c.Request().WithContext(ctx))
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

func TestProjectContextMiddlewareAuthorization(t *testing.T) {
	principal := auth.Principal{TenantID: "ten", ProjectIDs: map[string]struct{}{"own": {}},
		Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}, auth.ProjectAdmin: {}}}
	cases := []struct {
		name            string
		p               auth.Principal
		project, tenant string
		want            int
	}{
		{"missing principal", auth.Principal{}, "own", "ten", 401},
		{"user own", auth.Principal{Role: auth.RoleUser, TenantID: "ten", ProjectIDs: principal.ProjectIDs, Permissions: principal.Permissions}, "own", "ten", 200},
		{"user other with admin bit", auth.Principal{Role: auth.RoleUser, TenantID: "ten", ProjectIDs: principal.ProjectIDs, Permissions: principal.Permissions}, "other", "ten", 403},
		{"admin key other", principal, "other", "ten", 200},
		{"admin key system", principal, catalog.ReservedSystemProjectID, "ten", 200},
		{"ordinary key system", auth.Principal{TenantID: "ten", ProjectIDs: principal.ProjectIDs}, catalog.ReservedSystemProjectID, "ten", 403},
		{"cross tenant admin", principal, "other", "foreign", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			e.HTTPErrorHandler = errorHandler(Dependencies{})
			deps := Dependencies{Catalog: &stubCatalog{tenant: tc.tenant}}
			e.GET("/:projectID", func(c echo.Context) error { return c.NoContent(http.StatusOK) }, projectContextMiddlewareEcho(deps))
			req := httptest.NewRequest(http.MethodGet, "/"+tc.project, nil)
			if tc.name != "missing principal" {
				req = req.WithContext(WithPrincipal(req.Context(), tc.p))
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
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

func TestShouldRecordRequestLog(t *testing.T) {
	const slow = 600 * time.Millisecond
	const fast = 10 * time.Millisecond
	cases := []struct {
		name    string
		method  string
		route   string
		status  int
		latency time.Duration
		want    bool
	}{
		{"健康检查跳过", http.MethodGet, "/health/ready", 200, fast, false},
		{"指标端点跳过", http.MethodGet, "/metrics", 200, fast, false},
		{"自定义指标路径跳过", http.MethodGet, "/custom/metrics", 200, fast, false},
		{"SPA 回退跳过", http.MethodGet, "/*", 200, fast, false},
		{"空路由跳过", http.MethodGet, "", 200, fast, false},
		{"日志列表自身跳过", http.MethodGet, "/v1/projects/:id/logs", 200, fast, false},
		{"日志 retention 自身跳过", http.MethodGet, "/v1/projects/:id/logs/retention", 200, fast, false},
		{"普通 GET 快请求跳过", http.MethodGet, "/v1/projects/:id/databases", 200, fast, false},
		{"GET 错误保留", http.MethodGet, "/v1/projects/:id/databases", 500, fast, true},
		{"GET 慢请求保留", http.MethodGet, "/v1/projects/:id/databases", 200, slow, true},
		{"HEAD 快请求跳过", http.MethodHead, "/v1/x", 200, fast, false},
		{"POST 写方法保留", http.MethodPost, "/v1/projects/:id/kv", 200, fast, true},
		{"DELETE 写方法保留", http.MethodDelete, "/v1/projects/:id/kv", 200, fast, true},
		{"PUT 写方法保留", http.MethodPut, "/v1/projects/:id/logs/retention", 200, fast, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metricsPath := ""
			if tc.name == "自定义指标路径跳过" {
				metricsPath = "/custom/metrics"
			}
			if got := shouldRecordRequestLog(tc.method, tc.route, tc.status, tc.latency, metricsPath); got != tc.want {
				t.Fatalf("shouldRecordRequestLog(%s %s status=%d latency=%v) = %v, want %v",
					tc.method, tc.route, tc.status, tc.latency, got, tc.want)
			}
		})
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
