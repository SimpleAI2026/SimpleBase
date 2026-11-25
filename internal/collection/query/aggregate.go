// 文件说明：aggregate.go — 集合聚合接口的参数解析与 SQL 拼装（metric / group_by / order / limit / points）
//
// 用法
//
//	ParseAggregateQuery(values url.Values, source string, schema *collection.Collection, dialect godatabase.Dialect) (AggregateQuery, error)
//	ParseAggregateEnv(values url.Values, source string, env AggregateEnv) (AggregateQuery, error) // SQL 表出图：字段环境由调用方给出
//	FoldSeries(rows [][]any, seriesIdx int)   // 稀疏序列补齐
//
// 调用架构
//
//	[上游] internal/api（集合聚合、SQL 表聚合）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/collection/query/aggregate.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase（方言）, internal/collection, internal/utils/strutil
//	[同包] filter.go, env.go
//
// 主要符号
//
//	type AggregateQuery struct {…}
//	type AggregateMetric struct {…}
//	type AggregateField struct {…}
//	ParseAggregateQuery(values url.Values, source string, schema *collection.Collection, dialect godatabase.Dialect) (AggregateQuery, error)
//	ParseAggregateEnv(values url.Values, source string, env AggregateEnv) (AggregateQuery, error)
//	FoldSeries(rows [][]any, seriesIdx int)
//	... 另有 14 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §5.1）：聚合参数规则属于集合，但产出 SQL，故与 filter 同在
// collection/query 子包；错误为 *collection.ValidationError，文案与迁移前逐字一致。

package query

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
	// SeriesCap 是散点图分组数上限，超出的归入「其他」。
	SeriesCap = 12
)

// MetricKinds 是聚合度量种类全集。导出给 cloudagent 提示词做漂移防护测试。
var MetricKinds = []string{"count", "sum", "avg", "min", "max", "distinct"}

// AggregateQuery 是已编译的聚合请求。
type AggregateQuery struct {
	Points    bool
	Limit     int
	SQL       string
	Args      []any
	CountSQL  string
	CountArgs []any
	// Columns 是响应列名（x / series / 度量别名；散点为 x / y / size / series）。
	Columns []string
	// NeedsSeriesFold 表示散点带分组列，应用层要把分组数限制在 SeriesCap。
	NeedsSeriesFold bool
	// TotalSQL 非空时表示：同一 filter 下、不分组的第一个可累加度量总量（planv6.7 BI-F01）。
	// 只在调用方显式要 with_total、且有分组 + 单度量 + count/sum 时生成；avg/min/max 不可累加，不提供。
	TotalSQL  string
	TotalArgs []any
}

type AggregateMetric struct {
	kind  string
	field string
	alias string
	expr  string
}

type AggregateField struct {
	field collection.Field
	expr  string
}

// ParseAggregateQuery 把 query string 编译成 SQL。source 来自 collectionSource。
func ParseAggregateQuery(values url.Values, source string, schema *collection.Collection, dialect godatabase.Dialect) (AggregateQuery, error) {
	return ParseAggregateEnv(values, source, CollectionEnv(schema, dialect))
}

// ParseAggregateEnv 是聚合解析的共用入口：集合与 SQL 表只在 env（字段解析、过滤、散点排序）上不同。
func ParseAggregateEnv(values url.Values, source string, env AggregateEnv) (AggregateQuery, error) {
	mode := strings.TrimSpace(values.Get("mode"))
	if mode != "" && mode != "points" {
		return AggregateQuery{}, collection.InvalidQuery("mode must be points")
	}
	limit, err := parseAggregateLimit(values.Get("limit"), mode == "points")
	if err != nil {
		return AggregateQuery{}, err
	}
	filterSQL, filterArgs, err := env.Filter(values.Get("filter"))
	if err != nil {
		return AggregateQuery{}, err
	}
	where := ""
	if filterSQL != "" {
		where = " WHERE " + filterSQL
	}
	q := AggregateQuery{
		Limit:     limit,
		CountSQL:  "SELECT COUNT(*) FROM " + source + where,
		CountArgs: append([]any(nil), filterArgs...),
	}
	if mode == "points" {
		for _, key := range []string{"metrics", "group_by"} {
			if strings.TrimSpace(values.Get(key)) != "" {
				return AggregateQuery{}, collection.InvalidQuery("points mode cannot be combined with " + key)
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
		return 0, collection.InvalidQuery(fmt.Sprintf("limit must be between 1 and %d", max))
	}
	return n, nil
}

// lookupAggregateField 取字段表达式。created_at / updated_at / id 是内置列，
// 其余必须是已登记字段；json 字段一律拒绝做维度或度量。
func lookupAggregateField(name string, schema *collection.Collection, dialect godatabase.Dialect) (AggregateField, error) {
	name = strings.TrimSpace(name)
	if !collection.FieldNamePattern.MatchString(name) {
		return AggregateField{}, collection.InvalidQuery("invalid field name")
	}
	switch name {
	case "id":
		return AggregateField{field: collection.Field{Name: name, Type: "text"}, expr: "id"}, nil
	case "created_at":
		return AggregateField{field: collection.Field{Name: name, Type: "date"}, expr: dialect.CastTimestamp("created_at")}, nil
	case "updated_at":
		return AggregateField{field: collection.Field{Name: name, Type: "date"}, expr: dialect.CastTimestamp("COALESCE(updated_at, created_at)")}, nil
	}
	if schema == nil {
		return AggregateField{}, &collection.ValidationError{Code: "schema_not_registered", Field: name, Message: "schema required for aggregate"}
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
			return AggregateField{field: f, expr: dialect.JSONExtractNumber(path)}, nil
		case "bool":
			return AggregateField{field: f, expr: dialect.JSONExtractBool(path)}, nil
		case "date":
			return AggregateField{field: f, expr: dialect.CastTimestamp(dialect.JSONExtractText(path))}, nil
		case "json":
			return AggregateField{}, collection.InvalidQuery("json fields cannot be aggregated")
		default:
			return AggregateField{field: f, expr: dialect.JSONExtractText(path)}, nil
		}
	}
	return AggregateField{}, &collection.ValidationError{Code: "unknown_field", Field: name, Message: "field is not registered"}
}

func parseAggregateMetrics(raw string, env AggregateEnv) ([]AggregateMetric, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, collection.InvalidQuery("metrics is required")
	}
	parts := strings.Split(raw, ",")
	if len(parts) > aggregateMaxMetrics {
		return nil, collection.InvalidQuery(fmt.Sprintf("at most %d metrics", aggregateMaxMetrics))
	}
	seen := map[string]bool{}
	out := make([]AggregateMetric, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kind, name, hasField := strings.Cut(part, ":")
		m := AggregateMetric{kind: kind, field: name}
		switch kind {
		case "count":
			if hasField {
				return nil, collection.InvalidQuery("count takes no field")
			}
			m.alias, m.expr = "count", "COUNT(*)"
		case "sum", "avg", "min", "max", "distinct":
			if !hasField {
				return nil, collection.InvalidQuery(kind + " requires a field")
			}
			f, err := env.Lookup(name)
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
			return nil, collection.InvalidQuery("unknown metric")
		}
		if seen[m.alias] {
			return nil, collection.InvalidQuery("duplicate metric")
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
			return collection.InvalidQuery(kind + " requires a number field")
		}
	case "min", "max":
		if f.Type != "number" && f.Type != "date" {
			return collection.InvalidQuery(kind + " requires a number or date field")
		}
	}
	return nil
}

// parseGroupBy 解析 field / field:<unit> / field:bin=<step>。
func parseGroupBy(raw string, env AggregateEnv) (expr string, args []any, err error) {
	name, mod, hasMod := strings.Cut(strings.TrimSpace(raw), ":")
	f, err := env.Lookup(name)
	if err != nil {
		return "", nil, err
	}
	if !hasMod {
		return f.expr, nil, nil
	}
	if strings.HasPrefix(mod, "bin=") {
		if f.field.Type != "number" {
			return "", nil, collection.InvalidQuery("bin requires a number field")
		}
		step, perr := strconv.ParseFloat(strings.TrimPrefix(mod, "bin="), 64)
		if perr != nil || step <= 0 || math.IsInf(step, 0) || math.IsNaN(step) {
			return "", nil, collection.InvalidQuery("bin must be a positive number")
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
		return "", nil, collection.InvalidQuery("time bucket requires a date field")
	}
	bucket, ok := env.dialect.DateBucket(f.expr, mod)
	if !ok {
		return "", nil, collection.InvalidQuery("unsupported time bucket")
	}
	return bucket, nil, nil
}

func compileAggregate(q AggregateQuery, values url.Values, source, where string, filterArgs []any, env AggregateEnv) (AggregateQuery, error) {
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
			return q, collection.InvalidQuery("series cannot use bucket modifiers")
		}
		if !grouped {
			return q, collection.InvalidQuery("series requires group_by")
		}
		f, err := env.Lookup(raw)
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
			return "", collection.InvalidQuery("sort must reference a returned column")
		}
		dir := " ASC"
		if desc {
			dir = " DESC"
		}
		parts = append(parts, sqlAlias(name)+dir)
	}
	return strings.Join(parts, ", "), nil
}

func compilePoints(q AggregateQuery, values url.Values, source, where string, filterArgs []any, env AggregateEnv) (AggregateQuery, error) {
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
		sf, err := env.Lookup(raw)
		if err != nil {
			return q, err
		}
		switch sf.field.Type {
		case "text", "select", "bool", "email", "url":
		default:
			return q, collection.InvalidQuery("scatter series requires a text, select or bool field")
		}
		selects = append(selects, sf.expr+" AS series")
		columns = append(columns, "series")
		q.NeedsSeriesFold = true
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

func requireNumberField(label, name string, env AggregateEnv) (AggregateField, error) {
	if strings.TrimSpace(name) == "" {
		return AggregateField{}, collection.InvalidQuery(label + " is required")
	}
	f, err := env.Lookup(name)
	if err != nil {
		return AggregateField{}, err
	}
	if f.field.Type != "number" {
		return AggregateField{}, collection.InvalidQuery(label + " requires a number field")
	}
	return f, nil
}

// FoldSeries 把散点分组数限制在 SeriesCap：按出现次数取前 N，其余归并为「其他」。
// series 列是行内最后一列。
func FoldSeries(rows [][]any, seriesIdx int) {
	counts := map[string]int{}
	for _, row := range rows {
		counts[seriesKey(row[seriesIdx])]++
	}
	if len(counts) <= SeriesCap {
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
	for _, item := range list[:SeriesCap-1] {
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
