// 文件说明：data_model_test.go — internal/catalog 的单元测试（TestNormalizeDataModel、TestIsSQLDataModel 等）
//
// 用法
//
//	go test ./internal/catalog -run 'TestNormalizeDataModel|TestIsSQLDataModel'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/app 等 10 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/catalog/data_model_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] always_open_test.go, cache.go, cache_test.go, create_invalidate_test.go, data_model.go 等 20 个
//
// 主要符号
//
//	TestNormalizeDataModel(t *testing.T)
//	TestIsSQLDataModel(t *testing.T)

package catalog

import "testing"

func TestNormalizeDataModel(t *testing.T) {
	got, err := NormalizeDataModel("  SQL ")
	if err != nil || got != DataModelSQL {
		t.Fatalf("sql: %q %v", got, err)
	}
	got, err = NormalizeDataModel("")
	if err != nil || got != DataModelCollection {
		t.Fatalf("empty: %q %v", got, err)
	}
	if _, err := NormalizeDataModel("document"); err == nil {
		t.Fatal("expected invalid data model")
	}
}

func TestIsSQLDataModel(t *testing.T) {
	if !IsSQLDataModel(Database{Kind: DatabaseKindUser, DataModel: DataModelSQL}) {
		t.Fatal("user sql")
	}
	if IsSQLDataModel(Database{Kind: DatabaseKindUser}) {
		t.Fatal("empty model is collection")
	}
	if !IsSQLDataModel(Database{Kind: DatabaseKindSystem}) {
		t.Fatal("system empty column is sql")
	}
	if !IsSQLDataModel(Database{Kind: DatabaseKindSystem, DataModel: DataModelCollection}) {
		t.Fatal("system collection column is still sql")
	}
	if EffectiveDataModel(Database{Kind: DatabaseKindSystem, DataModel: DataModelCollection}) != DataModelSQL {
		t.Fatal("system effective model")
	}
	if IsSQLDataModel(Database{Kind: DatabaseKindKV, DataModel: DataModelSQL}) {
		t.Fatal("kv is not a sql data model")
	}
	if EffectiveDataModel(Database{Kind: DatabaseKindKV, DataModel: DataModelSQL}) != DataModelSQL {
		t.Fatal("kv effective model follows the column")
	}
	if EffectiveDataModel(Database{}) != DataModelCollection {
		t.Fatal("effective default")
	}
}
