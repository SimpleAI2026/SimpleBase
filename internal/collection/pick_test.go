// 文件说明：pick_test.go — internal/collection 的单元测试（TestParseFieldsAndPickGolden 等）
//
// 用法
//
//	go test ./internal/collection -run 'TestParseFieldsAndPickGolden'
//
// 调用架构
//
//	[上游] internal/api, internal/app, internal/collection/query
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/collection/pick_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] events.go, fieldtypes.go, fieldtypes_test.go, model.go, pick.go 等 7 个
//
// 主要符号
//
//	TestParseFieldsAndPickGolden(t *testing.T)

package collection

// pick_test.go：fields 投影的金标用例（迁移自 internal/api 时期的行为基线；文案逐字锁定）。

import (
	"reflect"
	"testing"
)

func TestParseFieldsAndPickGolden(t *testing.T) {
	schema := &Collection{Name: "orders", Fields: []Field{
		{Name: "id", Type: "string"}, {Name: "amount", Type: "number"}, {Name: "secret", Type: "string", Hidden: true},
	}}
	fields, err := ParseFields(" amount , id ", schema)
	// 输入顺序无关：按 schema 顺序输出。
	if err != nil || !reflect.DeepEqual(fields, []string{"id", "amount"}) {
		t.Fatalf("fields = %#v err=%v", fields, err)
	}
	if all, err := ParseFields("", schema); err != nil || all != nil {
		t.Fatalf("empty fields must select all: %v", err)
	}
	// 隐藏字段可以选（只是响应默认不展示）；未登记 / 形态非法的字段被拒。
	if _, err := ParseFields("secret", schema); err != nil {
		t.Fatalf("hidden field must stay selectable: %v", err)
	}
	for raw, code := range map[string]string{"nope": "unknown_field", "1bad": "invalid_fields"} {
		_, err := ParseFields(raw, schema)
		verr, ok := err.(*ValidationError)
		if !ok || verr.Code != code {
			t.Fatalf("%q: err = %#v, want code %q", raw, err, code)
		}
	}
	doc := map[string]any{"id": "a", "amount": 2, "secret": "x", "extra": true}
	// 系统列（id / created_at / updated_at）由调用方另行拼接，投影只取数据字段；嵌套字段按路径还原。
	// 文档里不存在的嵌套路径直接跳过。
	if got := PickDocument(doc, []string{"amount", "meta.owner.name"}); !reflect.DeepEqual(got, map[string]any{"amount": 2}) {
		t.Fatalf("pick = %#v", got)
	}
	if got := PickDocument(map[string]any{"a": map[string]any{"b": 1}}, []string{"a.b"}); !reflect.DeepEqual(got, map[string]any{"a": map[string]any{"b": 1}}) {
		t.Fatalf("nested pick = %#v", got)
	}
	if got := HideDocumentFields(doc, []string{"secret"}); got["secret"] != nil || got["id"] != "a" || got["extra"] != true {
		t.Fatalf("hide = %#v", got)
	}
	if v, ok := NestedValue(map[string]any{"a": map[string]any{"b": 1}}, []string{"a", "b"}); !ok || v != 1 {
		t.Fatalf("nested = %v %v", v, ok)
	}
}

