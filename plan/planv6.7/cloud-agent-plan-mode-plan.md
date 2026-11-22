# 通用助手 Plan 模式与站点默认可发布

- 日期：2026-11-19
- 状态：待实现。本文件中的接口、类型和表名均为提案。
- 关联：[首页示例搭建](../planv6.6/home-starter-prompts-plan.md)、[对象存储站点](../planv6.5/object-storage-sites-plan.md)。本方案接管前者的阶段编排，保留后者的站点隔离和访问规则。

## 1. 目标与边界

1. 移除实例级 `sites.enabled` 和项目级站点开启状态的发布门槛。默认具备发布能力，**对象默认私有**；公开 `web/` 仍是显式操作，继续要求权限与确认。
2. 云端 `gocloudagent/cloudagent` 提供通用 Plan 模式。固定展示顺序：**规划 → 数据库 → KV 存储 → 云函数 → 定时任务 → 前端站点**。对任意需求裁剪不适用阶段，并记录跳过原因；校验并入各阶段和最终汇总，不再有第七个阶段。
3. 通用助手输入区支持「对话 / Plan」。首页「试试这样说」只负责选需求和建项目，自动调用同一个 Plan 入口；不创建专用助手，不保留浏览器阶段调度器。
4. V1 只处理当前项目的渐进式搭建，不做跨项目工作流、不自动删除资源、不手工触发定时任务、不扩大全局工具权限。浏览器退出不影响后台执行。

## 2. 架构

```text
 Home: starter cards                    Console: composer
   | project + thread + seed             | mode = chat / plan
   +------------------+------------------+
                      v
             internal/api: auth + write gates
                  | chat              | plan
                  v                   v
             existing run     gocloudagent/cloudagent
                               Plan coordinator
                                   |  ^
                       stage run   |  | typed evidence
                                   v  |
                          existing runtime + tools
                                   |
                      internal/agentbridge + sbcli
                                   |
                 SQL / KV / functions / cron / objects
                                                     |
                                         web/ public rule
                                                     v
                            <project>.<site-domain>/web/

 Coordinator -> PlanStore interface -> host adapter -> system DB
 Console     <- Plan snapshot + replayable SSE <- host API
```

- `gocloudagent/cloudagent` 只放**纯逻辑**：阶段注册表、阶段技能映射、阶段提示词、证据判定（从工具卡片推导阶段是否完成）。不持有 run 生命周期，不依赖宿主包。
- `internal/api` 持有协调器：run 行、消息、审批、SSE 事件缓存都在宿主侧，协调器必须与它们同进程同包，避免把整套运行管道搬进独立模块。`internal/systemdb` 存储计划与阶段状态；`internal/app` 负责装配与停机。
- UI 只发送控制意图、读取快照和订阅事件。进度不再来自浏览器本地快照、模型文本或前端工具卡片推断。

## 3. 阶段契约

| ID | 阶段 | 可验收结果 |
|---|---|---|
| `plan` | 规划 | 结构化的阶段适用性、目标资源、依赖和校验项；服务端校验 schema、固定次序、当前项目范围，未通过不执行写操作 |
| `database` | 数据库 | 幂等迁移；读取并核对表/集合和预期结构，不以 SQL 文本或模型自报为准 |
| `kv` | KV 存储 | 仅为确有需要的场景创建键；命名空间、TTL 策略、读回校验；交付不展示值或密钥 |
| `functions` | 云函数 | 检查、保存、运行安全测试并读回路由；不调用会产生真实外部副作用的入口做探测 |
| `cron` | 定时任务 | 核对调度表达式、时区、目标和启用状态；自动化不强制启用或立即执行 |
| `site` | 前端站点 | 生成并上传 `web/index.html` 及必要静态资源，管理台 `/console/s3` 可见；授权公开 `web/` 后验证站点入口并输出服务端返回的完整子域名 URL |

规划阶段为只读。模型提议阶段目标，不决定执行权限，也不能自行生成任意工具白名单。先检查每阶段实际能力，再按需加载对应 SQL、KV、函数、cron、对象及站点技能。依赖项跳过时，依赖阶段标记阻塞，直到用户调整目标或提供替代依赖；不能用未完成资源继续。面向匿名站点的静态快照必须明确可公开，不能把数据库实时凭据放进网页。最终交付以已验证的结构化资源清单为准，区分已发布、已上传未公开、待处理和已跳过。

## 4. 执行、持久化与恢复

```text
 create(idempotency_key)
          |
          v
       queued ---> running(stage, attempt)
                      |   |       |
                      |   |       +--> awaiting_confirmation
                      |   |               | approve / reject / expire
                      |   |               v
                      |   +---------- reconcile current attempt
                      |
                +-----+------+
             verified       failed/uncertain --> paused
                |                              | retry / skip / takeover
                v                              v
       next stage / completed             queue / cancel
```

- `sys_agent_plans` 持有项目、线程、助手、状态、当前阶段、版本与幂等键；`sys_agent_plan_stages` 每计划六行，记录状态、尝试号、run ID、跳过原因与去敏结果。不存凭证或完整工具正文；保留期与对话历史政策对齐。
- **单写实例，不做多节点租约**：协调器是进程内单例，`sys_agent_plans` 的 `version` 只用于乐观并发（同一计划不被两个请求同时推进），不承担跨节点选主。线程互斥继续复用 `Runtime.ClaimThread`：阶段 run 用自身 run ID 占锁，协调器在两次阶段之间释放，普通对话因此能在阶段间隙插入并自动结束计划（`takeover` 语义）。
- 先持久化阶段意图和 run ID，再启动 run。恢复时先查该阶段 run 的落库状态：`completed` 则按证据推进，`running` 则续接 SSE，`failed/canceled` 则暂停等待用户选择。**进程重启后内存中的 run 已不存在**，启动时把所有 `running` 计划标记为 `interrupted`，由用户重试该阶段；不自动重跑，避免重复副作用。
- 外部副作用不承诺恰好一次。各工具使用稳定资源标识，遇到重复执行先比较目标状态；遇到不确定写入或目标冲突不自动覆盖。取消不回滚已落地资源，保留清单。重试增加尝试号，跳过保留原因，接手结束自动编排但不删除资源。
- 每阶段沿用单 run 限额，另设总时长、总尝试次数和总调用预算。每次推进重新校验权限和实例写状态；确认绑定当前计划/阶段/尝试/命令，不得复用于下一次尝试。

## 5. API 与界面

建议在现有项目/线程作用域下增加：

| 接口 | 行为 |
|---|---|
| `POST .../threads/:threadId/plans` | 接收 `goal`、`agent_id`、`idempotency_key`，可选 `source`/`template_id` 仅作元数据；返回计划 ID 与初始快照 |
| `GET .../threads/:threadId/plans` | 列出该线程最新及历史计划 |
| `GET .../threads/:threadId/plans/:planId` | 返回六阶段状态、尝试、交付和 `last_event_seq` |
| `GET .../plans/:planId/events` | 使用 `Last-Event-ID` 或 `after` 续订；游标过期回到快照，不补执行 |
| `POST .../plans/:planId/actions` | `retry` / `skip` / `cancel` / `takeover`，附目标阶段及 `expected_version`；复用现有工具确认接口 |

创建和控制 API 必须验证当前主体、项目与线程关系；旧版本操作返回 `409` 并重新读快照。模式切换只影响下一次发送，不取消或改变已有计划。进度条固定显示六项，展开尝试与确认信息；失败可重试/跳过/接手，公开审批未完成时显示「待确认」。页面刷新先取快照再从快照游标连 SSE；事件去重按计划 ID 和序号。

```text
 Home card -> confirm -> create project/thread -> POST plan -> open console
 Console Plan composer --------------------------^                |
                                                                  v
 reload / another tab -> GET snapshot -> SSE after cursor -> render
```

首页自动提交用稳定的启动键防止双击/刷新重复创建；组合建项目与创建计划失败时，保留已建项目和线程，重试补建计划而不是创建第二项目。URL 携带线程/计划 ID，不携带需求原文或凭据。保留旧 starter 会话的消息和交付内容，不把历史运行自动转为新 Plan。

## 6. 站点默认开启与旧配置迁移

- **直接删除** `Sites.Enabled`：配置结构体字段、YAML 键、`SIMPLEBASE_SITES_ENABLED` 环境变量、`sites_disabled` 原因码、`sys_project_sites` 表与 `SetProjectSite`、`site enable` 命令、前端开关与文案一并移除，不做兼容解析。站点是否可发布只由 `base_domain` 是否配置、项目 ID 是否 DNS 安全、是否系统项目决定。旧 `config.yaml` 里的 `sites.enabled` 键会被 YAML 解析忽略（`yamlSites` 不再声明该字段），无需迁移代码。
- 本地 `build.sh dev` 默认 `http://<project>.localhost:<实际后端端口>/`；非本地仍需配置独立 `sites.base_domain`、泛域名 DNS、TLS 和 Host 转发。无生产域名时返回 `site_domain_unconfigured`，允许上传但不宣称已发布，不从请求 Host 猜测生产域名。系统项目、无效项目 ID、只读实例等既有保护继续生效。
- 检查 `internal/api/site_dispatch.go`、`router.go`、站点 handler、CLI/技能和 `S3Manager.vue`：以合法站点 Host 和路径公开规则为准，移除实例/项目开关界面，保留路径公开/撤销。控制台/文件下载源仍强制 HTML 附件，禁止父域 Cookie；站点发布永不公开根，只允许申请 `web/`，并禁止把非站点或敏感对象放进该前缀。
- **升级前审计旧规则**：旧开关关闭时的历史公开规则不能在升级后悄悄生效。`sys_project_sites` 删除后，站点面只认 `sys_s3_public_paths`；因此升级脚本必须把「旧开关为关闭」的项目下的公开路径全部删除（这些路径此前从未真正对外可访问），只保留旧开关为开启的项目。该清理一次性执行、可重复，授权状态不明时按关闭处理。
- 发布顺序：先上传并校验对象 → 用户确认 `web/` 范围 → 写公开规则 → 从站点服务取实际 URL 并验证 → 生成交付卡。DNS/TLS 尚不可用时显示「规则已写入、访问未验证」，不报成功。验证限定配置中的站点地址，避免探测模型给出的任意 URL。

## 7. 落地顺序与验收

| 批次 | 改动位置与交付 |
|---|---|
| A | `internal/config`、`internal/api`、`internal/sbcli`、`S3Manager.vue`：默认可发布、旧配置兼容和路径授权迁移；先检查历史规则不会意外公开 |
| B | `gocloudagent/cloudagent`：阶段注册表、阶段技能映射、阶段提示词、证据判定（纯函数，无 run 生命周期） |
| C | `internal/systemdb`、`internal/api`、`internal/app`：计划表、协调器（复用现有 run/消息/审批/SSE）、查询/动作 API、启动时中断收敛 |
| D | `AgentManager.vue`、会话组件、API 类型和首页启动逻辑：输入区切换、新进度面板、首页共用入口；验证后移除 `useStarterRun` 的阶段推进和会话进度本地快照 |
| E | 文档、技能与端到端：保留历史读取，清理 `starter_stage` 等旧客户端驱动入口，做真实模型和站点访问验收 |

- 单测：用 fake runner 覆盖阶段裁剪、KV、工具成功证据、审批拒绝/过期、依赖阻塞、预算超限、取消与接手；证据缺失不得晋级。
- 集成：重复创建同一幂等键仅得到一条计划；同一计划并发推进只有一个成功（版本 CAS）；模拟副作用后、写回前崩溃；浏览器刷新能读回一致快照；服务重启后 `running` 计划变为 `interrupted` 且不自动重跑；验证只读实例、跨项目访问、系统项目及普通 run 并发拒绝。
- 站点回归：新项目默认可上传，未公开时匿名拒绝；批准后 `/console/s3` 可见且项目子域返回正确页面；撤销后不可读；旧关闭配置、旧公开规则、未配置生产域名、Vite 非默认端口和 DNS 不通均有用例。
- UI：普通对话不受影响；首页与输入区 Plan 呈现一致；刷新不重发；先后多次计划不串状态；双语文案、键盘操作和错误态通过。
- 门禁：`(cd gocloudagent && GOWORK=off go test ./...)`、`go test ./internal/...`、`cd ui && yarn typecheck && yarn test && yarn build`。端到端验收需从首页走完「规划 → 数据库 → KV 存储 → 云函数 → 定时任务 → 前端站点」并在对象存储及子域上核对真实产物；另用手动 Plan 验证同一执行路径。
