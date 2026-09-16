package objectstore

import (
	"fmt"
	"strings"
)

// KeyBuilder 生成 SimpleBase 在 S3 中的对象键。
//
// 安全约束：
//   - 所有 key 必须由 KeyBuilder 产生；HTTP 层不接受原始 S3 key。
//   - 只接受 UUID 形式的 tenantID/databaseID/backupID，禁止用用户输入 name 作为 key。
//   - 产生的 key 不含凭据、不携带 query string，禁止路径穿越（".."、前导"/"）。
//
// 布局见 plan2.md「对象约定」：
//
//	{root}/{env}/catalog/...
//	{root}/{env}/tenants/{tenantUUID}/databases/{databaseUUID}/
//	  descriptor.json
//	  state.json
//	  data/                        # DuckLake DATA_PATH（平面 B）
//	  backups/{backupUUID}/...
type KeyBuilder struct {
	RootPrefix  string // 例如 "simplebase"，对应 config.S3Config.Prefix
	Environment string // 例如 "prod"，用于多环境隔离
}

// uuidPattern 用于轻量校验 UUID（v4 形态：8-4-4-4-12 十六进制）。
// 不强校验版本位，只保证字符集与分段长度，避免引入额外依赖。
const uuidPatternLen = 36 // "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"

// validateID 校验 ID 是否为 UUID 形态。拒绝空串、非 UUID、含路径分隔符。
func validateID(name string, id string) error {
	if id == "" {
		return fmt.Errorf("objectstore: %s is empty", name)
	}
	if len(id) != uuidPatternLen {
		return fmt.Errorf("objectstore: %s must be a UUID, got length %d", name, len(id))
	}
	// 允许字符：十六进制 + 连字符
	for i, r := range id {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return fmt.Errorf("objectstore: %s has invalid UUID format at pos %d", name, i)
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return fmt.Errorf("objectstore: %s contains non-hex char %q", name, r)
			}
		}
	}
	if strings.Contains(id, "..") || strings.Contains(id, "/") {
		return fmt.Errorf("objectstore: %s contains path separator or traversal", name)
	}
	return nil
}

// joinKey 将若干已校验的分段拼成 S3 key，并保证：
//   - 不以 "/" 开头（避免 bucket 根路径歧义）
//   - 不出现连续 "/"
//   - 不出现 ".."
func joinKey(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		// 去掉前后多余 "/"，跳过空段
		p = strings.Trim(p, "/")
		if p == "" {
			continue
		}
		cleaned = append(cleaned, p)
	}
	return strings.Join(cleaned, "/")
}

// base 返回根前缀+环境的基础路径，已做清洗。
func (k KeyBuilder) base() string {
	return joinKey(k.RootPrefix, k.Environment)
}

// DatabasePrefix 返回某个数据库在 S3 中的完整前缀（不含尾部 "/"）。
// 调用方可在此前缀后追加 "data/" 等子路径（DuckLake 数据面）。
func (k KeyBuilder) DatabasePrefix(tenantID, databaseID string) (string, error) {
	if err := validateID("tenant_id", tenantID); err != nil {
		return "", err
	}
	if err := validateID("database_id", databaseID); err != nil {
		return "", err
	}
	return joinKey(k.base(), "tenants", tenantID, "databases", databaseID), nil
}

// DescriptorKey 返回 descriptor.json 的完整 key。
func (k KeyBuilder) DescriptorKey(tenantID, databaseID string) (string, error) {
	prefix, err := k.DatabasePrefix(tenantID, databaseID)
	if err != nil {
		return "", err
	}
	return prefix + "/descriptor.json", nil
}

// StateKey 返回 state.json 的完整 key。
func (k KeyBuilder) StateKey(tenantID, databaseID string) (string, error) {
	prefix, err := k.DatabasePrefix(tenantID, databaseID)
	if err != nil {
		return "", err
	}
	return prefix + "/state.json", nil
}

// DataPrefix 返回 data/ 子前缀（DuckLake DATA_PATH；本包不直接解析 Parquet）。
func (k KeyBuilder) DataPrefix(tenantID, databaseID string) (string, error) {
	prefix, err := k.DatabasePrefix(tenantID, databaseID)
	if err != nil {
		return "", err
	}
	return prefix + "/data", nil
}

// BackupPrefix 返回某次备份的对象前缀。
func (k KeyBuilder) BackupPrefix(tenantID, databaseID, backupID string) (string, error) {
	if err := validateID("tenant_id", tenantID); err != nil {
		return "", err
	}
	if err := validateID("database_id", databaseID); err != nil {
		return "", err
	}
	if err := validateID("backup_id", backupID); err != nil {
		return "", err
	}
	return joinKey(k.base(), "tenants", tenantID, "databases", databaseID, "backups", backupID), nil
}

// CatalogPrefix 返回 catalog 对象前缀。catalog 只能由唯一 Server 实例访问，
// 且使用与用户库不同的前缀段，防止用户 SQL 影响平台元数据。
func (k KeyBuilder) CatalogPrefix() string {
	return joinKey(k.base(), "catalog")
}

// DuckLakeCatalogKey 返回 DuckLake catalog.sqlite 的对象键。
func (k KeyBuilder) DuckLakeCatalogKey(tenantID, databaseID string) (string, error) {
	prefix, err := k.DatabasePrefix(tenantID, databaseID)
	if err != nil {
		return "", err
	}
	return prefix + "/catalog/catalog.sqlite", nil
}

// DuckLakeCatalogVersionKey 返回按快照 id 命名的 catalog 历史版本键。
func (k KeyBuilder) DuckLakeCatalogVersionKey(tenantID, databaseID string, snapshotID int64) (string, error) {
	if snapshotID <= 0 {
		return "", fmt.Errorf("objectstore: snapshot_id must be positive")
	}
	prefix, err := k.DatabasePrefix(tenantID, databaseID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/catalog/versions/%d.sqlite", prefix, snapshotID), nil
}

// DuckLakeDataURI 返回 DuckLake DATA_PATH（必须以 / 结尾的 s3 URI）。
func (k KeyBuilder) DuckLakeDataURI(bucket, tenantID, databaseID string) (string, error) {
	if bucket == "" {
		return "", fmt.Errorf("objectstore: bucket is required")
	}
	prefix, err := k.DataPrefix(tenantID, databaseID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s/", bucket, prefix), nil
}
