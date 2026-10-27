package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testEnv(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	return []string{"HOME=" + dir, "GOCLOUDAGENT_HOME=" + filepath.Join(dir, ".gocloudagent")}
}

// 配置落在 ~/.gocloudagent/config.json，权限必须收敛到 0600。
func TestSaveConfigEnforcesSecureMode(t *testing.T) {
	env := testEnv(t)
	path, err := SaveConfig(env, Config{Endpoint: "http://127.0.0.1:8080", Token: "sk-secret-value", ProjectID: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode = %04o, want 0600", perm)
	}
	// 目录也必须收敛（避免同机其他用户列目录）。
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir mode = %04o, want 0700", perm)
	}
	cfg, err := LoadConfig(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "sk-secret-value" || cfg.ProjectID != "p1" {
		t.Fatalf("round trip = %+v", cfg)
	}
}

// 组/其他用户可读的配置必须拒绝加载（不静默使用泄漏的凭据）。
func TestLoadConfigRejectsLooseMode(t *testing.T) {
	env := testEnv(t)
	path, err := SaveConfig(env, Config{Token: "sk-x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadConfig(env)
	if !errors.Is(err, ErrInsecureConfig) {
		t.Fatalf("err = %v, want ErrInsecureConfig", err)
	}
}

func TestLoadConfigMissingIsNotAnError(t *testing.T) {
	cfg, err := LoadConfig(testEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, Config{}) {
		t.Fatalf("cfg = %+v, want zero", cfg)
	}
}

// GOCLOUDAGENT_* 覆盖 SIMPLEBASE_*（命令行可临时切换目标）。
func TestMergeEnvPrecedence(t *testing.T) {
	env := []string{
		"SIMPLEBASE_URL=http://from-simplebase",
		"SIMPLEBASE_TOKEN=t1",
		"GOCLOUDAGENT_ENDPOINT=http://from-gocloudagent",
		"GOCLOUDAGENT_MODEL=m2",
		"GOCLOUDAGENT_YOLO=true",
		"GOCLOUDAGENT_MAX_ITERATIONS=3",
		"GOCLOUDAGENT_COMPACT_TRIGGER_TOKENS=99",
		"GOCLOUDAGENT_COMPACT_KEEP_TOKENS=7",
	}
	cfg := MergeEnv(Config{Endpoint: "http://from-file"}, env)
	if cfg.Endpoint != "http://from-gocloudagent" {
		t.Fatalf("endpoint = %q", cfg.Endpoint)
	}
	if cfg.Token != "t1" {
		t.Fatalf("token = %q", cfg.Token)
	}
	if cfg.Model != "m2" || !cfg.Yolo || cfg.MaxIterations != 3 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.CompactTriggerTokens != 99 || cfg.CompactKeepTokens != 7 {
		t.Fatalf("compaction overrides = %+v", cfg)
	}
	if cfg.Output != "text" {
		t.Fatalf("output default = %q, want text", cfg.Output)
	}
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"no endpoint", Config{Token: "t", ProjectID: "p"}, "endpoint"},
		{"no token", Config{Endpoint: "e", ProjectID: "p"}, "token"},
		{"no project", Config{Endpoint: "e", Token: "t"}, "project_id"},
		{"ok", Config{Endpoint: "e", Token: "t", ProjectID: "p"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want mention of %q", err, tc.want)
			}
		})
	}
}

func TestHomeDirRespectsOverride(t *testing.T) {
	if got := HomeDir([]string{"GOCLOUDAGENT_HOME=/tmp/ca"}); got != "/tmp/ca" {
		t.Fatalf("got %q", got)
	}
	if got := HomeDir([]string{"HOME=/tmp/home"}); got != "/tmp/home/.gocloudagent" {
		t.Fatalf("got %q", got)
	}
}

// overlayConfig 只覆盖显式给出的字段：Options.Config 未设的字段是零值，
// 不能把文件里已配好的值抹成空（否则纯文件配置的用户永远被判成「未配置」）。
func TestOverlayConfigKeepsFileValues(t *testing.T) {
	base := Config{
		Endpoint: "http://file", Token: "file-token", ProjectID: "file-project",
		Model: "file-model", Output: "json", SearchProvider: "brave",
		SearchAPIKey: "file-key", MaxIterations: 5,
	}
	got := overlayConfig(base, Config{})
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("empty override changed the base: %+v", got)
	}

	got = overlayConfig(base, Config{Endpoint: "http://flag", Yolo: true})
	if got.Endpoint != "http://flag" {
		t.Fatalf("endpoint = %q, want the explicit override", got.Endpoint)
	}
	if got.Token != base.Token || got.ProjectID != base.ProjectID || got.Model != base.Model || got.MaxIterations != 5 {
		t.Fatalf("unset fields should survive: %+v", got)
	}
	if !got.Yolo {
		t.Fatal("yolo override should apply")
	}
}
