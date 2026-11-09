package blob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testProject = "prj12345"
	testDB      = "33333333-3333-3333-3333-333333333333"
)

func TestDuckLakeCatalogKeys(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase"}

	duck, err := kb.DuckLakeSnapshotKey(testProject, testDB, 7, 1, "duckdb")
	if err != nil || !strings.HasSuffix(duck, ".ducklake") {
		t.Fatalf("duckdb snapshot key = %s err=%v", duck, err)
	}
	if !strings.Contains(duck, "/prj12345/databases/"+testDB+"/catalog/snapshots/") {
		t.Fatalf("snapshot key layout = %s", duck)
	}
	def, _ := kb.DuckLakeSnapshotKey(testProject, testDB, 7, 1, "")
	if def != duck {
		t.Fatalf("empty engine must default to duckdb: %s", def)
	}
	lite, err := kb.DuckLakeSnapshotKey(testProject, testDB, 7, 1, "sqlite")
	if err != nil || !strings.HasSuffix(lite, ".sqlite") {
		t.Fatalf("sqlite snapshot key = %s err=%v", lite, err)
	}
	if _, err := kb.DuckLakeSnapshotKey(testProject, testDB, 7, 1, "postgres"); err == nil {
		t.Fatal("unknown engine must fail")
	}
	if got := kb.InstanceFormatKey(); got != "simplebase/_system/instance-format.json" {
		t.Fatalf("instance format key = %s", got)
	}
	if got := kb.RootListingPrefix(); got != "simplebase/" {
		t.Fatalf("root listing prefix = %s", got)
	}
	if got := (KeyBuilder{}).RootListingPrefix(); got != "" {
		t.Fatalf("empty root listing prefix = %q", got)
	}
	uri, err := kb.DuckLakeDataURI("bucket", testProject, testDB)
	if err != nil {
		t.Fatal(err)
	}
	if want := "s3://bucket/simplebase/" + testProject + "/databases/" + testDB + "/data/"; uri != want {
		t.Fatalf("data uri = %s want %s", uri, want)
	}
}

func TestKeyBuilderRejectsBadIDs(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase"}
	cases := []struct{ name, project, db string }{
		{"short project", "p1", testDB},
		{"long project", "prj1234567", testDB},
		{"bad project char", "prj_1234", testDB},
		{"underscore system", "_system1", testDB},
		{"empty project", "", testDB},
		{"short db", testProject, "short"},
		{"empty db", testProject, ""},
		{"path traversal db", testProject, "../../etc/passwd0000000000000000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := kb.DatabasePrefix(tc.project, tc.db); err == nil {
				t.Fatalf("expected error for project=%q db=%q", tc.project, tc.db)
			}
		})
	}
}

func TestProjectDatabaseKeys(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "/sb/"}
	snap, err := kb.ProjectDatabaseSnapshotKey(testProject, 3, 2, ".sqlite")
	if err != nil || snap != "sb/prj12345/database/snapshots/00000000000000000003-00000000000000000002.sqlite" {
		t.Fatalf("snapshot = %q err=%v", snap, err)
	}
	if _, err := kb.ProjectDatabaseSnapshotKey(testProject, 0, 1, ".duckdb"); err == nil {
		t.Fatal("seq 0 must fail")
	}
	if _, err := kb.ProjectDatabaseSnapshotKey(testProject, 1, 1, ".bin"); err == nil {
		t.Fatal("unknown extension must fail")
	}
	mf, err := kb.ProjectDatabaseManifestKey(testProject, 7)
	if err != nil || mf != "sb/prj12345/database/manifest/00000000000000000007.json" {
		t.Fatalf("manifest = %q err=%v", mf, err)
	}
	ls, err := kb.ProjectDatabaseLeaseKey(testProject, 9)
	if err != nil || ls != "sb/prj12345/database/lease/00000000000000000009.json" {
		t.Fatalf("lease = %q err=%v", ls, err)
	}
	if _, err := kb.ProjectDatabaseLeaseKey(testProject, 0); err == nil {
		t.Fatal("epoch 0 must fail")
	}
	if _, err := kb.DuckLakeOrphanLedgerKey(testProject, testDB, "", "u"); err == nil {
		t.Fatal("empty date must fail")
	}
}

func TestValidateProjectID(t *testing.T) {
	for _, ok := range []string{"abcd1234", "ABC-1234"} {
		if err := ValidateProjectID(ok); err != nil {
			t.Fatalf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "short", "waytoolongid", "bad_id12", "ab..cd12", "ab/cd123"} {
		if err := ValidateProjectID(bad); err == nil {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}
}

func TestNewProjectIDIsValid(t *testing.T) {
	for i := 0; i < 32; i++ {
		id := NewProjectID()
		if err := ValidateProjectID(id); err != nil {
			t.Fatalf("NewProjectID produced invalid id %q: %v", id, err)
		}
	}
}

func TestMemoryRoundTrip(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	key := "tenants/x/catalog/catalog.sqlite"
	if err := s.PutBytes(ctx, key, []byte("sqlite-bytes"), "application/x-sqlite3"); err != nil {
		t.Fatal(err)
	}
	got, info, err := s.GetBytes(ctx, key)
	if err != nil || string(got) != "sqlite-bytes" || info.Size != 12 {
		t.Fatalf("got %q size %d err=%v", got, info.Size, err)
	}
	dest := filepath.Join(t.TempDir(), "sub", "catalog.sqlite")
	if _, err := s.DownloadFile(ctx, key, dest); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(dest); err != nil || string(b) != "sqlite-bytes" {
		t.Fatalf("download: %v %q", err, b)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Head(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("head after delete = %v, want ErrNotFound", err)
	}
	if _, err := s.DownloadFile(ctx, key, dest); !errors.Is(err, ErrNotFound) {
		t.Fatalf("download missing = %v", err)
	}
}

func TestMemoryPutIfAbsent(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	first, err := s.PutIfAbsent(ctx, "k", []byte("a"), "")
	if err != nil || first.ETag == "" {
		t.Fatalf("first: %+v err=%v", first, err)
	}
	if _, err := s.PutIfAbsent(ctx, "k", []byte("b"), ""); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("second = %v, want ErrPreconditionFailed", err)
	}
	if b, _, _ := s.GetBytes(ctx, "k"); string(b) != "a" {
		t.Fatalf("value overwritten: %q", b)
	}
}

func TestMemoryListAndDeleteMany(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
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
