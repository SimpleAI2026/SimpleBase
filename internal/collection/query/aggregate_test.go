// 文件说明：aggregate_test.go — 聚合查询在用户过滤之外追加 owner 条件的测试。
//
// 用法
//
//	go test ./internal/collection/query -run TestParseAggregateQueryFiltered
//
// 调用架构
//
//	[上游] 无（同包测试）
//	[本文件] internal/collection/query/aggregate_test.go
//	[下游] aggregate.go
//
// 主要符号
//
//	TestParseAggregateQueryFiltered

package query

import (
	"net/url"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/godatabase"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

func TestParseAggregateQueryFiltered(t *testing.T) {
	dialect, err := godatabase.DialectFor("")
	if err != nil {
		t.Fatal(err)
	}
	schema := &collection.Collection{Fields: []collection.Field{{Name: "status", Type: "text"}}}
	values := url.Values{"group_by": {"status"}, "metrics": {"count"}}
	cond := OwnerCondition(dialect, "owner_id")
	q, err := ParseAggregateQueryFiltered(values, "todos", schema, dialect, cond, []any{"user:1"})
	if err != nil {
		t.Fatalf("ParseAggregateQueryFiltered: %v", err)
	}
	if !strings.Contains(q.SQL, "json_extract_string(data, '$.owner_id') = ?") {
		t.Fatalf("owner condition missing in SQL: %s", q.SQL)
	}
	if len(q.Args) != 1 || q.Args[0] != "user:1" {
		t.Fatalf("unexpected args: %+v", q.Args)
	}
	if !strings.Contains(q.CountSQL, "json_extract_string(data, '$.owner_id') = ?") {
		t.Fatalf("owner condition missing in CountSQL: %s", q.CountSQL)
	}
	if len(q.CountArgs) != 1 || q.CountArgs[0] != "user:1" {
		t.Fatalf("unexpected count args: %+v", q.CountArgs)
	}
}

func TestParseAggregateQueryKeepsUserFilter(t *testing.T) {
	dialect, err := godatabase.DialectFor("")
	if err != nil {
		t.Fatal(err)
	}
	schema := &collection.Collection{Fields: []collection.Field{{Name: "status", Type: "text"}}}
	values := url.Values{"group_by": {"status"}, "metrics": {"count"}, "filter": {"status is not null"}}
	cond := OwnerCondition(dialect, "owner_id")
	q, err := ParseAggregateQueryFiltered(values, "todos", schema, dialect, cond, []any{"user:1"})
	if err != nil {
		t.Fatalf("ParseAggregateQueryFiltered: %v", err)
	}
	if !strings.Contains(q.SQL, "status") || !strings.Contains(q.SQL, "json_extract_string") {
		t.Fatalf("both filters expected: %s", q.SQL)
	}
	if len(q.Args) != 1 || q.Args[0] != "user:1" {
		t.Fatalf("unexpected args: %+v", q.Args)
	}
}
