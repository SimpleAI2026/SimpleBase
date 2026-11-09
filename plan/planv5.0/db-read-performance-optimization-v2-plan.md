# SimpleBase v5.0 数据库读取性能优化计划 2.0

> 日期：2026-10-08。状态：**待实施；本轮只落地可复现的 `benchmarks/dbread` 测试与此计划，不修改生产代码**。
> 来源：用户指定的 `simplebase-dev-logs.VsFgor/backend.log`（2026-10-08 14:27–14:31，223 行）；本地系统库 DuckDB catalog 的**只读副本**（原文件修改时间 **2026-10-02 22:22:43**，并非本次日志同期）；`benchmarks/dbread` 本地复测。前序：`plan/planv4.0/console-polish-and-systemdb-perf-plan.md` §1 与 `ducklake-rw-latency-eventual-consistency-plan.md` §3 P2；`plan/planv3.0/api-db-perf-validation-plan.md` §7。与 v5.0 既有代码/样式计划并列，不覆盖原计划。

## 0. 结论与边界

**已证实的现象**：多个接口慢在认证或尚未细分的 handler 阶段；较早的本地 catalog 副本存在大量小文件，本地对照基准能复现随文件数增长的读取开销；日志出现一次 DuckLake 同步写冲突。**待证实的机制**：本次远端读取是否因小文件变慢、同步/刷写是否与请求争用连接或 DuckDB 事务、各机制占比如何。不能将「两个问题叠加并导致排队」当成已定位根因。当前代码 `internal/database/ducklake/factory.go:168-169` 的 `SetMaxOpenConns(10)` / `SetMaxIdleConns(2)`，**v4 文档所称当前系统库只有 1 条连接已经过时**；本轮日志没有 `systemdb connection wait accumulated`，既不能证明没有等待，也不能证明必然排队。此外 `stage_timing` 对 systemdb/auth 等直连库调用没有 db_conn_queue/db_exec 段，其 `unattributed` 不是「实际 SQL 执行」的同义词。不能仅凭 API 耗时或单个阶段推断真实排队时间。

**需要优先验证的代码级瓶颈：JWT Principal 缓存的全局锁。** `internal/auth/session.go:281-306` 中 `PrincipalFromClaims` 从进入函数就获取 `principalMu`，且在锁内执行 `users.GetByID`、`users.PrincipalFromUser`（后者再查项目集）；缓存每 5s 过期一次。某个用户未命中时，**所有用户的认证请求都等待同一把锁**，所以不同路由同一波 `auth=1981ms` 或 `auth≈2516ms` 极可能是一次慢查询造成的锁队列；此处是高置信**静态竞争缺陷**，但本轮没有互斥锁等待 profiler，尚不能确认每个 1–2.5s 样本都来自该锁。下一步应在补齐锁等待/未命中/系统库查询分段后，优先改「仅短暂持锁查缓存，未命中在锁外查库 + 按用户单飞/版本化失效」，而非先改文件合并或连接池。失效与回填竞态必须覆盖禁用用户、改角色、项目权限变更后旧请求不能复活旧 Principal。

明确保留：系统库 API 写保护、单写实例/租约语义、catalog 恢复协议、数据一致性；禁止未经压测直接调大连接池、移除鉴权或删减数据文件。此次不连接生产 COS、不重置缓存、不改用户数据。

## 1. 本次日志的实测证据（时长单位：秒，`perf_stage.stages` 单位毫秒）

| 请求 / 时段 | 总耗时 | 阶段分解 | 结论 |
| --- | ---: | --- | --- |
| `POST /v1/auth/refresh`，14:28:44 | **7.712** | `unattributed=7712` | refresh 免 auth 中间件，时间在会话 repo 查找、用户查找、更新+签发；需细分，不能断言单条 SQL 慢。 |
| `GET /databases`，14:28:46 | 2.239 | `auth=1981, project_resolve=242, catalog_lookup=13` | 绝大多数在认证；**catalog 列表查询阶段为 13ms**，不能将其等同整个 handler。14:28:59 再次请求总计约 5.7ms（`catalog_lookup=5ms`）。 |
| `GET /projects`，14:28:46 / 14:31:45 | 2.333 / 2.530 | `auth=1981/2516` | 权限中间件耗时远超项目枚举本身。 |
| `GET /s3/objects`、`GET /gofunctions`，14:31:45 | 2.534 / 2.543 | 二者 `auth=2516/2517`，同时结束 | **同一并发波次的认证阶段耗时高度同步**；需要量化 SQL wait 与执行，不能简单归咎 handler。 |
| `GET /agents`，14:28:50 / 14:30:25 / 14:31:48 | 5.772 / 3.464 / 5.162 | 分别 `unattributed=3553/3464/2645` | 业务层不止普通 SELECT：可写实例的 `ListAgents` 先调用 `ensureAgents`；每个未跳过的内置 Agent 有两次 COUNT，若缺失还会 UPDATE/INSERT，然后列表查询。应先测读/写命中次数，再评估削减往返。 |
| `GET /auth/me`，14:28:49 / 14:31:48 | 1.870 / 2.608 | `auth=0, unattributed≈total` | 即便认证命中缓存，handler 再次 `GetByID(sys_users)`；固定每请求一次额外点查。 |
| `GET /users`，14:30:59 | 5.604 | `auth=2213, unattributed=3390` | `UsersHandler.List` 列表后逐用户 `ProjectIDsFor`；管理角色查全部 `sys_projects`，普通用户查自己的 `sys_project_owners`。存在按用户循环查询的 N+1 结构，但本次是否为主因仍需测每段时间/用户数。 |
| `GET /cron-jobs/:id/runs`，14:30:09 / 14:30:10 / 14:30:17 | 1.590 / 0.784 / 0.019 | auth 全 0，unattributed=1590/784/19 | 冷热差异显著，属于未细分的 systemdb 查询；仅靠总耗时无法判断是文件扫描或资源等待。 |
| `GET /logs`，14:30:48 / 14:31:20 | 3.048 / 3.162 | auth=1604/2489，余 1443/672 | 日志页实际读路径仍慢；应拆出 `QueryLogs` 查询与扫描时间。 |
| `GET /metrics/summary`，14:31:22 | 1.539 | auth=0，unattributed=1538 | 该次慢请求可能是缓存未命中、同 key 单飞等待或其他未分段环节；缓存未命中时 summary 做条件聚合、活跃库计数、直方图读取三次查询。 |
| `POST /kv`（有一次 14:29:30） | 1.760 | `auth=1737, db_exec=12` | KV 写的这一例瓶颈仍是认证，不是实际 DB 执行。 |

**异常**：14:30:30 系统库同步时报 `ducklake: flush inlined data: Transaction conflict - attempting to delete from table with index "22" - but another transaction has inserted into it`，两条 WARN 属于**同一次同步失败**（外层重复告警），随后 14:30:43 同步成功。只读 catalog 副本的 `ducklake_table.table_id=22` 对应 `sys_cloud_agents`。同一时段 `/agents` 接口的 `ensureAgents` 可能写此表，但**尚无 request_id/事务级关联**；优先在压测中构造相同并发竞争并取证，不宜未经验证直接加重试。

**注意计量偏差**：日志的 `stages` 被 `Duration.Milliseconds()` 截断，<1ms 显示 0；不要误读为完全无开销。部分请求时段有 `401`，只是刷新前未授权，不混入已授权读取延迟分位数。日志不足 5 分钟，每路由样本少，不能据此宣称稳定 P99。

## 1A. P1.1/P1.3 实施后验证与新证据（2026-10-08 20:03–20:07，backend.log 194 行）

**P1.1（锁外 I/O + 单飞 + 代数失效）验证通过**：

- 全部样本 `auth_lock_wait=0–0.009ms`（改前同波次 `auth_lock_wait≈1849ms`），锁 convoy 消除；
- 同波次多请求 `auth` 耗时高度一致（如 20:03:35 波次 8 个请求 auth≈1847ms），为同用户 follower 等待 owner 单飞，系统库只执行一次真实查询；
- 缓存命中请求 auth=0.02–0.08ms（`auth_cache:1`），`/auth/me` 命中 0.03–0.06ms（`me_cache:1`），P1.3 生效；
- systemdb 连接池全程 `wait_count_total=0`，无池拥塞。

**新瓶颈定位（按耗时排序）**：

| 瓶颈 | 证据 | 归因 |
| --- | --- | --- |
| `auth_user_load` 1.43–2.40s | 波次 20:03:35/20:03:50/20:04:11/20:04:25/20:04:34/20:04:42/20:04:51，间隔 15–60s 恰为 5s TTL 过期后前端重触发；同波次 `auth_project_expand` 仅 5–8ms | 慢在 `sys_users` 点查本身：远端 COS 小文件读放大 |
| `POST /auth/refresh` 8.0s | `auth_session_load=3354ms`（`sys_user_sessions` 点查）+ 4.6s unattributed（更新会话+签发未细分） | 同机制；需补 refresh 细分计时 |
| `/metrics/summary` 冷 2.65s→热 0.587s；`/metrics/trend` 冷 2.42s；`/logs` 1.05s（200 行） | 10s queryCache 未命中时全价查询 | P1.5 范围 |
| `/agents` unattributed 2.0–3.2s | ensureAgents COUNT 往返 + 列表查询 | P1.2 范围 |

**P2 证据门槛达成（当前 catalog 同口径基线，2026-10-08 20:26 本地副本拷贝离线统计）**：

- 快照总数 **1368**（10-02 旧副本为 415）；全库存活文件 **86 个 / 326 KB**（旧副本 228 个）。
- 与旧副本对比，既有 merge 对高频批量表效果显著：`sys_log_events` 42→**4**、`sys_metric_samples` 41→**4**、`sys_operations` 38→**1**、`sys_api_keys` 23→**1**。
- 但认证路径热点表**反向增长**：`sys_project_owners` **19 文件**、`sys_user_sessions` **18 文件**（旧 12）、`sys_users` **9 文件**（旧 1）。写入源为 refresh 轮转 UPDATE session、项目归属变更等频繁小事务；这些表正是每次 5s TTL 过期波次必读的表。
- 本地热态点查基准 1.4ms vs 远端实测 1.43–2.40s，与「每存活文件一次远端 HTTP 往返」模型一致（本地 35µs/文件线性项在远端放大为数百 ms/文件级 RTT）。

**下一步优先级修订**：

1. P1.4 `/v1/users` 批量化与 P1.5 QueryLogs/MetricsSummary 优化照计划推进；
2. P2 解禁：auth 热点表（`sys_user_sessions`/`sys_project_owners`/`sys_users`）按表 merge、refresh 轮转写放大量化、`POST /auth/refresh` 补细分计时；
3. 可评估延长 principal 缓存 TTL（5s→30s 级）：`RevokeAllForUser`/`InvalidateUser`/`InvalidatePrincipals` 已提供本进程即时失效 + 代数保护，延长 TTL 前须审计所有用户状态/角色变更入口均触发失效，且多实例场景另行论证；
4. catalog 快照链 1368 条对每次 ATTACH/USE 后首次查询的元数据加载成本需要单独量化（旧副本 415 → 现 1368，3.3 倍）。

### 1B. 第二轮实施（2026-10-08 20:45）

**失效链路审计结论（完整）**：`UserService.Update`/`Disable`/`ChangePassword` → `changed(id)` → `InvalidateUser`；`CreateProject` → `ProjectsChanged()`、`AssignProjectOwner` → `ProjectsChanged()` → `InvalidatePrincipals`（含旧 owner 失效）；handler 层禁用/改密另触发 `RevokeAllForUser`。唯一缺口 `MarkLogin`（更新 `last_login_at` 不失效）已补：`user.go` MarkLogin 现调用 `changed(id)`，新增测试 `TestCachedUserInvalidatedOnMarkLogin`。

**已落地改动**：

1. **principal 缓存 TTL 5s→30s**（`internal/auth/session.go`）：失效链路审计完整 + 即时失效 + 代数保护后，TTL 仅作兜底；5s TTL 过期波次的 `auth_user_load` 远端点查（1.4–2.4s/次）频率降为 1/6。多实例部署仍受 30s 兜底约束（须在多实例计划中论证）。
2. **`/auth/refresh` 细分计时**：`observer.go` 新增 `AuthStageSessionUpdate`（旧会话 UPDATE）与 `AuthStageSessionIssue`（新会话写入+签发）；`session.go` Refresh 流程补 `auth_user_load` 计时；`stage_timing.go` 桥接为 `auth_session_update` / `auth_session_issue` 阶段。
3. **P2.2 auth 热点表定向 merge**（`internal/systemdb/maintenance.go`）：新增 `compactAuthTables`——每 15 分钟对 `sys_user_sessions`/`sys_project_owners`/`sys_users` 按 `ducklake_data_file` 存活计数（≥4 才合并，统计失败跳过），`CALL ducklake_merge_adjacent_files(lake, table)` 定向合并；仅 writer 执行、步间让出、失败不中断；与全库 merge 共用执行通道（catalog 同步/冲突检测照常）。新增 5 个测试覆盖阈值跳过/统计失败/非 writer/无连接/单表失败续行。

**验证**：`go build ./internal/... ./cmd/...`、`go test -race ./internal/auth ./internal/api ./internal/systemdb`、`go test -short ./internal/database/ducklake ./benchmarks/dbread`、`go vet` 全绿，lint 0 错误。待远端实测：auth 波次 p95、15 分钟后三表存活文件数回落情况。

## 2. 只读 catalog 副本与本地 benchmark 验证

使用 **2026-10-02** 本地 catalog 文件的只读拷贝统计（不能代表 **2026-10-08** 日志时刻的文件数或同步状态）：catalog 有 415 个快照，`ducklake_data_file` 333 条历史记录，其中 **228 个 `end_snapshot IS NULL` 存活**、105 个已失效；全部存活文件约 422 KB，最大文件约 7.6 KB。重点表：`sys_log_events` **42 文件/482 行**、`sys_metric_samples` **41 文件/254 行**、`sys_operations` **38 文件/38 行**、`sys_api_keys` **23 文件/24 行**、`sys_user_sessions` **12 文件/13 行**、`sys_cloud_agents` **1 文件/6 行**、`sys_users` **1 文件/1 行**。这仅说明**较早的本地视图**跨多个系统表有极小文件；本次日志里系统库已同步到 snapshot_id=1305，与副本的 415 不能直接比较。P0 须以当前快照重新测 `end_snapshot IS NULL` 口径后才能把文件数当作本次问题的验收基线。

新增 `benchmarks/dbread/{harness.go,read_test.go,README.md}`：本地 DuckLake `DataInliningRowLimit=0` 模拟一笔 INSERT 一个 parquet（**不是说当前生产被强制设为 0**：当前 config 为 1000，同步前的 `ducklake_flush_inlined_data` 仍可制造小文件）。和 1000 的内联对照；`TestReadAmplificationReport` 可复现：日志写入轮数 1/10/50/150 时，**全库**存活文件 45/54/94/194（含 bootstrap 等其他表），`QueryLogs` 热态平均 1.38/2.01/3.45/7.07ms；不能把全库文件数直接解释为该 SQL 扫描文件数。`BenchmarkSingleWriteCost` 验证 inlining=0 每次写入 +1 文件、+1 快照（约 1.2 KB/文件），inlining=1000 不新增 parquet 但仍 +1 快照。**本地结果只能证明这种造数方式下读取耗时上涨；尚未分离 parquet 数量、快照/元数据大小与缓存状态各自贡献，更不能把线性外推的 COS RTT 当做已测网络耗时**；`UserPointLookupDense` 1~60 用户的热态点查约 1.4ms、未呈明显线性上升，也不能据此单独证明 dev 日志中 auth 1~2.5s 全是排队。必须用下一节的远端阶段量化。

## 3. 性能测试矩阵：在 `benchmarks` 下追加的下一轮测量

已落地的离线基准为基线；**后续阶段仍需补充**下列分场景测试，不能把尚未实现的测试写成已覆盖：

| 场景 | 指标与对照 | 安全约束 |
| --- | --- | --- |
| auth JWT 命中/未命中、API Key 命中/未命中，`/auth/me` | 独立计量 `VerifyAccess`、`PrincipalFromClaims` 的 `principalMu` 锁等待/查用户/加载项目集、handler 再次 `GetByID`；并发 1/8/32（同用户与不同用户两组），持续 >5s JWT **Principal 缓存** TTL 与 >30s Key TTL 跨越各一次；用锁/阻塞 profiler 与单用户过期突发验证「全局锁 convoy」 | 不在报告/日志输出 token、key/hash；用非生产凭据。 |
| systemdb 热点读：projects、agents、agent-threads、cron-jobs、logs、metrics | 固定文件数 1/10/50/150 的冷/热态，读取 p50/p95/p99/max、SQL `QueryContext` 与 `rows.Next/Close` 分段、返回行数 | 仅 `benchmarks` 临时库造数；远端只读试验明确记录快照、网络区域、对象请求数。 |
| 后台 flush + sync + `GET /agents` 并发 | A=无后台、B=15s flush、C=同步前 flush+catalog COPY、D=真实 COS；统计连接 wait_count/wait_duration 增量、open/in_use/idle、冲突次数及重试时间 | 不在真实用户库/生产对象前缀做写压测；使用独立隔离 prefix。 |
| `GET /users`：用户数 1/10/50 | 记录 `List`、`ProjectIDsFor` 查询次数；有无 N+1 优化的前后对照 | 查询结果必须与角色/权限模型一致。 |
| `GET /agents`：种子已齐/缺失/并发 | 比较 `ensureAgents` 每项目已齐时至多 4×2 COUNT、缺失时的 UPDATE/INSERT，及 `ListCloudAgents`，并发写冲突重现 | 种子删除/dismissed 不可被自动复活；只读实例不得写入。 |

`cmd/perfbench` 仍提供真实 app+HTTP 的全链路复测（本地 DevMode），但它采用 `DataInliningRowLimit=100` 且用 API Key，与这次日志中的登录 JWT + 远端 COS 不同。新的 `benchmarks/dbread` 做微基准；两者一起运行并分别标注环境。门槛：每个用例 ≥100 个成功样本、重复 3 轮、记录冷/热及变异系数；对后台冲突保留失败率与**脱敏后的错误类别和次数**，不只输出成功样本。输入安全：不读取真实密钥、不写生产库；禁止将用户指定的包含敏感配置的日志原文复制进仓库。

## 4. 优化路线（按证据门槛实施，先测再改）

### P0：补齐诊断（不改变行为）

1. 首先将 `internal/auth/session.go` 的 `PrincipalFromClaims` 拆成**锁等待 / 缓存命中或未命中 / 库查用户 / 展开项目集**阶段；并补充 `internal/api/auth_handler.go` 的 `Refresh`/`Me`、`systemdb.Store` 的关键读入口的低基数耗时（仅时长、路由、命中状态、行数；不得输出凭据、SQL/参数/消息正文）。同时跑 mutex/block profiler 定位锁 convoy；既有 `StageAuth` 与 `StageProject` 分段**不重叠**，子阶段只用于细分、不与旧 total 双算。
2. 系统库用 `DB.Stats().WaitCount/WaitDuration` **窗口前后差**评估整池拥塞（并发请求同时测量时全局累计值**不可归因到单个请求**）；必要时在单个关键仓储调用用 `DB.Conn(ctx)` 单独计获取连接耗时，再在**同一连接**上分别计 Query/Scan，并在使用完毕后及时关闭。仅做「围绕一段 QueryContext 增量」会混入其他请求的等待，不能作为精准阶段归因。输出 `open/in_use/idle/max_open` 水位；阶段毫秒改为浮点或微秒，避免 0ms 假象。
3. 系统同步流程区分 `ducklake_flush_inlined_data`、`COPY`、上传与 `merge`；对 `sys_cloud_agents` 事务冲突记录频率及具体任务类型（不泄露行内容）。在测试库上跑 `EXPLAIN ANALYZE`，按**目标表**统计查询涉及的存活文件、快照/元数据规模与 `httpfs` 请求数；必要时用「同快照数量、不同数据文件数」及「同文件数、不同快照数量」两组对照，隔离各自的影响。线上只读观测、禁止为诊断触发维护删除。
4. 记录**隔离测试环境**真实远端冷/热态已授权 `/v1/projects/:projectID/logs`、`/v1/projects/:projectID/agents`、`/v1/projects/:projectID/metrics/summary`、`/v1/auth/me` 的 p50/p95/p99；记录 JWT 缓存是否命中、并发、背景任务和数据规模。若测试期测得连接等待才评估连接池/公平性方案，否则优先减少有证据的查询往返与目标表扫描开销。

### P1：先消除认证锁 convoy，再减少每请求系统库往返

1. **优先级最高：`SessionService.PrincipalFromClaims` 的全局锁内 I/O**。P0 若证实 `principalMu` 等待占认证阶段主要部分，把 `users.GetByID`、`users.PrincipalFromUser` 移出全局锁；同用户缓存未命中采用按用户单飞，避免同一波重复 SQL，同时允许不同用户并行认证。锁内仅完成 map 查询/回填；采用失效代数或版本号阻止「查询期间禁用用户/角色或项目权限变更，旧结果晚到写回」；错误不缓存，维持最多 5s 的用户禁用可见性与原有权限语义。测试包括不同用户并发互不阻塞、同用户单飞、cache 失效与 in-flight 竞态、数据库错误传播、`go test -race ./internal/auth`；与改前并发 auth p95/锁等待及系统库查询数做对照。
2. 可写实例的 `GET /agents` 每次列表先运行 `SeedDefaultCloudAgentsWithSandbox`：种子已齐时每个内置 Agent 最多两次 COUNT（当前 4 个），缺失才 UPDATE/INSERT。先量化往返后，评估将播种移至**建项目/升级回填**流程或 per-project 单飞的受控惰性初始化；成功后缓存需在删除/dismissed、沙盒能力变化时正确失效，且多实例/重启不能遗漏回填；只读实例继续只读。新增「并发首次访问、种子未补齐、已删除不复活、升级回填失败重试」测试。同期 `sys_cloud_agents` 同步冲突需先验证是否由播种写入造成，再决定是否采用此方案或仅针对冲突的有界幂等重试。
3. `GET /auth/me` 复用可信 Principal 的身份/项目元数据时需保持**用户禁用和角色变更的 5s TTL 约束**；若仍需读取 `sys_users` 最新数据则不跳过。先确认端点契约是否要求实时 `display_name/status`，再决定缓存 `User`/合并查而非裸删校验。
4. `GET /users` 将逐用户 `ProjectIDsFor` 改为单次批量项目归属统计，保持管理角色看到全部项目与普通用户仅自己的权限语义；比较 1/10/50 用户的 SQL 次数和耗时，确认 N+1 后实施。
5. 日志/指标按当前配置 15s 后台合批；**10s 缓存只覆盖 metrics/summary、metrics/trend**，不能称所有日志/监控读都已缓存。先检查 `QueryLogs` 是否因 `ORDER BY occurred_at DESC LIMIT 200` 扫描所有文件，`MetricsSummary` 的三次查询是否可安全合并/借助 rollup 缩小扫描；不得把 `LIMIT` 当文件剪枝保证。`/logs/retention` 可评估事件驱动失效+短 TTL 的只读缓存；项目维度隔离，不缓存权限结果。

### P2：控制小文件与同步竞争（须先通过 P0 证据门槛）

1. 系统库维护现有 `systemdb/maintenance.go` **启动 60s 后首轮、随后默认每小时** merge；`cleanup_old_files` 默认 **dry_run**。日志里首轮 merge 为 76ms；**228 个存活文件来自六天前的副本，不是首轮 merge 后的计数**。先在隔离环境测 merge 前后同一 catalog 的各表文件数和读延迟，验证 `ducklake_merge_adjacent_files` 对日志/指标/审计/API Key 等表的效果及限制条件；若仍持续小文件，再按表统计阈值（例如 >32 存活文件）安排后台合并。避开高峰、仅 writer 执行；合并是数据写入，必须纳入 catalog 同步、冲突检测与回退路径。
2. 旧副本显示小文件源头不止异步日志：`sys_operations` 38 文件/38 行、`sys_api_keys` 23/24、`sys_s3_objects` 23/23。按**对应表**定位写入（审计 `AppendOperation` → `sys_operations`、API Key 签发/吊销 → `sys_api_keys`、S3 索引 upsert → `sys_s3_objects`；refresh/session 轮转是**另一张表** `sys_user_sessions`）并考虑安全的批量写/去重；保留失败重试与写入一致性，不推迟影响权限/会话安全的撤销；**不要**对所有元数据写强行加 15s 延迟。
3. `factory.go` 当前连接池上限 10，每条额外连接仅执行 `USE lake`；维护/同步与 foreground 共用 DuckDB catalog 可能引发 `flush inlined data` 事务冲突。基于 `WaitDuration` / 事务冲突证据评估同步器与种子写的互斥或低峰调度，验证多连接 ATTACH/USE、隔离与关闭顺序；禁止仅靠把上限从 10 加大（会增加并发冲突和内存），也不要照搬 v4 文档改回 1。
4. v4 `ducklake-rw-latency-eventual-consistency-plan.md` §3 P2 的外部 parquet/http 缓存开关、用户库维护仍可在独立真实 COS 基准下试验；先用 `duckdb_settings()` 证实版本支持、实测请求数和命中率，再考虑启用；确保 `lock_configuration` 依旧最后设置。`cleanup_old_files` 保持 7d 安全门槛与默认 dry-run，不执行 `delete_orphaned_files`。

## 5. 验收、回退与执行顺序

| 指标 | 本次可复核基线 | 目标（同网络、同项目、同数据量） |
| --- | --- | --- |
| JWT 认证（跨路由 5s 过期波次） | 同波次 `auth≈1.6–2.5s`，无法仅凭日志定位是否锁等待 | P0 证实后：锁外 I/O，认证热 p95 <50ms；过期波次 p95 <300ms；用户禁用/改权不得因缓存回填竞态绕过失效 |
| `GET /v1/projects/:projectID/databases` | 成功请求 5–15ms（日志少量热样本）；auth 慢时 2.2–2.53s | 同环境重测：JWT 命中热 p95 <100ms，冷 p95 <500ms；分开报告 auth 与列表查询 |
| `GET /v1/projects/:projectID/agents` | 本次成功样本 3.46–5.77s | 种子已齐且同网络条件下 p95 <300ms；首次播种单独计量；并发期间无未恢复的 `sys_cloud_agents` 冲突 |
| `GET /v1/projects/:projectID/logs?limit=200` | 本次成功样本 3.05–3.16s（含认证） | 同网络及数据规模：冷 p95 <1s、热 p95 <200ms；优化文件数后注明**文件数发生变化**，不得称「同文件数」 |
| `GET /v1/projects/:projectID/metrics/summary` | 本次成功样本 0.77–1.54s（auth=0） | 缓存命中 p95 <100ms、冷 p95 <500ms；单独标明命中率 |
| 系统库存活文件 | **未取得 10-08 同期基线**；10-02 本地副本 228（日志 42、指标 41、审计 38） | 先建当前同口径基线；在安全维护后热点表目标 ≤5 或说明无法合并原因，观察 1h 新增速率 |
| 事务冲突 | 日志见一次 sync 失败 / 下一轮成功 | 隔离压测冲突为 0 或记录有界恢复，写入无丢失；用最旧未同步提交的实际年龄验证 `max_lag=30s`，不把配置值当实际保证 |

步骤：**先 P0 定位 JWT 锁等待并补全 `benchmarks` 的冷热/并发/远端隔离基线 → P1 先处理经验证的认证锁 convoy，再减少读请求数 → P2 治理文件数/竞争 → 隔离 COS 环境冷热各 3 轮复测**。每项独立提交，运行 `go test ./benchmarks/dbread`、`go test ./internal/auth ./internal/api ./internal/systemdb ./internal/database/ducklake -short` 和 `go build ./internal/... ./cmd/...`；DuckLake 集成测试如不在 `-short` 中覆盖需单独运行。若 P1 引起身份过期/种子被错误恢复，回退 P1 而不回退数据库内容；若 P2 引发事务冲突，停止新维护调度、保持 catalog 同步与写请求正常，先验证快照/对象引用一致性再重试，不自动清理对象。
