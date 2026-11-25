// 文件说明：split_test.go — godatabase/sqlguard 的单元测试（TestSplitStatements 等）
//
// 用法
//
//	go test ./godatabase/sqlguard -run 'TestSplitStatements'
//
// 调用架构
//
//	[上游] godatabase/cmd, godatabase/ducklake, internal/agentbridge, internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] godatabase/sqlguard/split_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] classifier.go, classifier_test.go, reserved_prefix_test.go, validator.go
//
// 主要符号
//
//	TestSplitStatements(t *testing.T)

package sqlguard

import "testing"

func TestSplitStatements(t *testing.T) {
	parts, err := SplitStatements("CREATE TABLE t (id INTEGER); INSERT INTO t VALUES (1)")
	if err != nil || len(parts) != 2 {
		t.Fatalf("parts=%v err=%v", parts, err)
	}
	parts, err = SplitStatements("INSERT INTO t VALUES('a;b')")
	if err != nil || len(parts) != 1 {
		t.Fatalf("string semicolon: %v %v", parts, err)
	}
	parts, err = SplitStatements("SELECT 1; -- tail")
	if err != nil || len(parts) != 2 {
		t.Fatalf("comment tail: %#v %v", parts, err)
	}
	if FirstKeyword(parts[1]) != "" {
		t.Fatalf("comment keyword %q", FirstKeyword(parts[1]))
	}
	if _, err := SplitStatements("SELECT \x00 1"); err != ErrNulChar {
		t.Fatalf("nul: %v", err)
	}
	parts, err = SplitStatements("  ")
	if err != nil || len(parts) != 0 {
		t.Fatalf("blank: %#v %v", parts, err)
	}
}
