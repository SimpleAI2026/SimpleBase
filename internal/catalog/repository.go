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
	TransitionDatabase(ctx context.Context, id string, from []DatabaseStatus, to DatabaseStatus, at time.Time) (Database, error)
	MarkDeleted(ctx context.Context, id string, at time.Time) error
	UpsertProviderConfig(ctx context.Context, cfg LLMProviderConfig) error
	ListEnabledProviders(ctx context.Context, projectID string) ([]LLMProviderConfig, error)
	AppendUsage(ctx context.Context, events []UsageEvent) error
	AppendOperation(ctx context.Context, op Operation) error
	// ListOperations 查询审计事件。projectID 可为空表示全部。
	ListOperations(ctx context.Context, projectID, databaseID string, limit int) ([]Operation, error)

	// Job 队列（plan7.md）：Enqueue/Claim/Complete/Retry/MarkDeadLetter/ListJobs。
	JobQueue

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

// JobQueue 抽象后台任务的持久化与领取（plan7.md）。
// 实现必须保证 Claim 的原子性（单实例下用事务 + UPDATE...WHERE status='pending'）。
type JobQueue interface {
	Enqueue(ctx context.Context, job Job) error
	// Claim 原子地把最早一个 status=pending 且 run_after<=now 的任务置为 running，
	// 返回该任务。无可用任务返回 ErrNotFound。
	Claim(ctx context.Context, workerID string, now time.Time) (Job, error)
	// Complete 标记任务为 completed。任务必须处于 running 状态。
	Complete(ctx context.Context, id string) error
	// Retry 记录失败原因，attempt+1，状态回 pending，run_after=next。
	// 超过 max_attempts 则标记 dead_letter。
	Retry(ctx context.Context, id string, lastErr string, next time.Time) error
	// GetJob 按 ID 查询任务（供 API 查询状态）。
	GetJob(ctx context.Context, id string) (Job, error)
	// ListJobs 返回指定 database 的任务（按 created_at 降序，限 limit 条）。
	ListJobs(ctx context.Context, databaseID string, limit int) ([]Job, error)
}

// TenantProjectValidator 用于 service 校验 project 属于 tenant。
// 实现由 sqlite_repository 提供（查询 projects 表）
type TenantProjectValidator interface {
	ProjectBelongsToTenant(ctx context.Context, projectID, tenantID string) (bool, error)
	// GetProjectTenant 返回 project 所属的 tenant ID；project 不存在返回 ErrNotFound。
	GetProjectTenant(ctx context.Context, projectID string) (string, error)
}
