# planv6.7 · 云助手工作台：标签导航、共享会话与助手路由（AGC-*）

> **状态：** 待实现（2026-11-21 修订：补齐设计细则、前后端逐文件改动与测试清单）。本文以标签与会话为中心，替代首稿的左侧助手列表和三栏方案。
> **参考：** `thirdparty/buzz/desktop/src`（只参考交互，不引入源码、协议和依赖）。保留 [Plan 六阶段](cloud-agent-plan-mode-plan.md)、[现有控制台规范](../../ui/AGENTS.md) 和原有审批机制。
> **本次范围：** 标签导航及配置、跨助手的同一会话、每轮助手路由及身份展示；现有会话和历史可继续访问。不做多人频道、实时告警订阅、自动发送提示词或多助手并行回答。

## 0. 产品决策与阅读说明

| 位置 | 职责 |
| --- | --- |
| 左栏顶部 `[搜索…] [+]` | 在当前项目搜索标签和已可见的会话；此处 `+` 新建标签。 |
| 左栏 `#对话 [+]` | 固定入口；此处 `+` 新建一条**不绑定助手**的会话；会话列表缩进列在 `#对话` 下。 |
| 左栏 `#告警`、`#统计`、自定义标签 | 打开对应标签，进入配置好的助手、提示词和标签内会话；不引入频道或计划任务。 |
| 右侧主区 | 当前会话的消息和输入框；会话头可看到通用、数据库、对象存储、云函数、KV 等可用助手。默认通用助手选一个适合的助手回答；`@` 指定本轮回答者，**不换会话**。 |

**阅读顺序：** §1–§6 是产品与契约层（做什么、边界是什么）；§7–§10 是实现层（怎么改、改哪些文件、怎么测、怎么拆 PR）。§7/§8 中「修改前/修改后」均以 2026-11-21 的代码为基线，行号仅作定位参考。Buzz 的分组导航、点名和活动摘要参考见 §4.1；不把工具日志密度优化、原始事件或第三栏当成主流程的前置条件。

**待确认（不阻塞本文其余部分）：** ① 标签「提示词」按「点击填入输入框的快捷问题」实现；如需标签级背景说明（该标签所有对话自动附带的系统级提示），另立需求处理。② `#告警`/`#统计` 仅是预置分组，不接真实告警源或统计报表；如需接入数据另行规划。③ 标签会话只在标签页「最近会话」展示，左栏不展开二级；如需每个标签可展开再调整导航。

**本次修订对首稿的更正（以代码事实为准）：**

1. `AgentThread.StarterTemplate` 只是结构体字段，`sys_agent_threads` **没有** `starter_template` 列，首页搭建会话落库后与普通会话无法区分；控制台登录态创建会话时 `created_by = principal.APIKeyID` 为**空串**（登录态 Principal 无 APIKeyID），API Key 创建为 Key ID，定时任务为 `system-scheduler`。因此 `#对话` 分区不能靠 `created_by`/`StarterTemplate` 过滤，改为由新表 `sys_agent_thread_workspace.source` 显式标记来源（见 §2.1、§7.1）。
2. 「路由失败且通用助手不在候选中」不再报错让用户点名，改为回退到**标签配置顺序里的第一个可用助手**（`route_kind=fallback`），避免已落库的用户消息卡在失败态（见 §3.2）。
3. 不新增 `dispatching` run 状态（会牵动 `ListActiveAgentRuns`、`UpdateAgentRunStatus` 的终态判定与所有状态分支），改为 run 以 `running` 创建、`agent_id` 暂空，路由结论写入新表 `sys_agent_run_routes`，并通过新的 SSE `route` 事件推送（见 §3.3、§7.4）。

## 1. 导航与界面原型

### 1.1 桌面：左边找会话，右边与助手对话

```text
+-------------------------+----------------------------------------------------------+
| [ 搜索标签或会话... ] [+] | #对话 / 订单复盘                              [会话 ...] |
|                         | 助手：通用  数据库  对象存储  云函数  KV  [更多 v]          |
| v #对话            [+]  |----------------------------------------------------------|
|   > 订单复盘             | 你：帮我看看 orders 表结构                               |
|     发布页排查           | 通用助手 -> 数据库助手                                   |
|     缓存命中率排查       | 数据库助手：orders 包含 id、status 和 created_at。        |
|                         |                                                          |
|   #告警            [...]| 你：@对象存储助手 帮我核对报表文件                          |
|   #统计            [...]| 对象存储助手（指定）：报表文件位于 reports/ 下。            |
|   #发布检查        [...]|----------------------------------------------------------|
|                         | [ 输入问题，输入 @ 可指定一个助手                       ] |
|                         | [对话 | Plan]                    自动选择助手      [发送] |
+-------------------------+----------------------------------------------------------+
```

- 左栏一级是标签，不再显示助手列表；`#对话` 可展开或折叠，二级按最近更新降序显示会话，支持「加载更多」、会话行菜单重命名/归档。点击 `#对话` 进入该分区：优先恢复当前项目最近选中的对话会话，其次选第一条；空分区显示欢迎区与「新建会话」。进入页面或切换项目不自动创建记录。
- 两个 `+` 分别设置可见提示与可访问名称「新建标签」「新建会话」。对话旁的 `+` 展开分区、创建会话（不传标题，沿用首条消息自动命名；自动命名豁免名单仅 `New thread`/`云 Agent`/`云助手`，不引入「新会话」之类固定默认标题，否则左栏会出现一排同名条目；前端把豁免名单内的标题**显示**为本地化的「未命名会话」）、选中并聚焦输入框；请求失败保留原选中态。搜索只过滤导航，不改变选中会话；命中的会话显示所属标签。首期只搜标签和已加载的会话标题，未加载的不承诺全文搜索，保留分页入口。
- 助手条由当前会话的候选助手生成；窄栏收进「更多」。点击某助手或从 `@` 下拉选中，填入本轮点名，不发请求、不另建会话、不改变下一轮的默认行为。消息头显示实际回答者；正在执行/等待确认挂在对应会话，而不是全局切换助手。
- 移动端保持同一结构：主区头部 `[会话/标签]` 打开左侧 `Sheet`，选择后关闭并聚焦会话头；输入框、`@` 候选及「发送/停止」留在主区。用 `DefaultLayout` 的可用区做布局，消息列表独立滚动，不改全局导航。无常驻第三栏。

### 1.2 标签页与标签编辑

```text
+-------------------------+-----------------------------------------------+
| [ 搜索... ]        [+]  | #统计                                  [编辑] |
| v #对话            [+]  | 助手：[数据库] [KV] [通用]                     |
|     订单复盘             | 提示词：[查看昨日订单] [汇总缓存命中率]         |
|   #告警                 |                                               |
| > #统计                 | 最近会话：                                     |
|   #发布检查             |   昨日订单分析              [继续]            |
|                         |   [新建标签会话]                               |
+-------------------------+-----------------------------------------------+

+-------------------------- 编辑 #统计 --------------------------+
| 名称    [ 统计                                              ] |
| 助手    [数据库 x] [KV x] [通用 x] [选择项目内助手 v]          |
| 提示词  [查看昨日订单] [请按状态汇总昨日订单……]      [删除]   |
|         [+ 添加提示词]                                        |
|                                         [取消] [保存]        |
+----------------------------------------------------------------+
```

- `#告警`、`#统计` 是产品预置标签，不等于后台告警源、统计报表或订阅。名称/路由键固定（名称由前端按 `builtin_key` 本地化），可编辑助手和提示词；自定义标签可新建、改名、归档。`#对话` 不进入标签编辑器。
- 自定义标签由名称、多个助手、多个有序提示词组成，可只配助手或只配提示词。助手、提示词是标签页内容，不在左栏伪装成会话。点「新建标签会话」后所开会话归本标签，右侧仍使用同一套对话视图；标签页列出本标签最近会话，`#对话` 树不重复展示。提示词填入当前标签会话的输入框；如果尚无会话，则打开该标签的新会话草稿，实际发送前才创建并选中会话。已有非空草稿时先确认替换，绝不自动发送。提示词不是系统指令，不参与提高权限。
- `#对话` 可用全项目符合条件的助手；标签会话的自动路由和 `@` 候选限于标签配置。没有配置助手时使用通用助手；已配置但全部不可用时提示编辑标签或回到 `#对话`，不静默跳到标签之外。标签配置变化只影响下一轮，不改历史和已开始的 run。

## 2. 标签和会话的数据契约（AGC-NAV）

### 2.1 存储与默认标签

- **标签按项目共享，会话继续沿用现有访问范围。** 不把标签混进 `CloudAgent.module`，也不使用浏览器存储作为标签配置的权威来源。新增三张系统表（DDL 见 §7.1），全部经 `schema.go` 的 `systemTables` 增量 `CREATE TABLE IF NOT EXISTS`，不修改既有表、不要求 reset：
  - `sys_agent_workspace_tags`：标签本体与配置（`id`, `project_id`, `builtin_key`, `name`, `config_json`, `version`, `created_by`, 时间戳与 `archived_at`）。
  - `sys_agent_thread_workspace`：会话的工作台元数据（`project_id`, `thread_id`, `tag_id`, `dispatch_mode`, `source`, `created_at`）。每条会话最多一行，只在**创建会话的同一事务**里插入，之后只有「归档标签」会把 `tag_id` 清空；系统库没有外键/唯一约束兜底，唯一性靠「仅在建会话事务内插入」这一条写入路径保证，并由测试收口。
  - `sys_agent_run_routes`：每个 run 的路由结论（`project_id`, `run_id`, `thread_id`, `agent_id`, `route_kind`, `router_ms`, `created_at`），见 §3.3。
- `config_json` 由 Go 类型 `AgentTagConfig` 约束，只存有序 `agent_ids` 和 `prompts: [{id, title, content}]`；提示词不是密钥配置。首期每条会话最多属于一个标签，归属在创建后不移动，标签不是私密空间或授权边界。
- **预置标签：** ID 固定为 `alerts`、`stats`（同时是 `builtin_key`），自定义标签 ID 为服务端生成的 UUID，二者不会冲突；`chat`（`#对话`）是虚拟分区，不入表。只读列表把「内置默认 + 项目覆盖行」合并返回，**无写入副作用**；首次保存内置标签时才插入覆盖行（`version` 从 1 起）。内置默认配置以 `builtin_key` 引用助手，读取时解析为本项目助手 ID：

  | 标签 | 默认助手（按顺序，缺失的跳过） | 默认提示词 |
  | --- | --- | --- |
  | `alerts` | `logs`、`general` | 「最近 1 小时有哪些错误日志」「按级别统计今天的日志」 |
  | `stats` | `database`、`kv`、`general` | 「统计各表的行数」「最近 7 天新增了多少数据」 |

- 自定义标签拒绝空白、重名（同项目未归档标签名，大小写不敏感）和保留名称（`对话`/`告警`/`统计`/`chat`/`alerts`/`stats`）。导航一律用 ID，不用显示名。
- **归档自定义标签：** 「标签 `archived_at`」与「相关会话元数据 `tag_id` 清空」在同一个 `BeginTx` 事务内提交（单一仓库方法 `ArchiveAgentTag` 收口，套 `ducklake.RetryOnConflict`），**保留 `dispatch_mode=auto`**。原会话回到 `#对话`，不删除消息、run 或审批；活跃 run 继续使用原选定助手。原 `?thread=` 深链按会话实际归属定位。预置标签不可归档（返回 400 `agent_tag_builtin`）。
- **会话来源 `source`：** `workspace`（新工作台创建）、`starter`（首页一句话建应用）、`bi`（数据看板生成图表）。旧会话没有元数据行，视为 `legacy`。`#对话` 分区收录规则见 §2.2。
- 标签编辑器只列当前项目助手。保存时校验名称（1–32 字符）、助手 ID 归属、去重、提示词长度和数量：每项目最多 50 个未归档自定义标签、每标签最多 20 个助手及 20 条提示词、提示词标题 1–40 字符、正文 1–4,000 字符。助手归档后保留历史消息，配置里显示「助手已不可用」，下一轮实时剔除；允许从编辑器移除失效项，不自动替换成同名助手。更新用 `version` 比较（`expected_version`），冲突返回 409 `agent_tag_conflict` 并保留前端草稿；不记录提示词正文到日志或审计。

### 2.2 API、权限与兼容

以下路径基于 `/v1/projects/:projectID`，沿用项目上下文、权限和错误映射：

| 接口 | 改动 |
| --- | --- |
| `GET /agent-tags` | `require(database:read)`；返回预置与自定义标签、配置及版本；不含 `chat` 虚拟分区。响应 `{"tags":[agentTagDTO]}`，按「预置（alerts、stats）→ 自定义（created_at 升序）」排序。 |
| `POST /agent-tags` | `require(database:write)` + `writeGate`；body `{name, agent_ids, prompts}`；201 返回 `agentTagDTO`。 |
| `PATCH /agent-tags/:tagID` | 同上；body `{expected_version, name?, agent_ids?, prompts?}`；预置标签忽略 `name`（传了且不同则 400）。 |
| `DELETE /agent-tags/:tagID` | 同上；归档自定义标签；预置标签 400 `agent_tag_builtin`；204。 |
| `GET /agent-threads` | 增量支持 `tag_id=chat` 与 `tag_id=<标签 ID>`。`chat` 分区 = 「没有元数据行或 `tag_id` 为空」**且** `source` 不为 `bi` **且** `created_by = ''`（控制台登录态创建；API Key 创建的会话 `created_by` 为 Key ID、定时任务为 `system-scheduler`，均不混入）。不带 `tag_id` 时保持原语义；`agent_id` 旧过滤继续支持，两者同时传取交集。筛选先于 keyset 分页，游标仍校验项目归属。 |
| `POST /agent-threads` 与会话 DTO | body 增量支持 `tag_id`、`dispatch_mode`、`source`。新工作台显式传 `dispatch_mode=auto`（`#对话` 不传 `tag_id`，标签会话传真实标签 ID）；首页模板传 `source=starter`，数据看板传 `source=bi`。规则：提供 `tag_id` 必须 `dispatch_mode=auto`；`auto` 不允许同时设置 `agent_id`；`source` 仅接受上述枚举；三者都不传时**不写元数据行**（旧请求、SDK 完全不变）。列表与详情 DTO 返回可选 `tag_id`、`dispatch_mode`。 |
| `GET /agent-threads/:threadID/messages` | `messageDTO` 增量返回可选 `route_kind`（来自 `sys_agent_run_routes`，仅 assistant 消息）。 |
| `POST /agent-threads/:threadID/runs` | 请求体不变；`auto` 会话按 §3 路由。SSE 首个 `run` 事件增加可选 `agent_id`/`route_kind`，新增 `route` 事件；非流式响应增加 `route_kind`。 |

**旧会话不批量解绑。** 原 `agent_id` 绑定会话（含旧首页模板、旧 BI 会话）若满足 `chat` 规则就出现在 `#对话`，显示「固定：助手名」，可照常继续、重命名和归档，服务端保留 `agent_switch_required`；如需换助手，提供「新建自动路由会话」入口，不复制或丢弃历史。首页模板、定时任务、BI 专用入口和旧 SDK 不改默认创建口径（模板与 BI 仍绑定助手，只多带 `source`）。删除会话走现有软删，元数据随会话不可见；删除项目按现有系统库清理路径收口新增三表。服务端只读、系统项目和越权用例必须覆盖实际路由，不依赖前端按钮禁用。

## 3. 同一会话内的助手路由（AGC-ROUTE）

### 3.1 与现有行为的差异

`AgentManager.vue` 当前先选助手再按 `agent_id` 查会话（`loadThreads` 空列表时还会**自动建会话**）；`CreateRun` 发现 `thread.AgentID` 与 `@` 不同会返回 409 `agent_switch_required`；未绑定、未点名时 `resolveMentionedAgent` 只返回通用助手，没有自动分派。新逻辑只对 `dispatch_mode=auto` 的会话生效，不放宽旧会话或既有鉴权。

```text
用户发送
  -> 服务端加载会话、工作台元数据与标签，生成本轮候选助手（按能力过滤）
  -> 重试? ---------- 是 -> 原 run 的助手（retry）
  -> 有有效 @ ? ----- 是 -> 指定助手（mention）
  -> 候选只有一个? -- 是 -> 直接使用（auto，不调模型）
       | 否
       v
     ClaimThread -> 建 run(agent_id 暂空) -> 落用户消息 -> SSE: run + route{routing}
     通用路由器（只分类，不执行工具，限时 8s）
       | 合法 ID           -> 该助手（auto）
       | none/超时/越界/失败 -> 通用助手；通用不在候选则取候选第一个（fallback）
  -> 回填 run.agent_id，写 sys_agent_run_routes，SSE: route{agent_id, route_kind}
  -> 对选中的助手按现有能力、权限、审批规则执行
```

### 3.2 选择规则：重试优先，其次显式点名，再自动分派

| 情形 | 服务端行为 |
| --- | --- |
| 失败/停止后重试 | 沿用现有校验（同会话、最新、failed/canceled），复用原用户消息与**原 run 的 `agent_id`**；不重新分类、不重复插入用户消息。原助手已归档或不在当前候选中时返回 409 `retry_agent_unavailable`，前端提示另发新一轮。 |
| 有结构化 `mentions` | 使用 `agent_id` 而非显示名；一个 run 只允许一个不同 ID（沿用 `multiple_agents_not_supported`）。验证项目和候选范围：已归档/跨项目 400 `mentioned_agent_not_found`，存在但不在标签候选 400 `agent_not_in_tag`，能力不可用沿用 `cli_skills_unavailable`/`sandbox_unavailable`。不隐式改发给通用助手。仅输入 `@` 文本但未从候选选中、或名称有歧义时，前端不生成 mention，提示从候选选定。 |
| 无 `@`，自动路由会话 | 候选只有一个直接使用；否则调用路由器，按当前问题、最近 3 轮同会话上下文摘要和候选的名称/模块/简介分类。表结构与数据查询倾向数据库；文件、路径倾向对象存储；函数定义与运行记录倾向云函数；键名、TTL 倾向 KV；日志与错误倾向日志助手。追问可参考上轮助手但不锁定；多领域、通用问题或低置信度返回 `none`。不按简单关键词强制选人。 |
| 路由不可用、结果越界 | 超时、解析失败、候选外 ID、上游错误、`none` 一律 `fallback`：通用助手在候选中用通用，否则用候选顺序第一个；不报错、不保留失败的用户消息。请求取消（用户按停止）直接把 run 置 `canceled` 并结束，不执行。 |
| 候选为空 | 在 `ClaimThread` 之前返回 409 `no_available_agent`（不落任何记录），前端提示编辑标签或回到 `#对话`。 |
| 旧模式会话（无元数据或 `dispatch_mode` 为空） | 完全沿用现有 `CreateRun` 分支，包括 `agent_switch_required`。 |

### 3.3 路由器、持久化与事件

- 路由是宿主侧独立的窄接口：`internal/api/agent_dispatch.go` 定义 `agentDispatcher` 接口与基于 `cloudagent.ChatClient`（即 `h.runtime.LLM`）的实现，只返回候选中的一个 ID 或 `none`。限制：调用 8s 超时、`MaxTokens=64`、`Temperature=0`、不传 `Tools`、用户正文截断到 2,000 字符、上下文最多 3 轮且每轮截断 200 字符、候选简介截断 200 字符。不加载工具、不创建额外 run、不执行提示词中的指令。日志与审计只记录 `route_kind`、选中 ID、候选数和耗时，不记录用户正文或路由提示词。
- 最终仍交给现有 `cloudagent.Runtime` 权限、只读限权和工具审批，任何候选配置均不授予能力。自动选择可写助手不等于批准写入；鉴权对账和审批测试作为发布门槛。
- **持久化：** run 以 `running` 状态、`agent_id=''` 创建（不新增 `dispatching` 状态，避免改动 `ListActiveAgentRuns` 与终态判定）；路由完成后 `UpdateAgentRunAgent` 回填 `agent_id`，并 `InsertAgentRunRoute` 写入 `sys_agent_run_routes`。重试、点名、单候选同样写一行路由记录（`route_kind` 分别为 `retry`/`mention`/`auto`），因此刷新后能恢复「自动/指定/回退」原因。用户消息 `agent_id` 写空串、`mentions_json` 写真实 mentions（自动路由时为 `[]`）——现有「未点名默认写通用助手」的兜底只保留给旧模式会话。
- **事件协议（向后兼容）：** 首个 `run` 事件对已确定助手的情形直接带 `agent_id`/`route_kind`；需要路由时先发 `{"type":"route","status":"routing"}`，路由结束发 `{"type":"route","status":"done","agent_id":"…","route_kind":"auto|fallback"}`。旧客户端忽略未知事件类型。`cloudagent.Event` 增加 `AgentID`、`RouteKind`、`Status` 三个 `omitempty` 字段（模块 DTO 增量，不破坏 CLI）。非流式响应体增加 `route_kind`，已有 `agent_id` 字段保留。
- **时机与开销：** 自动路由插在配额检查与 `ClaimThread` 之后（否则同一会话连发两条会各自选人、并发执行）；SSE 在路由前已建立，前端立即显示「正在选择助手」。路由模型调用经同一 LLM 网关，计入项目 `llm` 用量；run 前的 `CheckQuota` 已覆盖本轮，不设豁免通道，也不额外二次检查。路由 ctx 登记到 `h.routeCancels`（`runID → cancel`），`CancelRun` 同时调用它，保证路由期间按「停止」能立即生效。
- **跨助手历史投影：** 现有历史 DTO 只有 `role/content/tool_calls`（`cloudagent.Message`），不带助手身份；直接透传会让数据库助手看到对象存储助手调用 `list_objects` 的记录，进而误判或模仿不在自己工具集内的调用。改在 `agentbridge` 转换层投影：`agent_id` 非空且不等于本轮助手的 assistant 消息，正文前缀「[助手名]」，工具调用压成一句摘要（如「[对象存储助手] 调用了 list_objects、head_object」）并把 `ToolCallsJSON` 置为 `[]`；本轮助手和 `agent_id` 为空的历史消息保持原样。只改投影，不改存储。
- **Plan：** 进入 Plan 不复用聊天分类器，也不隐式把问题转成 Plan。首页模板和旧 Plan 行为不动；`auto` 会话的 Plan 在请求未带 `agent_id` 时显式交给通用助手（要求通用助手在候选中，否则 409 `plan_unavailable`），仍用同一条会话。标签会话仅在本标签允许通用助手且现有 Plan 能力可用时显示开关。Plan 的阶段和审批继续由服务端状态驱动。

## 4. 前端设计与 Buzz 对照（AGC-UI）

### 4.1 组件树与职责

```text
AgentManager.vue（编排：projectId + section + threadId，URL ?tag= ?thread= ?starter=）
├─ AgentWorkspaceSidebar.vue（新）   左栏：搜索 + [+新建标签] + #对话树 + 标签列表
│    └─ 移动端由 Sheet 承载同一组件
├─ 主区 Card
│   ├─ 会话头：面包屑「#对话 / 标题」+ 回答者/运行态 + AgentRosterBar.vue（新）+ 「更多」菜单
│   ├─ AgentTagOverview.vue（新）    选中标签且未打开会话时显示：助手、提示词、最近会话
│   ├─ ConversationView.vue（改）    消息头显示实际回答者 + 路由原因徽标
│   ├─ PlanProgress.vue（不变）
│   └─ AgentComposer.vue（改）       候选限于本会话；「自动选择助手 / 本轮：@X」提示
├─ AgentTagModal.vue（新，components/modal/）  标签新建/编辑
├─ AgentFormModal.vue（不变）        助手管理移入「更多 → 管理助手」
└─ SbModal（重命名，沿用）
```

### 4.2 页面状态模型（`AgentManager.vue`）

| 状态 | 类型 | 说明 |
| --- | --- | --- |
| `section` | `ref<string>` | `'chat'` 或标签 ID；取代 `activeId` 作为左栏选中态。持久化到 `localStorage['sb.agent.section.<projectId>']` 只作恢复提示，不作权威。 |
| `chatThreads` / `chatCursor` | `ref<AgentThread[]>` / `ref<string>` | `#对话` 分区分页列表（`tag_id=chat`）。 |
| `tagThreads` | `ref<Record<string, AgentThread[]>>` | 标签最近会话（每标签首屏 20 条，按需加载）。 |
| `tags` | `ref<AgentTag[]>` | `GET /agent-tags` 结果。 |
| `threadId` | `ref<string>` | 当前会话；空表示「新会话草稿」或标签概览。 |
| `pendingDraft` | `ref<{ section: string } \| null>` | 标签提示词触发的「尚未创建的会话草稿」，发送时才创建。 |
| `activeThread` | `computed` | 在 `chatThreads ∪ tagThreads[*]` 中查找；找不到时用 `GET /agent-threads/:id` 补齐（深链）。 |
| `candidateAgents` | `computed` | auto 会话：`#对话` 为全部可用助手、标签会话为标签 `agent_ids` ∩ 可用助手；旧绑定会话为 `[绑定助手]`。供助手条与 `@` 候选共用。 |
| `drafts` | `Map<string, string>` | key = `${projectId}:${threadId \|\| 'new:'+section}`，切会话保存/恢复草稿；发送成功后清除。 |
| `lastSpeaker` | `computed` | 最近一条 assistant 消息的 `agentId`，用于会话头「当前回答者」。 |

删除：`activeId`、`switchTargetId`、`switchTarget`、`selectAgent`、`switchToMentionedAgent`、`refreshThreadsForAgent`、`runningAgentIds`、以及 `loadThreads` 中「空列表自动建会话」分支。

### 4.3 交互细则

- **新建会话（`#对话` 旁 `+`）：** `api.agentThreads.create(pid, { dispatchMode: 'auto' })` → 插入 `chatThreads` 头部 → 选中 → `composer.focus()`。失败 toast 并保持原选中。
- **新建标签会话：** 标签概览「新建标签会话」或点提示词时不立即建会话，置 `pendingDraft={section: tagId}`、`threadId=''`；`onSend` 发现 `pendingDraft` 时先 `create(pid, { tagId, dispatchMode: 'auto' })`，成功后再发 run；创建失败保留草稿。
- **点提示词：** 若当前草稿非空，`ConfirmAction` 确认替换；否则直接 `draft = prompt.content` 并聚焦；**绝不自动发送**。
- **助手条/`@`：** 点击助手条中的助手 = 在草稿开头插入 `@名称 ` 并登记结构化 mention（复用 `AiChatComposer` 的 `pickMention`，需 `defineExpose({ insertMention })`）；再次点击已选中助手取消。
- **发送：** auto 会话 mentions 为空时直接发送（不再塞 `activeAgent`）；旧绑定会话保持原逻辑。占位气泡 `module` 先为空并显示「正在选择助手…」，收到 `route.done` 或带 `agent_id` 的 `run` 事件后填 `agentId`/`module`/`routeKind`。
- **运行态：** `/agent-runs/active` 轮询保留，改为只驱动「会话行运行中标记」（`runningThreadIds`）；会话头显示「执行者：X」与停止按钮。
- **错误区分：** `no_available_agent`、`agent_not_in_tag`、`retry_agent_unavailable` 显示可操作提示（编辑标签/回到 `#对话`/重新发送），模型失败沿用 `runFailed`，审批待处理走确认卡。
- **深链：** `?thread=` 优先；拿到会话后按 `tag_id` 推导 `section`（与 `?tag=` 冲突以会话为准）；会话不存在显示 `SbEmptyState`「会话不存在或已归档」，不自动新建。`?starter=` 流程保持，仅建会话时多传 `source: 'starter'`。

### 4.4 Buzz 源码参考（路径相对于 `thirdparty/buzz/desktop/src`）

| 源码 | 可借鉴的界面机制 | 不照搬 |
| --- | --- | --- |
| `features/sidebar/ui/AppSidebar.tsx`、`CustomChannelSection.tsx`、`ChannelSectionDialogs.tsx` | 顶层编排创建分组；分组自己渲染折叠状态、选中态和二级条目。对应 `#对话` 与它下面的会话。 | Buzz 的分组收纳频道；SimpleBase 的标签配置助手和提示词，不照搬频道归属、拖放和社区协议。 |
| `features/messages/ui/MessageComposer.tsx`、`MentionAutocomplete.tsx` | 点名候选与输入文本分离，以确定的实体 ID 表达选择。 | 点名在这里覆盖当前轮路由，不新增发言对象和频道成员关系。 |
| `features/channels/ui/ChannelComposerActivityAccessory.tsx`、`BotActivityBar.tsx` | 让运行状态靠近输入框，详情按需查看。 | 不引入轮播、跨频道状态或常驻第三栏。 |
| `features/agents/ui/AgentSessionTranscriptList.tsx`、`activityRenderClasses/ActivityRow.tsx`、`agentSessionTranscriptGrouping.ts` | 回合分组和紧凑的可折叠活动行能保留工具细节，适合作为后续优化。 | 不照搬 ACP 会话边界、原始事件面板或 React 动画依赖。 |

## 5. 分期与验收（AGC-TEST）

| 分期 | 可交付结果 |
| --- | --- |
| P0 · 导航、标签与会话内派发 | 两个 `+`、`#对话` 二级会话、`#告警`/`#统计` 与自定义标签、配置持久化；新建共享会话的一轮一助手、可见回答者、自动分派及 `@`、重试与断线续接。按 §10 的 PR 顺序拆分：后端（表、API、路由）可先行合并，前端导航与会话视图随后；合入界面不得出现未接通的「自动选择助手」控件。两者作为一个版本整体验收上线——只做导航时新建会话无法自动选人，没有独立交付价值。可通过兼容层读旧会话。 |
| P1 · 可选体验优化 | 按 UX 验收反馈决定是否压缩同类活动行、增加回合摘要、扩展会话搜索；不是本次上线条件。 |

1. **端到端导航：** 创建带两个助手及两个提示词的 `#发布检查`，刷新后配置存在；点顶部 `+` 只开标签表单，点 `#对话` 旁 `+` 只建会话；搜索过滤不误切会话。点击提示词只填草稿。标签归档后会话回到 `#对话` 且历史仍在。
2. **同一会话：** 同一 `thread_id` 中先问订单表自动命中数据库助手，再 `@对象存储助手` 核对文件，再问 KV 的 TTL；每轮仅一个 run，答复各带正确身份，左侧只新增那一个会话；刷新后回答者、路由原因、历史和深链仍正确。
3. **边界：** 标签内只看到配置的助手；直接发 API 也不能点名范围外助手。覆盖跨项目 ID、重复提交、版本冲突、全部候选不可用、分类器超时/越界、无通用候选的回退、旧绑定冲突、失败重试固定原助手、路由期间停止。已有审批、只读能力、停止与续跑不得退化。
4. **组件与移动端：** 真实控件测试两个加号的名称和行为、选中状态、键盘点名、中文输入法 Enter、抽屉焦点返回；切项目后迟到的旧列表、旧 SSE 不得混入新项目。320px 窄屏仍可选标签、看到回答者、发送及停止，无横向滚动。
5. **实施验证：** `go test ./internal/api ./internal/systemdb ./internal/agentbridge ./internal/app`；`(cd gocloudagent && GOWORK=off go test ./...)`；`(cd ui && yarn typecheck && yarn test && yarn build)`。后端测试用 fake 路由器，不依赖真实模型或外网；前端验收不桩 `src/components/ui/` 交互组件。

## 6. 发布与回退

- 先确保新表可增量初始化，再发布依赖它们的接口与 UI；系统库为单写实例，不存在旧写节点把自动路由会话当普通会话的滚动更新问题。准备回退时先停止新模式入口再回退，不清理新增表、不删除历史会话或消息；回退后的旧版本把 auto 会话当「未绑定会话」处理（`agent_id` 为空 → 通用助手），可继续访问。
- 旧客户端继续调用原 API；新端遇旧后端（`GET /agent-tags` 404）时降级为旧版助手列表模式，不发送新模式请求。
- 阶段出口：P0 的数据与交互门槛全过再对用户放开；仅依靠模型关键词演示、两个入口共用 `+`、`@` 自动换会话或标签配置可绕权，均不视为验收通过。

## 7. 后端实现（逐文件改动）

> 依赖方向不变：`api → systemdb / agentbridge → gocloudagent`。所有新写路由挂 `writeGate` + `require(auth.DatabaseWrite)`，新增领域错误加入 `api/error.go` 的 `sentinelErrorMappings`，不得裸 `errors.New` 进 `WriteError`。

### 7.1 `internal/systemdb/schema.go`：新增三张表

修改前：`systemTables` 中 agent 相关只有 `sys_cloud_agents`、`sys_agent_threads`、`sys_agent_plans`、`sys_agent_messages`、`sys_agent_runs`、`sys_agent_approvals` 等。

修改后：在 `sys_agent_approvals` 之后追加（全部 `IF NOT EXISTS`，DuckLake 无 PK/UNIQUE，唯一性由应用层保证）：

```go
// planv6.7 AGC-NAV：工作台标签（预置 alerts/stats 的覆盖行 + 自定义标签）。
// 唯一性由应用层保证：(project_id, id) 至多一行；同项目未归档 name 不重复。
{name: "sys_agent_workspace_tags", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_workspace_tags (
		id VARCHAR NOT NULL,
		project_id VARCHAR NOT NULL,
		builtin_key VARCHAR NOT NULL DEFAULT '',
		name VARCHAR NOT NULL,
		config_json VARCHAR NOT NULL,
		version BIGINT NOT NULL DEFAULT 1,
		created_by VARCHAR NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		archived_at TIMESTAMP
	)`},
// planv6.7 AGC-NAV：会话的工作台元数据。只在建会话事务内插入，(project_id, thread_id) 至多一行。
{name: "sys_agent_thread_workspace", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_thread_workspace (
		project_id VARCHAR NOT NULL,
		thread_id VARCHAR NOT NULL,
		tag_id VARCHAR NOT NULL DEFAULT '',
		dispatch_mode VARCHAR NOT NULL DEFAULT '',
		source VARCHAR NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL
	)`},
// planv6.7 AGC-ROUTE：每个 run 的路由结论（auto/mention/fallback/retry），供刷新后恢复回答原因。
{name: "sys_agent_run_routes", ddl: `CREATE TABLE IF NOT EXISTS sys_agent_run_routes (
		project_id VARCHAR NOT NULL,
		run_id VARCHAR NOT NULL,
		thread_id VARCHAR NOT NULL,
		agent_id VARCHAR NOT NULL,
		route_kind VARCHAR NOT NULL,
		router_ms BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL
	)`},
```

同步：若 `systemdb` 有「系统表清单」相关测试（`schema_test.go` 断言表数量/名称），一并更新；项目删除的系统库清理路径若按表名枚举，加入三张新表。

### 7.2 `internal/systemdb/agent_tags.go`（新文件）

```go
package systemdb

const (
	AgentTagChat    = "chat"   // 虚拟分区，不入表
	AgentTagAlerts  = "alerts" // 预置标签 ID = builtin_key
	AgentTagStats   = "stats"
	DispatchAuto    = "auto"
	ThreadSourceWorkspace = "workspace"
	ThreadSourceStarter   = "starter"
	ThreadSourceBI        = "bi"
	RouteAuto, RouteMention, RouteFallback, RouteRetry = "auto", "mention", "fallback", "retry"
)

var (
	ErrAgentTagNotFound = errors.New("systemdb: agent tag not found")
	ErrAgentTagConflict = errors.New("systemdb: agent tag version conflict")
	ErrAgentTagDuplicate = errors.New("systemdb: agent tag name already exists")
	ErrAgentTagBuiltin  = errors.New("systemdb: builtin agent tag cannot be archived")
	ErrAgentTagLimit    = errors.New("systemdb: agent tag limit exceeded")
)

type AgentTagPrompt struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// AgentTagConfig 是 config_json 的唯一形状；不得存密钥。
type AgentTagConfig struct {
	AgentIDs []string         `json:"agent_ids"`
	Prompts  []AgentTagPrompt `json:"prompts"`
}

type AgentTag struct {
	ID, ProjectID, BuiltinKey, Name, CreatedBy string
	Config    AgentTagConfig
	Version   int64
	Persisted bool // false 表示内置默认、尚未写覆盖行
	CreatedAt, UpdatedAt time.Time
}

// AgentThreadWorkspace 是 sys_agent_thread_workspace 一行；旧会话无此行。
type AgentThreadWorkspace struct {
	ProjectID, ThreadID, TagID, DispatchMode, Source string
}

type AgentRunRoute struct {
	ProjectID, RunID, ThreadID, AgentID, RouteKind string
	RouterMS int64
}
```

仓库方法（均先判 `s == nil || s.db == nil → ErrUnavailable`，写后 `s.notifyWrite(ctx)`）：

| 方法 | 实现要点 |
| --- | --- |
| `ListAgentTagRows(ctx, projectID) ([]AgentTag, error)` | 只读未归档行（含预置覆盖行），`ORDER BY created_at, id`。内置默认合并放在 api 层（需要 builtin_key→助手 ID 解析）。 |
| `GetAgentTagRow(ctx, projectID, id)` | 未归档；无行返回 `sql.ErrNoRows`（预置标签无覆盖行时由 api 层回落默认）。 |
| `CreateAgentTag(ctx, t AgentTag)` | `lockAgentTag(projectID)`（复用 `lockAgentSeed` 同款 per-project flight 锁，单独一个 map 字段 `tagFlights`）；锁内 `COUNT` 未归档自定义标签 ≥50 → `ErrAgentTagLimit`；`lower(name)` 查重 → `ErrAgentTagDuplicate`；生成 UUID 插入，`version=1`。 |
| `UpsertBuiltinAgentTag(ctx, projectID, key, cfg, expectedVersion)` | 锁内查覆盖行：无行且 `expectedVersion==0` → 插入 `version=1`；有行且版本相等 → `UPDATE ... SET version=version+1`；否则 `ErrAgentTagConflict`。 |
| `UpdateAgentTag(ctx, projectID, id, name, cfg, expectedVersion)` | 锁内：`UPDATE ... WHERE id=? AND project_id=? AND archived_at IS NULL AND version=?`；`RowsAffected==0` 时再查是否存在区分 `ErrAgentTagNotFound`/`ErrAgentTagConflict`；改名需查重。 |
| `ArchiveAgentTag(ctx, projectID, id)` | 预置 ID → `ErrAgentTagBuiltin`。`ducklake.RetryOnConflict` + `BeginTx`：① `UPDATE sys_agent_workspace_tags SET archived_at=?` ② `UPDATE sys_agent_thread_workspace SET tag_id='' WHERE project_id=? AND tag_id=?`（不动 `dispatch_mode`）③ `Commit`。 |
| `CreateAgentThreadWithWorkspace(ctx, t AgentThread, ws AgentThreadWorkspace)` | 新方法，`BeginTx` 内插入 `sys_agent_threads` 与 `sys_agent_thread_workspace`；`ws` 为零值时等价于原 `CreateAgentThread`（不写元数据行）。原 `CreateAgentThread` 保留并改为调用它（`ws` 零值），定时任务、测试不受影响。 |
| `GetAgentThreadWorkspace(ctx, projectID, threadID)` | 无行返回零值 + `nil`（旧会话）。 |
| `ListAgentThreadWorkspaces(ctx, projectID, threadIDs []string) (map[string]AgentThreadWorkspace, error)` | 列表 DTO 批量补字段，避免 N+1；`IN (?, ?, …)` 动态占位。 |
| `UpdateAgentRunAgent(ctx, projectID, runID, agentID)` | `UPDATE sys_agent_runs SET agent_id=? WHERE id=? AND project_id=? AND agent_id=''`（只允许从空回填一次）。 |
| `InsertAgentRunRoute(ctx, r AgentRunRoute)` | 插入一行；同 run 已有行则跳过（先查后插）。 |
| `ListAgentRunRoutes(ctx, projectID, threadID) (map[string]AgentRunRoute, error)` | `runID → route`，供 `ListMessages` 补 `route_kind`。 |

`internal/systemdb/store.go`：`Store` 增加 `tagMu sync.Mutex`、`tagFlights map[string]chan struct{}`（与 `seedMu/seedFlights` 同构）。

### 7.3 `internal/systemdb/agents.go`：会话分区查询

修改前（`PageAgentThreads`）：

```go
func (s *Store) PageAgentThreads(ctx context.Context, projectID, agentID, cursor string, limit int) ([]AgentThread, string, error) {
	query := `SELECT id, project_id, title, COALESCE(agent_id, ''), created_by, created_at, updated_at FROM sys_agent_threads WHERE project_id=? AND archived_at IS NULL`
	args := []any{projectID}
	if agentID != "" { query += ` AND COALESCE(agent_id, '') = ?`; args = append(args, agentID) }
	...
```

修改后：签名改为接收过滤结构体（保留旧函数作薄包装，避免改动全部调用方）：

```go
// AgentThreadFilter 是会话分页过滤条件；零值等价于旧行为。
type AgentThreadFilter struct {
	AgentID string
	// TagID 为 "chat" 表示 #对话 分区；其他非空值表示精确标签分区。
	TagID string
}

func (s *Store) PageAgentThreads(ctx context.Context, projectID, agentID, cursor string, limit int) ([]AgentThread, string, error) {
	return s.PageAgentThreadsFiltered(ctx, projectID, AgentThreadFilter{AgentID: agentID}, cursor, limit)
}

func (s *Store) PageAgentThreadsFiltered(ctx context.Context, projectID string, f AgentThreadFilter, cursor string, limit int) ([]AgentThread, string, error) {
	query := `SELECT t.id, t.project_id, t.title, COALESCE(t.agent_id, ''), t.created_by, t.created_at, t.updated_at
		FROM sys_agent_threads t
		LEFT JOIN sys_agent_thread_workspace w ON w.project_id = t.project_id AND w.thread_id = t.id
		WHERE t.project_id=? AND t.archived_at IS NULL`
	args := []any{projectID}
	if f.AgentID != "" { query += ` AND COALESCE(t.agent_id, '') = ?`; args = append(args, f.AgentID) }
	switch f.TagID {
	case "":
	case AgentTagChat:
		// #对话：无标签归属、非 BI 来源、且由控制台登录态创建（created_by 为空）。
		query += ` AND COALESCE(w.tag_id, '') = '' AND COALESCE(w.source, '') <> 'bi' AND t.created_by = ''`
	default:
		query += ` AND w.tag_id = ?`
		args = append(args, f.TagID)
	}
	// 游标与排序逻辑不变，列名加 t. 前缀。
	...
}
```

`AppendAgentMessage` 不改；`ListAgentMessages` 不改（`route_kind` 在 api 层合并）。

### 7.4 `gocloudagent/cloudagent/run_types.go`：事件字段

修改前：`Event` 无助手身份字段。

修改后（模块内增量字段，`omitempty` 保证 CLI 与旧消费端不变）：

```go
type Event struct {
	...
	Approve          *bool    `json:"approve,omitempty"`
	// AgentID / RouteKind / Status 仅宿主路由事件使用（planv6.7 AGC-ROUTE）：
	// run 事件可带实际回答助手；route 事件 Status 为 routing / done。
	AgentID   string `json:"agent_id,omitempty"`
	RouteKind string `json:"route_kind,omitempty"`
	Status    string `json:"status,omitempty"`
}
```

### 7.5 `internal/agentbridge/principal.go`：跨助手历史投影

修改前：

```go
func Messages(msgs []systemdb.AgentMessage) []cloudagent.Message { ... out = append(out, Message(m)) ... }
func Message(m systemdb.AgentMessage) cloudagent.Message {
	return cloudagent.Message{Role: m.Role, Content: m.Content, ToolCallsJSON: m.ToolCallsJSON}
}
```

修改后：保留 `Messages`（旧路径语义不变），新增投影函数：

```go
// MessagesFor 把共享会话历史投影给本轮助手（planv6.7 AGC-ROUTE）：
// 其他助手的 assistant 消息加「[助手名]」前缀，工具调用压成一句摘要并清空 ToolCallsJSON，
// 避免本轮助手模仿不在自己工具集内的调用。names 为 agentID → 显示名。
func MessagesFor(msgs []systemdb.AgentMessage, currentAgentID string, names map[string]string) []cloudagent.Message {
	out := make([]cloudagent.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role != "assistant" || m.AgentID == "" || m.AgentID == currentAgentID {
			out = append(out, Message(m))
			continue
		}
		name := names[m.AgentID]
		if name == "" { name = "其他助手" }
		content := "[" + name + "] " + m.Content
		if tools := toolNames(m.ToolCallsJSON); len(tools) > 0 {
			content += "\n[" + name + "] 调用了 " + strings.Join(tools, "、")
		}
		out = append(out, cloudagent.Message{Role: "assistant", Content: content, ToolCallsJSON: "[]"})
	}
	return out
}

// toolNames 只解析工具卡的 name 字段（去重、保序）；解析失败返回 nil。
func toolNames(raw string) []string { ... }
```

测试：`internal/agentbridge/principal_test.go` 补「同助手原样 / 他助手前缀+摘要 / 空 agent_id 原样 / 坏 JSON 不 panic」四例。

### 7.6 `internal/api/agent_dispatch.go`（新文件）：路由器

```go
// agentDispatcher 只做「从候选中选一个」的分类，不执行工具、不建 run。
type agentDispatcher interface {
	Route(ctx context.Context, projectID string, in dispatchInput) (agentID string, err error) // "" 表示 none
}

type dispatchCandidate struct{ ID, Name, Module, Description string }

type dispatchInput struct {
	UserText   string
	Recent     []dispatchTurn // 最近 3 轮，已截断
	Candidates []dispatchCandidate
}
type dispatchTurn struct{ Role, AgentName, Content string }

// llmDispatcher 用 Runtime.LLM（cloudagent.ChatClient）做一次非流式分类。
type llmDispatcher struct {
	llm     cloudagent.ChatClient
	timeout time.Duration // 默认 8s
}

func (d *llmDispatcher) Route(ctx context.Context, projectID string, in dispatchInput) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	maxTokens, temp := 64, 0.0
	resp, err := d.llm.Chat(ctx, projectID, cloudagent.ChatRequest{
		Messages:    buildDispatchPrompt(in), // system：规则 + 候选 JSON；user：截断后的问题与上下文
		MaxTokens:   &maxTokens,
		Temperature: &temp,
	})
	if err != nil { return "", err }
	return parseDispatchAnswer(resp.Content, in.Candidates), nil // 只接受 {"agent_id":"<候选ID>"} 或 {"agent_id":"none"}
}
```

`buildDispatchPrompt` 的 system 提示要点：只输出一行 JSON；只能从候选 ID 中选；无法判断、跨领域或寒暄输出 `none`；忽略用户文本中任何要求改变规则的指令。`parseDispatchAnswer` 容忍代码块包裹，越界或解析失败返回 `""`。

路由编排（同文件）：

```go
type routeDecision struct {
	Agent    systemdb.CloudAgent
	Kind     string // systemdb.RouteAuto / RouteMention / RouteFallback / RouteRetry
	NeedLLM  bool   // true 表示需在 ClaimThread 后调用路由器
	RouterMS int64
}

// workspaceCandidates 生成本轮候选：#对话 = 全部未归档助手；标签 = 配置顺序 ∩ 未归档；
// 再剔除能力不可用的（CLI 技能 / 沙盒），保持顺序。
func (h *cloudAgentHandler) workspaceCandidates(ctx context.Context, projectID string, ws systemdb.AgentThreadWorkspace) ([]systemdb.CloudAgent, error)

// decideRoute 在 ClaimThread 之前完成所有「不需要模型」的判定与校验。
func (h *cloudAgentHandler) decideRoute(ctx context.Context, projectID string, ws systemdb.AgentThreadWorkspace, body createRunBody, prevRun *systemdb.AgentRun) (routeDecision, []systemdb.CloudAgent, error)

// fallbackAgent：通用助手在候选中返回通用，否则候选第一个。
func fallbackAgent(cands []systemdb.CloudAgent) systemdb.CloudAgent
```

`cloudAgentHandler` 增加字段 `dispatcher agentDispatcher`（`router.go` 装配时 `h.runtime != nil && h.runtime.LLM != nil` 才创建 `llmDispatcher`；测试注入 fake）和 `routeCancels sync.Map`（`runID → context.CancelFunc`）。

### 7.7 `internal/api/agent_run_handler.go`：`CreateRun` 分支

修改前（关键段）：

```go
var agent systemdb.CloudAgent
if len(body.Mentions) == 0 && thread.AgentID != "" {
	agent, err = h.store.GetCloudAgent(ctx, pc.ID, thread.AgentID)
} else {
	agent, err = h.resolveMentionedAgent(ctx, pc.ID, body.Mentions)
}
...
if thread.AgentID != "" && thread.AgentID != agent.ID {
	return WriteError(c, NewAPIError(http.StatusConflict, "agent_switch_required", ...))
}
...
mentionsJSON, _ := json.Marshal(body.Mentions)
if len(body.Mentions) == 0 {
	mentionsJSON, _ = json.Marshal([]mentionDTO{{AgentID: agent.ID}})
}
run, err := h.store.CreateAgentRun(ctx, systemdb.AgentRun{... AgentID: agent.ID, ...})
... AppendAgentMessage(... AgentID: agent.ID, MentionsJSON: string(mentionsJSON) ...)
... History: agentbridge.Messages(hist) ...
```

修改后（结构）：

```go
ws, err := h.store.GetAgentThreadWorkspace(ctx, pc.ID, threadID)
if err != nil { return WriteError(c, err) }
if ws.DispatchMode != systemdb.DispatchAuto {
	return h.createLegacyRun(c, pc, principal, thread, body) // 原逻辑整体搬入，行为零变化
}
return h.createAutoRun(c, pc, principal, thread, ws, body)
```

`createAutoRun` 步骤：

1. 绑定 body、空内容校验、`ensureAgents`、多 mention 校验（同旧逻辑）。
2. 重试校验（同旧逻辑，抽成 `h.loadRetry(...)` 两边复用），拿到 `prevRun`。
3. `decision, cands, err := h.decideRoute(...)`：候选为空 → 409 `no_available_agent`；mention 不在候选 → 400 `agent_not_in_tag`；重试助手不可用 → 409 `retry_agent_unavailable`。
4. `bi_context` 在 auto 会话中拒绝（400 `invalid_request`，BI 走自己的绑定会话）。
5. LLM 未配置、配额检查、`ClaimThread`（同旧逻辑）。
6. `CreateAgentRun`：`AgentID = decision.Agent.ID`（`NeedLLM` 时为空串）。
7. 落用户消息：`AgentID=""`，`MentionsJSON` = 真实 mentions（无则 `[]`）；重试复用原消息。
8. 流式：`beginSSE` 并写 `run` 事件（`NeedLLM` 时不带 `agent_id`，随后写 `route{status:"routing"}`）。为此把 `streamRun` 拆成 `openStream(c, run) *sseWriter` + `runStream(w, ...)`，`createLegacyRun` 仍调用组合后的 `streamRun`，事件序列不变。
9. `NeedLLM` 时：`routeCtx, cancel := context.WithCancel(context.WithoutCancel(runCtx))`，`h.routeCancels.Store(run.ID, cancel)`；调用 `h.dispatcher.Route`；`routeCtx` 被取消 → `UpdateAgentRunStatus(canceled)`，写 `end{reason:canceled}` 返回；否则得到 ID → 匹配候选成功为 `auto`，否则 `fallback`。`dispatcher == nil` 直接 `fallback`。
10. `UpdateAgentRunAgent` 回填；`InsertAgentRunRoute`；写 `route{status:"done", agent_id, route_kind}`。
11. 加载历史，`agentbridge.MessagesFor(hist, agent.ID, names)` 投影；能力检查（CLI/沙盒）已在候选过滤中完成；`skills := cloudagent.SkillsForModule(agent.Module)`；之后与旧逻辑一致进入 `runtime.StartRun`、`finishAgentRun`。
12. 非流式：路由同步完成后走 `completeRun`，响应体增加 `route_kind`。

`finishAgentRun` 审计 `Detail` 增加 `route=%s`（只记类别）。

### 7.8 `internal/api/agent_handler.go`：会话与消息接口

| 位置 | 修改前 | 修改后 |
| --- | --- | --- |
| `threadDTO` | `ID, Title, AgentID, CreatedAt, UpdatedAt, LastMessagePreview` | 增加 `TagID string \`json:"tag_id,omitempty"\``、`DispatchMode string \`json:"dispatch_mode,omitempty"\``；`toThreadDTO(t, ws)` 多一个参数。 |
| `messageDTO` | 无路由信息 | 增加 `RouteKind string \`json:"route_kind,omitempty"\``。 |
| `ListThreads` | 只读 `agent_id` | 读 `tag_id`：非空且不是 `chat` 时校验标签存在（预置 ID 恒存在；自定义需未归档，否则 404 `agent_tag_not_found`）；调用 `PageAgentThreadsFiltered`；用 `ListAgentThreadWorkspaces` 批量补 DTO。 |
| `CreateThread` | body `{title, agent_id}`，`created_by = principal.APIKeyID` | body 增加 `tag_id`、`dispatch_mode`、`source`；校验 §2.2 规则（`tag_id` 需存在且未归档；`dispatch_mode ∈ {"", "auto"}`；`source ∈ {"", "workspace", "starter", "bi"}`；`auto` + `agent_id` → 400；`tag_id` 无 `auto` → 400）；`auto` 时 `source` 默认 `workspace`；调用 `CreateAgentThreadWithWorkspace`。 |
| `GetThread` / `PatchThread` | 只返回会话本体 | 补 `GetAgentThreadWorkspace` 填 DTO。 |
| `ListMessages` | `toMessageDTO(m)` | 先 `ListAgentRunRoutes(threadID)`，assistant 消息按 `RunID` 填 `RouteKind`。 |
| `CancelRun` | 只调 `runtime.CancelRun` | 先 `if f, ok := h.routeCancels.LoadAndDelete(runID); ok { f.(context.CancelFunc)() }`，再走原逻辑。 |

### 7.9 `internal/api/agent_tag_handler.go`（新文件）：标签 CRUD

```go
type agentTagPromptDTO struct{ ID, Title, Content string } // json 标签 id/title/content
type agentTagDTO struct {
	ID         string              `json:"id"`
	BuiltinKey string              `json:"builtin_key,omitempty"`
	Name       string              `json:"name"`
	AgentIDs   []string            `json:"agent_ids"`
	Prompts    []agentTagPromptDTO `json:"prompts"`
	// UnavailableAgentIDs 是配置里已归档/不存在的助手，供编辑器标「助手已不可用」。
	UnavailableAgentIDs []string   `json:"unavailable_agent_ids"`
	Version    int64               `json:"version"`
	UpdatedAt  *time.Time          `json:"updated_at,omitempty"`
}

func (h *cloudAgentHandler) ListAgentTags(c echo.Context) error   // 合并内置默认 + 覆盖行 + 自定义
func (h *cloudAgentHandler) CreateAgentTag(c echo.Context) error  // denyReadonly + normalizeTagBody + store.CreateAgentTag
func (h *cloudAgentHandler) PatchAgentTag(c echo.Context) error   // 预置 → UpsertBuiltinAgentTag；自定义 → UpdateAgentTag
func (h *cloudAgentHandler) DeleteAgentTag(c echo.Context) error  // store.ArchiveAgentTag
func (h *cloudAgentHandler) resolveAgentTag(ctx, projectID, id string) (systemdb.AgentTag, error) // 供 ListThreads/CreateThread/CreateRun 共用
func builtinAgentTagDefaults(agents []systemdb.CloudAgent) map[string]systemdb.AgentTag   // builtin_key → 助手 ID
func normalizeTagBody(...) (name string, cfg systemdb.AgentTagConfig, err error)          // 长度/数量/去重/助手归属；提示词 ID 缺省生成 UUID
```

### 7.10 `internal/api/router.go` 与 `internal/api/error.go`

`router.go`（`/agent-threads` 路由组旁）新增：

```go
p.GET("/agent-tags", ah.ListAgentTags, require(auth.DatabaseRead))
p.POST("/agent-tags", ah.CreateAgentTag, require(auth.DatabaseWrite), writeGate)
p.PATCH("/agent-tags/:tagID", ah.PatchAgentTag, require(auth.DatabaseWrite), writeGate)
p.DELETE("/agent-tags/:tagID", ah.DeleteAgentTag, require(auth.DatabaseWrite), writeGate)
```

`ah := &cloudAgentHandler{...}` 装配处增加 `dispatcher: newLLMDispatcher(deps.CloudAgent)`（`nil` 安全）。系统项目写保护沿用 `writeGate` 与项目上下文中间件，补「系统项目拒绝」用例。

`error.go` 的 `sentinelErrorMappings`（放在 plan 相关条目旁）：

```go
{systemdb.ErrAgentTagNotFound, http.StatusNotFound, "agent_tag_not_found", "agent tag not found"},
{systemdb.ErrAgentTagConflict, http.StatusConflict, "agent_tag_conflict", "agent tag was modified, reload and retry"},
{systemdb.ErrAgentTagDuplicate, http.StatusConflict, "agent_tag_duplicate", "agent tag name already exists"},
{systemdb.ErrAgentTagBuiltin, http.StatusBadRequest, "agent_tag_builtin", "builtin agent tag cannot be deleted"},
{systemdb.ErrAgentTagLimit, http.StatusTooManyRequests, "agent_tag_limit_exceeded", "too many agent tags"},
{ErrNoAvailableAgent, http.StatusConflict, "no_available_agent", "no available agent for this thread"},
{ErrAgentNotInTag, http.StatusBadRequest, "agent_not_in_tag", "mentioned agent is not in this tag"},
{ErrRetryAgentUnavailable, http.StatusConflict, "retry_agent_unavailable", "the original agent is no longer available"},
```

### 7.11 `internal/api/plan_handler.go`：auto 会话的 Plan

修改前：`agentID` 为空时取 `thread.AgentID`，auto 会话为空串 → `agent not found`。

修改后：

```go
agentID := strings.TrimSpace(body.AgentID)
if agentID == "" {
	agentID = thread.AgentID
}
if agentID == "" {
	if ws, _ := h.store.GetAgentThreadWorkspace(ctx, pc.ID, threadID); ws.DispatchMode == systemdb.DispatchAuto {
		// auto 会话的 Plan 显式交给通用助手，且必须在本会话候选中。
		general, err := h.generalCandidate(ctx, pc.ID, ws)
		if err != nil { return WriteError(c, NewAPIError(http.StatusConflict, "plan_unavailable", "plan requires the general agent", rid)) }
		agentID = general.ID
	}
}
```

### 7.12 后端测试清单

| 文件 | 用例 |
| --- | --- |
| `internal/systemdb/agent_tags_test.go`（新） | 建/改/归档；重名（大小写）；50 上限；版本冲突；预置首次 upsert（`expected_version=0`）与再次 upsert；归档事务后会话 `tag_id` 清空且 `dispatch_mode` 保留；`CreateAgentThreadWithWorkspace` 零值不写元数据；`PageAgentThreadsFiltered` 的 `chat` 规则（API Key / cron / bi / 标签会话被排除，旧会话保留）与标签分区 + 游标；`UpdateAgentRunAgent` 只回填一次；`ListAgentRunRoutes`。 |
| `internal/api/agent_tag_handler_test.go`（新） | 权限（read 不能写）、只读实例 503、系统项目拒绝、跨项目助手 ID 400、超长提示词 400、预置不可删、列表合并默认值无写副作用。 |
| `internal/api/agent_dispatch_test.go`（新） | `parseDispatchAnswer`（合法 / 代码块 / 越界 / none / 垃圾）；`buildDispatchPrompt` 截断；`fallbackAgent`（有通用 / 无通用）；fake ChatClient 超时 → fallback。 |
| `internal/api/agent_auto_run_test.go`（新） | 同一会话三轮分别 auto→数据库、mention→对象存储、auto→KV，`runs.agent_id` 与 `route_kind` 正确，仅一个会话；单候选不调路由器；标签外 mention 400；候选空 409 且无 run/消息落库；路由期间 `CancelRun` → canceled；重试固定原助手且不调路由器；旧会话仍返回 `agent_switch_required`；SSE 事件顺序 `run → route(routing) → route(done) → … → end`；历史投影后其他助手工具卡不进入 `RunRequest.History`。 |
| `internal/api/agent_threads_test.go`（改） | `tag_id=chat` / 标签分区 / 与 `agent_id` 交集；`CreateThread` 新字段的非法组合。 |
| `internal/agentbridge/principal_test.go`（改） | `MessagesFor` 四例（见 §7.5）。 |
| fake 同步 | `fakeGW`、`fakeLLMSvc` 等如无接口变更则不动；新增 `fakeDispatcher`。`cron_agent_test.go` 的 `fakeStore` 不受影响（`CreateAgentThread` 签名不变）。 |

## 8. 前端实现（逐文件改动）

> 约束：对象形状用 `interface`；不新增 `any`；后端 snake_case 经 `http-api-converters.ts` 映射；文案只进 `ui/src/locales/` 两棵树（裸 `@` 写成 `{'@'}`）；三态用 `SbAsyncRegion`，确认用 `ConfirmAction`，弹窗用 `SbModal`，状态样式用 `data-[state=…]`。

### 8.1 `ui/src/services/types.ts`

修改前：

```ts
export interface AgentThread {
  id: string
  title: string
  agent_id?: string
  created_at: string
  updated_at: string
  last_message_preview?: string
}
export interface ChatMsg { role: ...; content: string; toolCalls?: ...; error?: string; canceled?: boolean; thinking?: string; module?: string }
export interface AgentMessage { id; role; content; agent_id?; mentions?; tool_calls?; run_id?; run_status?; error_code?; created_at }
export interface AgentStreamHandlers { onRun?: (runId: string) => void; ... }
agentThreads: {
  list: (projectId: string, agentId?: string) => Promise<AgentThread[]>
  page: (projectId: string, agentId?: string, limit?: number, cursor?: string) => Promise<AgentThreadPage>
  create: (projectId: string, title?: string, agentId?: string) => Promise<AgentThread>
  ...
}
```

修改后（增量，旧签名保持兼容）：

```ts
export type AgentRouteKind = 'auto' | 'mention' | 'fallback' | 'retry'   // 字面量联合允许用 type

export interface AgentThread {
  ...原字段
  /** 工作台标签 ID；#对话 与旧会话为空。 */
  tag_id?: string
  /** 'auto' 表示一轮一助手的共享会话；空为旧绑定模式。 */
  dispatch_mode?: 'auto'
}

export interface AgentTagPrompt { id: string; title: string; content: string }
export interface AgentTag {
  id: string
  builtinKey?: 'alerts' | 'stats'
  name: string
  agentIds: string[]
  prompts: AgentTagPrompt[]
  unavailableAgentIds: string[]
  version: number
  updatedAt?: string
}
export interface AgentTagInput { name?: string; agentIds: string[]; prompts: AgentTagPrompt[]; expectedVersion?: number }

export interface AgentThreadCreateOptions {
  title?: string
  agentId?: string
  tagId?: string
  dispatchMode?: 'auto'
  source?: 'workspace' | 'starter' | 'bi'
}

export interface AgentThreadPageQuery { agentId?: string; tagId?: string; limit?: number; cursor?: string }

export interface ChatMsg {
  ...原字段
  /** 实际回答助手 ID（服务端确认后才填）。 */
  agentId?: string
  routeKind?: AgentRouteKind
  /** 路由中（auto 会话首帧到 route.done 之间）。 */
  routing?: boolean
}

export interface AgentMessage { ...原字段; route_kind?: AgentRouteKind }

export interface AgentRouteEvent { status: 'routing' | 'done'; agentId?: string; routeKind?: AgentRouteKind }

export interface AgentStreamHandlers {
  onRun?: (runId: string, agentId?: string, routeKind?: AgentRouteKind) => void   // 追加可选参数，旧调用方不受影响
  onRoute?: (ev: AgentRouteEvent) => void
  ...其余不变
}

// API 接口
agentTags: {
  list: (projectId: string) => Promise<AgentTag[]>
  create: (projectId: string, input: AgentTagInput) => Promise<AgentTag>
  patch: (projectId: string, tagId: string, input: AgentTagInput) => Promise<AgentTag>
  remove: (projectId: string, tagId: string) => Promise<void>
}
agentThreads: {
  ...
  /** 新：按分区分页；旧 page(projectId, agentId, ...) 保留。 */
  pageBy: (projectId: string, q: AgentThreadPageQuery) => Promise<AgentThreadPage>
  /** create 第二参数兼容 string（旧：title）与 AgentThreadCreateOptions。 */
  create: (projectId: string, titleOrOpts?: string | AgentThreadCreateOptions, agentId?: string) => Promise<AgentThread>
  get: (projectId: string, threadId: string) => Promise<AgentThread>
}
```

### 8.2 `ui/src/services/http-api-converters.ts` 与 `http-api.ts`

- `toAgentThread`：增加 `tag_id: raw?.tag_id ? String(raw.tag_id) : undefined`、`dispatch_mode: raw?.dispatch_mode === 'auto' ? 'auto' : undefined`。
- `toAgentMessage`：增加 `route_kind: isRouteKind(raw?.route_kind) ? raw.route_kind : undefined`（新增 `isRouteKind` 守卫，不用 `any`）。
- 新增 `toAgentTag(raw: Record<string, unknown>): AgentTag`（snake → camel，数组兜底 `[]`）、`fromAgentTagInput(input): Record<string, unknown>`（camel → snake，`expected_version`）。
- SSE 解析 `onEvent`，修改前：

  ```ts
  if (obj?.type === 'run' && obj.run_id) {
    runId = String(obj.run_id)
    handlers.onRun?.(runId)
    return
  }
  ```

  修改后：

  ```ts
  if (obj?.type === 'run' && obj.run_id) {
    runId = String(obj.run_id)
    handlers.onRun?.(runId, obj.agent_id ? String(obj.agent_id) : undefined, isRouteKind(obj.route_kind) ? obj.route_kind : undefined)
    return
  }
  if (obj?.type === 'route') {
    handlers.onRoute?.({
      status: obj.status === 'done' ? 'done' : 'routing',
      agentId: obj.agent_id ? String(obj.agent_id) : undefined,
      routeKind: isRouteKind(obj.route_kind) ? obj.route_kind : undefined
    })
    return
  }
  ```

  `attachAgentRun` 的回放路径（约 990 行）同样处理 `route`，保证切走再回来时回答者正确。
- `http-api.ts`：新增 `agentTags` 四个方法（路径 `agentPath(projectId, '/agent-tags')`）；`agentThreads.pageBy` 传 `{ limit, cursor, tag_id, agent_id }`（空值不传）；`agentThreads.create` 归一化参数：`typeof titleOrOpts === 'string'` 走旧 body `{ title, agent_id }`，否则发 `{ title, agent_id, tag_id, dispatch_mode, source }`（空值省略）；`agentThreads.get` 调 `GET /agent-threads/:id`。

### 8.3 `ui/src/composables/useAgentConversation.ts`

修改前：`start(body: { content; mentions; module?; retry_of_run_id?; bi_context? })` 创建 `reply` 时只设 `module`；`bindHandlers.onRun` 只记 `runId`。

修改后：

```ts
export interface ConversationOptions {
  ...原字段
  /** agentId → module，供 route/run 事件到达后补头像；不传时只记 agentId。 */
  moduleOf?: (agentId: string) => string | undefined
}

function start(body: { ...; module?: string; agentId?: string; routing?: boolean }, handlers: Handlers) {
  ...
  const reply = reactive<ChatMsg>({ role: 'assistant', content: '', toolCalls: [], module: body.module, agentId: body.agentId, routing: !!body.routing })
  ...
}

// bindHandlers 内：
onRun: (id, agentId, routeKind) => {
  runId.value = id
  if (agentId) applySpeaker(reply, agentId, routeKind)
  handlers.onRun?.(id, agentId, routeKind)
},
onRoute: (ev) => {
  if (ev.status === 'routing') { reply.routing = true; phase.value = 'thinking' }
  else if (ev.agentId) applySpeaker(reply, ev.agentId, ev.routeKind)
  handlers.onRoute?.(ev)
},

function applySpeaker(reply: ChatMsg, agentId: string, routeKind?: AgentRouteKind) {
  reply.agentId = agentId
  reply.routeKind = routeKind
  reply.routing = false
  reply.module = opts.moduleOf?.(agentId) ?? reply.module
}
```

`statusText` 增加 `reply.routing` 时显示 `agentPhase.routing`（「正在选择助手…」）。`attach` 同样接收 `agentId`。

### 8.4 新组件

| 文件 | Props / Emits | 要点 |
| --- | --- | --- |
| `components/agent/AgentWorkspaceSidebar.vue` | props：`tags`、`chatThreads`、`section`、`threadId`、`runningThreadIds`、`hasMore`、`pending/error`（三态）；emits：`select-section(id)`、`select-thread(id)`、`create-tag`、`create-thread`、`load-more`、`rename-thread(id)`、`remove-thread(id)`、`edit-tag(id)`、`archive-tag(id)` | 顶部 `Input` 搜索（本地过滤 `tags` 与已加载会话标题，命中会话显示所属标签名）+ `Button size="icon-sm"`（`aria-label=t('components.agentWorkspace.newTag')`）。`#对话` 行使用 `Collapsible`（若 `ui/` 无则用 `button[aria-expanded]` + `v-show`），右侧 `+`（`aria-label=newThread`）。会话行：标题（豁免名单显示「未命名会话」）+ `relativeTime` + 运行中 `LoaderCircleIcon` + 行菜单（`Popover`：重命名 / `ConfirmAction` 归档）。标签行：`#名称` + 行菜单（编辑 / 归档，预置无归档）。选中态 `aria-current="page"`。 |
| `components/agent/AgentTagOverview.vue` | props：`tag`、`agents`（已本地化名称）、`threads`、`pending`；emits：`edit`、`pick-prompt(prompt)`、`open-thread(id)`、`new-thread` | 助手 chips（不可用者灰显 + 提示）、提示词按钮列表、最近会话列表、「新建标签会话」按钮；空态 `SbEmptyState`。 |
| `components/agent/AgentRosterBar.vue` | props：`agents`（候选）、`selectedId`、`fixed`（旧绑定会话）；emits：`pick(id)` | 横向 chips，溢出部分收进 `Popover`「更多」；`fixed` 时只显示「固定：助手名」与「新建自动路由会话」按钮（emit `new-auto`）。选中 chip 用 `data-[state=active]`。 |
| `components/modal/AgentTagModal.vue` | props：`open`、`tag?`（编辑时）、`agents`、`saving`；emits：`update:open`、`save(input: AgentTagInput)` | `SbModal` + `FieldGroup`：名称（预置只读）、助手多选（`Combobox` 多选 + 已选 chips，可移除不可用项）、提示词列表（标题 + `Textarea` 正文 + 上移/下移/删除，`+ 添加提示词`）；本地校验与后端同口径（长度、数量）；409 冲突时保留表单并显示「已被他人修改，重新加载」按钮。 |

### 8.5 `ui/src/components/agent/ConversationView.vue`

修改前：assistant 头像只按 `m.module` 显示图标，无名字。

修改后：

- 新 prop `agentNames: Record<string, string>`（`agentId → 显示名`）。
- assistant 气泡上方增加一行 `data-agent-speaker`：`{{ agentNames[m.agentId] || t('components.conversation.unknownAgent') }}`，`routeKind === 'mention'` 时追加徽标「指定」、`fallback` 追加「通用兜底」、`retry` 追加「重试」；`auto` 不加徽标（默认态）。
- `m.routing` 时显示「正在选择助手…」代替三点跳动的文字说明（三点保留）。
- 历史 `agentId` 指向已归档助手时显示中性占位「已移除的助手」。

### 8.6 `ui/src/components/agent/AgentComposer.vue` 与 `components/ai/AiChatComposer.vue`

- `AiChatComposer.vue`：`defineExpose({ focus })` → `defineExpose({ focus, insertMention: pickMention, clearMentions })`；`pickMention` 已有，仅暴露；新增 `clearMentions()` 清空 `selected` 并从文本移除 `@名称 `。
- `AgentComposer.vue`：透传 `insertMention`、`clearMentions`；新增 prop `routeHint?: string`，在 `#actions` 插槽左侧显示「自动选择助手」或「本轮：@X」（`aria-live="polite"`）。

### 8.7 `ui/src/pages/AgentManager.vue`（重构编排）

**模板**，修改前：`grid md:grid-cols-[220px_minmax(0,1fr)]` 左 `AgentListPanel`、右 Card 头部 `@活动助手` + 只读徽标 + `ThreadSwitcher` + 「更多」；`switchTarget` 横幅；`AgentWelcome` 按活动助手渲染。

修改后：

```vue
<div class="grid grid-cols-1 items-stretch gap-4 md:grid-cols-[240px_minmax(0,1fr)] xl:grid-cols-[260px_minmax(0,1fr)]">
  <AgentWorkspaceSidebar class="max-md:hidden" v-bind="sidebarProps" v-on="sidebarHandlers" />
  <Sheet v-model:open="navOpen"><SheetContent side="left" class="w-[85vw] max-w-80 p-0">
    <AgentWorkspaceSidebar v-bind="sidebarProps" v-on="sidebarHandlersClosingSheet" />
  </SheetContent></Sheet>

  <Card class="flex h-[calc(100svh-var(--header-height)-var(--page-pad)-var(--page-pad))] min-h-[420px] min-w-0 flex-col sm:min-h-[520px]">
    <CardHeader class="border-b">
      <CardTitle class="flex w-full flex-wrap items-center gap-2">
        <Button class="md:hidden" size="icon-sm" variant="ghost" :aria-label="t('components.agentWorkspace.openNav')" @click="navOpen = true"><PanelLeftIcon /></Button>
        <span class="min-w-0 truncate">{{ headerTitle }}</span>            <!-- 「#对话 / 订单复盘」 -->
        <span v-if="speakerLabel" class="text-xs text-muted-foreground">{{ speakerLabel }}</span>
        <Popover v-model:open="menuOpen"> ...会话组（重命名/归档）+ 助手组（管理助手/新建助手/定时任务）+ 标签组（编辑标签）... </Popover>
      </CardTitle>
      <AgentRosterBar v-if="threadId || pendingDraft" :agents="candidateAgents" :fixed="fixedAgent" @pick="onRosterPick" @new-auto="createChatThread" />
    </CardHeader>
    <CardContent ...>
      <AgentTagOverview v-if="isTagSection && !threadId && !pendingDraft" ... />
      <template v-else>
        <SbAsyncRegion ...><ConversationView :agent-names="agentNames" ...><template #empty><AgentWelcome :mode="welcomeMode" @pick="pickPrompt" @new-thread="createChatThread" /></template></ConversationView></SbAsyncRegion>
        <PlanProgress ... />
        <AgentComposer ref="composer" v-model="draft" :route-hint="routeHint" :mention-agents="mentionAgents" :disabled="composerDisabled" @send="onSend" @stop="onStop">...Chat/Plan 切换（planAvailable 为假时隐藏 Plan）...</AgentComposer>
      </template>
    </CardContent>
  </Card>
</div>
<AgentTagModal ... />
<AgentFormModal ... />   <!-- 不变，入口改到「更多 → 管理助手 / 新建助手」，编辑目标改为下拉选择 -->
```

**脚本**，函数级改动：

| 函数 | 修改前 | 修改后 |
| --- | --- | --- |
| `bootstrap` | `loadAgents → loadThreads`（无会话则自动创建） | `Promise.all([loadAgents(), loadTags()])` → `restoreSection()`：`?thread=` → `api.agentThreads.get` 推导 section；否则 `?tag=`；否则本地记忆；否则 `chat` → `loadChatThreads()` → 选第一条或显示欢迎区；**不自动建会话**。`GET /agent-tags` 404 时置 `legacyMode=true`，继续渲染旧 `AgentListPanel` 路径（保留到下个版本删除）。 |
| `loadThreads` | 按 `activeId` 拉列表，空则建会话 | 拆为 `loadChatThreads(reset)`（`pageBy({tagId:'chat'})`，分页追加）与 `loadTagThreads(tagId)`；均做 `projectId/section` 竞态校验。 |
| `selectAgent` / `switchToMentionedAgent` / `refreshThreadsForAgent` | 存在 | 删除；`onEnd` 回调改为 `refreshCurrentSection()`。 |
| `selectSection(id)`（新） | — | `detachRun()`、保存草稿、`section=id`、`threadId=''`、`pendingDraft=null`、`replaceQuery({ tag: id === 'chat' ? undefined : id, thread: undefined })`；`chat` 时恢复最近会话。 |
| `selectThread(id)` | 只接受当前列表内 ID | 允许深链补齐；切换前存草稿、切换后恢复草稿；`replaceQuery({ thread: id })`。 |
| `createChatThread()`（新，替代 `resetThread`） | `create(pid, t('nav.agents'), activeId)` | `create(pid, { dispatchMode: 'auto', source: 'workspace' })`，不传标题。 |
| `startTagDraft(tagId)`（新） | — | `pendingDraft={section: tagId}`、`threadId=''`、`chatMessages=[]`、聚焦。 |
| `pickPrompt(text)` | 直接覆盖草稿 | 草稿非空时打开 `ConfirmAction`（`promptReplaceOpen`），确认后覆盖；标签概览中点击会先 `startTagDraft`。 |
| `onSend(text, mentions)` | 必须有 speaker；mention 与 `activeId` 不同则弹切换横幅；`thread.agent_id` 冲突返回 | ① `pendingDraft` 时先建标签会话（失败保留草稿）；② `auto` 会话：mentions 原样（可为空），校验 mention 在 `candidateAgents` 内，`conv.start({ content, mentions, agentId: mentions[0]?.agent_id, module, routing: !mentions.length && candidateAgents.length > 1 })`；③ 旧绑定会话：沿用旧校验，冲突时提示「该会话固定为 X，可新建自动路由会话」并保留草稿；④ Plan：auto 会话 `createPlan(pid, id, '', content)`（服务端选通用）。错误码映射见 §4.3。 |
| `retryLast` | 不变 | 不变（服务端固定原助手）；`retry_agent_unavailable` 时提示另发。 |
| `toChatMessage` | `module` 来自 `agents.find(agent_id)` | 同时填 `agentId: m.agent_id`、`routeKind: m.route_kind`。 |
| `maybeStartStarter` | `create(pid, title, general.id)` | `create(pid, { title, agentId: general.id, source: 'starter' })`；建完 `section='chat'` 并插入 `chatThreads`。 |
| `runningAgentIds` | 存在 | 删除；`runningThreadIds` 保留并传给侧栏。 |
| 新 computed | — | `candidateAgents`、`fixedAgent`（旧绑定会话的助手）、`mentionAgents`（由 `candidateAgents` 生成）、`agentNames`、`routeHint`、`headerTitle`、`speakerLabel`、`planAvailable`（auto 会话需通用助手在候选中）、`isTagSection`。 |

`ui/src/composables/useBiAgent.ts`：`api.agentThreads.create(pid, title, a.id)` → `create(pid, { title, agentId: a.id, source: 'bi' })`，使 BI 会话不进入 `#对话`。

### 8.8 旧组件处置

| 文件 | 处置 |
| --- | --- |
| `components/agent/AgentListPanel.vue`、`AgentCard.vue` | 本版本保留，仅在 `legacyMode`（旧后端）下渲染；下个版本随兼容层删除。相关测试 `agent-components.test.ts`、`agent-split-components.test.ts`、`nav-icons.test.ts` 不动。 |
| `components/agent/ThreadSwitcher.vue` | 主流程不再使用（会话列表在侧栏）；保留到下个版本，`thread-switcher-time.test.ts` 不动。`template-inline-handlers.test.ts` 若枚举页面模板内联处理器，按新模板更新快照。 |
| `components/agent/AgentWelcome.vue` | 增加 `mode: 'chat-empty' \| 'thread-empty'` 与 `new-thread` 事件：分区无会话时显示「新建会话」按钮；会话为空时显示通用提示词。 |

### 8.9 多语言（`ui/src/locales/zh-CN.ts` 与 `en-US.ts` 同构新增）

```ts
components: {
  agentWorkspace: {
    search: '搜索标签或会话',           // Search tags or threads
    newTag: '新建标签',                  // New tag
    newThread: '新建会话',               // New thread
    chatSection: '对话',                 // Chat
    builtinTags: { alerts: '告警', stats: '统计' },
    untitled: '未命名会话',              // Untitled thread
    loadMore: '加载更多',
    openNav: '会话与标签',
    archiveTagConfirm: '归档标签「{name}」？其中的会话会回到 #对话。',
    fixedAgent: '固定：{name}',
    newAutoThread: '新建自动路由会话',
    autoRoute: '自动选择助手',
    mentionThisTurn: '本轮：{'@'}{name}',
    noAvailableAgent: '当前标签没有可用助手，请编辑标签或回到 #对话',
    agentNotInTag: '该助手不在当前标签中',
    retryAgentUnavailable: '原助手已不可用，请重新发送',
    replaceDraftConfirm: '用提示词替换当前输入？',
    threadNotFound: '会话不存在或已归档'
  },
  agentTagOverview: { agents: '助手', prompts: '提示词', recent: '最近会话', newThread: '新建标签会话', continue: '继续', unavailable: '助手已不可用' },
  agentTagModal: { createTitle: '新建标签', editTitle: '编辑 #{name}', name: '名称', agents: '助手', prompts: '提示词', promptTitle: '标题', promptContent: '内容', addPrompt: '添加提示词', conflict: '标签已被修改，请重新加载', reload: '重新加载' },
  conversation: { routing: '正在选择助手…', routeMention: '指定', routeFallback: '通用兜底', routeRetry: '重试', unknownAgent: '已移除的助手' }
},
agentPhase: { routing: '正在选择助手…' }
```

`ui/tests/i18n.test.ts` 会校验两棵树键集合一致；`mentionThisTurn` 的 `@` 必须转义。

### 8.10 前端测试清单

| 文件 | 用例 |
| --- | --- |
| `tests/AgentManager.test.ts`（改） | 旧用例中依赖 `activeId`/自动建会话的断言改为新流程；新增：进入页面不建会话；`#对话` 旁 `+` 只调 `agentThreads.create({dispatchMode:'auto'})`；顶部 `+` 只开标签弹窗；切项目后旧列表/SSE 不回写；深链 `?thread=` 推导 section；旧后端 404 降级。 |
| `tests/agent-workspace-sidebar.test.ts`（新） | 两个加号的 `aria-label` 与事件；搜索只过滤不切换；命中会话显示所属标签；`aria-current`；行菜单重命名/归档；未命名标题本地化。 |
| `tests/agent-tag-modal.test.ts`（新） | 校验长度/数量；不可用助手可移除；409 冲突保留表单；预置标签名称只读。 |
| `tests/agent-tag-overview.test.ts`（新） | 点提示词只填草稿（不触发发送）；非空草稿先确认；无会话时「新建标签会话」进入草稿态。 |
| `tests/useAgentConversation.test.ts`（改） | `run` 事件带 `agent_id` 直接定身份；`route routing → done` 更新 `agentId/module/routeKind/routing`；attach 回放同样生效。 |
| `tests/agent-composer.test.ts`（改） | `insertMention`/`clearMentions` 暴露；中文输入法 `compositionend` 前 Enter 不发送；`routeHint` 渲染。 |
| `tests/http-api-converters` 相关（改） | `toAgentThread`/`toAgentMessage` 新字段；`toAgentTag`；SSE `route` 帧解析；`create` 新旧两种参数形态的请求体。 |
| `tests/useBiAgent.test.ts`（改） | 建会话带 `source: 'bi'`。 |
| 覆盖率 | 新文件须让全局 statements/branches/functions 维持或提升（当前 branches 94.92%、functions 94.1% 未达 95%，本次新增代码不得拉低）。 |

## 9. 风险与对策

| 风险 | 对策 |
| --- | --- |
| 路由模型误判导致写入型助手被选中 | 路由只决定「谁回答」，写操作仍需助手 `writable` + 现有审批确认；审计记录 `route_kind`；验收覆盖「自动选到可写助手仍需确认」。 |
| 路由增加首字延迟 | 单候选与点名不调模型；路由 8s 上限且 `MaxTokens=64`；SSE 先发 `route{routing}` 给出反馈；超时直接 fallback。 |
| `#对话` 过滤依赖 `created_by=''` 判断控制台来源 | 已核对：登录态 Principal 无 `APIKeyID`，定时任务为 `system-scheduler`，API Key 为 Key ID；新增 `source` 列兜底 BI；测试固化三类来源。若未来登录态写入 `created_by`，需同步改分区条件（在 `PageAgentThreadsFiltered` 注释中标注）。 |
| DuckLake 无唯一约束导致重复元数据行 | 元数据只在建会话事务内插入，无其他写入路径；`GetAgentThreadWorkspace` 遇多行取最早一行并记 warn；测试覆盖并发建会话。 |
| 回退到旧版本后 auto 会话行为 | 旧版本把 `agent_id=''` 视为未绑定 → 通用助手，可继续使用；新表被忽略。 |
| `AgentManager.vue` 重构面大、回归风险高 | 按 §10 拆 PR；先落服务层与 composable（可独立测），再换模板；保留 `legacyMode` 路径一版。 |

## 10. 实施顺序与 PR 拆分

| PR | 内容 | 依赖 | 验证 |
| --- | --- | --- | --- |
| PR1 · 存储 | §7.1 三张表；§7.2 `agent_tags.go` + `store.go` 锁；§7.3 `PageAgentThreadsFiltered` 与 `CreateAgentThreadWithWorkspace`；`UpdateAgentRunAgent`/`InsertAgentRunRoute`/`ListAgentRunRoutes`；`agent_tags_test.go` | — | `go test ./internal/systemdb` |
| PR2 · 标签与会话 API | §7.8 会话 DTO/列表/创建/消息；§7.9 标签 CRUD；§7.10 路由与错误映射；handler 测试（权限、只读、系统项目） | PR1 | `go test ./internal/api -run 'AgentTag\|AgentThread'` |
| PR3 · 路由与运行 | §7.4 Event 字段（gocloudagent）；§7.5 `MessagesFor`；§7.6 路由器；§7.7 `CreateRun` 拆分；§7.8 `CancelRun`；§7.11 Plan；`agent_auto_run_test.go` | PR2 | `go test ./internal/api ./internal/agentbridge`；`(cd gocloudagent && GOWORK=off go test ./...)`；`./scripts/test.sh` |
| PR4 · 前端服务层 | §8.1–§8.3 类型、映射、SSE、composable；§8.7 中 `useBiAgent` 与 starter 的 `source`；对应单测 | PR2（契约冻结即可并行） | `cd ui && yarn typecheck && yarn test` |
| PR5 · 前端工作台 | §8.4–§8.9 新组件、`AgentManager.vue` 重构、文案；组件测试与 320px 检查 | PR3、PR4 | `cd ui && yarn typecheck && yarn test && yarn build`；`./build.sh dev` 手工走 §5 场景 1–4 |
| PR6 · 收尾 | 文档（`docs/` 云助手章节补标签与自动路由说明）、`internal/AGENTS.md`/`ui/AGENTS.md` 如有新约定则补充；`./scripts/smoke.sh` | PR5 | `./scripts/smoke.sh` |

PR3 合入前前端不得暴露「自动选择助手」控件（PR4 只改服务层，不改页面）；PR5 合入即视为 P0 功能完整，按 §5 验收后对用户放开。
