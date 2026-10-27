// 文件说明：schema.go — internal/systemdb 中 EnsureSystemSchema、SystemTableCount 等符号的实现
//
// 用法
//
//	EnsureSystemSchema(ctx context.Context, db *sql.DB) error  // EnsureSystemSchema 按最终态建表，幂等（CREATE TABLE IF NOT EXISTS）
//	SystemTableCount() int// SystemTableCount 返回系统表数量
//	同包调用：EnsureSystemSchema() → bootstrap.go；systemTables → schema_status.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/schema.go
//	         │ 依赖
//	         ▼
//	[下游] internal/catalog
//	[同包] agent_schedules.go, agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//	[关键引用] EnsureSystemSchema() ← bootstrap.go
//
// 主要符号
//
//	EnsureSystemSchema(ctx context.Context, db *sql.DB) error
//	SystemTableCount() int
//	... 另有 2 个顶层符号

package systemdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/linkxzhou/SimpleBase/internal/catalog"
)

// systemTables 是系统库的最终态表结构（全部 sys_* 表）。
//
// 项目尚未发布，不保留任何版本化迁移：表结构变更直接改这里，已存在的库用 reset 重建。
// DuckLake 不使用 PRIMARY KEY / AUTOINCREMENT；唯一性由应用层保证。
// DuckLake 不支持 ADD COLUMN 带 NOT NULL，也就不会再有补列语句——新增列直接写进对应建表语句。
var systemTables = []systemTable{
	{name: "sys_tenants", ddl: `CREATE TABLE IF NOT EXISTS sys_tenants (
			id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_projects", ddl: `CREATE TABLE IF NOT EXISTS sys_projects (
			id VARCHAR NOT NULL,
			tenant_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			database_path VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_databases", ddl: `CREATE TABLE IF NOT EXISTS sys_databases (
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
			updated_at TIMESTAMP NOT NULL,
			data_model VARCHAR DEFAULT 'collection'
		)`},
	{name: "sys_api_keys", ddl: `CREATE TABLE IF NOT EXISTS sys_api_keys (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			key_hash VARCHAR NOT NULL,
			permissions VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			revoked_at TIMESTAMP
		)`},
	{name: "sys_llm_provider_configs", ddl: `CREATE TABLE IF NOT EXISTS sys_llm_provider_configs (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			provider VARCHAR NOT NULL,
			credential_ref VARCHAR NOT NULL,
			enabled BIGINT NOT NULL,
			allowed_models_json VARCHAR NOT NULL,
			default_model VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_usage_events", ddl: `CREATE TABLE IF NOT EXISTS sys_usage_events (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			kind VARCHAR NOT NULL,
			provider VARCHAR NOT NULL,
			model VARCHAR NOT NULL,
			input_tokens BIGINT NOT NULL,
			output_tokens BIGINT NOT NULL,
			cost_micros BIGINT NOT NULL,
			request_id VARCHAR NOT NULL,
			occurred_at TIMESTAMP NOT NULL,
			duration_ms BIGINT DEFAULT 0
		)`},
	{name: "sys_operations", ddl: `CREATE TABLE IF NOT EXISTS sys_operations (
			id VARCHAR NOT NULL,
			database_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			principal_id VARCHAR NOT NULL,
			kind VARCHAR NOT NULL,
			request_id VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_jobs", ddl: `CREATE TABLE IF NOT EXISTS sys_jobs (
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
		)`},
	{name: "sys_project_quotas", ddl: `CREATE TABLE IF NOT EXISTS sys_project_quotas (
			project_id VARCHAR NOT NULL,
			max_databases BIGINT NOT NULL,
			max_storage_bytes BIGINT NOT NULL,
			max_llm_requests BIGINT NOT NULL,
			max_llm_tokens BIGINT NOT NULL,
			period_seconds BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_metric_samples", ddl: `CREATE TABLE IF NOT EXISTS sys_metric_samples (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			value_double DOUBLE NOT NULL,
			labels_json VARCHAR NOT NULL,
			occurred_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_metric_rollups_hourly", ddl: `CREATE TABLE IF NOT EXISTS sys_metric_rollups_hourly (
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			bucket_start TIMESTAMP NOT NULL,
			count BIGINT NOT NULL,
			sum DOUBLE NOT NULL,
			min DOUBLE NOT NULL,
			max DOUBLE NOT NULL
		)`},
	{name: "sys_s3_objects", ddl: `CREATE TABLE IF NOT EXISTS sys_s3_objects (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			object_key VARCHAR NOT NULL,
			size BIGINT NOT NULL,
			etag VARCHAR NOT NULL,
			content_type VARCHAR NOT NULL,
			last_modified TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			deleted_at TIMESTAMP
		)`},
	{name: "sys_s3_sync_runs", ddl: `CREATE TABLE IF NOT EXISTS sys_s3_sync_runs (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			listed BIGINT NOT NULL,
			inserted BIGINT NOT NULL,
			removed BIGINT NOT NULL,
			error VARCHAR NOT NULL,
			started_at TIMESTAMP NOT NULL,
			finished_at TIMESTAMP
		)`},
	{name: "sys_llm_sessions", ddl: `CREATE TABLE IF NOT EXISTS sys_llm_sessions (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			title VARCHAR NOT NULL,
			provider VARCHAR NOT NULL,
			model VARCHAR NOT NULL,
			created_by VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`},
	{name: "sys_llm_messages", ddl: `CREATE TABLE IF NOT EXISTS sys_llm_messages (
			id VARCHAR NOT NULL,
			session_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			role VARCHAR NOT NULL,
			content VARCHAR NOT NULL,
			token_input BIGINT NOT NULL,
			token_output BIGINT NOT NULL,
			request_id VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_llm_settings", ddl: `CREATE TABLE IF NOT EXISTS sys_llm_settings (
			project_id VARCHAR NOT NULL,
			default_provider VARCHAR NOT NULL,
			default_model VARCHAR NOT NULL,
			temperature DOUBLE NOT NULL,
			max_tokens BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			api_key VARCHAR DEFAULT '',
			base_url VARCHAR DEFAULT ''
		)`},
	{name: "sys_settings_global", ddl: `CREATE TABLE IF NOT EXISTS sys_settings_global (
			key VARCHAR NOT NULL,
			value_json VARCHAR NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_settings_project", ddl: `CREATE TABLE IF NOT EXISTS sys_settings_project (
			project_id VARCHAR NOT NULL,
			key VARCHAR NOT NULL,
			value_json VARCHAR NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_log_events", ddl: `CREATE TABLE IF NOT EXISTS sys_log_events (
			id VARCHAR NOT NULL,
			project_id VARCHAR,
			level VARCHAR NOT NULL,
			logger VARCHAR NOT NULL,
			message VARCHAR NOT NULL,
			fields_json VARCHAR NOT NULL,
			request_id VARCHAR NOT NULL,
			occurred_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_log_retention", ddl: `CREATE TABLE IF NOT EXISTS sys_log_retention (
			scope VARCHAR NOT NULL,
			project_id VARCHAR,
			keep_days BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_cloud_agents", ddl: `CREATE TABLE IF NOT EXISTS sys_cloud_agents (
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
			archived_at TIMESTAMP,
			builtin_key VARCHAR DEFAULT '',
			writable BIGINT DEFAULT 0
		)`},
	{name: "sys_agent_threads", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_threads (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			title VARCHAR NOT NULL,
			created_by VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP,
			agent_id VARCHAR
		)`},
	{name: "sys_agent_messages", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_messages (
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
		)`},
	{name: "sys_agent_runs", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_runs (
			id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			agent_id VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			error VARCHAR NOT NULL,
			started_at TIMESTAMP,
			finished_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			duration_ms BIGINT DEFAULT 0,
			prompt_tokens BIGINT DEFAULT 0,
			completion_tokens BIGINT DEFAULT 0,
			reasoning_tokens BIGINT DEFAULT 0,
			tool_calls BIGINT DEFAULT 0,
			error_code VARCHAR DEFAULT '',
			retry_of_run_id VARCHAR DEFAULT '',
			initiator_id VARCHAR
		)`},
	{name: "sys_agent_schedules", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_schedules (
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
		)`},
	{name: "sys_agent_schedule_runs", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_schedule_runs (
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
		)`},
	{name: "sys_cron_jobs", ddl: `CREATE TABLE IF NOT EXISTS sys_cron_jobs (
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
			archived_at TIMESTAMP,
			target_kind VARCHAR DEFAULT 'function',
			agent_id VARCHAR DEFAULT '',
			agent_thread_id VARCHAR DEFAULT '',
			agent_prompt VARCHAR DEFAULT ''
		)`},
	{name: "sys_cron_job_runs", ddl: `CREATE TABLE IF NOT EXISTS sys_cron_job_runs (
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
		)`},
	{name: "sys_users", ddl: `CREATE TABLE IF NOT EXISTS sys_users (
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
		)`},
	{name: "sys_user_sessions", ddl: `CREATE TABLE IF NOT EXISTS sys_user_sessions (
			id VARCHAR NOT NULL,
			user_id VARCHAR NOT NULL,
			refresh_token_hash VARCHAR NOT NULL,
			access_jti VARCHAR NOT NULL DEFAULT '',
			user_agent VARCHAR NOT NULL DEFAULT '',
			ip VARCHAR NOT NULL DEFAULT '',
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL,
			revoked_at TIMESTAMP
		)`},
	{name: "sys_project_owners", ddl: `CREATE TABLE IF NOT EXISTS sys_project_owners (
			project_id VARCHAR NOT NULL,
			user_id VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_go_funcs", ddl: `CREATE TABLE IF NOT EXISTS sys_go_funcs (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			active_version BIGINT NOT NULL DEFAULT 0,
			description VARCHAR NOT NULL DEFAULT '',
			created_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`},
	{name: "sys_go_func_versions", ddl: `CREATE TABLE IF NOT EXISTS sys_go_func_versions (
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
		)`},
	{name: "sys_go_func_invokes", ddl: `CREATE TABLE IF NOT EXISTS sys_go_func_invokes (
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
		)`},
	{name: "sys_sandboxes", ddl: `CREATE TABLE IF NOT EXISTS sys_sandboxes (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			cloud_name VARCHAR NOT NULL,
			source VARCHAR NOT NULL,
			thread_id VARCHAR,
			image VARCHAR NOT NULL,
			cpus BIGINT NOT NULL,
			memory_mib BIGINT NOT NULL,
			network VARCHAR NOT NULL,
			idle_timeout_s BIGINT NOT NULL,
			max_duration_s BIGINT NOT NULL,
			status VARCHAR NOT NULL,
			last_error VARCHAR NOT NULL DEFAULT '',
			created_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			started_at TIMESTAMP,
			last_active_at TIMESTAMP,
			expires_at TIMESTAMP,
			deleted_at TIMESTAMP,
			metadata VARCHAR DEFAULT '{}'
		)`},
	{name: "sys_cloud_agent_seed_dismissed", ddl: `CREATE TABLE IF NOT EXISTS sys_cloud_agent_seed_dismissed (
			project_id VARCHAR NOT NULL,
			builtin_key VARCHAR NOT NULL
		)`},
	{name: "sys_llm_provider_creds", ddl: `CREATE TABLE IF NOT EXISTS sys_llm_provider_creds (
			project_id VARCHAR NOT NULL,
			provider VARCHAR NOT NULL,
			credentials_json VARCHAR NOT NULL,
			default_model VARCHAR NOT NULL DEFAULT '',
			enabled BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_collections", ddl: `CREATE TABLE IF NOT EXISTS sys_collections (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			schema_mode VARCHAR NOT NULL DEFAULT 'json',
			status VARCHAR NOT NULL DEFAULT 'active',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			view_query VARCHAR DEFAULT ''
		)`},
	{name: "sys_collection_fields", ddl: `CREATE TABLE IF NOT EXISTS sys_collection_fields (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			collection VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			type VARCHAR NOT NULL,
			required BIGINT NOT NULL DEFAULT 0,
			unique_flag BIGINT NOT NULL DEFAULT 0,
			default_json VARCHAR NOT NULL DEFAULT '',
			options_json VARCHAR NOT NULL DEFAULT '{}',
			hidden BIGINT NOT NULL DEFAULT 0,
			readonly BIGINT NOT NULL DEFAULT 0,
			order_index BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_backups", ddl: `CREATE TABLE IF NOT EXISTS sys_backups (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			storage_key VARCHAR NOT NULL,
			size_bytes BIGINT NOT NULL,
			sha256 VARCHAR NOT NULL,
			engine VARCHAR NOT NULL,
			format_version BIGINT NOT NULL DEFAULT 0,
			status VARCHAR NOT NULL DEFAULT 'ready',
			error VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP
		)`},
	{name: "sys_api_key_policies", ddl: `CREATE TABLE IF NOT EXISTS sys_api_key_policies (
			key_id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			resources_json VARCHAR NOT NULL DEFAULT '[]',
			actions_json VARCHAR NOT NULL DEFAULT '[]',
			hidden_fields_json VARCHAR NOT NULL DEFAULT '[]',
			readonly_fields_json VARCHAR NOT NULL DEFAULT '[]',
			expires_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL
		)`},
	{name: "sys_function_triggers", ddl: `CREATE TABLE IF NOT EXISTS sys_function_triggers (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			kind VARCHAR NOT NULL,
			target VARCHAR NOT NULL DEFAULT '',
			events_json VARCHAR NOT NULL DEFAULT '[]',
			func_file VARCHAR NOT NULL,
			func_export VARCHAR NOT NULL,
			filter VARCHAR NOT NULL DEFAULT '',
			enabled BIGINT NOT NULL DEFAULT 1,
			created_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`},
}

// systemTable 是一张系统表的建表语句。
type systemTable struct {
	name string
	ddl  string
}

// EnsureSystemSchema 按最终态建表，幂等（CREATE TABLE IF NOT EXISTS）。
// 任一失败返回 catalog.ErrMigrationFailed，调用方应拒绝 listen。
func EnsureSystemSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("%w: db is nil", catalog.ErrMigrationFailed)
	}
	for _, t := range systemTables {
		if _, err := db.ExecContext(ctx, t.ddl); err != nil {
			return fmt.Errorf("%w: create %s: %v", catalog.ErrMigrationFailed, t.name, err)
		}
	}
	return nil
}

// SystemTableCount 返回系统表数量。
func SystemTableCount() int { return len(systemTables) }
