package api

import (
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
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

type aggResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Total     *float64 `json:"total"`
	Truncated bool     `json:"truncated"`
	Scanned   int64    `json:"scanned"`
}

// aggFixture 起一个真实引擎的数据服务并灌入 5 条订单，返回发请求的闭包。
func aggFixture(t *testing.T, engine string, scan int) func(query string) (int, aggResult, string) {
	t.Helper()
	dir := t.TempDir()
	path, err := catalog.DatabasePathForProject("prj00001", engine)
	if err != nil {
		t.Fatal(err)
	}
	db := catalog.Database{ID: "db-1", ProjectID: "prj00001", DatabasePath: path, DataModel: catalog.DataModelCollection}
	svc := realDataService{db: db, factory: &filedb.Factory{DataRoot: dir, CacheDir: dir, Syncer: filedb.NewLocalSyncer()}}
	schema := &collection.Collection{Name: "orders", Fields: []collection.Field{
		{Name: "status", Type: "select", Options: collection.FieldOptions{Values: []string{"paid", "open", "void"}}}, {Name: "amount", Type: "number"},
		{Name: "paid", Type: "bool"}, {Name: "placed", Type: "date"},
		{Name: "note", Type: "text"}, {Name: "payload", Type: "json"},
		{Name: "secret", Type: "text", Hidden: true},
	}}
	h := NewDataHandler(svc, true).WithCollections(staticSchemas{schema}, 50000).WithAggregateLimits(0, scan)
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithPrincipal(c.Request().Context(), auth.Principal{APIKeyID: "test"})
			ctx = WithProject(ctx, ProjectContext{ID: "prj00001"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	base := "/databases/:databaseID/data/collections/:collection"
	e.POST(base+"/documents", h.CreateDocument)
	e.GET(base+"/aggregate", h.Aggregate)
	do := func(method, target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	for _, doc := range []string{
		`{"status":"paid","amount":10,"paid":true,"placed":"2026-11-16T08:00:00Z","note":"a","secret":"x"}`,
		`{"status":"paid","amount":30,"paid":true,"placed":"2026-11-16T20:00:00Z","note":"b"}`,
		`{"status":"open","amount":5,"paid":false,"placed":"2026-11-17T09:00:00Z","note":"c"}`,
		`{"status":"open","amount":15,"paid":false,"placed":"2026-11-17T10:00:00Z"}`,
		`{"status":"void","amount":0,"paid":false,"placed":"2026-11-18T10:00:00Z"}`,
	} {
		if rec := do(http.MethodPost, "/databases/db-1/data/collections/orders/documents", doc); rec.Code != http.StatusCreated {
			t.Fatalf("seed: %d %s", rec.Code, rec.Body.String())
		}
	}
	return func(query string) (int, aggResult, string) {
		rec := do(http.MethodGet, "/databases/db-1/data/collections/orders/aggregate?"+query, "")
		var out aggResult
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out, rec.Body.String()
	}
}

func rowMap(res aggResult, key string) map[string][]any {
	out := map[string][]any{}
	for _, row := range res.Rows {
		out[toStr(row[0])] = row
	}
	return out
}

func toStr(v any) string {
	if v == nil {
		return ""
	}
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func TestAggregateRealEngines(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			get := aggFixture(t, engine, 0)

			// 无维度：KPI。
			code, res, body := get("metrics=" + url.QueryEscape("count,sum:amount,avg:amount,min:amount,max:amount,distinct:status"))
			if code != 200 || len(res.Rows) != 1 {
				t.Fatalf("kpi: %d %s", code, body)
			}
			row := res.Rows[0]
			if num(row[0]) != 5 || num(row[1]) != 60 || num(row[2]) != 12 || num(row[3]) != 0 || num(row[4]) != 30 || num(row[5]) != 3 {
				t.Fatalf("kpi values: %v", row)
			}
			if res.Columns[0] != "count" || res.Columns[1] != "sum_amount" || res.Scanned != 5 {
				t.Fatalf("kpi columns: %v scanned=%d", res.Columns, res.Scanned)
			}

			// 分类维度 + 度量降序。
			code, res, body = get("group_by=status&metrics=count&sort=-count")
			if code != 200 || len(res.Rows) != 3 || toStr(res.Rows[0][0]) != "paid" && toStr(res.Rows[0][0]) != "open" {
				t.Fatalf("category: %d %s", code, body)
			}
			if res.Columns[0] != "x" || res.Columns[1] != "count" {
				t.Fatalf("category columns: %v", res.Columns)
			}

			// 时间桶：按天。
			code, res, body = get("group_by=created_at:day&metrics=count")
			if code != 200 || len(res.Rows) != 1 {
				// 5 条记录在同一天写入，created_at 全部落在今天。
				t.Fatalf("created_at bucket: %d %s", code, body)
			}
			code, res, body = get("group_by=placed:day&metrics=count,sum:amount")
			byDay := rowMap(res, "x")
			if code != 200 || len(byDay) != 3 || num(byDay["2026-11-16"][1]) != 2 || num(byDay["2026-11-16"][2]) != 40 || num(byDay["2026-11-17"][2]) != 20 {
				t.Fatalf("placed day: %d %s", code, body)
			}
			// 默认按 x 升序。
			if toStr(res.Rows[0][0]) != "2026-11-16" || toStr(res.Rows[2][0]) != "2026-11-18" {
				t.Fatalf("default order: %v", res.Rows)
			}
			code, res, _ = get("group_by=placed:month&metrics=count")
			if code != 200 || len(res.Rows) != 1 || toStr(res.Rows[0][0]) != "2026-11" || num(res.Rows[0][1]) != 5 {
				t.Fatalf("placed month: %d %v", code, res.Rows)
			}
			code, res, _ = get("group_by=placed:week&metrics=count")
			if code != 200 || len(res.Rows) != 1 || toStr(res.Rows[0][0]) != "2026-11-16" {
				t.Fatalf("placed week: %d %v", code, res.Rows)
			}

			// 次维度：堆积 / 多折线。
			code, res, body = get("group_by=placed:day&series=status&metrics=sum:amount")
			if code != 200 || len(res.Columns) != 3 || res.Columns[0] != "x" || res.Columns[1] != "series" || res.Columns[2] != "sum_amount" {
				t.Fatalf("series: %d %s", code, body)
			}
			if len(res.Rows) != 3 {
				t.Fatalf("series rows: %v", res.Rows)
			}
			// 每天只有一种状态：按天 + 状态分组后与按天分组等价，金额按状态落在对应行。
			bySeries := map[string]float64{}
			for _, r := range res.Rows {
				bySeries[toStr(r[1])] += num(r[2])
			}
			if bySeries["paid"] != 40 || bySeries["open"] != 20 || bySeries["void"] != 0 {
				t.Fatalf("series sums: %v", bySeries)
			}
			// 真正的多系列：按状态分组、按是否已付拆系列。
			code, res, body = get("group_by=status&series=paid&metrics=count")
			if code != 200 || len(res.Rows) != 3 {
				t.Fatalf("status x paid: %d %s", code, body)
			}

			// 数值分箱（步长走参数绑定）。
			code, res, body = get("group_by=amount:bin=10&metrics=count")
			bins := rowMap(res, "x")
			if code != 200 || num(bins["0"][1]) != 2 || num(bins["10"][1]) != 2 || num(bins["30"][1]) != 1 {
				t.Fatalf("bin: %d %s", code, body)
			}

			// filter：用内置 created_at 与已登记字段联动。
			code, res, body = get("metrics=count&filter=" + url.QueryEscape(`status="paid" && placed>="2026-11-16T10:00:00Z"`))
			if code != 200 || num(res.Rows[0][0]) != 1 {
				t.Fatalf("filter: %d %s", code, body)
			}
			code, res, body = get("metrics=count&filter=" + url.QueryEscape(`created_at>="2000-01-01T00:00:00Z"`))
			if code != 200 || num(res.Rows[0][0]) != 5 {
				t.Fatalf("builtin created_at filter: %d %s", code, body)
			}
			code, res, body = get("metrics=count&filter=" + url.QueryEscape(`created_at<"2000-01-01T00:00:00Z"`))
			if code != 200 || num(res.Rows[0][0]) != 0 {
				t.Fatalf("builtin created_at filter empty: %d %s", code, body)
			}

			// truncated：limit=2，3 个分组。
			code, res, _ = get("group_by=status&metrics=count&limit=2")
			if code != 200 || !res.Truncated || len(res.Rows) != 2 {
				t.Fatalf("truncated: %d %+v", code, res)
			}

			// with_total：截断分组仍给出同口径总量，前端才能算出被截掉的尾部（BI-F01）。
			code, res, body = get("group_by=status&metrics=count&limit=2&with_total=1")
			if code != 200 || len(res.Rows) != 2 || res.Total == nil || *res.Total != 5 {
				t.Fatalf("with_total count: %d %s", code, body)
			}
			code, res, body = get("group_by=status&metrics=sum:amount&limit=2&with_total=1")
			if code != 200 || res.Total == nil || *res.Total != 60 {
				t.Fatalf("with_total sum: %d %s", code, body)
			}
			// 总量跟 filter 同口径，不是整表总量。
			code, res, body = get("group_by=status&metrics=count&limit=2&with_total=1&filter=" + url.QueryEscape(`status="paid"`))
			if code != 200 || res.Total == nil || *res.Total != 2 {
				t.Fatalf("with_total respects filter: %d %s", code, body)
			}
			// avg / min / max / distinct 不可累加：不给总量，避免前端把平均值相加当占比分母。
			for _, metric := range []string{"avg:amount", "min:amount", "max:amount", "distinct:status"} {
				code, res, body = get("group_by=status&metrics=" + url.QueryEscape(metric) + "&with_total=1")
				if code != 200 || res.Total != nil {
					t.Fatalf("with_total must be omitted for %s: %d %s", metric, code, body)
				}
			}
			// 不请求就不多跑一次查询：响应里没有 total 键。
			code, res, body = get("group_by=status&metrics=count")
			if code != 200 || res.Total != nil || strings.Contains(body, `"total"`) {
				t.Fatalf("total must be opt-in: %d %s", code, body)
			}
		})
	}
}

func TestAggregateRejectsInvalid(t *testing.T) {
	get := aggFixture(t, catalog.EngineSQLite, 0)
	for name, query := range map[string]string{
		"no metrics":          "group_by=status",
		"unknown metric":      "metrics=median:amount",
		"sum on text":         "metrics=sum:status",
		"sum on bool":         "metrics=sum:paid",
		"avg on date":         "metrics=avg:placed",
		"count with field":    "metrics=count:amount",
		"field required":      "metrics=sum",
		"unknown field":       "metrics=sum:nope",
		"hidden field":        "metrics=distinct:secret",
		"json field":          "metrics=distinct:payload",
		"too many metrics":    "metrics=count,sum:amount,avg:amount,min:amount,max:amount,distinct:status,distinct:note",
		"duplicate metric":    "metrics=count,count",
		"bucket on text":      "metrics=count&group_by=status:day",
		"bad bucket unit":     "metrics=count&group_by=placed:minute",
		"bin on text":         "metrics=count&group_by=status:bin=10",
		"bin zero":            "metrics=count&group_by=amount:bin=0",
		"bin negative":        "metrics=count&group_by=amount:bin=-1",
		"bin nan":             "metrics=count&group_by=amount:bin=NaN",
		"bin injection":       "metrics=count&group_by=" + url.QueryEscape("amount:bin=1) UNION SELECT 1--"),
		"series without x":    "metrics=count&series=status",
		"series bucket":       "metrics=count&group_by=placed:day&series=placed:day",
		"bad field name":      "metrics=count&group_by=" + url.QueryEscape("status; DROP TABLE orders"),
		"sort not returned":   "metrics=count&group_by=status&sort=amount",
		"sort injection":      "metrics=count&group_by=status&sort=" + url.QueryEscape("count; DROP TABLE orders"),
		"limit zero":          "metrics=count&limit=0",
		"limit too big":       "metrics=count&limit=501",
		"limit nan":           "metrics=count&limit=abc",
		"bad mode":            "mode=rows&metrics=count",
		"filter semicolon":    "metrics=count&filter=" + url.QueryEscape(`status="x"; DROP TABLE orders`),
		"filter unknown":      "metrics=count&filter=" + url.QueryEscape(`nope="x"`),
		"points with metrics": "mode=points&x=amount&y=amount&metrics=count",
		"points with group":   "mode=points&x=amount&y=amount&group_by=status",
		"points no x":         "mode=points&y=amount",
		"points x not number": "mode=points&x=status&y=amount",
		"points y not number": "mode=points&x=amount&y=note",
		"points size text":    "mode=points&x=amount&y=amount&size=note",
		"points series date":  "mode=points&x=amount&y=amount&series=placed",
		"points limit big":    "mode=points&x=amount&y=amount&limit=2001",
	} {
		code, _, body := get(query)
		if code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d: %s", name, code, body)
		}
	}
	// 注入尝试之后数据仍在。
	code, res, _ := get("metrics=count")
	if code != 200 || num(res.Rows[0][0]) != 5 {
		t.Fatalf("data damaged after injection attempts: %d %v", code, res.Rows)
	}
}

func TestAggregateScanLimit(t *testing.T) {
	get := aggFixture(t, catalog.EngineSQLite, 3)
	code, _, body := get("metrics=count")
	if code != http.StatusBadRequest || !strings.Contains(body, "filter_scan_limit_exceeded") {
		t.Fatalf("scan limit: %d %s", code, body)
	}
	// 缩小范围后放行。
	code, res, body := get("metrics=count&filter=" + url.QueryEscape(`status="void"`))
	if code != 200 || num(res.Rows[0][0]) != 1 {
		t.Fatalf("narrowed: %d %s", code, body)
	}
}

func TestAggregatePointsRealEngines(t *testing.T) {
	for _, engine := range []string{catalog.EngineDuckDB, catalog.EngineSQLite} {
		t.Run(engine, func(t *testing.T) {
			get := aggFixture(t, engine, 0)
			code, res, body := get("mode=points&x=amount&y=amount&size=amount&series=status")
			if code != 200 || len(res.Rows) != 5 {
				t.Fatalf("points: %d %s", code, body)
			}
			want := []string{"x", "y", "size", "series"}
			for i, c := range want {
				if res.Columns[i] != c {
					t.Fatalf("columns: %v", res.Columns)
				}
			}
			// 固定顺序：重复请求结果一致。
			_, again, _ := get("mode=points&x=amount&y=amount&size=amount&series=status")
			a, _ := json.Marshal(res.Rows)
			b, _ := json.Marshal(again.Rows)
			if string(a) != string(b) {
				t.Fatalf("points order is not stable:\n%s\n%s", a, b)
			}
			code, res, _ = get("mode=points&x=amount&y=amount&limit=2")
			if code != 200 || !res.Truncated || len(res.Rows) != 2 {
				t.Fatalf("points truncated: %d %+v", code, res)
			}
			// 过滤联动。
			code, res, _ = get("mode=points&x=amount&y=amount&filter=" + url.QueryEscape(`status="paid"`))
			if code != 200 || len(res.Rows) != 2 {
				t.Fatalf("points filter: %d %+v", code, res)
			}
		})
	}
}

func TestFoldSeriesCapsGroups(t *testing.T) {
	rows := [][]any{}
	for i := 0; i < 20; i++ {
		// 第 i 组出现 i+1 次，次数不同保证排序结果确定。
		for j := 0; j <= i; j++ {
			rows = append(rows, []any{1.0, 2.0, "g" + string(rune('a'+i))})
		}
	}
	foldSeries(rows, 2)
	groups := map[string]bool{}
	for _, row := range rows {
		groups[row[2].(string)] = true
	}
	if len(groups) != aggregateSeriesCap || !groups["其他"] {
		t.Fatalf("groups=%d %v", len(groups), groups)
	}
	// 次数最多的组必须保留。
	if !groups["gt"] {
		t.Fatal("largest group folded away")
	}
	// 不超过上限时不改动。
	small := [][]any{{1.0, 2.0, "a"}, {1.0, 2.0, "b"}}
	foldSeries(small, 2)
	if small[0][2] != "a" || small[1][2] != "b" {
		t.Fatalf("small mutated: %v", small)
	}
}

// 漂移防护：cloudagent 提示词承诺的度量语法，服务端必须全部接受（planv6.7 §3.4）。
func TestAggregateMetricKindsAccepted(t *testing.T) {
	get := aggFixture(t, catalog.EngineSQLite, 0)
	for _, kind := range aggregateMetricKinds {
		metric := kind
		switch kind {
		case "count":
		case "min", "max":
			metric = kind + ":amount"
		case "distinct":
			metric = kind + ":status"
		default:
			metric = kind + ":amount"
		}
		if code, _, body := get("metrics=" + url.QueryEscape(metric)); code != 200 {
			t.Errorf("metric %q rejected: %d %s", metric, code, body)
		}
	}
	// min/max 也接受 date。
	if code, _, body := get("metrics=min:placed,max:placed"); code != 200 {
		t.Errorf("min/max on date rejected: %d %s", code, body)
	}
}

func TestAggregateUnregisteredSchemaDegrades(t *testing.T) {
	dir := t.TempDir()
	path, _ := catalog.DatabasePathForProject("prj00001", catalog.EngineSQLite)
	db := catalog.Database{ID: "db-1", ProjectID: "prj00001", DatabasePath: path, DataModel: catalog.DataModelCollection}
	svc := realDataService{db: db, factory: &filedb.Factory{DataRoot: dir, CacheDir: dir, Syncer: filedb.NewLocalSyncer()}}
	h := NewDataHandler(svc, true).WithCollections(staticSchemas{nil}, 50000)
	e := echo.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := WithPrincipal(c.Request().Context(), auth.Principal{APIKeyID: "test"})
			ctx = WithProject(ctx, ProjectContext{ID: "prj00001"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	base := "/databases/:databaseID/data/collections/:collection"
	e.POST(base+"/documents", h.CreateDocument)
	e.GET(base+"/aggregate", h.Aggregate)
	req := httptest.NewRequest(http.MethodPost, "/databases/db-1/data/collections/notes/documents", strings.NewReader(`{"a":1}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if rec := httptest.NewRecorder(); func() int { e.ServeHTTP(rec, req); return rec.Code }() != http.StatusCreated {
		t.Fatal("seed failed")
	}
	call := func(q string) (int, string) {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/databases/db-1/data/collections/notes/aggregate?"+q, nil))
		return rec.Code, rec.Body.String()
	}
	// 未登记 schema：count 与 created_at 时间桶放行。
	if code, body := call("metrics=count"); code != 200 {
		t.Fatalf("count: %d %s", code, body)
	}
	if code, body := call("metrics=count&group_by=created_at:day"); code != 200 {
		t.Fatalf("created_at bucket: %d %s", code, body)
	}
	// 其余字段返回 schema_not_registered。
	if code, body := call("metrics=sum:a"); code != 400 || !strings.Contains(body, "schema_not_registered") {
		t.Fatalf("sum without schema: %d %s", code, body)
	}
	// 集合表从未创建：返回空结果而不是 500。
	if code, body := call("metrics=count"); code != 200 {
		t.Fatalf("existing: %d %s", code, body)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/databases/db-1/data/collections/ghost/aggregate?metrics=count", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"rows":[]`) {
		t.Fatalf("missing relation: %d %s", rec.Code, rec.Body.String())
	}
}

// 聚合是纯读：即使实例只读（writable=false）也可用，且只申请 ReadOnly lease。
// SQL 形态库（系统库恒为 sql）与其他集合接口同口径，返回 data_model_mismatch。
func TestAggregateIsReadOnlyPath(t *testing.T) {
	newRouter := func(db catalog.Database) (*echo.Echo, *fakeDataService) {
		svc := &fakeDataService{
			dbs: []catalog.Database{db},
			lease: &fakeSQLLease{queryQueue: []godatabase.QueryResult{
				{Rows: [][]any{{int64(2)}}},
				{Columns: []string{"count"}, Rows: [][]any{{int64(2)}}},
			}},
		}
		e := setupDataTestRouter(t, svc, false)
		e.GET("/v1/projects/:projectID/databases/:databaseID/data/collections/:collection/aggregate", NewDataHandler(svc, false).Aggregate)
		return e, svc
	}
	call := func(e *echo.Echo) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/projects/proj-1/databases/db-1/data/collections/t/aggregate?metrics=count", nil))
		return rec
	}
	e, svc := newRouter(catalog.Database{ID: "db-1", ProjectID: "proj-1", DataModel: catalog.DataModelCollection})
	if rec := call(e); rec.Code != http.StatusOK || svc.acquiredMode != godatabase.ReadOnly {
		t.Fatalf("user db: status %d mode %v: %s", rec.Code, svc.acquiredMode, rec.Body.String())
	}
	e, _ = newRouter(catalog.Database{ID: "db-1", ProjectID: "proj-1", Kind: catalog.DatabaseKindSystem})
	if rec := call(e); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "data_model_mismatch") {
		t.Fatalf("system db: status %d: %s", rec.Code, rec.Body.String())
	}
}
