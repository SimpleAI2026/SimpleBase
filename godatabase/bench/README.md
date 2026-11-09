# godatabase 系统库读路径基准

复现并量化「系统库读路径慢」的核心机制：**读放大**（存活文件数 → 查询耗时）。
由原 `benchmarks/dbread` 迁入；属于 `godatabase` 独立模块，只依赖 `godatabase/ducklake`，
不 import 根模块（`internal/systemdb` / `catalog` / `auth`）。

## 与原版的差异

原版经 `systemdb.Bootstrap` 打开系统库并调用 `Store.QueryLogs` / `MetricsSummary` / `auth` 仓储。
`godatabase` 不得 import 根模块，因此本包：

- 直接用 `ducklake.Factory` 打开 `Kind=system` 的库；
- 内联 `sys_log_events` / `sys_metric_samples` / `sys_users` / `sys_databases` 的 DDL
  与 `QueryLogs` / `MetricsSummary` / `GetByID` 的同形 SQL；
- 不含 systemdb bootstrap 的 seed 行与其余 60 余张系统表，因此基线文件数比原版少
  （原版 bootstrap 约 44 个文件）。**绝对数字不可与原基线直接对比，只看趋势。**

内联 SQL 随 `internal/systemdb`、`internal/auth` 演进时需手工同步。

## 为什么不能用 `cmd/perfbench`

`internal/api/perfbench` 测 HTTP 端到端（含 echo / 认证 / registry），噪声源多，
且跑在本地内联形态（`DataInliningRowLimit: 100`），无法量化「存活文件数 → 读取耗时」。

当前生产内联阈值为 **1000**，不是 0；同步前 `ducklake_flush_inlined_data` 仍会生成小文件。
本包把内联阈值作为参数暴露：`Inlining=0` 为最坏情况（每次写入一个文件），
`Inlining=1000` 对照生产设置。本地盘测试不等同远端 COS，**不得把本地数字当生产远端耗时**。

## 运行

在 `godatabase` 模块根执行（`go test` 包，无独立入口）：

```bash
cd godatabase

# 读放大可读报告（文件数 → 查询耗时）
GOWORK=off go test -run='TestReadAmplificationReport' -v ./bench

# 完整基准矩阵
GOWORK=off go test -run='^$' -bench=. -benchtime=20x -timeout 20m ./bench

# 只测写放大
GOWORK=off go test -run='^$' -bench='SingleWriteCost' -benchtime=50x ./bench

# 隔离临时库：首次查询、100 次热读及 1/8/32 并发各 128 次的分位数
GOWORK=off go test -run='^TestReadLatencyPhases$' -v -count=1 ./bench
```

`-short` 会跳过两个报告用例。

## 度量口径

| 指标 | 含义 |
| --- | --- |
| `files` / `live_files` | `__ducklake_metadata_lake.ducklake_data_file` 中 `end_snapshot IS NULL` 的文件数 |
| `live_files_sys_log_events` | 目标表存活文件数，仍不代表实际查询触达文件数 |
| `newfiles/write` | 每次写入新增的数据文件数（写放大） |
| `newsnaps/write` | 每次写入新增的 catalog 快照数 |
| `QueryLogs` | 日志查询（近 24h、limit=200）单次耗时 |
| `MetricsSummary` | 指标条件聚合 + 库计数 + 直方图三次查询的耗时 |
| `UserPointLookup` | `sys_users` 按 id 点查耗时 |

首读指造数完成后的首次同类查询，不是远端 S3 冷缓存。远端请求数与后台同步冲突
须在隔离 COS 前缀另测，不能从本地耗时推断。

## 已知边界

- 测的是「文件数 → CPU / 元数据开销」这一层，远端 COS 的每文件网络往返无法本地复现，
  需按 `plan/planv4.0/ducklake-rw-latency-eventual-consistency-plan.md` P0 在真实实例复测。
- `Inlining=0` 是最坏情况，不是生产配置；不注入 bug、不修改产品代码。
- 历史基线（2026-10-08，原 `benchmarks/dbread`，含 systemdb bootstrap 约 44 个基础文件）见
  `plan/planv5.0/db-read-performance-and-s3-architecture-plan.md`；
  `QueryLogs` 对存活文件数近似单文件线性。
