// 文件说明：dialect.go — godatabase 中 Dialect、IsInternalTableName 等符号的实现
//
// 用法
//
//	IsInternalTableName(name string) bool  // IsInternalTableName 是内部保留命名空间的唯一判定：所有以 "__" 开头的表
//	DialectFor(engine string) (Dialect, error)  // DialectFor 按目录中的引擎选择方言
//
// 调用架构
//
//	[上游] godatabase/cache, godatabase/ducklake, godatabase/filedb 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/dialect.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] query.go, runtime.go, serialize.go, snapshot.go, spec.go 等 6 个
//
// 主要符号
//
//	type Dialect interface {…}
//	IsInternalTableName(name string) bool
//	const InternalTablePrefix
//	DialectFor(engine string) (Dialect, error)
//	... 另有 8 个顶层符号

package godatabase

import (
	"fmt"
	"strings"
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
}

const internalTablePrefix = "__sb_"

// IsInternalTableName 是内部保留命名空间的唯一判定：所有以 "__" 开头的表
// 均不向用户暴露（含 __sb_ 系列和系统库历史内部表）。
func IsInternalTableName(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), "__")
}

// InternalTablePrefix 由 KV 与系统内部表使用。
const InternalTablePrefix = internalTablePrefix

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
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
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
		base += " AND c.table_name = " + sqlLiteral(table)
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
func (duckDBDialect) NullsFirst(expr string, descending bool) string {
	if descending {
		return expr + " DESC NULLS LAST"
	}
	return expr + " ASC NULLS FIRST"
}

type sqliteDialect struct{}

func (sqliteDialect) Engine() string { return EngineSQLite }

func (sqliteDialect) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
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
		base += " AND m.name = " + sqlLiteral(table)
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
func (sqliteDialect) NullsFirst(expr string, descending bool) string {
	if descending {
		return expr + " DESC"
	}
	return expr + " ASC"
}

func sqlLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
