# 数据库慢读与 S3 存储架构评估及实施计划（2026-10-10）

> 状态：首轮代理播种批量读与本地基准已落地；以下 P0–P4 是**待实施计划**，不是已完成承诺。保留原有响应协议、单写实例、`committed_local` 与异步 S3 同步语义。详细实施必须由阶段证据触发，未满足验收门槛不推进下一阶段。

## 证据边界

只读分析 `/var/folders/9d/g20j9n3968gc16bygrrh88qw0000gn/T/simplebase-dev-logs.*/backend.log` 中的后端日志。该目录包含多次运行，不能把不同运行的请求当作同一统计总体；下列是代表性样本而非生产 p99。日志原文包含环境配置，本文不复制原文、不记录凭据、SQL 参数或业务内容。`perf_stage.stages` 是毫秒，`http.duration` 和同步阶段 `duration` 是秒；`auth_user_load` 是 `auth` 的子阶段，不能相加；`unattributed` 不等于 SQL 执行。

| 样本（日志文件、行号） | 可确认的现象 | 尚需区分 |
| --- | --- | --- |
| `uP2ijU/backend.log:77-80` | GET 设置 5.907s，其中认证 5.124s、用户加载 5.121s、锁等待约 0.001ms；另一并发请求共享同波次认证耗时 | 用户点查触发的远端读取次数、网络/引擎各自耗时 |
| `JqxEYF/backend.log:55-74` | 登录缓存命中时 GET agents 3.039s、metrics summary 1.540s、databases 0.838s；认证仅 0.02–0.06ms | 播种查询、列表读取、远端小文件、后台争用的占比 |
| `35opRC/backend.log:48-60` | metrics summary 1.678s、agents 2.001s；同一批运行 databases 后续热读 4.58ms；首次 KV 1.55s（acquire 246ms），后续 KV 4ms | 用户库首次使用的连接初始化及执行时间 |
| `uP2ijU/backend.log:65-82` | 多次系统库池采样 `wait_count_total=0`，上限为 10 | DuckDB 内部竞争不包含在 sql.DB 的 WaitDuration 中 |
| `uP2ijU/backend.log:26-29,135` | 系统库同步曾有 flush 1.14s、快照上传 1.99s；另有上传耗时 213s 且报错 | 远端抖动与同步失败期间的实际数据可恢复水位；配置 max_lag 30s 不是成功上传的硬上界 |

本地隔离库 `go test -run '^TestReadLatencyPhases$' -v -count=1 ./benchmarks/dbread`，Apple M4 Pro：20 轮日志写入，inlining=0 时全库 67 个存活文件，首读 3.875ms、热读 p50 2.293ms/p95 3.172ms/p99 3.843ms；inlining=1000 时全库 0 个文件，首读 1.999ms、热读 p50 1.372ms/p95 1.751ms/p99 1.853ms（各 100 次成功读取）。此处“首读”只是新建本地库后的第一次同类查询；全库文件数不等于该 SQL 实际扫描文件数，不能推算真实 COS RTT。本轮额外本地基准：用户点查约 1.376ms、日志 dense-10 约 1.858ms、metrics dense-10 约 5.094ms（10x），与上述远端秒级耗时不是同一网络条件。

## 已采取的局部优化

`GET /agents` 在可写实例每次进入列表前调用 `SeedDefaultCloudAgentsWithSandbox`；原实现对四个默认 agent 每个最多两次 `COUNT`，种子齐全时最多 8 次系统库读，再读列表。改为按项目分别读取 dismissed key 和 existing key 两次，保持删除不复活、缺失播种和只读实例跳过播种；仅针对读往返放大，不改 catalog、租约及写成功语义。增加 `seed_default_agents` 与 `list_cloud_agents` 两个固定操作名的耗时日志（无项目 ID/SQL/行内容），帮助验证两阶段的远端收益。系统库定向 auth 表合并、30s Principal 缓存、认证锁外 I/O 已存在，不重复调整。没有在生产 COS 上复测前，不宣称远端 p95 改善倍数。

仍需在隔离 COS 前缀复测：同样已授权请求、相同项目数据与背景同步，统计 `seed_default_agents`/`list_cloud_agents`、`auth_user_load`、目标表存活文件数、远端 GET 数与错误率；对比冷/热 p50/p95/p99，各组至少 100 个成功样本并保留失败计数。若出现 213s 上传错误，先确认实际同步滞后和告警，勿仅调整配置宣称 RPO 已有上界。不进行生产写压测、清理或格式迁移。

## DuckLake + S3 是否合适

**适合**单写实例、DuckDB 分析/SQL 查询、数据量较大且按列扫描、允许写成功只表示本地提交和异步远端收敛的场景。DuckDB 是 SQL **计算引擎**，DuckLake 管表快照/文件引用，S3 存储 Parquet 数据；DuckLake catalog 当前位于本地 DuckDB 文件并通过不可变快照 + manifest 异步上传至 S3。读仍需下载/读取远端文件，且本地元数据和可内联行意味着这不是“计算节点无本地状态”的纯分离架构。

**不适合直接承诺**低延迟高频点查、事务提交即 S3 持久、无本地盘情况下零丢失，或多实例共享写入强一致。对象存储上的 Parquet 是不可变文件；当前本地 catalog 提交后，先 flush 内联数据、再 COPY catalog、上传快照和 manifest。`DurabilityFor` 返回 `committed_local`，后台通常按 15s 周期同步；实际成功落地时间受网络/重试影响，`max_lag=30s` 只是配置目标而不是上传故障时的数学保证。读写同一存活进程可看本地已提交状态；恢复或远端读副本只能看到已发布的快照。须继续核验 manifest 前数据对象全部可见、远端快照哈希和版本连续性、失败时本地领先的恢复流程，并监控实际最旧未同步提交年龄。

**DuckDB 与 SQLite 不是二选一的计算层方案。** 当前 `catalog_engine=duckdb` 是 DuckLake 元数据本地文件格式；改为 `sqlite` 只改变 DuckLake 的 catalog 后端，查询执行仍是 DuckDB，Parquet 仍在 S3。SQLite 数据库文件也不能直接当作 S3 上可更新、并发事务安全的在线数据库：S3 没有 SQLite 需要的共享随机写和文件锁。单写当前默认 DuckDB catalog 合理；SQLite catalog 只有隔离基准证实多本地连接/元数据竞争收益且承担格式迁移成本时才评估，不能在运行中切换。当前启动格式检查拒绝不兼容引擎，不重置已有数据。

要实现**提交即远端持久**，必须让写请求等待数据对象落地和 catalog 提交点可靠发布，定义远端故障下返回失败/重试的幂等规则，测量 RTT 与延迟预算；若允许本地 WAL+复制则必须公开可量化的 RPO。要实现**多个计算实例强一致写**，必须使用共享且支持事务、并发冲突检测/隔离的 catalog 服务与可靠写者 fencing；单纯换 SQLite catalog 或把它上传 S3 不够。当前每库 registry 唯一性是进程内保障，跨实例租约默认关闭；不可变 manifest 冲突只能帮助检测，不等于自动解决多写分叉。高频认证/权限/会话等控制面表的小文件点查，与 DuckLake 分析型数据面需求不同；若以后追求严格低延迟控制面，应单独评估外部事务存储及迁移边界，而不是在此次读优化中引入第二引擎。

## DuckLake 官方规范与扩展实现研究（资料核对：2026-10-10）

> **资料分级**：下文的“格式规范”指 DuckLake 1.0 [Specification](https://ducklake.select/docs/stable/specification/introduction)，定义元数据与查询规则；“官方扩展”指 [DuckDB Extension](https://ducklake.select/docs/stable/duckdb/introduction)，描述 DuckDB `ducklake` 扩展的具体行为。用户给出的 [DuckDBLab 中文介绍](https://duckdblab.org/zh/post/ducklake-v1-intro/) 是第三方解读，并非 DuckDB Foundation 的规范；它关于“无需外部编目服务”、文件级乐观锁或性能星级的表述不能替代规范和实测；不需要 Hive Metastore 也不等于 DuckLake 不需要 SQL catalog。以下仅原创概述，不复制文档全文；上游 stable 可能随版本更新，具体能力仍须以部署所用扩展版本验证。

### 1. 两层架构、文件布局和查询可见性

- **事务 catalog + 数据文件**：格式规范要求 catalog 数据库支持 SQL 事务及主键约束，保存表结构、快照、文件及统计信息；数据面使用 Parquet，可落对象、块或文件存储。**catalog 本身需要主键，与 DuckLake 用户数据表不强制执行主键是两个不同层次**，不能用后者推断 catalog 无需事务。来源：[规范导言](https://ducklake.select/docs/stable/specification/introduction)、[扩展不支持的功能](https://ducklake.select/docs/stable/duckdb/unsupported_features)。
- **核心元数据**：`ducklake_snapshot(snapshot_id, snapshot_time, schema_version, …)` 记录版本；`ducklake_schema`、`ducklake_table` 描述命名空间及路径；`ducklake_data_file` 包含 `table_id`、`path`、`begin_snapshot`、`end_snapshot`；`ducklake_delete_file` 把删除记录关联到原数据文件；`ducklake_file_column_stats` 提供文件级列统计；内联表把小批变更放进 catalog 而非 Parquet。通常路径由 catalog 的 `data_path` 为根，schema、table、文件依层级相对定位；也存在绝对路径，不能假设换根目录即可迁移所有文件。来源：[元数据表](https://ducklake.select/docs/stable/specification/tables/overview)、[路径](https://ducklake.select/docs/stable/duckdb/usage/paths)。
- **版本判定**：在目标快照 `sid` 下，元数据项可见当且仅当 `sid >= begin_snapshot AND (end_snapshot IS NULL OR sid < end_snapshot)`；先筛可见数据文件，再合并相同快照可见的删除文件及内联变更。`end_snapshot IS NULL` 的文件数仅代表**当前仍有效的文件**，不是指定查询实际读取的文件数。可用 min/max 统计做文件剪枝，但缺失统计或范围重叠仍需读文件和应用删除标记。来源：[规范查询规则](https://ducklake.select/docs/stable/specification/queries)、[分区与文件剪枝](https://ducklake.select/docs/stable/duckdb/advanced_features/partitioning)。
- **快照/时间旅行**：`current_snapshot()` 返回当前最大版本，显式事务的一次提交对应一次快照；扩展提供按版本或时间戳历史查询。历史快照可以过期，过期后不能把旧时间旅行当作永久可用的备份。来源：[快照](https://ducklake.select/docs/stable/duckdb/usage/snapshots)、[时间旅行](https://ducklake.select/docs/stable/duckdb/usage/time_travel)、[过期快照](https://ducklake.select/docs/stable/duckdb/maintenance/expire_snapshots)。

### 2. 官方事务、并发与 catalog 选择

- **单个共享 catalog 下的事务语义**：DuckDB 扩展提供 ACID、快照隔离，包含 DDL 的显式事务；并发提交可在 catalog 的 `ducklake_snapshot.snapshot_id` 主键处发生冲突。扩展检查 `ducklake_snapshot_changes` 判断逻辑冲突，允许时重试 catalog 提交，不必重新写已经生成的数据文件；删表、改表、删除和压缩交错可能不可合并。删除冲突可按**底层文件**发生，不应称为逐行锁或“同表写必然可并发”。上述机制依赖多个 writer 访问**同一个支持事务和约束的 catalog**。来源：[事务](https://ducklake.select/docs/stable/duckdb/advanced_features/transactions)、[冲突处理](https://ducklake.select/docs/stable/duckdb/advanced_features/conflict_resolution)、[规范导言](https://ducklake.select/docs/stable/specification/introduction)。
- **官方后端选型**：单客户端本地场景推荐 DuckDB catalog；同主机多进程推荐 SQLite catalog，但 SQLite 同时只有一个写者，锁竞争会重试；跨主机多客户端共享读写推荐 PostgreSQL（文档要求版本 ≥12）。这三者都是 **DuckLake 元数据 catalog 后端**，计算/读取仍由 DuckDB 扩展执行。把可写 SQLite/DuckDB 文件当 S3 对象直接共享，不会获得数据库文件锁和事务提交能力。来源：[选择 catalog 后端](https://ducklake.select/docs/stable/duckdb/usage/choosing_a_catalog_database)、[连接](https://ducklake.select/docs/stable/duckdb/usage/connecting)。
- **连接语义**：`ATTACH 'ducklake:<catalog>'` 建立 DuckLake；创建时 `DATA_PATH` 可指向 `s3://...`，连接已有湖时数据路径通常从 catalog 元数据读取；`READ_ONLY` 可限制连接写入。`OVERRIDE_DATA_PATH true` 是**当前连接的临时覆盖**，不更改 catalog 中持久化的路径，并可能导致原路径数据在当前连接不可读；不能把它等同迁移或自动复制。默认的 `CREATE_IF_NOT_EXISTS` 也意味着打开错误路径可能创建空 catalog，项目必须保留现有实例格式/manifest 校验。来源：[连接文档](https://ducklake.select/docs/stable/duckdb/usage/connecting)、[远端路径只读发布示例](https://ducklake.select/docs/stable/duckdb/guides/using_a_remote_data_path)。

### 3. S3、内联与查询性能

- **文件与对象存储**：DuckLake 的数据文件不原地修改，新增文件和元数据版本推进是核心模型。DuckDB 扩展可利用本地盘、S3 兼容存储、GCS、Azure 等文件系统；远端吞吐有规模优势，但网络时延、对象请求、传输费用会放大小文件查询成本。`DATA_PATH` 在 S3 并不意味着事务 catalog 也在 S3。官方提供的“公开对象存储 DuckLake”和“远端 data path”指南主要演示 **HTTPS 发布只读 catalog/文件**，不是多个 writer 直接在 S3 共享可写 catalog 的协议。来源：[选择存储](https://ducklake.select/docs/stable/duckdb/usage/choosing_storage)、[公开只读 DuckLake](https://ducklake.select/docs/stable/duckdb/guides/public_ducklake_on_object_storage)、[远端路径指南](https://ducklake.select/docs/stable/duckdb/guides/using_a_remote_data_path)。
- **小写入内联**：DuckLake 将低于配置阈值的小批插入及部分删除信息直接放入 SQL catalog 的内联数据表，避免每次生成小 Parquet；`ducklake_flush_inlined_data` 再把它们写为数据/删除文件，可能增加快照和对象 I/O。官方文档中内联行数默认 `10`（可设 `0` 禁用），持久化的 `set_option` 可覆盖 ATTACH 级设置；对于异步备份，未 flush 的行仅在本地 catalog 中，必须校验 flush 与数据对象落地先于可恢复 catalog 的发布。来源：[数据内联](https://ducklake.select/docs/stable/duckdb/advanced_features/data_inlining)、[配置](https://ducklake.select/docs/stable/duckdb/usage/configuration)。
- **文件组织与点查边界**：扩展文档默认 `target_file_size=512MB`、`parquet_compression=snappy`；目标大小不是每次写入的硬性文件下界。文件级 zone map/分区有机会减少扫描文件，但分区不是点查索引，也不能把 SQL `LIMIT` 当作远端文件裁剪保证。DuckLake 用户数据表不支持强制执行索引、PK/UNIQUE/FK；`UPDATE`、`DELETE` 和 `MERGE INTO` 并非因此不可用。认证、会话等频繁点查需要独立测延迟与请求数。来源：[配置](https://ducklake.select/docs/stable/duckdb/usage/configuration)、[分区](https://ducklake.select/docs/stable/duckdb/advanced_features/partitioning)、[不支持的功能](https://ducklake.select/docs/stable/duckdb/unsupported_features)。

### 4. 官方维护、备份与恢复边界

- `ducklake_merge_adjacent_files` 显式合并**符合条件、schema 版本相同**的小文件，可按表执行；`auto_compact` 控制批量维护涵盖哪些表，不意味着每次写后自动运行。合并产生新文件和元数据变更，历史文件不会立刻从对象存储删除；必须以合并前后**目标表活跃文件数、查询实际 GET、读时延**验证收益。来源：[合并相邻文件](https://ducklake.select/docs/stable/duckdb/maintenance/merge_adjacent_files)、[推荐维护](https://ducklake.select/docs/stable/duckdb/maintenance/recommended_maintenance)。
- `ducklake_expire_snapshots` 解除旧版本可访问性，不自动删除所有历史文件；`ducklake_cleanup_old_files` 对已排队删除的文件设 `older_than` 和 `dry_run` 才能降低长读事务风险。`ducklake_delete_orphaned_files` 可能把不属于当前目录追踪的对象列为孤儿，**不能**在混用前缀或生产环境无审查地启用。`CHECKPOINT` 在扩展文档中是综合维护（内联 flush、过期、合并、重写、旧文件/孤儿文件清理），不能当成单纯 fsync 或无副作用的“刷新”。来源：[过期快照](https://ducklake.select/docs/stable/duckdb/maintenance/expire_snapshots)、[文件清理](https://ducklake.select/docs/stable/duckdb/maintenance/cleanup_of_files)、[检查点](https://ducklake.select/docs/stable/duckdb/maintenance/checkpoint)。
- **灾备要同时覆盖目录和数据**：catalog 快照必须引用实际可用的数据/删除文件；只备份 Parquet 丢失 schema、版本和文件引用，只备份 catalog 无法重建丢失的 Parquet。catalog 备份之后的新事务可能留下未被备份 catalog 追踪的文件；恢复复制桶时还需核对持久 `DATA_PATH`。来源：[官方备份恢复指南](https://ducklake.select/docs/stable/duckdb/guides/backups_and_recovery)、[路径](https://ducklake.select/docs/stable/duckdb/usage/paths)。

### 5. 官方机制与 SimpleBase 当前实现的逐项对照

| 维度 | 上游能力／推荐 | 本项目当前实现及代码位置 | 不能据此承诺 |
| --- | --- | --- | --- |
| SQL 计算与 catalog | DuckDB 计算；单客户端 DuckDB catalog，多客户端远端可选择共享 PostgreSQL | `internal/config/config.go` 默认 `catalog_engine=duckdb`，兼容配置选择 `sqlite`；`internal/database/ducklake/factory.go` 本地 catalog ATTACH；`internal/app/catalog_engine.go` 校验已有实例格式 | 选择 SQLite 不等于共享远端事务 catalog；**当前没有** PostgreSQL catalog 装配 |
| 数据路径 | Parquet 可以存 S3，按元数据快照选择文件 | `internal/database/ducklake/secret.go` 的 `buildDataURI` 构造 S3 URI，`internal/database/ducklake/factory.go` 的 `bootAttachSQL` 将其设为 `DATA_PATH`；每库打开时下载或保留本地 catalog | S3 数据路径不等于查询无网络开销、catalog 无本地状态 |
| 同步与恢复 | 官方共享 catalog 支持事务和冲突检测；备份需同时保证元数据与数据文件 | `internal/database/ducklake/catalog_syncer.go` 同步前 `ducklake_flush_inlined_data`，`COPY FROM DATABASE` 到 staging、快照校验；项目自行上传不可变 catalog 快照，`internal/database/ducklake/manifest.go` 发布连续 manifest 并校验 SHA256 恢复 | manifest 链、`PutIfAbsent`、本地领先不覆盖、RPO 是 **SimpleBase 协议，不是官方跨实例多写保证** |
| 并发 | 多 writer 的冲突解决依赖共享事务 catalog | `internal/database/registry/registry.go` 每库单 handle；`internal/database/ducklake/factory.go` 连接池上限 10、空闲 2，写/同步协调；`internal/app/app.go` 可选 per-DB lease，默认关闭 | 单实例多连接不等于多实例同时写入安全；manifest 冲突只是探测与拒绝 |
| 写入可见性 | 官方 catalog 事务以 snapshot 表示提交 | `Factory.DurabilityFor` 为 `committed_local`；同步间隔默认 15s，`max_lag` 配置 30s；故障时实际延迟可能更久 | 写 API 200 不等于远端已持久；30s 不等于远端故障时硬 RPO |
| 内联/文件维护 | 扩展默认内联 10、target 512MB、snappy；merge 是显式维护 | `internal/config/config.go` 当前默认内联 1000、target 64MB、zstd；`internal/systemdb/maintenance.go` 自行安排全库/认证热点表定向 merge，清理默认 dry-run | 官方默认值不能替代项目配置；内联不是全远端恢复保证 |

**研究后的验证重点**：在隔离前缀对 `sys_users`、`sys_user_sessions`、`sys_log_events` 按表统计存活文件、snapshot 数与 `httpfs` 请求量；分别测同一 SQL 的首读/热读及同步/merge 后延迟。验证「数据对象上传完成 → catalog 快照上传 → manifest 发布」的可恢复性，检查失败和失租时不会发布引用缺失文件的 manifest。对真正跨实例读写，应单独评估共享事务 catalog（例如 PostgreSQL）、租约 fencing、权限和迁移成本；这不是把 catalog 文件上传 S3 即可完成的配置变化。以上是后续**验证建议**，不是已在生产 COS 执行的检查。

## 实施原则与阶段依赖

1. **先度量，再改变行为**：P0 补可归因日志与同口径样本；P1 优化已证实的查询往返；P2 处理远端读放大及冷开；P3 验证一致性恢复；P4 复测并根据门槛决定是否上线。每阶段单独回退，不将观测、缓存策略、存储格式混在一次发布。
2. **跨阶段不可变约束**：系统库始终用 `systemdb.Store.DB()` 常驻连接，API admin 项目只允许 SELECT；用户库经 `registry.Acquire`，不绕过 `sqlguard`、权限或只读实例写门禁；不新增依赖，不改 `go.mod`/前端配置，不向日志写凭据、SQL 参数或 LLM 正文；不修改生产 COS 数据、不删除 `.codebuddy` 或缓存。
3. **最小工作范围**：第一轮预计修改 `internal/auth/{observer,session,user_repository}.go`、`internal/api/{stage_timing,stage_auth_observer_test}.go`、`internal/systemdb/{agents,observe,maintenance}.go`、`internal/database/{registry/registry,ducklake/factory,ducklake/catalog_syncer}.go` 中**经测量触发的文件**及现有对应测试；本目录 `benchmarks/dbread` 增测试矩阵。某一阶段不具证据即不改其生产路径。
4. **部署前决策门**：若需求仍是单写且可容忍异步 RPO，保留 DuckDB catalog；若明确需要提交即远端持久或多写强一致，先另起架构设计/迁移评审，不在本计划里偷偷切 SQLite、扩大连接池、延长认证 TTL 或打开自动清理。

## P0：把慢点定位到可修改的具体阶段（先落观测，无行为变化）

### P0.1 认证点查及单飞

- **现状与改动点**：`internal/auth/session.go` 的 `PrincipalFromClaims` 已在锁外查库并按用户单飞，`loadPrincipal` 的 `auth_user_load` 包含整个 `users.GetByID`；`internal/auth/user_repository.go` 的 `GetByID` 实际是 `sys_users WHERE id=?`。在仓储边界增加可选的低基数测量接口或包装层（沿用 `auth.AuthObserver` 与 `api.stageAuthObserver`），在同一 `*sql.Conn` 上区分取连接、`QueryContext`、首行 `Scan`，实验时必须在同一连接执行并及时 `Close`；若保留默认 `QueryRowContext` 快路径，应保证观测关闭时完全沿用原调用。不输出用户 ID、查询参数、密钥；`auth_user_load` 仍为父阶段，新 `auth_user_conn`/`auth_user_query`/`auth_user_scan` 作为子阶段不纳入 `perfStages` 求和。连接池 `DB.Stats()` 差值只用作整个采样窗口的信号，不可归因为某个请求。可选通过短时 mutex/block profiler 对 DuckDB 内部竞争作验证。
- **单飞诊断**：在 `internal/auth/session.go:359-381` 区分 owner 库查、follower 等待、owner 失败后的 fallback 重查；先计失败重查量，再确定是否需要调度策略。修复 follower 等待时忽视自己 `ctx.Done()` 的问题，返回 `ctx.Err()`，不可将取消转成成功 Principal。失败批次若 8/32 个 follower 各自重查造成放大，优先让仍然有效的同批请求重新参与 per-user 单飞而非无界同时直查；错误不能缓存为成功，权限变更期间旧结果不得回填。`loadPrincipal` 现有代数保护只阻止旧缓存回填，**不保证在途请求即时撤销**，测试与对外描述应保持此边界。
- **文件/测试**：`internal/auth/{observer,session,user_repository,principal_singleflight_test,cached_user_test}.go`，`internal/api/{stage_timing,stage_auth_observer_test}.go`。使用阻塞仓储测试 follower 取消、8/32 并发 owner 失败、同用户单飞、不同用户不互阻、用户禁用/角色变更/项目 owner 转移与回填交错；`go test -race ./internal/auth ./internal/api`。附带修正 `session.go` 中 `CachedUser` 注释仍写 5s、实际 TTL 为 30s 的文字错误。
- **门槛**：先收集每种 auth 状态（命中、miss owner、miss follower、失败）各 ≥100 次、同/不同用户并发 1/8/32；如果库调用内部耗时或并发失败并非主因，停止对应行为改动，不盲目调大缓存 TTL。多实例失效仅由既有 TTL 兜底，不宣称即时传播。

### P0.2 系统库、冷打开、远端数据读取

- `internal/systemdb/observe.go` 已有固定 operation 的总耗时/行数与全池统计，`internal/systemdb/store.go` 的 10s 单飞缓存仅覆盖 metrics summary/trend。对 `sys_users`、`QueryLogs`、`MetricsSummary` 三个高耗时入口优先分别测「获取连接／查询／扫描」，仅在诊断阶段对同一个连接计时，不把 `unattributed` 直接视为 SQL；附加缓存 hit/miss、返回行数与目标表的存活文件数，按 `ducklake_table` 与 `ducklake_data_file` 关联，只记录汇总整数而不输出表行。对 metrics summary 的三次 SQL 分别计时，不把缓存命中样本混入冷查询的 p95。
- `internal/database/registry/registry.go`：在 `Acquire` 对同库 `opening` 等待和热 handle 命中分开计时，在 `openAndAcquire` 对真实 `factory.Open` 计时；`internal/database/ducklake/factory.go`：分别记录 `EnsureLocalCatalog`（manifest 发现与下载/校验）、连接 boot/Ping、扩展与版本检测、数据路径校验；`internal/database/ducklake/catalog_syncer.go` 已有 flush/copy/upload/manifest 分段，检查新分段与现有错误类别对齐。每段固定名称，数据库 ID 只允许用于现有内部 debug 日志，不作 Prometheus 高基数 label；不要打印 boot SQL/CREATE SECRET 或用户 SQL。`Open` 完成不等于首次 S3 Parquet 查询完成，首条查询要另计扫描文件与 `httpfs` GET/字节数。
- 本地基准扩展 `benchmarks/dbread/{harness.go,read_test.go,README.md}`：造数维持快照数不变而控制目标表文件数、反之维持文件数而增加快照，拆开小文件与元数据链的影响；增加目标表而非全库文件数、首读/热读、并发 1/8/32、错误率及缓存命中率。使用测试临时目录，不触碰生产 COS；本地冷读不等于远端冷缓存，远端矩阵须在隔离前缀单独执行。
- **P0 验收**：对同一已授权请求建立 `total→auth（含子段）→catalog/registry→systemdb SQL→scan→serialize` 可解释的计时树，并列出后台是否正在 flush/sync/merge。旧日志仅用于定位，不作为新实现改前后的同环境对照；采样含成功与失败，所有指标单位标明（日志 `perf_stage` 毫秒、同步段秒）。

## P1：减少系统库往返，防止播种竞态（P0 确认后）

- **现状**：`internal/api/agent_handler.go:135-153` 的 `ListAgents` 每次调用播种；`internal/systemdb/agents.go` 的 dismissed/existing 批量读取及 `seed_default_agents`/`list_cloud_agents` 低基数观测已完成。不要将既有“8 次点查→2 次按项目读取”写为未来任务。`ArchiveCloudAgent` 在事务内记录 dismissed；DuckLake 对这些表无主键约束，当前批量读与逐项写之间**没有覆盖整个检查-插入窗口**。
- **复现先行**：在测试库用 barrier 让 8/32 个首次列表同时读取缺失 key，再继续写入；统计 `(project_id,builtin_key)` 每个活跃内置 agent 数必须至多 1。再注入播种与归档交错、播种中途写失败，核对 dismissed 不复活、失败后再访问可补齐；只读实例调用列表必须零写。已有顺序测试 `internal/systemdb/agent_history_test.go` 可扩展但不能代替并发测试。
- **条件修复**：若确认重复写或写冲突，在 `systemdb.Store` 为**每项目**播种添加同进程单飞/串行（不要用一把全局锁阻塞所有项目），所有等待者支持 ctx 取消，错误结果允许后续重试；写前对 dismissed/existing 再核对或在库事务边界采用可靠条件检查，避免并发归档后旧检查结果回补。针对实际观测到的冲突类别使用 `ducklake.RetryOnConflict` 有界重试，仅重放幂等逻辑而非裸 INSERT；不能因单进程互斥声称支持跨实例并发写。装配保留只读实例跳过播种和后台同步后的关闭顺序。
- **验收/回退**：单项目并发 8/32、跨项目并发、归档交错、故障重试均无重复及复活；远端对照记录播种调用次数（种子齐全时目标 2 次读 + 1 次列表读）、错误率、P95 和 `systemdb read` 分段。出错时先回退新的并发控制，不回滚已有 agent 数据或改写 dismissed 表。

## P2：按证据优化小文件与冷开，不改变一致性

- **数据面优先**：按表统计活跃 Parquet 文件数（`end_snapshot IS NULL`）、实际 GET/字节数及相同 SQL 的首读/热读；`internal/systemdb/maintenance.go` 已有全库小时级维护和三张 auth 热点表每 15 分钟、≥4 文件定向 merge，先在隔离环境对比 merge 前后 auth_user_load、QueryLogs 和 MetricsSummary 的查询耗时与冲突频率；若目标表持续增长才按表调整阈值/节奏。禁止自动运行 `delete_orphaned_files`；过期与清理保持 7d 门槛及 dry_run。后台 merge 要在 writer 路径执行并纳入 catalog 同步，不能把维护写与前台写混为只读操作。
- **用户库条件优化**：`registry.Acquire` 已单创建者冷开；若 `acquire` 高且开放句柄命中低，先量化 idle close 后重开次数、manifest 探测 GET 和下载，评估只针对热点库的生命周期；若首次查询高而 Open 正常，则拆出 Parquet 冷读。若单库打开失败产生失败波次，再考虑有界 per-DB 可重试错误退避；引擎不匹配、manifest 断链等 fail-closed 故障必须立即返回，不缓存空 catalog 或吞错。相应测试在 `registry_test.go` 追加慢打开、失败波次、等待者取消/恢复；连接池上限 10/idle 2 保持不变，除非窗口等待与 CPU/冲突证据同时支持调整。
- **缓存与合并试验**：仅在隔离环境用 `duckdb_settings()` 核对当前 DuckDB 版本对 external file/Parquet 元数据缓存设置的支持，再在 `internal/database/ducklake/factory.go` 的 boot SQL 中逐项试验并设内存上界，维持 `lock_configuration` 最后执行；以同 SQL、同文件数比较远端请求数、p50/p95/p99 与错误率，不支持即跳过。用户库后台 merge 先做维护频率、writer 限制、sync 水位、关闭/失租与撤销行为的单独设计评审，不能复制过时 v4 的单连接假设或直接上线。
- **P2 验收**：若获益来自降低某表文件数，报告前后文件数，不能声称“同文件数性能优化”；原始慢点无复现时保留观测并停止该分支。远端不在生产前缀造数、合并、删除或重置缓存。

## P3：一致性与计算存储分离的可执行验证

- **既有机制**：`Factory.AfterWrite` 返回成功后本地快照标脏；`CatalogSyncer.captureCatalog` 在写锁内先 flush 内联数据、COPY catalog、检查快照不变；`syncOnce` 先 `PutIfAbsent` 快照，后 `WriteManifest` 发布指针，成功才推进 `last`/`lastSeq`。`TestEngineSyncAndColdStart` 仅把 catalog 存到内存 BlobStore，`DATA_PATH` 仍是原本地目录，**不能**证明全远端 Parquet 已可恢复。
- **测试环境**：明确由运维提供非生产凭据、单独 bucket 或一次性测试 `RootPrefix/Environment`、全新 UUID tenant/database、临时本地 CacheDir；不得复用生产 `Instance.ID` 所映射的命名空间。隔离前缀用独立启动配置，不修改生产 config；测试用例只准清理自己创建的本地临时目录，远端对象默认保留待审计，任何远端删除需另行授权。先以内存 BlobStore 注入阶段失败做确定性协议测试，真实 S3 集成用例必须 opt-in 且不进常规 `go test ./...`。
- **协议断言矩阵**：写入内联/非内联行→强制 Flush→确保 manifest 指向的快照存在且 size/SHA256 正确、快照引用的所有数据对象可 HEAD/读取→切换**全新**缓存恢复后逐行核对；注入数据对象失败、快照上传失败、manifest 写失败时不允许发布不可恢复视图。验证 manifest 连续序号、断链 fail-closed、相同 seq 冲突进入 split-brain、原本地 catalog 领先远端时不得被覆盖；失租库不得再 PUT，不同库/tenant/前缀读写不越界。允许在隔离环境模拟硬中断，但不得操作线上进程或缓存。
- **RPO 测量**：在 `catalog_syncer.go` 随 `MarkDirty` 记录**最旧未成功同步提交时间**；manifest 成功覆盖该水位后清除。暴露低基数持续年龄/告警（按当前指标约束评估，不用 DB ID 作为高基数 metric label）；目前 `SyncLag` 是快照差值而非秒。同步故障期间保持该年龄持续增长并记录恢复耗时，不把 `MaxLag=30s` 写成故障时硬保证。`DurabilityFor` 继续返回 `committed_local`。一致性矩阵写明同进程已提交可读、冷恢复只能读到远端 manifest 已发布快照、双 writer 禁止；没有共享事务 catalog 和可靠 fencing 前不声称多写。
- **架构决策门**：若产品要求独立计算节点任意重建也绝不丢已 ACK 的写入，则现有本地内联 + 异步 catalog 不满足，须另行设计同步远端提交和故障返回语义；若要求多读副本，先核对 Frozen DuckLake 的快照发布/刷新滞后；若要求多写强一致，须引入共享事务元数据服务及一致性协调，SQLite 文件直放 S3、把 catalog_engine 换为 SQLite 或开默认关闭的租约均不构成完整方案。架构迁移不包含在这轮性能优化里。

## P4：验证、上线与回退清单

| 维度 | 当前可用基线 | 验收与回退门槛 |
| --- | --- | --- |
| 认证未命中 | 日志个例 `auth_user_load=5.121s`；锁等待接近 0 | 同环境同快照、同账号类型比较 owner/follower p50/p95/p99，故障波次不能因新单飞失控增大回查次数；权限撤销行为须保持原边界 |
| `GET /agents` | 日志个例 3.039s；已完成批量播种读理论上最多 8→2 次 | 同环境验证实际系统库调用数、`seed_default_agents` 与 `list_cloud_agents` 耗时，种子齐全时最多 3 次读取；首次写入与归档交错无重复/复活；未真实 COS 复测前不填写延迟提升倍数 |
| `GET /logs` / `metrics/summary` | 本地日志热读 p50 2.293ms（67 个全库文件）；远端日志样本另计 | 目标表文件数与查询触达文件数分开记录，冷热/并发测试同环境三轮、每轮 ≥100 个成功样本并另报全部失败数；不只报告命中缓存的均值 |
| 冷打开 / 同步 | 某次 KV 首次请求 1.55s，其中 acquire 246ms；另一次同步上传 213s 失败 | 明确首次与热态样本、首次查询额外 Parquet GET；错误故障下恢复水位单调且最旧未同步提交年龄可观测，远端失败不能默认为 RPO≤30s |

每阶段执行针对性 `go test -race ./internal/auth ./internal/systemdb ./internal/api`、`go test -short ./internal/database/ducklake ./internal/database/registry ./benchmarks/dbread`；收尾执行 `go build ./internal/... ./cmd/...`、`go test ./...`、`go vet` 相关包，检查日志脱敏和 `git diff --check`。上线先仅放观测，隔离 COS 试验获数据后逐项灰度；每次只回退本阶段行为改动，不主动回滚 catalog 或删除 Parquet。生产端点不可达、测试前缀/凭据未获授权时标记远端验收**未执行**，不能用本地基准冒充。

## 2026-10-10 本轮实施与未验收项

- **已修改**：`auth` 的用户点查仅在启用阶段观察者时按同一连接计 `auth_user_conn/query/scan`，并新增单飞等待/owner 加载子段；follower 取消立即退出，同一 owner 失败波次共享错误，不永久缓存失败。系统库日志读记录 query/scan，registry 记录热命中、同库等待与真实 Open，DuckLake Open 记录 catalog 对账/boot/校验时间。以上子段不加入顶层归因。
- **并发正确性**：同项目 agent 播种及归档走进程内串行区，等待者支持取消；32 并发首次播种测试核对每个 builtin key 至多一条活跃记录。该约束不解决多实例同时写；归档后不复活的顺序测试仍保留。
- **本地测量**：`TestReadLatencyPhases` 在本地临时库、20 次日志 flush 下，inlining=0 时目标表 `sys_log_events` 有 20 个活跃文件（全库 67）：热态顺序 100 次 p50 2.227ms/p95 2.890ms/p99 3.186ms，32 并发 128 次 p50 18.852ms/p95 38.952ms/p99 50.558ms；inlining=1000 时目标表 0 个活跃文件：顺序 p50 1.388ms/p95 1.933ms/p99 1.972ms，32 并发 p50 9.739ms/p95 21.000ms/p99 27.097ms；各组失败 0。文件数、快照数与缓存冷热尚未独立控制，此数据**不能**当作远端收益或单因素因果证明。连接数、TTL、合并节奏、缓存开关未调整。
- **同步故障测试**：内存 BlobStore 对快照 PUT 和 manifest PUT 分别注入失败，验证远端未发布新 manifest、同步水位不推进；恢复后重新 Sync 成功并清除进程内最旧未同步提交年龄。新增 `OldestUnsyncedAge` 仅为进程内估计，重启后 0 不等于无未同步提交，manifest 故障持续时也不构成 30s 硬 RPO。现有 `TestEngineSyncAndColdStart` 仍仅测试本地 DATA_PATH 的 catalog 恢复，全远端 Parquet 冷恢复**未执行**。
- **验证**：`go build ./internal/... ./cmd/...`、`go test ./... -short -timeout 20m -parallel 1`、定向 `go test -race`（auth/systemdb/api/registry）及相关 `go vet` 通过。并行 `go test -short ./internal/database/ducklake` 在此机器触发默认 10 分钟超时；限制 `-parallel 1` 的短测试通过。`go test -race -short` 的 API 全包运行发现现有 `TestAgentSkillsE2E` 的 `agent_handler.go:744/814` 数据竞争；定向本轮相关 race 用例通过，未将该无关竞态归为本次已修复。隔离 COS 凭据/前缀未提供，真实远端请求数、故障恢复与端到端 p95 **未验收**，不能宣称线上延迟改善。

## 慢因复查与方案合理性评估（2026-10-10 复核）

本节是对上面 P0–P4 的**批判性复核**：慢因分类基本成立，但方案有若干处高估或遗漏，新增两项此前未识别的结构性问题。

### 一、慢因分类：哪些成立、哪些证据不足

| 结论 | 复核判定 |
| --- | --- |
| 认证锁 convoy 已消除、失败波次已收敛 | **成立**。代码已是锁外 I/O + 按用户单飞 + 失效代数；本轮又补了 follower 取消与同批共享错误。 |
| `auth_user_load` 1.4–5.1s 是远端小文件点查放大 | **方向成立、量级未证实**。本地热态点查仅约 1.4ms，说明延迟来自远端每文件往返；但**没有** `httpfs` 请求数或 `ducklake_data_file` 目标表文件数的同期实测，仍属推断。 |
| 系统库连接池排队 | **不成立**。多次采样 `wait_count_total=0`，且 DuckDB 内部等待不计入 `sql.DBStats`，扩大连接池无据。 |
| `/agents` 播种重复往返 | **成立且已修**（8→2 次）。 |
| 小文件读放大 | **成立**，本地基准确认单文件线性（≈35µs/文件本地），远端放大为 RTT 级。 |
| 用户库冷开慢 | **部分成立**。日志见首次 KV 1.55s（acquire 246ms）后热请求 4ms；但仅单例，且未区分是 `factory.Open`、远端 catalog 下载还是首次 Parquet 读取。 |

**遗漏的一项慢因**：`GET /agent-threads/:threadID/messages`（日志个例 1.20s，`unattributed≈total`）。其 SQL 是「子查询 `ORDER BY created_at DESC, id DESC LIMIT 200` + LEFT JOIN `sys_agent_runs` + 外层重新排序」，在 DuckLake 上无法靠索引优化，LIMIT 也不构成文件剪枝保证；`ListAgentMessages` 当时**没有** `observeRead` 观测，因此它在日志里只能表现为未归因时间。P0.2 的观测清单未包含它。

### 二、复核中发现的**结构性缺陷（此前计划未识别）**

1. **日志保留策略没有被真正执行。** `GetRetention`／`PutRetention`／`SeedGlobalRetention` 只读写 `sys_log_retention.keep_days`，全仓库**没有任何** `DELETE FROM sys_log_events` 或等价清理路径（`internal/systemdb/logs.go` 仅有 INSERT 与 SELECT）。这意味着 `sys_log_events` 只增不减：`QueryLogs` 默认只查最近 24h 可缓解读取，但**文件数与 catalog 体积会随运行时间无界增长**，直接抬高每次远端点查的元数据规模。这是"日志页慢"的长期根因之一，也是 `keep_days` 配置给人"已控制"的错觉。**修复需谨慎**：删除会推进 snapshot 并触发 flush/merge，须在写者上按保留窗口分批执行，并纳入 catalog 同步与冲突重试。
2. **`sys_metric_samples` 同理只增不减，且 `sys_metric_rollups_hourly` 是死表。** 迁移建了小时汇总表（migration v11），但全仓库**没有**任何写入或读取它的代码（仅 `migrate.go` 出现）；`MetricsTrend`／`MetricsSummary` 始终直接扫 `sys_metric_samples` 的原始行。因此小时 rollup 的设计意图（缩小扫描范围）**未落地**，趋势查询随保留窗口线性变慢。

这两项属于"数据无界增长"，与本计划 P2 的"小文件治理"是不同层面的问题：merge 只减少**文件数**，不减少**行数与快照数**，长期仍会退化。

### 三、方案设计是否合理

**合理的部分**：先观测后改动、区分子阶段不重复归因、跨实例多写不靠切 catalog 解决、不默认启用破坏性维护、`committed_local` 与 RPO 边界表述准确——这些与 DuckLake 官方语义一致（见上文 C.2／C.7／C.13／D.4）。

**需修正的部分**：

| 问题 | 说明 |
| --- | --- |
| P2 优先级偏低 | 计划把"无界增长"隐含在 P2 的 merge 里；但行数无界是**结构与容量**问题，应先于调连接池/缓存等微调。建议单列为更高优先级，并明确它属于"功能缺失"而非"性能优化"。 |
| "证据触发"可能永不触发 | 若无隔离 COS 环境，P2/P3 的多数条目**无数据可依**，计划会停在 P0。应明确"没有远端环境时的默认动作"（例如先只落观测与本地回归），避免计划悬空。 |
| 缓存与 TTL 组合未审计 | `MetricsSummaryCached` 是 10s TTL；`principalCacheTTL` 30s。两者叠加会让"用户改权限后 30s 内仍可看旧指标"，需确认这是否符合预期（当前注释只覆盖认证，未讨论指标可见性）。 |
| 排序表/分区未纳入 P2 | 官方 C.10/C.5（见资料整理）提供 min/max 剪枝，对 `sys_log_events.occurred_at`、`sys_metric_samples.occurred_at` 这类**时间序列表**理论上有效；计划完全未提。应作为"可评估项"列出，但**须先证明文件级剪枝确实生效**（当前 SQL 无 `PARTITIONED BY`/`SORTED BY`）。 |
| 未评估 `unattributed` 的共性 | 多个接口 `unattributed≈total`（agents、metrics、messages），说明**这些 handler 根本没有细分打点**。P0 只列了 `sys_users`/`QueryLogs`/`MetricsSummary`，应先把"凡 `unattributed` 占比高的路由都补段"作为通则，而不是逐个事后补。 |

### 四、修订后的优先级建议

1. **P0-补**：为 `ListAgentMessages`、`ListCloudAgents`、`MetricsTrend` 补 `observeRead`；把"路由 `unattributed` 占比 > 50% 就补段"写成通则。此前已落地 query/scan 拆分的 `QueryLogs` 只覆盖一个入口。
2. **P1（新）**：**先决定 `sys_log_events`／`sys_metric_samples` 的保留与 rollup 策略**——要么实现带保留窗口的清理任务（写者、分批、纳入同步），要么明确"永久保留"并改用 rollup 收敛扫描量。这是容量与长期延迟问题，不应拖到 P2。
3. **P2**：`sys_agent_messages` 等会话表同样无清理，需一并纳入上一条的保留策略讨论。
4. **P3**：在有隔离前缀时，测 `PARTITIONED BY`/`SORTED BY` 对时间列剪枝的实际收益，及 external file / Parquet metadata cache 的版本支持（须先 `duckdb_settings()` 核对，`lock_configuration` 仍最后）。
5. **P4**：维持原有验收与回退门槛。

**边界**：以上第 2/3 项涉及**删除生产数据**，必须在独立前缀验证、写者限定、分批、可回退，且不得与本次只读观测混发；本计划**不授权**执行，仅记录为待评审项。第 1 项属纯观测，风险低，可先做。

## DuckLake 官方原始资料详细整理

> **整理口径**：以下按 `ducklake.select/docs/stable/` 的官方页面逐页整理**原始地址 + 关键技术点**，并标注函数签名、参数、默认值与限制边界；不转载文档全文，只做原创概括和必要术语保留。所有内容核对于 2026-10-10，页面页脚的 `© 2026 DuckDB Foundation` 是**版权年份，不是发布时间**，官方页面也未标注作者与发布日期。`stable` 路径会随版本变化；固定结论须同时记录 DuckDB 与 `ducklake` 扩展版本。文中出现的 SQL 均为官方示例，未在本机执行。仓库内另有既存镜像 `docs/database/ducklake.md`（节标题形如 `specification:introduction`、`duckdb:introduction`、`advanced_features:data_inlining`、`advanced_features:transactions`、`maintenance`），可离线对照，但以在线原始页面为准。

### A. 格式规范（Specification）

#### A.1 导言

- 原始页面：<https://ducklake.select/docs/stable/specification/introduction>
- 页面标注规范版本 **1.0**，由两个组件构成：**catalog 数据库**（保存表结构、快照、文件引用等元数据）与**数据存储**（保存表数据）。
- catalog 的硬性要求：必须是支持 **SQL-92 事务与主键约束**的数据库；DuckLake 用普通 SQL 表和查询定义 catalog 信息。
- 数据文件采用 **Parquet**，可存放于对象存储、块存储或文件存储。
- 该页**未**规定：具体目录布局、快照生成与编号方式、并发读写行为。这些细节分别由「元数据表总览」「查询规则」和扩展文档的「事务／冲突解决」承担，不能仅凭导言推断。

#### A.2 元数据表总览

- 原始页面：<https://ducklake.select/docs/stable/specification/tables/overview>
- 页面给出 v1.0 catalog 的建表定义。与文件定位相关的关键字段：

| 元数据表 | 主键 | 与文件／路径相关的字段 |
| --- | --- | --- |
| `ducklake_snapshot` | `snapshot_id` | `snapshot_time`、`schema_version`；无路径字段 |
| `ducklake_schema` | `schema_id` | `path`、`path_is_relative`、`begin_snapshot`、`end_snapshot` |
| `ducklake_table` | — | `table_id`、`schema_id`、`path`、`path_is_relative`、`begin_snapshot`、`end_snapshot` |
| `ducklake_column` | — | 列类型定义在 `column_type`；嵌套类型通过 `parent_column` **递归**描述 |
| `ducklake_data_file` | `data_file_id` | `table_id`、`path`、`path_is_relative`、`begin_snapshot`、`end_snapshot` |
| `ducklake_delete_file` | `delete_file_id` | `table_id`、`data_file_id`、`path`、`path_is_relative`、`begin_snapshot`、`end_snapshot` |
| `ducklake_file_column_stats` | — | 文件级列统计（min/max、null 计数），供剪枝 |
| `ducklake_inlined_data_tables` | — | `table_id`、`table_name`、`schema_version`；用于存放内联数据片段，**无路径字段** |

- 页面明确的一点：schema、table、data_file、delete_file 各自记录 `path`（`VARCHAR`）与 `path_is_relative`（`BOOLEAN`），但**未**声明这些路径如何逐级拼接，建表语句中也**未声明外键**。相对路径的解析规则在「路径」页（见 B.5）。
- 本项目的核验口径：判断一条查询要付多少次远端对象读取，应查**目标表**的 `ducklake_data_file` 存活行，而不是全库计数（`end_snapshot IS NULL` 只表示当前仍有效）。

**规范元数据表完整清单**（v1.0 catalog，按官方导航；页面对应 `specification/tables/<表名>`）：

| 表 | 用途要点 |
| --- | --- |
| `ducklake_metadata` | catalog 级配置与全局选项（如 `data_path`、`expire_older_than`、`rewrite_delete_threshold`） |
| `ducklake_snapshot` | 快照版本（`snapshot_id` 主键、`snapshot_time`、`schema_version`） |
| `ducklake_snapshot_changes` | 每个快照的变更集，冲突检测与变更流依赖它 |
| `ducklake_schema` / `ducklake_schema_versions` | schema 及版本 |
| `ducklake_table` | 表定义与路径 |
| `ducklake_column` | 列定义（`column_type`、`parent_column`、`initial_default`、`column_id`） |
| `ducklake_column_mapping` / `ducklake_name_mapping` | 列/名称映射，支撑 field id remapping 与重命名 |
| `ducklake_data_file` | 数据文件（`table_id`、`path`、`begin/end_snapshot`、`encryption_key`） |
| `ducklake_delete_file` | 删除文件（关联 `data_file_id`） |
| `ducklake_file_column_stats` / `ducklake_table_column_stats` | 文件级与表级列统计（剪枝依据，字符串编码 + `extra_stats`） |
| `ducklake_file_variant_stats` | `variant` 子字段的逐文件统计 |
| `ducklake_file_partition_value` / `ducklake_partition_info` / `ducklake_partition_column` | 分区值与分区定义 |
| `ducklake_sort_info` / `ducklake_sort_expression` | 排序表定义（见 C.10） |
| `ducklake_inlined_data_tables` | 内联数据片段（见 C.4） |
| `ducklake_files_scheduled_for_deletion` | 待删文件登记表（见 D.4，清理的前置） |
| `ducklake_macro` / `ducklake_macro_impl` / `ducklake_macro_parameters` | 宏定义、实现与参数（见 C.14） |
| `ducklake_view` | 视图定义（见 C.14） |
| `ducklake_tag` / `ducklake_column_tag` | 表/视图与列上的注释（见 C.14） |

- 该清单用于按需定位元数据；**具体字段以在线规范页为准**，本计划未逐一核对全部字段与约束。

#### A.3 查询与可见性规则

- 原始页面：<https://ducklake.select/docs/stable/specification/queries>
- 统一可见性条件（起点包含、终点不包含）：

```sql
: sid >= begin_snapshot
AND ( end_snapshot IS NULL OR : sid < end_snapshot )
```

- 当前快照通常取 `SELECT max(snapshot_id) FROM ducklake_snapshot`；时间旅行把 `:sid` 换成历史 snapshot ID，并对所有带有效期的元数据套用同一条件。
- 读取一个表时：先筛该快照可见的 `ducklake_data_file`，再 **LEFT JOIN** 同一快照可见的 `ducklake_delete_file`（按 `data_file_id` + `table_id` + 有效期）。必须保留 LEFT JOIN，没有删除文件的文件也要读；同一快照内一个数据文件至多对应一个删除文件。重建结果 = 读数据文件的行 − 删除文件标记的行。
- 路径解析需依据各自的 `path_is_relative` 判断是否相对 catalog 的 `data_path`。
- 剪枝：在**已按快照选出的**数据文件上，用 `ducklake_file_column_stats` 的 `table_id`、`column_id`、`data_file_id` 及列 min/max 缩小范围，例如等值查询可保留满足 `(:value >= min_value OR min_value IS NULL) AND (:value <= max_value OR max_value IS NULL)` 的文件。注意三点：min/max 以**字符串**存储，数值列比较前必须转换类型；统计缺失或边界为 `NULL` 时**不能**据此排除文件；剪枝只决定"可能读哪些文件"，**不能替代**读取后应用删除标记与查询谓词。

#### A.4 数据类型与统计编码

- 原始页面：<https://ducklake.select/docs/stable/specification/data_types>
- 列类型定义在 `ducklake_column.column_type`；嵌套类型不使用单一字段，例如 `INT[]` 存为**两行**——一个 `list` 父列 + 一个 `int32` 子列，子列 `parent_column` 指向父列。
- 支持的类型：
  - 原始类型：`boolean`、`int8/16/32/64`、`uint8/16/32/64`、`int128/uint128`、`float32/float64`、`decimal(P,S)`、`time`、`timetz`、`date`、`timestamp`、`timestamptz`、`timestamp_s/ms/ns`、`interval`、`varchar`、`blob`、`json`、`uuid`。
  - 嵌套：`list`、`struct`、`map`；半结构化：`variant`；几何：`geometry`（含 `point`、`linestring`、`polygon`、`multipoint`、`multilinestring`、`multipolygon`、`linestring_z`、`geometrycollection`）。
  - 精度口径：`time`/`timetz`/`timestamp`/`timestamptz` 为微秒；`interval` 有 months／days／milliseconds 三种粒度。`variant` 以二进制编码（非字符串），可承载 `date`、`timestamp`、`decimal` 等更宽类型。
- 与 Parquet 明确相关的只有三处：`variant` 按 Parquet variant encoding 存储；`geometry` 用 Parquet 的 `geometry` 类型；`int128`、`uint128`、`timetz`、`interval` **当前没有 min/max 统计**（页面给出的原因是底层 Parquet 格式不支持）。页面**未**给出完整的"逻辑类型 → Parquet 物理类型"映射表。
- 统计值统一以**字符串**存于 `VARCHAR`（便于容纳 catalog 数据库本身不原生的类型）：整数为整数串，浮点为数值串（`inf`／`-inf`，`NaN` 排除在 min/max 外并用 `contains_nan` 标志表示），`decimal` 为数值串，日期时间为 ISO 8601（支持 `infinity`／`-infinity`，`timestamptz` 带 UTC 偏移），`blob` 为**十六进制编码**，`varchar`／`json`／`uuid` 按原样。
- 嵌套类型自身没有 min/max，统计**递归在子列**上；`geometry` 与 `variant` 不用字符串 min/max，而是以 **JSON** 存于 `extra_stats`（`geometry` 含包围盒 `bbox` 与 primitive 类型列表；`variant` 为 shredded 子字段数组，无法 shredded 时为 `NULL`；`variant` 的子字段逐文件统计在 `ducklake_file_variant_stats`）。
- catalog 后端差异：DuckDB catalog 可直接存全部 DuckDB 类型；PostgreSQL catalog 把 `UBIGINT`、`HUGEINT`、`UHUGEINT`、日期时间与嵌套类型落到 `VARCHAR`，`VARCHAR`／`JSON` 落到 `BYTEA`（但有"不能存空字节"的限制）；SQLite catalog 只有五种存储类，整数统一映射到 `INTEGER`，多数其他类型落到 `TEXT`，只有 `BLOB` 存为 `BLOB`。
- 该页明确写出的限制：`int128`／`uint128`／`timetz`／`interval` 无 min/max；嵌套类型自身无 min/max；**没有原生 variant 类型的 catalog（如 PostgreSQL、SQLite）目前不支持把 `variant` 作为内联值**；`NaN` 不参与 min/max。

### B. DuckDB 扩展：入门、连接与存储

#### B.1 扩展简介（入门）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/introduction>
- 安装：先装最新版 DuckDB，再 `INSTALL ducklake;`；使用时不单独 `LOAD`，直接以 `ducklake:` 前缀 `ATTACH`。
- 使用前必须决定两件事：**用哪种 catalog 数据库**（B.3）与**数据文件放哪里**（B.4）。
- 最小用法与默认布局：

```sql
ATTACH 'ducklake:my_ducklake.ducklake' AS my_ducklake;
USE my_ducklake;
```

  生成 `my_ducklake.ducklake`（一个带 DuckLake schema 的 DuckDB 数据库），插入数据后产生 `my_ducklake.ducklake.files` 目录存放 Parquet。
- 换数据目录：`ATTACH 'ducklake:my_other.ducklake' AS o (DATA_PATH 'some/other/path/')`；**`DATA_PATH` 记录在元数据里**，重新附加已有 catalog 无需重复指定。
- 操作范围与 DuckDB 基本一致（建 schema／表、插入、更新、删除、改表结构），另有 `snapshots()`、时间旅行 `AT (VERSION => 1)`，以及可直接用 glob 读 Parquet／delete 文件。
- 更新语义：`UPDATE` 被建模为**删除 + 插入**——原数据文件、删除标记文件（形如 `ducklake-*-delete.parquet`，记录被删行号）、新值文件三份并存。这一点解释了下文维护章节为何需要 `rewrite_data_files`。
- 已知限制与前置条件：不支持索引、主键、外键、`UNIQUE`、`CHECK` 约束（与其他 lakehouse 格式一致）；`DATA_PATH` 与数据库文件路径应使用**相对路径**；创建时**父目录必须已存在**；需显式指定库名或 `USE`，否则可能误用内存库；`DETACH` 前先 `USE memory;`；**DuckLake v1.0 要求 DuckDB v1.5.2 及以上**。

#### B.2 连接

- 原始页面：<https://ducklake.select/docs/stable/duckdb/usage/connecting>
- 通用形式：`ATTACH 'ducklake:<catalog 位置>' (DATA_PATH '<数据目录>');`
- catalog 形式：DuckDB 文件 `ducklake:duckdb_database.ducklake`；PostgreSQL `ducklake:postgres:dbname=postgres`（页面用它示范 catalog 在 PostgreSQL、数据在 `s3://`）；只读连接用 `(READ_ONLY)`；也可通过 secret `ATTACH 'ducklake:';` 或 `ATTACH 'ducklake:my_secret';`。
- 关键默认与陷阱：
  - `CREATE_IF_NOT_EXISTS` 默认为 **true**：指到不存在的 catalog 会自动创建。**这解释了本项目必须保留实例格式标记与 manifest 校验**——打开错误路径可能静默产生空 catalog。
  - `DATA_PATH` 只在创建时必需；连接已有 DuckLake 时从 catalog 读取已保存路径。
  - `OVERRIDE_DATA_PATH true` 是**当前连接的临时覆盖**，不修改元数据；在该连接中原 `DATA_PATH` 下的数据**将无法查询**。页面参数表另注：当传入的 `DATA_PATH` 与已保存路径不同时，`OVERRIDE_DATA_PATH` 默认即为 true。
  - 该页**未**给出 SQLite catalog 的连接格式，不能据此断定 SQLite 的写法与创建行为。

#### B.3 选择 catalog 数据库

- 原始页面：<https://ducklake.select/docs/stable/duckdb/usage/choosing_a_catalog_database>
- 官方推荐按客户端范围划分：

| 后端 | 官方推荐场景 | 多进程／多客户端读写 |
| --- | --- | --- |
| DuckDB | 单个客户端的本地数据仓库 | **仅限单客户端**，不适合作为多客户端共同写入的 catalog |
| SQLite | 多个**本地**客户端使用的本地数据仓库 | 可多进程，但**同一时刻只有一个写者**；DuckLake 采用 attach／detach 并在遇到写锁时重试，这**不等于并行写入** |
| PostgreSQL（≥12） | 多用户、客户端可能在远端的 lakehouse | 官方为共享场景推荐的后端 |

- **不要把 S3 当作共享 SQL catalog 本身。** 该页示例把 catalog 指向 DuckDB／SQLite 文件或 PostgreSQL 服务，数据位置另由 `DATA_PATH` 指定；S3 更适合作为与 catalog 分离的数据文件存储。页面未给出 S3 路径的配置语法，`data_files/` 示例不能当作 S3 配置指南。
- 三者都是 **catalog 后端**，查询计算仍由 DuckDB 执行。**本仓库当前没有 PostgreSQL catalog 装配**（详见正文第 5 节对照表）。

#### B.4 选择数据存储

- 原始页面：<https://ducklake.select/docs/stable/duckdb/usage/choosing_storage>
- 核心性质：DuckLake **不修改／不追加已有文件**，已有文件保持不可变。这降低了对文件系统一致性的要求，也让缓存更容易。
- 可用后端：本地文件、S3 及兼容对象存储、GCS、Azure Blob，以及 NFS／SMB／FUSE 等网络文件系统。取舍：本地快但不易共享；对象存储可扩展但有传输费用、远端延迟更高；站点内存储服务器／网络文件系统是折中。
- 该页**未**说明 `DATA_PATH` 的默认值或具体语法，也**未**规定数据必须与 catalog 同目录或必须分离。分离部署时必须同时管理两处的权限、可达性与备份。
- 只读使用者只需读取权限；写入与维护任务另需创建／列举／删除权限，应按实际操作最小化授予。文件不可变**不等于**写入不需要权限。

#### B.5 路径规则

- 原始页面：<https://ducklake.select/docs/stable/duckdb/usage/paths>
- `data_path` 是文件路径的**根**；新建时可通过参数指定，加载已有 DuckLake 时若未提供则从 `ducklake_metadata` 读取。
- 默认写入**相对路径**：schema 相对 `data_path`，table 相对 schema 路径，数据文件与删除文件相对 table 路径；各自记录在 `path` 与 `path_is_relative`。
- 目录命名：默认情况下，名称由字母数字组成的 schema／table 使用名称作目录，否则使用 **UUID** 目录。这不是"路径编码规范"，不能把 UUID 回退当作转义规则。
- 分区文件默认放在 Hive 风格目录（如 `year=2025/`），但分区值同时记录在 catalog 中，不依赖目录形式。
- 该页**未**规定迁移／搬迁时的自动重定位行为：解析仍从 `data_path` 逐级向下，不能断言"改 `data_path` 即可完成迁移"，尤其对绝对路径。

#### B.6 配置选项

- 原始页面：<https://ducklake.select/docs/stable/duckdb/usage/configuration>
- 两类配置：`ducklake_` 前缀的**扩展级**选项用 DuckDB `SET`；DuckLake 专属选项通过附加实例的 `set_option` 设置并**持久化在 `ducklake_metadata`**。专属选项可按**表 > schema > 全局 > 默认值**优先级设置。
- 页面列出的关键项与官方默认值：

| 选项 | 官方默认 | 含义 |
| --- | --- | --- |
| `data_inlining_row_limit`（及扩展级 `ducklake_default_data_inlining_row_limit`） | **10** | 单次插入可内联的最大行数；设 `0` 禁用内联 |
| `target_file_size` | **512MB** | 插入与压缩的**目标**文件大小，页面未称其为硬上限 |
| `parquet_compression` | **snappy** | 另有 `parquet_compression_level`，默认 3 |
| `ducklake_max_retry_count` | **10** | 事务冲突最大重试次数 |
| `ducklake_retry_backoff` | **1.5** | 指数退避系数 |
| `ducklake_retry_wait_ms` | **100** | 重试间隔毫秒 |

- 该页**未列出** `OVERRIDE_DATA_PATH` 与 `AUTOMATIC_MIGRATION`（两者出现在「连接」与「排障」页，见 B.2、F.4），不能把 `data_path` 与该名称等同。
- 提醒：本仓库当前配置为内联 **1000**、`target_file_size` **64MB**、压缩 **zstd**，是**项目默认值**，不能说成官方默认值（见正文第 5 节对照表）。

### C. DuckDB 扩展：事务、并发与可见性

#### C.1 事务

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/transactions>
- 支持 **ACID** 与对所有交互的**快照隔离**；`CREATE TABLE`、`ALTER TABLE` 等操作也在事务范围内。
- 提供 `BEGIN TRANSACTION` / `COMMIT` / `ROLLBACK`（`ABORT` 等义）。**一个已提交的事务（`BEGIN`–`COMMIT` 块）对应一个快照**。
- 事务具有"全部成功或全部不生效"语义，可包含多项更改；但该页**未单独说明多表更改的原子性**。
- 页面**未**说明自动提交行为，也**未**说明 catalog 元数据与数据文件的写入次序——因此不能从该页推断"数据文件必然先于 catalog 提交"或反之。

#### C.2 冲突解决

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/conflict_resolution>
- 该页描述的是**DuckDB 扩展**的行为，不能当作所有 DuckLake 实现的共同保证。
- 机制：`ducklake_snapshot.snapshot_id` 是**主键且从 0 递增**；两个连接并发提交可能尝试同一 snapshot ID，后者因主键冲突无法直接提交。扩展随后查询 `ducklake_snapshot_changes` 判断期间是否产生**逻辑冲突**：无逻辑冲突则在**元数据 catalog 中重试事务**，不必重写已生成的数据文件；存在逻辑冲突则中止并报错。并非所有主键冲突都可自动重试。
- 冲突范围（页面举例）：并发向同一张表插入是可重试的示例；插入遇到另一事务的删表／改表／数据删除会冲突；删除遇到删表／改表／插入／压缩也会冲突。因此**不能**概括为"同表并发写入一律允许"或"一律冲突"。
- 删除冲突的判定边界是**底层数据文件**：两个快照删除同一数据文件中的数据会冲突，删除同一表中**不同**数据文件的数据可并发。页面**未承诺**按具体"行"精确判定，所以删除不同行但落在同一文件仍属所列冲突情形。
- 页面还列出并发创建同名 schema／表／视图，以及与删除／修改／压缩交错产生的若干逻辑冲突。**其他引擎或 DuckLake 实现**是否采用相同的检测与重试策略，该页未说明。

#### C.3 快照与时间旅行

- 原始页面：<https://ducklake.select/docs/stable/duckdb/usage/snapshots>、<https://ducklake.select/docs/stable/duckdb/usage/time_travel>
- 快照代表对 DuckLake 的一次提交；对数据库的更改必须伴随快照。`snapshots()` 列出快照 ID、时间与变更集；`current_snapshot()` 返回最新快照 ID；`last_committed_snapshot()` 返回**现有打开连接**的最新已提交快照 ID（页面称它在多连接更新同一目标时可能有用，示例中新连接调用返回 `NULL`）。该页**未**说明读连接何时刷新、是否自动看到其他连接的新提交，**不能**把该函数解释为刷新机制。
- 时间旅行：单表 `SELECT * FROM tbl AT (VERSION => 3)` 或 `AT (TIMESTAMP => now() - INTERVAL '1 week')`；按历史状态挂载 `ATTACH 'ducklake:metadata.duckdb' (SNAPSHOT_VERSION 3)` 或 `(SNAPSHOT_TIME '2025-05-26 00:00:00')`。
- 保留边界：DuckLake 会保留历史 snapshot 及其变更集，**除非触发 compaction 且明确删除历史 snapshot**。因此不能假设历史版本永久可查；已删除的 snapshot 不再是有效 snapshot。页面**未**说明默认保留期限，也**未**说明时间戳如何精确对应到 snapshot。

#### C.4 数据内联（Data Inlining）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/data_inlining>
- 何时启用：**默认启用**，行数上限 **10**；一次插入或删除影响的行数**少于**阈值时内联，达到或超过则写 Parquet；设 `0` 禁用。可通过 DuckDB 全局默认值、每次 `ATTACH` 参数或持久化的表／schema／湖级选项配置，**持久化选项优先**，`ATTACH` 参数不持久化。
- 存在哪里：小批插入写入 **catalog 中**按表、按 schema 版本管理的内联数据表；对已有 Parquet 行的少量删除写入 catalog 的**内联删除表**；删除原本内联的行则更新该行的 `end_snapshot`。它们**不在**数据路径的 Parquet 文件中。
- Flush：`ducklake_flush_inlined_data` 可按整个湖、schema 或单表把内联内容写成 Parquet。内联插入生成数据文件；其中已删除的行还会生成带删除快照信息的 *partial deletion file*；针对已有 Parquet 的内联删除则生成或合并删除文件以保留时间旅行能力。有排序键时按 flush 当时的排序设置输出。对整个湖或 schema 执行 flush（含通过 `CHECKPOINT`）会**跳过 `auto_compact=false` 的表**；`auto_compact` **不会在每次写入后自动触发 flush**。
- 远端 catalog 恢复：该页**未**给出远端 catalog 的专门恢复步骤或一致性保证；但因未 flush 的行与删除记录保存在 catalog，**仅恢复数据路径中的 Parquet 不足以恢复这些变更**，备份与恢复必须覆盖 catalog 并注意两者一致性。DuckDB／PostgreSQL／SQLite 均支持内联；**非 DuckDB catalog 不支持对含 `VARIANT` 列的表内联**。
- 权衡：内联避免为每次微小变更创建 Parquet，减少小文件；代价是变更改写并留存在 catalog。若 catalog 在远端，访问延迟、catalog 负载与备份范围更值得关注；该页**未**提供远端延迟的量化数据。

#### C.5 分区与文件剪枝

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/partitioning>
- 可为表设置分区键，新写入数据按分区键拆分到不同文件；查询规划依据文件级 zone map（列 min/max）决定扫描哪些文件。对 `identity`、`year`、`month`、`day`、`hour` 这类**保序**分区，通常能形成较紧取值范围，从而剪掉不匹配文件，**有机会**减少对象读取与 GET 请求。
- 边界：**分区不是索引**。点查只有在条件能帮助排除文件时才明显受益，命中文件仍需读取；页面未给出 GET 降幅或点查延迟保证。支持 `bucket(N, col)`，但页面未声称它能像保序变换一样依靠紧密 min/max 有效剪枝。**更改分区键只影响新写入数据**，旧文件保持原布局，不能假定整表都按新规则剪枝。分区键记录在元数据中，不一定要写入文件或路径。

#### C.6 不支持的功能

- 原始页面：<https://ducklake.select/docs/stable/duckdb/unsupported_features>
- 需区分"**规范不支持**"与"**DuckDB 扩展尚未实现**"：该页列出的属于前者，未给出扩展未实现功能的清单。
- 不支持且未来不太可能支持：**索引**、强制执行的主键／`UNIQUE`／外键（文档提到可能考虑**不强制执行**的主键）。
- 规范层面的其他限制：用户定义类型、定长 `ARRAY`、`ENUM`、`CHECK` 约束、非字面量默认值、生成列（列为未来可能支持）；序列、`VARINT`、`BITSTRING`、`UNION` 类型（列为未来不太可能支持）。`DROP ... CASCADE` 目前不会删除视图等依赖对象。
- **易误读点**：`MERGE INTO` **是支持的**（因不支持主键，文档称 Upsert 只能通过它实现）；`UPDATE`、`DELETE` 该页**未**列为不支持。
- 与格式规范的区分：这里的"不支持主键"指 **DuckLake 用户数据表**；**catalog 数据库本身必须支持主键**（见 A.1），两个层次不能混为一谈。

#### C.7 约束（Constraints）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/constraints>
- 页面开宗明义：DuckLake 对约束的支持**非常有限**。**只支持 `NOT NULL`**；`PRIMARY KEY`、`FOREIGN KEY`、`UNIQUE`、`CHECK` 四类**全部不支持**。
- 语法：建表时 `CREATE TABLE tbl (col INTEGER NOT NULL);`；已有列 `ALTER TABLE tbl ALTER col SET NOT NULL;` / `DROP NOT NULL`。
- 该页**未**说明 `NOT NULL` 的执行时机、是否回填校验存量数据、违规时的报错行为；schema 演进的存量数据行为也未说明（见 C.8）。
- 实务影响：没有 PK／UNIQUE 意味着**没有靠数据库层保证的去重与唯一索引**，业务键唯一性必须由应用层或写入流程自行保证；没有 FK 与 `CHECK`，引用完整性与业务规则校验只能在上游做。
- 与本项目的关联：`sys_*` 表在 DuckLake 中同样**不能依赖数据库级唯一约束**，这正是仓库里"存在性由应用层判断"（如内置助手播种、dismissed 表）以及 `archive` 判重逻辑存在的原因。

#### C.8 Schema 演进

- 原始页面：<https://ducklake.select/docs/stable/duckdb/usage/schema_evolution>
- 核心前提：DuckLake 可在**不重写任何数据文件**的前提下演进表结构，全部通过标准 `ALTER TABLE` 完成。
- 支持的变更：加列／加 struct 字段（`ALTER TABLE tbl ADD COLUMN nested_column.new_field INTEGER;`）、删列／删 struct 字段（需完整路径）、重命名列／嵌套字段／表、类型变更。
- **类型变更只允许无损的"类型提升"**：`int8→int16/32/64`、`int16→int32/64`、`int32→int64`、`uint8→uint16/32/64`、`uint16→uint32/64`、`uint32→uint64`、`float32→float64`。由此可推出：**不可降级**（如 `int64→int32`）、**不可跨符号族**（`int32↔uint32`）、**不可跨类别**（数值↔字符串、浮点→整数）。列表外的类型（`int64`、`uint64`、`float64`、`date`、`decimal` 等）没有可提升目标。
- 机制（页面明确）：列不靠名字、而靠**字段标识符**追踪——`ducklake_column.column_id`，同时写入每个数据文件的 Parquet `field_id`。因此出现三种常态：删列后旧文件仍带该列；加列后旧文件不含新列；改类型后旧文件仍是旧类型。
- 读取时执行 **field id remapping**，三条规则：按匹配 `field_id` 读列，类型不一致时**cast** 到 schema 正确类型；文件有而 schema 无的列**忽略**；schema 有而文件无的列用 `ducklake_column.initial_default` **填充**。
- 结论：变更本身是**纯元数据操作，代价恒定 O(1)**，转换成本推迟到读取时按需支付。

#### C.9 Upsert（`MERGE INTO`）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/usage/upserting>
- 核心：**DuckLake 不支持主键，但 `MERGE INTO` 提供等价的 upsert 能力**。标准 DuckDB 的 `INSERT ... ON CONFLICT` 依赖约束判"冲突"，在 DuckLake 上不可用，需改写为 SQL 标准风格的 `MERGE INTO`——由用户显式写出连接条件与动作。
- 语法：`MERGE INTO 目标 [别名] USING 源 [别名] ON (条件) WHEN MATCHED THEN UPDATE|DELETE WHEN NOT MATCHED THEN INSERT;` 源可以是子查询／派生表；分支支持额外谓词 `WHEN MATCHED AND <condition> THEN ...`。
- 四种典型用法：整行 upsert、`UPDATE SET` 只改部分列、源只带键时按变更集 `DELETE`、带条件删除（`WHEN MATCHED AND salary >= 100000 THEN DELETE`）。
- 已明确限制：**不支持多个 `UPDATE` / `DELETE` 动作**——`WHEN MATCHED` 下并列两个分支会报 `Not implemented Error: MERGE INTO with DuckLake only supports a single UPDATE/DELETE action currently`。需拆成多条语句。错误中的 "currently" 表明是实现进度问题。
- 主键缺失的影响：判重责任转移到查询本身，`ON` 条件写错（过宽、可空列、浮点/时间戳等值）不会报错却得到非预期结果；重复执行同一 `MERGE INTO` 是否幂等取决于匹配键在当前数据上是否唯一；若同一业务键本就存在多行，`WHEN MATCHED` 会作用于所有匹配行。

#### C.10 排序表（Sorted Tables）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/sorted_tables>
- 语法：`ALTER TABLE events SET SORTED BY (event_time ASC);`，支持多列、`NULLS FIRST/LAST`、以及**任意表达式**（如 `date_trunc('hour', event_time)`、DuckLake 宏）；移除用 `ALTER TABLE events RESET SORTED BY;`。表达式在 **`SET SORTED BY` 执行时**即被解析校验。
- 落盘时机（页面明确三条路径）：`INSERT` 写 Parquet 默认排序（受 `sort_on_insert` 控制，默认 `true`）；**文件合并（compaction）始终排序**；**内联刷写（flush）始终排序**。
- 加速机制只有一种：通过排序使每个文件 min/max 区间更窄、重叠更少，从而让引擎跳过值域不重叠的**整个数据文件**。页面**未**提及 row group／页级裁剪，也**未**提及对 join、聚合、`ORDER BY` 消除的加速，更**未**量化排序的写入代价。
- 重要边界：排序必须**已落盘**才有效果，历史文件不会自动重排，需靠合并／重写重新生成；compaction／flush 使用**当前**的排序定义（改键后再合并会按**新**键重排）。
- 页面存在**表述张力**需实测：一处说关闭 `sort_on_insert` 后只有 compaction 与 flush 的输出仍排序；另一处说启用内联且 `sort_on_insert=false` 时，**超出内联行数限制的数据在写入 Parquet 前仍会被排序**。依赖该行为前应在自己环境验证。
- 与分区的区别（页面本身未做对比，为一般性理解）：排序作用于**文件内部物理有序**、依赖 min/max 与过滤条件相关性；分区把数据按分区值切分到**不同文件/目录**、按分区列直接排除文件组。两者可叠加。

#### C.11 行级血缘（Row Lineage）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/row_lineage>
- 该页很短：每行有唯一行标识 `rowid`，可作为虚拟列查询；标识符在**首次插入**时分配，**在 `UPDATE` 与 compaction 等文件搬迁过程中保持不变**。
- 用途：可借 `rowid` 判断"新增文件"是真的引入新行，还是仅仅在移动文件；**变更流（C.12）内部用它区分 update 与 delete+insert**。
- 该页**未**说明 `rowid` 的稳定性边界、是否跨快照／跨 compaction 永久有效，或存储成本。

#### C.12 变更数据流（Data Change Feed）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/data_change_feed>
- 入口：`table_changes(table, start, end)`，`start`/`end` **两端闭区间**，可用**快照 ID 或时间戳**。返回按**结束快照的 schema** 读取的变更集合，外加 `snapshot_id`、`rowid`、`change_type`（`insert`/`update_preimage`/`update_postimage`/`delete`）三列；**更新被拆成前后像两行**。
- 另有单类型子集函数：`table_deletions`（别名 `ducklake_table_deletions`）、`table_insertions`（别名 `ducklake_table_insertions`），签名一致，结果**无 `change_type` 列**。
- Schema 演化下的读取：区间内某列被 `DROP`，该列从**整个结果集**消失；被 `ADD`，加列之前变更行的该列填充**默认值**。
- **主要限制（页面明确）**：会**过期快照**的压缩操作（如 `rewrite_data_files` 移除被删行）会**截断可读取的 change feed**——被移除的行不再能由 change feed 返回。想保留可回放历史就必须控制 `expire_snapshots`、`cleanup_of_files`、`rewrite_data_files` 的保留窗口。
- 该页**未**给出任何性能／存储开销、复杂度、是否物化或能否下推过滤的说明，也**未**给出保留策略参数。

#### C.13 加密

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/encryption>
- 启用：**建库时**传入 `ATTACH 'ducklake:encrypted.ducklake' (DATA_PATH 'untrusted_location/', ENCRYPTED);`，是创建期开关；页面**未**提供把已存在未加密库转为加密库的语句，也**未**提供关闭加密的说明。
- 加密对象：启用后**所有写入数据目录的 Parquet 文件**自动加密（Parquet 模块化加密），无需逐表指定。页面通篇**未**说 catalog 数据库本身被加密。
- 密钥管理（页面明确）：密钥在**写文件时自动生成**，**每次写操作生成新密钥**（每文件独立密钥）；密钥保存在 catalog 的 `ducklake_data_file.encryption_key` 字段；读时**从 catalog 读取**并自动解密，交互方式与未加密库相同。
- 页面**未**提及：密钥轮换机制、根／主密钥或信封加密、KMS 集成、密钥泄露后的重加密流程。
- 推断（文档未写，应视为待验证）：**备份必须连 catalog 一起备**（解密密钥在 catalog 里，只备 Parquet 等于拿到无法解密的密文，且两者必须是同一时间点的一致快照）；**catalog 成为新的信任边界**（密钥以明文存于 catalog 表）；只读路径同样从 catalog 取密钥，**拿到 catalog 读取权限 ≈ 拿到全部解密能力**；分享文件给外部方时必须让对方能访问 catalog。

#### C.14 宏、视图与注释

**宏（Macros）** — <https://ducklake.select/docs/stable/duckdb/advanced_features/macros>

- 支持标量宏与表宏，使用 DuckDB 标准 `CREATE MACRO`：`CREATE MACRO add_values(a, b) AS a + b;`、`CREATE MACRO filtered_table(threshold) AS TABLE SELECT * FROM my_table WHERE value > threshold;`
- 参数：**默认参数** `b := 10`；**类型化参数** `a INTEGER`。
- 存储：宏**持久化在 DuckLake 元数据中**，分三张表——`ducklake_macro`（定义/名称）、`ducklake_macro_impl`（实现）、`ducklake_macro_parameters`（参数）。
- 时间旅行：把 DuckLake 附加到**先前的快照**时，看到的是**该时间点存在的宏集合**。
- 删除：`DROP MACRO add_values;`、表宏用 `DROP MACRO TABLE filtered_table;`。
- 该页**未**列出任何限制，也**未**说明宏的命名空间层级（是否按 schema 隔离、重名冲突、跨 schema 可见性）与权限影响。

**视图（Views）** — <https://ducklake.select/docs/stable/duckdb/advanced_features/views>

- 使用标准 `CREATE VIEW v1 AS SELECT * FROM tbl;`，无自定义方言；定义保存在元数据表 **`ducklake_view`**，不落数据存储层。
- 该页**未**展开与快照／时间旅行的关系，也**未**列出限制（是否可写、能否物化、能否作为 `INSERT`/`UPDATE` 目标、嵌套/跨 schema 引用是否受限）。"定义存于 `ducklake_view`"与其它元数据表带快照列这一事实**只能推断**视图大概率随快照版本化，**不是**该页结论。

**注释（Comments）** — <https://ducklake.select/docs/stable/duckdb/advanced_features/comments>

- 该页明确写到的注释对象只有**表、视图、列**三类（**未**提及 schema 与 tag 注释）。语法来自 DuckDB 核心：`COMMENT ON TABLE t IS '...';`、`COMMENT ON COLUMN t.c IS '...';`
- 存储：表／视图注释存 `ducklake_tag`，列注释存 `ducklake_column_tag`；**不写进数据文件**，属于 catalog 元数据。
- 页面称注释"**可以事务方式修改**"，但**未**说明是否单独产生 snapshot、是否在 `ducklake_snapshot_changes` 可读、时间旅行回退时是否一并回退。

#### C.15 日志（Logging）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/advanced_features/logging>
- DuckLake 不自建日志系统，而是复用 DuckDB 日志框架并注册专用类型，可与内置 `QueryLog` 并用：
  - `CALL enable_logging('DuckLakeMetadata');` —— 记录**每一条元数据查询及其耗时**，通过 `duckdb_logs_parsed('DuckLakeMetadata')` 读取 `catalog`、`query`、`elapsed_ms` 三字段。文档明确指出元数据查询是**性能关键路径**，catalog 在远端数据库（如 PostgreSQL）时往返延迟会被放大——这正是定位"哪条元数据 SQL 拖慢操作"的手段。
  - `CALL enable_logging('QueryLog');` —— 记录**每个连接上的所有查询**（含扩展内部语句），通过 `duckdb_logs` 按 `type` 过滤取 `type`、`message`。宽而不带计时。
  - 同时开启：`CALL enable_logging(['DuckLakeMetadata', 'QueryLog']);`
- 输出位置：默认写入**内存缓冲区**并以 `duckdb_logs` 视图查询；可 `storage = 'stdout'` 或 `storage_path = '/tmp/ducklake_logs'` 落盘。
- 该页**未**给出日志开销的量化数据，也**未**给出"仅排障时开启"的明确警告。默认内存缓冲区若长时间开启，应改用 stdout 或落盘以避免无界增长（合理推断）。

### D. 维护、备份与治理

#### D.1 推荐维护

- 原始页面：<https://ducklake.select/docs/stable/duckdb/maintenance/recommended_maintenance>
- 官方立场是**按需维护**，不是固定执行顺序或频率：

| 操作 | 何时考虑 | 影响 |
| --- | --- | --- |
| `merge_adjacent_files` | 小批写入且未使用内联，产生许多小 Parquet 时 | 产生存储读写开销；页面未规定频率 |
| `expire_snapshots` | 准备清理旧数据文件时 | 缩短可用的历史快照范围，影响时间旅行 |
| 文件清理 | 旧快照过期后需实际清理不再被引用的文件时 | **可能删除数据文件** |
| `CHECKPOINT` | 需要定期执行上述维护时 | 文档推荐用它做周期维护；**未**要求在前三项之后再额外执行一次 |

- 页面明确支持的顺序依赖只有一条：**先让引用旧文件的快照过期，再清理文件**；未规定合并与这两步之间的固定顺序，也未给出每天／每周频率。对因大量删除导致删除文件拖慢读取的表，页面建议重写相关数据文件。
- 该页**未**说明这些维护是否可在只读连接或只读存储上运行，不能据此认定支持。

#### D.2 合并相邻文件

- 原始页面：<https://ducklake.select/docs/stable/duckdb/maintenance/merge_adjacent_files>
- 用法：`CALL ducklake_merge_adjacent_files('lake');`、`CALL ducklake_merge_adjacent_files('lake','t', schema => 's');`
- 作用域与参数：不指定表时只处理 `auto_compact = true` 的表（默认如此）；`auto_compact` **只控制批量调用的纳入范围，不会在写入后自动触发合并**。`max_compacted_files` 限制**每表每次调用**的合并操作数；`min_file_size`／`max_file_size`（字节）排除过小／过大的文件；未指定 `max_file_size` 时其默认等于 `target_file_size`。
- 该函数**只合并相同 schema 版本**的文件。设置过排序键的表，合并输出按运行时当前排序键排序。
- 输出：每产生一个输出文件返回一行，`files_processed` 为该文件合并的输入文件数，`files_created` 恒为 1，可据此核对结果。**输出文件数取决于候选文件与目标大小，不能仅凭调用次数推断。**
- Snapshot 与删除边界：合并不要求先过期 snapshot；跨 snapshot 合并的输出成为 *partial data file*（Parquet 中以 `_ducklake_internal_snapshot_id` 记录逐行归属，元数据 `partial_max` 记录最高 snapshot ID），据称保留时间旅行与变更流。**合并完成不等于旧文件被删除**；回收须另走文件清理流程。只有存在可合并候选且查询涉及合并后文件时，才会降低需读取的文件数。

#### D.3 过期快照

- 原始页面：<https://ducklake.select/docs/stable/duckdb/maintenance/expire_snapshots>
- 影响：过期后不能再通过被过期的快照访问相应历史状态；会**解除**旧快照对文件的引用，但**不会立即自动删除文件**，仍需后续清理。
- 参数：`ducklake_expire_snapshots(..., versions => [2])` 指定快照 ID，或 `older_than => now() - INTERVAL '1 week'` 指定时间界限；`dry_run => true` 只列出将过期的快照。也可设置整个 catalog 的 `expire_older_than`（如 `'1 month'`），**该选项仅支持全局范围**，在其他范围设置会报错。

#### D.4 文件清理（两个不同函数）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/maintenance/cleanup_of_files>
- `ducklake_cleanup_old_files`：清理快照过期／文件合并后**已排队删除**的文件（记录在 `ducklake_files_scheduled_for_deletion`）。前提是留出足够时间让可能读取这些文件的事务结束（文档称若无持续数天的读取事务，删除几天前排队文件通常安全）。`older_than` 只清理在此之前被安排删除的文件；`dry_run => true` 只列出；`cleanup_all => true` 清理全部已排队文件、不保留等待窗口，有长事务时尤其危险。
- `ducklake_delete_orphaned_files`：清理**未被 catalog 追踪**的文件，可能由系统故障产生；同样支持 `older_than`、`dry_run`、`cleanup_all`；目录级选项 `delete_older_than`（如 `'1 week'`）。
- 关键危险条件：**"未被 DuckLake 追踪"不等于"可以安全删除"**。当数据路径或对象前缀与其他应用、备份或暂存文件共用，或仍有写入进行时，孤儿扫描可能把有用途的对象列为候选。应仅在确认该存储范围由该 DuckLake 独占、无在途写入与超长读事务、保留期满足需求后，先 `dry_run` 核对**完整对象路径**再考虑实际删除；**不要**在未核对清单时直接使用 `cleanup_all`。

#### D.5 重写被大量删除的数据文件

- 原始页面：<https://ducklake.select/docs/stable/duckdb/maintenance/rewrite_data_files>
- 背景：DuckLake 对删除采用 **merge-on-read**——删除时不改原文件，而写一个 delete file 指向被删记录位置；删除极快，但查询要额外读 delete file 并合并。对被大量删除的表，读性能会下降。
- `ducklake_rewrite_data_files` 把**删除比例超过阈值**的文件重写成只含未删除记录的新文件，消除 delete file 的读开销；重写后的文件可再用 `ducklake_merge_adjacent_files` 合并。
- 触发阈值 `rewrite_delete_threshold` **默认 0.95**（只清理几乎被删空的文件）。可单次传参 `delete_threshold => 0.5`，也可持久化 `set_option('rewrite_delete_threshold', 0.5)` 并作用于 catalog／schema／table 三级（层级越具体优先级越高）。调低阈值→重写更多→读更快但写与存储开销更大。
- 与删除文件的关系：`DELETE → 生成 delete file → 读时合并 → 删除比例累积超阈值 → rewrite 生成"干净"新文件 → 再 merge 成更大文件 → 旧文件与 delete file 交由过期／清理回收。` 该操作**不删除任何逻辑数据**，只是把"数据 + 删除掩码"换成"纯数据"表示。
- 自动运行：页面给出按表关闭的开关 `set_option('auto_compact', false, table_name => 'my_table')`，标题为"为特定表关闭自动压缩"，说明**存在由 `auto_compact` 控制的自动压缩／重写机制且默认启用**。但该页**未**说明自动触发时机、频率、是否有后台进程，或是否在写入／checkpoint 时顺带执行——这些需查相邻页面，不能据本页断言。
- 返回值：每输出文件一行，含 `schema_name`、`table_name`、`files_processed`、`files_created`（恒为 1）；统计每表实际产出需自行 `sum` 聚合。注意文档中 `CALL` 与 `FROM`（表函数）两种写法并存。
- 成本与风险（该页未专门讨论，属推断需自证）：重写要完整读取旧文件并写出新文件，阈值越低 I/O 与 CPU 成本越高；新文件写出时旧文件与 delete file 仍在，空间回收依赖快照过期与文件清理，短期存在双份占用；`auto_compact` 默认开启意味着系统可能在你不察觉时写入，对写延迟敏感或按请求计费的对象存储需评估。

#### D.6 检查点（CHECKPOINT）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/maintenance/checkpoint>
- `CHECKPOINT` 是面向整个 DuckLake 的维护操作，**依次调用六个函数**：`ducklake_flush_inlined_data` → `ducklake_expire_snapshots` → `ducklake_merge_adjacent_files` → `ducklake_rewrite_data_files` → `ducklake_cleanup_old_files` → `ducklake_delete_orphaned_files`。
- 配置区分：`expire_older_than` 控制快照多大年龄后默认可过期；`delete_older_than` 控制未使用文件多大年龄后可被两个清理函数移除；**快照过期不等同于其文件立即被删除**。`rewrite_delete_threshold`（0～1）控制文件被删数据至少达到多少才值得重写。`auto_compact` 默认开启，可按全局／schema／表设置；文档把内联刷出、文件合并、数据文件重写及**孤立文件删除**列为受其控制，但**未**把 `cleanup_old_files` 列入。
- 风险：运行 `CHECKPOINT` 可能重写或删除文件，并使过期快照不再可用于历史查询。页面**未**给出上述年龄选项的具体默认值、保证保留多少个快照，或并发运行与回滚的保证，因此**不能**断言具体保留期或"运行后可恢复到任意历史版本"。

### E. 元数据函数

#### E.1 列出文件（`ducklake_list_files`）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/metadata/list_files>
- 作用：表函数，把 catalog 中某张表在存储层的**数据文件与配套删除文件**清单列出；可选带入 `snapshot_version` 或 `snapshot_time` 查看历史时刻的文件集合。
- 用法：`FROM ducklake_list_files('catalog','table_name'[, schema => 'main'][, snapshot_version => 2][, snapshot_time => '...'])`。
- 返回字段：`data_file`、`data_file_size_bytes`、`data_file_footer_size`、`data_file_encryption_key`、`delete_file`、`delete_file_size_bytes`、`delete_file_footer_size`、`delete_file_encryption_key`——分两组一一对应，体现"数据文件 + 删除文件"布局。
- 限制：无删除文件时 `delete_*` 列为 `NULL`；加密库必须持有返回的密钥才能读取；`footer_size` 是"可选提供"，不要假定必然有值；`catalog` 必须已 ATTACH 且名称一致；**一次只能查一张表**；返回的是存储层信息，不含行数／列统计／分区值（需查 `ducklake_data_file`、`ducklake_file_column_stats`、`ducklake_file_partition_value` 等表）；`snapshot_version` 与 `snapshot_time` 同时使用时的优先级页面未明确；结果会**明文暴露加密密钥**，导出到日志／CSV／看板需谨慎。
- 典型用途：与对象存储对账（元数据有而文件缺失，或反向的孤儿文件）、把文件清单交给非 DuckDB 引擎、按 `*_size_bytes` 做容量与成本分析、清理／过期前确认仍被引用的文件、复现历史时点的文件集合。

#### E.2 登记外部文件（`ducklake_add_data_files`）

- 原始页面：<https://ducklake.select/docs/stable/duckdb/metadata/adding_files>
- 作用：把**已存在**的数据文件注册为某表的新文件，**不复制也不移动**；文档明确该操作**支持常规事务语义**，是一次正常提交而非绕过事务的旁路写入。典型场景是把外部生成的 Parquet 挂进 DuckLake 而不重写数据。
- 用法：`CALL ducklake_add_data_files('lake','people','people.parquet'[, schema => 's'][, allow_missing => true][, ignore_extra_columns => true])`。
- 列匹配：默认**所有**表列都必须存在于文件中，否则报错；`allow_missing => true` 时缺失列用该列 `initial_default` **逐列**填充（不是整行 NULL）；`ignore_extra_columns => true` 允许文件多出列，但多出的列会被彻底丢弃、**永久不可通过 DuckLake 查询**。
- 类型映射方向：允许**窄 → 宽**（如文件中 `int32` 登记到 `int64` 表列），**不允许宽 → 窄**；`decimal` 要求 `P' <= P` 且 `S' <= S`。类型不匹配即失败。
- 对统计与快照：页面**只**说明它走正常事务语义（与其他写入一样产生新快照，受过期／时间旅行／冲突解决管辖），**完全没有讨论统计**——文件行数与列 min/max 如何登记、与常规写入是否等价，需查规范的表定义，**不能假定等价**。
- 风险：
  - **文件所有权转移**（页面用醒目 Note 标出）：压缩类操作（`merge_adjacent_files`，或 `expire_snapshots` 后 `cleanup_old_files`）**可能直接删除这些文件**。不要在 DuckLake 之外继续读写、备份依赖或硬链接这些文件，也不要把它当"只读挂载"。
  - `allow_missing` 会静默用占位默认值掩盖模式漂移；`ignore_extra_columns` 使源文件列永久不可见。
  - 注册后若文件被外部进程覆盖或改写，catalog 记录的行数／统计／快照与实际内容不一致且**无任何提示**。
  - 路径解析、相对路径基准与远端路径行为本页未涉及（见 B.5、F.2）。
  - 多写者场景下与外部提交如何冲突需参考 C.2，不要默认可以随时安全插入。

### F. 指南与排障

#### F.1 备份与恢复

- 原始页面：<https://ducklake.select/docs/stable/duckdb/guides/backups_and_recovery>
- **必须同时备份两个对象**：catalog（元数据数据库）与 storage（Parquet 数据文件）；两者备份方式不同，只保一方不能覆盖另一方丢失。
- 时间点一致性：恢复后的 catalog 指向某个快照，该快照引用的数据文件也必须可用。文档特别指出：**catalog 备份之后提交的事务不会被该备份记录**——其数据文件可能仍存在，但恢复后的 catalog 不再追踪它们。批处理后应备份 catalog；持续微批／流式写入应定期备份；手动备份前先完成压实与文件清理，因为这些操作会改写或删除文件、改变快照对应的文件布局。
- 恢复方式：DuckDB／SQLite catalog 通过数据库副本恢复；PostgreSQL catalog 可用 SQL dump 或 PITR 恢复到指定时间点；S3／GCS 数据文件可用复制、备份服务或对象版本管理。若从跨桶副本恢复，需把 `DATA_PATH` 指向新桶。
- **可以单独备份 catalog**（用于 catalog 本身损坏／被篡改／被删），但那**不是完整的 DuckLake 备份**：数据文件也丢失时，仅有 catalog 无法恢复这些文件。

#### F.2 远端路径与公开只读发布

- 原始页面：<https://ducklake.select/docs/stable/duckdb/guides/using_a_remote_data_path>、<https://ducklake.select/docs/stable/duckdb/guides/public_ducklake_on_object_storage>
- 「远端数据路径」页实际演示的是：**先在本地生成 DuckLake**，初始化 catalog 时把持久 `DATA_PATH` 设为远端地址，填充数据时用 `OVERRIDE_DATA_PATH true` 临时改用本地目录写 Parquet，随后把 **catalog 文件连同整个数据目录**一起上传；读者通过远端地址附加 catalog。它**不是** S3 读写或共享 catalog 的完整配置指南：页面示例的发布地址是公开 R2 桶提供的 HTTPS URL，**未**给出 `s3://` 的 `DATA_PATH` 写法、直写 S3 流程、或 `httpfs` 的安装与配置要求。页面明确指出目前**不能修改 catalog 中已持久化的 data path**，且上传前需调用 `ducklake_flush_inlined_data`。
- 「公开对象存储只读湖」页演示把 DuckDB 格式 catalog（`.ducklake`）与数据目录放到公开桶、让用户经 HTTPS **无需认证**查询，概念上类似 Frozen DuckLakes 且设置更简单。页面**未**解释 Frozen DuckLake 的定义与冻结机制，也**未**说明 catalog 更新的一致性／并发发布方式；**未**给出桶策略、写权限隔离或防篡改配置——不能仅凭"公共只读"推断存储本身已禁止写入。该页只具体说明了 Cloudflare 跨站查询的 CORS 配置，未提供 S3 的 CORS 或其他 S3 专属限制，也未涉及请求／带宽成本、性能上限、缓存一致性与快照可见性。
- 对 SimpleBase 的含义：这两个页面都是**只读发布**模式，不能用来论证"多个 writer 可直接共享 S3 上的可写 catalog"。

#### F.3 访问控制

- 原始页面：<https://ducklake.select/docs/stable/duckdb/guides/access_control>
- 核心结论：**DuckLake 本身不实现访问控制**。权限由下层两套系统叠加：catalog 的 SQL 授权（谁能改元数据）+ 对象存储的路径前缀策略（谁能读写哪些数据文件）。文中的 Superuser／Writer／Reader 是**讲解用模型，非内置角色**。
- catalog 层（以 PostgreSQL 为例）：Superuser 需库级 `CREATE`、schema `public` 的 `CREATE, USAGE` 及所有表的 `SELECT, INSERT, UPDATE, DELETE`；Writer 只需 schema `USAGE` + 所有表 `SELECT, INSERT, UPDATE, DELETE`（**已足够完成包括 expire snapshots 在内的全部 DuckLake 操作**，不需要 `CREATE` 数据库／schema）；Reader 仅 `USAGE` + `SELECT`。同一角色多用户时应使用 `CREATE ROLE` 用户组 + `GRANT`，而非逐个建用户。跨规范版本迁移**必须**由 Superuser 执行。
- 存储层（S3，路径约定 `/schema/table/partition/data_file.parquet`）：按前缀下发权限，可做到 schema 级、表级乃至分区级隔离。Superuser 需要 `ListBucket, GetObject, PutObject, DeleteObject`；Writer 同样四项但限定 schema 前缀——**Writer 必须含 `s3:DeleteObject`**，因为压缩与清理需要重写数据文件；Reader 仅 `GetObject` 且限定具体表前缀（如需列举桶可保留 `ListBucket` 但用 `Condition` 限制前缀）。
- 实测行为与边界（官方明列）：
  - **Writer 能"凭空建空表"**：在它没有数据路径权限的 schema 里 `CREATE TABLE` 仍成功，因为建表**只写一条 catalog 元数据**，而写 catalog 是 Writer 被允许的；随后 `INSERT` 才失败（抛 DuckDB 通用 S3 403）。建议**把建表与插入包进同一事务**，让插入失败连带回滚建表。
  - Reader 的报错面不同：访问无权表 → S3 侧 `HTTP 403`；尝试建表 → PostgreSQL 侧 `permission denied for table ducklake_table`。
  - `ducklake_add_data_files` 登记的文件若不符合 `/schema/table/...` 命名，**路径级权限不会生效**于这些文件（呼应 E.2）。
  - 切换角色时要显式 `DROP SECRET` 旧的 S3／Postgres secret，否则扩展会默认套用上一组凭据。
- 最小权限建议：只读者 PostgreSQL `USAGE`+`SELECT`、S3 仅 `GetObject`；写入者 PostgreSQL `USAGE`+`SELECT, INSERT, UPDATE, DELETE`（**不给 `CREATE`**）、S3 四项限定目标 schema 前缀；Superuser 仅用于初始化、建 schema 与跨版本迁移。

#### F.4 排障

- 原始页面：<https://ducklake.select/docs/stable/duckdb/guides/troubleshooting>（规范链接为 `.html` 版本）
- 该页只列出两项问题，主题都是**版本不匹配**：
  1. **连接旧版 DuckLake**：报错形如 `DuckLake catalog version mismatch: catalog version is 0.3, but the extension requires version 1.0. To automatically migrate, set AUTOMATIC_MIGRATION to TRUE when attaching.` 处理方式是在 `ATTACH '...' (AUTOMATIC_MIGRATION);` 中开启自动迁移。
  2. **连接只读 DuckLake**：迁移需在 `__ducklake_metadata_*` 上执行 `CREATE`，只读挂载下失败；官方明确**无法**用预发布版本（v0.x）连接到只读 DuckLake，建议改用**较旧版本的 `ducklake` 扩展**先迁移到中间格式——具体做法是把数据复制到一个有写权限的 DuckLake，再用较新扩展连接该副本。
- 页面**未**提供其他错误、日志开关或诊断命令；并提示另见「不支持的功能」页。

#### F.5 从 DuckDB 迁移到 DuckLake

- 原始页面：<https://ducklake.select/docs/stable/duckdb/migrations/duckdb_to_ducklake>
- 全部特性受支持时，**一条 SQL 即可**：`ATTACH 'ducklake:my_ducklake.ducklake' AS my_ducklake;` + `ATTACH 'db.duckdb' AS my_duckdb;` + `COPY FROM DATABASE my_duckdb TO my_ducklake;`（方向为**源 → 目标**）。文档明确 catalog 后端用 DuckDB 文件、PostgreSQL 或 SQLite **都无所谓**。
- 存在不支持特性时有三类取舍：

| 特性 | 迁移行为 |
| --- | --- |
| 特殊类型 | 必须转为 DuckLake 支持的类型：`STRUCT` 定义的用户自定义类型可原样保留；`ENUM`、`UNION` → `VARCHAR`；`VARINT` → `INT` |
| 宏（Macro） | 可迁移到 **DuckDB 持久化数据库**；catalog 若是 PostgreSQL／SQLite／MySQL，**宏不受支持、无法迁移** |
| 生成列 | 等同"非字面量默认值"，插入时必须显式指定值，**没有 `VIRTUAL` 选项** |

- 官方另提供 Python 迁移脚本（页面内嵌可执行代码，本计划**未**运行）。其要点与已知取舍：类型映射为白名单（`VARINT → ::VARCHAR::INT`，`UNION`/`ENUM`/`BIT` → `::VARCHAR`，定长数组用正则转基类型数组）；表与视图迁移用 `CREATE TABLE IF NOT EXISTS ... AS SELECT`（CTAS），视图失败可重排队、**同一对象第二次失败即永久跳过**；宏迁移仅在 `--catalog-type duckdb` 下生效（先 DETACH DuckLake，再把其元数据文件 ATTACH 为 `ducklake_metadata` 写宏）。
- 脚本的**隐含代价**（由实现决定，文档未逐条声明）：CTAS 建表使**主键／外键／唯一约束、默认值、注释、索引、排序与分区设定都不会随数据迁移**；类型处理是白名单式，`DOUBLE[]`、`DECIMAL[]`、嵌套数组等可能原样透传后建表报错；视图定义是截去 `CREATE VIEW ` 前缀后直接重建，其它修饰未兼容；`information_schema` 查询**无 `ORDER BY`**，依赖较深的视图可能撞上"两次失败即跳过"上限；大批量数据 CTAS 会一次性全量写入，耗时与临时空间需提前评估。
- 页面明确：当前脚本**仅支持本地迁移**，尚未适配 S3、GCS 等远程对象存储。使用前应先对照「不支持的功能」页自检，并**备份源库、先在测试库演练**。

### G. 与本项目实现的对应关系

- 官方机制 / 本项目自行实现 / 不能据此承诺的对照表，见正文**「DuckLake 官方规范与扩展实现研究」第 5 节**，此处不重复。
- 使用本索引时的建议顺序：先按 A 确认格式与可见性语义（含完整规范表清单）→ 按 B/C 确认连接、catalog、事务前提与表级能力（约束、schema 演进、排序、变更流、加密）→ 按 D/E 确认维护与文件级事实查询 → 按 F 确认备份、权限、迁移与版本兼容 → 再回到正文对照表判断某项能力是官方保证还是项目协议。
- 对本项目最有参考价值的几项：**C.7 约束**（`sys_*` 表无法依赖数据库级唯一性）、**C.10 排序表**与 **C.5 分区**（`sys_log_events`／`sys_metric_samples` 等热点表减少远端文件扫描的可评估手段）、**C.12 变更流**（清理窗口会截断可回放历史）、**C.15 日志**（`DuckLakeMetadata` 的 `elapsed_ms` 可用于定位拖慢的元数据 SQL）。这些**均为待评估项**，尚未在本项目落地，也不能替代实测。

**补充阅读（非官方规范）**：[DuckDBLab 中文介绍](https://duckdblab.org/zh/post/ducklake-v1-intro/)；其结论须分别与上列规范和扩展文档对照——该文未说明 catalog 实际后端，其"无需 Hive Metastore 等编目服务"不等于 DuckLake 不需要 SQL catalog，性能星级也**不是**基准数据，不能当作实测结论。

**索引覆盖度**：以上 A–F 已覆盖官方导航的 Specification（导言、数据类型、查询、表总览及全部规范表）、DuckDB Extension 的 Usage（连接、catalog 选择、存储选择、快照、schema 演进、时间旅行、upsert、配置、路径）、Maintenance（推荐维护、合并、过期、清理、重写、checkpoint）、Advanced Features（约束、冲突解决、变更流、内联、加密、分区、事务、行级血缘、宏、视图、注释、排序表、日志）、Metadata（列文件、登记文件）、Migrations（DuckDB→DuckLake）、Guides（访问控制、备份恢复、公开只读、远端数据路径、排障）与不支持的功能页。官方 `FAQ` 页不在 `docs/stable` 路径下，**未**纳入；各页面在 stable 版本下可能随时增补新特性，取用前应再次核对。

