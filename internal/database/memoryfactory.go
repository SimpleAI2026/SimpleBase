// memoryfactory.go 提供 DevMode 下的 database.Factory 实现：每个数据库
// 打开一个独立的 :memory: SQLite 连接。仅用于本地开发，不持久化。
package database

import (
	"context"
	"database/sql"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// MemoryFactory 是 Factory 的 DevMode 实现。
// 每次打开创建一个独立的 :memory: SQLite 库；数据不持久化，进程重启即丢失。
type MemoryFactory struct {
	Logger  observability.Logger
	Metrics *observability.Metrics
}

// Open 打开一个 :memory: SQLite 数据库。
// 同一 databaseID 的多次 Open 会创建独立的内存库（不共享），适合 DevMode 隔离测试。
func (f *MemoryFactory) Open(ctx context.Context, db catalog.Database, mode AccessMode) (*sql.DB, error) {
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return sqlDB, nil
}
