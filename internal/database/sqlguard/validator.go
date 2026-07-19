// validator.go 定义 sqlguard 的错误类型。
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
