package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

func testConfig(writable bool) config.Config {
	s3 := config.S3Config{Region: "us-east-1", Bucket: "b", Prefix: "simplebase"}
	if !writable {
		s3 = config.S3Config{}
	}
	storage := config.StorageConfig{
		Database: config.StorageModeLocal,
		Files:    config.StorageModeLocal,
		DataDir:  "/tmp/simplebase-test-data",
	}
	if writable {
		// 可写实例校验路径：走远端模式以覆盖 S3 分支。
		storage.Database = config.StorageModeS3
	}
	return config.Config{
		HTTP: config.HTTPConfig{
			Address:      ":0",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			IdleTimeout:  5 * time.Second,
		},
		Storage:  storage,
		Instance: config.InstanceConfig{ID: "test-instance", Writable: writable},
		Database: config.DatabaseConfig{
			CacheDir:    "/tmp/simplebase-test-cache",
			IdleTimeout: time.Minute,
			MaxOpen:     2,
		},
		S3:   s3,
		Auth: config.AuthConfig{APIKeyHashSecret: "secret"},
		Limits: config.LimitsConfig{
			MaxRequestBytes:      1 << 20,
			MaxQueryRows:         100,
			QueryTimeout:         5 * time.Second,
			MaxConcurrentQueries: 4,
			MaxBatchStatements:   10,
			MaxSQLBytes:          4096,
		},
		Observability: config.ObservabilityConfig{
			LogLevel: "debug", LogFormat: "json", MetricsPath: "/metrics",
		},
	}
}

func TestNewReadonlyApp(t *testing.T) {
	cfg := testConfig(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, err := NewWithRegistry(ctx, cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if a == nil || a.echo == nil {
		t.Fatal("app or echo nil")
	}
	// 立即 shutdown 验证不泄漏
	sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer scancel()
	if err := a.Shutdown(sctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestNewWritableAppRejectsMissingS3(t *testing.T) {
	// 远端模式的可写实例必须配置 S3；缺少 bucket 时 Validate/assembleDeps 应失败。
	cfg := testConfig(true)
	cfg.S3.Bucket = ""
	_, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err == nil {
		t.Fatal("expected New to fail when s3 storage without bucket")
	}
}

func TestNewLocalStorageAppUsesSystemDatabase(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(true)
	cfg.Storage = config.StorageConfig{
		Database: config.StorageModeLocal,
		Files:    config.StorageModeLocal,
		DataDir:  filepath.Join(dir, "data"),
	}
	cfg.S3 = config.S3Config{}
	cfg.Database.CacheDir = filepath.Join(dir, "cache")
	ctx := context.Background()
	a, err := NewWithRegistry(ctx, cfg, prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("New local storage: %v", err)
	}
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(sctx)
	})
	if a.systemStore == nil {
		t.Fatal("expected system store")
	}
	if err := a.systemStore.Ping(ctx); err != nil {
		t.Fatalf("system ping: %v", err)
	}
	// 数据必须落在 data_dir，缓存目录只放可丢弃内容。
	sysMeta := a.systemStore.Meta()
	dataCatalog := filepath.Join(cfg.Storage.DataDir, sysMeta.ProjectID, "databases", sysMeta.ID, "catalog")
	if _, err := os.Stat(dataCatalog); err != nil {
		t.Fatalf("expected catalog under data_dir %s: %v", dataCatalog, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Database.CacheDir, "system", "dbs", sysMeta.ID, "catalog")); err == nil {
		t.Fatal("catalog must not live in cache_dir")
	}
}
