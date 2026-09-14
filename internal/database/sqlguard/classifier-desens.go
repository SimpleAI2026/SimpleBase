// Package sqlguard 提供低成本的 SQL 意图分类与策略校验。
//
// 它不是 SQL 防火墙，也不做完整的语法解析。只执行可测试、低延迟的规则：
//   - 去除前导空白与 SQL 注释后判断首关键字
//   - 拒绝空 SQL、多语句、NUL 字符
//   - 按意图（只读 / 写允许）过滤首关键字
//   - 永久拒绝 DuckDB 管理面指令：ATTACH/DETACH/INSTALL/LOAD/COPY/SET/PRAGMA/CALL 等
//
// 若无法可靠判断，保守拒绝并返回 ErrSQLNotAllowed。
package sqlguard

import (
	"strings"
)

// Intent 表示调用方期望的 SQL 意图。
type Intent uint8

const (
	// ReadOnly 仅允许 SELECT/WITH/EXPLAIN/DESCRIBE/SHOW 等只读关键字。
	ReadOnly Intent = iota
	// WriteAllowed 允许 DML/DDL，但仍拒绝危险关键字。
	WriteAllowed
)

// FirstKeyword 去除前导空白与 SQL 注释后返回首关键字（大写）。
// 空字符串表示无有效关键字（例如纯注释）。
func FirstKeyword(sql string) string {
	s := stripLeadingNoise(sql)
	if s == "" {
		return ""
	}
	end := firstTokenEnd(s)
	return strings.ToUpper(s[:end])
}

// Validate 按给定意图校验 SQL。
// 返回 nil 表示通过；非 nil 表示违反策略。
func Validate(sql string, intent Intent) error {
	if err := basicCheck(sql); err != nil {
		return err
	}

	kw := FirstKeyword(sql)
	if kw == "" {
		return ErrEmptySQL
	}

	if isAlwaysDenied(kw) {
		return ErrSQLNotAllowed
	}
	if (kw == "CREATE" || kw == "DROP") && isDeniedSchemaObject(sql) {
		return ErrSQLNotAllowed
	}

	switch intent {
	case ReadOnly:
		if !isReadOnlyKeyword(kw) {
			return ErrWriteInReadOnly
		}
	case WriteAllowed:
		// DML/DDL 允许；但仍拒绝 isAlwaysDenied
	}
	return nil
}

// basicCheck 执行与意图无关的基础校验。
func basicCheck(sql string) error {
	if sql == "" {
		return ErrEmptySQL
	}
	if strings.ContainsRune(sql, 0) {
		return ErrNulChar
	}
	if hasMultipleStatements(sql) {
		return ErrMultipleStatements
	}
	return nil
}

// hasMultipleStatements 检测分号后是否还有非注释内容。
// 末尾单个分号不算多语句。
func hasMultipleStatements(sql string) bool {
	// 逐字符扫描，跳过字符串字面量和注释，遇到分号后检查剩余是否还有有效 token
	i := 0
	n := len(sql)
	for i < n {
		c := sql[i]
		switch c {
		case '\'':
			// 字符串字面量：'' 为转义
			i++
			for i < n {
				if sql[i] == '\'' {
					if i+1 < n && sql[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case '"':
			// 标识符字面量："" 为转义
			i++
			for i < n {
				if sql[i] == '"' {
					if i+1 < n && sql[i+1] == '"' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case '-':
			if i+1 < n && sql[i+1] == '-' {
				// 行注释
				i += 2
				for i < n && sql[i] != '\n' {
					i++
				}
			} else {
				i++
			}
		case '/':
			if i+1 < n && sql[i+1] == '*' {
				// 块注释
				i += 2
				for i+1 < n && !(sql[i] == '*' && sql[i+1] == '/') {
					i++
				}
				if i+1 < n {
					i += 2
				}
			} else {
				i++
			}
		case ';':
			// 检查分号后是否还有有效内容
			rest := stripLeadingNoise(sql[i+1:])
			if rest != "" {
				return true
			}
			return false
		default:
			i++
		}
	}
	return false
}

// stripLeadingNoise 去除前导空白与 SQL 注释（行注释和块注释）。
func stripLeadingNoise(s string) string {
	i := 0
	n := len(s)
	for i < n {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < n && s[i+1] == '-':
			// 行注释
			i += 2
			for i < n && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && s[i+1] == '*':
			// 块注释
			i += 2
			for i+1 < n && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			}
		default:
			return s[i:]
		}
	}
	return ""
}

// firstTokenEnd 返回第一个 token 的结束位置。
func firstTokenEnd(s string) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
			c == '(' || c == ')' || c == ';' || c == ',' {
			return i
		}
	}
	return len(s)
}

// isReadOnlyKeyword 判断首关键字是否属于只读集合。
func isReadOnlyKeyword(kw string) bool {
	switch kw {
	case "SELECT", "WITH", "EXPLAIN", "VALUES", "DESCRIBE", "SHOW", "FROM", "SUMMARIZE", "PIVOT", "UNPIVOT":
		return true
	}
	return false
}

// isAlwaysDenied 判断关键字是否永久拒绝（不论意图）。
// 管理面指令由后端内部通道执行，不走用户 SQL API（planv2.0 §4.5）。
func isAlwaysDenied(kw string) bool {
	switch kw {
	case "ATTACH", "DETACH", "INSTALL", "LOAD", "LOAD_EXTENSION",
		"COPY", "EXPORT", "IMPORT", "SET", "PRAGMA", "CHECKPOINT", "CALL",
		"USE", "VACUUM", "PREPARE", "EXECUTE", "DEALLOCATE":
		return true
	}
	return false
}

// isDeniedSchemaObject 拦截 CREATE/DROP SECRET 与 CREATE/DROP MACRO。
func isDeniedSchemaObject(sql string) bool {
	switch schemaObjectKind(sql) {
	case "SECRET", "MACRO":
		return true
	}
	return false
}

func schemaObjectKind(sql string) string {
	s := stripLeadingNoise(sql)
	end := firstTokenEnd(s)
	rest := stripLeadingNoise(s[end:])
	for rest != "" {
		tokEnd := firstTokenEnd(rest)
		tok := strings.ToUpper(rest[:tokEnd])
		switch tok {
		case "OR", "REPLACE", "TEMP", "TEMPORARY", "UNIQUE", "RECURSIVE", "IF", "NOT", "EXISTS":
			rest = stripLeadingNoise(rest[tokEnd:])
			continue
		default:
			return tok
		}
	}
	return ""
}
