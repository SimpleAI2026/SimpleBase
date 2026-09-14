# SimpleBase 重构计划

> 基于 Turso/libSQL、S3 在线持久层与 Go 构建云端数据库，并集成 LLM 转发能力。
>
> 状态：实施阶段。Plan 1-6 已完成并通过测试；Plan 7-10 待实施。

## 计划完成状态

| 计划 | 状态 | 说明 |
| --- | --- | --- |
| Plan 1：工程基线、依赖与服务装配 | ✅ 已完成 | `cmd/simplebased`、`internal/app`、`internal/config`、`internal/api/router`、`internal/observability` 骨架就绪，单实例可启动，`/health/live`、`/health/ready` 可用 |
| Plan 2：S3 边界、对象命名与 Turso 持久层适配 | ✅ 已完成 | `internal/objectstore`（client/keys/descriptor/health）、`internal/database/turso`（DSN/factory）就绪，KeyBuilder/隔离/校验测试通过 |
| Plan 3：Catalog、项目隔离与领域状态 | ✅ 已完成 | `internal/catalog`（model/repository/service/migrations/errors/sqlite_repository）就绪，状态机、越权拒绝、补偿测试通过 |
| Plan 4：Turso 数据库运行时与单写 Registry | ✅ 已完成 | `internal/database/registry`（registry/handle/evictor）、`runtime.go`、`query.go`、`transaction.go`、`errors.go` 就绪，并发单写、Open 失败广播、空闲关闭测试通过 |
| Plan 5：认证、项目上下文、数据库管理 API | ✅ 已完成 | `internal/auth`（principal/service/middleware/api_key_repository）、`internal/api`（database_handler/error/request/context）就绪，跨 project 拒绝、错误协议、202 删除测试通过 |
| Plan 6：SQL API、执行边界与响应协议 | ✅ 已完成 | `internal/database/sqlguard`、`serialize.go`、`internal/api/sql_handler`、`sql_types.go` 就绪，sqlguard 11 测试、sql_handler 20 测试通过 |
| Plan 7：缓存、恢复点、删除任务与灾难恢复 | ✅ 已完成 | `internal/database/cache`、`internal/jobs`、catalog Job 模型与迁移就绪，路径校验/LRU/指数退避/dead_letter 测试通过 |
| Plan 8：LLM Gateway、流式转发、密钥与用量 | ✅ 已完成 | `internal/llmgateway` 就绪，项目隔离、单供应商 client 缓存、SSE 转发、CredentialRef 密钥模式，适配 litellm v1.5.8 |
| Plan 9：配额、审计、指标、部署与端到端验证 | ✅ 已完成 | `internal/usage`、`internal/audit`、`internal/deploy` 就绪，Preflight 检查、配额缓冲 Flush、审计脱敏测试通过 |
| Plan 10：迁移、切换、旧链路退役与后续扩展边界 | ✅ 已完成 | 旧链路已删除（Raft/gRPC/GORM/viper/旧 SQLite VFS），`go mod tidy` 清理；README/internal README/部署文档/迁移指南已更新 |

## 1. 产品定位

SimpleBase 的长期目标是成为类似 Supabase 的开源云端后端平台，逐步提供：

1. 基于 S3 扩展的 Serverless 云端数据库；
2. 统一 LLM 转发、路由和用量治理；
3. Serverless 函数运行时；
4. 身份认证、对象存储、实时订阅等平台能力。

首期只实现前两项：

- **Cloud Database**：使用 Turso/libSQL 兼容能力，以 S3 作为在线持久层，本地磁盘仅作为缓存和工作集。
- **LLM Gateway**：复用仓库中的 `litellm`，提供统一模型调用、流式转发、路由、鉴权和用量记录。

Serverless 函数运行时及其他平台能力仅预留边界，不进入首期开发。

## 2. 已确认的架构决策

### 2.1 S3 是在线持久层

S3 不是单纯的备份介质，而是数据库持久数据的最终来源。数据库服务可以使用本地磁盘保存热数据、页缓存、临时文件和可重建状态，但节点丢失本地磁盘后，必须能仅依靠 S3 中的持久数据恢复数据库。

采用以下约束：

- 不自行把 SQLite 数据库文件挂载成 S3 文件，也不对 S3 对象模拟任意 POSIX 随机写、文件锁或共享 WAL。
- 使用 Turso/libSQL 已支持的对象存储机制保存数据库持久状态；SimpleBase 负责配置、租户隔离、生命周期管理和可恢复性验证。
- 在线提交成功的定义必须与 Turso 的 S3 持久化确认语义一致；若上游采用异步上传，API 必须明确区分“本地已提交”和“已持久化到 S3”。
- 本地数据必须视为缓存，支持容量限制、淘汰、冷启动和从 S3 重建。
- S3 故障时默认停止确认新的持久写入，不以无限本地堆积掩盖持久层不可用。

### 2.2 首期不实现强一致 Catalog

首期只部署一个可写的 SimpleBase Server 实例。所有数据库写请求必须经过该实例的 API 路由，由进程内数据库注册表保证同一个 logical database 只有一个 writer/connection manager。

这是一项明确的范围约束，而不是完整的分布式一致性方案：

- 首期不做自动 primary 提升、Raft、租约、epoch/fencing 或多活写入。
- 不允许同时启动两个指向同一 S3 数据库前缀的可写 Server。
- 部署层必须固定副本数为 1；滚动升级采用先停旧实例、再启新实例的方式。
- Server 崩溃后由新实例从 S3 恢复；恢复完成前 API 返回不可写状态。
- 可通过只读实例扩展查询，但只读实例不得获得写路由和写凭据。

如果未来需要多个 API 实例和自动故障切换，必须先引入外部分布式锁/强一致元数据服务及 fencing，不能仅依赖负载均衡器或内存标记保证单写。

### 2.3 Turso Go 驱动

首期直接采用官方文档支持的 `turso.tech/database/tursogo`，通过标准 `database/sql` 使用：

```go
import (
    "database/sql"

    _ "turso.tech/database/tursogo"
)

conn, err := sql.Open("turso", dsn)
```

业务模型不直接依赖驱动包。SimpleBase 在 `internal/database` 中封装 DSN、连接生命周期、租户鉴权、超时、限流和错误映射，便于后续升级 Turso 而不影响上层模型。

## 3. 首期目标与非目标

### 3.1 目标

- 使用 Go 实现单实例数据库 Server、管理 API 和 SQL API。
- 每个 tenant/project 拥有独立 logical database 和 S3 对象前缀。
- 支持数据库创建、打开、关闭、SQL 执行、查询、事务、删除和从 S3 恢复。
- 支持本地缓存限制、空闲数据库卸载、冷启动和崩溃恢复。
- 提供 OpenAI 兼容或 SimpleBase 统一 LLM API，支持普通响应与流式响应。
- 为数据库与 LLM 共用身份认证、项目隔离、配额、审计和用量统计。
- 保留现有领域模型；旧 SQLite/Raft/VFS 实现经迁移验证后再删除。

### 3.2 非目标

- 多写节点、多活、自动 primary 选举和跨数据库分布式事务。
- 自研 SQLite VFS、WAL 复制、Raft FSM 或 S3 文件系统。
- 首期提供数据库只读副本的自动调度和全局边缘复制。
- 不在首期实现 Serverless 函数运行时、Web Studio、Realtime、Auth 产品或向量数据库。
- 修改或移除现有业务模型。

## 4. 总体架构

```text
 Clients / SDK / CLI
          │
          ▼
┌─────────────────────────────────────────────────────┐
│ SimpleBase Server（首期唯一可写实例）              │
│                                                     │
│ API Gateway                                         │
│ ├─ Auth / Project / Quota / Audit                   │
│ ├─ Database Management API                          │
│ ├─ SQL Query API                                    │
│ └─ LLM Gateway API                                  │
│                                                     │
│ Database Runtime                 LLM Runtime         │
│ ├─ DB Registry（每库唯一 writer）├─ litellm Router  │
│ ├─ tursogo / database/sql        ├─ Provider Keys   │
│ ├─ Local Cache Manager           ├─ Streaming       │
│ └─ Recovery Manager              └─ Usage Metering  │
└───────────────┬──────────────────────────┬──────────┘
                │                          │
                ▼                          ▼
       S3 Compatible Storage        LLM Providers
       在线持久数据与元数据          OpenAI/Anthropic/...
```

### 4.1 建议模块

| 模块 | 职责 |
| --- | --- |
| `cmd/simplebased` | 配置加载、服务启动、优雅关闭和恢复流程 |
| `internal/api` | HTTP 路由、请求验证、错误协议、流式响应 |
| `internal/auth` | API Key、项目身份、权限与上下文注入 |
| `internal/catalog` | 单实例元数据：tenant、project、database、LLM 配置和状态 |
| `internal/database` | `tursogo` 连接、事务、查询、生命周期和错误映射 |
| `internal/database/registry` | 进程内每库唯一 writer、引用计数、空闲卸载 |
| `internal/database/cache` | 本地缓存目录、配额、淘汰、冷启动 |
| `internal/s3` | S3 配置、前缀隔离、凭据、对象检查和恢复辅助 |
| `internal/llmgateway` | 对 `litellm` 的服务端封装、路由、流式转发与计量 |
| `internal/usage` | 数据库和模型调用配额、计费事件与聚合 |
| `internal/observability` | 日志、指标、追踪和审计事件 |

现有 `/litellm` 保持为独立的多供应商 Go 客户端能力；服务端只通过 `internal/llmgateway` 调用它，避免把 HTTP 鉴权、租户配额和持久化逻辑侵入客户端库。

## 5. S3 在线数据库设计

### 5.1 数据布局

对象键使用内部 UUID，不直接使用用户提供的名称：

```text
simplebase/{environment}/tenants/{tenant-id}/databases/{database-id}/
  data/                 # Turso 管理的在线持久对象
  metadata/descriptor   # SimpleBase 数据库描述与格式版本
  metadata/state        # 生命周期状态和最后验证信息
  backups/{backup-id}/  # 可选的独立恢复点
```

必须避免 SimpleBase 自己修改 `data/` 内由 Turso 管理的对象格式。`descriptor` 保存 format version、database ID、tenant ID、创建时间、Turso 版本、S3 区域和加密策略，不保存凭据。

### 5.2 打开与写入流程

1. API 完成 tenant/project/database 权限校验。
2. Registry 以 database ID 加锁；已有连接则复用，没有则创建初始化任务。
3. Database Runtime 使用该库专属 S3 前缀和本地缓存目录构建 Turso DSN。
4. Turso 从本地缓存或 S3 恢复必要状态，完成完整性和版本检查。
5. Registry 将数据库状态从 `opening` 切换为 `ready`，后续读写复用同一 connection manager。
6. 写事务通过该库唯一 writer 执行；提交结果按 Turso 的持久化确认级别返回。
7. 数据库空闲后关闭连接并释放可重建缓存，不删除 S3 持久对象。

建议状态机：

```text
creating → opening → ready → closing → closed
                  ↘ degraded
ready → deleting → deleted
closed/degraded → recovering → ready
```

### 5.3 单写路由

- Registry 使用 `map[databaseID]*DatabaseHandle` 和每库互斥控制初始化、关闭与删除。
- `DatabaseHandle` 持有唯一 `*sql.DB`、状态、活动请求数和最后访问时间。
- 所有写接口只能从 Registry 获取 `ReadWrite` handle，禁止绕过 API 直接构造 DSN。
- 管理接口与 SQL 接口共享同一 Registry，避免创建、删除与执行 SQL 并发冲突。
- 进程重启后 Registry 从 Catalog/S3 descriptor 重建，不把内存状态视为持久事实。

API 路由只能保证**单进程内**单写。因此部署文档必须将“仅一个可写实例”设为硬性要求，并在启动时使用部署级 instance identity 做基本冲突检测；检测只能防误配置，不能宣称具备分布式锁语义。

### 5.4 S3 故障与恢复

- 鉴权失败、bucket 不存在、格式不兼容时禁止打开数据库。
- S3 超时或不可用时，写请求失败或进入明确的只读/不可用状态；不得静默返回持久化成功。
- Server 崩溃后，新实例仅使用 S3 与 descriptor 恢复；本地缓存可以完全丢弃。
- 定期执行隔离恢复测试，确认 S3 数据能够在空缓存节点打开并通过 `PRAGMA integrity_check`。
- 独立备份仍有价值，但它是在线数据之外的恢复点，不改变 S3 作为在线持久层的定位。

## 6. 数据库 API

### 6.1 管理 API

- `POST /v1/projects/{project}/databases`：创建 logical database。
- `GET /v1/projects/{project}/databases/{id}`：查询状态、容量和最后持久化信息。
- `POST /v1/projects/{project}/databases/{id}/open`：预热数据库。
- `POST /v1/projects/{project}/databases/{id}/close`：释放本地资源。
- `POST /v1/projects/{project}/databases/{id}/backups`：创建独立恢复点。
- `POST /v1/projects/{project}/databases/{id}/restore`：恢复为新数据库后受控切换。
- `DELETE /v1/projects/{project}/databases/{id}`：先软删除，再异步清理 S3 对象。

### 6.2 SQL API

- `POST /v1/projects/{project}/databases/{id}/query`
- `POST /v1/projects/{project}/databases/{id}/execute`
- `POST /v1/projects/{project}/databases/{id}/batch`

约束：

- 使用参数化 SQL，不拼接用户输入。
- 强制 context deadline、最大批次数、最大返回行数、请求体大小和并发数。
- 事务必须绑定同一数据库 handle 和连接，不能跨 HTTP 请求长期悬挂。
- DDL、恢复、删除等高风险操作单独授权并写入审计日志。
- 旧 `/api/v1/createdb`、`execute`、`query` 仅提供限期兼容层。

## 7. LLM Gateway

### 7.1 首期能力

- 复用 `/litellm` 已有 provider、router、stream、resilience 和统一请求模型。
- 提供统一 Chat/Responses 风格 API，支持普通响应、SSE 流式响应、工具调用和结构化输出。
- 按 project 配置允许的 provider、model、默认路由、超时和预算。
- 支持平台托管密钥与用户自带密钥（BYOK）；密钥加密保存且绝不写日志。
- 记录 provider、model、token 用量、延迟、状态码和成本，不默认持久化 prompt/response 正文。

建议接口：

- `POST /v1/llm/chat/completions`
- `POST /v1/llm/responses`
- `GET /v1/llm/models`
- `GET /v1/projects/{project}/usage`

### 7.2 与数据库的关系

Database 与 LLM Gateway 共用 project、认证、配额、审计和 usage 基础设施，但运行时解耦：

- LLM 上游故障不能阻塞数据库 API。
- 数据库 S3 故障时，LLM 转发仍可工作；用量事件先进入有界内存队列，恢复后写入，队列满时采用明确的降级策略。
- 首期不允许模型自动访问用户数据库；未来的 SQL 工具调用必须由用户显式授权并执行只读/限额策略。

## 8. Catalog、认证与配额

首期 Catalog 不承担分布式选主，只保存产品元数据：

- tenants、projects、API keys；
- database ID、名称、S3 前缀、状态和格式版本；
- LLM provider 配置引用、模型策略和预算；
- usage 聚合、操作记录和恢复任务。

Catalog 可先由独立的 SimpleBase 系统数据库承载，并同样持久化到专属 S3 前缀。Catalog 只能由唯一 Server 实例访问。用户数据库与 Catalog 使用不同前缀和权限，防止用户 SQL 影响平台元数据。

认证至少区分 `database:read`、`database:write`、`database:admin`、`llm:invoke` 和 `project:admin`。所有资源请求先绑定 project，再解析 database/model，避免仅凭资源 ID 越权。

## 9. 安全与可观测性

- S3 bucket 默认私有，启用 TLS、SSE-KMS/等价加密、最小权限 IAM 和版本控制。
- 每个环境、Catalog 和租户使用可隔离的前缀与访问策略；生产凭据不写入仓库。
- LLM 密钥来自密钥管理服务或加密配置，日志只输出 provider/key ID 摘要。
- 审计数据库创建、删除、恢复、DDL、密钥变更和管理操作；SQL 参数与 LLM 正文默认不记录。
- 数据库指标：打开数量、冷启动延迟、查询延迟、事务失败、S3 延迟/错误、持久化水位、缓存命中和容量。
- LLM 指标：provider/model 请求量、首 token 延迟、总延迟、token、成本、限流、重试和流中断。
- 健康检查区分进程存活、Catalog 可用、S3 可用和 LLM provider 状态。

## 10. 现有代码迁移与清理

| 现有内容 | 处理方式 |
| --- | --- |
| `/litellm` | 保留，作为 LLM 客户端核心；新增服务端 gateway 封装 |
| `internal/database/db`、`driver`、`node`、`walfs`、`gorm_driver` | 冻结并提取契约测试，随后由 `tursogo` 数据层替换 |
| 自研 VFS、`sqlite3vfs`、`vfsextend`、Raft 数据面 | 新链路验收后删除，不迁移实现 |
| `internal/s3` | 重写为 Turso S3 配置、隔离、检查和生命周期辅助；不自行实现数据库对象格式 |
| `server` 与旧 HTTP 路由 | 迁移至新的 API Gateway，保留临时兼容层 |
| 现有领域模型 | 保留，通过 repository/connection factory 接入新数据库层 |
| 日志、Prometheus、工具包 | 仅复用通用且有测试的部分 |

存量数据库迁移：生成一致快照 → 导入 Turso/S3 新库 → 完整性与数据校验 → 双读比对 → 短暂停写 → 最终增量/快照 → 切换路由 → 旧库只读保留。整个过程不要求删除或重写领域模型。

## 11. 分阶段实施

### 阶段 A：Turso + S3 最小闭环

- 使用 `turso.tech/database/tursogo` 完成创建表、事务、参数化查询和并发测试。
- 配置 S3 在线持久层，验证空本地目录冷启动、重启恢复和提交持久化语义。
- 验证 S3 中断、超时、权限失败及本地缓存丢失时的行为。

**验收**：删除整个本地缓存后，能仅依赖 S3 打开数据库并通过数据及完整性校验；不会在 S3 未达到承诺持久级别时返回成功。

### 阶段 B：单实例云数据库服务

- 实现 Catalog、Database Registry、生命周期状态机和唯一 writer 路由。
- 实现数据库管理 API、SQL API、认证、限额、审计和指标。
- 实现缓存容量、空闲关闭、冷启动、软删除和恢复点。

**验收**：同库并发打开只产生一个 handle；创建/删除/SQL 不发生生命周期竞争；进程重启可从 S3 恢复所有 ready 数据库。

### 阶段 C：LLM Gateway

- 封装 `/litellm`，提供统一非流式与 SSE API。
- 实现 project 级 provider/model 策略、BYOK、超时、重试、配额与 usage。
- 保证数据库和 LLM 故障域隔离。

**验收**：至少两个 provider 可通过统一接口调用和流式返回；鉴权、限额、用量及敏感信息脱敏测试通过。

### 阶段 D：迁移与清理

- 迁移非关键存量数据库，完成校验、切换和回退演练。
- 压测 S3 冷启动、数据库并发、缓存淘汰和 LLM 流式连接。
- 新链路稳定后删除旧 VFS/Raft/WAL 复制及无用依赖，保留模型。

**验收**：业务回归、恢复演练、安全检查和容量基线通过；仓库不再存在两套可写数据库链路。

### 阶段 E：后续平台化（不属于首期）

- 多 API 实例、强一致 Catalog、分布式 writer ownership 与 fencing。
- 自动只读副本、区域调度和边缘读取。
- Serverless 函数运行时及数据库/LLM 安全绑定。
- Auth、Storage、Realtime 和管理控制台。

## 12. 主要风险

| 风险 | 应对 |
| --- | --- |
| 把 S3 当在线层后写延迟升高 | 明确 Turso 持久化语义、批量事务、本地缓存与性能基线 |
| 上游异步持久化造成提交语义误解 | API 暴露持久化级别和水位，故障测试覆盖已确认/未确认写入 |
| 两个 Server 误连同一数据库并写 | 首期副本数固定为 1、启动冲突检测、最小写凭据；文档明确不支持多写实例 |
| API 路由被误认为分布式一致性 | 明确只保证单进程，扩容前必须引入强一致 ownership/fencing |
| S3 短暂故障导致数据风险 | 停止确认持久写入、明确降级状态、恢复后校验 |
| Catalog 与用户库相互影响 | 独立数据库、前缀、权限、配额和恢复流程 |
| LLM 密钥或内容泄露 | KMS/密钥服务、日志脱敏、正文默认不落库、细粒度权限 |
| LLM 长连接消耗数据库资源 | 独立连接池、队列、限流、超时和故障隔离 |

## 13. 下一步

1. 按官方文档创建 `tursogo + S3` PoC，首先验证在线持久化确认语义和空缓存恢复。
2. 固化单实例部署约束、S3 前缀与 IAM 设计、Database Registry 接口。
3. 定义数据库、LLM、认证和 usage 的 v1 API 契约。
4. 提取现有数据库行为测试和 `litellm` 路由测试，再进入阶段 B/C 实现。
