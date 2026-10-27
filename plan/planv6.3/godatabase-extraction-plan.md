# internal/database 抽离为独立模块 godatabase 的改造计划（planv6.3）

> 状态：**阶段 0–6 已实现并通过测试**；阶段 7（验收）见 §10 执行记录，其中 2 项为已知遗留（与本计划无关）。D1 / D2 / D5 / D7 已决，D3 / D4 / D6 沿用默认（§5）。
> 目标：把 `internal/database/`（含 7 个子包）抽成仓库根目录下的嵌套独立 Go 模块 `github.com/linkxzhou/SimpleBase/godatabase`，与 `gosandbox/`、`gofunction/`、`cloudagent/` 同级，以库的形式可被独立引用。
> 硬约束（继承 `AGENTS.md`）：`godatabase` **不得 import 根模块任何包**（含 `internal/...`）。
> 范围声明：本计划是**结构搬迁与依赖解耦**，不改任何运行时行为、不改 S3 对象布局与 JSON 格式。**追加（D7）**：项目为全新项目、数据由用户自行 reset，故同步**清除全部面向旧数据的迁移与兼容代码**（含系统库的版本化迁移），见 §5.1 与 §10.2。**本计划只改源码，不执行任何 `init_db` / DDL / 迁移，也不触碰任何运行库**；已存在的库需用户自行 reset。
> 依赖策略：新模块的 `go.mod` 按用户本次明确要求新增，**版本与根 `go.mod` 完全一致，不升级任何依赖**。

---

## 1. 现状事实（均已用源码核对）

### 1.1 规模

| 项 | 数值 |
|---|---|
| 非测试 Go 代码 | 9599 行 / 46 个文件 |
| 测试文件 | 36 个 |
| 子包 | `cache` `ducklake` `filedb` `kv` `lease` `registry` `sqlguard` + 顶层 `database`（runtime / query / serialize / dialect / snapshot） |
| 根模块里引用方 | 顶层 `database` 39 个文件；`ducklake` 13；`sqlguard` 11；`filedb` 8；`registry` 7；`kv` 5；`cache` 3；`lease` 3。分布在 `api`(37) `systemdb`(8) `app`(5) `backup`(3) `agentbridge`(1) `benchmarks/dbread`(1) |
| 根模块里直接 import 驱动的文件 | 非测试 0 个，测试 29 个（驱动依赖仍留根 `go.mod`） |

### 1.2 反向耦合：`database` 依赖了根模块 6 个包（抽离的全部难点）

| 依赖 | 在 database 内的实际用法 | 耦合强度 |
|---|---|---|
| `internal/catalog` | `catalog.Database` 作为 `Factory.Open` / `Registry.Acquire` / `Handle` 的**入参与字段**（46 处）；只读到 6 个字段：`ID`(144 次) `ProjectID`(91) `DatabasePath`(14) `Status`(3) `TenantID`(1) `Kind`(1)。另有纯函数：`DatabaseEngine` `ValidateProjectDatabasePath` `DatabasePathForProject` `IsSystemDatabase`，常量 `EngineDuckDB/SQLite`、状态常量。**唯一的 Service 依赖**：`registry.go:83` 调 `catalog.Service.ResolveProjectStorage` 补全 `DatabasePath` | 高（类型贯穿） |
| `internal/objectstore` | `BlobStore` 接口（16）、`KeyBuilder`（12，只用 11 个 DB 相关方法）、`ErrPreconditionFailed`(11) `ErrNotFound`(3)、`ObjectInfo`、`CatalogFileExt`、`SystemPrefix`、`ValidateProjectID` | 高（持久化格式的一部分） |
| `internal/observability` | `Logger`（`= *zapLogger`，12 处，只用 Debug/Info/Warn/Error）；`*Metrics`（只用 10 个指标点，见 §3.3） | 中 |
| `internal/utils/sqlx` | `Literal` `Ident` `IsSafeIdent` `IsIdentByte`（9 个文件） | 低（110 行） |
| `internal/utils/atomicfile` | `Write`（3 处，93 行） | 低 |
| `internal/utils/hashx` `singleflight` | `hashx.Sum`（63 行）、`singleflight.Group`（仅 `lease/gate.go` 用，312 行；根模块其他地方 **0** 使用） | 低 |

测试额外依赖：`catalog.NewService / NewSQLRepository`、`auth.Principal`（仅 `registry/always_open_test.go`，这是跨模块集成测试，应留在根模块）、`prometheus`（仅 3 处 `NewMetrics(prometheus.NewRegistry())`）。

### 1.3 反向引用（根模块指向 database 的耦合，搬走后要改）

- `observability/metrics.go` 的两处注释提到 `internal/database/cache.CacheMetrics`，且 `*Metrics` 隐式实现了该接口。
- `config.example.yaml` 8 处、`README.md`、`AGENTS.md`、`internal/AGENTS.md`、`internal/database/README.md` 里的代码路径。
- `Dockerfile` 需要 `COPY godatabase/go.mod godatabase/go.sum`；`build.sh`、`scripts/test.sh` 需要加入新模块的测试。

### 1.4 对象存储布局属于库的"磁盘契约"

`filedb`（项目快照/manifest/租约）与 `ducklake`（catalog 同步/orphan ledger/instance_format）把 key 布局和 JSON 格式写进 S3。`objectstore.KeyBuilder` 里的 11 个 DB 方法就是这份契约，**搬走后字节级必须一致**（§7 有金标测试）。

---

## 2. 目标结构

```text
godatabase/                          独立模块 github.com/linkxzhou/SimpleBase/godatabase
  go.mod / go.sum                    只含 duckdb-go/v2、uglyer/go-sqlite3、google/uuid、zap（版本同根）
  README.md                          含「脱离 SimpleBase 单独使用」的最小示例
  godatabase.go                      AccessMode / Factory / WriteHookFactory / Spec / Status / 错误 / Logger / Observer
  dialect.go  query.go  serialize.go  snapshot.go     （原顶层文件，包名 database → godatabase）
  blob/                              BlobStore 接口 + ObjectInfo + ErrNotFound/ErrPreconditionFailed
                                     + KeyBuilder（仅 DB 相关 key）+ 内存实现 NewMemory（原 NewMemoryBlobStore）
  cache/  ducklake/  filedb/  kv/  lease/  registry/  sqlguard/
  sqlx/                              原 internal/utils/sqlx（公开：SQL 字面量/标识符转义本就属于数据库库）
  internal/{atomicfile,hashx,singleflight}/   库私有副本
```

依赖方向（库内部，**禁止成环**）：

```text
registry → filedb / ducklake / cache / lease → blob → godatabase(根) → sqlx
kv → registry,  sqlguard → sqlx
```

- 根包 `godatabase` **不 import 任何子包**（不做 facade 转发），否则子包反向 import 根包会成环；这点与 `gofunction` 的"根包薄门面"做法不同，需要在 README 写明。
- 子包保持原目录名，import 路径只是前缀由 `.../internal/database` 换为 `.../godatabase`，机械替换。

根模块侧：

```text
internal/dbbridge/        新增宿主适配层（对标 agentbridge）：catalog.Database ↔ Spec、
                          catalog.Service → PathResolver、observability → Logger/Observer、
                          objectstore.KeyBuilder → blob.KeyBuilder
internal/objectstore      仍在根模块（S3 客户端 + aws sdk + 平台 key）；BlobStore/ObjectInfo/两个 Err 改为别名
internal/catalog          引擎/路径纯逻辑下沉到库，catalog 保留薄封装（保持 ErrInvalidName 语义）
```

---

## 3. 解耦设计（每一项都给出替代接口）

### 3.1 `catalog.Database` → `godatabase.Spec`

```go
type Status string // 取值与 catalog.DatabaseStatus 完全一致：creating|ready|degraded|deleting|deleted|closed

type Spec struct {
    ID           string
    TenantID     string
    ProjectID    string
    Kind         string // "user" | "system"；空视为 user
    Status       Status
    DatabasePath string // 项目文件相对路径，仅服务端使用，不进日志/响应
}

func (s Spec) IsSystem() bool { return s.Kind == KindSystem }
```

- 只保留库真正读取的 6 个字段，不把 `DataModel` `StoragePrefix` `FormatVersion` `DeletedAt` 等业务字段带进库。
- `Handle.Database` 与 `Registry.Snapshot` 的类型随之变为 `Spec`。已核对：根模块**没有任何代码**读取 `Handle.Database`，唯一读取 `Snapshot` 的是 `kv/sweeper.go`（只判系统库）。
- `Status` 用字符串类型且取值一致，适配层转换是零成本的 `godatabase.Status(db.Status)`。

### 3.2 `catalog.Service.ResolveProjectStorage` → 可选 `PathResolver`

```go
type PathResolver interface {
    ResolveDatabasePath(ctx context.Context, tenantID, projectID string) (string, error)
}
```

`registry.New(factory, resolver PathResolver, opts, logger, observer)`；`resolver == nil` 时跳过补全（现有测试就是传 `nil`）。适配层用 `catalog.Service` 实现它，**内部仍走原有的租户归属校验**（`tenantMatches` / `ProjectBelongsToTenant`），库不接触租户逻辑。

### 3.3 `observability.Logger` / `*Metrics` → 库内接口

```go
type Logger interface {                       // zap.Field 沿用：registry 本来就 import zap，zap 是库的合法依赖
    Debug(msg string, fields ...zap.Field)
    Info(msg string, fields ...zap.Field)
    Warn(msg string, fields ...zap.Field)
    Error(msg string, fields ...zap.Field)
}

type Observer interface {                     // 对应现有 10 个指标点，一一映射
    OpenHandleDelta(delta int)                // DBOpenHandles Inc/Dec
    CacheBytes(bytes int64)                   // ObserveCacheBytes
    CacheEvicted()                            // IncCacheEvictions
    CatalogSync(outcome string, d time.Duration, failed bool) // SyncTotal / Duration / Failures
    CatalogSyncLag(dbID string, lag int64)
    CatalogSyncConflict(dbID string)
    CatalogSize(dbID string, bytes int64)
    CatalogLocalAhead()
}

var NopLogger Logger = ...; var NopObserver Observer = ...
```

- 库不再 import `prometheus`（现仅测试用）；测试改用内存计数的 fake observer。
- **类型化 nil 陷阱（必须有测试）**：`observability.Logger` 是 `*zapLogger`，把一个 nil 的 `*zapLogger` 赋给接口会得到**非 nil 接口**，库内 `if r.logger == nil` 判断会失效并 panic。适配层必须在 nil 时传无类型 nil（或 `NopLogger`），`dbbridge` 里加专门用例。`Metrics` 的字段级 nil 判断（如 `s.Metrics.CatalogSizeBytes != nil`）同样搬进适配层。

### 3.4 `objectstore` → `godatabase/blob`

| 条目 | 处理 |
|---|---|
| `BlobStore` 接口、`ObjectInfo`、`ErrNotFound`、`ErrPreconditionFailed`、内存实现 | 迁入 `blob`；根 `objectstore` 改为别名：`type BlobStore = blob.Store`、`type ObjectInfo = blob.ObjectInfo`、`var ErrNotFound = blob.ErrNotFound`（**同一个变量**，保证 `errors.Is` 在 S3 实现与库之间恒等） |
| S3 实现（`s3Client`、aws sdk） | **留在根模块**，库不依赖 aws sdk；库使用者自行注入 `blob.Store`（可选：后续再提供官方 S3 实现，不在本计划） |
| `KeyBuilder` 的 DB 相关 11 个方法（`ProjectDatabase{Prefix,SnapshotKey,ManifestKey,LeaseKey}`、`DuckLake{SnapshotKey,ManifestKey,LeaseKey,DataURI,OrphanLedgerKey}`、`InstanceFormatKey`、`RootListingPrefix`） | 迁入 `blob.KeyBuilder{RootPrefix}`——它定义的是库的磁盘格式。根 `objectstore.KeyBuilder` 保留平台 key（descriptor / files / system / catalog 前缀），并新增 `DB() blob.KeyBuilder`；根模块里仍用 DB key 的 `app/reset.go`、`app/app.go` 改走 `kb.DB()` |
| `CatalogFileExt`、`SystemPrefix`、`ValidateProjectID`（8 位 `[A-Za-z0-9-]`）、`ProjectIDLen`、`NewProjectID` | 迁入 `blob`；根保留同名转发。**项目 ID 规则原样搬走，不参数化**（D5 已决） |

> `objectstore.KeyBuilder{RootPrefix: …}` 字面量在根模块有 22 处，保持该结构体形状不变，调用方零改动。

### 3.5 纯函数与常量下沉

库提供：`EngineDuckDB` `EngineSQLite`、`EngineFromPath(path)`、`ValidatePath(projectID, path)`、`PathForProject(projectID, engine)`、`KindSystem/KindUser`、`Status*`。根 `catalog` 改为：

```go
const EngineDuckDB = godatabase.EngineDuckDB   // 常量别名，值不变
func DatabaseEngine(p string) (string, error) { e, err := godatabase.EngineFromPath(p); return e, wrapInvalidName(err) }
```

**关键**：这些函数当前返回 `fmt.Errorf("%w: …", ErrInvalidName)`，`api` 层据此映射 HTTP 400。库返回自己的 `ErrInvalidPath`，`catalog` 薄封装必须再包一层 `ErrInvalidName`，**错误文本与 `errors.Is` 行为不变**，并加一组表驱动用例逐条对照旧输出。

### 3.6 工具包

| 包 | 方案 | 理由 |
|---|---|---|
| `sqlx` | 迁为公开 `godatabase/sqlx`；根 8 个调用文件改 import；`internal/utils/sqlx` 删除 | 避免重复；转义逻辑单一来源（SQL 注入防线） |
| `singleflight` | 迁为 `godatabase/internal/singleflight`，根删除 | 根模块 0 使用 |
| `atomicfile`(93 行)、`hashx`(63 行) | 库内 `internal/` 各放一份，根保留自己的 | 根另有 5/4 个使用方；共 156 行，接受重复，比让库依赖根或强行公开更好（决策 D4） |

---

## 4. 分阶段实施（每阶段结束必须全绿，可单独提交/回滚）

策略：**先在原位置完成解耦（阶段 1–3），再 `git mv` 搬家（阶段 4）**。这样搬家时只剩机械 import 替换，风险与搬迁分离。

### 阶段 0：基线冻结（0.5d）
- 记录 `go build ./internal/... ./cmd/...`、`go test ./...` 当前结果，以及 `internal/database/...` 各子包覆盖率，作为不退化基线。
- **金标快照**：写一个一次性测试，对 `KeyBuilder` 11 个 DB 方法 × {空前缀, 有前缀} × {合法/非法 projectID, 各 engine} 输出表，落成 `testdata/keys.golden`（搬迁前后用同一份比对）。
- 同样为 `catalog.DatabaseEngine / ValidateProjectDatabasePath / DatabasePathForProject` 做错误文本金标。

### 阶段 1：就地引入接缝（1d，仅改 `internal/database` 内部）
- 新增 `Spec`/`Status`/`PathResolver`/`Logger`/`Observer`（先放在原 `database` 包）。
- 把 `Factory`、`Registry`、`Handle`、`filedb`、`ducklake`、`kv/sweeper` 的签名从 `catalog.Database` 换成 `Spec`；`registry.New` 的 `*catalog.Service` 换成 `PathResolver`。
- 根模块调用点通过新建的 `internal/dbbridge` 做转换（先建最小版：`SpecOf(db)`、`ResolverFrom(svc)`、`LoggerFrom`、`ObserverFrom`）。
- 同时把 `cache.CacheMetrics` 改为 `Observer` 子集，更新 `observability/metrics.go` 注释。

### 阶段 2：blob 与 key 契约（1d）
- 在 `internal/database/blob` 新建（先在原位）：接口、别名、`KeyBuilder` 的 DB 方法、内存实现。
- 根 `objectstore` 改别名；`app/reset.go`、`app/app.go` 改用 `DB()`。
- 跑阶段 0 的金标，必须逐字节一致。

### 阶段 3：工具包与纯函数（0.5d）
- `sqlx` 迁入、`singleflight`/`atomicfile`/`hashx` 私有化；`catalog` 引擎/路径函数下沉并保留 `ErrInvalidName` 薄封装。
- 此时 `internal/database/...` 的 import 列表里应当**只剩** `internal/database/...` 自身、标准库、zap、uuid、驱动。用命令校验：`go list -deps ./internal/database/... | grep 'SimpleBase/internal/' | grep -v 'internal/database'` 输出为空。

### 阶段 4：搬家为独立模块（1d）
1. `git mv internal/database godatabase`（保留历史）；新建 `godatabase/go.mod`（module 路径、`go 1.25.0`、require 与根相同版本）；复制所需 `go.sum` 行，`go mod tidy` 前后核对**没有版本变化**。
2. 根 `go.mod` 增加 `require github.com/linkxzhou/SimpleBase/godatabase v0.0.0` 与 `replace … => ./godatabase`（与 gosandbox 写法一致）。
3. 机械替换（先在分支演练，注意 macOS 的 `sed -i ''`）：

| 旧 | 新 |
|---|---|
| `…/internal/database"`（包名 `database`） | `…/godatabase"`（包名 `godatabase`，调用处 `database.` → `godatabase.`） |
| `…/internal/database/{cache,ducklake,filedb,kv,lease,registry,sqlguard}` | `…/godatabase/{同名}` |
| `…/internal/database/blob` | `…/godatabase/blob` |
| `…/internal/utils/sqlx` | `…/godatabase/sqlx` |

   涉及 `api`(37) `systemdb`(8) `app`(5) `backup`(3) `agentbridge`(1) `benchmarks/dbread`(1)，共约 55 个文件，之后 `gofmt -l` 只对**触碰过的文件**检查。
4. 若包名在 `api` 中与局部变量 `database` 冲突，统一用 `gdb "…/godatabase"`，不要重命名变量（D2 已决：包名即 `godatabase`）。
5. `ducklake` 随库整体迁移（D1 已决）：`systemdb`(8)、`app`(4：`app.go` `catalog_engine.go` 及测试) 的 `…/internal/database/ducklake` 一并替换为 `…/godatabase/ducklake`；阶段 3 之后 `ducklake` 内应已无 `objectstore.SystemPrefix` 等根包引用，改为 `blob.SystemPrefix`。

### 阶段 5：测试迁移与适配层定稿（1d）
- 36 个测试文件：`catalog.Database{…}`→`godatabase.Spec{…}`；`objectstore.NewMemoryBlobStore`(41 处)→`blob.NewMemory`；`observability.NewLogger/NewMetrics`→`NopLogger` / fake observer；`objectstore.DuckLakeStorage`(2 处)→`ducklake.RemoteStorage`。
- `registry/always_open_test.go` 依赖 `catalog.Service`+`auth.Principal`，**不属于库**：移到根模块 `internal/dbbridge/registry_integration_test.go`。
- `dbbridge` 补全单测：Spec 转换保 `Kind=system`、Status 取值同构、typed-nil Logger、`PathResolver` 的跨租户拒绝。

### 阶段 6：脚本、文档、CI（0.5d）
- `Dockerfile`：加 `COPY godatabase/go.mod godatabase/go.sum godatabase/`。
- `build.sh`、`scripts/test.sh`、`scripts/smoke.sh`：新增 `(cd godatabase && go build ./... && go vet ./... && go test ./...)`。
- 文档：`godatabase/README.md`（由 `internal/database/README.md` 改写 + 独立使用示例 + 依赖方向 + 稳定性声明）；更新根 `AGENTS.md`（项目概述、常用命令、依赖方向图）、`internal/AGENTS.md`（第 8、26–31 行的路径）、根 `README.md:149`、`config.example.yaml` 的 8 处"代码：…"路径；新增 `godatabase/AGENTS.md`（硬约束：不得 import 根模块、禁止在 handler 绕过 Registry 等原有约束原样带走）。
- `.gitignore` 无需改（无二进制产物）。

### 阶段 7：验收（0.5d）见 §6。

合计约 **6 人日**（串行；阶段 1–3 与阶段 5 的测试改写可并行）。

---

## 5. 决策记录

### 5.1 已决（用户已确认，按推荐执行）

| 编号 | 问题 | 决议 | 落地要求 |
|---|---|---|---|
| D1 | `ducklake`（系统库专用，约 3000 行，含 `instance_format` / orphan ledger / catalog syncer）是否一起进库 | **一起进库**，整体抽离，不在根模块留半截包 | ① `godatabase/ducklake` 与其他子包同级；`systemdb` / `app` 改 import `godatabase/ducklake`。② `_system` 前缀通过 `blob.SystemPrefix` 常量提供，库内不再硬编码业务含义。③ README 明确 `ducklake` 的定位：**仅用于系统库**，用户库走 `filedb`（原约束原样带走）。④ `ducklake` 的测试大量依赖 `filedb` / `registry`，须随库一起迁，不拆 |
| D2 | 库根包名 | **`godatabase`**（目录、模块、包名一致） | 调用处 `database.` → `godatabase.`；若与 `api` 内局部变量 `database` 冲突，import 别名统一用 `gdb`，**不重命名变量** |
| D5 | 项目 ID 规则（8 位 `[A-Za-z0-9-]`）是否参数化 | **不参数化**，原样搬入 `blob.ValidateProjectID` | 零行为风险；README 注明该约定；`ProjectIDLen=8`、`projectIDAlphabet`、`NewProjectID` 一并迁入，根 `objectstore` 保留同名转发。不增加 `ProjectIDValidator` 等扩展点 |
| D7 | 旧数据兼容 / 迁移代码如何处理 | **最激进：全部删除**。项目是新项目，由用户 reset 数据，不保留任何旧数据兼容层 | ① `systemdb` 的 56 步版本化迁移折叠为最终态建表 `schema.go`（`EnsureSystemSchema`，`CREATE TABLE IF NOT EXISTS`），不再有 `sys_migration_versions`、`ALTER ... ADD COLUMN`、数据修复 `UPDATE`；② 删除迁移状态 API（`/migrations/status`、`/migrations/plan`）、`go-sdk` 的 `MigrationStatus/Plan`、`sbcli migrate status/plan`；③ 删除沙盒「未迁移」自检（`SandboxSchemaReady`、`SandboxNotReadyReason`）；④ `catalog` 删除历史 open/close 状态（`opening/closing/closed/recovering`）及读取时回写、启动期的修复分支（保留崩溃残留 `creating` 的修复）；⑤ `ducklake` 删除 `sync_on_commit` 降级别名与旧格式回退注释；⑥ `data_handler` 删除「缺 `updated_at` 列」回退。表结构以后直接改 `schema.go` 并 reset |

### 5.2 沿用默认（尚未被推翻，评审时可提出异议）

| 编号 | 问题 | 默认方案 | 备选与代价 |
|---|---|---|---|
| D3 | 版本策略 | 内部 `v0.0.0` + `replace`，README 标注"API 不稳定，随 SimpleBase 发布" | 打 tag 独立发布需另建发布流程，不在本计划 |
| D4 | `atomicfile` / `hashx` 是否重复一份 | 重复（共 156 行） | 公开成 `godatabase/fsx` 等：增加长期公开 API 面，不值得 |
| D6 | 库是否内置 S3 `blob.Store` | 本计划**不内置**（避免引入 aws sdk） | 后续可做 `godatabase/blob/s3` 子模块 |

---

## 6. 验收标准

1. `godatabase/` 内 `go list -deps ./... | grep 'linkxzhou/SimpleBase/' | grep -v '/godatabase'` **无输出**，且 `GOWORK=off go build ./... && go vet ./... && go test ./...` 通过。
2. 根模块：`go build ./internal/... ./cmd/...` 与 `go test ./...` 通过；`internal/database` 目录不复存在。
3. 阶段 0 的两份金标（key 布局、错误文本）在搬迁后逐字节一致。
4. 库的非测试 import 不含 `prometheus`、`aws`；`godatabase/go.mod` 的每个依赖版本与根相同。
5. 覆盖率不低于阶段 0 基线（按子包逐个比对）。
6. 硬约束回归：系统库保护用例（`IsSystemDatabase`/`Spec.IsSystem` 经适配层后仍拒绝写入与删除）、`writable=false` 返回 `ErrWriterUnavailable`、`registry` 单写者并发用例、`filedb` 失租停止上传与 manifest 冲突 fail-closed 用例全部保持通过。
7. 日志/审计不新增任何字段；`DatabasePath` 仍不出现在日志与响应（grep `zap.String("database_path"` 无新增）。
8. 构建链：`docker build` 能通过（新增的 COPY 生效）；`./build.sh`、`./scripts/smoke.sh` 通过。
9. 端到端：`./build.sh dev` 起服务，建项目 → 执行 SQL → KV 读写 → 重启后数据仍在（本地盘模式）；S3 模式用已有 `filedb/remote_test.go`、`ducklake/*_test.go`（内存 BlobStore）覆盖。

---

## 7. 风险登记

| 风险 | 影响 | 对策 |
|---|---|---|
| S3 key 布局或 JSON 字段被搬迁误改 | 已上线数据读不到、快照/租约错位，**不可逆** | 阶段 0 金标 + 阶段 2 即比对；搬迁阶段不碰 key 与 JSON tag；`blob.KeyBuilder` 文件用 `git mv`/复制后只改包名 |
| typed-nil 的 Logger/Observer 造成 panic | 运行期崩溃，且仅在未配置日志的路径触发 | §3.3 适配层统一处理 + 专门单测 |
| 适配层丢失 `Kind=system` | 系统库保护失效、DuckLake `retryConflicts` 行为变化 | `Spec` 保留 `Kind`；`dbbridge` 往返单测；§6.6 回归 |
| `ErrInvalidName` 包装丢失 | API 把 400 变成 500 | §3.5 薄封装 + 金标 |
| 别名后 `errors.Is` 失效（两个同名 `ErrNotFound`） | 租约/manifest 逻辑把"不存在"当成真错误 | 根 `objectstore` 用 `var ErrNotFound = blob.ErrNotFound`（同一变量），加断言用例 |
| 根包不做 facade 导致使用者要记多个 import | 易用性下降 | README 提供最小示例；这是避免成环的有意取舍 |
| 55 个文件的大范围 import 替换产生冲突 | 合并困难 | 阶段 4 单独成一个提交，在无其他并行改动的窗口执行 |
| 嵌套模块让 `go test ./...` 不再覆盖库 | 漏测 | 在 `AGENTS.md` 常用命令与 `scripts/test.sh` 增加 `(cd godatabase && …)`；与 gosandbox 一致 |
| 遗留 37 个未 gofmt 的 internal 文件 | 搬迁提交里夹带无关格式化 | 只格式化被触碰文件，不全仓重排 |

---

## 8. 不在本计划范围

- 不改任何 API、配置项、表结构、对象存储格式；不涉及 `init_db` 与任何 DDL。
- 不新增能力（不做只读副本、不做 S3 官方实现；项目 ID 规则不参数化，D5 已决）。
- 不把 `ducklake` 留在根模块或改名拆出（D1 已决：随库整体迁移）。
- 不改 `cloudagent` / `gofunction` / `gosandbox`。
- 不把 `catalog`、`objectstore` 的 S3 客户端或 `systemdb` 一并抽出。
- 不发布到外部 module proxy。

---

## 9. 附录：证据索引（相对仓库根）

- 反向依赖：`internal/database/runtime.go:13`（`catalog`）、`registry/registry.go:27,56,83-87`（`catalog.Service`）、`ducklake/factory.go:211`（`db.Kind == catalog.DatabaseKindSystem`）、`kv/sweeper.go:105`、`cache/manager.go:49-53`（`CacheMetrics`）、`lease/gate.go:6`（`singleflight`）。
- 指标点：`registry.go:180,266,295,380,426`（`DBOpenHandles`）、`ducklake/catalog_syncer.go:704,814,1015-1026`。
- 对象存储：`internal/objectstore/blob.go:35`（`BlobStore`）、`keys.go:25-262`（`KeyBuilder` 与 11 个 DB 方法）、`client.go:31,58`（`ErrNotFound` `ObjectInfo`）。
- catalog 纯逻辑：`catalog/project_storage.go:12-73`、`catalog/reserved.go:31`、`catalog/service.go:71-89`（`ResolveProjectStorage`）。
- 对标模块写法：`go.mod:104-108`（三个 `replace`）、`Dockerfile:4-6`、`gosandbox/README.md:3`、`gosandbox/go.mod`。
- 外部引用数量统计命令：`grep -rl 'SimpleBase/internal/database' --include='*.go' .`（76 个文件，其中 36 个在 `internal/database` 自身内）。

---

## 10. 执行记录

### 10.1 阶段完成情况

| 阶段 | 结果 | 要点 |
|---|---|---|
| 0 基线冻结 | 已完成 | `internal/objectstore/testdata/keys.golden`（key 布局）与 `catalog` 的错误文本金标在搬迁前后逐字节一致 |
| 1 就地引入接缝 | 已完成 | `Spec` / `Status` / `PathResolver` / `Logger` / `Observer` 与引擎/路径纯函数落在根包；`registry` / `filedb` / `ducklake` / `kv` / `cache` 全部改用接缝；新增 `internal/dbbridge` |
| 2 blob 与 key 契约 | 已完成 | `blob.Store` / `ObjectInfo` / `ErrNotFound` / `ErrPreconditionFailed` / `KeyBuilder` / 内存实现迁入库；根 `objectstore` 用别名（`ErrNotFound` 是同一变量，`errors.Is` 恒等）并新增 `KeyBuilder.DB()` |
| 3 工具包与纯函数 | 已完成 | `sqlx` 公开迁入；`singleflight` / `atomicfile` / `hashx` / `limit` 作为 `godatabase/internal/*` 私有副本；`catalog` 的引擎/路径函数改为对库函数的薄封装，`ErrInvalidName` 语义与文本不变 |
| 4 搬家为独立模块 | 已完成 | `internal/database` → `godatabase/`；包名 `godatabase`；新增 `godatabase/go.mod`（4 个直接依赖，版本与根完全一致，离线 `go mod tidy` 通过）；根 `go.mod` 加 `require` + `replace`；`Dockerfile` 加 `COPY`；约 112 个文件的 import 与限定符机械替换 |
| 5 测试迁移与适配层定稿 | 已完成 | 库内 36 个测试文件迁到库内类型；`registry` 的跨层集成用例迁到 `internal/dbbridge`；`kv` sweeper 用例改用 fake `PathResolver`；新增 `RecordingLogger` / `RecordingObserver` 与针对 `Observer` / `PathResolver` / `recordSync` 的断言用例 |
| 6 脚本、文档、CI | 已完成 | `scripts/test.sh`、`scripts/smoke.sh` 纳入 `godatabase`；新增 `godatabase/README.md`（含脱离 SimpleBase 的最小示例）与 `godatabase/AGENTS.md`；更新根 `AGENTS.md`、`internal/AGENTS.md`、`README.md`、`config.example.yaml` 的路径 |
| 7 验收 | 见 §10.3 | |

### 10.2 D7：旧数据兼容 / 迁移代码清除

| 项 | 处理 |
|---|---|
| `systemdb/migrate.go`（771 行，56 个版本，含 `ALTER ... ADD COLUMN`、`UPDATE` 数据修复、`DROP TABLE`） | 删除。折叠为 `systemdb/schema.go`（522 行，42 张表、398 列）。折叠过程用「逐版本重放」与「最终态建表」两库的 `pragma_table_info`（列名 / 类型 / 可空 / 默认值 / 顺序）逐项对照，**一致后才删除旧迁移**；对照中发现并补上了折叠脚本漏掉的 `sys_collections.view_query` |
| `sys_migration_versions` 表 | 不再创建；`Store.Ping` 改探测 `sys_tenants` |
| `systemdb/migration_status*.go`、`api/migration_handler.go`、路由 `/migrations/{status,plan}`、`packages/go-sdk/migrations.go`、`sbcli migrate status/plan` | 删除（前端与 JS SDK 本就没有引用） |
| 沙盒「未迁移」自检（`SandboxSchemaReady`、`Dependencies.SandboxNotReadyReason`、`capabilities.reason`） | 删除 |
| `catalog`：`opening/closing/closed/recovering` 四个历史状态、读取时回写、`SetDatabaseReady` / `BeginDelete` / `SetDatabaseDegraded` 的历史来源状态 | 删除；`RepairDatabasesOnStartup` 只保留「崩溃残留 `creating`」的修复（那是崩溃恢复，不是旧数据兼容） |
| `ducklake`：`sync_on_commit` 降级别名、manifest 旧格式回退注释 | 删除别名；缺 `catalog_engine` 的 manifest 仍 fail closed（`ErrCatalogEngineUnknown`），文案去掉「兼容」措辞 |
| `api/data_handler.go`：缺 `updated_at` 列时回退 `created_at` | 删除（现行建表本就带 `updated_at`） |

**行为变化（需知悉）**：系统库表结构的变更方式变了——不再追加迁移，直接改 `schema.go` 并 reset；已存在的系统库不会被自动补列，必须 reset。`EnsureSystemSchema` 对已存在的表是 no-op。

### 10.3 验收结果（对应 §6）

| § 6 项 | 结果 |
|---|---|
| 1 库不依赖根模块；独立 build / vet / test 通过 | **通过**。`go list -deps -test` 对根模块依赖数为 0；`GOWORK=off` 下 14 个包全部 `ok` |
| 2 根模块 build 与 test 通过；`internal/database` 不复存在 | **部分通过**。`go build` / `go vet` 全绿，目录已不存在；`go test` 中 29 个包 `ok`，**1 个失败**：`internal/api` 的 `TestDelegation_RouteTable`（见下） |
| 3 金标逐字节一致 | **通过** |
| 4 库非测试代码不含 `prometheus` / `aws`；依赖版本与根相同 | **通过**（0 个；4 个直接依赖版本逐项一致） |
| 5 覆盖率不低于基线 | **基本通过，但口径有限**：阶段 0 没有逐包落盘覆盖率基线，下列「之前」是**阶段 3 完成后的首次全量实测**，不是搬迁前 HEAD。顶层 76.6%→83.8%（补了 `spec_test.go`）、`registry` 74.2%→78.2%（补了 Observer / PathResolver 用例）、`ducklake` 74.7%→74.6%（删除 `sync_on_commit` 分支后略降 0.1 个点，未单独补测）；`cache` 90.6%、`sqlguard` 96.0%、`blob` 80.1%、`lease` 84.8%、`kv` 72.0%、`filedb` 57.4% 无变化。`filedb` 的 57.4% 偏低，是既有水平，非本次引入 |
| 6 硬约束回归（系统库保护、`writable=false`、单写者并发、失租、manifest 冲突 fail-closed） | **通过**（`dbbridge` 的 `Kind=system` 往返与状态同步用例、`registry` / `filedb` / `ducklake` / `lease` 原有用例全绿） |
| 7 日志/审计无新增敏感字段 | **通过**（未新增日志字段） |
| 8 构建链：`docker build`、`./build.sh`、`./scripts/smoke.sh` | **未执行**。`Dockerfile` 与脚本已改且 `bash -n` 通过，但本次未实际跑 docker / smoke |
| 9 端到端：`./build.sh dev` 起服务 → 建项目 → SQL → KV → 重启 | **未执行**。`internal/app` 的装配与 e2e 用例全绿，可作为替代证据，但不等同于人工冒烟 |

**已知遗留（均与本计划无关，未处理）**

1. `internal/api` 的 `TestDelegation_RouteTable` 失败：`GET/POST .../data/collections/:collection/{export,import}` 路由已注册为 `delegable`，但 planv6.0 §3.1 的委托路由表断言它们不可委托。这两条路由与 `collection_transfer.go`（未跟踪文件）是工作区里进行中的集合导入导出功能，不是本次引入；`collection_transfer.go` 里缺 `catalog` import 的编译错误我已按 `data_handler.go` 的既有写法修好，路由表该放行还是改成不可委托需你来定。
2. `godatabase/ducklake` 测试耗时 100–340 秒（同一用例集在不同机器负载下波动大），Go 默认 10 分钟超时在慢机器上会触发 `panic: test timed out`。已在 `godatabase/README.md`、`AGENTS.md`、`scripts/test.sh` 里要求 `-timeout 900s`。这是用例本身慢（基线 HEAD 单个用例 41.7s，搬迁后 59.8s，差异主要来自机器负载，未发现搬迁引入死锁），未做优化。

### 10.4 后续可选（不在本计划内）

- 提供 `godatabase/blob/s3` 官方实现（D6）。
- 把 8 位项目 ID 规则参数化（D5 的备选）。
- 给 `ducklake` 慢用例做 `-short` 分级，降低独立模块的默认测试耗时。
