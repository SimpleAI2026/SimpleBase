# DuckLake 读写性能优化计划（最终一致版）v4.0

> 日期：2026-10-03。状态：**P1 已完成**（2026-10-04），P2 待做。
> 前置：`plan/planv3.0/ducklake-storage-latency-optimization-v2.0-plan.md`（下称 **v2.0**）、`plan/planv3.0/multi-instance-consistency-plan.md`（下称 **MIC**）、`plan/planv4.0/console-polish-and-systemdb-perf-plan.md` §1（系统库，下称 **CP**）。
> **前提变化**：本轮放弃多实例强一致（MIC 的同库 fencing、split-brain 检测、观察式租约）与“提交即远端持久”，只要求**单写实例 + 最终一致**：API 写成功 = 本地已提交；S3 在有界时间（RPO，默认 ≤ 30s）后收敛到最新状态；崩溃或丢盘最多丢失 RPO 窗口内的写入。MIC 中为强一致付出的同步链路成本因此可以移出请求路径或删除。

## 0. 结论

1. **写慢的主因不是 SQL 执行，而是“每次写 = 一个远端 Parquet + 一次 catalog 全量快照上传”。** 远端模式在 `factory.go:245-247` 强制 `DATA_INLINING_ROW_LIMIT 0`（MIC §4.2），每个 INSERT 都同步 PUT 一个 Parquet 到 COS；随后 `AfterWrite → MarkDirty → 200ms debounce → syncOnce` 在**同一条唯一连接**上 `COPY FROM DATABASE` 整个 catalog，再上传快照、写 manifest、执行清理。
2. **读慢的主因是小文件 + 远端读 + 单连接排队。** 每次写产生 1 个文件且用户库没有合并任务（`systemdb/maintenance.go` 只覆盖系统库），读取文件数线性增长；数据在 COS 上，每个文件至少需要远端读取 footer 和数据；单连接下，读请求还要排在 COPY/同步/写之后。
3. **最终一致前提下的核心改法：本地优先，远端异步。** 恢复数据内联，让小写入只进入本地 catalog；后台按批次 flush 和 merge 成大 Parquet；catalog 快照按 RPO 周期上传，不再随每次写触发；单实例关闭 S3 写租约，去掉首写 40s 判活；精简清理与 manifest 探测，把每次同步约 30 个 S3 请求降到约 3 个。
4. **不做**：不放开 `MaxOpenConns(1)`（`factory.go:150-155` 已实测证明不可行）；不迁移 PostgreSQL；不做同库多写。

## 1. 实测验证（本地探针，2026-10-03）

方法：在 `internal/database/ducklake` 写临时测试（已删除），使用 DuckDB 1.5.2 + 本地 `DATA_PATH`；对象存储使用内存 BlobStore，并给每次请求注入 **30ms** 延迟模拟 COS。机器为 macOS 本机。本地 SimpleBase 实例未运行（`:8080` 拒绝连接），因此**没有端到端 API 和真实 COS 数据**，下表只用于说明阶段量级。

| # | 场景 | 结果 | 说明 |
|---|---|---|---|
| A | 单条 INSERT（本地 DATA_PATH） | p50 1.9ms / p95 2.4–4.8ms | SQL 本身不慢 |
| B | INSERT + `AfterWrite`（`CurrentSnapshot` + 写 local-state.json） | p50 2.3–3.4ms | 每次写额外 1 次 SQL 和 1 次 fsync 级文件写，开销较小 |
| C | **单次 catalog 同步（Flush）** | **≈1.03s**；**28–29 次 GET + 3 次 PUT** | 30ms × 31 ≈ 0.93s；绝大多数是请求往返，COPY 本身不足 100ms |
| D | 持续写入（每 50ms 一次）时的读延迟 | p50 4.4ms / p95 7.4–8.5ms / max 18ms，空闲时 p50 1.0ms | 读请求排在 COPY/写之后，单连接争用使延迟升高 4–8 倍 |
| E | **内联关闭，300 次单行写后读最近 50 行** | 产生 **300 个 Parquet**；读取 p50 **10.8ms** | **本地盘**上已经慢 5 倍；COS 上每个文件都有远端往返（CP §1.2 R2 实测：150 个文件冷读 27s） |
| F | 内联 1000，同样 300 次写 | 0 个 Parquet；读取 p50 **2.3ms**；`flush_inlined_data` 4.6ms 后合并为 **1 个文件** | 内联同时降低写延迟（1.7ms 对 2.3ms）和读放大 |
| G | 首次打开（安装扩展）/ 热打开 / 远端冷打开 | 6.4s / 31ms / 180ms（4 次 GET） | 首次 6.4s 来自 `INSTALL` 扩展；生产环境必须预装扩展 |

### 1.1 单次同步 ~30 个 S3 请求的来源（C）

`syncOnce`（`catalog_syncer.go:422-606`）每轮执行：

1. `nextManifestSeq` → `ReadLatestManifest(fromSeq=cached)`：GET 锚点，再前探或二分，约 2–4 次 GET；
2. `PutIfAbsent` 快照 + `WriteManifest`：2 次 PUT（再加 local-state 等）；
3. **`pruneVersions`（:683-736）每轮读取保留窗口内全部 `KeepVersions=10` 个 manifest**，构建 `referenced`，再读取到期区间最多 10 个 manifest 并 DELETE：约 20 次 GET + N 次 DELETE；**这些清理请求在同步 goroutine 内串行执行**。

在 COS 广州 RTT 50–150ms 下，单轮同步约 **1.5–4.5s**。同步期间该库 `inflight=true`，新的 MarkDirty 只能等下一轮；如果 COPY 期间有新写入，`snapshot changed during copy` 会使整轮作废并重做（CP §1.2 R3：20 分钟内出现 5 次）。

### 1.2 写路径真实耗时（远端模式，推断）

```text
HTTP → auth（系统库）→ project 解析（系统库）→ GetDatabase（系统库）
     → WriteGate：首写同步 lease.Acquire（lease.go:263 从 epoch 1 线性 GET；他人/旧进程持租时 sleep TTL+Grace=40s）
     → Registry.Acquire（冷库：manifest GET + 下载 catalog）
     → INSERT：DuckLake 写 Parquet 到 COS（1 次 PUT，50–150ms）+ catalog 提交
     → AfterWrite：CurrentSnapshot + local-state 写入
     → 200ms 后：COPY（占用唯一连接）+ 约 30 次 S3 请求
```

- 稳态单写：≈ 系统库 3 次查询 + 1 次 COS PUT ≈ **0.2–0.5s**。同步占用连接时，读写都要排队。
- **重启后首写**：`probeMax` 每 10s 续租一次，一天产生约 8,640 个 epoch，需要**逐个 GET**（`lease.go:263-278`）；旧进程租约未过期时还要等待 40s，并且超过前端 15s 超时（v2.0 §2，CP §1.5 实测 45.9s / 500）。这是最严重的写慢点。

### 1.3 读路径真实耗时（推断 + CP 实测）

- 每个 Parquet 至少一次 COS 读取（footer，未命中缓存时还要读取数据）；`ORDER BY created_at DESC` 且没有 `LIMIT`（`data_handler.go:111`）时需要扫描全部文件；
- 没有设置远端读缓存：仓库中未出现 `enable_external_file_cache`、`parquet_metadata_cache`、`http_keep_alive` 等配置（grep 无结果），依赖 DuckDB 默认值；
- 单连接上同步、写和读串行执行。

## 2. 与旧计划的取舍（最终一致前提下）

| 旧决策 | 出处 | 本计划 | 理由 |
|---|---|---|---|
| 远端模式强制内联 = 0 | MIC §4.2 P0-0 | **改为允许内联（默认 1000），并在每次上传快照前 flush 到 Parquet** | 内联的风险是“catalog 丢失即丢数据”。最终一致只要求上传的快照可恢复：先 flush 再 COPY，可保证远端快照不含内联行（MIC §4.2 已给出这一替代方案）。本地 catalog 中的内联行属于 RPO 窗口，可以接受 |
| 每次写 debounce 200ms 全量同步 | v2.0 §4.4 | **按 RPO 周期同步（默认 15s，dirty 时）+ 关停 flush** | 同步频率从“每次写”降为“每个周期最多一次”，COPY 次数和 S3 请求按写 QPS 倍数下降 |
| `sync_on_commit` / `synced_s3` | v2.0 §4.4 | **废弃**；写响应统一为 `committed_local`，另暴露 `sync_lag_seconds` | 最终一致语义下不再承诺提交即同步 |
| per-DB 写租约（观察式判活、续租写对象） | MIC §4.6 | **单实例部署默认关闭**；保留实例级启动探针，防止误启动第二个实例 | 去掉首写 40s 等待和每 10s 一次 PUT；同库多写不在支持范围内 |
| manifest `PutIfAbsent` 冲突 = split-brain | MIC §4.4 | 保留 manifest 序列（恢复依赖它），冲突只告警，不额外加门禁 | 零成本保留 |
| `pruneVersions` 每轮读 10 个 manifest | v2.0 §3.3 | **移出同步路径，改为低频后台任务，并用本地缓存的引用表** | 每轮同步约 20 次 GET → 0 |
| 用户库无 compaction | CP §1.4 P1-C 仅系统库 | **用户库纳入后台维护（flush + merge）** | 读放大的根治手段 |

## 3. 方案

### P0 — 测量（0.5 天，必做，不改行为）

1. 打开 `observability.perf_stage_timing`，按阶段记录：`auth` / `project` / `lease_gate` / `registry_open` / `conn_wait` / `exec` / `after_write`；后台记录 `sync_copy` / `sync_put` / `sync_manifest` / `sync_prune` 的耗时和 S3 请求数，并按库打标签。
2. 在真实 COS 上复测 §1 中的 A/C/E/G：100 次单行写后读最近 50 行，记录文件数、冷/热读取耗时；单次同步的请求数和耗时；重启后首写耗时。
3. 输出“真实基线表”，作为 §5 验收的对照。**P1 各项都要用这张表证明收益，不能只依赖本地推断。**

### P1 — 写路径降耗（1.5 天）

**P1-1 恢复内联 + 同步前 flush**（`ducklake/factory.go:245-247`、`options.go`、`catalog_syncer.go:syncOnce`）
- 远端模式不再强制 `DATA_INLINING_ROW_LIMIT 0`，使用配置值（默认 1000；`config.yaml:39` 的注释同步修改）。
- `syncOnce` 在取 `snapBefore` **之前**执行 `CALL ducklake_flush_inlined_data('{alias}')`；flush 本身也会产生 snapshot，因此 flush 后再读取 `snapBefore`。flush 后断言 `ducklake_inlined_data_*` 为空，不为空则本轮不上传、记录告警并重试。
- 效果：小写入不再 PUT Parquet（单次写延迟 ≈ 本地提交 2ms 量级）；每个同步周期把 N 次写合并为 1 个 Parquet。
- DoD：远端模式写入 1 行后不产生新 Parquet；一轮同步后 `data/` 中新增 1 个文件，远端快照中 inlined 行数为 0；从远端快照冷启动能读到该行。

**P1-2 按 RPO 周期同步**（`catalog_syncer.go:MarkDirty/scheduleLocked`、`config` 的 `catalog_sync`）
- 新增模式 `mode: interval`（默认），`interval: 15s`、`max_lag: 30s`。`MarkDirty` 只记录 dirty 水位；**如果已有定时器则不重置**（修复 debounce 在持续写入下不断推迟的问题），定时器到期后执行一次同步。
- 写入持续时，在 `max_lag` 内必须完成一轮同步；同步失败按 1s→2s→…→30s 退避，并保留 dirty（`sync()` 结尾的重调度逻辑 `:357-365` 已具备，改为使用 interval）。
- 移除 `sync_on_commit`：配置值仍可解析，但降级为 interval，并在启动时记录 Warn。`DurabilityFor` 恒为 `committed_local`。
- 系统库沿用同一机制（CP P1-B 把系统库刷写间隔调到 15s，节奏一致）。
- DoD：10 QPS 持续写入 60s，同步次数 ≤ 5，`sync_lag_seconds` 峰值 ≤ 30s；停写 15s 内远端 manifest 覆盖最后一次写入。

**P1-3 同步路径瘦身：每轮 ≤ 3 次 S3 请求**（`catalog_syncer.go:554-568`、`manifest.go`）
- `nextManifestSeq`：进程内 `lastSeq` 已知时直接使用 `lastSeq+1`，不再访问远端；`PutIfAbsent` 冲突时才回退到远端发现（单写下几乎不会发生）。
- `pruneVersions` 移出 `syncOnce`：新增 per-syncer 后台 goroutine，每 10 分钟最多执行一轮；引用表改用**本进程写入 manifest 时记录的内存环形缓冲**（最近 KeepVersions 个 seq→snapshotKey），无需重新 GET manifest；缓冲为空（重启后）时才回退到远端读取。删除失败时计数并告警，不影响同步结果。
- 每轮同步只剩：快照 PUT + manifest PUT（+ 首轮发现 GET）。
- DoD：在单次同步计数测试中，稳态请求数 ≤ 3（本地基线 31）；修剪仍满足“窗口内被引用的快照不删”的现有测试。

**P1-4 单实例关闭写租约**（`config.yaml:17-18`、`app.go:480-525`）
- `instance.lease.enabled` 默认改为 `false`。关闭后 `WriteGate` 直接放行，不再执行 `probeMax`、40s 等待和每 10s 一次 PUT。
- 防误启动第二实例：保留启动时在 S3 写入 `instance-heartbeat/{boot}.json`，并读取最近一个 heartbeat；如果它在 2×TTL 内仍在更新，则**只告警，不阻止启动**，同时写入 `simplebase_instance_conflict` 指标。按最终一致前提，这类部署错误由运维兜底。
- 需要保留租约的部署（显式开启）：`probeMax` 从 epoch 1 线性探测改为“本地缓存 epoch + 指数/二分”，复用 `ReadLatestManifest` 的定位算法，请求数从 O(n) 降为 O(log n)；他人持租时立即返回 `503 lease_held + Retry-After`，异步执行判活（v2.0 §4 P1 首条），不再在请求内 sleep。
- DoD：重启后首写 < 500ms（远端冷开另计）；开启租约时，跨天运行后的首写 GET 数 ≤ 40。

**P1-5 `AfterWrite` 去掉同步落盘**（`factory.go:426-449`）
- `local-state.json` 的 `snapshot_id` 改为在同步时和关停时写入，不再每次写都执行 tmp+rename。这个水位只服务 `EnsureLocalCatalog` 的“本地领先”判断，同步周期粒度已经足够：丢失只会把本地误判为“不领先”，此时现有逻辑会保留本地文件并告警，不会覆盖。
- `CurrentSnapshot` 查询保留（MarkDirty 需要），后续可改为在 handle 上做计数，然后批量查询。

### P2 — 读路径降耗（1.5 天）

**P2-1 用户库后台维护：flush + merge**（新增 `ducklake/maintenance.go`，复用 `systemdb/maintenance.go` 的执行框架）
- 对已打开且 dirty 的用户库，在同步前执行 flush（P1-1），**每小时或每产生 50 个新增文件**执行一次 `ducklake_merge_adjacent_files`。只有执行完 merge 后，才在下一轮快照中体现合并结果。
- `expire_snapshots` / `cleanup_old_files` 沿用 CP P1-C 的保守策略：older_than 7d，不执行 `delete_orphaned_files`。
- 维护任务和同步一样只占用唯一连接，并且分批执行、每步之间让出连接，单步不超过 200ms 预算。
- DoD：300 次单行写后，存活文件数 ≤ 3；最近 50 行查询冷读耗时下降 ≥ 5 倍（P0 真实基线对比）。

**P2-2 远端读缓存**（`buildBootSQL`，`factory.go:229-280`）
- boot SQL 增加：`SET enable_external_file_cache = true`、`SET parquet_metadata_cache = true`、`SET http_keep_alive = true`。DuckDB 1.5 是否支持这些参数，需要先用 `duckdb_settings()` 核对并以实际支持为准，不支持则跳过。
- 注意 `lock_configuration = true` 必须保持在最后。
- 可选：`DATA_PATH` 改为本地优先（写本地，Parquet 文件不可变时异步复制到 S3，冷开时缺失文件按需拉取）。这项改造需要 DuckLake 支持多路径或自定义文件系统，**本轮不做，只记录**。

**P2-3 API 查询契约**（`api/data_handler.go:111`、`api/database_handler.go:191`）
- `ListDocuments` 增加 `LIMIT ? OFFSET ?` / 游标（默认 50，最大 500）；不再扫描到第 1000 行时返回 `row_limit_exceeded`。
- `GetDatabase` 去掉 `RefreshSync(3s)`，改为 `RefreshAsync` 并读取缓存（v2.0 §4 P1 已经列出这一项，本轮落实）。
- 文档写入的 `CREATE TABLE IF NOT EXISTS` 只在首次写入或缓存未命中时执行（进程内集合名缓存），避免每次写入都多执行一条 DDL 并推进 snapshot。

**P2-4 单连接公平性**（`catalog_syncer.go:syncOnce`）
- COPY 前检查 `sql.DB.Stats().WaitCount` 的增量。如果有前台请求在排队，则把本轮同步推迟 100ms，最多推迟到 `max_lag`。这样可避免同步插在前台请求高峰中。
- `snapshot changed during copy`：P1-2 降低同步频率后自然减少；仍然发生时直接安排下一轮，不记录为 Warn。

### P3 — 可选（评估项，不承诺）

- **catalog 增量同步**：catalog 增大到 MB 级以后，每轮 COPY 都是全量复制。DuckDB 引擎可以考虑直接上传 WAL 或做增量导出；需要单独验证 DuckLake 元数据的可恢复性。
- **独立备份连接**：`factory.go:150-155` 已证明第二连接不可行；除非 DuckLake 提供只读 attach 同一 catalog 文件的方式，否则不推进。

## 4. 风险与边界（必须写入部署文档）

| 风险 | 说明 | 兜底 |
|---|---|---|
| RPO 窗口丢数据 | 进程崩溃或本地盘丢失时，最多丢失最近 `max_lag`（30s）内的写入，以及 catalog 中尚未 flush 的内联行 | 正常关停先执行 flush + sync（`Close` 已有）；暴露 `sync_lag_seconds` 并在超过 60s 时告警 |
| 误起第二实例写同一前缀 | 关闭租约后不再阻止，可能出现 MIC §3.1 覆盖问题 | 心跳检测告警；部署文档继续要求 `replicas=1`；需要多实例时重新开启 `lease.enabled` |
| 内联行在本地 catalog | 本地 catalog 文件损坏会丢失这部分数据 | 同 RPO；flush 周期 = 同步周期 |
| merge 与读写争用唯一连接 | 维护期间请求排队 | 分批执行并让出连接；低峰期运行；按 `conn_wait` 指标调节 |
| 远端快照必须不含内联行 | flush 失败时上传的快照可能仍含内联行 | flush 后断言不满足就跳过上传（P1-1 DoD） |

## 5. 验收

| 指标 | 现状（来源） | 目标 |
|---|---|---|
| 单行写 API p95（热库，COS） | 0.2–0.5s 推断；同步占用时更高 | **< 50ms**（服务端，不含网络） |
| 重启后首写 | 45.9s / 500（CP 实测） | **< 1s**（含冷开） |
| 单次同步 S3 请求数 | 31（本地实测） | **≤ 3**（稳态） |
| 10 QPS 写入时的同步频率 | 每次写 debounce，持续写入时会被推迟 | ≤ 每 15s 一次；`sync_lag` ≤ 30s |
| 300 次单行写后读最近 50 行 | 300 个文件，本地 10.8ms，COS 秒级 | 文件数 ≤ 3；COS 冷读 < 300ms，热读 < 30ms |
| 持续写入时读 p95 / 空闲读 p95 | 8.5ms / 1.3ms（本地，4–8 倍） | ≤ 2 倍 |
| 停写后远端收敛 | — | ≤ 15s，可从远端冷启动读到最后一次写 |

测试：`go test ./internal/database/... ./internal/app/... ./internal/api/...`。新增测试：慢对象存储下同步请求计数、interval 模式持续写入不饿死、flush 后远端快照无内联行、关停 flush、租约关闭后 WriteGate 直通、用户库维护（fake 执行器，可中断）、文档分页。真实 COS 复测按 P0 方法执行并附报告。

### 5.1 P1 实施记录（2026-10-04）

**已完成项**：
- ✅ P1-1：恢复内联（默认 1000），同步前 flush（`factory.go:245`、`catalog_syncer.go:546-563`）
- ✅ P1-2：interval 同步模式（默认 15s，`max_lag: 30s`），废弃 `sync_on_commit`（`options.go`、`catalog_syncer.go:239-288`、`config.go:104-121`）
- ✅ P1-3：同步瘦身：进程内 seq 缓存、pruneVersions 移出同步路径、内存引用环（`catalog_syncer.go:351-353/608-616/683-736`、`manifest.go:88-98`）
- ✅ P1-4：租约默认关闭、`probeMax` 改为指数+二分（`lease.go:261-289`）、心跳告警保留（`app.go:480-525`）、API 错误映射补充 `lease_held`（`error.go:162-164`）
- ✅ P1-5：去掉每次写落盘 `local-state.json`，改为同步时写（`factory.go:426-449`、`catalog_syncer.go:608-616`）
- ✅ 配置示例和文档更新（`config.example.yaml:49/55-59`）
- ✅ 测试修正：同步默认模式、API 详情统计改为异步触发（`coverage_test.go:290`、`database_handler.go:191`）

**测试状态**：
- `go test ./internal/database/ducklake`：140.7s，全部通过
- `go test ./internal/api ./internal/config ./internal/app`：全部通过
- `go build ./cmd/... ./internal/...`：编译通过

**待验收**：P0 要求的真实 COS 环境性能复测尚未执行（需要运行中的 SimpleBase 实例）；当前改动已在本地内存模拟存储通过全回归。

## 6. 实施顺序与回滚

1. P0 测量 → 2. P1-4（收益最大、改动最小）→ 3. P1-3 → 4. P1-1 + P1-2（同一 PR，内联必须和“同步前 flush”一起上线）→ 5. P1-5 → 6. P2-3 → 7. P2-1 → 8. P2-2 / P2-4。

每项单独提交。回滚开关：`lease.enabled`、`catalog_sync.mode`（`debounce` 恢复旧行为）、`data_inlining_row_limit: 0`（恢复强制不内联）。都是配置项，回滚不需要发版。

## 7. 改动触点

- `internal/database/ducklake/{factory,catalog_syncer,manifest,options,local_state}.go`，新增 `maintenance.go`
- `internal/database/lease/lease.go`（仅在开启租约时改造 `probeMax`）
- `internal/app/app.go`（WriteGate、维护任务装配）
- `internal/api/{data_handler,database_handler}.go`
- `internal/config/{config,yaml,env}.go`、`config.example.yaml`（新增 `catalog_sync.interval/max_lag`，默认值调整）
- `docs/database/ducklake.md`、`docs/ops/deployment.md`（RPO 语义、`replicas=1`）
