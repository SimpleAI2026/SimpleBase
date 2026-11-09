// 文件说明：model.go — 首期单实例 Catalog
//
// 用法
//
//	type DatabaseStatus（导出类型，同包与上层均可使用）
//	type Tenant（导出类型，同包与上层均可使用）
//	同包调用：Database → cache.go, data_model.go 等；DatabaseStatus → errors.go, repository.go 等；Project → project_storage.go, repository.go 等
//
// 调用架构
//
//	[上游] benchmarks/dbread, internal/api, internal/app 等
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/catalog/model.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] cache.go, data_model.go, errors.go, project_storage.go, repository.go 等 9 个
//	[关键引用] Database ← cache.go, data_model.go, repository.go
//
// 主要符号
//
//	type DatabaseStatus string
//	const DatabaseCreating
//	const DatabaseReady
//	const DatabaseDegraded
//	const DatabaseDeleting
//	const DatabaseDeleted
//	... 另有 9 个顶层符号

package catalog

import (
	"time"
)

// DatabaseStatus 表示数据库生命周期状态。
// 用户可见：creating（仅创建请求内部）、ready、degraded、deleting、deleted。
type DatabaseStatus string

const (
	DatabaseCreating DatabaseStatus = "creating"
	DatabaseReady    DatabaseStatus = "ready"
	DatabaseDegraded DatabaseStatus = "degraded"
	DatabaseDeleting DatabaseStatus = "deleting"
	DatabaseDeleted  DatabaseStatus = "deleted"
)

// Tenant 是 SimpleBase 的租户。
type Tenant struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// Project 属于某个 Tenant，是资源隔离边界。
type Project struct {
	ID           string
	TenantID     string
	Name         string
	DatabasePath string
	CreatedAt    time.Time
}

// Database 描述一个 logical database 的 catalog 记录。
// 注意：StoragePrefix 由 KeyBuilder 产生的 data 前缀，不含用户输入 name。
type Database struct {
	ID        string
	TenantID  string
	ProjectID string
	Name      string
	Kind      string // "user" | "system"；空视为 user
	// DataModel 是用户库的数据形态："collection" | "sql"。
	// 空字符串在读取时视为 collection（迁移前的旧行）。系统库与 KV 不使用该字段做分流。
	DataModel     string
	Status        DatabaseStatus
	StoragePrefix string
	// DatabasePath 是项目级单文件的运行时位置，只从 sys_projects 补充，不写入 sys_databases。
	DatabasePath  string
	FormatVersion int
	DeletedAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// LLMProviderConfig 是 project 级的 LLM provider 配置引用。
// CredentialRef 指向密钥服务中的凭据标识，绝不保存 key 原文。
type LLMProviderConfig struct {
	ID                string
	ProjectID         string
	Provider          string
	CredentialRef     string
	Enabled           bool
	AllowedModelsJSON string
	DefaultModel      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// UsageEvent 是单次资源使用的计量事件。
// CostMicros 以微货币单位存储，避免浮点。
type UsageEvent struct {
	ID           string
	ProjectID    string
	Kind         string // "database" | "llm" | "sandbox"
	Provider     string
	Model        string
	InputTokens  int64
	OutputTokens int64
	CostMicros   int64
	DurationMs   int64 // sandbox 的执行时长；其他 kind 为零
	RequestID    string
	OccurredAt   time.Time
}

// Operation 记录管理操作审计事件。
type Operation struct {
	ID          string
	DatabaseID  string
	ProjectID   string
	PrincipalID string
	Kind        string // "create" | "delete" | ...
	RequestID   string
	Status      string // "ok" | "error"
	CreatedAt   time.Time
}

// Page 是分页参数。
type Page struct {
	Limit  int
	Cursor string // 不透明游标；空表示第一页
}

// LLMProviders 描述一个 project 的全部 LLM 供应商配置。
type LLMProviders struct {
	Default   string              `json:"default"`
	Providers []LLMProviderConfig `json:"providers"`
}

// ProjectQuota 是 project 维度的资源配额（plan9.md）。
// 零值表示不限制。数据库与 LLM 共用 project 维度但资源池独立。
type ProjectQuota struct {
	ProjectID       string
	MaxDatabases    int   // 最大数据库数；0 不限
	MaxStorageBytes int64 // 最大存储字节；0 不限
	MaxLLMRequests  int64 // 每周期最大 LLM 请求数；0 不限
	MaxLLMTokens    int64 // 每周期最大 LLM token 数；0 不限
	PeriodSeconds   int   // 配额周期秒数；默认 3600
	UpdatedAt       time.Time
}
