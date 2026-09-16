package catalog

import "database/sql"

// applyTestSchema 为 catalog 包内单测在 SQLite 上创建与系统库相同的 sys_* 表。
func applyTestSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sys_tenants (id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_projects (id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_databases (
			id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, name VARCHAR NOT NULL,
			kind VARCHAR NOT NULL, status VARCHAR NOT NULL, storage_prefix VARCHAR NOT NULL, format_version BIGINT NOT NULL,
			deleted_at TIMESTAMP, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_api_keys (
			id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, key_hash VARCHAR NOT NULL, permissions VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL, revoked_at TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS sys_llm_provider_configs (
			id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, provider VARCHAR NOT NULL, credential_ref VARCHAR NOT NULL,
			enabled BIGINT NOT NULL, allowed_models_json VARCHAR NOT NULL, default_model VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_usage_events (
			id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, kind VARCHAR NOT NULL, provider VARCHAR NOT NULL, model VARCHAR NOT NULL,
			input_tokens BIGINT NOT NULL, output_tokens BIGINT NOT NULL, cost_micros BIGINT NOT NULL, request_id VARCHAR NOT NULL,
			occurred_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_operations (
			id VARCHAR NOT NULL, database_id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, principal_id VARCHAR NOT NULL,
			kind VARCHAR NOT NULL, request_id VARCHAR NOT NULL, status VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_jobs (
			id VARCHAR NOT NULL, operation_id VARCHAR NOT NULL, database_id VARCHAR NOT NULL, project_id VARCHAR NOT NULL,
			type VARCHAR NOT NULL, status VARCHAR NOT NULL, attempt BIGINT NOT NULL, max_attempts BIGINT NOT NULL,
			run_after TIMESTAMP NOT NULL, payload_json VARCHAR NOT NULL, last_error VARCHAR NOT NULL,
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_project_quotas (
			project_id VARCHAR NOT NULL, max_databases BIGINT NOT NULL, max_storage_bytes BIGINT NOT NULL,
			max_llm_requests BIGINT NOT NULL, max_llm_tokens BIGINT NOT NULL, period_seconds BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
