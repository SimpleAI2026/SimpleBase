// 文件说明：schema_test.go — internal/catalog 的测试辅助代码
//
// 用法
//
//	同包调用：applyTestSchema() → service_test.go, sql_repository_test.go
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 10 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/catalog/schema_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] always_open_test.go, cache.go, cache_test.go, create_invalidate_test.go, data_model.go 等 20 个
//	[关键引用] applyTestSchema ← service_test.go, sql_repository_test.go
//
// 主要符号
//
//	applyTestSchema(db *sql.DB) error

package catalog

import "database/sql"

// applyTestSchema 为 catalog 包内单测在 SQLite 上创建与系统库相同的 sys_* 表。
func applyTestSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sys_tenants (id VARCHAR NOT NULL, name VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_projects (id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, name VARCHAR NOT NULL, database_path VARCHAR NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_databases (
			id VARCHAR NOT NULL, tenant_id VARCHAR NOT NULL, project_id VARCHAR NOT NULL, name VARCHAR NOT NULL,
			kind VARCHAR NOT NULL, status VARCHAR NOT NULL, storage_prefix VARCHAR NOT NULL, format_version BIGINT NOT NULL,
			deleted_at TIMESTAMP, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
			data_model VARCHAR NOT NULL DEFAULT 'collection')`,
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
		`CREATE TABLE IF NOT EXISTS sys_project_quotas (
			project_id VARCHAR NOT NULL, max_databases BIGINT NOT NULL, max_storage_bytes BIGINT NOT NULL,
			max_llm_requests BIGINT NOT NULL, max_llm_tokens BIGINT NOT NULL, period_seconds BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_site_quotas (
			project_id VARCHAR NOT NULL, max_requests BIGINT NOT NULL, max_egress_bytes BIGINT NOT NULL,
			period_seconds BIGINT NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sys_site_traffic (
			project_id VARCHAR NOT NULL, day VARCHAR NOT NULL, requests BIGINT NOT NULL, bytes BIGINT NOT NULL,
			updated_at TIMESTAMP NOT NULL)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
