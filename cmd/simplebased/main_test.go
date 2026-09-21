package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/config"
)

func TestEntrypointLoadFailsOnMissingConfiguredYAML(t *testing.T) {
	t.Setenv("SIMPLEBASE_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv("SIMPLEBASE_INSTANCE_ID", "id")
	t.Setenv("SIMPLEBASE_DB_CACHE_DIR", t.TempDir())
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "s")
	t.Setenv("SIMPLEBASE_INSTANCE_WRITABLE", "false")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected missing yaml error")
	}
}

func TestEntrypointLoadYAMLOnlyDev(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`
http:
  address: ":0"
instance:
  id: entry-dev
database:
  cache_dir: ` + dir + `
auth:
  api_key_hash_secret: entry-secret
dev_mode: true
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Instance.ID != "entry-dev" || !cfg.DevMode {
		t.Fatalf("%+v", cfg)
	}
}
