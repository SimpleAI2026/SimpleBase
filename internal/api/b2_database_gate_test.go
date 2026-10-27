package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// B2-database 表征（planv6.0 §8 B2）：GetDatabase → IsSystemDatabase 连续段在
// Execute/Batch 入口的行为——只读实例不解码不调用服务、非法 SQL 不 GetDatabase、
// 系统库不 Acquire、GetDatabase 失败不 Acquire、Acquire 失败仍持有 semaphore、
// 成功 lease 恰好 Release 一次——作为抽取取库/系统库拒写 helper 的等价证据。

func TestSQLHandler_ExecuteDatabaseGate(t *testing.T) {
	execReq := func(h *SQLHandler) (int, string) {
		return runSQL(t, h, http.MethodPost, "/databases/u/e", ExecuteRequest{SQLStatementRequest: SQLStatementRequest{SQL: "INSERT INTO t VALUES(1)"}})
	}
	newUserSvc := func() *fakeSQLService {
		return &fakeSQLService{
			db: catalog.Database{ID: "u", ProjectID: "proj-1", Status: catalog.DatabaseReady},
			lease: &fakeSQLLease{
				executeResult: godatabase.QueryResult{RowsAffected: 1},
				batchResults:  []godatabase.QueryResult{{RowsAffected: 1}},
			},
		}
	}

	// 1. 只读实例：不解码请求体、不调用任何服务方法。
	{
		svc := newUserSvc()
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), false)
		code, body := execReq(h)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("read-only status=%d body=%s", code, body)
		}
		if svc.calls.get != 0 || svc.calls.acquire != 0 {
			t.Fatalf("read-only must not touch service: %+v", *svc.calls)
		}
	}

	// 2. 非法 SQL：不 GetDatabase、不 Acquire。
	{
		svc := newUserSvc()
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), true)
		code, _ := runSQL(t, h, http.MethodPost, "/databases/u/e", ExecuteRequest{SQLStatementRequest: SQLStatementRequest{SQL: "SET max_threads=4"}})
		if code == http.StatusOK {
			t.Fatalf("invalid SQL unexpectedly succeeded")
		}
		if svc.calls.get != 0 || svc.calls.acquire != 0 {
			t.Fatalf("invalid SQL must not GetDatabase/Acquire: %+v", *svc.calls)
		}
	}

	// 3. 系统库：拒绝写入且不 Acquire、不持有 lease。
	{
		svc := newUserSvc()
		svc.db = catalog.Database{ID: "sys", ProjectID: "proj-1", Status: catalog.DatabaseReady, Kind: catalog.DatabaseKindSystem}
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), true)
		code, body := execReq(h)
		if code != http.StatusForbidden || !strings.Contains(body, "system") {
			t.Fatalf("system db status=%d body=%s", code, body)
		}
		if svc.calls.acquire != 0 {
			t.Fatalf("system db must not Acquire: acquire=%d", svc.calls.acquire)
		}
	}

	// 4. GetDatabase 失败：不 Acquire。
	{
		svc := newUserSvc()
		svc.getErr = catalog.ErrNotFound
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), true)
		code, _ := execReq(h)
		if code != http.StatusNotFound {
			t.Fatalf("get err status=%d", code)
		}
		if svc.calls.acquire != 0 {
			t.Fatalf("GetDatabase failure must not Acquire: %d", svc.calls.acquire)
		}
	}

	// 5. Acquire 失败：lease 为 nil，错误原样返回。
	{
		svc := newUserSvc()
		svc.acquireErr = godatabase.ErrWriterUnavailable
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), true)
		code, _ := execReq(h)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("acquire err status=%d", code)
		}
		if svc.lease != nil && svc.lease.released {
			t.Fatalf("acquire failure must not release a lease")
		}
	}

	// 6. 成功：恰好 Acquire 一次（ReadWrite），lease 恰好 Release 一次。
	{
		svc := newUserSvc()
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), true)
		code, body := execReq(h)
		if code != http.StatusOK || !strings.Contains(body, `"rows_affected":1`) {
			t.Fatalf("success status=%d body=%s", code, body)
		}
		if svc.calls.acquire != 1 || svc.acquiredMode != godatabase.ReadWrite {
			t.Fatalf("acquire=%d mode=%v want 1 ReadWrite", svc.calls.acquire, svc.acquiredMode)
		}
		if !svc.lease.released {
			t.Fatalf("lease must be released exactly once")
		}
	}
}

func TestSQLHandler_BatchDatabaseGate(t *testing.T) {
	txReq := func(h *SQLHandler) (int, string) {
		return runSQL(t, h, http.MethodPost, "/databases/u/b", BatchRequest{Transactional: true, Statements: []SQLStatementRequest{{SQL: "INSERT INTO t VALUES(1)"}}})
	}
	newUserSvc := func() *fakeSQLService {
		return &fakeSQLService{
			db: catalog.Database{ID: "u", ProjectID: "proj-1", Status: catalog.DatabaseReady},
			lease: &fakeSQLLease{
				executeResult: godatabase.QueryResult{RowsAffected: 1},
				batchResults:  []godatabase.QueryResult{{RowsAffected: 1}},
			},
		}
	}

	// 1. 只读实例：不解码、不调用服务。
	{
		svc := newUserSvc()
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), false)
		code, body := txReq(h)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("read-only status=%d body=%s", code, body)
		}
		if svc.calls.get != 0 || svc.calls.acquire != 0 {
			t.Fatalf("read-only must not touch service: %+v", *svc.calls)
		}
	}

	// 2. 非法 SQL（读语句）：不 GetDatabase、不 Acquire。
	{
		svc := newUserSvc()
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), true)
		code, _ := runSQL(t, h, http.MethodPost, "/databases/u/b", BatchRequest{Transactional: true, Statements: []SQLStatementRequest{{SQL: "SET max_threads=4"}}})
		if code == http.StatusOK {
			t.Fatalf("invalid batch SQL unexpectedly succeeded")
		}
		if svc.calls.get != 0 || svc.calls.acquire != 0 {
			t.Fatalf("invalid batch SQL must not GetDatabase/Acquire: %+v", *svc.calls)
		}
	}

	// 3. 系统库：拒绝且不 Acquire。
	{
		svc := newUserSvc()
		svc.db = catalog.Database{ID: "sys", ProjectID: "proj-1", Status: catalog.DatabaseReady, Kind: catalog.DatabaseKindSystem}
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), true)
		code, body := txReq(h)
		if code != http.StatusForbidden || !strings.Contains(body, "system") {
			t.Fatalf("system db status=%d body=%s", code, body)
		}
		if svc.calls.acquire != 0 {
			t.Fatalf("system db must not Acquire: acquire=%d", svc.calls.acquire)
		}
	}

	// 4. 成功：Acquire 一次（ReadWrite），lease Release 一次。
	{
		svc := newUserSvc()
		svc.calls = &serviceCallLog{}
		h := NewSQLHandler(svc, testSQLLimits(), true)
		code, body := txReq(h)
		if code != http.StatusOK || !strings.Contains(body, `"durability":"committed_local"`) {
			t.Fatalf("success status=%d body=%s", code, body)
		}
		if svc.calls.acquire != 1 || svc.acquiredMode != godatabase.ReadWrite {
			t.Fatalf("acquire=%d mode=%v want 1 ReadWrite", svc.calls.acquire, svc.acquiredMode)
		}
		if !svc.lease.released {
			t.Fatalf("lease must be released exactly once")
		}
	}
}
