# SimpleBase `internal/` 架构改造计划：按业务域分层的模块化单体

- 日期：2026-11-25；状态：**提案，未实施**。本轮只交付计划。
- 评估范围：`internal/` HTTP 入口、用例编排、基础设施适配与 `app` 组装。不改变部署拓扑、公开 API、数据库 schema 或存储格式。
- v2 修订（同日）：补充 §8「全 `internal/` 合理性评估」，覆盖 `systemdb`、领域包、运行时引擎、宿主适配与 `app`；据此调整 §3.2、H1/H2 与 §7。**§8 与前文冲突时以 §8 为准。**
- v3（同日）：架构范式对比（HMVC / 垂直切片 / 六边形 / DDD）与待决策项移到 [方案对比决策稿](internal-architecture-options-plan.md)。注意 §8.4 S1 与 A1 引用的 `plan_handler.go` / `runPlan` 已随 planv6.8 B14 删除，生命周期问题以决策稿 §1.2 P1 为准。
- 证据口径：以本次读取的工作树为准（HEAD `dead32f`，工作树已有其他改动）；`go list -mod=readonly` 在 Go 1.26.1 / darwin arm64 上可解析。下文的测试、压测和上线门禁是**未来执行项**，不是本次实测结果。
- 与 [已有精简计划](internal-simplify-refactor-plan.md) 的关系：该计划解决文件内重复、长函数、装配可读性等问题；其 §10 已落地路由中间件与若干 handler 的**同包拆分**。本提案解决**包边界与用例所有权**，不重做 §10，也不恢复其明确淘汰的抽象。以 [后端约束](../../internal/AGENTS.md) 和 [项目约束](../../AGENTS.md) 为兼容基线。

## 1. 结论与选型

**合理，推荐“按业务域组织 + 域内轻量分层”的模块化单体。** 保留 HMVC 中有价值的部分：每个业务域有自己的入口、请求/响应适配和业务编排；不采用控制器递归调控制器、每个模块自带容器或路由器的严格 HMVC。这里的 View 多数是 JSON/SSE/二进制编解码，真正的 SPA 已在 `ui/`，不再为后端制造 `model/view/controller` 三棵目录树。

| 方案 | 优点 | 对 SimpleBase 的代价 | 结论 |
| --- | --- | --- | --- |
| 保留现状，继续按文件拆分 | 改动小，能降低单文件复杂度 | 同包私有符号仍全域可见，编译器不会阻止跨域耦合；HTTP 仍可能持有调度/持久化编排 | 继续作为局部整理手段，不作为最终边界 |
| 严格 HMVC（父/子控制器或独立子路由器） | 子树分工显式 | 认证、委托、项目上下文、写守卫、限流和路由派生容易出现第二套；协议之间已经有明确的共享入口 | 不选 |
| 单体内按业务域分包，必要时加用例层 | Go 包边界可检查；只改变代码组织；兼容分批迁移 | 要先锁定路由与副作用；拆包有少量适配成本 | **推荐** |
| 完整 Clean Architecture / 微服务 | 隔离更强 | 全量复制 DTO/仓储，或引入远程调用和新的故障模式；不匹配当前单写实例 | 非本轮目标 |

触发拆分的不是 API 数量本身，而是职责和变化原因不同：协议变更不应牵动持久化；同一业务规则被 HTTP、cron、事件触发等多个入口消费时，不应由某个 HTTP handler 持有。只有单一调用点且已经是薄转发的代码，不额外造一层。

## 2. 现状：已分层，但 `api` 同时承担了多种职责

| 观察 | 本次核查的代码 | 改造含义 |
| --- | --- | --- |
| `api` 为单个大 Go 包 | `go list` 主机构建口径下 `internal/api` 123 个生产 Go 文件、23 个直接 `internal/` 依赖；另有 103 个同目录 `*_test.go`（文件数，不是用例数）。`app`、`systemdb` 分别有 20、38 个生产 Go 文件 | 问题不是没有分层，而是 `api` 把多个业务控制面和运行时适配放进同一个包。测试也不能因拆包被删掉 |
| 路由已有分组，没有必要从零设计 | [`router.go`](../../internal/api/router.go) 的 `NewRouter`，以及 [`routes_v1.go`](../../internal/api/routes_v1.go) 的 `mountV1Routes`、`mountDatabaseRoutes`、`mountSystemRoutes`，另有 Agent、函数、E2B 分组文件 | 保留一棵 Echo 路由和实际路由派生的 OpenAPI；拆 handler 不等于马上分散注册 |
| 存储与用例适配被 HTTP 包持有 | [`adapter.go`](../../internal/api/adapter.go) 同时含 DB/SQL lease、配额、审计和 LLM 适配；[`assemble_agent.go`](../../internal/app/assemble_agent.go) 给网关注入 `api.NewLLMTraceRecorder` | 请求协议转换留在入口；可以被网关、后台任务复用的适配由业务边界或 `app` 装配持有 |
| 非 HTTP 运行时穿过 API 类型 | [`assemble_api.go`](../../internal/app/assemble_api.go) 为事件分发装 `api.RecordEventFanout`、`api.TriggerEventPublisher`，为 cron 装 `api.GroupCronRunner` | 事件订阅和群协作执行各有明确所有者；HTTP 不应负责启动或驱动调度 |
| 同一业务有多个兼容入口 | [`mcp_dispatch.go`](../../internal/api/mcp_dispatch.go) 的 `subRequest` 经同一 Echo 路由树；E2B HTTP/WS 与 SSH 有独立认证语义；Chat 与 Responses 复用 LLM 网关 | 不把现有协议复用误判为控制器层调用。先拆内部实现，保留经过完整守卫的桥接方式 |

## 3. 目标结构：先定依赖，再决定目录

```text
cmd/simplebase
  └─ internal/app                         唯一组装点：配置、构造、启动、就绪、回收
       ├─ internal/api                    全局路由、门禁、兼容输出、迁移期未拆入口
       │    └─ internal/api/<domain>http  可选：本域 HTTP handler、DTO、协议编解码
       ├─ internal/usecase/<domain>       可选：有证据需要抽出的应用用例
       └─ 现有业务 / 基础能力              backup / collection / functionrun / trigger /
                                           cronjob / sandbox / llmgateway / agentbridge /
                                           auth / catalog / systemdb / dbbridge / objectstore …
```

示意依赖（箭头为允许的调用方向，是否直接 import 由消费方契约决定）：

```text
app ──→ api / 各入口构造器 / 用例 / 基础设施
api ──→ <domain>http ──→ 用例或现有业务契约
用例 ──→ 自己定义的最小端口 + 必要的稳定类型（例如 auth.Principal）
基础设施实现 / 宿主适配 ──→ 实现端口所需的稳定类型，必要时由 app 做类型转换
```

### 3.1 规则和例外

1. **入口负责协议**：Echo 的参数解析、HTTP 状态、`WriteError`、OpenAI/E2B 错误形状、SSE/WS 帧、下载头、请求级审计上下文留在入口。若抽出共享 `internal/api/httpkit`，只放实际已有多处复用的传输原语；它不 import `api`、域入口或存储实现。没有复用证据就用少量窄回调，不为试点先建公共工具包。上下文 key 始终只有一份实现。
2. **用例负责业务顺序**：项目/资源归属、状态转换、业务授权、事务/持久化后副作用及失败补偿随用例或既有业务包走。`context.Context` 负责取消传播，必要身份、项目 ID 与操作参数显式传；不用 `echo.Context`、请求全局变量或通用 `Dependencies` 当用例参数。路由守卫照旧；如果未来新增非 HTTP 入口，必须经该入口原有的鉴权和受信 scope 构造，再执行用例内资源与系统库约束。
3. **接口定义在消费方**：已有 `backup.Service`、`functionrun.Runner`、`sandbox.Manager`、`llmgateway.Service` 等能直接满足窄端口就复用，不复制 `Service/Repository/Model`。无业务逻辑的适配若只用于装配可留 `app` 私有；属于现有 `dbbridge`/`agentbridge` 职责的适配沿用其边界。禁止领域级“万能仓储”、Echo/SQL DTO 大面积透传、通用 `common`/`manager` 兜底包。`*sql.DB`、lease、驱动和持久化行类型不得成为**新用例**的接口契约；必要时只保留语义明确的会话或事务能力。
4. **禁止反向和横向耦合**：`api/<domain>http` 不 import `api`、同级 HTTP 包或 `app`；新 `usecase/*` 不 import 任意入口、Echo、`app` 或存储/驱动具体实现；域间共享走明确所有者的窄契约或现有事件，不互调 handler。引入新跨域依赖必须写清调用方、所有者与失败传播。`api/perfbench → app` 是现存压测入口例外，架构检查按包路径精确排除，不借此放宽生产包限制。
5. **保持独立模块**：`godatabase`、`gocloudagent`、`gosandbox`、`gofunction` 继续不 import 根模块；不改 `go.mod`、配置来源、对象键/manifest、系统库 schema 或现有 CLI/Echo 入口框架。`systemdb` 继续是常驻连接与仓储，不按每个域新开连接。
6. **只设一个组合入口**：`app` 构造、注入和关闭对象；`NewRouter` 挂载、连接路由级依赖及仅限入口的中间件。新域不得自行创建数据库连接、起后台 goroutine 或另开 Echo。迁移期 `api.Dependencies` 保留为装配兼容面；新模块仅收自己的 handler/配置/端口，不能接收整包依赖。只在替换现有调用方之后缩减对应字段；不承诺全量消灭该类型。

### 3.2 业务归属及源码落点

目标是**所有权地图**，不是要求一次创建全部目录。

| 域 / 层 | 现有落点 | 建议归属与迁移界线 |
| --- | --- | --- |
| 入口核心 / 身份 / 项目 | `router.go`、`routes_guards.go`、`context.go`、`policy_gate.go`、`route_meta.go`、`error.go`、`auth_handler.go`、`enduser_*`、`api_keys_*`、`projects_handler.go` | 统一守卫、路由 metadata、错误在 `api`；身份继续在 `auth`、项目元数据在 `catalog`。先理清 API Key 签发/吊销与 `NotifyWrite`，不把认证入口、业务授权或项目初始化拆成第二套 |
| 备份 / 恢复 | `backup_handler.go`、`mountBackupRealtimeRoutes`、`app/assemble_backup.go`、`internal/backup` | 首个 `api/backuphttp` 试点；复用 `backup.Service`、仓储和调度器。不迁实时订阅，不创建备份用例包 |
| 数据 / 实时 | `database_handler.go`、`sql_*`、`schema_*`、`collection_*`、`data_handler.go`、`kv_*`、`realtime_handler.go` | 数据库生命周期仍 `catalog`，用户库仍 `dbbridge`/`godatabase`，集合规则仍 `collection`；先挑一个集合变更验证 `usecase/data`，再决定扩到 SQL、schema、KV。SSE 订阅属于入口，`realtime` hub 仍是订阅方；不会随文件移动而改变数据提交顺序 |
| 对象存储 / 站点 | `s3_*`、`site_*`、`app/assemble_storage.go`、`objectstore` | `fileshttp` 按需承接管理路由、匿名下载、站点响应；全局站点改写仍由 `NewRouter` 在 `e.Pre` 注入。对象/索引/发布设置确需独立编排时再引入用例，不动 `objectstore` 存储契约 |
| 自动化 | `gofunction_*`、`function_trigger_*`、`cronjob_*`、`event_fanout.go`、`app/assemble_api.go` | `/go`、调试、触发器、cron 共用 `functionrun.Runner`。`trigger` 与 `cronjob` 保留运行时职责；事件生产方持窄发布端口，消费方在 `app` 接线。先迁桥接再考虑 HTTP 子包 |
| 模型 / Agent / MCP | `llm_handler.go`、`oai_*`、`llm_trace_recorder.go`、`agent_*`、`collab_*`、`plan_handler.go`、`mcp_*` | OpenAI 协议与 Agents API 分属不同入口；先把 trace 持久化适配从 HTTP 移到装配或所属业务边界，协议不重写。运行、审批、重试、协作等只有跨 HTTP/cron 或需独立测试的编排才抽；宿主 Agent 策略仍 `agentbridge`，通用运行时仍 `gocloudagent`。内置 MCP 留在入口侧，保持路由复用 |
| 沙盒 / E2B / SSH | `sandbox_*`、`e2b_*`、`routes_e2b.go`、`app/assemble_sandbox.go`、`sandbox/sshd` | 管理 API、E2B REST/WS、SSH 是不同传输边界；共享 `sandbox.Manager` 和既有驱动。先核对 `bindSandboxSSHAuth`、TerminalBridge 和关闭顺序，避免复制鉴权/票据逻辑 |
| 系统库持久化（v2 补） | `systemdb`（38 个生产文件 / 9.7k 行，覆盖 Agent、cron、函数、沙盒、站点、MCP、日志、指标、trace、设置） | 保持单包、单常驻连接与 `schema.go` 最终态；不按域拆物理包。新用例只依赖消费方窄接口，不再新增对 `*systemdb.Store` 具体类型的依赖。详见 §8.3 |
| 运维 / 观测 | `health.go`、`observability_routes.go`、`system_*`、`quota_audit_handler.go`、`usage`、`audit`、`observability` | 保留应用健康和独立观测监听；复用现有采集、审计、配额模块。优先收口入口依赖，不为只读查询建立用例包装 |

## 4. 不可回退的行为契约

| 边界 | 必须锁定的契约 |
| --- | --- |
| 路由与身份 | 方法、路径、参数、挂载条件与中间件顺序不变。保留 `e.Pre` 站点分流、公开健康/登录路由、认证→post-auth 限流→受保护项目组上下文、现有 end-user 独立鉴权、最后的 SPA fallback。对照 `require`/`delegable`、`policyGate`、`writeGate`，权限仍由 `auth.Principal.HasPermission` 体系判定。已删除路由仍返回原有 404。MCP 工具及 OpenAPI 的可调用范围不扩大 |
| 实例与系统库 | 实例只读仍拒绝写；对原本未挂载的入口保留未挂载，不能统一改写成 503。系统库用户入口只走 SELECT 与只读 `systemLeaseAdapter`，不经用户库 registry factory；写 handler 防线和 lease 防线都保留。系统内部维护的 `systemdb.Store` 路径不受影响；admin 日志/监控查询保留全局视图。用户删库仍只清理用户表/视图/序列与集合元数据，KV、文件、备份保留；保持 lease 取租/持租/失租和 checkpoint 相关契约 |
| 事件与生命周期 | 逐操作记录“变更、提交、`NotifyWrite`、索引、事件、审计”的现有时序，按路径保持，不能一概改成“提交即发布”。例如集合导入成功 `Commit → NotifyWrite → PublishRecord`；错误场景也保持原结果和副作用次数。实时 hub 与有界触发器队列不持久化、不承诺 exactly-once、不覆盖 SQL 直写。cron/trigger/realtime 的取消、排空、启动失败回收顺序不变；函数通用槽位只一份 |
| HTTP 与流 | 保留标准错误映射及顺序、错误优先级、request ID、JSON 的 `null`/`[]`、可见响应头、状态码。Chat 的双 `/v1`、首帧 assistant/`[DONE]`、Responses 的终止事件及无 `[DONE]`、SSE flush/取消，E2B/SSH 协议与权限分别测；继续使用现有 SSE 发送机制，不复造。匿名站点与控制台下载的 HTML 行为、路径/子域两种模式和 Cookie 域边界保持不变 |
| 持久化与日志 | 写入和同步通知次数不变；配置只用当前 YAML/环境变量机制；存储 key 与 manifest 不变。审计（含操作者）和日志保持既有脱敏，Agent trace 默认关闭、打开时仅走规定的脱敏并截断的 zap 通道；不扩大凭据或正文传播范围 |

## 5. 迁移批次（每批可单独交付）

### H0 · 固化行为基线（第一批，必做）

- 在实际实施分支重新跑依赖图、构建和回归基线，记录 HEAD、配置矩阵、工具链、已有失败/跳过原因；保留原工作树改动。不拿 2026-11-23 精简计划的数据代替新基线。
- 做路由表/协议特征测试：在固定 fake 装配下对比排序后的 `(method, path)` 与既有 OpenAPI 暴露集，不以 handler 名或注册所在包当契约。补 HTTP 行为矩阵而不只测路径：匿名、合法登录态/Key、跨项目、具/不具委托权限、读写实例、模块依赖存在/缺失、公开站点/LLM/E2B 组合。对可选资源明确 404、403、503 产生在哪一层；不盲目统一。
- 建立架构门禁：用 Go 标准库 AST 读取 repo 源码 `ImportsOnly`（含 tag 文件的生产 import），结合 `go list` 核对当前构建真实图；首批只强制 §3.1 对**新包**的规则，既有边界按包列基线，不搞全仓重写。输出违例 `importer → imported` 与文件，测试引入一条模拟违例能失败。既有例外写明原因、去除批次，不按 `*` 全局豁免。
- **完成定义**：基线矩阵可重复、路由差异可被发现、门禁能判好/坏；提交不改变生产行为。失败则先补基线，不进入 H1。

### H1 · 备份垂直切片试点（必做，检查是否值得扩张）

1. 先补 [`backup_handler.go`](../../internal/api/backup_handler.go) 的路由级回归：五条 `/v1/projects/:projectID/backups…`，检查 `project:admin`、默认不可委托、跨项目、写门禁、系统项目不允许访问。直调 handler 测既有守卫顺序（先系统项目再实例可写），别将路由层已先拒绝的错误当成 handler 的输出；备份不可用依赖时仍不挂载。
2. 新建 `internal/api/backuphttp`，只搬 handler、窄 `BackupService` 契约（既有 `backup.Item` 原样）、下载响应逻辑。初版 **路由继续在** `mountBackupRealtimeRoutes`，只换备份 handler 的引用，Realtime 原地不动。`app` 构造/传入候选 handler，`api` 可提供一个窄兼容构造器，在其中接项目上下文、错误输出和审计的实际实现；子包接明确的传输回调，例如解析 scoped ID、写错误、记录操作，不反向 import `api`。`backupAPIError` 与 `WriteError` 保留同一映射，不新建第二张错误表。
3. [`assemble_backup.go`](../../internal/app/assemble_backup.go) 继续只创建一个 `backup.Service` 共享给 HTTP 和调度器。启用条件、readiness 仓储、catch-up、启动失败回收保持原样。移动构造引用和 handler 的变更放同一提交，不做双注册或双写；迁完再确认是否值得把 HTTP 构造兼容函数缩小/删除。
4. **验收**：五条路由的方法与响应一致（列表 `items`、创建 201、删除/恢复 204、下载状态与三个相关响应头/reader 关闭）；备份业务错误保持原 status/code/request ID，审计 success/error 次数、内容、调用顺序不变；`backuphttp` 不 import 根路由包、Echo fake 能独立测备份而不启真实库。`TestBackupRestoreAndIntegrity`、`TestNewInvalidBackupScheduleClosesResources` 与 H0 路由门禁通过。
5. **继续/停止**：若唯一收益是把代码搬进目录，或者必须暴露大量通用 helper、只能靠重复守卫才能迁移，则撤回 H1 的生产补丁、保留契约测试，继续同包整理。通过标准是：备份改动能在局部完成、路由核心边界未扩大、只有确有必要的兼容回调。

### H2 · 脱钩非 HTTP 适配，再验证一个真实用例（试点通过才做）

1. **H2a：无行为搬移**。把 `api.NewLLMTraceRecorder` 的网关→系统库转换移到 `app` 私有适配（或核实只有单一职责后放所属适配包），迁映射测试与 nil 语义，不移动 LLM HTTP 查询或协议。把 `event_fanout.go` 中具体触发器适配/订阅组装从 `api` 剥离：生产方端口只负责发布，`app` 注册触发器和 `realtime` 消费方。先保留当前数据类型与时序对照；避免为少量进程内事件引入总线。若移动要求 `trigger` import `api`，由 `app` 做显式转换，不反向引用。
2. **H2b：集合变更切片**（v2：**已被 §8.6 的 A2 取代**，下文保留作原始思路记录；集合文档写入目前只有 HTTP 一个入口——内置 MCP 经 `subRequest` 回到同一路由，Agent 的数据库工具走的是 SQL 适配 `api.NewCloudAgentDB` 而非集合写入——单独建 `usecase/data` 不满足 §1 的拆分条件）。从 `DataHandler.UpdateDocument`（PUT，见 [`data_handler.go`](../../internal/api/data_handler.go)）及其 `loadDocumentForWrite`/publish 调用点制定一张时序表，先用 spy 锁定成功、资源不存在/错项目、只读实例、系统库、持久化失败及取消。把独立于 HTTP 的规则提取到 `usecase/data`（若 `collection` 已足够，优先扩现有包）：输入显式携带解析后的作用域和记录数据；使用消费方定义的集合写入端口和已有变更事件契约；保留现有 `WriteError`、DTO、内容校验顺序、事件数量。不能泄露 `Raw()`/DB lease 到新用例。PATCH、批处理、导入仍走原路径，验证无共享 helper 回归再陆续迁移。
3. **验收**：trace 注入不再依赖 HTTP 包；事件核心/映射不依赖 Echo；新用例用假端口验证顺序与取消，完整 HTTP 集成保持原行为，配置不变；`TestFunctionTriggerHTTPAndRecordEndToEnd` 和 `TestSQLHandler_DurabilitySemantics` 不退化。无法维持旧时序则停在 H2a，H2b 单独复核事务边界。

### H3 · 按域扩面（每域单独 PR，预算内停）

- 优先考虑 Data 的其余资源和 Files（先管理 API，再匿名 `/files/:token`、站点分流）；只搬**有明确所有权与回归门禁**的部分。Data 同时涉及 Project→KV 初始化、删库清理、系统库只读桥、审计和持久化，不批量迁全套；Files 先锁住站点可见性、索引写入和事件发布次序。
- 之后评估 LLM 协议与自动化入口。Chat/Responses 仍走同一网关；`oai_responses.go` 是单一协议转换，除非有独立证据，不按行数再次拆解。函数与 Cron 依赖保持唯一执行器、全局槽位与 API/定时目标语义。
- Agent/协作/MCP 及 E2B/SSH 最后，按运行编排、兼容协议、票据与生命周期分别验证；先把 `GroupCronRunner` 里与 HTTP 无关且可复用的编排交给 Agent 用例或现有 `agentbridge`，保留 SSE/HTTP 帧边界。内置 MCP 继续按 `subRequest` 走完整路由（包括携带受信主体的进程内调用）；**本计划不把它改为直调用例**，也不重写外置 MCP 与宿主工具的策略。
- `api` 保留中心路由定义直至有明确收益；如果同一域拥有多个独立入口且集中注册成为瓶颈，再让域包提供显式 `Mount(...)`，接收上层已建好的路由组和窄守卫（权限、委托、写入），不自建根路由/守卫，也不接受全量依赖或动态发现。
- **验收**：迁移域依赖方向、路由及写副作用对账全部通过；旧实现删除，不保留无调用 shim。未迁身份、运维、协议仍可留在 `api`，不是阻塞版本的遗留垃圾。

### H4 · 关闭过渡面（有实际迁移才做）

- 清理 H1–H3 实际产生的兼容构造器、临时接口和废弃 `Dependencies` 字段；只在所有消费方已切换后删除。依赖门禁从新增包收敛到已迁域；在 [`internal/README.md`](../../internal/README.md)、`internal/AGENTS.md` 及必要的根说明中更新实际依赖图、例外和新增 API 的模板，不写计划态架构为现状。
- 发布只替换二进制，不改 schema/对象布局；先本地和预发布验证，然后按单写部署流程滚动（不让两个写实例同时持有效写能力）。记录逐批的复盘矩阵与未迁理由；未通过的域单独退回，不能拿“全量完成”作为上线条件。

## 6. 验证、发布与回滚

### 6.1 门禁矩阵

| 门禁 | 怎么验 | 何时启用 |
| --- | --- | --- |
| build / vet / test | `go build ./internal/... ./cmd/...`、`go vet ./internal/... ./cmd/...`、`go test ./...`，且改动 Go 文件 `gofmt` 无漂移 | 每批；根模块 `./...` 不包含嵌套模块 |
| 入口契约 | `TestOpenAPIMatchesRealRouter`、`TestNewRouter_CrossProjectDeniedBeforeHandlers`、`TestDelegation_RouteTable`、`TestWriteGate_AllWriteRoutesBlockedOnReadOnly`、`TestRouteGuardsApplyPolicyGate`、H0 配置/身份快照 | 每批涉及路由、公共守卫或装配 |
| 兼容协议 | `TestOpenAIRoutePaths`、`TestOAIE2E_ResponsesNonStreamAndStream`、`TestB3AgentRouteSetLocked`、`TestBuiltinMCP_SubRequestKeepsRouteErrors`、`TestBuiltinMCPSession_ReadOnlyPrincipalCannotWrite`、`TestSiteHostDispatchBoundaries`、`TestSitePathServeRules`、`TestE2BRouteSetLocked`、`TestSandboxE2B_CrossProject`，按域加下载、SSE 首/终帧、取消与认证测试 | 每个协议改动批次运行相关套件；触及全局中间件则全跑 |
| 并发 / 持久化 | `TestLeaseWriteGateAcquireHoldAndLost`、`TestSQLHandler_DurabilitySemantics`、`TestFunctionTriggerHTTPAndRecordEndToEnd`、`TestRegisterCloserAndClose`、`TestNewInvalidBackupScheduleClosesResources`；对受影响包运行 `go test -race`（hub、调度、lease、关闭竞态），验证失败回收与关停 | 涉及 app、写路径、事件/调度均必跑；没有运行的组合记为未验证 |
| 端到端 | 在验证环境执行 `./scripts/smoke.sh`；本地 DuckDB/SQLite 分别覆盖创建项目、读写、恢复与重启；涉及权限/删库走根约束中的 CLI/示例门禁（含 `scripts/examples-verify/verify.sh`）；涉及实际 S3、OpenAI、E2B 等用相应测试服务验证，没有服务则登记阻塞的验收范围，不把跳过视为通过 | H4 或相关高风险域合并前；有独立模块改动才另跑该模块自己的 suite |
| 性能 | 用 `cmd/perfbench` 加 H0 选定热路径（数据读写、流的首帧/取消；受影响才重跑），同机同配置比较基线；至少 3 轮，记录 p95、alloc/op（有基准时）、错误率；真实变化与测量噪声分开 | 涉及热路径/中间件/流的批次。若复现 p95 增长超过 10%、本批新增分配或错误率上升，先解释或修复再推进；低于阈值不是性能承诺 |

### 6.2 承载与回滚策略

- PR 模板：原调用链 → 新调用链、涉及路径/守卫、状态与写入副作用、保留兼容函数、相关测试和风险。先锁行为，再搬边界，再减少旧层；性能/行为修复另开 PR，避免与等价搬迁混合。
- 同一 API 在任一版本只有一个实现和一次执行。用 `git revert` 回退单批或部署上一已验证二进制；不要保留运行期双写开关。回退不清数据、不 reset 系统库、不改变租约归属；发版期间从旧到新仍按现行单写实例启动/关停。
- 出现路由/权限差异、错误协议/流中断、审计或事件重复、租约异常等，立即停止后续域迁移，回退相关批次并复跑 H0 + 该域测试。H0 可独立保留；未发布新数据格式是允许回退的前提。

## 7. 最小交付范围与成功标准

- **建议 v6.8 只承诺 H0 + H1 + H2a + §8.6 的 A1（计划执行器外移）**；H1 定位为机制验证，A1 才是收益验证。H2b 按 §8.6 改为“集合纯逻辑下沉”，不再建 `usecase/data`。H3/H4 由实际复杂度和回归成本逐域批准，不按目录完成度推进。
- **过程指标**：迁移域打通一条真实调用链（路由→入口→业务/端口→装配），不增加 `api.Dependencies` 的通用字段；新增反向/跨域非法依赖为 0；H0 标记的路由、权限与协议差异为 0；每个新用例有不启动 HTTP/DB 的测试。
- **收益判断**：用“更改备份响应”“修订集合变更规则”“接入另一事件消费者”三类改动演练对比受影响包和 fake 数量。若仍需要同时改多个无关域、仅把复杂度从 `api` 搬进 `app` 或增加独立生命周期，停止扩大迁移，改回同包精简。
- **明确不做**：全量目录搬迁、大重命名、微服务/运行时插件、第二套身份/错误/路由框架、全量仓储接口化、为测试降低业务规则、接口或存储格式升级。

## 8. 全 `internal/` 合理性评估（v2 补充）

前文主要论证 `api`。本节逐层检查其他模块是否也需要“分层改造”，结论是：**问题集中在 `api` 一个包，其他模块大多已经是合格的领域包或基础设施包；对它们套用统一的 controller/service/repository 分层，成本大于收益。** 因此本计划的正确定位是“把 `api` 中不属于入口的代码归还给已有的下层”，而不是“给整个 `internal/` 换架构”。

### 8.1 量化现状（本次 `go list` 与源码统计，生产代码，不含 `_test.go`）

| 包 | 文件 / 行数 | 被 `internal/` 内包直接 import 次数 | 直接依赖的 `internal/` 包 | 定位 |
| --- | --- | --- | --- | --- |
| `api` | 123 / 31,677 | 1（`app`）+ `api/perfbench` | 23 | 入口 + 混入的编排 |
| `systemdb` | 38 / 9,667 | 7 | auth、catalog、dbbridge、objectstore、observability、utils×3 | 系统库持久化 |
| `sbcli` | 22 / 4,119 | 1 | utils/mask | CLI 入口（独立侧） |
| `app` | 20 / 3,657 | 1（`cmd`）+ perfbench | 21 | 组合根 |
| `auth` | 18 / 3,412 | 8 | utils/strutil | 领域（身份） |
| `config` | 10 / 3,301 | 5 | utils/strutil | 配置 |
| `agentbridge` | 19 / 2,680 | 3 | auth、cronjob、llmgateway、sbcli、systemdb… | 宿主适配 + Agent 业务 |
| `catalog` | 10 / 2,283 | 8 | auth、objectstore、observability | 领域（元数据状态机） |
| `sandbox` + `sshd` | 13 / 2,884 | 3 / 2 | config（`Store` 接口由 `app` 适配） | 领域 + 运行时 |
| `objectstore` | 11 / 1,930 | 5 | observability | 基础设施 |
| `llmgateway` | 6 / 1,214 | 4 | catalog、observability、systemdb | 领域服务 |
| `backup` | 5 / 803 | 2 | catalog、config、dbbridge、objectstore | 领域服务（含仓储、调度） |
| `collection` | 3 / 681 | 2 | 无 | 领域纯逻辑 + 仓储 |
| `cronjob` / `trigger` / `functionrun` | 1 / 364、1 / 254、1 / 154 | 3 / 2 / 3 | systemdb（类型）等 | 运行时引擎（窄接口） |
| `usage` / `audit` / `alerts` / `realtime` / `dbbridge` / `observability` | 均 ≤ 606 行 | — | — | 小型领域 / 基础设施 |

`api` 里的分布（按文件前缀聚合）：Agent + 协作 + 计划 + MCP + 运行事件约 7.6k 行，集合 / KV / SQL / schema / 数据约 7k 行，E2B + 沙盒约 3.6k 行，OpenAI 兼容约 1.5k 行；**不 import Echo 的生产文件约 5.5k 行**（如 `collection_aggregate_query.go` 490、`collection_filter.go` 418、`collab_group_store.go` 380、`gocloudagent_access.go` 274、`collab_cron_runner.go` 228、`db_stats_cache.go` 213）。`api` 中有 49 个生产文件直接使用 `systemdb`，调用最多的是 Agent 相关仓储方法（`GetCloudAgent`、`GetAgentTurn`、`ListAgentPlanStages`、`UpdateAgentPlanStage` 等）。

### 8.2 逐层判断

| 层 / 包 | 现状证据 | 需要分层改造吗 | 结论与动作 |
| --- | --- | --- | --- |
| **运行时引擎** `cronjob`、`trigger`、`functionrun` | 各自在包内定义 `Store` / `Runner` / `FunctionRunner` 窄接口，`*systemdb.Store` 只是满足者；单文件、职责单一 | 否 | **已是本计划要的形态**，作为新代码的样板。唯一瑕疵是接口签名里用 `systemdb.CronJob` 等行类型，可接受（同进程、同版本），不为去掉它复制 DTO |
| **领域服务** `backup`、`usage`、`audit`、`llmgateway`、`catalog`、`collection` | 有自己的模型、错误、消费方接口（`backup.Catalog`、`catalog.Repository`、`llmgateway.UsageRecorder`、`catalog.ProjectDataClearer` 等）；`backup` 还自带仓储与调度器 | 否 | 这些包就是“域内分层”的落点，不再在它们上面加 `usecase/*` 包装。`catalog` 的 `service.go` / `sql_repository.go`（742 / 701 行）已是 service + repository 两层 |
| **身份** `auth` | 18 文件，被 8 个包依赖；只依赖 `utils/strutil`；`Principal` / `Permission` 是全系统共享语言 | 否 | 保持为底层共享内核。`catalog → auth`、`systemdb → auth` 是合法向下依赖，不需要为“纯洁性”反转 |
| **系统库** `systemdb` | 单包承载约 20 类实体（agent_tags 573、traces 494、agent_collab 466、cron_jobs 412 行…）；依赖 `auth`、`catalog` 的模型与常量 | **不拆物理包** | 见 §8.3 |
| **宿主适配** `dbbridge`、`agentbridge` | `dbbridge` 166 行纯转换；`agentbridge` 在 `ports.go` 定义 `DatabaseAccess`、`ObjectAccess`、`LogReader`、`Sandbox`、`CommandClassifier` 等端口 | 否（`agentbridge` 局部收窄） | 二者就是“六边形适配器”，方向正确。`agentbridge` 的 `MCPConnector`、`MCPPolicy`、`LogAccess`、`SettingsAccess` 仍接 `*systemdb.Store` 具体类型，可在触及时改成窄接口，不单独立项 |
| **沙盒** `sandbox`、`sandbox/sshd`、`e2bproto` | `sandbox.Manager` 通过 `Store` 接口由 `app` 适配系统库，注释写明“隔离业务层与 SQL”；执行后端来自 `gosandbox/vm` | 否 | 下层已分好；问题在 `api` 的 `TerminalBridge` 与 `app.bindSandboxSSHAuth` 互相引用（router 内造的 `SandboxHandler` 回填给 SSH 认证），见 §8.4 |
| **基础设施** `objectstore`、`observability`、`alerts`、`realtime`、`web`、`utils/*` | 零或极少业务依赖；`utils/*` 只 import 标准库 | 否 | 保持 |
| **配置** `config` | 10 文件 3.3k 行，YAML + env + 校验；被 5 个包依赖（含 `sandbox`、`backup` 直接拿子结构） | 否 | 已有决策（精简计划 A6 / G-1）是“不动五处登记”；本计划沿用 |
| **CLI** `sbcli` | 22 文件 4.1k 行，只依赖 `utils/mask`；与 `sbdev.py` 逐字对账 | 否 | 它本身是一个与 HTTP 平行的入口，经 SDK 调 HTTP，不共享进程内用例；分层改造对它无收益 |
| **组合根** `app` | 20 文件，`App` 结构体约 46 个字段、46 个装配方法；`assembleDeps` 按序构造、`Close` 反向释放 | 局部 | 结构合理（单一组合根是本计划的前提）。问题是存在**环形延迟绑定**和**由 router 内部创建的长期对象**，见 §8.4 |
| **入口** `api` | 见 §2 与 §8.1 | **是** | 本计划主体 |

### 8.3 `systemdb`：为什么不拆成每域一个包

看起来 `systemdb` 是第二个“大泥球”，但拆它的收益很低：

1. **它是一个物理边界**：同一个 DuckLake 连接、同一份 `schema.go` 最终态、同一套 `NotifyWrite` / 异步 flush / 读观测。拆包后这些仍要共享，等于在包之间再导出一层内部 API。
2. **实体之间有事务和外键式关联**：Agent 线程 → 轮次 → 运行 → 消息 → 计划阶段 → 协作群 → 定时绑定，拆开后跨包事务更难写。
3. **问题不在它本身，而在调用面**：`api` 的 49 个文件直接拿 `*systemdb.Store` 调几十个具体方法，导致 HTTP 层成了 Agent 仓储的编排者。

**做法**：

- 不拆 `systemdb`；按需在包内用文件前缀（已经是 `agent_*`、`cron_*`、`sandboxes.go`…）组织即可。
- 新代码遵守 `cronjob` / `functionrun` 的样板：**消费方定义窄接口，`*systemdb.Store` 隐式满足**。迁出 `api` 的编排（§8.6 A1）就按这个方式取仓储，而不是把 `*systemdb.Store` 换个地方继续传。
- 架构门禁（H0）增加一条“只减不增”的计数：生产代码中直接引用 `*systemdb.Store` 具体类型的包与文件数，以本次基线（`api` 49 个文件，另有 `agentbridge`、`app`、`llmgateway` 等）为上限。

### 8.4 前文没有覆盖的真实结构问题

以下几处比“文件太多”更能说明分层的必要性，因为它们是**生命周期和依赖方向**的问题，不能靠同包拆文件解决：

| 编号 | 问题 | 证据 | 影响 |
| --- | --- | --- | --- |
| S1 | **HTTP handler 持有并启动后台工作** | `plan_handler.go` 的 `runPlan` 以 `context.Background()` 在 `go h.runPlan(...)` 中运行，直到计划完成 / 暂停 / 取消；`cloudAgentHandler` 持有 `planLocks`、`groupEngines`、`routeCancels`、`runEventHub`、`confirmHub`，启动时还 `convergeGroupEngines` | 关停时这些 goroutine 不在 `app` 的回收链里；计划执行逻辑无法脱离 HTTP 测试或被 cron 复用。（启动时 `systemdb.InterruptRunningPlans` 会把遗留 running 收敛为 interrupted，所以不是数据错误，而是归属错误） |
| S2 | **组合根与 router 之间的环形延迟绑定** | `collab_cron_runner.go`：`GroupCronRunner` 注释写明“app 的装配顺序是 cron 调度器 → api router，而 handler 在 router 内部创建，因此先返回可 Attach 的句柄”；未绑定时 `RunGroupCron` 返回 `collaboration runner is not attached` | 调度器依赖 HTTP handler 才能跑群定时，方向是 `cronjob → agentbridge → api(handler)`；只要群执行逻辑在 handler 上，就绕不开这个句柄 |
| S3 | **router 内部创建长期对象，再回填给 app** | `app.buildHTTPServers` 在 `api.NewRouter(deps)` 之后执行 `deps.Terminal.Server = a.bindSandboxSSHAuth(deps.Terminal, deps)`，注释为“认证器需要 NewRouter 内构造的 SandboxHandler” | 违背“`app` 唯一组装点”：一部分对象图由 router 决定，`app` 只能事后取 |
| S4 | **非 HTTP 适配挂在 `api`** | `api.NewLLMTraceRecorder` 被 `llmgateway.NewService` 使用；`api.RecordEventFanout` / `TriggerEventPublisher` 被 `assembleFunctionPlane` 用于触发器；`app/assemble_agent.go` 用 `api.NewCloudAgentDB`、`api.NewCloudAgentObj`、`api.NewCloudAgentLLM`（`gocloudagent_access.go`，274 行，不 import Echo）实现 `agentbridge.DatabaseAccess` / `ObjectAccess` 与模型客户端 | 网关、触发器、Agent 运行时为了拿适配器要经过 HTTP 包；前文 H2a 覆盖了前两项，Agent 适配并入 A4 |
| S5 | **领域纯逻辑留在 `api`** | `collection_filter.go`、`collection_aggregate_query.go`（共约 900 行，不 import Echo，只依赖 `collection.Collection` 与 `godatabase.Dialect`）；KV 命令表 `kv_cmd_*.go` 的解析函数签名是 `func(c echo.Context, argv []string)`，Echo 只用于写参数错误和取 ctx | 这两块本应属于 `collection` 与 `godatabase/kv` 的上层；KV 命令语义因此只能经 HTTP 测试 |

S1–S3 合起来说明：**`api` 中最需要移出的是 Agent 运行编排，而不是备份或集合 CRUD**。前文选备份作试点是因为它最安全，但它本身没有上述问题，只能验证“机制可行”，不能证明“分层值得”。

### 8.5 前文设计的偏差与修正

| 前文条目 | 偏差 | 修正 |
| --- | --- | --- |
| §3 目标结构中的 `internal/usecase/<domain>` | 领域服务包已存在（`backup`、`collection`、`llmgateway`、`cronjob`…），再造平行的 `usecase` 树会让同一域有两个“业务层” | **默认把编排放进已有领域包或 `agentbridge`**；只有确实找不到所有者、且有两个以上入口复用时，才新建包。Agent 运行编排是唯一候选（见 A1），包名按域取（如 `internal/agentrun`），不使用通用 `usecase` 前缀 |
| H1 备份试点作为“是否值得扩张”的判据 | 备份 handler 只有 170 行、已经调用 `backup.Service`，无后台任务、无跨层回填 | H1 只验证“子包 + 窄回调 + 路由门禁”的**机制**；“是否值得”改由 A1 判定 |
| H2b 集合变更用例 | 集合文档写入只有一个进程内入口（内置 MCP 经 `subRequest` 回到同一路由；Agent 数据库工具走独立的 SQL 适配），不满足 §1 的“多入口复用”条件 | 改为 A2：把 Echo 无关的纯逻辑下沉，handler 留在 `api` |
| §3.2 中 Agent 域“最后迁移” | S1/S2 说明 Agent 是收益最大的域；E2B/SSH 与 OpenAI 协议才是风险大、收益小的部分 | Agent **运行编排**提前（A1），Agent **HTTP 协议**（SSE、审批、重试的帧格式）仍留在 `api`，不随之搬迁 |
| §3.1 规则 3 只约束“新用例” | 没有约束既有下层包继续新增对 `*systemdb.Store` 的具体依赖 | §8.3 的“只减不增”计数纳入 H0 门禁 |

### 8.6 修订后的批次（替换 H2b，补充 A1–A4）

| 编号 | 内容 | 依赖 | 风险 | 验收 |
| --- | --- | --- | --- | --- |
| **A1** | **Agent 计划 / 群协作执行器外移**：把 `runPlan` / `runPlanStage` / `applyStageProposal` / `nextPlanStage`、群引擎（`groupEngineFor`、`convergeGroupEngines`、`collabGroupMembers`、`collab_group_store.go`）与 `GroupCronRunner` 的执行部分移到 `internal/agentrun`（或经评审后并入 `agentbridge`）。执行器由 `app` 构造，持有自己的 `ctx`、`planLocks`、引擎表，在 `Close` 中取消并等待；仓储经消费方窄接口；`api` 的 `cloudAgentHandler` 只做请求解析、权限、SSE/JSON 输出并调用执行器。`GroupCronRunner` 的 Attach 句柄删除，cron 直接拿执行器 | H0 | 中–高：计划状态机、版本号乐观锁、群轮次认领、SSE 事件顺序 | 消除 S1、S2；`api` 中 `go h.runPlan` 为 0；`GroupCronRunner.Attach` 删除；关停测试覆盖“运行中计划在 `Close` 后不再写库”；`TestB3AgentRouteSetLocked`、协作 e2e、cron 群定时测试不退化；计划 / 轮次状态序列逐条与基线一致 |
| **A2** | **集合与 KV 纯逻辑下沉**：`collection_filter.go`、`collection_aggregate_query.go`、`collection_pick.go` 中与 Echo 无关的部分移入 `collection`（或其子包，避免 `collection` 引入 `godatabase` 方言之外的依赖）；KV 命令表把解析签名从 `echo.Context` 改为返回参数错误值，由 handler 统一转 `WriteError`，命令表随后可移到 `api/kvhttp` 或保留原地 | H0 | 低–中：SQL 生成与错误文案 | 生成的 SQL / 参数、错误 code 与文案逐字一致（金标测试）；`collection` 不 import `api` / Echo；KV 命令可脱离 HTTP 单测 |
| **A3** | **终端 / SSH 装配归位**：把 `SandboxHandler` 中 SSH 认证需要的部分（票据校验、项目沙盒查找）抽成 `app` 能先于 router 构造的对象，`bindSandboxSSHAuth` 在 `NewRouter` 之前完成；router 只接收已构造的 `TerminalBridge` | A1 之后任意时间 | 中：SSH 认证与票据一次性语义 | 消除 S3；`TestSandboxE2B_CrossProject`、终端票据与 SSH 认证测试通过；`buildHTTPServers` 不再在 `NewRouter` 之后回填字段 |
| **A4** | **Agent 宿主适配归位与端口收窄**（随手做，不单独排期）：`api/gocloudagent_access.go` 的 `NewCloudAgentDB` / `NewCloudAgentObj` / `NewCloudAgentLLM` 移到 `agentbridge`（它们实现的就是 `agentbridge` 的端口），SQL 只读守卫与系统库保护语义原样保留；`MCPConnector`、`MCPPolicy`、`LogAccess`、`SettingsAccess`、`llmgateway.NewSystemCredSource` 改为接消费方接口 | 无 | 低–中：Agent SQL 工具的只读判定与系统库拒绝 | `app/assemble_agent.go` 不再从 `api` 取 Agent 适配；§8.3 的具体类型计数下降；fail-closed 测试（`TestMCPPolicyFailsClosedOnUnknownServer` 等）与 Agent 数据库工具测试通过 |

推荐顺序：H0 → H1（机制）→ H2a → **A1（收益验证）** → A2 → A3；A4 随触及的文件顺带。A1 完成后用 §7 的“三类改动演练”重新评估：如果计划 / 协作逻辑的改动不再需要动 `api`，且关停、cron 不再依赖 router，说明分层有效，再考虑 H3；否则停在 A1，其余域只做同包整理。

### 8.7 结论

- **对整个 `internal/`：现有分层基本合理，不建议整体改为 HMVC 或统一的三层模板。** `cronjob` / `functionrun` / `trigger` / `sandbox` 的“消费方窄接口 + `app` 适配”已经是目标形态，`catalog` / `backup` / `llmgateway` / `collection` 已经是域内分层，`systemdb` 作为单一物理持久化边界应保留。
- **对 `api`：需要改，但重点不是按资源拆子包，而是把三类不属于入口的东西还回去**——后台执行与长期状态（S1/S2，A1）、对象图构造（S3，A3）、领域纯逻辑与非 HTTP 适配（S4/S5，H2a/A2）。完成这些之后，`api` 剩下的才是真正的“控制器 + 视图”，是否再按域拆子包（H1/H3）就只是组织偏好，可以按需决定。
- **判断标准**：一次改动是否让“改业务规则不必动 HTTP 包”“后台任务的生命周期归 `app` 管理”“`systemdb` 具体类型的使用面变小”。三条都没有改善的迁移，一律不做。
