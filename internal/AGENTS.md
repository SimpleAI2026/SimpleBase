# SimpleBase 后端约束（internal/）

本文件约束 `internal/` 下所有 Go 包的修改边界。架构详见 `internal/README.md` 与 `plan/`。

## 分层与依赖方向

```text
cmd/simplebase → app → api → auth / catalog / dbbridge / objectstore
                              ↓            └→ godatabase（嵌套独立模块，数据库运行时库）
                              ↓
                        systemdb（常驻系统库连接）
                        agentbridge → gocloudagent（嵌套独立模块）/ auth / systemdb / sbcli / sqlguard / llmgateway
                        usage / audit → catalog
```

- 依赖只能自上而下；**禁止反向 import**（如 `catalog` import `api`）。`godatabase` 是嵌套独立模块，**不得 import 根模块任何包**，宿主类型经 `internal/dbbridge` 转换。
- `litellm/` 为独立客户端库，不得 import `internal/*`。
- `app` 是唯一装配点：所有跨模块依赖在 `assembleDeps` 中按序创建，`Close` 反向释放。新增模块必须挂进装配顺序并在失败路径回收。

## 各模块职责边界

| 模块 | 职责 | 禁止 |
| --- | --- | --- |
| `api` | Echo 路由、handler、错误协议、适配器 | 业务逻辑下沉；handler 直接依赖具体实现（必须走 `Dependencies` 接口 + adapter） |
| `auth` | Principal、Permission、API key 校验 | 在 handler 里解析 token |
| `catalog` | 元数据状态机（creating→ready→deleting…）、sys_* 表仓库 | 直接执行用户 SQL |
| `dbbridge` | 宿主与独立 `godatabase` 模块之间的适配层：`catalog.Database` ↔ `Spec`、`catalog.Service` → `PathResolver`、`observability` → `Logger`/`Observer`（nil 必须返回无类型 nil）、`Registry` 包装 | import `api` / `app`；把 `Kind=system`、`Status` 在转换中丢失或改写；让 typed-nil 流入库 |
| `godatabase`（嵌套模块，目录在仓库根 `godatabase/`） | 单文件用户库 `filedb`、系统库 `ducklake`、进程内单写 `registry`、`kv`、`lease`、`cache`、`sqlguard`、`blob`（对象存储接口与 key 布局） | import 根模块任何包（含 `internal/...`）；用 `ducklake` 打开用户库；绕过 `registry` 直连 DSN；把 `database_path`、DSN 写入日志/审计/API 响应；改动 S3 key 布局或 manifest JSON 字段（磁盘契约，金标测试在 `internal/objectstore`） |
| `objectstore` | S3 边界、KeyBuilder、descriptor | 用户可控字符串直接拼对象键 |
| `systemdb` | 系统库 bootstrap、最终态建表（`schema.go`，无版本化迁移）、常驻连接；`schema_status.go` 提供只读 Status/Plan | 新增版本化迁移或 `ALTER` 补列：改表结构直接改 `schema.go` 并 reset |
| `functionrun` | `/go`、调试试跑、cron、事件触发器共享的活动版本执行器与全局并发槽位；逐次写调用流水 | 另建并发信号量或绕过活动版本解析 |
| `trigger` | 记录/文件/HTTP 事件的有界进程内队列、匹配与最多三次重试 | 声称持久化或 exactly-once；覆盖 SQL 直写；把事件正文写日志 |
| `agentbridge` | 宿主与独立 `gocloudagent` 模块之间的适配：身份 / 助手 / 消息映射、`CommandPolicy`、最终错误分类、委托签发、日志与设置 store 适配、`InProcessCLI`、agent 定时任务执行（`CronAgentExecutor` 实现 `cronjob.AgentRunner`） | import `api` / `app`（它们复用本包，不能反向）；在映射中丢失 Role / TenantID / ProjectIDs；让委托令牌携带比本轮降权后更多的权限 |
| `gocloudagent`（嵌套模块，目录在仓库根 `gocloudagent/`） | 只读工具，或在 `skills_cli` 打开时经 CLI 调既有 HTTP API；沙盒工具经 `Sandbox` 接口；本机 `shell` 在 `shell_isolation.enabled` 时经 `gosandbox/host` | import 根模块任何包（含 `internal/...`、`skills`）；直接写 DuckLake / S3 / 系统库；向沙盒注入 SimpleBase 凭据；直接 import `sandbox` 或 `gosandbox/vm`（`gosandbox/host` 是唯一例外，仅用于本机 shell）；隔离开启但不可用时退回裸执行；策略缺失时放行命令 |
| `sandbox` | 云沙盒业务层：Manager（资源、限额、路径、reaper）+ Store 元数据；执行后端（Driver、local、fake）来自独立模块 `gosandbox/vm` | 读写 DuckLake/S3/系统库凭据；向沙盒注入 SimpleBase 凭据；隔离不可用时退回裸执行；向沙盒内命令暴露 `data_dir` / 缓存目录 / `config.yaml` / `.env`；直接 import `gosandbox/host`（经 `gosandbox/vm/local` 间接使用） |
| `gosandbox`（嵌套模块） | `vm/`：云沙盒执行后端（`local` 复用 `host`，`fake` 仅测试）；`host/`：本机进程隔离（bwrap + Landlock + seccomp / Seatbelt）；`cmd/gosandbox-inner`：沙盒内 helper | import 根模块任何包（含 `internal/...`）；`host` import `vm`（依赖只能 `vm/local → host`，不得反向）；在 `vm/local` 之外为 `host` 新增 `Driver` 实现；让 `local` 在隔离不可用时退回裸执行 |
| `llmgateway` | provider 配置、Chat/Stream、CredentialRef | 密钥从环境变量自动发现 |
| `utils` | 零业务依赖的纯函数（`crontab` / `strutil` / `limit` / `mask` / `hashx` / `atomicfile`） | import 任何根模块业务包；把绑定驱动/协议或适配层的东西搬进来 |

## 系统库规则（重要）

系统库 `simplebase-system`（kind=system，project=admin）有**两条访问路径**，改动前必须认清：

1. **内部直连**：`systemdb.Store` 持有常驻 `*sql.DB`（CacheDir 为 `<cache>/system/dbs`）。catalog、auth、日志、指标、S3 索引、LLM 会话全部走这条。
2. **API 桥接（只读）**：admin 项目下用户请求（查看数据 / SQL 查询）经 `api.sqlServiceAdapter.Acquire` 判定 `IsSystemDatabase && ReadOnly` 时桥接到 `Store.DB()`（`systemLeaseAdapter`），**不经 registry**。

硬性约束：

- 系统库**禁止删除、禁止写入**：
  - `catalog.BeginDeleteDatabase` / `DeleteDatabase` 对 kind=system 返回 `ErrSystemProtected`；
  - `SQLHandler.Execute/Batch` 与 `DataHandler` 写接口（CreateCollection / CreateDocument / UpdateDocument / PatchDocument / BatchDocuments / DeleteDocument / TruncateCollection）及 `CollectionMetaHandler` 写接口前置 `IsSystemDatabase` 拒绝；
  - `systemLeaseAdapter.Execute/Batch` 防御性拒绝（双保险）。
- 系统库**只允许 SELECT**：`SQLHandler.Query` 走 `sqlguard.Validate(ReadOnly)`。
- **不得**通过 registry 的用户库 factory 打开系统库——两者 CacheDir 不同，会得到空 catalog。
- admin 项目（`ReservedSystemProjectID`）下日志/监控查询不过滤 project（`systemdb.IsAdminProject`）。

## API 协议约定

- 错误统一走 `WriteError` + `error.go` 的映射；新增错误必须定义领域错误（`catalog/errors.go` 风格），不得裸返回字符串。
- 写操作 handler 必须检查 `writable`（实例级只读返回 `ErrWriterUnavailable`）。
- SQL handler 遵守 limits：并发 semaphore、行数上限、请求体上限；审计只记录 SHA-256 与元数据，不落原 SQL。
- 所有带 `:projectID` 的路由经过 `projectContextMiddlewareEcho` 注入 ProjectContext。

### 对外兼容出口（OpenAI Chat Completions）

- 路由：`POST /v1/projects/:projectID/v1/chat/completions`、`POST .../v1/responses`、`GET .../v1/models`，权限 `llm:invoke`，实现见 `llm_handler.go`、`oai_dto.go` 与 `oai_responses.go`。
- **路径里的两段 `v1` 是刻意设计，不是笔误，不得"顺手修正"**：前一段是 SimpleBase 的 API 版本，后一段是 OpenAI SDK 约定的资源前缀，只有两段都在，`OpenAI(base_url=".../projects/<id>/v1")` 才能原样拼出 `/chat/completions`。`oai_dto_test.go` 的路由测试锁定了该路径，旧的 `/llm/chat`、`/llm/stream`、`/llm/providers` 已删除且必须保持 404。
- **错误形状是本规约唯一允许的偏离**：兼容路由组内走 `writeOAIError`（`{"error":{"message","type","param","code"}}`），因为 OpenAI SDK 解析不了 `WriteError` 的信封。`code` 与脱敏文案仍来自 `error.go` 的映射，不新造分类；其余 handler 不得使用 `writeOAIError`。
- 流式以 `data: [DONE]` 收尾；首帧为 `delta.role="assistant"`；工具调用增量按 `index` 聚合，`id`/`name` 只在首个分片出现。新增消费方必须识别 `[DONE]`，否则不会判定流结束。
- **Responses API（`oai_responses.go`）是 chat 之上的协议转换，不是第二套网关**：请求转成 `LLMRequest`、走既有 `LLMService.Chat/Stream`，再把结果编码回 Responses 形状。不要改成直接调 litellm 的 `Responses`——它只支持 OpenAI 一个 provider，会让 DeepSeek 等兼容网关失效。只支持无状态子集；`previous_response_id`/`conversation`/`background`/结构化输出/内置工具一律 400，不得为"看起来兼容"而静默忽略。其流式**没有 `[DONE]`**，以 `response.completed`/`incomplete`/`failed` 收尾，且 chat 流无法做到的"流开始后报错"在这里用 `response.failed` 事件表达。
- `tool_choice` 仅在 HTTP 层与 `llmgateway.Request` 之间透传；云助手 agent 循环不设置它。

## 测试要求

- 每个模块带单元测试；改动必须保持 `go build ./internal/... ./cmd/...` 与 `go test ./...` 通过。
- handler 测试用 fake service（见 `data_handler_test.go` 风格），不依赖真实 DuckDB/S3。
- 新增写路径必须补「系统库拒绝」用例。

## 其他

- 配置只从 `config.yaml` / `SIMPLEBASE_` 环境变量加载（`internal/config`）：YAML 为非密钥权威来源，env 覆盖；不得引入其他配置来源。
- 审计事件默认脱敏；不得在日志/审计中输出 SQL 参数或 LLM 正文。
  **唯一例外**：`agent.trace_log=true`（`SIMPLEBASE_AGENT_TRACE_LOG=true`）时，云助手会输出
  提示词、用户输入、工具参数与结果、助手输出（planv6.0 v2 §5）。该开关默认关闭；打开后
  内容先经 `RedactSecrets` 脱敏并按 2000 字符截断，且只写 zap 日志，**不得**写入系统库
  logs 表（助手自身的 `search_logs` 会把日志读回 LLM 上下文，造成跨用户泄漏）。
  未打开开关时只输出元数据（标识、长度、SHA-256 前缀、耗时、token 数）。
- 不新增第三方依赖除非用户明确要求；不改 `go.mod` 结构。
