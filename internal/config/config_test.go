package config

import (
	"testing"
	"time"
)

func setRequiredEnvs(t *testing.T, writable bool, s3Incomplete bool) {
	t.Helper()
	t.Setenv("SIMPLEBASE_HTTP_ADDRESS", ":9090")
	t.Setenv("SIMPLEBASE_HTTP_READ_TIMEOUT", "10s")
	t.Setenv("SIMPLEBASE_HTTP_WRITE_TIMEOUT", "10s")
	t.Setenv("SIMPLEBASE_HTTP_IDLE_TIMEOUT", "10s")
	t.Setenv("SIMPLEBASE_INSTANCE_ID", "test-instance")
	t.Setenv("SIMPLEBASE_INSTANCE_WRITABLE", boolStr(writable))
	t.Setenv("SIMPLEBASE_DB_CACHE_DIR", "/tmp/simplebase-cache")
	t.Setenv("SIMPLEBASE_DB_IDLE_TIMEOUT", "1m")
	t.Setenv("SIMPLEBASE_DB_MAX_OPEN", "4")
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "test-secret")
	t.Setenv("SIMPLEBASE_LIMITS_MAX_REQUEST_BYTES", "1024")
	t.Setenv("SIMPLEBASE_LIMITS_MAX_QUERY_ROWS", "100")
	t.Setenv("SIMPLEBASE_LIMITS_QUERY_TIMEOUT", "5s")
	t.Setenv("SIMPLEBASE_LIMITS_MAX_CONCURRENT_QUERIES", "8")
	t.Setenv("SIMPLEBASE_LIMITS_MAX_BATCH_STATEMENTS", "50")
	t.Setenv("SIMPLEBASE_LIMITS_MAX_SQL_BYTES", "4096")

	if writable {
		t.Setenv("SIMPLEBASE_S3_REGION", "us-east-1")
		t.Setenv("SIMPLEBASE_S3_PREFIX", "simplebase")
		if !s3Incomplete {
			t.Setenv("SIMPLEBASE_S3_BUCKET", "test-bucket")
		} else {
			t.Setenv("SIMPLEBASE_S3_BUCKET", "")
		}
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestLoadRejectsEmptyAddress(t *testing.T) {
	t.Setenv("SIMPLEBASE_HTTP_ADDRESS", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestLoadRejectsInvalidEngine(t *testing.T) {
	setRequiredEnvs(t, false, false)
	t.Setenv("SIMPLEBASE_DB_ENGINE", "postgres")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid engine")
	}
}

func TestLoadRejectsEmptyCacheDir(t *testing.T) {
	setRequiredEnvs(t, false, false)
	t.Setenv("SIMPLEBASE_DB_CACHE_DIR", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for empty cache dir")
	}
}

func TestLoadRejectsWritableMissingS3Bucket(t *testing.T) {
	setRequiredEnvs(t, true, true)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for writable without s3 bucket")
	}
}

func TestLoadRejectsInvalidTimeout(t *testing.T) {
	setRequiredEnvs(t, false, false)
	t.Setenv("SIMPLEBASE_HTTP_READ_TIMEOUT", "0s")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestLoadSucceedsWithCompleteConfig(t *testing.T) {
	setRequiredEnvs(t, true, false)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTP.Address != ":9090" {
		t.Fatalf("unexpected address: %s", cfg.HTTP.Address)
	}
	if cfg.Limits.QueryTimeout != 5*time.Second {
		t.Fatalf("unexpected query timeout: %v", cfg.Limits.QueryTimeout)
	}
	if cfg.Database.Engine != EngineDuckLake {
		t.Fatalf("unexpected engine: %s", cfg.Database.Engine)
	}
	if cfg.Database.DuckLake.MemoryLimit != "512MB" {
		t.Fatalf("unexpected ducklake memory_limit: %s", cfg.Database.DuckLake.MemoryLimit)
	}
	if cfg.Database.DuckLake.CatalogSync.Mode != "debounce" {
		t.Fatalf("unexpected sync mode: %s", cfg.Database.DuckLake.CatalogSync.Mode)
	}
}

func TestRedactedOmitsSecrets(t *testing.T) {
	setRequiredEnvs(t, true, false)
	t.Setenv("SIMPLEBASE_S3_ACCESS_KEY", "AKIAEXAMPLE")
	t.Setenv("SIMPLEBASE_S3_SECRET_KEY", "supersecret")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	view := cfg.Redacted()
	s3View, ok := view["s3"].(map[string]any)
	if !ok {
		t.Fatal("s3 view missing")
	}
	if s3View["has_access_key"] != true {
		t.Errorf("expected has_access_key=true")
	}
	// 红队检查：Redacted 不得出现明文密钥
	for k, v := range s3View {
		if v == "AKIAEXAMPLE" || v == "supersecret" {
			t.Errorf("secret leaked in s3.%s", k)
		}
	}
}

func TestValidateRejectsLegacyEngines(t *testing.T) {
	for _, engine := range []string{"turso", "local", "sqlite"} {
		t.Setenv("SIMPLEBASE_INSTANCE_ID", "id")
		t.Setenv("SIMPLEBASE_DB_CACHE_DIR", "/tmp/c")
		t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "0123456789abcdef0123456789abcdef")
		t.Setenv("SIMPLEBASE_DB_ENGINE", engine)
		t.Setenv("SIMPLEBASE_INSTANCE_WRITABLE", "false")
		_, err := Load()
		if err == nil {
			t.Fatalf("engine %q: expected validation error", engine)
		}
	}
	t.Setenv("SIMPLEBASE_DB_ENGINE", "ducklake")
	t.Setenv("SIMPLEBASE_INSTANCE_WRITABLE", "false")
	if _, err := Load(); err != nil {
		t.Fatalf("ducklake should be accepted: %v", err)
	}
}
