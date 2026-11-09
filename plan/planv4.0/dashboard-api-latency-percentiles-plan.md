# v4.0 监控大盘接口耗时 P50 / P90 / P99 计划

> 日期：2026-10-06
> 状态：**已实施（2026-10-06）**，执行记录见 §6。
> 范围：`internal/api`、`internal/systemdb`、`ui/src/pages/Dashboard.vue`、`ui/src/services` 及相关测试。

## 1. 目标与口径

在当前项目监控大盘增加 **接口端到端耗时 P50、P90、P99**，帮助识别均值掩盖的尾延迟。与现有 `GET /v1/projects/:projectID/metrics/summary` 一起返回；不新增页面、路由或第三方依赖。保留请求数、错误率、平均耗时和活跃数据库等已有字段。

- 窗口：最近滚动 24 小时，与 `MetricsSummary` 当前窗口一致；项目页只统计当前 `project_id`，admin 系统项目统计所有**有项目归属**的请求（沿用现有 summary 规则）。
- 样本：`accessLogMiddleware` 完成时记录的所有带项目上下文的 HTTP 请求，包含成功/失败及鉴权、排队、执行、序列化耗时；与现有 `http_latency_ms` 同一采样时刻和过滤条件。不从被筛选的 `sys_log_events` 推算，亦不把 GoFunction 内部调用耗时混入接口耗时。
- 精度：单位毫秒，记录浮点毫秒，避免 `Duration.Milliseconds()` 截断低于 1ms 的请求；展示值可四舍五入为整数毫秒。Pxx 是**窗口内所有请求**的分位数，而非每批/每分钟 Pxx 的算术平均。
- 暂不按路由或 HTTP 方法拆分大盘分位数；Prometheus 已有按低基数路由/方法聚合的 `HTTPRequestDuration`，本需求需要项目范围、跨进程重启仍可查看的统计。

## 2. 现状与问题

1. `internal/api/router.go` 的 `accessLogMiddleware` 已记录 `http_latency_ms`、`http_requests`，但 `internal/systemdb/async_flush.go` 按 `(project_id, name)` 将延迟**求和并附带 count**后写入 `sys_metric_samples`。现有 `MetricsSummary` 对求和行取 `AVG`，并非严格的请求加权均值；**仅靠 sum/count 无法还原 P50/P90/P99**。
2. `internal/observability/metrics.go` 的 Prometheus 直方图只有 `route/method` 标签，禁止把 projectID 放进标签；不可直接当成项目级大盘数据源。日志中间件对正常快请求有筛选，日志样本存在选择偏差。
3. `internal/systemdb/metrics.go` 的 summary 已有 10s TTL + 单飞缓存；`ui/src/pages/Dashboard.vue` 在“请求趋势”卡片描述里显示均值，通过 `ui/src/services/http-api.ts` 的 `toMetricsSummary` 映射 `types.ts` 类型。新增字段沿用这个链路，不额外并行发查询请求。

## 3. 方案

### 3.1 采集与持久化

在现有 `RecordMetric` 热指标预聚合中增加一种 `http_latency_histogram_ms` 内部样本类型：请求完成时按项目更新内存中**固定边界、累计桶计数**及总数；与现有日志/指标后台 flush 一起写入 `sys_metric_samples`。持久化采用**每项目每批一行**：`name='http_latency_histogram_ms'`，`labels_json` 是版本化的紧凑桶计数 JSON，`value_double` 是本批请求总数，`occurred_at` 是刷盘时间。不改表结构、不写每请求明细，也不将项目 ID/请求 ID 写入 Prometheus label。选择固定边界并在代码中声明和测试，例如：`[1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 30000, +Inf]` 毫秒；超过最高有限桶的长尾需单独记录最大值或者在 UI 标为 `≥30s`，不可把 `+Inf` 当成数值返回。

写路径沿用 `Store` 的缓冲与异步通知，不阻塞请求，不额外触发写事务；错误、flush 失败时与现有指标保持同样的重入队/重试语义，合并时**相同桶逐桶相加**而不是覆盖。确保 `FlushMetrics`、`flushAllAsync`、`Close` 等所有 drain 路径均持久化该桶；按现有 flush 周期写入，避免一批延迟样本膨胀成十余行。无项目上下文的 `/health` 等沿用当前不计入 summary 的行为；不从现有聚合求和样本逆推分位数，也不改变既有 Prometheus 指标。

### 3.2 查询、计算与 API

`MetricsSummary` 继续在 24h 时间窗口内按项目过滤，额外读取这一类直方图行并逐桶加总（严格限定 `name`，避免加载其他指标）。使用累积桶计数确定每个目标分位数所在桶，给出**桶内线性插值估计**；确保每次合并验证 JSON 版本、边界数组长度、计数非负/有限，总计数与 `value_double` 一致。不合格行不可默默混入结果，需按项目约定返回可定位的错误或显式降级并记录异常；查询及扫描错误不可像当前 summary 那样吞掉后返回全 0。

建议扩展同一个 JSON：

```json
{
  "total_requests": 1234,
  "error_rate": 0.012,
  "avg_latency_ms": 42.3,
  "active_databases": 3,
  "latency_p50_ms": 18.5,
  "latency_p90_ms": 95.1,
  "latency_p99_ms": 440.0,
  "latency_sample_count": 1200
}
```

无直方图样本（老数据、刚升级、低流量项目）时 `latency_p*_ms` 返回 `null`，`latency_sample_count=0`，不能误报 `0ms`；历史数据不回填，窗口覆盖满 24h 后口径自然稳定。窗口含部分新样本时 `latency_sample_count` 明示分位数覆盖量，防止与 `total_requests` 混淆。`+Inf` 桶命中时不要返回 Infinity/NaN JSON，按约定返回可表示的下界并向 UI 标注 `≥`；所有字段为近似统计值。评估并顺手修正现有平均耗时：新数据应以 `SUM(http_latency_ms) / SUM(http_latency_ms_count)` 计算而不是 `AVG(sum 行)`，明确旧逐请求历史行兼容策略与验证样例，不改动请求数/错误率原有口径。

### 3.3 前端

在 `MetricsSummary` 类型、`toMetricsSummary` 映射、`services/mock.js` 与测试 mock 中新增分位数和样本数；使用数值或 `null`，不把缺字段转成 0。`Dashboard.vue` 的“请求趋势”区域展示 P50/P90/P99 三个紧凑统计值（`ms`、统一 24h 标签），保留现有趋势图及资源汇总；无数据呈 `—` + “暂无近 24 小时接口样本”，超量程标识 `≥30s`。沿用 `api.metrics.summary` 一次请求、项目切换和已有加载/错误态；宽屏三列、窄屏纵向，不新增专门页面。文案说明“全项目所有接口，近 24 小时，估算分位数”，避免让用户理解为单一 API 的精确分位数。

## 4. 实施顺序与验收

1. 后端：固定桶定义、采样及异步聚合、失败重试和单行刷盘；补充兼容旧指标的查询与 summary 字段。
2. 前端：类型、映射、mock、大盘数值/空态；不改全局 API 抽象签名。
3. 测试：多请求样本已知分布（含并发、桶边界、低于 1ms、`+Inf`、跨两次 flush、跨项目和 admin 汇总）、空窗与旧库兼容、刷新/缓存 10s 后更新、flush 失败重入队、关闭前刷盘；前端覆盖映射、项目切换、空态和响应式展示。
4. 验证：`go build ./internal/... ./cmd/...`、`go test ./internal/systemdb ./internal/api/...`、`ui` 的 `yarn test` / `yarn build`；完整 `go test ./...` 如因真实 DuckLake 集成用例耗时而未完成，应如实标注，不能宣称通过。用固定 1000 个已知耗时样本核对 P50/P90/P99 误差不得超过命中桶的宽度；混合两个项目的样本核对项目隔离和 admin 汇总；高并发请求延迟不因采集器同步落盘而增加。

## 5. 风险与回退

- 固定桶只能估计，尤其稀疏流量和 `+Inf` 长尾；UI 明确估算值与样本数，必要时调整**未来**桶版本并保证旧版本可解码/合并，禁止静默混算。
- 每 flush 一行会随项目数和时间增长；先沿用现有 `sys_metric_samples` 生命周期，记录 24h 查询行数/耗时，超过预算再评估按小时滚动压缩和保留策略，不引入每请求写系统库。
- 历史指标没有分位数信息，老版本返回的缺字段应在前端显示空态；回退时停止产生新 histogram 行，旧版本会忽略未知 `name`，已有 summary 字段保持兼容。
- 本计划**不改变** DuckLake 的连接池、安全边界和系统库只读桥接；admin 跨项目统计仍在已授权接口内执行。

## 6. 执行记录（2026-10-06）

### 6.1 计划评审结论

计划整体符合需求（项目级 24h P50/P90/P99、复用 summary 接口、不新增依赖/表）。执行时做了两处收敛：

- **超量程表达**：不额外记录最大值，后端返回 `latency_overflow_ms`（=30000），分位数落入 `+Inf` 桶时取该下界，前端据此显示 `≥30s`。
- **损坏行处理**：选择「显式降级」——跳过并 `Warn` 记录跳过行数，不混入结果、不让整个 summary 失败；SQL 查询/扫描错误则直接返回错误。

### 6.2 改动

| 文件 | 内容 |
| --- | --- |
| `internal/systemdb/latency_histogram.go`（新增） | v1 固定桶（左开右闭）、`observe`/`merge`/`quantile`（桶内线性插值）、`{"v":1,"b":[…]}` 编解码与校验（版本、桶数、总数一致、NaN/Inf） |
| `internal/systemdb/async_flush.go` / `store.go` | `http_latency_ms` 预聚合时同步更新按项目直方图；drain 时每项目一行 `http_latency_histogram_ms`；失败沿用 `requeue` 重入队，`Close` 经 `FlushMetrics` 刷盘 |
| `internal/systemdb/metrics.go` | summary 新增 `latency_p50/p90/p99_ms`（无样本为 `null`）、`latency_sample_count`、`latency_overflow_ms`；均值改为 `SUM/SUM(_count)`，窗口内无 `_count` 行时回退旧 `AVG`；查询错误不再吞掉 |
| `internal/api/router.go` | `http_latency_ms` 改为浮点毫秒（`Microseconds()/1000`），不再截断 <1ms |
| `ui/src/services/{types,http-api,mock}.ts/js`、`ui/src/test/api-mock.ts` | 新字段类型与映射，缺失/非法值保持 `null` |
| `ui/src/pages/Dashboard.vue` | 「请求趋势」卡片内新增 P50/P90/P99 三格（`sm` 以上三列、窄屏纵向），说明文案、样本数、`—` 空态、`≥30s`、≥1s 显示秒 |

### 6.3 测试与验证

- 新增 `internal/systemdb/latency_histogram_test.go`：桶边界/<1ms/NaN/`+Inf`、1000 个均匀样本误差 ≤ 命中桶宽、解码校验、并发写入 + 跨两次 flush、项目隔离与 admin 汇总、加权均值、旧逐请求数据回退、损坏行跳过、flush 失败重入队、查询错误上抛。
- `internal/api/system_handlers_test.go`：空库 summary 返回 `null` 分位数；空 schema 时 summary 不再返回 200。
- 前端：`Dashboard.test.ts`（分位数展示、秒/超量程格式、空态）、`http-api.test.ts`（映射与旧后端兼容）。
- 结果：`go build ./internal/... ./cmd/...`、`go vet ./internal/systemdb ./internal/api` 通过；除 `internal/database/ducklake`（真实 DuckLake 集成用例耗时长，未纳入本次运行）外 `go test` 全部通过；`yarn build` 通过；`yarn test` 93 文件 / 506 用例通过，覆盖率 Stmts 98.7% / Branch 95.12% / Funcs 95.09%（满足 95% 阈值）。

### 6.4 未覆盖 / 后续

- 「10s 缓存后更新」「高并发下请求延迟不受采集影响」未做专项压测：缓存沿用既有 `queryCache`，采集仅在已有锁内做 O(1) 桶计数，不新增同步 IO。
- 历史数据不回填，升级后需满 24h 口径才稳定；`latency_sample_count` 可能小于 `total_requests`。
- 直方图行随项目数 × flush 次数增长，后续按 §5 观察 24h 查询行数与耗时。
