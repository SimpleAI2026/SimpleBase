package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
	t.Setenv("SIMPLEBASE_AUTH_APIKEY_SECRET", "entry-secret")
	body := []byte(`
http:
  address: ":0"
instance:
  id: entry-dev
storage:
  database: local
  files: local
  data_dir: ` + dir + `/data
database:
  cache_dir: ` + dir + `/cache
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMPLEBASE_CONFIG_PATH", path)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Instance.ID != "entry-dev" || cfg.IsDBRemote() {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Storage.DataDir != dir+"/data" {
		t.Fatalf("storage: %+v", cfg.Storage)
	}
}

func TestRunDispatchWithoutServiceConfig(t *testing.T) {
	t.Setenv("SIMPLEBASE_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--endpoint", "http://localhost:8080", "--token", "test-token", "--project", "p", "database", "unknown"}, &stdout, &stderr); code != 1 {
		t.Fatalf("CLI exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `"code":"usage"`) || strings.Contains(stderr.String(), "missing.yaml") {
		t.Fatalf("CLI did not use its own config: %s", stderr.String())
	}
}

func TestRunServiceLoadsConfig(t *testing.T) {
	t.Setenv("SIMPLEBASE_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "missing.yaml") {
		t.Fatalf("service exit=%d stderr=%s", code, stderr.String())
	}
}

func TestRunResetRejectsInvalidFlags(t *testing.T) {
	t.Setenv("SIMPLEBASE_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"reset", "--unknown"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "missing.yaml") {
		t.Fatalf("reset config exit=%d stderr=%s", code, stderr.String())
	}
	stderr.Reset()
	if code := runReset(config.Config{}, []string{"--unknown"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("reset flag exit=%d stderr=%s", code, stderr.String())
	}
}
