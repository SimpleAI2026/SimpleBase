package objectstore

import (
	"errors"
	"fmt"
	"time"
)

// DescriptorFormatVersion 是当前 descriptor 的格式版本。
// 创建后只允许兼容升级；读取方必须拒绝不兼容的更高版本。
const DescriptorFormatVersion = 1

// TursoStorage 记录 Turso/libSQL 在 S3 上的存储配置摘要。
// 禁止包含任何密钥（AccessKey/SecretKey/KMSKeyID 仅存在于服务配置与 IAM）。
type TursoStorage struct {
	// Endpoint 为空表示使用 AWS 默认 S3 endpoint。
	Endpoint string `json:"endpoint,omitempty"`
	Region   string `json:"region"`
	Bucket   string `json:"bucket"`
	// Prefix 是该数据库在 S3 中的 data/ 前缀（由 KeyBuilder.DataPrefix 产生）。
	Prefix string `json:"prefix"`
	// ForcePathStyle 与 endpoint 配套，用于 MinIO 等兼容实现。
	ForcePathStyle bool `json:"force_path_style,omitempty"`
	// KMSKeyIDRef 仅保存密钥的引用标识（如 KMS key alias），不保存密钥本身。
	// 若为空则使用 bucket 默认加密。
	KMSKeyIDRef string `json:"kms_key_id_ref,omitempty"`
}

// Descriptor 描述一个 SimpleBase logical database 的平台元数据。
// 它由 SimpleBase 写入 S3 的 descriptor.json，创建后只允许兼容升级；
// 禁止用用户输入 name 作为 object key，name 仅存在 catalog。
//
// 必填字段（见 Validate）：format_version、tenant_id、project_id、database_id、
// created_at、turso_storage、data_prefix、status。
type Descriptor struct {
	FormatVersion int          `json:"format_version"`
	TenantID      string       `json:"tenant_id"`
	ProjectID     string       `json:"project_id"`
	DatabaseID    string       `json:"database_id"`
	Name          string       `json:"name"`          // 仅用于展示，不作为 key
	CreatedAt     time.Time    `json:"created_at"`
	TursoStorage  TursoStorage `json:"turso_storage"`
	DataPrefix    string       `json:"data_prefix"` // 等价于 TursoStorage.Prefix，冗余便于校验
	Status        string       `json:"status"`      // 见 StatusXxx 常量
	TursoVersion  string       `json:"turso_version,omitempty"`
}

// 状态机取值（与 plan.md 5.2 状态机一致）。
const (
	StatusCreating  = "creating"
	StatusOpening   = "opening"
	StatusReady     = "ready"
	StatusClosing   = "closing"
	StatusClosed    = "closed"
	StatusDegraded  = "degraded"
	StatusDeleting  = "deleting"
	StatusDeleted   = "deleted"
	StatusRecovering = "recovering"
)

// Validate 校验 descriptor 的关键字段。
// 调用时机：PutJSON 前校验；从 S3 读取后再次校验，防止被篡改或格式漂移。
func (d *Descriptor) Validate() error {
	if d == nil {
		return errors.New("objectstore: descriptor is nil")
	}
	if d.FormatVersion <= 0 {
		return errors.New("objectstore: descriptor.format_version must be positive")
	}
	if d.FormatVersion > DescriptorFormatVersion {
		return fmt.Errorf("objectstore: descriptor.format_version %d is newer than supported %d",
			d.FormatVersion, DescriptorFormatVersion)
	}
	if err := validateID("tenant_id", d.TenantID); err != nil {
		return err
	}
	if err := validateID("project_id", d.ProjectID); err != nil {
		return err
	}
	if err := validateID("database_id", d.DatabaseID); err != nil {
		return err
	}
	if d.CreatedAt.IsZero() {
		return errors.New("objectstore: descriptor.created_at is required")
	}
	if d.TursoStorage.Bucket == "" {
		return errors.New("objectstore: descriptor.turso_storage.bucket is required")
	}
	if d.TursoStorage.Region == "" {
		return errors.New("objectstore: descriptor.turso_storage.region is required")
	}
	if d.TursoStorage.Prefix == "" {
		return errors.New("objectstore: descriptor.turso_storage.prefix is required")
	}
	if d.DataPrefix == "" {
		return errors.New("objectstore: descriptor.data_prefix is required")
	}
	if d.DataPrefix != d.TursoStorage.Prefix {
		return fmt.Errorf("objectstore: descriptor.data_prefix %q != turso_storage.prefix %q",
			d.DataPrefix, d.TursoStorage.Prefix)
	}
	if d.Status == "" {
		return errors.New("objectstore: descriptor.status is required")
	}
	return nil
}

// State 是 SimpleBase 写入 state.json 的运维状态。
// 它只记录运维信息（最后打开、验证时间等），不参与数据库提交语义；
// state 写失败只记录 degraded，不伪造数据库提交结果。
type State struct {
	DatabaseID      string    `json:"database_id"`
	Status          string    `json:"status"`
	LastOpenedAt    time.Time `json:"last_opened_at,omitempty"`
	LastVerifiedAt  time.Time `json:"last_verified_at,omitempty"`
	LastPersistHint string    `json:"last_persist_hint,omitempty"` // "local" | "s3-confirmed" | "unknown"
	UpdatedAt       time.Time `json:"updated_at"`
}

// Validate 校验 state 字段一致性。
func (s *State) Validate() error {
	if s == nil {
		return errors.New("objectstore: state is nil")
	}
	if err := validateID("database_id", s.DatabaseID); err != nil {
		return err
	}
	if s.Status == "" {
		return errors.New("objectstore: state.status is required")
	}
	if s.UpdatedAt.IsZero() {
		return errors.New("objectstore: state.updated_at is required")
	}
	return nil
}
