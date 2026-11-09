// 文件说明：storage_settings_handler.go — internal/api 的内部实现（settingsStorageHandler、storageSettingsDTO 等）
//
// 用法
//
//	type settingsStorageHandler（包内使用）
//	type storageSettingsDTO（包内使用）
//	同包调用：snapshot() → database_handler.go, oai_responses.go；settingsStorageHandler → router.go；GetStorage() → router.go
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/storage_settings_handler.go
//	         │ 依赖
//	         ▼
//	[下游] internal/auth, internal/config, internal/observability
//	[同包] adapter.go, agent_handler.go, agent_schedule_handler.go, api_key_policy.go, api_keys_handler.go 等 59 个
//	[关键引用] snapshot() ← database_handler.go, oai_responses.go
//
// 主要符号
//
//	type settingsStorageHandler struct {…}
//	type storageSettingsDTO struct {…}
//	storageSource(envKey string, fromYAML bool) string
//	(h *settingsStorageHandler) GetStorage(c echo.Context) error
//	(h *settingsStorageHandler) snapshot() (storageSettingsDTO, error)
//	(h *settingsStorageHandler) yamlStorageKeys() (map[string]bool, error)
//	... 另有 6 个顶层符号

package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/config"
	"github.com/linkxzhou/SimpleBase/internal/observability"
)

// storageSettings 是持久层配置的读写入口。
//
// 语义（plan/planv6.0/storage-local-first-plan.md 阶段 E）：
//   - 存储模式以 config.yaml 为准，UI 只是修改配置的入口；
//   - 修改后需要「停服 → ./build.sh reset -y → ./build.sh dev」，不迁移数据；
//   - 只允许 superadminl1 修改，admin 只读。
type settingsStorageHandler struct {
	cfg    config.Config
	path   string
	logger observability.Logger
}

// storageSettingsDTO 是 GET 响应。
type storageSettingsDTO struct {
	Database string `json:"database"` // local | s3
	Files    string `json:"files"`    // local | s3
	DataDir  string `json:"data_dir"`
	CacheDir string `json:"cache_dir"`

	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`

	// Source 记录每个字段的实际来源，UI 据此禁用被环境变量接管的输入框。
	Source map[string]string `json:"source"`

	ConfigPath     string `json:"config_path"`
	WritableConfig bool   `json:"writable_config"`

	// DataPresent 指示本地/远端当前是否已有数据，用于提示需要 reset。
	DataPresent map[string]bool `json:"data_present"`
}

// effectiveSource 返回字段来源：env | yaml | default。
func storageSource(envKey string, fromYAML bool) string {
	if _, ok := os.LookupEnv(envKey); ok {
		return "env"
	}
	if fromYAML {
		return "yaml"
	}
	return "default"
}

// GetStorage 返回当前存储配置（admin 及以上可读）。
func (h *settingsStorageHandler) GetStorage(c echo.Context) error {
	dto, err := h.snapshot()
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, dto)
}

func (h *settingsStorageHandler) snapshot() (storageSettingsDTO, error) {
	cfg := h.cfg
	yamlKeys, err := h.yamlStorageKeys()
	if err != nil {
		return storageSettingsDTO{}, err
	}
	dto := storageSettingsDTO{
		Database:       cfg.EffectiveDBMode(),
		Files:          cfg.EffectiveFilesMode(),
		DataDir:        cfg.Storage.DataDir,
		CacheDir:       cfg.Database.CacheDir,
		Bucket:         cfg.S3.Bucket,
		Prefix:         strings.Trim(cfg.S3.Prefix, "/"),
		ConfigPath:     h.path,
		WritableConfig: false,
		Source: map[string]string{
			"database": storageSource("SIMPLEBASE_STORAGE_DATABASE", yamlKeys["database"]),
			"files":    storageSource("SIMPLEBASE_STORAGE_FILES", yamlKeys["files"]),
			"data_dir": storageSource("SIMPLEBASE_DATA_DIR", yamlKeys["data_dir"]),
		},
		DataPresent: map[string]bool{},
	}

	// 只有本进程能把配置写回磁盘时才允许改：容器内只读挂载 / 镜像内配置场景
	// 由配置项本身决定，这里用「文件存在且可写」做判断。
	if fi, statErr := os.Stat(h.path); statErr == nil && !fi.IsDir() {
		f, openErr := os.OpenFile(h.path, os.O_WRONLY, 0)
		if openErr == nil {
			dto.WritableConfig = true
			_ = f.Close()
		}
	}

	// 本地数据目录是否已有内容（决定是否需要提示 reset）。
	if entries, readErr := os.ReadDir(cfg.Storage.DataDir); readErr == nil && len(entries) > 0 {
		dto.DataPresent["local"] = true
	}
	if dto.Database == config.StorageModeS3 || dto.Files == config.StorageModeS3 {
		// 不访问远端：仅当 bucket 已配置时标注"远端可能有数据"，避免启动时联网。
		dto.DataPresent["s3"] = cfg.S3.Bucket != ""
	}
	return dto, nil
}

// yamlStorageKeys 读取 config.yaml 中显式出现的 storage 键，用于区分 yaml 与默认值。
func (h *settingsStorageHandler) yamlStorageKeys() (map[string]bool, error) {
	out := map[string]bool{}
	data, err := os.ReadFile(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return out, err
	}
	return yamlStorageKeysFrom(&root), nil
}

// yamlStorageKeysFrom 在文档节点中定位 storage 段并列出已出现的键。
func yamlStorageKeysFrom(root *yaml.Node) map[string]bool {
	out := map[string]bool{}
	if root == nil || len(root.Content) == 0 {
		return out
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		k, v := doc.Content[i], doc.Content[i+1]
		if k.Value != "storage" || v.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(v.Content); j += 2 {
			out[v.Content[j].Value] = true
		}
	}
	return out
}

// PutStorage 修改 storage.* 并写回 config.yaml（仅 superadminl1）。
//
// 保留原有注释与键顺序：用 yaml.Node 原地改值，不重新序列化整个文档。
func (h *settingsStorageHandler) PutStorage(c echo.Context) error {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return WriteError(c, auth.ErrMissingCredentials)
	}
	if principal.Role != auth.RoleSuperAdmin {
		return WriteError(c, NewAPIError(http.StatusForbidden, "forbidden",
			"only superadminl1 can change storage settings", RequestIDFromContext(c.Request().Context())))
	}

	var body struct {
		Database     *string `json:"database"`
		Files        *string `json:"files"`
		DataDir      *string `json:"data_dir"`
		ConfirmReset bool    `json:"confirm_reset"`
	}
	if err := c.Bind(&body); err != nil {
		return WriteError(c, err)
	}
	if !body.ConfirmReset {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request",
			"confirm_reset must be true: changing storage mode requires ./build.sh reset and wipes all data",
			RequestIDFromContext(c.Request().Context())))
	}

	// 校验目标值本身合法。
	want := map[string]string{}
	if body.Database != nil {
		want["database"] = strings.ToLower(strings.TrimSpace(*body.Database))
	}
	if body.Files != nil {
		want["files"] = strings.ToLower(strings.TrimSpace(*body.Files))
	}
	if body.DataDir != nil {
		want["data_dir"] = strings.TrimSpace(*body.DataDir)
	}
	if len(want) == 0 {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request",
			"nothing to update", RequestIDFromContext(c.Request().Context())))
	}
	for _, key := range []string{"database", "files"} {
		if v, ok := want[key]; ok && v != config.StorageModeLocal && v != config.StorageModeS3 {
			return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request",
				key+" must be local or s3", RequestIDFromContext(c.Request().Context())))
		}
	}
	if v, ok := want["data_dir"]; ok && v == "" {
		return WriteError(c, NewAPIError(http.StatusBadRequest, "invalid_request",
			"data_dir must not be empty", RequestIDFromContext(c.Request().Context())))
	}

	// 被环境变量接管的字段不允许通过文件修改（改了也不生效）。
	yamlKeys, err := h.yamlStorageKeys()
	if err != nil {
		return WriteError(c, err)
	}
	envKeys := map[string]string{
		"database": "SIMPLEBASE_STORAGE_DATABASE",
		"files":    "SIMPLEBASE_STORAGE_FILES",
		"data_dir": "SIMPLEBASE_DATA_DIR",
	}
	for key, val := range want {
		if storageSource(envKeys[key], yamlKeys[key]) != "env" {
			continue
		}
		// 已经由环境变量覆盖为同一值时不算冲突。
		if key == "database" && h.cfg.EffectiveDBMode() == val {
			delete(want, key)
			continue
		}
		if key == "files" && h.cfg.EffectiveFilesMode() == val {
			delete(want, key)
			continue
		}
		if key == "data_dir" && h.cfg.Storage.DataDir == val {
			delete(want, key)
			continue
		}
		return WriteError(c, NewAPIError(http.StatusConflict, "conflict",
			key+" is controlled by "+envKeys[key]+"; change it in .env instead",
			RequestIDFromContext(c.Request().Context())))
	}
	if len(want) == 0 {
		dto, err := h.snapshot()
		if err != nil {
			return WriteError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"settings": dto, "restart_required": true, "reset_required": true})
	}

	if err := h.patchYAML(want); err != nil {
		if errors.Is(err, errConfigNotWritable) {
			return WriteError(c, NewAPIError(http.StatusConflict, "conflict",
				"config file is not writable; set the values via environment variables instead",
				RequestIDFromContext(c.Request().Context())))
		}
		return WriteError(c, err)
	}

	if h.logger != nil {
		h.logger.Info("storage settings updated via API",
			zap.String("by", principal.Username),
			zap.Any("changes", want),
			zap.String("config_path", h.path))
	}

	dto, err := h.snapshot()
	if err != nil {
		return WriteError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"settings":         dto,
		"restart_required": true,
		"reset_required":   true,
		"commands":         []string{"./build.sh reset", "./build.sh dev"},
	})
}

var errConfigNotWritable = errors.New("config file is not writable")

// patchYAML 原地修改 storage.* 下的若干键，保留注释与顺序。
// 文件不存在时按最小模板创建。
func (h *settingsStorageHandler) patchYAML(changes map[string]string) error {
	if h.path == "" {
		return errConfigNotWritable
	}
	data, err := os.ReadFile(h.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("api: read config: %w", err)
	}

	var root yaml.Node
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("api: parse config: %w", err)
		}
	}
	if err := patchStorageNode(&root, changes); err != nil {
		return err
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("api: marshal config: %w", err)
	}

	dir := filepath.Dir(h.path)
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return errConfigNotWritable
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("api: write config tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("api: close config tmp: %w", err)
	}
	// 备份原文件后再替换：写坏了也能人工回滚。
	if len(data) > 0 {
		if err := os.WriteFile(h.path+".bak", data, 0o600); err != nil {
			_ = os.Remove(tmpName)
			return fmt.Errorf("api: backup config: %w", err)
		}
	}
	if err := os.Rename(tmpName, h.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("api: replace config: %w", err)
	}
	return nil
}

// patchStorageNode 在文档节点中定位/创建 storage 段并设置键值。
func patchStorageNode(root *yaml.Node, changes map[string]string) error {
	if root == nil {
		return errors.New("api: empty yaml root")
	}
	if root.Kind == 0 {
		root.Kind = yaml.DocumentNode
	}
	if len(root.Content) == 0 {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return errors.New("api: config root is not a mapping")
	}

	storage := findMappingValue(doc, "storage")
	if storage == nil {
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "storage"}
		storage = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc.Content = append(doc.Content, key, storage)
	}
	for k, v := range changes {
		if node := findMappingValue(storage, k); node != nil {
			node.Value = v
			if node.Tag == "" || node.Tag == "!!null" {
				node.Tag = "!!str"
			}
			continue
		}
		storage.Content = append(storage.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
	}
	return nil
}

// findMappingValue 返回 mapping 节点中指定 key 的值节点（不存在返回 nil）。
func findMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}
