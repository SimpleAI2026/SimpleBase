package objectstore

import (
	"strings"
	"testing"
)

func TestDuckLakeCatalogKeys(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase", Environment: "test"}
	tenant := "11111111-1111-1111-1111-111111111111"
	db := "33333333-3333-3333-3333-333333333333"
	cat, err := kb.DuckLakeCatalogKey(tenant, db)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cat, "/catalog/catalog.sqlite") {
		t.Fatalf("catalog key = %s", cat)
	}
	ver, err := kb.DuckLakeCatalogVersionKey(tenant, db, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ver, "/catalog/versions/7.sqlite") {
		t.Fatalf("version key = %s", ver)
	}
	uri, err := kb.DuckLakeDataURI("bucket", tenant, db)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "s3://bucket/") || !strings.HasSuffix(uri, "/data/") {
		t.Fatalf("data uri = %s", uri)
	}
}
