# 多实例一致性：DuckLake 方案的冲突分析与收口计划

> **仓库**：SimpleBase（https://github.com/linkxzhou/SimpleBase）
> **状态**：plan-only。本 PR 只新增本文，不改产品代码。
> **日期**：2026-09-27 初稿；2026-09-28 合并评审修订（v2）。
> **Verified against**：`main` @ `5579782`。下文每条结论都标注了核对过的文件与行号。
> **关联**：`plan/planv2.0/db-ducklake-plan.md`（§2.5 冲突解决 / §4.4 CatalogSyncer / §2.10 备份语义）、
> `plan/planv3.0/key-value-ducklake-plan.md`（§2 并发模型）、`plan/planv3.0/database-always-open-plan.md`（常驻句柄）、
> `docs/ops/deployment.md`（`replicas=1` 硬性约束）、`docs/database/ducklake.md`（DuckLake 1.0 规格镜像）。
>
> **v2 修订摘要**（2026-09-28 评审后）：
> 1. 新增 §1.3-§1.5「DuckLake 如何用 Parquet 解决一致性」——理论依据，缺了它后面的方案选择无从判断。
> 2. **去掉对 `If-Match` on PUT 的依赖**：生产端点腾讯云 COS 不支持它（仅有 `x-cos-forbid-overwrite` ≈ create-if-absent，
>    且存储桶开启版本控制时失效）。v1 的租约续约与 ETag 守卫两道防线全部押在这个能力上 → 落地即失效。
>    v2 全部改为只依赖 `create-if-absent`（所有端点的最小公共能力）。
> 3. **新增 P0-0「关闭远端 data inlining」并列为第一优先级**：`data_inlining_row_limit: 100` 把用户数据写进
>    catalog，是 §3.1/§3.8「不可恢复」的唯一成因。关掉后从"静默丢数据"降级为"丢指针、可从 Parquet 重建"。
> 4. **catalog 远端表示改为「不可变快照 + manifest 指针」**（原 ETag 守卫方案的替换）：把 Parquet 的
>    UUID 命名策略搬到 catalog 上，结构上消除"覆盖"这一操作，使 CAS 不再必需。
> 5. **租约粒度从实例级改为 per-database**，失租处理从"退出进程"改为"释放该库句柄"；
>    过期判定改为 epoch 观察式（v1 的"用 LastModified 避免时钟漂移"论证不成立：`now` 仍是本地时钟，
>    跨时钟减法没有被消除，且 `LastModified` 只有秒级精度）。
> 6. **控制面与数据面分离**（新增 §6.4）：`sys_*` 需要 CAS/UNIQUE/行锁，DuckLake 一概不提供
>    （`ducklake.md:1627`），与 catalog 后端无关。控制面独立存储是比"整体换 PG catalog"小一个数量级的方案 3 前置。
> 7. 只读副本直接采用官方 **Frozen DuckLake** 模式（`ducklake.md:1778`/`:4082` 已确认 `READ_ONLY` ATTACH 支持，
>    删除 v1 §5.2 的"需前置验证"与退化分支）。
> 8. 补齐 v1 缺失的 **split-brain 恢复流程**（§9）与**孤儿账本**（§4.8，失租实例留下的孤儿否则永远无人回收）。

---

## 0. 一句话结论

**当前实现不支持多实例。** 不是"可能不一致"，而是两个实例同时跑会**静默丢已提交事务**、
**误删对方的 Parquet 数据文件**、并且**元数据面直接分裂成两套**。
根因是 DuckLake 的并发正确性依赖「唯一共享的 catalog 数据库」，而本仓库把 catalog 实现成了
「每实例本地 SQLite 副本 + 全量文件覆盖同步到 S3」，DuckLake 自带的冲突检测机制因此完全失效。

**收口的第一步不是加锁，而是三件零依赖的事（P0）**：关闭远端内联、catalog 远端表示不可变化、
`EnsureLocalCatalog` 不再无条件覆盖。三者都只依赖 `create-if-absent`（COS 可用），
即可把 §3.9 里 8 条故障中的 6 条从"静默不可恢复"变成"可检测可恢复"；租约（P1）随后才做，
且按 per-database 粒度做——实例级租约 + 失租退出的终点是永久单写，堵死了通往水平扩展的路。

本文结构：**§1 冲突分析的机制基础**、**§2-§3 故障模式与损坏路径**、
**§4-§6 收口方案**（P0/P1 堵漏 → 只读扩展 → 控制面分离与共享 catalog）、
**§7-§11 阶段、验收、运维与不做清单**。

---

## 1. 一致性依赖点：DuckLake 要什么，我们给了什么

### 1.1 DuckLake 规格的要求

`docs/database/ducklake.md` §Specification §Introduction 开篇即写明两个构件：

| 构件 | 规格要求 |
|---|---|
| **Catalog database** | 「requires a database that supports **transactions and primary key constraints** as defined by the SQL-92 standard」 |
| **Data storage** | Parquet 文件；**从不原地修改**，一致性要求极低 |

并发写的正确性机制（`plan/planv2.0/db-ducklake-plan.md` §2.5 已转述）：

- `ducklake_snapshot.snapshot_id` 是**主键**。两个并发事务尝试写入同一个 `snapshot_id` 时，
  后者因 PK 冲突失败 → 扩展读取 `ducklake_snapshot_changes` 做变更集分析 → 无逻辑冲突则自动重试
  （`ducklake_max_retry_count` 默认 10、`ducklake_retry_wait_ms` 100、`ducklake_retry_backoff` 1.5）。
- 逻辑冲突（同名建表、删改竞争、压缩与删除竞争）才中止事务（`ducklake.md:2883-2908`）。

**这套机制成立的前提是：所有 writer 写的是同一个 catalog 数据库实例。**

### 1.2 本仓库的实现

| 环节 | 实现 | 位置 |
|---|---|---|
| catalog 位置 | 本地文件 `{cache_dir}/{db-id}/catalog/catalog.sqlite` | `ducklake/factory.go:217-225`（`layoutFor`）、`:146-151`（`ATTACH 'ducklake:sqlite:...'`） |
| catalog 同步 | `ATTACH staging (TYPE SQLITE)` → `COPY FROM DATABASE` → 读整个文件 → `PutBytes` **覆盖** | `ducklake/catalog_syncer.go:277-320` |
| 触发时机 | 写提交后 `MarkDirty` → 200ms debounce → `Sync` | `catalog_syncer.go:107-137`；`registry/handle.go:100-117` |
| 冷启动 | `Head` 命中就 `DownloadFile` **覆盖本地** catalog.sqlite | `catalog_syncer.go:389-411`；`factory.go:53-57` |
| 单写保证 | 进程内 `map[string]*entry` + `sync.Mutex` | `registry/registry.go:25-41`；包注释 `registry/handle.go:1-5` 明确写「**这不是分布式一致性方案；部署层必须保证全局只有一个可写实例运行**」 |
| 条件写 | **无**。`PutBytes(ctx, key, data, contentType)` 没有 `If-Match` / `If-None-Match` 参数 | `objectstore/blob.go:20-47` |
| 连接池 | `SetMaxOpenConns(1)` / `SetMaxIdleConns(1)` | `factory.go:78-79` |
| 数据内联 | `DATA_INLINING_ROW_LIMIT 100` 写进 ATTACH 与 set_option | `factory.go:138`、`:180`；`options.go:44`；`config.yaml:32` |

### 1.3 机制基础：DuckLake 究竟怎么"通过 Parquet"解决一致性

**它不通过 Parquet 解决一致性——它把一致性从对象存储里彻底搬走了。** 这是理解后续所有方案选择的钥匙。

Parquet 层只承担三条不变量：

| 不变量 | 机制 | 核对 |
|---|---|---|
| **文件名全局唯一** | 文件名 `ducklake-{UUIDv7}.parquet` 由 writer 本地生成，无需协调 | `ducklake.md:4319`、`:4692-4706` 实际文件名样本 |
| **永不原地修改** | 数据文件 append-only；删除写独立 `*-deletes.parquet`（`ducklake_delete_file`）；压缩 = 写新文件 + 改 catalog 指针 | `ducklake.md:876`、`:1044`；官方「Files in DuckLake are immutable」 |
| **可见性由 catalog 决定** | `ducklake_data_file.begin_snapshot / end_snapshot` 区间控制；未被 catalog 引用的文件对所有读者不可见 | `ducklake.md:1004-1005` |

推论：**对象存储只需要「单对象 PUT 原子 + read-after-write」**。不需要原子 rename、不需要 LIST 强一致、
不需要条件写、不需要重试窗口（Iceberg/Delta 需要在 S3 上重写 manifest list + 换 root pointer，DuckLake 连这一层都不要）。

提交协议（真正的一致性点）：

```text
1. writer 本地生成 UUID 文件名，直接 PUT parquet 到 data/{schema}/{table}/
   —— 此时文件是「未被引用的字节」，任何读者都看不到，可并发、无冲突
2. BEGIN（catalog 数据库的一个真 SQL 事务）
     INSERT ducklake_data_file(...)            -- 注册文件
     UPDATE ducklake_table_stats / column_stats
     INSERT ducklake_snapshot(snapshot_id = max+1, next_catalog_id, next_file_id)
     INSERT ducklake_snapshot_changes(snapshot_id, changes_made)
   COMMIT                                       ← 唯一的提交点（ducklake.md:425-473）
3. 失败：parquet 留在 S3 成为孤儿，由 ducklake_delete_orphaned_files 回收
```

冲突后不是直接失败：读 `ducklake_snapshot_changes` 做变更集分析（`ducklake.md:2876-2881`）。
无逻辑冲突 → **自动重试且不重写任何 Parquet**——因为 parquet 已写好且不可变，重试只是换 snapshot_id
重新 INSERT 一次元数据。有逻辑冲突 → 中止并报错。**「Parquet 不可变 + UUID 命名」的真正作用是
让冲突重试变成纯元数据操作（成本 ≈ 一次 INSERT），这是 DuckLake 敢用乐观并发的前提，
而不是它解决一致性的手段。**

### 1.4 三条硬推论（收口方向的依据）

1. **数据面在多实例下天然安全。** 两个实例并发写 parquet 不会互相覆盖（UUID）、不会互相破坏（不可变）。
   §3 的全部损坏路径**没有一条**源于 parquet 本身。
2. **危险有三类，全在 catalog 侧**：
   - (a) catalog 文件被整体覆盖（§3.1、§3.3、§3.8）——因为我们把「一个 SQL 事务」退化成「一次全文件 PUT」；
   - (b) 基于单边 catalog 视图的**删除**动作（§3.2 的 `delete_orphaned_files` / `cleanup_old_files` / `expire_snapshots`）；
   - (c) **data inlining** 把数据从 parquet 搬进 catalog（§3.1 放大因素 1）——它主动破坏 §1.3 第一条不变量，
     把「可重建」变成「不可恢复」。
3. **正确的收口方向是「收缩 catalog 的职责 + 让 catalog 的远端表示变成不可变对象序列」**，
   而不是「给整个实例加一把大锁」。把 Parquet 的 UUID 命名策略搬到 catalog 上（§4.4），
   「覆盖」这个操作从协议里消失，对 CAS（`If-Match`）的依赖随之消失。

### 1.5 本仓库结论

DuckLake 的 PK 冲突检测**永远不会触发**：实例 A 和实例 B 各自写自己本地的 `catalog.sqlite`，
两边都从 `snapshot_id=100` 推进到 `101`，各自的 SQLite 里都只有一行 `101`，没有任何主键冲突。
冲突被推迟到 S3 层，而 S3 层是**无条件覆盖**（last-writer-wins），没有检测、没有重试、没有报错。

---

## 2. 部署形态决定故障模式（两种，都是坏的）

一个容易被忽略的事实：S3 key 前缀里嵌了实例 ID。

```go
// internal/app/app.go:297-300
keys := objectstore.KeyBuilder{
    RootPrefix:  cfg.S3.Prefix,
    Environment: cfg.Instance.ID,      // ← 实例 ID 进了对象前缀
}
```

`KeyBuilder.base()` = `joinKey(RootPrefix, Environment)`（`objectstore/keys.go:115-117`），
所有 key 都在它之下（`DatabasePrefix` → `objectstore/keys.go:121-129`）。
`ducklake.RemoteStorage.Environment` 同样取 `cfg.Instance.ID`（`app.go:531`）。

因此「起两个实例」有两种截然不同的后果：

| 形态 | `instance.id` | 结果 | 严重度 |
|---|---|---|---|
| **A. 前缀分叉** | 不同（如 `prod-1` / `prod-2`） | 两个实例读写**完全不同的 S3 前缀**。用户在 A 建的库/项目在 B 上不存在；请求落到哪个实例决定看到哪份数据。不丢数据，但**行为不确定**，负载均衡后表现为"数据随机消失/出现" | 高（可恢复） |
| **B. 前缀重叠** | 相同（如都配 `prod`，典型的"横向扩容"直觉做法） | 共享同一前缀 → §3 的全部损坏路径 | **致命（不可恢复）** |

形态 A 还有一个隐藏坑：`instance.id` 参与了对象前缀，意味着**它不能改**。
改 `instance.id` 等价于换一个全新的空 bucket 前缀，现有所有库"消失"。
这一点目前没有任何文档或启动校验提示（`docs/ops/deployment.md:32` 只写了
「`id` 用于对象前缀与日志身份，不提供分布式锁语义」，没说改了会丢数据）。

下文 §3 讨论形态 B（前缀重叠），因为它是"想做多实例"的人实际会配出来的形态。

---

## 3. 形态 B（前缀重叠）的损坏路径

按严重度排序。每条都给出可复现的时序。

### 3.1 【致命】catalog 互相覆盖 → 已提交事务静默消失

```text
T0  S3:  catalog.sqlite @ snapshot=100
T1  A.Open / B.Open → EnsureLocalCatalog 各自下载 snap=100      catalog_syncer.go:389
T2  A: INSERT 提交 → A 本地 catalog snap=101（含内联数据）
T3  B: INSERT 提交 → B 本地 catalog snap=101（内容不同！）
T4  A: debounce 到期 → Sync → PutBytes(catalog.sqlite, A@101)   catalog_syncer.go:310
    A: PutBytes(versions/101.sqlite, A@101)                     catalog_syncer.go:318
T5  B: debounce 到期 → Sync → PutBytes(catalog.sqlite, B@101)   ← 覆盖
    B: PutBytes(versions/101.sqlite, B@101)                     ← 连历史版本一起覆盖
T6  下次冷启动只能看到 B@101。A 在 T2 提交的事务彻底消失。
```

三个放大因素：

1. **内联数据使覆盖等于丢数据（唯一成因，也是 P0-0 的靶子）。** `data_inlining_row_limit: 100`
   （`config.yaml:32`，`options.go:44` 默认同值，`factory.go:138`/`:180` 双处写入）。
   DuckLake 数据内联（`docs/database/ducklake.md` §Data Inlining，`:226-228`、`:1152-1163`）
   意味着 ≤100 行的写入**数据本身就存在 catalog SQLite 里**，不产生 Parquet。
   BaaS 的典型小写入几乎全落在这条路径上 → catalog 被覆盖 = 数据行直接消失，
   连"孤儿 Parquet 可以捞回来"都做不到。**反之，把远端模式下内联关闭后，
   这条故障从"静默丢数据"降级为"丢指针"——parquet 仍在 S3，
   可用 `ducklake_add_data_files`（`ducklake.md:3733-3764`）重新登记恢复（见 §9 恢复流程）。**
2. **历史版本不是退路。** `versions/{snapshot_id}.sqlite` 以 snapshot id 命名
   （`objectstore/keys.go:165-174`）。两实例分配同一个 id，历史版本同样被覆盖。
   `pruneVersions`（`catalog_syncer.go:341-358`）还会**算出** `current - keep` 这个 key 直接
   `Delete`——不 LIST、不确认该版本是否实际存在、是否是最后一个完好副本。
   若 `keep=3` 而中间几个版本 PUT 失败过，它会删掉实际存在的最后一个好版本。
3. **完全静默。** `Sync` 返回 `nil` → `recordSync(err=nil)` 记 `outcome="ok"`
   （`catalog_syncer.go:360-386`）→ `simplebase_catalog_sync_total{outcome="ok"}` 递增。
   API 照常返回 `durability: committed_local`（`factory.go:294-306`）。
   **没有任何指标、日志或响应字段能暴露这次丢失。**

另有两条 v1 未记录的实现缺陷（本次评审发现，与 §4.4/§4.5 的修复绑定）：

- **`COPY FROM DATABASE` 的快照取值顺序反了**：`syncOnce` 先 `COPY`（`catalog_syncer.go:289`）
  得到 staging 文件，**再** `CurrentSnapshot`（`:295`）读快照号。当前因 `SetMaxOpenConns(1)`
  （`factory.go:78`）两值间不会有写入，但一旦将来放开连接数，`versions/{snap}.sqlite`
  就会写进一个与 `snap` 不匹配的文件体。应在 COPY 前取并断言不变。
- **COPY 期间全库停顿**：同一单连接上执行 `COPY FROM DATABASE`，catalog 增长到几十 MB 后，
  每次 debounce 同步都会造成一次全库停顿。这是单实例就存在的可用性问题，
  `simplebase_catalog_size_bytes`（§4.9）是它的早期信号，也是 §6.3「必须走共享 catalog」的前置指标。

### 3.2 【致命】维护任务误删对方的数据文件（真删，不可恢复）

`docs/database/ducklake.md` §Checkpoint 的 `CHECKPOINT` 按序执行：

```text
ducklake_flush_inlined_data → ducklake_expire_snapshots → ducklake_merge_adjacent_files
→ ducklake_rewrite_data_files → ducklake_cleanup_old_files → ducklake_delete_orphaned_files
```

其中 `ducklake_delete_orphaned_files` 的语义是「清理**不被 catalog 追踪**的文件」
（`db-ducklake-plan.md` §2.9）。判定依据是**执行它的那个实例的 catalog 视图**。

```text
T1  B 写入大批数据 → s3://.../data/main/orders/xxx.parquet（合法，B 的 catalog 引用它）
T2  A 的 maintenance job 跑 CHECKPOINT
T3  A 的 catalog 里没有 xxx.parquet（A 从没见过 B 的 catalog）
T4  ducklake_delete_orphaned_files 判定为孤儿 → DELETE
T5  B 的 catalog 仍引用 xxx.parquet → 后续查询 "file not found"
```

这比 §3.1 更彻底：§3.1 丢的是 catalog 指针（数据文件还在 S3，理论上可人工捞），
§3.2 是**数据文件本身被删除**。同理 `ducklake_cleanup_old_files`、
`ducklake_expire_snapshots` 都会基于单边视图删东西。

> 注 1：Parquet 不可变性在这里帮不上忙——不可变保证"不被改坏"，不保证"不被删掉"。
> 注 2：`db-ducklake-plan.md` §2.10 已引用官方警告「压缩/清理任务只应在手动备份前做」，
> 但那是针对单实例备份窗口的讨论，没覆盖多实例互删。

### 3.3 【致命】ID 空间重复分配 → catalog 永久不可合并

`ducklake_snapshot` 行携带 `next_catalog_id` / `next_file_id`，
`ducklake_table_stats` 携带 `next_row_id`（`docs/database/ducklake.md` §Queries §Snapshot Creation / §INSERT，`:466-469`）。
三者都是单调递增的全局分配器。

两实例从同一个 snapshot 分叉后，会把**同一个 `table_id` 分给不同的表**、
**同一个 `row_id_start` 分给不同的数据文件**。后果：

- 事后无法做"合并两份 catalog"的补救（没有可用的冲突解决规则）。
- 行级血缘（`rowid` 虚拟列，`db-ducklake-plan.md` §2.7）语义破坏，
  Change Feed（`table_changes`，Phase 5 规划的 CDC 底座）不可信。

### 3.4 【致命】系统库分裂 → 元数据面变成两套

`systemdb/locator.go` 的 `locator.json` 存**本地文件系统**：

```go
// internal/app/app.go:320
LocatorDir: filepath.Join(cfg.Database.CacheDir, "system")
```

```go
// internal/systemdb/bootstrap.go:41-56
loc, ok, err := LoadLocator(in.LocatorDir)
if !ok {
    loc = Locator{ DatabaseID: uuid.NewString(), ... }   // ← 本地没有就新建一个 UUID
    SaveLocator(in.LocatorDir, loc)
}
```

两个实例的 `cache_dir` 是各自容器的本地盘（`docs/ops/deployment.md:46`
明确「本地缓存可完全丢弃」），所以：

**两个实例会各自 `uuid.NewString()` 创建一个不同 `database_id` 的系统库。**

系统库装着整个元数据面（`systemdb/migrate.go` 的 `sys_*` 表）：

| 表 | 分裂后的症状 |
|---|---|
| `sys_projects` / `sys_databases` | 在 A 建的项目/库，在 B 上 404 |
| `sys_api_keys` | A 签发的 API Key 在 B 上认证失败（`auth.NewSQLAPIKeyRepository(s.db)`，`systemdb/store.go:64`） |
| `sys_users` / `sys_sessions` | 登录会话不互通，请求打到另一实例即掉线（`app.go:356-361`） |
| `sys_cron_jobs` | 见 §3.5 |
| `sys_metrics` / `sys_logs` | 监控数据一半在 A 一半在 B，控制台看到的是残缺曲线 |
| `sys_migration_versions` | 各自跑一遍迁移（`ApplySystemMigrations`，`bootstrap.go:87`） |

注意：即使把 `cache_dir` 挂到共享卷来"解决"这点，也只是让两个实例
**同时以 writer 模式打开同一个本地 SQLite catalog 文件**，
撞上 DuckDB sqlite 扩展的文件锁，或者更糟——写坏文件。这不是修复。

### 3.5 【高】依赖系统库 CAS 的调度器重复执行

`internal/cronjob/scheduler.go:2` 的实现范式是「tick 扫描 + CAS 认领 + 并发上限」，
CAS 落在 `ClaimCronJob`（`systemdb/cron_jobs.go:274-289`）：

```sql
UPDATE sys_cron_jobs
SET next_run_at = ?, last_run_at = ..., updated_at = ?
WHERE id = ? AND archived_at IS NULL AND enabled = 1
  AND ((? IS NULL AND next_run_at IS NULL) OR next_run_at = ?)
```

靠 `RowsAffected()==0` 判断"已被别人认领"。这在**同一个数据库行**上才有意义。
系统库分裂（§3.4）后，A 和 B 各自 `UPDATE` 自己那份 `sys_cron_jobs`，
**两边 CAS 都返回成功** → 定时任务每个实例各跑一遍。

后果按业务面展开：

- 用户 cronjob 触发的云函数重复执行（重复副作用：重复发邮件、重复扣款）。
- `internal/cloudagent/scheduler.go` 同范式 → Agent 任务重复调 LLM（重复计费）。
- `internal/usage` 配额计数分裂 → 限额失效（用户实际可用两倍额度）。
- `internal/audit` 流水分裂 → 审计不完整。
- `kv.Sweeper`（`database/kv/sweeper.go`，`key-value-ducklake-plan.md` §4.5）
  在两边各自扫 TTL，各自删各自的 catalog 副本 → 叠加 §3.1 的覆盖。

**即使做了共享 catalog（§6），这一条也不能靠 DuckLake 自身解决**——
依据 `ducklake.md:2900-2903`，数据类逻辑冲突只列了 insert / delete / alter / compact 的组合，
**`UPDATE`-vs-`UPDATE` 不在冲突列表内**（UPDATE = delete + insert，两个 UPDATE 各自产生
delete file 与 data file，变更集层面不相交）→ 大概率判为可重试 → 两边都 `RowsAffected=1` → CAS 仍失效。
**这不是"待实测"，设计上就要按 CAS 不可靠处理**：换乐观版本号列 + 显式校验，
或把调度认领移出 DuckLake（§6.4 控制面独立）。另注意：这意味着
`ClaimCronJob` 的 CAS 在**当前单实例**下也有隐患（任何绕过单连接的路径都会触发），
与多实例无关，修复优先级因此更高。

### 3.6 【高】KV 层失去最后防线

`plan/planv3.0/key-value-ducklake-plan.md` §2 的并发模型论证原文：

> **并发模型**：Registry 保证进程内每库唯一 writer（`internal/AGENTS.md` 硬性约束「不得绕过 Registry」）。
> kv 包所有写操作在同一 `*sql.DB` 上用 `BEGIN/COMMIT` 事务包裹「读-改-写」序列，
> 即可在无 ON CONFLICT 的前提下保证原子 upsert，**无并发窗口**。
> **已核对** `ducklake/factory.go` L78-79：`SetMaxOpenConns(1)` / `SetMaxIdleConns(1)`
> —— 单连接池意味着事务外的语句串行执行，事务内更是独占连接；
> 只要「读-改-写」在**同一个事务**里就绝对安全（不存在第二个并发连接做 interleaving）。

这段论证在多实例下**整体失效**，而且 **DuckLake 不提供任何兜底**——
`ducklake.md:1627` 明确不支持 PRIMARY KEY / UNIQUE 约束（这也是 kv 表设计里去掉代理主键、
`CREATE TABLE kv.keys ("key" VARCHAR NOT NULL, ...)` 无唯一约束的原因，见该计划 §3.2）。

具体症状：

| 机制 | 单实例（设计依赖） | 多实例（实际） |
|---|---|---|
| 类型守卫（WRONGTYPE） | 事务内先 `SELECT "type"` 再写 | 两实例同时判"不存在"→ 各建一个不同类型的同名 key |
| `kv.keys` 唯一性 | 靠守卫 + 单写 | 同名 key 出现多行，`Get` 结果取决于扫描顺序 |
| `len` 计数 | 事务内 `UPDATE len = len ± n` | 两边各 `+n` 后覆盖 → 计数错乱，`LRANGE` 负索引归一化出错 |
| `version` 单调 | 事务内 `+1` | 回退/重复 |
| 空结构自动删 key | 元素删空同事务删 `kv.keys` | 一边删 key、一边还在写元素 → 悬挂元素行（无外键，无 CASCADE） |
| `Rename` 组合事务 | 读旧+删新+改 key 列同事务 | 交叉执行产生两份数据行指向同一 key |

### 3.7 【中】陈旧读：即使做"1 写 N 读"也存在

`EnsureLocalCatalog` **只在 `Factory.Open` 调用一次**（`factory.go:53-57`），
之后没有任何 re-download / re-attach 路径。

叠加 `plan/planv3.0/database-always-open-plan.md` 的锁定决策
（「创建成功即 ready」「用户侧没有打开/关闭」「进程仍可在空闲时释放本地连接」）：

- 只读实例的 catalog 视图停留在 `Open` 那一刻，**无限期陈旧**。
- 唯一的刷新契机是 `CloseIdle`（默认 `idle_timeout: 5m`，`config.yaml:26`）
  把句柄关掉、下次 `Acquire` 重开。但常驻流量恰好会**阻止** idle 关闭
  → 高流量的只读实例反而**永远看不到新数据**。

这条决定了：**做只读副本必须新增主动刷新机制**（§5），不是改个配置就行。

### 3.8 【中】单实例也存在：Sync 失败后冷启动回滚覆盖本地

这一条与多实例无关，但同一处代码，顺手记录：

```text
T1  写提交 → 本地 catalog snap=105
T2  Sync 失败（S3 抖动/凭据过期）→ 本地仍 105，S3 仍 100
    recordSync 记 error + 告警，但本地文件保持领先        catalog_syncer.go:360-386
T3  CloseIdle 关闭句柄（BeforeClose → Flush 也失败）      registry/registry.go:256-285
T4  下次 Acquire → Factory.Open → EnsureLocalCatalog
    Head 命中 → DownloadFile 覆盖本地 catalog.sqlite      catalog_syncer.go:398-410
T5  本地回退到 snap=100。101~105 的事务（含内联数据）消失。
```

`EnsureLocalCatalog` 无条件覆盖，**不比较本地与远端的 snapshot 水位**。
多实例只是放大了这条路径（对方的 PUT 会让你的 Head 永远命中一个"更新但不是你的"版本）。

### 3.9 小结表

| # | 故障 | 触发条件 | 可恢复性 | 是否有告警 | P0 后（§4） |
|---|---|---|---|---|---|
| 3.1 | catalog 覆盖，事务消失 | 两实例写同一库 | **否**（内联数据） | 无（记 ok） | 可检测、可恢复（内联关闭 + 不可变快照） |
| 3.2 | 孤儿清理误删 Parquet | 任一实例跑 CHECKPOINT | **否** | 无 | 需 P1 门禁 + 孤儿账本 |
| 3.3 | ID 空间重复分配 | 两实例写同一库 | **否** | 无 | 可检测（`PutIfAbsent` 冲突即确证） |
| 3.4 | 系统库分裂 | 两实例启动 | 是（需人工合并） | 无 | §5.4 locator 移 S3 / §6.4 控制面独立 |
| 3.5 | cron/agent 重复执行 | §3.4 之后 | 副作用不可撤销 | 无 | §6.4（CAS 不可靠是设计事实） |
| 3.6 | KV 语义损坏 | 两实例写同一 KV 库 | 部分 | 无 | per-DB 租约保持"分片单写" |
| 3.7 | 只读实例陈旧读 | 只读实例 + 常驻流量 | 是（重启） | 无 | §5 Frozen DuckLake 模式刷新 |
| 3.8 | Sync 失败后回滚覆盖 | **单实例也会** | 否 | 部分（sync failed 日志） | P0-1 修复（水位比较） |

**最危险的不是"会坏"，而是"坏了完全不知道"。** 8 条里 7 条零告警。

---

## 4. 收口阶段一（P0+P1，本计划推荐先做）：把静默损坏变成明确失败

目标分两层：

- **P0（零依赖，单 PR 必做）**：不依赖租约、不依赖 `If-Match`、不改部署形态，
  就把 §3.1 / §3.3 / §3.8 从"静默不可恢复"变成"写入失败 + 告警 + 可恢复"。
- **P1（紧随其后）**：per-database 写互斥 + 维护任务门禁，堵住 §3.2 / §3.6。

**为什么不用 v1 的"实例级租约 + If-Match 守卫"**：目标生产端点腾讯云 COS（`config.yaml:48`）
**不支持 `If-Match` on PUT**（其 `If-Match` 只作用于 GET/HEAD/Copy）。更危险的是，
AWS SDK 的 `PutObjectInput.IfMatch` 字段发给 COS 会被**静默忽略**而非报错——
代码与指标都以为有保护，实际退化为无条件写，比现状更难排查。COS 仅支持
`x-cos-forbid-overwrite: true`（≈ `If-None-Match: *`，冲突返回 **409 `ObjectAlreadyExists`**，
非标准 412），**且存储桶开启版本控制时该头失效**。因此本方案**只使用
`create-if-absent` 这一个条件原语**——它是所有端点的最小公共能力。

### 4.1 决策表（v2 修订）

| # | 决策 | 理由 |
|---|---|---|
| 1 | 单写约束从"文档约定"升级为"代码强制" | `docs/ops/deployment.md:11` 的 `replicas=1` 目前零强制；`registry/handle.go:1-5` 的注释也只是注释 |
| 2 | **第一优先级是关闭远端 data inlining**（P0-0），而非加锁 | 它是 §3.1/§3.8「不可恢复」的唯一成因；一行配置即可把故障从"丢数据"降为"丢指针"。见 §4.2 |
| 3 | 强制手段用**对象存储 + `create-if-absent`**，不引入新中间件，**不依赖 `If-Match`** | 不为这一件事引入 etcd/Redis/ZK；`create-if-absent` 是所有端点的最小公共能力（COS 用私有头） |
| 4 | catalog 远端表示改为**不可变快照 + manifest 指针**（P0-2） | 把 Parquet 的 UUID 命名策略搬到 catalog：两个 writer 即使抢到同一 snapshot_id 也写到不同 key，**结构上无法覆盖** → CAS 不再必需。这是 v1「ETag 守卫」的替换 |
| 5 | 租约粒度为 **per-database**（P1-1），控制面另行走 §6.4 | v1 选实例级的理由（"§3.4 是实例级问题"）在控制面独立后不再成立。实例级租约 + 失租退出的终点是永久单写；per-DB 租约打开"分片单写"的水平扩展路径（N 实例并发可写，每库仍单写，KV 论证在库粒度继续成立）。LB 按 `databaseID` 做一致性哈希路由是有界工程项 |
| 6 | per-DB 失租 → **释放该库句柄**（不退出进程）；实例级探针失败才拒绝启动 | 粒度从进程降到库：可用性从"全挂"变成"部分库短暂不可写"。v1"降级只读会撞 §3.7"的顾虑只对实例级成立；库级失租后该库直接 not-ready，不给陈旧读 |
| 7 | 修 §3.8（本地领先时不下载覆盖）（P0-1） | 单实例正确性问题，顺路修 |
| 8 | `instance.id` 变更做启动检测并拒绝 | §2 形态 A 的隐藏坑，改 id = 丢全部数据 |
| 9 | 维护任务额外校验持租 + **孤儿账本**（P1-2） | §3.2 是不可恢复损坏，多一道门禁；账本保证"永不删掉账本里没有的文件"，同时解决失租实例留下的孤儿无人回收的漏洞（v1 遗漏：失租退出 → 崩溃留下的孤儿 parquet，因清理任务需持租而**永远不会被回收**） |

### 4.2 改动 P0-0：远端模式关闭 data inlining（第一优先级）

```yaml
database:
  ducklake:
    # 远端（S3）模式下必须为 0：内联会把用户数据写进 catalog，
    # 使 catalog 覆盖从"丢指针"升级为"丢数据"（见 §3.1 放大因素 1）。
    data_inlining_row_limit: 0        # 本地/DevMode 可保留 100
```

- 触点：`internal/database/ducklake/options.go:44`、`factory.go:138`/`:180`、`config.yaml:32`。
- 若为写延迟必须保留内联，则退而求其次：在 `syncOnce` 的 `COPY FROM DATABASE`
  （`catalog_syncer.go:289`）**之前**插入 `CALL {alias}.flush_inlined_data()`，
  并断言 flush 后 catalog 中无 inlined 行 → 保证 PUT 出去的 catalog 永不含用户数据。
  二者必须择一，不允许保持现状。
- 配套：开启 `merge_adjacent_files` 定期压缩抵消小文件放大；`target_file_size` 保持现值。
- **DoD**：远端模式下写入 1 行后 `data/` 下出现新 parquet；
  删除 S3 上的 catalog 后能用 `ducklake_add_data_files`（`ducklake.md:3733-3764`）重建表（§9 恢复演练）。

### 4.3 改动 P0-1：`EnsureLocalCatalog` 不再无条件覆盖（修 §3.8）

v1 方案要求"直读本地 sqlite 取 `max(snapshot_id)`"，引入额外的 sqlite 驱动依赖与文件损坏处理。简化：

- 本地在 `{cache_dir}/{db-id}/catalog/` 旁维护 `local-state.json`：
  `{snapshot_id, synced_snapshot_id, etag, updated_at}`——由 `syncOnce` 成功后写入、
  `AfterWrite`（`factory.go:268-279`）更新 `snapshot_id`。
- `EnsureLocalCatalog` 比较 `local-state.json.snapshot_id` 与远端 manifest（P0-2）的 `snapshot_id`：

```text
local > remote  → 不下载；Warn 日志 + simplebase_catalog_local_ahead_total 递增；
                  立即触发一次 Sync 把本地推上去（若随后 PutIfAbsent 冲突 → split-brain，按 §4.4 处理）
local <= remote → 下载到独立 staging 路径，成功后 rename 覆盖
本地无 state 文件（新实例/清过缓存）→ 正常下载
state 文件损坏/缺失但 catalog.sqlite 存在 → 保守视为 0，允许下载（与 v1 语义一致）
```

- 不再需要 `github.com/uglyer/go-sqlite3` 直读副本（v1 §4.5 的依赖被消除）。
- **DoD**：模拟 Sync 失败 → 本地领先 → 重新 `Open` → 本地 catalog 未被覆盖，
  随后一次 Sync 成功推上去；`catalog_local_ahead_total` 递增。

### 4.4 改动 P0-2：catalog 远端表示 = 不可变快照 + manifest 指针（替换 v1 的 ETag 守卫）

核心思想（§1.4 推论 3）：**把 Parquet 的 UUID 命名策略搬到 catalog 上，让"覆盖"在结构上不存在。**

```text
{root}/{env}/db/{tenant}/{dbid}/catalog/
  snapshots/{snapshot_id:020d}-{writer_epoch}.sqlite   ← 不可变；PutIfAbsent 写入
  manifest/{seq:020d}.json                             ← 指针序列；PutIfAbsent 写入；seq 单调递增
  # 旧 key catalog.sqlite 与 versions/{snap}.sqlite 保留为兼容期镜像，只读，
  # 不再作为事实来源；一个版本后删除。
```

- **快照对象 key 含 `writer_epoch`**（P1-1 的租约 epoch；无租约时用实例启动 UUID）。
  两个 writer 即使抢到同一个 `snapshot_id`，也写到**不同的 key**，永不互相覆盖。
  被抢占的旧 writer 写出的对象一眼可辨（epoch 过期），事后审计与清理有据。
- `manifest.json` 极小（<1KB，含 `{seq, snapshot_id, snapshot_key, writer_epoch, updated_at}`），
  但它的更新同样退化为不可变序列：writer 推进 `seq` 用 `PutIfAbsent`，
  **409/412 即意味着第二个 writer 存在** → 这就是 split-brain 检测，
  只需 `create-if-absent`，无需 CAS。
- reader 冷启动：LIST `manifest/` 取最大 seq；或从缓存 seq 向前探测 `seq+1, seq+2…` 直到 404（O(1) 请求）。
- `pruneVersions` 重写：按 manifest 记录的存活快照列表删除，
  **不再靠 `current - keep` 算 key**（修 §3.1 放大因素 2 里"删掉最后一个好版本"的缺陷）。
- **DoD**：两进程同时向同一库 PUT `manifest/{seq}.json`，有且只有一个成功；
  失败方得到 `ErrPreconditionFailed` 并标 split-brain。

### 4.5 改动 P0-3：`BlobStore` 增加 `PutIfAbsent`（唯一的新原语）

```go
// internal/objectstore/blob.go
// PutIfAbsent 仅当 key 不存在时写入；已存在返回 ErrPreconditionFailed。
// 端点适配：S3/GCS/R2 → If-None-Match: *；
//           腾讯云 COS → x-cos-forbid-overwrite: true（私有头，冲突返回 409 ObjectAlreadyExists）。
// 注意：COS 在存储桶开启对象版本控制时该头失效，启动探针必须覆盖这一点。
PutIfAbsent(ctx context.Context, key string, data []byte, contentType string) (ObjectInfo, error)
```

- `PutBytes` 原签名与无条件语义**完全不变**（现有调用方零改动）——比 v1 的
  `PutBytesCond(BlobPutOptions{IfMatch, IfNoneMatch})` 收敛：不给任何调用方
  误用 `IfMatch` 的机会（发到 COS 会被静默忽略）。
- 错误映射：HTTP 412、HTTP 409（COS `ObjectAlreadyExists`）→ `ErrPreconditionFailed`。
- `memoryBlobStore`（`blob.go:114-183`）同步实现，ETag 用内容 SHA256 前 16 hex。
- **启动探针（必做，替代 v1 的"前置验证一票否决"）**：启动时对
  `{root}/{env}/catalog/.probe-{uuid}` 连续两次 `PutIfAbsent`，第二次必须失败。
  探针失败 → 记 Fatal 日志 + `simplebase_objectstore_cas_supported=0`，
  **禁用一切依赖互斥的能力**（租约、manifest 推进、维护任务），
  `/health/ready` 暴露原因，而不是让代码以为自己有保护。
  探针通过 → `simplebase_objectstore_cas_supported=1`。
- **版本控制冲突**：备份若依赖对象版本控制，会令 COS 的 `x-cos-forbid-overwrite` 失效。
  两者必须二选一：本方案下备份由 `snapshots/` 不可变序列天然提供（保留 `KeepVersions` 份），
  **存储桶不开版本控制**。写进部署文档。

### 4.6 改动 P1-1：per-database 写租约（epoch 观察式，只用 `PutIfAbsent`）

```text
key: {root}/{env}/db/{tenant}/{dbid}/catalog/lease/{epoch:020d}.json
payload: { owner_id, instance_id, hostname, pid, acquired_at, renewed_at, ttl_ms, epoch }
```

租约算法——**只做同时钟差 + 状态变化观察，不做跨时钟减法**：

```text
Acquire(dbID):
  1. 从已知 epoch 向前探测（lease/{E+1}.json → 404 停止），找到当前最大 epoch E
  2. 若无任何 epoch → PutIfAbsent(lease/0001.json, payload{epoch:1}) 成功即持租
  3. 若 E 存在：
     a. owner == 自己（同 instance.id + 同 pid）→ 续租路径：PutIfAbsent(lease/{E+1}.json)
     b. owner != 自己 → 观察式判活（本地单调时钟 time.Since）：
          obs0 = 当前最大 epoch
          sleep(ttl + grace)                    ← 单调时钟，不做 now − 服务端时刻 的跨时钟减法
          obs1 = 当前最大 epoch
          obs1 == obs0 → 持租者已停止续约（或已死）→ PutIfAbsent(lease/{E+1}.json) 抢占
          obs1 >  obs0 → 活着 → ErrLeaseHeld（错误含 owner 的 instance_id/hostname/pid）
  4. 持租后启动续约 goroutine：每 ttl/3 执行 PutIfAbsent(lease/{epoch+1}.json)
     - 409/412 → 被抢占 → onLost(dbID)

onLost(dbID):
  1. CatalogSyncer 对该库跳过 Flush（CloseNoFlush 语义，复用 v1 设计）
  2. Registry 移除该库 entry → 该库转 not-ready（拒绝新 Acquire，进行中请求自然结束）
  3. simplebase_instance_lease_lost_total{database_id} 递增 + Error 日志
  4. 不退出进程；编排系统看到库级 not-ready 后按路由表把该库流量导向持租实例
```

设计说明：

- **v1 的"用 S3 `LastModified` 避免时钟漂移"不成立**：判定式 `now - renewed_at >= ttl`
  里的 `now` 仍是本地时钟，换成服务端 `LastModified` 后仍是跨时钟减法，
  且 `LastModified` 只有秒级精度。观察式判活只用"同一本地单调时钟测出的两个时刻差"
  加"服务端 epoch 是否变化"这一个布尔信号，才真正不依赖时钟同步。
- **续约本身就是 fencing**：epoch 单调递增，P0-2 的快照对象 key 含 `writer_epoch`，
  被抢占的旧 writer 写出的对象不覆盖任何东西、且一眼可辨。
- 代价：续约 = 每 ttl/3 一个小对象。用 lifecycle 规则自动过期，或每 N 次续约后清理旧 epoch
  （删除失败无害，lifecycle 兜底）。
- 另保留**一把实例级租约**，仅用于"维护任务 leader 选举"（粒度从"写"降为"维护"），
  不再用于写互斥。
- 路由亲和：LB/网关按 `databaseID` 一致性哈希把写路由到持租实例。未持租实例收到写请求时
  返回 409 + 当前 owner 信息（可自动触发接管流程）。这是有界工程项，
  远小于 v1 方案 3 的"整体换 PG + 重写 kv 包"。

### 4.7 改动 P1-2：维护任务门禁 + 孤儿账本（修 §3.2 全链路）

门禁（复用 v1 §4.7）：

```text
runMaintenance(dbID):
  if !lease.ValidFor(dbID) { skip + Warn + simplebase_maintenance_skipped_total{reason="no_lease"} }
  if syncer.SyncLag(dbID) > 0 { skip }        // 有未同步事务时不做破坏性操作
  ...CHECKPOINT...
```

**新增孤儿账本**（补 v1 遗漏：失租/崩溃实例留下的孤儿 parquet，因清理任务需持租而永远无人回收）：

- 每次写 parquet 前把 `{file, db, epoch, ts}` 追加写入
  `{root}/{env}/orphans/{date}/{uuid}.json`（`PutIfAbsent`，小对象，一次性写入）。
- 事务提交成功后**不删账本**，交给清理任务比对。
- 清理任务 = 账本 ∪ LIST 结果，与 catalog 引用做差集，**只删"账本里存在 &&
  `older_than` 超期"的文件** → 永远不会删掉账本里没有的文件（消除 §3.2 误删的机制本身），
  同时失租实例留下的孤儿也能被安全回收。
- `delete_older_than` 从 `1d` 调到 `7d`（`config.yaml:43`）；默认先 `dry_run` 记录再执行。

### 4.8 改动 P0-4：配置与启动校验（复用 v1 §4.6）

```yaml
instance:
  id: "simplebase-prod-1"
  writable: true
  # 新增：写租约（仅 writable=true 且配了 S3 时生效）
  lease:
    enabled: true          # 默认 true；DevMode / 无 S3 时自动跳过
    ttl: 30s
    renew_interval: 10s    # 建议 ttl/3
    grace: 10s             # 观察式判活的等待窗口
    on_lost: release_db    # release_db（默认，释放该库句柄）| exit（保留，整实例退出）
```

对应 `internal/config/config.go`：`InstanceConfig`（`config.go:44-47`）增加
`Lease InstanceLeaseConfig`，`yaml.go` 补映射（参考 `yamlCatalogSync`，`yaml.go:91`）。

**`instance.id` 变更检测**（§2 形态 A 的坑）：
在 `{cache_dir}/system/locator.json` 旁边写 `instance-identity.json`
（`{instance_id, s3_prefix, first_seen_at}`）。启动时若本地已有该文件且
`instance_id` 或 `s3_prefix` 与当前配置不一致 → **拒绝启动**，
错误信息明确指出「改变 instance.id 或 s3.prefix 会导致既有数据不可见；
如确需迁移请走 §10 的迁移流程，或清空 cache_dir 确认这是全新部署」。

### 4.9 新增指标与告警

| 指标 | 类型 | 含义 | 告警 |
|---|---|---|---|
| `simplebase_instance_lease_state{database_id}` | Gauge | 1=持租 / 0=未持租 | ==0 且该库应有写流量 → P1 |
| `simplebase_instance_lease_renew_failures_total{database_id}` | Counter | 续约失败 | 连续 >2 → P1 |
| `simplebase_instance_lease_lost_total{database_id}` | Counter | 失租（释放句柄） | >0 → P0 |
| `simplebase_instance_lease_epoch{database_id}` | Gauge | 当前 epoch | 非预期跳变 → P1（发生过抢占） |
| `simplebase_catalog_sync_conflicts_total{database_id}` | CounterVec | `PutIfAbsent` 冲突（manifest/snapshot） | **>0 → P0（split-brain 确证）** |
| `simplebase_catalog_local_ahead_total` | Counter | 本地领先远端（§3.8 路径） | >0 → P1 |
| `simplebase_catalog_inlined_rows{database_id}` | Gauge | catalog 内联行数 | P0-0 生效后应恒为 0，非 0 → P0 |
| `simplebase_catalog_size_bytes{database_id}` | Gauge | catalog 文件体积 | 超阈值（32MB）→ P1（§6.3 前置信号） |
| `simplebase_objectstore_cas_supported` | Gauge | 启动探针结果 | ==0 → P0（互斥能力已禁用） |
| `simplebase_maintenance_skipped_total{reason}` | CounterVec | 维护任务跳过 | 持续 → P2 |

现有 `CatalogSyncTotal` / `CatalogSyncFailures` / `CatalogSyncLag`
（`observability/metrics.go:101-117`）保持不变，新指标同文件追加。

### 4.10 收口阶段一的能力边界（必须写进文档）

不提供：

- 同一个库的多实例并发写（那是 §6 的事；per-DB 租约给的是"N 实例 × 各自的库"）。
- 写高可用（库的持租实例挂了，该库写不可用直到接管完成，RTO = 观察 ttl+grace + 接管 + 冷启动）。
- 读扩展（只读实例仍有 §3.7 陈旧读，需 §5）。

提供：

- 误起第二个实例写同一库时，第二个实例对该库**拿不到租约**（而不是静默损坏）。
- 租约异常失效时，旧 writer 的 catalog 写入因 `writer_epoch` key 与 `PutIfAbsent`
  **结构上不覆盖任何东西**，且冲突计数器确证 split-brain 并告警。
- 单实例的 §3.8 数据丢失路径被修复；`instance.id` 误改被拦下。
- **§3.1/§3.8 从"不可恢复"变为"可恢复"**：数据全在 parquet，恢复流程见 §9。

---

## 5. 收口阶段二（P2，可选）：1 写 N 读（采用官方 Frozen DuckLake 模式）

在阶段一之上做**读扩展**。不解决写高可用。仅当读负载确实成为瓶颈时才做。

### 5.1 形态

```text
        ┌─────────────────────────────────────────┐
        │  LB：写请求 → writer；读请求 → reader 池   │
        └──────────┬──────────────────┬───────────┘
                   ▼                  ▼
        ┌──────────────────┐  ┌──────────────────┐
        │ writer（持租）    │  │ reader × N       │
        │ writable: true   │  │ writable: false  │
        │ 维护任务在此      │  │ 无维护任务        │
        └────────┬─────────┘  └────────┬─────────┘
                 │ PUT snapshot+manifest│ HEAD manifest/{seq+1}（只读）
                 ▼                      ▼
        S3：catalog/snapshots/*.sqlite + manifest/*.json + data/*.parquet
```

关键：**reader 不写 catalog、不跑维护任务、不取写租约**（只读无需互斥）。
`instance.id` 必须与 writer **相同**（共享 S3 前缀，§2），
这也意味着 reader 不能用 `instance.id` 做身份区分 —— 需要新增
`instance.role: writer | reader` 配置，与 `writable` 解耦（`writable=false` 目前
在 `app.go:301` 直接 `return nil` 跳过整个依赖装配，reader 需要一条新的装配路径）。

### 5.2 catalog 刷新器（§3.7 的修复）

v2 相比 v1 的简化：刷新探测直接复用 §4.4 的 manifest 序列，不再需要按库逐个 HEAD catalog 对象。

```
internal/database/ducklake/catalog_refresher.go   （新增）

// 每 Interval 对每个已打开的库：
//   1. HEAD manifest/{cached_seq + 1}.json → 404 则无新快照（绝大多数 tick 走这条，成本 = 一次 HEAD）
//   2. 存在 → GET manifest（拿到 snapshot_key）→ 下载快照到 staging → 执行 re-attach（见下）
```

**re-attach 的难点**：DuckLake catalog 是在 `Factory.Open` 的 connector init 回调里
`ATTACH 'ducklake:sqlite:...'`（`factory.go:65-72`、`:146-151`），
且随后执行了 `SET lock_configuration = true`（`factory.go:160`）**锁死配置**。
无法在运行中 DETACH/ATTACH。三条路径：

| 路径 | 做法 | 代价 | 评价 |
|---|---|---|---|
| **R1 换 handle** | 新建一个 DuckDB 实例（ATTACH 新 catalog 文件）→ 原子替换 `registry.entry.handle` → 旧 handle 引用清零后关闭 | 每次刷新一次冷启动（ATTACH + 扩展已 LOAD，实测应在 10ms 量级）；需给 Registry 加 `Swap(dbID, newHandle)` | **推荐**。语义最干净，与现有 `entry` 结构契合 |
| R2 运行中 re-attach | 去掉 `lock_configuration`，DETACH + ATTACH | 破坏安全边界（`lock_configuration` 是防用户 SQL 改配置的关键）；且 DETACH 时若有活跃查询会失败 | 否决 |
| R3 SNAPSHOT_VERSION | 每次查询按 `AT (VERSION => n)` 读 | 不解决 catalog 文件本身陈旧（新 snapshot 根本不在本地 catalog 里） | 作为**刷新机制**否决；但保留为可选能力：`SNAPSHOT_VERSION`（`ducklake.md:1807`、`:2182`）提供读一致性锚点（同一会话内单调读、钉版调试），与 R1 互补而非竞争 |

R1 的实现要点：

- `Registry` 新增 `Swap(ctx, dbID string, open func() (*sql.DB, error)) error`：
  打开新连接 → 加锁替换 `e.handle` → 旧 handle 标 `stateClosing`，
  等 `active==0` 后 `closeLocked()`（复用 `Shutdown` 的等待逻辑，`registry.go:304-315`）。
- 进行中的查询继续用旧 handle（旧 catalog 视图），**不中断请求**；
  新请求拿到新 handle。这是可接受的读一致性（单调读在单个请求内成立）。
- reader 的 `Factory` 以只读方式 ATTACH：
  `ATTACH 'ducklake:sqlite:...' AS lake (..., READ_ONLY)`。
  **官方文档已确认支持**：`ducklake.md:1778`（`ATTACH 'ducklake:postgres:...' (READ_ONLY)`）、
  `:4082`（「Both `READ_ONLY` and regular attaching modes will work」）。
  ~~v1 的"需前置验证 + 退化分支"删除~~。
  仍保留的防线：**禁止 reader 装配 CatalogSyncer**（`Syncer = nil` 或 `NewLocalSyncer()`），
  确保它物理上不会 PUT——`READ_ONLY` 是数据面约束，物理不装配才是纵深防御。

### 5.3 reader 的其他约束

| 项 | 要求 | 原因 |
|---|---|---|
| 系统库 | reader **必须连 writer 的系统库**，不能自己 Bootstrap | 否则 §3.4 分裂。需要把 `locator.json` 移到 S3（见 §5.4；§6.4 控制面独立后此问题整体消失） |
| `sys_logs` / `sys_metrics` 写入 | reader 需要写这两张表（`StartPeriodicFlush`，`store.go:175`） | **这是写操作** → 与"reader 不写"矛盾。方案：reader 的 metrics/logs 经 HTTP 上报给 writer，或直接禁用 reader 的 systemdb 写入（丢失 reader 侧的日志/指标，可接受，Prometheus 仍能抓 `/metrics`）。§6.4 后自然消失 |
| cron / agent 调度器 | reader 一律不启动 | §3.5 |
| `kv.Sweeper` | reader 不启动 | 写操作 |
| 缓存管理 | reader 独立 `cache_dir`，独立 LRU | 各自本地盘 |
| 数据新鲜度 | 文档承诺"最终一致，滞后 ≤ `refresh_interval` + writer 的 `debounce`" | 默认 5s + 200ms ≈ 5.2s |

### 5.4 前置改动：系统库身份移到 S3

reader 要连 writer 的系统库，`locator.json` 就不能在本地盘（§3.4）。改为：

```go
// internal/objectstore/keys.go
// SystemLocatorKey 返回系统库身份对象键（{root}/{env} 级唯一）。
func (k KeyBuilder) SystemLocatorKey() string {
    return joinKey(k.base(), "catalog", "system-locator.json")
}
```

`systemdb.Bootstrap`（`bootstrap.go:41-56`）改为：

```text
1. 先读 S3 SystemLocatorKey
   - 命中 → 用它（并写一份到本地作为缓存/离线兜底）
   - ErrNotFound → 仅当持有该系统库的写租约（§4.6，系统库本身也是一个 dbID）时才
     uuid.NewString() 创建，并用 PutIfAbsent 写入；冲突 → 说明被抢先，回到 1 重读
2. 本地 locator.json 降级为只读缓存；与 S3 不一致时以 S3 为准并告警
3. DevMode / 无 S3 → 保持现有本地行为
4. 兼容升级路径保留：「本地有、S3 无 → 把本地 locator 提升写入 S3（PutIfAbsent）」一次性迁移
```

> §6.4（控制面独立存储）落地后，本节整体作废——控制面不再走 DuckLake/S3 locator。
> 在此之前它仍是 §3.4 的止血方案。

### 5.5 阶段二的边界

- 不提供写高可用：writer 挂了，写不可用，直到接管（per-DB 租约让接管从"整实例"变为"受影响的库"）。
- 不提供强一致读：reader 有 ≈5s 滞后。需要强一致的读请求必须路由到 writer
  （LB 上需要一条"强一致读"通道，或 SDK 提供 `consistent: true` 参数走 writer）。
- reader 的 HEAD 成本：库数多时按库 HEAD 仍为 O(N)。
  优化：把"哪些库变了"聚合到实例级单对象（writer 维护 `catalog/dirty-index.json`，
  本身也是 `PutIfAbsent` 序列）→ O(1)。**库数 >100 时必需，不是可选。**

---

## 6. 收口阶段三（P3/P4，长期）：控制面独立 + 共享 PostgreSQL catalog

v1 把方案 3 定义为"整个仓库换 PG catalog"。v2 修正：**先把控制面拆出来（P3），
数据面是否上 PG（P4）退化为按库灰度的可选决策**——因为 per-DB 租约（§4.6）已经提供
"N 实例 × 分片单写"的水平扩展，只有"同一个库需要多实例并发写"才真正需要共享 catalog。

### 6.1 为什么共享 PG 能解决问题

把 catalog 从"本地 SQLite + 文件同步"换成"共享 PG"后，
§1.1 的 DuckLake 原生机制**立即生效**：

```sql
ATTACH 'ducklake:postgres:dbname=simplebase_catalog host=... ' AS lake
       (DATA_PATH 's3://bucket/prefix/data/');
```

| 故障 | 阶段一（P0/P1） | 共享 PG（P4） |
|---|---|---|
| §3.1 catalog 覆盖 | 结构上不可覆盖 + 可恢复 | **不存在**（同一个 catalog，PK 冲突 + 自动重试） |
| §3.2 孤儿误删 | 账本门禁 | **不存在**（全局视图） |
| §3.3 ID 重复分配 | `PutIfAbsent` 拦下 | **不存在**（PG 序列/事务保证） |
| §3.4 系统库分裂 | §5.4 S3 locator | **不存在**（P3 后控制面直接在 PG） |
| §3.5 CAS 失效 | 按"CAS 不可靠"设计 | **仍不可靠**（`UPDATE`-vs-`UPDATE` 不判冲突，§3.5；P3 把认领移出 DuckLake 才根治） |
| §3.6 KV 语义 | per-DB 租约保持分片单写 | 需改造（乐观版本号重试）或保留路由亲和 |
| §3.7 陈旧读 | manifest 刷新 ≈5s | **不存在**（每次查询读当前 snapshot） |
| §3.8 回滚覆盖 | P0-1 修复 | **不存在**（无本地 catalog 文件） |
| 写高可用 | 无（库级接管） | **有**（任一实例都能写同一库） |
| 写扩展 | 有（分片单写，线性于实例数） | 有（同一库多写，受 PG 写吞吐限制） |

**`CatalogSyncer` 整个删掉**（`catalog_syncer.go` 412 行 + `syncer.go` + 相关测试），
`EnsureLocalCatalog` 删掉，冷启动变成"连 PG 即可"（无本地状态，容器真正无状态）。

### 6.2 代价与必须解决的问题

| # | 问题 | 说明 | 处理 |
|---|---|---|---|
| 1 | 引入 PG 运维依赖 | 与"Serverless / S3 为唯一事实"的产品叙事冲突（`db-ducklake-plan.md` §一） | 产品决策题。可用云托管 PG（RDS/Supabase/Neon），SLA 由云厂商保证 |
| 2 | `internal/systemdb` 迁移 | 21 个文件、`migrate.go` 17KB 的 `sys_*` DDL | **P3 先行拆分后此项大幅缩小**：控制面已迁走，剩下的用户库 catalog 迁移不涉及 `sys_*` |
| 3 | §3.5 CAS 是否成立 | ~~必须实测~~ → **按 §3.5 的推论直接判 CAS 失效**（`UPDATE`-vs-`UPDATE` 不在逻辑冲突列表） | 调度认领移出 DuckLake 放进 PG 普通表（`pg_try_advisory_lock` 或普通行锁），或改版本号列 + 显式校验。**P3 一并解决，不留到 P4** |
| 4 | §3.6 KV 读-改-写 | 论证依赖单写 + 单连接 | (a) 保留 per-DB 路由亲和（分片单写，KV 论证继续成立——阶段一已具备）；(b) 改乐观并发（每次写校验 `version`，冲突重试），需重写 `internal/database/kv/` 6 个仓库。**只有确需同一库多写才走 (b)** |
| 5 | Registry 语义 | `registry` 的"进程内单写"不再等于"全局单写" | 包注释与 `internal/AGENTS.md` 的约束描述要改；`SetMaxOpenConns(1)`（`factory.go:78`）可按库放开 |
| 6 | catalog PG 的写放大 | 每个事务 = 一个 snapshot 行 + 变更集行；高频小写场景 PG 成为瓶颈 | **P0-0 已关内联**，此问题弱化为纯元数据写放大；仍需压测。`simplebase_catalog_size_bytes` 是前置观测指标 |
| 7 | 维护任务协调 | 多实例都想跑 CHECKPOINT | 保留 §4.6 的实例级租约做"维护 leader 选举"，或用 PG advisory lock（PG 原生，更可靠） |
| 8 | 迁移路径 | 存量 SQLite catalog → PG | DuckLake 无官方 catalog 迁移工具。路径：对每个库导出全表数据（`COPY ... TO parquet`）→ 在新 PG catalog 上重建（CTAS）→ 校验行数/抽样哈希。**time travel 历史会丢失**（snapshot 链重建），需产品决策。**P0-0 关内联后，数据面本就全在 parquet，此路径风险显著低于 v1 评估** |

### 6.3 判定：什么时候该做 P4

不要为了"架构先进"做它。触发条件（满足任一）：

- **同一个库需要多实例并发写**成为硬需求（分片单写不满足业务形态，例如库数少但单库写极重）。
- **写高可用 SLA 收紧**：要求单库 RTO < 接管时间（观察 ttl+grace + 接管 + 冷启动）。
- **catalog 体积导致同步不可行**：`simplebase_catalog_size_bytes` 超阈值（§4.9）。
  —— 一旦成真，阶段一/二的全量快照模型直接破产，只能走 P4。
  注意 P0-0 关内联后 catalog 只含元数据，此触发条件的到达时间被大幅推迟。

### 6.4 P3（新增，先于 P4）：控制面独立存储

**核心洞察**：控制面（`sys_*`）与数据面的需求正交，而控制面放在 DuckLake 里本身就是错误选择——
与多实例无关：

| | 控制面 `sys_*` | 数据面（用户库 / KV） |
|---|---|---|
| 数据量 | 小（MB 级） | 大（GB~TB） |
| 写模式 | 高频小写、**需要 CAS / 唯一约束 / 行锁** | 分析型批写 + 小写 |
| DuckLake 能否满足 | **否**。无 PK/UNIQUE/CHECK（`ducklake.md:1627`）、无 `FOR UPDATE`、`UPDATE`-vs-`UPDATE` 不判冲突（§3.5） | 是 |
| 一旦分裂 | 整个产品不可用（§3.4/§3.5） | 单库受损 |

**控制面迁出 DuckLake，改用真 OLTP 存储**：单实例 → 本地 SQLite/嵌入式；多实例 → 共享 PG。

收益：

- §3.4（系统库分裂）、§3.5（CAS 失效/重复调度）**直接消失**，不需要 §5.4 的 S3 locator；
- `ClaimCronJob` 的 CAS 恢复为真 CAS（PG 行锁）；`sys_api_keys` / `sys_users` 拿回 UNIQUE 约束；
- reader 与 writer 天然共享控制面（§5.3 表格里"系统库"与"sys_logs/metrics"两行约束整体消失）；
- 数据面的多实例演进（per-DB 租约 → P4）可以独立推进，不必等"整体上 PG"这个大决策；
- 迁移成本：`sys_*` 数据量小（MB 级），一次性导出导入即可，**不涉及 time travel 历史丢失**
  （v1 §6.2 第 8 项的最大痛点在控制面并不存在）。

部署矩阵：

| 形态 | 控制面 | 数据面 |
|---|---|---|
| 单实例 | 嵌入式 SQLite | 本地 DuckLake（现状） |
| 单写多读 | 共享 PG（或 SQLite on 共享卷，不推荐） | 阶段二 |
| 分片多写 | 共享 PG | 阶段一 per-DB 租约 |
| 真多写 | 共享 PG | P4 共享 PG catalog（按库灰度） |

---

## 7. 阶段划分（v2 修订：P0/P1 同 PR，顺序与 v1 相反）

**关键顺序变化**：v1 的第一步是"加租约"，v2 的第一步是
"**关闭远端内联 + catalog 远端表示不可覆盖 + 本地领先不覆盖**"。
前者需要 COS 不具备的能力（`If-Match`）且终点是单写；
后者零依赖、单 PR 可完成，且直接改变故障的可恢复性。

### Phase A（P0）— 内联、不可变快照与守卫（必做，零依赖条件写之外的能力）

1. `PutIfAbsent` + `ErrPreconditionFailed`（§4.5），`memoryBlobStore` 同步实现；
   **启动探针**（两次 `PutIfAbsent` 第二次必须失败），失败则禁用互斥能力并暴露 `/health`。
2. **P0-0：远端模式 `data_inlining_row_limit: 0`**（或 Sync 前强制 `flush_inlined_data`）+ 压缩策略（§4.2）。
3. P0-2：manifest + 不可变 `snapshots/{snap}-{epoch}.sqlite`（§4.4）；`pruneVersions` 重写。
4. P0-1：`local-state.json` 水位比较 + staging 下载（§4.3）。
5. `syncOnce` 顺序修正：先取 snapshot 再 COPY，COPY 后复核未变（§3.1 实现缺陷）。
6. `instance-identity.json` 变更检测（§4.8）。
7. 新指标（§4.9）。
8. **DoD**：见 §8「Phase A」。

### Phase B（P1）— per-DB 租约与维护门禁（必做，与 A 同 PR 或紧随）

1. `internal/database/lease` 包：观察式 epoch 判活（§4.6），只用 `PutIfAbsent`。
2. 失租 → 释放该库句柄（`Registry` 移除 entry + `CloseNoFlush` + not-ready）。
3. 写路由亲和：LB/网关按 `databaseID` 哈希；未持租返回 409 + owner 信息。
4. 维护任务持租校验 + **孤儿账本**（§4.7）；`delete_older_than` 调 7d。
5. `instance.lease.*` 配置 + `yaml.go` 映射（§4.8）。
6. **DoD**：见 §8「Phase B」。

### Phase C — 文档与运维（与 A+B 同 PR 或紧随）

见 §10。核心是把"`replicas=1` 是约定"改成"写同一库需持租，违反会明确失败"；
新增 §9 的 split-brain 恢复 Runbook。

### Phase D（P2）— 只读副本（可选，独立排期）

§5 全部内容。触发条件：读负载成为瓶颈。
前置：Phase A+B 完成 + `dirty-index.json` 聚合优化（库数 >100 时必需）。
采用官方 Frozen DuckLake 模式；`READ_ONLY` ATTACH 官方已确认支持，无前置验证项。

### Phase E（P3）— 控制面独立存储（推荐尽早，独立排期）

§6.4。触发条件：§3.5 造成业务损失、或需要多实例共享控制面（阶段二的前置）。

### Phase F（P4）— 数据面共享 PG catalog（可选，按库灰度）

§6.1-§6.3。触发条件见 §6.3。需要独立的计划文档。

---

## 8. 验收（DoD）

### Phase A（P0）

- [ ] **启动探针**有测试证据：目标 endpoint（腾讯云 COS）对同一 key 连续两次
      `PutIfAbsent` 第二次返回 409/412；探针失败路径（fake 不支持的 store）下
      互斥能力被禁用且 `/health/ready` 暴露原因。
- [ ] `PutIfAbsent` 单测：create-if-absent 成功/冲突两种路径；
      `memoryBlobStore` 与 `s3Client` 行为一致；409 与 412 都映射到 `ErrPreconditionFailed`。
- [ ] **P0-0 生效**：远端模式下写入 1 行后 `data/` 出现新 parquet；
      `simplebase_catalog_inlined_rows` 恒为 0；
      **删除 S3 上的全部 catalog 对象后，用 `ducklake_add_data_files` 能把数据重建出来（§9 恢复演练）。**
- [ ] manifest 单测：两个 writer 并发 PUT `manifest/{seq}.json`，仅一个成功，
      失败方标 split-brain、`catalog_sync_conflicts_total` 递增、库标 `degraded`。
- [ ] 快照对象不可变：同 `snapshot_id` 不同 `writer_epoch` 写入不互相覆盖；旧 epoch 对象保留可审计。
- [ ] `pruneVersions` 只删 manifest 记录的存活列表之外的对象（构造"中间版本缺失"场景不断言）。
- [ ] §3.8 回归：模拟 Sync 失败 → 本地领先 → 重新 Open → **本地 catalog 未被覆盖**、
      `catalog_local_ahead_total` 递增、随后一次 Sync 推送成功。
- [ ] `syncOnce` 顺序断言：COPY 期间 snapshot 不变（fake 慢速连接注入验证）。
- [ ] 修改 `instance.id` 后启动被拒绝，错误信息指向迁移流程。
- [ ] DevMode / 无 S3 时探针与 P0 行为自动跳过，本地开发流程不受影响。

### Phase B（P1）

- [ ] 租约单测：首次获取、续约（epoch 前进）、他人持有时 `ErrLeaseHeld`（错误含 owner 信息）、
      **观察式过期抢占**（fake 时钟推进 ttl+grace 且 epoch 不变 → 抢占成功）、
      并发抢占只有一个成功。
- [ ] **集成测试：两个实例同时打开同一 writable 库，第二个对该库拿不到租约，
      写请求返回 409 且错误信息含当前 owner 的 instance_id / hostname / pid。**
- [ ] 失租注入（手工删除/篡改 lease 对象）→ 该库在 ≤ ttl+grace 内转 not-ready、
      `lease_lost_total{database_id}` 递增、**进程不退出**、其他库不受影响。
- [ ] 失租后 `Close` 不发起任何 PUT（fake BlobStore 断言零调用）。
- [ ] 写路由亲和：未持租实例的写请求返回 409 + owner 信息；接管后流量恢复。
- [ ] 孤儿账本：崩溃注入（写 parquet 后、事务提交前杀进程）→ 账本记录该文件；
      清理任务只删"账本内 && 超期"的文件，**账本外的文件永不被删**。
- [ ] 维护任务门禁：无租约 / SyncLag>0 时 CHECKPOINT 被跳过并计数。
- [ ] `writable: false` 实例不取任何写租约。
- [ ] 现有 `go test ./internal/...` 全绿；`ducklake` / `objectstore` / `systemdb`
      三个包的覆盖率不下降。

### Phase D（P2，若做）

- [ ] reader 不写任何 S3 对象（fake BlobStore 断言零 PUT/Delete）。
- [ ] writer 提交后，reader 在 ≤ `refresh_interval + debounce` 内能读到新数据。
- [ ] reader 的刷新探测是 O(1)（manifest seq 前探），N 库场景下通过 `dirty-index.json` 保持 O(1)。
- [ ] reader 的 catalog 刷新不中断进行中的查询（旧 handle 存活至引用清零）。
- [ ] reader 不启动 cron / agent / kv.Sweeper 调度器；不装配 CatalogSyncer。
- [ ] reader 与 writer 共享同一系统库 `database_id`（Phase E 后改为共享同一控制面存储）。
- [ ] `READ_ONLY` ATTACH 下 reader 的写请求在 DuckDB 层即失败（纵深防御验证）。

### Phase E（P3，若做）

- [ ] `sys_*` 迁出后：`ClaimCronJob` 并发认领只有一个成功（PG 行锁下的真 CAS）。
- [ ] 两实例共享控制面：A 创建的项目/API Key/会话在 B 立即可见。
- [ ] 控制面数据迁移演练：SQLite 导出 → PG 导入 → 行数/抽样哈希校验。

---

## 9. Split-brain 恢复流程（v1 缺失，运维必需）

`simplebase_catalog_sync_conflicts_total > 0`（split-brain 确证）后的处置，写进 Runbook：

```text
1. 定位两个 writer：读 lease/{epoch}.json 与冲突时的 manifest/{seq}.json，
   payload 含 instance_id / hostname / pid / epoch。
2. 停掉其中一个（优先停 epoch 较小、或 snapshot 水位较低的那个）。
3. 判定损伤范围：
   a. P0-0 已生效（内联关闭）→ 数据全在 parquet，只是指针分叉：
      - 对每个库，选 snapshot 水位较高的一侧 catalog 作为基线；
      - LIST data/ 目录，与基线 catalog 的 ducklake_data_file 做差集；
      - 差集里的 parquet 用 CALL ducklake_add_data_files(...) 重新登记
        （ducklake.md:3733-3764）；
      - 校验行数与抽样哈希；记录 rowid 血缘已断（Change Feed 从该点起重建）。
   b. 内联仍开启（未做 P0-0 的存量部署）→ 两侧 catalog 各含独家数据行，无法自动合并：
      - 分别把两侧 catalog 里的用户表 COPY ... TO parquet 导出；
      - 在新库上 CTAS 重建 + 业务侧去重（需业务主键语义，无通用规则）；
      - time travel 历史丢失。→ 这正是 P0-0 必须排第一的理由。
4. 恢复后手工解除 degraded（不自动恢复，保留 v1 决策）。
5. 事后审计：epoch 序列 + 快照对象的 writer_epoch 完整可回放。
```

**一个只能检测不能恢复的告警，运维价值有限。** 本节是 §4.9 各 P0 告警的配套闭环。

---

## 10. 文档清单（实现 PR 一并改）

- [ ] `docs/ops/deployment.md`：
      §硬性约束表的"副本数 `replicas=1`"一行补充「**同一库的写已由 per-DB 租约强制；
      未持租实例的写请求返回 409**」；新增「写租约」小节（参数、失租行为、告警、恢复流程 §9）；
      §32 行关于 `instance.id` 的描述补充「**改变它等价于换空存储，现已由启动检测拒绝**」；
      新增「存储桶不得开启对象版本控制」（§4.5，否则 COS 条件写失效）；
      Runbook 增加「实例卡在 not-ready / 反复重启」与「`catalog_sync_conflicts_total > 0`
      怎么办（§9）」两个处置流程。
- [ ] `internal/AGENTS.md`：单写约束从"进程内"升级描述为"进程内 Registry + per-DB S3 租约"。
- [ ] `internal/database/README.md`：补 `lease` 包、manifest、孤儿账本的位置说明。
- [ ] `plan/planv2.0/db-ducklake-plan.md`：§十 风险表的
      「单写实例约束被误突破 / 低 / 沿用 v1.0 部署约束」一行改为指向本文
      （等级从"低"提升为"高"，因为无强制手段）；§十一 开放问题增加一条
      「多实例支持路线见 `plan/planv3.0/multi-instance-consistency-plan.md`」。
- [ ] `plan/planv3.0/key-value-ducklake-plan.md`：§2 并发模型段落补一行
      「该论证以单写实例为前提；多实例下的失效分析见
      `multi-instance-consistency-plan.md` §3.6；per-DB 租约下按库粒度继续成立」。
- [ ] `plan/planv3.0/database-always-open-plan.md`：§6 运行时段落补一行
      「常驻句柄在只读副本场景会导致陈旧读，见
      `multi-instance-consistency-plan.md` §3.7」。
- [ ] `config.example.yaml`：新增 `instance.lease` section 与注释；
      `data_inlining_row_limit` 注释标注「远端模式必须为 0，见
      `multi-instance-consistency-plan.md` §4.2」。
- [ ] `README.md`：部署章节若提到扩容，明确"当前同一库不支持多写实例；分片单写见本文 §4.6"。

---

## 11. 风险与不做

### 风险

| 风险 | 处理 |
|---|---|
| 目标对象存储不支持条件写 | 启动探针（§4.5）实测 `create-if-absent`；失败则禁用互斥能力、`/health` 暴露、退回纯单实例模式。**不再有 v1 的 B' 退化方案**——观察式判活 + 不可变快照在探针失败时整体不启用，宁可明确不可用也不静默假保护 |
| COS 存储桶开启版本控制导致 `x-cos-forbid-overwrite` 失效 | 部署文档明确禁止开启版本控制（§4.5）；备份改由 `snapshots/` 不可变序列承担；探针每次启动都跑，配置漂移会被立即发现 |
| AWS SDK 的 `IfMatch` 字段被 COS 静默忽略 | API 收敛为 `PutIfAbsent` 单原语，不存在 `IfMatch` 调用面（§4.5） |
| 租约误判导致健康实例被抢 | 观察式判活只用本地单调时钟 + epoch 变化布尔（§4.6），不依赖跨时钟差；`ttl=30s` + `grace=10s` 给足余量；续约失败先重试 N 次再判失租；`lease_renew_failures_total` 先告警再动作 |
| S3 抖动导致续约失败雪崩 | 续约失败不立即判失租（连续 ≥3 次 / 超过 ttl）；续约请求单独短超时，不与业务请求争抢连接池 |
| per-DB 租约的路由亲和改造范围 | 仅网关/LB 增加 `databaseID` 哈希路由 + 409 语义；未持租实例自动触发接管。是有界工程项，与 v1 方案 3 的"整体换 PG"相比风险低一个数量级 |
| lease 对象被人误删 | epoch 单调递增可事后审计；Runbook 写明"删 `lease/{epoch}.json` 会让对应库触发接管" |
| 新增 `PutIfAbsent` 破坏现有 `PutBytes` 调用方 | `PutBytes` 保持原签名与无条件语义，新增独立方法；只有 lease/manifest/账本使用新原语 |
| `local-state.json` 损坏导致误判 | 损坏视为 0 走保守下载分支（§4.3）；state 文件由 rename 原子写 |
| Phase B 的水位比较依赖 state 文件与 manifest 一致性 | manifest 是唯一事实来源；state 仅本地缓存，冲突时以 manifest 为准并告警 |
| catalog 体积超阈值（§6.3） | `simplebase_catalog_size_bytes` 是 P4 决策的触发指标；P0-0 关内联后只含元数据，到达时间大幅推迟 |
| COPY FROM DATABASE 全库停顿（§3.1 实现缺陷） | `syncOnce` 顺序修正 + 体积告警；长期解法是 P4（无本地 catalog） |

### 不做

- **不引入 etcd / Redis / ZooKeeper / Consul** 做分布式锁。只用已有的 S3 + `create-if-absent`。
- **不在本计划里实现"同一库多实例并发写"**。per-DB 租约提供的是分片单写；真多写见 §6（P4），需独立计划文档。
- **不做实例级自动 failover 切换**。库级失租 → 释放句柄 → 路由层按 409 重定向/接管；实例编排交给 k8s。
- **不做 catalog 增量同步（WAL shipping）**。它是 `db-ducklake-plan.md` §十一 开放问题 2 的内容，与本计划正交。
- **不改 DuckLake 数据面布局**（`data/` 前缀、Parquet 格式、descriptor、UUIDv7 命名）。
- **不改 `Registry` 的 `CloseIdle` / `Shutdown` / `max_open` 语义**（Phase D 才加 `Swap`）。
- **不把 degraded 自动恢复为 ready**（split-brain 标记的 degraded 需按 §9 流程人工确认后恢复）。
- **不改云 Agent 工具**（`internal/cloudagent` 的 `list_databases` / `readonly_sql` 等）。
- **存储桶不开启对象版本控制**（与 §4.5 的条件写冲突；备份走 `snapshots/` 序列）。
- 本 PR 不实现任何代码。

---

## 12. 文件与触点（v2 修订）

```text
新增
  internal/database/lease/lease.go              — per-DB 租约：观察式 epoch 获取/续约/释放
  internal/database/lease/payload.go            — 租约 JSON 载荷
  internal/database/lease/lease_test.go
  internal/database/ducklake/manifest.go        — manifest 序列读写 + seq 前探（Phase A）
  internal/database/ducklake/catalog_refresher.go   — Phase D 才需要
  internal/database/ducklake/orphan_ledger.go   — 孤儿账本（Phase B）

修改（Phase A）
  internal/objectstore/blob.go        — BlobStore.PutIfAbsent + ErrPreconditionFailed + 启动探针
  internal/objectstore/client.go      — COS 适配：x-cos-forbid-overwrite + 409 映射
  internal/objectstore/keys.go        — ManifestKey / SnapshotKey / LeaseKey / OrphanLedgerKey / ProbeKey
  internal/database/ducklake/catalog_syncer.go — 不可变快照 + manifest 写入（:305-320 重写）、
                                                 pruneVersions 按 manifest（:341-358 重写）、
                                                 syncOnce 顺序修正（:278-298）、
                                                 local-state.json 读写、CloseNoFlush
  internal/database/ducklake/options.go — 远端模式 data_inlining_row_limit 强制 0（:44）
  internal/database/ducklake/factory.go — EnsureLocalCatalog 接 manifest 水位（:53-57）
  internal/config/config.go           — InstanceConfig.Lease（:44-47）
  internal/config/yaml.go             — instance.lease 映射（参考 :91 yamlCatalogSync）
  internal/app/app.go                 — 探针与租约装配（:297 之后、:319 之前）；identity 检测
  internal/observability/metrics.go   — 新增 10 个指标（:101-117 之后追加）
  config.yaml / config.example.yaml   — data_inlining_row_limit、instance.lease、delete_older_than

修改（Phase B）
  internal/database/registry/registry.go        — 失租释放句柄（Remove(dbID)）+ Swap（Phase D）
  internal/database/ducklake/factory.go         — READ_ONLY ATTACH 分支（:146-151）
  internal/app/app.go                            — reader 装配路径（:301 的 writable 短路需拆开）
  internal/gateway（或等价入口）                  — databaseID 一致性哈希路由 + 409 语义

修改（Phase E，若做）
  internal/systemdb/*                            — 控制面迁移至嵌入式 SQLite / PG
  internal/systemdb/bootstrap.go                 — locator 移除（§5.4 作废）
  internal/cronjob/scheduler.go                  — ClaimCronJob 改走控制面存储的真 CAS

不改
  internal/database/kv/*                （P4 需要同库多写才动；分片单写下现论证成立）
  internal/cloudagent/*
  DuckLake 数据面布局与 descriptor 格式
```

---

## 附：v1 → v2 变更索引（评审追溯）

| v1 内容 | v2 处置 |
|---|---|
| §1（一致性依赖点） | 保留为 §1.1/§1.2；新增 §1.3-§1.5 机制基础（Parquet 三不变量、提交协议、三条推论） |
| §2/§3（部署形态与损坏路径） | 全部保留为 §2/§3；§3.1 补两条实现缺陷（COPY 顺序、全库停顿）；§3.5 从"需实测"改为"CAS 失效是设计事实"（依据 `ducklake.md:2900-2903`） |
| §4.2 `BlobPutOptions{IfMatch, IfNoneMatch}` | 收敛为 `PutIfAbsent` 单原语（§4.5）；`IfMatch` 调用面整体删除 |
| §4.3 实例级租约 + `IfMatch` 续约 + `LastModified` 判过期 + 失租退出 | 改为 per-DB 租约 + epoch 观察式判活 + 失租释放句柄（§4.6） |
| §4.4 catalog ETag 守卫 | 替换为不可变快照 + manifest 指针（§4.4），结构上消除覆盖 |
| §4.5 直读 sqlite 取 max(snapshot_id) | 简化为 `local-state.json` 水位比较（§4.3），去掉 sqlite 驱动依赖 |
| §4.7 维护任务持租校验 | 保留（§4.7）并追加孤儿账本（修"失租实例孤儿永不回收"漏洞） |
| §5.2 READ_ONLY "需前置验证" + 退化分支 | 删除；官方文档已确认支持（`ducklake.md:1778`/`:4082`） |
| §5.2 R3 SNAPSHOT_VERSION "否决（无效）" | 改为"作为刷新机制否决，保留为读一致性锚点"（§5.2） |
| §5.4 locator 移 S3 | 保留为 §5.4（Phase E 前的止血）；Phase E 后整体作废 |
| §6 方案 3 = 整体换 PG catalog | 拆为 P3（控制面独立，§6.4）+ P4（数据面按库灰度共享 PG，§6.1-§6.3）；§6.1 表格 §3.4/§3.5 两格修正为"部分解决/仍不可靠" |
| §7 Phase A "前置验证一票否决" | 改为"启动探针 + 失败禁用互斥并暴露 /health"（§4.5） |
| §8-§11（DoD/文档/风险/触点） | 按上述变化重写；新增 §9 split-brain 恢复流程；§11 不做清单追加"存储桶不开版本控制" |
| — | 新增 P0-0（关闭远端内联，§4.2，第一优先级） |
| — | 新增 §6.4 部署矩阵（单实例/单写多读/分片多写/真多写） |
