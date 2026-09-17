# SimpleBase 内部模块说明

`internal/` 下的包按职责分层。所有业务路由经 `api` 层进入，底层依赖通过 `app` 装配。

## 模块清单

### `app`
依赖装配与服务生命周期。创建系统库、catalog、registry、auth、llmgateway、usage、audit、Cloud Agent 等全部依赖；`Shutdown` 按序停止。`RunWithSignal` 监听 SIGINT/SIGTERM 并在超时内优雅退出。

### `config`
从 `config.yaml`（或 `SIMPLEBASE_` 前缀环境变量）加载配置。结构体覆盖 HTTP、instance、database、s3、auth、llm、limits、observability、system_database 全部字段。历史 `catalog.database_id` / `SIMPLEBASE_CATALOG_DATABASE_ID` 已删除。

### `api`
HTTP 路由层（Echo v4）。职责：
- `router.go`：全局中间件（requestID、recover、accessLog、bodyLimit）、`/health/*`、`/metrics`、`/v1` 路由组挂载。
- `database_handler.go`：数据库管理 API（create/list/get/open/close/delete）。
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
单实例元数据。SQL 仓库（`sql_repository.go`）+ 内存模型（`model.go`）+ 系统库迁移（`systemdb/migrate.go`）。表：tenants、projects、databases、api_keys、llm_provider_configs、project_quotas、audit_operations。`sys_jobs` DDL 仅为迁移历史保留，应用层不再读写。`service.go` 提供领域操作与状态机（creating→opening→ready→closing→closed、deleting→deleted、recovering）。`errors.go` 定义领域错误（越权、未找到、状态冲突）。

### `database`
数据库运行时。
- `ducklake/`：唯一用户库引擎。DuckDB + DuckLake（本地/S3 Parquet），实现 `database.Factory`。
- `registry/`：进程内每库唯一 writer。`map[databaseID]*DatabaseHandle` + 每库互斥。`Open` 幂等（已打开则复用），`Close` 引用计数归零后关闭，`Evictor` 按空闲超时卸载。
- `cache/`：本地缓存目录管理。路径校验防穿越、LRU 淘汰、活跃库保护（不淘汰正在使用的库）、容量配额。
- `sqlguard/`：SQL 执行边界。DuckDB 方言拒绝清单（ATTACH/SET/CALL/COPY 等）、只读检测、多语句与 NUL 拦截。
- `serialize.go`：行序列化为 JSON（含 DuckDB DECIMAL/UUID/嵌套类型）。
- `runtime.go`、`query.go`、`transaction.go`、`errors.go`：运行时核心。

### `objectstore`
S3 边界。`client.go`（AWS SDK v2 封装）、`keys.go`（KeyBuilder 生成隔离前缀）、`descriptor.go`（database descriptor 读写）、`health.go`（S3 可达性检查）。对象键使用内部 UUID，禁止用户名直接入键。

### `cloudagent`
项目级 Cloud Agent 运行时。只读工具（数据库 / S3 / 日志 / 设置）+ 会话线程，依赖 LLM Gateway 与系统库。

### `llmgateway`
LLM 服务端封装。项目隔离的 provider 配置（从 catalog `llm_provider_configs` 读取），单供应商 client 缓存。`Chat` 非流式、`Stream` SSE 流式（用量累计）、`ListProviders`。密钥采用 CredentialRef 模式，禁止环境变量自动发现。适配 litellm v1.5.8 的 `NewWithProvider` 单供应商 API。

### `usage`
配额检查与用量聚合。`Check` 在请求前校验 project 配额（数据库数、存储、LLM 请求/token）。`Flush` 批量缓冲写入 catalog。`ListOperations` 查询操作记录。

### `audit`
不可变审计事件。`Record` 写入 catalog `audit_operations`。`Redact` 对 SQL 参数与 LLM 正文脱敏，默认不落库正文。事件覆盖：数据库创建/删除/恢复/DDL/密钥变更/管理操作。

### `observability`
日志（Zap，`*log.Logger` 封装）、Prometheus 指标、健康检查聚合。

### `log`
Logger 本体。旋转/Tee 辅助已删除。

## 依赖关系

```text
cmd/simplebased → app → api → auth → catalog
                         ↓        ↓
                   database/* ←───┘
                   objectstore
                   cloudagent → llmgateway, catalog, systemdb
                   llmgateway → litellm, catalog, usage, audit
                   usage → catalog
                   audit → catalog
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
