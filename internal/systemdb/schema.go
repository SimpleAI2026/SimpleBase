// 文件说明：schema.go — internal/systemdb 中 EnsureSystemSchema 等符号的实现
//
// 用法
//
//	EnsureSystemSchema(ctx context.Context, db *sql.DB) error  // EnsureSystemSchema 按最终态建表，幂等（CREATE TABLE IF NOT EXISTS）
//	同包调用：EnsureSystemSchema() → bootstrap.go；systemTables → schema_status.go
//
// 调用架构
//
//	[上游] godatabase/bench, internal/agentbridge, internal/api 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/systemdb/schema.go
//	         │ 依赖
//	         ▼
//	[下游] internal/catalog
//	[同包] agents.go, async_flush.go, bootstrap.go, cron_jobs.go 等 22 个
//	[关键引用] EnsureSystemSchema() ← bootstrap.go
//
// 主要符号
//
//	EnsureSystemSchema(ctx context.Context, db *sql.DB) error
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
	// production-readiness-plan §8.2.6：终端用户（应用自己的用户体系，与管理员/平台会话隔离）。
	{name: "sys_end_users", ddl: `CREATE TABLE IF NOT EXISTS sys_end_users (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			external_id VARCHAR NOT NULL DEFAULT '',
			password_hash VARCHAR NOT NULL DEFAULT '',
			display_name VARCHAR NOT NULL DEFAULT '',
			email VARCHAR NOT NULL DEFAULT '',
			status VARCHAR NOT NULL DEFAULT 'active',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	// production-readiness-plan §8.3.3：系统级审计事件（与项目级 sys_operations 分离；
	// actor 域分隔 user:<id> | key:<id> | enduser:<id> | system）。
	{name: "sys_audit_events", ddl: `CREATE TABLE IF NOT EXISTS sys_audit_events (
			id VARCHAR NOT NULL,
			occurred_at TIMESTAMP NOT NULL,
			actor VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL DEFAULT '',
			kind VARCHAR NOT NULL,
			detail VARCHAR NOT NULL DEFAULT ''
		)`},
	// production-readiness-plan §8.3.5：告警去重状态（同 kind+project 60s 窗口聚合）。
	{name: "sys_alert_state", ddl: `CREATE TABLE IF NOT EXISTS sys_alert_state (
			kind VARCHAR NOT NULL,
			project VARCHAR NOT NULL DEFAULT '',
			first_at TIMESTAMP NOT NULL,
			last_at TIMESTAMP NOT NULL,
			count BIGINT NOT NULL DEFAULT 0,
			severity VARCHAR NOT NULL DEFAULT 'warning',
			detail VARCHAR NOT NULL DEFAULT ''
		)`},
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
	// planv6.5：路径级外部访问开关。kind=file 精确匹配；kind=folder 为以 "/" 结尾的前缀，"" 表示整个桶。
	{name: "sys_site_published_paths", ddl: `CREATE TABLE IF NOT EXISTS sys_site_published_paths (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			path VARCHAR NOT NULL,
			kind VARCHAR NOT NULL,
			created_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL
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
	// planv6.5 model-settings-and-trace-plan §3.1：每次模型调用一条 trace。
	// request_id 与 sys_log_events / sys_audit_events 互查；二期（§9）扩列为 span 模型：
	// trace_id 聚合一次顶层调用（agent 场景 = run_id，API 直调 = request_id），
	// span_kind 区分 llm / tool 行，input/output/tools_json 承载内容观测。
	{name: "sys_llm_traces", ddl: `CREATE TABLE IF NOT EXISTS sys_llm_traces (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			request_id VARCHAR NOT NULL,
			source VARCHAR NOT NULL,
			provider VARCHAR NOT NULL,
			model VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			stream BIGINT NOT NULL,
			duration_ms BIGINT DEFAULT 0,
			input_tokens BIGINT DEFAULT 0,
			output_tokens BIGINT DEFAULT 0,
			error_code VARCHAR DEFAULT '',
			error_message VARCHAR DEFAULT '',
			trace_id VARCHAR NOT NULL DEFAULT '',
			caller VARCHAR NOT NULL DEFAULT '',
			span_kind VARCHAR NOT NULL DEFAULT 'llm',
			name VARCHAR NOT NULL DEFAULT '',
			input_json VARCHAR NOT NULL DEFAULT '',
			output_json VARCHAR NOT NULL DEFAULT '',
			tools_json VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL
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
	// sys_agent_approvals 是破坏性工具调用的审批队列（planv6.6）。
	// 与 sys_agent_runs 分离：run 是执行记录，审批是「待办」，
	// 需要跨请求查询（列表页）与超时收敛，不能只活在内存 confirmHub 里。
	// 唯一性由应用层保证：(project_id, run_id, call_id) 至多一条 pending。
	{name: "sys_agent_approvals", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_approvals (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			run_id VARCHAR NOT NULL,
			call_id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL DEFAULT '',
			agent_id VARCHAR NOT NULL DEFAULT '',
			tool_name VARCHAR NOT NULL DEFAULT '',
			argv_json VARCHAR NOT NULL DEFAULT '[]',
			message VARCHAR NOT NULL DEFAULT '',
			status VARCHAR NOT NULL,
			initiator_id VARCHAR NOT NULL DEFAULT '',
			resolved_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP,
			resolved_at TIMESTAMP
		)`},
	// planv6.7 AGC-NAV：工作台标签（预置 alerts/stats 的覆盖行 + 自定义标签）。
	// 唯一性由应用层保证：(project_id, id) 至多一行；同项目未归档 name 不重复。
	{name: "sys_agent_workspace_tags", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_workspace_tags (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			builtin_key VARCHAR NOT NULL DEFAULT '',
			name VARCHAR NOT NULL,
			config_json VARCHAR NOT NULL,
			version BIGINT NOT NULL DEFAULT 1,
			created_by VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`},
	// planv6.7 AGC-NAV：会话的工作台元数据。只在建会话事务内插入，(project_id, thread_id) 至多一行。
	// 归档标签只清空 tag_id，保留 dispatch_mode（AGC-NAV §2.1）。
	{name: "sys_agent_thread_workspace", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_thread_workspace (
			project_id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL,
			tag_id VARCHAR NOT NULL DEFAULT '',
			dispatch_mode VARCHAR NOT NULL DEFAULT '',
			source VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL
		)`},
	// sys_mcp_servers 是项目级外部 MCP Server 配置（planv6.2 §6-1）。
	// headers_json 存静态 Header（含凭据），与 sys_llm_settings.api_key 同口径：
	// 只服务端内部使用，对外只回 has_credential，禁止记日志。
	// tools_policy_json 存每工具的 read_only / destructive 标记——这是权限判定的
	// 唯一事实来源，Server 自报的 annotations 只作配置向导建议。
	{name: "sys_mcp_servers", ddl: `CREATE TABLE IF NOT EXISTS sys_mcp_servers (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			name VARCHAR NOT NULL,
			transport VARCHAR NOT NULL,
			url VARCHAR NOT NULL DEFAULT '',
			headers_json VARCHAR NOT NULL DEFAULT '{}',
			timeout_s BIGINT NOT NULL DEFAULT 60,
			enabled BIGINT NOT NULL DEFAULT 1,
			tools_policy_json VARCHAR NOT NULL DEFAULT '{}',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
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
	// planv6.7 v2 协作群：会话级协作元数据（类型 + 活动轮次占用）。
	// (project_id, thread_id) 至多一行，与建会话同事务写入；active_turn_id 用 CAS 抢占。
	{name: "sys_agent_thread_collab", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_thread_collab (
			project_id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL,
			tag_id VARCHAR NOT NULL DEFAULT '',
			thread_kind VARCHAR NOT NULL DEFAULT 'chat',
			protocol_version BIGINT NOT NULL DEFAULT 2,
			active_turn_id VARCHAR NOT NULL DEFAULT '',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	// 轮次：一次发送或一次定时触发。(project_id, id) 至多一行；
	// 同会话同幂等键唯一（先查后插，在项目级互斥内完成）。
	{name: "sys_agent_turns", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_turns (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL,
			tag_id VARCHAR NOT NULL DEFAULT '',
			initiator_kind VARCHAR NOT NULL DEFAULT 'user',
			initiator_id VARCHAR NOT NULL DEFAULT '',
			idempotency_key VARCHAR NOT NULL DEFAULT '',
			request_hash VARCHAR NOT NULL DEFAULT '',
			status VARCHAR NOT NULL,
			route_json VARCHAR NOT NULL DEFAULT '{}',
			config_version BIGINT NOT NULL DEFAULT 0,
			config_snapshot_json VARCHAR NOT NULL DEFAULT '{}',
			error_code VARCHAR NOT NULL DEFAULT '',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	// 任务：轮次内交给某个助手或某个人的一项职责。人工回复靠 version 做 CAS。
	{name: "sys_agent_tasks", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_tasks (
			id VARCHAR NOT NULL,
			project_id VARCHAR NOT NULL,
			turn_id VARCHAR NOT NULL,
			thread_id VARCHAR NOT NULL,
			seq BIGINT NOT NULL DEFAULT 0,
			target_kind VARCHAR NOT NULL,
			target_id VARCHAR NOT NULL DEFAULT '',
			instruction VARCHAR NOT NULL DEFAULT '',
			depends_json VARCHAR NOT NULL DEFAULT '[]',
			status VARCHAR NOT NULL,
			attempt BIGINT NOT NULL DEFAULT 0,
			run_id VARCHAR NOT NULL DEFAULT '',
			assignee_id VARCHAR NOT NULL DEFAULT '',
			reply_text VARCHAR NOT NULL DEFAULT '',
			reply_outcome VARCHAR NOT NULL DEFAULT '',
			replied_by VARCHAR NOT NULL DEFAULT '',
			error_code VARCHAR NOT NULL DEFAULT '',
			version BIGINT NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`},
	// 任务 × run 的重试历史：不覆盖旧 run，便于回溯每次尝试。
	{name: "sys_agent_task_runs", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_task_runs (
			project_id VARCHAR NOT NULL,
			task_id VARCHAR NOT NULL,
			attempt BIGINT NOT NULL,
			run_id VARCHAR NOT NULL,
			status VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`},
	// 消息身份旁挂：不改 sys_agent_messages 列，补齐轮次/任务/发言人。
	{name: "sys_agent_message_meta", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_message_meta (
			project_id VARCHAR NOT NULL,
			message_id VARCHAR NOT NULL,
			turn_id VARCHAR NOT NULL DEFAULT '',
			task_id VARCHAR NOT NULL DEFAULT '',
			speaker_kind VARCHAR NOT NULL DEFAULT 'agent',
			speaker_id VARCHAR NOT NULL DEFAULT '',
			kind VARCHAR NOT NULL DEFAULT 'agent',
			created_at TIMESTAMP NOT NULL
		)`},
	// 轮次事件：seq 在轮次内单调递增，供「快照 + 续传」。
	{name: "sys_agent_turn_events", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_turn_events (
			project_id VARCHAR NOT NULL,
			turn_id VARCHAR NOT NULL,
			seq BIGINT NOT NULL,
			kind VARCHAR NOT NULL,
			entity_id VARCHAR NOT NULL DEFAULT '',
			payload_json VARCHAR NOT NULL DEFAULT '{}',
			created_at TIMESTAMP NOT NULL
		)`},
	// 群定时绑定：复用既有 cron 任务，不改 cron 表。
	{name: "sys_agent_group_cron", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_group_cron (
			project_id VARCHAR NOT NULL,
			job_id VARCHAR NOT NULL,
			tag_id VARCHAR NOT NULL,
			thread_policy VARCHAR NOT NULL DEFAULT 'reuse_thread',
			thread_kind VARCHAR NOT NULL DEFAULT 'scheduled',
			thread_id VARCHAR NOT NULL DEFAULT '',
			owner_user_id VARCHAR NOT NULL DEFAULT '',
			prompt VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			archived_at TIMESTAMP
		)`},
	// 触发去重：(project_id, job_id, fire_key) 至多一行。
	{name: "sys_agent_cron_fires", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_cron_fires (
			project_id VARCHAR NOT NULL,
			job_id VARCHAR NOT NULL,
			fire_key VARCHAR NOT NULL,
			turn_id VARCHAR NOT NULL DEFAULT '',
			thread_id VARCHAR NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL
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
