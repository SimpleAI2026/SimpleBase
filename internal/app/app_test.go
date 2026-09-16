package app

import (
	"context"
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
	return config.Config{
		HTTP: config.HTTPConfig{
			Address:      ":0",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			IdleTimeout:  5 * time.Second,
		},
		Instance: config.InstanceConfig{ID: "test-instance", Writable: writable},
		Database: config.DatabaseConfig{
			CacheDir:    "/tmp/simplebase-test-cache",
			IdleTimeout: time.Minute,
			MaxOpen:     2,
			MaxIdle:     1,
		},
		S3:            s3,
		Catalog:       config.CatalogConfig{DatabaseID: "catalog"},
		Auth:          config.AuthConfig{APIKeyHashSecret: "secret"},
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
	// Writable 非 DevMode 实例必须配置 S3；缺少 bucket 时 assembleDeps 应直接失败。
	cfg := testConfig(true)
	cfg.S3.Bucket = ""
	_, err := NewWithRegistry(context.Background(), cfg, prometheus.NewRegistry())
	if err == nil {
		t.Fatal("expected New to fail when writable without bucket")
	}
}
