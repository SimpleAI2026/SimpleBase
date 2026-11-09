<!-- status: completed -->
<!-- progress: 2026-07-19 完成：cache.Manager（LRU 淘汰、容量保障、防穿越）、jobs.Queue+Worker（持久化、Claim 原子性、指数退避、dead_letter）、delete_database/backup/restore/verify_recovery handler。Registry 增加 IsActive 查询。全部测试通过。 -->

# Plan 7：缓存、恢复点、删除任务与灾难恢复

## 目标
补全 S3 在线持久层周边的运维生命周期：本地缓存可丢失、可控卸载；恢复点和恢复流程可审计；删除采取异步软删除与延迟清理。不得自行复制/拼接 Turso 的在线 `data/` 对象。

## 新增文件
```text
internal/database/cache/manager.go
internal/database/cache/evict.go
internal/database/recovery/service.go
internal/database/recovery/verifier.go
internal/backup/service.go
internal/jobs/queue.go
internal/jobs/worker.go
internal/jobs/delete_database.go
internal/jobs/create_backup.go
internal/jobs/restore_database.go
```

## 缓存管理
```go
type CacheManager struct { root string; maxBytes int64; maxDatabases int; registry *registry.Registry }
func (m *CacheManager) Path(databaseID string) (string, error)
func (m *CacheManager) Usage(ctx context.Context) (CacheUsage, error)
func (m *CacheManager) Evict(ctx context.Context, targetBytes int64) (EvictionResult, error)
func (m *CacheManager) Remove(databaseID string) error
```

> open/close 产品面已废弃，见 plan/planv3.0/database-always-open-plan.md。

约束：
1. `Path` 验证 UUID，使用 `filepath.Join(root, databaseID)` 并检查结果仍在 root 下。
2. 只能淘汰已 `closed`、registry 中 `active==0` 且非恢复/备份中的库；先调用 `Registry.CloseDatabase`，再删除缓存目录。
3. 所有缓存删除仅作用本地路径，不调用 S3 删除。
4. 超过容量时优先 LRU；无法腾出空间则新 open 返回 `cache_capacity_exceeded`，禁止静默破坏活跃库。

## 后台任务模型
Catalog 新增 `operations`（可见操作）与 `jobs`（执行记录）表：
```go
type JobType string
const ( DeleteDatabaseJob JobType = "delete_database"; BackupJob = "backup"; RestoreJob = "restore"; VerifyRecoveryJob = "verify_recovery" )
type Job struct { ID, OperationID, DatabaseID string; Type JobType; Status JobStatus; Attempt int; RunAfter time.Time; PayloadJSON string; LastError string }
type Queue interface { Enqueue(ctx context.Context, job Job) error; Claim(ctx context.Context, workerID string, now time.Time) (Job, error); Complete(ctx context.Context, id string) error; Retry(ctx context.Context, id string, err error, next time.Time) error }
```
单实例 worker 以轮询/通知方式 claim；任务必须幂等，使用 operation ID 和状态机防止崩溃后重复删除或重复恢复。失败指数退避，有最大次数和可观测 dead-letter 状态。

## 恢复点与备份
优先调用 Turso 官方支持的快照/备份机制；适配接口必须隐藏上游细节：
```go
type Snapshotter interface {
    Create(ctx context.Context, source catalog.Database, destination BackupLocation) (Snapshot, error)
    Restore(ctx context.Context, snapshot Snapshot, target catalog.Database) error
}
type BackupService struct { snapshots Snapshotter; objects objectstore.Client; catalog *catalog.Service }
func (s *BackupService) Create(ctx context.Context, db catalog.Database) (Operation, error)
func (s *BackupService) RestoreAsNew(ctx context.Context, source BackupID, target CreateDatabaseInput) (Operation, error)
```
在官方能力不足前，不实现复制活跃 `.db`、`.db-wal`、`.shm` 文件的自研备份。每个已发布恢复点必须保存 source/target IDs、创建时间、Turso 版本、校验信息、状态和不可变 manifest。

## 恢复流程
1. 管理 API 创建新 target database，状态 `creating/recovering`，生成新的 UUID/prefix。
2. worker 从指定恢复点调用 `Snapshotter.Restore`；不覆盖运行中源库。
3. 以空缓存目录打开 target，执行 `PRAGMA integrity_check` 与预定义健康查询。
4. 成功：状态 `ready`，写 operation 结果；失败：`degraded`，保留证据与目标以便排查/重试。
5. “回滚”通过 alias/应用路由切换（后续功能）实现，不直接覆盖生产 database ID。

## 删除流程
`DELETE` 只执行：授权 → catalog `ready/closed/degraded → deleting` → enqueue job → 202。worker：关闭 registry handle → 检查保留截止时间 → 删除已知 database prefix（含备份按策略）→ 标记 deleted。任何中断可安全重试；禁止 handler 同步 `DeletePrefix`。

## 恢复演练与验收
- 每日/每周调度 `VerifyRecoveryJob`：从 S3 在临时 UUID + 空 cache 恢复，运行完整性检查，完成后只删除临时前缀。
- 缓存目录整个删除后，用户库与 catalog 均可重新打开。
- S3/DNS/KMS 权限失败不得让 worker 误标 completed。
- 删除任务重复执行不影响其他 tenant prefix；恢复目标不覆盖源库。
