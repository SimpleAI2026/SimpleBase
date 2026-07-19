# internal/database — 数据库运行时

SimpleBase 数据库运行时层。基于 Turso/libSQL（`libsql` 驱动）与 `database/sql`，提供连接管理、单写路由、本地缓存、SQL 执行边界与行序列化。

## 子包

### `turso/`
DSN 构建与连接工厂。从 database descriptor + S3 prefix + 本地缓存目录生成 `libsql` DSN。业务模型不直接依赖驱动包。

### `registry/`
进程内每库唯一 writer。
- `map[databaseID]*DatabaseHandle` + 每库互斥，保证同一 logical database 只有一个 writer/connection manager。
- `Open` 幂等：已打开则复用，未打开则创建初始化任务。
- `Close` 引用计数归零后关闭连接。
- `Evictor` 按空闲超时卸载，释放可重建缓存，不删除 S3 持久对象。
- 进程重启后从 catalog/S3 descriptor 重建，不把内存状态视为持久事实。

### `cache/`
本地缓存目录管理。
- 路径校验防穿越（拒绝 `..` 与绝对路径）。
- LRU 淘汰：按 `cache_max_bytes` 与 `cache_max_databases` 触发。
- 活跃库保护：正在使用的库不被淘汰。
- 缓存可完全丢弃，丢失后从 S3 恢复。

### `sqlguard/`
SQL 执行边界。
- context deadline 强制超时。
- 最大返回行数、最大批次数限制。
- 只读检测：query 路径禁止 DML/DDL。

## 顶层文件

- `runtime.go` — 运行时核心接口。
- `query.go` — 查询执行（参数化、行数限制）。
- `transaction.go` — 事务执行（绑定同一 handle 与连接）。
- `serialize.go` — 行序列化为 JSON。
- `errors.go` — 运行时错误映射。
- `tursofactory.go` — Turso 连接工厂入口。

## 约束

- 所有写接口只能从 Registry 获取 `ReadWrite` handle，禁止绕过 API 直接构造 DSN。
- 管理接口与 SQL 接口共享同一 Registry，避免创建/删除/执行 SQL 并发冲突。
- 单写保证仅限单进程；部署层必须固定副本数为 1。
