# CloudAgent 精简、定时任务统一与独立 CLI 计划（v6.0）

> **仓库**：SimpleBase（`github.com/linkxzhou/SimpleBase`，Go 1.25，单 go.mod）
> **日期**：2026-10-14
> **核对基线**：`main` @ `2896f82` + 2026-10-14 工作区未提交改动（`git status --short` 有 463 条，含 planv6.0 多份文档与 `internal/` 未提交改动；实施前必须重新核对差异）
> **依据**：[`internal/gocloudagent/eino-readme.md`](../../internal/gocloudagent/eino-readme.md)（Eino v0.9.19 权威文档，2311 行）、[`internal/gocloudagent/pi-readme.md`](../../internal/gocloudagent/pi-readme.md)（Pi 对照与既有目标设计，887 行）、[`AGENTS.md`](../../AGENTS.md)、[`internal/AGENTS.md`](../../internal/AGENTS.md)、[`ui/AGENTS.md`](../../ui/AGENTS.md)
> **关联计划**：[`cloud-assistant-chat-ux-plan-v2.md`](./cloud-assistant-chat-ux-plan-v2.md)（写开关 `writable`、trace 日志）、[`cloud-assistant-skills-cli-v2-plan.md`](./cloud-assistant-skills-cli-v2-plan.md)（委托白名单、`@` 解析）、[`backend-code-simplification-plan.md`](./backend-code-simplification-plan.md)（两份 scheduler 曾**刻意未统一**，见其 §2 末段）、[`cloud-agent-module-mentions-and-thread-routing-plan.md`](./cloud-agent-module-mentions-and-thread-routing-plan.md)（thread 归属校验）
> **约束**：不新增第三方依赖（例外见 §12 开放问题 Q3）；不改 `go.mod` 结构；对象形状用 TS `interface`；中英文案双树同步；**不自动执行 `init_db` 或任何 ALTER/DDL**；迁移只提供定义与手动命令，由用户执行。

---

## 0. 结论速览

本计划覆盖用户提出的四件事，它们有严格的前后依赖，不能并行开工：

| 议题 | 内容 | 依赖 | 核心产出 |
| --- | --- | --- | --- |
| **A** | 剥离 `gocloudagent` 的定时任务能力，统一到 `internal/cronjob`；UI 定时任务支持「云函数」或「Agent」两种目标，Agent 提示词上限 16KiB | 无（可先做） | 统一任务表 + 两个 Executor + 16KiB 校验 |
| **B** | 删除 `gocloudagent/scheduler.go` 与 3 个 scheduler 测试文件（约 1290 行） | 依赖 A | 删除清单与迁移去向 |
| **C** | 基于 `eino-readme.md` 重新组织 `gocloudagent`：中间件化、多 Agent、调度流程 | 依赖 A/B（先拆走调度，职责才干净） | 三层拆分（`gocloudagent` / `gocloudagent/agentcore` / `gocloudagent/cli` + `gocloudagent/tools`） |
| **D** | 新增 `cmd/gocloudagent` 独立 CLI（session 存 `~/.gocloudagent/`、jsonl、web 搜索、shell） | 依赖 C 的分层 | 新二进制 + 新包 + build 脚本 |
| **E** | 对齐 `pi-readme.md` 的能力：工具管道、三层事件、会话树与分支、压缩、skills 完善、prompt templates、CLI 外壳模式、评估 harness、项目信任 | 与 C、D 交织（见 §13） | 十项能力 + 三处明确不引入 |

**一句话决策**：把「什么时候跑」和「跑什么」彻底分开——`cronjob` 只管前者（tick/CAS/并发/落库），Agent 的执行体下沉到新包 `internal/agentjob`；`gocloudagent` 退回「一次 run 怎么跑」，不再认识 schedule。

**议题 E 的定性**：`pi-readme.md` §4.3 已给出「在 Go / Eino 上的设计」的骨架（`RunLoop`、`StreamDelta`、`ToolExec`、`SessionView`、`compact`）。本计划**部分采用**：把与 loop 无关但确属缺口的十项能力补齐（§13），把「自研 `RunLoop` 替换 `runner.Run`」降级为**有触发条件的可选阶段**，理由见 §13.3 与 §13.11。

**六个必须先确认的开放问题**（见 §12）：搜索后端选型、Agent 目标任务的 thread 复用策略、是否允许引入 `doublestar/v4` 间接依赖、历史 `sys_agent_schedules` 数据是否回填、CLI 模型出口、CLI shell 确认策略；其中 §13 新增两项（压缩阈值口径、是否上自研 loop）。

---

## 1. 现状核对（全部带证据）

### 1.1 定时任务是两份同构实现

`internal/cronjob/scheduler.go`（311 行）与 `internal/gocloudagent/scheduler.go`（386 行）是**照搬级同构**，前者自己的包注释就写着「调度范式镜像 `internal/gocloudagent/scheduler.go`：tick 扫描 + CAS 认领 + 并发上限」（`internal/cronjob/scheduler.go:2`）。

逐项对照：

| 结构 | `cronjob` | `gocloudagent` |
| --- | --- | --- |
| 常量块 | `schedulerTick=30s`、`schedulerConcurrency=4`（:26-31） | `schedulerTick=30s`、`schedulerConcurrency=4`、`scheduleHistoryLimit=40`、`schedulerPrincipalID`（:23-28） |
| 状态机字段 | `mu` / `inFlight` / `stopped` / `sctx` / `wg`（:59-64） | 完全同名同义（:71-76） |
| `Start` / `Stop` | :74 / :108 | :86 / :120 |
| `tickOnce` | :118 | :130 |
| `runDue` | :138（含 `nextRun` 三态） | :150（只认 cron，`NextAfter` 失败即停用） |
| `Trigger` | :180（`stopped` 判定与 `wg.Add` 同锁） | :179（同锁，同注释 LIFE-02） |
| `claimInFlightLocked` | :275 | :346 |
| 错误哨兵 | `ErrSchedulerStopped`（:50） | `ErrSchedulerStopped` + `ErrScheduleExecuting`（:58/:61） |

执行体差异只在最后一段：`cronjob` 调 `Runner.RunFunction`（云函数），`gocloudagent` 调 `Runner.StartRun`（Agent），并多出 7 段 agent 专有逻辑（配额、归档停用、thread 重建、跨 agent 拒绝、只读 principal、`NormalizeSkills`、`UpdateAgentRunMetrics` 可选断言）。

测试重复度：`cronjob` 三个测试文件共 13 个 Test，`gocloudagent` 三个共 18 个（含子测）；其中 `scheduler_lifecycle_test.go` 两边**逐行相同**（`Stop` 取消手动触发、`Stop` 后 Trigger 报错、50 并发 Stop/Trigger 不 panic），`scheduler_test.go` 约 6 个场景同构，`coverage_test.go` 约 4 个子测与 `scheduler_more_test.go` 同构。粗算 **13 个 Test 里约 10 个是跨包同场景重复**。

### 1.2 表与 API 不对称

| 维度 | 云函数（`sys_cron_jobs`，迁移 v29） | Agent（`sys_agent_schedules`，迁移 v26） |
| --- | --- | --- |
| DDL 位置 | `internal/systemdb/migrate.go:406-431` | `:362-377` |
| 调度模式 | 三态：`cron` / `interval` / `once` | 只有 `cron_expr`（单态） |
| 目标字段 | `func_file`、`func_export`、`input_json` | `agent_id`、`prompt`、`thread_id` |
| 快照字段 | `last_status`、`last_error`、`run_count` | 无 |
| run 记录 | `sys_cron_job_runs`（含 `duration_ms`、`response_json`） | `sys_agent_schedule_runs`（含 `run_id`、`thread_id`、`agent_id`） |
| 值域约束 | `name` 唯一性由应用层保证（`cronJobNameRe`，`cronjob_handler.go:30`）；`input_json` ≤ 8KiB（:33） | **`prompt` 无任何长度校验**（`agent_schedule_handler.go:137` 只判 `TrimSpace != ""`） |
| 软删 | `archived_at` | `archived_at` |
| HTTP 路由 | `/cron-jobs*`（`router.go:492-498`），全部 `delegable` | `/agent-schedules*`（`router.go:542-549`），全部 `require`（**不可委托**） |
| 触发权限 | `DatabaseWrite` + writeGate | **`DatabaseRead` + writeGate**（`router.go:549`） |
| 审计 | 每个写操作 `recordAudit`（`cronjob_handler.go:555`） | **无审计**（`agent_schedule_handler.go` 全文无 `audit`） |
| handler | `CronJobHandler`（572 行，有 service 接口吗？无，直接持 `*systemdb.Store`） | `agentScheduleHandler`（无 service 接口，直接持 `*systemdb.Store`） |

`sys_agent_schedules.cron_expr` 是 `VARCHAR NOT NULL`，没有 interval/once 概念——这是「统一」时最大的能力不对称，也是必须扩列的原因。

### 1.3 gocloudagent 与 Eino 能力对照

`gocloudagent` 当前用到的 Eino 面（`eino-readme.md` §15.6 的表格，已核对源码一致）：

| 用了 | 没用 |
| --- | --- |
| `adk.NewChatModelAgent` + `adk.NewRunner`（`runtime.go:210-242`） | `compose.Chain` / `Graph` / `Workflow` |
| `model.ToolCallingChatModel` 适配器 `gatewayChatModel`（`model.go:19`） | `flow/agent/react` |
| `utils.InferTool`（`tools.go:111` 起，7 个只读 + 4 个沙盒） | `flow/agent/multiagent/host` |
| `compose.ToolsNodeConfig`（只填 `Tools`） | 全部 `adk/middlewares/*` |
| `compose.GetToolCallID`（CLI 确认与 `callGate`） | `adk/prebuilt/*`（deep / planexecute / supervisor） |
| `schema.Pipe` / `ConcatMessages` 的 Index 规则 | Checkpoint / `tool.StatefulInterrupt` |
| `adk.ErrExceedMaxIterations` 转 `Reason=max_iterations` | embedding / retriever |

`eino-readme.md` §9.4 明确写了现状定性：**「gocloudagent 的『多能力』不是多 Agent：一个 `ChatModelAgent`，工具列表由模块和 skill 决定」**。用户要求「如何使用多 agent，如何设计 agent 调度流程」，正是要改这一点。

两个已被 eino 上游标记为**不推荐**的方向，本计划不采用（`eino-readme.md` §8.2 / §9.4）：

- `adk.SetSubAgents` 全量上下文移交（transfer）——上游源码注释：「全量上下文移交在经验上并不更好」；
- `prebuilt/supervisor`——上游标记为不推荐，优先 `AgentTool` 或 `deep`。

`gocloudagent` 的 import 边界很干净：生产文件只 import `internal/auth`、`internal/crontab`（仅 scheduler.go）、`internal/systemdb`、`internal/observability`、`internal/llmgateway`（仅 errors.go 取哨兵）、`internal/sbcli`、`internal/database/sqlguard`，加 `skills`（embed 资源）。**不 import `api` / `app` / `cronjob`**，无环。

### 1.4 cronjob 的边界与成环风险

`cronjob` 生产代码只 import `internal/crontab` + `internal/systemdb`（`scheduler.go:6-17`）——**不 import 任何 Agent 相关包**。

两条让 `cronjob` 认识 Agent 的路径：

1. `cronjob` → `gocloudagent` 直接 import：**不成环**（`gocloudagent` 不 import `cronjob`），但会把 eino + litellm + sbcli + sqlguard 整条链拉进 `cronjob`，并破坏「本包不直接依赖内核」的现状声明。
2. **接口注入 + 上层适配**（与既有 `cronjob.Runner` 同构）：`cronjob` 只定义注入点，实现放上层——无 import 变化，符合 `app` 是唯一装配点的约定。

**本计划选 2。** 成环的确切触发条件是：若把 `gocloudagent.Scheduler` 整体搬进 `cronjob` 后，`gocloudagent` 又保留一个兼容薄壳 `gocloudagent.NewScheduler` 引用 `cronjob` 的类型，则立刻成环。因此**不做兼容薄壳**，一次性删除（见 §3）。

### 1.5 cmd 与 CLI 先例

| 事实 | 证据 |
| --- | --- |
| `cmd/` 只有两个入口：`simplebase`（2.0KB）、`perfbench`（1.6KB） | `ls cmd/` |
| 用标准库 `flag` + 手写分支，**没有 cobra** | `cmd/simplebase/main.go:21-50` |
| 分发规则只有三条：`args[0] != "reset"` → 全部交给 `sbcli.RunIO`；`reset` → `runReset`；无参数 → 启动服务 | 同上 :22-35 |
| `internal/sbcli` 是完整的资源 CLI（37KB `commands.go`，72 个命令） | `internal/sbcli/commands.go` `dispatch` :96-233 |
| `sbcli` 配置在 `$XDG_CONFIG_HOME/simplebase/config.json`（回落 `~/.config/simplebase/`），**权限必须 0600** 否则 `config_insecure` | `internal/sbcli/config.go:19-42` |
| 环境变量覆盖：`SIMPLEBASE_URL` / `SIMPLEBASE_TOKEN` / `SIMPLEBASE_PROJECT_ID` / `SIMPLEBASE_OUTPUT` / `SIMPLEBASE_AGENT_RUN` | 同上 :71-88 |
| 输出信封：stdout 一行 `{"ok":true,"data":...}`，stderr 一行 `{"ok":false,"error":{...}}`，退出码 `0/1/2/4` | `internal/sbcli/output.go:10-39` |
| `build.sh` **只编一个目标**，无 `-ldflags`、无 Makefile、无版本变量 | `build.sh:307-310` |
| `build.sh` 的 `case` 只认 `-h`/`dev`/`build`/`reset` | `build.sh:775-800` |
| 全仓 **零命中** `jsonl` / `~/.gocloudagent` / `web_search` / `bash` 工具 | 已核查 |

结论：**`~/.gocloudagent/` + jsonl 会话是全新东西**，没有可复用的本地会话持久化层；云端会话只在系统库（`sys_agent_threads` / `sys_agent_messages` / `sys_agent_runs`）。

### 1.6 前端现状

| 对象 | 位置 | 要点 |
| --- | --- | --- |
| 路由 | `ui/src/router/index.ts:73-78` `cron-jobs` → `pages/CronJobs.vue`；`agents` → `pages/AgentManager.vue`（Agent 定时**没有独立路由**，内嵌在 Agent 页） | |
| 页面 | `pages/CronJobs.vue`（319 行，`api.cronjobs.*`） | 调度人类化文案 `scheduleText` :234-242 |
| Agent 定时弹窗 | `components/ai/AgentScheduleModal.vue`（12KB），`api.agentSchedules.*` | 提示词是裸 `Textarea` + 4 行，**无长度限制**（:14-19）；错误提示用裸 `<p class="text-destructive">`（:44），未走 `FieldError` |
| 云函数表单 | `components/modal/CronJobModal.vue` + `components/cronjob/useCronJobForm.ts` + `CronJobScheduleFields.vue` | 「选择云函数」是 `ui/select` 下拉两级联动（文件 → 导出函数），数据源 `api.gofunctions.list`，只列 `published && activeVersion > 0`（`useCronJobForm.ts:240-248`） |
| 类型 | `services/types.ts`：`CronJobItem`（camelCase，:664-690）、`CronJobWire`（:712-732）、`AgentSchedule`（**snake_case**，:535-548） | 两套命名风格并存 |
| 长度校验 | 全 `ui/src` 只有 `CreateProjectModal.vue` 两处 `maxlength`；其余靠校验函数 + `FieldError` | `Textarea` 组件 props 只有 `class/defaultValue/modelValue`，**不透传 `maxlength`** |
| 测试 | `ui/tests/{CronJobs,CronJobModal,CronJobRunsModal,AgentScheduleModal,AgentManager}.test.ts` | 桩统一走 `src/test/api-mock.ts` |
| coverage | `ui/vite.config.ts:40-45`，四项阈值均 95；当前 functions 基线 94.24%（未达标） | |

16KiB 限制需要**三处同改**：后端校验 + 前端校验/字数提示 + 错误码映射（`utils/agent-errors.ts` 风格）。

### 1.7 `pi-readme.md` 的能力清单与本仓库缺口

`pi-readme.md` 描述的是 pi（`earendil-works/pi`，commit `3daa2028`）的完整形态，全文六章。它的 §3.3 已经把 Pi 与本仓库逐项对照过一次（26 行表），但那是**描述差异**，不是**待办清单**。本节把它转成可执行的能力清单。

**先划清界限**：pi 是 npm monorepo（11 个 package、Node ≥22.19、TUI + RPC + SDK + Web 无前端），本仓库是单 go.mod 的 Go 服务。**不能按包对照移植**。可借鉴的是**机制形状**，不是代码。

Pi 的十一项机制，逐项判定：

| # | Pi 机制 | Pi 的实现 | 本仓库现状 | 判定 | 落点 |
| --- | --- | --- | --- | --- | --- |
| P1 | **工具管道六段式** | `prepareToolCall`：查找 → `prepareArguments` → `validateToolArguments` → `beforeToolCall` → 检查 abort → `execute` → `afterToolCall`（`agent-loop.ts`，`pi-readme` §2.3） | 校验靠 `utils.InferTool`；阻断靠 `errorTolerantTool` + `destructiveArgv` 塞在单个 CLI 工具内；无 `afterToolCall` | **采用** | §13.1 |
| P2 | **阻断是一等结果** | `beforeToolCall` 返回 `{block:true, reason}`，reason 成为错误文本，可选 `terminate` | 无通用钩子；`errorTolerantTool` 只把 error 转 JSON | **采用** | §13.1 |
| P3 | **三层事件分别命名** | 供应商流 `AssistantMessageEvent`（`text_delta`/`toolcall_delta`/`toolcall_end`/`done`）／agent 流 `AgentEvent`（`tool_execution_start/update/end`）／durable 流（commit 后投影） | 只有一层 `Event`；`tool_call_delta` 与 `tool_call` 混用同一名字空间；handler 的 `thinking` 是空闲心跳、**名字撞车**模型推理（`pi-readme` §3.3 已记录） | **部分采用** | §13.2 |
| P4 | **`tool_execution_update` 部分结果** | 工具通过 `onUpdate` 推增量 | 无。handler 每 2 秒按已开始的 call 补一帧 `tool_progress`（伪增量） | **采用**（仅沙盒与 CLI 长命令） | §13.2 |
| P5 | **执行模式与顺序保证** | 默认 parallel，任一工具声明 `sequential` 则整批顺序；`stopReason === "length"` 时**不执行**工具，给每个 call 一条错误结果（`failToolCallsFromTruncatedMessage`） | ADK `ToolsNode` 默认并行；`length` 场景无处理 | **采用** | §13.1 |
| P6 | **会话树（`id`/`parentId`/`leafId`）** | `SessionManager`，`CURRENT_SESSION_VERSION = 3`，`~/.pi/agent/sessions/--<path>--/<ts>_<id>.jsonl`；`getBranch`/`branch`/`resetLeaf`/`forkFrom`/`getTree` | 系统库线性表（`AgentMessage` 无 `parent_id`）；历史靠 `maxHistoryChars=12000` 截断 | **CLI 采用，云端暂缓** | §13.4 |
| P7 | **压缩（compaction）** | `shouldCompact`（`contextTokens > contextWindow - reserveTokens`）、`prepareCompaction`（按 `keepRecentTokens` 找切点，**切点落在完整 turn**）、`compact`（调模型写摘要）；默认 `reserveTokens=16384` / `keepRecentTokens=20000`；原文不删，只是不再进上下文 | **无压缩**，超限靠截断，截断点不保证落在完整工具回合（`pi-readme` §3.3 明确记录） | **采用** | §13.5 |
| P8 | **Skills 按需读** | 提示里只有 name/description/path，正文靠 `read` 或 `/skill:name`；`disable-model-invocation: true` 不进提示 | `SkillPrompt` 把选中子技能**全文**注入（上限 8192 rune） | **采用** | §4.2 + §13.6 |
| P9 | **Prompt templates** | `~/.pi/agent/prompts/`，`$1`/`$@`/`${1:-default}` 替换，`expandPromptTemplate` 在用户消息落地前展开 | 无 | **CLI 采用，云端不做** | §13.7 |
| P10 | **多外壳共用一套会话** | 同一 `AgentSession` 服务 TUI / print / JSON / RPC / SDK 五种模式（`pi-readme` §2.8） | 只有 SSE 一个出口（`agent_handler.go`） | **部分采用** | §13.8 |
| P11 | **行为评估 harness** | `packages/evals`：`createPiCodingAgentHarness` 在临时目录建 session、逐步 prompt、收集 messages；`createTaskPlan` 展开成 `(case, variant, repetition)` | 只有协议层 e2e（`agent_skills_e2e_test.go` 断言 SSE 顺序），无行为层评估 | **采用**（缩小版） | §13.9 |

另有两项**必须明确不引入**（`pi-readme` §3.2 已给理由，在此复述并强化）：

| Pi 做法 | 不引入的理由 |
| --- | --- |
| **默认无授权**（bash/edit/write 直接以进程用户执行，`pi-readme` §2.7：Pi 没有内置的文件系统/进程/网络/凭据权限系统，也不在每次工具调用前询问，唯一门是项目信任） | SimpleBase 的破坏性 CLI 必须确认、定时任务（`Headless`）必须拒绝（`cli_tool.go:221-261`）。把 Pi 的「问一次项目是否可信」搬过来会**放宽**现有保护。**保留 `destructiveArgv` + `ConfirmationWaiter` + Headless 拒绝** |
| **Codemode（QuickJS WASM 沙箱）** + **扩展与工具同进程** | `pi-readme` §3.2：「云 Agent 不能加载任意 TypeScript。技能只能约束到已经存在的 HTTP API」。且 `pi-readme` §2.6 记录 Pi 的 `tool_call` 扩展事件**改参后不再复校**，这对云 Agent 是不可接受的。SimpleBase 的沙盒边界走既有 `Sandbox` 接口（microVM），不进 WASM |

**唯一需要在本计划里立刻处理的「名字撞车」**（P3 的直接后果）：`handler` 的 `thinking` 事件当前含义是「2 秒没有 token 也没有工具」，而 Pi 的 `thinking_delta` 是模型推理增量。两者不能共用 `thinking`。本计划**不改现有 `thinking` 语义**（前端已依赖），若要暴露推理增量，必须新起 type（如 `reasoning_delta`）且默认关闭——见 §13.2。


---

## 2. 议题 A：定时任务能力统一到 cronjob

### 2.1 目标形态

```
                      ┌──────────────────────────────┐
UI / API  ──────────► │  CronJobHandler              │
（统一 /cron-jobs）    │  target_kind = function|agent│
                      └──────────────┬───────────────┘
                                     │ systemdb
                      ┌──────────────▼───────────────┐
                      │ sys_cron_jobs                │
                      │ + target_kind/agent_id/      │
                      │   agent_thread_id/agent_prompt│
                      └──────────────┬───────────────┘
                                     │ 扫表 / CAS
                      ┌──────────────▼───────────────┐
                      │ cronjob.Scheduler（只管调度） │
                      │  tick·claim·inFlight·落库    │
                      └──────────────┬───────────────┘
                                     │ Executor 接口（注入）
            ┌────────────────────────┼────────────────────────┐
            ▼                                                 ▼
 ┌──────────────────────┐                       ┌──────────────────────┐
 │ functionExecutor      │                       │ agentExecutor        │
 │ (app 层，现有          │                       │ (新包 internal/       │
 │  systemDBRunner)      │                       │  agentjob)           │
 │ → gofunction.RunJSON  │                       │ → gocloudagent.Runtime │
 └──────────────────────┘                       └──────────────────────┘
```

**核心原则**：`cronjob` 只管「什么时候跑、能不能跑、跑结果怎么落库」，**不认识** Agent / gofunction / gocloudagent；「跑什么」由注入的 `Executor` 决定。这既消除了两份同构实现，又保住了 `cronjob` 零内核依赖的现状。

### 2.2 数据模型：扩展 `sys_cron_jobs`，不回填旧表

**决策 D-A1**：把 `sys_cron_jobs` 扩展成统一任务表，`sys_agent_schedules` / `sys_agent_schedule_runs` 冻结为归档表（只读保留，不删表、不回填）。

理由：`sys_cron_jobs` 是能力超集（三态调度 + 快照 + 软删 + 审计），`sys_agent_schedules` 是子集；反向扩展（给 agent 表加 interval/once/快照）要改的代码更多，且会留下第二张带 `name` 唯一性语义缺失的表。

**迁移 v49**（新版本号，接 `migrate.go` 现有最大 v48）：

```go
{
    // v49：统一定时任务目标（planv6.0 gocloudagent-cli-and-cronjob-unification-plan §2.2）。
    // target_kind=function 时沿用 func_file/func_export；=agent 时用 agent_id/agent_prompt。
    // DuckLake 不支持 ADD COLUMN NOT NULL，全部用带 DEFAULT 的可空列；
    // 列追加在末尾，避免 scanCronJob 的按位扫描错位（cronJobColumns 同步更新）。
    version: 49,
    name:    "sys_cron_jobs_target_kind",
    stmt: `ALTER TABLE sys_cron_jobs ADD COLUMN target_kind VARCHAR DEFAULT 'function';
        ALTER TABLE sys_cron_jobs ADD COLUMN agent_id VARCHAR DEFAULT '';
        ALTER TABLE sys_cron_jobs ADD COLUMN agent_thread_id VARCHAR DEFAULT '';
        ALTER TABLE sys_cron_jobs ADD COLUMN agent_prompt VARCHAR DEFAULT ''`,
},
```

**同步改动点**（漏一个就会按位扫描错位）：

| 位置 | 改动 |
| --- | --- |
| `internal/systemdb/migrate.go` | 新增 v49（上述） |
| `internal/systemdb/cron_jobs.go` | `CronJob` 结构体加 4 字段；`cronJobColumns`（:30-34）末尾追加 4 列；`scanCronJob`（:368）按同序加 4 个 `&` 目标；`CreateCronJob` / `UpdateCronJob` 的 INSERT/UPDATE 列表同步 |
| `internal/systemdb/cron_jobs.go` | `ListDueCronJobs` 的 WHERE **不加** `target_kind` 过滤（两种目标都要扫） |
| `internal/api/cronjob_handler.go` | `cronJobDTO` 加 4 个 JSON 字段；`toCronJobDTO` 映射 |

**旧表处理**：

- 不 DROP、不改结构，保留为历史归档；
- 旧 API `/agent-schedules*` 保留**一个版本周期**作为迁移期只读兼容入口（见 §2.6），内部读 `sys_agent_schedules`，不再写；
- 历史 `sys_agent_schedule_runs` 的 run 记录**不回填**（`sys_cron_job_runs` 没有 `run_id`/`thread_id` 列，强行回填需要再加两列且会污染统一表的行语义）。这是明确取舍：CronJobs 页只展示迁移后的新 run；旧 run 需要查看时用归档表查询（UI 暂不提供入口，在 §12 Q4 交给用户决定）。

> **约束重申**：本节迁移定义随代码提交，**由用户手动执行**，或在用户以 `TIMER_NO_RUN=0` 正常启动服务时由启动流程应用。代理不代为执行任何 DDL。

### 2.3 `cronjob` 接口改造

把 `Runner` 换成按目标分发的 `Executor`，并保留 `Store` 不变（`Store` 的方法都是实体无关的通用操作）：

```go
// Executor 执行一条到期任务。由上层按 target_kind 实现（app/agentjob）。
// 实现方负责：读目标、执行、写 run 记录与指标；返回给调度器用于状态映射。
type Executor interface {
    // Run 执行一次。返回的 Result 供调度器做统一状态/快照落库。
    // err != nil 表示失败；调度器按 ctx.Err() 与错误文本映射 failed/canceled。
    Run(ctx context.Context, j systemdb.CronJob, trigger string) (Result, error)
}

// Result 是一次执行的落库摘要。
type Result struct {
    // Response 是回显给 UI 的响应体（调度器负责按 runResponseLimit 截断）。
    Response string
    // Metrics 是 agent 类型专有的用量指标；function 类型留零值。
    Metrics RunMetrics
}

type RunMetrics struct {
    DurationMS       int64
    PromptTokens     int
    CompletionTokens int
    ReasoningTokens  int
    ToolCalls        int
    ErrorCode        string
}
```

`Scheduler.execute` 改为：

```go
start := time.Now()
res, runErr := s.Exec.Run(ctx, j, trigger)
duration := time.Since(start).Milliseconds()
// 错误映射与状态落库保持现有逻辑（:227-239）
// 若 res.Metrics 非零，调用可选接口写 sys_agent_runs 指标（见 §2.4）
```

**保留不变**（不得在本次改动中变动，见 `backend-code-simplification-plan.md` §2「不强行统一」的边界被本次用户指令覆盖，但**行为**仍不得变）：

- `Scheduler.Trigger` 的 `stopped` 判定与 `wg.Add` 同锁语义（LIFE-02）；
- `Scheduler.Start` 的 `sctx` 与手动任务解耦语义（LIFE-01）；
- `ClaimCronJob` 的 CAS 语义与 `nextRun` 的三态推进（含 interval 以认领时刻为基准防漂移）；
- `once` 执行后自动停用；
- 坏 cron 停用并告警；
- 失败状态里 `"context canceled"` → `canceled` 的文本判定（保持现状，不趁机改成 `errors.Is`——那属于独立修复项）；
- `updateJobSnapshot` 的「先重取再合并」防覆盖逻辑。

### 2.4 新增包 `internal/agentjob`

职责：**把一条 Agent 定时任务执行成一次 Agent run**。这是从 `gocloudagent/scheduler.go:209-330`（`execute` 主体）平移过来的代码，加上原属 `ScheduleStore` 的 Agent 侧 store 调用。

```
internal/agentjob/
  executor.go        // Executor 实现：Execute(ctx, job, trigger)
  executor_test.go   // 从 gocloudagent/scheduler_test.go 平移 + 适配
  access.go          // 接口声明（Store / Runner / Quota）
```

**接口声明**（避免 import 环，且让 fake 好写）：

```go
package agentjob

// Runner 抽象一次 Agent run；*gocloudagent.Runtime 天然满足。
type Runner interface {
    StartRun(ctx context.Context, req gocloudagent.RunRequest, emit func(gocloudagent.Event)) (gocloudagent.RunResult, error)
}

// Store 是本包需要的系统库操作（只列实际用到的）。
type Store interface {
    GetCloudAgent(ctx context.Context, projectID, id string) (systemdb.CloudAgent, error)
    GetAgentThread(ctx context.Context, projectID, id string) (systemdb.AgentThread, error)
    CreateAgentThread(ctx context.Context, t systemdb.AgentThread) (systemdb.AgentThread, error)
    AppendAgentMessage(ctx context.Context, msg systemdb.AgentMessage) (systemdb.AgentMessage, error)
    ListAgentMessages(ctx context.Context, projectID, threadID string, limit int) ([]systemdb.AgentMessage, error)
    CreateAgentRun(ctx context.Context, r systemdb.AgentRun) (systemdb.AgentRun, error)
    UpdateAgentRunStatus(ctx context.Context, projectID, id, status, errMsg string, started, finished *time.Time) error
}

// Quota 抽象配额检查；*usage.Service 满足。
type Quota interface {
    CheckQuota(ctx context.Context, projectID string, kind string) error
}

// JobUpdater 让 executor 回写任务的 thread 绑定（thread 被归档后重建的场景）。
type JobUpdater interface {
    GetCronJob(ctx context.Context, projectID, id string) (systemdb.CronJob, error)
    UpdateCronJob(ctx context.Context, j systemdb.CronJob) (systemdb.CronJob, error)
}
```

**保留的 agent 专有行为**（逐条从 `gocloudagent/scheduler.go` 平移，行为不变）：

| 原位置 | 行为 | 新位置 |
| --- | --- | --- |
| :226-231 | 配额不足 → `failed` + `llm quota exceeded`，**不调用 Runner** | `agentjob.Execute` 开头 |
| :233-243 | agent 不存在 → `failed` + 停用任务（`Enabled=false`） | 同 |
| :239-243 | `NormalizeSkills(nil, agent.Module)` 失败 → `failed`；成功则作为 `RunRequest.Skills` | 同（**P1 修复点**：原 bugfix 计划 §2 的 P1 已完成，此处保持） |
| :245-264 | thread 不存在 → 重建（Title `"Scheduled: " + agent.Name`，`CreatedBy` 为调度身份）并**回写任务的 thread 绑定** | 用 `JobUpdater` 回写 `agent_thread_id` |
| :261-264 | thread 属于别的 agent → `failed`，不产消息 | 同 |
| :266-272 | 只读 principal：`APIKeyID = "system-scheduler"`，只带 `DatabaseRead` | 同（常量迁到 `agentjob`） |
| :274-290 | 创建 `AgentRun`、插入 `[scheduled] ` 前缀的 user 消息、列表历史时**排除刚插入的这条** | 同 |
| :296-300 | `StartRun` 带 `Stream:false`、`Headless:true` | 同 |
| :303-314 | `UpdateAgentRunMetrics` 的**可选类型断言**（fake 无此方法时跳过） | 保留断言；`Metrics` 经 `Result` 回传给调度器统一写 |
| :376-386 | `sanitizeErr` 300 字符截断 | 迁到 `agentjob`（`cronjob` 已有 `truncate`，两者语义不同：一个是错误专用，一个是通用） |

**指标写入的收口**：现在 `UpdateAgentRunMetrics` 在 `gocloudagent/scheduler.go` 用类型断言直接打 store，改动后由调度器统一处理——即 `Result.Metrics` 非空且 `Store` 实现 `MetricsWriter` 时才写：

```go
type MetricsWriter interface {
    UpdateAgentRunMetrics(ctx context.Context, projectID, runID string, m systemdb.AgentRun) error
    UpdateAgentRunStatus(ctx context.Context, projectID, id, status, errMsg string, started, finished *time.Time) error
}
```

> 注意：`agentjob` 需要 `UpdateAgentRunStatus`（run 状态），而 `sys_cron_job_runs` 的 `run_id` 列不存在。因此 agent 类型的 run 状态**仍然写 `sys_agent_runs`**（这是 Agent 会话的正确归属），`sys_cron_job_runs` 只记「调度执行了一次」。两者通过 `job_id` + 时间对齐。这一点必须在实现里写清注释，避免后人误以为重复落库。

### 2.5 app 装配改动

`internal/app/app.go` 当前（:247-268）创建两个调度器，改动后只创建一个：

```go
// 统一定时任务调度器（planv6.0 §2.5）：一个 Scheduler，两类 Executor。
if a.systemStore != nil {
    var exec cronjob.Executor = &functionExecutor{store: a.systemStore} // 原 systemDBRunner 改名
    if runtime != nil {                                                  // 仅运行时齐备时挂 Agent 能力
        exec = cronjob.NewRouter(&functionExecutor{store: a.systemStore},
            agentjob.New(a.systemStore, runtime, a.usageSvc))
    }
    cronScheduler = cronjob.NewScheduler(a.systemStore, exec)
    a.cronScheduler = cronScheduler
    // ctx 惰性单例逻辑保持
    cronScheduler.Start(a.schedulerBaseCtx)
}
```

改动清单：

| 位置 | 改动 |
| --- | --- |
| `app.go:81-82` | 删除 `agentScheduler *gocloudagent.Scheduler` 字段；`cronScheduler` 保留 |
| `app.go:247-257` | 整段删除（agent scheduler 装配） |
| `app.go:259-268` | 改为上述统一装配 |
| `app.go:785-800` | 删除 `a.agentScheduler.Stop()` 分支；保留 `cronScheduler.Stop()` 分支（:801-813） |
| `app.go:307-308` | `api.Dependencies.AgentScheduler` 字段删除；`CronScheduler` 保留 |
| `app.go` `cloudAgentRuntime()`（:933-975） | **不变** |
| `internal/api/router.go:67-70` | 删除 `AgentScheduler *gocloudagent.Scheduler` 字段 |
| `internal/api/router.go:541` | `agentScheduleHandler` 构造去掉 `scheduler` 依赖（改为只读兼容层） |
| `internal/app/cron_runner.go` | 类型改名 `systemDBRunner` → `functionExecutor`，方法签名适配 `cronjob.Executor` |

**新增 `cronjob.NewRouter`**（或在 app 层写一个 15 行的 router）：按 `j.TargetKind` 分发到两个 Executor，`TargetKind` 为空或 `function` 走 function，`agent` 走 agent，其他值返回错误（不静默降级）。

**顺带修一个既有缺口**：探索发现两个 scheduler 都**没有**走 `registerCloser`，`Close()` 只做 `schedulerCancel()` 而不调 `Stop()`（`app.go:723-744`），所以 `stopped` 不会被置位、`wg.Wait` 不执行。统一后只在 `Shutdown` 保留一条 `Stop()` 分支，并在 `Close` 路径补上同样的 `Stop()` 收敛（或明确注释「Close 由 Shutdown 兜底」）。**这是一个行为变更点，必须写进 PR 描述并补测试**。

### 2.6 API 层改动

**CronJobHandler**（`internal/api/cronjob_handler.go`）：

- `cronJobDTO` 追加：

  ```go
  TargetKind    string `json:"target_kind"`              // "function" | "agent"
  AgentID       string `json:"agent_id,omitempty"`
  AgentThreadID string `json:"agent_thread_id,omitempty"`
  AgentPrompt   string `json:"agent_prompt,omitempty"`
  ```

- 新增常量与错误码：

  | 常量 | 值 | 用途 |
  | --- | --- | --- |
  | `cronJobAgentPromptLimit` | `16 * 1024` | **用户明确要求的 16K 上限**（字节口径，与 `cronJobInputLimit` 的 `len()` 一致） |
  | `CronJobTargetFunction` | `"function"` | |
  | `CronJobTargetAgent` | `"agent"` | |

  | 错误码 | HTTP | 触发 |
  | --- | --- | --- |
  | `invalid_target_kind` | 400 | `target_kind` 不是 function/agent |
  | `agent_not_found` | 400 | `agent_id` 在该项目不存在 |
  | `agent_prompt_required` | 400 | `target_kind=agent` 但 prompt 为空 |
  | `agent_prompt_too_long` | 400 | `len(agent_prompt) > 16*1024` |

- `validateTarget`（:483）按 `target_kind` 分支：function 走现有 `ResolveActiveSource` 校验；agent 走 `GetCloudAgent` 存在性校验。**不要在 agent 分支里复用 gofunction 的校验**。
- `isTargetMissing` / `buildTargetIndex`（:503-531）同样按 kind 分支。function 保留现有 index 构建；agent 用 `ListCloudAgents` 建集合。**注意**：agent 类型必须**逐条校验**而非事后过滤，否则列表页会展示指向已删除助手的任务（`TargetMissing` 在 UI 上用于提示）。
- `Trigger`（:381）保持 `DatabaseWrite` + writeGate；agent 任务额外做一次配额检查（原 `TriggerScheduleRun` 有 `usage.CheckQuota(ctx, pc.ID, "llm")`，见 `agent_schedule_handler.go:335`）。
- **补审计**：agent 类型任务的写操作必须 `recordAudit`（与 function 类型一致）——这是现 `agentScheduleHandler` 缺失的一项，统一后自然补上。

**AgentScheduleHandler**（`internal/api/agent_schedule_handler.go`）降级为**迁移期只读兼容层**：

- `GET /agent-schedules` → 把 `sys_agent_schedules` 的启用行映射成 `agentScheduleDTO`（保持现有 JSON 形状，**不改字段名**，避免前端一次性切换的回归）；
- `GET /agent-schedules/:scheduleID/runs` → 保留；
- `POST` / `PATCH` / `DELETE` / `POST .../run` → 返回 `410 Gone` + `agent_schedules_merged`，并在响应体里给 `migrated_to: "/cron-jobs"`。

理由：前端分两步切换（先加新入口，再删旧入口）比一次性切换安全；`410` 比静默 404 更容易定位。

**路由**（`router.go:540-550`）：

- 删除 `deps.AgentScheduler` 的 nil 判定分支（:548-550）；
- 兼容层的 4 个写路由删除，留 2 个只读路由；
- 一个版本周期后在下一个 plan 中删除整个兼容层。

### 2.7 需要删除 / 迁移的文件清单

| 文件 | 行数 | 处理 |
| --- | --- | --- |
| `internal/gocloudagent/scheduler.go` | 386 | **删除**（执行体迁 `internal/agentjob/executor.go`） |
| `internal/crontab/crontab.go` 的注释 | — | 更新：注释写着「从 `internal/gocloudagent/cron.go` 迁移而来，gocloudagent 保留薄封装兼容」，薄封装已不存在 |
| `internal/api/agent_schedule_handler.go` | 349 | 缩减为兼容层（保留 DTO + 2 个只读 handler） |
| `internal/api/agent_schedule_handler_test.go` | — | 保留只读路径用例，删除 trigger/CAS 用例（迁到 `agentjob` + `cronjob`） |

---

## 3. 议题 B：删除 `gocloudagent` 的三个 scheduler 测试文件

这是议题 A 的必然结果——被测对象已经不存在了。

| 文件 | 行数 | Test 数 | 去向 |
| --- | --- | --- | --- |
| `internal/gocloudagent/scheduler_test.go` | 477 | 10 | 见下表逐条 |
| `internal/gocloudagent/scheduler_more_test.go` | 271 | 4 | 见下表 |
| `internal/gocloudagent/scheduler_lifecycle_test.go` | 156 | 4 | **直接删除**（与 `cronjob/scheduler_lifecycle_test.go` 逐行相同，只保留 cronjob 那一份） |

逐条去向：

| 原 Test | 场景 | 处置 |
| --- | --- | --- |
| `TestSchedulerScheduledSuccess`（:217） | 到期→claim→执行，断言 `RunRequest` 的 skills/principal/Stream/Headless | **迁 `agentjob`**：断言不变（principal 仍 `system-scheduler`、权限仍 `database_read`、`Stream=false`、`Headless=true`）；`[scheduled]` 前缀与落库断言改为经 fake `JobUpdater` + fake `Store` |
| `TestSchedulerQuotaExceeded`（:313） | 配额失败不调 Runner | **迁 `agentjob`** |
| `TestSchedulerAgentArchivedDisablesSchedule`（:334） | agent 不存在→停用 | **迁 `agentjob`**（停用改为断言 `JobUpdater.UpdateCronJob` 被调用且 `Enabled=false`） |
| `TestSchedulerThreadArchivedRecreates`（:355） | thread 不存在→重建+回写 | **迁 `agentjob`**（回写断言改 `agent_thread_id`） |
| `TestSchedulerRejectsCrossAgentThread`（:379） | 跨 agent thread 拒绝 | **迁 `agentjob`** |
| `TestSchedulerBadCronDisablesSchedule`（:393） | 坏 cron 停用 | **删除**（`cronjob/scheduler_test.go:239` 已覆盖同场景，且现在只有一份 `runDue`） |
| `TestSchedulerClaimLostSkips`（:414） | CAS 失败不执行 | **删除**（`cronjob/scheduler_test.go:221` 已覆盖） |
| `TestSchedulerManualTriggerKeepsNextRun`（:433） | 手动触发不改排期 | **删除**（`cronjob/scheduler_test.go:285` 已覆盖） |
| `TestSchedulerManualTriggerInFlightRejected`（:463） | 执行中再触发被拒 | **删除**（`cronjob/scheduler_test.go:313` 已覆盖） |
| `TestSchedulerStartStopAndTickErrors`（more :81） | nil/未配置 Start/Stop、tick 覆盖、list 报错写日志 | **部分合并**：tick/日志部分删除（cronjob 已覆盖）；**保留**「nil 调度器与未配置不 panic」并在 `cronjob` 侧补 `TargetKind` 路由的 nil 分支 |
| `TestSchedulerExecuteErrorBranches`（more :124） | 8 个子测的错误分支 | **拆分**：`insert run 失败` / `runner 失败落 failed` / `context canceled 落 canceled` → 合并进 `cronjob/coverage_test.go`（已同构）；`create run 失败` / `append user 失败` / `list messages 报错仍执行` / `重建 thread 失败` / `重建后 update 失败` / `disableSchedule update 失败` → **迁 `agentjob`** |
| `TestSchedulerRunDueNextAfterAndClaimError`（more :216） | 不可能 cron→停用 | **删除**（`cronjob` 同构覆盖） |
| `TestSchedulerTriggerAndInFlightHelpers`（more :236） | nil Trigger、inFlight 上限与 release、`sanitizeErr` | **拆分**：inFlight 部分删除（cronjob 覆盖）；`sanitizeErr` 迁 `agentjob` 测试 |
| `TestScheduler_StopCancelsManualTrigger` / `_TriggerAfterStop` / `_StopTriggerConcurrent` / `_ManualTriggerSurvivesRequestCancel` | LIFE-01/02 生命周期 | **全部删除**（cronjob 三份逐行相同） |

**净效果**：删除约 904 行测试，新增 `internal/agentjob/executor_test.go` 约 350 行 + `internal/cronjob/scheduler_test.go` 追加约 80 行。**测试总数下降但覆盖的独立行为不减少**——减少的是同场景在两份实现上的重复。

**验收门槛**：`go test -race -short -count=3 ./internal/cronjob ./internal/agentjob` 通过，且 `cronjob` 包新增的 `TargetKind` 分发分支有独立用例（function / agent / 非法值三态）。

---

## 4. 议题 C：基于 `eino-readme.md` 重新组织 `gocloudagent`

### 4.1 目标架构：从 13 个平铺文件到四层

当前 13 个生产文件是**平铺**的，职责边界靠命名区分。问题在于：`runtime.go`（713 行）同时承担「eino 装配」「事件循环」「历史投影」「错误归一」「线程锁」，而 CLI 需求（议题 D）需要复用其中「与运行时无关」的部分却拿不到（`gatewayChatModel` 是私有类型，`Event` 混了 `RunID`/`ToolCallsJSON` 等云端字段）。

**决策 D-C1**：按**运行时依赖**切三层，而不是按功能切。

```
internal/gocloudagent/            ← 云端专用：只保留「项目/主体/系统库」这三件事的耦合
  runtime.go      StartRun（装配 adk，消费 agentcore 的事件）
  access.go       DatabaseAccess / ObjectAccess / LogAccess / SettingsAccess / Sandbox
  tools.go        7 个只读数据工具 + 4 个沙盒工具 + RunContext
  cli_tool.go     simplebase 工具（委托令牌 + argv 白名单 + 破坏性确认）
  cli_runner.go   CLIRunner（原样迁）
  skills.go       skill 前缀表 + `@` 解析 + AllowArgv
  modules.go      模块目录（原样迁）
  trace.go        tracer（原样迁）
  store_access.go （原样迁）

internal/gocloudagent/agentcore/  ← 子包：与运行时无关的通用层，云端 Runtime 与本地 CLI 共用
  types.go        ChatClient / ChatRequest / ChatResponse / DeltaStream（网关形状）
  model.go        Model（原 gatewayChatModel）+ text/native/auto 协议
  stream.go       事件消费与增量合并（ConsumeVariant / ConsumeAssistant / IsToolDelta）
  redact.go       RedactSecrets + Truncate
  messages.go     schema 消息构造辅助

internal/gocloudagent/cli/        ← 子包（议题 D）：命令行外壳
  command.go      子命令分派（run / resume / sessions / config）
  repl.go         交互式 REPL + 流式渲染
  runner.go       SessionRunner（装配 adk，产出 CLIEvent）
  session.go      jsonl 会话树（~/.gocloudagent/sessions/*.jsonl）
  client.go       HTTPClient（实现 agentcore.ChatClient，走 /llm/chat 与 /llm/stream）
  config.go       ~/.gocloudagent/config.json（0600 强制）
  prompt.go       prompt templates（~/.gocloudagent/prompts/*.md）

internal/gocloudagent/tools/      ← 子包（议题 D）：本机工具
  tools.go        Workspace + shell / read_file / write_file / edit_file / glob / grep
  search.go       SearchClient（duckduckgo 免 key / brave 需 key）
  fsutil.go       globWalk / grepWalk（自实现，不引 doublestar）
```

**命名说明**：父包的 `cli_tool.go` / `cli_runner.go` 是**云端**把 `simplebase` CLI 当作工具调用（委托令牌那套）；子包 `cli/` 是**本地**命令行外壳。两者都叫 "cli" 但方向相反——前者是云端调 CLI，后者是 CLI 调云端。阅读时以目录层级区分（`gocloudagent/cli/` 才是外壳）。

**决策 D-C1**：按**运行时依赖**切三层，而不是按功能切。子包都挂在 `gocloudagent` 下（而不是平级新包），因为它们复用的正是 `gocloudagent` 的技能授权（`AllowArgv`）、提示词约定与事件语义；平级会让人误以为是独立模块。


**不做的拆分**（当前过度设计的反例）：

- 不为 `Event` 造两套类型——`agentcore.Event` 放通用字段，`gocloudagent` 在自己的 `RunResult` 里补云端字段（`RunID`、`ToolCallsJSON` 已经是 `RunResult` 的字段，不是 `Event` 的，现状是对的）；
- 不引入 `SessionView` / `ContextBuilder` 接口（`pi-readme.md` §4.2 的目标形状）——云端的会话投影现在只有一个实现（`historyToSchema`），造接口是纯转发层；
- 不新建 `loop.go` 自研 `RunLoop`——`pi-readme.md` §4.10 把「自有 loop 替换 `runner.Run`」列为步骤 4，成本高收益低，且会引入「两套 `MaxIterations`、两套错误码」的双轨风险（其 §4.11 已列为风险项）。**本计划不做自有 loop**，改为用 ADK 中间件解决问题（§4.2）。

### 4.2 中间件化：用 ADK 能力替换四项自研

`eino-readme.md` §9.2 的 `ChatModelAgentConfig.Handlers`（`[]ChatModelAgentMiddleware`）与 `adk/middlewares/*` 是 v0.9.19 已提供的能力。逐项对照 gocloudagent 现状：

| 现状 | 自研实现 | 目标 | 决策 |
| --- | --- | --- | --- |
| Skill 正文注入 | `SkillPrompt` 把选中子技能**全文**拼进 Instruction（`skills.go:134-150`，上限 8192 rune） | `adk/middlewares/skill`：提示里只有 name/description，正文靠 `skill` 工具按需读 | **采用**（见下方 Backend 实现说明） |
| 历史截断 | `historyToSchema` 从近往远装，预算 `maxHistoryChars=12000`，单条截 2000（`runtime.go:514-546`） | `adk/middlewares/summarization`：按 token 阈值摘要，保留原文 | **采用**（依赖 `sonic`，已在 go.mod） |
| 工具结果过长 | `tool_result` 事件截 4000 rune（`runtime.go:286-288`） | `adk/middlewares/reduction`：超长工具输出 offload 到文件 + 清空旧工具结果 | **暂缓**（见下） |
| 破坏性确认 | `callGate.wait/release` 跨 goroutine 握手（`cli_tool.go` + `runtime.go:328-330`） | `adk` 的 `WrapInvokableToolCall` 在 `tool_call` 事件之后、执行之前阻断 | **采用**（这才是 `pi-readme.md` §4.5 的正确落点） |

**关于 `skill` 中间件的 Backend**：

`adk/middlewares/skill.NewBackendFromFilesystem` 要 `adk/filesystem.Backend`，而 gocloudagent 的 skill 在 `//go:embed simplebase`（`skills/embed.go:8`）里，不是 OS 文件系统。两个选项：

- 选项 1：自研 `skill.Backend` 实现，直接读 `embed.FS`（约 40 行：`List` 返回各 `references/*.md` 的 frontmatter，`Get` 返回 `Skill{FrontMatter, Content}`）。**无新依赖**。
- 选项 2：用 `adk/filesystem.NewInMemoryBackend()` + `NewBackendFromFilesystem`。但 `adk/filesystem` 依赖 `github.com/bmatcuk/doublestar/v4`，**该模块不在当前 `go.sum`**（已核查：`grep -c doublestar go.sum` = 0），需要 `go mod tidy` 新增间接依赖。

**决策 D-C2**：选**选项 1**（自研 Backend）。理由是 `AGENTS.md` 硬性约束「不新增第三方依赖」，且 40 行自研代码远小于新依赖的长期维护面。把这一判断记在 `internal/AGENTS.md` 的 gocloudagent 行里。

**关于 `reduction` 中间件（暂缓的原因）**：它也要 `adk/filesystem.Backend`（同上依赖问题），且它依赖 `ReadFileToolName`（offload 后必须有工具能读回来）——云端 Agent 的沙盒才有文件系统，且沙盒路径限制在 `/workspace`。现在 `tool_result` 截断到 4000 rune 是**简单且够用**的。**本计划不引入 reduction**，等 CLI 侧（议题 D，有真实文件系统）验证后再回看。

**关于确认改造的落点**（`pi-readme.md` §4.5 的目标顺序）：

```text
目标顺序（去掉 callGate 的跨 goroutine 握手）：
1. 模型流结束，assistant 消息里已有完整 ToolCall
2. 循环同步 emit `tool_call`（call id 已稳定）
3. 中间件 WrapInvokableToolCall:
     非破坏性 → emit tool_start → 执行 → emit tool_result
     破坏性   → emit confirmation_required → Confirm.Wait → emit confirmation_resolved
                → 未批准：返回错误 JSON，不 emit tool_start
                → Headless：直接拒绝，不发 confirmation_required
4. 执行 → emit tool_result
```

**必须保住的顺序契约**（现有 e2e 断言，见 `internal/api/agent_skills_e2e_test.go` 与 `ui/tests/agent-skills-stream.test.ts`）：`tool_call` → `confirmation_required` → `confirmation_resolved` → `tool_start` → `tool_result`，且 `confirmation_required` 与 `tool_call` 的 call id 相同。`callGate` 删除后这一顺序由**循环同步 emit** 保证——这是改造的关键风险点，必须补「确认拒绝时 CLI 未启动」的用例。

### 4.3 多 Agent 设计

用户要求「如何使用多 agent，如何设计 agent 调度流程」。先说**不做**的，再说做的。

**不做**（上游明确不推荐，`eino-readme.md` §8.2 / §9.4）：

- `adk.SetSubAgents` 的 transfer（全量上下文移交）——父 Agent 自动带 `transfer_to_agent` 工具，子 Agent 拿全家桶上下文。上游源码注释：「全量上下文移交在经验上并不更好」；
- `prebuilt/supervisor`（把子 Agent 包成只能交回 supervisor 的形态）——上游标记不推荐；
- `flow/agent/multiagent/host`——与 ADK transfer 是两条产品线，且它要求 Host 模型把 Specialist 的 `Name` 当工具名调用，与 gocloudagent 的 `@` 点名协议冲突。

**做的：三段式能力分层，按任务复杂度选择。**

| 层级 | 何时用 | Eino 构造 | gocloudagent 落点 |
| --- | --- | --- | --- |
| **L1 单 Agent** | 默认。绝大多数问数、查日志、只读 SQL 场景 | `adk.NewChatModelAgent`（现状） | `runtime.go` 保持，只加中间件 |
| **L2 通用助手 + Specialist 子 Agent** | 跨域任务（「查一下日志里的错误，然后建个定时汇总」）。此时工具列表从「模块默认工具」升级为「`task` 工具 + 通用工具」 | `adk.NewAgentTool(ctx, specialist, opts...)` 挂在父 Agent 的 `ToolsConfig.Tools` 里 | 新增 `specialists.go`：把 8 个 module 各构造成一个 specialist `ChatModelAgent` |
| **L3 顺序/循环编排** | 固定流程（计划→执行→汇总） | `adk.NewSequentialAgent` / `NewLoopAgent` | 仅用于 CLI 的「多步任务」模式（议题 D），云端不用 |

**L2 的具体设计**（这是本次的核心新增）：

```go
// specialists.go
// buildSpecialists 把内置 module 构造成子 Agent，供通用助手按需调用。
// 子 Agent 不共享父的完整历史（WithFullChatHistoryAsInput 不开启），
// 只拿到 task 工具传进来的 request 文本 —— 避免上下文膨胀与跨域串味。
func buildSpecialists(ctx context.Context, deps specialistDeps) ([]tool.BaseTool, error) {
    var out []tool.BaseTool
    for _, m := range Modules(deps.SandboxAvailable) {
        agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
            Name:        m.ID,
            Description: m.Description,   // 会成为 task 的工具描述，模型据此路由
            Instruction: ModuleTemplate(m.ID),
            Model:       deps.Model,      // 同一个 gatewayChatModel（协议一致）
            ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
                Tools: deps.toolsFor(m.ID),   // 按 module 的 DefaultTools 装配
            }},
            MaxIterations: 4,   // 子任务比主任务短
        })
        if err != nil { return nil, err }
        t, err := adk.NewAgentTool(ctx, agent)
        if err != nil { return nil, err }
        out = append(out, t)
    }
    return out, nil
}
```

关键点（全部来自 `eino-readme.md`，逐条已核对）：

1. **子 Agent 必须有非空 `Name` 与 `Description`** ——`NewAgentTool` 把它们当工具名与描述（`adk/agent_tool.go` 注释明确）。`Description` 质量直接决定路由准确率，因此 module 的 `Description` 要改写成「什么时候该用我」而不是「我是什么」。
2. **`EmitInternalEvents`** ——子 Agent 的事件要能冒泡到父 Runner 才能让 UI 看到工具调用。`adk.ToolsConfig.EmitInternalEvents` 控制这一点。**默认关闭**，开启后需要确认 UI 能正确处理 `AgentName != ""` 的事件（现有 `consumeAssistant` 只看 `MessageOutput`，多了一层 Agent 事件需要过滤还是展示，属于实现时要定的细节）。
3. **模型必须能用 `model.WithTools` 调用 option 看到工具** ——`gatewayChatModel` 的 `Generate`/`Stream` 已经用 `model.GetCommonOptions` 合并 option 工具（`model.go:70-71`，`mergeTools(m.bound, options.Tools)`），**现状已正确**。`eino-readme.md` §5.1 警告的「只实现 `WithTools` 方法却忽略 option 的模型在 ADK 里看不到工具」不适用于本仓库。
4. **`MaxIterations` 是三个上限之一**（`eino-readme.md` §13.4）：`react.MaxStep`（本仓库不用）、ADK `MaxIterations`、compose `WithMaxRunSteps`（不用）。父 Agent 与每个子 Agent 各有一份，要分别配置。

**L2 的启用开关**：新增 `Runtime.Specialists bool`（或按 module 自动判定：`len(Modules) > 1 && skills_cli 打开`）。**默认关闭**，先在测试与本地验证路由准确率，再考虑默认开启。理由：`@` 点名协议（`ResolveAtToken` + `NormalizeSkills`）已经提供了「用户显式选能力」的通道，隐式多 Agent 路由是**额外**能力，不应在未验证时替换现有行为。

**Agent 调度流程**（用户问的第二件事），给出端到端时序：

```text
POST /agent-threads/:id/runs
  → CreateRun handler: 解析 mentions/skills → NormalizeSkills → claimThread
  → Runtime.StartRun:
      buildSnapshot（只读快照，按 module）
      AssembleInstruction（platform → capability → module → agent → env → snapshot）
      buildTools（SkillsCLI ? buildSkillTools : buildTools）+（Specialists ? task 工具）
      adk.NewChatModelAgent{Name, Description, Instruction, Model, ToolsConfig, MaxIterations}
      adk.NewRunner{Agent, EnableStreaming=req.Stream}
      runner.Run(ctx, historyToSchema(history) + UserMessage(text))
      事件循环：
        tool 角色      → consumeVariant → tool_result
        assistant 角色 → consumeAssistant → token / tool_call_delta / tool_call
      收尾：ToolCallsJSON / usage / Reason / ClassifyError
  → handler: SSE 写 run / token / … / usage / end；落 sys_agent_messages + sys_agent_runs
```

**中间件插入点**（本次新增，插入到上面的 `adk.NewChatModelAgent` 配置里）：

```go
Handlers: []adk.ChatModelAgentMiddleware{
    skillMW,        // §4.2：skill 目录进提示，正文按需读
    confirmMW,      // §4.2：WrapInvokableToolCall 做破坏性确认
    summarizationMW,// §4.2：token 阈值摘要
},
```

顺序有意义：`Handlers` 按注册顺序处理（`eino-readme.md` §9.2）。`confirmMW` 必须在 skill 之后（skill 只读，不参与确认），`summarizationMW` 放最后（它改的是 state 里的历史）。

### 4.4 分阶段迁移（每步都保持现有 e2e 通过）

| 步骤 | 内容 | 完成标准 |
| --- | --- | --- |
| 0 | 建 `internal/gocloudagent/agentcore`，把 `model.go` / `prompt.go` / `errors.go` / 协议解析**原样搬入**（只改包名与导出名） | `go build ./internal/... ./cmd/...` 通过；`gocloudagent` 测试零改动通过 |
| 1 | `internal/agentjob` 抽出（议题 A 的产物），删除 `gocloudagent/scheduler.go` + 3 个测试文件 | §3 的门槛 |
| 2 | `confirmMW` 替换 `callGate`；`tool_start` 对只读工具也发出 | 现有 SSE 顺序测试通过；新增「确认拒绝时 CLI 未启动」 |
| 3 | `skill` 中间件 + 自研 embed Backend；`SkillPrompt` 改为索引 + 本轮 id 列表 | 子技能全文不再默认进 Instruction；未知 id 仍 `unknown_skill` |
| 4 | `summarization` 中间件替换 `maxHistoryChars` 截断 | 摘要失败时保留原路径（不丢历史）；摘要正文先 `RedactSecrets` 再落库 |
| 5 | `specialists.go`（L2 多 Agent），默认关闭 | 开关打开时 `task` 工具路由到正确 module；关闭时行为与步骤 4 完全一致 |
| 6 | `internal/gocloudagent/cli` + `internal/gocloudagent/tools` + `cmd/gocloudagent`（议题 D） | §5 的门槛 |

**顺序不可对调**：步骤 2/3 会改 Instruction 与事件语义，必须在 `agentjob` 抽走之后做（否则要在两处同步改）；步骤 4 的摘要需要 skill 已改成「按需读」才能正确保留 skill 上下文（`summarization` 的 `PreserveSkills` 配置项正是为此）。

---

**入口依赖约定**（实现时确定）：`cmd/gocloudagent` **只 import `internal/gocloudagent`**，由父包门面（`cli_entry.go` 的 `RunCLI` / `SetCLIVersion`）转发到子包。理由：入口不应知道 CLI 拆成了哪几个子包，否则子包重组会牵动 `cmd`。门面只转发、不含逻辑，也不复制子包类型。

> 有一个必须注意的约束：**子包不得 import 父包**（父包门面 import `cli`，反向就成环）。因此需要同时引用父包（如 `AllowArgv`）与子包（如 `cli.PromptTemplate`）的断言，只能放在父包的测试里——见 `internal/gocloudagent/cli_entry_test.go` 的 `TestPromptTemplateExpansionDoesNotGrantSkillPermission`。

## 5. 议题 D：`cmd/gocloudagent` 独立 CLI

### 5.1 目标与命令面

参考 claude-code / codex 的交互形态，但**命令面要克制**——不要把 `sbcli` 的 72 个命令再包一层。

```
gocloudagent                       # 交互式 REPL（默认命令）
gocloudagent "查一下本地有没有 TODO" # 一次性执行（有参数即 headless）
gocloudagent -p "..."              # 同上，显式写法（对齐 claude-code 的 -p）
gocloudagent run [flags] "..."     # 显式 headless，支持 --json 输出
gocloudagent resume <session-id>   # 恢复会话
gocloudagent sessions [list|show|rm]  # 会话管理
gocloudagent config [get|set|path] # 配置
gocloudagent --version / --help
```

**与 `cmd/simplebase` 的关系**：独立二进制，**不改** `cmd/simplebase` 的分发逻辑（`main.go:22` 那行保持不动）。理由：`simplebase` 是服务端二进制，它同时充当「服务端」和「人类操作者的资源 CLI」（`sbcli.RunIO`）；把 Agent CLI 塞进去会让 `os.Args[0] == "simplebase"` 时的语义更混乱，也破坏 `app.go:938-962` 里「服务自身二进制复用为 CLI」的既有约定。

`build.sh` 需要加一个目标（现有 `case` 只认 4 个分支，`build.sh:775-800`；唯一的 `go build` 在 :307-310）：

```bash
# cmd_build 内追加
if [[ ! -d "$ROOT_DIR/cmd/gocloudagent" ]]; then … fi
go build -o "$ROOT_DIR/gocloudagent" ./cmd/gocloudagent
```

同时更新 `usage()`（`build.sh:30-79`）与 `AGENTS.md` 的「常用命令」「代码结构」两节。`Dockerfile:6`（当前 `CGO_ENABLED=0 go build -o /out/simplebase ./cmd/simplebase`）**不改**——CLI 不进服务镜像，由用户单独安装。

### 5.2 会话存储 `~/.gocloudagent/`

```
~/.gocloudagent/
  config.json                 # 0600，权限不合规拒绝加载（复用 sbcli/config.go:39-42 的 errInsecureConfig 语义）
  sessions/
    <session-id>.jsonl        # 一行一条 entry，append-only
  skills/                     # 用户自定义 skill（可选，见 §5.3）
  history                     # REPL 输入历史（纯文本，一行一条）
```

`config.json` 形状（与 `sbcli` 的 `loadedConfig` 保持字段名一致，便于人类在两边复用同一份 token）：

```go
type Config struct {
    Endpoint  string `json:"endpoint"`
    Token     string `json:"token"`
    ProjectID string `json:"project_id"`
    Model     string `json:"model"`      // CLI 独有：默认模型
    Output    string `json:"output"`     // "text" | "json"
}
```

配置优先级（从低到高，对齐 `sbcli` 的既有约定）：`~/.gocloudagent/config.json` → `~/.config/simplebase/config.json`（复用人类模式的 token） → `SIMPLEBASE_*` env → `GOCLOUDAGENT_*` env → 命令行 flag。

**jsonl entry 形状**（借鉴 `pi-readme.md` §4.6 的会话树，但**落盘字段必须脱敏**）：

```jsonc
{"type":"session","v":1,"id":"...","cwd":"...","endpoint":"...","project":"...","model":"...","created_at":"..."}
{"type":"message","id":"m1","parent":"","role":"user","content":"...","created_at":"..."}
{"type":"message","id":"m2","parent":"m1","role":"assistant","content":"...","tool_calls":[{"id":"c1","name":"shell","arguments":"{...}"}]}
{"type":"message","id":"m3","parent":"m2","role":"tool","tool_call_id":"c1","name":"shell","content":"...","is_error":false}
{"type":"compaction","id":"k1","parent":"m2","summary":"...","first_kept":"m2","created_at":"..."}
```

设计要点：

1. **`parent` 形成树**，`session` 行可选加 `leaf` 字段记当前叶子（支持未来的分支/回溯）。第一阶段只做线性（`parent` 恒为前一条的 id），树是「加字段不改格式」的渐进路径——这是 `pi-readme.md` §4.6 的做法，也是唯一不返工的做法。
2. **`content` 落盘前经 `RedactSecrets`**（`prompt.go:91`）。本地文件虽不属系统库审计范围，但 `~/.gocloudagent/sessions/` 会被用户备份/同步，密钥泄漏面与日志同级。**这一条与云端「审计不落正文」的约束不一致是刻意的**：CLI 是本地工具，用户需要 `resume` 看到完整历史；口令是「脱敏但不截断」（云端是「既不落正文又截断」）。在文档与代码注释里写明这个差异。
3. **不落盘 `~/.gocloudagent/sessions/` 之外的任何东西**；不写系统库、不写 S3。
4. `sessions rm` 做真删除（`os.Remove`），不做墓碑。

### 5.3 工具面（这是 CLI 与云端的真正分野）

| 工具 | 云端 | CLI | 说明 |
| --- | --- | --- | --- |
| `list_databases` / `readonly_sql` / `list_objects` / `head_object` / `search_logs` / `log_level_stats` / `list_collections` | ✅（`skills_cli=false`） | ❌ | CLI 走 `simplebase` CLI 工具即可 |
| `simplebase`（argv 白名单 + 委托令牌 + 确认） | ✅（`skills_cli=true`） | ✅（**但换鉴权方式**） | 见下 |
| `sandbox_exec` / `_shell` / `_read_file` / `_write_file` | ✅（需 Sandbox） | ❌ | CLI 有本地 shell，不需要云沙盒 |
| **`shell`** | ❌ | ✅ **新增** | `os/exec`，`/bin/sh -c`，cwd 限于 workspace |
| **`read_file` / `write_file` / `edit_file` / `glob` / `grep`** | ❌ | ✅ **新增** | 本地文件系统，路径限制在 workspace（可选放开） |
| **`web_search`** | ❌ | ✅ **新增** | 需搜索后端，见 §12 Q1 |
| `fetch_url` | ❌ | ⚪ 可选 | 抓 URL 转文本 |

**CLI 侧的 skill 与 prompt 入口**（§13.6 / §13.7）：CLI 同时支持两种前缀，**语义严格区分**：

```text
@database  → skill（能力授权）：进 RunRequest.Skills，决定 AllowArgv 的前缀集合
/review 3  → prompt template（文本展开）：读 ~/.gocloudagent/prompts/review.md，$1 = "3"
```

两者**不混用**（§13.7）：展开一段模板文本不会获得任何写权限；`AllowArgv` 永远只看 `RunRequest.Skills`。这与云端只有 `@` 的现状不同，但云端不引入 `/`（§13.7 给了理由：会让「用户输入原文」与「实际发给模型的内容」不一致，影响落库与审计语义）。

**CLI 侧也支持 `disable-model-invocation: true`**（§13.6 E2）：标记为该值的 skill 不出现在模型可见的 skill 目录里，只能由用户显式 `@` 触发。


**`simplebase` 工具在 CLI 下的鉴权**：云端用「委托令牌 + argv 前缀白名单 + 破坏性确认」（`cli_tool.go`）。CLI 下用户就是本人，不需要委托令牌——直接用用户配置里的 token，且 **argv 白名单可以放宽**（用户自己敲的命令没有「skill_not_loaded」的概念）。实现上给 `cli_tool.go` 加一个模式开关：

```go
type CLIProfile struct {
    // Delegated 为真时走委托令牌 + skill 前缀白名单（云端）；
    // 为假时直接用 cfg.Token，不做 AllowArgv 检查（本地 CLI）。
    Delegated bool
    // Headless 为真时破坏性操作直接拒绝（云端定时任务）。
    Headless bool
    // Confirm 为 nil 时破坏性操作直接拒绝（CLI 未接确认 UI 时）。
    Confirm ConfirmationWaiter
}
```

**`shell` 的安全边界**（这是 CLI 与云端安全模型的根本差异，必须写明）：

- 云端：`sandbox_shell` 跑在 microVM 里，命令长度 ≤ 4096 字节（`tools.go:86`），路径限 `/workspace`；
- CLI：`shell` 跑在**用户自己的机器**上，就是用户权限。参考 claude-code / codex 的既有做法：
  1. 默认 cwd = 启动目录，路径工具限制在 cwd 子树内（`filepath.Rel` 判定，拒绝 `..` 逃逸）；
  2. 破坏性命令（`rm -rf`、`git push --force`、`curl | sh` 等）需要确认——复用现有的 `destructiveArgv` 思路但**必须新写规则表**（CLI 命令空间与 `simplebase` 命令空间完全不同，不能复用 `destructiveArgv` 的白名单）；
  3. `--yolo` / `--dangerously-skip-permissions` flag 跳过确认（显式、可审计地承担风险）；
  4. **不注入任何 SimpleBase 凭据到 shell 子进程的 env**（与云端「不向沙盒注入凭据」同一原则）。

**`web_search` 的实现选项**（`AGENTS.md` 禁止新增依赖，所以只能走标准库 `net/http`）：

- 选项 A：DuckDuckGo HTML 端点（免 key），解析结果页；
- 选项 B：Brave / Serper / Tavily 等 JSON API（需 key，配在 `~/.gocloudagent/config.json`）；
- 选项 C：不实现，只在文档里说明可用 `simplebase function invoke` 调用户自己的搜索函数。

**本计划推荐 A + B 双通道**：无 key 时用 A（尽力而为，可能因反爬失效），配了 key 时用 B。**这是 §12 Q1，需要用户拍板**。

### 5.4 REPL 交互设计

不引入 TUI 库（无新依赖）。用标准输入 + 行缓冲 + ANSI 转义：

```text
> 查一下 internal/gocloudagent 有没有 TODO
  ⠋ thinking
  ▶ shell({"command":"grep -rn TODO internal/gocloudagent"})
    internal/gocloudagent/runtime.go:59: // Skills 是本轮已校验的 skill id。
    ...
  ✓ 找到 3 处，分布在 runtime.go / skills.go / cli_tool.go。
```

要点：

- 流式渲染直接在 stdout 追加 `token` 事件（不重绘），工具调用用 `▶` 前缀 + 参数摘要（`redactArgv` 复用），结果截断展示；
- `Ctrl+C` 第一次取消当前 run（对应 `Runtime.CancelRun`），第二次退出；
- `Ctrl+D` 退出；
- 空闲时不做动画（无 `thinking` 心跳——那是 SSE 的产物，本地不需要）；
- 非 TTY 时（管道）自动切 headless 模式并输出纯文本（无 ANSI）。

### 5.5 CLI 复用 `gocloudagent` 的方式

`gocloudagent/cli` 不重新实现 eino 装配，而是**复用 `agentcore` + 一个精简的 `Runtime`**：

```go
// gocloudagent/cli 自己组装，不复用 gocloudagent.Runtime
// （Runtime 绑了 project/principal/systemdb/sandbox，CLI 全都不要）
func (s *Session) run(ctx context.Context, input string, emit func(agentcore.Event)) (string, error) {
    model := agentcore.NewModel(s.client, s.cfg.Model, agentcore.ProtocolNative)
    tools := append(shellTools(s.workspace), webTools(s.cfg)...)
    tools = append(tools, s.simplebaseTool())
    agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:        "gocloudagent-cli",
        Instruction: cliInstruction(s.cfg),
        Model:       model,
        ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools}},
        MaxIterations: s.cfg.MaxIterations,
        Handlers:    []adk.ChatModelAgentMiddleware{summarizationMW},
    })
    runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: true})
    // 消费事件，写入 jsonl
}
```

**这是 `internal/gocloudagent/agentcore` 存在的全部理由**：`gatewayChatModel` 与协议解析是「与运行时无关」的，两个消费者（云 `Runtime`、CLI `Session`）共用它们，而不共用 `Runtime` 的 project/principal 绑定。

CLI 侧的 `ChatClient` 实现：直接调 `llmgateway`？不行——`llmgateway` 要 `ProviderResolver` / 系统库配置。CLI 的正确做法是**直连供应商 HTTP API**（用 `litellm/providers` 已有的驱动），或者**走 SimpleBase 的 HTTP API**（`/v1/...` 有没有 chat 端点？没有——目前只有 agent run 端点）。

三个选项：

1. CLI 直接调供应商 API（读 `~/.gocloudagent/config.json` 里的 provider key）——参考 codex / claude-code 的做法，但要把密钥管理再做一遍；
2. CLI 调 SimpleBase 的 agent run 端点（`POST /agent-threads/:id/runs` + SSE）——**复用服务端已有的模型配置与密钥**，但需要服务端在线，且要处理项目/助手/委托令牌；
3. 混合：优先 2（服务端可用时），回落 1。

**决策 D-D1**：**选项 2 为主，选项 1 作为后续可选**。理由：

- `AGENTS.md` 的配置约束是「配置只从 `config.yaml` + `SIMPLEBASE_*` 加载，不得引入其他配置来源，密钥不得落 YAML」。CLI 天然要落 key 在用户主目录——这与约束冲突。走服务端就完全避开这个问题；
- 服务端已有 `llmgateway`（多供应商 + 熔断 + 用量配额），CLI 再造一套会分裂密钥配置（`pi-readme.md` §3.2 已经把这个列为「不该照搬」）；
- 代价是需要服务端在线。但 CLI 的本地工具（shell / 文件 / 搜索）**不需要**服务端，只有模型推理需要。断网时给出明确错误即可。

这也意味着：**CLI 的模型出口是「HTTP → SimpleBase → llmgateway」**，`agentcore.Model` 的实现需要一个 HTTP-based `ChatClient`（新增一个实现，不复杂）。SSE 消费用标准库 `net/http` 的流式读取。

---

## 6. 前端改动

### 6.1 CronJobs 页：加「任务类型」二选一

`components/modal/CronJobModal.vue` + `components/cronjob/useCronJobForm.ts` + `CronJobScheduleFields.vue`：

| 改动 | 位置 | 说明 |
| --- | --- | --- |
| 新增「任务类型」选择 | Modal 顶部 | 用 `ui/select`（新代码不写原生 `<select>`，`ui/AGENTS.md` 明确）或 radio group；两个选项：云函数 / 助手 |
| `form.targetKind: 'function' \| 'agent'` | `useCronJobForm.ts` 的 `CronJobForm` interface | |
| 类型为 function 时 | 保留现有「文件 → 导出函数」两级下拉 | 不动 |
| 类型为 agent 时 | 新增「选择助手」下拉（数据源 `api.agents.list(projectId)`，只列非归档）+ 提示词 `Textarea` | 助手列表复用 `AgentManager.vue` 的加载逻辑口径 |
| 提示词长度提示 | Textarea 下方 | 「1234 / 16384 字符」计数器；超限时 `FieldError` + `okButtonProps.disabled` |
| `canSave` 扩展 | `useCronJobForm.ts:254-263` | agent 类型时：`agentId` 非空 + `prompt` trim 非空 + `len(prompt) <= 16384` |
| 提交体扩展 | `services/types.ts` 的 `CronJobCreate`（:748-760） | 加 `targetKind` / `agentId` / `agentPrompt`（camelCase，映射函数转 snake_case） |

**表单字段规范**（`ui/AGENTS.md` 硬性要求，当前 `AgentScheduleModal.vue` 违反了）：

- 字段用 `ui/field` 的 `Field` / `FieldGroup` / `FieldLabel` / `FieldError`，**不手写裸 `<label>` + `<p class="text-destructive">`**（`AgentScheduleModal.vue:44` 就是反例，本次必须改掉）；
- `Textarea` **不透传 `maxlength`**（`ui/components/ui/textarea/Textarea.vue:6-10` props 只有 `class/defaultValue/modelValue`）。加长度限制只能走两条路：(a) 校验函数 + `FieldError` + 可用 `@input` 截断；(b) 给 `Textarea` 加 `maxlength` prop——后者属 `ui/` fork 库改动（`ui/AGENTS.md`：「只增删组件不改风格约定」），**需要单独判断**。本计划选 (a)，不动 `ui/`。

### 6.2 `CronJobItem` 与列表页

`services/types.ts`：

```ts
export interface CronJobItem {
  id: string
  name: string
  description: string
  targetKind: 'function' | 'agent'          // 新增
  agentId?: string                          // 新增
  agentThreadId?: string                    // 新增
  agentPrompt?: string                      // 新增
  scheduleKind: CronScheduleKind
  cronExpr: string
  intervalSeconds?: number
  runAt?: string
  funcFile: string
  funcExport: string
  inputJson: string
  enabled: boolean
  lastRunAt?: string
  nextRunAt?: string
  lastStatus: string
  lastError: string
  runCount: number
  targetMissing: boolean
  createdAt: string
  updatedAt: string
}
```

`pages/CronJobs.vue`：

- 列表加「类型」列（云函数显示 `funcFile#funcExport`，助手显示助手名）；
- 「目标缺失」提示对 agent 类型同样生效（助手被删时 `targetMissing=true`）；
- `scheduleText`（:234-242）不动。

### 6.3 AgentManager 页：schedule 入口改为跳转

`pages/AgentManager.vue` 的 `openSchedule`（:529）与 `AgentScheduleModal` 改为：

- 菜单项 `schedule` 保留，但改为「跳转到定时任务页并预选该助手」→ `router.push({ name: 'cron-jobs', query: { agent: agent.id } })`；
- `CronJobModal` 读 `route.query.agent` 做预选；
- `AgentScheduleModal.vue` + `ui/tests/AgentScheduleModal.test.ts` **一个版本后删除**（与后端兼容层同生命周期）。

### 6.4 i18n 与测试

新增文案（`zh-CN.ts` 与 `en-US.ts` **必须同构**，`ui/tests/i18n.test.ts` 会卡）：

| key | 含义 |
| --- | --- |
| `components.cronJobModal.targetKind` | 任务类型 |
| `components.cronJobModal.targetFunction` | 云函数 |
| `components.cronJobModal.targetAgent` | 助手 |
| `components.cronJobModal.selectAgent` | 选择助手 |
| `components.cronJobModal.noAgents` | 还没有助手 |
| `components.cronJobModal.agentPrompt` | 提示词 |
| `components.cronJobModal.agentPromptHint` | 每次执行会把这段文本作为用户消息发给助手 |
| `components.cronJobModal.agentPromptTooLong` | 提示词超过 16384 字符 |
| `components.cronJobModal.agentPromptCount` | `{used} / {max}` |
| `pages.cronJobs.colTarget` | 类型 |
| `pages.cronJobs.targetAgent` | 助手：{name} |
| `error.agent_prompt_too_long` 等 4 个错误码 | 后端错误码映射 |

测试：

- `ui/tests/CronJobModal.test.ts` 加 agent 类型分支（切换类型清空另一侧字段、16KiB 边界、`canSave` 组合）；
- `ui/tests/http-api.test.ts` 加新字段的 snake_case 映射用例；
- 删除 `AgentScheduleModal.test.ts`（与兼容层同批）。

**注意 i18n 陷阱**：`vue-i18n` 把 `@` 当 linked message 起始符，文案里裸 `@` 必须写 `{'@'}`（`AGENTS.md` 记录，`AgentManager` 场景已踩过）。

### 6.5 coverage 风险

拆出的新逻辑都会计入 coverage（只有 `src/components/ui/**` 被排除，`ui/vite.config.ts:33-39`）。当前 functions 94.24% **未达标**，新增代码必须同步补测，别让它继续掉。不改阈值（`ui/AGENTS.md` 明确禁止）。

---

## 7. 迁移与数据库

| 版本 | 表 | 内容 | 是否可自动执行 |
| --- | --- | --- | --- |
| v49 | `sys_cron_jobs` | 加 `target_kind` / `agent_id` / `agent_thread_id` / `agent_prompt` 四列 | 定义随代码提交，**由用户执行** |

**不做的迁移**：

- 不 DROP `sys_agent_schedules` / `sys_agent_schedule_runs`；
- 不回填历史 agent schedule 到 `sys_cron_jobs`（见 §2.2 的取舍说明）；
- 不改 `sys_agent_schedules.prompt` 的长度语义（它是归档表，16KiB 限制只对新写入生效）。

**手动执行命令**（由用户决定何时执行；注意 `build.sh reset` 会删本地 cache 重建空库，是另一回事）：

```sql
-- 由用户自行在系统库执行；代理不代为执行
ALTER TABLE sys_cron_jobs ADD COLUMN target_kind VARCHAR DEFAULT 'function';
ALTER TABLE sys_cron_jobs ADD COLUMN agent_id VARCHAR DEFAULT '';
ALTER TABLE sys_cron_jobs ADD COLUMN agent_thread_id VARCHAR DEFAULT '';
ALTER TABLE sys_cron_jobs ADD COLUMN agent_prompt VARCHAR DEFAULT '';
```

正常启动流程（`TIMER_NO_RUN=0`）会经 `systemdb.ApplySystemMigrations` 自动应用（`migrate.go:683` 起，幂等）。用户本地常以 `TIMER_NO_RUN=1` 运行，该模式跳过 `init_db`，因此新列不会自动生效——**必须由用户手动执行或正常启动一次**。

---

## 8. 分阶段实施与门槛

| 阶段 | 内容 | 进入下一阶段的门槛 |
| --- | --- | --- |
| **A0 前置** | 核对工作区差异（463 条未提交）、确认开放问题 Q1-Q4 | 用户对 Q1-Q4 给出选择 |
| **A1 迁移定义** | v49 迁移 + `cron_jobs.go` 的列/scan/Create/Update 同步 + 测试 | `go test -short ./internal/systemdb` 通过（用隔离临时库） |
| **A2 cronjob 接口** | `Executor` / `Result` / `NewRouter`；`cronjob` 测试补 TargetKind 三态 | `go test -race -short -count=3 ./internal/cronjob` |
| **A3 agentjob** | 新包 + 从 `gocloudagent/scheduler.go` 平移执行体 + 测试 | `go test -race -short -count=3 ./internal/agentjob` |
| **A4 app 装配** | 统一装配 + 删除 agentScheduler 字段 + `Close`/`Shutdown` 收敛 | `go build ./internal/... ./cmd/...`；`go test -short ./internal/app` |
| **A5 API** | cronjob handler 扩展 + agent-schedule 兼容层 + 审计补齐 | `go test -short ./internal/api`；`scripts/smoke.sh` |
| **B 删除** | 删 `gocloudagent/scheduler.go` + 3 个测试文件；清 `crontab` 注释 | `go build ./...` + 全仓 `grep -rn "gocloudagent.Scheduler\|NewScheduler"` 零命中（除 cronjob） |
| **C0 agentcore** | 建包 + 原样搬迁（`model` / `prompt` / `errors` / 协议） | 全部 `gocloudagent` 测试零改动通过 |
| **C1 confirmMW** | 替换 `callGate`；`tool_start` 对只读工具也发 | 现有 SSE 顺序 e2e 通过；新增「确认拒绝时 CLI 未启动」 |
| **C2 skill 中间件** | 自研 embed Backend；`SkillPrompt` 改索引 | 子技能全文不再默认进 Instruction |
| **C3 summarization** | 替换 `maxHistoryChars` | 摘要失败不丢历史；摘要先脱敏 |
| **C4 specialists** | L2 多 Agent，**默认关闭** | 开关关闭时与 C3 行为逐字节一致 |
| **D CLI** | `gocloudagent/cli` + `gocloudagent/tools` + `cmd/gocloudagent` + `build.sh` | §9 的 CLI 验收项 |
| **UI 前端** | CronJobs 表单 + 类型 + i18n + 测试 | `cd ui && yarn test && yarn build`；coverage 不下降 |
| **F 文档** | `AGENTS.md` / `internal/AGENTS.md` / `eino-readme.md` §15 / `docs/` | 链接与命令与实际一致 |

> **编号约定**：议题字母为 A（统一调度）/ B（删除）/ C（重组 gocloudagent）/ D（独立 CLI）/ **E（对齐 Pi，§13）**。上表的 **E1-E8 是议题 E 的子阶段**，与「UI 前端」阶段无关（后者原名「E 前端」，为避免与 E1-E8 混淆已改名）。

**议题 E（Pi 能力，§13）的阶段与归并**：

| 阶段 | 内容 | 归入 |
| --- | --- | --- |
| **E1 工具管道** | `BeforeTool` / `BeforeDecision`；`UnknownToolsHandler`；`length` 截断处理；`ExecuteSequentially=true` | C1 之后、C2 之前 |
| **E2 事件** | `isToolDelta` 去 `delta:` 前缀判据；`tool_start` 覆盖只读工具 | 紧随 E1 |
| **E3 压缩** | `summarization` 中间件 + 自定义 `Finalize`（`cutByTurn`）；替换 `maxHistoryChars` | 与 C3 合并（C3 原本就是「用 summarization 替换 `maxHistoryChars`」，E3 补上「切点必须落在完整 turn」的 Finalize） |
| **E4 skills** | 目录化提示；`disable-model-invocation`；`@` 选中正文缓存 | 与 C2 合并 |
| **E5 评估** | `eval_test.go` 表驱动 8 个案例 | C 阶段收尾 |
| **E6 CLI 外壳** | `print` / `json` 模式；prompt templates；会话级信任 | D 阶段 |
| **E7 tool 进度** | `tool_execution_update`（CLI / 沙盒流式） | D 阶段 |
| **E8 云端会话树** | `parent_id` / `kind` / `leaf_id` 迁移 + 树投影 | **本次不实施**（阻塞项见 §13.4）；待 §12 Q8 |

**A → B → C（含 E1-E5）→ D（含 E6-E7）严格串行**：B 依赖 A（被删文件的职责已迁走）；C 依赖 B（否则要在两处同步改事件语义）；D 依赖 C（复用 `agentcore`）。E1 必须在 C1 之后（确认中间件就位才能把 `beforeTool` 接进去），E2 在 E1 之后（`tool_start` 的发送点由 E1 的管道决定），E3 在 E2 之后（压缩切点要对齐 turn，而 turn 的边界由 E1/E2 的事件明确）。

**UI 前端阶段可以与 C/D 并行**，因为它只依赖 A5 的 API 契约。

---

## 9. 验证矩阵

```sh
# 每个阶段（按涉及范围选择）
go build ./internal/... ./cmd/...
go vet ./internal/... ./cmd/...

# A 阶段
go test -short -count=1 ./internal/systemdb ./internal/cronjob ./internal/agentjob ./internal/app ./internal/api
go test -race -short -count=3 ./internal/cronjob ./internal/agentjob

# B 阶段（删除后）
grep -rn "gocloudagent.Scheduler\|gocloudagent.NewScheduler" --include='*.go' .   # 期望零命中
go test -short -count=1 ./internal/gocloudagent

# C 阶段
go test -short -count=1 ./internal/gocloudagent/... ./internal/api
go test -race -short -count=3 ./internal/gocloudagent ./internal/api
go test -count=1 -run TestAgentSkills ./internal/api           # SSE 顺序回归

# E 阶段（Pi 能力，§13）
go test -short -count=1 -run 'TestEval|TestToolPipeline|TestCompaction|TestSkill' ./internal/gocloudagent
#   E1 工具管道：unknown_tool 不失败、length 截断不执行、确认串行、block 不发 tool_start
#   E2 事件：isToolDelta 仅凭 Extra 判定、tool_start 覆盖只读工具
#   E3 压缩：切点不拆开工具回合、摘要失败保留原路径、摘要先脱敏
#   E4 skills：skill 工具正文仅来自 KnownSkill、disable-model-invocation 不进描述
#   E5 评估：8 个表驱动案例（假模型，无外网）
go test -count=1 -run TestAgentSkills ./internal/api           # 顺序契约仍成立

# D 阶段（CLI）
go test -short -count=1 ./internal/gocloudagent/cli ./internal/gocloudagent/tools ./cmd/gocloudagent
go build -o /tmp/gocloudagent ./cmd/gocloudagent && /tmp/gocloudagent --help
/tmp/gocloudagent run --dry-run "test"                            # 不真调模型

# UI 前端阶段
cd ui && yarn test && yarn build
cd ui && yarn vitest run tests/CronJobModal.test.ts

# 全量
go test ./...
./scripts/smoke.sh
```

**不得使用的手段**：删测试、放宽断言、加 `retry`/`sleep` 绕过；`git checkout/restore/reset/stash` 清工作区；自动 commit；执行 `init_db` 或任何 ALTER/DDL；对运行库做写操作。

**CLI 验收补充**（D 阶段）：

1. `gocloudagent "列出当前目录"` 在无服务端时给出明确的「服务端未配置」错误，而不是 panic；
2. 生成的 jsonl 每行都是合法 JSON，`parent` 链完整；
3. `gocloudagent resume <id>` 能还原文；`sessions rm` 后文件消失；
4. `shell` 工具的路径逃逸（`cd ..` / 绝对路径）被拒绝或需确认；
5. `~/.gocloudagent/config.json` 权限不是 0600 时拒绝加载；
6. 全程不写系统库、不写 S3。

---

## 10. 风险与不做的事

### 10.1 风险表

| 风险 | 后果 | 处理 |
| --- | --- | --- |
| `cron_jobs.go` 加列而漏改 scan/列常量 | 按位扫描错位，读到错字段（**静默数据错误**） | §2.2 的同步改动表逐项核对；补「四字段 round-trip」测试；`go test ./internal/systemdb` |
| 调度统一后 `next_run_at` 推进语义漂移 | 任务漏跑或重复跑 | 保留 `nextRun` 三态实现原样；`cronjob/scheduler_test.go` 的边界断言（`*/10`）不动 |
| `Close()` 补 `Stop()` 引入新的等待 | 关停变慢或死锁 | 有界超时（现有 `Shutdown` 已有 `select` + `ctx.Done()` 模式）；补并发 Stop/Close 测试 |
| 删 `callGate` 后 UI 先看到确认后看到 `tool_call` | 前端拿不到 call id | `emit tool_call` 在中间件之前且同步；补 e2e 顺序断言 |
| skill 改成「按需读」后模型不再知道怎么写命令 | 工具调用失败率上升 | 顶层 `SKILL.md` 索引必须含「何时用哪个 skill」；`@` 选中的 skill 本轮正文可直接缓存进首次工具结果，避免多一次 round-trip |
| `summarization` 摘要写进系统库时带密钥 | 跨用户泄漏 | 摘要落库前 `RedactSecrets`；摘要请求正文不写日志（`internal/AGENTS.md` 的 trace 例外边界不覆盖摘要） |
| L2 多 Agent 路由不准 | 调错 specialist，用户看到答非所问 | **默认关闭**；`Description` 写成「什么时候用我」；先本地验证再考虑默认开启 |
| `EmitInternalEvents` 开启后 UI 收到 `AgentName != ""` 的事件 | 前端渲染错乱 | 开启前先补前端对 `AgentName` 的处理（过滤或加前缀），并在 e2e 里断言 |
| CLI 把 token 落 `~/.gocloudagent/config.json` | 与「密钥不得落 YAML」的约束冲突 | 走服务端模型出口（决策 D-D1），CLI 只存 SimpleBase 的 token（与 `sbcli` 同级别，已是既成事实）；provider key 不进 CLI 配置 |
| `go.sum` 因 doublestar 变动 | 违反「不新增依赖」 | 决策 D-C2 已避免：自研 skill Backend，不用 `adk/filesystem` |
| 前端 16KiB 限制只做前端 | 绕过 UI 直调 API 可写超长 prompt | 后端**必须**校验（§2.6），前端只是体验层 |
| coverage 继续下滑 | `yarn test` 失败 | 新增代码同步补测；不降阈值 |
| **模型输出被截断（`FinishReason="length"`）却照常执行残缺 `ToolCalls`** | 参数不完整 → 工具报错，或**危险操作按半截参数执行** | §13.1：为 `length` 时每个 call 发 `truncated` 错误且**不执行**；补 eval 用例 |
| **eino `summarization` 的默认 Finalize 把全部历史换成一条摘要** | 模型丢掉近期工具结果，答非所问（不是「压缩」而是「失忆」） | §13.5：必须提供自定义 `Finalize` 保留近期完整回合（`cutByTurn`）；已核实 `DefaultFinalize` 行为 |
| **压缩切点拆开工具回合** | 下一轮模型看到没有 result 的 tool call | `cutByTurn` 以完整回合为单位，并补「assistant 的 tool call 未全部配对时把切点前移」；eval 有专项用例 |
| **`tool_start` 扩大到只读工具后 handler 的 `tool_progress` 行为变化** | 前端可能重复渲染进度帧 | `tool_progress` 是 handler 侧按已开始的 call 计时的；扩大后需核对前端去重逻辑 |
| **`thinking` 与新推理事件撞名** | 前端把推理当空闲心跳（或反之） | §13.2：**不改 `thinking` 语义**；推理增量必须新起 type 且默认关闭 |
| **CLI 的目录级信任被误当授权** | 别处 clone 的同名目录继承信任 | §13.10：倾向**会话级**信任（`--trust`，进程退出失效）；`destructiveArgv` 的确认**不因信任而跳过** |
| **prompt template 展开被误当 skill 授权** | 「展开一段文本就获得写权限」的错觉 | §13.7：`/` 只走 template、`@` 只走 skill，两者不混用；`AllowArgv` 仍只看 `RunRequest.Skills` |
| 云端会话树迁移（v50）在跨 Agent 交接未定时仓促上 | 执行者归属混乱、旧 run 重试不安全 | §13.4：**本次不实施**；阻塞项是「run 无有效技能快照」+「跨 Agent 交接未决」 |

### 10.2 明确不做的事

1. **不做 `gocloudagent.Scheduler` 兼容薄壳**——会成环（§1.4），也违背「不留死代码」；
2. **不引入 `flow/agent/react` / `multiagent/host`**——`eino-readme.md` §8.2 明确不推荐，且 host 的工具名路由与 `@` 协议冲突；
3. **不引入 `adk/prebuilt/supervisor`**——上游标记不推荐；
4. **不引入 `adk/middlewares/reduction`**——依赖 `adk/filesystem`（doublestar），收益不足；等 CLI 侧（有真实 FS）验证后再回看；
5. **不自研 `RunLoop` 替换 `runner.Run`**（**有条件例外**：见 §13.3 的 T1/T2/T3 触发条件。不满足任一条就继续用 ADK + 中间件；满足则一次替换干净，不留双轨）。理由：`pi-readme.md` §4.11 的双轨风险（两套 `MaxIterations`、两套错误码）是真实成本，而 §4.2 的中间件化已经能解决确认与 skill 两项；
6. **不做 `tool.StatefulInterrupt` + Checkpoint**——那要求确认能跨进程重启恢复，是产品问题不是技术问题（`pi-readme.md` §4.10 步骤 7 的条件是「仅当产品要求进程重启后续跑」）；
7. **不回填历史 agent schedule 数据**（§2.2）；
8. **不改 `ui/components/ui/`**（`Textarea` 不加 prop）；
9. **不改 `vite.config.ts` / 阈值 / `tsconfig.json`**；
10. **不把 CLI 打进服务镜像**（`Dockerfile` 不动）；
11. **不做 CLI 的 provider key 管理**（决策 D-D1）；
12. **不做 steering / 运行中插话**（`pi-readme.md` §4.3 明确「Steering 先不做」，需要前端先有「运行中插入」的 API）。

**§13（Pi 能力）新增的「不做」**：

13. **不引入 Pi 的「默认无授权」信任模型**——Pi 的 bash/edit/write 直接以进程用户权限执行，也不在每次工具调用前询问（`pi-readme` §2.7）；本仓库保留 `destructiveArgv` + `ConfirmationWaiter` + `Headless` 拒绝三层（§13.10）；
14. **不引入 Codemode（QuickJS WASM）**——`pi-readme` §3.2：云 Agent 不能加载任意 TypeScript，技能只能约束到已存在的 HTTP API；且 Pi 的 `tool_call` 扩展改参后**不再复校**，对云 Agent 不可接受（§1.7）；
15. **不引入 `packages/tui` 的差分渲染**——`pi-readme` §3.2：「要搬的是事件名字和顺序，不是 `TuiAltScreen`」。CLI 用行缓冲 + ANSI 即可（§5.4）；
16. **不引入 pi-ai 的供应商矩阵与 OAuth 设备码**——`pi-readme` §3.2：会分裂密钥配置。模型出口仍是服务端 `llmgateway`（决策 D-D1）；
17. **不引入 durable 的 `GenerationTask`/`ToolTask`**——`pi-readme` §2.1 与 §3.2 都指出这是实验性 harness，且与 `adk.Runner` 是**两套控制流**；先上会把确认、SSE 与系统库写路径绑在未稳定的任务图上；
18. **不做 `disable-model-invocation` 之外的扩展机制**（extensions / hooks / MCP 客户端）——`pi-readme` §2.6 的 `ExtensionAPI.on` 有 40 个事件、扩展与工具同进程；本仓库的能力边界是「已有 HTTP API + `Sandbox` 接口」，不需要进程内扩展宿主；
19. **不做 RPC 模式与独立 SDK**（§13.8）——`json` 模式已能满足脚本化需求，Go 侧复用通过 `internal/gocloudagent/agentcore` 实现；
20. **不做云端会话树（v50 迁移）**（§13.4）——阻塞项是「run 无有效技能快照」+「跨 Agent 交接未决」，见 `cloud-agent-module-mentions-and-thread-routing-plan.md` §2；
21. **不做工具结果的 offload（`adk/middlewares/reduction`）**——它依赖 `adk/filesystem`（doublestar，见 Q3），且需要 `ReadFileToolName` 能读回 offload 的文件；云端 Agent 没有通用文件系统。CLI 侧（有真实 FS）验证后再回看。

---

## 11. 交接模板与停止条件

### 11.1 每个阶段报告格式

```text
阶段 ID / 子任务：
状态：待核对 / 待表征 / 可实施 / 已验证 / 已存在 / 暂缓 / 阻塞
基线：HEAD、允许文件已有差异说明
允许文件 / 实际修改文件：
源码证据：函数名、原连续片段、调用方
表征证据：新增或复用的真实测试名，旧实现运行结果
等价复核：错误、nil/零值、时序、次数、资源/锁逐项结果
验证：完整命令、退出码；未运行或失败的具体原因
保留复杂度 / 未实施项：
下一步：仅建议下一个子任务，不自动执行
```

未经运行的「预计通过」、测试文件存在、静态代码阅读**均不能填为测试通过**。

### 11.2 立即停止本阶段的条件

- 必须改变导出签名、SSE 协议/错误文本、权限、锁/资源时序或 schema 才能实施（schema 除外：本计划已授权 v49）；
- 表征测试在旧实现失败且不能证明断言错误；
- 测试会连接运行库、真实 S3/沙盒，或需安装依赖、执行迁移/重置数据；
- 修改文件被并行工作改变，差异无法区分；
- 需要新增依赖才能继续（回到 §12 Q3）。

### 11.3 可直接交给下一模型的指令

> 阅读本计划 §8 的阶段表与 §2–§5 的对应章节，本次只执行 `<阶段 ID>`。先核对 `git rev-parse --short HEAD`、工作区差异与目标文件的实际行号；在旧实现上补并运行表征测试，通过后才执行卡片允许的改动。不得改变公开 API、SSE 事件顺序、权限、锁/资源/持久化时序；不新增依赖、不改配置文件、不访问运行数据库、不执行 ALTER/DDL、不自动 commit。证据不足按 §11.2 停止，不猜测实现；完成后按 §11.1 报告真实验证结果并停止，不自动继续下一阶段。

---

## 12. 开放问题（需用户拍板）

| # | 问题 | 选项 | 本计划倾向 |
| --- | --- | --- | --- |
| **Q1** | `web_search` 的搜索后端 | A 免 key（DuckDuckGo HTML，可能失效）／B 需 key（Brave/Serper/Tavily）／C 不实现 | A + B 双通道（无 key 用 A，配 key 用 B） |
| **Q2** | 同一助手的多条定时任务，是否共用一个 thread？ | A 每条任务一个专属 thread（现状语义）／B 同一助手的所有任务共用 thread | A（保持现有 thread 归属校验与 `agent_switch_required` 语义，改动面最小） |
| **Q3** | 是否允许引入 `github.com/bmatcuk/doublestar/v4`（eino `adk/filesystem` 的间接依赖） | A 不允许（自研 skill Backend）／B 允许（用 eino 原生 Backend + 未来可上 `reduction`） | A（决策 D-C2；若将来 CLI 的 `glob`/`grep` 要复用 eino 实现，再回来讨论） |
| **Q4** | 历史 `sys_agent_schedules` 的 run 记录是否需要在 UI 可见？ | A 不可见（只保留归档表）／B 加只读「历史记录」入口 | A（避免为一个过渡期需求增加页面与文案） |
| **Q5** | CLI 的模型出口 | A 走 SimpleBase HTTP API（需要服务端在线）／B 直连供应商 API（CLI 自管 key）／C 混合 | A（决策 D-D1；B 与「密钥不落本地配置」的既有约束冲突） |
| **Q6** | CLI 的 `shell` 工具默认是否需要确认？ | A 只对破坏性命令确认（参考 claude-code）／B 每次命令都确认／C 默认不确认（`--yolo` 才确认） | A（`destructiveArgv` 思路 + 新规则表） |
| **Q7** | CLI 的「项目信任」采用哪种粒度？（Pi 用 `~/.pi/agent/trust.json` 记目录级信任，`pi-readme` §2.7） | A 无信任概念（每次确认）／B 目录级（`~/.gocloudagent/trust.json`）／C 会话级（`--trust`，进程退出失效） | **C 会话级**：CLI 是本地工具，但 `destructiveArgv` 的确认**不因信任而跳过**（§13.10） |
| **Q8** | 云端会话树（`sys_agent_messages` 加 `parent_id`/`kind` + `sys_agent_threads` 加 `leaf_id`，v50）本次是否实施？ | A 不实施（本次只做 CLI 侧树）／B 实施 | **A 不实施**：阻塞项是「run 无有效技能快照」+「跨 Agent 交接未决」（§13.4 / `cloud-agent-module-mentions-and-thread-routing-plan.md` §2） |
| **Q9** | 压缩触发口径：用 eino 的绝对 `Trigger.ContextTokens`，还是 Pi 的「窗口 - reserveTokens」？ | A 绝对值（简单，但换模型要改配置）／B 按模型上下文窗口算（窗口未知时不压缩，对齐 `pi-readme` §4.6）／C 用 `Trigger.ContextMessages`（按消息条数，最粗） | **A**（**已核实 llmgateway 不暴露窗口**：全仓无 `ContextWindow`/`context_length` 字段，`Service` 只有 `Chat`/`Stream`/`ListProviders`/`ProviderModels`。选 B 需要先给 `llmgateway` 加「模型 → 窗口」能力，超出本计划范围；先按 **A + 保守默认值**，并在 `Trigger.ContextMessages` 上加兜底条数）。若将来 `llmgateway` 补上窗口，再切 B |

---

## 13. 议题 E：对齐 `pi-readme.md` 的能力

本节是 §1.7 判定表的落地设计。每一项都给出**Pi 的做法 → 本仓库的落点 → 必须保住的行为 → 测试**。骨架代码参考 `pi-readme.md` §5，但**不照抄**——它是设计稿，省略了 import、错误类型与 `systemdb` 映射。

### 13.1 工具管道六段式与「阻断是一等结果」（P1 / P2 / P5）

**Pi 的做法**（`pi-readme` §2.3）：`prepareToolCall` 是固定六段——查找工具 → 可选 `prepareArguments` → `validateToolArguments` → `beforeToolCall` → 检查 `AbortSignal` → `execute` → `afterToolCall`。`beforeToolCall` 返回 `{block: true, reason, terminate}` 时**不执行工具**，`reason` 成为错误文本。`afterToolCall` 可按字段覆盖 `content`/`isError`/`terminate`（**不做深合并**）。

**现状差距**：

| Pi 段 | 本仓库现状 | 缺口 |
| --- | --- | --- |
| 查找工具 | ADK `ToolsNode` 内部按名查找；未知工具名默认返回 error（未设 `UnknownToolsHandler`） | 无 `UnknownToolsHandler` 注册，模型幻觉的工具名会让整次 `Invoke` 失败（`eino-readme.md` §6.3） |
| `prepareArguments` | 无 | — |
| `validateToolArguments` | `utils.InferTool` 反射出 JSON Schema，调用时反序列化成 `T` | 校验在**反序列化处**，失败信息不统一；CLI 另有 `AllowArgv` 等第二层校验 |
| `beforeToolCall`（可阻断） | **无通用钩子**。破坏性判定 `destructiveArgv` 塞在 `execSimplebase` 内部（`cli_tool.go:221-261`） | 这是最大缺口：确认逻辑与工具实现耦合 |
| 检查 abort | `errorTolerantTool` 对 `context.Canceled`/`DeadlineExceeded` 原样抛出（`tools.go:304-306`） | 已正确 |
| `execute` | `InvokableRun` | — |
| `afterToolCall` | 无 | 无统一的结果后处理点 |

**落点**（ADK 版本，不引入自研 loop）：

```go
// before_tool.go（新增）
// BeforeTool 对齐 pi 的 beforeToolCall。Block 时不执行工具，Code/Reason 成为错误结果。
type BeforeTool func(ctx context.Context, call PreparedCall) (BeforeDecision, error)

type PreparedCall struct {
    CallID string
    Name   string
    Args   json.RawMessage
}

type BeforeDecision struct {
    Block     bool
    Code      string // skill_not_loaded / confirmation_denied / confirmation_unavailable / usage
    Reason    string
    Terminate bool
}
```

接入方式二选一，按 §C1 阶段的实施顺序：

- **过渡（ADK 中间件）**：`adk.ChatModelAgentConfig.Handlers` 里放一个实现 `WrapInvokableToolCall` 的 handler，在真正执行前跑 `beforeTool` 链。`eino-readme.md` §9.2 明确 `WrapInvokableToolCall` 可以「包单次工具执行」。
- **终态（自研 loop）**：只在 §13.3 的条件成立时才走。

**`UnknownToolsHandler` 必须补上**（`eino-readme.md` §16 常见问题表把「tool not found」列为已知现象）：

```go
ToolsNodeConfig: compose.ToolsNodeConfig{
    Tools: tools,
    UnknownToolsHandler: func(_ context.Context, name, _ string) (string, error) {
        return skillFail("unknown_tool", "未知工具: "+name), nil
    },
    ToolCallMiddlewares: []compose.ToolMiddleware{confirmMW},
}
```

**`stopReason === "length"` 的处理**（P5，Pi 的 `failToolCallsFromTruncatedMessage`）：当前 `gocloudagent` 对模型输出被截断（`FinishReason == "length"`）**没有任何处理**——如果被截断的消息里带了残缺的 `ToolCalls`，ADK 会照样执行它们。这是**真实缺陷**（参数不完整 → 工具执行错误或危险操作）。

落点：在事件循环里检查 assistant 消息的 `ResponseMeta.FinishReason`；为 `"length"` 时对每个 `ToolCall` 直接发 `tool_result` 错误（`{"ok":false,"error":{"code":"truncated"}}`），**不执行**，并给整轮 `Reason="length"`。`gatewayChatModel` 已经把上游 `FinishReason` 透传进 `ChatResponse.FinishReason`（`access.go:52`），`parseAssistant`/`assistantToolMessage` 需要把 `ToolCalls` 与 `FinishReason` 一起放进 `schema.Message`（用 `ResponseMeta.FinishReason`）。

**「先 preflight 再并发」的顺序保证**（P5）：ADK `ToolsNode` 默认并行（`ExecuteSequentially: false`）。Pi 的做法是「先按顺序 preflight（含 `beforeToolCall`），允许的调用并发执行」。**必须保证**：同一轮两个破坏性确认不能交错（`pi-readme` §3.3 已记录「确认必须顺序：同一轮两笔删除不能交错弹窗」）。

落点：`simplebase` 工具始终声明顺序执行，因此 `ToolsNodeConfig.ExecuteSequentially = true`（**最保守，也最简单**）。只读工具本来无副作用，牺牲的并行度可以接受。若后续要做并行优化，再引入 Pi 的「preflight 顺序 + 执行并发」两级模型。

**测试**：

| 用例 | 锁定的行为 |
| --- | --- |
| unknown tool | 模型给出未注册工具名 → `tool_result` + `is_error`，**整轮不失败** |
| length 截断 | `FinishReason="length"` + 残缺 `ToolCalls` → 每个 call 一条 `truncated` 错误，CLI `Run` 调用次数为 0 |
| 确认顺序 | 同一轮两个破坏性 call → `confirmation_required` 串行出现，不交错 |
| block 不执行 | `beforeTool` 返回 `Block` → 结果是错误 JSON，`tool_start` **不发出** |

### 13.2 三层事件命名与部分结果（P3 / P4）

**Pi 的三层**（`pi-readme` §2.2）：供应商流 `AssistantMessageEvent`、agent 流 `AgentEvent`、durable 投影流。**不要叠成一种**。

**本仓库的现状与决策**：

| 事件 | 现状 | 决策 |
| --- | --- | --- |
| `token` | `consumeAssistant` 按 `chunk.Content` 发（`runtime.go:666-670`） | **不改名**（前端已依赖） |
| `tool_call_delta` | 增量参数，`isToolDelta` 判定（id 前缀 `delta:` 或 `Extra["delta"]`） | **不改名**，但**去掉 `delta:` 前缀这个 hack**——Pi 的 call id 在 `toolcall_delta` 阶段就是稳定的。见下 |
| `tool_call` | 完整参数 | 保留 |
| `tool_start` | 只在 `execSimplebase` 确认通过后发（`cli_tool.go`） | **扩大到所有工具**（只读也发），位置在执行前。这样 handler 的 `tool_progress` 能挂上只读 call（`pi-readme` §3.3 已把这个列为差距） |
| `tool_result` | 截 4000 rune，带 `duration_ms`/`truncated`/`is_error` | 保留 |
| `tool_execution_update` | **无** | **新增**（仅沙盒与 CLI 长命令），见下 |
| `thinking` | handler 的空闲心跳（2 秒无 token 无工具） | **不改语义**。若要暴露模型推理增量，**必须新起 type**（如 `reasoning_delta`）且默认关闭（`pi-readme` §3.3 明确记录「名字撞车」） |
| `tool_progress` | handler 每 2 秒按已开始的 call 补一帧 | `tool_start` 扩大到只读工具后，这个机制的价值才完整 |

**`delta:` 前缀的清理**：`isToolDelta` 现在靠 `strings.HasPrefix(tc.ID, "delta:")`（`runtime.go:597-599`）。`gatewayChatModel.Stream` 在 id 未到时写 `"pending"`（`eino-readme.md` §15.2 记录），一旦写出占位符又改成真实 id 就会触发 `cannot concat ToolCalls with different tool id`。**这是已知的脆弱点**。

Pi 的做法是：`toolcall_delta` 是独立事件类型，`contentIndex` 区分块，call id 与它解耦。落点：**保留 `Extra["delta"]=true` 作为主判据，去掉 `delta:` 前缀判据**；`gatewayChatModel` 不再往 `ID` 里塞前缀，`isToolDelta` 只看 `Extra`。这一步是纯清理，不改对外事件名。

**`tool_execution_update`（P4）**：仅对**长时间运行的 CLI / 沙盒命令**有意义。落点：

- `CLIRunner.Run` 改为可选流式接口，或新增 `RunStream`（`io.Writer` 回调），把子进程 stdout 的行推成 `tool_execution_update`；
- `Sandbox` 接口同理（现有 `Exec`/`Shell` 是一次性返回，需要 `eino` 的 `StreamableTool` 或新增回调方法）；
- **云端 Agent 的第一版可以不做**，因为 CLI 默认超时 60 秒、沙盒命令通常短。这一项标记为「CLI 侧优先」（交互式 REPL 里长命令的进度反馈体验差异最大）。

**测试**：`tool_start` 对只读工具也发出的断言；`isToolDelta` 仅凭 `Extra` 判定的断言（构造 `ID` 无前缀的增量 chunk）。

### 13.3 自研 `RunLoop` 的触发条件（对 §4「不做」的修正）

§4/§10.2 原本把「自研 `RunLoop` 替换 `runner.Run`」列为**不做**，理由是 `pi-readme` §4.11 的双轨风险。读完 `pi-readme` 后需要**修正为有条件做**：`pi-readme` §4.3 的结论是「ADK middleware 只作为确认改造的过渡」，且给出两条 ADK 解决不了的问题：

1. **参数增量与执行开始是两种事件**——ADK 的 `MessageVariant` 把工具增量塞进 `schema.ToolCall`，call id 要等前缀剥掉才稳定（§3.3 明确记录）；
2. **压缩切点会切在完整 turn 中间**——ADK 的 `summarization` 与 Pi 的 `compact` 语义不同（见 §13.5）。

因此决策改为：

```text
触发条件（满足任一条才启动自研 RunLoop，全部为「已确认的缺陷」）：
  T1. 13.1 的 length 截断处理在 ADK 事件循环里做不到干净实现
      （需要在模型流结束后、工具执行前介入，ADK 的事件粒度不给这个位置）
  T2. 13.5 的压缩切点必须落在完整 turn
      （eino summarization 的 Finalize 钩子够用则不需要自研；见 13.5 的结论）
  T3. UI 明确要求「参数还在长」与「工具已开始」是两个独立事件且 call id 稳定
      （当前靠 Extra["delta"] + 前缀剥离，属于补丁）

不满足任一条件：继续用 ADK + 中间件（现状 + §4.2），不写自研 loop。
满足任一条件：按 pi-readme §4.10 的步骤 4 替换，且必须一次替换干净（不留双轨）。
```

**为什么不能「两套并存」**（`pi-readme` §4.11 的风险项）：`react.AgentConfig.MaxStep`、`adk.ChatModelAgentConfig.MaxIterations`、`compose.WithMaxRunSteps` 是三套独立上限（`eino-readme.md` §13.4），同时存在会有两套 `MaxIterations` 与两套错误码。本仓库只设 ADK 的 `MaxIterations`（默认 8，`runtime.go:207-209`），自研 loop 必须接管这一个数字，不能新增第二个。

**若启动自研 loop，接口形状**（对齐 `pi-readme` §4.2 与本仓库现状）：

```go
// 保留现有名字：Event / RunRequest / RunResult / ConfirmationWaiter / ChatClient
// 新增（仅内部）：
type StreamDelta struct {
    Kind      string // start, text, thinking, toolcall_delta, toolcall_end, done, error
    Text      string
    CallID    string
    Name      string
    ArgsDelta string
    ToolCall  *schema.ToolCall
    Stop      string // stop, length, tool_use, error, aborted
    Err       error
}

type ModelStreamer interface {
    Stream(ctx context.Context, msgs []*schema.Message, tools []*schema.ToolInfo) (<-chan StreamDelta, error)
}
```

`gatewayChatModel` 加 `StreamDeltas` 实现（`pi-readme` §5.2 的骨架可直接用，它把 `*schema.StreamReader[*schema.Message]` 收成 `<-chan StreamDelta`，使 loop 不必认识 `delta:` 前缀）。

**必须保住的语义**（与 ADK 路径逐项一致，否则替换不算完成）：`MaxIterations` 默认 8 与超限文案、`RunTimeout` 默认 180s、`CancelRun` 按 RunID 取消、`claimThread`/`releaseThread`、`Reason` 取值（`stop`/`max_iterations`/`canceled`/`timeout`/`error`）、`ToolCallsJSON` 卡片数组形状。

### 13.4 会话树与分支（P6）

**Pi 的数据模型**（`pi-readme` §2.5）：`~/.pi/agent/sessions/--<path>--/<ts>_<id>.jsonl`；首条 `SessionHeader`，其后每条 `SessionEntryBase` 带 `id`/`parentId`/`timestamp`；`leafId` 指向当前条目；`getBranch(fromId)` 从叶子回溯到根；`branch(fromId)` 只移动叶子，**不复制消息**；`getTree()` 得到 `SessionTreeNode`。模型上下文不是整棵树，而是**根到叶子的路径**（`buildContextEntries` 在路径上找最近的 compaction）。

**本仓库的落点**：**分两处，策略不同。**

| 位置 | 现在 | 决策 |
| --- | --- | --- |
| **CLI（`~/.gocloudagent/sessions/*.jsonl`）** | 全新 | **直接做树**（`parent` 字段从第一天就在）。第一阶段 `parent` 恒为前一条（线性），但**格式预留**，后续加 `branch`/`leaf` 不需要迁移。理由：本地文件没有迁移成本，且 `resume`/`fork` 是 CLI 的核心体验 |
| **云端（系统库）** | `AgentMessage` 无 `parent_id`；历史线性 + 字符截断 | **暂缓**（需要迁移 v50 加 `parent_id`/`kind`/`leaf`，且要跨 thread 归属校验与 `agent_switch_required` 语义）。见下 |

**云端暂缓的具体理由**（不是"以后再说"，而是有明确阻塞项）：

1. `pi-readme` §3.3 已记录：「run 当前无有效技能字段，必须先定义快照及旧 run 重试安全退化」——会话树的分支会放大这个问题；
2. `cloud-agent-module-mentions-and-thread-routing-plan.md` §2 的「跨 Agent 单轮交接」仍待决策，会话树的执行者归属与它耦合；
3. 现有的 `historyToSchema` 折叠策略（tool 消息压成 `UserMessage("TOOL_RESULT\n...")`）是**为避免残缺 tool_call id 而刻意做的**（`eino-readme.md` §15.4 与 §16 都记录了这个取舍）。改成树之前必须先解决「历史里的 tool call 能安全还原成配对」。

**云端的渐进路径**（供后续 plan 使用，本次不实施）：

```text
v50 迁移（仅定义，不实施）：
  ALTER TABLE sys_agent_messages ADD COLUMN parent_id VARCHAR;   -- 空 = 线性根
  ALTER TABLE sys_agent_messages ADD COLUMN kind VARCHAR;        -- message / compaction / branch_summary
  ALTER TABLE sys_agent_threads  ADD COLUMN leaf_id VARCHAR;     -- 当前叶子
旧数据空 parent → 按线性链投影（`pi-readme` §4.10 步骤 6 的做法：不回填历史）。
```

**投影算法**（`pi-readme` §5.5 的 `projectBranch` 可直接用）：从 `leaf` 走到根、反转、找最近的 `kind=compaction`、只把摘要与该点之后的条目交给模型，**原文保留在表里供界面查看全树**。

### 13.5 压缩（P7）

**Pi 的做法**（`pi-readme` §2.5 / §5.4）：`shouldCompact` 条件为 `enabled && contextTokens > contextWindow - reserveTokens`，默认 `reserveTokens=16384`、`keepRecentTokens=20000`；`prepareCompaction` 按 `keepRecentTokens` 找切点，**切点落在完整 turn**（不拆开工具调用与结果）；`compact` 调模型写摘要；**原文不删**，后续请求只是不再把切点之前的消息放进上下文。

**现状**：`gocloudagent` **无压缩**。`historyToSchema` 从最近往回装，预算 `maxHistoryChars=12000`，单条截 2000（`runtime.go:514-546`）。截断点不保证落在完整回合——`pi-readme` §3.3 明确记录这个缺陷。

**关键核实结果（决定实施方案）**：`eino` 的 `adk/middlewares/summarization` **不是** Pi 的语义。已读源码确认：

| 维度 | eino `summarization` | Pi `compact` |
| --- | --- | --- |
| 触发 | `Trigger.ContextTokens`（默认 160000）或 `ContextMessages` | `contextTokens > contextWindow - reserveTokens` |
| 保留近期 | **不保留**。`DefaultFinalize` 把**全部非 system 上下文**换成一条摘要（`splitSystemAndContextMsgs` + `postProcessSummary`） | 保留最近 `keepRecentTokens` 的完整回合 |
| 保留 skills | `FinalizerBuilder.PreserveSkills`（`MaxSkills` 默认 5、`MaxTokensPerSkill` 默认 5000、`SkillsTokenBudget` 默认 25000） | 无对应 |
| 切点对齐 turn | **无此概念**（切点是"全部 context"） | `cutByTurn` 按 turn 切 |
| 重试/故障转移 | `RetryConfig`（`MaxRetries` 默认 3）、`FailoverConfig` | 无 |

**落点**：用 `summarization` 中间件，但**必须提供自定义 `Finalize`**（`Config.Finalize` 是 `TypedFinalizeFunc[M]`，签名 `func(ctx, originalMessages []M, summary M) ([]M, error)`，且「设置后中间件不做任何后处理」）：

```go
// compaction.go
// finalizeKeepRecent 实现 Pi 的「摘要头 + 保留近期完整回合」语义。
// 先用 DefaultFinalize 处理切点之前的部分（得到 system 消息 + 摘要），
// 再把切点之后的原始消息原样接回。
func finalizeKeepRecent(keepTokens int) summarization.FinalizeFunc {
    return func(ctx context.Context, original []*schema.Message, summary *schema.Message) ([]*schema.Message, error) {
        _, contextMsgs := splitLeadingSystem(original)      // 与 middleware 同口径
        cut := cutByTurn(original, keepTokens, estimateTokens)
        if cut <= 0 {
            return summarization.DefaultFinalize(ctx, original, summary)
        }
        head, err := summarization.DefaultFinalize(ctx, original[:cut], summary)
        if err != nil {
            return nil, err
        }
        return append(head, original[cut:]...), nil
    }
}
```

`cutByTurn` 直接按 `pi-readme` §5.4 的骨架实现（从尾部按完整回合累加，超过 `keepTokens` 即停），并补一条 Pi 的细节：**若某个 assistant 的 tool call 还没有全部对应的 tool 消息，把切点移到该 assistant 之前**（`pi-readme` §5.4 末句）。

**配置落点**（与 Pi 的默认值对比，按云 Agent 的实际规模调）：

| 参数 | Pi 默认 | 本计划取值 | 理由 |
| --- | --- | --- | --- |
| `reserveTokens` | 16384 | 与 `Trigger.ContextTokens` 合并使用 | eino 用绝对 token 阈值，不用"窗口 - reserve"；按 `llmgateway` 已知模型窗口设置，**窗口未知时不压缩**（对齐 `pi-readme` §4.6） |
| `keepRecentTokens` | 20000 | 4096 | `pi-readme` §4.6 明确「云 Agent 工具定义比 Pi 的编码工具短，不照抄 16384」 |
| 摘要写入位置 | `compaction` 条目（树） | **云端：不新增表**，摘要作为 `schema.SystemMessage("[compacted]\n"+summary)` 只在**本轮**生效 | 云端没有会话树，落库需要迁移；本轮生效可立刻解决「上下文超限」 |

**硬约束**：

1. **摘要正文不写日志、不写审计**。`internal/AGENTS.md` 的 trace 例外**不覆盖**摘要——`tracer.Payload` 不得用于摘要内容；
2. **落库/进上下文前跑 `RedactSecrets`**（`prompt.go:91`）；
3. **摘要失败必须保留原路径**（不丢历史、不改 `Reason`、不给用户新增可见错误码）——Pi 的 `compact` 失败语义，`pi-readme` §5.4 的 `continue` 就是它；
4. **不启用 `EmitInternalEvents`**（会往事件流里插 `ActionTypeBeforeSummarize` 等自定义事件，前端不认识）。

**测试**：切点不拆开工具回合（构造「assistant 带 2 个 tool call + 只有 1 个 tool 结果」的尾部，断言切点移到该 assistant 之前）；摘要失败后 `view` 不变；摘要文本先脱敏（构造含 `sk-` 的摘要，断言落进上下文的版本已 `[REDACTED]`）。

### 13.6 Skills 完善（P8）

**现状**（`skills.go`，260 行）：`skillPrefixes` 8 个前缀表、`skillAliases` 中文别名、`ResolveAtToken`、`NormalizeSkills`、`AllowArgv`、`SkillPrompt`（选中子技能**全文**注入，上限 8192 rune）、`readSkill`/`stripFrontmatter`、`walkSkillDocs` + 两个对账函数（`EmbeddedSkillAllows` / `EmbeddedSkillReadOnly`）。embed 位置 `skills/simplebase/SKILL.md` + `references/*.md`（9 个文件）。

**Pi 的做法**（`pi-readme` §2.6）：`formatSkillsForPrompt` **只把 name/description/path 写进系统提示**；正文由模型用 `read` 按路径去读，或用户发 `/skill:name`；`disable-model-invocation: true` 的技能不进提示；发现规则是「目录有 `SKILL.md` 就是技能根，不再往下」。

**落点，四项**：

| # | 改动 | 依据 |
| --- | --- | --- |
| E1 | 提示里**只放目录**（name + description + 「何时用」），正文靠 13.6 的 `skill` 工具按需读 | §4.2 已决定用 `adk/middlewares/skill` + 自研 embed Backend |
| E2 | 支持 `disable-model-invocation: true` | Pi 的 frontmatter 字段（`SkillFrontmatter`）。**本仓库的 skill 文档还没有这个字段**（`skills/simplebase/references/*.md` 用的是 `<!-- allow: -->` / `<!-- readonly: -->` 注释）——若要支持，需要在 frontmatter 加字段并让 `walkSkillDocs` 解析 |
| E3 | 保留 `@` 解析，**不引入 `/skill:name`** | `pi-readme` §3.3 明确：「`@` 是控制台协议，要保留。Pi 的斜杠命令不能替换它」。CLI 侧可以额外支持 `/skill:name`（本地工具，无前端协议约束） |
| E4 | `@` 选中的 skill **本轮正文直接缓存**进首次工具结果，省一次 round-trip | `pi-readme` §4.7：「已经通过 `@` 选中的技能可以在本轮把正文直接放进工具结果缓存，避免模型多一次 `skill_read`」 |

**必须保住的授权边界**（`pi-readme` §4.11 的风险项，也是本仓库的核心安全机制）：

```text
AllowArgv 只看 RunRequest.Skills，不因为模型调用了 skill 工具就扩大 argv 前缀。
读正文 ≠ 获得写权限。Pi 把它们混在「能 read 就能照着做」里，云 Agent 不能。
```

**顶层 `SKILL.md` 的改造重点**：E1 之后，模型不再看到子技能全文，因此顶层索引**必须**承担「何时用哪个 skill」的职责。当前顶层文件需要按 Pi 的 `formatSkillsForPrompt` 形状重写为「技能表 + 使用时机」，而不是「目录说明」。

**对账测试的更新**：现有 `skills_test.go` 把 embed 的 `<!-- allow: -->` 与 `skillPrefixes` 对账（12 个 Test）。E1/E2 之后要**追加**两项：`skill` 工具返回的正文必须来自 `KnownSkill` 白名单（不是任意路径）；`disable-model-invocation` 的技能不出现在工具描述里。

### 13.7 Prompt templates（P9，仅 CLI）

**Pi 的做法**（`pi-readme` §2.6）：模板文件在 `~/.pi/agent/prompts/`，类型 `PromptTemplate`（`name`/`description`/`argumentHint?`/`content`），`expandPromptTemplate` 在**用户消息落地前**做替换，支持 `$1`、`$@`、`${1:-default}`；`parseCommandArgs` / `substituteArgs` / `loadPromptTemplates` 是同文件导出。

**落点**：**只在 CLI 做**，云端不做。理由：云端的用户输入入口是 `RunRequest.UserText`，由 HTTP handler 组装，加模板替换会让「用户输入的原文」与「实际发给模型的内容」不一致，影响 `sys_agent_messages` 的落库语义与审计。

CLI 侧实现（`~/.gocloudagent/prompts/*.md`，YAML frontmatter 提供 `name`/`description`/`argument-hint`）：

```text
gocloudagent /review 3          → 展开 prompts/review.md，$1 = "3"
gocloudagent /explain           → $1 缺失，用 ${1:-整个文件} 的默认值
```

替换语法只支持 `$1`~`$9`、`$@`（全部参数）、`${N:-默认值}`。**不做嵌套、不做表达式**——Pi 的 `substituteArgs` 也就是这个范围。

**与 skill 命令的区分**：`/` 前缀只走 prompt template（用户主动提供的提示词片段），`@` 只走 skill（能力授权）。**两者不混用**，避免「展开一段文本就获得了写权限」的错觉。

### 13.8 多外壳模式（P10）

**Pi 的做法**（`pi-readme` §2.8）：同一 `AgentSession` 服务 TUI / print / JSON / RPC / SDK 五种模式；`ExtensionMode = "tui" | "rpc" | "json" | "print"`。

**落点**：**部分采用**，落点全在 CLI（云端已有 SSE 一个出口，够了）。

| 模式 | 实现 | 用途 |
| --- | --- | --- |
| `interactive`（默认） | 行缓冲 REPL + ANSI 流式渲染 | 人类交互（§5.4 已设计） |
| `print` | `gocloudagent "..."` 或 `-p "..."` | 一次性执行，纯文本输出，**非 TTY 时自动切换** |
| `json` | `gocloudagent run --json "..."` | stdout 每行一个 JSON 事件（**与云端 SSE 的事件名保持一致**，便于脚本复用） |
| `rpc` | 不做 | `pi-readme` §2.8 说明 RPC 是 stdin/stdout JSONL，供 IDE 集成。本计划无此需求 |
| `sdk` | 不做（Go 库即 SDK） | Go 侧的复用已经通过 `internal/gocloudagent/agentcore` 实现，不需要额外的进程内 SDK |

**关键约束**：`json` 模式的事件名**必须与 SSE 一致**（`token`/`tool_call_delta`/`tool_call`/`tool_start`/`tool_result`/`error`/`end`），否则脚本要维护两套解析。这是 `internal/gocloudagent/agentcore` 统一事件类型的直接收益。

### 13.9 行为评估 harness（P11）

**Pi 的做法**（`pi-readme` §2.9）：`packages/evals` 的 `createPiCodingAgentHarness` 在临时目录里 `createAgentSession`、逐步 `prompt`、收集 `session.messages`；`createTaskPlan` 把用例展开成 `(case, variant, repetition)`；`*.docs.eval.ts` 用 Docker 双变体算「有文档 vs 无文档」的 lift。

**现状**：只有**协议层** e2e（`internal/api/agent_skills_e2e_test.go` 断言 SSE 顺序，`agent_skills_router_e2e_test.go` 打真实 router），**没有行为层**——即「同一个任务、固定模型、记录会话」的用例。

**落点**（`pi-readme` §4.9 的缩小版，**不引入 vitest-evals 或 Docker 双变体**）：

新增 `internal/gocloudagent/eval_test.go`（或独立包 `internal/gocloudagent/eval`）：

```go
type EvalCase struct {
    Name     string
    Skills   []string
    Headless bool
    Script   []scriptStep    // 假模型按顺序返回的 delta
    Want     []wantEvent     // 事件类型、name、call_id 关系、is_error
    WantCode string          // 期望的错误码
}
```

**假模型不访问网络**（延续现有 `scriptedLLM`/`scriptModel` 的做法）。断言打在 `Event.Type` 与工具结果的错误码上，**不比较模型散文**。

**必须覆盖的案例**（`pi-readme` §4.9 的表，逐条对应本仓库）：

| 案例 | 锁定的行为 | 关联章节 |
| --- | --- | --- |
| 只读 SQL | `readonly_sql` 的写语句被 `sqlguard` 拒绝 | 现状已有（`tools_test.go`） |
| 未加载技能 | `simplebase` argv `database ...` 在 skills 为空时 `skill_not_loaded`，且**没有** `tool_start` | §13.1 |
| 删除确认拒绝 | `confirmation_required` 后 `Approve: false` → `confirmation_denied`，CLI `Run` **未被调用** | §4.2 / §13.1 |
| Headless 删除 | 无 `confirmation_required`，有 `confirmation_resolved` 且未批准 | 现状已有（`writable_test.go` 相关） |
| 参数增量 | 先若干 `tool_call_delta`，再一条 `tool_call`，**call id 相同** | §13.2 |
| 压缩切点 | 工具调用和它的结果**不被摘要切成两半** | §13.5 |
| **length 截断**（新增） | `FinishReason="length"` → 每个 call 一条 `truncated` 错误，不执行 | §13.1 |
| **unknown tool**（新增） | 未注册工具名 → `is_error` 结果，整轮不失败 | §13.1 |

`failIfCalled` 模式（`pi-readme` §5.6）：一个实现 `CLIRunner` 的 fake，`Run` 时 `t.Fatal`——用它证明确认发生在子进程之前。

**协议顺序断言继续留在 HTTP e2e**，不把 SSE 解析搬进 `eval_test.go`（`pi-readme` §5.6 的明确分工）。

### 13.10 项目信任：一个需要显式处理的空缺（不引入 Pi 的模型）

`pi-readme` §2.7 记录：Pi **没有**内置的文件系统/进程/网络/凭据权限系统，也不在每次工具调用前询问；**存在的门只有项目信任**（`ProjectTrustStore` 记在 `~/.pi/agent/trust.json`，默认 `defaultProjectTrust: "ask"`），且 print/json/RPC 模式**不能**弹出信任对话框。

**这个模型不能引入云端**（`pi-readme` §3.2：「把 Pi 的信任模型换成『问一次项目是否可信』会放宽现有保护」）。云端的授权是三层既有的：

```text
1. HTTP 层：auth.Require(permission) + writeGate（路由级）
2. Agent 层：Agent.Writable（实例/助手级，migration v48）
3. 工具层：AllowArgv + destructiveArgv + ConfirmationWaiter + Headless 拒绝
```

**但 Pi 的「项目信任」概念在 CLI 侧有一个真实对应物需要决策**：CLI 在**用户的机器**上运行 `shell` / `write_file`，那就是用户自己的权限。`pi-readme` §2.7 说明 Pi 用 `trust.json` 记住「这个目录可信」，避免每次问。

落点（新增开放问题 **Q7**，见 §12）：

| 选项 | 语义 | 代价 |
| --- | --- | --- |
| A 无信任概念 | 每次启动重新确认 `shell` 的破坏性命令 | 繁琐，但与云端语义统一 |
| B 目录级信任 | `~/.gocloudagent/trust.json` 记「这个 cwd 已信任」，可信目录内跳过非破坏性命令的确认 | 与 Pi 同形；需要处理「目录被换掉」的边界 |
| C 会话级信任 | `--trust` flag 对本次会话生效，进程退出即失效 | 最保守，无需持久化 |

**本计划倾向 C**（会话级），理由：CLI 是本地工具，用户的容忍度高于云端；但**破坏性命令的确认必须保留**（`pi-readme` §4.11 的风险项），信任只影响「非破坏性命令是否需要确认」，不影响 `destructiveArgv`。

### 13.11 议题 E 的阶段与依赖

| 阶段 | 内容 | 依赖 | 门槛 |
| --- | --- | --- | --- |
| **E1 工具管道** | `BeforeTool`/`BeforeDecision`；`UnknownToolsHandler`；`length` 截断处理；`ExecuteSequentially=true` | C1（确认中间件已就位） | §13.1 的四条测试全绿 |
| **E2 事件** | `isToolDelta` 去前缀判据；`tool_start` 覆盖只读工具 | E1 | SSE 顺序 e2e 不变 |
| **E3 压缩** | `summarization` + 自定义 `Finalize`（`cutByTurn`）；替换 `maxHistoryChars` | E2 | §13.5 的三条测试全绿 |
| **E4 skills** | 目录化提示；`disable-model-invocation`；`@` 正文缓存 | §4.2 的 skill 中间件 | `skills_test.go` 对账扩展通过 |
| **E5 评估** | `eval_test.go` 表驱动，8 个案例 | E1-E4 | 假模型，无外网；`go test -short ./internal/gocloudagent` |
| **E6 CLI 外壳** | `print`/`json` 模式；prompt templates；会话级信任 | D（CLI 骨架） | §13.8 的事件名与 SSE 一致 |
| **E7 tool 进度** | `tool_execution_update`（CLI / 沙盒流式） | E6 | 长命令有增量输出；不影响一次性返回路径 |
| **E8 云端会话树** | `parent_id`/`kind`/`leaf_id` 迁移 + 树投影 | 阻塞（见 §13.4） | **本次不实施**，待 §12 Q8 决策 |

**E1-E5 可以并入 C 阶段**（它们是 `gocloudagent` 内部的能力补齐，与议题 C 的中间件化天然同批）。**E6-E7 并入 D 阶段**。**E8 独立后置**。

**E8 的阻塞项**：`pi-readme` §3.3 与 `cloud-agent-module-mentions-and-thread-routing-plan.md` §2 都指向同一个未决问题——**run 没有有效技能快照，且跨 Agent 交接未定**。会话树会放大这个问题，因此必须先解决它，而不是先加 `parent_id`。

---

## 14. 参考

- Eino 能力与版本细节：[`internal/gocloudagent/eino-readme.md`](../../internal/gocloudagent/eino-readme.md)（v0.9.19；§8 多 Agent、§9 ADK、§11 中断与 Checkpoint、§13 错误与重试、§15 本仓库用法、§16 常见问题）
- Pi 完整形态与目标设计：[`internal/gocloudagent/pi-readme.md`](../../internal/gocloudagent/pi-readme.md)（§2 十一项能力、§2.3 工具管道、§2.5 会话树与压缩、§2.6 skills/模板/扩展、§2.7 权限与沙箱、§2.8 五种外壳、§2.9 评估、§3.1 可借鉴、§3.2 不该照搬、§3.3 逐项对照、§4.2 接口、§4.5 工具注册与确认、§4.6 会话与压缩、§4.7 skill 加载、§4.9 评估、§4.10 迁移步骤、§4.11 风险、§5 骨架代码）
- 既有两份 scheduler 的实现：`internal/cronjob/scheduler.go`、`internal/gocloudagent/scheduler.go`（本次删除）
- 曾明确「不统一」的边界：[`backend-code-simplification-plan.md`](./backend-code-simplification-plan.md) §2 末段
- 跨 Agent 交接与会话归属的未决项：[`cloud-agent-module-mentions-and-thread-routing-plan.md`](./cloud-agent-module-mentions-and-thread-routing-plan.md) §2
- 前端与 i18n 约定：[`ui/AGENTS.md`](../../ui/AGENTS.md)
- 后端边界：[`internal/AGENTS.md`](../../internal/AGENTS.md)、[`AGENTS.md`](../../AGENTS.md)

