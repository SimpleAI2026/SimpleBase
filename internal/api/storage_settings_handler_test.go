package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
)

// newStorageTestHandler 构造指向临时 config.yaml 的 handler。
func newStorageTestHandler(t *testing.T, cfg config.Config, yamlContent string) (*settingsStorageHandler, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if yamlContent != "" {
		if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &settingsStorageHandler{cfg: cfg, path: path}, path
}

func storageTestConfig(t *testing.T) config.Config {
	cfg := config.Config{}
	cfg.Storage.Database = config.StorageModeLocal
	cfg.Storage.Files = config.StorageModeLocal
	cfg.Storage.DataDir = t.TempDir()
	return cfg
}

func TestStorageSettingsGet(t *testing.T) {
	h, _ := newStorageTestHandler(t, storageTestConfig(t), "storage:\n  database: local\n  files: local\n")
	e := echo.New()
	e.GET("/v1/system/storage", h.GetStorage)

	req := httptest.NewRequest(http.MethodGet, "/v1/system/storage", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var dto storageSettingsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.Database != "local" || dto.Files != "local" {
		t.Fatalf("modes: %+v", dto)
	}
	if dto.Source["database"] != "yaml" || dto.Source["files"] != "yaml" || dto.Source["data_dir"] != "default" {
		t.Fatalf("source: %+v", dto.Source)
	}
	if !dto.WritableConfig {
		t.Fatal("config file should be writable")
	}
}

func TestStorageSettingsPutRequiresConfirmAndSuperAdmin(t *testing.T) {
	h, _ := newStorageTestHandler(t, storageTestConfig(t), "storage:\n  database: local\n")
	e := echo.New()
	e.PUT("/v1/system/storage", h.PutStorage)

	put := func(principal auth.Principal, body string) int {
		req := httptest.NewRequest(http.MethodPut, "/v1/system/storage", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		ctx := req.Context()
		if principal.UserID != "" || principal.APIKeyID != "" {
			ctx = WithPrincipal(ctx, principal)
		}
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	super := auth.Principal{UserID: "u1", Username: "super", Role: auth.RoleSuperAdmin}
	admin := auth.Principal{UserID: "u2", Username: "admin", Role: auth.RoleAdmin}

	// 缺 confirm_reset → 400。
	if c := put(super, `{"database":"s3"}`); c != http.StatusBadRequest {
		t.Fatalf("missing confirm: %d", c)
	}
	// 非超管 → 403。
	if c := put(admin, `{"database":"s3","confirm_reset":true}`); c != http.StatusForbidden {
		t.Fatalf("admin put: %d", c)
	}
	// 非法值 → 400。
	if c := put(super, `{"database":"oss","confirm_reset":true}`); c != http.StatusBadRequest {
		t.Fatalf("bad mode: %d", c)
	}
	// 无主体 → 400。
	if c := put(super, `{"confirm_reset":true}`); c != http.StatusBadRequest {
		t.Fatalf("empty body: %d", c)
	}
	// 无 principal → 401。
	if c := put(auth.Principal{}, `{"database":"s3","confirm_reset":true}`); c != http.StatusUnauthorized {
		t.Fatalf("no principal: %d", c)
	}
}

func TestStorageSettingsPutPatchesYAML(t *testing.T) {
	yamlSrc := "# 顶部注释\nstorage:\n  database: local   # 行内注释\n  files: local\nhttp:\n  address: ':8080'\n"
	cfg := storageTestConfig(t)
	h, path := newStorageTestHandler(t, cfg, yamlSrc)
	e := echo.New()
	e.PUT("/v1/system/storage", h.PutStorage)

	req := httptest.NewRequest(http.MethodPut, "/v1/system/storage",
		strings.NewReader(`{"database":"s3","files":"s3","data_dir":"/tmp/sb-alt","confirm_reset":true}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(WithPrincipal(req.Context(),
		auth.Principal{UserID: "u1", Username: "super", Role: auth.RoleSuperAdmin}))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"database: s3", "files: s3", "data_dir: /tmp/sb-alt", "# 顶部注释", "# 行内注释", "address: ':8080'"} {
		if !strings.Contains(text, want) {
			t.Fatalf("patched yaml missing %q:\n%s", want, text)
		}
	}
	// 备份文件应存在且内容为原始版本。
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bak), "database: local   # 行内注释") {
		t.Fatalf("backup mismatch: %s", bak)
	}

	// 再 GET：确认接口可用且返回新模式。
	e2 := echo.New()
	e2.GET("/v1/system/storage", h.GetStorage)
	req2 := httptest.NewRequest(http.MethodGet, "/v1/system/storage", nil)
	rec2 := httptest.NewRecorder()
	e2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("get after put: %d %s", rec2.Code, rec2.Body.String())
	}
	var dto storageSettingsDTO
	if err := json.Unmarshal(rec2.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.Source["database"] != "yaml" {
		t.Fatalf("source after put: %+v", dto.Source)
	}
}

func TestStorageSettingsPutEnvConflict(t *testing.T) {
	t.Setenv("SIMPLEBASE_STORAGE_DATABASE", "local")
	cfg := storageTestConfig(t)
	h, _ := newStorageTestHandler(t, cfg, "storage:\n  database: local\n")
	e := echo.New()
	e.PUT("/v1/system/storage", h.PutStorage)

	req := httptest.NewRequest(http.MethodPut, "/v1/system/storage",
		strings.NewReader(`{"database":"s3","confirm_reset":true}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(WithPrincipal(req.Context(),
		auth.Principal{UserID: "u1", Username: "super", Role: auth.RoleSuperAdmin}))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("env conflict: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStorageSettingsPutMissingConfigFile(t *testing.T) {
	// config.yaml 不存在时按最小模板创建。
	cfg := storageTestConfig(t)
	h, path := newStorageTestHandler(t, cfg, "")
	e := echo.New()
	e.PUT("/v1/system/storage", h.PutStorage)

	req := httptest.NewRequest(http.MethodPut, "/v1/system/storage",
		strings.NewReader(`{"database":"s3","confirm_reset":true}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(WithPrincipal(req.Context(),
		auth.Principal{UserID: "u1", Username: "super", Role: auth.RoleSuperAdmin}))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put create: %d %s", rec.Code, rec.Body.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "database: s3") {
		t.Fatalf("created config: %s", raw)
	}
}

func TestYamlStorageKeysFrom(t *testing.T) {
	if got := yamlStorageKeysFrom(nil); len(got) != 0 {
		t.Fatalf("nil root: %v", got)
	}
	var root yaml.Node
	src := "storage:\n  database: local\n  data_dir: /d\nother: 1\n"
	if err := yamlUnmarshalForTest(src, &root); err != nil {
		t.Fatal(err)
	}
	got := yamlStorageKeysFrom(&root)
	if !got["database"] || !got["data_dir"] || got["files"] {
		t.Fatalf("keys: %v", got)
	}
}

// yamlUnmarshalForTest 隔离 gopkg.in/yaml.v3 导入，避免测试文件直接依赖。
func yamlUnmarshalForTest(src string, root *yaml.Node) error {
	return yaml.Unmarshal([]byte(src), root)
}
