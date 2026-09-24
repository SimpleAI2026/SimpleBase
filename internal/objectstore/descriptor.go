package objectstore

import (
	"errors"
	"fmt"
	"time"
)

// DescriptorFormatVersion 是当前 descriptor 的格式版本。
// v1 为 Turso 时代（turso_storage）；v2 起为 DuckLake-only（ducklake_storage）。
// 创建后只允许兼容升级；读取方必须拒绝不兼容的更高版本，并拒绝 v1 存量。
const DescriptorFormatVersion = 2

// DuckLakeStorage 记录 DuckLake 在 S3 上的存储配置摘要（平面 B）。
// 禁止包含任何密钥（AccessKey/SecretKey 仅存在于服务配置与 IAM）。
type DuckLakeStorage struct {
	// Endpoint 为空表示使用 AWS 默认 S3 endpoint。
	Endpoint string `json:"endpoint,omitempty"`
	Region   string `json:"region"`
	Bucket   string `json:"bucket"`
	// Prefix 是该数据库在 S3 中的 data/ 前缀（由 KeyBuilder.DataPrefix 产生）。
	Prefix string `json:"prefix"`
	// ForcePathStyle 与 endpoint 配套，用于 MinIO 等兼容实现。
	ForcePathStyle bool `json:"force_path_style,omitempty"`
	// KMSKeyIDRef 仅保存密钥的引用标识（如 KMS key alias），不保存密钥本身。
	KMSKeyIDRef string `json:"kms_key_id_ref,omitempty"`
}

// Descriptor 描述一个 SimpleBase logical database 的平台元数据。
// 它由 SimpleBase 写入 S3 的 descriptor.json；禁止用用户输入 name 作为 object key。
//
// 必填字段（见 Validate）：format_version、tenant_id、project_id、database_id、
// created_at、ducklake_storage、data_prefix、status。
type Descriptor struct {
	FormatVersion   int             `json:"format_version"`
	TenantID        string          `json:"tenant_id"`
	ProjectID       string          `json:"project_id"`
	DatabaseID      string          `json:"database_id"`
	Name            string          `json:"name"` // 仅用于展示，不作为 key
	CreatedAt       time.Time       `json:"created_at"`
	DuckLakeStorage DuckLakeStorage `json:"ducklake_storage"`
	DataPrefix      string          `json:"data_prefix"` // 等价于 DuckLakeStorage.Prefix，冗余便于校验
	Status          string          `json:"status"`
	// Engine 固定 ducklake；写入时填充，读取时若非空且非 ducklake 则拒绝。
	Engine string `json:"engine,omitempty"`
}

// 状态机取值（与 plan.md 5.2 状态机一致）。
const (
	StatusCreating   = "creating"
	StatusOpening    = "opening"
	StatusReady      = "ready"
	StatusClosing    = "closing"
	StatusClosed     = "closed"
	StatusDegraded   = "degraded"
	StatusDeleting   = "deleting"
	StatusDeleted    = "deleted"
	StatusRecovering = "recovering"
)

// Validate 校验 descriptor 的关键字段。
// 调用时机：PutJSON 前校验；从 S3 读取后再次校验，防止被篡改或格式漂移。
// 存量 Turso（format_version<=1 或缺少 ducklake_storage）一律拒绝——产品决策：丢弃不迁移。
func (d *Descriptor) Validate() error {
	if d == nil {
		return errors.New("objectstore: descriptor is nil")
	}
	if d.FormatVersion <= 0 {
		return errors.New("objectstore: descriptor.format_version must be positive")
	}
	if d.FormatVersion < 2 {
		return errors.New("objectstore: descriptor format v1 (turso) is no longer supported; recreate the database on DuckLake")
	}
	if d.FormatVersion > DescriptorFormatVersion {
		return fmt.Errorf("objectstore: descriptor.format_version %d is newer than supported %d",
			d.FormatVersion, DescriptorFormatVersion)
	}
	if err := validateID("tenant_id", d.TenantID); err != nil {
		return err
	}
	if err := ValidateProjectID(d.ProjectID); err != nil {
		return err
	}
	if err := validateID("database_id", d.DatabaseID); err != nil {
		return err
	}
	if d.CreatedAt.IsZero() {
		return errors.New("objectstore: descriptor.created_at is required")
	}
	if d.Engine != "" && d.Engine != "ducklake" {
		return fmt.Errorf("objectstore: descriptor.engine %q is not supported (only ducklake)", d.Engine)
	}
	if d.DuckLakeStorage.Bucket == "" {
		return errors.New("objectstore: descriptor.ducklake_storage.bucket is required")
	}
	if d.DuckLakeStorage.Region == "" {
		return errors.New("objectstore: descriptor.ducklake_storage.region is required")
	}
	if d.DuckLakeStorage.Prefix == "" {
		return errors.New("objectstore: descriptor.ducklake_storage.prefix is required")
	}
	if d.DataPrefix == "" {
		return errors.New("objectstore: descriptor.data_prefix is required")
	}
	if d.DataPrefix != d.DuckLakeStorage.Prefix {
		return fmt.Errorf("objectstore: descriptor.data_prefix %q != ducklake_storage.prefix %q",
			d.DataPrefix, d.DuckLakeStorage.Prefix)
	}
	if d.Status == "" {
		return errors.New("objectstore: descriptor.status is required")
	}
	return nil
}
