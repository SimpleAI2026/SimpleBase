package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/filedb"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// sqlSeededService 在真实 DuckDB / SQLite 上建一张 SQL 表并灌入数据。
// 不同引擎的列类型名不同（DuckDB: VARCHAR / DOUBLE / TIMESTAMP；SQLite: TEXT / REAL / TIMESTAMP），
// 所以按引擎各写一份 DDL，才能验证 sqlColumnKind 对真实元数据的归类。
func sqlSeededService(t *testing.T, engine string) realDataService {
	t.Helper()
	dir := t.TempDir()
	path, err := catalog.DatabasePathForProject("prj00001", engine)
	if err != nil {
		t.Fatal(err)
	}
	db := catalog.Database{ID: "db-sql", ProjectID: "prj00001", DatabasePath: path, DataModel: catalog.DataModelSQL}
	svc := realDataService{db: db, factory: &filedb.Factory{DataRoot: dir, CacheDir: dir, Syncer: filedb.NewLocalSyncer()}}

	ddl := map[string][]string{
		catalog.EngineDuckDB: {
			`CREATE TABLE orders (id BIGINT, status VARCHAR, amount DOUBLE, paid BOOLEAN, placed_at TIMESTAMP, note VARCHAR)`,
		},
		catalog.EngineSQLite: {
			// SQLite 的 TIMESTAMP / BOOLEAN 是亲和性声明：列类型名原样出现在 pragma_table_info 里。
			`CREATE TABLE orders (id INTEGER, status TEXT, amount REAL, paid BOOLEAN, placed_at TIMESTAMP, note TEXT)`,
		},
	}[engine]
	rows := []string{
		`INSERT INTO orders VALUES (1, 'paid', 10, true, '2026-11-16 08:00:00', 'a')`,
		`INSERT INTO orders VALUES (2, 'paid', 30, true, '2026-11-16 20:00:00', 'b')`,
		`INSERT INTO orders VALUES (3, 'open', 5, false, '2026-11-17 09:00:00', 'c')`,
		`INSERT INTO orders VALUES (4, 'open', 15, false, '2026-11-17 10:00:00', NULL)`,
		`INSERT INTO orders VALUES (5, 'void', 0, false, '2026-11-18 10:00:00', NULL)`,
	}
	conn, err := svc.Acquire(context.Background(), db, godatabase.ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range append(ddl, rows...) {
		if _, err := conn.Execute(context.Background(), godatabase.Statement{SQL: stmt}); err != nil {
			conn.Release()
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	conn.Release()
	return svc
}

func sqlTestEcho(h *SchemaHandler) *echo.Echo {
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithPrincipal(c.Request().Context(), auth.Principal{APIKeyID: "test"})
			ctx = WithProject(ctx, ProjectContext{ID: "prj00001"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.GET("/databases/:databaseID/schema/tables/:table/aggregate", h.AggregateTable)
	e.GET("/databases/:databaseID/schema/tables/:table/rows", h.ListTableRows)
	return e
}

// sqlAggFixture 返回聚合请求闭包。
func sqlAggFixture(t *testing.T, engine string, scan int) func(table, query string) (int, aggResult, string) {
	t.Helper()
	svc := sqlSeededService(t, engine)
	e := sqlTestEcho(NewSchemaHandler(svc, true, testSQLLimits()).WithAggregateLimits(0, scan))
	return func(table, query string) (int, aggResult, string) {
		req := httptest.NewRequest(http.MethodGet, "/databases/db-sql/schema/tables/"+table+"/aggregate?"+query, strings.NewReader(""))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		var out aggResult
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out, rec.Body.String()
	}
}

// sqlRowsFixture 返回明细（分页行）请求闭包。
func sqlRowsFixture(t *testing.T, engine string) func(table, query string) (int, string) {
	t.Helper()
	svc := sqlSeededService(t, engine)
	e := sqlTestEcho(NewSchemaHandler(svc, true, testSQLLimits()))
	return func(table, query string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/databases/db-sql/schema/tables/"+table+"/rows?"+query, strings.NewReader(""))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
}

func TestSQLTableAggregateRealEngines(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			get := sqlAggFixture(t, engine, 0)

			// KPI：六种度量一次算出。
			code, res, body := get("orders", "metrics="+url.QueryEscape("count,sum:amount,avg:amount,min:amount,max:amount,distinct:status"))
			if code != 200 || len(res.Rows) != 1 {
				t.Fatalf("kpi: %d %s", code, body)
			}
			row := res.Rows[0]
			if num(row[0]) != 5 || num(row[1]) != 60 || num(row[2]) != 12 || num(row[3]) != 0 || num(row[4]) != 30 || num(row[5]) != 3 {
				t.Fatalf("kpi values: %v", row)
			}
			if res.Scanned != 5 {
				t.Fatalf("scanned = %d", res.Scanned)
			}

			// 分类维度。
			code, res, body = get("orders", "group_by=status&metrics=count&sort=-count")
			by := rowMap(res, "x")
			if code != 200 || len(by) != 3 || num(by["paid"][1]) != 2 || num(by["open"][1]) != 2 || num(by["void"][1]) != 1 {
				t.Fatalf("category: %d %s", code, body)
			}

			// 时间桶：真实 TIMESTAMP 列，按天 / 月 / 周。
			code, res, body = get("orders", "group_by=placed_at:day&metrics=count,sum:amount")
			byDay := rowMap(res, "x")
			if code != 200 || len(byDay) != 3 || num(byDay["2026-11-16"][1]) != 2 || num(byDay["2026-11-16"][2]) != 40 || num(byDay["2026-11-17"][2]) != 20 {
				t.Fatalf("day bucket: %d %s", code, body)
			}
			code, res, _ = get("orders", "group_by=placed_at:month&metrics=count")
			if code != 200 || len(res.Rows) != 1 || toStr(res.Rows[0][0]) != "2026-11" || num(res.Rows[0][1]) != 5 {
				t.Fatalf("month bucket: %d %v", code, res.Rows)
			}
			code, res, _ = get("orders", "group_by=placed_at:week&metrics=count")
			if code != 200 || len(res.Rows) != 1 || toStr(res.Rows[0][0]) != "2026-11-16" {
				t.Fatalf("week bucket: %d %v", code, res.Rows)
			}

			// 数值分箱（步长走参数绑定）。
			code, res, body = get("orders", "group_by=amount:bin=10&metrics=count")
			bins := rowMap(res, "x")
			if code != 200 || num(bins["0"][1]) != 2 || num(bins["10"][1]) != 2 || num(bins["30"][1]) != 1 {
				t.Fatalf("bin: %d %s", code, body)
			}

			// 次维度：状态 × 是否已付。
			code, res, body = get("orders", "group_by=status&series=paid&metrics=count")
			if code != 200 || len(res.Columns) != 3 || res.Columns[1] != "series" {
				t.Fatalf("series: %d %s", code, body)
			}

			// 过滤：真实列比较、时间范围、联动 chip 形态都走同一套解析。
			code, res, body = get("orders", "metrics=count&filter="+url.QueryEscape(`status="paid" && amount>=20`))
			if code != 200 || num(res.Rows[0][0]) != 1 {
				t.Fatalf("filter: %d %s", code, body)
			}
			code, res, body = get("orders", "metrics=count&filter="+url.QueryEscape(`placed_at>="2026-11-17T00:00:00Z" && placed_at<"2026-11-18T00:00:00Z"`))
			if code != 200 || num(res.Rows[0][0]) != 2 {
				t.Fatalf("time range filter: %d %s", code, body)
			}
			code, res, body = get("orders", "metrics=count&filter="+url.QueryEscape(`note is null`))
			if code != 200 || num(res.Rows[0][0]) != 2 {
				t.Fatalf("is null filter: %d %s", code, body)
			}
			code, res, body = get("orders", "metrics=count&filter="+url.QueryEscape(`status in ["paid","void"]`))
			if code != 200 || num(res.Rows[0][0]) != 3 {
				t.Fatalf("in filter: %d %s", code, body)
			}

			// with_total：SQL 表同样给出同口径总量（占比分母），且与 filter 一致。
			code, res, body = get("orders", "group_by=status&metrics=count&limit=2&with_total=1")
			if code != 200 || !res.Truncated || res.Total == nil || *res.Total != 5 {
				t.Fatalf("with_total: %d %s", code, body)
			}
			code, res, body = get("orders", "group_by=status&metrics=count&with_total=1&filter="+url.QueryEscape(`status="paid"`))
			if code != 200 || res.Total == nil || *res.Total != 2 {
				t.Fatalf("with_total filter: %d %s", code, body)
			}
		})
	}
}

func TestSQLTableAggregatePointsRealEngines(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			get := sqlAggFixture(t, engine, 0)
			code, res, body := get("orders", "mode=points&x=amount&y=id&size=amount&series=status")
			if code != 200 || len(res.Rows) != 5 {
				t.Fatalf("points: %d %s", code, body)
			}
			for i, c := range []string{"x", "y", "size", "series"} {
				if res.Columns[i] != c {
					t.Fatalf("columns: %v", res.Columns)
				}
			}
			// 排序稳定：重复请求一致；没有 created_at 的表按 id 升序。
			_, again, _ := get("orders", "mode=points&x=amount&y=id&size=amount&series=status")
			a, _ := json.Marshal(res.Rows)
			b, _ := json.Marshal(again.Rows)
			if string(a) != string(b) {
				t.Fatalf("points order is not stable:\n%s\n%s", a, b)
			}
			if num(res.Rows[0][1]) != 1 || num(res.Rows[4][1]) != 5 {
				t.Fatalf("expected id ascending, got %v", res.Rows)
			}
			code, res, _ = get("orders", "mode=points&x=amount&y=id&limit=2")
			if code != 200 || !res.Truncated || len(res.Rows) != 2 {
				t.Fatalf("truncated: %d %+v", code, res)
			}
		})
	}
}

func TestSQLTableAggregateScanLimitRealEngines(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			get := sqlAggFixture(t, engine, 3)
			code, _, body := get("orders", "metrics=count")
			if code != http.StatusBadRequest || !strings.Contains(body, "filter_scan_limit_exceeded") {
				t.Fatalf("scan limit: %d %s", code, body)
			}
			// 缩小范围后在上限内即可通过。
			code, res, body := get("orders", "metrics=count&filter="+url.QueryEscape(`status="paid"`))
			if code != 200 || num(res.Rows[0][0]) != 2 {
				t.Fatalf("narrowed: %d %s", code, body)
			}
		})
	}
}

func TestSQLTableAggregateRealEnginesRejectsUnsafeNames(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			get := sqlAggFixture(t, engine, 0)
			for name, q := range map[string]string{
				"group_by injection":  "metrics=count&group_by=" + url.QueryEscape(`status"; DROP TABLE orders; --`),
				"metric injection":    "metrics=" + url.QueryEscape(`sum:amount) FROM orders; DROP TABLE orders; --`),
				"filter injection":    "metrics=count&filter=" + url.QueryEscape(`status="x" || 1=1; DROP TABLE orders`),
				"sort outside result": "metrics=count&group_by=status&sort=" + url.QueryEscape(`secret`),
			} {
				code, _, body := get("orders", q)
				if code != http.StatusBadRequest {
					t.Fatalf("%s: got %d %s", name, code, body)
				}
			}
			// 表还在：注入没有生效。
			code, res, body := get("orders", "metrics=count")
			if code != 200 || num(res.Rows[0][0]) != 5 {
				t.Fatalf("table must be intact: %d %s", code, body)
			}
			// 表名本身也不可注入。
			code, _, _ = get(url.PathEscape(`orders"; DROP TABLE orders; --`), "metrics=count")
			if code != http.StatusBadRequest && code != http.StatusNotFound {
				t.Fatalf("bad table name must be rejected, got %d", code)
			}
		})
	}
}
