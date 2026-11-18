package config

// r2_env_domains_test.go：planv6.0 R2-B1 env 侧表征测试（提取前先行）。
// 锁定 DuckLake 与 S3 两个 env 域：未设置保持默认、设置空串覆盖为空（LookupEnv 语义）、
// 非法数字保持原值、DEBOUNCE_MS 以毫秒解析。不并行（t.Setenv）。

import (
	"testing"
	"time"
)

func TestR2_EnvDuckLakeDomain(t *testing.T) {
	base := defaults()

	t.Setenv("SIMPLEBASE_DUCKLAKE_MEMORY_LIMIT", "")
	t.Setenv("SIMPLEBASE_DUCKLAKE_THREADS", "4")
	t.Setenv("SIMPLEBASE_DUCKLAKE_DATA_INLINING_ROW_LIMIT", "bad")
	t.Setenv("SIMPLEBASE_DUCKLAKE_SYNC_MODE", "async")
	t.Setenv("SIMPLEBASE_DUCKLAKE_SYNC_DEBOUNCE_MS", "250")
	t.Setenv("SIMPLEBASE_DUCKLAKE_SYNC_MAX_LAG", "7s")
	t.Setenv("SIMPLEBASE_DUCKLAKE_MAINT_REWRITE_DELETE_THRESHOLD", "0.5")

	cfg := loadFromEnv()
	d := cfg.Database.DuckLake
	if d.MemoryLimit != "" {
		t.Fatalf("empty env must override to empty: %q", d.MemoryLimit)
	}
	if d.Threads != 4 || d.CatalogSync.Mode != "async" || d.CatalogSync.MaxLag != 7*time.Second {
		t.Fatalf("values not applied: %+v", d)
	}
	if d.DataInliningRowLimit != base.Database.DuckLake.DataInliningRowLimit {
		t.Fatal("invalid int must keep default")
	}
	if d.CatalogSync.Debounce != 250*time.Millisecond {
		t.Fatalf("debounce=%v", d.CatalogSync.Debounce)
	}
	if d.Maintenance.RewriteDeleteThreshold != 0.5 {
		t.Fatalf("threshold=%v", d.Maintenance.RewriteDeleteThreshold)
	}
	if d.CatalogEngine != base.Database.DuckLake.CatalogEngine {
		t.Fatal("unset engine must keep default")
	}

	t.Setenv("SIMPLEBASE_DUCKLAKE_SYNC_DEBOUNCE_MS", "x")
	cfg = loadFromEnv()
	if cfg.Database.DuckLake.CatalogSync.Debounce != base.Database.DuckLake.CatalogSync.Debounce {
		t.Fatal("invalid debounce must keep default")
	}
}

func TestR2_EnvS3Domain(t *testing.T) {
	base := defaults()

	t.Setenv("SIMPLEBASE_S3_BUCKET", "bk")
	t.Setenv("SIMPLEBASE_S3_PREFIX", "")
	t.Setenv("SIMPLEBASE_S3_ACCESS_KEY", "ak")
	t.Setenv("SIMPLEBASE_S3_SECRET_KEY", "sk")
	t.Setenv("SIMPLEBASE_S3_FORCE_PATH_STYLE", "true")

	cfg := loadFromEnv()
	if cfg.S3.Bucket != "bk" || cfg.S3.AccessKey != "ak" || cfg.S3.SecretKey != "sk" || !cfg.S3.ForcePathStyle {
		t.Fatalf("values not applied: %+v", cfg.S3)
	}
	if cfg.S3.Prefix != "" {
		t.Fatalf("empty env must override prefix: %q", cfg.S3.Prefix)
	}
	if cfg.S3.Region != base.S3.Region || cfg.S3.Endpoint != base.S3.Endpoint {
		t.Fatal("unset fields must keep default")
	}
}
