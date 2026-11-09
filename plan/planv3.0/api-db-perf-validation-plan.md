# internal/api DB 操作性能验证计划（性能与主要耗时分解）

> 日期：2026-09-28（2026-09-29 增补 §2.4 Benchmark 用例方案）。状态：plan-only；本计划只定义**测量与验证方案**，不修改实现。
> 范围：`internal/api` 下所有触达数据库的 HTTP 端点（SQL / Data / KV / Database 管理 / 行数统计），目标是回答两个问题：
> 1. 每类 DB 操作在冷/热、本地/S3、空闲/竞争场景下的 p50/p95/p99 是多少；
> 2. 耗时主要花在链路哪一段（auth → catalog → WriteGate/租约 → registry Acquire/冷开 → 单连接排队 → SQL 执行 → 序列化 → onWrite 同步）。
> 关联：`plan/planv3.0/ducklake-storage-latency-optimization-v2.0-plan.md`（§2 已给出静态慢点假设，本计划负责**实测验证**）；`internal/AGENTS.md`（分层与系统库约束，测试不得违反）。

## 0. 被测对象清单（端点 × DB 路径）

| 分组 | 路由 | DB 路径特征 |
|---|---|---|
| SQL | `POST /v1/projects/:p/databases/:d/query` | 只读租约；sqlguard(ReadOnly)；maxRows 扫描上限 |
| SQL | `POST /v1/projects/:p/databases/:d/execute` | 读写租约 + WriteGate；写后 onWrite→MarkDirty |
| SQL | `POST /v1/projects/:p/databases/:d/batch` | 同上；transactional 两种模式分开测 |
| Data | `GET/POST .../data/collections[/:collection]`、`POST/PUT/DELETE .../documents` | `systemGuard`+`acquire` 两次 GetDatabase；CreateDocument 两次 Execute；ListDocuments 无 LIMIT（>1000 行报 row_limit_exceeded） |
| KV | `POST /v1/projects/:p/kv`（cmd / 五种类型写入） | 项目 KV catalog 查询/补建 + 租约 + kv.Store 自管事务 + NotifyWrite |
| 管理面 | `GET /databases`（列表 + RowCounts 缓存）、`GET /databases/:id`（RefreshSync 3s）、`POST /databases`、`DELETE /databases/:id` | 列表触发后台刷新风暴；详情可能同步等统计；建/删库走系统库+S3 |
| 系统库桥接 | admin 项目下上述只读端点 | `systemLeaseAdapter` 直连常驻连接，不经 registry；与用户库对比 |

## 1. 耗时分解模型（每请求 9 段）

每个请求按以下阶段计时，**总和必须≈端到端耗时**，否则说明有未观测段：

| # | 阶段 | 代码位置 | 已知风险假设 |
|---|---|---|---|
| S1 | auth 认证 | `internal/auth/middleware.go` | 每请求查系统库，单连接排队 |
| S2 | project 解析 | `projectContextMiddlewareEcho`（router.go） | 同上，与 S1 串行叠加 |
| S3 | body 解码/校验 + sqlguard | `sql_handler.go:351-419` | 大 body / 100 条 batch 校验开销 |
| S4 | catalog GetDatabase | `adapter.go:93-95` → 系统库 | DataHandler 写路径重复 2 次（`data_handler.go:280-308`） |
| S5 | 并发 semaphore | `sql_handler.go:102-120` | MaxConcurrentQueries=64，满时直接 429 而非排队 |
| S6 | Acquire（热）/ WriteGate / 冷开 | `registry.go:83-168`、`factory.Open`、`lease.Manager.Acquire` | 冷开=manifest GET+SQLite 下载+boot SQL；首写=O(epoch) 探测、他人持租等 40s |
| S7 | 单连接排队 + SQL 执行 | `factory.go:90` MaxOpenConns=1、`query.go` | COPY/后台统计占用同一连接；`*sql.DB.Stats().WaitDuration` |
| S8 | 结果序列化 + JSON 响应 | `database/serialize.go`、handler | 大结果集 / 1000 行文档反序列化 |
| S9 | onWrite 同步链路 | `handle.go:80-117`、`factory.go:293-316`、CatalogSyncer | sync_on_commit 模式在请求内等 COPY+S3 PUT（`context.Background()`） |

**输出契约**：任何结论必须标注属于哪一段；禁止只用响应里的 `DurationMS`（只覆盖 S7 执行，`sql_handler.go:177`）推断整链耗时。

## 2. 测量基础设施（先行建设，不改行为）

### 2.1 分段计时中间件（新增，默认关闭）

- 在 `internal/api` 增加 `stage_timing.go`：echo 中间件 + `context.Context` 挂载 `*StageTimer`，handler/adapter 在 S1–S9 边界打点（`time.Since` 累加）。
- 配置开关 `SIMPLEBASE_PERF_STAGE_TIMING=true`（config 增加布尔项，遵循 YAML+env 双来源约束）；关闭时为零开销（nil 判空）。
- 输出两条通道：
  1. Prometheus histogram `simplebase_api_stage_seconds{route,stage}`，**buckets 扩展为 0.001–40s**（现 `prometheus.DefBuckets` 上限 10s，无法区分 40s 租约等待尾部）；
  2. 每请求一行结构化日志（debug 级），只含 request_id/各段毫秒数/行数/错误码，**禁止记录 SQL 与参数**（internal/AGENTS.md 审计约束）。
- 在 `Handle`/`systemLeaseAdapter` 包一层计时壳记录 S7 内的 `DB.Stats().WaitCount/WaitDuration` 增量，区分「排队」与「执行」。

### 2.2 压测工具

- 新增 `internal/api/perfbench/`（独立 package，不进生产二进制）：
  - `harness.go`：直接装配真实 Router + 真实 catalog/registry/ducklake factory（本地盘 = DevMode；假 S3 = minio 或 objectstore memory 实现），绕过 HTTP 网络栈用 `httptest`，排除网络噪声。
  - `loadgen.go`：固定并发 worker（1/8/32/64/128）+ 每 worker 固定请求数的闭环压测；输出 hdrhistogram 风格 p50/p95/p99/max 与错误率。
  - `stage_report.go`：聚合 stage timing，输出各段耗时占比表。
- 端到端（含真实 HTTP/前端超时行为）用 `k6` 或 `vegeta` 脚本，放 `scripts/perf/`，仅本地运行。
- Go 微基准（`BenchmarkXxx`，`go test -bench`）覆盖纯函数段：sqlguard 校验、serializeRows、KV 命令解析，与端到端结果互证。

### 2.3 数据与场景矩阵

| 维度 | 取值 |
|---|---|
| 存储模式 | 本地盘（DevMode）/ 假 S3（注入 50ms/200ms 延迟）/ 真实 S3（同城，单独授权） |
| 库温度 | 冷（首请求触发 factory.Open）/ 热（handle ready）/ 温（本地 catalog 在、句柄已关） |
| 库数量 | 1 / 8 / 200（验证列表 N 规模与后台刷新风暴） |
| 表/数据量 | 每库 0 / 10 / 200 表；单表 0 / 1k / 100k 行；单行 100B / 10KB 文档 |
| 租约状态 | 已持租 / 首次持租（epoch 1/100/1000）/ 他人持租（观察 40s 等待与前端 15s 超时的关系） |
| 并发 | 1 / 8 / 32 / 64 / 128（跨 MaxConcurrentQueries=64 临界点） |
| 后台干扰 | 无 / RowCounts 刷新中 / CatalogSyncer COPY 中 / KV TTL Sweeper 运行中 |

默认超时预算基线：HTTP WriteTimeout=30s、SQL QueryTimeout=30s、前端 Axios 15s、租约 TTL+Grace=40s（`config.go:233-297`）。超过预算的请求单独计数。

### 2.4 测试用例 Benchmark 方案（2026-09-29 增补）

目标：把「验证 DB 的 HTTP 增删改查性能」落成**可执行、可复现、可分析**的用例集。用例按 SQL / Data / KV / 系统库四组覆盖增删改查，每条用例同时回答三件事：

1. **多快**：p50/p95/p99/max（闭环压测，样本规则见 §2.4.3）；
2. **慢在哪**：九段耗时占比（§2.1 stage timing，强制开启）；
3. **为什么慢**：结合 `DB.Stats()`、pprof、S3 操作计数定位到代码位置（§2.4.5 分析流程）。

#### 2.4.1 用例矩阵

统一前置：每条用例在**独立库**上执行（`bench-{group}-{n}`），压测前完成建表/预热写入，避免冷开与建表开销混入稳态数字；冷开行为由 Phase 2 场景叠加单独测量（见下表）。认证统一用测试 API Key（`Authorization: Bearer <key>`）。

**A. SQL 组（query/execute/batch 三端点）**

| 用例 ID | 类型 | 请求（路由 + 载荷） | 验证点 |
|---|---|---|---|
| A1 | 查 | `query {"sql":"SELECT 1"}` | 链路下限：S1–S5 固定开销之和；S7 应 <1ms |
| A2 | 查 | `query` 主键点查（`WHERE id=?`，命中 1 行） | 热路径典型读；对比 A1 得出 catalog/租约之外的开销 |
| A3 | 查 | `query` 范围扫 100 行（索引列 `BETWEEN`） | S7 扫描开销基线 |
| A4 | 查 | `query` 全表扫 100k 行（maxRows=1000 截断） | S7 扫描 vs S8 序列化占比翻转点 |
| A5 | 增 | `execute INSERT INTO bench_sql VALUES (?,... )` 单行 | 单写端到端；S6 WriteGate 首写 vs 后续写差异 |
| A6 | 改 | `execute UPDATE ... WHERE id=?` 单行（先插入固定行数） | 写放大：onWrite→MarkDirty 是否每次推进 snapshot |
| A7 | 删 | `execute DELETE ... WHERE id=?` 单行 | 同 A6；验证删除是否触发相同同步链路 |
| A8 | 增(批) | `batch` 10 条 INSERT × transactional on/off | 两种模式开销差；非事务逐条 onWrite 次数 |
| A9 | 增(批) | `batch` 100 条 INSERT × transactional on/off | 上限边界；S3 校验开销随条数线性度 |
| A10 | 边界 | 并发 8/32/64/65/128 压 A2 与 A5 | semaphore 429 比例；MaxOpenConns=1 排队曲线（§2.4.3） |

**B. Data 组（文档六端点，database-scoped 路由）**

| 用例 ID | 类型 | 请求 | 验证点 |
|---|---|---|---|
| B1 | 查 | `GET .../data/collections`（0 表 / 200 表两档） | information_schema 查询成本随表数变化 |
| B2 | 查 | `GET .../data/collections/docs`（100 / 1000 / 1001 行） | 1001 行必现 `row_limit_exceeded` 契约；每行 JSON 反序列化开销 |
| B3 | 增 | `POST .../documents`（100B 文档 ×N） | CreateDocument 两次 Execute（幂等 DDL + INSERT）实测开销；对比 A5 单 Execute 的差值 |
| B4 | 改 | `PUT .../documents/:id`（10KB 文档） | 大 body 解码 + UPDATE；两次 GetDatabase（systemGuard+acquire）叠加 |
| B5 | 删 | `DELETE .../documents/:id` | 最短写路径；与 A7 对比得出固定开销占比 |
| B6 | 边界 | B3 并发 8/32/64 ×100B 文档 | 写路径排队；幂等 DDL 在并发下是否退化（锁等待） |

**C. KV 组（`POST /v1/projects/:p/kv` 单端点）**

| 用例 ID | 类型 | 请求体 | 验证点 |
|---|---|---|---|
| C1 | 查 | `{"type":"cmd","argvs":["GET","k1"]}` | 热读下限；HasSchema 前置探测开销 |
| C2 | 查 | `{"type":"cmd","argvs":["HGETALL","h1"]}`（100 field） | 读放大：多行扫描 + JSON 组装 |
| C3 | 增/改 | `{"type":"String","args":{"key":"k1","value":"v1"}}` | kv.Store 自管事务 + NotifyWrite 链路 |
| C4 | 增/改 | `{"type":"cmd","argvs":["SET","k1","v1"]}` 与 C3 对比 | cmd 与 typed 两路径差（parse/分类开销） |
| C5 | 增 | `{"type":"cmd","argvs":["INCR","counter"]}` ×100 | 自管事务读-改-写在单连接下的串行化 |
| C6 | 删 | `{"type":"cmd","argvs":["DEL","k1"]}` | 删除路径；TTL 元数据表联动 |
| C7 | 查 | `{"type":"cmd","argvs":["LRANGE","list1","0","-1"]}`（100 元素） | List 编码扫描成本 |

**D. 系统库桥接组（admin 项目，只读对照）**

| 用例 ID | 类型 | 请求 | 验证点 |
|---|---|---|---|
| D1 | 查 | admin 下 `query "SELECT count(*) FROM sys_databases"` | `systemLeaseAdapter` 直连常驻连接（无 registry/无租约）与用户库 A2 的端到端差 = 租约+单连接架构成本 |
| D2 | 查 | admin 下 Data `GET collections` | 桥接路径与 B1 对比 |
| D3 | 写(拒) | admin 下 `execute/batch/CreateDocument` | 验证拒绝路径零 DB 开销（只 catalog 判定） |

用例与场景矩阵（§2.3）的组合规则：A/B/C/D 用例默认在「热库 + 本地盘 + 无后台干扰 + 并发 1」跑出基线，再按下表叠加场景维度形成完整执行集：

| 场景维度 | 套用用例 | 对应 Phase |
|---|---|---|
| 冷/温启动（重启进程或 CloseDatabase 后首请求 ×每用例 1 次） | A2、A5、B3、C3 | Phase 2 |
| 假 S3 50ms/200ms 注入 | A2、A5、B3、C3 | Phase 2 |
| 并发 8/32/64/128 | A2、A5、B3、C5 | Phase 3 |
| 后台干扰（RowCounts 刷新 / CatalogSyncer COPY / KV TTL sweeper） | A2、A5 | Phase 3 |
| 200 库规模 | 列表端点 + A2 | Phase 2 |

#### 2.4.2 Benchmark 实现分层

按 §2.2 的 harness 分两层实现，源码落 `internal/api/perfbench/`（独立 package，build tag `perf`，不进生产二进制、不被 `cmd/simplebased` import）：

1. **HTTP 语义层（主要承载）**：`harness.go` 用 `httptest.NewServer` 包真实 Router + 真实 catalog/registry/ducklake factory（本地盘 DevMode；假 S3 用 objectstore 内存实现注入固定延迟）。用例函数签名固定为：

   ```go
   // bench_case.go — 每条用例一个函数，A/B/C/D 前缀与 §2.4.1 矩阵一一对应。
   type benchCase struct {
       id     string                   // "A2" / "B3" ...
       desc   string
       setup  func(t TB, env *benchEnv) // 建库建表/预热（不计入样本）
       req    func(env *benchEnv, i int) *http.Request
       check  func(t TB, resp *http.Response, body []byte) // 状态码 + 契约断言（row_limit_exceeded 等）
   }

   func runCase(b *testing.B, bc benchCase, opt benchOptions)
   // benchOptions: concurrency（b.RunParallel 并发或固定 worker）、stageTiming 强制开启、
   //               samples、s3Latency、warmup。
   ```

   运行入口同时提供 `TestMain` 风格（`-run TestPerfCase_A2`，供 CI 冒烟）与 `Benchmark` 风格（`-bench BenchmarkCase_A2`，供 `-count=10` 稳态采样）。

2. **纯函数微基准层（辅助归因）**：`go test -bench` 覆盖 sqlguard.Validate、serializeRows、kv 命令 parse、CreateDocumentSQL 拼接等纯 CPU 段，与 HTTP 层数字互证——若 HTTP 层 p99 远大于各纯函数段之和，差额必在 S1–S6/S9 的 I/O 与等待段。

#### 2.4.3 采样与统计规则

- 稳态样本：每用例 ≥500 请求或 ≥30s（与 §4 验收一致）；`-count=10` 取各次 p50/p95/p99 的中位数报告，同时报告 min/max 以暴露抖动。
- 并发用例：固定 worker 数闭环（每 worker 固定请求数），不用开环泊松注入（排除到达过程噪声，聚焦服务端行为）。
- 输出直方图：不引入新依赖，perfbench 内实现简化 HDR（桶宽 1ms→8ms→64ms 对数桶），落盘 CSV/JSON 到 `output/perf/<date>/<case>.json`（字段：case、scenario、concurrency、latency 分位数、错误码计数、各段耗时分位数、DB.Stats 快照）。
- 冷开用例：每轮重建环境（新进程或 CloseDatabase），重复 ≥10 轮取分布，报告**分布而非均值**（冷开方差大）。
- 环境固定：`GOMAXPROCS`、CPU 型号、DuckDB 缓存目录清空策略写进报告头部；跑基准期间禁跑其他测试。

#### 2.4.4 基线参照与预算

- 每组用例附**原生基线**对照：A 组附 `database/sql` 直连 DuckDB 文件（绕过 HTTP/租约）同 SQL 的耗时；B 组附等价两条 SQL 的直连耗时；KV 组附直连 `kv.Store` 耗时。HTTP 层耗时 − 直连耗时 = **架构固定成本**，这是「性能问题分析」的核心分母。
- 预算标尺（超出即在报告中标红并进入 Top-N）：热读 p99 ≤50ms（本地盘）、热写 p99 ≤100ms（本地盘，不含 sync_on_commit）、冷开 p95 ≤5s（本地盘）/ ≤15s（假 S3 200ms）、并发 64 下 p99 相对并发 1 劣化 ≤10×。

#### 2.4.5 性能问题分析流程（每条异常用例执行）

1. **分段归因**：从 stage timing 直方图取该用例 p99 的段占比，定位最大段；
2. **排队分离**：若 S7 占比高，读 `DB.Stats().WaitDuration` 增量——排队占比 >30% 判定单连接瓶颈，否则是 SQL 执行本身；
3. **CPU 剖析**：`-cpuprofile` 重跑单用例（并发 32），`go tool pprof -top` 找热点函数，对照 §1 表的代码位置列；
4. **S3 操作计数**：假 S3 实现记录每用例的 GET/PUT/LIST 次数与字节，冷开/写用例必须给出「每请求 S3 请求数」；
5. **结论落盘**：写入 Top-N 列表——问题、实测数字、复现命令、疑似代码位置、建议优化方向（不实施，另立项）。

#### 2.4.6 用例清单文件

用例矩阵以机器可读形式同步维护一份 `internal/api/perfbench/cases.json`（字段：id、group、route、payload 模板、setup 步骤、断言），harness 启动时加载并**校验与代码内注册的用例一致**，防止矩阵文档与实现漂移。

## 3. 分阶段执行

### Phase 0 — 基线打通（1 天）

- 落地 §2.1 分段计时 + §2.2 harness 骨架；`go build ./... && go test ./internal/...` 保持全绿。
- 用 1 库/1 表/1k 行跑通 SQL query/execute/batch、Data 六端点、KV SET/GET，确认各段耗时总和与端到端误差 <5%，否则补齐观测盲区后再进入正式测量。

### Phase 1 — 热路径基线（本地盘，无后台干扰）

- 目标：拿到「一切顺利」时的下限与段占比。
- 执行 §2.4.1 用例矩阵 A1–A9、B1–B5、C1–C7、D1–D3（并发 1 基线 + A10/B6 并发档），并补 §2.4.4 原生直连基线对照：
  - SQL：A1 `SELECT 1` 已含链路下限；A2 点查、A3/A4 扫描对比；A5–A7 增/改/删；A8/A9 batch 10/100 条 × transactional 开/关。
  - Data：B1 ListCollections（0/200 表）、B2 ListDocuments（100/1000/1001 行——验证 1001 行必现 `row_limit_exceeded` 的契约行为）、B3 CreateDocument（验证两次 Execute 的开销：createCollectionSQL 幂等 DDL + INSERT，及两次 onWrite 是否各推进一次 snapshot）、B4/B5 Update/Delete。
  - KV：C1–C7 覆盖 GET/HGETALL/SET/INCR/DEL/LRANGE 及 typed vs cmd 双路径。
  - 系统库：D1–D3 桥接对照。
- 产出：每端点 p50/p95/p99 + 九段耗时占比表 + 「HTTP 层耗时 − 直连基线」架构成本列；标注与静态假设（§1 表）不符之处。

### Phase 2 — 冷开与租约（本地盘 + 假 S3）

- 冷开分解：重启进程/逐库 CloseDatabase 后对 A2/A5/B3/C3 各首请求执行 §2.4.3 冷开采样（≥10 轮），分解 S6 为 manifest 发现 GET 次数与耗时、catalog SQLite 下载、DuckDB boot SQL（INSTALL/LOAD 扩展、ATTACH、set_option×3）、Ping/版本断言；对比本地盘与假 S3（50/200ms 注入延迟）；报告每请求 S3 操作数（§2.4.5 第 4 步）。
- 温启动：本地 catalog 存在仅重开 DuckDB 的耗时（隔离 S3 因素）。
- 首次写租约：epoch 水位 1/100/1000 时 `WriteGate` 的 GET 次数与耗时（验证 `lease.go` 线性探测假设）；他人持租时请求内等待实测，记录 15s 前端超时后服务端是否仍完成提交（后台写不停）。
- 200 库列表：列表接口耗时 + 触发 RowCounts 后台刷新后 5s 内 SQL/Data 端点的 p95 变化（验证后台统计抢占单连接的干扰假设）。

### Phase 3 — 并发与竞争（本地盘 + 假 S3）

- 单连接排队：A10 用例并发 1→128 压同一热库，绘制「并发 vs p50/p95/p99」曲线；从 `DB.Stats()` 读出 WaitCount/WaitDuration，计算排队占比；验证 MaxOpenConns=1 下吞吐是否随并发平坦化。
- semaphore 边界：并发 64/65/128 时 `query` 的 429 比例与延迟（`acquireSem` 是非排队拒绝）。
- COPY 干扰：写循环触发 CatalogSyncer sync 期间打只读 query，测量 p99 抬升；对比 debounce 与 sync_on_commit 两种模式（后者验证 S9 进入请求关键路径的实测代价）。
- 系统库争用：高频写系统库（模拟 cron/审计）时，auth+project 解析（S1+S2）与 admin 只读查询的延迟变化。
- 混合负载：70% query / 20% execute / 10% batch(10) 持续 5 分钟，记录慢请求直方图与各段漂移。

### Phase 4 — 报告与结论

- 输出 `plan/planv3.0/api-db-perf-baseline-report.md`：
  1. 每端点 × 每场景的 p50/p95/p99/max、错误率、吞吐（数据源：`output/perf/` 下 §2.4.3 规定格式的 JSON）；
  2. 九段耗时占比热力表（端点 × 阶段）；
  3. **主要耗时 Top-N 排序**（按 §2.4.5 流程产出），每条给出：实测数值、复现条件与命令、代码位置、建议优化方向（只建议、不实施，优化另行立项）；
  4. 与 §1 静态假设的对照表（证实/证伪）；
  5. 「HTTP 层 − 直连基线」架构固定成本表（§2.4.4），按端点列出；
  6. 建议 SLO 草案（按热/冷、本地/S3 分级，不设单一数字）。

## 4. 验收标准

- 所有测量可在干净 checkout 上一键复现：`go test ./internal/api/perfbench/ -tags perf`（TestPerf 冒烟）/ `-bench=. -benchtime=...`（稳态采样）（或等价 make 目标）；k6 脚本附运行说明。
- §2.4.1 用例矩阵全部落地且与 `cases.json` 一致（§2.4.6 校验通过）；每条用例的契约断言（状态码、row_limit_exceeded、429 等）在代码中断言，不只靠人工看数。
- 每端点至少覆盖「热库本地盘」「冷库假 S3」「并发 64」三个场景；每场景样本 ≥500 请求或 ≥30s（冷开用例按 §2.4.3 取 ≥10 轮分布）。
- 九段耗时分解覆盖率 ≥95%（未归因时间单独列出，不允许归入「其他」后忽略）。
- 每组端点附 §2.4.4 直连基线对照；报告中的每个 Top-N 结论都有原始数据（`output/perf/` 下 JSON/CSV）支撑；无实测数据不写入结论。
- 测试期间 `go test ./...` 保持通过；新增代码不改 `go.mod`、不改 config 结构（仅追加字段）、handler 测试继续走 fake service；perfbench 包不进 `-tags ''` 默认构建。

## 5. 边界与禁止项

- 本计划**只测量，不优化**；发现的问题写入报告与后续优化 plan，不在本计划分支内改实现（计时中间件除外）。
- 不绕过 registry 直连 DSN、不为压测调大 `MaxOpenConns(1)`、不关闭 sqlguard——测的是真实路径。
- 压测数据不落审计/日志正文；SQL 只记录 SHA-256 与元数据（沿用 `sqlSHA256`）。
- 假 S3 结论必须标注「注入延迟模拟」；真实 S3/COS 数字单独授权后补充，不与本地数字混排。
- 不改 `.gitignore`/`package.json`/CI；perfbench 包不得被 `cmd/simplebased` import。

## 6. 触点清单

- 观测新增：`internal/api/stage_timing.go`（已落地）、`internal/api/perfbench/{harness,loadgen,bench_case,stage_report,cases.json}`、`internal/observability/metrics.go`（buckets/新指标）、`internal/config`（开关字段）。
- 被测路径（只读引用）：`internal/api/{sql_handler,data_handler,kv_handler,database_handler,db_stats,db_stats_cache,adapter}.go`、`internal/database/{query,serialize}.go`、`internal/database/registry/{registry,handle}.go`、`internal/database/ducklake/{factory,catalog_syncer,manifest}.go`、`internal/database/lease/lease.go`、`internal/auth/middleware.go`、`internal/app/app.go`。
- 报告产出：`plan/planv3.0/api-db-perf-baseline-report.md`、`output/perf/`（原始数据）。


## 7. 首轮压测问题分析与优化方案（2026-09-29 增补）

> 数据来源：`./build.sh perf --http --db --kv -c 1 -n 20 --warmup 3`（本地盘 DevMode，darwin/arm64 14C，go1.26.1），原始 JSON 见 `output/perf/2026-09-29/`。
> 样本量仅 n=20、c=1，**只用于定位方向，不作为基线结论**；所有「预期收益」为基于链路推算的估计值，须按 §7.5 复测确认。
> 按 §5 约定：P0 测量修正在本计划内完成；P1 起的实现优化放在后续优化分支/plan，本节只给方案与验收口径。

### 7.1 数据重读：先修正首轮的错误解读

首轮报告里「auth 占 24–43%」**是测量假象，不是真实热点**。证据与原因：

- 每个 `/v1` 用例都满足 `auth% ≈ total%`（如 KV-SET `auth=42.2% total=42.2%`、DB-DOC-LIST `auth=36.9% total=37.0%`）。
- `timedAuthMiddleware`（`internal/api/stage_timing.go:242`）在 `wrapped(c)` 前后计时，而 `wrapped = inner(next)` 会**同步执行后面的整条链路**（project → handler → DB）。所以 auth 段 ≈ 端到端耗时。
- 连带问题：每请求 `已归因之和 ≥ total`，`unattributed = total − attributed` 为负后被截成 0。`/v1` 路由的 unattributed 实际失效；报告里非零的 unattributed 主要来自 §7.2 所说的 `/metrics` 抓取污染。
- perfbench 算段占比时把 `total` 也放进了分母，所以所有百分比都被摊薄约一半。

去掉 auth、按 `total` 重新归一化后的真实分布（占端到端比例）：

| 用例 | p50 ms | project_resolve | catalog_lookup | db_exec | on_write | 其余（auth 实耗 + 框架 + 日志，未直接测到） |
|---|---|---|---|---|---|---|
| HTTP-SELECT1 | 19.9 | 20.3% | 46.1% | 0.5% | — | ≈33% |
| DB-SELECT | 29.1 | 10.5% | 24.8% | 13.7% | — | — |
| DB-UPDATE | 31.6 | 10.3% | 21.7% | 17.8% | — | — |
| DB-BATCH（10 条） | 35.0 | 18.2% | 36.4% | 21.7% | — | — |
| DB-DOC-LIST | 31.1 | 17.8% | 41.1% | 11.7% | — | — |
| KV-GET | 30.9 | 8.4% | 22.1% | 未计时 | — | — |
| KV-SET | 54.8 | 10.6% | 23.5% | 未计时 | 3.0% | — |
| KV-INCR | 56.2 | 8.9% | 23.2% | 未计时 | 2.6% | — |

由此换算单次系统库点查的耗时：SELECT 1 的 `project_resolve`（1 次查询）≈ 4.0ms，`catalog_lookup`（2 次查询）≈ 9.2ms，即**每次约 4–4.6ms**。

### 7.2 问题清单（现象 → 根因 → 优化建议 → 预期 → 验证）

#### 问题 M1：分段计时有三处系统性偏差（测量层，P0）

- **现象**：见 §7.1，auth 包含下游全部耗时；unattributed 失效；占比被摊薄；KV 没有 db_exec；ListDatabases 的 catalog 查询没有计时。
- **根因**：
  1. `timedAuthMiddleware` 的计时范围包住了 `next`。
  2. perfbench 的 `stageDelta` 把 `total` 计入分母；前一次 `/metrics` 抓取请求本身也被计入下一次快照的增量（该请求的 unattributed = 它自己的 total），而且没有按用例 route 过滤。
  3. KV 走 `Raw()` 自管事务，绕过了 `timedLease`；`dbServiceAdapter.ListDatabases` 没有包 `StageCatalog`。
- **优化建议**：
  - auth 只计认证本身：每个请求构造 `inner(func(c) error { scope.Done(); return next(c) })`，进入 `next` 前结束计时。认证失败时由外层补一次 `Done`（`Done` 幂等）。补单测：下游 sleep 50ms，auth 段必须 < 5ms。
  - perfbench 改为进程内读 `prometheus.Gatherer`（harness 已持有 registry），不再发 HTTP 抓取。占比统一以 `total` 为分母、不含 total 本身；按用例 `Endpoint` 对应的 route 过滤；排除 `/metrics`、`/health/*`。
  - KV 分发器在事务执行外包 `StageDBExec`（commit 单独记一个子段 `db_commit`，便于拆出写放大）；`ListDatabases` 包 `StageCatalog`；列表的 `enrichList` 计入 `serialize`。
- **预期**：九段覆盖率从「不可判定」提升到 §4 要求的 ≥95%，unattributed 恢复意义。
- **验证**：`HTTP-SELECT1` 的 `auth + project + catalog + db_exec + unattributed ≈ 100%`（±2%）。

#### 问题 P1：每请求 4 次串行的系统库点查，是固定开销的主体（P1，收益最大）

- **现象**：`SELECT 1` 的 p50 为 19.9ms，DuckDB 执行只有约 0.1ms（0.5%）；project + catalog 两段就占 66%，再加推算约 4–5ms 的 auth，**约 85–90% 的耗时是元数据查询**。所有 `/v1` 用例都至少付出约 17ms 的底价。
- **根因**（逐次列出，都打在同一个系统 DuckLake 上）：
  1. `auth.Service.Authenticate` → `FindByHash`：`sys_api_keys JOIN sys_projects`，每请求 1 次，无缓存（`internal/auth/service.go:68`、`api_key_repository.go:24`）。
  2. `projectContextMiddlewareEcho` → `GetProjectTenant`：1 次（`database_handler.go:305`）。
  3. `catalog.GetDatabase` → `ensureProjectAccess`：持有 `ProjectAdmin` 的 Key 会**再查一次 `GetProjectTenant`**，与第 2 步重复（`catalog/service.go:397-408`）；随后 `repo.GetDatabase` 再查 1 次。
  4. KV 路径 `GetKVDatabase` → `ProjectBelongsToTenant` + `GetDatabaseByName`：2 次，其中归属校验与第 2 步重复（`catalog/service.go:98-117`）。
  5. 系统库是 DuckLake 表，没有索引，点查实际是小表扫描加 catalog 元数据解析，单次约 4ms，远高于 SQLite/内存点查的微秒级。
- **优化建议**（按投入产出排序）：
  1. **去掉重复查询（零风险）**：project 中间件已解析出 `ProjectContext{ID, TenantID}`。`ensureProjectAccess` 与 `GetKVDatabase` 改为接收或读取这个已解析的 tenant，比对 `principal.TenantID` 即可，不再查库。每请求少 1 次查询（约 4ms，约 20%）。
  2. **API Key 认证缓存**：在 `auth.Service` 内缓存 `keyHash → Principal`，TTL 30s，LRU 上限 10k。吊销/删除 Key 时本进程主动失效；多实例靠 TTL 兜底（最坏 30s 吊销延迟，写入安全说明）。无效 Key 做 5s 负缓存，并按 IP 限流，防止用随机 Key 打穿缓存。JWT 通道的 `PrincipalFromClaims` 同样处理（用户禁用时主动失效）。
  3. **project→tenant 缓存**：项目创建后 tenant 不可变，只需在删除项目时失效。用进程内 `sync.Map` 加上限即可。
  4. **database 行缓存**：key 为 `(projectID, databaseID)`，值包含 status/kind。catalog Service 是写入唯一入口（单写实例 + per-db 租约），所以 `TransitionDatabase/SetDatabaseReady/Delete` 成功后同步失效，保证本实例一致；只读实例用短 TTL（2–5s）。KV 的 kind=kv 行按 projectID 缓存。
  5. **（远期）** 热元数据（keys/projects/databases）换成内存镜像或系统库原生 DuckDB 表加索引，DuckLake 只作持久化与同步。本轮不做，等 1–4 落地后看剩余占比再决定。
- **预期**：命中缓存后每请求系统库查询 4 → 0，`SELECT 1` 的 p50 从约 20ms 降到 **≤3ms**；DB/KV 读用例 p50 下降 15–17ms（约 50–60%）。
- **验证**：新增 `HTTP-SELECT1-HOT` 用例（预热后命中缓存）；`project_resolve + catalog_lookup + auth` 合计 < 10%；另测吊销 Key 后 ≤TTL 内返回 401 的契约用例。

#### 问题 P2：系统库单连接是全实例的串行点，决定吞吐天花板（P1，并发下最危险）

- **现象**：c=1 下 `acquire=0%`、没有 `db_conn_queue`，问题尚未暴露。但按 P1 推算，每请求占用系统库约 16ms，全实例 QPS 理论上限约 `1000 / 16 ≈ 60`，**与用户库数量无关**。
- **根因**：`ducklake/factory.go:90` 对系统库同样 `SetMaxOpenConns(1)`。认证、project、catalog 读，访问日志/指标写，后台 RowCount 刷新、cron、agent 调度全挤在这一条连接上。
- **优化建议**：
  1. 先落地 P1 缓存，把请求路径上的系统库读从 4 次降到约 0 次（治本）。
  2. 系统库读写分离：写（日志/指标 flush、catalog 变更）保留单连接；只读元数据查询单独开只读连接池（同一 DuckDB 实例多连接，MVCC 读不阻塞写，2–4 个连接）。需验证 DuckLake 在同进程多连接下 catalog 的可见性语义，先在测试中确认。注意这是**优化方案**，与 §5 的「压测期间不得调大 MaxOpenConns」不冲突。
  3. 为系统库补 `db_conn_queue` 观测：系统库查询也包一层 Stats 差值计时，新增 `system_db_queue` 段。
- **预期**：c=32 时系统库排队占比 < 5%，QPS 随 CPU 线性扩展，直到被用户库单连接限制。
- **验证**：Phase 3 跑 `-c 8/32/64`，对比优化前后 QPS 曲线的拐点，以及 `system_db_queue` 的 p99。

#### 问题 P3：长尾 max 300–560ms，来自请求线程同步 flush 系统日志/指标（P1）

- **现象**：几乎每个用例都有一次 300–560ms 的 max（HTTP-SELECT1 318ms、DB-SELECT 497ms、KV-GET 555ms），而 p95 通常只有几十毫秒。离群点和操作类型无关，大约每用例出现 1 次，符合**周期性事件**的特征。
- **根因**：`accessLogMiddleware`（`router.go:525-545`）每个 `/v1` 请求调用 1 次 `RecordLog`、2 次 `RecordMetric`。缓冲达到 64 条时，`RecordLog/RecordMetric` **在当前请求的 goroutine 里同步执行** `FlushLogs/FlushMetrics`（`systemdb/logs.go:60`、`metrics.go:51`）：逐条 64 次 `INSERT`、每条一个 DuckLake 事务，然后 `notifyWrite → AfterWrite`（catalog 同步标记）。按每请求 2 条指标算，约每 32 个请求就有一个请求要承担几百毫秒的 flush，同时还独占系统库单连接，阻塞其他请求的认证查询（与 P2 叠加）。另外 `StartPeriodicFlush` 每 2s 触发一次，也会抢这条连接。
- **优化建议**：
  1. **请求路径永不 flush**：`Record*` 只做无锁/短锁入队；缓冲满时非阻塞通知后台 flusher（`chan struct{}` 容量 1）。队列设硬上限（如 10k），超出就丢弃并计数 `simplebase_system_log_dropped_total`，保证请求路径 O(1)。
  2. **批量写**：flusher 在一个事务里用多行 `INSERT ... VALUES (...),(...)` 或 DuckDB Appender 写入整批，事务数从 64 降到 1；`notifyWrite` 每批只调用 1 次。
  3. **指标预聚合**：`http_requests / http_latency_ms / http_errors` 在内存按 `(project, name, 时间桶)` 聚合（count/sum/max），每个 flush 周期每项目只写 1–3 行，写入量约降 100 倍。`/metrics/summary` 与 `trend` 的查询口径同步改为 sum(count)。
  4. 访问日志支持采样或只记非 2xx（配置项，默认全量，保持现有行为）。
- **预期**：用例 max 从 300–560ms 降到与 p99 同一量级（< 2×p99）；并发下认证查询不再被 flush 阻塞。
- **验证**：n ≥ 500 时 `max/p99 < 2`；新增 `system_flush_seconds` 直方图，确认 flush 不在请求 goroutine 执行（pprof goroutine 标签或单测断言）。

#### 问题 P4：写路径绝对延迟偏高（KV 写比读贵约 24ms，SQL 写 p50 25–50ms）（P2）

- **现象**：KV-SET 54.8ms 对 KV-GET 30.9ms，KV-INCR 56.2ms；KV-DEL（SET+DEL 两次写）104.9ms，约为单写的 2 倍，说明成本和写事务次数成正比。DB-INSERT 两轮 p50 分别为 41.6ms 和 25.3ms，波动大。`on_write` 只占 1–3%，说明 debounce 模式下 `MarkDirty` 本身很便宜。
- **根因（待 profile 确认）**：每个写语句都是一次独立的 DuckLake 事务。DuckLake commit 要在 catalog 元数据库里写 snapshot、`snapshot_changes`，以及小数据量时的内联数据行，本地单行写的主要成本在这里。另外 `CreateDocument` 每次都先跑一遍幂等的 `CREATE TABLE IF NOT EXISTS`（两次 Execute，两次元数据事务）。
- **优化建议**：
  1. **先测再改**：对 KV-SET / DB-INSERT 采集 CPU + block profile（perfbench 增加 `-pprof` 开关），把 DuckLake commit、catalog 写、`AfterWrite` 分开计量；配合 M1 的 `db_commit` 子段。
  2. **Data 建表缓存**：DataHandler 在进程内缓存「`(databaseID, collection)` 已存在」，命中后跳过 DDL，每次文档写少一个元数据事务。删除集合/删库时失效；DDL 失败回退到原路径。
  3. **KV 组提交（group commit）**：同一项目 KV 的并发写在 ≤1ms 窗口内合并为一个事务（每库本来就是单连接串行，合并不改变语义；每条命令的结果按原顺序返回，失败时整组回退为逐条重试）。c=1 无收益，c≥8 时写吞吐预计提升数倍。
  4. 对外文档引导批量写：SQL 用 `batch`（DB-BATCH 10 条 p50 35ms，折合每条约 3.5ms，比单条 INSERT 快约 10 倍），KV 用 `MSET/HSET` 多字段。
  5. 评估 `data_inlining_row_limit` 对小写入的影响（本地 dev=100），在假 S3 场景对比 0/100/1000 三档（数据面优化，需另外评估一致性影响）。
- **预期**：Data 写每次少约 1 个事务（估计 −30%）；KV 写在并发下吞吐提升；单条写 p50 的最终目标由 profile 结果确定。
- **验证**：KV-SET 与 KV-GET 的差值、DB-DOC-CREATE 与 DB-INSERT 的差值在优化前后的变化；新增 c=16 的 KV-SET 用例。

#### 问题 P5：健康检查不是「廉价」探针（P2）

- **现象**：HTTP-HEALTH（`/health/ready`）p50 为 6–17ms。这个请求没有认证、没有业务逻辑，理论上应 < 1ms。
- **根因**：`healthService.Ready`（`app/app.go:100`）每次都执行 `Ping` + `SELECT COUNT(*) FROM sys_migration_versions`，而且和业务请求抢同一条系统库连接（P2）。负载均衡器高频探测时，会直接吃掉系统库的吞吐。
- **优化建议**：Ready 结果缓存 1–2s（后台 ticker 刷新，失败立即生效，恢复需连续 2 次成功），请求路径只读原子变量；`/health/live` 保持不访问任何依赖。
- **预期**：`/health/ready` p50 < 0.5ms，不再占用系统库连接。
- **验证**：HTTP-HEALTH 的 p99 < 1ms；注入系统库故障后 ≤2s 内 Ready 变红。

#### 问题 P6：数据库列表路径有未计时开销与后台刷新竞争（P3）

- **现象**：HTTP-LISTDB p50 为 22.8ms，未归因 41.9%（M1 修正后需重看）；max 为 360–426ms。
- **根因**：`ListDatabases` 的 catalog 查询没有计时（M1）；新库的 RowCount 缓存未命中时会触发 `RefreshAsync`，后台逐库 `Acquire + COUNT(*)`，与前台请求争用用户库单连接和系统库连接。
- **优化建议**：补计时；确认 `RefreshAsync` 有 singleflight 和并发上限（没有就补上，上限建议 2）；刷新任务降级为低优先级，前台同库有请求时让步。
- **验证**：连续请求列表时，每个库 30s 内最多刷新 1 次（计数指标），列表 p99 不受刷新影响。

### 7.3 压测工具自身问题（P0，与 M1 一起修）

| # | 问题 | 影响 | 修正 |
|---|---|---|---|
| T1 | 默认 n=20 太小，p95 与 p99 是同一个样本 | 分位数无统计意义 | `build.sh perf` 默认 `-n 500`；n < 100 时报告标注 `LOW-SAMPLE` |
| T2 | 复合用例（DOC-UPDATE-DELETE 是 4 个请求，KV-DEL 是 2 个）与单请求用例混排 | 延迟不可横向比较 | 结果增加 `requests_per_sample` 字段，报告同时给出「每样本」和「每请求」两列 |
| T3 | DB-SELECT 空表点查（setup 未播种） | 绕过扫描与序列化成本 | setup 播种 1k 行，断言 `row_count == 1` |
| T4 | `-d` 补测段的 `elapsed` 计算有误（被重设为总墙钟，含两段之间的间隔） | `-d > 0` 时 QPS 偏低 | 分段累计实际执行时长，补测的错误码与 latencies 统一合并 |
| T5 | 所有用例共用一个进程和系统库，前面用例的日志/指标缓冲影响后面用例 | 跨用例干扰，max 位置随机 | 每个用例开始前强制 `FlushLogs/FlushMetrics` 并等待后台任务空闲；或加 `-isolate` 选项，每个用例单独起一个 env |
| T6 | stage 快照走 HTTP `/metrics`，抓取请求本身污染增量 | unattributed 虚高 | 见 M1，改为进程内 Gatherer |
| T7 | 只有 c=1 | P2 类串行瓶颈完全不可见 | `build.sh perf` 增加 `--sweep 1,8,32,64` 输出 QPS–延迟曲线 |

### 7.4 优先级与落地顺序

| 优先级 | 项 | 改动面 | 风险 | 预期收益（估） |
|---|---|---|---|---|
| **P0** | M1 计时修正 + T1–T7 工具修正 | `stage_timing.go`、perfbench | 低（只改观测） | 数据可信，是后续所有结论的前提 |
| **P1-a** | P1.1 去掉重复 tenant 查询 | `catalog/service.go`、KV adapter | 低 | 每请求 −4ms（约 −20%） |
| **P1-b** | P3 日志/指标异步化 + 批量写 + 预聚合 | `systemdb/{logs,metrics}.go`、`router.go` | 中（指标查询口径变化） | 长尾 max −90%；系统库写压力约 −100× |
| **P1-c** | P1.2–P1.4 认证、project、database 缓存 | `auth`、`catalog` | 中（吊销/状态失效语义） | 读请求 p50 −15ms（约 −50–60%） |
| **P1-d** | P2 系统库读写分离 + 排队观测 | `systemdb`、factory | 中高（DuckLake 多连接语义需验证） | 并发吞吐天花板提升 |
| **P2** | P4 写路径 profile → 建表缓存 / KV 组提交 | `data_handler`、`kv_handler` | 中 | Data 写约 −30%；KV 并发写吞吐提升 |
| **P2** | P5 Ready 缓存 | `app.go` | 低 | 探针 < 0.5ms |
| **P3** | P6 列表刷新治理 | `db_stats_cache.go` | 低 | 列表 p99 稳定 |

顺序原则：**先修测量（P0）→ 再做零风险去重（P1-a）→ 治长尾（P1-b）→ 上缓存（P1-c）→ 最后动并发模型（P1-d）**。每一步单独提交，并附优化前后的 `output/perf/` 对比数据。

### 7.4.1 执行记录（2026-09-29，P0 + P1-a/b/c/d 全部落地）

五步已按序完成，全量 `go test ./...` 30 包通过；压测 `go run ./cmd/perfbench -suite all -c 8 -n 300` 16 用例 0 setup-failed / 0 err。

**P0 计时与工具修正（M1 + T1–T7）**

- `stage_timing.go`：auth 段改为「认证成功放行前结算」（`Scoped`/`Settle` 登记
  式 scope），不再吞掉下游链路；新增 `db_commit` 段（KV 自管事务 commit）。
- KV 写事务接入计时（`kv.Store.ObserveCost` 暴露 exec/commit 分段）；
  `ListDatabases` 两条路径（adapter 与 handler 直连）补 `catalog_lookup` 打点。
- perfbench：快照改进程内 `prometheus.Gatherer`（不再发 `/metrics` 请求污染
  统计）；占比分母改为 total 段（排除 total 自身）；`elapsed` 累计补测段真实
  时长；每用例前 `quiesce()` 排空日志/指标缓冲（消除跨用例 flush 长尾）；
  复合用例声明 `requests_per_sample`（报告延迟自动折算单请求）；DB-SELECT
  播种 1000 行并断言命中 1 行；LOW-SAMPLE 标记 n<100。
- 排障增强：`WriteError` 未映射错误原先静默 500，现输出 `unhandled api error`
  日志（本次正是靠它定位 P1-d 回退的根因）。

**P1-a 去重复查询**

- `catalog/tenant_ctx.go`：project 中间件把解析出的 `{projectID, tenantID}`
  注入 ctx；`ensureTenantMatch`/`GetKVDatabase` 命中即免第二次归属点查。

**P1-b 日志/指标异步化（P3）**

- `systemdb/async_flush.go`：后台 flusher（信号合并、满批非阻塞通知），
  请求路径 O(1)；批量单事务 INSERT（64 行 1 事务）替代逐条写；
  热指标（http_requests/errors/latency_ms）按 (project,name) 内存预聚合，
  写入量约 −100×。`Close` 先停 flusher 并排空尾部。race 测试通过。

**P1-c 三级缓存（P1.2–P1.4）**

- `auth/cache.go`：keyHash→Principal，TTL 30s；负缓存 5s 防打穿；
  `RevokeAPIKeyByID` 吊销即全量失效（吊销立即生效契约有测试）。
- `catalog/cache.go`：project→tenant（TTL 5m，删项目失效）、database 行
  （TTL 2s，状态迁移/删除/建库同步失效）、KV 行（TTL 2s）。
  写路径失效点：CreateDatabase / TransitionDatabase(deleting) /
  MarkDatabaseDeleted / SetDatabaseReady / SetDatabaseDegraded。

**P1-d 系统库并发模型（P2）——实测后收敛为「保持单连接 + 排队观测」**

- 实测：放宽 `MaxOpenConns` 后第二连接 boot 失败——DuckDB boot 序列
  （SET memory_limit … SET lock_configuration=true）是连接级语义，
  首连接锁配置后次连接不可重放；改为「boot 只跑一次」又丢失
  `USE lake` search_path。**连接上限回退为 1**，瓶颈改由 P1-c 缓存 +
  P1-b 异步写消除（该结论已写入 `factory.go` 注释）。
- 保留排队观测：`systemdb.Store.observeConnStats` 周期采样
  `*sql.DB.Stats()` 等待增量并告警。

**优化效果（c=8 与首轮对比，同机 DevMode）**

| 指标 | 优化前（首轮基线） | 优化后 | 说明 |
|---|---|---|---|
| HTTP-SELECT1 p50 | ~19.9ms | **0.15ms** | auth 缓存 + 异步日志 |
| HTTP-SELECT1 QPS（c=2） | ~60 | **~11000** | 系统库点查全部命中缓存 |
| DB-DOC-LIST p50 | ~29ms | 5.9ms | |
| 任意用例 max | 300–560ms（日志攒批同步写） | 43–238ms | 长尾源转为 DuckLake checkpoint 本身 |
| auth 段占比 | 24–43%（测量错误） | 2–3%（真实值） | M1 修正后 |

新暴露的问题（转入 §7.4 P2 待办）：DB 写用例 `db_conn_queue` 占比 660%
（计时重叠口径问题，排队观测含租约等待，需拆分租约排队与语句排队）；
KV 写 p50 100ms+（`kv_commit` 独立事务 + NotifyWrite，指向 P4 组提交）；
DB-DOC 系列 stage 全 unattributed（data 路由的 route 模板未匹配上）；
health 2.7ms（P5 Ready 缓存未做）。

### 7.5 复测口径与目标（优化分支验收）

- 固定命令：`./build.sh perf --all -n 500 --warmup 50`，外加 `--sweep 1,8,32`（T7 落地后）；每组跑 3 次取中位数；环境信息随报告落盘。
- 目标（本地盘 DevMode，热库；在 P0 修正后的首轮基线上重新校准）：

| 指标 | 当前（首轮，n=20） | 目标 |
|---|---|---|
| HTTP-SELECT1 p50 | 19.9ms | ≤ 3ms |
| DB-SELECT / KV-GET p50 | 29–31ms | ≤ 12ms |
| 任意用例 max / p99 | 5–15× | < 2× |
| `/health/ready` p99 | 18.7ms | < 1ms |
| 九段覆盖率 | 不可判定 | ≥ 95% |
| c=32 QPS（SELECT1） | 未测（理论约 60） | ≥ 1000 |
| c=32 系统库排队占比 | 未测 | < 5% |

- 每项优化必须附回归证明：`go test ./...` 通过；缓存类优化附失效契约用例（吊销 Key、删库、状态迁移后不返回旧数据）。
