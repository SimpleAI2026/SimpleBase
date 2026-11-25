// 文件说明：system_redact_test.go — internal/api 的单元测试（TestSystemSensitiveColumns、TestSystemSQLReferencesSensitive 等）
//
// 用法
//
//	go test ./internal/api -run 'TestSystemSensitiveColumns|TestSystemSQLReferencesSensitive|TestRedactSystemResult'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/system_redact_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestSystemSensitiveColumns(t *testing.T)
//	TestSystemSQLReferencesSensitive(t *testing.T)
//	TestRedactSystemResult(t *testing.T)

package api

import "testing"

func TestSystemSensitiveColumns(t *testing.T) {
	for _, name := range []string{
		"password_hash", "key_hash", "refresh_token_hash", "access_jti",
		"credentials_json", "credential_ref", "PASSWORD_HASH",
	} {
		if !isSystemSensitiveColumn(name) {
			t.Fatalf("%s should be sensitive", name)
		}
	}
	for _, name := range []string{"token_input", "token_output", "max_tokens", "prompt_tokens", "request_id", "id"} {
		if isSystemSensitiveColumn(name) {
			t.Fatalf("%s should stay visible", name)
		}
	}
}

func TestSystemSQLReferencesSensitive(t *testing.T) {
	blocked := []string{
		`SELECT password_hash FROM sys_users`,
		`SELECT password_hash AS h FROM sys_users`,
		`SELECT substr(password_hash, 1, 4)`,
		`SELECT "password_hash" FROM sys_users`,
		"SELECT key_hash, refresh_token_hash",
	}
	for _, sql := range blocked {
		if !systemSQLReferencesSensitive(sql) {
			t.Fatalf("expected block %s", sql)
		}
	}
	allowed := []string{
		`SELECT id, username FROM sys_users`,
		`SELECT * FROM sys_users`,
		`SELECT 'password_hash' AS label`,
		"SELECT token_input FROM sys_usage_events",
		"-- password_hash\nSELECT 1",
		`SELECT id /* password_hash */ FROM sys_users`,
	}
	for _, sql := range allowed {
		if systemSQLReferencesSensitive(sql) {
			t.Fatalf("expected allow %s", sql)
		}
	}
}

func TestRedactSystemResult(t *testing.T) {
	rows := [][]any{{"secret", "ada"}, {"other", "bob"}}
	got := redactSystemResult([]string{"password_hash", "username"}, rows)
	if len(got) != 1 || got[0] != "password_hash" {
		t.Fatalf("names %v", got)
	}
	if rows[0][0] != nil || rows[0][1] != "ada" || rows[1][0] != nil {
		t.Fatalf("rows %#v", rows)
	}
	if redactSystemResult([]string{"id"}, [][]any{{"1"}}) != nil {
		t.Fatal("expected no redaction")
	}
}
