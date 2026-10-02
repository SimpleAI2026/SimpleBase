package objectstore

import (
	"context"
	"strings"
	"testing"
)

func TestDuckLakeCatalogKeys(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase", Environment: "test"}
	tenant := "11111111-1111-1111-1111-111111111111"
	db := "33333333-3333-3333-3333-333333333333"

	duck, err := kb.DuckLakeSnapshotKey(tenant, db, 7, 1, "duckdb")
	if err != nil || !strings.HasSuffix(duck, ".ducklake") {
		t.Fatalf("duckdb snapshot key = %s err=%v", duck, err)
	}
	def, _ := kb.DuckLakeSnapshotKey(tenant, db, 7, 1, "")
	if def != duck {
		t.Fatalf("empty engine must default to duckdb: %s", def)
	}
	lite, err := kb.DuckLakeSnapshotKey(tenant, db, 7, 1, "sqlite")
	if err != nil || !strings.HasSuffix(lite, ".sqlite") {
		t.Fatalf("sqlite snapshot key = %s err=%v", lite, err)
	}
	if _, err := kb.DuckLakeSnapshotKey(tenant, db, 7, 1, "postgres"); err == nil {
		t.Fatal("unknown engine must fail")
	}

	if got := kb.InstanceFormatKey(); got != "simplebase/test/catalog/instance-format.json" {
		t.Fatalf("instance format key = %s", got)
	}
	if got := kb.ResetLockKey(); got != "simplebase/test/catalog/reset.lock" {
		t.Fatalf("reset lock key = %s", got)
	}
	if got := kb.TenantsPrefix(); got != "simplebase/test/tenants" {
		t.Fatalf("tenants prefix = %s", got)
	}

	uri, err := kb.DuckLakeDataURI("bucket", tenant, db)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "s3://bucket/") || !strings.HasSuffix(uri, "/data/") {
		t.Fatalf("data uri = %s", uri)
	}
}

func TestMemoryBlobStoreListAndDeleteMany(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryBlobStore()
	for _, k := range []string{"p/a", "p/b", "p/c", "q/x"} {
		_ = s.PutBytes(ctx, k, []byte(k), "")
	}
	var all []string
	cursor := ""
	for {
		keys, next, err := s.List(ctx, "p/", cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, keys...)
		if next == "" {
			break
		}
		cursor = next
	}
	if strings.Join(all, ",") != "p/a,p/b,p/c" {
		t.Fatalf("list = %v", all)
	}
	if err := s.DeleteMany(ctx, all); err != nil {
		t.Fatal(err)
	}
	if keys, _, _ := s.List(ctx, "", "", 0); len(keys) != 1 || keys[0] != "q/x" {
		t.Fatalf("after delete = %v", keys)
	}
}
