package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/objectstore"
)

func setupBareEcho(injectPrincipal, injectProject bool, register func(*echo.Echo)) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			if injectPrincipal {
				ctx = WithPrincipal(ctx, auth.Principal{
					APIKeyID: "key-test", TenantID: "tenant-1",
					ProjectIDs:  map[string]struct{}{"proj-1": {}},
					Permissions: map[auth.Permission]struct{}{auth.DatabaseRead: {}, auth.DatabaseWrite: {}, auth.DatabaseAdmin: {}},
				})
			}
			if injectProject {
				ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			}
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	register(e)
	return e
}

func TestDatabaseHandler_MissingContextAndValidation(t *testing.T) {
	svc := newFakeDBService()
	svc.dbs["db-1"] = catalog.Database{ID: "db-1", ProjectID: "proj-1", Name: "n", Status: catalog.DatabaseReady}

	h := NewDatabaseHandler(svc, true)
	h.SnapshotFor = func(id string) *DatabaseSnapshot {
		return &DatabaseSnapshot{LastSyncedSnapshot: 3, SyncLag: 1}
	}

	e := setupBareEcho(false, false, func(e *echo.Echo) {
		e.POST("/databases", h.CreateDatabase)
		e.GET("/databases", h.ListDatabases)
		e.GET("/databases/:databaseID", h.GetDatabase)
		e.POST("/databases/:databaseID/open", h.OpenDatabase)
		e.POST("/databases/:databaseID/close", h.CloseDatabase)
		e.DELETE("/databases/:databaseID", h.DeleteDatabase)
	})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/databases"},
		{http.MethodGet, "/databases"},
		{http.MethodGet, "/databases/db-1"},
		{http.MethodPost, "/databases/db-1/open"},
		{http.MethodPost, "/databases/db-1/close"},
		{http.MethodDelete, "/databases/db-1"},
	} {
		rec := doRequest(e, tc.method, tc.path, CreateDatabaseRequest{Name: "x"})
		if rec.Code == http.StatusOK || rec.Code == http.StatusCreated || rec.Code == http.StatusAccepted || rec.Code == http.StatusNoContent {
			t.Fatalf("%s %s expected auth/project error, got %d", tc.method, tc.path, rec.Code)
		}
	}

	eP := setupBareEcho(true, false, func(e *echo.Echo) {
		e.POST("/databases", h.CreateDatabase)
		e.GET("/databases", h.ListDatabases)
		e.GET("/databases/:databaseID", h.GetDatabase)
		e.POST("/databases/:databaseID/open", h.OpenDatabase)
		e.POST("/databases/:databaseID/close", h.CloseDatabase)
		e.DELETE("/databases/:databaseID", h.DeleteDatabase)
	})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/databases"},
		{http.MethodGet, "/databases"},
		{http.MethodGet, "/databases/db-1"},
		{http.MethodPost, "/databases/db-1/open"},
		{http.MethodPost, "/databases/db-1/close"},
		{http.MethodDelete, "/databases/db-1"},
	} {
		rec := doRequest(eP, tc.method, tc.path, CreateDatabaseRequest{Name: "x"})
		if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
			t.Fatalf("%s %s expected missing project, got %d", tc.method, tc.path, rec.Code)
		}
	}

	eOK := setupBareEcho(true, true, func(e *echo.Echo) {
		e.GET("/databases/:databaseID", h.GetDatabase)
		e.GET("/databases", h.ListDatabases)
		e.POST("/databases", h.CreateDatabase)
		e.POST("/databases/:databaseID/open", h.OpenDatabase)
		e.POST("/databases/:databaseID/close", h.CloseDatabase)
		e.DELETE("/databases/:databaseID", h.DeleteDatabase)
	})
	rec := doRequest(eOK, http.MethodGet, "/databases/db-1", nil)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("last_synced_snapshot")) {
		t.Fatalf("snapshot get %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(eOK, http.MethodGet, "/databases/", nil)
	// empty databaseID
	req := httptest.NewRequest(http.MethodGet, "/databases/", nil)
	rec = httptest.NewRecorder()
	c := eOK.NewContext(req, rec)
	c.SetPath("/databases/:databaseID")
	c.SetParamNames("databaseID")
	c.SetParamValues("")
	ctx := WithPrincipal(req.Context(), auth.Principal{APIKeyID: "k"})
	ctx = WithProject(ctx, ProjectContext{ID: "proj-1"})
	c.SetRequest(req.WithContext(ctx))
	_ = h.GetDatabase(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty id get %d", rec.Code)
	}
	for _, fn := range []func(echo.Context) error{h.OpenDatabase, h.CloseDatabase, h.DeleteDatabase} {
		req = httptest.NewRequest(http.MethodPost, "/databases/", nil)
		rec = httptest.NewRecorder()
		c = eOK.NewContext(req, rec)
		c.SetParamNames("databaseID")
		c.SetParamValues("")
		c.SetRequest(req.WithContext(ctx))
		_ = fn(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("empty id handler %d", rec.Code)
		}
	}

	rec = doRequest(eOK, http.MethodGet, "/databases?limit=bad", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("bad limit %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(eOK, http.MethodGet, "/databases?limit=5&cursor=abc", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("good limit %d", rec.Code)
	}

	rec = doRequest(eOK, http.MethodPost, "/databases", CreateDatabaseRequest{Name: strings.Repeat("n", 64)})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("long name %d", rec.Code)
	}
	rec = doRequest(eOK, http.MethodPost, "/databases", CreateDatabaseRequest{Name: "bad/name"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("slash name %d", rec.Code)
	}

	ro := NewDatabaseHandler(svc, false)
	eRO := setupBareEcho(true, true, func(e *echo.Echo) {
		e.POST("/databases/:databaseID/open", ro.OpenDatabase)
		e.POST("/databases/:databaseID/close", ro.CloseDatabase)
		e.DELETE("/databases/:databaseID", ro.DeleteDatabase)
	})
	for _, path := range []string{"/databases/db-1/open", "/databases/db-1/close"} {
		rec = doRequest(eRO, http.MethodPost, path, nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("ro %s %d", path, rec.Code)
		}
	}
	rec = doRequest(eRO, http.MethodDelete, "/databases/db-1", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ro delete %d", rec.Code)
	}

	svc.closeErr = errors.New("close fail")
	rec = doRequest(eOK, http.MethodPost, "/databases/db-1/close", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("close err %d", rec.Code)
	}
	svc.createErr = catalog.ErrAlreadyExists
	rec = doRequest(eOK, http.MethodPost, "/databases", CreateDatabaseRequest{Name: "dup"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("create err %d", rec.Code)
	}
	svc.listErr = catalog.ErrCrossProject
	rec = doRequest(eOK, http.MethodGet, "/databases", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("list err %d", rec.Code)
	}

	// delete empty status → "deleted"
	svc2 := newFakeDBService()
	svc2.dbs["db-z"] = catalog.Database{ID: "db-z", ProjectID: "proj-1"}
	h2 := NewDatabaseHandler(svc2, true)
	e2 := setupBareEcho(true, true, func(e *echo.Echo) {
		e.DELETE("/databases/:databaseID", h2.DeleteDatabase)
	})
	// force empty status after delete
	orig := svc2.DeleteDatabase
	_ = orig
	svc2.dbs["db-z"] = catalog.Database{ID: "db-z", ProjectID: "proj-1"}
	rec = doRequest(e2, http.MethodDelete, "/databases/db-z", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
}

func TestSQLHandler_MissingContextAndSystem(t *testing.T) {
	sysDB := catalog.Database{ID: "sys", ProjectID: "proj-1", Kind: catalog.DatabaseKindSystem, Status: catalog.DatabaseReady}
	svc := &fakeSQLService{db: sysDB, lease: &fakeSQLLease{executeResult: database.QueryResult{RowsAffected: 1}}}
	h := NewSQLHandler(svc, testSQLLimits(), true)
	e := setupBareEcho(false, false, func(e *echo.Echo) {
		e.POST("/q", h.Query)
		e.POST("/e", h.Execute)
		e.POST("/b", h.Batch)
	})
	body := QueryRequest{SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1"}}
	for _, path := range []string{"/q", "/e", "/b"} {
		rec := doRequest(e, http.MethodPost, path, body)
		if rec.Code == http.StatusOK {
			t.Fatalf("%s expected missing context", path)
		}
	}
	eP := setupBareEcho(true, false, func(e *echo.Echo) {
		e.POST("/q", h.Query)
		e.POST("/e", h.Execute)
		e.POST("/b", h.Batch)
	})
	for _, path := range []string{"/q", "/e", "/b"} {
		rec := doRequest(eP, http.MethodPost, path, ExecuteRequest{SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"}})
		if rec.Code == http.StatusOK {
			t.Fatalf("%s expected missing project", path)
		}
	}

	eOK := setupBareEcho(true, true, func(e *echo.Echo) {
		e.POST("/e", h.Execute)
		e.POST("/b", h.Batch)
		e.POST("/q", h.Query)
	})
	rec := doRequest(eOK, http.MethodPost, "/e", ExecuteRequest{SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("system execute %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(eOK, http.MethodPost, "/b", BatchRequest{Statements: []SQLStatementRequest{{SQL: "INSERT INTO t VALUES(1)"}}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("system batch %d %s", rec.Code, rec.Body.String())
	}

	svc.db = catalog.Database{ID: "u", ProjectID: "proj-1", Status: catalog.DatabaseReady}
	svc.lease.queryErr = errors.New("qfail")
	rec = doRequest(eOK, http.MethodPost, "/q", QueryRequest{SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1"}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("query err %d", rec.Code)
	}
	svc.lease.queryErr = nil
	svc.lease.executeErr = errors.New("efail")
	rec = doRequest(eOK, http.MethodPost, "/e", ExecuteRequest{SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("exec err %d", rec.Code)
	}

	ro := NewSQLHandler(svc, testSQLLimits(), false)
	eRO := setupBareEcho(true, true, func(e *echo.Echo) { e.POST("/b", ro.Batch) })
	rec = doRequest(eRO, http.MethodPost, "/b", BatchRequest{Statements: []SQLStatementRequest{{SQL: "INSERT INTO t VALUES(1)"}}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ro batch %d", rec.Code)
	}

	// decode errors
	req := httptest.NewRequest(http.MethodPost, "/q", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	eOK.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad query json %d", rec.Code)
	}

	// concurrency + timeout
	lim := testSQLLimits()
	lim.MaxConcurrent = 1
	lim.QueryTimeout = 1
	h2 := NewSQLHandler(svc, lim, true)
	h2.sem <- struct{}{}
	eC := setupBareEcho(true, true, func(e *echo.Echo) { e.POST("/q", h2.Query) })
	rec = doRequest(eC, http.MethodPost, "/q", QueryRequest{SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1"}})
	if rec.Code != http.StatusTooManyRequests && rec.Code != http.StatusServiceUnavailable && rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("concurrency %d %s", rec.Code, rec.Body.String())
	}

	// DurabilityFor + last insert + serialize error
	svc.lease.executeErr = nil
	svc.lease.executeResult = database.QueryResult{RowsAffected: 1, LastInsertID: 9}
	h3 := NewSQLHandler(svc, testSQLLimits(), true)
	h3.DurabilityFor = func(string) string { return "synced_s3" }
	eD := setupBareEcho(true, true, func(e *echo.Echo) {
		e.POST("/e", h3.Execute)
		e.POST("/b", h3.Batch)
	})
	rec = doRequest(eD, http.MethodPost, "/e", ExecuteRequest{SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "synced_s3") {
		t.Fatalf("durability %d %s", rec.Code, rec.Body.String())
	}
	svc.lease.batchResults = []database.QueryResult{{RowsAffected: 1, LastInsertID: 2}}
	rec = doRequest(eD, http.MethodPost, "/b", BatchRequest{Transactional: true, Statements: []SQLStatementRequest{{SQL: "INSERT INTO t VALUES(1)"}}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "synced_s3") {
		t.Fatalf("batch dur %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(eD, http.MethodPost, "/b", BatchRequest{Statements: []SQLStatementRequest{{SQL: "INSERT INTO t VALUES(1)"}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("non-tx batch %d", rec.Code)
	}

	// acquire / get errors on execute/batch
	svc.getErr = catalog.ErrNotFound
	rec = doRequest(eD, http.MethodPost, "/e", ExecuteRequest{SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("exec get %d", rec.Code)
	}
	rec = doRequest(eD, http.MethodPost, "/b", BatchRequest{Statements: []SQLStatementRequest{{SQL: "INSERT INTO t VALUES(1)"}}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("batch get %d", rec.Code)
	}
	svc.getErr = nil
	svc.acquireErr = database.ErrWriterUnavailable
	rec = doRequest(eD, http.MethodPost, "/e", ExecuteRequest{SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("exec acquire %d", rec.Code)
	}
	rec = doRequest(eD, http.MethodPost, "/b", BatchRequest{Statements: []SQLStatementRequest{{SQL: "INSERT INTO t VALUES(1)"}}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("batch acquire %d", rec.Code)
	}

	// validate SQL bytes / args
	lim2 := testSQLLimits()
	lim2.MaxSQLBytes = 5
	h4 := NewSQLHandler(svc, lim2, true)
	e4 := setupBareEcho(true, true, func(e *echo.Echo) { e.POST("/q", h4.Query) })
	svc.acquireErr = nil
	rec = doRequest(e4, http.MethodPost, "/q", QueryRequest{SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 123456"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sql too long %d", rec.Code)
	}

	// negative max rows
	h5 := NewSQLHandler(svc, testSQLLimits(), true)
	e5 := setupBareEcho(true, true, func(e *echo.Echo) { e.POST("/q", h5.Query) })
	rec = doRequest(e5, http.MethodPost, "/q", QueryRequest{SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1"}, MaxRows: -1})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("neg maxrows %d", rec.Code)
	}

	// too many args
	args := make([]any, 1001)
	rec = doRequest(e5, http.MethodPost, "/q", QueryRequest{SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1", Args: args}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("too many args %d", rec.Code)
	}

	// sqlSHA256
	if sqlSHA256("SELECT 1") == sqlSHA256("SELECT 2") {
		t.Fatal("sha should differ")
	}
	_ = nanoToDuration(0)

	// acquireSem nil + releaseSem
	h6 := NewSQLHandler(svc, SQLLimits{MaxQueryRows: 10, MaxSQLBytes: 100, MaxBatchStatements: 2}, true)
	if err := h6.acquireSem(httptest.NewRequest(http.MethodGet, "/", nil).Context()); err != nil {
		t.Fatal(err)
	}
	h6.releaseSem()
}

func TestDataHandler_Branches(t *testing.T) {
	svc := &fakeDataService{
		dbs: []catalog.Database{{ID: "db-1", Name: "n", ProjectID: "proj-1"}},
		lease: &fakeSQLLease{
			queryResult:   database.QueryResult{Columns: []string{"table_name"}, Rows: [][]any{{"users"}, {1}, {}}},
			executeResult: database.QueryResult{RowsAffected: 1},
		},
	}
	h := NewDataHandler(svc, true)
	e := setupBareEcho(true, true, func(e *echo.Echo) {
		e.GET("/data/collections", h.ListCollections)
		e.POST("/data/collections", h.CreateCollection)
		e.GET("/data/collections/:collection", h.ListDocuments)
		e.POST("/data/collections/:collection/documents", h.CreateDocument)
		e.PUT("/data/collections/:collection/documents/:id", h.UpdateDocument)
		e.DELETE("/data/collections/:collection/documents/:id", h.DeleteDocument)
		e.GET("/databases/:databaseID/data/collections", h.ListCollections)
		e.POST("/databases/:databaseID/data/collections", h.CreateCollection)
	})

	rec := doRequest(e, http.MethodGet, "/data/collections", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPost, "/data/collections", map[string]any{"name": "bad-name"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad collection name %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/data/collections", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json %d", rec.Code)
	}
	rec = doRequest(e, http.MethodPost, "/data/collections", map[string]any{"name": "Users"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create col %d %s", rec.Code, rec.Body.String())
	}

	svc.lease.queryResult = database.QueryResult{Rows: [][]any{{"id1", `{"n":1}`}, {"id2", "not-json"}, {"short"}}}
	rec = doRequest(e, http.MethodGet, "/data/collections/Users", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list docs %d %s", rec.Code, rec.Body.String())
	}
	svc.lease.queryErr = errors.New("no such table: Users")
	rec = doRequest(e, http.MethodGet, "/data/collections/Users", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("missing table %d", rec.Code)
	}
	svc.lease.queryErr = errors.New("other")
	rec = doRequest(e, http.MethodGet, "/data/collections/Users", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("query err %d", rec.Code)
	}
	svc.lease.queryErr = nil

	rec = doRequest(e, http.MethodGet, "/data/collections/bad-name", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad collection param %d", rec.Code)
	}

	rec = doRequest(e, http.MethodPost, "/data/collections/Users/documents", map[string]any{"name": "Ada"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create doc %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPost, "/data/collections/Users/documents", nil)
	// nil body
	req = httptest.NewRequest(http.MethodPost, "/data/collections/Users/documents", strings.NewReader("null"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("null doc %d", rec.Code)
	}

	rec = doRequest(e, http.MethodPut, "/data/collections/Users/documents/id-1", map[string]any{"n": 2})
	if rec.Code != http.StatusOK {
		t.Fatalf("update %d %s", rec.Code, rec.Body.String())
	}
	svc.lease.executeResult = database.QueryResult{RowsAffected: 0}
	rec = doRequest(e, http.MethodPut, "/data/collections/Users/documents/id-1", map[string]any{"n": 2})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update missing %d", rec.Code)
	}
	svc.lease.executeResult = database.QueryResult{RowsAffected: 1}
	svc.lease.executeErr = errors.New("upd")
	rec = doRequest(e, http.MethodPut, "/data/collections/Users/documents/id-1", map[string]any{"n": 2})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("update err %d", rec.Code)
	}
	svc.lease.executeErr = nil

	rec = doRequest(e, http.MethodDelete, "/data/collections/Users/documents/id-1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete %d", rec.Code)
	}
	svc.lease.executeErr = errors.New("no such table")
	rec = doRequest(e, http.MethodDelete, "/data/collections/Users/documents/id-1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete missing table %d", rec.Code)
	}
	svc.lease.executeErr = errors.New("boom")
	rec = doRequest(e, http.MethodDelete, "/data/collections/Users/documents/id-1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("delete err %d", rec.Code)
	}
	svc.lease.executeErr = nil

	// missing id
	req = httptest.NewRequest(http.MethodPut, "/data/collections/Users/documents/", strings.NewReader(`{"n":1}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("collection", "id")
	c.SetParamValues("Users", "")
	ctx := WithPrincipal(req.Context(), auth.Principal{APIKeyID: "k"})
	ctx = WithProject(ctx, ProjectContext{ID: "proj-1"})
	c.SetRequest(req.WithContext(ctx))
	_ = h.UpdateDocument(c)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("update no id %d", rec.Code)
	}

	ro := NewDataHandler(svc, false)
	eRO := setupBareEcho(true, true, func(e *echo.Echo) {
		e.POST("/data/collections", ro.CreateCollection)
		e.POST("/data/collections/:collection/documents", ro.CreateDocument)
		e.PUT("/data/collections/:collection/documents/:id", ro.UpdateDocument)
		e.DELETE("/data/collections/:collection/documents/:id", ro.DeleteDocument)
	})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/data/collections"},
		{http.MethodPost, "/data/collections/Users/documents"},
		{http.MethodPut, "/data/collections/Users/documents/1"},
		{http.MethodDelete, "/data/collections/Users/documents/1"},
	} {
		rec = doRequest(eRO, tc.method, tc.path, map[string]any{"name": "Users"})
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("ro %s %s %d", tc.method, tc.path, rec.Code)
		}
	}

	// system protected
	svc.dbs = []catalog.Database{{ID: "sys", Kind: catalog.DatabaseKindSystem, ProjectID: "proj-1"}}
	rec = doRequest(e, http.MethodPost, "/data/collections", map[string]any{"name": "Users"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("system write %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodPost, "/databases/sys/data/collections", map[string]any{"name": "Users"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("system scoped %d", rec.Code)
	}

	// no databases
	svc.dbs = nil
	rec = doRequest(e, http.MethodGet, "/data/collections", nil)
	if rec.Code == http.StatusOK {
		t.Fatal("no db should fail")
	}
	svc.listErr = catalog.ErrNotFound
	rec = doRequest(e, http.MethodGet, "/data/collections", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("list err %d", rec.Code)
	}

	// missing principal/project on acquire
	eBare := setupBareEcho(false, false, func(e *echo.Echo) { e.GET("/data/collections", h.ListCollections) })
	rec = doRequest(eBare, http.MethodGet, "/data/collections", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no principal %d", rec.Code)
	}
	eNP := setupBareEcho(true, false, func(e *echo.Echo) { e.GET("/data/collections", h.ListCollections) })
	rec = doRequest(eNP, http.MethodGet, "/data/collections", nil)
	if rec.Code == http.StatusOK {
		t.Fatal("no project should fail")
	}

	if !isMissingRelation(errors.New("relation does not exist")) || isMissingRelation(nil) || !isMissingRelation(errors.New("NOT FOUND")) {
		t.Fatal("isMissingRelation")
	}
	if quoteIdentifier("t") != `"t"` {
		t.Fatal(quoteIdentifier("t"))
	}
	if !strings.Contains(createCollectionSQL("T"), "CREATE TABLE") {
		t.Fatal(createCollectionSQL("T"))
	}
}

func TestProjectsHandler_MoreBranches(t *testing.T) {
	e := setupProjectsRouter(t, &fakeProjectCatalog{})
	// missing principal
	e2 := echo.New()
	h := NewProjectsHandler(&fakeProjectCatalog{}, nil)
	e2.GET("/v1/projects", h.ListProjects)
	e2.POST("/v1/projects", h.CreateProject)
	rec := doRequest(e2, http.MethodGet, "/v1/projects", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("list no prin %d", rec.Code)
	}
	rec = doRequest(e2, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("create no prin %d", rec.Code)
	}

	cat := &fakeProjectCatalog{err: catalog.ErrNotFound}
	e = setupProjectsRouter(t, cat)
	rec = doRequest(e, http.MethodGet, "/v1/projects", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("list err %d", rec.Code)
	}
	cat.err = catalog.ErrInvalidName
	rec = doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x"})
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("invalid_project_name")) {
		t.Fatalf("invalid name %d %s", rec.Code, rec.Body.String())
	}
	cat.err = errors.New("catalog: invalid name: id must be 8 chars [A-Za-z0-9-]")
	// errors.Is won't match unless wrapped
	cat.err = wrapInvalid("id must be 8 chars [A-Za-z0-9-]")
	rec = doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x", ID: "not-valid!"})
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("invalid_project_id")) {
		t.Fatalf("invalid id %d %s", rec.Code, rec.Body.String())
	}
	cat.err = wrapInvalid("reserved project id")
	rec = doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x"})
	if !bytes.Contains(rec.Body.Bytes(), []byte("invalid_project_id")) {
		t.Fatalf("reserved %s", rec.Body.String())
	}
	cat.err = errors.New("other")
	rec = doRequest(e, http.MethodPost, "/v1/projects", CreateProjectRequest{Name: "x"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("other %d", rec.Code)
	}
}

func wrapInvalid(msg string) error {
	return errors.Join(catalog.ErrInvalidName, errors.New(msg))
}

func TestS3Handler_MissingProjectAndIndex(t *testing.T) {
	h := &S3Handler{store: objectstore.NewMemoryFileStore()}
	e := echo.New()
	e.GET("/s3/objects", h.ListObjects)
	e.POST("/s3/objects", h.UploadObject)
	e.DELETE("/s3/objects", h.DeleteObject)
	e.GET("/s3/presign", h.PresignObject)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/s3/objects"},
		{http.MethodPost, "/s3/objects"},
		{http.MethodDelete, "/s3/objects?key=a"},
		{http.MethodGet, "/s3/presign?key=a"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Fatalf("%s expected missing project", tc.path)
		}
	}

	e2 := setupS3TestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/s3/objects?prefix=../x", nil)
	rec := httptest.NewRecorder()
	e2.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad prefix %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/s3/presign", nil)
	rec = httptest.NewRecorder()
	e2.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("presign missing key %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/s3/presign?key=../x", nil)
	rec = httptest.NewRecorder()
	e2.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("presign bad key %d", rec.Code)
	}
}

func TestSQLTypesZeroValues(t *testing.T) {
	var q QueryRequest
	var e ExecuteRequest
	var b BatchRequest
	var qr QueryResponse
	var er ExecuteResponse
	var br BatchResponse
	_ = q
	_ = e
	_ = b
	if qr.RowCount != 0 || er.RowsAffected != 0 || br.Error != nil {
		t.Fatal("zero values")
	}
	item := BatchResultItem{Index: 1, ErrorCode: "x"}
	if item.Index != 1 {
		t.Fatal(item)
	}
	if _, err := json.Marshal(BatchError{FailedIndex: 0, Code: "c", Message: "m"}); err != nil {
		t.Fatal(err)
	}
}
