// 文件说明：validator.go — 定义 sqlguard 的错误类型
//
// 用法
//
//	同包调用：ErrEmptySQL → classifier-desens.go；ErrMultipleStatements → classifier-desens.go；ErrNulChar → classifier-desens.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/sqlguard/validator.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] classifier-desens.go
//	[关键引用] ErrEmptySQL ← classifier-desens.go
//
// 主要符号
//
//	var ErrEmptySQL
//	var ErrMultipleStatements
//	var ErrNulChar
//	var ErrSQLNotAllowed
//	var ErrWriteInReadOnly
//	validator.go 定义 sqlguard 的错误类型。

package sqlguard

import "errors"

var (
	// ErrEmptySQL 表示 SQL 为空或仅含注释。
	ErrEmptySQL = errors.New("sqlguard: empty sql")
	// ErrMultipleStatements 表示 SQL 包含多条语句（分号后还有非注释内容）。
	ErrMultipleStatements = errors.New("sqlguard: multiple statements are not allowed")
	// ErrNulChar 表示 SQL 包含 NUL 字符。
	ErrNulChar = errors.New("sqlguard: nul character not allowed in sql")
	// ErrSQLNotAllowed 表示 SQL 的首关键字被永久拒绝或无法可靠判断。
	ErrSQLNotAllowed = errors.New("sqlguard: sql statement not allowed")
	// ErrWriteInReadOnly 表示只读意图下使用了非只读关键字。
	ErrWriteInReadOnly = errors.New("sqlguard: write statement not allowed in read-only intent")
)
