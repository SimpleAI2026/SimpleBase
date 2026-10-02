# DuckLake 存储与多实例接口延迟优化计划 v2.0

> 日期：2026-09-28。状态：plan-only；**本次仅更新本计划，不修改实现**。
> 基线：当前工作区（`main` @ `5579782` 加大量**未提交**改动，包括已实现的行数后台缓存、manifest 二分发现、租约修复），而非部署版本。本文“已确认”仅指静态代码路径；**没有生产端到端延迟样本或 COS 实测**。旧版 v2.0 中将已修复问题描述为“现状”的段落已按当前代码更正。
> 关联：`plan/planv3.0/multi-instance-consistency-plan.md`（2026-09-28 v2）。本计划是其**性能与存储选型补充**，不覆盖原文的一致性恢复操作规范。**原计划关于 PostgreSQL catalog／控制面迁移（§6、Phase E/F）的路线在本计划中不实施；其他存在冲突的设计以本计划为准。**

## 0. 决策摘要

1. **架构锁定：DuckLake + 每库本地 SQLite catalog + S3 Parquet 数据及 catalog 不可变备份。** 保留 SQLite，不迁往 PostgreSQL，也不将 S3 文件当成原生可写 catalog。S3 快照/manifest 属于本项目自建持久化与接管协议，必须先满足一致性验证，不能当作 DuckLake 官方多 writer 能力。
2. **后续扩展采用按库分片单写，而非同库多实例并发写**：每库任一时刻只允许一个持租实例拥有可写 SQLite catalog；不同库分配给不同实例。只读副本需安全刷新 catalog 并声明读滞后；控制面 `sys_*` 同样必须有唯一主写者、可恢复的共享身份以及明确的读写路由。没有这些前置条件，不应宣称已具备多实例高可用。性能优先验证**全量复制、单连接争用、S3 请求扇出及小 Parquet**，而不是换 catalog 后端。
3. **先确保安全边界再优化**：旧版所述 manifest 删除、全局失租标志、探针失败继续启动、续租网络失败未回调等，当前工作树已经有针对性修改；但仍需验证其完整性，尤其是 manifest 断链识别、失租后已取得的 handle、快照引用保留及失败传播。不得直接增大连接池或牺牲 fail-closed。
4. **当前前端超时是预算不匹配与多个慢链叠加，并非已证实的单一根因**：Axios 默认 15s，服务端写超时/SQL 查询默认 30s；当前数据库列表已有 `RowCounts` 后台刷新，不再在请求内逐库 `COUNT(*)`，但单库详情仍可能同步等 3s。重点验证系统库认证/项目解析排队、冷开 manifest GET/下载、写租约线性探测/40s 接管、COPY 占用单连接及后台统计对前台的干扰。无生产分段耗时前不能宣称任何一项是实际 p95 主因。

## 1. 官方资料与架构选型（截至本计划核对）

- [DuckLake 1.0 发布说明](https://ducklake.select/2026/04/13/ducklake-10/)：1.0 格式与 DuckDB v1.5.2 参考实现；catalog 为 SQLite/PostgreSQL/DuckDB 等数据库，Parquet 作为数据文件；说明并发、元数据缓存、加速 `duckdb_views()`、`COUNT(*)` 等改进。公告中某测试的提速不可直接推断本项目 API 的实际收益。默认内联阈值文章称 10 行；本项目配置是 100，且远端 `buildBootSQL` 已强制 0。
- [Choosing a Catalog Database](https://ducklake.select/docs/stable/duckdb/usage/choosing_a_catalog_database)：单本地客户端 DuckDB；多本地客户端 SQLite（仍有写锁限制）；多用户/远程客户端 PostgreSQL（要求 PG ≥12）；MySQL 文档有接入方式但不推荐。**没有**“S3 作为可写 catalog DB”的官方选项。
- [Choosing Storage](https://ducklake.select/docs/stable/duckdb/usage/choosing_storage)：S3 等是 `DATA_PATH` 的存储选项；不能据此推论为 catalog 后端。[Public DuckLake on Object Storage](https://ducklake.select/docs/stable/duckdb/guides/public_ducklake_on_object_storage) 将 catalog 文件上传后通过 HTTPS 发布，明确是公开只读场景。
- 本仓库 `internal/database/ducklake/extension.go:11-14` 将最低版本设为 DuckDB 1.5.2 并依赖 `ducklake`/`sqlite`/`httpfs`；`go.mod:11` 的 `duckdb-go/v2` 版本需在目标构建环境核对实际 `SELECT version()`/扩展版本，不能仅凭 Go 模块号推断。先用官方 `ducklake_settings()` 与 `DuckLakeMetadata` 日志对比部署版本、元数据往返与历史版本。

| 架构/部署形态 | 同库并发写 | 延迟及一致性边界 | 本计划决策 |
|---|---|---|---|
| DuckLake + 本地 SQLite catalog + S3 Parquet/不可变快照（现状形态） | **不支持**；单库只能一个 writer | 冷开恢复 SQLite，写后 COPY/上传，S3 请求及单连接争用；现实现仍有 §3 的缺陷 | **唯一实施架构**；修复协议后用于单实例或按库分片 |
| 多实例、按库分片单写 | **每库不支持**；不同库可分散至不同实例 | 每库持租、路由亲和、失租拒写、系统库唯一主写者及身份恢复；读副本需刷新与新鲜度契约 | 后续扩容方向，须完成 §4 P2 验收 |
| DuckLake + 纯 S3 可写 catalog | 官方资料未提供 | S3 不提供 DuckLake 所需的 SQL 事务及主键语义 | 不实施 |

**无需迁移 catalog 后端**。优化对象是本地 SQLite 副本的创建/同步/恢复协议：保留 DuckLake 表结构和历史快照，停写后先确保最终 catalog 快照上传成功，再灰度升级单库协议与路由；不能仅凭对象版本的存在声称数据已同步。控制面 `sys_*` 继续使用 DuckLake+SQLite+S3，但其高频小事务、唯一性与 CAS 约束需单独验证和实现串行写入；系统库身份必须能从 S3 权威对象恢复，禁止丢失本地 `locator.json` 就新建空系统库。

## 2. 延迟路径与量级假设（静态证据，非实测）

| 用户感知 | 实际路径 | 可确认的慢点与验证 |
|---|---|---|
| 进入数据库页/切换项目 | `ui/src/pages/Databases.vue:341-347` → 列表；`internal/api/database_handler.go:149-167,202-220` 查系统库后逐项读缓存，未命中调用 `RefreshAsync`；`internal/app/app.go:173-189` 已装配 `RowCounts` | **旧版列表同步 N+1 已在当前工作树移走**，但列表仍依赖认证、项目解析和系统库查询；`internal/api/db_stats_cache.go:106-123,143-150` 对当前页每个未命中库立即起 goroutine，只有实际统计受 `sem` 限制，排队 goroutine 无全局上限且**排队时间不计入** 5s 背景超时（取到 `sem` 后才 `WithTimeout`），失败后无退避可被下一次列表重复触发；冷开统计仍逐表 `COUNT(*)`（`db_stats.go:17-44`），与前台争用户库/系统库单连接。区分列表本身耗时与列表触发后 5s 内其他 API 的 p95；避免把旧链写成当前根因。 |
| 获取单库详情/认证/项目解析 | `internal/api/database_handler.go:238-244` 的 `GetDatabase` 仍同步 `RefreshSync(...,3s)`（`:170-200`）；`internal/auth/middleware.go:70-111` 每个业务请求认证，`database_handler.go:293-309` 每个项目路由解析 tenant | 单库详情冷缓存会占用最多 3s；系统库单连接繁忙时鉴权、项目解析、列表和详情都会排队（需实测）。文档写路由的 `systemGuard` 与 `acquire` 重复 `GetDatabase`（legacy 项目级路由还会两次 List + 两次 Get，`data_handler.go:246-308`）；仅 `CreateDocument` 额外连续两次 `Execute`/写后回调（`:160-168`），`UpdateDocument`/`DeleteDocument` 分别只执行一次，须分别计时并检查创建集合幂等逻辑。 |
| 展开集合/文档及 SQL | `internal/api/data_handler.go:46-71,102-133,246-275` 读系统库后 `Registry.Acquire`；`sql_handler.go:144-177` 查询；`database/query.go` 执行及扫描 | 冷库需要最新 manifest 发现/SQLite 下载/扩展 ATTACH；文档列表 SQL 无 `LIMIT`，在 Go 侧最多扫描 1000 行，超过限制由 `database.Query` 返回 `ErrRowLimitExceeded`，并非正常分页或截断（`database/query.go:29-49`）；仍需排序/扫描并可能读取小 Parquet 文件。热库被 COPY 占用时也等待。`DurationMS` 不覆盖鉴权、系统库、Acquire、序列化。 |
| 首次写/他人持租 | `internal/database/registry/registry.go:180-199` 在 `Acquire` 前运行 WriteGate；`internal/app/app.go:458-490` 同步调用 `lease.Manager.Acquire`；`internal/database/lease/lease.go:188-250` 从 epoch 1 顺序 GET | 每次新进程首次持租对历史 epoch 做 **O(续租次数)** 远端 GET；默认 10s 续租一天约 8,640 次 GET（仅理论量级，不是实测）。他人持租时请求内等 `TTL+Grace=40s`（`config.go:240-245`），超过前端 15s/SQL 30s，先发生客户端超时；多个请求重复进 WriteGate 时还可能重复远端探测（需并发测试）。 |
| 冷开/后台同步和回收 | `ducklake/catalog_syncer.go:760-853` `EnsureLocalCatalog` 查询 manifest，必要时下载全量 SQLite；`manifest.go:68-143` 指数+二分定位；`catalog_syncer.go:425-529` 用同一连接 COPY，随后上传快照、写 manifest、旧镜像、清理 | 冷开 GET 为 **O(log manifest 数)** 而非旧版 64 次上限或全量线性；本地有 state 锚点仍有远端往返，`manifest.go:146-161` 的 seq1 缺失检查仅探测 2 的幂，不能充分证明无其他 seq（正确性待修）。COPY 占用每库唯一连接（`factory.go:89-95`）；即使异步，200ms debounce 下也可能影响前台尾延迟；`pruneVersions` 每轮读取保留窗口及到期区间 manifest（`catalog_syncer.go:639-691`），在**同步 goroutine**内串行 S3 请求，不再从 seq1 每次重扫。 |
| 系统库共用连接 | `internal/app/app.go:384-410,664-704` 装配系统库与同步器；`internal/systemdb/store.go` 写后标脏；`ducklake/factory.go:89-91` 每库一连接 | 系统库高频写和 COPY 可能推高 JWT/API Key 校验、项目解析、列表与控制面操作的等待；对照系统库 `DB.Stats().WaitDuration`、sync 频率和请求延迟。共享 syncer 不等于不同库共享一条连接，须按系统库/用户库分开量测。 |
| 建库/建项目请求 | `api/database_handler.go:101-130` → `catalog/service.go:119-196` 顺序写系统库、远端 descriptor、回写 ready；`api/projects_handler.go:75-120,123-142` 建项目后同步创建 KV 库、`KVInit` 打开数据库建 schema | 每个步骤都在 HTTP 请求内，系统库 COPY 排队、descriptor S3 PUT、KV 首写租约/冷开均可能叠加；KV 初始化失败仍返回项目 201，不能以 201 判断 KV 已就绪。分开测建库/建项目阶段耗时与前端超时后副作用；如改为异步，必须给出兼容的 creating/ready 状态、幂等重试与错误可见性。 |
| 项目 KV 请求 | `api/kv_handler.go:56-94` 每次先在系统库查项目 KV 行，缺行还在请求内建库，再 `Acquire` 打开 KV 库；读写分流后 `kv.Store` 自管事务 | 项目创建时 KV 初始化失败仍返回 201（`projects_handler.go:117-142`），其后首次 KV 请求可能触发控制面写、S3 descriptor、写租约探测及 DuckLake 冷开；与热 KV 命令分开量测，缓存项目→KV ID 时必须有失效/归属校验，避免跨项目泄漏。 |
| 超时预算 | `ui/src/services/http.ts:7` Axios 默认 **15s**；`internal/config/config.go:233-235,292-297` HTTP 写超时/SQL 默认 **30s**；Data/KV handler 多处直接用请求 context | HTTP 401 还会触发额外 10s refresh 和一次 15s 重试（`ui/src/services/http.ts:66-101`），需区分真实慢查询与鉴权重试；客户端先放弃不等于服务端立即停止；`sync_on_commit` 时 `catalog_syncer.go:235-241` 明确使用 `context.Background()`，可能在前端超时后仍继续 COPY/上传。优先消除阻塞，再制定 API ≤ 客户端 < 网关/服务器的一致预算；**不以盲目延长前端 timeout 代替优化**。 |

**静态结论与验证界限**：已确认算法复杂度、单连接队列和超时配置；没有证明现网哪个阶段占主导。旧版“列表 200 库×3s”“manifest 最多 64 次”“每次从 seq1 清理”“任一库失租导致全库停同步”均不再是当前代码行为，不应用作本轮验收基线。

## 3. 安全门禁与未完成项（优化前先验证）

1. **已调整，但发现协议仍不完备。** 当前 `pruneVersions` 只清理快照、保留 manifest（`catalog_syncer.go:632-691`），`ReadLatestManifest` 已由 64 次前探改为指数/二分（`manifest.go:57-143`），`mirrorLegacyKeys` 在 manifest 成功后才调用（`catalog_syncer.go:502-526`）。但 `manifestExistsBeyond` 在 seq1 缺失时只查 2、4、8…（`manifest.go:146-161`）：例如 seq1、seq2 均缺但 seq3 存在（只丢 seq1、seq2 仍在则会被检测），可误判为空库并回退旧镜像；`nextManifestSeq` 在失效锚点后重查并对 nil 返回 seq1（`catalog_syncer.go:573-592`），还可能把旧链再次写成新起点；一般性中间断链也不能靠单次二分保证识别。任何读到的 manifest 要核验 key 内 seq 与 payload `Seq` 一致、snapshot 引用存在且可打开；空库与链损坏区分需可靠的远端锚点/列举或外部权威状态，不能通过有限次 404 推断不存在。历史断链不得自动“重建空库”。
2. **失租回调已 per-DB，仍缺提交级 fencing。** `catalog_syncer.go:139-153` 已按库置位，`lease.go:267-284` 网络失败也触发回调，`lease.go:177-185` OwnerID 含 bootID，探针在要求租约时失败拒绝启动（`app.go:323-348`）。但 `registry.Remove` (`registry.go:225-251`) 只摘除新请求句柄，已持有 lease 的写仍可能继续执行；`syncOnce` 仅在 COPY 前检查 `lostLease`（`catalog_syncer.go:411-423`），在 COPY/上传期间失租仍可继续 `PutIfAbsent`（`:425-515`）。写提交前及上传前后须建立可证明的 owner/epoch fencing，并在失去确认时拒写、拒上传；单纯读本地布尔值不足以阻止竞态。不要通过缓存旧 epoch 或加连接数绕过。
3. **快照修剪的引用安全。** `catalog_syncer.go:661-685` 只查看最近 `KeepVersions` 个 manifest 的引用，然后删除窗口外快照；历史 manifest 永久可见但其快照可能已删除。明确保留窗口之外仅供审计、不可据其执行恢复/接管；恢复时只有窗口内 manifest 可作为候选，必须验证引用存在。特别是当前最新值发现只按最大 seq 返回（`manifest.go:129-143`），**若最新 manifest 引用被删除，不能自动退到更旧快照或旧镜像**。遇保留窗口缺失或快照/manifest 不一致 fail closed；清理失败则停清理、保留数据并告警，不得因清理慢而阻塞已成功上传的快照可见性。当前 `pruneVersions` 的过期项 GET/DELETE 失败只 break 后返回 nil（`catalog_syncer.go:674-691`），而 `syncOnce` 又忽略其返回值（`:528-529`），清理失败不会反馈到同步结果，须独立暴露错误计数与重试水位；断链与 COS 条件写在真实版本控制配置下演练。
4. **本地与系统库一致性。** 系统库 `BindWithCacheDir` 已记录实际目录（`ducklake/factory.go:114-122`、`catalog_syncer.go:174-197`）；但 `systemdb/bootstrap.go` 本地 `locator.json` 的跨节点恢复身份仍需独立实现/验证。`EnsureLocalCatalog` 对 state 丢失或远端旧镜像的处理（`catalog_syncer.go:752-853`）需以真实本地领先/断网/缓存丢失实验验收；`Factory.Open` 对 `IsLocalAhead` 只记录告警后继续打开（`factory.go:53-68`），注释称随后应主动 Sync，但当前 `Factory.Open` 到 `Bind` 后返回（`factory.go:114-132`）无补同步动作；后续无新写时须确认 `dirty`/远端能否最终收敛，否则先拒绝依赖远端持久化的成功声明，并增加本地领先后空闲恢复实验。不能为了减少 GET 跳过水位验证。
5. **关闭与持久化结果必须准确。** `CatalogSyncer.Close` 当前会走 `allowClosed` 等待 inflight 并 flush（`catalog_syncer.go:361-390`），不应再描述为“closed 后 Sync 必然拒绝”。当前 `cache/evict.go:102-115` 已在 Close 失败时保留目录；但 `CatalogSyncer.Unbind` 即使 Flush 失败仍摘除绑定并删 dirty（`catalog_syncer.go:200-214`），`registry/handle.go:119-134` 仍关闭连接，须验证本地目录保留、重开后补同步/失败告警，不能因为关闭失败就认为水位已保存。继续验证其它关停路径：`systemdb/store.go:114-124` 忽略 `BeforeClose`/Flush 错误并关闭系统库连接，可能丢失唯一未上传的本地 catalog。`sync_on_commit` 仍使用 `context.Background()` 并在 `MarkDirty` 内等待 COPY/上传（`:217-242`）；记录失败日志不等于 API 写成功即远端持久化。必须明确 `committed_local`/同步失败与客户端超时后提交结果，避免不安全自动重试。
6. **写中同步任务可能丢失（正确性门禁）。** `MarkDirty` 以一次性 debounce 定时器触发同步（`catalog_syncer.go:217-252`）；同库 `Sync` 处于 `inflight` 时新触发直接返回 nil（`:305-309`）。如果旧 Sync 已复制快照且正在上传，新写发生并触发的定时器恰在旧 Sync 完成前到期，旧同步只清理至旧 snapshot 的 `dirty`（`:531-536`），余下脏水位可能没有后继任务，无新写/Flush 时长期滞后。须用慢 S3 + 并发写稳定复现，修为同步完成检查水位并继续调度（失败需限时退避），测试最终 S3 manifest 覆盖最新提交；不要把 `SyncLag` 有值当作已安排重试。
7. **无写重复全量同步（性能与对象成本）。** `CatalogSyncer.sync` 仅在 `targetSnap > 0 && targetSnap <= already` 时跳过（`catalog_syncer.go:317-328`），`dirty==0` 的已绑定库在 `Flush`/`Close` 时仍会进入 `syncOnce`，执行全量 COPY、快照/manifest/镜像上传及清理（`:342-390,393-529`）；`Factory.BeforeClose` 会走此路径（`factory.go:318-327`）。先对照无写 Close/重复 Flush 的 S3 请求数、manifest seq 增量与接口连接等待，修复前不得通过加大连接池掩盖全量复制。
8. **不可直接改大连接池。** `factory.go:89-91` 单连接虽易排队，但 SQLite catalog、COPY 一致性及 KV 读改写依赖它；任何多连接/独立备份连接先通过并发正确性实验。`orphan_ledger.go` 只有辅助结构不能证明所有 Parquet 写入前已登记，禁止以优化清理为名自动删除未知对象。

## 4. 分阶段实施（每项可单独回滚）

### P0 — 测量与止损（始终保留 SQLite catalog）

- **先确定部署版本**：核对运行中二进制是否包含 `RowCounts`、manifest 指数/二分、per-DB 失租等当前未提交实现；按 request ID/路由分别量测首页列表、详情、collections、documents、SQL、KV、登录与冷首写，用 1/8/200 库 × 每库 0/10/200 表的固定数据规模、同地区和固定并发量测首/次列表、详情、集合/文档、SQL/KV、首写；记录实际返回的页数、manifest/epoch 数、catalog 大小、Parquet 数量与冷热。不得拿未部署代码解释线上性能。
- **分段观测与超时**：客户端记录 15s Axios 超时/取消/HTTP 状态，服务端记录整链 p50/p95/p99、超时率，分段 auth → project resolver → catalog SQL → WriteGate/lease GET 与等待 → registry 命中/冷开（manifest GET、catalog 下载、BootSQL）→ DB 连接等待（`*sql.DB.Stats()` WaitCount/WaitDuration）→ SQL 执行/scan → JSON → 返回；后台单独记录每库 row-count 队列/任务/错误、COPY/上传/修剪的耗时与 GET/PUT/DELETE 次数。做空闲 vs 高频写、列表后 5s 内 vs 不加载列表、冷/热、单/多库对照；现有 `internal/observability/metrics.go:60-92,114-130` 可复用 HTTP/DB/S3/sync 指标，但 `prometheus.DefBuckets` 最大有限上界是 10s，需扩展到 15/30/40s 并辅以浏览器计时才能区分超时尾部；记录阶段分布而非只看 SQL `DurationMS`，不记录 SQL 内容或密钥。
- **止损门禁**：核验 §3 的远端锚点/断链、正在提交时失租、快照/manifest 引用；先暂停对一致性未验证的破坏性维护。前端若仅因为服务端 30s 与客户端 15s 不一致而超时，可在确认写操作幂等/超时后实际提交语义后临时调整预算，但不得靠单纯提高 timeout 掩盖 40s 等待和无界排队。检查已有内联行是否已刷出；`data_inlining_row_limit=0` 会制造小文件，不宣称它能直接降低读延迟。
- **故障与负载矩阵**：11→12 次同步、seq1/seq2 均丢失但 seq3 存在、任意中间空洞、>64/>1000 seq、删除被引用快照、全丢本地缓存、一天后租约首次 Acquire、并发首次写、另一 owner 持租、网络断连、COPY 期间读、COPY/上传期间再次写且此后空闲（检查 dirty 最终必被同步）、系统库高频写、无写重复 Flush/Close（验证零新 manifest/COPY）、关停 Flush/淘汰失败；双实例同前缀与真实 COS 条件写/版本控制需授权实测。

### P1 — 低风险热路径降耗（维持单写架构）

- **首次写先从请求关键路径移除长等待**：`lease.Manager.probeMax` 当前从 1 顺序 GET 到最大 epoch（`lease.go:234-250`）；设计支持重启恢复的权威高水位/索引与有界验证，禁止仅凭可覆盖 `latest.json` 判持租。对已有 owner 的 40s 观察式判活改为异步接管任务；API 在短预算内返回结构化 `lease_held`/`retry_after`（当前 `api/error.go:47-70,146-167,207-219` 未映射 `lease.ErrLeaseHeld`，会成为泛化 500），写后读路由继续指向现 owner。同库并发首次写 singleflight + 有界队列/独立请求 deadline；验证取消后无新的租约/本地提交、网络不确定性 fail closed。
- **减少远端发现往返但不削弱安全**：保留当前连续不可变 manifest 索引和指数/二分定位，修复 §3 中 seq1 缺失/中间空洞的发现协议；对本地可靠 `SyncedSeq` 可考虑常数次前探，前提是有远端权威水位并证明没有跳 seq/陈旧副本。冷库按 Open 阶段统计 manifest GET、旧镜像 Head、SQLite 下载/扩展引导耗时；若考虑 LIST 最大 key，要先验证真实 COS 分页/一致性，不可只靠可覆盖指针。快照清理改成独立低优先级限流任务，保留引用和进度验证，不阻塞当前 sync 的必要持久化步骤。
- **先消除无写 COPY/重复 manifest**：在 `Sync/Flush/Close` 使用持久水位、dirty 与远端已确认 manifest 一致性做真正的 no-op 判定，禁止 `dirty==0` 在正常已同步状态重复整库 COPY/上传（§3.6）；首次建库、同步失败重试、本地领先与本地 state 丢失不能仅凭内存零值跳过。注入式断网/重启测试验证该门禁既减少空闲同步频率又不漏已提交写。
- **系统库与用户库隔离 COPY 压力**：先量化系统库 auth/项目查询的 `*sql.DB` 等待与 COPY 重叠；仅真实 dirty 同步，合并/限频系统库小事务（以允许的持久化滞后为约束），将快照工作迁移独立连接前验证 SQLite 备份一致性和 KV 单写语义。对 `CatalogSyncer.Sync` 阶段分别计时（连接等待/`CurrentSnapshot`、ATTACH/COPY/DETACH、读暂存、快照 PUT、manifest 发现/PUT、兼容镜像、清理）；200ms debounce 下虽非同步等待写结果，后台仍在同一连接排队。当前同库 `Sync` 若发现已有 `inflight` 会直接返回 nil（`catalog_syncer.go:305-309`），`MarkDirty` 重置计时器后一次回调恰被跳过时可能长期留下 dirty，需做写并发 + 慢 S3 + 无后续写入的测试并补同步完成后检查水位/重调度；不能只通过降低 debounce 提速。`sync_on_commit` 的整库同步在请求中执行且使用 Background、失败只记日志（`catalog_syncer.go:235-241`）；`registry/handle.go:80-115` 与 `systemdb/store.go:68-72` 又忽略写后回调错误，先定义超时后提交/失败传播和响应 durability，再按 SLA 决定是否保留该模式；不能悄悄降低持久性。
- **控制台列表与详情分别处理**：当前 `RowCounts` 已移除列表的同步逐库统计；保留列表元数据快速返回，但 `RefreshAsync` 要在起 goroutine **前**通过有界队列/令牌限速、任务超时从开始排队算、过期/失败可退避重试，避免 200 库触发 200 个等待中的 goroutine 和 S3 冷开风暴；优先页面可见库，当前前端固定 `limit=200`（`ui/src/services/http-api.ts:755-762`）后本地分页（`Databases.vue:324-325`），且没有消费 `next_cursor`；改按服务端游标分页并保留跨页导航，而不是把整个 200 库视为可见或默默丢弃 200 以后的库。`GetDatabase` 不同步刷新 3s 统计，返回可省略的字段并按需单独请求；用写后水位、TTL 和错误码检查统计新鲜度，不允许 `COUNT(*)` 错误时把部分统计当准确值（当前 `db_stats.go:35-44` 会跳过单表错误并返回总数；`listTableNames` 用 `maxRows=200`，第 201 表会报 `ErrRowLimitExceeded`，见 `db_stats.go:49-56`、`database/query.go:45-49`）。比较统计开/关时列表以及后续接口的 p95。
- **创建接口单独限时并解耦初始化**：在权限/状态校验、建库 descriptor、状态回写、项目 KV 初始化各段记录耗时；项目创建目前同步执行 `ensureProjectKV`/`InitKV`（`projects_handler.go:117-142`），会进入系统库 + S3 + KV 首写租约/冷开路径，若实测超过预算，拆为持久化可恢复的异步初始化并给前端明确 ready/failed 状态；不能简单将 `InitKV` 放入无持久状态的 goroutine，防止实例崩溃后数据面未就绪却显示成功。建库仍必须在 descriptor 持久化成功后才能回 ready，失败返回语义/客户端重试须可判定。
- **精简具体 API 链路**：`DataHandler` 写路径先 `systemGuard` 再 `acquire` 重复读元数据；复用鉴权后的 catalog 结果，但不得省略跨项目/系统库校验。文档列表 SQL 加 `LIMIT`/游标和稳定排序，修复超过 1000 行时当前直接返回 `row_limit_exceeded` 而非分页的契约问题；对照 `EXPLAIN ANALYZE` 验证排序/扫描收益；`CreateDocument` 每次 `CREATE TABLE IF NOT EXISTS` 再 `INSERT`（`data_handler.go:160-168,322-325`），量测已存在表时 DDL 耗时/是否推进 snapshot；只有证明其造成实质开销且能处理并发建表后才考虑跳过，不能预设减少同步次数。检查 `information_schema` 耗时与单连接排队，集合列表同样以 `maxRows=200` 拒绝第 201 个表（`data_handler.go:46-61`），需明确分页/截断契约；再考虑句柄保温/预算化预热；不要直接提高 `MaxOpenConns(1)`。

### P2 — 同一架构下横向扩展：按库单写、可接管、可观测

- **用户库按库分片**：每库的本地 SQLite catalog 只能在当前 owner 实例可写，S3 上共享不可变快照与 Parquet；网关按 `(tenantID, dbID)` 找 owner 路由写和要求强一致的读，其他实例不得以自己的 SQLite 副本写同一库。实例可同时持有不同库的租约，分片迁移先停止旧 owner 写、完成最终快照并核验 manifest/对象引用，再由新 owner 恢复/接管，避免两套本地 catalog 分叉。
- **系统库仍为 DuckLake + SQLite + S3**：`sys_*` 只有一个控制面主写者，不同节点不能凭各自本地 locator 创建系统库并并行修改会话、项目或调度状态；将 locator ID 与 catalog 版本纳入共享可恢复身份，启动时从远端权威状态恢复。需要跨节点认证/实时控制面读取时明确转发主写者，或使用冻结的只读副本并显式声明滞后；Cron/Agent 的 CAS/唯一约束无法仅靠 DuckLake 数据表提供，优先主写者串行执行并设计崩溃接管/重复任务幂等，不承诺严格 exactly-once。
- **只读节点**：仅在具备一致性快照引用、刷新/句柄安全切换、失效时拒绝陈旧读后启用。普通读可声明有界最终一致，写后读/管理面鉴权与权限变更读走 owner；配置不同的 `instance.id` 不得改变共享 S3 命名空间；实例身份改用独立随机启动 ID。吞吐和可用性随数据库数分片扩展，**单个热点库仍是一个 writer，单库写高可用受租约失效与接管耗时限制**。
- **验收前置**：双实例同前缀/不同库可同时写、同库只有一个 owner；owner 故障或网络分区时不允许两个本地 SQLite 同时认为提交成功；接管后校验系统库身份、表/快照/Parquet 引用、所有权路由与重复任务副作用；报告失效/接管 RTO 和每库 S3 请求开销。做不到上述条件时保持单实例部署，不以“可扩展”为由放宽一致性限制。
- 如需只读 S3 公开发布，另行生成冻结快照并验证新鲜度和认证边界，**不能**作为在线可写后端。

## 5. 验收与决策门槛

- 功能：在整个实施周期均保持 DuckLake + SQLite catalog + S3 数据/备份；两实例同库最多一人写、不同库可分片；任何网络故障、修剪/重启、失租或探针失败不得导致两个 writer 同时成功提交；控制面唯一主写、跨实例统一系统库身份、故障接管后用户库快照与 Parquet 文件引用一致，恢复演练能读到存量数据。不要假定关闭内联即可无损恢复所有 metadata/表定义/历史快照。
- 性能：**先测当前部署基线再定数值 SLO**；同一区域/数据规模/并发下按路由、冷热、前端 15s 预算比较 p50/p95/p99、前端取消与服务端后续提交比例、504/超时率、每次冷开/首写远端操作数、连接等待与 COPY 耗时。列表 p95 与列表触发后台统计后的详情/SQL p95 要同时改善，不可只把延迟转嫁后台；接管窗口立即给可重试的明确状态而非请求内等 40s；长时运行租约、>64 seq 的冷开 S3 操作数不得随历史线性爆炸（租约现状须先改）。单库吞吐与多库分片吞吐分别报告。
- 测试：本次核验当前工作树 `go test ./internal/api/... ./internal/database/ducklake/... ./internal/database/lease/... ./internal/app/...` 已通过（本地单元测试，不是性能数据）；现有 `internal/api/db_stats_cache_test.go` 与 `ui/src/pages/Databases.test.ts`/`ui/src/services/http-api.test.ts` 为单库逻辑/模拟测试，不能代替 200 库后台任务或真实超时压测；实施阶段补齐 `go test ./internal/database/... ./internal/app/... ./internal/api/... ./internal/systemdb/...`，并增加有耗时/请求数断言的慢对象存储、并发首次写、RowCounts 负载、系统库 COPY 干扰、超时取消和多实例故障注入测试。真实 COS/生产环境联调需单独授权及凭据，本文没有实测生产 p95。

## 6. 实施触点与来源

- 性能主路径：`internal/database/ducklake/{catalog_syncer,manifest,factory,extension}.go`、`internal/database/lease/lease.go`、`internal/database/registry/{registry,handle}.go`、`internal/systemdb/{bootstrap,store}.go`、`internal/api/{database_handler,db_stats_cache,db_stats,data_handler,kv_handler,sql_handler}.go`、`ui/src/services/http.ts`、`ui/src/{pages/Databases.vue,components/databases/CollectionPanel.vue}`。
- 架构接线：`internal/app/app.go`、`internal/config/{config,env,yaml}.go`、`internal/catalog/sql_repository.go`、`internal/objectstore/{blob,client,keys}.go`；配置/部署文档与迁移 runbook 后续实施 PR 同步更新。
- 官方依据：本计划 §1 的 DuckLake 官方 1.0 公告、catalog 选型、storage 选型与公开只读指南；不将原计划中“manifest 已保证多实例安全”的设计推断当作实测事实。
