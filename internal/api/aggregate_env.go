// 文件说明：aggregate_env.go — 聚合编译的「字段环境」抽象（planv6.7 扩展：SQL 表出图）
//
// 集合与 SQL 表共用同一套聚合编译（度量 / 维度 / 时间桶 / 分箱 / 散点），差别只有三处：
//   - 字段怎么变成 SQL 表达式：集合是 JSON 路径提取，SQL 表是真实列；
//   - filter 里的字段怎么解析：同上；
//   - 散点的稳定排序列：集合是 created_at / id，SQL 表取自己的时间列与主键。
//
// aggregateEnv 把这三处收口，编译主体不再关心数据源是什么。

package api

import (
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

type aggregateEnv struct {
	dialect godatabase.Dialect
	// lookupFn 把字段名解析成带类型的 SQL 表达式；必须自行完成白名单校验。
	lookupFn func(name string) (aggregateField, error)
	// filterFn 编译 filter 表达式。
	filterFn func(raw string) (string, []any, error)
	// order 是散点点集的稳定排序子句（含前导空格的 " ORDER BY ..."），可为空。
	order string
}

func (e aggregateEnv) lookup(name string) (aggregateField, error) { return e.lookupFn(name) }

func (e aggregateEnv) filter(raw string) (string, []any, error) { return e.filterFn(raw) }

func (e aggregateEnv) orderClause() string { return e.order }

// collectionEnv 是集合（JSON 文档表）的字段环境，行为与改造前完全一致。
func collectionEnv(schema *collection.Collection, dialect godatabase.Dialect) aggregateEnv {
	return aggregateEnv{
		dialect: dialect,
		lookupFn: func(name string) (aggregateField, error) {
			return lookupAggregateField(name, schema, dialect)
		},
		filterFn: func(raw string) (string, []any, error) {
			return compileFilter(raw, schema, dialect)
		},
		order: " ORDER BY created_at DESC, id DESC",
	}
}

// sqlColumnKind 把引擎的列类型归到聚合关心的四类。无法识别的归为 text（只能做分类 / 去重）。
func sqlColumnKind(dataType string) string {
	t := strings.ToUpper(strings.TrimSpace(dataType))
	// 去掉长度 / 精度：VARCHAR(20) → VARCHAR，DECIMAL(10,2) → DECIMAL。
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = t[:i]
	}
	switch {
	case t == "BOOLEAN" || t == "BOOL":
		return "bool"
	case strings.HasPrefix(t, "TIMESTAMP") || t == "DATETIME" || t == "DATE":
		return "date"
	case t == "BIGINT" || t == "INTEGER" || t == "INT" || t == "SMALLINT" || t == "TINYINT" || t == "HUGEINT" ||
		t == "UBIGINT" || t == "UINTEGER" || t == "USMALLINT" || t == "UTINYINT" || t == "INT2" || t == "INT4" || t == "INT8" ||
		t == "DOUBLE" || t == "FLOAT" || t == "REAL" || t == "DECIMAL" || t == "NUMERIC" || t == "FLOAT4" || t == "FLOAT8":
		return "number"
	case t == "JSON" || t == "BLOB" || t == "BYTEA" || t == "STRUCT" || t == "LIST" || t == "MAP" || t == "UNION" || t == "ARRAY":
		// 结构化 / 二进制列不能分组也不能统计。
		return "json"
	default:
		return "text"
	}
}

// sqlTableColumn 是一张 SQL 表里一个可用于聚合的真实列。
type sqlTableColumn struct {
	Name string
	Kind string
}

// sqlTableEnv 构造 SQL 表的字段环境。
//
// 列名只来自引擎元数据（TableColumnsSQL），且必须再通过 fieldNamePattern；表达式一律用
// sqlx.Ident 引用，所以用户传入的字段名只能「命中」真实列，不可能拼进 SQL 文本。
// sensitive 返回 true 的列（系统库的凭证列）视为不存在。
func sqlTableEnv(dialect godatabase.Dialect, columns []sqlTableColumn, sensitive func(string) bool) aggregateEnv {
	byName := make(map[string]sqlTableColumn, len(columns))
	for _, col := range columns {
		if !fieldNamePattern.MatchString(col.Name) || (sensitive != nil && sensitive(col.Name)) {
			continue
		}
		byName[col.Name] = col
	}
	resolve := func(name string) (collection.Field, string, error) {
		name = strings.TrimSpace(name)
		if !fieldNamePattern.MatchString(name) {
			return collection.Field{}, "", invalidQuery("invalid field name")
		}
		col, ok := byName[name]
		if !ok {
			return collection.Field{}, "", &collection.ValidationError{Code: "unknown_field", Field: name, Message: "column does not exist"}
		}
		ident := sqlx.Ident(col.Name)
		field := collection.Field{Name: col.Name, Type: col.Kind}
		switch col.Kind {
		case "date":
			return field, dialect.CastTimestamp(ident), nil
		case "json":
			return collection.Field{}, "", invalidQuery("json fields cannot be aggregated")
		default:
			return field, ident, nil
		}
	}
	return aggregateEnv{
		dialect: dialect,
		lookupFn: func(name string) (aggregateField, error) {
			f, expr, err := resolve(name)
			if err != nil {
				return aggregateField{}, err
			}
			return aggregateField{field: f, expr: expr}, nil
		},
		filterFn: func(raw string) (string, []any, error) {
			return compileFilterWith(raw, nil, dialect, resolve)
		},
		order: sqlTableOrder(columns, byName),
	}
}

// sqlTableOrder 为散点点集挑一个**确定性**排序：优先时间列倒序，再用 id 升序兜底；都没有就按第一列。
// 只会用到 byName 里存在的列（敏感列已被剔除）。
//
// 承诺边界（planv6.7 BI-F12）：这里不读主键元数据，也无法证明选中的列唯一。
//   - 有「时间列」时：结果按时间从新到旧，但同值行之间只由 id 兜底；没有 id 时并列行的先后由引擎决定，
//     截断边界处哪几条进来并不保证可复现；
//   - 没有时间列时：只是「按某个列排序取前 N 行」，不是「最新 N 行」，也不能当成随机抽样。
//
// 因此前端只说「返回前 N 个点，不是全部数据」，不说「最新」或「随机样本」。
func sqlTableOrder(columns []sqlTableColumn, byName map[string]sqlTableColumn) string {
	lower := make(map[string]string, len(byName))
	for name := range byName {
		lower[strings.ToLower(name)] = name
	}
	idCol, hasID := lower["id"]
	for _, candidate := range []string{"occurred_at", "created_at", "updated_at", "started_at"} {
		if actual, ok := lower[candidate]; ok {
			order := " ORDER BY " + sqlx.Ident(actual) + " DESC"
			if hasID {
				order += ", " + sqlx.Ident(idCol) + " ASC"
			}
			return order
		}
	}
	if hasID {
		return " ORDER BY " + sqlx.Ident(idCol) + " ASC"
	}
	for _, col := range columns {
		if _, ok := byName[col.Name]; ok {
			return " ORDER BY " + sqlx.Ident(col.Name) + " ASC"
		}
	}
	return ""
}
