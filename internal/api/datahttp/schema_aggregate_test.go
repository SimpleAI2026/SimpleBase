// 文件说明：schema_aggregate_test.go — internal/api 的单元测试（TestSQLColumnKind、TestAggregateTableGroupsRealColumns 等）
//
// 用法
//
//	go test ./internal/api -run 'TestSQLColumnKind|TestAggregateTableGroupsRealColumns|TestAggregateTableTimeBucketAndBinUseColumnTypes|TestAggregateTableRejectsBadInput'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/schema_aggregate_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	setupSchemaAggregateRouter(t *testing.T, svc *fakeDataService) *echo.Echo
//	var ordersColumns
//	sqlAggregateFixture(columns [][]any, results ...godatabase.QueryResult) (*fakeDataService, *fakeSQLLease)
//	aggregateURL(table string, q url.Values) string
//	TestSQLColumnKind(t *testing.T)
//	TestAggregateTableGroupsRealColumns(t *testing.T)
//	TestAggregateTableTimeBucketAndBinUseColumnTypes(t *testing.T)
//	TestAggregateTableRejectsBadInput(t *testing.T)
//	... 另有 9 个顶层符号

package datahttp

import (
	"github.com/linkxzhou/SimpleBase/internal/collection/query"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
)

func setupSchemaAggregateRouter(t *testing.T, svc *fakeDataService) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	h := NewSchemaHandler(svc, true, testSQLLimits())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Principal{APIKeyID: "key-test", TenantID: "tenant-1", ProjectIDs: map[string]struct{}{"proj-1": {}}}
			ctx := httpkit.WithPrincipal(c.Request().Context(), p)
			ctx = httpkit.WithProject(ctx, httpkit.ProjectContext{ID: "proj-1", TenantID: "tenant-1"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.GET("/v1/projects/:projectID/databases/:databaseID/schema/tables/:table/aggregate", h.AggregateTable)
	return e
}

var ordersColumns = [][]any{
	{"orders", "id", "BIGINT", "NO"},
	{"orders", "status", "VARCHAR", "YES"},
	{"orders", "amount", "DECIMAL(10,2)", "YES"},
	{"orders", "paid", "BOOLEAN", "YES"},
	{"orders", "placed_at", "TIMESTAMP", "YES"},
	{"orders", "payload", "JSON", "YES"},
}

func sqlAggregateFixture(columns [][]any, results ...godatabase.QueryResult) (*fakeDataService, *fakeSQLLease) {
	queue := []godatabase.QueryResult{{Rows: columns}}
	queue = append(queue, results...)
	lease := &fakeSQLLease{queryQueue: queue}
	svc := &fakeDataService{
		dbs:   []catalog.Database{{ID: "db-sql", ProjectID: "proj-1", Kind: catalog.DatabaseKindUser, DataModel: catalog.DataModelSQL, DatabasePath: "proj0001/database.duckdb"}},
		lease: lease,
	}
	return svc, lease
}

func aggregateURL(table string, q url.Values) string {
	return "/v1/projects/proj-1/databases/db-sql/schema/tables/" + table + "/aggregate?" + q.Encode()
}

func TestSQLColumnKind(t *testing.T) {
	cases := map[string]string{
		"BIGINT": "number", "INTEGER": "number", "int": "number", "DOUBLE": "number", "DECIMAL(10,2)": "number", "REAL": "number", "NUMERIC(8, 3)": "number",
		"BOOLEAN": "bool", "bool": "bool",
		"TIMESTAMP": "date", "TIMESTAMP WITH TIME ZONE": "date", "DATETIME": "date", "DATE": "date",
		"VARCHAR": "text", "VARCHAR(20)": "text", "TEXT": "text", "UUID": "text", "": "text", "SOMETHING_NEW": "text",
		"JSON": "json", "BLOB": "json", "STRUCT(a INTEGER)": "json", "INTEGER[]": "text",
	}
	for in, want := range cases {
		if got := query.SQLColumnKind(in); got != want {
			t.Errorf("query.SQLColumnKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAggregateTableGroupsRealColumns(t *testing.T) {
	svc, lease := sqlAggregateFixture(ordersColumns,
		godatabase.QueryResult{Rows: [][]any{{int64(10)}}},
		godatabase.QueryResult{Rows: [][]any{{"paid", int64(6), 120.5}, {"open", int64(4), 80.0}}},
	)
	e := setupSchemaAggregateRouter(t, svc)
	rec := doRequest(e, http.MethodGet, aggregateURL("orders", url.Values{"metrics": {"count,sum:amount"}, "group_by": {"status"}, "sort": {"-count"}}), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"columns":["x","count","sum_amount"]`, `"scanned":10`, `"truncated":false`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if svc.acquiredMode != godatabase.ReadOnly {
		t.Fatalf("aggregate must use a read-only lease, got %v", svc.acquiredMode)
	}
	var agg string
	for _, stmt := range lease.queryStmts {
		if strings.Contains(stmt.SQL, "GROUP BY") {
			agg = stmt.SQL
		}
	}
	// 真实列：引用成 "status"，不出现 JSON 提取；表名同样被引用。
	for _, want := range []string{`"status" AS x`, `SUM("amount") AS sum_amount`, `FROM "orders"`, `GROUP BY x`, `ORDER BY count DESC`} {
		if !strings.Contains(agg, want) {
			t.Errorf("sql missing %s:\n%s", want, agg)
		}
	}
	if strings.Contains(agg, "json_extract") || strings.Contains(strings.ToLower(agg), "$.") {
		t.Errorf("SQL table aggregate must not use JSON paths:\n%s", agg)
	}
}

func TestAggregateTableTimeBucketAndBinUseColumnTypes(t *testing.T) {
	// 每次请求先读一次列元数据，所以两次请求各排：列 → count → 结果。
	svc, lease := sqlAggregateFixture(ordersColumns,
		godatabase.QueryResult{Rows: [][]any{{int64(3)}}},
		godatabase.QueryResult{Rows: [][]any{{"2026-11-16", int64(3)}}},
		godatabase.QueryResult{Rows: ordersColumns},
		godatabase.QueryResult{Rows: [][]any{{int64(3)}}},
		godatabase.QueryResult{Rows: [][]any{{float64(0), int64(3)}}},
	)
	e := setupSchemaAggregateRouter(t, svc)
	rec := doRequest(e, http.MethodGet, aggregateURL("orders", url.Values{"metrics": {"count"}, "group_by": {"placed_at:day"}}), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bucket %d %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, aggregateURL("orders", url.Values{"metrics": {"count"}, "group_by": {"amount:bin=50"}}), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bin %d %s", rec.Code, rec.Body.String())
	}
	var bucket, bin string
	for _, stmt := range lease.queryStmts {
		switch {
		case strings.Contains(stmt.SQL, `"placed_at"`) && strings.Contains(stmt.SQL, "GROUP BY"):
			bucket = stmt.SQL
		case strings.Contains(stmt.SQL, `"amount"`) && strings.Contains(stmt.SQL, "GROUP BY"):
			bin = stmt.SQL
			if len(stmt.Args) == 0 || stmt.Args[0] != float64(50) {
				t.Errorf("bin step must be bound as an argument: %#v", stmt.Args)
			}
		}
	}
	if !strings.Contains(bucket, "TIMESTAMP") || !strings.Contains(bucket, `"placed_at"`) {
		t.Errorf("date column must be cast as a timestamp: %s", bucket)
	}
	if strings.Contains(bin, "50") {
		t.Errorf("bin step must not be inlined: %s", bin)
	}
}

func TestAggregateTableRejectsBadInput(t *testing.T) {
	cases := []struct {
		name  string
		table string
		q     url.Values
		code  int
		err   string
	}{
		{"unknown column", "orders", url.Values{"metrics": {"count"}, "group_by": {"nope"}}, 400, "unknown_field"},
		{"json column", "orders", url.Values{"metrics": {"count"}, "group_by": {"payload"}}, 400, "invalid_request"},
		{"sum on text", "orders", url.Values{"metrics": {"sum:status"}}, 400, "invalid_request"},
		{"bucket on non date", "orders", url.Values{"metrics": {"count"}, "group_by": {"status:day"}}, 400, "invalid_request"},
		{"bin on non number", "orders", url.Values{"metrics": {"count"}, "group_by": {"status:bin=10"}}, 400, "invalid_request"},
		{"injection via column name", "orders", url.Values{"metrics": {"count"}, "group_by": {`status"; DROP TABLE orders; --`}}, 400, "invalid_request"},
		{"injection via metric field", "orders", url.Values{"metrics": {`sum:amount) FROM users --`}}, 400, "invalid_request"},
		{"filter on unknown column", "orders", url.Values{"metrics": {"count"}, "filter": {`ghost=1`}}, 400, "unknown_field"},
		{"filter with semicolon", "orders", url.Values{"metrics": {"count"}, "filter": {`status="a";`}}, 400, "invalid_filter"},
		{"bad table name", "1bad", url.Values{"metrics": {"count"}}, 400, "invalid_request"},
		{"internal table", "__sb_meta", url.Values{"metrics": {"count"}}, 404, "table_not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, lease := sqlAggregateFixture(ordersColumns)
			e := setupSchemaAggregateRouter(t, svc)
			rec := doRequest(e, http.MethodGet, aggregateURL(tc.table, tc.q), nil)
			if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.err) {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body.String(), tc.code, tc.err)
			}
			// 校验失败必须发生在任何数据查询之前：只允许读过一次列元数据。
			for _, stmt := range lease.queryStmts {
				if strings.Contains(stmt.SQL, "COUNT(*)") || strings.Contains(stmt.SQL, "GROUP BY") {
					t.Fatalf("a rejected request must not reach the data tables: %s", stmt.SQL)
				}
			}
		})
	}
}

func TestAggregateTableMissingTable(t *testing.T) {
	svc, _ := sqlAggregateFixture(nil)
	e := setupSchemaAggregateRouter(t, svc)
	rec := doRequest(e, http.MethodGet, aggregateURL("ghost", url.Values{"metrics": {"count"}}), nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "table_not_found") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAggregateTableRejectsCollectionDatabases(t *testing.T) {
	lease := &fakeSQLLease{}
	svc := &fakeDataService{dbs: []catalog.Database{{ID: "db-sql", ProjectID: "proj-1", Kind: catalog.DatabaseKindUser, DataModel: catalog.DataModelCollection}}, lease: lease}
	e := setupSchemaAggregateRouter(t, svc)
	rec := doRequest(e, http.MethodGet, aggregateURL("orders", url.Values{"metrics": {"count"}}), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "data_model_mismatch") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAggregateTableScanLimit(t *testing.T) {
	svc, lease := sqlAggregateFixture(ordersColumns, godatabase.QueryResult{Rows: [][]any{{int64(defaultAggregateScan + 1)}}})
	e := setupSchemaAggregateRouter(t, svc)
	rec := doRequest(e, http.MethodGet, aggregateURL("orders", url.Values{"metrics": {"count"}}), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "filter_scan_limit_exceeded") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	for _, stmt := range lease.queryStmts {
		if strings.Contains(stmt.SQL, "SELECT COUNT(*) AS count") {
			t.Fatalf("over the scan limit must not run the aggregate: %s", stmt.SQL)
		}
	}
}

func TestAggregateTableTruncatedFlag(t *testing.T) {
	rows := [][]any{{"a", int64(1)}, {"b", int64(1)}, {"c", int64(1)}}
	svc, _ := sqlAggregateFixture(ordersColumns, godatabase.QueryResult{Rows: [][]any{{int64(3)}}}, godatabase.QueryResult{Rows: rows})
	e := setupSchemaAggregateRouter(t, svc)
	rec := doRequest(e, http.MethodGet, aggregateURL("orders", url.Values{"metrics": {"count"}, "group_by": {"status"}, "limit": {"2"}}), nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"truncated":true`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAggregateTableFilterCompilesAgainstRealColumns(t *testing.T) {
	svc, lease := sqlAggregateFixture(ordersColumns,
		godatabase.QueryResult{Rows: [][]any{{int64(2)}}},
		godatabase.QueryResult{Rows: [][]any{{int64(2)}}},
	)
	e := setupSchemaAggregateRouter(t, svc)
	rec := doRequest(e, http.MethodGet, aggregateURL("orders", url.Values{
		"metrics": {"count"}, "filter": {`status="paid" && amount>=100 && placed_at>="2026-11-01T00:00:00Z"`},
	}), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var countStmt string
	var args []any
	for _, stmt := range lease.queryStmts {
		if strings.Contains(stmt.SQL, "COUNT(*)") && strings.Contains(stmt.SQL, "WHERE") {
			countStmt, args = stmt.SQL, stmt.Args
			break
		}
	}
	for _, want := range []string{`"status" = ?`, `"amount" >= ?`, `"placed_at"`} {
		if !strings.Contains(countStmt, want) {
			t.Errorf("filter SQL missing %s:\n%s", want, countStmt)
		}
	}
	if strings.Contains(countStmt, "paid") || strings.Contains(countStmt, "100") {
		t.Errorf("filter values must be bound, not inlined:\n%s", countStmt)
	}
	if len(args) != 3 {
		t.Errorf("args = %#v", args)
	}
}

func TestAggregateTablePointsOrderAndNullFilter(t *testing.T) {
	// 时间列叫 placed_at，不在候选名单（occurred_at / created_at / updated_at / started_at）里，
	// 所以稳定排序回落到主键。带 created_at 的表见 TestSQLTableOrderFallbacks。
	svc, lease := sqlAggregateFixture(ordersColumns,
		godatabase.QueryResult{Rows: [][]any{{int64(2)}}},
		godatabase.QueryResult{Rows: [][]any{{1.0, 2.0}}},
		godatabase.QueryResult{Rows: ordersColumns},
	)
	e := setupSchemaAggregateRouter(t, svc)
	rec := doRequest(e, http.MethodGet, aggregateURL("orders", url.Values{"mode": {"points"}, "x": {"amount"}, "y": {"id"}}), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var points string
	for _, stmt := range lease.queryStmts {
		if strings.Contains(stmt.SQL, "AS x") {
			points = stmt.SQL
		}
	}
	for _, want := range []string{`"amount" AS x`, `"id" AS y`, `"amount" IS NOT NULL`, `ORDER BY "id" ASC`} {
		if !strings.Contains(points, want) {
			t.Errorf("points SQL missing %s:\n%s", want, points)
		}
	}
	rec = doRequest(e, http.MethodGet, aggregateURL("orders", url.Values{"mode": {"points"}, "x": {"status"}, "y": {"id"}}), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("x must be a number column: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAggregateTableSystemDatabaseHidesSensitiveColumns(t *testing.T) {
	cols := [][]any{
		{"sys_users", "id", "VARCHAR", "NO"},
		{"sys_users", "password_hash", "VARCHAR", "NO"},
		{"sys_users", "created_at", "TIMESTAMP", "YES"},
	}
	lease := &fakeSQLLease{queryQueue: []godatabase.QueryResult{{Rows: cols}, {Rows: cols}, {Rows: cols}}}
	svc := &fakeDataService{dbs: []catalog.Database{{ID: "db-sql", ProjectID: "proj-1", Kind: catalog.DatabaseKindSystem, DataModel: catalog.DataModelSQL}}, lease: lease}
	e := setupSchemaAggregateRouter(t, svc)
	for _, q := range []url.Values{
		{"metrics": {"count"}, "group_by": {"password_hash"}},
		{"metrics": {"distinct:password_hash"}},
		{"metrics": {"count"}, "filter": {`password_hash="x"`}},
	} {
		rec := doRequest(e, http.MethodGet, aggregateURL("sys_users", q), nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown_field") {
			t.Fatalf("sensitive column must look nonexistent, got %d %s for %v", rec.Code, rec.Body.String(), q)
		}
	}
	for _, stmt := range lease.queryStmts {
		if strings.Contains(stmt.SQL, "password_hash") && strings.Contains(stmt.SQL, "GROUP BY") {
			t.Fatalf("sensitive column reached SQL: %s", stmt.SQL)
		}
	}
}

func TestSQLTableOrderFallbacks(t *testing.T) {
	cols := func(names ...string) ([]query.SQLTableColumn, map[string]query.SQLTableColumn) {
		out := make([]query.SQLTableColumn, 0, len(names))
		by := map[string]query.SQLTableColumn{}
		for _, n := range names {
			c := query.SQLTableColumn{Name: n, Kind: "text"}
			out = append(out, c)
			by[n] = c
		}
		return out, by
	}
	for _, tc := range []struct {
		names []string
		want  string
	}{
		{[]string{"id", "created_at"}, ` ORDER BY "created_at" DESC, "id" ASC`},
		{[]string{"id", "name"}, ` ORDER BY "id" ASC`},
		{[]string{"name", "city"}, ` ORDER BY "name" ASC`},
		{[]string{"occurred_at", "created_at"}, ` ORDER BY "occurred_at" DESC`},
		{nil, ``},
	} {
		c, by := cols(tc.names...)
		if got := query.SQLTableOrder(c, by); got != tc.want {
			t.Errorf("query.SQLTableOrder(%v) = %q, want %q", tc.names, got, tc.want)
		}
	}
}

func TestSQLTableEnvSkipsInvalidAndSensitiveColumns(t *testing.T) {
	dialect, err := godatabase.DialectFor(catalog.EngineDuckDB)
	if err != nil {
		t.Fatal(err)
	}
	env := query.SQLTableEnv(dialect, []query.SQLTableColumn{
		{Name: "ok_col", Kind: "number"},
		{Name: "bad-name", Kind: "number"},
		{Name: "1starts_with_digit", Kind: "number"},
		{Name: "secret", Kind: "text"},
	}, func(n string) bool { return n == "secret" })
	if _, err := env.Lookup("ok_col"); err != nil {
		t.Fatalf("ok_col: %v", err)
	}
	for _, name := range []string{"bad-name", "1starts_with_digit", "secret", "missing", ""} {
		if _, err := env.Lookup(name); err == nil {
			t.Errorf("%q must not resolve", name)
		}
	}
}
