// 文件说明：collection_aggregate_query.go — 集合聚合接口的参数解析与 SQL 拼装
//
// 设计约束（planv6.7 §3.2）：
//   - 所有标识符来自 schema 登记字段，且再过一遍 fieldNamePattern；
//   - 表达式只由 Dialect.JSONExtract* / CastTimestamp / DateBucket 与固定聚合模板拼出；
//   - 数值分箱步长走参数绑定；不接受任何用户 SQL 片段。

package api

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/collection"
	"github.com/linkxzhou/SimpleBase/internal/utils/strutil"
)

const (
	aggregateDefaultLimit = 100
	aggregateMaxLimit     = 500
	aggregatePointsLimit  = 1000
	aggregatePointsMax    = 2000
	aggregateMaxMetrics   = 6
	// aggregateSeriesCap 是散点图分组数上限，超出的归入「其他」。
	aggregateSeriesCap = 12
)

// aggregateMetricKinds 是聚合度量种类全集。导出给 cloudagent 提示词做漂移防护测试。
var aggregateMetricKinds = []string{"count", "sum", "avg", "min", "max", "distinct"}

// aggregateQuery 是已编译的聚合请求。
type aggregateQuery struct {
	Points    bool
	Limit     int
	SQL       string
	Args      []any
	CountSQL  string
	CountArgs []any
	// Columns 是响应列名（x / series / 度量别名；散点为 x / y / size / series）。
	Columns []string
	// needsSeriesFold 表示散点带分组列，应用层要把分组数限制在 aggregateSeriesCap。
	needsSeriesFold bool
	// TotalSQL 非空时表示：同一 filter 下、不分组的第一个可累加度量总量（planv6.7 BI-F01）。
	// 只在调用方显式要 with_total、且有分组 + 单度量 + count/sum 时生成；avg/min/max 不可累加，不提供。
	TotalSQL  string
	TotalArgs []any
}

type aggregateMetric struct {
	kind  string
	field string
	alias string
	expr  string
}

type aggregateField struct {
	field collection.Field
	expr  string
}

// parseAggregateQuery 把 query string 编译成 SQL。source 来自 collectionSource。
func parseAggregateQuery(values url.Values, source string, schema *collection.Collection, dialect godatabase.Dialect) (aggregateQuery, error) {
	return parseAggregateEnv(values, source, collectionEnv(schema, dialect))
}

// parseAggregateEnv 是聚合解析的共用入口：集合与 SQL 表只在 env（字段解析、过滤、散点排序）上不同。
func parseAggregateEnv(values url.Values, source string, env aggregateEnv) (aggregateQuery, error) {
	mode := strings.TrimSpace(values.Get("mode"))
	if mode != "" && mode != "points" {
		return aggregateQuery{}, invalidQuery("mode must be points")
	}
	limit, err := parseAggregateLimit(values.Get("limit"), mode == "points")
	if err != nil {
		return aggregateQuery{}, err
	}
	filterSQL, filterArgs, err := env.filter(values.Get("filter"))
	if err != nil {
		return aggregateQuery{}, err
	}
	where := ""
	if filterSQL != "" {
		where = " WHERE " + filterSQL
	}
	q := aggregateQuery{
		Limit:     limit,
		CountSQL:  "SELECT COUNT(*) FROM " + source + where,
		CountArgs: append([]any(nil), filterArgs...),
	}
	if mode == "points" {
		for _, key := range []string{"metrics", "group_by"} {
			if strings.TrimSpace(values.Get(key)) != "" {
				return aggregateQuery{}, invalidQuery("points mode cannot be combined with " + key)
			}
		}
		return compilePoints(q, values, source, where, filterArgs, env)
	}
	return compileAggregate(q, values, source, where, filterArgs, env)
}

func parseAggregateLimit(raw string, points bool) (int, error) {
	def, max := aggregateDefaultLimit, aggregateMaxLimit
	if points {
		def, max = aggregatePointsLimit, aggregatePointsMax
	}
	if strings.TrimSpace(raw) == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > max {
		return 0, invalidQuery(fmt.Sprintf("limit must be between 1 and %d", max))
	}
	return n, nil
}

// lookupAggregateField 取字段表达式。created_at / updated_at / id 是内置列，
// 其余必须是已登记字段；json 字段一律拒绝做维度或度量。
func lookupAggregateField(name string, schema *collection.Collection, dialect godatabase.Dialect) (aggregateField, error) {
	name = strings.TrimSpace(name)
	if !fieldNamePattern.MatchString(name) {
		return aggregateField{}, invalidQuery("invalid field name")
	}
	switch name {
	case "id":
		return aggregateField{field: collection.Field{Name: name, Type: "text"}, expr: "id"}, nil
	case "created_at":
		return aggregateField{field: collection.Field{Name: name, Type: "date"}, expr: dialect.CastTimestamp("created_at")}, nil
	case "updated_at":
		return aggregateField{field: collection.Field{Name: name, Type: "date"}, expr: dialect.CastTimestamp("COALESCE(updated_at, created_at)")}, nil
	}
	if schema == nil {
		return aggregateField{}, &collection.ValidationError{Code: "schema_not_registered", Field: name, Message: "schema required for aggregate"}
	}
	for _, f := range schema.Fields {
		if f.Name != name {
			continue
		}
		if f.Hidden {
			break
		}
		path := "$." + name
		switch f.Type {
		case "number":
			return aggregateField{field: f, expr: dialect.JSONExtractNumber(path)}, nil
		case "bool":
			return aggregateField{field: f, expr: dialect.JSONExtractBool(path)}, nil
		case "date":
			return aggregateField{field: f, expr: dialect.CastTimestamp(dialect.JSONExtractText(path))}, nil
		case "json":
			return aggregateField{}, invalidQuery("json fields cannot be aggregated")
		default:
			return aggregateField{field: f, expr: dialect.JSONExtractText(path)}, nil
		}
	}
	return aggregateField{}, &collection.ValidationError{Code: "unknown_field", Field: name, Message: "field is not registered"}
}

func parseAggregateMetrics(raw string, env aggregateEnv) ([]aggregateMetric, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, invalidQuery("metrics is required")
	}
	parts := strings.Split(raw, ",")
	if len(parts) > aggregateMaxMetrics {
		return nil, invalidQuery(fmt.Sprintf("at most %d metrics", aggregateMaxMetrics))
	}
	seen := map[string]bool{}
	out := make([]aggregateMetric, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kind, name, hasField := strings.Cut(part, ":")
		m := aggregateMetric{kind: kind, field: name}
		switch kind {
		case "count":
			if hasField {
				return nil, invalidQuery("count takes no field")
			}
			m.alias, m.expr = "count", "COUNT(*)"
		case "sum", "avg", "min", "max", "distinct":
			if !hasField {
				return nil, invalidQuery(kind + " requires a field")
			}
			f, err := env.lookup(name)
			if err != nil {
				return nil, err
			}
			if err := checkMetricType(kind, f.field); err != nil {
				return nil, err
			}
			m.alias = kind + "_" + name
			if kind == "distinct" {
				m.expr = "COUNT(DISTINCT " + f.expr + ")"
			} else {
				m.expr = strings.ToUpper(kind) + "(" + f.expr + ")"
			}
		default:
			return nil, invalidQuery("unknown metric")
		}
		if seen[m.alias] {
			return nil, invalidQuery("duplicate metric")
		}
		seen[m.alias] = true
		out = append(out, m)
	}
	return out, nil
}

// checkMetricType：sum/avg 只接受 number；min/max 接受 number/date；distinct 接受任意非 json。
func checkMetricType(kind string, f collection.Field) error {
	switch kind {
	case "sum", "avg":
		if f.Type != "number" {
			return invalidQuery(kind + " requires a number field")
		}
	case "min", "max":
		if f.Type != "number" && f.Type != "date" {
			return invalidQuery(kind + " requires a number or date field")
		}
	}
	return nil
}

// parseGroupBy 解析 field / field:<unit> / field:bin=<step>。
func parseGroupBy(raw string, env aggregateEnv) (expr string, args []any, err error) {
	name, mod, hasMod := strings.Cut(strings.TrimSpace(raw), ":")
	f, err := env.lookup(name)
	if err != nil {
		return "", nil, err
	}
	if !hasMod {
		return f.expr, nil, nil
	}
	if strings.HasPrefix(mod, "bin=") {
		if f.field.Type != "number" {
			return "", nil, invalidQuery("bin requires a number field")
		}
		step, perr := strconv.ParseFloat(strings.TrimPrefix(mod, "bin="), 64)
		if perr != nil || step <= 0 || math.IsInf(step, 0) || math.IsNaN(step) {
			return "", nil, invalidQuery("bin must be a positive number")
		}
		// 步长走参数绑定，不进 SQL 文本。Floor 的输入表达式在 SQLite 方言里会重复出现，
		// 占位符个数随之变化，所以先生成表达式，再按其中 '?' 的个数补齐参数（前 N-1 个是 value/step
		// 的步长，最后一个是乘回去的步长，全部是同一个 step）。
		expr := env.dialect.Floor(f.expr+" / ?") + " * ?"
		args = make([]any, strings.Count(expr, "?"))
		for i := range args {
			args[i] = step
		}
		return expr, args, nil
	}
	if f.field.Type != "date" {
		return "", nil, invalidQuery("time bucket requires a date field")
	}
	bucket, ok := env.dialect.DateBucket(f.expr, mod)
	if !ok {
		return "", nil, invalidQuery("unsupported time bucket")
	}
	return bucket, nil, nil
}

func compileAggregate(q aggregateQuery, values url.Values, source, where string, filterArgs []any, env aggregateEnv) (aggregateQuery, error) {
	metrics, err := parseAggregateMetrics(values.Get("metrics"), env)
	if err != nil {
		return q, err
	}
	var selects, groups []string
	var selectArgs []any
	columns := []string{}
	// 维度只在 SELECT 里写一次表达式，GROUP BY 与 ORDER BY 引用别名（x / series），
	// 这样分箱步长的占位符不会重复出现；参数顺序固定为 SELECT → WHERE。
	grouped := false
	if raw := strings.TrimSpace(values.Get("group_by")); raw != "" {
		expr, args, err := parseGroupBy(raw, env)
		if err != nil {
			return q, err
		}
		selects = append(selects, expr+" AS x")
		selectArgs = append(selectArgs, args...)
		groups = append(groups, "x")
		columns = append(columns, "x")
		grouped = true
	}
	if raw := strings.TrimSpace(values.Get("series")); raw != "" {
		if strings.Contains(raw, ":") {
			return q, invalidQuery("series cannot use bucket modifiers")
		}
		if !grouped {
			return q, invalidQuery("series requires group_by")
		}
		f, err := env.lookup(raw)
		if err != nil {
			return q, err
		}
		selects = append(selects, f.expr+" AS series")
		groups = append(groups, "series")
		columns = append(columns, "series")
	}
	for _, m := range metrics {
		selects = append(selects, m.expr+" AS "+sqlAlias(m.alias))
		columns = append(columns, m.alias)
	}
	order, err := compileAggregateOrder(values.Get("sort"), columns, grouped)
	if err != nil {
		return q, err
	}
	sqlText := "SELECT " + strings.Join(selects, ", ") + " FROM " + source + where
	if len(groups) > 0 {
		sqlText += " GROUP BY " + strings.Join(groups, ", ")
	}
	if order != "" {
		sqlText += " ORDER BY " + order
	}
	sqlText += " LIMIT " + strconv.Itoa(q.Limit+1)
	q.SQL = sqlText
	q.Args = append(selectArgs, filterArgs...)
	q.Columns = columns
	// 占比分母：分组结果会被 limit 截断，前端拿不到尾部类目。这里额外给出同一 filter、
	// 不分组的可累加度量总量，让「其他」= 总量 − 已返回分组之和，而不是只对部分分组求占比。
	if grouped && wantAggregateTotal(values) && len(metrics) == 1 && isAdditiveMetricKind(metrics[0].kind) {
		q.TotalSQL = "SELECT " + metrics[0].expr + " FROM " + source + where
		q.TotalArgs = append([]any(nil), filterArgs...)
	}
	return q, nil
}

// wantAggregateTotal 判断调用方是否要「同口径总量」。默认不要：多一次聚合查询，只在画占比时才值得。
func wantAggregateTotal(values url.Values) bool {
	switch strutil.Normalize(values.Get("with_total")) {
	case "1", "true":
		return true
	default:
		return false
	}
}

// isAdditiveMetricKind：只有 count / sum 能在分组间相加；avg / min / max / distinct 相加没有意义。
func isAdditiveMetricKind(kind string) bool {
	return kind == "count" || kind == "sum"
}

// sqlAlias 度量别名只由字母数字下划线组成（来自已校验字段名），这里再兜底一次。
func sqlAlias(alias string) string {
	var b strings.Builder
	for _, r := range alias {
		if r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// compileAggregateOrder 只允许按本次响应列排序。缺省：有维度时按主维度 x 升序（时间桶、分箱、
// 分类都适合；前端条形图需要按度量降序时显式传 sort=-count），无维度时不排序。
func compileAggregateOrder(raw string, columns []string, grouped bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if !grouped {
			return "", nil
		}
		return "x ASC", nil
	}
	allowed := map[string]bool{}
	for _, c := range columns {
		allowed[c] = true
	}
	parts := []string{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		desc := strings.HasPrefix(item, "-")
		name := strings.TrimPrefix(item, "-")
		if !allowed[name] {
			return "", invalidQuery("sort must reference a returned column")
		}
		dir := " ASC"
		if desc {
			dir = " DESC"
		}
		parts = append(parts, sqlAlias(name)+dir)
	}
	return strings.Join(parts, ", "), nil
}

func compilePoints(q aggregateQuery, values url.Values, source, where string, filterArgs []any, env aggregateEnv) (aggregateQuery, error) {
	q.Points = true
	xf, err := requireNumberField("x", values.Get("x"), env)
	if err != nil {
		return q, err
	}
	yf, err := requireNumberField("y", values.Get("y"), env)
	if err != nil {
		return q, err
	}
	selects := []string{xf.expr + " AS x", yf.expr + " AS y"}
	columns := []string{"x", "y"}
	conds := []string{xf.expr + " IS NOT NULL", yf.expr + " IS NOT NULL"}
	if raw := strings.TrimSpace(values.Get("size")); raw != "" {
		sf, err := requireNumberField("size", raw, env)
		if err != nil {
			return q, err
		}
		selects = append(selects, sf.expr+" AS size")
		columns = append(columns, "size")
	}
	if raw := strings.TrimSpace(values.Get("series")); raw != "" {
		sf, err := env.lookup(raw)
		if err != nil {
			return q, err
		}
		switch sf.field.Type {
		case "text", "select", "bool", "email", "url":
		default:
			return q, invalidQuery("scatter series requires a text, select or bool field")
		}
		selects = append(selects, sf.expr+" AS series")
		columns = append(columns, "series")
		q.needsSeriesFold = true
	}
	cond := strings.Join(conds, " AND ")
	if where == "" {
		where = " WHERE " + cond
	} else {
		where += " AND " + cond
	}
	// 固定排序保证同参数结果稳定（集合按 created_at / id，SQL 表按自己的时间列与主键）；多取一行用来判断 truncated。
	q.SQL = "SELECT " + strings.Join(selects, ", ") + " FROM " + source + where + env.orderClause() + " LIMIT " + strconv.Itoa(q.Limit+1)
	q.Args = append([]any(nil), filterArgs...)
	q.CountSQL = "SELECT COUNT(*) FROM " + source + where
	q.CountArgs = append([]any(nil), filterArgs...)
	q.Columns = columns
	return q, nil
}

func requireNumberField(label, name string, env aggregateEnv) (aggregateField, error) {
	if strings.TrimSpace(name) == "" {
		return aggregateField{}, invalidQuery(label + " is required")
	}
	f, err := env.lookup(name)
	if err != nil {
		return aggregateField{}, err
	}
	if f.field.Type != "number" {
		return aggregateField{}, invalidQuery(label + " requires a number field")
	}
	return f, nil
}

// foldSeries 把散点分组数限制在 aggregateSeriesCap：按出现次数取前 N，其余归并为「其他」。
// series 列是行内最后一列。
func foldSeries(rows [][]any, seriesIdx int) {
	counts := map[string]int{}
	for _, row := range rows {
		counts[seriesKey(row[seriesIdx])]++
	}
	if len(counts) <= aggregateSeriesCap {
		return
	}
	type kv struct {
		key string
		n   int
	}
	list := make([]kv, 0, len(counts))
	for k, n := range counts {
		list = append(list, kv{k, n})
	}
	// 稳定排序：次数降序，次数相同按 key 升序，保证结果可复现。
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && (list[j].n > list[j-1].n || list[j].n == list[j-1].n && list[j].key < list[j-1].key); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
	keep := map[string]bool{}
	for _, item := range list[:aggregateSeriesCap-1] {
		keep[item.key] = true
	}
	for _, row := range rows {
		if !keep[seriesKey(row[seriesIdx])] {
			row[seriesIdx] = "其他"
		}
	}
}

func seriesKey(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
