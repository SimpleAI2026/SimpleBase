package objectstore

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// KeyBuilder 生成 SimpleBase 的存储对象键。
//
// 同一套相对布局同时用于本地持久根（`storage.data_dir`）与 S3 桶：
//
//	{root}/_system/...                            # 平台级对象（项目 ID 不会以下划线开头）
//	{root}/{projectID}/databases/{databaseUUID}/  # 库前缀
//	  descriptor.json
//	  catalog/...                                 # catalog（本地活文件 / S3 快照+manifest）
//	  data/                                       # DuckLake DATA_PATH
//	  orphans/...
//	{root}/{projectID}/files/{userKey}            # 用户文件
//
// 安全约束：
//   - 所有 key 必须由 KeyBuilder 产生；HTTP 层不接受原始 key。
//   - 只接受固定形态的 projectID 与 UUID 形式的 databaseID，禁止用用户输入 name 作 key。
//   - 产生的 key 不含凭据、不携带 query string，禁止路径穿越（".."、前导 "/"）。
type KeyBuilder struct {
	// RootPrefix 是可选根前缀，对应 config.S3Config.Prefix；为空表示根目录。
	RootPrefix string
}

// uuidPatternLen 是 UUID 的固定长度（8-4-4-4-12 十六进制）。
const uuidPatternLen = 36

// ProjectIDLen 是项目 ID 的固定长度（字母/数字/连字符）。
const ProjectIDLen = 8

// SystemPrefix 是平台级对象的目录名。项目 ID 长度固定且仅含 [A-Za-z0-9-]，
// 因此不会与 "_system" 冲突。
const SystemPrefix = "_system"

// projectIDAlphabet 是项目 ID 生成字符集（不含大写，便于 URL/路径）。
const projectIDAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// ValidateProjectID 校验项目 ID：恰好 8 位 [A-Za-z0-9-]，且无路径穿越。
func ValidateProjectID(id string) error {
	if id == "" {
		return fmt.Errorf("objectstore: project_id is empty")
	}
	if len(id) != ProjectIDLen {
		return fmt.Errorf("objectstore: project_id must be %d chars, got length %d", ProjectIDLen, len(id))
	}
	for i, r := range id {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-'
		if !ok {
			return fmt.Errorf("objectstore: project_id contains invalid char %q at pos %d", r, i)
		}
	}
	if strings.Contains(id, "..") || strings.Contains(id, "/") {
		return fmt.Errorf("objectstore: project_id contains path separator or traversal")
	}
	return nil
}

// NewProjectID 生成随机 8 位项目 ID（[a-z0-9]）。
func NewProjectID() string {
	b := make([]byte, ProjectIDLen)
	if _, err := rand.Read(b); err != nil {
		/* v8 ignore next 2 -- crypto/rand 失败极罕见，退化为固定长度占位 */
		return "proj-fb0"
	}
	for i := range b {
		b[i] = projectIDAlphabet[int(b[i])%len(projectIDAlphabet)]
	}
	return string(b)
}

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

// joinKey 将若干已校验的分段拼成 key，并保证：
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

// base 返回根前缀，已做清洗。为空表示根目录。
func (k KeyBuilder) base() string {
	return strings.Trim(k.RootPrefix, "/")
}

// ProjectPrefix 返回某个项目在存储中的前缀（不含尾部 "/"）。
func (k KeyBuilder) ProjectPrefix(projectID string) (string, error) {
	if err := ValidateProjectID(projectID); err != nil {
		return "", err
	}
	return joinKey(k.base(), projectID), nil
}

// DatabasePrefix 返回某个数据库的完整前缀（不含尾部 "/"）。
// 调用方可在此前缀后追加 "data/" 等子路径（DuckLake 数据面）。
func (k KeyBuilder) DatabasePrefix(projectID, databaseID string) (string, error) {
	if err := ValidateProjectID(projectID); err != nil {
		return "", err
	}
	if err := validateID("database_id", databaseID); err != nil {
		return "", err
	}
	return joinKey(k.base(), projectID, "databases", databaseID), nil
}

// DescriptorKey 返回 descriptor.json 的完整 key。
func (k KeyBuilder) DescriptorKey(projectID, databaseID string) (string, error) {
	prefix, err := k.DatabasePrefix(projectID, databaseID)
	if err != nil {
		return "", err
	}
	return prefix + "/descriptor.json", nil
}

// DataPrefix 返回 data/ 子前缀（DuckLake DATA_PATH；本包不直接解析 Parquet）。
func (k KeyBuilder) DataPrefix(projectID, databaseID string) (string, error) {
	prefix, err := k.DatabasePrefix(projectID, databaseID)
	if err != nil {
		return "", err
	}
	return prefix + "/data", nil
}

// FilesPrefix 返回某个项目的用户文件前缀（不含尾部 "/"）。
func (k KeyBuilder) FilesPrefix(projectID string) (string, error) {
	prefix, err := k.ProjectPrefix(projectID)
	if err != nil {
		return "", err
	}
	return prefix + "/files", nil
}

// SystemPrefix 返回平台级对象的完整前缀（不含尾部 "/"）。
// 承载实例格式标记、reset 锁、CAS 探针等不属于任何项目的对象。
func (k KeyBuilder) SystemPrefix() string {
	return joinKey(k.base(), SystemPrefix)
}

// SystemKey 返回平台级对象的完整 key。
func (k KeyBuilder) SystemKey(name string) string {
	name = strings.Trim(name, "/")
	return joinKey(k.base(), SystemPrefix, name)
}

// RootListingPrefix 返回存储根的列举前缀（带尾部 "/"）。
// 空根前缀时返回 ""（列举整个桶）。供"根下是否已有数据"的探测使用。
func (k KeyBuilder) RootListingPrefix() string {
	base := k.base()
	if base == "" {
		return ""
	}
	return base + "/"
}

// CatalogPrefix 返回实例级 CAS 探针前缀。
// 与 reset 锁、实例格式标记同属 _system/，但单独一段便于按需清理。
func (k KeyBuilder) CatalogPrefix() string {
	return joinKey(k.base(), SystemPrefix, "catalog")
}

// InstanceFormatKey 返回实例级 catalog 格式标记对象键（ducklake-duckdb-catalog-plan §4.1）。
// 与 CAS 探针同在 _system/ 下；PutIfAbsent 创建，只有 reset 能删除。
func (k KeyBuilder) InstanceFormatKey() string {
	return joinKey(k.base(), SystemPrefix, "instance-format.json")
}

// ResetLockKey 返回 reset 互斥锁对象键（ducklake-duckdb-catalog-plan §5.2）。
func (k KeyBuilder) ResetLockKey() string {
	return joinKey(k.base(), SystemPrefix, "reset.lock")
}

// DuckLakeSnapshotKey 返回不可变快照对象键（multi-instance-consistency-plan §4.4）：
// snapshots/{snapshot_id:020d}-{writer_epoch:020d}.{ext}。
// ext 由 catalog engine 决定（duckdb→.ducklake，sqlite→.sqlite）。
// 两个 writer 即使分配到同一 snapshot_id，epoch 不同则 key 不同，结构上无法互相覆盖。
func (k KeyBuilder) DuckLakeSnapshotKey(projectID, databaseID string, snapshotID, writerEpoch int64, engine string) (string, error) {
	if snapshotID <= 0 {
		return "", fmt.Errorf("objectstore: snapshot_id must be positive")
	}
	if writerEpoch < 0 {
		return "", fmt.Errorf("objectstore: writer_epoch must be non-negative")
	}
	ext, err := CatalogFileExt(engine)
	if err != nil {
		return "", err
	}
	prefix, err := k.DatabasePrefix(projectID, databaseID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/catalog/snapshots/%020d-%020d%s", prefix, snapshotID, writerEpoch, ext), nil
}

// CatalogFileExt 返回 catalog 引擎对应的文件后缀（含点）。
// 未知引擎返回错误，防止生成无校验的 key。
func CatalogFileExt(engine string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "duckdb", "":
		return ".ducklake", nil
	case "sqlite":
		return ".sqlite", nil
	default:
		return "", fmt.Errorf("objectstore: unknown catalog engine %q", engine)
	}
}

// DuckLakeManifestKey 返回 manifest 序列对象键：manifest/{seq:020d}.json。
// seq 单调递增；writer 推进 seq 用 PutIfAbsent，冲突即 split-brain 确证。
func (k KeyBuilder) DuckLakeManifestKey(projectID, databaseID string, seq int64) (string, error) {
	if seq <= 0 {
		return "", fmt.Errorf("objectstore: manifest seq must be positive")
	}
	prefix, err := k.DatabasePrefix(projectID, databaseID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/catalog/manifest/%020d.json", prefix, seq), nil
}

// DuckLakeLeaseKey 返回 per-database 写租约对象键：lease/{epoch:020d}.json。
func (k KeyBuilder) DuckLakeLeaseKey(projectID, databaseID string, epoch int64) (string, error) {
	if epoch <= 0 {
		return "", fmt.Errorf("objectstore: lease epoch must be positive")
	}
	prefix, err := k.DatabasePrefix(projectID, databaseID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/catalog/lease/%020d.json", prefix, epoch), nil
}

// DuckLakeOrphanLedgerKey 返回孤儿账本对象键：orphans/{date}/{uuid}.json。
func (k KeyBuilder) DuckLakeOrphanLedgerKey(projectID, databaseID, date, fileUUID string) (string, error) {
	prefix, err := k.DatabasePrefix(projectID, databaseID)
	if err != nil {
		return "", err
	}
	if date == "" || fileUUID == "" {
		return "", fmt.Errorf("objectstore: orphan ledger date and file uuid are required")
	}
	return fmt.Sprintf("%s/orphans/%s/%s.json", prefix, date, fileUUID), nil
}

// DuckLakeDataURI 返回 DuckLake DATA_PATH（必须以 / 结尾的 s3 URI）。
func (k KeyBuilder) DuckLakeDataURI(bucket, projectID, databaseID string) (string, error) {
	if bucket == "" {
		return "", fmt.Errorf("objectstore: bucket is required")
	}
	prefix, err := k.DataPrefix(projectID, databaseID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s/", bucket, prefix), nil
}
