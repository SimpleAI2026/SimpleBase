# Cloud Agent 定时执行（Agent Schedules）

> 状态：计划稿
> 配套：`cloud-agent-plan.md`、`proto-http.md` §3.12、`ui-plan-v2.md`
> 范围：`internal/`（新表 + 调度器 + HTTP）与 `ui/`（Agent 列表定时入口 + 提示词弹窗）
> 约束：**不新增第三方依赖**（cron 自实现受限子集）；单实例部署，进程内调度

## 1. 目标

- Agent 列表每个 agent 可配置**一条定时执行**：到点由后端调度器以指定**提示词**自动触发一次非流式 run，结果落到该 schedule 专属 thread（会话内可回看）。
- UI：`AgentManager.vue` 左栏 agent 卡片新增「定时」按钮，弹窗内填写**提示词框** + 频率 + 启用开关，并展示下次执行时间与最近执行历史。
- 定时执行与手动对话**同一条只读路径**：复用 `cloudagent.Runtime.StartRun`（eino ChatModelAgent + 只读工具），不新增任何写工具。

## 2. 明确不做（MVP）

- 分布式调度 / 多实例选主（沿用单实例可写副本 = 1 的部署约束）
- 时区选择：cron 一律按 **UTC** 解释，UI 标注 `(UTC)`
- 失败自动重试：失败只写 `failed` 记录，不重跑
- 每个 agent 多条 schedule：**每 agent 至多 1 条未归档 schedule**（应用层保证）
- 把 prompt / 密钥写入 `sys_cloud_agents` 或日志；审计与日志仍脱敏

## 3. 数据模型（系统 DuckLake）

`internal/systemdb/migrate.go` 追加两条只前进迁移（当前最高 v25）。

### v26 `sys_agent_schedules`

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | VARCHAR | UUID |
| `project_id` | VARCHAR | 项目隔离 |
| `agent_id` | VARCHAR | 目标 agent |
| `thread_id` | VARCHAR | 执行结果写入的专属 thread（创建时自动建） |
| `prompt` | VARCHAR | 定时执行的提示词（必填，非空） |
| `cron_expr` | VARCHAR | 受限 5 字段 cron，UTC |
| `enabled` | BIGINT | 1/0 |
| `last_run_at` | TIMESTAMP | 可空 |
| `next_run_at` | TIMESTAMP | 由服务端按 cron 计算 |
| `created_by` | VARCHAR | 创建者 APIKeyID |
| `created_at` / `updated_at` / `archived_at` | TIMESTAMP | 软删模式与 `sys_cloud_agents` 一致 |

### v27 `sys_agent_schedule_runs`

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | VARCHAR | UUID |
| `schedule_id` | VARCHAR | 关联 schedule |
| `project_id` / `agent_id` / `thread_id` | VARCHAR | 冗余，便于按项目/线程过滤 |
| `run_id` | VARCHAR | 关联 `sys_agent_runs.id` |
| `trigger` | VARCHAR | `scheduled` / `manual` |
| `status` | VARCHAR | 复用 `queued/running/completed/failed/canceled` 常量 |
| `error` | VARCHAR | 失败原因（脱敏） |
| `started_at` / `finished_at` | TIMESTAMP | 可空 |
| `created_at` | TIMESTAMP | — |

DuckLake 不用 PRIMARY KEY；唯一性与「每 agent 一条」由应用层保证。

## 4. Cron 解析（`internal/cloudagent/cron.go` + `cron_test.go`）

`systemdb` 是底层包，**不得 import `cloudagent`**；因此 cron 解析只在 `cloudagent` 与 `api` 层发生，store 只存时间戳。

- 5 字段：`分 时 日 月 周`，分钟粒度，UTC。
- 支持语法：`*`、`*/n`、`n`、`a-b`、逗号列表组合（如 `0 8 * * 1,4`）。
- 接口：

```go
func ParseCron(expr string) (CronSpec, error)          // 非法字段/越界 → 确定错误（handler 映射 400）
func (c CronSpec) NextAfter(t time.Time) (time.Time, error) // 逐分钟前进，上限 2 年；找不到 → 错误
```

- `NextAfter` 为朴素迭代（每 schedule 每次 tick 一次，量级极小），不做字段级跳转优化。
- 周 `0/7` 均视为周日；`日` 与 `周` 同时受限时取并集（标准 cron 语义）。

## 5. Store（`internal/systemdb/agent_schedules.go`）

模型：`AgentSchedule`、`AgentScheduleRun`（字段与上表一一对应，`LastRunAt` 用 `sql.NullTime` 扫描）。

方法（风格对齐 `agents.go`，全部 project 作用域 + `notifyWrite`）：

```go
CreateAgentSchedule(ctx, s AgentSchedule) (AgentSchedule, error)
GetAgentSchedule(ctx, projectID, id string) (AgentSchedule, error)
GetAgentScheduleByAgent(ctx, projectID, agentID string) (AgentSchedule, error) // ErrNoRows=未配置
ListAgentSchedules(ctx, projectID string) ([]AgentSchedule, error)
UpdateAgentSchedule(ctx, s AgentSchedule) (AgentSchedule, error)               // 含 enabled / thread_id / next_run_at
ArchiveAgentSchedule(ctx, projectID, id string) error
ListDueAgentSchedules(ctx, now time.Time, limit int) ([]AgentSchedule, error)  // enabled=1 AND next_run_at <= now
ClaimAgentSchedule(ctx, id string, expectNext, newNext, lastRun time.Time) (bool, error)
InsertAgentScheduleRun(ctx, r AgentScheduleRun) error
ListAgentScheduleRuns(ctx, projectID, scheduleID string, limit int) ([]AgentScheduleRun, error)
```

`ClaimAgentSchedule` 是 CAS 认领：`UPDATE ... SET next_run_at=?, last_run_at=? WHERE id=? AND next_run_at=?`，`RowsAffected==0` 返回 false——同一任务不会被并发执行两次。

## 6. 调度器（`internal/cloudagent/scheduler.go`）

### 依赖（接口，保持可测）

```go
type ScheduleRunner interface { // *cloudagent.Runtime 天然满足
    StartRun(ctx context.Context, req RunRequest, emit func(Event)) (RunResult, error)
}
type ScheduleStore interface { // *systemdb.Store 天然满足
    GetCloudAgent(...) / GetAgentThread(...) / CreateAgentThread(...)
    AppendAgentMessage(...) / ListAgentMessages(...)
    CreateAgentRun(...) / UpdateAgentRunStatus(...)
    ListDueAgentSchedules(...) / ClaimAgentSchedule(...) / UpdateAgentSchedule(...)
    InsertAgentScheduleRun(...) / RecordLog(...)
}
type ScheduleQuota interface { CheckQuota(ctx, projectID, kind string) error } // usage.Service 满足

type Scheduler struct {
    Store ScheduleStore
    Runner ScheduleRunner
    Quota  ScheduleQuota
    // tick 常量 30s；并发信号量默认 4；in-flight map 防 goroutine 泄漏
}
func (s *Scheduler) Start(ctx context.Context)   // goroutine + ticker，ctx cancel 即停
func (s *Scheduler) Trigger(ctx context.Context, schedule AgentSchedule) // 手动触发入口（同一条执行路径）
```

### tick 流程（每 30s）

1. `ListDueAgentSchedules(now, 100)` 取到期任务。
2. 对每条：`ParseCron` 解析失败（历史数据被改坏）→ `UpdateAgentSchedule(enabled=false)` + `RecordLog(warn)`；正常则算 `next := spec.NextAfter(now)`。
3. `ClaimAgentSchedule(id, old next_run_at, next, now)` CAS 认领；失败即跳过。
4. 信号量内 `go execute(schedule, trigger="scheduled")`。

### execute 流程（scheduled 与 manual 共用）

1. `Quota.CheckQuota(projectID, "llm")` 失败 → `InsertAgentScheduleRun(failed, "llm quota exceeded")`，结束。
2. `GetCloudAgent`：agent 已归档 → 记 failed `agent archived` 并 `enabled=false` 该 schedule。
3. `GetAgentThread`：thread 被归档 → 新建 thread（title `Scheduled: <agent name>`）并 `UpdateAgentSchedule.thread_id`。
4. 构造**内部 principal**（安全边界见 §8）+ `CreateAgentRun(running)` + `AppendAgentMessage(role=user, content="[scheduled] "+prompt)`。
5. `Runner.StartRun`（`Stream=false`，history 取该 thread 最近 40 条）。
6. 成功：`AppendAgentMessage(assistant)` + `UpdateAgentRunStatus(completed)`；失败/取消：对应状态与错误。
7. `InsertAgentScheduleRun(run_id, trigger, status)` + `RecordLog(logger="scheduler", message="agent schedule executed", fields={schedule_id, run_id, status})`——日志页可见，不落 prompt 原文。

## 7. HTTP（`internal/api/agent_schedule_handler.go` + `router.go`）

前缀 `:p` = `/v1/projects/:projectID`。错误协议走 `WriteError`。

| Method | Path | 权限 | 说明 |
|---|---|---|---|
| GET | `:p/agent-schedules` | DatabaseRead | `{ "schedules":[AgentScheduleDTO] }` |
| POST | `:p/agent-schedules` | DatabaseWrite | `{ agent_id, prompt, cron_expr, enabled?, thread_id? }` |
| GET | `:p/agent-schedules/:scheduleID` | DatabaseRead | 详情 |
| PATCH | `:p/agent-schedules/:scheduleID` | DatabaseWrite | `{ prompt?, cron_expr?, enabled?, thread_id? }` |
| DELETE | `:p/agent-schedules/:scheduleID` | DatabaseWrite | **204** 软删 |
| GET | `:p/agent-schedules/:scheduleID/runs` | DatabaseRead | `{ "runs":[...] }`，默认 20 条 |
| POST | `:p/agent-schedules/:scheduleID/run` | DatabaseRead | 手动立即执行（quota `llm`），不改 `next_run_at` |

校验与语义：

- `prompt` 空 → 400；`cron_expr` 解析失败 → 400 `invalid cron expression`；`agent_id` 不存在 → 400。
- 该 agent 已有未归档 schedule → **409 `schedule already exists`**（前端据此切 PATCH）。
- 创建/更新时服务端（handler 层）调用 `cloudagent.ParseCron + NextAfter(now)` 计算 `next_run_at` 后交给 store；`thread_id` 为空时自动 `CreateAgentThread(title="Scheduled: <agent name>")`。
- DTO：`last_run_at` / `next_run_at` 用 `*time.Time` + omitempty；附带 `agent_name` 便于 UI 渲染。
- `router.go`：`Dependencies` 增加 `AgentScheduler *cloudagent.Scheduler`；挂载在 `deps.System != nil` 分支，`deps.AgentScheduler != nil` 才挂 `/run` 手动触发路由。

`proto-http.md` 追加 §3.13 记录上表。

## 8. 安全边界

- 定时执行无 HTTP 请求上下文，构造**内置只读身份**：`auth.Principal{APIKeyID: "system-scheduler", ProjectIDs: {projectID}, Permissions: {DatabaseRead}}`——权限上限即只读工具所需，不经过 API key 鉴权，不得授予写权限。
- Prompt 组装仍走 `AssembleInstruction` + `RedactSecrets`，密钥永不入 prompt。
- 日志 / `sys_agent_schedule_runs.error` 只存脱敏错误摘要，不落 prompt 正文与工具结果原文。

## 9. 装配与生命周期（`internal/app/app.go`）

- `assembleDeps`：`systemStore != nil && cloudAgentRuntime != nil` 时创建 `cloudagent.Scheduler{Store: a.systemStore, Runner: runtime, Quota: a.usageSvc}`，传入 `api.Dependencies.AgentScheduler`。
- `Start()` 中 `scheduler.Start(ctx)`（goroutine）；`Shutdown` cancel ctx 并等待 in-flight 收敛（超时随 shutdown deadline）；`registerCloser`。
- **不新增 config 项**：tick/并发数为包内常量，后续需要再配置化（改 `config.yaml` 需另行确认）。

## 10. 前端（`ui/`）

### 10.0 ASCII UI 样式图

#### a. AgentManager 页（两栏布局，左栏 agent 卡片新增「定时」入口 + 定时状态 Badge）

```text
┌────────────────────────────────────────────────────────────────────────────────────┐
│ ◆ SimpleBase          [使用文档] [项目: demo ▾] [设置] [刷新]                        │
├──────────┬─────────────────────────────────────────────────────────────────────────┤
│ 监控大盘  │  Cloud Agent                                                            │
│ 数据库管理│  按模块的只读 Agent；composer 输入 @ 点名                                 │
│ S3 对象   │                                                                         │
│ Cloud Agt │ ┌─ Agents ──────────────────────────┐ ┌─ @Database ───────────────────┐  │
│ 日志管理  │ │                    [↻ 刷新] [＋新建]│ │ database                      │  │
│ 设置      │ │                                    │ │                               │  │
│          │ │ ╔════════════════════════════════╗ │ │        (消息区)                │  │
│          │ │ ║ Database           [database]  ║ │ │                               │  │
│          │ │ ║ Inspect project databases and  ║ │ │  ┌──────────────────────┐      │  │
│          │ │ ║ run readonly SQL               ║ │ │  │[scheduled] 检查订单表│ ◤   │  │
│          │ │ ║ ────────────────────────────── ║ │ │  └──────────────────────┘      │  │
│          │ │ ║ [编辑] [⏱ 定时] [删除]          ║ │ │              ╭───────────────╮│  │
│          │ │ ║ ⏱ 每天 08:00 (UTC) ● 已启用    ║ │ │        ⬤ Bot│共 3 张表，今日  ││  │
│          │ │ ╚════════════════════════════════╝ │ │              │写入 1.2k 行... ││  │
│          │ │ ┌────────────────────────────────┐ │ │              ╰───────────────╯│  │
│          │ │ │ S3                   [s3]      │ │ │                               │  │
│          │ │ │ List and inspect project ...   │ │ │                               │  │
│          │ │ │ ────────────────────────────── │ │ │                               │  │
│          │ │ │ [编辑] [⏱ 定时] [删除]          │ │ │                               │  │
│          │ │ │                                │ │ │                               │  │
│          │ │ └────────────────────────────────┘ │ │                               │  │
│          │ │ ┌────────────────────────────────┐ │ ├───────────────────────────────┤  │
│          │ │ │ Logs                 [logs]    │ │ │ @Database 询问… ⏎          ➤  │  │
│          │ │ │ [编辑] [⏱ 定时] [删除]          │ │ │          点名 @Database 后发送；│  │
│          │ │ │                                │ │ │          工具只读             │  │
│          │ │ └────────────────────────────────┘ │ └───────────────────────────────┘  │
│          │ └────────────────────────────────────┘                                   │
└──────────┴─────────────────────────────────────────────────────────────────────────┘
```

- agent 卡片操作区在「编辑」「删除」之间插入「⏱ 定时」按钮；
- 已配置 schedule 时卡片底部追加一行 `⏱ <频率摘要> ● 已启用`（停用为 `○ 已停用`，频率摘要取 cron 预设名或原始表达式）；
- 卡片处于选中态（高亮描边）时同样可见。

#### b. AgentScheduleModal（核心弹窗：提示词框 + 频率 + 启用 + 历史）

```text
                                        ┌─ ⏱ 定时执行：Database ─────────────────────────────────────┐
                                        │                                                  │
                                        │  提示词 *                                        │
                                        │  ┌──────────────────────────────────────────────────┐│
                                        │  │ 检查订单表今日写入量，异常时在回复中标注。          ││
                                        │  │ （作为定时触发的用户消息发送给该 Agent）            ││
                                        │  │                                                  ││
                                        │  └──────────────────────────────────────────────────┘│
                                        │                                                  │
                                        │  执行频率 (UTC)                          启用      │
                                        │  ┌───────────────────────────┐      ╭───○──────●─╮   │
                                        │  │ 每天 08:00 (UTC)        ▾ │      ╰──────────────╯   │
                                        │  └───────────────────────────┘                           │
                                        │        ▲                                               │
                                        │        ┆ 下拉展开：                                       │
                                        │        ╔═╗ ┌─────────────────────────────────────────┐  │
                                        │        ║▸║ │ 每 15 分钟          */15 * * * *        │  │
                                        │        ║ │ │ 每小时              0 * * * *           │  │
                                        │        ║ │ │ 每天 08:00 (UTC)   0 8 * * *     ✓      │  │
                                        │        ║ │ │ 每周一 08:00 (UTC) 0 8 * * 1            │  │
                                        │        ║ │ │ 自定义 Cron…        手动输入表达式      │  │
                                        │        ╚═╝ └─────────────────────────────────────────┘  │
                                        │  自定义选中时追加一行（带校验提示）：                       │
                                        │  ┌───────────────────────────┐                          │
                                        │  │ 0 8,20 * * 1-5            │  ✓ 下次 2026-09-18 20:00 │
                                        │  └───────────────────────────┘                          │
                                        │                                                  │
                                        │  下次执行 2026-09-19 08:00 UTC   上次执行 2026-09-18  │
                                        │  结果会话  Scheduled: Database                     │
                                        │                                                  │
                                        │  ── 执行历史 ────────────────────────────────────  │
                                        │  ● completed   09-18 08:00 → 08:01        [查看会话] │
                                        │  ● completed   09-17 08:00 → 08:00        [查看会话] │
                                        │  ● failed      09-16 08:00  llm quota exc… [查看会话] │
                                        │  ◌ running     09-16 08:00（手动触发）               │
                                        │  ○ 无 schedule 记录时显示 SbEmptyState「暂无执行记录」│
                                        │                                                  │
                                        │            [🗑 删除]  [▶ 立即执行]     [取消] [保存] │
                                        └──────────────────────────────────────────────────┘
```

- 新建态隐藏「下次执行 / 上次执行 / 结果会话 / 执行历史 / 删除」，仅提示词 + 频率 + 启用；
- 「立即执行」后弹窗不关闭，历史区插入一条 running 记录并轮询刷新（2s × 15 次，超时提示稍后在历史中查看）；
- 「查看会话」链接到该 schedule 专属 thread：切换右侧对话至该 thread（或新开 thread 抽屉，见 §10.2 交互补充）；
- cron `Input` 失焦即校验：非法表达式红框 + `✗ 表达式非法`，合法则显示 `✓ 下次 …`（复用前端轻量校验，最终以服务端 400 为准）。

#### c. 定时执行在专属 thread 内的消息形态（复用 AiChat 气泡）

```text
┌─ @Database · Scheduled: Database ──────────────────────────────┐
│                                                                │
│                     ┌────────────────────────────────┐ ◤       │
│                     │ ⏱ [scheduled] 检查订单表今日写入量 │        │
│                     │    异常时在回复中标注             │        │
│                     └────────────────────────────────┘        │
│      ╭──────────────────────────────────────────╮             │
│ ⬤ Bot│ 共 3 张表：orders / order_items / refunds。│            │
│      │ 今日 orders 写入 1,240 行，环比 +8%，正常。 │            │
│      ╰──────────────────────────────────────────╯             │
│           ┌ readonly_sql ──────────────────┐                 │
│           │ {"database_id":"…","sql":"…"}  │                 │
│           │ …rows…                         │                 │
│           └────────────────────────────────┘                 │
└────────────────────────────────────────────────────────────────┘
```

- `[scheduled]` 前缀消息 role=user，头像仍为 UserIcon；工具卡片复用现有 `toolCalls` 渲染；
- 该 thread 与手动对话 thread 并存于 `GET :p/agent-threads`，title 即 `Scheduled: <agent name>`，用户可在右侧顶部「会话」切换查看（本期不单独做 thread 列表 UI，依赖现有 ListThreads 返回顺序）。

### 10.1 services 层

- `types.ts`：新增 `AgentSchedule`、`AgentScheduleRun`、`AgentScheduleBody`；`Api` 增加 `agentSchedules` 域：

```ts
agentSchedules: {
  list: (projectId: string) => Promise<AgentSchedule[]>
  create: (projectId: string, body: AgentScheduleBody) => Promise<AgentSchedule>
  patch: (projectId: string, scheduleId: string, body: Partial<AgentScheduleBody>) => Promise<AgentSchedule>
  remove: (projectId: string, scheduleId: string) => Promise<void>
  runs: (projectId: string, scheduleId: string) => Promise<AgentScheduleRun[]>
  trigger: (projectId: string, scheduleId: string) => Promise<AgentScheduleRun>
}
```

- `http-api.ts` 实现（snake_case 映射，`toAgentSchedule` 风格）；`mock.js` 同步实现同一签名（含 `next_run_at` 模拟推进）。

### 10.2 页面与组件

- `pages/AgentManager.vue`：
  - agent 卡片操作区新增「定时」按钮（`ClockIcon` + `data-icon="inline-start"`）；
  - `scheduleByAgent: Record<agentId, AgentSchedule | null>`，`bootstrap()` 时并行 `agentSchedules.list` 回填；卡片上以小 Badge 显示启用状态与频率摘要（见样式图 a）；
  - 打开弹窗传 `agent` + 已有 schedule（有为编辑态、无为新建态）；
  - 「查看会话」回调：将右侧 `threadId` 切到 schedule.thread_id 并拉取消息（复用 `ensureThread` 的消息映射逻辑）。
- 新组件 `components/ai/AgentScheduleModal.vue`（`SbModal` 封装，`width: 640`）：
  - **提示词 Textarea（必填）**——本次需求核心；
  - 频率 `Select` 预设：`每 15 分钟`(`*/15 * * * *`)、`每小时`(`0 * * * *`)、`每天 08:00 (UTC)`(`0 8 * * *`)、`每周一 08:00 (UTC)`(`0 8 * * 1`)、`自定义 Cron`（选中时显示 cron `Input` + 即时校验，标注 UTC）；
  - `Switch` 启用开关；已有 schedule 时展示 `next_run_at` / `last_run_at` / 结果会话标题；
  - 执行历史：最近 5 次 `runs`，状态用 `Badge`（completed 绿点 / failed 红点 / running Spinner / canceled 灰点），每行带「查看会话」；
  - 底部操作（走 `SbModal` 的 `#footer` 插槽）：删除（`ConfirmAction` 包裹）、立即执行、取消、保存；
  - 保存：POST 新建，遇 409 `schedule already exists` 自动转 PATCH 目标 schedule 重试一次；PATCH 直接更新。
- 全部 toast 用 `vue-sonner`；不引入新组件库依赖，复用现有 `ui/` 基础件（`Select` / `Switch` / `Badge` / `Textarea` / `Input` / `Button`）。

## 11. 测试与验收

后端（`go build ./internal/... ./cmd/...`、`go test ./...` 必须通过）：

- `cron_test.go`：`*/15`、`0 8 * * *`、`0 8 * * 1,4`、月末/周语义、非法表达式（字段数、越界、坏字符）确定错误、`NextAfter` 上限。
- `systemdb`：schedule CRUD、`GetAgentScheduleByAgent` 唯一性、`ClaimAgentSchedule` 二次认领返回 false、`ListDueAgentSchedules` 过滤 enabled/时间。
- `scheduler_test.go`：fake Runner（成功/失败/配额拒绝）断言——run/message/schedule_run 三处落库、agent 归档自动 disable、manual trigger 不改 `next_run_at`。
- `agent_schedule_handler_test.go`：400/409/204 路径（fake service 风格对齐 `agent_handler_test.go`）。

前端（`yarn build` 必须通过）：mock 下创建 → 列表回显（卡片 Badge）→ 弹窗编辑 → 立即执行（历史出现 running 并轮询到 completed）→ 「查看会话」切到专属 thread。

验收清单：

1. Agent 卡片出现「定时」入口与状态 Badge（样式图 a）；弹窗布局符合样式图 b，提示词框为必填核心字段。
2. 配置 `*/1 * * * *`（或「立即执行」），专属 thread 内出现样式图 c 的 `[scheduled] <prompt>` 用户消息与 assistant 回复（含工具卡片），`sys_agent_schedule_runs` 记录 completed。
3. 停用（enabled=false）后 tick 不再触发；删除 agent 后 schedule 自动停用。
4. 定时执行日志出现在日志页（logger=scheduler），不含 prompt 原文与任何密钥。
