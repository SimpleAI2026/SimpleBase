# DuckLake catalog 引擎可配置方案（DuckLake + DuckDB/SQLite + S3）

> 日期：2026-09-29（v2.1：评审修订 6 处——§4 启动校验为硬性前提、List 报错 fail closed、本地探测范围、reset 租约判活、删除 evict 改动项、默认值与 P0 的矛盾；另 STORAGE_VERSION 挪到 P0 验证）。v2 替换了 v1 的“双格式兼容 + 存量迁移”方案。状态：**部分完成，尚未验收**（逐项核查见 §8；P0 未完成、P1 部分完成、P2 部分完成）。
> 基线：`main` @ `5579782` 加上当前工作区里未提交的改动。
> 关联：`ducklake-storage-latency-optimization-v2.0-plan.md`（**v2.0**）、`multi-instance-consistency-plan.md`（**MIC**）。本计划只把 catalog 存储引擎改为**可配置**，其余机制都不动：S3 不可变快照 + manifest 链、per-DB 写租约、split-brain 检测、失租后 fail-closed。v2.0 §0-1 的“架构锁定 SQLite”修订为“锁定**本地单文件 catalog**，引擎由配置决定（推荐 DuckDB）”。

---

## 0. 结论与原则

1. **catalog 引擎由 `config.yaml` 决定，取值 `duckdb` 或 `sqlite`**，整个实例（用户库 + 系统库）统一使用一种，**默认 `duckdb`**。
2. **不做兼容，也不做迁移**：不支持两种格式并存，不写 SQLite↔DuckDB 迁移器，不写兼容镜像。
3. **不兼容就拒绝启动**：配置的引擎和已有数据（本地缓存或 S3）的引擎不一致时，`app.New` 直接返回错误，进程退出码非 0。错误信息要说明如何 reset。
4. **切换引擎的唯一方式是 reset**：执行显式的 `simplebased reset`，清空本实例的全部历史数据库（本地缓存 + S3 前缀下的 catalog、Parquet、manifest、租约、系统库、locator），然后按新引擎从空状态启动。**reset 会丢掉所有数据，必须二次确认**。
5. 为什么推荐 DuckDB：官方选型写的是“单客户端用 DuckDB，多个本地客户端用 SQLite”（`docs/database/ducklake.md:1852-1868`）。本项目是每库一个 DuckDB 实例、`SetMaxOpenConns(1)`、靠租约保证单写（`ducklake/factory.go:89-91`），属于单客户端。换成 DuckDB 后，服务实例不用加载 `sqlite` 扩展，catalog 类型也是原生的。保留 `sqlite` 选项，作为已知可用的回退，也便于 P0 做对照。

---

## 1. 现状依赖点（SQLite 硬编码）

| 触点 | 现状 | 位置 |
|---|---|---|
| 本地布局 | `{cache}/{db}/catalog/catalog.sqlite` | `ducklake/factory.go:232-270` |
| 挂载 | `ATTACH 'ducklake:sqlite:{CatalogFile}' AS lake (...)` | `ducklake/factory.go:162-177` |
| 扩展 | `requiredExtensions = ducklake, sqlite, httpfs` | `ducklake/extension.go:14-19,50-79` |
| 同步快照 | `ATTACH staging.sqlite (TYPE SQLITE)` + `COPY FROM DATABASE __ducklake_metadata_lake` | `ducklake/catalog_syncer.go:448-523` |
| 远端 key | `catalog/snapshots/{snap}-{epoch}.sqlite`、兼容镜像 `catalog/catalog.sqlite`、`versions/{snap}.sqlite` | `objectstore/keys.go:155-191` |
| Content-Type | `application/x-sqlite3` | `catalog_syncer.go:513,649` |
| 冷启动 | manifest 失败时回退旧 key `catalog.sqlite` | `catalog_syncer.go:804-833` |
| manifest | 不带格式字段 | `ducklake/manifest.go:19-25` |
| 系统库 | 共用 Factory/Remote/Syncer，只是 CacheDir 不同 | `internal/app/app.go:705-720` |
| 启动入口 | `config.Load` → `app.New` → `RunWithSignal`，没有子命令 | `cmd/simplebased/main.go` |

`inspect.go`、`kv/`、`registry/`、`query.go` 都通过 DuckLake 抽象访问，与引擎无关，**不改**。

---

## 2. 配置

```yaml
database:
  engine: ducklake
  ducklake:
    catalog_engine: duckdb      # SIMPLEBASE_DUCKLAKE_CATALOG_ENGINE；duckdb | sqlite，默认 duckdb
                                # 实例级、一次性选择；与已有数据不一致时拒绝启动，只能 `simplebased reset` 后切换
```

- `config.Validate` 只接受 `duckdb`/`sqlite`，其他值直接报错。
- 映射到 `ducklake.Options.CatalogEngine`（`internal/app/app.go` 中的 `duckLakeOptions`）。用户库和系统库 Factory 用同一个值，**不允许系统库单独配置**。
- `config.example.yaml`、`config.yaml`、`internal/config/{config,yaml,env}.go` 要同步修改（本计划获批后实施，属于经用户授权的配置变更）。

---

## 3. 引擎差异（按配置分支，不混用）

| 项 | `duckdb` | `sqlite` |
|---|---|---|
| 本地 catalog 文件 | `catalog/catalog.ducklake`（另有 `catalog.ducklake.wal`） | `catalog/catalog.sqlite` |
| ATTACH | `ATTACH 'ducklake:{path}' AS lake (...)` | `ATTACH 'ducklake:sqlite:{path}' AS lake (...)` |
| 必需扩展 | `ducklake`, `httpfs` | `ducklake`, `sqlite`, `httpfs` |
| 同步 staging | `ATTACH '{staging}.ducklake' AS sb_catalog_backup` | `ATTACH '{staging}.sqlite' AS sb_catalog_backup (TYPE SQLITE)` |
| 快照 key | `catalog/snapshots/{snap:020d}-{epoch:020d}.ducklake` | `…/{snap}-{epoch}.sqlite` |
| Content-Type | `application/octet-stream` | `application/x-sqlite3` |

两种引擎共用的部分：

- `DATA_INLINING_ROW_LIMIT` 在远端模式下强制为 0（MIC §4.2），`OVERRIDE_DATA_PATH true`，`AUTOMATIC_MIGRATION false`，`lock_configuration`。
- 同步都用 `COPY FROM DATABASE` 写入新的 staging 文件，并在 COPY 前后校验 snapshot 未变化（`catalog_syncer.go:448-497`）。对 DuckDB 来说，这等于每次同步都压缩一次，避免活文件越用越大。
- **删除兼容路径**：`mirrorLegacyKeys`、`EnsureLocalCatalog` 里“无 manifest 时回退 `catalog.sqlite`”的分支、`DuckLakeCatalogKey`/`DuckLakeCatalogVersionKey`。没有 manifest 就按新库处理，前提是先通过 §4 的实例级格式校验。

### 3.1 manifest 增加字段

```json
{ "seq": 42, "snapshot_id": 1234, "snapshot_key": "...ducklake",
  "writer_epoch": 7, "updated_at": "...",
  "catalog_engine": "duckdb",       // 必填；与配置不一致 → 打开失败
  "size": 1048576, "sha256": "..." } // 下载后在 rename 之前校验
```

manifest 缺少 `catalog_engine`（旧数据）视为不兼容，处理方式和 §4 一样：直接报错，提示先 reset。

---

## 4. 启动兼容性校验（fail fast）

### 4.1 实例级格式标记

- **远端**：`{root_prefix}/{env}/catalog/instance-format.json`，使用 `KeyBuilder.CatalogPrefix()`，与现有 CAS 探针同在一个前缀下：
  ```json
  { "catalog_engine": "duckdb", "format_version": 1,
    "created_at": "...", "created_by": "{instance_id}/{boot_id}" }
  ```
  用 `PutIfAbsent` 创建，**永远不覆盖**，只有 reset 能删除它。
- **本地**：`{cache_dir}/instance-format.json`，内容相同，远端不可用时（dev_mode/本地模式）用它判断。

### 4.2 校验流程（`app.New` 中，放在 CAS 探针之后、`systemdb.Bootstrap` 之前）

```
读远端标记（Remote.Enabled 时）/ 本地标记
├─ 两者都不存在：
│    ├─ 扫描到任何已有数据（见 4.3） → 报错 ErrCatalogEngineUnknown（历史数据无标记，无法判定）
│    └─ 完全干净 → 写标记（远端 PutIfAbsent；冲突说明另一个实例抢先 → 重读后再比较）→ 继续
├─ 标记.catalog_engine == 配置 → 继续（本地缺标记时补写）
└─ 不一致，或远端与本地标记互相矛盾 → 报错 ErrCatalogEngineMismatch，拒绝启动
```

> **硬性前提（v2.1 修订）**：删除旧 key 回退后，一个只有 `catalog/catalog.sqlite`、没有 manifest 的旧库会被当作新库，建空 catalog 并写 manifest，历史数据被“隐藏”。唯一能拦住它的就是本节的实例级校验，所以 **§3 删兼容路径与 §4 启动校验必须同一版本发布，不允许单独上线 §3**。

### 4.3 “已有数据”探测（只在没有标记时执行，O(1) 级请求）

- 本地：以下三处库目录下存在任何 `catalog/catalog.sqlite` 或 `catalog/catalog.ducklake`，或系统库 locator 文件（`{cache_dir}/system/locator.json`）存在：
  - `{cache_dir}/*`（用户库）
  - `{cache_dir}/system/dbs/*`（系统库）
  - `{cache_dir}/dev/dbs/*`（dev_mode 用户库）
- 远端：对 `{base}/tenants/` 做一次 `List(prefix, max=1)`（系统库也在 `tenants/{reserved}/` 下，一次覆盖）。
- **List 报错即 fail closed**：COS 在自定义 endpoint 下 `ListObjectsV2` 可能误报 NoSuchKey/404（见 `objectstore/client.go` 的 `Check` 注释）。任何 List 错误（包括 NotFound 语义）都返回错误、拒绝启动；只有 List 成功且返回 0 个对象才算“干净”。绝不把错误当成空前缀写标记。

### 4.4 每次打开库时再校验一次（防御）

`Factory.Open` / `EnsureLocalCatalog`：满足以下任一情况时返回 `ErrCatalogEngineMismatch`，不打开库，也不下载：

- manifest 的 `catalog_engine` 与配置不一致；
- snapshot_key 的后缀与配置不一致；
- 本地存在另一种引擎的 catalog 文件。

这样可以兜住“标记正确，但个别库被手工篡改或残留”的情况。

### 4.5 报错信息（示例）

```
simplebased: catalog engine mismatch: config database.ducklake.catalog_engine=duckdb,
existing data uses sqlite (marker s3://bucket/simplebase/prod/catalog/instance-format.json).
Switching engines is not supported and requires wiping ALL databases:
  simplebased reset --confirm=<instance_id>
or set catalog_engine back to "sqlite".
```

- 同时暴露指标 `simplebase_catalog_engine_mismatch=1`，并写一条 Error 日志。**日志里不能出现凭据**。

---

## 5. reset 命令

### 5.1 用法

```
simplebased reset --confirm=<instance_id> [--local-only] [--dry-run]
```

- `cmd/simplebased/main.go` 在 `config.Load` 之后分派子命令：`reset` 走 `app.Reset(ctx, cfg, opts)`，**不启动 HTTP 服务，也不打开任何数据库**。
- `--confirm` 必须等于配置中的 `instance.id`（环境名），防止误清其他环境。缺少或不匹配就拒绝执行。
- `--dry-run`：只列出将要删除的本地路径、远端前缀和对象数量，然后退出。
- `--local-only`：只清本地缓存（用于远端已经由运维清空的场景）。**远端标记仍在且与配置不一致时，下次启动仍会报错**。
- 以下情况拒绝执行：`dev_mode=false` 且没有设置 `SIMPLEBASE_ALLOW_RESET=1`（生产环境要多一道确认）。

### 5.2 执行步骤

1. **排他性**：远端模式下，用 `PutIfAbsent` 写 `{base}/catalog/reset.lock`（带 owner 和过期时间），失败说明有另一个 reset 在跑，直接退出。检测到有效的实例租约（有存活实例正在写）时拒绝执行，要求先停掉所有实例。reset 本身不能和运行中的实例并发执行。
   - **租约判活（v2.1 修订）**：租约是“观察式”判活，续约 = 每 `renew_interval` 用 `PutIfAbsent` 写 `lease/{epoch+1}.json`。因此 reset 的判活方式是：分页 List `{base}/tenants/`，统计所有 `*/catalog/lease/*.json` 对象（按库记录最大 epoch）；等待 `instance.lease.ttl + instance.lease.grace`（默认约 40s）；再 List 一次。**任一库的最大 epoch 增加 → 有存活实例 → 拒绝执行**。两次 List 任一失败同样拒绝。`--local-only` 不做此检查。
2. **删除远端**（Remote.Enabled）：分页 List 后批量 Delete。范围是：
   - `{base}/tenants/**`：用户库的 descriptor、catalog 快照、manifest、Parquet、租约、孤儿账本；
   - 系统库所在前缀和 locator；
   - `{base}/catalog/instance-format.json`；
   - **保留** CAS 探针对象和 `reset.lock`，最后再删。
   
   删除按批次打日志和计数。任一批失败就停止并返回错误。**不写新标记**，因此下次启动仍然会拒绝，直到 reset 成功完成。
3. **删除本地**：`{cache_dir}` 下全部内容（用户库、`system/dbs`、`instance-format.json`、`.wal`、staging），以及系统库 locator 目录。删除前做路径安全检查：拒绝空路径、`/` 和家目录；目标必须是配置中的 cache_dir 或 locator dir 本身。
4. **收尾**：删除 `reset.lock`，输出摘要（删除的对象数和字节数），退出码为 0。下次正常启动时，按“完全干净”分支写入新标记，重建系统库。

### 5.3 要求

- 可以重复执行：中途失败后重跑，从剩余对象继续删。
- 不删除 bucket 本身，也不删除 `{root_prefix}/{env}` 之外的任何对象。
- 文档（`docs/ops/deployment.md`）中说明：reset = 清空本实例的全部数据，不可恢复；执行前应自行备份 bucket 前缀。

---

## 6. 改动清单（文件级）

| 文件 | 改动 |
|---|---|
| `internal/config/{config,yaml,env}.go`、`config.example.yaml`、`config.yaml` | 新增 `database.ducklake.catalog_engine`，校验取值，默认 duckdb |
| `internal/database/ducklake/options.go` | `CatalogEngine` 字段与常量 `EngineDuckDB`/`EngineSQLite`；`normalized` 默认 duckdb |
| `internal/database/ducklake/factory.go` | `layoutFor(cacheDir, id, engine)`；`buildBootSQL` 按引擎生成 ATTACH；Open 检查另一种引擎的本地文件残留 |
| `internal/database/ducklake/extension.go` | `requiredExtensions(engine)`；DuckDB 引擎下不再 INSTALL/LOAD sqlite |
| `internal/database/ducklake/catalog_syncer.go` | staging 后缀和 ATTACH 类型、Content-Type、sha256、manifest 字段按引擎区分；**删除** `mirrorLegacyKeys` 和旧 key 回退；`EnsureLocalCatalog` 校验引擎和 sha256，下载替换前清理残留 `.wal` |
| `internal/database/ducklake/manifest.go` | 新增 `CatalogEngine`/`Size`/`SHA256`，读取时校验 |
| `internal/database/ducklake/errors.go`（新） | `ErrCatalogEngineMismatch`、`ErrCatalogEngineUnknown` |
| `internal/database/ducklake/instance_format.go`（新） | 读写和校验实例级标记（本地 + 远端），提供 `DetectExistingData` |
| `internal/objectstore/keys.go` | `DuckLakeSnapshotKey` 增加 engine 参数决定后缀；新增 `InstanceFormatKey`、`ResetLockKey`；删除 `DuckLakeCatalogKey`/`VersionKey` |
| `internal/objectstore/blob.go` | `BlobStore` 目前只有 Head/Put/PutIfAbsent/Get/Download/Delete，需要新增 `List(prefix, cursor, limit)` 和 `DeleteMany(keys)`（S3 ListObjectsV2 / DeleteObjects），reset 和探测要用 |
| `internal/app/app.go` | 启动时调用 §4.2 的校验；映射 CatalogEngine；新增 `Reset(ctx, cfg, opts)` |
| `cmd/simplebased/main.go` | 分派 `reset` 子命令及参数 |
| `internal/database/sqlguard/classifier-desens.go` | 拒绝用户 SQL 中出现 `__ducklake_metadata_` 标识符（与引擎无关，建议顺带修复） |
| `internal/database/README.md`、`docs/ops/deployment.md` | 引擎配置、不兼容时的报错、reset 流程、key 布局（MIC §9 中的 `*.sqlite` 改为按引擎区分） |

> `internal/database/cache/evict.go` **不需要改**（v2.1 修订）：`dirSize`/`removeAll` 以整个库目录为单位，`.wal` 已被计入和删除。

---

## 7. 风险与对策

1. **同一进程内出现两个 DuckDB 实例打开同一个 catalog 文件会损坏**（POSIX 锁以进程为单位，拦不住同进程的第二次打开）。
   - 对策：Registry 保证在 `Close` 完全返回之前，同 ID 不能再次 Open；Evict、失租 `closeNoFlush`、`Remove` 都要遵守这一点。
   - 加一个进程级 `map[absPath]` 守卫，重复打开时直接报错。
   - 补竞态测试（在 evict 的同时 Acquire、在 Close 期间 Open）。
   - SQLite 引擎也受益于这个守卫。
2. **DuckDB 文件格式与版本绑定**：DuckDB 升级后写出的新 storage 格式，旧二进制读不了。
   - 对策：创建 catalog 时固定 `STORAGE_VERSION`，与 `MinDuckDBVersion` 对齐。**前提未确认**：ducklake 的 `ATTACH 'ducklake:...'` 是否透传 `STORAGE_VERSION` 给底层 DuckDB catalog 文件，需在 P0 验证；不支持时退化为“升级 DuckDB 版本 = 按不兼容处理（报错 → reset）”，并在 `format_version` 中记录创建时的 DuckDB 版本。
   - 实例标记的 `format_version` 用来记录这个约定；以后要提升 storage version，也按“不兼容 → 报错 → reset”处理，或者另立计划。
3. **小库的 DuckDB 文件可能更大**（按块分配）。P0 实测 1/10/200 张表时的快照大小；如果明显变大，快照上传前做 zstd 压缩。manifest 新增 `content_encoding` 字段；引入直接依赖需要另行确认。
4. **WAL 残留**：下载替换前删除 `catalog.ducklake.wal`。只有确认本地不领先时才替换，这一点现有逻辑已经保证。
5. **reset 误操作会丢掉全部数据**：通过 `--confirm=<instance_id>`、生产环境额外设置环境变量、`--dry-run`、租约存活检查、`reset.lock` 五层防护。reset 不能从 HTTP API 触发。
6. **多实例同时首次启动，抢写标记**：用 `PutIfAbsent` 保证只有一个实例写成功，失败的一方重读标记再比较。配置不一致的实例会被拒绝启动，避免同一前缀下出现两种引擎。
7. **单连接占用**（v2.0 §3-8）不因换引擎而变好或变坏。**不允许靠放大 `MaxOpenConns` 来回避**。

---

## 8. 分阶段实施与验收

> **2026-09-29 状态核查**：以下状态以当前工作区代码和可见测试为准，不等同于生产环境或 MinIO/COS 实测；原验收要求保留，未覆盖的项目不标记完成。
>
> | 阶段 | 状态 | 已有证据 | 尚缺 |
> |---|---|---|---|
> | P0 | **未完成（部分行为已测试）** | `internal/database/ducklake/engine_test.go` 覆盖双引擎写入、sync、冷开及同进程重复打开；`catalog_syncer_test.go` 覆盖写入发生在快照上传期间的最终同步；`runtime_test.go` 包含普通 time travel 和版本查询。 | 未见计划中的双引擎矩阵基准与 p95/快照大小对比，未验证 `STORAGE_VERSION` 能否透传、COPY 期间并发写的一致性，也未见目标 S3 DATA_PATH 双引擎读写实测；因此无法判定默认引擎是否达到门槛。`output/perf/2026-09-29/` 是 API/DB/KV 场景结果，不是该矩阵。 |
> | P1 | **部分完成，仍有阻塞，未验收** | 配置、引擎分支、快照/manifest、实例标记和 `reset` 已有主体实现；本轮补充双引擎配置、SQL 内部 metadata 标识符拦截、租约双次 List 与 tenant+db 分组、S3 DeleteObjects 逐项错误检查和专项测试。 | 仍需解决下述未关闭的安全项，并补启动标记冲突/未知数据、reset 安全/失败重试/重建、双引擎 S3/系统库重启恢复及 MinIO 前缀隔离验收。 |
>
> **P1 必须先修复、再验收的阻塞项（静态审计，尚未做故障注入）**：
> - **已修复部分**：`internal/app/reset.go` 即使首次无租约也等待后重新 List，租约按 tenant+databaseID 统计；`internal/objectstore/blob.go` 在批删/单删时检查逐项错误。`reset.lock` 不再自动过期接管（缺少安全的条件删除）；仍须验证长时间 reset 的锁策略、失败重跑/人工排障流程与并发安全，不能把锁问题标成全部解决。
> - **未完成**：`reset` dry-run 远端计数与实际删除口径不一致，未提供删除字节数摘要；生产模式仍须完成租约与锁故障注入、MinIO 越界删除验证。
> - **待解决**：`internal/app/app.go` 只读实例跳过启动引擎校验；`internal/database/ducklake/instance_format.go` 把本地目录读取/文件探测失败视作无数据。`catalog_syncer.go` 本轮已改为旧 WAL 删除失败时拒绝替换、下载校验要求正大小和 SHA-256；仍需扩大启动 fail-closed 专项测试。
> - `internal/database/registry/registry.go:214-218,236-251,310-323` 在 Close 完成前删除同 ID entry；`internal/database/cache/evict.go:102-123` Close 后删目录期间仍可能 Acquire。虽然 `ducklake/factory.go` 已有进程级重复打开守卫，§7 的完整生命周期/淘汰竞态尚未满足，也缺对应并发测试。
> - 当前系统库 locator 位于 `cache_dir/system`（`internal/app/app.go:401-404`），属于 `reset` 清理范围；不要把“未单独删除 cache_dir 外 locator”误记为已发现缺陷。
> | P2 | **部分完成，未验收** | 本轮将配置默认值显式设为 `duckdb`，`config.example.yaml` 新部署推荐 `duckdb`，存量 COS 配置 `config.yaml` 显式保留 `sqlite`；`docs/ops/deployment.md`、`internal/database/README.md` 已补引擎布局、升级与 reset 说明。 | P0 双引擎基准门槛尚无结果，推荐默认值不能判定为最终验收通过；生产 COS/MinIO 未验证。 |
>
> **本轮验证**：`go test ./internal/database/... ./internal/objectstore/... ./internal/app/... ./cmd/...` 复跑通过；`go build ./internal/... ./cmd/...` 通过。一次中间测试运行遇到 `internal/database/kv/sweeper.go` 的 `close of nil channel` 偶发 panic，复跑通过；该竞态并未因此修复。测试通过不代表上述缺失的验收场景已覆盖。

### P0 — 实测（spike/基准，不改服务路径）

- 在目标构建环境（`duckdb-go/v2 v2.10505.0` 实际的 `SELECT version()` 与 ducklake 扩展版本）下，验证 DuckDB 引擎的以下行为：
  - 建库、重开、`OVERRIDE_DATA_PATH`、S3 DATA_PATH 读写；
  - `COPY FROM DATABASE` 在 COPY 期间有写入时的一致性；
  - time travel；
  - 同进程重复打开同一文件的行为。
- 基准：两种引擎各跑一遍，1/10/200 表 × 0/1k/100k 行 × 10/1000 次提交。测量：
  - 冷开耗时；
  - 写提交 p50/p95；
  - sync 耗时和连接占用时长；
  - 快照字节数；
  - `snapshots()`/`list_files`/`information_schema` 耗时。
- **门槛**：DuckDB 的冷开和写入 p95 不差于 SQLite，小库快照 ≤ SQLite 的 2 倍（超过就启用压缩后重测）。**不达标时默认值保持 `sqlite`**，但可配置的能力照常实现。

### P1 — 引擎可配置 + 启动校验 + reset

- 实现 §2–§6 的全部内容。
- 验收：
  - duckdb、sqlite 各自全链路通过：建库 → 写 → sync → 删本地缓存 → 冷开 → time travel → 系统库重启恢复；
  - **不兼容场景都报错且不改动任何数据**：
    - 标记为 sqlite、配置为 duckdb；
    - 标记为 duckdb、配置为 sqlite；
    - 没有标记但有历史数据；
    - 本地标记与远端标记矛盾；
    - manifest 的 engine 或后缀不一致；
    - 本地残留另一种引擎的文件；
  - reset：
    - dry-run 列出的清单准确；
    - confirm 错误时拒绝；
    - 有存活租约时拒绝；
    - 中途失败后重跑能完成；
    - 完成后按新引擎启动成功，系统库重建，旧对象为 0；
    - 不触碰 `{root_prefix}/{env}` 之外的对象（用 MinIO 验证）；
  - `go test ./internal/database/... ./internal/objectstore/... ./internal/app/... ./cmd/...` 通过。

### P2 — 默认值与文档

- **默认值说明（v2.1 修订，消除与 P0 的矛盾）**：代码默认值为 `duckdb`，这是“新部署”的推荐值；P0 基准未达标时，只把 `config.example.yaml` 的推荐值改回 `sqlite`，代码默认值的调整另行决定。
- **现有部署（仓库内 `config.yaml` 指向已有 SQLite 数据的 COS 前缀）必须显式配置 `catalog_engine: sqlite`**，否则升级后被 §4 拦截无法启动。这一行随本计划的代码一起提交。
- 根据 P0 结论确定默认值（优先 duckdb）；更新 `config.example.yaml`、`docs/ops/deployment.md`、`internal/database/README.md`。
- 现有部署升级到新版本时：如果配置取默认值 duckdb，而已有数据是 sqlite，会被 §4 拦截。上线说明中要写明两种选择：显式配置 `catalog_engine: sqlite` 继续使用，或者备份后执行 reset。

---

## 9. 不做

- 不做 SQLite ↔ DuckDB 数据迁移，不支持两种格式并存，不写兼容镜像；
- 不支持用户库和系统库使用不同引擎；
- 不提供 HTTP 形式的 reset；
- 不把 S3 当作可写 catalog；不放开远端 data inlining；不放大 `MaxOpenConns`；不改租约、manifest 链和 split-brain 语义；不引入 PostgreSQL。
