// 文件说明：schema.go — godatabase/kv 中 EnsureSchema 等符号的实现
//
// 用法
//
//	EnsureSchema(ctx context.Context, db *sql.DB) error  // EnsureSchema 幂等初始化 KV 保留表与索引
//	同包调用：ensureSchemaLocked() → executor.go
//
// 调用架构
//
//	[上游] internal/api, internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/kv/schema.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] core.go, executor.go, hash.go, key.go, list.go 等 10 个
//	[关键引用] ensureSchemaLocked() ← executor.go
//
// 主要符号
//
//	EnsureSchema(ctx context.Context, db *sql.DB) error
//	... 另有 2 个顶层符号

package kv

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaDDL 是 kv 保留表的幂等建表语句。
// 表名统一使用 __sb_ 前缀：SQL、集合与 KV 共享同一个项目数据库文件，
// 内部表不得出现在 Schema API 或用户可见的集合列表中。
// 列名 "key" 始终加引号；一致性由应用层事务保证。
var schemaDDL = []string{
	// key 元数据：type 见 TypeID；etime 过期时间 unix 毫秒（NULL 永久）；
	// len 元素计数（string 为 NULL）；version 每次写 +1（应用层维护）。
	`CREATE TABLE IF NOT EXISTS __sb_kv_keys (
		"key"   VARCHAR NOT NULL,
		"type"  SMALLINT NOT NULL,
		version BIGINT NOT NULL,
		etime   BIGINT,
		mtime   BIGINT NOT NULL,
		len     BIGINT
	)`,
	`CREATE TABLE IF NOT EXISTS __sb_kv_strings (
		"key" VARCHAR NOT NULL,
		value BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS __sb_kv_lists (
		"key" VARCHAR NOT NULL,
		pos   DOUBLE NOT NULL,
		elem  BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS __sb_kv_sets (
		"key" VARCHAR NOT NULL,
		elem  BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS __sb_kv_hashes (
		"key" VARCHAR NOT NULL,
		field VARCHAR NOT NULL,
		value BLOB NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS __sb_kv_zsets (
		"key"  VARCHAR NOT NULL,
		elem   BLOB NOT NULL,
		score  DOUBLE NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS __sb_kv_keys_key ON __sb_kv_keys ("key")`,
	`CREATE INDEX IF NOT EXISTS __sb_kv_strings_key ON __sb_kv_strings ("key")`,
	`CREATE INDEX IF NOT EXISTS __sb_kv_lists_key ON __sb_kv_lists ("key", pos)`,
	`CREATE INDEX IF NOT EXISTS __sb_kv_sets_key ON __sb_kv_sets ("key")`,
	`CREATE INDEX IF NOT EXISTS __sb_kv_hashes_key ON __sb_kv_hashes ("key", field)`,
	`CREATE INDEX IF NOT EXISTS __sb_kv_zsets_key ON __sb_kv_zsets ("key")`,
}

// EnsureSchema 幂等初始化 KV 保留表与索引。
// 调用时机（key-value-ducklake-plan §3.3）：
//   - 数据库创建成功后由 api.KVInitializer 立即初始化（正常路径）；
//   - 历史库首次 KV 写时由 Store.ensureSchema 兜底补建。
//
// db 必须是经 Registry 获取的项目文件连接，不得绕过 Registry 直连 DSN。
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
