// 文件说明：validator.go — godatabase/sqlguard 中 ErrEmptySQL 等符号的实现
//
// 用法
//
//	var ErrEmptySQL / ErrMultipleStatements / ErrNulChar / ErrSQLNotAllowed / ErrWriteInReadOnly
//
// 调用架构
//
//	[上游] godatabase/cmd, godatabase/ducklake, internal/agentbridge, internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/sqlguard/validator.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] classifier.go, classifier_test.go, reserved_prefix_test.go, split_test.go
//
// 主要符号
//
//	var ErrEmptySQL / ErrMultipleStatements / ErrNulChar / ErrSQLNotAllowed / ErrWriteInReadOnly

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
