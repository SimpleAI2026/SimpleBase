// 文件说明：keys.go — internal/objectstore 中 ProjectIDLen、SystemPrefix 等符号的实现
//
// 用法
//
//	NewProjectID() string
//	ValidateProjectID(id string) error// ValidateProjectID 见 blob.ValidateProjectID
//	CatalogFileExt(engine string) (string, error)  // CatalogFileExt 见 blob.CatalogFileExt
//	同包调用：KeyBuilder → descriptor.go, filestore.go；ValidateProjectID() → descriptor.go；DataPrefix() → descriptor.go
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/objectstore/keys.go
//	         │ 依赖
//	         ▼
//	[下游] godatabase/blob
//	[同包] blob.go, client.go, content.go, content_local.go, content_s3.go 等 10 个
//	[关键引用] KeyBuilder ← descriptor.go, filestore.go
//
// 主要符号
//
//	const ProjectIDLen
//	const SystemPrefix
//	ValidateProjectID(id string) error
//	NewProjectID() string
//	CatalogFileExt(engine string) (string, error)
//	type KeyBuilder struct {…}
//	... 另有 25 个顶层符号

package objectstore

import (
	"fmt"
	"strings"

	"github.com/linkxzhou/SimpleBase/godatabase/blob"
)

// 项目 ID 与系统前缀的规则属于数据库运行时的磁盘契约，唯一实现在 godatabase/blob。
// 这里只保留同名转发，保持 objectstore 现有调用方不变。
const (
	ProjectIDLen = blob.ProjectIDLen
	SystemPrefix = blob.SystemPrefix
)

// ValidateProjectID 见 blob.ValidateProjectID。
func ValidateProjectID(id string) error { return blob.ValidateProjectID(id) }

// NewProjectID 见 blob.NewProjectID。
func NewProjectID() string { return blob.NewProjectID() }

// CatalogFileExt 见 blob.CatalogFileExt。
func CatalogFileExt(engine string) (string, error) { return blob.CatalogFileExt(engine) }

// KeyBuilder 生成 SimpleBase 的存储对象键。
//
// 数据库运行时相关的 key（项目快照 / manifest / 租约、DuckLake catalog、实例格式标记）
// 由内嵌的 blob.KeyBuilder 提供并随之提升；本类型只增加平台自己的 key
// （descriptor / 项目文件 / 平台对象 / CAS 探针）。
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

// DB 返回数据库运行时使用的 key 构造器（同一根前缀）。
func (k KeyBuilder) DB() blob.KeyBuilder { return blob.KeyBuilder{RootPrefix: k.RootPrefix} }

// 以下方法是 blob.KeyBuilder 的转发。不用结构体嵌入：保持 `KeyBuilder{RootPrefix: …}`
// 字面量写法不变（嵌入会让字段提升后无法这样构造）。

func (k KeyBuilder) ProjectPrefix(projectID string) (string, error) {
	return k.DB().ProjectPrefix(projectID)
}
func (k KeyBuilder) DatabasePrefix(projectID, databaseID string) (string, error) {
	return k.DB().DatabasePrefix(projectID, databaseID)
}
func (k KeyBuilder) DataPrefix(projectID, databaseID string) (string, error) {
	return k.DB().DataPrefix(projectID, databaseID)
}
func (k KeyBuilder) RootListingPrefix() string { return k.DB().RootListingPrefix() }
func (k KeyBuilder) InstanceFormatKey() string { return k.DB().InstanceFormatKey() }
func (k KeyBuilder) ProjectDatabasePrefix(projectID string) (string, error) {
	return k.DB().ProjectDatabasePrefix(projectID)
}
func (k KeyBuilder) ProjectDatabaseSnapshotKey(projectID string, seq, epoch int64, ext string) (string, error) {
	return k.DB().ProjectDatabaseSnapshotKey(projectID, seq, epoch, ext)
}
func (k KeyBuilder) ProjectDatabaseManifestKey(projectID string, seq int64) (string, error) {
	return k.DB().ProjectDatabaseManifestKey(projectID, seq)
}
func (k KeyBuilder) ProjectDatabaseLeaseKey(projectID string, epoch int64) (string, error) {
	return k.DB().ProjectDatabaseLeaseKey(projectID, epoch)
}
func (k KeyBuilder) DuckLakeSnapshotKey(projectID, databaseID string, snapshotID, writerEpoch int64, engine string) (string, error) {
	return k.DB().DuckLakeSnapshotKey(projectID, databaseID, snapshotID, writerEpoch, engine)
}
func (k KeyBuilder) DuckLakeManifestKey(projectID, databaseID string, seq int64) (string, error) {
	return k.DB().DuckLakeManifestKey(projectID, databaseID, seq)
}
func (k KeyBuilder) DuckLakeLeaseKey(projectID, databaseID string, epoch int64) (string, error) {
	return k.DB().DuckLakeLeaseKey(projectID, databaseID, epoch)
}
func (k KeyBuilder) DuckLakeOrphanLedgerKey(projectID, databaseID, date, fileUUID string) (string, error) {
	return k.DB().DuckLakeOrphanLedgerKey(projectID, databaseID, date, fileUUID)
}
func (k KeyBuilder) DuckLakeDataURI(bucket, projectID, databaseID string) (string, error) {
	return k.DB().DuckLakeDataURI(bucket, projectID, databaseID)
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

// uuidPatternLen 是 UUID 的固定长度（8-4-4-4-12 十六进制）。
const uuidPatternLen = 36

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

// DescriptorKey 返回 descriptor.json 的完整 key。
func (k KeyBuilder) DescriptorKey(projectID, databaseID string) (string, error) {
	prefix, err := k.DatabasePrefix(projectID, databaseID)
	if err != nil {
		return "", err
	}
	return prefix + "/descriptor.json", nil
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

// CatalogPrefix 返回实例级 CAS 探针前缀。
// 与 reset 锁、实例格式标记同属 _system/，但单独一段便于按需清理。
func (k KeyBuilder) CatalogPrefix() string {
	return joinKey(k.base(), SystemPrefix, "catalog")
}

// ResetLockKey 返回 reset 互斥锁对象键（ducklake-duckdb-catalog-plan §5.2）。
func (k KeyBuilder) ResetLockKey() string {
	return joinKey(k.base(), SystemPrefix, "reset.lock")
}
