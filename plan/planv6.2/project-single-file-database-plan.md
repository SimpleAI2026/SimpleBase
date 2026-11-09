# 项目级单文件数据库改造计划

## 1. 背景与目标

当前用户库模型是“一个项目包含多个 logical database，每个 database 一套 DuckLake catalog + DATA_PATH”。其中 `catalog_engine=duckdb|sqlite` 只选择 DuckLake 元数据 catalog 的格式，用户数据仍可能位于独立 Parquet 文件中，并不等于单文件数据库。

本次改造目标：

1. 每个普通项目最多一个默认数据库，对应一个完整的 `.duckdb` 或 `.sqlite` 主文件。
2. `sys_projects` 增加 `database_path`，保存该项目数据库文件的相对位置。
3. SQL、集合文档和项目 KV 共用该项目文件，不再创建独立的 KV 物理库。
4. 本地模式直接持久化项目文件；S3 模式通过本地工作副本、不可变文件快照、manifest 和项目级写租约持久化。
5. 不迁移、不读取、不回退旧 DuckLake 用户库布局；旧实例格式直接拒绝启动，要求重新初始化。
6. 系统库继续由 `systemdb.Bootstrap + locator.json + ducklake.Factory` 独立引导，不改为普通项目文件，也不经过用户 Registry。

### 1.1 核心产品决策

采用 **项目与数据库 1:1**，不采用“多个 logical database 共享一个文件”：

- DuckDB 可以用 schema 隔离多个逻辑库，但 SQLite 没有等价语法；两种引擎无法共用安全的命名空间方案。
- 对用户 SQL 做表名重写不可靠，也会破坏视图、触发器、CTE、引用和错误语义。
- 因此每个项目只允许一条未删除的 `kind=user` 默认数据库记录；`databaseID` 继续作为 API 授权和资源标识，但不再是物理存储边界。
- `POST /databases` 在项目已有默认数据库时返回 `409 already_exists`；删除默认数据库表示清空该项目全部 SQL、集合和 KV 数据，之后允许重新创建。
- `kind=kv` 记录及独立 KV 文件取消。KV 使用项目文件内的保留内部表。

### 1.2 非目标

- 不保证同一条 SQL 在 DuckDB 和 SQLite 上均可执行；原生 SQL API 使用项目实际引擎的方言。
- 不保留旧 per-database DuckLake DATA_PATH、Parquet、DuckLake snapshot 或 catalog-only 同步语义。
- 不实现旧数据在线转换、双读、双写、路径 fallback 或自动数据搬迁。
- 不把物理路径、DSN、S3 key 暴露到公开 API、日志或审计。
- 不将系统库迁移成 SQLite，也不改变系统库禁止写入、只允许 SELECT 的保护边界。

## 2. 目标模型

### 2.1 元数据关系

```text
Project (1) ─── (0..1 未删除) Database
   │                         │
   │                         └─ API/授权标识 databaseID
   ├─ database_path          └─ data_model / status / timestamps
   └─ 唯一物理文件、连接、缓存、同步和写租约边界
```

`sys_databases` 暂时保留，用于默认数据库状态机、API 标识和列表展示；它不是第二个物理存储来源。文件位置只从 `sys_projects.database_path` 读取，禁止再从 `projectID + databaseID` 推导。

`storage_prefix` 不再具有 DATA_PATH 含义。全新 schema 中删除 `sys_databases.storage_prefix` 和 `catalog.Database.StoragePrefix`，同步删除 descriptor 的 DATA_PATH 用法，避免形成两个路径事实来源。

### 2.2 系统表字段

直接修改全新系统库 v2 schema：

```sql
CREATE TABLE IF NOT EXISTS sys_projects (
    id VARCHAR NOT NULL,
    tenant_id VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    database_path VARCHAR NOT NULL,
    created_at TIMESTAMP NOT NULL
)
```

字段约定：

- `database_path` 使用 `/` 分隔，相对于本地 `storage.data_dir`；S3 模式下同时作为相对于 `s3.prefix` 的逻辑规范位置。
- 普通项目格式固定为 `{projectID}/database.duckdb` 或 `{projectID}/database.sqlite`。
- 引擎只由受校验的扩展名确定，不再增加第二个 `engine` 字段，避免二者不一致。
- `sb-admin` 是引导例外，固定保存空字符串；普通项目禁止空值。系统库位置仍只由 locator 和系统 Factory 决定，空字符串不是旧路径 fallback。
- 路径由服务端生成，项目创建请求不得传入路径。可选 `engine=duckdb|sqlite` 为空时使用实例默认值。
- 对外项目响应可以返回 `engine`，不得返回 `database_path`。

同步修改：

- `internal/catalog/model.go`：`Project` 增加 `DatabasePath`，提供统一的 `Engine()`/`ValidateDatabasePath()` 所需的领域表达；不在各调用方分别解析后缀。
- `internal/catalog/repository.go`：增加按项目读取存储元数据的能力，如 `GetProjectStorage`，避免 Factory 直接查询系统表。
- `internal/catalog/sql_repository.go`：项目 INSERT、SELECT、Scan 全部包含新字段；提供默认数据库唯一性检查。
- `internal/catalog/service.go`：创建项目时生成路径；创建/获取默认数据库时组合项目路径与 database 元数据。
- `internal/systemdb/seed.go`：`sb-admin` 显式写入空路径。
- `internal/systemdb/migrate.go`：只调整全新 v2/v3 schema；不新增 ALTER/backfill 迁移。

### 2.3 不兼容格式策略

- 将 `internal/database/ducklake/instance_format.go` 的实例格式标识泛化为实例存储格式，版本从 1 提升为 2，并记录用户库模型 `project_file`。
- 格式检查仍在系统库 bootstrap/migration 之前执行。检测到版本 1、无标记旧目录、旧 `{project}/databases/{db}/catalog` 或旧远端 marker 时直接拒绝启动并提示重新初始化。
- `DetectLocalData`、远端数据探测和 `simplebase reset` 同时识别新布局；reset 清理项目文件、版本、manifest、lease 和缓存。
- 不新增 v51 `ALTER TABLE`，不执行 `init_db`，不自动 ALTER/DDL。实施完成后由用户通过正常全新初始化流程或手动重建环境生效；本计划本身不执行任何数据库操作。

## 3. 文件布局与路径安全

### 3.1 本地模式

```text
{storage.data_dir}/
  instance-format.json
  _system/                         # 系统库与实例元数据，保持独立
  {projectID}/
    database.duckdb                # 或 database.sqlite
    database.duckdb.wal            # 引擎运行期可能短暂存在
    database.sqlite-wal            # 引擎运行期可能短暂存在
    database.sqlite-shm            # 引擎运行期可能短暂存在
    files/                         # 现有用户文件布局可保持

{database.cache_dir}/
  projects/{projectID}/tmp/
```

“单文件”指一个权威主数据库文件，不禁止引擎运行期 WAL/SHM 临时伴生文件。备份和上传前必须生成一致性快照，不能直接把 WAL 当作第二份权威数据。

### 3.2 S3 模式

活跃文件只能位于本地缓存，不允许用 DuckDB/SQLite 直接打开 S3 URI：

```text
{database.cache_dir}/projects/{projectID}/
  database.duckdb|database.sqlite # 可丢弃工作副本
  local-state.json
  staging/
  tmp/

{s3.prefix}/{projectID}/
  database/
    snapshots/{seq:020d}-{epoch:020d}.duckdb|.sqlite
    manifest/{seq:020d}.json
    lease/{epoch:020d}.json
  files/
```

`sys_projects.database_path={projectID}/database.ext` 是逻辑规范位置；manifest 指向实际不可变快照。禁止上传可变 `current` 文件后再无版本覆盖，避免部分上传和读写竞态。

在 `internal/objectstore/keys.go` 新增项目级 `ProjectDatabasePrefix`、`ProjectSnapshotKey`、`ProjectManifestKey`、`ProjectLeaseKey`，旧的 per-database DATA_PATH 和 DuckLake snapshot key 不再用于用户库。所有对象键仍由内部 ID 生成，不拼接项目名称或路径参数。

### 3.3 统一路径校验

将用户文件路径解析集中在 `internal/database/filedb/path.go`：

1. 只接受普通项目非空、清理后与原文一致的 slash 相对路径。
2. 拒绝绝对路径、`..`、`.`、反斜杠、NUL、重复分隔符和非 `.duckdb/.sqlite` 后缀。
3. 要求路径精确等于服务端为该项目生成的规范路径，防止系统表异常导致跨项目引用。
4. 本地拼接后使用 `filepath.Abs + filepath.Rel` 验证仍在根目录内；检查父目录符号链接，拒绝逃逸根目录。
5. 引擎由后缀解析，并在打开时验证实际文件格式和 Ping；不允许仅凭后缀打开来源不明的文件。
6. 普通项目路径为空、系统项目路径非空、两个项目引用同一路径均视为元数据错误并拒绝打开。

## 4. 用户库引擎与统一运行时

### 4.1 新增统一 `filedb.Factory`

新增 `internal/database/filedb/`，实现现有 `database.Factory` 和写钩子接口；它是用户库唯一 Factory，Registry 仍是唯一入口。按项目路径选择内部 driver：

- `duckdb_driver.go`：直接用 `duckdb.NewConnector(databaseFile)` 打开主文件，不再创建 `:memory: + ATTACH ducklake:`。
- `sqlite_driver.go`：使用现有 `github.com/uglyer/go-sqlite3` 打开文件；不新增第三方依赖。
- `factory.go`：路径校验、本地/远端恢复、打开守卫、Ping、资源回收、同步器绑定。
- `dialect.go`：统一引擎标识、元数据查询、DDL 类型映射、完整性检查和一致性快照能力。

约束更新：

- `internal/AGENTS.md` 将“`database/ducklake` 唯一存储引擎 Factory”改为“用户库只能经 `filedb.Factory + Registry`；系统库保留 `ducklake.Factory`”。
- `internal/database/README.md` 更新项目文件模型、本地/S3 布局和单写边界。
- 禁止 handler、catalog 或 gocloudagent 直接构造 DSN 打开文件。

### 4.2 引擎初始化

DuckDB：

- 直接打开项目文件，保留 memory limit、threads、temp directory 和允许目录等安全设置。
- 保留同进程绝对路径 open guard；句柄完全关闭后才释放。
- 不再加载用户库 DuckLake 扩展，不再设置 `DATA_PATH`、`USE lake` 或 DuckLake options。

SQLite：

- 初始实现将 `SetMaxOpenConns(1)`，先保证 DDL、事务、KV 和 checkpoint 的串行一致性，再基于基准测试决定是否开放只读连接池。
- 明确设置 `busy_timeout`、`journal_mode=WAL`、`synchronous=FULL`；每个连接执行相同初始化。
- 打开后执行 `PRAGMA integrity_check` 的快速/必要检查；损坏文件不得继续写入或覆盖远端正常版本。

### 4.3 Registry 改为项目粒度

修改 `internal/database/runtime.go` 和 `internal/database/registry/`：

- 定义项目级 `StorageTarget`，包含 `ProjectID`、可选默认 `DatabaseID`、`DatabasePath`、`Engine`、`Kind`；由 catalog resolver 构造。
- `Factory.Open` 接收 `StorageTarget`，不再从一个 `catalog.Database` 推导物理路径。
- `entries` 改为 `map[projectID]*entry`；同一项目的 SQL、集合和 KV 共享一个 `*sql.DB`、open guard 和写钩子。
- Handle 同时保存项目目标与本次逻辑数据库上下文，日志继续记录 projectID/databaseID，但不记录路径。
- `CloseDatabase` 的物理实现改为关闭整个项目；删除前必须无活跃引用。增加 `CloseProject`、`RemoveProject`、`OpenedProjectIDs`，淘汰、失租和 shutdown 全按项目处理。
- `max_open`、cache 的 `max_databases` 明确按打开的项目文件数计算，并补上实际容量检查，不能只保存配置不执行。
- KV sweeper 改为枚举已打开项目，再使用该项目 Handle；不得按已经删除的 `kind=kv` ID 查找。

系统库只读桥接仍走 `systemLeaseAdapter`，不得为了复用新 Registry 而改变。

## 5. SQL 方言与内部表隔离

### 5.1 方言接口

在 API 与引擎之间提供窄接口，由 `filedb` 或独立 `database/dialect` 包实现，handler 不自行判断驱动名拼接 SQL：

- 列出用户表和列。
- 查询单表列定义。
- 统计用户表行数。
- 校验/映射 Schema API 允许的类型。
- 执行内部完整性检查与快照。
- 返回统一的表、列、nullable、类型和统计 DTO。

DuckDB 使用 `information_schema`；SQLite 使用 `sqlite_master` 和表值 `pragma_table_info` 等内部查询。统一过滤 `__sb_`、SQLite 内部表、视图和虚拟表，确保 Schema API 输出形状不变。

需要改造：

- `internal/api/schema_handler.go`
- `internal/api/schema_rows.go`
- `internal/api/db_stats.go` / `db_stats_cache.go`
- `internal/api/gocloudagent_access.go`
- `internal/api/data_handler.go` 中引擎相关错误识别和 DDL
- `internal/database/query.go` / `serialize.go` 的驱动值与错误测试

原生 `/query`、`/execute`、`/batch` 不做 SQL 转译；文档明确 SQL 方言由项目引擎决定。

### 5.2 SQL 边界

`internal/database/sqlguard` 继续作为唯一 SQL 边界：

- 保留 ATTACH、DETACH、PRAGMA、VACUUM、CHECKPOINT、LOAD、COPY、EXPORT、USE 等管理语句拦截。
- 新增 `__sb_` 保留命名空间检查，用户 SQL 不得创建、读取、修改或删除内部表；测试大小写、引号、注释、换行和多语句绕过。
- 按引擎补充只读判断和危险函数/语句用例，不能把 DuckDB 的只读分类器未经验证直接套给 SQLite。
- 内部 DDL、KV、迁移和快照走受控内部通道，不放宽用户 SQL API。

### 5.3 KV 合并到项目文件

修改 `internal/database/kv/`：

- 删除 `CREATE SCHEMA kv` 和 `kv.*` 物理依赖，统一使用 `__sb_kv_keys`、`__sb_kv_strings` 等保留表。
- DuckDB 与 SQLite 各自提供 DDL/事务方言，外部 KV 行为、TTL 和错误协议保持一致。
- 建立必要索引；利用单文件事务保证 keys 元数据和 value 表的一致性。
- `EnsureSchema` 由项目打开/首次 KV 使用时触发，不能创建第二条 `sys_databases` 记录。
- `internal/api/kv_init.go`、`kv_handler.go`、adapter 和 app 装配改为按项目 Acquire。
- 删除 `CreateKVDatabase`、`GetKVDatabase` 的独立物理库语义及相关测试。
- 删除默认数据库时明确同时删除 KV；文档和 API 测试覆盖此行为。

## 6. 项目与默认数据库生命周期

### 6.1 创建项目

`catalog.Service.CreateProject`：

1. 校验权限、租户、项目 ID 和引擎。
2. 生成 `{projectID}/database.{ext}`，写入 `sys_projects.database_path`。
3. 不直接执行物理文件 I/O；文件在默认数据库或 KV 首次打开时创建。
4. 不再调用 `CreateKVDatabase`。

项目创建请求可选 `engine`，实例配置提供默认值；项目响应返回引擎。引擎创建后不可修改，不提供路径更新接口。

### 6.2 创建默认数据库

`CreateDatabase`：

1. 读取项目及 `database_path`，普通项目路径必须有效。
2. 检查项目没有 `creating/ready/degraded/deleting` 的用户数据库；已存在返回 `ErrAlreadyExists`。
3. 创建唯一 `kind=user` 记录，不再生成 DataPrefix、descriptor 或独立 catalog 目录。
4. 首次 Acquire 创建项目主文件并初始化；`init_sql` 失败时走同一默认数据库删除补偿，不残留半初始化文件。
5. 状态修复、缓存失效、配额和审计仍按项目/默认 databaseID 工作；`max_databases` 对普通项目自然限制为 1。

### 6.3 删除默认数据库

`DeleteDatabaseSync` 不再调用旧 `DatabasePrefix`：

1. 将默认数据库置为 `deleting`，阻止新 Acquire。
2. 按项目停止新同步并等待 in-flight 完成，确认无活跃引用后关闭项目 Handle。
3. 正常持有租约时先完成必要 Flush；删除开始后禁止旧 writer 再发布新 manifest。
4. 本地删除主文件、WAL/SHM、tmp；S3 删除项目 `database/` 下的快照、manifest 和 lease，不碰 `{project}/files/`。
5. 标记默认数据库 deleted。项目记录和 `database_path` 保留，可用于之后重新创建默认数据库。
6. 任一步失败保持 `deleting/degraded`，不得提前允许重新创建，避免旧异步任务写入同一路径。

系统库仍由 `ErrSystemProtected` 拒绝删除；补系统库拒绝和实例只读测试。

## 7. S3 一致性同步与租约

不能复用 DuckLake 的 catalog-only 上传逻辑。新增 `filedb.Syncer`，可复用现有 CAS、防抖、保留版本和观测思路，但文件快照格式与状态全部按项目重建。

### 7.1 冷启动恢复

1. 按项目读取最新完整 manifest，校验格式版本、engine、path、seq、epoch、快照 key、size 和 SHA-256。
2. 下载到 `staging`，校验后原子 rename 为本地工作文件；清理旧 WAL/SHM。
3. 比较 `local-state.json` 与远端水位。本地较旧则替换，本地较新则按明确的 local-ahead 策略保留并尽快持租发布；无法判定时 fail closed，禁止静默覆盖。
4. 打开文件并执行引擎完整性检查，再绑定同步状态。

### 7.2 一致性快照

- 写成功后按项目标脏，支持 debounce/interval；空闲关闭和正常 shutdown 前 Flush。
- DuckDB 通过受控 staging 数据库和 `COPY FROM DATABASE` 等经实测验证的一致性导出机制生成新文件；不能复制仍有未 checkpoint 状态的活主文件。
- SQLite 在受控读/写门禁下使用一致性备份能力（优先实测 `VACUUM INTO` staging）并 checkpoint，生成独立快照文件。
- 快照完成后验证可打开、完整性、大小和 SHA-256，再上传不可变对象。
- 先 PUT snapshot，再以 `PutIfAbsent` 发布下一 seq manifest；冲突视为 split-brain，停止写入并失租，不重试覆盖。
- 上传或 manifest 失败保留 dirty 与本地文件，不删除缓存，不推进已同步水位。

### 7.3 项目级写租约

修改 `internal/database/lease/` 和 app 接线：

- lease map、key、renew、epoch、onLost 全部改为 projectID；同项目 SQL、集合、KV 只有一把写租约。
- 首个 ReadWrite Acquire 前获得项目租约；发布 manifest 前再次校验 epoch。
- 失租后同步器立即停止 PUT，Registry `RemoveProject`，旧 Handle 禁止继续写和 CloseFlush。
- `TryDo`、`SetWriterEpoch`、reset lease 扫描和观测指标改为项目粒度。
- 系统库现有租约/同步不被用户项目改造破坏；若共享底层机制，明确 target kind，禁止 key 冲突。

### 7.4 缓存与版本清理

- `internal/database/cache` 从 UUID database 目录改为项目目录，Usage 统计主文件、staging 和 tmp；淘汰前按项目关闭并 Flush，活跃项目禁止淘汰。
- local-state、dirty、lastSeq、writerEpoch、timer、inflight、lostLease 全部按 projectID 建索引。
- `keep_versions` 只清理不再被 manifest 引用的旧文件快照，保留回滚所需对象；孤儿清理由 ledger/标记机制防止误删未发布对象。
- API 中原 `snapshot` 若保留，只表示文件 manifest 的 `last_synced_seq/sync_lag`，不再宣称 DuckLake snapshot；也可以随公开契约调整改为 `storage_sync`。

## 8. 配置、装配与公开调用方

### 8.1 配置

在 `internal/config` 和 `config.example.yaml` 中明确：

- `database.engine` 改为普通项目默认引擎 `duckdb|sqlite`，不再固定 `ducklake`。
- `database.ducklake` 仅保留系统库 DuckLake 配置，注释删除“用户库唯一引擎”语义。
- 文件同步可复用 debounce、interval、keep_versions 等通用值，但配置命名和注释改为项目文件同步；不引入新的配置来源。
- 更新 `SIMPLEBASE_DB_ENGINE` 校验、示例 env、默认值和配置测试。项目级 engine 只覆盖新建项目，已有项目始终以路径后缀为准。

### 8.2 App 装配

修改 `internal/app/app.go`：

```text
格式 v2 检查
→ 系统 ducklake.Factory / systemdb.Bootstrap
→ catalog Repository/Service
→ 用户 filedb.Factory + filedb.Syncer
→ 项目级 lease + Registry
→ KV/Schema/Data/SQL/gocloudagent adapters
→ cache/maintenance/reset
```

- 用户 Factory 与系统 Factory 分离，系统 Syncer 不绑定用户项目状态。
- 所有新增资源进入 `assembleDeps` 和反向 Close，失败路径释放。
- `newUserDatabaseFactory`、`snapshotFromFactory`、KV initializer、sweeper、lease onLost、reset 全部改用新接口。
- DuckLake maintenance 只处理系统库；文件快照清理由 `filedb.Syncer` 负责。

### 8.3 API、UI、SDK 与 Agent

公共 API 可继续使用 `projectID + databaseID`，避免泄露物理路径，但行为变为默认数据库 1:1：

- 第二个 CreateDatabase 返回 409；ListDatabases 最多返回一条普通用户库。
- CreateProject 可选 engine，Project 响应包含 engine，不包含路径。
- UI 创建项目增加引擎选择；数据库页隐藏/禁用第二库创建入口，并说明删除默认库会同时清空 KV。
- `ui/src/services/types.ts` 使用 `interface` 扩展契约，同步 `zh-CN.ts` 和 `en-US.ts`，不写新的 `any`。
- Go/JS SDK 更新 CreateProject 的可选 engine 与 Project.engine；不暴露数据库路径。
- sbcli/gocloudagent 继续通过 API 使用 databaseID，不直接访问文件；补充引擎返回和单默认库测试。
- SQL/Schema/集合/KV/gocloudagent 在两种引擎下行为一致；原生 SQL 方言差异写入文档。

## 9. 分阶段实施清单

### 阶段 A：元数据与格式闸门

- [x] 修改全新 `sys_projects`/`sys_databases` schema、`Project`、`Database` 和 Repository。
- [x] 实现路径生成、引擎解析、普通项目/系统项目校验及测试 schema 更新。
- [x] 实例格式升级为 v2，拒绝所有旧用户库布局，无 fallback/backfill。
- [x] 更新 reset 和数据探测；不执行任何本地数据库迁移。

验证：`go test ./internal/catalog ./internal/systemdb ./internal/config ./internal/app`。

### 阶段 B：本地单文件引擎

- [x] 新增 `filedb` 统一 Factory、DuckDB/SQLite driver、路径安全和 open guard。
- [x] 完成项目级 StorageTarget、Registry、缓存与关闭语义。
- [x] 用真实临时文件验证建表、读写、事务、重启持久化、损坏文件和并发 Acquire。
- [x] 用户库打开链路不再出现 DuckLake ATTACH/DATA_PATH。

验证：`go test ./internal/database/... ./internal/catalog`，并补充两种引擎的冷启动与并发测试。

### 阶段 C：默认数据库、方言与 KV

- [x] 实施默认数据库唯一约束、创建/删除生命周期和系统库保护。
- [x] 方言化 Schema、行读取、统计、gocloudagent 元数据查询。
- [x] KV 改用 `__sb_` 表并共享项目 Handle，完成 TTL 和事务测试。
- [x] sqlguard 增加内部命名空间和 SQLite 安全用例。

验证：`go test ./internal/api ./internal/database/... ./internal/sbcli`；覆盖第二库 409、删库清空 KV、系统库拒绝和实例只读。

### 阶段 D：S3 文件同步与项目租约

- [x] 新增项目 snapshot/manifest/local-state 与两种引擎的一致性快照。
- [x] 项目级 lease、失租关闭、缓存淘汰、版本保留、reset 扫描。
- [x] 删除旧用户库 descriptor/DATA_PATH/catalog-only 同步接线；系统 DuckLake 保留。
- [x] 测试冷启动、本地 ahead、checksum 错误、CAS 冲突、上传失败、失租禁止 Flush、删除与 in-flight 竞态。

验证：`go test ./internal/objectstore ./internal/database/... ./internal/app`，使用 fake BlobStore 做确定性测试，再执行已有的 S3 集成测试入口（若环境具备）。

### 阶段 E：配置、API 与调用方收尾

- [x] 更新配置、示例、项目 engine 契约、UI、Go/JS SDK、sbcli/gocloudagent 文档。
- [x] 删除无效的旧用户库 DuckLake 测试和代码，保留系统库专属测试。
- [x] 更新 `AGENTS.md`、数据库 README、架构图与公开文档。
- [x] 全量回归并检查无物理路径、DSN、SQL 参数进入日志/审计。

## 10. 关键测试矩阵

至少覆盖以下组合：

| 维度 | 用例 |
|---|---|
| 引擎 | DuckDB、SQLite |
| 存储 | local、S3 冷缓存、S3 热缓存 |
| 数据面 | SQL、Schema、集合文档、KV/TTL、gocloudagent 只读查询 |
| 并发 | 同项目多请求单 Handle、写与快照互斥、失租、重复创建默认库 |
| 生命周期 | 首次创建、重启、空闲关闭、删除后重建、shutdown、实例只读 |
| 安全 | 路径穿越、符号链接、伪造扩展名、损坏文件、`__sb_` 注入、系统库写/删 |
| 一致性 | WAL 未合并、快照失败、manifest CAS 冲突、本地 ahead、远端更新、缓存淘汰 |

最终验证命令：

```bash
go build ./internal/... ./cmd/...
go test ./...
(cd gocloudagent && GOWORK=off go test ./...)
cd ui && yarn build && yarn test
cd packages/js-sdk && npm run typecheck && npm test
```

## 11. 验收标准

1. 新建普通项目后，系统表只保存一个合法相对 `database_path`，公开 API 不返回该路径。
2. 一个项目的 SQL、集合和 KV 始终使用同一个主文件和同一个项目级 Handle；不存在第二个 per-database DuckLake catalog/DATA_PATH。
3. DuckDB 与 SQLite 都能完成本地写入、关闭、重开恢复；项目内不能创建第二个默认数据库。
4. S3 重启冷缓存后能按 manifest 恢复完整文件；WAL、上传中断、失租和 CAS 冲突不会发布损坏或覆盖新版本。
5. 删除默认数据库只清理该项目数据库资源，不影响项目 files；系统库仍不可写不可删。
6. 旧格式实例在迁移前被拒绝，无任何旧路径 fallback、自动 ALTER 或隐式数据转换。
7. 全量构建与测试通过，UI/SDK/gocloudagent 不依赖物理路径，日志与审计无路径、DSN 或敏感正文泄漏。

## 12. 实施状态

阶段 A 至 E 的代码与文档改造已完成，第 9 节清单已全部勾选。

已验证：

- `go build ./internal/... ./cmd/...`、`go vet` 无报错。
- `go test ./...` 通过，下面两项除外。
- `(cd gocloudagent && GOWORK=off go test ./...)` 通过。
- `internal/database/ducklake` 整包通过，耗时约 232 秒。
- `packages/js-sdk` 的 `npm run typecheck` 与 8 个用例通过。
- `ui` 的 `yarn build` 成功，`tests/docs-order.test.ts` 通过。
- 日志与审计代码中没有输出 `database_path`、DSN 的位置，API 响应不返回路径。

遗留项：

- `skills/simplify-code-eval-std/evals/files/20-go-minimal` 在 `go test ./...` 中构建失败。该目录未被本次改动触及，目录里只有 `input.go.txt`，缺少 `readName` 的实现，属于评测夹具本身的状态，未处理。
- `ui` 的 `vue-tsc` 在 `ThreadSwitcher.vue`、`AgentScheduleModal.vue`、`KvApiPanel.vue` 等文件报错，均不在本次改动范围内，未处理。
- `ui` 全量 `yarn test` 的汇总结果没有拿到，只确认了与本次改动直接相关的 `docs-order` 测试。
- 删除默认数据库与 in-flight 请求的竞态，目前由 `TestCloseDatabase_WithActiveReturnsError` 和 `Registry.Remove` 的等待逻辑覆盖，没有“并发写请求同时执行 DELETE”的端到端用例。
- 计划里提到的架构图没有单独更新。仓库中未找到描述用户库 DuckLake 布局的图。
- S3 集成测试入口依赖真实环境，本次未执行，S3 路径只用 fake BlobStore 做了确定性测试。
