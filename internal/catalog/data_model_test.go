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
	if IsSQLDataModel(Database{Kind: DatabaseKindSystem, DataModel: DataModelSQL}) {
		t.Fatal("system is not a sql data model")
	}
	if IsSQLDataModel(Database{Kind: DatabaseKindKV, DataModel: DataModelSQL}) {
		t.Fatal("kv is not a sql data model")
	}
	if EffectiveDataModel(Database{}) != DataModelCollection {
		t.Fatal("effective default")
	}
}
