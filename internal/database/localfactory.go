// localfactory.go 提供 DevMode 下的 database.Factory 持久化实现：
// 每个数据库打开一个独立的本地磁盘 SQLite 文件，进程重启后数据保留。
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// LocalFactory 是 Factory 的 DevMode 持久化实现。
// 数据库文件落在 Dir 目录下，以 databaseID（UUID）命名，重启后复用同一文件。
type LocalFactory struct {
	Dir     string // 用户库文件目录（由 app 装配层注入，通常位于 cache_dir 下）
	Logger  observability.Logger
	Metrics *observability.Metrics
}

// Open 打开 Dir 下以 databaseID 命名的 SQLite 文件；不存在时自动创建。
// databaseID 由 catalog 保证为 UUID，此处再次校验防止路径穿越。
func (f *LocalFactory) Open(ctx context.Context, db catalog.Database, mode AccessMode) (*sql.DB, error) {
	if _, err := uuid.Parse(db.ID); err != nil {
		return nil, fmt.Errorf("local factory: invalid database id: %w", err)
	}
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("local factory: create dir %s: %w", f.Dir, err)
	}
	path := filepath.Join(f.Dir, db.ID+".db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000", path)
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	// registry 已保证单写，限制单连接避免 SQLite 文件锁竞争。
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return sqlDB, nil
}
