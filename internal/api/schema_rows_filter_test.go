// 文件说明：schema_rows_filter_test.go — internal/api 的单元测试（TestTableRowsFilterAppliesToCountAndPage、TestTableRowsWithoutFilterIsUnchanged 等）
//
// 用法
//
//	go test ./internal/api -run 'TestTableRowsFilterAppliesToCountAndPage|TestTableRowsWithoutFilterIsUnchanged|TestTableRowsFilterRejectsUnknownAndUnsafe|TestTableRowsFilterHidesSensitiveColumnsOnSystemDatabase'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/schema_rows_filter_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	rowsURL(table string, q url.Values) string
//	TestTableRowsFilterAppliesToCountAndPage(t *testing.T)
//	TestTableRowsWithoutFilterIsUnchanged(t *testing.T)
//	TestTableRowsFilterRejectsUnknownAndUnsafe(t *testing.T)
//	TestTableRowsFilterHidesSensitiveColumnsOnSystemDatabase(t *testing.T)
//	TestTableRowsFilterRealEngines(t *testing.T)

package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

func rowsURL(table string, q url.Values) string {
	return "/v1/projects/proj-1/databases/db-sql/schema/tables/" + table + "/rows?" + q.Encode()
}

func TestTableRowsFilterAppliesToCountAndPage(t *testing.T) {
	lease := &fakeSQLLease{queryQueue: []godatabase.QueryResult{
		{Rows: ordersColumns},
		{Rows: [][]any{{int64(1)}}},
		{Rows: [][]any{{int64(1), "paid", 30.0, true, "2026-11-16 20:00:00", nil}}},
	}}
	svc := &fakeDataService{dbs: []catalog.Database{{ID: "db-sql", ProjectID: "proj-1", Kind: catalog.DatabaseKindUser, DataModel: catalog.DataModelSQL, DatabasePath: "proj0001/database.duckdb"}}, lease: lease}
	e := setupSchemaRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, rowsURL("orders", url.Values{"filter": {`status="paid" && amount>=20`}, "limit": {"10"}}), nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var countStmt, pageStmt godatabase.Statement
	for _, stmt := range lease.queryStmts {
		switch {
		case strings.Contains(stmt.SQL, "COUNT(*)"):
			countStmt = stmt
		case strings.Contains(stmt.SQL, "LIMIT"):
			pageStmt = stmt
		}
	}
	for name, sqlText := range map[string]string{"count": countStmt.SQL, "page": pageStmt.SQL} {
		if !strings.Contains(sqlText, `WHERE ("status" = ? AND "amount" >= ?)`) {
			t.Errorf("%s SQL missing the bound filter:\n%s", name, sqlText)
		}
		// 值只能以占位符出现：不能有单引号字符串字面量，也不能有 `= 'paid'` / `>= 20` 这类内联比较。
		// （"paid" 带双引号是列名，会出现在 SELECT 投影里，不属于内联值。）
		if strings.Contains(sqlText, `'`) || strings.Contains(sqlText, `= paid`) || strings.Contains(sqlText, ">= 20") {
			t.Errorf("%s SQL must not inline filter values:\n%s", name, sqlText)
		}
	}
	// 参数顺序：先是 filter 的参数，分页的 limit / offset 追加在最后。
	if len(countStmt.Args) != 2 || countStmt.Args[0] != "paid" || countStmt.Args[1] != float64(20) {
		t.Errorf("count args = %#v", countStmt.Args)
	}
	if len(pageStmt.Args) != 4 || pageStmt.Args[0] != "paid" || pageStmt.Args[2] != 10 || pageStmt.Args[3] != 0 {
		t.Errorf("page args = %#v", pageStmt.Args)
	}
}

func TestTableRowsWithoutFilterIsUnchanged(t *testing.T) {
	lease := &fakeSQLLease{queryQueue: []godatabase.QueryResult{
		{Rows: ordersColumns}, {Rows: [][]any{{int64(5)}}}, {Rows: [][]any{}},
	}}
	svc := &fakeDataService{dbs: []catalog.Database{{ID: "db-sql", ProjectID: "proj-1", Kind: catalog.DatabaseKindUser, DataModel: catalog.DataModelSQL, DatabasePath: "proj0001/database.duckdb"}}, lease: lease}
	e := setupSchemaRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, rowsURL("orders", url.Values{}), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	for _, stmt := range lease.queryStmts {
		if strings.Contains(stmt.SQL, "WHERE") && !strings.Contains(stmt.SQL, "information_schema") {
			t.Errorf("no filter must mean no WHERE: %s", stmt.SQL)
		}
		if strings.Contains(stmt.SQL, "LIMIT") && len(stmt.Args) != 2 {
			t.Errorf("page args = %#v", stmt.Args)
		}
	}
}

func TestTableRowsFilterRejectsUnknownAndUnsafe(t *testing.T) {
	for name, filter := range map[string]string{
		"unknown column": `ghost=1`,
		"semicolon":      `status="a";`,
		"injection":      `status="x" || 1=1; DROP TABLE orders`,
		"bad operator":   `status=>"a"`,
	} {
		lease := &fakeSQLLease{queryQueue: []godatabase.QueryResult{{Rows: ordersColumns}}}
		svc := &fakeDataService{dbs: []catalog.Database{{ID: "db-sql", ProjectID: "proj-1", Kind: catalog.DatabaseKindUser, DataModel: catalog.DataModelSQL, DatabasePath: "proj0001/database.duckdb"}}, lease: lease}
		e := setupSchemaRouter(t, svc, true)
		rec := doRequest(e, http.MethodGet, rowsURL("orders", url.Values{"filter": {filter}}), nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d %s", name, rec.Code, rec.Body.String())
		}
		for _, stmt := range lease.queryStmts {
			if strings.Contains(stmt.SQL, "COUNT(*)") || strings.Contains(stmt.SQL, `FROM "orders"`) {
				t.Errorf("%s: a rejected filter must not reach the data table: %s", name, stmt.SQL)
			}
		}
	}
}

func TestTableRowsFilterHidesSensitiveColumnsOnSystemDatabase(t *testing.T) {
	cols := [][]any{
		{"sys_users", "id", "VARCHAR", "NO"},
		{"sys_users", "password_hash", "VARCHAR", "NO"},
		{"sys_users", "created_at", "TIMESTAMP", "YES"},
	}
	lease := &fakeSQLLease{queryQueue: []godatabase.QueryResult{{Rows: cols}}}
	svc := &fakeDataService{dbs: []catalog.Database{{ID: "db-sql", ProjectID: "proj-1", Kind: catalog.DatabaseKindSystem}}, lease: lease}
	e := setupSchemaRouter(t, svc, true)
	rec := doRequest(e, http.MethodGet, rowsURL("sys_users", url.Values{"filter": {`password_hash="x"`}}), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown_field") {
		t.Fatalf("a sensitive column must not be filterable (it would leak by probing): %d %s", rec.Code, rec.Body.String())
	}
}

func TestTableRowsFilterRealEngines(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			get := sqlRowsFixture(t, engine)
			code, body := get("orders", "filter="+url.QueryEscape(`status="paid"`)+"&limit=10")
			if code != 200 || !strings.Contains(body, `"total":2`) {
				t.Fatalf("filtered total: %d %s", code, body)
			}
			// amount>=15 命中 30 与 15 两行；limit=1 只返回一页，total 仍是过滤后的总数。
			code, body = get("orders", "filter="+url.QueryEscape(`amount>=15`)+"&limit=1&offset=0")
			if code != 200 || !strings.Contains(body, `"total":2`) || strings.Count(body, `"paid"`) < 1 {
				t.Fatalf("paged filter: %d %s", code, body)
			}
			code, body = get("orders", "limit=10")
			if code != 200 || !strings.Contains(body, `"total":5`) {
				t.Fatalf("no filter: %d %s", code, body)
			}
		})
	}
}
