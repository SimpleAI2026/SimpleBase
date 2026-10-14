package objectstore

import (
	"strings"
	"testing"
)

const (
	testProject  = "prj12345"
	testProject2 = "prj67890"
	testDB       = "33333333-3333-3333-3333-333333333333"
)

func TestKeyBuilderDatabasePrefix(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase"}
	got, err := kb.DatabasePrefix(testProject, testDB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "simplebase/" + testProject + "/databases/" + testDB
	if got != want {
		t.Fatalf("DatabasePrefix = %q, want %q", got, want)
	}
}

func TestKeyBuilderDescriptorKey(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase"}
	got, err := kb.DescriptorKey(testProject, testDB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "simplebase/" + testProject + "/databases/" + testDB + "/descriptor.json"
	if got != want {
		t.Fatalf("DescriptorKey = %q, want %q", got, want)
	}
}

func TestKeyBuilderEmptyRoot(t *testing.T) {
	kb := KeyBuilder{}
	got, err := kb.DatabasePrefix(testProject, testDB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, testProject+"/databases/") {
		t.Fatalf("empty root prefix = %q", got)
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

func TestKeyBuilderProjectIsolation(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase"}
	p1, err := kb.DatabasePrefix(testProject, testDB)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := kb.DatabasePrefix(testProject2, testDB)
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 || strings.HasPrefix(p1, p2) || strings.HasPrefix(p2, p1) {
		t.Fatalf("projects share the same prefix: %q vs %q", p1, p2)
	}
}

func TestKeyBuilderSystemAndCatalogPrefix(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase"}
	if got := kb.CatalogPrefix(); got != "simplebase/_system/catalog" {
		t.Fatalf("CatalogPrefix = %q", got)
	}
	if got := kb.SystemPrefix(); got != "simplebase/_system" {
		t.Fatalf("SystemPrefix = %q", got)
	}
	if got := kb.SystemKey("reset.lock"); got != "simplebase/_system/reset.lock" {
		t.Fatalf("SystemKey = %q", got)
	}
	if got := kb.InstanceFormatKey(); got != "simplebase/_system/instance-format.json" {
		t.Fatalf("InstanceFormatKey = %q", got)
	}
	if got := kb.ResetLockKey(); got != "simplebase/_system/reset.lock" {
		t.Fatalf("ResetLockKey = %q", got)
	}
}

func TestKeyBuilderFilesPrefix(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase"}
	got, err := kb.FilesPrefix(testProject)
	if err != nil {
		t.Fatal(err)
	}
	if got != "simplebase/"+testProject+"/files" {
		t.Fatalf("FilesPrefix = %q", got)
	}
}

func TestValidateProjectID(t *testing.T) {
	if err := ValidateProjectID("abcd1234"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProjectID("ABC-1234"); err != nil {
		t.Fatal(err)
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
