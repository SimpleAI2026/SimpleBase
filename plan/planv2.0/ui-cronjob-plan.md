# 定时任务（Cron Jobs）控制台 + 云函数定时调用计划

> 状态：**implemented（2026-09-19 落地；存储用 v29/v30 两版本迁移——v30 拆分运行记录表）**
> 日期：2026-09-19
> 范围：`ui/`（侧栏 + 列表 + 任务 Modal + 运行记录）· `internal/api`（CRUD + trigger）· `internal/cronjob`（新：调度器）· `internal/crontab`（新：cron 引擎公共化）· `internal/systemdb`（v29 两张表）
> 前置：`ui-gofunction-plan.md`（云函数已落地，`gofunction.RunJSON` 是执行内核）；`agent-schedule-plan.md`（调度范式来源）

---

## 0. 背景与目标

参考 Supabase Cron Jobs（pg_cron 包装）为项目提供**云函数定时调用**能力：

- 用户在控制台创建定时任务，绑定「某个云函数文件的某个导出函数」；
- 支持两种时间配置：**定时执行**（5 字段 cron 表达式）与**固定间隔**（每 N 分钟/小时/天）；
- 到点由服务端调度器以 `gofunction.RunJSON` 执行，入参为任务配置的固定 JSON（默认 `{}`）；
- 每次执行落运行记录（状态/耗时/错误/返回值截断），支持手动「立即执行」。

非目标（v1 不做）：

- 不做 SQL / HTTP webhook 类型任务（只调云函数）；
- 不做失败重试与连续失败自动禁用；
- 不做秒级 cron（沿用现有分钟粒度）；
- 不接配额计费（云函数执行耗资源，记入后续）。

## 1. 关键约束

1. 云函数签名契约不变：恰好 1 入参 1 返回值（见 ui-gofunction-plan §3.1）。任务入参是固定 JSON，执行时按 `RunJSON` 既有规则反序列化；类型不匹配在执行时记 failed，**保存任务时不做类型级 dry-run**（函数可能有副作用）。
2. 执行超时沿用解释器内部上限（10s），调度器不另设。
3. 调度器单进程语义：CAS 认领防重（复用 agent-schedules 已验证的模式），tick 30s，并发上限 4。
4. 系统项目（admin）只读：列表空、写操作 503。
5. 定时任务与 Cloud Agent 定时执行（agent-schedules）是**两套独立功能**，表、API、UI 均分离，仅共享 cron 解析引擎。
6. 所有时间 UTC；`next_run_at` 服务端计算，前端只展示不计算。

## 2. 现状调研（复用点）

| 已有 | 位置 | 复用方式 |
|---|---|---|
| cron 解析 `ParseCron` / `CronSpec.NextAfter` | `internal/cloudagent/cron.go` | 纯函数无依赖，抽到公共包 `internal/crontab`（§5.1） |
| 调度循环范式：tick / `ListDue*` / CAS `Claim*` / inFlight / `Trigger` | `internal/cloudagent/scheduler.go` | 镜像实现 `internal/cronjob/scheduler.go`，执行器换成 RunJSON |
| 执行内核 `RunJSON(ctx, seqid, name, source, funcName, body)` | `gofunction/jsonrun.go` | 直接调用；超时/编译/绑定错误已由哨兵区分 |
| 任务目标数据 `sys_gofunctions` | v28 | 保存任务时校验文件存在 + 导出函数在 exports 内 |
| metrics 通道 `RecordMetric` / audit 通道 `AuditService` | `internal/api` | 直接复用，新增 3 个指标与 kind=`cronjob` |
| UI 组件：Table*/SbModal/Select/RadioGroup/Switch/ConfirmAction | `ui/src/components` | 直接复用；侧栏/路由按 GoFunctions 同法接入 |

## 3. 术语与两种时间模式

| 模式 | `schedule_kind` | 用户输入 | next_run_at 计算 |
|---|---|---|---|
| 定时执行 | `cron` | 5 字段 cron（分 时 日 月 周，UTC） | `ParseCron(expr).NextAfter(now)` |
| 固定间隔 | `interval` | 数值 + 单位（分钟/小时/天）→ 秒数 | 认领时刻 + `interval_seconds`（容忍执行漂移，不以 last_run_at 累加） |

- cron 校验失败 → 400 `invalid_cron_expression`；interval 合法域 60s ~ 86400s*30（1 分钟 ~ 30 天），越界 → 400 `invalid_interval`。
- 切换模式时互斥字段清空（cron 模式 `interval_seconds=NULL`，反之亦然）。

## 4. 存储设计（v29）

### 4.1 `sys_cron_jobs`（任务表）

```sql
CREATE TABLE IF NOT EXISTS sys_cron_jobs (
    id VARCHAR NOT NULL PRIMARY KEY,
    project_id VARCHAR NOT NULL,
    name VARCHAR NOT NULL,              -- 任务名：字母开头，字母数字_-，1~63
    description VARCHAR NOT NULL DEFAULT '',
    schedule_kind VARCHAR NOT NULL,     -- cron / interval
    cron_expr VARCHAR,                  -- kind=cron 时非空
    interval_seconds BIGINT,            -- kind=interval 时非空
    func_file VARCHAR NOT NULL,         -- 云函数文件名（sys_gofunctions.name）
    func_export VARCHAR NOT NULL,       -- 导出函数名（大写开头）
    input_json VARCHAR NOT NULL DEFAULT '{}',  -- 固定入参（JSON 原文，≤8KB）
    enabled INTEGER NOT NULL DEFAULT 1,
    last_run_at TIMESTAMP,
    next_run_at TIMESTAMP,
    last_status VARCHAR NOT NULL DEFAULT '',   -- 最近一次 run 状态快照（列表页直显）
    last_error VARCHAR NOT NULL DEFAULT '',    -- 截断 300 字符
    run_count BIGINT NOT NULL DEFAULT 0,
    created_by VARCHAR NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    archived_at TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sys_cron_jobs_name ON sys_cron_jobs(project_id, name);
CREATE INDEX IF NOT EXISTS idx_sys_cron_jobs_due ON sys_cron_jobs(enabled, next_run_at);
```

软删沿用 `archived_at`；唯一索引与 `sys_gofunctions` 同款（软删后同名可复建）。

### 4.2 `sys_cron_job_runs`（运行记录表）

```sql
CREATE TABLE IF NOT EXISTS sys_cron_job_runs (
    id VARCHAR NOT NULL PRIMARY KEY,
    job_id VARCHAR NOT NULL,
    project_id VARCHAR NOT NULL,
    trigger VARCHAR NOT NULL,           -- scheduled / manual
    status VARCHAR NOT NULL,            -- running / completed / failed / canceled
    error VARCHAR NOT NULL DEFAULT '',  -- 截断 300
    duration_ms BIGINT NOT NULL DEFAULT 0,
    response_json VARCHAR,              -- 返回值 JSON 截断 4KB；NULL=无输出
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sys_cron_job_runs_job ON sys_cron_job_runs(project_id, job_id, created_at);
```

### 4.3 Store 方法（`internal/systemdb/cron_jobs.go`）

镜像 `agent_schedules.go`：

```
CreateCronJob / GetCronJob(by id) / GetCronJobByName / ListCronJobs(projectID)
UpdateCronJob（可变字段：description/schedule/目标/input/enabled/next_run_at/快照）
ArchiveCronJob（软删；同步清 next_run_at）
ListDueCronJobs(ctx, now, limit)      -- enabled=1 AND next_run_at<=now ORDER BY next_run_at
ClaimCronJob(ctx, id, expectNext, newNext, lastRun) (bool, error)  -- CAS 同 agent 版
InsertCronJobRun / UpdateCronJobRunStatus（status/error/duration/response/finished）
ListCronJobRuns(ctx, projectID, jobID, limit)
```

DTO 结构体 `CronJob` / `CronJobRun` 字段与表一一对应；`next_run_at` 用 `sql.NullTime` 扫描（同 agent 版）。

---

## 5. 内核与调度器设计

### 5.1 cron 引擎公共化（`internal/crontab`）

- 将 `internal/cloudagent/cron.go` **移动**为 `internal/crontab/crontab.go`（`ParseCron`/`CronSpec.NextAfter` 原样，纯函数零依赖，附带全部测试迁移）。
- `internal/cloudagent/cron.go` 改薄封装保持兼容：

  ```
  type CronSpec = crontab.CronSpec          // type alias
  func ParseCron(expr string) (CronSpec, error) { return crontab.ParseCron(expr) }
  ```

  → cloudagent 既有调用点与测试零改动；cronjob 直接 import `internal/crontab`，语义干净。
- 语法不变：受限 5 字段（分 时 日 月 周），支持 `*`、`*/n`、`n`、`a-b`、逗号列表；UTC；分钟粒度。

### 5.2 调度器（`internal/cronjob/scheduler.go`）

镜像 `cloudagent.Scheduler` 的骨架与并发语义：

```
type Runner interface {
    // RunFunction 执行一次云函数调用；返回原始 JSON 输出。
    RunFunction(ctx context.Context, projectID, funcFile, funcExport string, input json.RawMessage) ([]byte, error)
}

type Store interface {
    GetCronJob / UpdateCronJob / ListDueCronJobs / ClaimCronJob
    InsertCronJobRun / UpdateCronJobRunStatus
    RecordLog(ev systemdb.LogEvent)
}

type Scheduler struct { Store; Runner; tick=30s; inFlight map; 并发上限 4 }
Start(ctx) / Stop() / tickOnce() / runDue() / Trigger(ctx, job) / execute()
```

执行流程（`execute`）：

1. `InsertCronJobRun` 落一条 `running` 记录；
2. 调 `Runner.RunFunction`（实现见 §5.3）；
3. 成功 → `UpdateCronJobRunStatus(completed, duration, response_json 截断 4KB)`；
   失败 → `failed` + 截断 300 的 error（`ErrTimeout`/`ErrCompile`/`ErrBind` 哨兵 → 明确文案）；
4. 回写任务快照：`last_run_at`、`last_status`、`last_error`、`run_count+1`（UpdateCronJob 一并完成）；
5. metrics 三指标（§9）。

排期推进（`runDue`）：

- `cron`：`spec.NextAfter(now)`；
- `interval`：`now + interval_seconds`；
- `ClaimCronJob(id, expectNext=job.NextRunAt, newNext, lastRun=now)` 失败即放弃（已被认领/修改）；
- cron 表达式被改坏（历史脏数据）：记日志并 `enabled=false`，同 agent 版。

手动触发（`Trigger`）：同 agent 版——认领 inFlight 后异步执行，**不动 next_run_at**，返回 202。

### 5.3 Runner 实现（`internal/api` 层注入）

```
func(ctx, projectID, file, export, input):
    g, err := store.GetGoFunction(ctx, projectID, file)   // 404 → 明确错误「gofunction not found」
    out, err := gofunction.RunJSON(ctx, rid("cron"), file, g.Source, export, input)
    // RunJSON 内部已处理：函数未导出 → function_not_found；编译失败 → ErrCompile；绑定失败 → ErrBind；超时 → ErrTimeout
```

注意分层：`cronjob` 包不 import `gofunction`（Runner 接口注入，保持内核可测）；实现放 `internal/api`（或 app 装配处），因为需要同时访问 store 与内核。

### 5.4 生命周期接线（`internal/app/app.go`）

- `Dependencies` 增加 `CronScheduler *cronjob.Scheduler`；
- 启动：`systemStore != nil` 时构造并 `Start`（与 agentScheduler 同一段），复用 `schedulerBaseCtx`；
- 停止：与 agentScheduler 同一超时窗口内 `Stop()`；
- router.go：`mountV1Routes` 守卫内挂载管理面（同 gofunctions），handler 持有 `deps.CronScheduler` 用于 trigger。

## 6. HTTP API 设计

挂在 `/v1/projects/:projectID/cron-jobs`，权限与 gofunctions 一致（读 `DatabaseRead`，写 `DatabaseWrite`），admin 项目只读空列表。

| 方法 | 路径 | 说明 | 响应 |
|---|---|---|---|
| GET | `/cron-jobs` | 列出（新→旧） | `{ jobs: [CronJob] }` |
| POST | `/cron-jobs` | 创建 | 201 `CronJob`（含首次 next_run_at） |
| GET | `/cron-jobs/:jobID` | 详情 | `CronJob` |
| PATCH | `/cron-jobs/:jobID` | 更新（name 不可改；可改 description/schedule/目标/input/enabled） | `CronJob`（重算 next_run_at） |
| DELETE | `/cron-jobs/:jobID` | 软删 | 204 |
| GET | `/cron-jobs/:jobID/runs?limit=` | 运行记录（新→旧，默认 20 上限 100） | `{ runs: [CronJobRun] }` |
| POST | `/cron-jobs/:jobID/trigger` | 手动立即执行（异步，不改排期） | 202 `{ run_id }` |

### 6.1 DTO（snake_case，与 agentScheduleDTO 同款风格）

```json
// CronJob
{
  "id": "...", "name": "nightly-refresh", "description": "每天凌晨汇总",
  "schedule_kind": "cron", "cron_expr": "0 2 * * *", "interval_seconds": null,
  "func_file": "hello", "func_export": "Hello",
  "input_json": "{\"name\":\"cron\"}",
  "enabled": true,
  "last_run_at": "...", "next_run_at": "...",
  "last_status": "completed", "last_error": "", "run_count": 12,
  "target_missing": false,
  "created_at": "...", "updated_at": "..."
}
// CronJobRun
{ "id": "...", "trigger": "scheduled", "status": "failed",
  "error": "execution timeout", "duration_ms": 10021,
  "response_json": null, "started_at": "...", "finished_at": "...", "created_at": "..." }
```

`target_missing`：列表/详情联查 `sys_gofunctions` + `ValidateHTTPFuncs` 判定目标函数当前是否可用（文件被删/改名/函数被去导出 → true，前端红标提示；**不阻断**既有排期，执行时记 failed）。

### 6.2 请求体校验

```json
// POST / PATCH
{ "name": "nightly-refresh", "description": "...",
  "schedule_kind": "cron" | "interval",
  "cron_expr": "0 2 * * *",            // kind=cron 必填
  "interval_seconds": 3600,            // kind=interval 必填，60 ~ 2592000
  "func_file": "hello", "func_export": "Hello",
  "input_json": "{ ... }",             // 可省，默认 "{}"；仅做 JSON 语法校验，≤8KB
  "enabled": true }
```

服务端校验顺序：name 规则 → schedule 字段（cron 解析 / interval 域）→ 目标存在（`GetGoFunction` + export ∈ exports）→ input_json 合法性 → 写库（`next_run_at` 立即计算落库）。

### 6.3 错误码

| 状态 | code | 场景 |
|---|---|---|
| 400 | `invalid_cron_job_name` | 任务名不符规则 |
| 400 | `invalid_cron_expression` | cron 解析失败 |
| 400 | `invalid_interval` | interval 越界/缺失 |
| 400 | `invalid_request` | body 无法解析 / input_json 非合法 JSON |
| 400 | `gofunction_not_found` | 目标云函数文件不存在 |
| 400 | `function_not_exported` | 目标函数不在 exports |
| 404 | `cron_job_not_found` | 任务不存在 |
| 409 | `cron_job_exists` | 同名冲突 |
| 409 | `cron_job_running` | trigger 时任务正在执行中 |
| 503 | `system_database_unavailable` / `readonly_mode` | 同现有 |

## 7. 前端方案

### 7.1 侧栏与路由

- `ui/src/router/index.ts`：`children` 加 `{ path: 'cron-jobs', name: 'cron-jobs', component: () => import('../pages/CronJobs.vue'), meta: { title: '定时任务' } }`（懒加载，独立 chunk）。
- `ui/src/components/NavMenu.vue`：`iconMap['cron-jobs'] = TimerIcon`（lucide `Timer`）；`routeOrder` 插入 `'gofunctions', 'cron-jobs', 'agents'` 之间。

```
┌──────────────┬────────────────────────────────────────────────────┐
│ 仪表盘        │  定时任务                          [新建定时任务]   │
│ 数据库        │  项目内的定时调用配置；目标为云函数导出函数          │
│ 存储          │ ┌────────────────────────────────────────────────┐ │
│ 云函数        │ │ 名称      │调度       │目标      │启用│下次执行│ │
│ 定时任务 ◄──  │ │ nightly-  │cron       │hello.    │ ●  │02:00  │ │
│ 云 Agent      │ │ refresh   │0 2 * * *  │Hello     │    │UTC    │ │
│ 日志          │ │ ──────────┼───────────┼──────────┼────┼───────┤ │
│ 设置          │ │ cache-    │每 10 分钟  │cache.    │ ○  │已暂停 │ │
│              │ │ warm      │           │Warm      │    │       │ │
│              │ └────────────────────────────────────────────────┘ │
└──────────────┴────────────────────────────────────────────────────┘
```

### 7.2 列表页（`ui/src/pages/CronJobs.vue`，镜像 GoFunctions.vue）

```
┌─ 定时任务 ───────────────────────────────────────────[新建定时任务]┐
│ 共 N 个任务 · 调度按 UTC 执行                                     │
├──────────────────────────────────────────────────────────────────┤
│ 名称        │ 调度         │ 目标函数      │ 状态     │ 启用 │ 操作 │
│─────────────┼──────────────┼──────────────┼──────────┼─────┼─────│
│ nightly-    │ 0 2 * * *    │ hello.Hello  │ ● 成功   │ [■] │ ▶ 运行│
│ refresh     │ 定时执行      │              │ 2h 前    │     │ ✎ 编辑│
│             │              │              │          │     │ ⏱ 记录│
│             │              │              │          │     │ 🗑 删除│
│─────────────┼──────────────┼──────────────┼──────────┼─────┼─────│
│ cache-warm  │ 每 10 分钟    │ cache.Warm   │ ✕ 失败   │ [ ] │ …    │
│             │ 固定间隔      │              │ 10m 前   │     │      │
└──────────────────────────────────────────────────────────────────┘
  空态：「还没有定时任务」/ 描述「新建定时任务，到点自动调用云函数」/ CTA 新建
  admin 项目：「系统项目不支持定时任务」，无 CTA
```

列说明：

- **名称**：`name` + tooltip 描述；
- **调度**：cron → 等宽显示表达式 + 副行「定时执行」；interval → 「每 N 分钟/小时/天」人类化 + 副行「固定间隔」；
- **目标函数**：`file.Export`，`target_missing=true` 时红色 + ⚠ tooltip「目标函数缺失，执行将失败」；
- **状态**：`last_status` Badge（completed 绿 / failed 红 / running 蓝 / 空灰「未运行」）+ 副行 `last_run_at` 相对时间；`last_error` 非空时 tooltip 展示；
- **启用**：Switch 即时切换（PATCH enabled），切换后 `next_run_at` 重算回显；
- **下次执行**（列可并入状态副行，视宽度）：`next_run_at` 本地时间 + UTC tooltip；
- **操作**：▶ 立即执行（trigger，toast「已触发」并刷新记录）、✎ 编辑、⏱ 运行记录、🗑 删除（ConfirmAction：「确认删除 name？历史运行记录保留但不再排期。」）。

### 7.3 新建/编辑 Modal（`ui/src/components/modal/CronJobModal.vue`）

```
┌─ 新建定时任务 ────────────────────────────────────────────────┐
│ 任务名 *   [ nightly-refresh_________________ ]               │
│            字母开头，可含字母数字_-（创建后不可修改）          │
│ 描述       [ 每天凌晨汇总昨日数据_______________ ]             │
│                                                              │
│ 时间执行 *  (●) 定时执行   ( ) 固定间隔                        │
│ ┌─ kind=cron ─────────────────────────────────────────────┐ │
│ │ cron 表达式 * [ 0 2 * * * _______ ] [常用预设 ▾]          │ │
│ │   预设：每小时(0 * * * *) / 每天 02:00 / 每周一 09:00 …    │ │
│ │   分 时 日 月 周，UTC；示例 0 2 * * * = 每天 02:00 UTC     │ │
│ └──────────────────────────────────────────────────────────┘ │
│ ┌─ kind=interval ─────────────────────────────────────────┐ │
│ │ 每 [ 10__ ] [ 分钟 ▾ ] 执行一次                          │ │
│ │   单位：分钟 / 小时 / 天；范围 1 分钟 ~ 30 天             │ │
│ └──────────────────────────────────────────────────────────┘ │
│                                                              │
│ 云函数 *    [ 选择文件 ▾  hello.go ]                         │
│ 导出函数 *  [ 选择函数 ▾  Hello      ]  ← 随文件联动          │
│ 入参 JSON   [ { "name": "cron" }____________ ]  可选，默认 {} │
│             将在每次执行时作为请求 body 传入                   │
│                                                              │
│ ⓘ 目标函数须为云函数中已导出的大写函数；修改云函数后此处       │
│   自动按最新代码执行。任务到点执行超时上限 10 秒。             │
│                                          [ 取消 ] [ 保存 ]   │
└──────────────────────────────────────────────────────────────┘
```

交互细节：

- **文件 Select**：`api.gofunctions.list(projectId)`；空列表时禁用并提示「先创建云函数」+ 跳转链接；
- **导出函数 Select**：选中文件的 `exports` 数组；联动重置（换文件清空函数）；
- **编辑模式**：name 禁用；其余可改；保存即 PATCH；
- **校验**：前端做必填/名称正则/cron 粗校验（5 字段）；权威校验在服务端，400 时 Alert 红条展示 message；
- **入参**：Textarea 3 行，失焦 JSON 格式化校验（非法即红框），保存原样上传字符串；
- 保存成功 toast「任务已创建，下次执行 {next_run_at 本地时间}」。

### 7.4 运行记录（Drawer，右侧滑出，宽 480）

```
┌─ 运行记录 · nightly-refresh ──────────────────────── [×] ┐
│ [▶ 立即执行]                  状态筛选 [全部 ▾]           │
│ ┌──────────────────────────────────────────────────────┐ │
│ │ ● completed  scheduled  1.2s   2026-09-19 02:00:03   │ │
│ │   返回 {"message":"hello, cron"}              [复制]   │ │
│ ├──────────────────────────────────────────────────────┤ │
│ │ ✕ failed     scheduled  10.0s  2026-09-18 02:00:00   │ │
│ │   execution timeout                          [复制]    │ │
│ ├──────────────────────────────────────────────────────┤ │
│ │ ● completed  manual     0.8s   2026-09-17 15:22:10   │ │
│ └──────────────────────────────────────────────────────┘ │
│ 加载更多…（每页 20）                                      │
└──────────────────────────────────────────────────────────┘
```

- 数据：`GET /cron-jobs/:id/runs?limit=`；打开即加载，trigger 后 1.5s 轮询一次直至 running 记录消失（复用 agent 运行记录轮询思路）；
- `response_json`/`error` 截断展示 + 复制按钮；
- 筛选：全部 / 成功 / 失败（前端过滤）。

### 7.5 服务层与 mock

- `types.ts`：`CronJobItem` / `CronJobRunItem`（camelCase）；
- `http-api.ts`：`cronjobs.list/get/create/update/remove/runs/trigger`，snake↔camel 适配；
- `mock.js`：内存数组；种子 1 条（绑定 hello/Hello，cron `*/10 * * * *`）；trigger 直接同步执行 mock 云函数逻辑——简化为立即返回一条 completed run（不真调函数）；next_run_at 前端按 interval 简单推算、 cron 固定 +1h（mock 不追求精确）。

## 8. 安全 / 隔离 / 权限

| 项 | 决策 |
|---|---|
| 项目隔离 | 全部 SQL 带 `project_id`；调度器执行路径按 job.ProjectID 取云函数源码，无跨项目读取 |
| 管理面权限 | 读 `DatabaseRead`，写 `DatabaseWrite`（同 gofunctions） |
| 执行身份 | 调度器内部直接调 `RunJSON`，不经 HTTP/auth；云函数本身无鉴权概念（内核纯函数） |
| admin 项目 | 只读空列表；写 503；调度器对 admin 项目任务天然无数据可查 |
| 审计 | create/update/delete/trigger 记 `kind=cronjob`，detail 只含动作与任务名（不含 input_json 原文） |
| 错误脱敏 | run.error 截断 300；response_json 截断 4KB；源码/入参原文不进日志 |
| writable=false | 写操作 503 `readonly_mode`（调度器内部写库不受限，同 agent 版） |

## 9. Metrics 与可观测

| 指标 | 时机 | labels |
|---|---|---|
| `cronjob_runs` | 每次执行（含失败与手动） | `job=name` |
| `cronjob_run_errors` | 执行 failed | `job=name` |
| `cronjob_run_duration_ms` | 每次执行耗时 | `job=name` |

走 `deps.System.RecordMetric`（5 分钟批量落库）。调度器异常（cron 解析失败禁用、claim 冲突）写 `RecordLog`（logger=`cronjob`）。

## 10. 测试矩阵

- `internal/crontab`：既有 cron 测试整体迁移（解析/NextAfter/边界），零改动通过；
- `internal/cronjob/scheduler_test.go`：fake Store + fake Runner——到期扫描、CAS 认领失败跳过、completed/failed 落库、interval 推进、cron 表达式损坏自动禁用、Trigger 异步执行、并发上限；
- `internal/systemdb`：v29 迁移幂等、CRUD、软删唯一索引复建、ListDueCronJobs 窗口、ClaimCronJob CAS；
- `internal/api/cronjob_handler_test.go`：CRUD 全套、校验错误码（§6.3 全表）、admin 只读、trigger 409 冲突、runs 分页、target_missing 标记、metrics 三指标落库、audit kind；
- 前端：`yarn build` 通过；手测建任务 → 等 1 分钟级 cron 触发 → 运行记录出现。

## 11. 契约同步

- `plan/planv2.0/proto.http`：`### 云函数定时任务` 段（list/create/patch/delete/runs/trigger 样例）；
- `plan/planv2.0/proto-http.md`：新增 §3.14 定时任务表（含错误码）；
- `docs/`：`docs/cronjob/overview.md`（概念 + 两种模式 + 限制）、`docs/cronjob/usage.md`（控制台操作 + 排坑），`_meta.json` 注册 `{ "id": "cronjob", "title": "定时任务" }`。

## 12. 风险与坑（评审清单）

1. **cron 包迁移的连锁改动**——cloudagent 内所有 `ParseCron` 引用点与测试需随 alias 方案验证；若嫌动面广，备选方案是 cronjob 直接 `import cloudagent` 复用（语义差但零迁移成本）。**建议按 alias 方案做**，一次性理清。
2. **interval 漂移**：以「认领时刻 + interval」推进而非 `last_run_at + interval`，长任务不会错位压缩；文档注明。
3. **目标变更**：云函数改名/删除后任务不自动清理；`target_missing` 只是提示，执行照常记 failed——文档写清「任务不随云函数删除而删除」。
4. **input_json 与函数签名不匹配**：保存时不 dry-run（副作用），运行时 ErrBind → failed；UI Alert 提示用户自行保证。
5. **next_run_at 时区**：一律 UTC 存储与计算；前端展示本地化；cron 表达式语义即 UTC（与 Supabase pg_cron 一致），文档与 Modal 提示均注明。
6. **调度器重启追赶**：重启后 `next_run_at <= now` 的任务会立即补跑一次（at-least-once），不追赶历史漏跑——与 agent 版一致，文档注明。
7. **response_json 体积**：截断 4KB 防大返回撑爆系统库；超长提示用户自行落库。
8. **与 agent-schedules 的 UI 区分**：侧栏两项相邻，命名「定时任务」（云函数）vs 云 Agent 页内「定时执行」tab——文档与空态文案互相指引，避免混淆。

## 13. 开发步骤

- 阶段 A（存储）：v29 迁移 + `cron_jobs.go` + 测试；
- 阶段 B（内核）：`internal/crontab` 迁移 + alias 兼容 + `internal/cronjob` 调度器 + 测试；
- 阶段 C（API）：handler + 路由 + Runner 注入 + app 接线 + 测试；
- 阶段 D（前端）：types/http-api/mock + 路由侧栏 + CronJobs.vue + CronJobModal.vue + 运行记录 Drawer；
- 阶段 E（契约与文档）：proto.http / proto-http.md §3.14 / docs/cronjob/*；
- 每阶段完成后跑 `go test ./...` 与 `yarn build` 验证。

## 14. 验证步骤

1. `go build ./... && go vet ./...` 干净；
2. `go test ./internal/... ./gofunction/...` 全绿（含新增调度器与 handler 测试）；
3. `cd ui && yarn build` 通过，`CronJobs` 路由独立 chunk；
4. 启动后端 + 前端 dev：建一个 `* * * * *`（每分钟）任务绑 `hello/Hello`，等 1~2 分钟确认运行记录出现 completed；手动 trigger 一次确认 manual 记录；停用后确认不再产生新记录；删除云函数文件确认 `target_missing` 红标。
