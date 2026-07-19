# SimpleBase 内部模块说明

`internal/` 下的包按职责分层。所有业务路由经 `api` 层进入，底层依赖通过 `app` 装配。

## 模块清单

### `app`
依赖装配与服务生命周期。创建 catalog、registry、auth、jobs worker、llmgateway、usage、audit 等全部依赖；`Start` 启动 job worker，`Shutdown` 按序停止。`RunWithSignal` 监听 SIGINT/SIGTERM 并在超时内优雅退出。

### `config`
从 `config.yaml`（或 `SIMPLEBASE_` 前缀环境变量）加载配置。结构体覆盖 HTTP、instance、database、s3、catalog、auth、llm、limits、observability 全部字段。

### `api`
HTTP 路由层（Echo v4）。职责：
- `router.go`：全局中间件（requestID、recover、accessLog、bodyLimit）、`/health/*`、`/metrics`、`/v1` 路由组挂载。
- `database_handler.go`：数据库管理 API（create/list/get/open/close/backup/restore/delete）。
- `sql_handler.go`：SQL 执行 API（query/execute/batch），参数化、超时、行数限制。
- `llm_handler.go`：LLM Gateway API（chat/stream/providers），SSE 流式转发。
- `quota_handler.go`、`audit_handler.go`：配额与审计查询。
- `context.go`：请求上下文（Principal、ProjectContext、RequestID）。
- `error.go`：统一错误协议（JSON + HTTP 状态码映射）。
- `adapter.go`、`adapters_plan79.go`：`api.Dependencies` 接口到具体实现的适配器桥接。
- `health.go`：存活与就绪检查。

### `auth`
认证与权限。`Principal` 表示已认证主体；`Permission` 区分 `database:read/write/admin`、`llm:invoke`、`project:admin`。`APIKeyMiddleware` 解析 Bearer token 并从 catalog 加载 API key。`Require` 在路由层按权限拦截。`api_key_repository` 从 catalog 持久化读取。

### `catalog`
单实例元数据。SQLite 仓库（`sqlite_repository.go`）+ 内存模型（`model.go`）+ 迁移（`migrations.go`）。表：tenants、projects、databases、api_keys、llm_provider_configs、jobs、project_quotas、audit_operations。`service.go` 提供领域操作与状态机（creating→opening→ready→closing→closed、deleting→deleted、recovering）。`errors.go` 定义领域错误（越权、未找到、状态冲突）。

### `database`
数据库运行时。
- `turso/`：DSN 构建（`libsql` 驱动）与连接工厂。从 database descriptor + S3 prefix + 本地缓存目录生成 DSN。
- `registry/`：进程内每库唯一 writer。`map[databaseID]*DatabaseHandle` + 每库互斥。`Open` 幂等（已打开则复用），`Close` 引用计数归零后关闭，`Evictor` 按空闲超时卸载。
- `cache/`：本地缓存目录管理。路径校验防穿越、LRU 淘汰、活跃库保护（不淘汰正在使用的库）、容量配额。
- `sqlguard/`：SQL 执行边界。context deadline、最大返回行数、最大批次数、只读检测（query 禁止 DML/DDL）。
- `serialize.go`：行序列化为 JSON。
- `runtime.go`、`query.go`、`transaction.go`、`errors.go`：运行时核心。

### `objectstore`
S3 边界。`client.go`（AWS SDK v2 封装）、`keys.go`（KeyBuilder 生成隔离前缀）、`descriptor.go`（database descriptor 读写）、`health.go`（S3 可达性检查）。对象键使用内部 UUID，禁止用户名直接入键。

### `jobs`
持久化异步任务。`queue.go`（JobQueue + Worker 循环）、指数退避重试、`dead_letter` 终态。处理器：`DeleteDatabase`（异步清理 S3）、`Backup`（创建恢复点）、`Restore`（恢复为新库）、`VerifyRecovery`（完整性校验）。Job 状态持久化到 catalog `jobs` 表，进程重启后恢复。

### `llmgateway`
LLM 服务端封装。项目隔离的 provider 配置（从 catalog `llm_provider_configs` 读取），单供应商 client 缓存。`Chat` 非流式、`Stream` SSE 流式（用量累计）、`ListProviders`。密钥采用 CredentialRef 模式，禁止环境变量自动发现。适配 litellm v1.5.8 的 `NewWithProvider` 单供应商 API。

### `usage`
配额检查与用量聚合。`Check` 在请求前校验 project 配额（数据库数、存储、LLM 请求/token）。`Flush` 批量缓冲写入 catalog。`ListOperations` 查询操作记录。

### `audit`
不可变审计事件。`Record` 写入 catalog `audit_operations`。`Redact` 对 SQL 参数与 LLM 正文脱敏，默认不落库正文。事件覆盖：数据库创建/删除/恢复/DDL/密钥变更/管理操作。

### `deploy`
Preflight 检查。启动时执行：缓存目录可写 + 容量、S3 可达、单写 flock 锁（防多实例误启）。

### `observability`
日志（Zap，`*log.Logger` 封装）、Prometheus 指标、健康检查聚合。

### `log`、`prom`、`utils`
通用工具包。

## 依赖关系

```text
cmd/simplebased → app → api → auth → catalog
                         ↓        ↓
                   database/* ←───┘
                   objectstore
                   jobs → catalog
                   llmgateway → litellm, catalog, usage, audit
                   usage → catalog
                   audit → catalog
                   deploy → objectstore, cache
```

`litellm/` 为独立客户端库，不 import `internal/*`；服务端通过 `llmgateway` 调用。

## 测试

每个模块带单元测试。关键覆盖：
- `registry`：并发单写、Open 失败广播、空闲关闭。
- `cache`：路径穿越拒绝、LRU 淘汰、活跃库保护。
- `catalog`：状态机、越权拒绝、补偿。
- `sqlguard`：11 项边界测试。
- `sql_handler`：20 项协议测试。
- `api/router_test.go`：路由与中间件。
