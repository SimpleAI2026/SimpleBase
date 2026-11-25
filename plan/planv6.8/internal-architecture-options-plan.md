# `internal/` 架构方案对比与决策稿（HMVC / 垂直切片 / 六边形 / DDD）

- 日期：2026-11-25；状态：**待决策**。本文只比较方案，不改代码。
- 上游：[分层改造计划](internal-layered-hmvc-architecture-plan.md)（v2，含 §8 全 `internal/` 评估）。本文是它的 v3：把“要不要分层”升级为“选哪种架构范式”，并修正 v2 中已过时的事实（§1.2）。
- 约束基线：[项目约束](../../AGENTS.md)、[后端约束](../../internal/AGENTS.md)。所有方案都不改公开 API、系统库 schema、对象键布局、配置来源、`go.mod`，四个嵌套独立模块仍不得 import 根模块。
- 证据口径：HEAD `dead32f` + 当前工作树（含大量未提交改动）；`go build ./internal/... ./cmd/...` 通过。数字为生产代码（不含 `_test.go`），统计脚本见各节说明；工时为粗估，用于方案间相对比较。

## 1. 问题陈述（选方案之前先对齐要解决什么）

### 1.1 现状数字

| 指标 | 值 |
| --- | --- |
| `internal/api` | 122 个生产文件 / 31,095 行；直接依赖 23 个 `internal/` 包；191 处路由注册调用 |
| `api` 测试 | 104 个 `_test.go`，**全部是 `package api` 白盒测试**，共 583 个 `Test*` 函数 |
| `api` 使用 `systemdb` | 44 个生产文件，引用 107 个不同的 `systemdb` 导出符号 |
| `systemdb` | 37 文件 / 9,357 行，`*Store` 导出方法 186 个（其中 Agent 相关文件 79 个） |
| `api` 按候选业务域分布（行） | 数据面 8,162 · Agent 协作 7,296 · 沙盒/E2B 3,616 · 入口内核 3,595 · 自动化 2,073 · LLM 1,955 · IAM 1,588 · 运维 1,423 · 文件/站点 1,204 |
| `systemdb` 按候选业务域分布（行） | Agent 3,163 · 平台（连接/schema/flush/查询）2,500 · 观测 1,791 · 自动化 920 · 文件/站点 396 · 沙盒 339 · 设置/终端用户 248 |

### 1.2 结构问题（v2 §8.4 复核后）

| 编号 | 问题 | 当前证据 | v2 描述是否仍成立 |
| --- | --- | --- | --- |
| P1 | **Agent 运行生命周期挂在 HTTP handler 上，`app` 关停不覆盖** | `cloudAgentHandler` 持有 `groupEngines`（每项目一个 `group.Engine`，其 `execute` 以 `WithoutCancel` 起 goroutine）、`routeCancels`、`events`、`confirms`；`runStream` 以 `context.WithoutCancel` 运行（SSE 断开后 run 继续）；`App.Shutdown` 停 HTTP / cron / trigger / KV sweeper / registry，**没有取消或等待 Agent run 与群轮次** | **部分过时**：`plan_handler.go` 与 `go h.runPlan` 已随 planv6.8 B14 删除；但生命周期问题转移到 run 与群引擎，依然成立 |
| P2 | **cron → HTTP handler 的环形延迟绑定** | `GroupCronRunner.Attach(h)` 在 `newCloudAgentHandler` 内调用；未绑定时返回 `collaboration runner is not attached` | 成立 |
| P3 | **router 构造期修改共享运行时 / 回填对象** | `newCloudAgentHandler` 把 `confirmHub` 写进 `deps.CloudAgent.Confirm` 与 `RuntimeProvider.Confirm`；`app.buildHTTPServers` 在 `NewRouter` 之后回填 `deps.Terminal.Server = a.bindSandboxSSHAuth(...)` | 成立，且比 v2 多一处（Confirm 注入） |
| P4 | **非 HTTP 适配器住在 `api`** | `api.NewLLMTraceRecorder`（网关用）、`api.RecordEventFanout` / `TriggerEventPublisher`（触发器用）、`api.NewCloudAgentDB/Obj/LLM`（Agent 运行时用） | 成立 |
| P5 | **领域规则留在入口** | 集合过滤 / 聚合约 900 行不 import Echo；KV 命令解析签名绑 `echo.Context`；Agent 状态变更（`UpdateAgentRunStatus` / `UpdateAgentTurnStatus`）分散在 `api` 与 `agentbridge` 多处直接调仓储 | 成立 |
| P6 | **白盒测试与包边界强耦合** | 583 个测试函数全部在 `package api` 内，大量直接构造私有类型 | 新增：任何“拆 `api` 子包”的方案都要为此付出迁移成本 |

**决策目标**（用来给方案打分）：G1 消除 P1–P3 的生命周期与依赖方向错误；G2 让“改业务规则不必动 HTTP 包”；G3 可被编译器 / 门禁检查的边界；G4 迁移成本与回归风险可控；G5 不制造第二套鉴权 / 错误 / 路由。

## 2. 候选方案

### A. 现状 + 同包整理（基线）

继续 `internal-simplify-refactor-plan.md` 的做法：同包拆文件、去重复、加 import 门禁，不新增包。

- 适用：团队只想控制文件复杂度。
- 解决：P5 的一小部分（纯函数可以集中到同包文件）。**不解决 P1–P4**：生命周期与依赖方向是包级 / 对象图问题，拆文件改变不了。

### B. 严格 HMVC（Hierarchical MVC）

每个业务模块是一个自带 Model / View / Controller 的三元组，父控制器把请求委派给子三元组，模块可独立挂载。典型实现：PHP Kohana / Laravel modules、早期 Java Portlet。

- 映射到本项目：`internal/api/<module>/{controller,view,model}`，模块自带 `Mount(router)`，父 controller 通过内部调用子 controller 组合页面或响应。
- 匹配度低的原因：
  1. HMVC 的核心价值是**服务端视图组合**（一个页面由多个子请求渲染的片段拼成）。本项目的 View 是 JSON / SSE / 二进制，UI 在 `ui/` 的 SPA，没有需要组合的服务端视图。
  2. “子控制器调用”在本项目已有对应物且有意为之：内置 MCP 经 `subRequest` 回到完整路由树，保证鉴权 / 委托 / `writeGate` / 审计不被绕开。HMVC 若推广为通用的控制器间调用，会诱导绕开路由守卫。
  3. 模块自带路由与中间件会产生第二套鉴权、限流、项目上下文——正是 G5 要避免的。
  4. HMVC 不处理 P1–P4：它关心请求分发层次，不关心后台运行的所有权与依赖方向。
- 可取之处：**每个模块提供显式 `Mount` 函数**这一点有价值（本项目已有 `mountAgentRoutes` / `mountE2BRoutes` 雏形），被方案 C 吸收。

### C. 垂直切片 / 按功能分包的模块化单体（Vertical Slice / Package by Feature）

按业务域把入口拆成 `internal/api/<domain>http` 子包，每个子包含本域 handler、DTO、`Mount`；共享内核（守卫、错误、上下文）留在 `api`。代表：Jimmy Bogard 的 Vertical Slice Architecture、Simon Brown 的 “package by component”。这是 v1 计划 H1/H3 的主体。

- 解决：G3（子包边界可检查）；P4 可顺带。**对 P1–P3 帮助有限**：把 `cloudAgentHandler` 挪进 `api/agenthttp`，后台运行仍挂在 handler 上。
- 成本：P6 —— 104 个白盒测试文件要随 handler 迁移并改为子包内测试或导出测试接口；共享 helper（`WriteError` 被 50 个文件使用、`requestID` 43、`principalOf` 20）需要抽成 `httpkit` 或注入回调。

### D. 六边形 / 端口与适配器 + 应用服务层（Hexagonal / Clean Architecture-lite）

把“用例编排 + 运行生命周期”从入口移到**应用服务**；应用服务依赖自己定义的端口（窄接口），由 `app` 注入 `systemdb` / `godatabase` / `gocloudagent` 的适配器；HTTP、cron、MCP 都只是驱动适配器。代表：Alistair Cockburn 的 Hexagonal、Robert Martin 的 Clean Architecture（取其依赖规则，不取四层目录模板）。

- 本项目已有大量该模式的实例：`cronjob.Store`、`functionrun.Store`、`trigger.Store`、`sandbox.Store`、`agentbridge/ports.go`（`DatabaseAccess`、`ObjectAccess`、`LogReader`…）、`dbbridge`。缺的只是**Agent 协作域的应用服务**与若干放错位置的适配器。
- 解决：P1、P2、P3、P4 直接解决；P5 按需。G5 天然满足（入口仍是唯一的 Echo 路由树）。
- 成本：只动一个域的编排与装配，`api` 包不拆，白盒测试大部分原地保留。

### E. DDD（领域驱动设计）——完整战略 + 战术

战略层：划分限界上下文（Bounded Context）、上下文映射（Context Map：共享内核、客户/供应商、防腐层 ACL）、子域分类（核心 / 支撑 / 通用）。战术层：聚合（Aggregate）与聚合根守护不变量、按聚合的仓储（Repository）、领域服务、领域事件、值对象。代表：Eric Evans《Domain-Driven Design》、Vaughn Vernon《Implementing DDD》。

完整落地形态：

```text
internal/<context>/
  domain/        聚合、值对象、领域事件、仓储接口（不依赖任何基础设施）
  application/   应用服务（用例、事务边界、发布领域事件）
  infra/         仓储实现（从 systemdb 拆出）、外部系统 ACL
  interfaces/    HTTP / MCP / cron 适配
```

- 解决：G1–G3 全部，且长期演进最清晰。
- 成本（本项目特有的高成本点）：
  1. **`systemdb` 要按聚合拆仓储**：186 个导出方法、`api` 依赖 107 个符号；同一个 DuckLake 连接、`schema.go` 最终态、`NotifyWrite` / 异步 flush / 读观测是共享的物理边界，拆包后需要再导出一层“平台内核”。
  2. **聚合需要从行模型中提炼**：现在的 `systemdb.AgentRun` / `AgentTurn` / `CronJob` 是贫血行结构，状态迁移规则散在 `api`、`agentbridge`、`gocloudagent/group/state.go`；改成聚合根要逐个梳理不变量，这是语义重构而非搬迁。
  3. P6 的测试迁移成本与方案 C 相同或更高。
  4. 嵌套模块 `gocloudagent/group` 已经拥有 Turn / Task 状态机（`turnTransitions` 等），宿主再建一套 Agent 聚合会形成双重状态机，必须明确只保留一处。

### F. DDD 战略设计 + 六边形应用服务（推荐的混合）

用 DDD 的**战略**工具画清上下文与依赖方向，只在**核心且复杂**的上下文（Agent 协作）使用战术模式（应用服务、聚合守护状态迁移、领域事件），其余上下文保持现有“领域包 + 窄端口”形态；入口不拆包或按需拆。即 D 的落地 + E 的边界语言。代表做法：Vernon 所说的 “DDD-Lite 只用于核心域”，以及 Spring Modulith / “modular monolith with bounded contexts”。

### G. 其他范式（评估后不作为主方案）

| 范式 | 结论 | 理由 |
| --- | --- | --- |
| CQRS（读写分离模型） | 不采用 | 写路径已经强依赖单写实例 + lease；读模型无独立存储需求。日志 / 指标查询已天然是只读路径，无需形式化 |
| 事件驱动 / Event Sourcing | 不采用（保留进程内领域事件） | `trigger` 明确不承诺持久化与 exactly-once；系统库没有事件存储需求。保留现有 `RecordEvent` / `ObjectEvent` 进程内发布即可 |
| 微服务 / 插件化 | 不采用 | 单写实例 + 单系统库连接；拆进程会引入分布式事务与新故障模式 |
| 经典三层（Controller-Service-Repository 全量模板） | 不采用 | 已有领域包是“按需两层”；对每个资源补全三层会产生大量只转发的 service |

## 3. 方案打分

评分 1–5（5 最好）。“成本”与“风险”分越高代表越低成本 / 越低风险。

| 维度（权重） | A 同包整理 | B 严格 HMVC | C 垂直切片 | D 六边形应用服务 | E 完整 DDD | **F 战略DDD+六边形** |
| --- | --- | --- | --- | --- | --- | --- |
| G1 修复生命周期 / 依赖方向 P1–P3（30%） | 1 | 1 | 2 | 5 | 5 | **5** |
| G2 业务规则与 HTTP 解耦（20%） | 2 | 2 | 3 | 4 | 5 | **4** |
| G3 边界可检查（15%） | 2 | 3 | 4 | 3 | 5 | **4** |
| G4 迁移成本与风险（25%） | 5 | 2 | 2 | 4 | 1 | **4** |
| G5 不制造第二套守卫 / 路由（10%） | 5 | 1 | 4 | 5 | 4 | **5** |
| **加权分** | 2.75 | 1.75 | 2.70 | 4.25 | 3.90 | **4.40** |
| 粗估工作量 | 3–5 人日 / 持续 | 25–40 人日 | 15–25 人日 | 8–12 人日 | 40–60 人日 | 10–15 人日（不含可选 C） |

工作量假设：一名熟悉代码的工程师；含补测试与回归，不含真实 S3 / LLM / E2B 环境的联调排期。E 的主要成本在 `systemdb` 拆仓储与聚合提炼；C 的主要成本在 104 个白盒测试文件迁移。

## 4. 推荐方案 F 的具体设计

### 4.1 限界上下文与子域

| 上下文 | 子域类型 | 现有代码（下层 / 入口 / 持久化） | 对外语言（共享内核或公开契约） |
| --- | --- | --- | --- |
| 身份与访问 IAM | 通用 | `auth` / `api` 的 auth、users、api_keys、enduser、login_guard、policy_gate（1,588 行）/ `auth` 自有仓储 + `systemdb/enduser_repo` | **共享内核**：`auth.Principal`、`Permission`、`HasPermission`、委托标记 |
| 项目与目录 Catalog | 支撑 | `catalog`、`dbbridge` / `projects_handler`、`database_handler` / `catalog` 仓储 | `catalog.Database` 状态机（creating→ready→deleting→deleted，degraded）、`ReservedSystemProjectID` |
| 数据服务 Data | **核心** | `collection`、`godatabase`（registry / kv / sqlguard）/ SQL、schema、collection、KV、realtime（8,162 行）| 领域事件 `RecordEvent`（发布给 Automation、Realtime） |
| 文件与站点 Files | 支撑 | `objectstore` / s3、site（1,204 行）/ `systemdb` sites、s3index | 领域事件 `ObjectEvent` |
| 自动化 Automation | 支撑 | `functionrun`、`trigger`、`cronjob`、`gofunction` / gofunction、function_trigger、cronjob（2,073 行）/ `systemdb` cron、gofuncs、triggers | `cronjob.AgentRunner`（向 Agent 上下文要执行能力） |
| Agent 协作 | **核心（最复杂）** | `agentbridge`、`gocloudagent`（cloudagent / group / plan）/ agent、collab、mcp、run_events（7,296 行）/ `systemdb` agent_*（3,163 行）| 应用服务接口（见 4.3）；领域事件：run / turn 状态变化 |
| LLM 网关 | 通用 | `llmgateway`、`usage` / oai、llm（1,955 行）/ `systemdb` llm、traces | `llmgateway.Service`、`UsageRecorder` |
| 沙盒 | 支撑 | `sandbox`、`sandbox/sshd`、`e2bproto`、`gosandbox` / sandbox、e2b（3,616 行）/ `systemdb` sandboxes | `sandbox.Manager`、终端票据 |
| 运维治理 | 通用 | `backup`、`audit`、`alerts`、`observability`、`usage` / backup、quota、system、health（1,423 行）/ `systemdb` 观测（1,791 行）| 审计事件（脱敏）、告警总线 |
| 平台（非业务上下文） | — | `app`（组合根）、`config`、`systemdb` 平台部分（连接、schema、flush、查询，2,500 行）、`api` 入口内核（3,595 行）、`web`、`utils` | 唯一组合根、唯一路由树、唯一系统库连接 |

### 4.2 上下文映射（Context Map）

```text
                    ┌──────────── 共享内核：auth.Principal / Permission ────────────┐
                    │                                                              │
 Agent 协作 ──ACL(agentbridge.ports)──▶ 数据服务 / 文件 / 观测（DatabaseAccess、ObjectAccess、LogReader）
 Agent 协作 ──Customer/Supplier──▶ LLM 网关（cloudagent.ChatClient ← llmgateway）
 Agent 协作 ──ACL(agentbridge)──▶ gocloudagent（通用子域，独立模块，不 import 宿主）
 自动化     ──Customer/Supplier──▶ Agent 协作（cronjob.AgentRunner / GroupCronRunner）
 数据服务   ──领域事件 RecordEvent──▶ 自动化(trigger)、实时(realtime)
 文件       ──领域事件 ObjectEvent──▶ 自动化(trigger)
 数据服务   ──ACL(dbbridge)──▶ godatabase（通用子域，独立模块）
 沙盒       ──ACL(sandbox.Store / gosandbox/vm)──▶ gosandbox（通用子域，独立模块）
 运维(backup) ──Conformist──▶ 目录 + 数据服务（复用 catalog / registry 契约）
```

判断：**防腐层和共享内核在本项目已经存在且方向正确**（`dbbridge`、`agentbridge`、四个独立模块）。缺陷集中在两点——自动化 → Agent 的供应关系经由 HTTP handler 实现（P2）；领域事件的发布器实现住在 `api`（P4）。

### 4.3 核心域（Agent 协作）的战术设计

目标：新建 `internal/agentrun`（名称待决，见 §6 D2）作为 **Agent 协作上下文的应用服务层**。

| 元素 | 内容 | 来源 |
| --- | --- | --- |
| 应用服务 `agentrun.Service` | `StartDirectRun`、`StartAutoRun`、`CancelRun`、`Approve/Reject`、`CreateTurn`、`ResumeTurn`、`RunGroupCron`、`ReplayEvents`、`Close(ctx)` | 从 `agent_run_handler.go`、`agent_run_auto.go`、`collab_coordinator.go`、`collab_cron_runner.go`、`agent_approval_handler.go` 中提取非 HTTP 部分 |
| 长期状态（归应用服务所有） | `runEventHub`、`confirmHub`、`routeCancels`、`groupEngines`（每项目 `group.Engine`）、启动收敛 `Converge` | 现 `cloudAgentHandler` 字段 |
| 端口（消费方定义） | `RunStore`（run / message / approval）、`ThreadStore`、`GroupStore`（现 `collabGroupStore` 的实现迁为适配器）、`AgentCatalog`、`Quota`、`Audit` | 仓储实现仍是 `*systemdb.Store`，由 `app` 注入 |
| 聚合与不变量 | **AgentRun**：状态迁移（queued / running / completed / failed / canceled）集中到一个 `transitionRun` 并带版本校验；**Turn / Task**：唯一权威是 `gocloudagent/group/state.go`，宿主不再复制规则 | 现分散在 `api`、`agentbridge/cron_agent.go` 的 `UpdateAgentRunStatus` 直调 |
| 领域事件 | `RunEvent`（既有 `cloudagent.Event` 流）由应用服务写入 `runEventHub`；HTTP 只订阅并编码为 SSE | 保持现有帧格式 |
| 生命周期 | `app` 构造，持有根 `ctx`；`Shutdown` 中在停 HTTP 之后、停 cron 之前调用 `agentrun.Service.Close(ctx)`：取消在途 run / 轮次，等待受 ctx 超时约束，超时记录告警；启动时由 `app` 调 `Converge` | 解决 P1 |
| 依赖方向 | `api`（agent handler）→ `agentrun`；`cronjob` → `agentbridge.CronAgentExecutor` → `agentrun`（经 `GroupCronRunner` 端口）；`agentrun` 不 import `api` / Echo / `app` | 解决 P2（删除 `Attach`） |
| Confirm 注入 | `confirmHub` 由 `app` 先构造并注入 `cloudagent.Runtime` 与 `RuntimeProvider`，router 不再修改共享运行时 | 解决 P3 的一半 |

### 4.4 其他上下文的动作（不做战术建模）

| 上下文 | 动作 | 解决 |
| --- | --- | --- |
| 数据服务 | 集合过滤 / 聚合纯逻辑下沉 `collection`；KV 命令解析改为返回参数错误值（去掉 `echo.Context`）；`RecordEventFanout` 移到 `app` 私有或 `realtime` 旁的事件包 | P4、P5 |
| LLM 网关 | `NewLLMTraceRecorder` 移到 `app` 私有适配器 | P4 |
| Agent ↔ 数据 / 文件 | `NewCloudAgentDB/Obj/LLM` 移到 `agentbridge`（它们实现的就是 `agentbridge` 端口），SQL 只读守卫与系统库拒绝原样保留 | P4 |
| 沙盒 | 终端票据校验与沙盒查找抽成 `app` 在 `NewRouter` 前可构造的对象；`bindSandboxSSHAuth` 前置 | P3 另一半 |
| `systemdb` | **不拆物理包**。新代码只经消费方窄接口使用；门禁统计“直接引用 `*systemdb.Store` 的生产文件数”只减不增（基线 `api` 44） | G3 |
| IAM / 目录 / 运维 / 自动化 | 维持现状；它们已经是“领域包 + 窄端口” | — |

### 4.5 入口层（`api`）的定位

- `api` 保持**单一包、单一路由树、单一守卫与错误协议**（不采用 HMVC 的模块自带路由）。
- 是否继续按域拆 `api/<domain>http`（方案 C）作为**可选第二阶段**：F 完成后，`agent_*` handler 只剩协议代码，届时再判断拆包收益；不拆也满足 G1–G2。

## 5. 推荐方案的实施阶段

| 阶段 | 内容 | 产出 / 门禁 | 估算 |
| --- | --- | --- | --- |
| F0 基线 | 路由表与 OpenAPI 快照、权限 / 写门禁矩阵、Agent run 与群轮次的状态序列与 SSE 帧金标、关停行为测试（新增：运行中 run 在 `Shutdown` 后不再写库）、import 门禁（`agentrun` 不 import `api`/Echo/`app`；`*systemdb.Store` 计数） | 基线可重复；门禁能判好坏 | 2–3 人日 |
| F1 适配器归位 | P4 全部：trace recorder、事件 fanout、CloudAgent DB/Obj/LLM 适配移出 `api` | `app` 不再从 `api` 取非 HTTP 适配器；行为零差异 | 1–2 人日 |
| F2 Agent 应用服务 | 建 `agentrun.Service`，迁长期状态与编排；`app` 构造并在 `Shutdown` 中关闭；删除 `GroupCronRunner.Attach`；Confirm 由 `app` 注入 | P1、P2、P3(Confirm) 消除；`TestB3AgentRouteSetLocked`、协作 e2e、cron 群定时、审批、重试、SSE 金标不退化；`-race` 通过 | 5–7 人日 |
| F3 聚合不变量 | `AgentRun` 状态迁移收口到应用服务单点；宿主侧 Turn / Task 规则只引用 `gocloudagent/group` | 状态迁移调用点收敛到 1 处（门禁计数） | 1–2 人日 |
| F4 沙盒装配前置 + 数据纯逻辑下沉 | P3 余项、P5 | SSH / 终端票据测试、集合 SQL 金标、KV 错误文案逐字一致 | 2–3 人日 |
| F5（可选）入口按域拆包 | 即方案 C，只对 Agent、数据两个最大域试点 | 见 v1 计划 H1/H3 | 另计 8–12 人日 |

回滚：每阶段单独 PR，只做等价搬迁与生命周期接线，`git revert` 即可回退；不引入数据格式变化。

## 6. 需要你确定的决策

| 编号 | 决策 | 选项 | 建议 |
| --- | --- | --- | --- |
| **D1** | 采用哪个方案 | A 同包整理 / B 严格 HMVC / C 垂直切片 / D 六边形应用服务 / E 完整 DDD / **F 战略 DDD + 六边形** | **F**；若只能投入一周，取 F0–F2（等价于 D） |
| D2 | Agent 应用服务的落点 | ① 新包 `internal/agentrun`；② 并入 `internal/agentbridge`；③ `internal/agent/{application,domain}` 目录化 | **①**：`agentbridge` 的职责是“宿主 ↔ gocloudagent 适配”，再放应用服务会混职责；③ 是完整 DDD 目录，F 阶段收益不足 |
| D3 | 入口是否按域拆子包（方案 C） | 不拆 / F 完成后对 Agent、数据试点 / 现在全量拆 | **F 完成后再决定**；现在拆要先付 104 个白盒测试文件的迁移成本 |
| D4 | `systemdb` 是否按上下文拆包 | 不拆 + 窄接口门禁 / 只拆 Agent 仓储为子包 / 全量按聚合拆 | **不拆**；如 F2 后 Agent 仓储接口仍过宽，再评估“只拆 Agent 子包” |
| D5 | 领域事件机制 | 保持进程内同步 fanout / 引入统一进程内事件总线 | **保持现状**，只移动所有权；当出现第三个事件消费者时再评估总线 |
| D6 | 关停语义 | ① `Shutdown` 时取消在途 run / 轮次，超时内等待收尾写库（run 落 `canceled` 或 `failed`，`AgentRun*` 现无 interrupted 状态，新增需改 schema 语义）；② 等待自然完成到超时；③ 维持现状（进程退出即中断，靠启动收敛修正） | **①**；需要你确认是否接受“发版时在途 run 被取消”的用户可见行为，以及取消后落 `canceled` 还是 `failed` |

## 7. 各方案不做什么（防止范围蔓延）

- 任何方案都不改：公开 API 路径与错误形状、OpenAI 兼容协议、SSE 帧、系统库 schema、对象键、配置来源、权限判定入口、`subRequest` 式的内置 MCP。
- 选 F 时不做：全量 `domain/application/infra` 目录化、`systemdb` 拆包、为非核心上下文补 service 层、统一事件总线、CQRS。
- 选 C / E 时额外需要：先单独立项评估白盒测试迁移方案（导出测试辅助 vs. 子包内重写），否则不启动。
