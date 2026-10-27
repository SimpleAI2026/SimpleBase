// 文件说明：sqlx.go — SQL 标识符与字符串字面量的转义构造
//
// 用法
//
//	Ident(name string) string  // Ident 把 name 写成双引号标识符，内部的 " 双写转义
//	Literal(s string) string  // Literal 把 s 写成单引号字符串字面量，内部的 ' 双写转义
//
// 调用架构
//
//	[上游] godatabase/ducklake, godatabase/filedb, godatabase/sqlguard 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/sqlx/sqlx.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//
// 主要符号
//
//	Ident(name string) string
//	Literal(s string) string
//	IsIdentByte(c byte) bool
//	IsSafeIdent(name string) bool

package sqlx

import (
	"strings"
	"unicode"
)

// Ident 把 name 写成双引号标识符，内部的 " 双写转义。
//
// Ident("") 返回 `""`（空标识符），调用方须自行拒绝空名字。
func Ident(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Literal 把 s 写成单引号字符串字面量，内部的 ' 双写转义。
func Literal(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// IsIdentByte 判断 c 是否为 ASCII 标识符字节 [A-Za-z0-9_]。
// 用于 SQL 词法扫描（逐字节），不用于名字合法性判断。
func IsIdentByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// IsSafeIdent 判断 name 是否可作为不加引号的标识符：
// 首字符为 Unicode 字母或下划线，其后为字母、数字或下划线。
func IsSafeIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}
