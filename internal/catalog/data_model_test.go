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
