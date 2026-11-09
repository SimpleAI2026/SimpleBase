// 文件说明：system_redact.go — internal/api 的内部实现（systemSensitiveColumns、isSystemSensitiveColumn…
//
// 用法
//
//	同包调用：isSystemSensitiveColumn() → schema_rows.go；systemSQLReferencesSensitive() → sql_handler.go；redactSystemResult() → sql_handler.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/system_redact.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/sqlx
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] isSystemSensitiveColumn() ← schema_rows.go
//
// 主要符号
//
//	var systemSensitiveColumns
//	isSystemSensitiveColumn(name string) bool
//	systemSQLReferencesSensitive(sql string) bool
//	readSQLQuotedIdent(sql string, i int) (string, int)
//	redactSystemResult(columns []string, rows [][]any) []string

package api

import (
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
)

// systemSensitiveColumns 是系统库里不能离开服务端的凭证列。
// 比较时忽略大小写。用量列（token_input 等）不在此列。
var systemSensitiveColumns = map[string]struct{}{
	"password_hash":      {},
	"key_hash":           {},
	"refresh_token_hash": {},
	"access_jti":         {},
	"credentials_json":   {},
	"credential_ref":     {},
}

func isSystemSensitiveColumn(name string) bool {
	_, ok := systemSensitiveColumns[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// systemSQLReferencesSensitive 在跳过字符串字面量和注释后，查找敏感列标识符。
// 用来挡住 SELECT password_hash AS h 这类把列名改掉的查询。SELECT * 不含列名，改由结果列名抹除。
func systemSQLReferencesSensitive(sql string) bool {
	for i := 0; i < len(sql); {
		switch {
		case sql[i] == '\'':
			i++
			for i < len(sql) {
				if sql[i] == '\'' {
					i++
					if i < len(sql) && sql[i] == '\'' {
						i++
						continue
					}
					break
				}
				i++
			}
		case i+1 < len(sql) && sql[i:i+2] == "--":
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case i+1 < len(sql) && sql[i:i+2] == "/*":
			i += 2
			for i+1 < len(sql) && sql[i:i+2] != "*/" {
				i++
			}
			if i+1 < len(sql) {
				i += 2
			}
		case sql[i] == '"':
			ident, next := readSQLQuotedIdent(sql, i)
			if isSystemSensitiveColumn(ident) {
				return true
			}
			i = next
		case sqlx.IsIdentByte(sql[i]):
			start := i
			for i < len(sql) && sqlx.IsIdentByte(sql[i]) {
				i++
			}
			if isSystemSensitiveColumn(sql[start:i]) {
				return true
			}
		default:
			i++
		}
	}
	return false
}

func readSQLQuotedIdent(sql string, i int) (string, int) {
	i++
	var b strings.Builder
	for i < len(sql) {
		if sql[i] == '"' {
			i++
			if i < len(sql) && sql[i] == '"' {
				b.WriteByte('"')
				i++
				continue
			}
			break
		}
		b.WriteByte(sql[i])
		i++
	}
	return b.String(), i
}

// redactSystemResult 把结果里的敏感列改成 nil，并返回这些列名。没有命中时返回 nil。
func redactSystemResult(columns []string, rows [][]any) []string {
	indexes := make([]int, 0)
	names := make([]string, 0)
	for i, name := range columns {
		if isSystemSensitiveColumn(name) {
			indexes = append(indexes, i)
			names = append(names, name)
		}
	}
	if len(indexes) == 0 {
		return nil
	}
	for _, row := range rows {
		for _, idx := range indexes {
			if idx < len(row) {
				row[idx] = nil
			}
		}
	}
	return names
}
