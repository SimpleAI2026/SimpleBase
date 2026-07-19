<!-- status: completed -->
<!-- verified: 2026-07-19 registry/handle/evictor 测试通过 -->

# Plan 4：Turso 数据库运行时与单写 Registry

## 目标
用 `turso.tech/database/tursogo` 和 `database/sql` 替换新链路中的旧 `internal/database/db`、`driver`、`node`、`walfs`。实现单进程的每库唯一 connection manager；这不是多实例一致性方案。

## 新目录
```text
internal/database/runtime.go
internal/database/errors.go
internal/database/query.go
internal/database/transaction.go
internal/database/registry/registry.go
internal/database/registry/handle.go
internal/database/registry/evictor.go
internal/database/registry/registry_test.go
internal/database/turso/factory.go
```

## 接口与数据结构
```go
type AccessMode uint8
const ( ReadOnly AccessMode = iota; ReadWrite )
type Factory interface { Open(ctx context.Context, db catalog.Database, mode AccessMode) (*sql.DB, error) }
type Handle struct {
  Database catalog.Database
  DB *sql.DB
  mode AccessMode
  state atomic.Uint32
  mu sync.RWMutex
  active atomic.Int64
  lastUsed atomic.Int64
  closeOnce sync.Once
}
type Registry struct {
  factory Factory
  catalog *catalog.Service
  handles map[string]*Handle
  mu sync.Mutex
  idleTimeout time.Duration
  maxOpen int
}
func (r *Registry) Acquire(ctx context.Context, db catalog.Database, mode AccessMode) (*Lease, error)
type Lease struct { Handle *Handle; release func() }
func (l *Lease) Release()
func (r *Registry) CloseDatabase(ctx context.Context, databaseID string) error
func (r *Registry) CloseIdle(ctx context.Context, now time.Time) error
func (r *Registry) Shutdown(ctx context.Context) error
```

## `Acquire` 精确算法
1. 校验 catalog status：deleting/deleted 返回 `ErrDatabaseDeleting`；degraded 仅允许恢复路径；Writable server=false 时拒绝 `ReadWrite`。
2. `Registry.mu.Lock()`；查 `handles[databaseID]`。
3. 已有 ready handle：`active++`、刷新 lastUsed，解锁，返回 Lease。
4. 没有：插入 `opening` placeholder（可用 `openCall` channel），解锁；只有创建者调用 `factory.Open`。
5. `factory.Open`：构建已验证 Turso DSN、`sql.Open("turso", dsn)`、配置 `SetMaxOpenConns/SetMaxIdleConns/SetConnMaxLifetime`、`PingContext`、可选 `PRAGMA integrity_check`；任一步失败 Close。
6. 创建者重新加锁，成功替换 placeholder 为 ready Handle，失败删除 placeholder 并广播同一错误；更新 catalog 状态。
7. 其他 waiter 等待 openCall 或 context cancel，绝不在锁内进行网络/S3 I/O。

建议实现 internal `entry`：`handle *Handle; opening chan struct{}; openErr error`，而不是用裸 map 容易重复打开。

## 查询执行接口
```go
type Statement struct { SQL string; Args []any }
type QueryResult struct { Columns []string; Rows [][]any; RowsAffected int64; LastInsertID int64; Duration time.Duration }
func (h *Handle) Query(ctx context.Context, stmt Statement, maxRows int) (QueryResult, error)
func (h *Handle) Execute(ctx context.Context, stmt Statement) (QueryResult, error)
func (h *Handle) Batch(ctx context.Context, stmts []Statement, transactional bool) ([]QueryResult, error)
```
- `Query` 使用 `DB.QueryContext`，defer Close rows；扫描为 JSON 可表示值，行数超过 maxRows 则停止并返回 `ErrRowLimitExceeded`（不可静默截断）。
- `Execute` 使用 `DB.ExecContext`，不接受多语句字符串；调用方 batch 必须拆成 `[]Statement`。
- `Batch(transactional=true)` 使用 `BeginTx`、逐条 `ExecContext`、任一失败 Rollback；成功 Commit。事务不能跨 HTTP 请求。
- 建立 SQL classifier：只用于授权/路由/审计，不能把正则分类当安全沙箱。首期禁止 `ATTACH`、`DETACH`、`VACUUM INTO`、`load_extension` 及任何可逃逸库目录的语句。

## 生命周期与缓存
- `CloseIdle` 每分钟运行一次，仅关闭 `active==0 && now-lastUsed>idleTimeout` 的 ready handle；从 map 删除前先标 closing，防止 Acquire 复用关闭中的 DB。
- 缓存目录只允许 `filepath.Join(cacheDir, databaseID)`，ID 必须 UUID；禁止用户 path。
- 关闭 `*sql.DB` 后只删除可重建的 cache 文件；S3 `data/` 不由本模块删除。
- server shutdown：停止新 Acquire → 等待有限时间 active=0 → Close 全部 DB；超时记录告警并继续关闭。

## 与旧代码关系
旧 `internal/database/db/ha_sqlite_db_manager.go`、`driver/*` 可供抽取行为测试，但新代码不得 import 它们。Plan 10 才删除。

## 测试与验收
- 100 个并发 Acquire 同库仅调用 factory.Open 一次。
- Open 失败所有 waiter 获得相同错误，下一次 Acquire 可重试。
- Lease Release 幂等；空闲关闭不关闭 active handle。
- 使用真实/测试 Turso 驱动验证 `PrepareContext → QueryContext → Scan → Close`、事务 rollback 和 cancel。
