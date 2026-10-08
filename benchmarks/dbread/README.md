# SimpleBase 系统库读路径基准（dbread）

复现并量化「系统库读路径慢」的核心机制：**读放大**。

## 为什么不能用 `cmd/perfbench`

`internal/api/perfbench` 测的是 HTTP 端到端（含 echo/认证/registry），噪声源多；
更重要的是它跑在**本地内联形态**（`DataInliningRowLimit: 100`，见 `internal/api/perfbench/harness.go`），
无法直接量化「存活文件数 → 读取耗时」的关系。

当前生产 `config.yaml` 的内联阈值为 **1000**，不是 0；同步前调用
`ducklake_flush_inlined_data` 时仍会生成小文件。本目录把内联阈值作为**可调参数**显式暴露：
`Inlining=0` 模拟最坏情况（每次写入一个文件），`Inlining=1000` 对照当前生产设置。
本地盘测试不等同远端 COS 真实请求，**不得将本地数字当生产远端耗时**。

## 运行

本目录是普通包 `dbread`。下面的 `go test` 就是入口。

```bash
# 读放大的可读报告（文件数 → 查询耗时）
go test -run='TestReadAmplificationReport' -v ./benchmarks/dbread

# 完整基准矩阵
go test -run='^$' -bench=. -benchtime=20x -timeout 20m ./benchmarks/dbread

# 只测写放大
go test -run='^$' -bench='SingleWriteCost' -benchtime=50x ./benchmarks/dbread
```

## 度量口径

| 准则 | 含义 |
| --- | --- |
| `live_files` | `__ducklake_metadata_lake.ducklake_data_file` 中 `end_snapshot IS NULL` 的文件数（直接对应一次查询要付的远端往返次数） |
| `newfiles/write` | 每次写入新增的数据文件数（写放大） |
| `newsnaps/write` | 每次写入新增的 catalog 快照数 |
| `QueryLogs` | `systemdb.Store.QueryLogs(limit=200)` 单次耗时 |
| `MetricsSummary` | `systemdb.Store.MetricsSummary` 单次耗时（条件聚合 + 直方图） |
| `UserPointLookup` | `auth` 仓储 `GetByID` 单次耗时（认证路径的固定开销主体） |

## 2026-10-08 基线（Apple M4 Pro，本地盘）

`Inlining=0`，每次写入独立文件；`live_files` 含 bootstrap 基线约 44 个文件：

| 写入轮数 | live_files | 存活文件总字节 | QueryLogs | MetricsSummary |
| --- | --- | --- | --- | --- |
| 1 | 45 | 27.9 KB | 1.379 ms | 14.321 ms |
| 10 | 54 | 40.9 KB | 2.005 ms | 15.054 ms |
| 50 | 94 | 98.8 KB | 3.448 ms | 13.898 ms |
| 150 | 194 | 243.9 KB | 7.066 ms | 13.461 ms |

`QueryLogs` 对 `live_files` 近似**单文件线性**（≈ 35 µs/文件）：

```
QueryLogs ≈ 1.11ms + live_files × 0.032ms        （本地盘，仅 footer + 元数据扫描）
```

`-benchtime=5x` 的同口径点数（含冷启动噪声）：

| 基准 | ns/op |
| --- | --- |
| `LogsQueryDense/dense-10` | 2,147,808 |
| `LogsQueryDense/dense-50` | 4,088,825 |
| `LogsQueryDense/dense-150` | 9,974,875 |
| `LogsQueryInlined`（内联基线） | 2,709,000 |
| `MetricsSummaryDense/dense-10` | 5,452,083 |
| `MetricsSummaryDense/dense-50` | 10,542,483 |
| `MetricsSummaryDense/dense-120` | 13,653,892 |
| `MetricsSummaryInlined` | 4,406,692 |
| `UserPointLookupDense/dense-1..60` | 1,375,967 – 1,501,967（无明显线性趋势） |

写放大（`-benchtime=20x`）：

| 形态 | ns/op | newfiles/write | newbytes/write | newsnaps/write |
| --- | --- | --- | --- | --- |
| `inlining-0`（最坏情况） | 2,731,142 | **1.000** | 1,243 | 1.000 |
| `inlining-1000`（当前阈值） | 2,414,523 | **0** | 0 | 1.000 |

结论：

1. **每次写入必然 1 个新快照**（本基准的两种形态一致）；`inlining=0` 时额外 1 个新数据文件，
   本地样本约 1.24 KB/文件。实际系统库 `sys_log_events` 为 42 个存活文件/482 行，
   文件来源需结合各次 flush、同步与维护进一步核对。
2. **真实 COS 耗时不能从本地文件数直接换算**：远端文件读取可能并发、缓存命中或
   被谓词剪枝。当前日志中 `/logs` 约 3.1s；请在隔离的 COS 测试前缀、相同文件数下，
   以 HTTP 阶段、DuckDB profile 和对象存储请求计数分别验证，不把理论 RTT 累加当实测。
3. `MetricsSummary` 包含指标条件聚合、活跃数据库计数、直方图三次查询；本地基准
   `dense-120` 相比内联约 3.1 倍，但受现有样本、缓存与 SQL 计划影响，
   不能断言每次均读取所有 41 个远端文件。
4. `auth` 用户点查在 1~60 用户的本地热态基准下约 1.4ms，未呈明显线性上升；
   dev 日志里的 `auth` 段 0.8–2.5s **需要进一步分段测量**，可能包含系统库查询等待、
   5s 主体缓存失效后的项目集加载或 DuckDB / COS 的其他开销，不能断言只有一个原因。
   当前代码系统库连接池上限为 **10**，不是「唯一连接」。

## 已知边界

- 这些基准跑在**本地盘**上，测的是「文件数 → CPU/元数据开销」这一层；
  远端 COS 的每文件网络往返无法在本地复现，需按
  `plan/planv4.0/ducklake-rw-latency-eventual-consistency-plan.md` P0 的方法在真实
  实例上复测。
- `Inlining=0` 复刻的是「每次写入独立文件」的最坏情况，不是当前生产配置；
  不注入 bug 或修改产品代码。
