package objectstore

import "time"

// DescriptorFormatVersion 是当前 descriptor 的格式版本，存储配置使用 ducklake_storage。
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
// 写入字段包括 format_version、tenant_id、project_id、database_id、created_at、
// ducklake_storage、data_prefix、status。
type Descriptor struct {
	FormatVersion   int             `json:"format_version"`
	TenantID        string          `json:"tenant_id"`
	ProjectID       string          `json:"project_id"`
	DatabaseID      string          `json:"database_id"`
	Name            string          `json:"name"` // 仅用于展示，不作为 key
	CreatedAt       time.Time       `json:"created_at"`
	DuckLakeStorage DuckLakeStorage `json:"ducklake_storage"`
	DataPrefix      string          `json:"data_prefix"` // 与 DuckLakeStorage.Prefix 保持一致
	Status          string          `json:"status"`
	// Engine 固定为 ducklake。
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
