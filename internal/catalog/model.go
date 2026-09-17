// Package catalog 实现首期单实例 Catalog。
//
// Catalog 只保存产品元数据与配置，不承担分布式选主。它使用实例级系统 DuckLake
// （sys_* 表），绝不能与用户 SQL 使用同一 database ID。用户 query 无法读取系统表。
//
// 设计原则（见 plan3.md）：
//   - 所有读取必须带 projectID 或先由 service 验证资源归属；
//     不存在 GetDatabase(id) 这种越权高风险 API。
//   - 状态转换通过 UPDATE ... WHERE status IN (...) 并检查 affected rows，防止并发错序。
//   - migration 只能前进；Catalog 启动失败则整个 server 不 ready。
package catalog

import (
	"time"
)

// DatabaseStatus 表示数据库生命周期状态（见 plan.md 5.2 状态机）。
type DatabaseStatus string

const (
	DatabaseCreating   DatabaseStatus = "creating"
	DatabaseOpening    DatabaseStatus = "opening"
	DatabaseReady      DatabaseStatus = "ready"
	DatabaseClosing    DatabaseStatus = "closing"
	DatabaseClosed     DatabaseStatus = "closed"
	DatabaseDegraded   DatabaseStatus = "degraded"
	DatabaseDeleting   DatabaseStatus = "deleting"
	DatabaseDeleted    DatabaseStatus = "deleted"
	DatabaseRecovering DatabaseStatus = "recovering"
)

// Tenant 是 SimpleBase 的租户。
type Tenant struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// Project 属于某个 Tenant，是资源隔离边界。
type Project struct {
	ID        string
	TenantID  string
	Name      string
	CreatedAt time.Time
}

// Database 描述一个 logical database 的 catalog 记录。
// 注意：StoragePrefix 由 KeyBuilder 产生的 data 前缀，不含用户输入 name。
type Database struct {
	ID            string
	TenantID      string
	ProjectID     string
	Name          string
	Kind          string // "user" | "system"；空视为 user
	Status        DatabaseStatus
	StoragePrefix string
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
	Kind         string // "database" | "llm"
	Provider     string
	Model        string
	InputTokens  int64
	OutputTokens int64
	CostMicros   int64
	RequestID    string
	OccurredAt   time.Time
}

// Operation 记录管理操作审计事件。
type Operation struct {
	ID          string
	DatabaseID  string
	ProjectID   string
	PrincipalID string
	Kind        string // "create" | "delete" | "open" | "close" | "restore" | ...
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
