// 文件说明：reserved_prefix_test.go — godatabase/sqlguard 的单元测试（TestValidateRejectsReservedIdentifiers 等）
//
// 用法
//
//	go test ./godatabase/sqlguard -run 'TestValidateRejectsReservedIdentifiers'
//
// 调用架构
//
//	[上游] godatabase/cmd, godatabase/ducklake, internal/agentbridge, internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/sqlguard/reserved_prefix_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] classifier.go, classifier_test.go, split_test.go, validator.go
//
// 主要符号
//
//	TestValidateRejectsReservedIdentifiers(t *testing.T)

package sqlguard

import "testing"

func TestValidateRejectsReservedIdentifiers(t *testing.T) {
	cases := []string{
		`SELECT * FROM __sb_kv_keys`,
		`select * from "__SB_KV_STRINGS"`,
		"SELECT * FROM `__sb_kv_lists`",
		`SELECT * FROM sqlite_master`,
		`DROP TABLE __sb_kv_keys`,
		`SELECT /* c */ * FROM __sb_kv_sets`,
		"SELECT *\nFROM\n__sb_kv_hashes",
	}
	for _, sql := range cases {
		for _, intent := range []Intent{ReadOnly, WriteAllowed} {
			if err := Validate(sql, intent); err == nil {
				t.Fatalf("expected %q to be rejected (intent %d)", sql, intent)
			}
		}
	}
	for _, sql := range []string{`SELECT '__sb_kv_keys'`, `SELECT 1 -- __sb_kv_keys`, `SELECT * FROM users`} {
		if err := Validate(sql, ReadOnly); err != nil {
			t.Fatalf("expected %q to pass: %v", sql, err)
		}
	}
}
