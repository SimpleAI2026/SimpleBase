package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// migration 定义一条只前进的迁移。
type migration struct {
	version int
	name    string
	stmt    string
}

// migrations 是按版本号升序的迁移列表。
// 每条 migration 在独立事务中执行并记录版本；失败则整体回滚，server 不 ready。
var migrations = []migration{
	{
		version: 1,
		name:    "create_tenants",
		stmt: `CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);`,
	},
	{
		version: 2,
		name:    "create_projects",
		stmt: `CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			UNIQUE(tenant_id, name)
		);`,
	},
	{
		version: 3,
		name:    "create_databases",
		stmt: `CREATE TABLE IF NOT EXISTS databases (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			project_id TEXT NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			storage_prefix TEXT NOT NULL,
			format_version INTEGER NOT NULL,
			deleted_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_databases_project_name
			ON databases(project_id, name) WHERE deleted_at IS NULL;
		CREATE INDEX IF NOT EXISTS idx_databases_project_status
			ON databases(project_id, status);`,
	},
	{
		version: 4,
		name:    "create_api_keys",
		stmt: `CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			key_hash TEXT NOT NULL,
			permissions TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			revoked_at DATETIME
		);`,
	},
	{
		version: 5,
		name:    "create_llm_provider_configs",
		stmt: `CREATE TABLE IF NOT EXISTS llm_provider_configs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			credential_ref TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			allowed_models_json TEXT NOT NULL,
			default_model TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(project_id, provider)
		);`,
	},
	{
		version: 6,
		name:    "create_usage_events",
		stmt: `CREATE TABLE IF NOT EXISTS usage_events (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			cost_micros INTEGER NOT NULL,
			request_id TEXT NOT NULL,
			occurred_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_usage_events_project_time
			ON usage_events(project_id, occurred_at);`,
	},
	{
		version: 7,
		name:    "create_operations",
		stmt: `CREATE TABLE IF NOT EXISTS operations (
			id TEXT PRIMARY KEY,
			database_id TEXT NOT NULL,
			project_id TEXT NOT NULL,
			principal_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			request_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_operations_database_time
			ON operations(database_id, created_at);`,
	},
	{
		version: 8,
		name:    "create_jobs",
		stmt: `CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			operation_id TEXT NOT NULL,
			database_id TEXT NOT NULL,
			project_id TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			attempt INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 5,
			run_after DATETIME NOT NULL,
			payload_json TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_jobs_status_runafter
			ON jobs(status, run_after);
		CREATE INDEX IF NOT EXISTS idx_jobs_database_type
			ON jobs(database_id, type);`,
	},
	{
		version: 9,
		name:    "create_project_quotas",
		stmt: `CREATE TABLE IF NOT EXISTS project_quotas (
			project_id TEXT PRIMARY KEY,
			max_databases INTEGER NOT NULL DEFAULT 0,
			max_storage_bytes INTEGER NOT NULL DEFAULT 0,
			max_llm_requests INTEGER NOT NULL DEFAULT 0,
			max_llm_tokens INTEGER NOT NULL DEFAULT 0,
			period_seconds INTEGER NOT NULL DEFAULT 3600,
			updated_at DATETIME NOT NULL
		);`,
	},
}

// ApplyMigrations 在给定 db 上按顺序执行所有未应用的迁移。
// 每条迁移在独立事务中执行并记录版本到 migration_versions 表。
// 任一失败返回 ErrMigrationFailed（含原因），server 不 ready。
//
// 可重复执行：已应用的迁移跳过。
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("%w: db is nil", ErrMigrationFailed)
	}
	// 确保 migration_versions 表存在
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS migration_versions (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("%w: create migration_versions: %v", ErrMigrationFailed, err)
	}

	for _, m := range migrations {
		applied, err := isMigrationApplied(ctx, db, m.version)
		if err != nil {
			return fmt.Errorf("%w: check version %d: %v", ErrMigrationFailed, m.version, err)
		}
		if applied {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return fmt.Errorf("%w: apply %s (v%d): %v", ErrMigrationFailed, m.name, m.version, err)
		}
	}
	return nil
}

func isMigrationApplied(ctx context.Context, db *sql.DB, version int) (bool, error) {
	var v int
	err := db.QueryRowContext(ctx,
		"SELECT version FROM migration_versions WHERE version = ?", version).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// applyOne 在单事务中执行 DDL 并记录版本。
// 注意：SQLite 不允许在事务内执行多条 DDL（部分驱动限制），因此对含多语句的
// migration 按分号拆分逐条执行，全部成功后再记录版本。
func applyOne(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmts := splitStatements(m.stmt)
	for _, s := range stmts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO migration_versions(version, name) VALUES(?, ?)",
		m.version, m.name); err != nil {
		return err
	}
	return tx.Commit()
}

// splitStatements 按分号拆分 SQL，保留 CREATE INDEX 等完整语句。
// 不处理字符串内的分号（catalog migration 不含数据，纯 DDL）。
func splitStatements(s string) []string {
	out := make([]string, 0, 4)
	for _, part := range strings.Split(s, ";") {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
