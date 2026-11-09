package app

import (
	"os"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/config"
)

func writeFileForTest(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// baseStorageConfig 返回一个写入临时目录的本地存储配置。
func baseStorageConfig(dir string) config.Config {
	return config.Config{
		Storage: config.StorageConfig{
			Database: config.StorageModeLocal,
			Files:    config.StorageModeLocal,
			DataDir:  dir,
		},
	}
}

func TestCheckStorageIdentityFirstStartWrites(t *testing.T) {
	dir := t.TempDir()
	cfg := baseStorageConfig(dir)
	if err := checkStorageIdentity(cfg); err != nil {
		t.Fatal(err)
	}
	// 二次启动一致：通过。
	if err := checkStorageIdentity(cfg); err != nil {
		t.Fatal(err)
	}
	// 身份文件落在 data_dir/_system 下。
	if _, err := os.Stat(storageIdentityPath(dir)); err != nil {
		t.Fatalf("identity file: %v", err)
	}
}

func TestCheckStorageIdentityRejectsChange(t *testing.T) {
	dir := t.TempDir()
	cfg := baseStorageConfig(dir)
	if err := checkStorageIdentity(cfg); err != nil {
		t.Fatal(err)
	}

	// 切到远端：必须拒绝。
	remote := cfg
	remote.Storage.Database = config.StorageModeS3
	remote.S3.Bucket = "b"
	err := checkStorageIdentity(remote)
	if err == nil {
		t.Fatal("storage.database change must be rejected")
	}
	if !strings.Contains(err.Error(), "build.sh reset") {
		t.Fatalf("error should point at the reset command: %v", err)
	}

	// 换 bucket：必须拒绝。
	remote2 := cfg
	remote2.S3.Bucket = "other"
	if err := checkStorageIdentity(remote2); err == nil {
		t.Fatal("bucket change must be rejected")
	}

	// 换 prefix：必须拒绝。
	remote3 := cfg
	remote3.S3.Prefix = "sb/other"
	if err := checkStorageIdentity(remote3); err == nil {
		t.Fatal("prefix change must be rejected")
	}

	// 损坏的 identity 文件：覆盖重写，不阻塞启动。
	if err := writeFileForTest(storageIdentityPath(dir), []byte("{broken")); err != nil {
		t.Fatal(err)
	}
	if err := checkStorageIdentity(cfg); err != nil {
		t.Fatalf("corrupted identity should self-heal: %v", err)
	}
}

func TestNormalizeMode(t *testing.T) {
	if got := normalizeMode(""); got != config.StorageModeLocal {
		t.Fatalf("empty mode = %q", got)
	}
	if got := normalizeMode(" S3 "); got != config.StorageModeS3 {
		t.Fatalf("whitespace/case = %q", got)
	}
}
