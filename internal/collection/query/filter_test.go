// 文件说明：filter_test.go — internal/collection/query 的单元测试（TestCompileFilterGoldenSQL、TestCompileFilterErrorsKeepCodes 等）
//
// 用法
//
//	go test ./internal/collection/query -run 'TestCompileFilterGoldenSQL|TestCompileFilterErrorsKeepCodes'
//
// 调用架构
//
//	[上游] internal/api
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/collection/query/filter_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] aggregate.go, env.go, filter.go
//
// 主要符号
//
//	testSchema() *collection.Collection
//	TestCompileFilterGoldenSQL(t *testing.T)
//	TestCompileFilterErrorsKeepCodes(t *testing.T)

package query

// filter_test.go：filter → SQL 的金标用例（迁移自 internal/api 时期的行为基线；语法与文案逐字锁定）。
// 语法：比较 = != > >= < <=、文本包含 ~、in [...]、is null、逻辑 && || 与括号。

import (
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

func testSchema() *collection.Collection {
	return &collection.Collection{Name: "orders", Fields: []collection.Field{
		{Name: "id", Type: "string"}, {Name: "title", Type: "text"}, {Name: "amount", Type: "number"},
		{Name: "status", Type: "select"}, {Name: "note", Type: "json"},
	}}
}

func TestCompileFilterGoldenSQL(t *testing.T) {
	dialect, err := godatabase.DialectFor(godatabase.EngineSQLite)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		raw    string
		wantIn []string
		args   int
	}{
		{`status = "open"`, []string{`json_extract(data, '$.status') = ?`}, 1},
		{`amount > 10 && status != "closed"`, []string{` AND `, `CAST(json_extract(data, '$.amount') AS REAL) > ?`, `json_extract(data, '$.status') != ?`}, 2},
		{`amount >= 1 || amount <= 2`, []string{` OR `}, 2},
		{`status in ["a", "b"]`, []string{` IN (`}, 2},
		{`note is null`, []string{` IS NULL`}, 0},
		{`title ~ "op"`, []string{` LIKE `}, 1},
		{`(status = "a" || status = "b") && amount >= 1`, []string{`(`, ` OR `, ` AND `, `CAST(json_extract(data, '$.amount') AS REAL) >= ?`}, 3},
	}
	for _, tc := range cases {
		sql, args, err := CompileFilter(tc.raw, testSchema(), dialect)
		if err != nil {
			t.Fatalf("%q: %v", tc.raw, err)
		}
		for _, want := range tc.wantIn {
			if !strings.Contains(sql, want) {
				t.Fatalf("%q: sql = %s, want substring %q", tc.raw, sql, want)
			}
		}
		if len(args) != tc.args {
			t.Fatalf("%q: args = %#v, want %d", tc.raw, args, tc.args)
		}
	}
	if sql, args, err := CompileFilter("   ", testSchema(), dialect); sql != "" || args != nil || err != nil {
		t.Fatalf("empty filter must be a no-op: %q %#v %v", sql, args, err)
	}
}

func TestCompileFilterErrorsKeepCodes(t *testing.T) {
	dialect, _ := godatabase.DialectFor(godatabase.EngineSQLite)
	cases := []struct {
		raw     string
		code    string
		message string
	}{
		{`status =`, "invalid_filter", "expected value"},
		{`nope = 1`, "unknown_field", "field is not registered"},
		{`status = "a"; DROP TABLE t`, "invalid_filter", "invalid filter"},
		{`status in`, "invalid_filter", "in requires 1..32 values"},
		{`status in []`, "invalid_filter", ""},
		{`amount ~ "2"`, "invalid_filter", ""},
	}
	for _, tc := range cases {
		_, _, err := CompileFilter(tc.raw, testSchema(), dialect)
		verr, ok := err.(*collection.ValidationError)
		if !ok || verr.Code != tc.code || (tc.message != "" && verr.Message != tc.message) {
			t.Fatalf("%q: err = %#v, want code %q message %q", tc.raw, err, tc.code, tc.message)
		}
	}
}
