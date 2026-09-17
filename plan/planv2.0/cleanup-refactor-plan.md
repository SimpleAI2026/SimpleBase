# SimpleBase 代码清理与重构计划（Cleanup & Refactor Plan）

> **日期**：2026-09-17（v2 修订版，基于对 v1 版本的复核）
> **范围**：`internal/`（约 22,000+ 行 Go）+ `ui/src`（约 5,800+ 行 Vue/TS/JS）
> **依据**：`plan/planv2.0/` 全部规划（ui-plan-v2 / databases-and-s3 / system-database / cloud-agent / proto-http）
> **v2 修订说明**：v1 版本完成分析时，代码库刚合入 Cloud Agent（`internal/cloudagent`、`AgentManager.vue`）功能，部分结论已被后续代码演进推翻（如 `SbCodeBlock` 从"零引用"变为"已被复用"、`LlmManager.vue` 从"待补功能"变为"孤儿页面"）。本版本已重新核对全部条目并修正。
> **结论摘要**：DuckLake-only 与系统库 Phase A/B 已落地，Cloud Agent Phase 0-2 已落地；v1.0 时代的 Turso 双轨、jobs 队列、多引擎装配已整体失效但仍留在主构建中；LLM 对话页已被 Cloud Agent 取代但旧页面未清理；前端 FaaS/WS Logs/metrics 旧路径与后端能力错位。

---

## 0. 一句话目标

**删除全部已退役/无调用的代码路径，让 `internal/` 与 `ui/src` 的每一行代码都对应 planv2.0 中「已立项且要保留」的能力**；预期后端净删约 2,000–2,500 行、前端净删约 700–900 行。

---

## 1. 判定标准

| 标签 | 含义 | 处置 |
|---|---|---|
| **A 死代码** | 全仓 `rg` 无任何生产调用方（含测试） | 直接删除 |
| **B 退役计划已拍板** | planv2.0 明确写「退役/丢弃/不做」，代码仍在 | 按 plan 删除 |
| **C 占位/双轨** | 与保留路径功能重复或恒返回占位值 | 重构收敛为单实现 |
| **D 前端无后端** | 前端调用后端不存在的路由（后端未立项） | 删除或降级提示 |
| **E 被新功能取代** | 旧实现已被 Cloud Agent 等新功能替代但未清理 | 删除旧实现，保留被复用的共享部件 |

---

## 2. 后端清理清单（internal/）

### 2.1 整包/整文件删除（A / B 类）

| # | 路径 | 行数 | 标签 | 依据 |
|---|---|---|---|---|
| B1 | `internal/jobs/create_backup.go` | 202 | B | databases-and-s3-plan §4.2「备份恢复不做（501）」；`Snapshotter` 无任何实现方；`BackupHandler`/`RestoreHandler`/`VerifyRecoveryHandler` 从未被注册到 Worker |
| B2 | `internal/jobs/queue.go` + `queue_test.go` | 278+248 | B | `app.go:62-63` 声明 `jobWorker/jobEnqueuer` 但**从未赋值**（rg 验证无 `NewWorker`/`NewEnqueuer` 生产调用）；软删已改为同步清理 → **`internal/jobs/` 整包删除** |
| B3 | `internal/deploy/preflight.go` | 130 | A | 全仓无 import（rg `internal/deploy` 零命中）；缓存目录校验已由 `cache.NewManager` 承担，S3 检查已由 health `Ready` 承担 → **整包删除** |
| B4 | `internal/catalog/alias.go` | 7 | A | `NewSQLiteRepository` 别名零调用方 |
| B5 | `internal/log/`（`config.go`+`log_tree.go`） | 40+80 | A | `SetDefaultLogMode`/`NewTee`/`NewTeeWithRotate`/`ResetDefault` 零外部调用；observability 只用 `log.New`。**保留 `log.go`（Logger 本体）与 `log_test.go`**，删 `config.go`、`log_tree.go` → 顺带从 go.mod 移除 `natefinch/lumberjack`（唯一使用方是 log_tree.go） |
| B6 | `internal/prom/prometheus.go` + test | 100+75 | A | 仅被 `observability/logger.go` 注释提及，无实际 import；指标已由 `observability/metrics.go` 全覆盖 → **整包删除** |
| B7 | `internal/utils/`（const/environ/fs/roundtrip/secretkey/util/sum，共 9 文件） | ~700 | A | **全仓零 import**（rg `internal/utils` 零命中） → 整包删除 |

**小计：约 1,800 行直接删除**

### 2.2 文件内死代码行级删除（A/B 类）

| # | 文件 | 位置 | 内容 | 标签 | 说明 |
|---|---|---|---|---|---|
| L1 | `internal/app/app.go` | 62-63、191-192（`JobEnqueuer` 字段行号随当前文件为准）、369-375、390-396 | `jobWorker`/`jobEnqueuer` 字段、`JobEnqueuer: api.NewJobEnqueuer(...)`、`Start`/`Shutdown` 中 worker 分支 | B | 随 B2 删除；`import jobs` 一并去掉 |
| L2 | `internal/api/router.go` | `Dependencies.JobEnqueuer` 字段 + `JobEnqueuer`/`JobInput` 接口定义（约 43、136-147 行） | 同上 | B | 零消费（handler 从不读它）；**注意**：`router.go` 现已 import `internal/cloudagent`，删除时不要误删 CloudAgent 相关行 |
| L3 | `internal/api/adapters_plan79.go` | `jobEnqueuerAdapter` + `NewJobEnqueuer`（约 153-172 行） | 同上 | B | 唯一调用方是 app.go（将删） |
| L4 | `internal/catalog/model.go` | `JobType` 四枚举、`JobStatus`、`Job` struct | B | jobs 删除后仅 `sql_repository.go` 的表操作还引用——见 L5 |
| L5 | `internal/catalog/repository.go` + `sql_repository.go` + **`job_queue_test.go`**（139 行，v1 遗漏） | 接口方法 `Enqueue/Claim/Complete/Retry/MarkDeadLetter/GetJob/ListJobs` + `sys_jobs` 表读写实现 + **对应测试文件整删** | B | 生产零调用；`sys_jobs` 表 DDL（migrate.go）**保留**（兼容已部署实例的迁移历史，加注释 deprecated）；**v1 遗漏点**：`internal/catalog/job_queue_test.go` 整个文件依赖被删的 Job 方法，必须同步整删，否则编译失败 |
| L6 | `internal/catalog/service.go` | `GetDatabaseForJob` | B | 仅 `jobs/create_backup.go` 调用，随 B1 删除；`SetDatabaseDegraded` **保留**（registry.go degraded 状态机仍在用，不要误删） |
| L7 | `internal/objectstore/keys.go` | `StateKey()`、`BackupPrefix()` | A | 零调用方（仅 keys_test.go 引用）→ 连同 keys_test.go 对应用例删除 |
| L8 | `internal/objectstore/descriptor.go` | `State` struct + `Validate` | A | `state.json` 机制无读写方 |
| L9 | `internal/api/database_handler.go` + `router.go` | `CreateBackup`/`RestoreDatabase` 501 handler + 路由挂载（`/databases/:id/backups`、`/restore`） | C→B | 可选项，见 §5 R2 |
| L10（**v2 新增**） | `internal/config/config.go` | `CatalogConfig` struct（字段 `DatabaseID`）+ `Validate()` 中 `catalog.database_id is required` 校验 + yaml/env 解析分支 | A | **v1 遗漏**：全仓 rg 确认 `cfg.Catalog.DatabaseID` 除 config.go 自身的加载/校验/脱敏打印外，**无任何消费方**（不驱动任何实际连接，系统库走的是 `SystemDatabase.Name`）。这是 Turso 时代「catalog 是独立 libsql 库」叙事的残留字段。**处置建议**：删除该结构体与校验，同步删 `config.example.yaml:63`、`.env:43`（`SIMPLEBASE_CATALOG_DATABASE_ID`）、`config_test.go:20` 的对应设置——**属 breaking change，需确认现网是否有依赖此环境变量的部署脚本** |
| L11（**v2 新增**） | `internal/observability/metrics.go` | `JobsTotal *prometheus.CounterVec`、`JobDurationSeconds *prometheus.HistogramVec`、`IncJobTotal()`、`ObserveJobDuration()` | B | **v1 只写"rg 确认后删"，现已确认具体 4 个符号**；随 jobs 删除同步移除，注意 `simplebase_jobs_total`/`simplebase_job_duration_seconds` 两个指标名从 Grafana/告警规则中一并摘除（若有） |
| — | `internal/database/hooks.go`（`WriteHookFactory`） | — | **保留** | ducklake Factory 实现它、registry 消费——活代码，不删 |
| — | `internal/llmgateway/catalog_resolver.go`、`streaming.go` | — | **保留** | app.go:329、service.go:228 均在用 |
| — | `internal/cloudagent/`、`internal/api/cloudagent_access.go` | — | **保留，不在本计划清理范围** | Cloud Agent Phase 0-2 已落地的活代码；`go.mod` 的 `cloudwego/eino v0.9.19` 依赖随之保留 |

### 2.3 go.mod 依赖清理（B2/B5/B7 完成后）

```bash
go mod tidy
```

预期移除：`github.com/natefinch/lumberjack`。`cloudwego/eino`、`voocel/litellm`、`duckdb-go`、`go-sqlite3` 均在用，不动。

### 2.4 文档同步（**v2 大幅扩充**——v1 低估了残留量）

| 文件 | 具体问题 | 动作 |
|---|---|---|
| `internal/README.md` | 第 8/29/44-45/56/72/76 行提及 `jobs worker`、`jobs → catalog`、`deploy` 模块说明 | 删除对应段落 |
| `README.md` | 第 3 行「基于 Turso/libSQL」、第 22 行「Turso/libSQL 驱动」、第 36 行同上、第 83-84 行 backups/restore 路由表、第 145 行 `turso/` 目录（**该目录已不存在，文档描述与代码脱节**）、第 149 行 `internal/jobs/` 说明、第 160 行技术栈「Turso/libSQL」 | **整体重写**架构概览与技术栈两节为「DuckLake-only + S3」叙事；补充 Cloud Agent 一节（当前 README 完全没提 Cloud Agent，是本计划外的另一个文档缺口） |
| `docs/deployment.md` | 第 62 行 `dsn: "libsql://simplebase-catalog.turso.io"`、第 237/239 行「由 Turso 管理」/「Turso 平台功能」 | 改为系统 DuckLake 库表述；catalog dsn 示例整段删除或改为 `system_database.name` |
| `docs/migration-guide.md` | 整篇标题「迁移到 Turso/libSQL + S3 新链路」，Turso 已退役后文档自相矛盾 | **需产品决策**：① 标记为历史存档（文首加 deprecated 说明，保留供归档查阅）；② 重写为「从 Turso 时代迁移到 DuckLake-only」的第二次迁移指南。本计划建议①，避免过度投入 |
| `config.example.yaml` | 第 63 行 `database_id: "simplebase-catalog"  # 遗留字段` | 随 L10 一并删除（若采纳 L10） |

---

## 3. 前端清理清单（ui/src/）

### 3.1 整文件删除

| # | 路径 | 行数 | 标签 | 依据 |
|---|---|---|---|---|
| F1 | `src/pages/FaaSManager.vue` | ~190 | D+B | 后端无 FaaS 模块，路由 `hidden: true`；建议删除页面 + mock.functions + faas 域 + types.FaasFunction |
| F2 | `src/services/mock.js` 中 `faas` 域 + `functions` 种子 | ~35 | B | 随 F1 |
| F3 | `src/services/http-api.ts` 中 `faas` 域 | ~13 | D | 调用不存在的 `/faas/*` |
| F4 | `src/services/types.ts` 中 `FaasFunction` + `Api.faas` | ~10 | B | 随 F1 |
| F5 | `vite.config.ts` 的 `/ws` 代理 | 2 | B | 后端无 WS；Logs 改 HTTP 后代理无用 |
| F6 | `.env.example` 的 `VITE_WS_BASE_URL` 注释 | 2 | B | 同上 |
| **F7（v2 撤销）** | ~~`SbCodeBlock.vue`~~ | — | ~~A~~ | **v1 结论已过时**：复核发现该组件现被 `src/components/modal/DocumentListModal.vue` 使用（文档字段展示），**非死代码，不删** |
| **F8（v2 新增）** | `src/pages/LlmManager.vue`（97 行） | 97 | **E** | **v1 遗漏 / 结论方向错误**：v1 认为该页「缺会话列表，需要补功能」；复核发现路由 `/llm` 已 `redirect: '/agents'`，`LlmManager.vue` **未被任何路由 name 引用、未被任何组件 import**，是纯孤儿页面（Cloud Agent 已完整替代其功能）。**应直接删除整个文件**，而非增强 |
| **F9（v2 新增）** | `src/components/NavMenu.vue` 第 37 行 `llm: RobotOutlined` | 1 | A | `iconMap.llm` 键：路由体系中已无 `name: 'llm'` 的路由（只有 redirect），此键永不命中 `iconMap[r.name]` 查找，为死配置 |

### 3.2 行级清理与修正

| # | 文件 | 位置 | 问题 | 动作 |
|---|---|---|---|---|
| U1 | `src/services/http-api.ts` | `metrics.summary/trend` | 调 `GET /metrics/summary`（根路径，缺项目前缀），后端实际路由是 `:p/metrics/summary`（已在 router.go 272-274 行确认存在） | 改为 `'/v1/projects/'+projectId+'/metrics/summary'` 并加 `projectId` 参数 |
| U2 | `src/services/http-api.ts` + `types.ts` | `logs.connect` 仍连 `ws://…/ws/logs`（后端永不存在） | **重构**为 `logs.list(projectId, params)` + `logs.retention`（后端 `GET :p/logs` 已实现，见 router.go 278-280） |
| U3 | `src/services/types.ts` | 缺 `logs`（HTTP 版）域；`audit` 域视需要补 | 补齐（`llmSessions`/`llmSettings`/`agents`/`agentThreads` **已在当前代码补齐，v1 描述"待补"已过时**，仅 `logs` 仍需重构） |
| U4 | `src/pages/Logs.vue` | 整页基于 WS `connect()` | **重写**为 HTTP 查询页：level/q/from/to 过滤 + 手动刷新；删除 connect/disconnect/backendSupported |
| U5 | `src/pages/Settings.vue` | 第 3/9 行 alert「后端 §3.9 凭证接口未就绪」 | **已过时**：`llm/settings` 服务端 API 已实现（router.go 295-296：`GET/PUT :p/llm/settings`）。去掉 alert；`stores/settings.ts` 的 provider/model 默认值改走服务端 API，localStorage 仅留厂商 Key 明文缓存 |
| ~~U6~~ | ~~`LlmManager.vue` 补会话侧栏~~ | — | **v2 撤销，见 F8**：该页应删除而非增强 |
| U7 | 根目录 `simplebased`（90MB 二进制）、`output/cfbee745…/` 两个脱敏 md | — | 构建产物/临时产物入库，建议清理工作区（`.gitignore` 已含 `simplebased`，检查是否已被 git 跟踪并 `git rm --cached`） |
| U8 | `internal/web/dist/`、`ui/dist/` | — | 构建产物跟踪策略——独立决策，不在本计划强制 |

**小计：前端直接删除约 350 行（FaaS ~250 + LlmManager 97 + NavMenu 1 行）+ 重写 Logs 约 135 行**

---

## 4. 交叉验证记录（审计留痕，v2 复核）

```
rg "internal/jobs"        → 仅 app.go / adapters_plan79.go / metrics.go（声明，无 NewWorker 调用）
rg "NewWorker|NewEnqueuer" --type go | grep -v jobs/_test → 0 命中
rg "internal/deploy"      → 0 命中
rg "internal/utils"       → 0 命中
rg "internal/prom"        → 仅注释提及
rg "cfg.Catalog\."        → 仅 config.go 自身（加载/校验/脱敏），无消费方【v2 新增验证】
rg "SbCodeBlock" ui/src/  → FaaSManager.vue + DocumentListModal.vue（【v2】不再是零引用）
rg "LlmManager" ui/src/router/index.ts → 0 命中（只有 /llm redirect）【v2 新增验证】
rg "LlmManager" ui/src/**/*.vue ui/src/**/*.ts → 仅自身文件【v2 新增验证：确认孤儿页面】
rg "JobsTotal|JobDurationSeconds" --type go → 仅 metrics.go 定义 + jobs/queue.go 消费（将随 jobs 删除）【v2 坐实符号名】
rg "job_queue_test" internal/catalog/ → 存在，139 行，依赖将删除的 Repository.Enqueue 等方法【v2 新增发现】
rg "cloudwego/eino" go.mod → 存在，Cloud Agent 依赖，不在清理范围
rg "internal/cloudagent" internal/jobs internal/deploy internal/utils internal/prom → 0 命中（Cloud Agent 不依赖待删包，删除安全）
```

---

## 5. 重构计划与方案（Refactor）

### R1【P0】删除 jobs 子系统 + app 装配收敛（后端）

1. 删 `internal/jobs/`（3 文件）、`internal/deploy/`、`internal/utils/`、`internal/prom/`、`internal/catalog/alias.go`、`internal/log/config.go`、`internal/log/log_tree.go`。
2. 删 `internal/catalog/job_queue_test.go`（**v2 补充**，否则编译报错）。
3. `app.go`：删 L1 列出的代码块与 `jobs` import。
4. `router.go`：删 `JobEnqueuer`/`JobInput` 与 `Dependencies.JobEnqueuer`（**注意保留 `cloudagent` 相关 import 与字段，不要一并误删**）。
5. `adapters_plan79.go`：删 jobEnqueuerAdapter。
6. `catalog/repository.go` + `sql_repository.go`：删 6 个 Job 方法实现；`service.go` 删 `GetDatabaseForJob`。
7. `migrate.go`：`sys_jobs` DDL **保留**（版本历史兼容），加注释「deprecated, retained for migration history」。
8. `observability/metrics.go`：删 `JobsTotal`/`JobDurationSeconds`/`IncJobTotal`/`ObserveJobDuration`（**v2 已坐实符号名**）。
9. `go mod tidy` → `go build ./... && go test ./...`。
10. 更新 `internal/README.md`。

**验收**：`go vet ./...` 干净；`go test ./...` 全绿；`rg "internal/jobs" → 0`。

### R2【P0】备份/恢复 501 路由退役（可选）

- 删 `database_handler.go` 的 `CreateBackup`/`RestoreDatabase` + `router.go` 挂载。
- `proto-http.md` §3.1 增勘误。
- 若产品要保留占位契约则跳过本条。

### R3【P1，v2 撤销】~~SbCodeBlock 归位~~

**撤销**：该组件已被 `DocumentListModal.vue` 实际使用，非死代码，无需处置。

### R4【P1】Logs 页 HTTP 化（前端）

```ts
logs: {
  list: (projectId: string, q: { level?: string; q?: string; from?: string; to?: string; limit?: number }) => Promise<LogEvent[]>
  getRetention: (projectId: string) => Promise<{ scope: string; keepDays: number; updatedAt: string }>
  putRetention: (projectId: string, keepDays: number) => Promise<void>
}
```

- `http-api.ts` 实现三方法；`mock.js` 的 `logs.connect` 改为 `list`。
- `Logs.vue` 重写为过滤表单 + 表格；删 WebSocket 相关代码。
- 删 `vite.config.ts` `/ws` 代理、`.env.example` WS 注释。

### R5【P2，v2 大幅精简】types.ts 补齐

**v1 描述的 `llmSessions`/`llmSettings`/`agents`/`agentThreads` 域已在当前代码全部补齐**（Cloud Agent 落地时一并完成）。本条仅剩 `logs`（见 R4）与可选 `audit.list`。

### R6【P2】Settings 数据源切换（前端）

- `stores/settings.ts` 的 provider/model 默认值改走 `llmSettings.get/put`（服务端 API 已实现）。
- `Settings.vue` 删过时 alert。
- 验收：改默认模型后另一浏览器（同 Key）可见。

### R7【P1，v2 改写】~~LlmManager 会话化~~ → **删除孤儿页面**

**v1 方向错误**：v1 建议给 `LlmManager.vue` 补会话侧栏作为「向 Cloud Agent 过渡」；复核发现 Cloud Agent 已完整落地且 `LlmManager.vue` 已是零引用孤儿页面。

**正确动作**：
1. 删除 `ui/src/pages/LlmManager.vue`。
2. 删除 `NavMenu.vue` 第 37 行 `llm: RobotOutlined` 死键。
3. 检查 `router/index.ts` 的 `{ path: '/llm', redirect: '/agents' }` 是否需要保留（建议保留，兼容旧书签/外链）。
4. **不要删除** `useAiChat.ts`、`components/ai/AiChat.vue`——两者仍被 `AgentManager.vue` 复用。
5. 检查 `types.ts`/`http-api.ts` 的 `Api.llm.providers/chat/stream` 域：`useAiChat.ts` 内部仍调用 `api.llm.chat/stream`（`AgentManager.vue` 通过 `custom-send` 绕过了它，但 `useAiChat` 默认路径仍依赖），**保留此域**，不要随 LlmManager 页面一起删除。

### R8【P2】metrics 域接通（前端）

`http-api.ts` 的 `metrics.summary/trend` 路径改为 `:p/metrics/summary|trend` 并加 `projectId` 首参；Dashboard 趋势图渲染真实数据。

### R9【P2，v2 新增】CatalogConfig.DatabaseID 退役（后端配置，需产品确认）

见 §2.2 L10。**这是一个 breaking change**，建议先确认现网 `.env`/部署脚本是否设置了 `SIMPLEBASE_CATALOG_DATABASE_ID`，若有依赖需先发公告再移除；若确认无外部依赖可直接删除结构体+校验+配置示例。

### R10【P2，v2 新增】文档补写：README.md 缺 Cloud Agent 一节

`README.md` 当前完整未提及 Cloud Agent（`/agents`、`internal/cloudagent`），与代码现状脱节程度已超出「清理」范畴，属于文档补写任务，建议与 §2.4 的 Turso 措辞清理合并为一个文档 PR。

---

## 6. 实施排期（v2 调整）

| 阶段 | 内容 | 估算 | 依赖 |
|---|---|---|---|
| **C1** | R1 后端大删（jobs/deploy/utils/prom/log 冗余 + job_queue_test.go + go mod tidy + README） | 0.5–1 d | 无 |
| **C2** | R2 备份路由退役（可选） | 0.5 d | C1 |
| **C3** | **R7 删除 LlmManager.vue + NavMenu 死键**（v2 提升优先级，纯删除零风险） | 0.25 d | 无，可与 C1 并行 |
| **C4** | R4 Logs HTTP 化 | 1 d | 无 |
| **C5** | R6 Settings 服务端化 + R8 metrics 接通 | 1 d | C4 |
| **C6** | R9 CatalogConfig.DatabaseID 退役（**需先确认无外部依赖**） | 0.5 d | 产品确认 |
| **C7** | R10 + §2.4 文档补写/勘误（README/deployment/migration-guide） | 1 d | C1 |
| **C8** | FaaSManager 删除收尾 + 验收清单 | 0.5 d | C1 |

**总估：4.75–5.75 人天**。C1/C3 可立即开始（零风险纯删除）。

---

## 7. 验收标准（DoD）

### 后端

1. `rg "internal/jobs|internal/deploy|internal/utils|internal/prom" --type go` → **0 命中**。
2. `go build ./...`、`go vet ./...`、`go test ./...` 全绿；`go mod tidy` 后 lumberjack 移除，`cloudwego/eino` 保留。
3. `./build.sh dev` 启动：Databases / SqlConsole / Data / S3 / LLM chat / Cloud Agent 全链路可用。
4. `sys_migration_versions` 含历史全部版本；已部署实例升级后启动不报错。

### 前端

5. `yarn build` 零报错；`npx vue-tsc --noEmit` 无类型错误。
6. `rg "faas|/ws/logs|wsBase" src/` → 0 命中；Logs 页用 `GET :p/logs` 真实出数据。
7. `rg "LlmManager" src/` → 0 命中（除本文档）。
8. Settings 修改默认模型 → 刷新/换浏览器仍生效。
9. Dashboard 趋势图渲染 `:p/metrics/trend` 数据。

### 仓库卫生

10. `git status` 中无 `simplebased` 二进制、无 `output/` 脱敏产物。
11. README.md 不再出现「Turso/libSQL」作为当前架构描述；补充 Cloud Agent 一节。

---

## 8. 风险与不做的事

| 风险 | 对策 |
|---|---|
| 删 `sys_jobs` Go 路径后未来「后台任务」需求回归 | git 历史可恢复；Cloud Agent 的 `sys_agent_runs` 走新表新包 |
| **CatalogConfig.DatabaseID 删除是 breaking change** | 先 grep 部署脚本/CI 配置确认无 `SIMPLEBASE_CATALOG_DATABASE_ID` 依赖，再执行 R9 |
| Logs HTTP 化后无实时性 | 一期不做 WS；页面加「手动刷新 + 可选轮询」 |
| **不做**：`litellm` → Eino 迁移 | Cloud Agent 用 eino，LLM Gateway 仍用 litellm，两者并存是当前既定架构，不在本计划合并 |
| **不做**：`docs/migration-guide.md` 重写 | 建议标记 deprecated 存档而非重写，另开产品决策 |
| **不做**：`internal/web/dist` git 跟踪策略变更 | 涉及发布流程，独立决策 |

---

## 9. 与既有计划的关系

| 文档 | 本计划对它的动作 |
|---|---|
| `ui-plan-v2.md` | 执行 FaaS 菜单从「隐藏」升级为「删除」；关闭 Logs 改 HTTP；Dashboard 趋势图接入 |
| `databases-and-s3-plan.md` | 执行 §4.2 退役清单中剩余的 jobs 字段清理；backups/restore 501 → 移除（可选） |
| `system-database-plan.md` | 执行 Phase C「删除死代码」的前端半边（Mock 日志 WS） |
| `cloud-agent-plan.md` | **R7 修正**：确认 Cloud Agent 已完全替代 LLM 对话页，触发旧页面删除 |
| `proto-http.md` | 随 R2/R4 同 PR 勘误；§3.12 Cloud Agent 已存在，无需本计划补充 |

---

## 10. 附录：v1 → v2 变更摘要

| 条目 | v1 结论 | v2 修正 | 原因 |
|---|---|---|---|
| 文档落盘 | 已写入 `plan/planv2.0/` | **重新确认写入**（v1 实际未落盘到该路径） | 环境/会话问题 |
| SbCodeBlock | 建议删除（零引用） | **撤销**，保留 | 已被 DocumentListModal.vue 复用 |
| LlmManager.vue | 建议补会话侧栏 | **改为直接删除** | 已是孤儿页面，Cloud Agent 已替代 |
| NavMenu.vue | 未提及 | 新增：删 `llm` 死键 | 随 LlmManager 一起排查发现 |
| CatalogConfig.DatabaseID | 未提及 | 新增 L10/R9 | 新一轮 rg 排查发现的死配置 |
| job_queue_test.go | 未点名 | 新增到 L5 | 删除 Job 方法后会导致编译失败 |
| metrics.go Job 符号 | 只写"待确认" | 坐实为 4 个具体符号 | 补充验证 |
| README/deployment 文档残留 | 轻描"检查即可" | 扩充为具体行号 + 整节重写建议 | 复核发现残留比预期重 |
| eino/Cloud Agent | 不存在 | 明确列为"保留，不清理" | 代码库演进新增功能 |
