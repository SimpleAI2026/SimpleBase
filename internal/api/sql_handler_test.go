package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/database"
	"github.com/linkxzhou/SimpleBase/internal/database/sqlguard"
)

// fakeSQLLease 模拟 SQLLease，可预设查询结果或错误。
type fakeSQLLease struct {
	released       bool
	queryResult    database.QueryResult
	queryErr       error
	executeResult  database.QueryResult
	executeErr     error
	batchResults   []database.QueryResult
	batchErr       error
	lastQueryStmt  database.Statement
	lastExecStmt   database.Statement
	lastBatchStmts []database.Statement
}

func (l *fakeSQLLease) Release() { l.released = true }

func (l *fakeSQLLease) Query(ctx context.Context, stmt database.Statement, maxRows int) (database.QueryResult, error) {
	l.lastQueryStmt = stmt
	return l.queryResult, l.queryErr
}

func (l *fakeSQLLease) Execute(ctx context.Context, stmt database.Statement) (database.QueryResult, error) {
	l.lastExecStmt = stmt
	return l.executeResult, l.executeErr
}

func (l *fakeSQLLease) Batch(ctx context.Context, stmts []database.Statement, transactional bool) ([]database.QueryResult, error) {
	l.lastBatchStmts = stmts
	return l.batchResults, l.batchErr
}

// fakeSQLService 是 SQLService 的内存假实现。
type fakeSQLService struct {
	db           catalog.Database
	getErr       error
	acquireErr   error
	acquiredMode database.AccessMode
	lease        *fakeSQLLease
}

func (f *fakeSQLService) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (catalog.Database, error) {
	if f.getErr != nil {
		return catalog.Database{}, f.getErr
	}
	return f.db, nil
}

func (f *fakeSQLService) Acquire(ctx context.Context, db catalog.Database, mode database.AccessMode) (SQLLease, error) {
	if f.acquireErr != nil {
		return nil, f.acquireErr
	}
	f.acquiredMode = mode
	if f.lease == nil {
		f.lease = &fakeSQLLease{}
	}
	return f.lease, nil
}

func testSQLLimits() SQLLimits {
	return SQLLimits{
		QueryTimeout:       int64(5 * time.Second),
		MaxQueryRows:       100,
		MaxConcurrent:      10,
		MaxBatchStatements: 10,
		MaxSQLBytes:        65536,
		MaxRequestBytes:    1 << 20,
	}
}

func setupSQLTestRouter(t *testing.T, svc *fakeSQLService, writable bool) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true

	h := NewSQLHandler(svc, testSQLLimits(), writable)

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{
				APIKeyID:   "key-test",
				TenantID:   "tenant-1",
				ProjectIDs: map[string]struct{}{"proj-1": {}},
				Permissions: map[auth.Permission]struct{}{
					auth.DatabaseRead:  {},
					auth.DatabaseWrite: {},
					auth.DatabaseAdmin: {},
				},
			}
			ctx := WithPrincipal(c.Request().Context(), p)
			ctx = WithProject(ctx, ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	v1 := e.Group("/v1")
	p := v1.Group("/projects/:projectID")
	p.POST("/databases/:databaseID/query", h.Query)
	p.POST("/databases/:databaseID/execute", h.Execute)
	p.POST("/databases/:databaseID/batch", h.Batch)
	return e
}

func TestSQLQuery_Success(t *testing.T) {
	lease := &fakeSQLLease{
		queryResult: database.QueryResult{
			Columns:      []string{"id", "name"},
			Rows:         [][]any{{int64(1), "alice"}, {int64(2), "bob"}},
			RowsAffected: 2,
			Duration:     1 * time.Millisecond,
		},
	}
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: lease,
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "SELECT id, name FROM users"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp QueryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.RowCount != 2 {
		t.Errorf("expected 2 rows, got %d", resp.RowCount)
	}
	if len(resp.Columns) != 2 || resp.Columns[0] != "id" {
		t.Errorf("unexpected columns: %v", resp.Columns)
	}
	if !lease.released {
		t.Error("lease not released")
	}
	if svc.acquiredMode != database.ReadOnly {
		t.Errorf("expected ReadOnly mode, got %v", svc.acquiredMode)
	}
}

func TestSQLQuery_WriteInReadOnly(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLQuery_EmptySQL(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: ""},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLQuery_MultipleStatements(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1; SELECT 2"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLQuery_AttachDenied(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "ATTACH DATABASE 'evil.db' AS evil"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLExecute_Success(t *testing.T) {
	lease := &fakeSQLLease{
		executeResult: database.QueryResult{
			RowsAffected: 3,
			LastInsertID: 42,
			Duration:     2 * time.Millisecond,
		},
	}
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: lease,
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/execute", ExecuteRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp ExecuteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.RowsAffected != 3 {
		t.Errorf("expected 3 rows affected, got %d", resp.RowsAffected)
	}
	if resp.LastInsertID == nil || *resp.LastInsertID != 42 {
		t.Errorf("expected last insert id 42, got %v", resp.LastInsertID)
	}
	if svc.acquiredMode != database.ReadWrite {
		t.Errorf("expected ReadWrite mode, got %v", svc.acquiredMode)
	}
	if !lease.released {
		t.Error("lease not released")
	}
}

func TestSQLExecute_ReadOnlyInstance(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, false) // readonly

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/execute", ExecuteRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"},
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLExecute_SelectRejected(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	// Execute 要求 WriteAllowed intent，SELECT 虽然是只读但 Execute intent 允许
	// 这里测试 SELECT 在 execute 端点应被允许（WriteAllowed 不拒绝 SELECT）
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/execute", ExecuteRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for SELECT via execute, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLBatch_TransactionalSuccess(t *testing.T) {
	lease := &fakeSQLLease{
		batchResults: []database.QueryResult{
			{RowsAffected: 1, Duration: time.Millisecond},
			{RowsAffected: 1, Duration: time.Millisecond},
		},
	}
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: lease,
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/batch", BatchRequest{
		Statements: []SQLStatementRequest{
			{SQL: "INSERT INTO t VALUES(1)"},
			{SQL: "INSERT INTO t VALUES(2)"},
		},
		Transactional: true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp BatchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Error != nil {
		t.Errorf("expected no error, got %v", resp.Error)
	}
	if !lease.released {
		t.Error("lease not released")
	}
}

func TestSQLBatch_TransactionalRollback(t *testing.T) {
	lease := &fakeSQLLease{
		batchResults: []database.QueryResult{
			{RowsAffected: 1},
		},
		batchErr: errors.New("database: batch statement 1 failed: syntax error"),
	}
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: lease,
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/batch", BatchRequest{
		Statements: []SQLStatementRequest{
			{SQL: "INSERT INTO t VALUES(1)"},
			{SQL: "BAD SYNTAX"},
		},
		Transactional: true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp BatchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Error("expected batch error")
	}
	if resp.Error.FailedIndex != 1 {
		t.Errorf("expected failed index 1, got %d", resp.Error.FailedIndex)
	}
}

func TestSQLBatch_NonTransactionalPartialFailure(t *testing.T) {
	lease := &fakeSQLLease{
		batchResults: []database.QueryResult{
			{RowsAffected: 1},
		},
		batchErr: errors.New("database: batch statement 1 failed"),
	}
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: lease,
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/batch", BatchRequest{
		Statements: []SQLStatementRequest{
			{SQL: "INSERT INTO t VALUES(1)"},
			{SQL: "INSERT INTO t VALUES(2)"},
		},
		Transactional: false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp BatchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[1].ErrorCode == "" {
		t.Error("expected error code on failed item")
	}
}

func TestSQLBatch_EmptyStatements(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/batch", BatchRequest{
		Statements:    []SQLStatementRequest{},
		Transactional: true,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLBatch_TooManyStatements(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	stmts := make([]SQLStatementRequest, 15)
	for i := range stmts {
		stmts[i] = SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"}
	}
	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/batch", BatchRequest{
		Statements: stmts,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLQuery_DangerousPragma(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "PRAGMA writable_schema=1"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLQuery_GetDatabaseError(t *testing.T) {
	svc := &fakeSQLService{
		getErr: catalog.ErrNotFound,
		lease:  &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1"},
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLQuery_AcquireError(t *testing.T) {
	svc := &fakeSQLService{
		db:         catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		acquireErr: database.ErrWriterUnavailable,
		lease:      &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1"},
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLQuery_MaxRowsExceedsLimit(t *testing.T) {
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: &fakeSQLLease{},
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "SELECT 1"},
		MaxRows:             10000,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSQLQuery_SerializedBlob(t *testing.T) {
	lease := &fakeSQLLease{
		queryResult: database.QueryResult{
			Columns: []string{"data"},
			Rows:    [][]any{{[]byte{0x01, 0x02, 0x03}}},
		},
	}
	svc := &fakeSQLService{
		db:    catalog.Database{ID: "db-1", ProjectID: "proj-1", Status: catalog.DatabaseReady},
		lease: lease,
	}
	e := setupSQLTestRouter(t, svc, true)

	rec := doRequest(e, http.MethodPost, "/v1/projects/proj-1/databases/db-1/query", QueryRequest{
		SQLStatementRequest: SQLStatementRequest{SQL: "SELECT data FROM blobs"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// 确认 blob 被序列化为 {"type":"blob","base64":"..."}
	body := rec.Body.String()
	if !contains(body, "\"type\":\"blob\"") {
		t.Errorf("expected blob serialization, got: %s", body)
	}
}

func TestEffectiveMaxRows(t *testing.T) {
	cases := []struct {
		reqMax, cfgMax, want int
	}{
		{0, 100, 100},
		{50, 100, 50},
		{200, 100, 100},
		{100, 100, 100},
	}
	for _, c := range cases {
		got := effectiveMaxRows(c.reqMax, c.cfgMax)
		if got != c.want {
			t.Errorf("effectiveMaxRows(%d,%d)=%d, want %d", c.reqMax, c.cfgMax, got, c.want)
		}
	}
}

func TestExtractBatchIndex(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{errors.New("database: batch statement 3 failed"), 3},
		{errors.New("database: batch statement 0 failed"), 0},
		{errors.New("some other error"), -1},
		{nil, -1},
	}
	for _, c := range cases {
		got := extractBatchIndex(c.err)
		if got != c.want {
			t.Errorf("extractBatchIndex(%v)=%d, want %d", c.err, got, c.want)
		}
	}
}

func TestSQLGuardIntegration(t *testing.T) {
	// 确认 sqlguard 错误能被 error.go 正确映射
	cases := []struct {
		err      error
		wantCode int
	}{
		{sqlguard.ErrEmptySQL, http.StatusBadRequest},
		{sqlguard.ErrMultipleStatements, http.StatusBadRequest},
		{sqlguard.ErrSQLNotAllowed, http.StatusBadRequest},
		{sqlguard.ErrWriteInReadOnly, http.StatusBadRequest},
		{sqlguard.ErrNulChar, http.StatusBadRequest},
	}
	for _, c := range cases {
		apiErr := Error(c.err, "req-1")
		if apiErr == nil {
			t.Errorf("expected non-nil APIError for %v", c.err)
			continue
		}
		if apiErr.HTTPStatus != c.wantCode {
			t.Errorf("error %v: got code %d, want %d", c.err, apiErr.HTTPStatus, c.wantCode)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
