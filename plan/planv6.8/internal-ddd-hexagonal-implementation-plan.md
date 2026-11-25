# `internal/` F 方案落地实现计划（战略 DDD + 六边形应用服务）

- 日期：2026-11-25；状态：**已决策，执行中**（自 P0 起，按 §7 顺序推进）。本文是最终实施计划与执行依据。
- **执行确认（同日）**：D6-c（孤儿 run 启动收敛）**不做**；数据面合为一个 `api/datahttp`、沙盒与 E2B 合为一个 `api/sandboxhttp`（§6.2）。本文是唯一依据，不再对照决策稿与分层计划。
- **v2 修订稿（同日）**：对照代码逐条复核 v1，修正 12 处，见下方「修订对照」。其中 R1–R4 是会让 D6「关停时取消并在超时内等待收尾写库」落空的正确性缺陷，R5–R7 与 D3「按业务域全量拆子包」不一致，R8 是门禁基线无法复现。
- 决策来源：用户已确认 [方案对比决策稿](internal-architecture-options-plan.md) §6 全部六项决策（见 §0）。本文合并该决策稿与 [分层改造计划](internal-layered-hmvc-architecture-plan.md)（v3）；三份文档冲突时**以本文为准**。
- 约束基线：[项目约束](../../AGENTS.md)、[后端约束](../../internal/AGENTS.md)。不改公开 API、系统库 schema、对象键布局、配置来源、`go.mod`；四个嵌套独立模块（`godatabase`、`gocloudagent`、`gosandbox`、`gofunction`）继续不得 import 根模块。
- 证据口径：**基线不可手抄**。本文写作时 HEAD 为 `dead32f`，工作树另有 647 个未提交改动（`internal/` 下 259 个，含 [精简计划](internal-simplify-refactor-plan.md) §10 标注「尚未提交」的结果）。文中数字与 `文件:行` 均为该工作树快照，只用于说明量级；P0 的第一步是把工作树落成提交并由脚本生成基线（§8.3）。

## 修订对照（v1 → v2）

| # | v1 写法 | 问题（证据） | v2 修正 |
| --- | --- | --- | --- |
| R1 | 关停顺序：`httpServer.Shutdown` → … → 第 4 步 `agentRun.Close(ctx)` | `App.Shutdown` 首步就是 `httpServer.Shutdown(ctx)`（`internal/app/app.go:468`），它会等全部在途请求结束：非流式 run 用的是请求 ctx（`internal/api/agent_run_handler.go:194`），流式 run 与轮次事件流的 SSE 连接也都挂在请求上；而整个 `Shutdown` 共用一个预算（`http.shutdown_timeout` 默认 30s，`cmd/simplebase/main.go:53`）。轮到第 4 步时预算多半已耗尽：取消发得出去，「等待收尾写库」立即超时 | 两阶段：`agentrun.BeginShutdown()`（非阻塞）提到第 0 步，`agentrun.Wait(ctx)` 放在 cron / trigger 之后、`a.Close` 之前，且 `Wait` 使用独立的收尾预算（§3.4） |
| R2 | 「cancel 根 ctx → 在途 run / 轮次的 LLM 调用与工具执行收到取消」 | 对群轮次不成立：`group.Engine` 用 `context.WithCancel(context.WithoutCancel(ctx))` 起 `execute`（`gocloudagent/group/engine.go:214,1094`），执行 ctx 与传入 ctx 脱钩；唯一取消通道是私有的 `registry.cancelTurn`，公开的 `Engine.Cancel` 逐个等待最多 5s（`engine.go:797`）。v1 把它列为「前置核查项」但没有落成方案 | 升级为前置 PR：`group.Engine.BeginShutdown()` + `WaitIdle(ctx)`（§3.5）；`agentrun.Wait` 汇总各项目引擎 |
| R3 | 关停缺口只列了「直接 run 不受 `App.Shutdown` 管理」 | 另有两处：(a) cron 群轮次今天已被 `schedulerCancel()` 取消，但 `RunHeadless` 调 `cancelTurn` 后立即返回（`engine.go:997-999`），`cronScheduler.Stop()` 随之返回，之后 `a.Close` 关闭系统库，而 `execute` 的收尾与任务 run 的 `finishAgentRun` 仍在写（`collab_group_runner.go:97`，错误被 `_ =` 吞掉）；(b) **run 没有启动收敛**：`ListActiveAgentRuns` 只被列表接口使用（`agent_thread_handler.go:178`），其注释自述「进程重启后可能残留未收尾的 running 行，调用方需再与运行时内存态核对」（`systemdb/agent_runs.go:152`）但无人核对，进程被杀时 `running` 的 run 永久滞留 | (a) 纳入 P0 关停测试场景与 P2 修复；(b) 经确认**不处理**（D6-c 否决），维持现状并登记为不做 |
| R4 | `ExecuteRun`「同步与 SSE 两条路径共用」，未写 ctx 来源 | 现状两条路径取消语义不同：非流式跟随请求取消，流式 `WithoutCancel` 脱离请求（`agent_run_handler.go:348`）。若统一从根 ctx 派生，会悄悄改变非流式行为（客户端断开后 run 继续跑并落库） | 明确「行为不变」口径：执行 ctx = 根 ctx ∪ 调用方额外取消源；非流式传请求 ctx，流式不传（§3.3） |
| R5 | §6.2「不建 httpkit，每域构造器注入窄回调」 | 与 D3 矛盾。跨域复用的内核原语不止 `WriteError`：错误协议 49 个文件、`requestID` 42、`principalOf` 20、SSE 帧写出 8 个文件（5 个域）、`audit_helper` 8 个文件（6 个域）、`stage_timing` 8 个文件，context key 必须单一实现。回调注入意味着每个子包自带一个假 `WriteError`，错误协议等价性失去编译器保证 | 新增叶子包 `internal/api/httpkit`；`api` 以类型别名 / 函数变量转发，迁移期零调用点改动（§6.1）。可行性已核实：`error.go` 的 import 列表均不反向依赖 `api` |
| R6 | P5 批次表只覆盖部分文件，「数据面其余」没有清单；`mcp_*`、IAM、运维、项目无归属 | 无法验收「`api` 残留 = 内核」；数据面按 sql / schema / collection / kv 拆四包后，`SQLLease` / `DataService` 被 10+ 个文件跨包使用，会逼出同级 import 或再建共享包 | 给出 121 个生产文件的完整归属表；**按限界上下文拆、一个上下文一个 `*http` 包**（数据面合为 `datahttp`，沙盒与 E2B 合为 `sandboxhttp`）（§6.2） |
| R7 | `agent_plan_tools.go` 列入 P5b（agenthttp） | `runPlanningTools` / `turnPlanningTools` 的参数已是 `context.Context`，是 `RunRequest.Context.ExtraTools` 的应用层装配 | 随 P2 进 `agentrun/plan_tools.go` |
| R8 | 门禁基线手抄：「`api` 44 个生产文件引用 `*systemdb.Store`」 | 三份文档互不一致且与代码都对不上：`api` 生产文件 122 / 123 / 121（工作树）/ 99（HEAD）；`*systemdb.Store` 字面引用在 `api` 只有 18 个文件（import `systemdb` 任意符号的是 42 个），44 是旧工作树的「import systemdb」口径 | 基线由 P0 脚本生成落盘，计划只留快照与命令；两个口径分开计数（§8.3） |
| R9 | P2 5–7 人日；P5 8–12 人日 | P2 现含上游 `group` 改动与两阶段关停；P5 在 D3 全量拆下与决策稿给方案 C 的 15–25 人日同量级 | P2 7–9（含上游 1）、P5 15–20；并给出「P0–P4 即拿到 F 的核心收益」的承诺口径（§7） |
| R10 | 标题是「战略 DDD」，正文没有上下文映射 | 决策稿被本文取代后，实施者失去「新代码放哪」的依据 | 收进 §1.1–1.4（精简版 + 落点判定三问） |
| R11 | 领域事件归属写「归 trigger 或独立 internal/eventbus」 | 与 D5「只调归属、不引总线」不协调；现状 fan-out 在 `app.go:241` 构造并靠 `Add()` 在两处注册（隐式顺序） | 事件类型归发布方领域包、端口留在发布调用点、fan-out 与 trigger ACL 归 `app` 私有并用构造参数取代 `Add()`（§4） |
| R12 | 端口表列了不存在的 `GetAgentTag` / `UpsertAgentTag`，并漏掉群协作适配器需要的约 18 个方法；P3 写「agentbridge 收尾改调 agentrun 或登记例外」 | `agentrun → agentbridge`（实现 `GroupCronRunner`），`agentbridge` 不能反向 import；run 状态不变量的执行点其实已在 `systemdb.UpdateAgentRunStatus` 的条件更新（LIFE-04，`agent_runs.go:198-229`） | 端口表按实测重列并要求 P0 脚本再生成（§3.2）；收尾走 `agentbridge` 定义、`agentrun` 实现的 `RunFinalizer`（§3.7）；P3 不新建 CAS |

## 0. 决策记录（不再待定）

| 编号 | 决策 | 结论 |
| --- | --- | --- |
| D1 | 架构范式 | **F：战略 DDD + 六边形应用服务**（DDD 战略工具画边界，仅 Agent 协作核心域用战术模式） |
| D2 | Agent 应用服务落点 | **新建 `internal/agentrun`**；不并入 `agentbridge`（其职责是宿主 ↔ gocloudagent 适配），不做 `internal/agent/{domain,application}` 目录化 |
| D3 | `api` 入口 | **按业务域拆子包**。v2 细化：子包边界 = §1.1 的限界上下文；新增叶子包 `api/httpkit` 承载协议内核（D3-a） |
| D4 | `systemdb` | **不拆物理包**；新代码只经消费方窄接口使用，门禁统计「直接引用 `*systemdb.Store` 的生产文件数」只减不增 |
| D5 | 领域事件 | **保持进程内同步 fanout**，只移动代码归属，不引入事件总线 |
| D6 | 关停语义 | **关停时取消在途 Agent run / 轮次，在超时内等待收尾写库**。run 落既有 `canceled`；有执行循环的轮次由引擎按取消语义落 `canceled`（其 running 任务落 `interrupted`）；接受「发版时在途 run 被取消」的用户可见行为 |
| D6-a | D6 落地形态（v2） | 两阶段：`BeginShutdown()` 非阻塞取消，`Wait(ctx)` 等收尾；`BeginShutdown` 后拒绝新建 run / 轮次（503 `service_stopping`） |
| D6-b | 非流式 run 的取消语义（v2） | **保持现状**：请求取消或服务停机任一发生即取消；流式仍只受服务停机 / `CancelRun` / `RunTimeout` 影响 |
| D6-c | 孤儿 run 启动收敛（v2） | **不做**（用户确认）：进程被杀遗留的 `running` run 维持现状，不新增启动扫描与状态改写 |
| D3-a | 协议内核叶子包（v2） | `internal/api/httpkit`：错误协议、context key、request ID、SSE 帧写出、审计辅助、分段计时、跨域共享的 handler 侧端口；`api` 与各 `<context>http` 都可 import，它不 import 任何上层 |

## 1. 目标架构

### 1.1 限界上下文与子域（战略 DDD，本文的规范依据）

| 上下文 | 子域 | 所有者包（领域 / 应用） | 入站适配 | 出站端口（消费方定义） | 发布语言 |
| --- | --- | --- | --- | --- | --- |
| Agent 协作 | **核心（最复杂）** | **`agentrun`（应用服务）**、`agentbridge`（宿主 ACL）、`gocloudagent`（通用子域，独立模块） | `api/agenthttp`、cron（`cronjob → agentbridge.GroupCronRunner`）、内置 MCP（经路由树） | `agentrun.Ports`（§3.2）、`agentbridge.ports` | `agentrun.Service`、`agentrun.Err*`、run / 轮次状态值 |
| 数据服务 | **核心** | `collection`、`godatabase`（独立模块）、`dbbridge` | `api/datahttp` | `DataService` / `SQLService` / `KVService`（定义在 `datahttp`） | 领域事件 `collection.RecordEvent` |
| 文件与站点 | 支撑 | `objectstore` | `api/filehttp` | — | 领域事件 `objectstore.ObjectEvent` |
| 自动化 | 支撑 | `functionrun`、`trigger`、`cronjob`、`gofunction`（独立模块） | `api/functionhttp` | `cronjob.Store/Runner/AgentRunner`、`trigger.Dispatcher` | `trigger.Event`（由 `app` 内 ACL 翻译） |
| LLM 网关 | 通用 | `llmgateway`、`usage` | `api/llmhttp` | `llmgateway.Service`、`UsageRecorder`、`TraceRecorder` | — |
| 沙盒 | 支撑 | `sandbox`、`sandbox/sshd`、`e2bproto`、`gosandbox`（独立模块） | `api/sandboxhttp`（管理 + E2B 兼容 + SSH） | `sandbox.Store`、`sandbox.Manager` | 终端票据 |
| 身份与访问 | 通用 | `auth` | `api/iamhttp` | — | **共享内核** `auth.Principal` / `Permission` |
| 项目与目录 | 支撑 | `catalog` | `api/cataloghttp` | `catalog.Service`、`KVInitializer`（定义在 `cataloghttp`） | `catalog.Database` 状态机 |
| 运维治理 | 通用 | `backup`、`audit`、`observability`、`alerts` | `api/backuphttp`、`api/opshttp` | `backup.Service` | 审计事件 |
| 平台（非业务） | — | `app`（组合根）、`config`、`systemdb`、`web`、`utils` | `api`（入口内核）、`api/httpkit` | — | 唯一组合根、唯一路由树、唯一系统库连接 |

### 1.2 上下文映射

```text
                    ┌──────────── 共享内核：auth.Principal / Permission ────────────┐
                    │                                                              │
 Agent 协作 ──ACL(agentbridge.ports)──▶ 数据 / 文件 / 观测（DatabaseAccess、ObjectAccess、LogReader）
 Agent 协作 ──Customer/Supplier──▶ LLM 网关（cloudagent.ChatClient ← llmgateway）
 Agent 协作 ──ACL(agentbridge)──▶ gocloudagent（通用子域，独立模块，不 import 宿主）
 自动化     ──Customer/Supplier──▶ Agent 协作（cronjob.AgentRunner → agentbridge.GroupCronRunner / RunFinalizer ← agentrun 实现）
 数据服务   ──领域事件 collection.RecordEvent──▶ 自动化(trigger)、实时(realtime)
 文件       ──领域事件 objectstore.ObjectEvent──▶ 自动化(trigger)
 数据服务   ──ACL(dbbridge)──▶ godatabase（独立模块）
 沙盒       ──ACL(sandbox.Store / gosandbox/vm)──▶ gosandbox（独立模块）
 项目目录   ──Customer/Supplier──▶ 数据服务（建项目后初始化 KV：cataloghttp.KVInitializer ← datahttp）
 运维(backup) ──Conformist──▶ 目录 + 数据服务
```

判断不变：防腐层与共享内核已存在且方向正确（`dbbridge`、`agentbridge`、四个独立模块）。缺陷集中在三点——自动化 → Agent 的供应关系经由 HTTP handler 实现（P2）、领域事件的发布器实现住在 `api`（P4）、非 HTTP 适配住在 `api`（P4）。

### 1.3 依赖图与依赖规则（改造后）

```text
cmd/simplebase
  └─ internal/app                      唯一组装点：构造、注入、启动收敛、关停；app 私有适配
       │                               （trace recorder、事件 fan-out、trigger ACL）
       ├─ internal/api                 入口内核：路由树、守卫、Echo 错误兜底、中间件、健康 / 观测、
       │    │                          站点分流、内置 MCP 派发
       │    ├─ internal/api/httpkit    ★叶子包：协议内核 + 跨域共享的 handler 侧端口
       │    └─ internal/api/<context>http
       │          iam / catalog / data / file / function / agent / llm / sandbox / backup / ops
       ├─ internal/agentrun            ★Agent 协作上下文的应用服务
       ├─ internal/agentbridge         宿主 ↔ gocloudagent 的 ACL（端口 + 运行时适配，不含应用服务）
       └─ 其余领域 / 基础设施（不变）
```

依赖规则（AST 门禁强制，§8.3）：

1. `app → 任意`；只有 `app`（及既有例外 `api/perfbench`、`cmd`）可以 import `api`。
2. `api/httpkit` 不 import `api`、任何 `<context>http`、`app`；可以 import 领域包、`agentrun`（哨兵错误映射）与独立模块。
3. `<context>http → httpkit / agentrun / 领域包`；**禁止** import `api`、`app`、同级 `<context>http`。
4. `api → httpkit / <context>http`；`api` 不再承载 handler、DTO 与协议转换。
5. `agentrun → agentbridge / auth / systemdb（仅行类型）/ gocloudagent / llmgateway / usage / audit`；禁止 `api*`、Echo、`app`。
6. `agentbridge`、`cronjob` 不 import `agentrun`：`agentrun` 实现 `agentbridge` 定义的 `GroupCronRunner` 与 `RunFinalizer`，由 `app` 注入。
7. 领域事件类型只存在于发布方领域包；消费方在 `app` 接线；不出现 `internal/eventbus`。

### 1.4 新代码落点判定（三问）

1. **有没有 HTTP / WS / SSH 帧或状态码？** 有 → 对应的 `<context>http`；属于协议内核的原语（错误、context、SSE、审计辅助）→ `httpkit`。
2. **是否被两个以上入口（HTTP / cron / 事件 / 内置 MCP）复用，或需要跨请求的生命周期？** 是 → 应用服务（`agentrun`）或已有领域包；找不到所有者且只有一个调用方 → 留在调用方，不建包。
3. **是否只是把外部系统或独立模块翻译成本上下文的语言？** 是 → 该上下文的 ACL（`agentbridge` / `dbbridge`）；只在装配处使用的无业务适配 → `app` 私有。

任何情况下：`systemdb` 只经消费方窄接口；事件类型归发布方；`*sql.DB` / lease / 行类型不成为新接口的契约。

### 1.5 问题 → 解法映射

| 编号 | 问题 | 解法 | 批次 |
| --- | --- | --- | --- |
| P1 | Agent run 生命周期挂在 HTTP handler 上（`runStream` 用 `WithoutCancel`、群引擎自起 goroutine），`App.Shutdown` 不覆盖；cron 群轮次收尾不等（R3a） | `agentrun.Service` 持根 ctx；两阶段关停；上游 `Engine.BeginShutdown/WaitIdle` | P2 |
| P2 | cron → HTTP handler 环形延迟绑定（`GroupCronRunner.Attach`） | cron 直接拿 `agentrun.Service`，删除 `Attach` | P2 |
| P3 | router 构造期改共享运行时（`Confirm` 注入）/ 回填对象（`Terminal.Server`） | confirmHub、SSH 认证器由 `app` 先构造 | P2、P4 |
| P4 | 非 HTTP 适配住在 `api`（trace recorder、事件 fan-out、CloudAgent DB / Obj / LLM） | 分别移到 `app` 私有 / `agentbridge`；事件类型归发布方 | P1 |
| P5 | 领域规则留在入口（集合过滤聚合、KV 命令签名绑 Echo） | 纯逻辑下沉 `collection`；KV 解析改返回错误值 | P4 |
| P6 | 白盒测试与包边界耦合（103 个测试文件全在 `package api`） | 测试随 handler 迁子包；路由级测试留 `api` | P5 |
| P7 | 入口内核原语无法被子包复用（回调注入会复制错误协议） | `httpkit` 叶子包 + `api` 别名转发 | P5a |
| P8 | `api` 文件没有完整归属 | §6.2 全量归属表；按上下文而非资源拆包 | P5 |

## 2. 不可回退的行为契约

| 边界 | 必须锁定 |
| --- | --- |
| 路由与身份 | 方法、路径、参数、挂载条件、中间件顺序不变；`e.Pre` 站点分流、公开健康 / 登录路由、认证 → post-auth 限流 → 项目上下文、end-user 独立鉴权、SPA fallback 原样；`require` / `delegable`、`policyGate`、`writeGate` 权限判定不动；已删路由保持 404 |
| Agent 协作协议 | SSE 首帧 `run`、`[DONE]` 与 usage / end 收尾、ReplayEvents 的 `stream_gap` 与跳号检测、群轮次 v3 帧（`groupEventPayload`）、`X-Run-Id` / `X-Turn-Id` 响应头、审批 `confirmation_required/resolved` 事件、`TestB3AgentRouteSetLocked` 锁定的路由集——逐字节不变 |
| 非流式 run 取消语义 | 请求取消即取消 run（D6-b）；流式 run 断开后继续（现状） |
| 实例与系统库 | 只读实例拒绝写；系统库只 SELECT、禁删禁写、不经 registry factory；lease 取 / 持 / 失与 checkpoint 契约不变 |
| 事件与生命周期 | 「变更 → 提交 → NotifyWrite → 索引 → 事件 → 审计」时序按路径保持；trigger 不持久化、不承诺 exactly-once；cron / trigger / realtime 的取消排空顺序除新增的第 0、5 步外不变 |
| 停机后的待办轮次 | `waiting_human` / `awaiting_plan_confirm` 轮次没有执行循环，关停不取消、不改状态；下次启动仍走既有 `Converge`（`interrupted` + 待办保持 `waiting_human`，可经回复 / 恢复继续） |
| 持久化与日志 | 写入与同步通知次数不变；审计只记元数据；`agent.trace_log` 默认关闭、开启仅走脱敏截断的 zap 通道 |

**本计划显式引入的行为变化（需知会，不在上表锁定范围内）**

| 变化 | 现状 | 改后 |
| --- | --- | --- |
| 关停取消在途 run / 轮次（D6） | 进程退出即中断，run 永久 `running` | run → `canceled`；有执行循环的轮次 → `canceled`（error_code `canceled`，同用户取消），其 running 任务 → `interrupted` |
| 停机窗口的新请求 | 直到连接关闭前仍可创建 run | `BeginShutdown` 后 `POST …/runs`、`…/turns`、群定时返回 503 `service_stopping`（新增错误码，加入 `sentinelErrorMappings`） |
| cron agent run 的收尾（P3） | 与直接 run 是两套收尾：先写指标后写状态、用 `strings.Contains` 判取消、无审计 | 统一走 `finishRun`；每个 cron agent run 多一条 `agent.run` 审计（仅元数据） |
| 关停总时长 | 被在途 LLM 调用拖满 30s 预算 | 取消后通常数秒内完成 |

## 3. `internal/agentrun` 详细设计（P2 主体）

### 3.1 包结构

```text
internal/agentrun/
  service.go        Service：根 ctx / cancel、stopping、WaitGroup、events、confirms、groupEngines、Ports；
                    NewService、BeginShutdown、Wait、Close
  ports.go          消费方窄接口（§3.2）；*systemdb.Store 隐式满足，app 注入
  errors.go         ErrStopping、ErrTurnsRequired、ErrWorkspaceUpgradeRequired、ErrGroupArchived、
                    ErrGroupMemberUnavailable、ErrTurnBusy、ErrTurnConflict …（自 api/agent_handler.go 迁入）
  run.go            StartDirectRun、ExecuteRun、CancelRun、finishRun（原 finishAgentRun）、
                    recordApprovalEvent、transitionRun
  run_events.go     runEventHub（原 api/run_events.go 上半：append / replay / unsubscribe / evict / drop）
  confirm.go        confirmHub（原 api/run_events.go 下半：Wait / Prepare / DropRun / Resolve）
  plan_tools.go     planningTools / runPlanningTools / turnPlanningTools / replayPlan / requestsReplan（原 agent_plan_tools.go）
  group_engine.go   groupEngineFor、collabGroupMembers、workspaceCandidates、collabGroupIdentity（原 collab_group_engine.go）
  group_store.go    collabGroupStore（实现 group.Store，原 collab_group_store.go）
  group_runner.go   RunTask（实现 group.TaskRunner，原 collab_group_runner.go）
  turn.go           SubmitTurn（原 runGroupTurn）、claimTurn、RunScheduledTurn、ConfirmTurnPlan、CancelTurn、
                    RetryTask、ReplyHuman、ResumeTurn、SubscribeTurn（动作类为薄转发，让 agenthttp 不持有 Engine）
  cron.go           RunGroupCron、groupCronThread、cronFireKey（实现 agentbridge.GroupCronRunner）
  converge.go       Converge：群引擎启动收敛（原 convergeGroupEngines）
```

### 3.2 端口（消费方窄接口）

签名直接使用 `systemdb.AgentRun` / `systemdb.AgentTurn` 等行类型，`*systemdb.Store` 隐式满足（同 `cronjob.Store`、`functionrun.Runner`、`agentbridge.CronAgentStore` 样板）。不复制 DTO、不建万能仓储、不把 `*sql.DB` / lease 泄露为契约。下表为工作树快照；**P0 用 `grep -oh 'store\.[A-Z][A-Za-z]*(' <迁出文件>` 机械生成完整清单**写入 `ports.go` 注释，人工增删须在 PR 说明。

| 端口 | 方法 |
| --- | --- |
| `RunStore` | `CreateAgentRun`、`GetAgentRun`、`UpdateAgentRunStatus`、`UpdateAgentRunMetrics`、`AppendAgentMessage`、`RecordAgentToolSpans`、`ListAgentMessages` |
| `ApprovalStore` | `CreateAgentApproval`、`GetPendingAgentApproval`、`ResolveAgentApproval`、`CancelRunApprovals` |
| `ThreadStore` | `GetAgentThread`、`CreateAgentThread`、`CreateAgentThreadWithWorkspace`、`ArchiveAgentThread`、`GetAgentThreadWorkspace`、`GetAgentThreadCollab` |
| `CollabStore` | `CollabSchemaReady`、`CreateAgentTurn`、`GetAgentTurn`、`UpdateAgentTurnStatus`、`ClearAgentTurnError`、`ClaimAgentTurn`、`ReleaseAgentTurn`、`ListRunningAgentTurns`、`ListAgentTurns`、`LastAgentTurnSeq`、`AppendAgentTurnEventAtomic`、`ListAgentTurnEvents`、`CreateAgentThreadCollab`、`ListAgentTasks`、`ReplaceAgentTasks`、`GetAgentTask`、`UpdateAgentTask`、`AppendAgentTaskRun`、`ListHumanAgentTasks`、`CountPendingHumanTasks`（`collabGroupStore` 适配器的全部依赖；**轮次 / 任务状态迁移只经 `group.Engine` → 本适配器**，`agentrun` 不另写状态机） |
| `TagStore` | `GetAgentTagRow`、`UpsertBuiltinAgentTag`（`resolveAgentTag` 的预置群回退迁入 `agentrun`，走此端口） |
| `AgentCatalog` | `GetCloudAgent`、`ListCloudAgents` |
| `GroupCronStore` | `ClaimCronFire`、`CompleteCronFire`、`UpsertAgentGroupCron` |
| `Quota` | `CheckQuota(ctx, projectID, kind) error`（`*usage.Service` 满足） |
| `Audit` | `Record(ctx, audit.Event) error`（`*audit.Service` 满足；`finishRun` 的 `agent.run` 审计） |

构造函数：

```go
func NewService(cfg Config, ports Ports, runtime *cloudagent.Runtime, confirms *ConfirmHub) *Service
// cfg 显式携带 ConfirmTimeout、SkillsCLI、SandboxAvailable、Writable，
// 取代 handler 对 runtime.Provider.(*agentbridge.RuntimeProvider) 的类型断言与 6 个小访问器。
```

`Capabilities`（SkillsCLI / SandboxAvailable）由同一个 `cfg` 暴露给 HTTP 层做 503 判定，HTTP 不再自行反查 provider。

### 3.3 Service 方法、来源与 ctx 语义

| 方法 | 来源（现 `api` 符号） | 说明 |
| --- | --- | --- |
| `StartDirectRun(ctx, DirectRunInput) (DirectRun, error)` | `createDirectRun`：配额检查、`ClaimThread`、run 行创建、用户消息落库、`loadRetry`、`historyWithoutUserTurn`、`ExtraTools` 装配 | `ctx` 只用于落库；返回 `DirectRun{Run, Request cloudagent.RunRequest}`；`stopping` 时返回 `ErrStopping` |
| `ExecuteRun(extra context.Context, d DirectRun, emit) (RunOutcome, error)` | `completeRun` / `runStream` 共用的 `runtime.StartRun` + `finishRun` + `recordApprovalEvent` | **执行 ctx = 根 ctx ∪ `extra` 的取消**（`context.AfterFunc(extra, cancel)`）；非流式传请求 ctx，流式传 `nil`（D6-b）。在调用方 goroutine 执行并登记 `WaitGroup`；收尾写库用独立 grace ctx（对齐现 `finishAgentRun(context.Background())`）。返回 `RunOutcome{Run, Assistant *systemdb.AgentMessage, Result, RunErr}`，入口不再回表 |
| `CancelRun(ctx, projectID, runID)` | `CancelRun` handler 执行体 | `runtime.CancelRun` + running / queued → canceled + `CancelRunApprovals`；权限与发起人校验留入口 |
| `ResolveConfirm(ctx, projectID, runID, callID, approve, actor)` | `ConfirmRun` | hub 放行 + 持久审批收敛 |
| `SubmitTurn(ctx, TurnInput) (TurnHandle, error)` | `runGroupTurn` | 引擎组装、`BeforeFence`（用户消息落库）、`engine.Submit` |
| `SubscribeTurn(projectID, turnID, after)` | `streamGroupTurn` | 返回 `*group.Subscription`；SSE 帧编码留入口 |
| `ConfirmTurnPlan` / `CancelTurn` / `RetryTask` / `ReplyHuman` / `ResumeTurn` | `collab_handler.go` / `collab_coordinator.go` 对 `engine.X` 的 7 处调用 | 薄转发（各 3–5 行）；归属校验（如「仅接收人或项目管理员」）留入口，状态校验在引擎 |
| `RunGroupCron(ctx, projectID, jobID, bind)` | `GroupCronRunner.RunGroupCron` + `groupCronThread` + `runScheduledTurn` + `cronFireKey` | 实现 `agentbridge.GroupCronRunner`；`stopping` 时返回 `ErrStopping` |
| `ReplayRunEvents(runID, after)` | `runEventHub.replay` | 返回帧、gap 标记与订阅通道；HTTP 半边留入口 |
| `Converge(ctx)` | `convergeGroupEngines` | 群引擎启动收敛（`ListRunningAgentTurns → interrupted`） |
| `BeginShutdown()` / `Wait(ctx)` / `Close(ctx)` | **新增** | `Close = BeginShutdown + Wait`；`app.Shutdown` 用前两者 |

留在入口（`api/agenthttp`）的职责不变：Echo 解析、`requireProject`、`PrincipalFromContext`、`WriteError`、SSE 帧写出、heartbeat / thinking workers、`canWritePrincipal` / `canCancelConfirm`、DTO 与映射。`agenthttp` 只 import `gocloudagent/group` 的**类型**（`Event`、`Subscription`），不持有 `group.Engine` / `cloudagent.Runtime`。

### 3.4 生命周期与关停时序（D6 落地）

```text
启动（app.NewWithRegistry，替换 assembleScheduler 里的空 GroupCronRunner）：
  confirms := agentrun.NewConfirmHub()            // app 先构造
  runtime := a.cloudAgentRuntime(confirms)       // rt.Confirm / provider.Confirm 在装配处赋值，router 不再写回
  agentRun := agentrun.NewService(cfg, ports, runtime, confirms)
  assembleScheduler(..., agentRun)               // cron 直接持 Service，删除 Attach
  buildAPIDeps: AgentRun: agentRun               // Dependencies.AgentRun 取代 CollabRunner
  全部装配完成后：agentRun.Converge(ctx)          // 先于 HTTP 监听（a.Start 在 New 之后）

关停（App.Shutdown；第 0、5 步新增，其余顺序不变）：
  0. agentRun.BeginShutdown()   非阻塞：stopping=true；cancel 根 ctx；各项目引擎 engine.BeginShutdown()
                                   （只取消有执行循环的轮次；waiting_human / awaiting_plan_confirm 不动）
  1. httpServer.Shutdown(ctx)   在途 run 已在收尾：SSE 写完 end 帧即返回，不再被 LLM 长调用拖满预算
  2. metricsServer.Shutdown(ctx)
  3. schedulerCancel() + cronScheduler.Stop()    cron 群轮次已在第 0 步取消，Stop 很快返回
  4. triggerDispatcher.Close()
  5. agentRun.Wait(graceCtx)   ★ 等全部收尾写库完成；超时记 zap Warn 并列出残留 runID / turnID，不阻塞后续
     graceCtx = context.WithTimeout(context.Background(), 10s)   // 常量，不新增配置项；
                                                                   // 独立于主预算，避免第 1 步被长期 SSE 占满后收尾拿不到时间
  6. kvSweeper / handleEvictor / a.Close(ctx)    既有；此后进程内不再有 Agent 写
```

状态落点：

| 对象 | 关停时终态 | 写入者 | 依据 |
| --- | --- | --- | --- |
| 直接 run（流式 / 非流式） | `canceled`，error_code 空（同现状的运行时取消） | `finishRun`（grace ctx） | `cloudagent/runtime.go:287-288` 把取消归一为 `context.Canceled`；`finishAgentRun` 据此落 canceled |
| 群任务 run | `canceled` | `RunTask` 内 `finishRun` | `collab_group_runner.go:97-110` |
| 有执行循环的轮次（running 等） | `canceled`（error_code `canceled`，与用户 `Cancel` 同一语义） | 引擎 `cancelFromWithin`（`cleanupCtx = WithoutCancel`） | `engine.go:689-712`；`TestEngineCancelDoesNotBecomeFailure` 锁定 |
| 任务 | running → `interrupted`；pending / blocked / waiting_human → `canceled` | 同上 | 同上 |
| 待确认审批 | 等待因取消中断，按取消分支收敛 | `recordApprovalEvent` | `cloudagent/confirmation.go:88-113` |
| `waiting_human` / `awaiting_plan_confirm` 轮次 | 不变；下次启动 `Converge` → `interrupted` | 既有路径 | `engine.go:322-335`（人工待办释放执行循环） |

**P2a 上游前置（见 §3.5）完成前不开始 P2b**；若上游改动受阻，降级方案是「直接 run 两阶段 + 群轮次只取消 cron 路径（已由 `schedulerCancel` 覆盖）+ 其余靠启动收敛」，并在 PR 记录差异。

### 3.5 上游前置改动：`gocloudagent/group`（P2a，独立 PR，可先行合并）

现状缺口：引擎没有「取消全部并等待」的入口；`registry.cancelTurn` 私有；`Engine.Cancel` 逐个等待 5s；`RunHeadless` 在 `ctx.Done()` 时不等 `execute` 收尾就返回（R3a）。

```go
// BeginShutdown 进入停机：之后的 Submit 返回 ErrEngineClosed；取消全部已登记（有执行循环）的轮次，
// 不逐个等待。幂等、非阻塞。
func (e *Engine) BeginShutdown()
// WaitIdle 等待全部执行循环退出（收尾写库完成）；无活动轮次时立即返回；超时返回 ctx.Err()。
func (e *Engine) WaitIdle(ctx context.Context) error
```

- `registry` 增加 `closed` 标志、`cancelAll()` 与「轮次表为空」的通知（`release` 时广播）；`register` 在 `closed` 时返回 `ErrEngineClosed`。
- `RunHeadless` 的 `ctx.Done()` 分支改为：`cancelTurn` 后在有界时间内等 `at.done`（复用 `Cancel` 的 5s 上限），使 `cronScheduler.Stop()` 返回时轮次收尾已完成。
- 不改 `Engine.Cancel`（用户取消）的语义，不改任何状态迁移表。
- 测试（`gocloudagent/group/engine_test.go`）：① `BeginShutdown` 后 `Submit` 返回 `ErrEngineClosed`；② 运行中轮次走 `cancelFromWithin`（轮次 canceled、running 任务 interrupted），`WaitIdle` 返回后 Store 无后续写；③ 无活动轮次时 `WaitIdle` 立即返回；④ 幂等；⑤ `RunHeadless` 在 ctx 取消后返回时轮次已终态。
- 约束：`gocloudagent` 是独立模块，只改自身；按其命令验证（`cd gocloudagent && GOWORK=off go build ./... && go vet ./... && go test ./...`）。

### 3.6 `api` 与 `app` 侧改造

| 现文件 | 改造 |
| --- | --- |
| `agent_handler.go` | `cloudAgentHandler` 删除 `events`、`confirms`、`groupEngines`、`store`、`runtime` 字段与 `newCloudAgentHandler` 内的 hub 创建 / `Confirm` 写回 / `convergeGroupEngines` / `CollabRunner.Attach`；改为持 `svc *agentrun.Service`；哨兵错误迁 `agentrun/errors.go`，`api` 保留 `var ErrGroupArchived = agentrun.ErrGroupArchived` 别名至 P5b |
| `agent_run_handler.go` | `createDirectRun` 瘦身为「解析 → 校验 → `svc.StartDirectRun` → 流式走 `svc.ExecuteRun(nil, …)` + SSE 编码 / 非流式 `svc.ExecuteRun(reqCtx, …)` + JSON」；`finishAgentRun`、`recordApprovalEvent`、`onRunEvent`、`historyWithoutUserTurn`、`loadRetry`、`resolveDirectAgent` 的存储部分迁出 |
| `agent_approval_handler.go` | `CancelRun` / `ConfirmRun` 保留权限与发起人校验，执行体调 `svc.CancelRun` / `svc.ResolveConfirm` |
| `collab_coordinator.go`、`collab_handler.go` | `runGroupTurn`、`claimTurn` 迁出；`turnContext`、请求解析、归属校验留入口；引擎动作改调 `svc.*` |
| `collab_group_engine.go`、`collab_group_store.go`、`collab_group_runner.go`、`agent_plan_tools.go` | 整文件迁 `agentrun` |
| `collab_cron_runner.go` | 删除 `GroupCronRunner` / `Attach` / `NewGroupCronRunner`；执行逻辑迁 `agentrun/cron.go` |
| `run_events.go` | hub 两类型迁出；`ConfirmRun` / `ReplayEvents` 的 HTTP 半边留入口 |
| `routes_agent.go`、`router.go` | `Dependencies` 新增 `AgentRun *agentrun.Service`，删除 `CollabRunner` |
| `app/assemble_api.go` | `apiHandlers` 删除 `collabRunner`；`assembleScheduler` 直接接收 `agentrun.Service` |
| `app/assemble_agent.go` | `cloudAgentRuntime(confirms)`；`App` 新增 `agentRun` 字段 |
| `app/app.go` | `NewWithRegistry` 末尾 `agentRun.Converge`；`Shutdown` 插入第 0、5 步 |

### 3.7 聚合不变量收口（P3）

- run 状态迁移（`queued / running → completed | failed | canceled`）的**执行点已经在仓储**：`systemdb.UpdateAgentRunStatus` 带 LIFE-04 条件更新（终态不可覆盖，`agent_runs.go:198-229`）。P3 **不新建 CAS、不复制状态机**，只做三件事：
  1. `agentrun.transitionRun` 成为 `UpdateAgentRunStatus` 的唯一调用点（带目标状态与错误码规则：取消 → 空码；失败 → `ClassifyError`）；
  2. `agentbridge` 定义、`agentrun` 实现窄接口，供 cron agent run 收尾：

     ```go
     // agentbridge
     type RunFinalizer interface {
         Finish(ctx context.Context, in RunFinish) error
     }
     ```

     `CronAgentExecutor` 经 `WithFinalizer` 注入；`CronAgentStore` 去掉 `UpdateAgentRunStatus`、`UpdateAgentRunMetrics`、`RecordAgentToolSpans`。`agentbridge` 不 import `agentrun`，由 `app` 接线；
  3. 统一差异：取消判定由 `strings.Contains(err.Error(), "context canceled")` 改为 `errors.Is`；写顺序统一为 `finishRun`；cron agent run 补 `agent.run` 审计（§2 已登记）。
- Turn / Task 的唯一权威保持 `gocloudagent/group/state.go`；门禁：`internal/api/**` 与 `agentbridge` 内 `UpdateAgentRunStatus` 直调点归零，`UpdateAgentTurnStatus` 只出现在 `agentrun/group_store.go`。

### 3.8 P2 的测试影响面（实测）

P2 不需要大规模迁移白盒测试：103 个测试文件里**没有一个**直接调用 `newCloudAgentHandler`（都经路由树或 harness），只有 7 个文件触及将被迁出的私有符号——`run_events_test.go`（6 处）、`run_events_lifecycle_test.go`（4）、`agent_skills_e2e_test.go`（4）、`e2b_routes_test.go`（3）、`collab_cron_test.go`（2）、`agent_e2e_harness_test.go`（2）、`agent_optimization_test.go`（1）。处理：hub 用例迁为 `package agentrun` 内部测试；harness 与 `e2b_routes_test` 改为注入 `agentrun.Service`（提供 `agentrun.NewService` 的测试构造：fake `cloudagent.Runtime` + 测试库）。迁入 `agentrun` 的写路径用例（`agent_run_gate_security`、`collab_e2e`、`collab_cron`）随迁，并要求在 `agentrun` 包内**不起 Echo**即可运行。

## 4. P1 适配器归位（低风险先行批次）

| 改动 | 现位置 | 去向 | 验收 |
| --- | --- | --- | --- |
| `NewLLMTraceRecorder` | `api/llm_trace_recorder.go` | `app` 私有适配器（构造点在 `assembleLLMService`，仅一处使用） | `app/assemble_agent.go` 不再引用 `api.NewLLMTraceRecorder`；trace 落库行为与 nil 语义不变 |
| 领域事件（R11） | `RecordEvent` 定义在 `api/data_handler.go:62`；`ObjectEvent`、`recordEventPublisher`、`objectEventPublisher`、`RecordEventFanout`、`TriggerEventPublisher` 在 `api/event_fanout.go`；fan-out 在 `app.go:241` 构造、`Add()` 在 `app.go:242` 与 `assemble_api.go:106` 两处注册 | ① 事件**类型**归发布方领域包：`collection.RecordEvent`、`objectstore.ObjectEvent`（发布语言）；② **端口接口**留在发布调用点所在包（`DataHandler.WithEvents`、s3 handler 的 `objectEventPublisher`），签名改用领域类型；③ fan-out 与 `→ trigger.Event` 的翻译（自动化的 ACL）归 `app` 私有（`app/event_fanout.go`），用构造参数 `newEventFanout(realtime, triggerEvents)` 取代可变的 `Add()`；`RealtimeHandler.PublishRecord` 改收 `collection.RecordEvent` | `trigger` / `realtime` 消费链不经过 `api`；发布顺序 = 构造参数顺序（realtime → trigger，与现状一致）；`TestFunctionTriggerHTTPAndRecordEndToEnd` 通过；`api` 不再出现 `RecordEventFanout` |
| `NewCloudAgentDB`、`NewCloudAgentObj` | `api/gocloudagent_access.go`（不 import Echo） | `agentbridge`（它们实现的就是 `agentbridge.DatabaseAccess` / `ObjectAccess`）；`CatalogService` / `RegistryService` / 文件存储改为 `agentbridge` 内定义的窄接口，不新增对 `objectstore` 具体类型的依赖 | SQL 只读守卫、系统库拒绝、SSRF 防护语义原样；`TestMCPPolicyFailsClosedOnUnknownServer` 等失败关闭测试通过 |
| `NewCloudAgentLLM` | 同上 | `agentbridge`，但**不是纯搬迁**：它依赖 `api.LLMService` 的 DTO（`LLMRequest` / `LLMStreamReader`，`gocloudagent_access.go:205-243`），迁入时改为直接消费 `llmgateway.Service`（`agentbridge` 已依赖），映射函数随迁，`app` 少一层 `api.NewLLMService` 中转 | Agent 的 LLM 调用路径行为不变；`gocloudagent_access_test.go` 迁 `agentbridge` |
| `llmgateway.NewSystemCredSource` 等具体类型 | `agentbridge` 若干端口仍接 `*systemdb.Store` | 随触及文件改成窄接口（不单独排期） | §8.3 门禁计数不上升 |

## 5. P4 数据纯逻辑下沉 + 沙盒装配前置

### 5.1 集合与 KV（P5 语义修复）

- `collection_filter.go`（418 行）、`collection_aggregate_query.go`（490 行）、`collection_pick.go`（125 行）、`collection_doc.go`、`collection_query.go`、`aggregate_env.go` 中 Echo 无关部分移入 `internal/collection`（避免 `collection` 引入 `godatabase.Dialect` 之外的新依赖；若确需方言层抽象，建 `collection/query` 子包）。
- KV 命令表 `kv_cmd_*.go`：解析签名从 `func(c echo.Context, argv []string)` 改为返回参数错误值，handler 统一转 `WriteError`；命令表随 `datahttp` 迁移（§6.2）。
- **金标测试先行**：生成的 SQL / 参数、错误 code 与文案逐字一致后再搬。

### 5.2 终端 / SSH 装配（P3 剩余项）

- 把 `SandboxHandler` 中 SSH 认证所需（票据校验、项目沙盒查找）抽成 `app` 在 `NewRouter` 之前可构造的独立对象；`bindSandboxSSHAuth` 前置到 `buildHTTPServers` 中 `api.NewRouter(deps)` 之前，`deps.Terminal.Server` 不再事后回填。
- 票据一次性语义、`TestSandboxE2B_CrossProject`、终端 SSH 认证测试通过。

## 6. P5 入口按限界上下文拆子包（D3）

### 6.1 `httpkit`：协议内核叶子包（P5a，替代 v1 的「回调注入」）

```text
internal/api/httpkit/     叶子包：被 api 与全部 <context>http import；不 import 它们、不 import app
  error.go        APIError / APIErrorBody / NewAPIError / Error / sentinelErrorMappings / WriteError /
                  mapEchoError / httpStatusToCode / safeMessage（echoHandler 的 Echo 兜底仍留 api/error.go）
  context.go      WithRequestID / RequestIDFromContext / ProjectContext / WithProject / ProjectFromContext /
                  WithPrincipal / PrincipalFromContext / ErrProjectMissing / requestID / requireProject /
                  projectOf / projectID / principalOf / decodeJSONBody / queryLimit（context key 的唯一实现）
  sse.go          sseWriter / newSSEWriter / writeSSEFrame / beginSSE
  audit.go        AuditEvent / AuditService / UsageService / LLMService / CacheService 及其适配器
                  （来自 adapter.go；被 ≥ 3 个域的 handler 使用）；audit_helper 的统一入口
  timing.go       stage_timing.go（8 个文件跨域使用）；public_base.go（3 个文件跨域使用）
```

- `api` 保留同名转发：`type APIError = httpkit.APIError`、`var WriteError = httpkit.WriteError` 等，迁移期 49 + 42 + 20 个调用点**零改动**；P6 决定是否清理转发。`requestLogger` 全局随之迁移。
- 域专属端口（`DataService` / `SQLService` / `SQLLease` / `KVService` / `CatalogService` / `RegistryService`）**不进** `httpkit`，随所属上下文的子包迁移（§6.2）。
- 依赖可行性：`error.go` 现有 import（echo、`gocloudagent/{cloudagent,group,plan}`、`godatabase/{kv,lease,sqlguard}`、`gosandbox/vm`、`agentbridge`、`auth`、`catalog`、`objectstore`、`observability`、`sandbox`、`systemdb`）均不反向依赖 `api`；`httpkit → agentrun` 仅为哨兵错误映射，`agentrun` 不 import `httpkit`。
- 不新增依赖：门禁与别名都用标准库。

### 6.2 完整文件归属表（工作树快照：121 个生产文件 / 30,449 行；行数用于估算）

| 目标包 | 迁入文件（行数） | 备注 |
| --- | --- | --- |
| **`api`（内核残留，17 个文件 ≈ 2.7k）** | `router.go`(388)、`routes_v1.go`(300)、`routes_agent.go`(92)、`routes_e2b.go`(131)、`routes_function.go`(76)、`routes_guards.go`(60)、`route_meta.go`(215)、`policy_gate.go`(93)、`middleware_accesslog.go`(154)、`middleware_bodylimit.go`(87)、`middleware_http.go`(262)、`middleware_ratelimit.go`(194)、`health.go`(62)、`observability_routes.go`(96)、`mcp_dispatch.go`(206)、`site_dispatch.go`(123)、`error.go` 的 `errorHandler` | 路由注册仍在 `api` 的 `mount*Routes` 完成，子包只提供 handler 构造函数（不自带 `Mount`）；`mcp_dispatch` 需要 Echo 实例，向 `agenthttp` 注入 `SubRequester` 窄回调（内置 MCP 继续走完整路由树） |
| **`api/httpkit`**（≈ 1.4k） | 见 §6.1（`error.go` 其余部分、`context.go`、`sse.go`、`audit_helper.go`、`stage_timing.go`、`public_base.go`、`adapter.go` 的 cache / usage / audit / llm 适配器） | P5a 建立 |
| `api/iamhttp`（7 个文件，1,495） | `auth_handler.go`(407)、`users_handler.go`(267)、`api_keys_handler.go`(323)、`api_key_policy.go`(124)、`enduser_handler.go`(172)、`enduser_auth.go`(45)、`login_guard.go`(157) | `login_guard` 被 `middleware_ratelimit` 与 `router` 使用：内核 import `iamhttp`（规则 4 允许） |
| `api/cataloghttp`（1 个文件，202） | `projects_handler.go` | 定义 `KVInitializer` 接口；实现来自 `datahttp`，由 `app` 接线 |
| `api/datahttp`（≈ 30 个文件，≈ 9.3k） | 数据库：`database_handler.go`(441)、`db_stats.go`(137)、`db_stats_cache.go`(213)、`init_sql.go`(73)；SQL：`sql_handler.go`(521)、`sql_types.go`(110)；Schema：`schema_handler.go`(291)、`schema_rows.go`(306)、`schema_sql.go`(123)、`schema_aggregate.go`(133)；集合：`data_handler.go`(763)、`collection_meta_handler.go`(304)、`collection_doc.go`、`collection_query.go`、`collection_transfer.go`(576)、`collection_aggregate.go`、`collection_filter.go`、`collection_pick.go`、`collection_aggregate_query.go`、`aggregate_env.go`；KV：`kv_handler.go`(415)、`kv_commands.go`(325)、`kv_cmd_{hash,key,list,set,string,zset}.go`(1,282)、`kv_init.go`(156)；实时：`realtime_handler.go`(192)；`adapter.go` 的 db / sql / lease 适配器（含 `systemLeaseAdapter` 系统库只读双保险） | **一个上下文一个包**：`SQLLease` / `DataService` 被 10+ 个文件跨资源使用，按资源拆四包会逼出同级 import；包内按现有文件前缀组织。纯逻辑已在 P4 下沉到 `collection`（**已确认**：合为一个包） |
| `api/filehttp`（5 个文件，1,081） | `s3_handler.go`(250)、`s3_content_handler.go`(294)、`s3_site_handler.go`(248)、`site_handler.go`(196)、`site_store.go`(93) | 站点分流仍由 `NewRouter` 的 `e.Pre` 注入（`site_dispatch.go` 留内核） |
| `api/functionhttp`（6 个文件，1,995） | `gofunction_handler.go`(544)、`gofunction_invoke.go`(265)、`gofunction_errors.go`(108)、`function_trigger_handler.go`(222)、`cronjob_handler.go`(674)、`cronjob_target.go`(182) | 执行器唯一性不动 |
| `api/agenthttp`（18 个文件 ≈ 4.4k，P2 之后） | `agent_handler.go`、`agent_run_handler.go`、`agent_approval_handler.go`、`agent_crud_handler.go`、`agent_dto.go`、`agent_tag_handler.go`、`agent_thread_handler.go`、`agent_group_handler.go`、`collab_coordinator.go`、`collab_handler.go`、`collab_query_handler.go`、`collab_group_stream.go`、`run_events.go` 的 HTTP 半边、`mcp_handler.go`(319)、`mcp_builtin.go`(202)、`mcp_builtin_simplebase.go`(288)、`mcp_builtin_admin.go`(163)、`mcp_builtin_session.go`(135)、`mcp_test_connect.go`(19) | 白盒测试最多（约 32 个测试文件），P5 中工作量最大的一批 |
| `api/llmhttp`（4 个文件，1,876） | `llm_handler.go`(200)、`oai_dto.go`(664)、`oai_responses.go`(852)、`llm_traces.go`(160) | 双 `/v1` 路径契约锁定；`oai_responses.go` 不按行数拆 |
| `api/sandboxhttp`（12 个文件，3,616） | `sandbox_handler.go`(462)、`sandbox_images_handler.go`(155)、`sandbox_logs_handler.go`(221)、`sandbox_snapshot_handler.go`(299)、`sandbox_e2b_dto.go`(242)、`sandbox_list.go`(226)、`e2b_auth.go`(331)、`e2b_envd_handler.go`(509)、`e2b_envd_watch.go`(292)、`e2b_process_handler.go`(406)、`e2b_terminal.go`(376)、`e2b_tickets.go`(97) | `e2b_*` 与 `sandbox_*` 互相引用 `SandboxHandler` / `TerminalBridge` 数十处，必须同包（**已确认**：合为一个包）；路由集由 `TestE2BRouteSetLocked` 锁定 |
| `api/backuphttp`（1 个文件，170） | `backup_handler.go` | 与 `httpkit` 同批（机制验证） |
| `api/opshttp`（5 个文件，1,095） | `system_handlers.go`(325)、`system_redact.go`(144)、`quota_audit_handler.go`(127)、`storage_settings_handler.go`(428)、`migration_handler.go`(71) | 只读查询为主，不建用例包装 |
| 迁往非入口包（P1 / P2 已完成） | `llm_trace_recorder.go`、`event_fanout.go`、`gocloudagent_access.go` → `app` / `agentbridge`；`agent_plan_tools.go`、`collab_group_{engine,store,runner}.go`、`collab_cron_runner.go`、`run_events.go` 的 hub 部分 → `agentrun` | — |

### 6.3 拆分顺序与止损规则

- 顺序：**P5a**（`httpkit` + `backuphttp`，机制验证）→ **P5b** `agenthttp`（P2 之后，只余协议代码）→ `iamhttp` / `cataloghttp`（小、独立）→ `llmhttp` → `functionhttp` → `filehttp` → `opshttp` → `sandboxhttp` → `datahttp`（最大、依赖 `httpkit` 最深，最后拆）。
- 有了 `httpkit` 后不再有「先拆不依赖共享 helper 的域」的约束；顺序只按风险从低到高与依赖关系排。
- 每域单独 PR，验收四条：① 子包不 import `api` / `app` / 同级（AST 门禁）；② 用例集合与通过数一致（§6.4）；③ 路由契约测试全绿；④ `internal/api/` 顶层文件数恰好减少被迁文件数。
- **止损规则**：某域若无法在不暴露大量私有 helper、不复制守卫的前提下迁出，则该域保留在 `api`、记录原因，不阻塞其余域。「全量完成」不是上线条件。

### 6.4 白盒测试迁移

- 路由级测试（挂真实 Echo 路由树的 e2e，约 17 个文件经 `NewRouter` 构造）留 `package api` 或迁 `api_test` 外部包，经导出的测试路由构造器测。
- handler 单元测试随 handler 迁子包，私有类型构造改为子包内测试；无法脱离共享私有状态的用例降级为经路由树测并登记。
- **可执行口径**：`go test -json ./internal/... ` 导出 `(包, Test 名, PASS/FAIL/SKIP)`，按 **Test 名集合**（忽略包路径变化）比对迁移前后：并集相同、PASS 数相同；每批 PR 附 before / after 清单。禁止借迁移删用例。

## 7. 批次总表与依赖

| 批次 | 内容 | 依赖 | 粗估 | 风险 |
| --- | --- | --- | --- | --- |
| **P0** 基线与门禁 | 把当前工作树落成提交；基线脚本（§8.3）；路由表 + OpenAPI 快照；权限 / 写门禁矩阵；run / 轮次状态序列与 SSE 帧金标；关停测试夹具（记录 Close 后写次数的 store spy、阻塞型 fake LLM / planner）；AST import 门禁 + `systemdb` 计数；模拟违例测试 | — | 3 人日 | 低 |
| **P1** 适配器归位 | §4 全部 | P0 | 2 人日 | 低–中（fail-closed 语义） |
| **P2a** 上游能力 | `group.Engine.BeginShutdown / WaitIdle`、`RunHeadless` 等待收尾（§3.5） | —（可与 P0 / P1 并行） | 1 人日 | 中（独立模块，自身测试） |
| **P2b** agentrun 应用服务 | §3.1–3.3、§3.6、§3.8；`Converge`；Confirm 前置；删 `Attach` | P0、P2a | 4–5 人日 | **中–高**（状态机、乐观锁、群轮次认领、SSE 顺序） |
| **P2c** app 接线与关停 | §3.4 两阶段关停；`TestAgentRunShutdownCancelsAndDrains` 由红转绿；`-race` | P2b | 2 人日 | 中（关停竞态） |
| **P3** 聚合不变量收口 | §3.7（`transitionRun` 唯一调用点、`RunFinalizer`） | P2 | 1–2 人日 | 中 |
| **P4** 数据纯逻辑 + SSH 前置 | §5 | P0（与 P2 并行） | 2–3 人日 | 低–中（SQL 金标、票据一次性） |
| **P5a** `httpkit` + `backuphttp` | §6.1；备份五条路由回归 | P0 | 2 人日 | 低 |
| **P5b–P5j** 其余 9 个上下文包 | §6.2，每域单独 PR | P5a；`agenthttp` 需 P2；`datahttp` 需 P4 | 13–18 人日 | 中（白盒测试迁移） |
| **P6** 关闭过渡面 | 清理 `api` 的 `httpkit` 转发与 `Dependencies` 冗余字段；内核残留文件清单落金标；更新 `internal/README.md`、`internal/AGENTS.md` 依赖图与例外清单 | P2–P5 实际迁移完成后 | 2 人日 | 低 |

- 合计约 **32–41 人日**。**承诺口径**：P0–P4（约 15–18 人日）即拿到 F 的全部核心收益（G1 生命周期与依赖方向、G2 规则与 HTTP 解耦、G3 边界可检查）；P5 是 D3 的组织性改造，按 §6.3 止损规则逐域推进。
- 推荐执行序：**P0 →（P1 ∥ P2a）→ P2b → P2c → P3 → P4 → P5a → P5b…P5j → P6**。P2c 后按「三类改动演练」（改备份响应 / 修订集合规则 / 接入另一事件消费者）复核收益，不达标则停止 P5 扩面。

## 8. 门禁矩阵

| 门禁 | 命令 / 测试 | 时机 |
| --- | --- | --- |
| 构建 / 静态 | `go build ./internal/... ./cmd/...`、`go vet`、改动文件 `gofmt` 无漂移 | 每批 |
| 路由契约 | `TestOpenAPIMatchesRealRouter`、`TestNewRouter_CrossProjectDeniedBeforeHandlers`、`TestDelegation_RouteTable`、`TestWriteGate_AllWriteRoutesBlockedOnReadOnly`、`TestRouteGuardsApplyPolicyGate`、`TestB3AgentRouteSetLocked`、`TestE2BRouteSetLocked`、`TestSiteHostDispatchBoundaries` | 每批 |
| Agent 协作 | `collab_e2e_test` 系列、`agent_run_gate_security`、`agent_e2e_*`、审批 / 重试用例、SSE 金标、`collab_cron_test` | P2、P3、P5b |
| 并发 / 关停 | `TestRegisterCloserAndClose`；**新增 `TestAgentRunShutdownCancelsAndDrains`**（规格见 §8.2）；`go test -race ./internal/agentrun/... ./internal/api/... ./internal/app/...` | P2 起每批 |
| 持久化 | `TestLeaseWriteGateAcquireHoldAndLost`、`TestSQLHandler_DurabilitySemantics`、`TestFunctionTriggerHTTPAndRecordEndToEnd` | 涉及写路径批次 |
| 数据纯逻辑 | SQL 生成金标、KV 错误文案逐字对比（P0 先建基线） | P4、`datahttp` |
| 端到端 | `./scripts/smoke.sh`；DuckDB / SQLite 双覆盖；权限 / 删库走 `scripts/examples-verify/verify.sh` | P6 或高风险域合并前 |
| 性能 | `cmd/perfbench` 热路径（数据读写、SSE 首帧 / 取消）同机 3 轮对比基线 | 涉及热路径批次；p95 增长 >10% 先解释或修复 |
| 用例集合 | §6.4 的 Test 名集合比对 | P5 每批 |
| 架构门禁 | §8.3 | 每批 |

### 8.1 关停行为测试规格（`TestAgentRunShutdownCancelsAndDrains`）

- 夹具：阻塞至 ctx 取消的 fake LLM 与 fake planner；系统库用现有 e2e 测试库；store spy 记录 `a.Close` 之后的写次数。
- 场景：1 个流式直接 run + 1 个非流式直接 run + 1 个 cron 群轮次（`RunScheduledTurn → RunHeadless`）+ 1 个 `waiting_human` 轮次。
- 断言：① `BeginShutdown` 立即返回，`Wait(5s)` 返回 nil；② 三个在途 run 均 `canceled`，有执行循环的轮次 `canceled`（其 running 任务 `interrupted`），`waiting_human` 轮次状态不变；③ `Wait` 返回后 spy 写计数不再增长，`a.Close` 后为 0；④ `BeginShutdown` 之后 `StartDirectRun` / `SubmitTurn` / `RunGroupCron` 返回 `ErrStopping`；⑤ `-race` 下运行。
- P0 提交夹具与场景脚手架，用例随 P2c 提交并由红转绿。

### 8.2 `systemdb` 使用面计数（D4）

两个口径分开统计、分开设基线：(a) import `internal/systemdb` 的生产文件数；(b) 出现 `*systemdb.Store` 字面的生产文件数。工作树快照：`api` 为 42 / 18，`app` — / 6，`agentbridge` — / 3，`llmgateway` — / 1，`cronjob` — / 1。规则：每个包「只减不增」；新包（`agentrun`、`<context>http`）的 `*systemdb.Store` 字面基线为 0，import 计数以引入时的实际值为基线（`agentrun` 只用行类型，执行时快照为 9 / 0）。

### 8.3 架构门禁实现（P0 交付，随 `go test ./...` 执行）

- 落点：`internal/app/arch_gate_test.go`（标准库 `go/parser` 的 `ImportsOnly` 扫描 `internal/**/*.go` 的全部非测试文件，含带 build tag 的文件——拒绝类门禁宁可过近似；根 `go.mod` 的 `golang.org/x/tools` 是 indirect，不得为此改成直接依赖）。
- 规则：§1.3 的 7 条；违例输出 `importer → imported` 与文件。既有例外逐条登记（原因 + 去除批次）：`api/perfbench → app`（压测入口）、迁移期的 `api → <context>http`（规则 4 允许，无需例外）。
- 基线：`internal/app/testdata/arch-baseline.txt`（包 → 两个 `systemdb` 计数），由 `go test ./internal/app -run TestArchitectureGate -update` 生成；评审时核对 diff 只有下降。
- 内核残留金标：P5 每批更新 `internal/api` 顶层生产文件清单，P6 锁定为 §6.2 的 17 个文件。
- 自检：引入一条模拟违例（临时在 `agentrun` import echo）必须失败；`scripts/smoke.sh` 增加一行运行该测试。

## 9. 回滚与发布

- 每批次单独 PR、可独立 `git revert`；不做运行期双写、不引入数据格式变化（P2 无 schema 变更）。
- P2 回退即恢复 `GroupCronRunner.Attach` 与 handler 内收敛；启动收敛保证回退后数据一致（残留轮次落 `interrupted`）。
- P2a（上游）向后兼容：不调用 `BeginShutdown` 的旧宿主行为不变，可独立保留。
- P5a 的 `httpkit` 以别名转发，回退只需还原子包与路由引用。
- 未通过门禁的域单独退回；发布只替换二进制，按现行单写实例流程滚动。

## 10. 明确不做（防范围蔓延）

- 不做完整 DDD 目录化（`domain/application/infra`）、不按聚合拆 `systemdb`、不为非核心上下文补 service 层、不引入统一事件总线、不做 CQRS / Event Sourcing / 微服务、不新建 `internal/usecase/*` 通用树。
- 不改：公开 API 路径与错误形状（除新增 503 `service_stopping`）、OpenAI 兼容协议、SSE 帧格式、系统库 schema、对象键、配置来源、权限判定入口、`subRequest` 式内置 MCP。
- 不重写 `oai_responses.go` 等单一协议转换文件；不复制 Turn / Task 状态机（唯一权威在 `gocloudagent/group/state.go`）；run 状态不变量不在应用层再造一套 CAS（执行点在 `systemdb` 的 LIFE-04 条件更新）。
- 不为关停收尾新增配置项（`Wait` 的 10s 为常量）；不改变长期 SSE（realtime、`waiting_human` 轮次事件流）占用 `httpServer.Shutdown` 预算的既有行为——如 P2c 实测发现它挤占后续步骤，再在 P6 评估给第 1 步单独设子预算。
- 不处理 `RowCountCache.RefreshAsync` 的 5 秒内短生命周期 goroutine（`db_stats_cache.go:127-166`）；`datahttp` 迁移时可顺手加 `Wait`，不单独立项。
- 不做 run 的启动收敛（D6-c 已否决）：进程被杀遗留的 `running` run 维持现状。
- 不区分「用户取消」与「关停取消」的 error_code（两者均为空）；如需审计区分，另提。
