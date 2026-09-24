package objectstore

import (
	"strings"
	"testing"
	"time"
)

func TestKeyBuilderDatabasePrefix(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase", Environment: "prod"}
	tenant := "11111111-1111-1111-1111-111111111111"
	db := "33333333-3333-3333-3333-333333333333"

	got, err := kb.DatabasePrefix(tenant, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "simplebase/prod/tenants/11111111-1111-1111-1111-111111111111/databases/33333333-3333-3333-3333-333333333333"
	if got != want {
		t.Errorf("DatabasePrefix = %q, want %q", got, want)
	}
	// 禁止路径穿越
	if strings.Contains(got, "..") {
		t.Errorf("prefix contains path traversal: %s", got)
	}
	// 禁止前导 "/"
	if strings.HasPrefix(got, "/") {
		t.Errorf("prefix has leading slash: %s", got)
	}
}

func TestKeyBuilderDescriptorKey(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase", Environment: "prod"}
	tenant := "11111111-1111-1111-1111-111111111111"
	db := "33333333-3333-3333-3333-333333333333"
	got, err := kb.DescriptorKey(tenant, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "/descriptor.json") {
		t.Errorf("descriptor key should end with /descriptor.json: %s", got)
	}
}

func TestValidateProjectIDAndNewProjectID(t *testing.T) {
	if err := ValidateProjectID(""); err == nil {
		t.Fatal("empty")
	}
	if err := ValidateProjectID("short"); err == nil {
		t.Fatal("too short")
	}
	if err := ValidateProjectID("toolong-id"); err == nil {
		t.Fatal("too long")
	}
	if err := ValidateProjectID("bad!char"); err == nil {
		t.Fatal("invalid char")
	}
	if err := ValidateProjectID("pro-test"); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if err := ValidateProjectID("AdminSys"); err != nil {
		t.Fatalf("valid mixed case: %v", err)
	}
	id := NewProjectID()
	if err := ValidateProjectID(id); err != nil {
		t.Fatalf("generated id invalid: %v (%v)", id, err)
	}
	if len(id) != ProjectIDLen {
		t.Fatalf("len=%d", len(id))
	}
}

func TestKeyBuilderRejectsNonUUID(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase", Environment: "prod"}
	cases := []struct {
		name   string
		tenant string
		db     string
	}{
		{"empty tenant", "", "33333333-3333-3333-3333-333333333333"},
		{"empty db", "11111111-1111-1111-1111-111111111111", ""},
		{"user name as id", "my-tenant-name", "33333333-3333-3333-3333-333333333333"},
		{"too short", "12345", "33333333-3333-3333-3333-333333333333"},
		{"with slash", "11111111-1111-1111-1111-1111111111/1", "33333333-3333-3333-3333-333333333333"},
		{"traversal", "..", "33333333-3333-3333-3333-333333333333"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := kb.DatabasePrefix(tc.tenant, tc.db); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestKeyBuilderTenantIsolation(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase", Environment: "prod"}
	t1 := "11111111-1111-1111-1111-111111111111"
	t2 := "22222222-2222-2222-2222-222222222222"
	db := "33333333-3333-3333-3333-333333333333"

	p1, _ := kb.DatabasePrefix(t1, db)
	p2, _ := kb.DatabasePrefix(t2, db)
	if p1 == p2 {
		t.Errorf("tenants share the same prefix: %s", p1)
	}
	// t2 的前缀不应是 t1 前缀的子串（除共享的 base 段）
	if strings.Contains(p1, t2) {
		t.Errorf("tenant 1 prefix leaks tenant 2 id: %s", p1)
	}
}

func TestKeyBuilderCatalogPrefix(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase", Environment: "prod"}
	got := kb.CatalogPrefix()
	want := "simplebase/prod/catalog"
	if got != want {
		t.Errorf("CatalogPrefix = %q, want %q", got, want)
	}
}

func TestKeyBuilderEnvironmentIsolation(t *testing.T) {
	kbProd := KeyBuilder{RootPrefix: "simplebase", Environment: "prod"}
	kbStaging := KeyBuilder{RootPrefix: "simplebase", Environment: "staging"}
	tenant := "11111111-1111-1111-1111-111111111111"
	db := "33333333-3333-3333-3333-333333333333"
	p1, _ := kbProd.DatabasePrefix(tenant, db)
	p2, _ := kbStaging.DatabasePrefix(tenant, db)
	if p1 == p2 {
		t.Errorf("environments share the same prefix: %s", p1)
	}
}

func TestDescriptorValidateRejectsFutureVersion(t *testing.T) {
	desc := &Descriptor{
		FormatVersion:   DescriptorFormatVersion + 1,
		TenantID:        "11111111-1111-1111-1111-111111111111",
		ProjectID:       "pro-test",
		DatabaseID:      "33333333-3333-3333-3333-333333333333",
		CreatedAt:       time.Now().UTC(),
		DuckLakeStorage: DuckLakeStorage{Region: "us-east-1", Bucket: "b", Prefix: "p"},
		DataPrefix:      "p",
		Status:          StatusCreating,
	}
	if err := desc.Validate(); err == nil {
		t.Error("expected error for future format version")
	}
}

func TestDescriptorValidateMismatchedPrefix(t *testing.T) {
	desc := &Descriptor{
		FormatVersion:   DescriptorFormatVersion,
		TenantID:        "11111111-1111-1111-1111-111111111111",
		ProjectID:       "pro-test",
		DatabaseID:      "33333333-3333-3333-3333-333333333333",
		CreatedAt:       time.Now().UTC(),
		DuckLakeStorage: DuckLakeStorage{Region: "us-east-1", Bucket: "b", Prefix: "p1"},
		DataPrefix:      "p2",
		Status:          StatusCreating,
	}
	if err := desc.Validate(); err == nil {
		t.Error("expected error for mismatched data_prefix")
	}
}

func TestDescriptorValidateRejectsV1Turso(t *testing.T) {
	desc := &Descriptor{
		FormatVersion: 1,
		TenantID:      "11111111-1111-1111-1111-111111111111",
		ProjectID:     "pro-test",
		DatabaseID:    "33333333-3333-3333-3333-333333333333",
		CreatedAt:     time.Now().UTC(),
		DataPrefix:    "p",
		Status:        StatusCreating,
	}
	if err := desc.Validate(); err == nil {
		t.Fatal("expected error rejecting turso format v1")
	}
}
