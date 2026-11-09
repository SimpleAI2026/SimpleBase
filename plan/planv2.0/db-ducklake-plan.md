# SimpleBase DB 后端重写计划：基于 SQLite + DuckLake 的 ServerlessDB

> 目标：用 **DuckDB + DuckLake 扩展（SQLite catalog + S3 Parquet 数据文件）** 替换现有 Turso/libSQL 数据面，
> 实现以 S3 为唯一持久事实来源的 Serverless 数据库。
> 依据文档：[DuckLake Documentation (1.0 stable)](https://ducklake.select/docs/stable/)，
> 本地完整镜像：`docs/ducklake-docs.md`（单文件版，含规格全部 28 张 catalog 表定义）。
> 制定日期：2026-09-14（同日依据完整文档修订）
> 前置版本：`plan/planv1.0/plan.md`（Turso/libSQL 链路，Plan 1-11 已完成）

## 目录

- [一、为什么重写](#一为什么重写)
- [二、DuckLake 关键事实（来自完整文档）](#二ducklake-关键事实来自完整文档)
- [三、目标架构](#三目标架构)
- [四、关键设计](#四关键设计)
- [五、SQL 兼容矩阵（用户可见的语义变化）](#五sql-兼容矩阵用户可见的语义变化)
- [六、模块改动清单](#六模块改动清单)
- [七、分阶段实施计划](#七分阶段实施计划)
- [八、API 与协议变化](#八api-与协议变化)
- [九、存量数据迁移](#九存量数据迁移)
- [十、风险与应对](#十风险与应对)
- [十一、开放问题](#十一开放问题)

## 一、为什么重写

v1.0 数据面基于 Turso/libSQL，存在以下问题：

1. **分析能力弱**：libSQL 是 OLTP 引擎，SimpleBase 作为 BaaS 平台，用户数据天然需要分析型查询（聚合、窗口、JSON、Parquet 导出）。
2. **S3 持久层语义依赖上游**：Turso 的 S3 持久化确认语义不透明，v1.0 计划中「本地已提交 vs 已持久化到 S3」的区分始终是灰色地带。
3. **格式封闭**：数据困在 libSQL 私有复制格式里，用户无法用其他工具直接读取自己的数据。

DuckLake 恰好解决这三点：

- **开放格式**：数据文件是标准 Parquet，catalog 是标准 SQL 表（v1.0 规格共 28 张，全部公开可查），任何引擎（DuckDB/Spark/Trino/DataFusion/pg_ducklake）都能读。
- **S3 原生**：DuckLake 从不原地修改文件（不改内容、不追加），一致性要求极低，天然适配对象存储。
- **分析级引擎**：DuckDB 提供完整的 OLAP SQL、Parquet/CSV/JSON 直读、时间旅行、schema 演进、分区、排序表。

### 与现有架构的契合度（重写成本低的根本原因）

| 现有资产 | 在 DuckLake 方案中的去向 |
|---|---|
| `database.Factory` 接口返回 `*sql.DB` | **不变**。`go-duckdb` 是标准 `database/sql` 驱动 |
| `internal/database/registry`（每库唯一 writer） | **不变**。DuckDB/DuckLake 本身就是单写模型，语义完全匹配 |
| `query.go` / `transaction.go`（SQL 执行原语） | **基本不变**，仅适配 `LastInsertID` 等差异 |
| `internal/database/cache`（本地缓存 LRU） | **不变**。缓存对象从 libSQL 文件变为 DuckLake catalog 文件 + DuckDB 临时目录 |
| `internal/catalog` / `auth` / `api` / `usage` / `audit` / `jobs` | **不变**。存储无关的平台层 |
| `internal/database/turso/` + `tursofactory.go` | **退役**，由 `internal/database/ducklake/` 替代 |
| `internal/database/sqlguard` | **改造**。DuckDB 方言 + 文件/网络访问边界 |
| `internal/database/serialize.go` | **改造**。DuckDB 类型系统（DECIMAL/TIMESTAMPTZ/嵌套类型）→ JSON |

## 二、DuckLake 关键事实（来自完整文档)

计划的所有设计都基于以下已验证的文档事实。每条标注了 `docs/ducklake-docs.md` 中的来源章节。

### 2.1 版本要求（硬性）

**DuckLake v1.0 需要 DuckDB v1.5.2+**（Introduction §Using DuckLake from a Client）。
go-duckdb 绑定必须映射到 DuckDB ≥ v1.5.2，这是 Phase 1 前置验证的第 0 项。
catalog 版本不匹配时会报 `DuckLake catalog version mismatch`，需显式 `AUTOMATIC_MIGRATION` 才迁移（Troubleshooting）。

### 2.2 两段式存储模型

DuckLake = **catalog（SQL 数据库）+ data files（Parquet）**。
catalog 存元数据（snapshot、schema、文件清单、统计、内联数据），data files 存真实数据。
catalog  schema 完全公开：28 张表，含 `ducklake_snapshot`（PK 冲突即写冲突检测机制）、`ducklake_data_file`、`ducklake_metadata`（kv 配置）等（Specification §Tables）。

### 2.3 SQLite 作为 catalog

```sql
INSTALL ducklake; INSTALL sqlite;
ATTACH 'ducklake:sqlite:metadata.sqlite' AS my_ducklake (DATA_PATH 'data_files/');
```

- SQLite catalog 支持多进程：默认每次查询 ATTACH/DETACH + 写锁重试超时（Choosing a Catalog Database）。
- 与我们「单写实例 + 进程内 Registry」的部署约束叠加后，并发模型非常保守、安全。
- SQLite catalog 中内联数据的类型编码：整数→`BIGINT`，其余大多→`VARCHAR`，`BLOB`→`BLOB`（Specification §Data Types §SQLite），无二进制格式风险。

### 2.4 存储后端与不可变性

- 数据文件可落在本地盘、S3/GCS/Azure、NFS 等任何 DuckDB 支持的文件系统。
- **DuckLake 从不修改已存在的文件**，极大降低一致性要求、简化缓存（Choosing Storage）。
- **路径默认相对存储**（`path_is_relative`，Paths）：catalog 记录相对路径，catalog + data 整体可搬迁，对迁移/恢复/换 bucket 友好。
- 已知限制：**catalog 中持久化的 data_path 目前不可修改**；跨 bucket 恢复需每次连接使用 `OVERRIDE_DATA_PATH true`（Using a Remote Data Path）。

### 2.5 事务、快照与冲突解决

- 完整 ACID + 快照隔离，DDL 也有事务语义；**一个已提交事务 = 一个 snapshot**（Transactions）。
- 冲突检测机制：`ducklake_snapshot.snapshot_id` 主键约束，并发写产生 PK 冲突 → 基于 `ducklake_snapshot_changes` 的变更集分析自动重试；逻辑冲突（同建表、删改竞争、压缩与删除竞争等）才中止（Conflict Resolution）。
- 重试参数：`ducklake_max_retry_count`（默认 10）、`ducklake_retry_wait_ms`（100）、`ducklake_retry_backoff`（1.5）。

### 2.6 快照函数与提交消息（同步水位与审计的直接依据）

- `my_ducklake.snapshots()`：列出全部快照及变更集、author、commit_message。
- `my_ducklake.current_snapshot()`：最新快照 id —— **CatalogSyncer 的水位直接取它，无需外部 manifest**。
- `my_ducklake.last_committed_snapshot()`：当前连接已提交的最新快照（多连接场景区分）。
- `CALL my_ducklake.set_commit_message(author, message, extra_info => '...')`：事务内设置提交者/消息/附加 JSON —— **把 auth principal + request_id 写入快照，审计能力免费获得**；`require_commit_message` 选项可强制（Snapshots）。

### 2.7 时间旅行与变更馈送（CDC）

- 查询级：`SELECT * FROM tbl AT (VERSION => 3)` / `AT (TIMESTAMP => ...)`；连接级：ATTACH 参数 `SNAPSHOT_VERSION` / `SNAPSHOT_TIME`（Time Travel）。
- **Data Change Feed**：`table_changes('tbl', start, end)` 返回两快照间的 insert/update_preimage/update_postimage/delete；`table_insertions` / `table_deletions` 为子集；边界可用快照 id 或时间戳（Data Change Feed）。这是未来 Realtime/CDC 订阅的免费底座。
- 行级血缘：每行有稳定 `rowid` 虚拟列，UPDATE/压缩后保持（Row Lineage）。

### 2.8 数据内联（Data Inlining）——对同步设计影响最大

- **默认开启，行限制 10**（`ducklake_default_data_inlining_row_limit`）：小于限制行的 INSERT/DELETE **直接写入 catalog 内的内联表，不产生任何 Parquet 文件**（Data Inlining）。
- 内联数据与 Parquet 数据语义完全一致，参与 time travel；`ducklake_flush_inlined_data` 将其物化为 Parquet（含 partial deletion file，保留快照信息）。
- **对 SimpleBase 的意义**：
  - BaaS 典型负载是大量小写入 → 大多数写事务只动 catalog SQLite → **catalog 同步本身就承载了数据**，debounce 窗口内崩溃的语义更干净（事务整体丢失，无孤儿 Parquet）。
  - catalog 随内联数据增长 → CHECKPOINT（含 flush）与 catalog 版本裁剪成为容量治理的关键。
  - 限制：`VARIANT` 列在 SQLite catalog 下无法内联（自动跳过该表内联）；嵌套类型以 VARCHAR 存储、读回自动转换。
- ATTACH 参数 `DATA_INLINING_ROW_LIMIT` 为连接级覆盖；`set_option('data_inlining_row_limit', ...)` 为持久化三级（global/schema/table）覆盖。

### 2.9 维护操作（全部有 dry_run，直接映射 jobs 框架）

`CHECKPOINT` 一条语句按序执行（Checkpoint）：

```text
ducklake_flush_inlined_data → ducklake_expire_snapshots → ducklake_merge_adjacent_files
→ ducklake_rewrite_data_files → ducklake_cleanup_old_files → ducklake_delete_orphaned_files
```

各函数独立可调，要点：

| 函数 | 要点 |
|---|---|
| `ducklake_merge_adjacent_files` | **无需 expire 快照**，合并产物是 partial data file（内嵌 `_ducklake_internal_snapshot_id` 列），time travel/change feed 完全保留、对用户透明；支持 `max_compacted_files`（控内存）、`min/max_file_size`（分层压缩：如 <1MB→5MB→32MB→128MB 三级）；返回值含 files_processed/files_created |
| `ducklake_expire_snapshots` | `versions => [...]` / `older_than => ts` / `dry_run`；catalog 级默认 `set_option('expire_older_than', '1 month')` |
| `ducklake_cleanup_old_files` | 清理已调度删除的文件（`ducklake_files_scheduled_for_deletion`）；文档建议删除调度超过数天的文件（前提是没那么长的读事务）；支持 `older_than`/`dry_run`/`cleanup_all` |
| `ducklake_delete_orphaned_files` | 清理不被 catalog 追踪的孤儿文件（系统故障残留）；支持 `older_than`/`dry_run`；catalog 级默认 `delete_older_than` |
| `ducklake_rewrite_data_files` | 重写删除比例超阈值（默认 0.95，`rewrite_delete_threshold` 可调）的文件，消除 merge-on-read 读放大 |
| `auto_compact` 选项 | 控制表是否参与批量压缩（不触发自动压缩）；可 global/schema/table 三级 |

### 2.10 Catalog 备份语义（durability 设计的依据）

[Backups and Recovery](https://ducklake.select/docs/stable/duckdb/guides/backups_and_recovery) 明确：

- SQLite catalog 备份：`ATTACH 'sqlite:backup.db' AS backup; COPY FROM DATABASE db TO backup;`（文档背书的一致性拷贝，可在 catalog 被占用时执行）。
- **关键警告**：catalog 备份之后提交的事务不被该备份追踪——数据仍在 data files 里，但 catalog 指向旧 snapshot。
  这正是我们 catalog 同步方案的崩溃一致性语义：**崩溃窗口内的事务表现为「数据文件孤儿 + catalog 回退到上一同步点」**；若事务是内联写入，则整体只存在于丢失的 catalog 中，连孤儿文件都没有。两种残留都由 `ducklake_delete_orphaned_files` 清理，不产生脏数据。
- 压缩/清理任务只应在手动备份前做（它们会改写/删除数据文件，改变快照的文件布局）。

### 2.11 不支持的特性（规格级，用户可见）

[Unsupported Features](https://ducklake.select/docs/stable/duckdb/unsupported_features) + Constraints，必须在产品层收口：

**不太可能支持（视为永久限制）**：

- 索引、主键/强制唯一约束、外键（data lake 场景强制执行代价过高；未来或有 BigQuery 式非强制 PK）
- Upsert 只能走 `MERGE INTO`（不支持 `INSERT ... ON CONFLICT`；且 MERGE INTO 目前只支持单个 UPDATE/DELETE 动作）
- Sequences（**注意：DuckDB 有 SEQUENCE 但 DuckLake 不支持** —— ID 生成必须用应用侧 UUID/ULID 或 `uuid()` 函数）
- `VARINT` / `BITSTRING` / `UNION` 类型

**未来可能支持（暂不支持）**：

- 用户自定义类型、定长 `ARRAY`、`ENUM`、`CHECK` 约束
- 非字面量默认值（`DEFAULT now()` 不允许，`DEFAULT '2025-08-08'` 允许）
- 生成列（`GENERATED ... VIRTUAL`）
- `DROP ... CASCADE` 的依赖级联

**约束**：仅支持 `NOT NULL`（含 `ALTER ... SET/DROP NOT NULL`）。

### 2.12 Schema 演进与类型提升

- `ALTER TABLE` 支持 ADD/DROP/RENAME 列（含 struct 嵌套字段路径）、RENAME 表、`ALTER ... SET TYPE`（仅无损提升：`int8→int16→int32→int64`、`uint8→…→uint64`、`float32→float64`）。
- 基于 Parquet `field_id` 的列映射：删列后旧文件仍带该列（读时忽略）、加列后旧文件缺该列（读时补 `initial_default`）、类型提升读时自动 CAST。重写零成本。

### 2.13 加密

- `ATTACH ... (ENCRYPTED)` 开启 Parquet 加密；**每个文件独立密钥，密钥存 catalog 的 `ducklake_data_file.encryption_key` 字段**。
- 推论：**catalog SQLite 即密钥库** → S3 上 catalog 对象必须私有 + SSE + 最小 IAM；开启 ENCRYPTED 后 catalog 泄露 = 数据泄露。

### 2.14 可观测性钩子

- `CALL enable_logging('DuckLakeMetadata')`：结构化记录每条 catalog 元数据查询及 `elapsed_ms`（`duckdb_logs_parsed('DuckLakeMetadata')` 查询）——可直接桥接为 SimpleBase 指标。
- `CALL enable_logging('QueryLog')`：全量查询轨迹（含扩展内部 SQL），调试与审计用。
- `ducklake_list_files('catalog', 'table')`：列出表的数据/删除文件、大小、加密 key——管理端「表文件详情」能力。
- `ducklake_add_data_files('catalog', 'table', 'file.parquet')`：**把已有 Parquet 文件注册进表**（所有权移交 DuckLake，后续压缩/清理会删除它）——批量导入与迁移的第三条路径。

### 2.15 访问控制参考模式

官方 Access Control 指南的角色模型（Superuser/Writer/Reader）= catalog 级权限 + 存储级权限叠加；S3 路径约定 `/{schema}/{table}/{partition}/file.parquet` 允许 IAM 按 schema/table 前缀授权；Writer 需要 `s3:DeleteObject`（压缩/清理要删文件）。SimpleBase 的 per-DB 前缀 IAM 可直接套用该模式。

## 三、目标架构

```text
 Clients / SDK / UI
        │
        ▼
┌────────────────────────────────────────────────────────────┐
│ SimpleBase Server（唯一可写实例，部署约束不变）             │
│                                                            │
│ API Gateway（不变）                                        │
│ ├─ Auth / Project / Quota / Audit                          │
│ ├─ Database Management API                                 │
│ ├─ SQL API（query/execute/batch + snapshot/durability）     │
│ └─ LLM Gateway（不变）                                     │
│                                                            │
│ Database Runtime                                           │
│ ├─ Registry（每库唯一 writer，不变）                        │
│ ├─ DuckLakeFactory ──┐                                     │
│ │   per-DB: DuckDB 实例（go-duckdb, database/sql）         │
│ │   ├─ ATTACH ducklake:sqlite:{cache}/catalog.sqlite       │
│ │   │      (DATA_PATH 's3://bucket/prefix/data/')          │
│ │   ├─ CREATE SECRET（S3 凭据，httpfs）                    │
│ │   └─ SET memory_limit/threads/allowed_paths              │
│ ├─ CatalogSyncer（catalog.sqlite ↔ S3，水位=快照 id）       │
│ ├─ Cache Manager（不变，管理 catalog 文件与临时目录）        │
│ └─ Maintenance Jobs（CHECKPOINT / 孤儿清理 / 版本保留）     │
└───────────────┬────────────────────────────────────────────┘
                │
                ▼
       S3 Compatible Storage
       {prefix}/databases/{db-id}/
         ├─ catalog/catalog.sqlite              ← DuckLake catalog（唯一可变状态，含内联数据与加密密钥）
         ├─ catalog/versions/{snapshot_id}.sqlite ← 历史版本（库级 PITR）
         ├─ data/{schema}/{table}/...parquet    ← DuckLake DATA_PATH（不可变，扩展独占管理）
         └─ metadata/descriptor                 ← SimpleBase 描述符（不变）
```

### 3.1 Serverless 语义

- **冷启动**：从 S3 下载 catalog.sqlite（KB~MB 级，含内联数据）到本地缓存 → 打开 DuckDB → ATTACH → ready。Parquet 数据文件按需从 S3 读取，无需全量拉取。
- **空闲卸载**：Registry 空闲超时关闭 handle → CatalogSyncer 强制最终同步 → 本地缓存可淘汰。S3 是唯一持久事实。
- **崩溃恢复**：新实例仅凭 S3 上的 catalog + data files 恢复；崩溃窗口内未同步的事务按 §2.10 语义回退到上一同步点，孤儿文件由维护任务清理。

## 四、关键设计

### 4.1 Go 驱动选型：`github.com/marcboeker/go-duckdb`

- 标准 `database/sql` 驱动（注册名 `duckdb`），与现有 `Factory`/`query.go`/`transaction.go` 零摩擦对接。
- **版本硬要求：绑定须映射到 DuckDB ≥ v1.5.2**（DuckLake v1.0 的最低版本，§2.1）；锁定 go-duckdb 与扩展版本，禁止自动升级。
- CGO 依赖：项目已使用 CGO（`uglyer/go-sqlite3`），不引入新的构建约束类别。
- 备选方案（否决）：DuckDB CLI 子进程（无连接复用、错误语义差）；自研 C API 封装（重复造轮子）。

### 4.2 扩展引导（Extension Bootstrap）

每个 DuckDB 实例启动时执行：

```sql
INSTALL ducklake;  -- 版本锁定，首次需网络或预置 extension_directory
INSTALL sqlite;
INSTALL httpfs;    -- S3 访问
LOAD ducklake; LOAD sqlite; LOAD httpfs;
```

- 生产镜像**预下载扩展二进制**并设 `extension_directory`，运行时零网络依赖（air-gapped 可用）。
- `AUTOMATIC_MIGRATION` 默认关闭：catalog schema 版本升级走显式发布流程（先备份 catalog，再以 `AUTOMATIC_MIGRATION` ATTACH 一次，验证后恢复常态）。

### 4.3 每库一个 DuckDB 实例（v1），共享引擎为后续优化

- v1：每个 logical database 对应独立 DuckDB 实例（`:memory:` 引擎 + ATTACH ducklake catalog），与 Registry 的 per-DB handle 模型一一对应。
- 资源边界：`SET memory_limit = '{cfg}'`、`SET threads = {n}`，配合 Registry `MaxOpen` + 空闲淘汰，总量可控。
- 后续优化（不在本期）：单 DuckDB 实例 ATTACH N 个 ducklake，共享 buffer manager，降低多库场景内存占用。需先解决单连接串行化与故障域耦合问题。

### 4.4 Catalog 同步（CatalogSyncer）——本方案的核心新组件

SQLite catalog 必须在本地文件系统（DuckDB sqlite 扩展的要求），它是唯一可变状态（含元数据 + 内联数据 + 加密密钥）。

**水位模型（无需外部 manifest）**：

- catalog 内的 `ducklake_snapshot` 表即真相；同步水位 = `current_snapshot()` 返回值。
- 每次成功同步后记录 `last_synced_snapshot_id`（内存 + descriptor 旁注）；S3 上的 `versions/{snapshot_id}.sqlite` 以快照 id 命名，天然幂等。

**写路径（防抖批量同步，默认，已决策）**：

```text
用户写事务 COMMIT 成功（= 新 snapshot 落入本地 catalog）
  → CatalogSyncer.MarkDirty(dbID, snapshotID)
  → 后台同步器按 debounce_ms（默认 200ms）合并触发 Sync(dbID):
      1. ATTACH 'sqlite:{staging}.sqlite' AS backup
      2. COPY FROM DATABASE __ducklake_metadata_{name} TO backup  -- §2.10 文档背书的一致性拷贝
      3. DETACH backup
      4. PUT s3://.../catalog/catalog.sqlite（staging 文件）
      5. 可选：保留 versions/{current_snapshot}.sqlite
      6. last_synced_snapshot_id = current_snapshot()
```

- 配置保留 `sync_on_commit` 模式供强持久化场景切换。
- 关闭/空闲淘汰前强制最终同步（flush-on-close 不受防抖影响）。
- API 响应保留 v1.0 的持久化级别语义：`committed_local` / `synced_s3`；防抖模式下写响应恒为 `committed_local`，同步水位（`last_synced_snapshot_id` 与 `current_snapshot()` 的差值）通过指标与数据库状态接口暴露。

**读路径（冷启动）**：

```text
Open(dbID):
  1. HEAD s3://.../catalog/catalog.sqlite → 不存在 = 全新库，本地创建
  2. GET 到 {cache_dir}/{db-id}/catalog.sqlite（带校验：大小/mtime/可选 SHA256）
  3. 打开 DuckDB → CREATE SECRET → ATTACH 'ducklake:sqlite:...' (DATA_PATH 's3://.../data/')
  4. 校验 settings() 的 data_path 与 descriptor 一致（防串库）
  5. 校验 catalog 版本与扩展匹配（否则按 §4.2 迁移流程处理）
```

**崩溃一致性**（直接采用 §2.10 文档语义）：

| 崩溃点 | 结果 | 恢复动作 |
|---|---|---|
| COMMIT 前 | 事务不存在 | 无 |
| 内联写 COMMIT 后、同步前 | 事务整体只存在于丢失的本地 catalog，**无孤儿文件** | 回退到上一同步点，语义最干净 |
| Parquet 写 COMMIT 后、同步前 | S3 上有孤儿 Parquet，catalog 是旧 snapshot | 回退到上一同步点；`ducklake_delete_orphaned_files` 清理孤儿 |
| 上传中途 | S3 对象不完整或旧版本 | S3 PUT 原子性保证要么旧要么新；校验失败则回退 versions/ |

**明确承诺**：「API 返回写成功」= 本地 catalog 已提交；「已持久化到 S3」= 同步完成。两级别都在 API 与指标中暴露。这与 DuckLake 官方备份语义完全一致，不发明新语义。

### 4.5 安全边界：用户 SQL 的文件/网络访问控制

DuckDB 能力远超 SQLite（`read_parquet('s3://...')`、`COPY TO`、`ATTACH`、httpfs 任意 URL），必须收口：

1. **sqlguard 方言改造**：
   - 永久拒绝：`ATTACH/DETACH/INSTALL/LOAD/COPY/EXPORT/IMPORT/CREATE SECRET/CREATE MACRO/SET/PRAGMA/CHECKPOINT/CALL`（管理面指令由后端内部通道执行，不走用户 SQL API；`CALL` 含全部维护函数与 `set_option`）。
   - 只读路径允许：`SELECT/WITH/EXPLAIN/DESCRIBE/SHOW`。
   - 写路径允许：`INSERT/UPDATE/DELETE/MERGE INTO/CREATE TABLE/CREATE SCHEMA/ALTER/DROP TABLE/CREATE VIEW/BEGIN/COMMIT/ROLLBACK` 等（§2.11 不支持的语法由引擎自然报错，无需额外拦截）。
2. **文件系统边界**：`SET allowed_paths = ['s3://{bucket}/{db-prefix}/data/', '{cache_dir}/{db-id}/']`，用户 SQL 即使注入表函数也无法越出本库前缀（Phase 1 前置验证任务必须验证 `allowed_paths` 对 s3 前缀与表函数生效）。
3. **S3 凭据隔离**：`CREATE SECRET` 使用本库前缀最小权限（参考 §2.15 官方 IAM 模式：写角色需 `s3:PutObject/GetObject/DeleteObject/ListBucket` 且限定前缀）；secret 由后端创建，用户 SQL 无法读取 secret 内容。
4. **catalog 即密钥库**（§2.13）：bucket 默认私有 + SSE；ENCRYPTED 库的 catalog 对象访问审计加强。
5. 资源边界沿用现有 sqlguard：超时、最大行数、最大批次数、请求体大小。

### 4.6 类型序列化改造（serialize.go）

DuckDB 类型远多于 SQLite（§2.2 规格全表），需定义 JSON 映射并补测试：

| DuckDB 类型 | JSON 表示 |
|---|---|
| 整型全族（int8~int64、uint8~uint32）/ float32/64 / BOOLEAN / VARCHAR | 原生 |
| uint64 / int128 / uint128 (HUGEINT) | string（防 JS 精度丢失） |
| DECIMAL(P,S) | string |
| TIMESTAMP / TIMESTAMPTZ / TIMESTAMP_S/MS/NS / DATE / TIME / TIMETZ | RFC3339 / ISO8601 string |
| INTERVAL | string |
| BLOB | base64 string |
| UUID | string |
| JSON | 解析后为 JSON 值 |
| LIST / STRUCT / MAP | 递归转 JSON 值（go-duckdb 复合类型需用 `sql.Scanner` 或 CAST 为 JSON，Phase 1 前置验证） |
| VARIANT | CAST 为 JSON 后输出（SQLite catalog 下不进内联表，§2.8） |
| GEOMETRY | GeoJSON string 或 WKT（Phase 1 验证后定） |

`QueryResult.LastInsertID`：DuckDB 无此概念且 DuckLake 不支持 sequences（§2.11），恒为 0，API 文档标注废弃（ID 生成引导至应用侧 UUID 或 `uuid()` 函数；DuckDB 支持 `RETURNING` 子句可回显）。

### 4.7 审计与快照提交消息集成

- 每个用户写事务在 COMMIT 前执行 `CALL {lake}.set_commit_message('{principal_id}', '{op_summary}', extra_info => '{"request_id":"...","project":"..."}')`（§2.6）。
- 效果：`snapshots()` 天然成为按库的审计流水，与现有 `internal/audit` 互补（audit 记 API 面，snapshot 记数据面）。
- 可选：`set_option('require_commit_message', 'true')` 强制所有事务带消息（默认关闭，避免兼容性问题）。

### 4.8 配置变化（config.go）

```yaml
database:
  engine: ducklake                 # 新增：ducklake | local（dev）
  cache_dir: /var/lib/simplebase/cache
  ducklake:                        # 新增 section
    memory_limit: 512MB            # 每 DuckDB 实例
    threads: 2
    extension_dir: /opt/simplebase/extensions   # 预置扩展（版本锁定）
    data_inlining_row_limit: 100   # 小写入内联进 catalog（默认 10；BaaS 小写场景调高，减少小文件与孤儿窗口）
    parquet_compression: zstd
    target_file_size: 64MB         # BaaS 场景远小于默认 512MB
    require_commit_message: false
    catalog_sync:
      mode: debounce               # debounce（默认，已决策）| sync_on_commit
      debounce_ms: 200
      keep_versions: 10            # catalog 历史版本保留数
    maintenance:
      checkpoint_interval: 1h      # CHECKPOINT 周期
      expire_older_than: 7d        # 快照过期（time travel 窗口）
      delete_older_than: 1d        # 文件清理安全窗（须大于最长读事务）
      rewrite_delete_threshold: 0.95
```

### 4.9 DevMode

DevMode 同样跑 DuckLake，仅把 `DATA_PATH` 换成本地目录（`{cache_dir}/dev/dbs/{db-id}/data/`），catalog 落本地不同步 S3。
**开发与生产同一代码路径**，比 v1.0 的「dev 用纯 SQLite、prod 用 Turso」更不易出现环境差异 bug。

## 五、SQL 兼容矩阵（用户可见的语义变化）

从 libSQL/SQLite 方言迁移到 DuckLake，用户侧必须知晓的差异（§2.11、§2.12 汇总）：

| 特性 | libSQL/SQLite（旧） | DuckLake（新） | 迁移指引 |
|---|---|---|---|
| 主键 / UNIQUE | 支持 | **不支持** | 唯一性改应用层保证；upsert 用 `MERGE INTO` |
| 自增 `AUTOINCREMENT` / `rowid` 主键 | 支持 | **不支持**（无 sequences） | 应用侧 UUID/ULID，或 `uuid()` 默认值（字面量限制见下） |
| 非字面量默认值 `DEFAULT now()` | 支持 | **不支持** | 插入时显式赋值，或应用层生成 |
| 索引 | 支持 | **不支持** | 分析引擎靠统计裁剪 + 排序表（`SET SORTED BY`）+ 分区替代 |
| 外键 / CHECK | 支持 | 不支持（仅 `NOT NULL`） | 应用层校验 |
| 生成列 | 支持 | 不支持 | 物化为普通列，写入时计算 |
| `INSERT ... ON CONFLICT` | 支持 | **不支持** | `MERGE INTO ... WHEN MATCHED THEN UPDATE WHEN NOT MATCHED THEN INSERT`（单动作限制） |
| `last_insert_rowid()` | 支持 | 不支持 | `RETURNING` 子句 |
| 事务内多条 DDL | 支持 | 支持（DDL 也有事务语义，更优） | — |
| ALTER 改列类型 | 宽松（动态类型） | 仅无损提升（int8→…→int64 等） | 宽化之外的转换需 CTAS 重建 |
| ENUM/UNION/VARINT/BIT | 部分支持 | 不支持 | 迁移时映射 VARCHAR/INT（§九） |
| 嵌套类型 LIST/STRUCT/MAP | 无 | **新增支持** | 新能力 |
| Time travel / change feed | 无 | **新增支持** | `AT (VERSION=>...)` / `table_changes` |
| 分区 / 排序表 | 无 | **新增支持** | `SET PARTITIONED BY` / `SET SORTED BY` |

## 六、模块改动清单

### 新增

| 模块 | 职责 |
|---|---|
| `internal/database/ducklake/extension.go` | 扩展 INSTALL/LOAD、DuckDB ≥ v1.5.2 版本断言、extension_dir 配置 |
| `internal/database/ducklake/factory.go` | 实现 `database.Factory`：建 DuckDB 实例、CREATE SECRET、ATTACH、SET 资源与路径边界 |
| `internal/database/ducklake/syncer.go` | CatalogSyncer：debounce 同步、冷启动下载、快照水位、版本保留、校验 |
| `internal/database/ducklake/options.go` | ducklake `set_option` 默认值与按库覆盖（inlining/compression/target_file_size 等） |
| `internal/database/ducklake/maintenance.go` | CHECKPOINT / merge / expire / cleanup / rewrite / flush_inlined 的封装（供 jobs 调用，全部支持 dry_run） |
| `internal/database/ducklake/audit.go` | 写事务提交消息注入（§4.7） |
| `internal/database/ducklake/inspect.go` | snapshots() / list_files() / settings() 的管理面封装（数据库状态接口） |
| `internal/jobs/maintenance_handler.go` | 周期性维护任务，接入现有 jobs.Worker |

### 修改

| 模块 | 改动 |
|---|---|
| `internal/app/app.go` | 装配 `DuckLakeFactory` 替换 `TursoFactory`；DevMode 走本地 DATA_PATH |
| `internal/config/config.go` | 新增 `database.ducklake` section（§4.8） |
| `internal/database/sqlguard/` | DuckDB 方言关键字表 + §4.5 拒绝清单（含 CALL/SET/ATTACH 等） |
| `internal/database/serialize.go` | §4.6 类型映射 |
| `internal/database/query.go` | `LastInsertID` 恒 0 的语义标注；`RowsAffected` 容错保留 |
| `internal/api/sql_handler.go` | 响应增加 `durability` 字段；可选 `snapshot_version` 请求参数（Phase 5） |
| `internal/api/database_handler.go` | 数据库状态响应增加同步水位（`current_snapshot`/`last_synced_snapshot`） |
| `internal/database/cache/` | 缓存单元从 libSQL 文件变为 `{catalog.sqlite + duckdb 临时目录}`，淘汰逻辑不变 |
| `internal/observability/` | 桥接 `DuckLakeMetadata` 日志为指标（catalog 查询延迟分位） |

### 退役（Phase 4 验收后删除）

- `internal/database/turso/`（整个包）
- `internal/database/tursofactory.go`、`localfactory.go`（DevMode 并入 DuckLakeFactory）
- go.mod 中的 turso 相关依赖（如有）

## 七、分阶段实施计划

### Phase 1：核心运行时（本地 DATA_PATH，不接 S3）

> 已决策：不做独立 PoC 阶段，直接进入实现。原 PoC 的一票否决项下沉为 Phase 1 的**前置验证任务**，
> 在写首批单测时一并验证，失败立即回修设计（备选：catalog 用 PostgreSQL、或引擎侧禁用外部访问改用预签名通道）。

**前置验证任务（随首批单测完成，一票否决）**：

0. go-duckdb 绑定的 DuckDB 版本 **≥ v1.5.2**（§2.1，硬要求）。
1. 可 `INSTALL/LOAD ducklake + sqlite + httpfs`（extension_dir 预置路径可用）。
2. SQLite catalog + 本地 DATA_PATH：建表/写入/事务/回滚/time travel（`AT (VERSION => ...)`）/ change feed（`table_changes`）全部可用。
3. `SET allowed_paths` 能阻止用户 SQL 读取本库前缀之外的路径（含 `read_parquet` 等表函数）。
4. `memory_limit`/`threads` 生效；并发连接行为符合 `database/sql` 池预期。
5. 复合类型（LIST/STRUCT/MAP/DECIMAL/TIMESTAMPTZ/VARIANT/GEOMETRY）经 `database/sql` 读出后的 Go 表示，确定 §4.6 serialize 方案。
6. 数据内联行为：小写不产生 Parquet 文件、`current_snapshot()` 水位正确推进、`COPY FROM DATABASE` 备份含内联表。

**实现任务**：

- 实现 `internal/database/ducklake`（extension/factory/options/audit/inspect，syncer 仅本地空实现）。
- app 装配切换、config 新增 section、DevMode 切到 DuckLake。
- sqlguard 方言改造 + serialize 类型映射 + `LastInsertID` 语义调整。
- data_handler 的文档 ID 策略核对：当前已生成 UUID，与 DuckLake 无 sequences 的约束兼容（§2.11）。
- **验收**：前置验证 7 项全部有测试证据；现有 SQL API 测试（sql_handler 20 个、sqlguard 11 个）在 DuckLake 引擎上全部通过；data_handler 的集合/文档 CRUD 正常；`go test ./...` 绿。

### Phase 2：S3 持久层与 CatalogSyncer

- 前置验证（MinIO）：SQLite catalog + S3 DATA_PATH 写 Parquet 到 S3、读回、`COPY FROM DATABASE` 一致性备份 catalog、`ducklake_delete_orphaned_files` 清理验证。
- httpfs + CREATE SECRET 装配（§2.15 IAM 模式）；DATA_PATH 指向 S3。
- CatalogSyncer 完整实现：debounce 批量同步（默认）、sync_on_commit 可选、关闭强制同步、冷启动下载、descriptor/data_path 一致性校验、按快照 id 的版本保留。
- API 响应增加 `durability` 字段；数据库状态接口暴露同步水位；指标增加同步水位差/延迟/失败计数。
- **验收（对齐 v1.0 阶段 A 标准）**：删除整个本地缓存后仅凭 S3 打开数据库且数据完整；kill -9 注入在「commit 后、防抖同步完成前」窗口，恢复后 catalog 回退到上一同步点、无脏数据、孤儿文件可被清理；S3 不可用时同步失败有明确指标与降级状态，不静默返回已持久化。

### Phase 3：维护任务与资源治理

- `maintenance_handler.go`：周期 `CHECKPOINT`（含 flush_inlined/expire/merge/rewrite/cleanup/orphan 全链路）、按 §4.8 配置 expire/delete 窗口、catalog versions 裁剪。
- 维护策略默认先 `dry_run` 记录再执行；压缩采用分层策略（<1MB→5MB→32MB，§2.9），大表用 `max_compacted_files` 控内存。
- 缓存管理适配（catalog 文件 + DuckDB 临时目录计入容量）。
- 压测基线：冷启动延迟、debounce 同步的吞吐/水位表现、内联 flush 频率与 catalog 体积曲线、小文件合并效果。
- **验收**：连续写入 1 万小事务后文件数被 merge 收敛、catalog 体积被 flush+版本裁剪收敛；expire+cleanup 后 S3 存储量下降可观测；维护任务失败有告警指标且不影响在线读写。

### Phase 4：迁移与旧链路退役

- 迁移工具 `cmd/simplebase-migrate`（§九）：Turso/libSQL 库 → DuckDB sqlite 扩展直读 → CTAS + 类型映射 → 校验 → 切换。
- 双跑比对后删除 `internal/database/turso/` 与相关依赖。
- **验收**：至少一个生产等效数据集迁移校验通过；仓库不再存在两套可写数据库链路；文档更新（README、部署、迁移指南含 §五兼容矩阵）。

### Phase 5：新能力开放（可独立排期，不阻塞前四期）

- SQL API 支持 `snapshot_version`/`snapshot_time` 参数（time travel 查询，§2.7）。
- **Change Feed API**：`GET .../tables/{t}/changes?from=&to=` 封装 `table_changes`，为 Realtime/CDC 订阅打底。
- **批量导入**：`POST .../tables/{t}/register_files` 封装 `ducklake_add_data_files`，用户把已有 Parquet 注册进表（§2.14）。
- 外部表能力：允许用户在受控前缀下 `read_parquet/read_csv`（配合 allowed_paths 与显式授权）。
- DuckLake `ENCRYPTED`（Parquet 静态加密，§2.13）与分区/排序表助手。
- 只读副本探索：`READ_ONLY` ATTACH + 定期拉取 catalog（官方 Public DuckLake/Remote Data Path 模式）。
- 共享 DuckDB 引擎（单实例多 ATTACH）降低多库内存占用。

## 八、API 与协议变化

| 端点 | 变化 |
|---|---|
| `POST .../query` | 响应增加 `durability`；可选请求参数 `snapshot_version`（Phase 5） |
| `POST .../execute` / `batch` | 响应增加 `durability`；`last_insert_id` 废弃（恒 0，文档引导 `RETURNING` 与应用侧 UUID） |
| `GET .../databases/{id}` | 响应增加 `snapshot` 水位：`current_snapshot` / `last_synced_snapshot` / `sync_lag` |
| `POST .../databases/{id}/open` | 语义不变（预热 = 下载 catalog + ATTACH） |
| 管理/认证/LLM/usage/audit API | **不变** |

SQL 方言变化见 §五兼容矩阵，迁移指南随 Phase 4 发布。

## 九、存量数据迁移

1. **libSQL/SQLite 源**：数据文件本质是 SQLite 格式 → DuckDB sqlite 扩展直接 `ATTACH 'old.db' (TYPE sqlite)` 读取，逐表 `CREATE TABLE {lake}.t AS SELECT * FROM old.t`。
2. **类型映射**（参考官方迁移脚本的 TYPE_MAPPING，Migrations §DuckDB to DuckLake）：
   - SQLite 动态类型 → DuckDB 静态类型，重点验证 REAL/TEXT 混合列、INTEGER 主键；
   - 不支持的类型映射：`VARINT→::VARCHAR::INT`、`ENUM/UNION→::VARCHAR`、`BIT→::VARCHAR`、定长数组→`LIST`；
   - 主键/索引/生成列/非字面量默认值：按 §五兼容矩阵在迁移报告中逐项列出，由应用层决策。
3. **DuckDB 源**（若有）：`COPY FROM DATABASE my_duckdb TO my_ducklake` 一条命令（全部特性受支持时）。
4. **Parquet 存量**：`ducklake_add_data_files` 直接注册，零拷贝（注意所有权移交，§2.14）。
5. **校验**：行数、抽样哈希、逐列类型核对；迁移报告落 catalog job。
6. Turso/S3 上的存量库：先按 v1.0 恢复流程落本地，再走上述流程。

## 十、风险与应对

| 风险 | 等级 | 应对 |
|---|---|---|
| go-duckdb CGO 构建/交叉编译复杂 | 中 | 项目已有 CGO；CI 固定 macOS/Linux 两目标；版本锁定 |
| go-duckdb 版本滞后于 DuckDB v1.5.2 | 高 | Phase 1 前置验证第 0 项一票否决；不满足则评估自编译 DuckDB C API 或暂缓 |
| 扩展运行时下载依赖外网 | 中 | 镜像预置 extension_dir；启动时版本断言 |
| catalog 同步窗口丢「已提交未同步」事务 | 中 | 语义与 DuckLake 官方备份一致并写进 API 契约；debounce 窗口默认 200ms 可控，强持久化场景切 sync_on_commit；内联写无孤儿残留；孤儿清理兜底 |
| 内联数据致 catalog 膨胀、同步带宽上升 | 中 | `data_inlining_row_limit` 调优 + CHECKPOINT flush + versions 裁剪；Phase 3 压测出体积曲线 |
| DuckDB 内存占用（分析引擎默认吃 80% 内存） | 中 | 每实例 `memory_limit` + `threads` 硬限制；Registry MaxOpen + 空闲淘汰 |
| `allowed_paths` 对表函数/S3 前缀不生效（越权读） | 高 | Phase 1 前置验证一票否决；不生效则禁用外部访问并改用内部通道 |
| 用户 SQL 语义断层（无 PK/索引/sequences/ON CONFLICT） | 高 | §五兼容矩阵写进用户文档与迁移指南；data_handler 文档模型已用 UUID 不受影响；管理端 UI 增加方言提示 |
| catalog 含加密密钥（ENCRYPTED 库） | 中 | bucket 私有 + SSE + 最小 IAM；catalog 对象访问审计 |
| DuckLake 扩展快速演进、catalog schema 迁移 | 低 | `AUTOMATIC_MIGRATION=false`；升级走显式流程 + 迁移前 catalog 备份 |
| 单写实例约束被误突破 | 低 | 沿用 v1.0 部署约束：副本数固定 1、启动冲突检测；DuckLake 的 snapshot_id PK 冲突检测与 SQLite catalog 锁重试是额外防线而非保证 |

## 十一、开放问题

1. **catalog 版本保留策略**：`keep_versions` 与 `expire_older_than` 快照过期策略如何联动，才能支持「库级 PITR」？（Phase 3 定）
2. **大库 catalog 膨胀**：高频写场景 catalog.sqlite 增长到数百 MB 时，debounce 全量 PUT 的带宽成本是否可接受？是否需要增量同步（WAL shipping）？（Phase 2 压测后定，默认不同步 WAL）
3. **多库共享引擎**的收益/风险比，是否提前到 Phase 3？（默认不提前）
4. **data_handler 的 JSON 文档模型**是否改用 DuckDB 原生 JSON/VARIANT 类型存储（现为 TEXT）？注意 VARIANT 在 SQLite catalog 下无法内联（§2.8）。（Phase 1 顺带评估）
5. **change feed 与 audit 的关系**：`table_changes` 是否作为数据面审计的唯一来源，替代部分 API 面审计？（Phase 5 定）

## 附：参考资料

- [DuckLake Documentation (1.0 stable)](https://ducklake.select/docs/stable/) ｜ 本地完整镜像 `docs/ducklake-docs.md`
- [Connecting（ATTACH 参数全表）](https://ducklake.select/docs/stable/duckdb/usage/connecting)
- [Choosing a Catalog Database（SQLite catalog 语义）](https://ducklake.select/docs/stable/duckdb/usage/choosing_a_catalog_database)
- [Configuration（set_option 全表）](https://ducklake.select/docs/stable/duckdb/usage/configuration)
- [Snapshots（快照函数与提交消息）](https://ducklake.select/docs/stable/duckdb/usage/snapshots)
- [Time Travel](https://ducklake.select/docs/stable/duckdb/usage/time_travel) ｜ [Data Change Feed](https://ducklake.select/docs/stable/duckdb/advanced_features/data_change_feed)
- [Data Inlining](https://ducklake.select/docs/stable/duckdb/advanced_features/data_inlining)
- [Transactions](https://ducklake.select/docs/stable/duckdb/advanced_features/transactions) ｜ [Conflict Resolution](https://ducklake.select/docs/stable/duckdb/advanced_features/conflict_resolution)
- [Checkpoint](https://ducklake.select/docs/stable/duckdb/maintenance/checkpoint) ｜ [Merge Files](https://ducklake.select/docs/stable/duckdb/maintenance/merge_adjacent_files) ｜ [Expire](https://ducklake.select/docs/stable/duckdb/maintenance/expire_snapshots) ｜ [Cleanup](https://ducklake.select/docs/stable/duckdb/maintenance/cleanup_of_files)
- [Backups and Recovery（catalog 备份与崩溃语义）](https://ducklake.select/docs/stable/duckdb/guides/backups_and_recovery)
- [Access Control（IAM 模式）](https://ducklake.select/docs/stable/duckdb/guides/access_control)
- [Unsupported Features](https://ducklake.select/docs/stable/duckdb/unsupported_features)
- [DuckDB to DuckLake 迁移](https://ducklake.select/docs/stable/duckdb/migrations/duckdb_to_ducklake)
