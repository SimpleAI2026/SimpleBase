package objectstore

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 对齐 plan/planv2.0/databases-and-s3-plan.md：descriptor v2、平面 key 隔离、无 turso 用户库路径。

func TestPlan_DescriptorV2RequiresDuckLakeStorage(t *testing.T) {
	desc := &Descriptor{
		FormatVersion: DescriptorFormatVersion,
		TenantID:      "11111111-1111-1111-1111-111111111111",
		ProjectID:     "pro-test",
		DatabaseID:    "33333333-3333-3333-3333-333333333333",
		CreatedAt:     time.Now().UTC(),
		Engine:        "ducklake",
		DuckLakeStorage: DuckLakeStorage{
			Region: "us-east-1", Bucket: "b", Prefix: "p",
		},
		DataPrefix: "p",
		Status:     StatusReady,
	}
	if err := desc.Validate(); err != nil {
		t.Fatalf("valid v2 descriptor: %v", err)
	}
}

func TestPlan_DescriptorRejectsNonDuckLakeEngine(t *testing.T) {
	desc := &Descriptor{
		FormatVersion: DescriptorFormatVersion,
		TenantID:      "11111111-1111-1111-1111-111111111111",
		ProjectID:     "pro-test",
		DatabaseID:    "33333333-3333-3333-3333-333333333333",
		CreatedAt:     time.Now().UTC(),
		Engine:        "turso",
		DuckLakeStorage: DuckLakeStorage{
			Region: "us-east-1", Bucket: "b", Prefix: "p",
		},
		DataPrefix: "p",
		Status:     StatusReady,
	}
	if err := desc.Validate(); err == nil {
		t.Fatal("expected reject engine=turso")
	}
}

func TestPlan_PlaneAAndPlaneBPrefixesDoNotOverlap(t *testing.T) {
	kb := KeyBuilder{RootPrefix: "simplebase", Environment: "prod-1"}
	tenant := "11111111-1111-1111-1111-111111111111"
	dbID := "33333333-3333-3333-3333-333333333333"
	dbPrefix, err := kb.DatabasePrefix(tenant, dbID)
	if err != nil {
		t.Fatal(err)
	}
	dataPrefix, err := kb.DataPrefix(tenant, dbID)
	if err != nil {
		t.Fatal(err)
	}
	// 平面 A（用户文件）由 FileStore 使用独立 root，形如 {root}/{env}/files/...
	planeARoot := kb.base() + "/files"
	if strings.HasPrefix(planeARoot, dbPrefix) || strings.HasPrefix(dbPrefix, planeARoot) {
		t.Fatalf("plane A root %q overlaps database prefix %q", planeARoot, dbPrefix)
	}
	if !strings.HasPrefix(dataPrefix, dbPrefix) {
		t.Fatalf("data prefix %q should be under database prefix %q", dataPrefix, dbPrefix)
	}
	// 删库 DeletePrefix(dbPrefix) 不应匹配 files/
	if strings.HasPrefix(planeARoot+"/proj/x", dbPrefix+"/") {
		t.Fatal("user file path must not match database delete prefix")
	}
}

func TestPlan_NoTursoUserDatabasePackage(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// .../internal/objectstore/plan_databases_s3_test.go → repo root
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	tursoDir := filepath.Join(repoRoot, "internal", "database", "turso")
	if st, err := os.Stat(tursoDir); err == nil && st.IsDir() {
		t.Fatalf("plan DoD: turso user-db package must be removed, still exists: %s", tursoDir)
	}
	for _, name := range []string{"tursofactory.go", "localfactory.go", "memoryfactory.go"} {
		p := filepath.Join(repoRoot, "internal", "database", name)
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("plan DoD: legacy factory %s must be removed", name)
		}
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "internal", "jobs", "delete_database.go")); err == nil {
		t.Fatal("plan: DeleteDatabaseHandler file must stay removed")
	}
}
