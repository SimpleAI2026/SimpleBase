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
		// deprecated: retained for migration history. The jobs worker was removed;
		// this table is no longer read or written by application code.
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
	{
		version: 26,
		name:    "sys_agent_schedules",
		stmt: `CREATE TABLE IF NOT EXISTS sys_agent_schedules (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			agent_id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL,
			prompt VARCHAR NOT NULL,
			cron_expr VARCHAR NOT NULL,
			enabled BIGINT NOT NULL,
			last_run_at TIMESTAMP,
			next_run_at TIMESTAMP,
			created_by VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`,
	},
	{
		version: 27,
		name:    "sys_agent_schedule_runs",
		stmt: `CREATE TABLE IF NOT EXISTS sys_agent_schedule_runs (
			id VARCHAR NOT NULL,
			schedule_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			agent_id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL,
			run_id VARCHAR NOT NULL,
			trigger VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			error VARCHAR NOT NULL,
			started_at TIMESTAMP,
			finished_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL
		)`,
	},
	{
		// v28：云函数（ui-gofunction-plan §5）。源码为权威，exports_json 冗余。
		version: 28,
		name:    "sys_gofunctions",
		stmt: `CREATE TABLE IF NOT EXISTS sys_gofunctions (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			source VARCHAR NOT NULL,
			exports_json VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`,
	},
	{
		// v29：定时任务（ui-cronjob-plan §4.1）。目标为云函数导出函数；两种调度模式。
		version: 29,
		name:    "sys_cron_jobs",
		stmt: `CREATE TABLE IF NOT EXISTS sys_cron_jobs (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			description VARCHAR NOT NULL DEFAULT '',
			schedule_kind VARCHAR NOT NULL,
			cron_expr VARCHAR,
			interval_seconds BIGINT,
			run_at TIMESTAMP,
			func_file VARCHAR NOT NULL,
			func_export VARCHAR NOT NULL,
			input_json VARCHAR NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
			last_run_at TIMESTAMP,
			next_run_at TIMESTAMP,
			last_status VARCHAR NOT NULL DEFAULT '',
			last_error VARCHAR NOT NULL DEFAULT '',
			run_count BIGINT NOT NULL DEFAULT 0,
			created_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`,
	},
	{
		// v30：定时任务运行记录（ui-cronjob-plan §4.2）。response_json 截断 4KB。
		version: 30,
		name:    "sys_cron_job_runs",
		stmt: `CREATE TABLE IF NOT EXISTS sys_cron_job_runs (
			id VARCHAR NOT NULL,
			job_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			trigger VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			error VARCHAR NOT NULL DEFAULT '',
			duration_ms BIGINT NOT NULL DEFAULT 0,
			response_json VARCHAR,
			started_at TIMESTAMP,
			finished_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL
		)`,
	},
	{
		// v31：控制台用户（login-auth-plan §3.1）。password_hash 为 argon2id 编码串。
		version: 31,
		name:    "sys_users",
		stmt: `CREATE TABLE IF NOT EXISTS sys_users (
			id VARCHAR NOT NULL,
			username VARCHAR NOT NULL,
			password_hash VARCHAR NOT NULL,
			role VARCHAR NOT NULL,
			display_name VARCHAR NOT NULL DEFAULT '',
			email VARCHAR NOT NULL DEFAULT '',
			status VARCHAR NOT NULL,
			must_change_password BIGINT NOT NULL DEFAULT 0,
			created_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			last_login_at TIMESTAMP,
			disabled_at TIMESTAMP
		)`,
	},
	{
		// v32：OAuth2 refresh 会话（login-auth-plan §3.2）。只存 refresh 摘要。
		version: 32,
		name:    "sys_user_sessions",
		stmt: `CREATE TABLE IF NOT EXISTS sys_user_sessions (
			id VARCHAR NOT NULL,
			user_id VARCHAR NOT NULL,
			refresh_token_hash VARCHAR NOT NULL,
			access_jti VARCHAR NOT NULL DEFAULT '',
			user_agent VARCHAR NOT NULL DEFAULT '',
			ip VARCHAR NOT NULL DEFAULT '',
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL,
			revoked_at TIMESTAMP
		)`,
	},
	{
		// v33：项目归属（login-auth-plan §3.3）。user 只可见自己创建的项目。
		version: 33,
		name:    "sys_project_owners",
		stmt: `CREATE TABLE IF NOT EXISTS sys_project_owners (
			project_id VARCHAR NOT NULL,
			user_id VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`,
	},
	{
		// v34：云函数实体（gofunction-versions-testplan §4.1）。active_version=0 表示未发布。
		version: 34,
		name:    "sys_go_funcs",
		stmt: `CREATE TABLE IF NOT EXISTS sys_go_funcs (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			active_version BIGINT NOT NULL DEFAULT 0,
			description VARCHAR NOT NULL DEFAULT '',
			created_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`,
	},
	{
		// v35：云函数不可变版本快照（§4.2）。source 为权威。
		version: 35,
		name:    "sys_go_func_versions",
		stmt: `CREATE TABLE IF NOT EXISTS sys_go_func_versions (
			id VARCHAR NOT NULL,
			func_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			version BIGINT NOT NULL,
			source VARCHAR NOT NULL,
			exports_json VARCHAR NOT NULL,
			note VARCHAR NOT NULL DEFAULT '',
			created_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL
		)`,
	},
	{
		// v36：云函数调用流水（§4.3）。不存 body/响应原文。
		version: 36,
		name:    "sys_go_func_invokes",
		stmt: `CREATE TABLE IF NOT EXISTS sys_go_func_invokes (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			func_name VARCHAR NOT NULL,
			function_name VARCHAR NOT NULL,
			version BIGINT NOT NULL,
			channel VARCHAR NOT NULL,
			status_code BIGINT NOT NULL,
			duration_ms BIGINT NOT NULL,
			request_id VARCHAR NOT NULL,
			actor VARCHAR NOT NULL DEFAULT '',
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
