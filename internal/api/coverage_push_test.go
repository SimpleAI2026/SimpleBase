package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

func TestRecordAuditHelpers(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	gh := NewGoFunctionHandler(nil, true, nil)
	gh.recordAudit(c, "p", "create:fn") // nil audit

	aud := &fakeAudit{}
	gh2 := NewGoFunctionHandler(nil, true, aud)
	gh2.recordAudit(c, "p", "create:fn")
	if aud.last.Kind != "gofunction" || aud.last.PrincipalID != "" {
		t.Fatalf("no principal audit=%+v", aud.last)
	}
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2 = req2.WithContext(WithPrincipal(req2.Context(), auth.Principal{APIKeyID: "k"}))
	c2 := e.NewContext(req2, httptest.NewRecorder())
	gh2.recordAudit(c2, "p", "update:fn")
	if aud.last.PrincipalID != "k" {
		t.Fatalf("principal=%q", aud.last.PrincipalID)
	}

	ch := NewCronJobHandler(nil, true, nil, nil)
	ch.recordAudit(c, "p", "create:job")
	aud2 := &fakeAudit{}
	ch2 := NewCronJobHandler(nil, true, nil, aud2)
	ch2.recordAudit(c2, "p", "create:job")
	if aud2.last.Kind != "cronjob" || aud2.last.PrincipalID != "k" {
		t.Fatalf("cron audit=%+v", aud2.last)
	}
}

func TestNilStoreAndMissingProjectHandlers(t *testing.T) {
	e := echo.New()
	gh := NewGoFunctionHandler(nil, true, nil)
	cj := NewCronJobHandler(nil, true, nil, nil)
	sch := &agentScheduleHandler{}

	mount := func(injectProject bool) *echo.Echo {
		ee := echo.New()
		ee.HideBanner = true
		ee.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				ctx := c.Request().Context()
				if injectProject {
					ctx = WithProject(ctx, ProjectContext{ID: "proj-1"})
				}
				c.SetRequest(c.Request().WithContext(ctx))
				return next(c)
			}
		})
		ee.GET("/gf", gh.List)
		ee.GET("/gf/:name", gh.Get)
		ee.PUT("/gf/:name", gh.Update)
		ee.DELETE("/gf/:name", gh.Delete)
		ee.POST("/gf", gh.Create)
		ee.GET("/cj", cj.List)
		ee.GET("/cj/:jobID", cj.Get)
		ee.GET("/cj/:jobID/runs", cj.ListRuns)
		ee.DELETE("/cj/:jobID", cj.Delete)
		ee.GET("/as", sch.ListSchedules)
		ee.GET("/as/:scheduleID", sch.GetSchedule)
		ee.GET("/as/:scheduleID/runs", sch.ListScheduleRuns)
		ee.DELETE("/as/:scheduleID", sch.DeleteSchedule)
		ee.PATCH("/as/:scheduleID", sch.PatchSchedule)
		ee.POST("/as", sch.CreateSchedule)
		ee.POST("/as/:scheduleID/run", sch.TriggerScheduleRun)
		return ee
	}

	eNo := mount(false)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/gf"},
		{http.MethodGet, "/gf/n"},
		{http.MethodPut, "/gf/n"},
		{http.MethodDelete, "/gf/n"},
		{http.MethodPost, "/gf"},
		{http.MethodGet, "/cj"},
		{http.MethodGet, "/cj/j"},
		{http.MethodGet, "/cj/j/runs"},
		{http.MethodDelete, "/cj/j"},
		{http.MethodGet, "/as"},
		{http.MethodGet, "/as/s"},
		{http.MethodGet, "/as/s/runs"},
		{http.MethodDelete, "/as/s"},
		{http.MethodPatch, "/as/s"},
		{http.MethodPost, "/as"},
		{http.MethodPost, "/as/s/run"},
	} {
		rec := doRequest(eNo, tc.method, tc.path, map[string]any{"name": "X", "source": "package main"})
		if rec.Code == http.StatusOK || rec.Code == http.StatusCreated || rec.Code == http.StatusNoContent {
			t.Fatalf("%s %s expected missing project, got %d", tc.method, tc.path, rec.Code)
		}
	}

	eYes := mount(true)
	for _, path := range []string{"/gf", "/gf/n", "/cj", "/cj/j", "/cj/j/runs"} {
		rec := doRequest(eYes, http.MethodGet, path, nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s nil store status=%d %s", path, rec.Code, rec.Body.String())
		}
	}

	_ = e
}

func TestSQLSerializeAndDecodeExecute(t *testing.T) {
	svc := &fakeSQLService{
		db: catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{
			queryResult: database.QueryResult{Columns: []string{"f"}, Rows: [][]any{{func() {}}},
			},
		},
	}
	e := setupSQLTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1"},
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("serialize %d %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/databases/db-1/execute", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad execute json %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/databases/db-1/batch", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad batch json %d", rec.Code)
	}
}

func TestGoFunctionDTOAndScheduleGet(t *testing.T) {
	dto := toGoFunctionDTO(systemdb.GoFunction{Name: "Fn", Exports: nil}, true)
	if dto.Source != "" || dto.File != "Fn.go" || dto.Exports == nil || len(dto.Exports) != 0 {
		t.Fatalf("%+v", dto)
	}

	store := openAgentStore(t)
	e := echo.New()
	h := &agentScheduleHandler{store: store}
	e.GET("/s/:scheduleID", h.GetSchedule)
	e.GET("/s/:scheduleID/runs", h.ListScheduleRuns)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithProject(c.Request().Context(), ProjectContext{ID: catalog.DevProjectID})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	// re-register after middleware? echo Use is global; routes already registered.
	// Serve with injected context via wrapper
	wrap := echo.New()
	wrap.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithProject(c.Request().Context(), ProjectContext{ID: catalog.DevProjectID})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	wrap.GET("/s/:scheduleID", h.GetSchedule)
	wrap.GET("/s/:scheduleID/runs", h.ListScheduleRuns)
	rec := doRequest(wrap, http.MethodGet, "/s/missing", nil)
	if rec.Code == http.StatusOK {
		t.Fatal("missing schedule")
	}
	rec = doRequest(wrap, http.MethodGet, "/s/missing/runs", nil)
	if rec.Code == http.StatusOK {
		t.Fatal("missing schedule runs")
	}
}

func TestDataAcquireGetError(t *testing.T) {
	svc := &fakeDataService{getErr: catalog.ErrNotFound, dbs: []catalog.Database{{ID: "db-1", ProjectID: "proj-1"}}}
	e := setupDataTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
	svc.getErr = nil
	svc.acquireErr = database.ErrWriterUnavailable
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("acquire %d", rec.Code)
	}

	// legacy get after list
	svc.acquireErr = nil
	svc.getErr = catalog.ErrNotFound
	rec = doRequest(e, http.MethodGet, "/v1/projects/proj-1/data/collections", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("legacy get %d", rec.Code)
	}
}

func TestCreateCollectionExecuteError(t *testing.T) {
	svc := &fakeDataService{
		dbs:   []catalog.Database{{ID: "db-1", ProjectID: "proj-1"}},
		lease: &fakeSQLLease{executeErr: errorsNew("exec")},
	}
	e := setupDataTestRouter(t, svc, true)
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/data/collections", map[string]any{"name": "Users"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("exec err %d %s", rec.Code, rec.Body.String())
	}
}

func errorsNew(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
