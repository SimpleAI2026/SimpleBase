// 文件说明：keys_ducklake_test.go — internal/objectstore 的单元测试（TestDuckLakeCatalogKeys、TestMemoryBlobStoreListAndDeleteMany 等）
//
// 用法
//
//	go test ./internal/objectstore -run 'TestDuckLakeCatalogKeys|TestMemoryBlobStoreListAndDeleteMany'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/api, internal/app, internal/backup 等 6 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/objectstore/keys_ducklake_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] blob.go, blob_test.go, branches_test.go, client.go, client_test.go 等 24 个
//
// 主要符号
//
//	TestDuckLakeCatalogKeys(t *testing.T)
//	TestMemoryBlobStoreListAndDeleteMany(t *testing.T)

package objectstore

import (
	"context"
	"strings"
	"testing"
)

func TestDuckLakeCatalogKeys(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase"}
	proj := "prj12345"
	db := "33333333-3333-3333-3333-333333333333"

	duck, err := kb.DuckLakeSnapshotKey(proj, db, 7, 1, "duckdb")
	if err != nil || !strings.HasSuffix(duck, ".ducklake") {
		t.Fatalf("duckdb snapshot key = %s err=%v", duck, err)
	}
	if !strings.Contains(duck, "/prj12345/databases/"+db+"/catalog/snapshots/") {
		t.Fatalf("snapshot key layout = %s", duck)
	}
	def, _ := kb.DuckLakeSnapshotKey(proj, db, 7, 1, "")
	if def != duck {
		t.Fatalf("empty engine must default to duckdb: %s", def)
	}
	lite, err := kb.DuckLakeSnapshotKey(proj, db, 7, 1, "sqlite")
	if err != nil || !strings.HasSuffix(lite, ".sqlite") {
		t.Fatalf("sqlite snapshot key = %s err=%v", lite, err)
	}
	if _, err := kb.DuckLakeSnapshotKey(proj, db, 7, 1, "postgres"); err == nil {
		t.Fatal("unknown engine must fail")
	}

	if got := kb.InstanceFormatKey(); got != "simplebase/_system/instance-format.json" {
		t.Fatalf("instance format key = %s", got)
	}
	if got := kb.ResetLockKey(); got != "simplebase/_system/reset.lock" {
		t.Fatalf("reset lock key = %s", got)
	}
	if got := kb.RootListingPrefix(); got != "simplebase/" {
		t.Fatalf("root listing prefix = %s", got)
	}
	if got := (KeyBuilder{}).RootListingPrefix(); got != "" {
		t.Fatalf("empty root listing prefix = %q", got)
	}

	uri, err := kb.DuckLakeDataURI("bucket", proj, db)
	if err != nil {
		t.Fatal(err)
	}
	want := "s3://bucket/simplebase/" + proj + "/databases/" + db + "/data/"
	if uri != want {
		t.Fatalf("data uri = %s want %s", uri, want)
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
