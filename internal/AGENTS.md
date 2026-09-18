# SimpleBase 后端约束（internal/）

本文件约束 `internal/` 下所有 Go 包的修改边界。架构详见 `internal/README.md` 与 `plan/`。

## 分层与依赖方向

```text
cmd/simplebased → app → api → auth / catalog / database / objectstore
                              ↓
                        systemdb（常驻系统库连接）
                        cloudagent → llmgateway / catalog / systemdb
                        usage / audit → catalog
```

- 依赖只能自上而下；**禁止反向 import**（如 `catalog` import `api`、`database` import `app`）。
- `litellm/` 为独立客户端库，不得 import `internal/*`。
- `app` 是唯一装配点：所有跨模块依赖在 `assembleDeps` 中按序创建，`Close` 反向释放。新增模块必须挂进装配顺序并在失败路径回收。

## 各模块职责边界

| 模块 | 职责 | 禁止 |
| --- | --- | --- |
| `api` | Echo 路由、handler、错误协议、适配器 | 业务逻辑下沉；handler 直接依赖具体实现（必须走 `Dependencies` 接口 + adapter） |
| `auth` | Principal、Permission、API key 校验 | 在 handler 里解析 token |
| `catalog` | 元数据状态机（creating→ready→deleting…）、sys_* 表仓库 | 直接执行用户 SQL |
| `database/ducklake` | 唯一存储引擎 Factory（DuckDB + DuckLake） | 新增第二引擎旁路 |
| `database/registry` | 进程内每库唯一 writer，Acquire/Lease 生命周期 | 绕过 Registry 直连 DSN 打开用户库 |
| `database/sqlguard` | SQL 边界（拒绝清单、只读检测、多语句拦截） | handler 内自行做 SQL 过滤 |
| `database/cache` | 缓存目录 LRU、路径防穿越 | 淘汰活跃库 |
| `objectstore` | S3 边界、KeyBuilder、descriptor | 用户可控字符串直接拼对象键 |
| `systemdb` | 系统库 bootstrap、迁移、常驻连接 | — |
| `cloudagent` | 只读工具运行时 + 会话线程 | 提供写工具 |
| `llmgateway` | provider 配置、Chat/Stream、CredentialRef | 密钥从环境变量自动发现 |

## 系统库规则（重要）

系统库 `simplebase-system`（kind=system，project=admin）有**两条访问路径**，改动前必须认清：

1. **内部直连**：`systemdb.Store` 持有常驻 `*sql.DB`（CacheDir 为 `<cache>/system/dbs`）。catalog、auth、日志、指标、S3 索引、LLM 会话全部走这条。
2. **API 桥接（只读）**：admin 项目下用户请求（查看数据 / SQL 查询）经 `api.sqlServiceAdapter.Acquire` 判定 `IsSystemDatabase && ReadOnly` 时桥接到 `Store.DB()`（`systemLeaseAdapter`），**不经 registry**。

硬性约束：

- 系统库**禁止删除、禁止写入**：
  - `catalog.BeginDeleteDatabase` / `DeleteDatabase` 对 kind=system 返回 `ErrSystemProtected`；
  - `SQLHandler.Execute/Batch` 与 `DataHandler` 四个写接口（CreateCollection / CreateDocument / UpdateDocument / DeleteDocument）前置 `IsSystemDatabase` 拒绝；
  - `systemLeaseAdapter.Execute/Batch` 防御性拒绝（双保险）。
- 系统库**只允许 SELECT**：`SQLHandler.Query` 走 `sqlguard.Validate(ReadOnly)`。
- **不得**通过 registry 的用户库 factory 打开系统库——两者 CacheDir 不同，会得到空 catalog。
- admin 项目（`ReservedSystemProjectID`）下日志/监控查询不过滤 project（`systemdb.IsAdminProject`）。

## API 协议约定

- 错误统一走 `WriteError` + `error.go` 的映射；新增错误必须定义领域错误（`catalog/errors.go` 风格），不得裸返回字符串。
- 写操作 handler 必须检查 `writable`（实例级只读返回 `ErrWriterUnavailable`）。
- SQL handler 遵守 limits：并发 semaphore、行数上限、请求体上限；审计只记录 SHA-256 与元数据，不落原 SQL。
- 所有带 `:projectID` 的路由经过 `projectContextMiddlewareEcho` 注入 ProjectContext。

## 测试要求

- 每个模块带单元测试；改动必须保持 `go build ./internal/... ./cmd/...` 与 `go test ./...` 通过。
- handler 测试用 fake service（见 `data_handler_test.go` 风格），不依赖真实 DuckDB/S3。
- 新增写路径必须补「系统库拒绝」用例。

## 其他

- 配置只从 `config.yaml` / `SIMPLEBASE_` 环境变量加载（`internal/config`），不得引入其他配置来源。
- 审计事件默认脱敏；不得在日志/审计中输出 SQL 参数或 LLM 正文。
- 不新增第三方依赖除非用户明确要求；不改 `go.mod` 结构。
