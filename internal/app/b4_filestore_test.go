package app

// b4_filestore_test.go：planv6.0 B4「文件存储选择」块表征测试。
// 在提取私有装配方法前锁定旧实现行为（writable 提前返回之前、只读也装配 fileStore）：
//   1. storage.files=local：fileStore 为本地实现，objectStore 保持 nil；
//   2. 只读实例同样装配 fileStore（块位于 writable 提前返回之前）；
//   3. storage.files=s3 且无 bucket：返回 "storage.files=s3 requires s3.bucket"。
// S3 客户端创建成功路径依赖真实远端，不在表征范围
// （既有 TestAssembleDepsCreatesS3ClientsThenFailsBootstrap 已覆盖失败传播）。

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

func fileStoreTestConfig(t *testing.T, writable bool, files string) config.Config {
	t.Helper()
	cfg := testConfig(writable)
	cfg.Storage = config.StorageConfig{
		Database: config.StorageModeLocal,
		Files:    files,
		DataDir:  t.TempDir(),
	}
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = t.TempDir()
	return cfg
}

func TestB4_LocalFileStoreSelected(t *testing.T) {
	cfg := fileStoreTestConfig(t, true, config.StorageModeLocal)
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	if a.fileStore == nil {
		t.Fatal("fileStore must be assembled for local files mode")
	}
	if a.objectStore != nil {
		t.Fatalf("local mode must not create object store, got %T", a.objectStore)
	}
}

func TestB4_ReadOnlyAssemblesFileStore(t *testing.T) {
	cfg := fileStoreTestConfig(t, false, config.StorageModeLocal)
	a, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	if a.fileStore == nil {
		t.Fatal("readonly instance must still assemble fileStore (block sits before early return)")
	}
}

func TestB4_S3FilesWithoutBucketRejected(t *testing.T) {
	cfg := fileStoreTestConfig(t, false, config.StorageModeS3)
	_, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err == nil {
		t.Fatal("expected storage.files=s3 without bucket to fail")
	}
	if !strings.Contains(err.Error(), "storage.files=s3 requires s3.bucket") {
		t.Fatalf("unexpected error: %v", err)
	}
}
