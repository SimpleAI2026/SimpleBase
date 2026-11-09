// 文件说明：classifier-desens.go — 低成本的 SQL 意图分类与策略校验
//
// 用法
//
//	FirstKeyword(sql string) string  // FirstKeyword 去除前导空白与 SQL 注释后返回首关键字（大写）
//	Validate(sql string, intent Intent) error  // Validate 按给定意图校验 SQL
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/sqlguard/classifier-desens.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/sqlx
//	[同包] validator.go
//
// 主要符号
//
//	type Intent uint8
//	const ReadOnly
//	FirstKeyword(sql string) string
//	Validate(sql string, intent Intent) error
//	SplitStatements(sql string) ([]string, error)
//	... 另有 11 个顶层符号

package sqlguard

import (
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase/sqlx"
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
	if hasDuckLakeMetadataIdentifier(sql) {
		return ErrSQLNotAllowed
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
		if err := validateReadOnlyBody(sql, kw); err != nil {
			return err
		}
	case WriteAllowed:
		// DML/DDL 允许；但仍拒绝 isAlwaysDenied
	}
	return nil
}

// validateReadOnlyBody 对首关键字放行的语句做纵深检查：
// WITH … 的主语句、EXPLAIN … 的被解释语句仍必须是只读的。
// 只用词法扫描（复用 firstTokenEnd 的停字符），不做子串匹配。
func validateReadOnlyBody(sql, kw string) error {
	s := stripLeadingNoise(sql)
	switch kw {
	case "WITH":
		if err := checkCTEMainStatement(s); err != nil {
			return err
		}
	case "EXPLAIN":
		if err := checkExplainSubject(s); err != nil {
			return err
		}
	}
	return nil
}

// checkCTEMainStatement 在跳过全部 CTE 定义后定位主语句关键字。
// 词法规则：正确跳过字符串、注释、引号标识符；括号外的结构是
// WITH [RECURSIVE] name [(cols)] AS [NOT] [MATERIALIZED] (subquery)
//   [, name [(cols)] AS …]*
// <主语句>
// 每个 CTE 定义从「读到 AS」开始、到「其后括号配对结束」为止；
// 括号配对结束后的下一个标识符，若非上下文关键字即主语句开头。
func checkCTEMainStatement(s string) error {
	i, n := 0, len(s)
	depth := 0
	sawAS := false // 已读到当前 CTE 的 AS，其后的 '(' 即定义体
	for i < n {
		c := s[i]
		switch {
		case c == '\'':
			i = skipString(s, i)
		case c == '-' && i+1 < n && s[i+1] == '-':
			i = skipLineComment(s, i)
		case c == '/' && i+1 < n && s[i+1] == '*':
			i = skipBlockComment(s, i)
		case c == '"':
			i = skipQuotedIdent(s, i)
		case c == '(':
			depth++
			i++
		case c == ')':
			depth--
			i++
		case c == ',' && depth == 0:
			sawAS = false // 下一个 CTE 名开始
			i++
		case c == ';' && depth == 0:
			return nil // 多语句场景由 basicCheck 拦截，这里防御性放行
		case isSpace(c):
			i++
		case isIdentStart(c):
			end := identEnd(s, i)
			kw := strings.ToUpper(s[i:end])
			if depth > 0 {
				i = end
				continue
			}
			switch kw {
			case "WITH", "RECURSIVE":
				i = end
			case "AS", "NOT", "MATERIALIZED":
				sawAS = true
				i = end
			default:
				if !sawAS {
					// 尚未读到 AS：这是 CTE 名（或列名表前的名字），跳过
					i = end
					continue
				}
				if !isReadOnlyKeyword(kw) || isAlwaysDenied(kw) {
					return ErrWriteInReadOnly
				}
				return nil
			}
		default:
			i++
		}
	}
	return nil
}

// checkExplainSubject 拒绝 EXPLAIN ANALYZE（会真实执行被解释语句）。
// 仅 EXPLAIN（不带 ANALYZE）才是纯只读；EXPLAIN (ANALYZE …) 的
// 括号选项形式同样拒绝（防御）。
func checkExplainSubject(s string) error {
	i, n := 0, len(s)
	// 跳过 EXPLAIN
	for i < n && !isIdentStart(s[i]) {
		i++
	}
	i = skipIdent(s, i)
	// 逐 token 检查 EXPLAIN 选项；遇到 '(' 时逐标识符扫描其内部
	for {
		j := i
		for j < n && isSpace(s[j]) {
			j++
		}
		if j >= n {
			return nil
		}
		if s[j] == '(' {
			k := j + 1
			for k < n && s[k] != ')' {
				if isIdentStart(s[k]) {
					end := identEnd(s, k)
					if strings.ToUpper(s[k:end]) == "ANALYZE" {
						return ErrWriteInReadOnly
					}
					k = end
					continue
				}
				k++
			}
			i = k + 1
			continue
		}
		if !isIdentStart(s[j]) {
			return nil
		}
		end := identEnd(s, j)
		kw := strings.ToUpper(s[j:end])
		switch kw {
		case "ANALYZE":
			return ErrWriteInReadOnly
		case "FORMAT", "PLAIN", "OPTIMIZE", "VERBOSE", "COSTS", "PLAN", "ESTIMATED":
			i = end
			// FORMAT 之后跟类型名，跳过一个 token
			if kw == "FORMAT" {
				k := end
				for k < n && isSpace(s[k]) {
					k++
				}
				if k < n && isIdentStart(s[k]) {
					i = identEnd(s, k)
				}
			}
		default:
			return nil
		}
	}
}

// reservedIdentifierPrefixes 是用户 SQL 不得引用的标识符前缀：
// DuckLake 元数据表、SimpleBase 内部表（KV 等）、SQLite 内部表。
var reservedIdentifierPrefixes = []string{"__ducklake_metadata_", "__sb_", "sqlite_"}

func hasReservedPrefix(identifier string) bool {
	lower := strings.ToLower(identifier)
	for _, p := range reservedIdentifierPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func hasDuckLakeMetadataIdentifier(sql string) bool {
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
			i++
			var identifier strings.Builder
			for i < len(sql) {
				if sql[i] == '"' {
					i++
					if i < len(sql) && sql[i] == '"' {
						identifier.WriteByte('"')
						i++
						continue
					}
					break
				}
				identifier.WriteByte(sql[i])
				i++
			}
			if hasReservedPrefix(identifier.String()) {
				return true
			}
		case sqlx.IsIdentByte(sql[i]):
			start := i
			for i < len(sql) && sqlx.IsIdentByte(sql[i]) {
				i++
			}
			if hasReservedPrefix(sql[start:i]) {
				return true
			}
		default:
			i++
		}
	}
	return false
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
	parts, err := SplitStatements(sql)
	if err != nil {
		return false
	}
	n := 0
	for _, part := range parts {
		if FirstKeyword(part) == "" {
			continue
		}
		n++
		if n > 1 {
			return true
		}
	}
	return false
}

// SplitStatements 按分号拆成语句。字符串、标识符和注释里的分号不作为分隔符。
// 空字符串与纯空白返回 nil。含 NUL 时返回 ErrNulChar。
func SplitStatements(sql string) ([]string, error) {
	if strings.ContainsRune(sql, 0) {
		return nil, ErrNulChar
	}
	var raw []string
	i := 0
	start := 0
	n := len(sql)
	for i < n {
		switch sql[i] {
		case '\'':
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
				i += 2
				for i < n && sql[i] != '\n' {
					i++
				}
			} else {
				i++
			}
		case '/':
			if i+1 < n && sql[i+1] == '*' {
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
			raw = append(raw, sql[start:i])
			i++
			start = i
		default:
			i++
		}
	}
	raw = append(raw, sql[start:])
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out, nil
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

// —— 只读纵深检查的词法辅助（与 hasDuckLakeMetadataIdentifier 的扫描规则一致）——

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isIdentStart(c byte) bool {
	return sqlx.IsIdentByte(c)
}

func identEnd(s string, i int) int {
	for i < len(s) && sqlx.IsIdentByte(s[i]) {
		i++
	}
	return i
}

func skipIdent(s string, i int) int {
	return identEnd(s, i)
}

func skipString(s string, i int) int {
	// s[i] == '\''；支持 '' 转义
	i++
	for i < len(s) {
		if s[i] == '\'' {
			i++
			if i < len(s) && s[i] == '\'' {
				i++
				continue
			}
			return i
		}
		i++
	}
	return len(s)
}

func skipLineComment(s string, i int) int {
	for i < len(s) && s[i] != '\n' {
		i++
	}
	return i
}

func skipBlockComment(s string, i int) int {
	i += 2
	for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
		i++
	}
	if i+1 < len(s) {
		i += 2
	}
	return i
}

func skipQuotedIdent(s string, i int) int {
	i++
	for i < len(s) {
		if s[i] == '"' {
			i++
			if i < len(s) && s[i] == '"' {
				i++
				continue
			}
			return i
		}
		i++
	}
	return len(s)
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
