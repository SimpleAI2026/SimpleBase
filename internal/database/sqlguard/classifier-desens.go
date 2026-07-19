// Package sqlguard 提供低成本的 SQL 意图分类与策略校验。
//
// 它不是 SQL 防火墙，也不做完整的语法解析。只执行可测试、低延迟的规则：
//   - 去除前导空白与 SQL 注释后判断首关键字
//   - 拒绝空 SQL、多语句、NUL 字符
//   - 按意图（只读 / 写允许）过滤首关键字
//   - 永久拒绝危险关键字：ATTACH/DETACH/LOAD_EXTENSION/部分 PRAGMA
//
// 若无法可靠判断，保守拒绝并返回 ErrSQLNotAllowed。
package sqlguard

import (
	"strings"
)

// Intent 表示调用方期望的 SQL 意图。
type Intent uint8

const (
	// ReadOnly 仅允许 SELECT/WITH/EXPLAIN/受控 PRAGMA。
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

	// PRAGMA 只允许白名单内的安全查询项
	if kw == "PRAGMA" {
		if !isAllowedPragma(sql) {
			return ErrSQLNotAllowed
		}
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
// PRAGMA 经 isAllowedPragma 白名单过滤后视为只读。
func isReadOnlyKeyword(kw string) bool {
	switch kw {
	case "SELECT", "WITH", "EXPLAIN", "VALUES", "PRAGMA":
		return true
	}
	return false
}

// isAlwaysDenied 判断关键字是否永久拒绝（不论意图）。
func isAlwaysDenied(kw string) bool {
	switch kw {
	case "ATTACH", "DETACH", "LOAD_EXTENSION":
		return true
	}
	return false
}

// isAllowedPragma 判断 PRAGMA 是否在安全白名单内。
// 允许只读信息查询；拒绝 writable_schema/load_extension 等危险项。
func isAllowedPragma(sql string) bool {
	// 提取 PRAGMA 后的标识符
	s := stripLeadingNoise(sql)
	// 去掉 "PRAGMA" 前缀（已由 FirstKeyword 确认大小写无关）
	if len(s) < 6 {
		return false
	}
	rest := strings.TrimSpace(s[6:])
	if rest == "" {
		return false
	}
	// 取标识符部分（到 = 或 ( 或空白为止）
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c == '=' || c == '(' || c == ' ' || c == '\t' || c == ';' {
			end = i
			break
		}
	}
	name := strings.ToUpper(rest[:end])

	switch name {
	case "WRITABLE_SCHEMA", "LOAD_EXTENSION":
		return false
	}
	// 允许其余信息查询型 PRAGMA（如 database_list, integrity_check, table_info 等）
	return true
}
