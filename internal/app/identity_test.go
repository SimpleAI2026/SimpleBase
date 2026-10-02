package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFileForTest(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func TestCheckInstanceIdentityFirstStartWrites(t *testing.T) {
	dir := t.TempDir()
	if err := checkInstanceIdentity(dir, "inst-1", "sb/prod"); err != nil {
		t.Fatal(err)
	}
	// 二次启动一致：通过。
	if err := checkInstanceIdentity(dir, "inst-1", "sb/prod"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckInstanceIdentityRejectsChange(t *testing.T) {
	dir := t.TempDir()
	if err := checkInstanceIdentity(dir, "inst-1", "sb/prod"); err != nil {
		t.Fatal(err)
	}
	err := checkInstanceIdentity(dir, "inst-2", "sb/prod")
	if err == nil {
		t.Fatal("instance id change must be rejected")
	}
	if !strings.Contains(err.Error(), "migration") {
		t.Fatalf("error should point to runbook: %v", err)
	}

	err2 := checkInstanceIdentity(dir, "inst-1", "sb/other")
	if err2 == nil {
		t.Fatal("prefix change must be rejected")
	}

	// 损坏的 identity 文件：覆盖重写，不阻塞启动。
	path := filepath.Join(dir, "system", "instance-identity.json")
	if err := writeFileForTest(path, []byte("{broken")); err != nil {
		t.Fatal(err)
	}
	if err := checkInstanceIdentity(dir, "inst-9", "sb/x"); err != nil {
		t.Fatalf("corrupted identity should self-heal: %v", err)
	}
}
