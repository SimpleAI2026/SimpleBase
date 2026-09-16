package catalog

import "database/sql"

// NewSQLiteRepository 是 NewSQLRepository 的别名（历史调用点兼容）。
func NewSQLiteRepository(db *sql.DB) Repository {
	return NewSQLRepository(db)
}
