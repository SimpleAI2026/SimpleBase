// 文件说明：dialect.go — godatabase 中 Dialect、IsInternalTableName 等符号的实现
//
// 用法
//
//	type Dialect interface{…}  // Dialect 描述用户库引擎之间必须差异化的元数据查询
//	IsInternalTableName(name string) bool  // IsInternalTableName 是内部保留命名空间的唯一判定：所有以 "__" 开头的表 均不向用户暴露（含 __sb_ 系列和系统库历史内部表）
//	DialectFor(engine string) (Dialect, error)  // DialectFor 按目录中的引擎选择方言
//
// 调用架构
//
//	[上游] godatabase/cache, godatabase/cmd, godatabase/ducklake, godatabase/filedb 等 16 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/dialect.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] coverage_test.go, errmatch.go, query.go, query_test.go, runtime.go 等 10 个
//
// 主要符号
//
//	type Dialect interface{…}
//	var DateBucketUnits
//	dateBucketFormat(unit string) (string, bool)
//	const InternalTablePrefix
//	IsInternalTableName(name string) bool
//	DialectFor(engine string) (Dialect, error)
//	type duckDBDialect struct{…}
//	(duckDBDialect) Engine() string
//	... 另有 26 个顶层符号

package godatabase

import (
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
)

// Dialect 描述用户库引擎之间必须差异化的元数据查询。
// 只用于内部表结构、行分页和统计；用户 SQL 原样执行，不做转译。
type Dialect interface {
	Engine() string
	// TableColumnsSQL 返回 table_name, column_name, data_type, is_nullable 四列。
	// table 为空表示列出全部用户表。
	TableColumnsSQL(table string) string
	// CountRowsSQL 统计指定表的行数。
	CountRowsSQL(table string) string
	// QuoteIdent 引用标识符。
	QuoteIdent(name string) string
	// IsInternalTable 判断是否属于 SimpleBase 内部表（不向用户暴露）。
	IsInternalTable(name string) bool
	JSONExtractText(path string) string
	JSONExtractNumber(path string) string
	JSONExtractBool(path string) string
	CastTimestamp(expr string) string
	NullsFirst(expr string, descending bool) string
	// DateBucket 把时间表达式截断到 unit（hour/day/week/month/year）并输出文本，
	// 同一 unit 在两个引擎得到相同格式；unit 不在白名单时 ok=false，调用方必须拒绝。
	DateBucket(expr, unit string) (sql string, ok bool)
	// Floor 向负无穷取整（返回 DOUBLE / REAL）。SQLite 默认构建没有 FLOOR，需要手写。
	Floor(expr string) string
	// TimestampParam 返回与 CastTimestamp 同一口径的「参数侧」时间表达式（形如 CAST(? AS TIMESTAMP)）。
	// SQLite 的 datetime() 输出 'YYYY-MM-DD HH:MM:SS'，若参数仍是 RFC3339 的 'T' 分隔文本，
	// 逐字符比较时空格 < 'T'，范围过滤会整体错位，所以两侧必须都经 datetime() 规整。
	TimestampParam() string
}

// DateBucketUnits 是 DateBucket 接受的 unit 白名单。
var DateBucketUnits = []string{"hour", "day", "week", "month", "year"}

// dateBucketFormat 返回 unit 对应的 strftime 输出格式。
func dateBucketFormat(unit string) (string, bool) {
	switch unit {
	case "hour":
		return "%Y-%m-%d %H:00", true
	case "day", "week":
		return "%Y-%m-%d", true
	case "month":
		return "%Y-%m", true
	case "year":
		return "%Y", true
	default:
		return "", false
	}
}

// InternalTablePrefix 由 KV 与系统内部表使用。
const InternalTablePrefix = "__sb_"

// IsInternalTableName 是内部保留命名空间的唯一判定：所有以 "__" 开头的表
// 均不向用户暴露（含 __sb_ 系列和系统库历史内部表）。
func IsInternalTableName(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), "__")
}

// DialectFor 按目录中的引擎选择方言。
func DialectFor(engine string) (Dialect, error) {
	switch engine {
	case "", EngineDuckDB:
		return duckDBDialect{}, nil
	case EngineSQLite:
		return sqliteDialect{}, nil
	default:
		return nil, fmt.Errorf("database: unsupported engine %q", engine)
	}
}

type duckDBDialect struct{}

func (duckDBDialect) Engine() string { return EngineDuckDB }

func (duckDBDialect) QuoteIdent(name string) string {
	return sqlx.Ident(name)
}

func (d duckDBDialect) TableColumnsSQL(table string) string {
	base := `SELECT c.table_name, c.column_name, c.data_type, c.is_nullable
		FROM information_schema.columns c
		JOIN information_schema.tables t
		  ON c.table_catalog = t.table_catalog
		 AND c.table_schema = t.table_schema
		 AND c.table_name = t.table_name
		WHERE t.table_catalog = current_database()
		  AND t.table_schema = current_schema()
		  AND t.table_type = 'BASE TABLE'`
	if table != "" {
		base += " AND c.table_name = " + sqlx.Literal(table)
	}
	return base + " ORDER BY c.table_name, c.ordinal_position"
}

func (d duckDBDialect) CountRowsSQL(table string) string {
	return "SELECT COUNT(*) FROM " + d.QuoteIdent(table)
}

func (duckDBDialect) IsInternalTable(name string) bool { return IsInternalTableName(name) }
func (duckDBDialect) JSONExtractText(path string) string {
	return "json_extract_string(data, '" + path + "')"
}
func (duckDBDialect) JSONExtractNumber(path string) string {
	return "CAST(json_extract(data, '" + path + "') AS DOUBLE)"
}
func (duckDBDialect) JSONExtractBool(path string) string {
	return "CAST(json_extract(data, '" + path + "') AS BOOLEAN)"
}
func (duckDBDialect) CastTimestamp(expr string) string { return "CAST(" + expr + " AS TIMESTAMP)" }
func (duckDBDialect) TimestampParam() string           { return "CAST(? AS TIMESTAMP)" }
func (duckDBDialect) NullsFirst(expr string, descending bool) string {
	if descending {
		return expr + " DESC NULLS LAST"
	}
	return expr + " ASC NULLS FIRST"
}

func (duckDBDialect) Floor(expr string) string { return "FLOOR(" + expr + ")" }

// DateBucket：DuckDB 先 date_trunc（week 取周一），再 strftime 成文本。
func (duckDBDialect) DateBucket(expr, unit string) (string, bool) {
	format, ok := dateBucketFormat(unit)
	if !ok {
		return "", false
	}
	return "strftime(date_trunc('" + unit + "', " + expr + "), '" + format + "')", true
}

type sqliteDialect struct{}

func (sqliteDialect) Engine() string { return EngineSQLite }

func (sqliteDialect) QuoteIdent(name string) string {
	return sqlx.Ident(name)
}

func (d sqliteDialect) TableColumnsSQL(table string) string {
	base := `SELECT m.name AS table_name, p.name AS column_name, p.type AS data_type,
		CASE WHEN p."notnull" = 0 THEN 'YES' ELSE 'NO' END AS is_nullable
		FROM sqlite_master m
		JOIN pragma_table_info(m.name) p
		WHERE m.type = 'table'
		  AND m.name NOT LIKE 'sqlite\_%' ESCAPE '\'
		  AND m.name NOT LIKE '__sb\_%' ESCAPE '\'`
	if table != "" {
		base += " AND m.name = " + sqlx.Literal(table)
	}
	return base + " ORDER BY m.name, p.cid"
}

func (d sqliteDialect) CountRowsSQL(table string) string {
	return "SELECT COUNT(*) FROM " + d.QuoteIdent(table)
}

func (sqliteDialect) IsInternalTable(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return IsInternalTableName(lower) || strings.HasPrefix(lower, "sqlite_")
}
func (sqliteDialect) JSONExtractText(path string) string { return "json_extract(data, '" + path + "')" }
func (sqliteDialect) JSONExtractNumber(path string) string {
	return "CAST(json_extract(data, '" + path + "') AS REAL)"
}
func (sqliteDialect) JSONExtractBool(path string) string {
	return "CAST(json_extract(data, '" + path + "') AS BOOLEAN)"
}
func (sqliteDialect) CastTimestamp(expr string) string { return "datetime(" + expr + ")" }
func (sqliteDialect) TimestampParam() string           { return "datetime(?)" }
func (sqliteDialect) NullsFirst(expr string, descending bool) string {
	if descending {
		return expr + " DESC"
	}
	return expr + " ASC"
}

// Floor：CAST AS INTEGER 向零截断，负数非整数时要再减 1 才是向负无穷取整。
// expr 会出现多次，调用方传入的必须是无副作用的纯表达式。
func (sqliteDialect) Floor(expr string) string {
	return "(CAST(" + expr + " AS INTEGER) - (" + expr + " < CAST(" + expr + " AS INTEGER)))"
}

// DateBucket：SQLite 没有 date_trunc，用 strftime 直接格式化；week 先回退到
// 本周一（weekday 1 = 周一，与 DuckDB date_trunc('week') 对齐）。
func (sqliteDialect) DateBucket(expr, unit string) (string, bool) {
	format, ok := dateBucketFormat(unit)
	if !ok {
		return "", false
	}
	if unit == "week" {
		return "strftime('" + format + "', " + expr + ", '-' || ((CAST(strftime('%w', " + expr + ") AS INTEGER) + 6) % 7) || ' days')", true
	}
	return "strftime('" + format + "', " + expr + ")", true
}
