package catalog

import (
	"context"
	"time"
)

// Repository 是 Catalog 持久化抽象。
//
// 安全约束（见 plan3.md）：
//   - 所有读取必须带 projectID 或先由 service 验证资源归属；
//     不存在 GetDatabase(id) 这种越权高风险 API。
//   - TransitionDatabase 使用 UPDATE ... WHERE status IN (...) 并检查 affected rows。
type Repository interface {
	CreateTenant(ctx context.Context, tenant Tenant) error
	CreateProject(ctx context.Context, project Project) error
	// ListProjectsByTenant 列出租户下全部项目（按 created_at）。
	ListProjectsByTenant(ctx context.Context, tenantID string) ([]Project, error)
	CreateDatabase(ctx context.Context, db Database) error
	GetDatabase(ctx context.Context, projectID, databaseID string) (Database, error)
	ListDatabases(ctx context.Context, projectID string, page Page) ([]Database, string, error)
	// ListDatabasesByKind 按类型列出数据库（admin 项目查询 kind=system 系统库）。
	ListDatabasesByKind(ctx context.Context, projectID, kind string, page Page) ([]Database, string, error)
	// ListDatabasesByStatuses 列出未软删、状态属于 statuses 的库。仅启动修复使用。
	ListDatabasesByStatuses(ctx context.Context, statuses []DatabaseStatus) ([]Database, error)
	TransitionDatabase(ctx context.Context, id string, from []DatabaseStatus, to DatabaseStatus, at time.Time) (Database, error)
	MarkDeleted(ctx context.Context, id string, at time.Time) error
	UpsertProviderConfig(ctx context.Context, cfg LLMProviderConfig) error
	ListEnabledProviders(ctx context.Context, projectID string) ([]LLMProviderConfig, error)
	AppendUsage(ctx context.Context, events []UsageEvent) error
	AppendOperation(ctx context.Context, op Operation) error
	// ListOperations 查询审计事件。projectID 可为空表示全部。
	ListOperations(ctx context.Context, projectID, databaseID string, limit int) ([]Operation, error)

	// LLM 供应商配置（plan8.md）。
	LLMProviderStore

	// 配额（plan9.md）。
	QuotaStore

	// Repository 必须同时提供 TenantProjectValidator，供 Service 校验资源归属。
	TenantProjectValidator
}

// QuotaStore 抽象 project 配额的持久化（plan9.md）。
type QuotaStore interface {
	GetQuota(ctx context.Context, projectID string) (ProjectQuota, error)
	UpsertQuota(ctx context.Context, quota ProjectQuota) error
	// SumUsageSince 汇总 project 在 [since, now] 区间内的用量。
	// 返回 LLM 请求数、LLM token 数、数据库存储字节数。
	SumUsageSince(ctx context.Context, projectID string, since time.Time) (UsageSummary, error)
}

// UsageSummary 是配额检查用的用量汇总。
type UsageSummary struct {
	LLMRequests     int64
	LLMTokens       int64
	DatabaseStorage int64
}

// LLMProviderStore 抽象 LLM 供应商配置的持久化（plan8.md）。
// 密钥以密文存储；读取时由 Service 层解密。
type LLMProviderStore interface {
	GetLLMProviders(ctx context.Context, projectID string) (LLMProviders, error)
	SetLLMProviders(ctx context.Context, projectID string, providers LLMProviders) error
}

// TenantProjectValidator 用于 service 校验 project 属于 tenant。
// 实现由 sql_repository 提供（查询 sys_projects 表）
type TenantProjectValidator interface {
	ProjectBelongsToTenant(ctx context.Context, projectID, tenantID string) (bool, error)
	// GetProjectTenant 返回 project 所属的 tenant ID；project 不存在返回 ErrNotFound。
	GetProjectTenant(ctx context.Context, projectID string) (string, error)
}
