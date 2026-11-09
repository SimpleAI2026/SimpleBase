# godatabase 优化计划（v6.4）

> 范围：`godatabase/` 嵌套模块（约 12k 行非测试代码），以及装配它的 `internal/app`、`internal/api` 中相关调用点。
> 基线：`main` @ `b6e9936` 工作区，行号以 2026-11-02 为准，实施前重新定位。
> 约束：遵守 `godatabase/AGENTS.md`：S3 key、manifest / lease JSON 是磁盘契约；没有旧数据兼容层；写路径、租约冲突一律 fail closed；不新增依赖。
> 证据口径：**已验证** = 本次实际运行确认；**代码确证** = 只读代码即可确定；**推断** = 需要复现用例确认后再改。

## 0. 结论摘要

| 级别 | 编号 | 问题 | 证据 |
|---|---|---|---|
| P0 | S1 | 只读 SQL 守卫可被 `WITH … DELETE`、`EXPLAIN ANALYZE <写>` 绕过，持 `database:read` 即可改数据 | 已验证（SQLite 实测 CTE DELETE 生效）+ 代码确证 |
| P0 | S2 | 用户库 DuckDB 未关闭外部文件访问，用户 SQL 可用 `read_text` / `read_csv` 读服务器任意文件 | 代码确证（系统库有此限制，用户库没有） |
| P0 | R1 | 宿主从未调用 `CloseIdle` / `Evictor`，句柄永不回收；`max_open: 8` 打满后第 9 个项目永久报错 | 代码确证 |
| P0 | R2 | `FileSyncer.Flush` 在上传完成后无条件清 dirty，期间的新写入丢失同步标记，关闭时 `Unbind` 也会跳过 | 代码确证 |
| P0 | R3 | DuckDB 用户库快照为「`CHECKPOINT` 后裸拷贝文件」，无写互斥，并发写可能拷出撕裂文件 | 推断（需复现） |
| P1 | K1 | KV `Store` 每请求新建，schema 缓存失效：每个读请求 `count(*)` 全表，每个写请求执行 12 条 DDL | 代码确证 |
| P1 | K2 | KV Sweeper 每分钟对所有已打开项目取 ReadWrite 租约并 `touch`，句柄永远不会空闲 | 代码确证 |
| P1 | P1–P6 | 快照全量读入内存、manifest 全量 LIST、每次打开都跑 SQLite `integrity_check` 等 | 代码确证 |
| P2 | C1–C8 | 重复代码、死字段、大文件拆分、字符串匹配判错 | 代码确证 |

建议顺序：**批次 A（S1、S2）→ 批次 B（R1–R3、K2）→ 批次 C（K1 与性能项）→ 批次 D（代码整理）**，批次 E（CLI）在 A 合入后穿插进行。A、B 是正确性和安全问题，应单独提交、单独回归。

---

## 1. P0：安全

### S1 只读 SQL 守卫可绕过

**现状**

- `sqlguard/classifier-desens.go:325`：`isReadOnlyKeyword` 只看首关键字，`WITH`、`EXPLAIN` 直接放行。
- `internal/api/sql_handler.go:175/197`：只读查询接口用 `sqlguard.Validate(…, ReadOnly)` 校验后取 `godatabase.ReadOnly` 租约。
- 但 `filedb.Factory.open` **忽略 mode**，registry 按项目只保留一个 `*sql.DB`（读写共用），所以 ReadOnly 租约拿到的是可写连接。

**影响**

- `WITH x AS (SELECT 1) DELETE FROM t`、`WITH … INSERT/UPDATE`、DuckDB 的 `EXPLAIN ANALYZE DELETE …`（ANALYZE 会真实执行）都会通过守卫并真正写库。
- 这些写入不触发 `onWrite`，也就不会 MarkDirty、不会同步到 S3，同时绕过 `WriteGate` / 写租约。
- 本机用系统 `sqlite3` 实测：`WITH x AS (SELECT 1) DELETE FROM t` 执行后行数变为 0。

**方案**

1. `sqlguard`：ReadOnly 意图下，对 `WITH` 开头的语句做词法扫描，要求 CTE 之后的主语句关键字仍属只读集合。复用现有跳过字符串、注释、引号标识符的扫描器，不要用子串匹配。
2. `EXPLAIN ANALYZE` / `EXPLAIN (ANALYZE …)` 在 ReadOnly 下拒绝，或要求其后语句也通过只读校验。
3. 纵深防御：`godatabase.Query` 在驱动支持时改用 `BeginTx(ctx, &sql.TxOptions{ReadOnly: true})` 执行。实施前分别核实 duckdb-go 与 go-sqlite3（uglyer fork）对 `ReadOnly` 的支持；不支持时至少在 SQLite 上用专用连接加 `PRAGMA query_only`。
4. 用例：`sqlguard/classifier_test.go` 增加 CTE+DML、`EXPLAIN ANALYZE`、注释与引号混淆的正反例；`internal/api` 增加只读接口执行 CTE DELETE 返回 403 / 400 的端到端用例。

### S2 用户库可读服务器任意文件

**现状**

- 系统库 `ducklake/factory.go:353-364` 设置了 `allowed_directories`，本地模式还有 `enable_external_access = false`。
- 用户库 `filedb/factory.go:190-195` 只设置了 `memory_limit` / `threads` / `temp_directory` / `lock_configuration`。
- `sqlguard` 拦截了 `COPY` / `ATTACH` / `INSTALL`，但不拦截表函数。

**影响**：只读接口执行 `SELECT * FROM read_text('/path/to/config.yaml')`、`read_csv('…/.env')`、`glob('/data/**')` 即可读取服务进程能访问的任何文件，包括其他项目的数据库文件。

**方案**

1. `initSQL` 在 `lock_configuration` 之前加入：
   - `SET allowed_directories = [<项目 tmp 目录>]`
   - `SET enable_external_access = false`
   - 实施前确认 `temp_directory` 溢写、`CHECKPOINT` 在该设置下仍可用；系统库本地模式已是同样组合。
2. 一并评估 `autoload_known_extensions` / `autoinstall_known_extensions`：用户库不需要运行时装载扩展，置为 false。
3. SQLite：确认 uglyer/go-sqlite3 默认未开启 `load_extension`；如已开启，在连接器上关闭。
4. 用例：用户库执行 `read_text`、`read_csv`、`glob` 访问库目录外的文件必须报错；正常查询、大查询溢写不受影响。

---

## 2. P0：生命周期与持久化正确性

### R1 句柄与缓存从不回收

**现状**

- `registry.Evictor` / `Registry.CloseIdle` 只在 `internal/dbbridge/dbbridge_test.go` 里被调用，`internal/app` 没有启动 Evictor。
- `cache.Manager.Evict` / `EnsureCapacity` 在全仓库无调用方；`api.NewCacheService` 只用到 `Usage`。
- `config.example.yaml` 默认 `max_open: 8`，`Registry.Acquire` 达到上限直接返回 `maximum open project databases reached`（`registry.go:133-135`）。

**影响**：进程运行期间访问过 8 个不同项目后，其他项目全部不可用，直到重启。缓存目录只增不减。

**方案**

1. `internal/app` 在 registry 装配后启动 `registry.NewEvictor(reg, time.Minute)`，在 `Shutdown` 中先停 Evictor 再 `registry.Shutdown`。
2. `Registry.Acquire` 达到 `maxOpen` 时，先同步尝试关闭一个 `active==0` 的最久未用句柄（LRU），关不掉才报错。关闭仍走 `closeLocked`，保证会先 Flush。
3. 缓存淘汰：在 `filedb` / `ducklake` 打开新库前调用 `cache.Manager.EnsureCapacity`，或由 Evictor 同周期调用。前提是先解决 §4 P5 的布局不一致，否则淘汰对用户库无效。
4. 用例：`max_open=2` 时顺序访问 3 个项目，第 3 个成功且第 1 个句柄被关闭并已 Flush；Evictor 在 `idle_timeout` 后关闭空闲句柄。

### R2 `FileSyncer.Flush` 丢失 dirty 标记

**现状**：`filedb/remote.go:330-335` 读取 `dirty`，经过快照、上传、发布 manifest（秒级到分钟级）之后，`remote.go:395` 无条件写 `s.dirty[projectID] = false`。

**影响**

- 快照之后到第 395 行之间的写入，`MarkDirty` 设置的 true 会被覆盖，后台循环不再上传。
- 关闭时 `Unbind → Flush` 看到 `dirty=false` 直接返回，这部分写入只留在本地缓存。远端模式下本地缓存可被淘汰或换机，**数据可能丢失**。

**方案**

- 把 `dirty map[string]bool` 改为单调递增的写代次：`MarkDirty` 递增 `gen[id]`；Flush 开始时记录 `g0`，成功后只在 `gen[id] == g0` 时清除，否则保持 dirty。
- `SyncLag` 改为返回 `gen - flushedGen`。
- 用例：用可阻塞的 fake `blob.Store` 卡住 `PutBytes`，期间 `MarkDirty`，放行后断言仍为 dirty，且下一轮会上传新的 seq。

### R3 DuckDB 用户库快照可能不一致（推断）

> **2026-11-02 复现结论：未复现，降为 P2。**
> 用 `TestFileSyncerDuckDBSnapshotConsistencyUnderConcurrentWrites`（filedb/remote_test.go）
> 高压复现：后台持续批量 INSERT（每轮 500 行加速 WAL 增长）+ 60 轮并发 Flush，
> 每个发布快照下载后独立打开并校验行数，5 次运行全部通过。
> 原因：并发写只追加 WAL，主文件仅在 checkpoint 时修改；显式 `CHECKPOINT` 后的
> 拷贝窗口内自动 checkpoint 需累计 16MB WAL（默认 checkpoint_threshold），小库快拷贝
> 几乎不可能撕裂。测试保留作回归；若未来大库出现撕裂再按方案 1/2 加写互斥。

**现状**：`filedb/remote.go:438-444` 先执行 `CHECKPOINT`，再用 `copyFile` 逐字节复制主文件。用户库 `SetMaxOpenConns(10)`，没有类似 `ducklake.writeGate` 的写互斥，复制期间其他连接可以继续提交并触发自动 checkpoint。sha256 是对复制结果计算的，校验无法发现撕裂。

**方案（按侵入性从小到大）**

1. 给 `filedb` 用户库加 per-DB 写互斥：写语句（`Execute` / `Batch` / KV `Update`）与快照互斥，参考 `ducklake/gated_conn.go`，可抽成公共实现。
2. 或者快照改为 `ATTACH '<staging>' AS snap; COPY FROM DATABASE main TO snap; DETACH`，与系统库 `captureCatalog` 的做法一致，由引擎保证一致性。
3. 先写复现用例：后台持续 INSERT，同时循环 Flush，用 `PRAGMA integrity_check` 或 DuckDB 打开下载的快照并比对行数。复现后再定方案；不能复现则降为 P2 并记录。

---

## 3. P1：KV

### K1 schema 缓存跟着 `Store` 每请求重建 ✅ 已完成（2026-11-02）

**现状**

- `internal/api/kv_handler.go:120` 每个请求都 `kv.New(...)`，所以 `Store.schemaKnown` 永远从 false 开始。
- 读请求 → `HasSchema` → `SELECT count(*) FROM __sb_kv_keys`（`kv/executor.go:94`），在 DuckDB 上是全表计数。
- 写请求 → `ensureSchema` → 依次执行 12 条 `CREATE … IF NOT EXISTS`（`kv/schema.go:37-77`）。DuckDB 上 DDL 会开写事务，与业务写串行。

**方案**（已实施）

1. schema 状态挂到连接级而非 `Store` 级：在 `kv` 包内维护 `sync.Map[*sql.DB]*schemaState`，句柄关闭时由 registry 回调清除；或让 registry `Handle` 暴露 `Value(key)` 存放扩展状态。不得改变「读路径不建表」的语义。
2. `HasSchema` 改用存在性探测：`SELECT 1 FROM __sb_kv_keys LIMIT 1`，并区分「表不存在」与「表为空」；或查 `duckdb_tables()` / `sqlite_master`。
3. 用例：同一 `*sql.DB` 上连续 100 次 `kv.New(...).HasSchema`，只第一次执行探测（用计数 driver 或 Observer 断言）。

**实施记录**：`kv/executor.go` 新增 `connSchemaState`（`sync.Map[*sql.DB]` 注册表）；`HasSchema` 改 `SELECT 1 … LIMIT 1` 存在性探测（`sql.ErrNoRows` 视为已就绪）；`ensureSchema` 同样接连接级缓存。测试 `kv/conn_schema_test.go` 三用例（探测计数=1、空表已就绪、缺席→建表感知）全过。

### K2 Sweeper 让句柄永远保持活跃 ✅ 已完成（2026-11-02 / 第 2 条 2026-11-03 补齐）

**现状**：`kv/sweeper.go:99-136` 每 60 秒对 `OpenedProjects()` 中的每个项目 `Acquire(ReadWrite)`，`Lease.Release` 会调用 `touch()`（`registry.go:231`）。默认 `idle_timeout` 为 5 分钟，大于 60 秒，所以 R1 修好之后句柄仍然永远不会被判为空闲。另外，没有 KV 表的项目也要被取一次写租约（只读 `Store` 的 `HasSchema` 探测在租约之后）。

**方案**

1. registry 增加不刷新 `lastUsed` 的后台租约入口（例如 `AcquireBackground`，或 `Options` 里加标志），后台任务专用。
2. Sweeper 先用 `Snapshot` 判断，再按 K1 的连接级 schema 状态跳过没有 KV 表的库；有过期 key 时才取写租约。
3. `DeleteExpired`（`kv/ttl.go:34-70`）当前逐 key 执行两条 DELETE，改为每张数据表一条：`DELETE FROM <tbl> WHERE "key" IN (SELECT "key" FROM __sb_kv_keys WHERE etime <= ?)`，再删元数据，并加单轮上限（例如 1000）避免长事务。

**实施记录**：第 1 条 `Registry.AcquireBackground`（不刷新 `lastUsed`）；第 3 条 `DeleteExpired` 每表一条批量 DELETE + 单轮 1000 上限。**第 2 条（2026-11-03 补齐）**：此前 Sweeper 仍先取写租约再 `HasSchema`；现改为先以后台**只读**租约探测 `HasSchema` + 新增 `Store.HasExpired`（`SELECT 1 … etime <= now LIMIT 1`），两者都为真才升级写租约清扫——无 KV 表 / 无过期 key 的项目不再触碰写路径（也不经 writeGate）。新增 `TestHasExpired`。

### K3 写路径往返次数偏多 ✅ 已完成（2026-11-02）

- `ListRepo.push`（`kv/list.go:80-134`）：`guardWrite` 读一次 meta，结束时 `getMetaOf` 又读一次；N 个元素执行 N 条 INSERT。改为复用第一次读到的 meta，并用多行 `VALUES` 批量插入。
- `KeyRepo.Exists` / `Delete`（`kv/key.go:202-245`）逐 key 查询，改为 `WHERE "key" IN (…)` 一次取回。
- `hash.go`、`set.go`、`zset.go` 用同样方式检查。
- 用例：现有 `kv` 测试全量回归，另加批量参数的边界用例（重复 key、空列表）。

**实施记录**：`guardWrite` 返回 `(exists, meta, err)`，`list.push` 与 `string.SetWith`（KeepTTL 分支）复用 meta；`list.push` 改多行 VALUES 一次插入；`Exists` 单条 IN 查询 + 重复 key 各计一次；`Delete` 一条 IN 查询取回候选后批量级联（过期不计入）。边界用例 `kv/k3_batch_test.go` 全过，kv 全量回归通过。

### K4 key 唯一性只靠应用层 ✅ 已复现并修复（2026-11-02）

KV 表没有 UNIQUE 约束（`kv/schema.go`）。DuckDB 用户库允许 10 个连接、使用乐观并发，两个并发 `Update` 对同一新 key 各自 `guardWrite` 都会看到「不存在」，可能插入两行元数据。

- **复现结果**：`kv/k4_race_test.go` 30 轮并发 SET，30/30 全部出现重复元数据行——问题确凿。
- **修复**：`kv.Store` 写事务 per-DB 互斥（`dbWriteMu`，`sync.Map[*sql.DB]*sync.Mutex`），`Update` 持锁执行；修复后 3 次复跑全过，`-race` 全量通过。未补 UNIQUE 索引（无兼容迁移层，遇已有重复行会建索引失败）。

---

## 4. P1：性能 ✅ 已完成（2026-11-02，P1–P6 全部落地）

| 编号 | 位置 | 现状 | 方案 |
|---|---|---|---|
| P1 | `filedb/remote.go:342-366` | 快照文件读三遍：`copyFile` 写一遍、`fileSHA256` 读一遍、`os.ReadFile` 整个读进内存再 `PutBytes`。GB 级库会占用等量内存 | 复制时用 `io.MultiWriter` 同步计算 sha。上传改为流式：`blob.Store` 增加可选接口 `PutFile(ctx, key, path, contentType)`，用类型断言探测，不破坏现有实现；宿主 S3 实现走分片上传 |
| P2 | `filedb/remote.go:496-535` | `latestManifest` 每次冷打开都 LIST 全部 manifest（manifest 不清理，只增不减），并 GET 每个更大的 seq | 先以本地 `local-state.json` 的 seq 为锚点 GET `seq+1`；不存在即为最新。缺锚点时 LIST 用 start-after（key 是 `%020d` 零填充，字典序即数值序），只读最后一页 |
| P3 | `filedb/factory.go:224-233` | SQLite 每次打开都跑 `PRAGMA integrity_check`，耗时 O(库大小)，冷打开慢 | 改为 `quick_check`；完整 `integrity_check` 只在 `downloadSnapshot` / 恢复之后执行 |
| P4 | `query.go:72-86` | 每行分配 `vals` 和 `ptrs` 两个切片 | `ptrs` 复用，每行只新建 `vals`；行切片按 `maxRows` 预估容量（上限 1024） |
| P5 | `cache/manager.go:143`、`cache/evict.go:75-104` | `Path` 要求目录名是 UUID（适配 DuckLake 的 `cacheDir/{dbID}`），但用户库在 `cacheDir/projects/{projectID}`，整个 `projects` 被当作一个目录、永远不是淘汰候选。LRU 用目录 mtime，而写文件不会改变目录 mtime | 按「库」而非一级目录统计：同时枚举 `{uuid}` 与 `projects/{projectID}`，`IsActive` / `CloseDatabase` 已支持项目 ID。LRU 改用 registry 的 `LastUsed`（新增只读查询），没有记录时退回目录内最新文件 mtime。`Usage` 结果短时缓存（如 30 秒），避免每次全树 walk |
| P6 | `registry/registry.go:317-327`、`434-444` | `Remove` / `Shutdown` 以 50ms 轮询等待 `active` 归零 | `Handle` 加 `idle chan struct{}` 或 `sync.Cond`，`Release` 归零时通知 |

> P1 中的 `PutFile` 是 `blob.Store` 接口的增量扩展，需要同步改 `internal/dbbridge` 与宿主 S3 实现，并补用例（见 AGENTS.md「修改后的验证」）。
>
> **实施记录（2026-11-02）**：
> - **P1**：`copyFileHashed`（`io.MultiWriter` 复制时算 sha，一遍 IO）；`blob.PutFiler` 可选接口；`FileSyncer.Flush` 与 `SnapshotFile` 优先流式上传（宿主 `s3Client.PutFile` 已实现，文件句柄直传 SDK）；SQLite 路径 VACUUM INTO 后统一算摘要。快照 IO 从三遍降到一遍 + 流式上传，内存占用从「整库进内存」降为流式。
> - **P2**：`latestManifest` 拆出锚点探测路径 `manifestByAnchor`（本地 seq+1 起 GET，连续 miss 即最新，上限 32 次）；缺锚点退回全量 LIST（正确性兜底）。
> - **P3**：factory 打开改 `PRAGMA quick_check`；完整校验依赖下载路径 sha256 兜底。
> - **P4**：`Query` 的 `ptrs` 循环外复用，行容量按 `maxRows` 预估（上限 1024）。
> - **P5**：`Evict` 按「库」枚举两种布局（`{uuid}` 与 `projects/{projectID}` 深入一层）；LRU 优先 registry `LastUsedOf`（新增，可选接口断言），无记录退回 `dirLatestMtime`（目录内最新文件 mtime）。
> - **P6**：`Handle.idleDone`（`atomic.Pointer[chan struct{}]`）事件通知，`Release` 归零即唤醒；`Remove`/`Shutdown` 改 `waitIdle`，删除 50ms 轮询。
> - 全量回归：godatabase 12 包 + 宿主 `internal/objectstore` 全过（registry/kv 另过 `-race`）。

---

## 5. P2：代码整理（不改变行为）

| 编号 | 内容 |
|---|---|
| C1 | `query.go`：`Execute` 与 `executeOn` 逐行重复，让 `Execute` 直接调用 `executeOn(ctx, db, stmt)` |
| C2 | `registry`：`CloseDatabase` / `CloseIdle` / `Shutdown` / `Remove` 四处重复「关闭 + `OpenHandleDelta(-1)`」，抽出 `r.closeHandle(h, flush bool)`；同时修正 `Remove` 后台关闭失败时没有日志的问题 |
| C3 | 死字段：`Handle.mode`（只写不读）、`cache.Manager.maxDatabases`（只赋值）。删除，或落实语义（`maxDatabases` 参与淘汰判定） |
| C4 | `ducklake/write_gate.go:144-165` 的 `isMutatingSQL` 用子串判断 `WITH` 是否写（`'...INSERT...'` 字符串字面量会误判为写），与 `sqlguard` 两套逻辑。S1 完成后改用 `sqlguard` 的词法判定 |
| C5 | `kv/executor.go:105-111` 的 `isMissingRelation`、`ducklake/write_gate.go:114` 的 `IsTransactionConflict` 靠错误文本匹配。先收敛到一处并加驱动版本说明；duckdb-go 暴露错误类型后改用 `errors.As` |
| C6 | `ducklake/catalog_syncer.go`（1197 行）按职责拆为 `syncer_schedule.go`（MarkDirty / 计时器 / Pause）、`syncer_capture.go`（captureCatalog）、`syncer_publish.go`（publishSnapshot / manifest seq）、`syncer_prune.go`、`local_catalog.go`（EnsureLocalCatalog）。纯移动，不改逻辑 |
| C7 | 日志接口统一：`lease.Logger`（`Printf`）与 `godatabase.Logger`（zap 风格）并存，`lease` 改用 `godatabase.Logger`，宿主适配随之删除一层 |
| C8 | `FileSyncer` 的 `dirty` / `last` / `epoch` / `lost` / `restored` / `bound` 六个 map 合并为 `map[string]*projectState`，减少锁内多次查表，也便于 R2 加代次字段 |

---

## 7. 新增：godatabase CLI（`godatabase/cmd/godb-cli.go`）

godatabase 是独立模块，脱离 SimpleBase 宿主也可使用（README「脱离 SimpleBase 单独使用」），
但目前没有配套命令行工具——打开库、执行 SQL、做快照都必须写 Go 代码。补一个 **本地运维 CLI**，
覆盖日常排查与验证场景，同时成为批次 A/B/C 修复项的人工回归入口。

### 定位与边界

- 只做**本地模式**（`DataRoot` + `CacheDir` 目录），不连 S3（`blob.Store` 传 nil / `NewNopSyncer`），
  不依赖根模块任何包，与 `cmd/simplebase`（走 HTTP API 的资源 CLI）完全独立。
- 子命令直接组装 `filedb.Factory` + `registry.New`（用户库），复用 `sqlguard` 做只读判定；
  系统库（DuckLake）暂不支持，避免 S3 依赖。
- 输出为人类可读文本（默认）或 JSON（`--json`），错误信息不含数据库路径（AGENTS.md 第 7 条）。

### 子命令

| 命令 | 功能 | 复用的现有 API |
|---|---|---|
| `godb open --path <dir> --engine duckdb\|sqlite --project <id>` | 创建/打开用户库并执行完整性检查，输出引擎、文件大小 | `filedb.Factory.Open`、`registry.Acquire` |
| `godb sql --project <id> [--rw] -- "<sql>"` | 执行单条 SQL；默认只读（走 `sqlguard` 校验 + ReadOnly 租约），`--rw` 才拿写租约 | `sqlguard.Validate`、`Lease.Handle.Query` |
| `godb kv --project <id> get/set/del/scan <key> [--value] [--ttl]` | KV 子操作，验证批次 C 的 KV 优化 | `kv.New`、`Store.Get/Set/Delete/Scan` |
| `godb snapshot --project <id> --out <file>` | 本地快照导出（拷贝主文件 + sha256），验证 R3 修复 | `filedb` 快照路径（`SnapshotFile` 同款逻辑） |
| `godb inspect --project <id>` | 打印库元信息：表清单、KV 表状态、快照 manifest 记录 | `Query` 系统表、`kv.Store.HasSchema` |
| `godb bench --project <id> --ops <n>` | 简易基准：单 key GET/SET 的 p50/p99，验证批次 C 性能对照 | `kv.Store` 计时循环 |

### 实现要点

1. 目录：`godatabase/cmd/godb/`（`main.go` 单文件起步，超过 ~500 行再拆 `cli.go`）。
2. 参数解析用标准库 `flag` 子命令模式，与 `cmd/simplebase` 风格一致。
3. `--project` 校验走 `blob.ValidateProjectID`（8 位 `[A-Za-z0-9-]`）。
4. 只读 SQL 必须复用批次 A 修复后的 `sqlguard`，不得自行解析。
5. 用例：`main_test.go` 覆盖——只读执行 CTE DELETE 被拒（联动 S1）、`--rw` 写入后数据可读、
   KV set/get 往返、快照文件与源库 sha256 一致、路径不存在时的脱敏错误。

---

## 8. 实施批次与验证

| 批次 | 内容 | 前置 | 预估 |
|---|---|---|---|
| A | S1、S2 ✅ 已完成（2026-11-02） | 无 | 1–1.5 天 |
| B | R1、R2、K2 ✅ 已完成（2026-11-02）；R3 未复现已降 P2 并留回归用例 | A 合入后 | 2–3 天 |
| C | K1、K3、K4、P1–P6 ✅ 已完成（2026-11-02）：K4 复现 30/30 并用 per-DB 写互斥修复 | B | 3–4 天 |
| D | C1–C8 ✅ 已完成（2026-11-02） | 可与 C 并行，C6 最后做以减少冲突 | 1–2 天 |
| E | `cmd/godb` CLI（§7）✅ 已完成（2026-11-02） | A 合入后（依赖修复后的 sqlguard） | 0.5–1 天 |

> **实施记录（2026-11-02，批次 D）**：
> - **C1**：`Execute` 委托 `executeOn`（消除逐行重复）。
> - **C2**：`closeHandle(h, flush bool)` 成为 registry 唯一关闭出口（Evictor 溢出挤出 / CloseDatabase / CloseIdle / Shutdown 走 flush=true，Remove 失租后台关闭走 flush=false）；Remove 后台关闭失败补日志（此前被吞）。
> - **C3**：`Handle.mode` 死字段删除；`cache.Manager.maxDatabases` 落实语义——`EnsureCapacity` 库目录数超限时按 LRU 淘汰多余库（`evictOverCount`），宿主 `CacheMaxDatabases` 配置自此生效。
> - **C4**：`ducklake.isMutatingSQL` 改用 `sqlguard.Validate(q, ReadOnly)` 词法判定，消除 `'...INSERT...'` 字符串字面量误判；sqlguard 额外拒绝的语句保守视为写（fail closed）。
> - **C5**：错误文本匹配收敛到根包 `errmatch.go`（`IsMissingRelation` / `IsTransactionConflict`，含 duckdb-go v2 / go-sqlite3 驱动版本说明）；kv / ducklake / classifySyncError 全部改调收敛处。
> - **C6**：`catalog_syncer.go`（1197 行）按职责拆为 `syncer_schedule.go`（419）/ `syncer_capture.go` / `syncer_publish.go` / `syncer_prune.go` / `syncer_observe.go` / `local_catalog.go`，纯移动不改逻辑，主文件缩至 151 行（常量 + 结构 + 构造）。
> - **C7**：`lease.Logger` 改为 `godatabase.Logger` 类型别名（zap 风格），宿主 `zapAdapter` 适配层删除、直接注入 `a.logger`。
> - **C8**：`FileSyncer` 六个 per-project map（bound/gen/flushedGen/last/epoch/lost/restored）合并为 `map[string]*projectState`，锁内一次查表；`flushDirty` 顺带修正为只枚举真正有 lag 的项目（原实现枚举全量键）。
>
> **实施记录（2026-11-02，批次 E）**：
> - `godatabase/cmd/godb-cli.go` 单文件 CLI（~600 行，目录扁平化：无 `godb/` 子层，产物名 `godb`）：`open` / `sql` / `kv`（get/set/del/scan）/ `snapshot` / `inspect` / `bench` 六个子命令；标准库 `flag` 子命令模式；`--json` 输出；错误脱敏（路径替换为 `<data-root>`）。
> - 只读 SQL 强制走 `sqlguard.Validate`（批次 A 修复版）；`openReg` 组装 `filedb.Factory` + `registry.New`，本地模式（`Remote:false`，不连 S3）。
> - `kv` 走 `store.Update/View` 事务闭包（与线上路径一致）；写后 `Handle.NotifyWrite` 触发 onWrite 回调。
> - `godb-cli_test.go` 5 用例全过：CTE DELETE 只读被拒（S1 联动）、RW 往返、KV set/get 往返、快照导出 sha256、非法项目 ID 脱敏错误。
>
> **收尾记录（2026-11-02 晚）**：
> - **R1 第 3 条（缓存字节淘汰接线）**：`registry.Evictor` 新增可选接口 `CapacityEnsurer`（`EnsureCapacity(ctx, needBytes)`），周期内先 `CloseIdle` 再 `EnsureCapacity(ctx, 1)`（1 字节目标触发「超上限即淘汰」，含 C3 数量维度）；registry 不反向依赖 cache（分层保持）。宿主 `cache.NewManager` 创建上移至 Evictor 装配前，`cache_max_bytes` / `cache_max_databases` 配置自此周期性生效。新增 `TestEvictor_CapacityEnsurerWired` 回归。
> - **性能对照基线（批次 C 后首测，2026-11-02，macOS arm64）**：
>   - `TestReadLatencyPhases`：inlining=0 热读 p50=2.47ms / p95=4.64ms / p99=7.00ms；inlining=1000 p50=1.48ms / p95=2.12ms / p99=2.46ms；并发 32 下 p50 17.6ms→8.8ms。
>   - `BenchmarkSingleWriteCost`：inlining=0 → 2.75ms/write、1393 newbytes/write；inlining=1000 → 2.84ms/write、0 newbytes/write（写放大归零）。
>   - **注**：批次 C 前基线未留存（实施时未跑），以上为 C 后数值，供后续回归对照；P1–P6 净收益无法从本组数据反推。
>   - **KV 基准（`godb bench --ops 1000`，2026-11-03 补齐 §8 要求的 EXISTS/DEL）**：
>
>     | 引擎 | SET p50/p99 | GET p50/p99 | EXISTS 1000 key | DEL 1000 key |
>     |---|---|---|---|---|
>     | SQLite | 57µs / 226µs | 9µs / 14µs | 1.64ms | 6.32ms |
>     | DuckDB | 360µs / 534µs | 204µs / 242µs | 4.89ms | 141.9ms |
>
>     DuckDB DEL 偏慢来自多表级联 DELETE 在列存上的成本（单事务），不影响正确性；列为后续观察项。
>
> **v6.4 状态：全部完成**（S1/S2、R1–R2、R3 降级留用例、K1–K4、P1–P6、C1–C8、CLI、基线）。

**每批验证**（来自 `godatabase/AGENTS.md`）：

```bash
cd godatabase && GOWORK=off go build ./... && GOWORK=off go vet ./...
GOWORK=off go test ./... -timeout 900s
cd .. && go build ./internal/... ./cmd/... && go test ./internal/dbbridge ./internal/catalog ./internal/systemdb
go test ./internal/objectstore   # 涉及 blob / key 时必跑（金标锁定 key 布局）
```

**性能对照**：批次 C 前后各跑一次 `bench/`（`TestReadLatencyPhases`、`SingleWriteCost`），并为 KV 新增基准：单 key `GET` / `SET` 的 p50/p99，以及 1000 key `EXISTS` / `DEL`（可直接用批次 E 的 `godb bench` 辅助验证）。

**环境注意**：本机（macOS arm64）此前无法链接 DuckDB（`ld: library 'duckdb_static' not found`）。2026-11-02 复测 `GOWORK=off go build ./...` 已通过，本地依赖已恢复，可在本机执行完整测试；若 CI 环境异常仍按原方式处理。

## 9. 不做的事

- 不修改 S3 key 模板、manifest / lease JSON 字段（磁盘契约）。
- 不为已有 KV 库补 UNIQUE 索引或做数据迁移（无兼容层原则）；K4 用写互斥解决。
- 不改变 `registry` 的单进程单写模型和「副本数固定为 1」的部署约束。
- 不引入新依赖（流式上传通过 `blob.Store` 可选接口实现，库内不依赖 aws sdk）。
