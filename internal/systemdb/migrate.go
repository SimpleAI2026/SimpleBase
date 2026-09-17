package systemdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// migration 是一条只前进的系统库 DDL。
type migration struct {
	version int
	name    string
	stmt    string
}

// systemMigrations 覆盖 plan §5 全部 sys_* 表。
// DuckLake 不使用 PRIMARY KEY / AUTOINCREMENT；唯一性由应用层保证。
var systemMigrations = []migration{
	{
		version: 1,
		name:    "sys_tenants",
		stmt: `CREATE TABLE IF NOT EXISTS sys_tenants (
			id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 2,
		name:    "sys_projects",
		stmt: `CREATE TABLE IF NOT EXISTS sys_projects (
			id VARCHAR NOT NULL,
			tenant_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 3,
		name:    "sys_databases",
		stmt: `CREATE TABLE IF NOT EXISTS sys_databases (
			id VARCHAR NOT NULL,
			tenant_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			kind VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			storage_prefix VARCHAR NOT NULL,
			format_version BIGINT NOT NULL,
			deleted_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 4,
		name:    "sys_api_keys",
		stmt: `CREATE TABLE IF NOT EXISTS sys_api_keys (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			key_hash VARCHAR NOT NULL,
			permissions VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			revoked_at TIMESTAMP
		)`,
	},
	{
		version: 5,
		name:    "sys_llm_provider_configs",
		stmt: `CREATE TABLE IF NOT EXISTS sys_llm_provider_configs (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			provider VARCHAR NOT NULL,
			credential_ref VARCHAR NOT NULL,
			enabled BIGINT NOT NULL,
			allowed_models_json VARCHAR NOT NULL,
			default_model VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 6,
		name:    "sys_usage_events",
		stmt: `CREATE TABLE IF NOT EXISTS sys_usage_events (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			kind VARCHAR NOT NULL,
			provider VARCHAR NOT NULL,
			model VARCHAR NOT NULL,
			input_tokens BIGINT NOT NULL,
			output_tokens BIGINT NOT NULL,
			cost_micros BIGINT NOT NULL,
			request_id VARCHAR NOT NULL,
			occurred_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 7,
		name:    "sys_operations",
		stmt: `CREATE TABLE IF NOT EXISTS sys_operations (
			id VARCHAR NOT NULL,
			database_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			principal_id VARCHAR NOT NULL,
			kind VARCHAR NOT NULL,
			request_id VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 8,
		name:    "sys_jobs",
		stmt: `CREATE TABLE IF NOT EXISTS sys_jobs (
			id VARCHAR NOT NULL,
			operation_id VARCHAR NOT NULL,
			database_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			type VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			attempt BIGINT NOT NULL,
			max_attempts BIGINT NOT NULL,
			run_after TIMESTAMP NOT NULL,
			payload_json VARCHAR NOT NULL,
			last_error VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 9,
		name:    "sys_project_quotas",
		stmt: `CREATE TABLE IF NOT EXISTS sys_project_quotas (
			project_id VARCHAR NOT NULL,
			max_databases BIGINT NOT NULL,
			max_storage_bytes BIGINT NOT NULL,
			max_llm_requests BIGINT NOT NULL,
			max_llm_tokens BIGINT NOT NULL,
			period_seconds BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 10,
		name:    "sys_metric_samples",
		stmt: `CREATE TABLE IF NOT EXISTS sys_metric_samples (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			value_double DOUBLE NOT NULL,
			labels_json VARCHAR NOT NULL,
			occurred_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 11,
		name:    "sys_metric_rollups_hourly",
		stmt: `CREATE TABLE IF NOT EXISTS sys_metric_rollups_hourly (
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			bucket_start TIMESTAMP NOT NULL,
			count BIGINT NOT NULL,
			sum DOUBLE NOT NULL,
			min DOUBLE NOT NULL,
			max DOUBLE NOT NULL
		)`,
	},
	{
		version: 12,
		name:    "sys_s3_objects",
		stmt: `CREATE TABLE IF NOT EXISTS sys_s3_objects (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			object_key VARCHAR NOT NULL,
			size BIGINT NOT NULL,
			etag VARCHAR NOT NULL,
			content_type VARCHAR NOT NULL,
			last_modified TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			deleted_at TIMESTAMP
		)`,
	},
	{
		version: 13,
		name:    "sys_s3_sync_runs",
		stmt: `CREATE TABLE IF NOT EXISTS sys_s3_sync_runs (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			listed BIGINT NOT NULL,
			inserted BIGINT NOT NULL,
			removed BIGINT NOT NULL,
			error VARCHAR NOT NULL,
			started_at TIMESTAMP NOT NULL,
			finished_at TIMESTAMP
		)`,
	},
	{
		version: 14,
		name:    "sys_llm_sessions",
		stmt: `CREATE TABLE IF NOT EXISTS sys_llm_sessions (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			title VARCHAR NOT NULL,
			provider VARCHAR NOT NULL,
			model VARCHAR NOT NULL,
			created_by VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`,
	},
	{
		version: 15,
		name:    "sys_llm_messages",
		stmt: `CREATE TABLE IF NOT EXISTS sys_llm_messages (
			id VARCHAR NOT NULL,
			session_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			role VARCHAR NOT NULL,
			content VARCHAR NOT NULL,
			token_input BIGINT NOT NULL,
			token_output BIGINT NOT NULL,
			request_id VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 16,
		name:    "sys_llm_settings",
		stmt: `CREATE TABLE IF NOT EXISTS sys_llm_settings (
			project_id VARCHAR NOT NULL,
			default_provider VARCHAR NOT NULL,
			default_model VARCHAR NOT NULL,
			temperature DOUBLE NOT NULL,
			max_tokens BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 17,
		name:    "sys_settings_global",
		stmt: `CREATE TABLE IF NOT EXISTS sys_settings_global (
			key VARCHAR NOT NULL,
			value_json VARCHAR NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 18,
		name:    "sys_settings_project",
		stmt: `CREATE TABLE IF NOT EXISTS sys_settings_project (
			project_id VARCHAR NOT NULL,
			key VARCHAR NOT NULL,
			value_json VARCHAR NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 19,
		name:    "sys_log_events",
		stmt: `CREATE TABLE IF NOT EXISTS sys_log_events (
			id VARCHAR NOT NULL,
			project_id VARCHAR,
			level VARCHAR NOT NULL,
			logger VARCHAR NOT NULL,
			message VARCHAR NOT NULL,
			fields_json VARCHAR NOT NULL,
			request_id VARCHAR NOT NULL,
			occurred_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 20,
		name:    "sys_log_retention",
		stmt: `CREATE TABLE IF NOT EXISTS sys_log_retention (
			scope VARCHAR NOT NULL,
			project_id VARCHAR,
			keep_days BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 21,
		name:    "sys_log_exports",
		stmt: `CREATE TABLE IF NOT EXISTS sys_log_exports (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			filter_json VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			finished_at TIMESTAMP
		)`,
	},
	{
		version: 22,
		name:    "sys_cloud_agents",
		stmt: `CREATE TABLE IF NOT EXISTS sys_cloud_agents (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			module VARCHAR NOT NULL,
			description VARCHAR NOT NULL,
			system_prompt VARCHAR NOT NULL,
			tool_ids VARCHAR NOT NULL,
			model_override VARCHAR NOT NULL,
			team_enabled BIGINT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`,
	},
	{
		version: 23,
		name:    "sys_agent_threads",
		stmt: `CREATE TABLE IF NOT EXISTS sys_agent_threads (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			title VARCHAR NOT NULL,
			created_by VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`,
	},
	{
		version: 24,
		name:    "sys_agent_messages",
		stmt: `CREATE TABLE IF NOT EXISTS sys_agent_messages (
			id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			role VARCHAR NOT NULL,
			content VARCHAR NOT NULL,
			agent_id VARCHAR NOT NULL,
			mentions_json VARCHAR NOT NULL,
			tool_calls_json VARCHAR NOT NULL,
			run_id VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`,
	},
	{
		version: 25,
		name:    "sys_agent_runs",
		stmt: `CREATE TABLE IF NOT EXISTS sys_agent_runs (
			id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			agent_id VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			error VARCHAR NOT NULL,
			started_at TIMESTAMP,
			finished_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL
		)`,
	},
}

// ApplySystemMigrations 按版本顺序应用全部系统表。可重复执行。
// 任一失败返回 catalog.ErrMigrationFailed，调用方应拒绝 listen。
func ApplySystemMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("%w: db is nil", catalog.ErrMigrationFailed)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sys_migration_versions (
		version BIGINT NOT NULL,
		name VARCHAR NOT NULL,
		applied_at TIMESTAMP NOT NULL
	)`); err != nil {
		return fmt.Errorf("%w: create sys_migration_versions: %v", catalog.ErrMigrationFailed, err)
	}

	for _, m := range systemMigrations {
		applied, err := isMigrationApplied(ctx, db, m.version)
		if err != nil {
			return fmt.Errorf("%w: check version %d: %v", catalog.ErrMigrationFailed, m.version, err)
		}
		if applied {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return fmt.Errorf("%w: apply %s (v%d): %v", catalog.ErrMigrationFailed, m.name, m.version, err)
		}
	}
	return nil
}

func isMigrationApplied(ctx context.Context, db *sql.DB, version int) (bool, error) {
	var v int64
	err := db.QueryRowContext(ctx,
		"SELECT version FROM sys_migration_versions WHERE version = ?", version).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func applyOne(ctx context.Context, db *sql.DB, m migration) error {
	// DuckLake DDL 可能无法放进与 DML 同一事务；逐条执行后再记录版本。
	for _, s := range splitStatements(m.stmt) {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	_, err := db.ExecContext(ctx,
		"INSERT INTO sys_migration_versions(version, name, applied_at) VALUES(?, ?, ?)",
		m.version, m.name, time.Now().UTC())
	return err
}

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

// MigrationCount 返回已定义的系统迁移条数（不含 sys_migration_versions 自身）。
func MigrationCount() int { return len(systemMigrations) }
