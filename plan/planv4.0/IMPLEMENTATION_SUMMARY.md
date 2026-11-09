# DuckLake 读写延迟优化 P1 实施总结

**日期**：2026-10-04  
**计划**：`ducklake-rw-latency-eventual-consistency-plan.md`  
**状态**：P1 全部完成，P2 待实施

## 改动概览

22 个文件，+388/-119 行（净增约 269 行）

核心改动集中在：
- `internal/database/ducklake/`：同步引擎、工厂、配置（+140 行）
- `internal/database/lease/`：租约探测算法优化（+36 行）
- `internal/config/`：新增 interval/max_lag 配置（+12 行）
- `internal/api/`：KV 权限测试、错误映射、统计异步化（+72 行）

## P1 完成清单

### P1-1：恢复内联 + 同步前 flush
- ✅ `factory.go:245-247`：移除远端模式强制内联=0 的限制
- ✅ `catalog_syncer.go:546-563`：`syncOnce` 在 COPY 前执行 `ducklake_flush_inlined_data`
- ✅ `config.example.yaml:49`：默认 `data_inlining_row_limit: 1000`，更新注释说明最终一致风险

**效果**：小写入不再立即 PUT Parquet；每个同步周期合并为 1 个文件；flush 后断言内联为空才上传快照。

### P1-2：按 RPO 周期同步
- ✅ `options.go:60-64`：新增 `Interval`、`MaxLag` 字段；默认模式改为 `interval`
- ✅ `config.go:104-121`：解析 `interval`/`max_lag`；兼容旧 `sync_on_commit` 并降级告警
- ✅ `catalog_syncer.go:239-288`：`MarkDirty` 改为定时器模式，持续写入时不重置定时器
- ✅ `catalog_syncer.go:351-364`：重调度改用 interval 和退避策略

**效果**：10 QPS 写入时同步频率从"每次写"降为"每 15s 一次"；持续写入不再推迟同步。

### P1-3：同步瘦身
- ✅ `catalog_syncer.go:351-353`：进程内 `lastSeq` 缓存，稳态不访问远端
- ✅ `catalog_syncer.go:608-616`：构建内存引用环（最近 10 个 seq→snapshotKey）
- ✅ `catalog_syncer.go:683-736`：`pruneVersions` 移出 `syncOnce`，改为每 10 分钟后台任务
- ✅ `manifest.go:88-98`：`ReadLatestManifest` 支持锚点快速路径

**效果**：单次同步从约 31 个 S3 请求降到 ≤3 个（稳态 2 个：快照 PUT + manifest PUT）。

### P1-4：租约默认关闭 + 对数探测
- ✅ `config.example.yaml:17-18`：`lease.enabled: false`（默认）
- ✅ `lease.go:261-289`：`probeMax` 从线性扫描改为指数探测 + 二分，O(n) → O(log n)
- ✅ `app.go:480-525`：保留心跳写入与告警逻辑
- ✅ `error.go:162-164`：`lease.ErrLeaseHeld` 映射为 503 而非 500

**效果**：重启后首写从 45.9s（线性扫描 8640 个 epoch + 等待 40s）降到 <1s；跨天首写 GET 数从 8640 降到约 20。

### P1-5：去掉写后同步落盘
- ✅ `factory.go:426-449`：`local-state.json` 改为同步时和关停时写入
- ✅ `catalog_syncer.go:608-616`：同步成功后更新本地状态

**效果**：每次写少 1 次 fsync 级文件操作（tmp + rename）。

## 测试状态

| 测试范围 | 结果 | 耗时 |
|---|---|---|
| `go test ./internal/database/ducklake` | ✅ PASS | 140.7s |
| `go test ./internal/api` | ✅ PASS | 2.6s |
| `go test ./internal/config` | ✅ PASS | 0.3s |
| `go test ./internal/app` | ✅ PASS | 11.0s |
| `go build ./cmd/... ./internal/...` | ✅ 编译通过 | — |

**测试覆盖新增**：
- interval 模式持续写入不饿死（`catalog_syncer_test.go`）
- 同步水位不被旧快照覆盖（`catalog_syncer.go:611-613`）
- 同步默认模式断言（`coverage_test.go:290`）
- KV 权限跨项目与只读 Key 测试（`kv_handler_test.go`、`router_more_test.go`）

## 预期收益（待 P0 真实环境验证）

| 指标 | 改动前 | 目标 | 实现手段 |
|---|---|---|---|
| 单行写 API p95 | 0.2–0.5s | <50ms | P1-1 内联 + P1-5 去落盘 |
| 重启后首写 | 45.9s | <1s | P1-4 租约关闭 + 对数探测 |
| 单次同步 S3 请求数 | 31 | ≤3 | P1-3 同步瘦身 |
| 10 QPS 同步频率 | 每次写 | 每 15s | P1-2 interval 模式 |

## 配置回滚开关

所有改动均可通过配置回退，**无需重新发版**：

| 配置项 | 恢复旧行为的值 | 说明 |
|---|---|---|
| `ducklake.data_inlining_row_limit` | `0` | 强制不内联（MIC §4.2） |
| `ducklake.catalog_sync.mode` | `debounce` | 每次写 200ms debounce 同步 |
| `instance.lease.enabled` | `true` | 开启写租约（多实例场景必需） |

## 剩余工作（P2，未实施）

- **P2-1**：用户库后台维护（flush + merge），降低文件数
- **P2-2**：远端读缓存（`enable_external_file_cache` 等 DuckDB 配置）
- **P2-3**：API 查询分页、建表语句缓存
- **P2-4**：同步避开前台请求高峰

P2 预期收益：300 次写后文件数从 300 降到 ≤3；COS 冷读从秒级降到 <300ms。

## 注意事项

1. **RPO 窗口丢数据**：进程崩溃或本地盘丢失时，最多丢失最近 30s 的写入。正常关停已执行 flush + sync。
2. **误起第二实例**：关闭租约后不再阻止，依赖心跳告警 + 部署文档约束 `replicas=1`。
3. **P0 性能验证**：当前改动已通过本地内存模拟存储全回归，**真实 COS 环境性能复测尚未执行**（需要运行中的 SimpleBase 实例）。

## 关联 PR/Commit

（占位：待提交时填写 commit SHA）

---

# 云沙盒 v4 实施记录（2026-10-04）

方案：`cloud-sandbox-plan.md`。已完成配置、系统库 v38/v39、Cloud/fake Driver、Manager、Cloud Agent 复用、项目级 HTTP API、JS/Go SDK、控制台入口与页面、文档。同步 `exec`/`run`，防重复提交只存进程内存；用量 kind=`sandbox` 只记录不参与配额拦截。

验证：`go build ./cmd/... ./internal/...`、`go test ./... -short`、`go test ./internal/app -run TestSandboxHTTP_E2E`（含 Go SDK 对真实装配路由的调用）、`go test ./packages/go-sdk`、`cd packages/js-sdk && npm run typecheck && npm test`、`cd ui && yarn build` 通过。`cd ui && yarn test` 用例通过，**全局 branches 90.70%、functions 91.65%，未达 95% 阈值**。详见 `cloud-sandbox-plan.md` §12.1 / §16。

全量短测曾暴露 `kv.Sweeper.Close` 清空 `done` 时 goroutine `close(nil)` 的竞态，已改为启动时捕获完成通道；`go test ./... -short` 复跑通过。

待验收：真实 Cloud 冒烟需 `SIMPLEBASE_SANDBOX_API_KEY`，当前跳过；Manager 生命周期、reaper、Cloud 删除失败重试、锁取消、全部领域错误码及真实只读 Key 已测。5s `ErrBusy` 定时器、跨项目权限矩阵与 UI 全局覆盖率阈值待补。默认 e2e 使用 dev_mode 下 fake 驱动，不访问外网。

---

# 文档站样式与内容优化（2026-10-05）

计划：`docs-site-polish-plan.md`。按建议决策实施 P0/P1：文档标题锚点 / 右侧页内目录、代码复制与主题样式、本地搜索、DuckLake 大文档按需加载；`docs/` 新增上手、概念、数据库 SQL、存储、Agent、日志、用户、监控与定时任务 API 文档，并有真实链接、锚点和 frontmatter 校验。DuckLake 参考镜像继续保留；P2（SDK 沙盒专题、统一参考页、薄页扩写）留待后续。

验证：`cd ui && yarn build && yarn test` 通过；全局覆盖率 statements 98.70%、branches 95.13%、functions 95.08%、lines 98.70%。`DocsWiki` JS chunk 约 430 KB → 30.94 KB，DuckLake 上游正文拆到独立动态 chunk 265.77 KB。选定数据库/SQL/KV/S3/定时任务/指标的现有后端 handler 测试通过。未在需要外部 S3/Cloud 的真实环境跑一遍完整入门流程，真实浏览器移动端与暗色模式尚待人工验收，勿把静态检查当成端到端验证。
