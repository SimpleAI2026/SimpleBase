# 系统库（System Database）统一存储计划

> **状态**：Phase A 已落地；Phase B 大部分 API 已落地（metrics/S3 索引/LLM 会话与 settings/logs/audit）；Phase C 前端对齐未做  
> **日期**：2026-09-16  
> **决策**：后端业务与管控数据全部落入 **DuckLake Databases**；启动时创建并迁移系统表；**不兼容历史数据**（可整库丢弃）  
> **范围**：监控大盘、数据库列表、S3 对象列表索引、LLM 对话、设置配置、日志与日志管理；以及今日仍落在平台 `catalog.db`（SQLite）中的租户 / 项目 / API Key / 配额 / 审计等元数据  
> **关联**：[`db-ducklake-plan.md`](./db-ducklake-plan.md)、[`databases-and-s3-plan.md`](./databases-and-s3-plan.md)、[`proto-http.md`](./proto-http.md)、[`ui-settings-chat-plan.md`](./ui-settings-chat-plan.md)、[`ui-plan-v2.md`](./ui-plan-v2.md)

---

## 0. 一句话目标

**实例内只保留一个「系统 DuckLake 库」作为后端唯一元数据与管控面存储；用户数据仍走普通用户 DuckLake 库。**  
启动时保证系统库存在、系统 schema 已应用、种子数据就绪；各 HTTP 模块读写改为系统库中的表，废弃平台 SQLite `catalog.db`、浏览器 localStorage 中的设置/密钥（作为权威源）、以及 Logs/Dashboard 的空壳/Mock 数据源。

---

## 1. 背景与现状问题

### 1.1 数据面分裂（必须收敛）

| 模块 / 页面 | 当前权威存储 | 问题 |
|---|---|---|
| 数据库列表 / 租户 / 项目 / API Key | `.cache/platform/catalog.db` 或 DevMode `.cache/dev/catalog.db`（SQLite） | 与「Databases = DuckLake」产品叙事不一致；双轨装配（DevMode vs 生产） |
| 监控大盘 | 无 JSON 指标表；Prometheus `/metrics` 文本；Dashboard 临时拼 `quota` + `databases` | 无趋势、无历史、无法按项目查询 |
| S3 对象列表 | **实时**列平面 A（`FileStore.List`）；无持久索引 | 无法做跨页检索、审计、配额统计；大前缀 list 贵 |
| LLM 对话 | 网关实时调用；会话状态主要在前端；`llm_provider_configs` / `usage_events` 在 catalog SQLite | 对话历史不落库；设置页 Key 在 localStorage，与服务端凭证脱节 |
| 设置 | `ui` `localStorage`（`sb_settings_v1`） | 多端不同步；无法审计；后端 §3.9 未落地 |
| 日志 | 文件 zap / Mock WS；`operations` 审计占位恒空 | Logs 页不可用；无查询、无保留策略、无按项目过滤 |

### 1.2 产品约束（与既有计划对齐）

1. **用户库**继续 DuckLake-only（`databases-and-s3-plan.md`），本计划不改用户 SQL / 文档 API 语义。  
2. **平面 A / B** 仍分清：用户上传文件与 DuckLake parquet 仍走 S3；本计划新增的是 **平面 C：系统元数据表**（存在系统 DuckLake 内，其自身 DATA_PATH 仍可落平面 B）。  
3. **HTTP 路径**尽量保持 [`proto-http.md`](./proto-http.md) 已有前缀；缺口（metrics JSON、logs、settings、chat history）在本计划补契约。  
4. **历史兼容：不做。** 允许删除 `.cache/platform`、`.cache/dev`、旧 `catalog.db`、前端 settings localStorage；用户需重新建项目 / 配 Key。

---

## 2. 核心决策（锁定）

| # | 决策 | 说明 |
|---|---|---|
| D1 | **单一系统库 / 实例** | 每个 SimpleBase 实例恰好一个系统 DuckLake 库（逻辑名固定，如 `simplebase-system`），跨项目共享管控面；行级用 `project_id` 隔离 |
| D2 | **系统库也是 DuckLake** | 与用户库同一引擎 / Factory / Syncer；DevMode DATA_PATH 本地盘，非 DevMode 走 S3 平面 B |
| D3 | **废弃平台 SQLite catalog** | `internal/catalog` 的 SQLite Repository 退役；Repository 改为对系统库执行 SQL（或薄封装） |
| D4 | **启动必迁移** | `App.assembleDeps`：确保系统库 ready → `ApplySystemMigrations` → 种子 tenant/project/api_key → 再对外 listen |
| D5 | **丢弃历史** | 无迁移脚本从旧 catalog.db / localStorage 导入；文档写明「升级即重置管控面」 |
| D6 | **S3 列表 = 索引表 + 可选实时校验** | `sys_s3_objects` 由上传/删除 API 维护；列表 API 默认读表；提供 `?refresh=1` 触发对平面 A 的 list 对账（异步或同步短超时） |
| D7 | **密钥不进明文列** | LLM / 设置凭证只存 `credential_ref` 或加密 blob；明文仅存实例密钥管理（env / KMS），与现有 `CredentialRef` 方向一致 |
| D8 | **project_id 必须为 UUID** | 与 descriptor Validate 一致；DevMode 种子改为 UUID（废弃字符串 `proj-01` 作为权威 ID；展示名可仍为「商城后台」） |

---

## 3. 目标架构

```
┌─────────────────────────────────────────────────────────────┐
│                     HTTP /v1/...                            │
│  Dashboard · Databases · S3 · LLM · Settings · Logs · …     │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│              SystemStore（新）                               │
│  打开/持有系统 DuckLake 连接；模块 Repository 全走此连接      │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  DuckLake DB: simplebase-system（系统库）                    │
│  ┌─────────────┐ ┌──────────────┐ ┌─────────────────────┐  │
│  │ 管控元数据   │ │ 观测 / 日志   │ │ 产品模块表           │  │
│  │ tenants …   │ │ metrics_*    │ │ s3_objects          │  │
│  │ projects    │ │ log_events   │ │ llm_sessions/msgs   │  │
│  │ databases   │ │ operations   │ │ settings_*          │  │
│  │ api_keys …  │ │              │ │                     │  │
│  └─────────────┘ └──────────────┘ └─────────────────────┘  │
│         DATA_PATH → 平面 B（或 DevMode 本地盘）               │
└─────────────────────────────────────────────────────────────┘
                            │
          用户库创建时 ───────┴──────► 普通用户 DuckLake（仅业务表）
```

### 3.1 与用户库的边界

| | 系统库 | 用户库 |
|---|---|---|
| 谁创建 | 进程启动（幂等） | 用户 API `POST .../databases` |
| 谁可删 | **禁止**经用户 DELETE；仅运维销毁实例 | 用户可软删 |
| 表内容 | 管控 + 六大模块 | 用户业务 SQL / 文档集合 |
| 是否出现在 Databases 列表 | **默认隐藏**（`kind=system`）；运维开关可显示只读 | 正常列出 |

---

## 4. 启动流程（必须）

```
1. Load config（含 S3 / DevMode / instance_id）
2. 解析系统库定位器：
   - 优先：config `system_database.id`（若已落盘）
   - 否则：约定路径 `.cache/system/locator.json`（含 database_id、tenant_id）
3. 若定位器不存在：
   a. 用最小引导（见 §4.1）分配 reserved tenant + 创建系统 DuckLake
   b. 写 locator.json +（非 DevMode）写 descriptor 到 S3
4. Factory.Open(系统库, ReadWrite)
5. ApplySystemMigrations(conn)     ← 本计划全部系统表
6. SeedIfEmpty（tenant / 默认 project / Dev API Key / 默认配额）
7. 用系统库连接装配：
   CatalogService、Auth、Usage、Audit、Settings、Logs、Metrics、S3Index …
8. 挂载 HTTP；/health/ready 检查系统库 Ping +（非 DevMode）S3 Check
```

### 4.1 引导期「鸡生蛋」

系统库的 `databases` 表要记录自己，但不能依赖尚未打开的系统库。约定：

1. **引导只允许写 locator 文件 + 直接调 DuckLake Factory.Create/Open**，不经过 CatalogService。  
2. 系统库首次 Open 且 migrations 成功后，**回填**一行 `sys_databases`（见 §5），`kind='system'`，`id` 与 locator 一致。  
3. 之后所有用户库生命周期只通过 CatalogService → 系统库表。

引导代码放在 `internal/systemdb`（新包），禁止散落在 `app.go` 长函数里。

### 4.2 失败语义

| 失败点 | 行为 |
|---|---|
| 系统库 Open 失败 | 进程退出，不 listen |
| Migration 失败 | 进程退出（与今日 catalog migration 一致） |
| Seed 失败 | 进程退出 |
| 运行中系统库连接掉线 | `/health/ready` → 503；写路径返回 `system_store_unavailable` |

---

## 5. 系统 Schema（v1）

> 引擎：DuckLake。**不使用 PRIMARY KEY / AUTOINCREMENT**（与现有 DuckLake 约束一致）；唯一性由应用层 + `CREATE UNIQUE INDEX`（若扩展支持）或启动校验保证。  
> 所有表名建议前缀 `sys_`，避免与用户库习惯冲突。  
> `ApplySystemMigrations` 自管 `sys_migration_versions(version, name, applied_at)`。

### 5.1 管控元数据（取代 catalog.db）

对应今日 `internal/catalog/migrations.go` v1–v9，迁入系统库后表名加前缀（实现时可映射，计划以 `sys_*` 为准）：

| 表 | 用途 | 关键列（摘要） |
|---|---|---|
| `sys_tenants` | 租户 | `id`, `name`, `created_at` |
| `sys_projects` | 项目 | `id`(UUID), `tenant_id`, `name`, `created_at` |
| `sys_databases` | **数据库列表**（用户库 + 系统库自身） | `id`, `tenant_id`, `project_id`, `name`, `kind`(`user`\|`system`), `status`, `storage_prefix`, `format_version`, `deleted_at`, `created_at`, `updated_at` |
| `sys_api_keys` | API Key | `id`, `project_id`, `key_hash`, `permissions`, `created_at`, `revoked_at` |
| `sys_llm_provider_configs` | 服务端 LLM 供应商 | 同今日 + `credential_ref` |
| `sys_usage_events` | LLM/API 用量 | 同今日 |
| `sys_operations` | 操作审计（真写入，不再占位） | 同今日 |
| `sys_jobs` | 后台任务（若仍需要） | 同今日 |
| `sys_project_quotas` | 配额 | 同今日 |

**Databases 页**数据源：`SELECT … FROM sys_databases WHERE project_id=? AND kind='user' AND deleted_at IS NULL`。

### 5.2 监控大盘（Dashboard）

| 表 | 用途 |
|---|---|
| `sys_metric_samples` | 原始/分钟级采样：`id`, `project_id`, `name`, `value_double`, `labels_json`, `occurred_at` |
| `sys_metric_rollups_hourly` | 小时汇总：`project_id`, `name`, `bucket_start`, `count`, `sum`, `min`, `max` |

采集：

- 请求中间件异步写入（采样，避免每请求同步写系统库成为瓶颈；可内存 ring buffer + 批量 flush）。  
- 定时任务从 `sys_databases` / `sys_usage_events` / `sys_s3_objects` 聚合写入 rollup。

API（补 `proto-http.md` §6.1）：

```
GET /v1/projects/:projectID/metrics/summary
GET /v1/projects/:projectID/metrics/trend?days=7
```

响应从上述表查询，**禁止**再让前端解析 Prometheus 文本。

### 5.3 S3 对象存储列表（索引）

| 表 | 用途 |
|---|---|
| `sys_s3_objects` | 对象索引：`project_id`, `object_key`, `size`, `etag`, `content_type`, `last_modified`, `created_at`, `deleted_at` |
| `sys_s3_sync_runs` | 对账任务：`id`, `project_id`, `status`, `listed`, `inserted`, `removed`, `error`, `started_at`, `finished_at` |

写路径：

- `PUT/POST` 上传成功 → upsert `sys_s3_objects`  
- `DELETE` 成功 → 软删或硬删行  
- `GET .../s3/objects` **默认读表**（分页）；`refresh=true` 时对平面 A ListObjectsV2 对账并更新表  

> 物理对象仍在平面 A；表只是索引。删除历史索引数据不影响已有 S3 文件，除非显式跑全量 reconcile。

### 5.4 LLM 对话信息

| 表 | 用途 |
|---|---|
| `sys_llm_sessions` | `id`, `project_id`, `title`, `provider`, `model`, `created_by`, `created_at`, `updated_at`, `archived_at` |
| `sys_llm_messages` | `id`, `session_id`, `project_id`, `role`, `content`, `token_input`, `token_output`, `request_id`, `created_at` |
| `sys_llm_settings` | 项目级默认：`project_id`, `default_provider`, `default_model`, `temperature`, `max_tokens`, `updated_at` |

API（在 `proto-http.md` 增 §3.9 / §3.10）：

```
GET/POST   /v1/projects/:p/llm/sessions
GET/DELETE /v1/projects/:p/llm/sessions/:id
GET/POST   /v1/projects/:p/llm/sessions/:id/messages
GET/PUT    /v1/projects/:p/llm/settings
# 现有 chat/stream 成功后服务端写入 messages + usage_events
```

设置页厂商 Key：走 `sys_llm_provider_configs.credential_ref`（服务端托管）；**取消 localStorage 作为权威源**（前端可读缓存，以 GET settings/providers 为准）。

### 5.5 设置配置

| 表 | 用途 |
|---|---|
| `sys_settings_global` | 实例级：`key`, `value_json`, `updated_at`（如默认保留天数） |
| `sys_settings_project` | 项目级：`project_id`, `key`, `value_json`, `updated_at`（主题偏好可仍本地；**模型默认、功能开关**进库） |
| `sys_settings_user` | （可选，二期）按 principal：`principal_id`, `project_id`, `key`, `value_json` |

一期建议：

- 主题：可继续本地（无安全含义）  
- 默认模型 / 供应商 / 温度：必须进 `sys_llm_settings` / `sys_settings_project`  
- 功能开关、日志保留天数：`sys_settings_global`

### 5.6 日志与日志管理

| 表 | 用途 |
|---|---|
| `sys_log_events` | 结构化应用/访问日志：`id`, `project_id`(可空), `level`, `logger`, `message`, `fields_json`, `request_id`, `occurred_at` |
| `sys_log_retention` | `scope`(`global`\|`project`), `project_id`, `keep_days`, `updated_at` |
| `sys_log_exports` | 导出任务元数据（可选） |

写入：

- zap 增加 **SystemDB Core**（异步批量），与文件 sink 并行；失败只打 stderr，不阻断请求。  
- HTTP access：在已有 access 中间件中写摘要行（采样可配）。  
- `sys_operations` 继续作为「业务审计」；`sys_log_events` 作为「运行日志」。Logs 页默认查 `sys_log_events`。

API（取代不存在的 `/ws/logs`）：

```
GET  /v1/projects/:p/logs?level=&q=&from=&to=&cursor=&limit=
POST /v1/projects/:p/logs/query          # 复杂过滤（可选）
GET/PUT /v1/projects/:p/logs/retention
# 不做 WebSocket 一期；前端轮询或手动刷新
```

定时：按 `sys_log_retention` 删除过期 `sys_log_events` / 过旧 `sys_metric_samples`。

---

## 6. 代码与包结构（建议）

```
internal/systemdb/
  locator.go          # locator.json 读写
  bootstrap.go        # 创建/打开系统库
  migrate.go          # ApplySystemMigrations + 全部 DDL
  seed.go             # Dev/空库种子
  store.go            # 持有 *sql.DB / 生命周期

internal/catalog/     # 保留接口；实现改为 systemdb SQL（sqlite_repository.go 删除或改名）
internal/metricsapi/  # summary/trend 查询
internal/logsapi/     # log_events 查询与写入适配
internal/settings/    # settings 读写
# llm：session/message repository 新增；chat handler 写库
```

`internal/app/app.go`：删掉「platform catalog SQLite」与「DevMode 另一套 catalog」分支中的 **双 SQLite 文件**路径，统一：

```
systemStore := systemdb.Bootstrap(...)
catalog.NewService(systemStore.CatalogRepo(), ...)
```

DevMode **只影响 DATA_PATH 是否本地**，不再影响「用不用系统库」。

---

## 7. API / 契约变更摘要

| 领域 | 变更 |
|---|---|
| Databases §3.1 | 实现改为读 `sys_databases`；列表默认过滤 `kind=user` |
| S3 §3.4 | List 默认读 `sys_s3_objects`；增 `refresh`；upload/delete 维护索引 |
| Quota / Audit §3.6 | Audit **真查** `sys_operations`；去掉空数组占位 |
| Metrics §6.1 | **落地** summary/trend，数据来自 `sys_metric_*` |
| Logs | 新 §；轮询 HTTP，废弃 WS 依赖 |
| Settings / LLM sessions | 新 §；前端去掉 settings 权威 localStorage |
| Projects | `proj-01` 种子改为 UUID；文档与 Dev Key 说明更新 |

`proto-http.md` / `proto.http` 必须与实现同 PR 更新。

---

## 8. 前端影响（本计划后端为主，前端跟进）

| 页面 | 改动 |
|---|---|
| Dashboard | 接 `metrics/summary|trend`；去掉 Mock 依赖 |
| Databases | 无路径变更；注意 project UUID |
| S3Manager | 列表仍原 API；感知分页与 refresh |
| LlmManager / AiChat | 会话列表/历史走新 API；发送后以服务端消息为准 |
| Settings | 读写下发到服务端；localStorage 仅作主题缓存 |
| Logs | 改 HTTP 查询；删除 WS 连接逻辑 |

---

## 9. 明确不做

1. 从旧 `catalog.db` / localStorage **迁移**历史数据。  
2. 一期 WebSocket 实时日志。  
3. 把用户业务表塞进系统库。  
4. 多实例共享同一系统库写（仍单 writer 实例模型）。  
5. FaaS 模块（仍无后端则保持 Mock，另立 plan）。  
6. 系统库对终端用户开放任意 SQL（SqlConsole 默认不可选系统库）。

---

## 10. 实施阶段

### Phase A — 骨架（阻塞后续） — **已落地**

1. 新增 `internal/systemdb`：locator、bootstrap、migrate（先迁入今日 catalog 等价表）、seed  ✔  
2. `app.go` 切换装配；删除 platform/dev `catalog.db` 路径  ✔  
3. 全量 catalog 单测改为 DuckLake 系统库或测试用内存 DuckLake / 同构 sys_* SQLite  ✔  
4. DevMode + 非 DevMode 冷启动验收：空目录启动 → 系统库 ready → 种子 Key 可调 `GET /v1/projects`  ✔（DevMode 单测覆盖）

### Phase B — 六大模块表与 API — **大部分已落地**

1. metrics 表 + summary/trend API + 中间件采样  ✔  
2. s3_objects 索引 + List/Upload/Delete 挂钩 + refresh  ✔  
3. llm_sessions/messages/settings + chat 写库  ✔（stream 未落库；provider-catalog 未做）  
4. settings_global/project API  ✔  
5. log_events + retention + Logs HTTP API；operations 真写入  ✔（保留任务未做）  

### Phase C — 前端对齐与清理

1. Dashboard / Logs / Settings / Chat 接新 API  
2. 文档：`proto-http.md`、README、`.env.example`（系统库相关配置）  
3. 删除死代码：sqlite catalog driver 路径、Audit 空实现、前端 Mock 日志 WS  

### Phase D — 加固

1. 系统库备份/只读展示开关  
2. 日志/指标保留任务  
3. S3 索引全量 reconcile 任务与指标  

---

## 11. 验收标准

| # | 标准 |
|---|---|
| A1 | 空 `cache_dir` 启动一次即可：系统库存在、`sys_migration_versions` 含本计划全部版本 |
| A2 | 进程目录中 **不再创建** `platform/catalog.db` / `dev/catalog.db` 作为权威 catalog |
| A3 | 用种子 Key：创建用户库、列表、SQL、删库全链路通过；`sys_databases` 可见对应行 |
| A4 | 上传/删除 S3 对象后 `sys_s3_objects` 行数与 List API 一致；`refresh=1` 可修复人为删行 |
| A5 | LLM 多轮对话刷新页面后历史仍在；`sys_llm_messages` 有对应行 |
| A6 | Settings 修改默认模型后另一浏览器（同 Key）可见 |
| A7 | Logs 页能按 level/时间查到 access 或业务错误日志；无 WS 依赖 |
| A8 | Dashboard summary/trend 返回 JSON 且来自系统表（可用 SQL 对账） |
| A9 | 故意损坏系统库 → 进程拒绝 ready / 或启动失败，不出现「半套 SQLite 兜底」 |

---

## 12. 风险与对策

| 风险 | 对策 |
|---|---|
| 系统库成为写热点 | 指标/日志异步批量；会话消息可批量；关键路径（创建库、鉴权）保持同步短事务 |
| DuckLake 无强 PK | 应用层 UUID；唯一索引能建则建；单测覆盖重复插入 |
| 系统库 S3 同步延迟 | 管控读走本地 cache catalog 文件（现有 Syncer）；`durability` 对系统写入同样暴露 |
| 误删系统库 | API 层拒绝 `kind=system` 的 DELETE；UI 隐藏 |
| 启动变慢 | migrations 幂等；仅首次建库重；ready 探针超时可配 |

---

## 13. 配置草案

```yaml
# config.yaml / SIMPLEBASE_* 
system_database:
  name: "simplebase-system"          # 逻辑名
  # id 由 locator 持久化，一般不必手写
  hide_from_list: true
  metrics_flush_interval: 2s
  log_flush_interval: 2s
  log_keep_days: 14
```

环境变量示例：`SIMPLEBASE_SYSTEM_DB_NAME`、`SIMPLEBASE_LOG_KEEP_DAYS`。

---

## 14. 文档与沟通

1. 本文件为后端改造权威计划；实现时勾选 Phase。  
2. 同步更新：`proto-http.md`、`ui-plan-v2.md`（Dashboard/Logs/Settings 缺口关闭）、`ui-settings-chat-plan.md`（二期后端改为「已规划落地」）。  
3. 发布说明明确：**升级将清空管控面与对话/设置/日志历史；用户 DuckLake 数据平面若未删库则仍保留，但 catalog 丢失后需运维按 S3 descriptor 恢复或重建项目绑定（本计划一期不做自动找回）。**

> 一期若无法安全「找回」用户库与新系统 catalog 的绑定，产品需接受：**用户库数据文件可能仍在 S3，但新系统库无记录则列表不可见**。可选加急跟进 Phase E「从 S3 descriptor 前缀扫描重建 `sys_databases`」——**不在本计划必达**，单列风险告知。

---

## 15. 总结

将「平台 SQLite catalog + 前端 localStorage + Mock 日志/指标」收敛为 **一个 DuckLake 系统库 + 启动强制建表**，六大产品模块各有表与 API，历史数据一律丢弃。用户业务库模型不变；管控面与观测面与 Databases 产品同一存储引擎，便于备份、同步与运维心智统一。
