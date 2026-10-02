package kv

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaDDL 是 kv schema 的幂等建表语句。
// 注意：表名刻意用复数（keys/strings/lists/sets/hashes/zsets），
// 避开 set 等 DuckDB 语句关键字；列名 "key" 始终加引号。
// DuckLake 不支持主键/唯一索引/外键/触发器，一致性由应用层事务保证。
var schemaDDL = []string{
	`CREATE SCHEMA IF NOT EXISTS kv`,
	// key 元数据：type 见 TypeID；etime 过期时间 unix 毫秒（NULL 永久）；
	// len 元素计数（string 为 NULL）；version 每次写 +1（应用层维护）。
	`CREATE TABLE IF NOT EXISTS kv.keys (
		"key"   VARCHAR NOT NULL,
		"type"  SMALLINT NOT NULL,
		version BIGINT NOT NULL,
		etime   BIGINT,
		mtime   BIGINT NOT NULL,
		len     BIGINT
	)`,
	`CREATE TABLE IF NOT EXISTS kv.strings (
		"key" VARCHAR NOT NULL,
		value BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS kv.lists (
		"key" VARCHAR NOT NULL,
		pos   DOUBLE NOT NULL,
		elem  BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS kv.sets (
		"key" VARCHAR NOT NULL,
		elem  BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS kv.hashes (
		"key" VARCHAR NOT NULL,
		field VARCHAR NOT NULL,
		value BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS kv.zsets (
		"key"  VARCHAR NOT NULL,
		elem   BLOB NOT NULL,
		score  DOUBLE NOT NULL
	)`,
}

// EnsureSchema 幂等初始化 kv 系统表（CREATE SCHEMA + 6 张表）。
// 调用时机（key-value-ducklake-plan §3.3）：
//   - 数据库创建成功后由 api.KVInitializer 立即初始化（正常路径）；
//   - 历史库首次 KV 写时由 Store.ensureSchema 兜底补建。
// db 必须是经 Registry 获取的 DuckLake 连接，不得绕过 Registry 直连 DSN。
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	return ensureSchemaLocked(ctx, db)
}

// ensureSchemaLocked 顺序执行幂等 DDL。单连接池（SetMaxOpenConns(1)）
// 保证不会并发执行；失败时不标记 schemaKnown，下次写操作重试。
func ensureSchemaLocked(ctx context.Context, db *sql.DB) error {
	for _, ddl := range schemaDDL {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("kv: ensure schema: %w", err)
		}
	}
	return nil
}
