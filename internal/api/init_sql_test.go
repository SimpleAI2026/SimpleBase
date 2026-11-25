// 文件说明：init_sql_test.go — internal/api 的单元测试（TestPrepareInitSQL 等）
//
// 用法
//
//	go test ./internal/api -run 'TestPrepareInitSQL'
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/init_sql_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] adapter.go, adapters_test.go, agent_active_runs_test.go, agent_approval_handler.go, agent_crud_handler.go 等 207 个
//
// 主要符号
//
//	TestPrepareInitSQL(t *testing.T)

package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase/sqlguard"
)

func TestPrepareInitSQL(t *testing.T) {
	if stmts, err := prepareInitSQL("  "); err != nil || stmts != nil {
		t.Fatalf("blank: %v %v", stmts, err)
	}
	stmts, err := prepareInitSQL("CREATE TABLE users (id INTEGER); INSERT INTO users VALUES (1)")
	if err != nil || len(stmts) != 2 {
		t.Fatalf("ok: %v %v", stmts, err)
	}
	if _, err := prepareInitSQL("CREATE TABLE t (id INTEGER); DROP TABLE t"); !errors.Is(err, sqlguard.ErrSQLNotAllowed) {
		t.Fatalf("drop: %v", err)
	}
	if _, err := prepareInitSQL("SELECT 1"); !errors.Is(err, sqlguard.ErrSQLNotAllowed) {
		t.Fatalf("select: %v", err)
	}
	if _, err := prepareInitSQL("ATTACH 'x'"); !errors.Is(err, sqlguard.ErrSQLNotAllowed) {
		t.Fatalf("attach: %v", err)
	}
	if _, err := prepareInitSQL(strings.Repeat("a", maxInitSQLBytes+1)); err == nil {
		t.Fatal("expected size limit")
	}
}
