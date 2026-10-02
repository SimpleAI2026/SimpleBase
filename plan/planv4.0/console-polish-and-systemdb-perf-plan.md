# SimpleBase v4.0 控制台体验统一 & 系统库性能治理计划

## 原始需求

现在有如下几个问题需要修改：

1. `/var/folders/9d/g20j9n3968gc16bygrrh88qw0000gn/T//simplebase-dev-logs.diXgon/backend.log` 分析这里的日志，为什么调用 `http://127.0.0.1:5173/v1/projects/4ne07m48/logs?limit=200` 接口会很慢，其他一些接口请求也很慢
2. `ui/src/pages/Databases.vue:144-196`、`ui/src/components/databases/kv/KvPanel.vue:70-83`、`ui/src/components/databases/CollectionPanel.vue:22-27`、`ui/src/components/databases/kv/KvApiPanel.vue:40-69`、`ui/src/pages/CronJobs.vue:40` 等，`>操作<` table 这一栏"操作"上的按钮现在有的加了 icon，有些没有加，现在改为统一不加 icon
3. `ui/src/components/settings/ConnectionPanel.vue:36-59` 这个 key 和 `ui/src/components/settings/ConnectionPanel.vue:13-24` 为合并一个，不应该有多个才对，统一为 API Key
4. `ui/src/components/settings/ConnectionPanel.vue:7-28` 太宽，导致 input 框输入时，部分被遮挡
5. `SettingsPanel.vue` 这里的主题颜色应该将按钮换成太阳或者月亮放到 `ui/src/layouts/DefaultLayout.vue:72-87` 这里，去除 `SettingsPanel.vue` 这里的 tab
6. `CronJobRunsDrawer.vue` 这里不用抽屉展示，而是改为统一的 modal 框，点击"立即执行"和"记录"都是直接弹出模态框即可 `ui/src/pages/CronJobs.vue:123-135`，去除 `CronJobRunsDrawer.vue` 抽屉的样式

基于如上需求，先分析问题和解决方案，写 plan 放到 `planv4.0`

---

> 日期：2026-09-30
> 范围：`ui/src/`（操作列 / 设置弹窗 / 顶栏主题 / 定时任务运行记录）、`internal/systemdb`、`internal/api`（日志中间件）、`internal/database/ducklake`（系统库维护）、`internal/database/lease`（KV 写租约）
> 状态：**已执行完毕（2026-09-30）**。§2–§6 前端全部落地；§1 P0-验证由 P1-C 首轮 dry-run 观察替代，P1-A/B/C/D/E 全部落地。遗留：前端全局覆盖率 functions/branches 低于 95% 阈值（缺口集中在既有 KV 组件与 SbModal 等非本任务文件，见 §7.5）。
> 依据：`simplebase-dev-logs.diXgon/backend.log`（09:36–09:55，238 行）+ 本地系统库 catalog 副本（`.cache/system/.../catalog.sqlite`，只读拷贝后用 sqlite3 统计）+ 相关源码通读
> 编号：§1 对应需求 1（性能），§2–§6 对应需求 2–6，§7 汇总排期与验收

---

## 0. 总览

| # | 需求 | 结论 | 改动面 | 风险 |
| --- | --- | --- | --- | --- |
| 1 | `/logs?limit=200` 及其他接口慢 | **不是某个 SQL 慢**，是「系统库单连接 + 每个请求都写日志 + 每 2s 一次小文件写/catalog 同步 + 无 compaction 导致小 parquet 文件堆积」四者叠加；另发现 KV 写路径 45s 卡顿的独立问题 | 后端 5 处 | 中（涉及 DuckLake 维护，必须遵守 multi-instance-consistency §3.2） |
| 2 | 操作列按钮统一不带 icon | 只有 3 个页面带 icon（Databases / CronJobs / S3Manager），其余已是纯文字 | 前端 3 文件 + 文档 | 低 |
| 3 | 连接页两个 Key 合并为「API Key」 | 合并为**一个 API Key 区块**：当前使用的 Key 输入 + 已签发列表 + 签发入口，一套标题、一套说明 | 前端 1 文件 | 低 |
| 4 | 连接页太宽、输入框被遮挡 | 根因是滚动容器 `overflow-y-auto` 裁剪了 Input 的 `ring-3` 聚焦光环 + 内容无宽度上限 | 前端 2 文件 | 低 |
| 5 | 主题按钮移到顶栏（太阳/月亮），去掉设置里的「外观」tab | 主题切换进 `DefaultLayout` 顶栏图标组；`SettingsModal` 删「外观」tab；`SettingsTab` 类型收窄 | 前端 6 文件 + 测试 | 低（有 deep-link 兼容点） |
| 6 | 运行记录不再用抽屉，「立即执行」「记录」都弹模态框 | `CronJobRunsDrawer` → `modal/CronJobRunsModal`（`SbModal`）；两个入口收敛为同一条触发链路 | 前端 2 文件 + 测试 4 个 | 低 |

建议执行顺序：**§2 → §4 → §3 → §5 → §6**（前端，互相独立，可各自一个 commit）；**§1 与前端并行**，且 §1 内先做 P0-验证再做其他。

---

## 1. 需求 1：接口慢的根因分析与方案

### 1.1 现象（来自 backend.log）

按耗时 > 0.3s 的请求整理（`duration` 单位秒）：

| 时间 | 请求 | 耗时 | 备注 |
| --- | --- | --- | --- |
| 09:36:49 | GET `/projects/:id/databases` | 2.1 | 冷启动 |
| 09:36:52 | GET `/projects/:id/s3/objects` | 5.5 | |
| 09:37:13 | GET `metrics/trend` | **23.8** | 登录后前端并发 9 个请求 |
| 09:37:14 | GET `agents` | **27.8** | |
| 09:37:15 | GET `gofunctions` / `auth/me` | **28.2** / 13.3 | `auth/me` 只是查用户也 13s |
| 09:37:16 | GET `cron-jobs` / `metrics/summary` / `quota` | **29.3** / 24.0 / 29.0 | |
| 09:38:06 | GET `logs`（即用户所说的 `limit=200`） | **27.5** | 同时 `logs/retention` 27.7s（一条极小的查询） |
| 09:38:12 | GET `logs` | 0.46 | 同一接口热态 |
| 09:38:45 | 上面那批接口再来一遍 | 0.9–1.1 | 热态 |
| 09:55:30 | GET `logs` | **5.6** | 仅隔 17 分钟，又变慢 |
| 09:39:57 / 09:40:19 | POST `/kv` | **45.9** / **45.5（500）** | 与系统库无关，见 §1.5 |

三个关键信号：

1. **同批请求几乎同时结束**（23.8→29.3s），说明它们不是各自慢，而是在**同一个资源上排队**。
2. `systemdb connection wait accumulated` 的 `wait_total` 累计：33s → 88s → 234s → 325s（`store.go:233` 的观测日志），`open:1`，`in_use:1`——**系统库连接池只有 1 条连接，所有请求在排队**。
3. `logs` 与 `logs/retention` 同时完成（27.47 / 27.68），说明它们前面有一个**占着连接约 27s 的操作**，而不是自身耗时。

### 1.2 根因（按贡献度排序）

#### R1：系统库只有 1 条连接，所有 `sys_*` 读写串行

- `ducklake/factory.go:154-155`：`SetMaxOpenConns(1)` / `SetMaxIdleConns(1)`。注释写明这是 §7.2 P6 的**有意设计**（boot 序列含 ATTACH/USE/`SET lock_configuration`，均为连接级语义；DuckLake 单写者），因此**不能简单调大连接数**。
- 于是 `auth/me`、`projects`、`agents`、`gofunctions`、`cron-jobs`、`quota`、`metrics/*`、`logs`、日志/指标刷写、catalog 同步……全部在这一条连接上排队。登录后前端并发 9 个请求 = 队尾请求要等前面 8 个跑完（≈3s × 9 ≈ 27s，与日志吻合）。

#### R2：每条查询本身就慢（≈2–3s）——小 parquet 文件堆积

对本地系统库 catalog 副本统计（`ducklake_data_file` 中仍存活的文件）：

| 表 | 存活数据文件 | 总行数 | 平均每文件 |
| --- | --- | --- | --- |
| `sys_log_events` | **150** | 388 | 2.6 行 / 1.8KB |
| `sys_metric_samples` | **116** | 378 | 3.3 行 / 1.0KB |
| `sys_migration_versions` | 37 | 37 | 1 行 |
| `sys_operations` | 27 | 27 | 1 行 |
| `sys_go_func_invokes` | 15 | 15 | 1 行 |
| 其余 `sys_*`（agents / cron_jobs / go_funcs / users …） | 各 1–7 | 各 1–7 | 1 行 |

`ducklake_snapshot` 已有 **454** 个快照。成因链：

1. 远端（S3/COS）模式下 `factory.go` 强制 `DataInliningRowLimit = 0`（防覆盖丢数据，见 `multi-instance-consistency-plan §4.2`）——**每次 INSERT 都直接落一个 parquet 文件到 COS**；
2. 系统库日志/指标 **每 2s 刷一次**（`config.yaml: metrics_flush_interval / log_flush_interval = 2s`），每次刷写 = 每张表 1 个新文件 + 1 个 snapshot；
3. **没有任何 compaction**：`maintenance.checkpoint_interval / expire_older_than / delete_older_than / rewrite_delete_threshold` 已被解析进 `Options`（`app.go:890`），但全仓非测试代码里**没有任何地方调用** `ducklake_merge_adjacent_files` / `ducklake_expire_snapshots` / `ducklake_cleanup_old_files`（grep 验证）。配置是空转的。
4. 一次读取要对每个存活文件至少 2 次 COS 往返（footer + 数据），本机到 COS 广州约 50–150ms。`ORDER BY occurred_at DESC LIMIT 200` 无法做文件剪枝，`metrics/summary` 更是 **6 条独立的 `SUM(...)` 全表扫描**（`metrics.go:72-94`），`metrics/trend` 2 条。

推论：冷读 `sys_log_events` 的 150 个文件 ≈ 数秒到二十多秒；热态（DuckDB 元数据缓存）0.46s；之后又有新文件写入，需重新拉取，5.6s。三个现象都吻合。

> 说明：R2 的「文件数 → 延迟」是由统计数据 + 代码推断，尚未做 `EXPLAIN ANALYZE` 直接证明；§1.4 的 P0-验证 就是用最小成本证实/证伪它。

#### R3：catalog 同步占用同一条连接

- `catalog_syncer.go:482-499`：同步 = `ATTACH 备份库` → `COPY FROM DATABASE` → `CHECKPOINT` → `DETACH`，**全部经 `sqlDB`（即系统库唯一连接）**，然后再上传快照与 manifest 到 COS。
- 每次写提交后 `AfterWrite → MarkDirty`（`factory.go:426`）→ debounce 200ms → 同步。日志里 `ducklake catalog synced` 每 2s 一次，snapshot_id 每次 +2；稳态下每 2s tick 就出现约 0.42s 的 `wait_delta`（≈20% 的时间连接被同步占住）。
- 同步与刷写竞争时出现 5 次 `catalog snapshot moved during copy; retry next sync`（09:36:41、09:37:16、09:43:18、09:53:04、09:55:31）——白白多一轮 COPY。

#### R4：每个 HTTP 请求都会写一条 `sys_log_events`（自我放大）

- `router.go:530`：请求中间件对**每个请求**（包括 `/logs`、`/logs/retention`、健康检查、静态资源 `/*`）调用 `RecordLog`。
- 效果：① 只要页面有任何请求，下一个 2s 刷写就产生新 parquet + 新 snapshot + 一次 catalog 上传（R2/R3 的源头）；② 打开「日志」页本身就在制造日志，日志页充满 `GET /v1/.../logs`、`GET /*` 这类噪声。

#### R5：环境因素

`dev_mode: false` + 远端 COS（广州）+ 本机网络，每次 S3 往返成本高；这放大 R2/R3，但不是可改的根因。

### 1.3 为什么之前的优化（§7.2 P1/P3/P6）没挡住

P3 把「请求路径同步 flush」改成了后台批量写，**降低了请求路径的写延迟**，但没改变「写多少个文件」；P6 结论是连接数必须为 1。也就是说**读放大（文件数）与写放大（每 2s 一批）从未治理**，且 maintenance 配置无执行体。

### 1.4 方案

按「先证实 → 止血（减少产生）→ 治本（合并回收）→ 削峰（读缓存）」排序。

#### P0-验证（半天）：证实 R2

- 对本地系统库执行一次 `CALL ducklake_merge_adjacent_files('lake')`（只针对 `sys_log_events` / `sys_metric_samples`），再对比 `/logs?limit=200`、`metrics/summary` 冷/热耗时与文件数。
- 同时打开 `observability.perf_stage_timing`（已是 `true`）读取各阶段耗时，确认时间花在「等连接」还是「执行」。
- 判定：合并后若 `/logs` 冷态 < 1s → R2 成立，继续 P1；否则回头查 R1/R3。

#### P1-A（止血）：收敛日志写入量（R4）

- `router.go` 日志中间件改为**选择性记录**：
  - 不记录：`/health/*`、`/metrics`、静态路由 `/*`、`/v1/projects/:id/logs*` 自身、以及 status < 400 且耗时 < 500ms 的普通 GET；
  - 保留：status ≥ 400、耗时 ≥ 500ms（慢请求）、所有写方法（POST/PUT/PATCH/DELETE）；
  - **指标不变**（仍走 `RecordMetric` 内存预聚合，本身很省）。
- 不新增配置项（AGENTS 硬性约束 4/「不改配置文件结构」）；规则写成中间件内的小函数 + 单测。
- 副作用：日志页从「全量 access log」变成「关键事件日志」，需在 `docs/` 日志说明里同步一句。

#### P1-B（止血）：放慢并合并刷写（R2/R3）

- `config.yaml` / `config.example.yaml` 的 `system_database.log_flush_interval`、`metrics_flush_interval` 默认值 **2s → 15s**（只改默认值，不改结构）；`app.go:538` 传参不变。
- `flushAllAsync` 已把日志/指标放在同一个后台循环里，进一步做到**同一轮只触发一次 `notifyWrite`**（现状：日志一次、指标一次、聚合一次，最多 3 次 → 3 次 snapshot + 3 次 catalog 同步触发）。改为收集三批后在一个事务提交、末尾 `notifyWrite` 一次。
- 收益：小文件产生速率 ↓ 约 7–20×，catalog 上传频率同量级下降；`snapshot moved during copy` 重试也随之消失。
- 代价：进程崩溃最多丢失 15s 缓冲（`Close` 已有兜底 flush，正常退出不丢）。日志是运维诊断数据，可接受。

#### P1-C（治本）：把 maintenance 配置落地（R2）

新增系统库维护循环（放在 `systemdb`，由 `app` 装配、`Close` 反向释放）：

1. **启动后 60s 一次** + **每 `maintenance.checkpoint_interval`（1h）一次**：
   `ducklake_merge_adjacent_files` → `ducklake_expire_snapshots(older_than => expire_older_than)` → `ducklake_cleanup_old_files(older_than => delete_older_than)`。
2. 仅在 `writable` 且**持有系统库写权**时执行（单写者；非 writer 实例直接跳过）。
3. 走系统库唯一连接，但**分批 + 每步间让出**（`max_compacted_files` 限量），避免一次占连接过久；执行期间记 `stage_timing`。
4. **必须先读 `multi-instance-consistency-plan §3.2`**：`cleanup_old_files` / `delete_orphaned_files` 基于单边视图删除有丢数据风险，因此：
   - 只做 `cleanup_old_files`（只清「已被 catalog 标记待删除」的文件），**不**做 `delete_orphaned_files`；
   - `older_than` 用已配置的 7d，且经 `orphan_ledger`（`reason: expire_snapshots`）留账；
   - 先 `dry_run` 记录日志一轮再放开真正删除（可用一个短期开关，验证后移除）。
5. 用户库的 compaction 不在本次范围（用户库有自己的写者生命周期，另立计划）；本次只处理系统库。

#### P1-D（削峰）：读路径

1. `MetricsSummary`：6 条独立聚合合并成 **1 条条件聚合**（`SUM(CASE WHEN name=... THEN value_double END)`），扫描次数 6 → 1。`MetricsTrend` 2 条按需合并。
2. `metrics/summary`、`metrics/trend`、`quota` 加**进程内短 TTL 缓存**（10–15s）+ `singleflight`，项目维度键；仪表盘刷新不再逐次打 COS。写入路径无需失效（指标本身是近似值）。
3. `QueryLogs`：`q.From` 为空时后端默认取 `now - 24h`（不超过 retention），让 DuckLake 有机会按 `occurred_at` 做文件剪枝；前端日志页默认也带 `from`。`limit` 上限保持 500。
4. 前端：日志页首屏并发 `logs` + `logs/retention` → 保留并发，但 `retention` 结果可缓存（每项目一次），不需每次进入页面都取。

#### P2（评估，本次不做）

- **系统库读写连接分离**：受 `factory.go:150-153` 结论限制（第二连接会丢 ATTACH/`search_path`/配置被锁），要做需改 boot 序列，风险高；待 P1 完成后看 `wait_total` 增速再决定。
- **catalog 同步脱离业务连接**（用独立只读连接或直接对 SQLite 文件做快照）：可根治 R3，但涉及 syncer 重构与新连接语义，另立计划。
- **系统库专用同步节奏**（如 5–10s debounce，系统库数据可重放性强）：需要 `Factory` 支持按库覆盖 `Options`，先看 P1-B 收益。

### 1.5 独立问题：KV 写路径 45s 卡顿 + 500（非系统库问题）

- 现象：09:39:57 `POST /kv` 45.9s 成功；09:40:19 `POST /kv` 45.5s 后 **500**，错误为 `write lease held by another instance … owner=simplebase-dev-1/85487/…`——owner 的 **PID 就是当前进程**。`kv sweeper` 同库随后同样报错。
- 时间线：两个请求区间 09:39:12–09:39:57 与 09:39:34–09:40:19 **重叠**。`lease.go:196-205`：发现「他人持租」时**无条件 sleep `TTL + Grace`（≈45s）** 再探测；请求 B 醒来时看到 epoch 已被同进程的请求 A 推进（A 已获取并开始续约），于是判定 `ErrLeaseHeld`。
- 结论（高置信推断）：`WriteGate`（`app.go:477-504`）**对同一 DB 没有串行化**，同进程并发首写会各自 `Acquire`，互相把对方当「他人」；此外重启后首次写（残留旧租约）必然经历 45s 观察等待。
- 方案（P1-E，独立 commit）：
  1. `WriteGate` 内按 `db.ID` 加 `singleflight`/per-DB 互斥，同进程只允许一次 `Acquire`；
  2. `Acquire` 探测到 `cur.OwnerID` 与本进程 `instanceID+pid` 前缀一致但 bootID 不同（同 PID 重启）时明确日志说明；
  3. 观察等待期间返回**可识别的「正在获取写租约」状态**（HTTP 503 + Retry-After，或 UI 提示），不要让请求挂 45s；
  4. 补并发用例：两个 goroutine 同时首写同一 DB → 只有一个 `Acquire`，另一个复用结果。
- 需要再复现一次确认（重启进程 → 立刻并发发两个 KV 写），不满足则退回方案 3 仅做体验层修补。

### 1.6 验收指标

| 指标 | 现状 | 目标 |
| --- | --- | --- |
| `sys_log_events` 存活文件数 | 150（20 分钟内） | 合并后 ≤ 5，稳态增速 ≤ 4 个/小时 |
| `sys_metric_samples` 存活文件数 | 116 | 同上 |
| 登录后 9 个并发请求最慢一个 | 29s | < 3s（冷）/ < 500ms（热） |
| `GET /logs?limit=200` | 27s 冷 / 0.46s 热 / 5.6s | 冷 < 1s，热 < 200ms |
| `logs/retention` | 27s（被前序占用） | < 100ms |
| `catalog synced` 频率（空闲） | 每 ~2s | ≤ 每 15s |
| `wait_total` 增速（空闲） | ≈ 0.4s / 2s | < 0.05s / 15s |
| `snapshot moved during copy` | 5 次 / 20 分钟 | 0 |
| KV 首次写 | 45.9s；并发首写 500 | 并发首写不再 500；等待期有明确状态 |

测试要求（`internal/AGENTS.md`）：新增/改动均补单测——日志选择性记录规则、`flushAllAsync` 单次 `notifyWrite`、maintenance 循环（fake 执行器，非 writer 跳过、dry-run、错误不 panic、`Close` 可中断）、`MetricsSummary` 合并查询与旧口径结果一致、`WriteGate` 并发用例；`go build ./internal/... ./cmd/...` 与 `go test ./...` 通过；不触碰系统库写保护约束。

---

## 2. 需求 2：「操作」列按钮统一不带 icon

### 2.1 现状盘点（逐个「操作」列核对）

| 位置 | 现状 | 动作 |
| --- | --- | --- |
| `pages/Databases.vue` 桌面表格 L144–196 | SQL（`CodeIcon`）、新建集合（`PlusIcon`）、删除（`Trash2Icon`）带 icon；三者文字都是 `hidden lg:inline`（窄屏只剩 icon）；「受保护」带 `ShieldCheckIcon` | 去 icon、去 `hidden lg:inline`（否则文字在 <lg 时会消失），「受保护」去 icon |
| `pages/Databases.vue` 移动端卡片 L52–59 | 按钮纯文字；「受保护」带 `ShieldCheckIcon` | 「受保护」去 icon，与桌面一致 |
| `pages/CronJobs.vue` L123–147 | 立即执行（`PlayIcon`）、记录（`HistoryIcon`）、编辑（`PencilIcon`）、删除（`Trash2Icon`） | 全部去 icon |
| `pages/S3Manager.vue` L68–79 | 打开（`EyeIcon`）、删除（`Trash2Icon`） | 去 icon |
| `KvPanel.vue` L70–83 / `CollectionPanel.vue` L22–27 / `KvApiPanel.vue` L40–69 | 已是纯文字 | 不动 |
| `GoFunctions.vue` / `Users.vue` / `AgentManager.vue` / `DocumentListModal.vue` / KV 三个 Editor | 已是纯文字 | 不动 |

### 2.2 实现要点

- 只删除按钮内的 `<XxxIcon data-icon="inline-start" />`，同步清理 `<script>` 里不再使用的 icon import（`Databases.vue` 中 `PlusIcon`/`MinusIcon`/`DatabaseIcon`/`RefreshCwIcon`/`ShieldCheckIcon` 是否仍被别处使用需逐个确认后再删；`CronJobs.vue` 的 `PlusIcon`/`RefreshCwIcon`/`AlertTriangleIcon` 仍被表头/目标列使用，保留）。
- Databases 桌面表格里 `Tooltip` 的保留原则：只保留**提供额外信息**的（「数据库未就绪」「系统库：删除与写入已被服务端拒绝」）；仅重复按钮文字的 Tooltip（「删除」）去掉，避免多余包装。
- 列宽：去 icon 后按钮变窄，`CronJobs` 的 `sb-col-act min-w-32` 需要容纳 4 个纯文字按钮，改为 `min-w-56` 并保持 `flex-wrap`；`Databases` `min-w-36` 视实际截图微调。
- **范围边界**：表头工具栏的「刷新 / 新建 / 上传」按钮、展开行的 +/− 图标、状态图标（`AlertTriangleIcon` 目标缺失、`FileIcon`、`DatabaseIcon` 名称前缀）**不属于「操作列」，保持不变**。
- 规范落盘：`ui/AGENTS.md` 「样式与 UI 约定」新增一条——「表格『操作』列按钮一律纯文字，不加 icon」，并修正现有「按钮内图标加 `data-icon`」条款的适用范围（限工具栏/表单按钮）。
- 测试：`ui/tests` 中未发现对操作列 icon 的断言（grep `data-icon|Icon` 无命中），预期无需改测试；仍需跑 `yarn test` 确认。

---

## 3. 需求 3：连接页两个 Key 合并为「API Key」

### 3.1 现状问题

`ConnectionPanel.vue` 里同时存在两套「Key」概念，标题/位置/交互都不同：

1. 顶部「API Key」输入框（L13–24）+ 「保存 / 恢复默认（DevMode）」——**当前浏览器请求使用的 Key**（写 localStorage）；
2. 下方「项目 API Keys」+「签发新 Key」+ 列表 + 吊销（L36–59）——**服务端已签发的 Key 记录**（明文只在签发时出现一次）。

用户面对的是两处「Key」，且签发出来的 Key 需要**手动复制再粘贴到上面的输入框**才生效，动线割裂。

### 3.2 方案：一个「API Key」区块，三层信息

```text
API Key
├─ [输入框：当前使用的 Key（password）]            [保存] [恢复默认（DevMode）]
│   说明（FieldDescription，一句话）：DevMode 种子 Key sb_live_dev_key_12345；生产环境请签发
├─ 已签发（N）                                      [签发新 Key]
│   └─ 列表：id · 权限                               [吊销]
└─ （签发后一次性提示）新 Key：sb_live_xxx           [复制] [设为当前使用]
```

- 标题统一叫「API Key」，去掉「项目 API Keys」这个第二标题；已签发列表作为同一区块的子内容，副标题「已签发（N）」。
- **签发后的一次性提示**新增「设为当前使用」按钮：点击 = `key.value = secret` + `authStore.updateKey(secret)`，消除手动复制粘贴。**不自动替换**当前 Key（新 Key 权限可能小于现用 Key，静默替换会让后续请求 403）。
- DevMode 说明段落（现 L30–34 的长段落）压缩为输入框下方的 `FieldDescription`；`Separator` 从 3 个减为 0–1 个。
- 逻辑不变：`saveKey/resetKey/loadKeys/issueKey/revokeKey`、`canIssue`、401 `Alert` 保留；只调整模板结构，新增 `useIssued()` 一个方法。
- 测试：`ConnectionPanel.test.ts`、`components-coverage.test.ts`（ConnectionPanel 部分）按新结构调整选择器，新增「设为当前使用」用例。

---

## 4. 需求 4：连接页太宽，输入框输入时被遮挡

### 4.1 根因

- `SettingsModal.vue` 内容容器：`max-h-[min(80vh,720px)] overflow-y-auto pt-4`。CSS 中只要 `overflow-y` 不是 `visible`，`overflow-x` 会被强制计算为 `auto`，于是子元素**超出容器盒模型的部分被裁剪**。
- `Input.vue` 的聚焦态是 `focus-visible:ring-3`（`box-shadow` 光环，画在输入框**外侧**）。`ConnectionPanel` 的 `FieldGroup` 占满容器 100% 宽度、容器又没有水平 padding，输入框贴着容器边缘，聚焦时左右光环（及部分边框）被裁掉——即用户看到的「输入时部分被遮挡」。
- `Modal` 宽度 `max-width=900` 对连接页太宽：一个 API Key 输入框被拉到 ~850px，视觉上失衡（`SettingsPanel` 的模型页已限制 `max-w-md/lg`，连接页没有）。

> 该根因来自源码推断（Input 聚焦样式 + 容器 overflow），改完后必须在浏览器里实测聚焦左右边缘。

### 4.2 方案

1. `SettingsModal.vue`：滚动容器加 `px-1 -mx-1`（给光环留出空间，同时不改变视觉对齐）。
2. `ConnectionPanel.vue`：根容器加 `max-w-lg`（与模型页 `max-w-lg` 一致）；「当前项目」输入、API Key 输入都不再撑满 900px。
3. Modal `maxWidth`：`900 → 720`。供应商页 2 列卡片在 720px 下每列约 330px，可容纳（现卡片只有名称/协议/Badge/Key 掩码）；实现时用 720/1280 两档视口截图确认无换行溢出，若供应商页不满意再回退到 840。**全局固定一个宽度**，不做「按 tab 变宽」（会造成切 tab 时弹窗跳动）。
4. 顺手检查：`ConnectionPanel` 下方的已签发列表 `li` 使用 `truncate`，父级需 `min-w-0`（已有），确认窄宽度下不横向溢出。

---

## 5. 需求 5：主题切换移到顶栏，去掉设置里的「外观」tab

### 5.1 范围界定（含一个假设）

「去除 SettingsPanel 这里的 tab」在 `SettingsPanel.vue` 里对应的是 `section === 'appearance'`（主题）——本计划**只去掉「外观」tab**，「连接 / 模型 / 供应商」三个 tab 保留（模型与供应商仍是项目 LLM 默认值与本地 Key 的唯一入口）。若你的意思是去掉全部 tab，请在评审时指出，见 §7.3。

### 5.2 顶栏太阳/月亮按钮

- 位置：`DefaultLayout.vue` L71–98 的图标组（`rounded-lg border bg-muted/30` 那一组），顺序调整为 **主题 → 设置 → 刷新**（在 `ui/AGENTS.md` 的「右上角顺序」条款同步改：使用文档 → 项目切换器 → 主题 → 设置 → 刷新）。
- 交互：单个 `ghost/icon` 按钮 + `Tooltip`，图标为**当前状态**（浅色显示 `SunIcon`，深色显示 `MoonIcon`），点击切换到另一个；`aria-label` = 「切换为深色主题 / 切换为浅色主题」。
- 状态来源：`useSettingsStore().effectiveTheme`（已有 getter，`system` 会解析为实际明暗）；点击调用 `settings.setTheme(effective === 'dark' ? 'light' : 'dark')`。
- **`system` 模式的处理**：UI 不再提供「跟随系统」入口（sun/moon 二态）；`store` 仍保留三态类型与 `App.vue` 的 `matchMedia` 监听，兼容 localStorage 里已存的 `system` 值——首次点击后变为显式 light/dark。不动 `stores/settings.ts` 的存储结构与 `settings.test.ts`。
- 移动端：顶栏图标组是 `hidden sm:flex`，侧栏底部 `sm:hidden` 列表（L18–28）新增一项「切换主题」，保持与设置/刷新同样的 `min-h-11` 样式。

### 5.3 移除「外观」tab

| 文件 | 改动 |
| --- | --- |
| `components/settings/SettingsPanel.vue` | 删除 `section === 'appearance'` 模板块、`onTheme`、`ToggleGroup*` / `FieldTitle` / `ThemeMode` 的 import；`section` 联合类型收窄为 `'models' \| 'providers'` |
| `components/SettingsModal.vue` | 删「外观」`TabsTrigger` / `TabsContent`；`onTab` 白名单去掉 `appearance`；副标题「主题、连接、默认模型与厂商 API Key」→「连接、默认模型与厂商 API Key」 |
| `stores/auth.ts` | `SettingsTab = 'connection' \| 'models' \| 'providers'` |
| `layouts/DefaultLayout.vue` | `SETTINGS_TABS` 去掉 `appearance`；**deep-link 兼容**：`?settings=appearance`（旧书签）不再匹配 tab，走已有的「无 tab → 只打开弹窗」分支（落到默认 `connection`），不报错 |
| `ui/AGENTS.md` | 「全局设置」条款、「右上角顺序」条款同步 |

- `components/ui/toggle-group`：`SettingsPanel` 不再引用后，需**全局确认是否仍有其他引用**才决定保留（`ui/AGENTS.md` 规定删除 ui 组件前必须确认零引用；若零引用则一并删除并同步桶导出，否则保留）。
- 测试调整：`auth.test.ts`（用 `appearance` 做 tab 例子 → 改 `models`）、`SettingsModal.test.ts`（tab 切换序列与「外观」断言）、`SettingsPanel.test.ts`（删 appearance 用例）、`DefaultLayout.test.ts`（`?settings=appearance` 用例改为「回退到 connection」；新增主题按钮点击/图标切换/`system` 首次点击用例）、`router.test.ts` 仅测重定向，不变。覆盖率阈值 95% 需保持。

---

## 6. 需求 6：运行记录改为统一模态框

### 6.1 现状

- `CronJobRunsDrawer.vue` 使用 `Sheet`（抽屉）；页面里「立即执行」`trigger()` 是**页面自己调 API，成功后 `openRuns()` 再开抽屉**，而抽屉内部另有一套 `trigger()` + 1.5s 轮询直到 running 收敛。
- 后果：从列表点「立即执行」→ 抽屉打开时只 `load()` 一次，若此时记录仍是 `running`，**不会轮询**，需要用户手点「刷新」；两条触发链路行为不一致。

### 6.2 方案

1. **组件**：`components/CronJobRunsDrawer.vue` → 迁移为 `components/modal/CronJobRunsModal.vue`（与其他业务弹窗同目录），外壳换成 `SbModal`：`title="运行记录 · {name}"`、`description` 沿用「funcFile.funcExport · 已执行 N 次」、`:hide-footer="true"`、`maxWidth={720}`；内容区（工具栏：立即执行 / 刷新 / 状态筛选；运行卡片列表：状态、耗时、返回值、错误、复制）**原样保留**，仅去掉 `Sheet*` 与抽屉专有的 `w-full sm:max-w-lg`、`border-b` 头部样式；列表区 `max-h-[60vh] overflow-y-auto`。
2. **统一入口**：页面两个按钮都只是「开模态框」：
   - 「记录」→ `openRuns(record)`（`autoTrigger=false`）；
   - 「立即执行」→ `openRuns(record, { trigger: true })`：模态框打开后**自行**调用内部 `trigger()`（API + 轮询到 `running` 收敛），页面不再自己调 `api.cronjobs.trigger`，`triggering` Set 与页面 `trigger()` 一并删除，按钮的 `disabled` 改由模态框内部 `triggering` 状态承担（页面侧不再需要）。
   - 新增 prop `autoTrigger?: boolean`；`watch(open)` 中：先 `load()`，若 `autoTrigger` 则接着 `trigger()`；关闭时清理轮询（用 `open` 标志打断 `for` 循环，避免关闭后继续请求）。
3. **`triggered` 事件**保持：触发成功后 emit，页面 `load()` 刷新列表上的「上次状态」快照。
4. **抽屉资产清理**：`components/ui/sheet` 仍被 `KvDetailSheet` 与 `Sidebar` 使用，**保留**；只删除 `CronJobRunsDrawer.vue`。
5. **引用/文档同步**：
   - 测试改名与更新：`tests/CronJobRunsDrawer.test.ts` → `CronJobRunsModal.test.ts`；`CronJobs.test.ts`、`coverage-gaps.test.ts`、`components-coverage.test.ts`、`components-interactions.test.ts`（其中「sheet close」用例改为 modal close）、`pages-coverage.test.ts` 的 stub 名称同步；新增「autoTrigger 打开即触发并轮询」「关闭后停止轮询」用例；
   - `docs/cronjob/overview.md` L38「运行记录抽屉」→「运行记录弹窗」；
   - `plan/planv2.0/*` 中的历史提及不改（历史文档）。

---

## 7. 汇总

### 7.0 执行状态（2026-09-30 回写）

| 项 | 状态 | 产出 |
| --- | --- | --- |
| §2 操作列去 icon | ✅ | Databases / CronJobs / S3Manager 纯文字化；`ui/AGENTS.md` 已加规范 |
| §3 API Key 合并 | ✅ | ConnectionPanel 单区块 + `useIssued()` 设为当前使用；测试补齐 |
| §4 弹窗宽度/光环 | ✅ | Modal 900→720；滚动容器 `px-1 -mx-1`；ConnectionPanel `max-w-lg` |
| §5 主题移顶栏 | ✅ | DefaultLayout 太阳/月亮按钮；SettingsPanel 删外观 tab；deep-link 兼容 |
| §6 运行记录模态框 | ✅ | `CronJobRunsModal`（SbModal）；autoTrigger 统一入口；Drawer 已删 |
| §1 P1-A 选择性日志 | ✅ | `router.go` `shouldRecordRequestLog`；docs/ops/deployment.md 已说明 |
| §1 P1-B 刷写合批 | ✅ | 默认 2s→15s；`async_flush.go` 三批一事务 + 单次 `notifyWrite` |
| §1 P1-C maintenance | ✅ | `systemdb/maintenance.go` 启动 60s + 周期 merge/expire/cleanup（dry-run 首轮） |
| §1 P1-D 读路径削峰 | ✅ | `query_cache.go` 短 TTL 单飞；MetricsSummary 合并单查询；summary/trend/quota 走缓存；QueryLogs 默认 24h；前端 Logs 页默认 from + retention 每项目取一次 |
| §1.5 P1-E 写租约 | ✅ | `lease/gate.go`（Do/TryDo/Clear）；WriteGate 按库 TryDo 串行化；`ErrAcquiring`→503+Retry-After；同 PID 重启提示日志 |

验收：`go build ./internal/... ./cmd/...` 通过；`go test ./internal/...` 全绿；`yarn test` 401/401 通过。遗留见 §7.5。

### 7.1 提交拆分（每项独立可 revert）

| 顺序 | commit | 内容 |
| --- | --- | --- |
| 1 | `ui: 操作列按钮去 icon` | §2 |
| 2 | `ui: 设置弹窗宽度与聚焦光环裁剪修复` | §4 |
| 3 | `ui: 连接页 API Key 合并` | §3 |
| 4 | `ui: 主题切换移至顶栏，移除外观 tab` | §5 |
| 5 | `ui: 定时任务运行记录改为模态框` | §6 |
| 6 | `perf(systemdb): 验证 + 日志选择性记录 + 刷写节流合批` | §1 P0-验证 / P1-A / P1-B |
| 7 | `perf(systemdb): 系统库 maintenance（merge / expire / cleanup）` | §1 P1-C |
| 8 | `perf(systemdb): 指标查询合并与短 TTL 缓存、日志默认时间窗` | §1 P1-D |
| 9 | `fix(lease): WriteGate 按库串行化 + 等待期状态` | §1.5 |

§2–§6 与 §1 互不依赖，可并行。§3/§4/§5 都改 `ConnectionPanel` / `SettingsModal`，按上表顺序提交避免冲突。

### 7.2 每步必须通过

- 前端：`cd ui && yarn build && yarn test`（覆盖率 95% 阈值），样式遵守 `ui/AGENTS.md`（`interface` 不用 `type`、Tailwind class、`SbModal`、`vue-sonner`）。**注意**：`SettingsTab` 是 `type` 别名的既有写法，仅收窄成员，不属于本次新增。
- 后端：`go build ./internal/... ./cmd/...`、`go test ./...`；不改 `go.mod`；不新增配置结构；日志/审计不落 SQL 参数与 LLM 正文。
- 视觉：§2/§4/§5/§6 各用桌面（1280）与窄屏（<640）各截一次图确认（尤其 Databases 操作列文字不再被 `hidden lg:inline` 隐藏、连接页聚焦光环完整）。

### 7.3 需要你在评审时确认的点

1. **§5.1**：仅移除「外观」tab（保留 连接/模型/供应商）——是否符合预期？
2. **§5.2**：主题只保留「浅色 ↔ 深色」二态，不再提供「跟随系统」入口（历史 `system` 值仍兼容）——是否接受？
3. **§3.2**：签发后「设为当前使用」需手动点击，而不是自动替换当前 Key——是否接受？
4. **§1 P1-A**：日志页从「全量 access log」变为「关键事件（≥400、慢请求、写操作）」——是否接受？（指标口径不变）
5. **§1 P1-B**：日志/指标刷写间隔 2s → 15s，崩溃最多丢 15s 缓冲——是否接受？
6. **§1 P1-C**：系统库 compaction 会真实删除 COS 上已被标记淘汰的旧 parquet/快照，首轮以 dry-run 观察——是否同意由我先落地 dry-run 版本？

### 7.4 风险与回退

| 风险 | 缓解 |
| --- | --- |
| compaction 误删 S3 对象 | 仅 `cleanup_old_files`（不做 orphan 清理）、`older_than=7d`、首轮 dry-run、经 orphan_ledger 留账；只在 writer 且持系统库写权时运行 |
| 合并期间占住系统库唯一连接，反而更慢 | 限量分批 + 步间让出；首次全量合并放在启动后低峰窗口；记 stage_timing 观察 |
| 选择性日志导致排障线索变少 | 保留 ≥400 / 慢请求 / 写操作；指标全量保留；ZAP 进程日志（`http` 行）本身完整未变 |
| 去 icon 后窄屏按钮换行/溢出 | §2.2 调整列宽 + `flex-wrap`，截图验收 |
| 旧书签 `?settings=appearance` | §5.3 已定义回退行为并补用例 |
| 弹窗宽度收窄影响供应商页 | §4.2 第 3 点双视口验证，必要时回退 840 |

### 7.5 遗留与后续

1. **前端全局覆盖率阈值未达**：`yarn test --coverage` 报 functions 93.5% / branches 91.7%（阈值 95%）。缺口文件（KvPanel 79%、SbModal 75%、Dashboard 67%、DataTabs 75% 等）**均为本任务之前已存在的组件**，本任务新增/改动的文件函数覆盖均为 100%（ConnectionPanel/Logs.vue/auth.ts 等）。HEAD 基线本身同样不达标（docs-catalog 断言与 `_meta.json` 在 HEAD 即不一致）。建议另立测试补齐计划，不阻塞本 plan 验收。
2. **§1.5 复现验证**：P1-E 的并发首写修复建议按 plan 原文做一次实测复现（重启进程 → 立刻并发两个 KV 写 → 观察是否 503+Retry-After 而非 45s 挂等），验证后移除观察。
3. **P1-C dry-run 解除**：maintenance 首轮 dry-run 观察一轮（确认 orphan_ledger 记账与文件数下降）后放开真实删除。
4. **验收指标（§1.6）**：需在真实环境（COS 广州）跑一轮对比，观测 `wait_total` 增速、`sys_log_events` 存活文件数、`snapshot moved during copy` 次数。
