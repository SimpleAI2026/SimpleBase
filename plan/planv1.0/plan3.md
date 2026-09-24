<!-- status: completed -->
<!-- verified: 2026-07-19 catalog 测试通过 -->

# Plan 3：Catalog、项目隔离与领域状态

## 目标
实现首期单实例 Catalog。它只保存产品元数据与配置，不承担分布式选主。Catalog 使用独立的系统数据库和独立 S3 前缀，绝不能与用户 SQL 使用同一 database ID。

## 目录骨架
```text
internal/catalog/model.go
internal/catalog/repository.go
internal/catalog/service.go
internal/catalog/migrations.go
internal/catalog/errors.go
internal/catalog/sqlite_repository.go
internal/catalog/service_test.go
```

## 领域类型（`model.go`）
```go
type DatabaseStatus string
const (
  DatabaseCreating DatabaseStatus = "creating"
  DatabaseOpening  DatabaseStatus = "opening"
  DatabaseReady    DatabaseStatus = "ready"
  DatabaseClosing  DatabaseStatus = "closing"
  DatabaseClosed   DatabaseStatus = "closed"
  DatabaseDegraded DatabaseStatus = "degraded"
  DatabaseDeleting DatabaseStatus = "deleting"
  DatabaseDeleted  DatabaseStatus = "deleted"
)
type Tenant struct { ID, Name string; CreatedAt time.Time }
type Project struct { ID, TenantID, Name string; CreatedAt time.Time }
type Database struct { ID, TenantID, ProjectID, Name string; Status DatabaseStatus; StoragePrefix string; FormatVersion int; DeletedAt *time.Time; CreatedAt, UpdatedAt time.Time }
type LLMProviderConfig struct { ID, ProjectID, Provider, CredentialRef string; Enabled bool; AllowedModelsJSON string; DefaultModel string; CreatedAt, UpdatedAt time.Time }
type UsageEvent struct { ID, ProjectID, Kind, Provider, Model string; InputTokens, OutputTokens int64; CostMicros int64; RequestID string; OccurredAt time.Time }
```

## Repository 接口
```go
type Repository interface {
  CreateTenant(ctx context.Context, tenant Tenant) error
  CreateProject(ctx context.Context, project Project) error
  CreateDatabase(ctx context.Context, db Database) error
  GetDatabase(ctx context.Context, projectID, databaseID string) (Database, error)
  ListDatabases(ctx context.Context, projectID string, page Page) ([]Database, string, error)
  TransitionDatabase(ctx context.Context, id string, from []DatabaseStatus, to DatabaseStatus, at time.Time) (Database, error)
  MarkDeleted(ctx context.Context, id string, at time.Time) error
  UpsertProviderConfig(ctx context.Context, cfg LLMProviderConfig) error
  ListEnabledProviders(ctx context.Context, projectID string) ([]LLMProviderConfig, error)
  AppendUsage(ctx context.Context, events []UsageEvent) error
}
```
所有读取必须带 `projectID` 或先由 service 验证资源归属；不能存在 `GetDatabase(id)` 这种越权高风险 API。

## 表与迁移
创建 migration 表，再按顺序创建 `tenants`、`projects`、`databases`、`api_keys`、`llm_provider_configs`、`usage_events`、`operations`。关键索引：
- `projects(tenant_id, name)` unique；
- `databases(project_id, name)` 对未删除记录 unique；
- `databases(project_id, status)`；
- `usage_events(project_id, occurred_at)`；
- `operations(database_id, created_at)`。

所有 migration 只能前进；实现 `ApplyMigrations(ctx, db)`，每条 migration 在事务中执行并记录版本。Catalog 启动失败则整个 server 不 ready。

## Service 函数
```go
type CreateDatabaseInput struct { TenantID, ProjectID, Name string }
func (s *Service) CreateDatabase(ctx context.Context, in CreateDatabaseInput) (Database, error)
func (s *Service) GetDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (Database, error)
func (s *Service) BeginDeleteDatabase(ctx context.Context, principal auth.Principal, projectID, databaseID string) (Database, error)
func (s *Service) SetDatabaseReady(ctx context.Context, id string) error
func (s *Service) SetDatabaseDegraded(ctx context.Context, id string, cause error) error
```
`CreateDatabase`：验证名称 → 验证 project 属于 tenant → UUID → 建立不可猜测 prefix → 状态 creating → 写 DB 记录 → 调用 objectstore 写 descriptor → 失败标记 degraded 或补偿软删 → 返回。不要让 handler 直接写数据库。

> open/close 产品面已废弃，见 plan/planv3.0/database-always-open-plan.md。

## 状态转换规则
- `creating → opening → ready`；失败 `creating/opening → degraded`。
- `ready → closing → closed`；失败可回 `ready` 或 `degraded`，必须记录 operation。
- 仅 `closed/degraded → opening/recovering → ready`。
- `ready/closed/degraded → deleting → deleted`；deleted 不得恢复原 ID。
- repository 的 `TransitionDatabase` 使用 `UPDATE ... WHERE status IN (...)` 并检查 affected rows，防止并发错序。

## 测试与验收
- service 测试创建幂等、跨 project 拒绝、非法状态转换、descriptor 写失败补偿。
- repository 集成测试 migration 可重复执行，唯一约束和软删除后同名重建行为正确。
- Catalog 的 database 与用户 database 使用不同 prefix；用户 query 无法读取系统表。
