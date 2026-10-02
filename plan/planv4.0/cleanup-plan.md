# SimpleBase v4.0 后端清理计划

> 日期：2026-09-29（v1.1 修订于同日，见文末修订记录）
> 范围：`internal/`、`gofunction/`、`cmd/`、`.github/`、`packages/` 及仓库根目录的遗留代码与产物
> 方法：以「非测试代码引用追踪 + UI/JS-SDK 消费端点核对 + git 跟踪状态 + CI/文档与实现比对」四重证据定性；全部结论均经 grep / `go build` / `go vet` / `go test` 验证
> 基线：`go build ./...` 与 `go vet ./...` 通过；`go test ./internal/... ./gofunction/ ./examples/...` 全绿（lease 包 30s、gofunction 11s 最慢）

---

## 0. 结论总览

| 级别 | 含义 | 数量 |
| --- | --- | --- |
| A | 无风险垃圾/误提交产物/失效 CI，立即处理 | 9 项 |
| B | 死代码（无任何消费方），删除需跑测试回归 | 6 项 |
| C | 过期文档修正 | 3 项 |
| D | 需决策的退役候选（功能完整但已无消费方/属一次性工具） | 5 项 |
| E | 明确保留（防误删清单） | — |

预计收益：git 仓库瘦身 ≥17MB（误提交二进制），本地磁盘释放 ~880MB（`thirdparty/buzz` 661MB + 根目录构建产物 220MB），删除 ~850 行死代码，修复 1 个必然失败的 CI 工作流，文档与实现对齐。

---

## A. 立即清理（零风险）

### A1. git 误提交的测试二进制：`gofunction/gofunction.test`（17.3MB）
- **证据**：`git ls-files -s gofunction/gofunction.test` 确认被跟踪；`file` 显示为 Mach-O arm64 可执行文件，是 `go test -c` 的产物。
- **动作**：
  ```bash
  git rm --cached gofunction/gofunction.test
  echo 'gofunction/gofunction.test' >> .gitignore
  rm gofunction/gofunction.test   # 本地也删
  ```

### A2. 无引用目录 `datasets/`（git 跟踪中）
- **证据**：仅 2 个 md（公共数据集链接清单）；全仓 grep（Go/TS/build.sh/workflows）无任何引用。
- **动作**：`git rm -r datasets/`。

### A3. 历史脱敏产物 `output/cfbee745-7a00-40e4-a292-49fd4f478b22/`
- **证据**：内容为旧版（Turso/libSQL 时代）plan 脱敏稿；无代码/文档引用。
- **动作**：`git rm -r output/cfbee745-7a00-40e4-a292-49fd4f478b22`；`output/perf/` 为 perfbench 输出目录（build.sh 引用），保留并确认勿提交（当前未被跟踪）。

### A4. 一次性迁移技能 `skills/`（git 跟踪中，25 个文件）
- **证据**：`skills/migrate-radix-to-base`、`skills/shadcn` 是 UI 从 Radix 迁移到 shadcn/base 的一次性 AI 技能；迁移已完成（planv2.0 收尾）；无代码/构建引用。
- **动作**：`git rm -r skills/`。

### A5. 磁盘垃圾：`thirdparty/buzz/`（661MB，Rust 项目）
- **证据**：无任何 Go import（仅 `kv/core.go` 注释提到 redka）；`.gitignore` 已含 `thirdparty`，纯本地参考材料；buzz 与本项目无代码关系。
- **动作**：`rm -rf thirdparty/buzz`（redka 2.6MB 为 KV 语义参考，建议留）。

### A6. 根目录构建产物二进制：`simplebased`（90MB）+ `perfbench`（132MB）
- **证据**：均已 gitignore；属本地构建残留。
- **动作**：删除即可（需要时 `go build -o simplebased ./cmd/simplebased` 重新生成）。

### A7. `gofunction/bench_report.md` 归档位置
- **证据**：性能测试报告（2026-09-19），是 planv2.0 性能优化的验收产物。
- **动作**：`git mv gofunction/bench_report.md plan/planv2.0/gofunction-bench-report.md`（与其他验收文档同级）。可选。

### A8. `.cache/` 本地缓存目录
- **证据**：`config.yaml` 的 `database.cache_dir: ".cache"`，运行时数据。
- **动作**：不在本计划清理；仅提示 dev 环境可用 `./simplebased reset --confirm=simplebase-dev-1` 维护。

### A9. 失效 CI 工作流：`.github/workflows/build-image.yml`（v1.1 新增）
- **证据**：
  - 第 29 行 `cd cmd/http && go build .` —— `cmd/http` **不存在**（实际只有 `cmd/simplebased`、`cmd/perfbench`），任何 tag push 必然失败；
  - `go-version: '1.20'` —— 项目要求 Go 1.25（`go.mod: go 1.25.0`），无法编译；
  - 镜像 tag 为 `ccr.ccs.tencentyun.com/serverlessv1/lessdb:...` —— LessDB 时代遗留，与现名 SimpleBase 不符；
  - actions 版本陈旧（checkout@v3、setup-go@v3、build-push-action@v2）。
- **动作**（二选一，默认修复）：
  1. **修复**：对齐 `build1.21.yml` 的写法 —— Go 1.25、`go build ./...`、`docker build`（现有 `Dockerfile` 已是 golang:1.25 + distroless，可直接用）、镜像名改为 SimpleBase 命名空间；
  2. **删除**：若镜像发布已改用其他流程，`git rm .github/workflows/build-image.yml`。
- **附带**：`build1.21.yml` 文件名与 workflow name（`Go1.25`）漂移，顺带重命名为 `build.yml`（可选，注意分支保护规则引用）。

---

## B. 死代码清理（低风险，删除后必须回归测试）

### B1. `internal/systemdb/gofunctions.go` 整个兼容层 + `toGoFunctionDTO`
- **证据**：
  - 文件头自述「兼容层：旧 GoFunction 读写 API 映射到 sys_go_funcs / sys_go_func_versions……供既有测试与内部调用过渡」。
  - 非测试代码对 `ListGoFunctions/CreateGoFunction/UpdateGoFunction/GetGoFunction/ArchiveGoFunction/CountGoFunctions` 的调用：**0 处**。
  - 生产链路（`gofunction_handler.go`、`cron_runner.go`、`cronjob_handler.go`）全部已走新 API（`GetGoFunc/AppendGoFuncVersion/ResolveActiveSource` 等）。
  - `gofunction_handler.go:90` 的 `toGoFunctionDTO` 仅被 `coverage_push_test.go:156` 调用。
- **动作**：
  1. 删除 `internal/systemdb/gofunctions.go` 与 `internal/systemdb/gofunctions_test.go`；
  2. 迁移 3 个测试文件到新 API：
     - `internal/api/cronjob_handler_test.go:26`（`CreateGoFunction` 播种 → `CreateGoFunc` + `AppendGoFuncVersion` + `ActivateGoFuncVersion`）；
     - `internal/app/more_test.go:159`（同上）；
     - `internal/systemdb/coverage_agents_more_test.go:357-433`（nilStore/ErrUnavailable 用例改为覆盖新 API）；
  3. 删除 `gofunction_handler.go` 中 `goFunctionDTO`/`toGoFunctionDTO`（87-107 行）及 `coverage_push_test.go:156` 对应用例；
  4. **连带修订（v1.1 补充）**：`cron_jobs.go:37` 注释 `FuncFile string // sys_gofunctions.name` → 改为 `sys_go_funcs.name`；
  5. `sys_gofunctions` 表 DDL（`migrate.go:404`）按「迁移历史保留」处理（同 sys_jobs 惯例），**勿删 DDL**；但 `coverage_agents_more_test.go:429` 直接 `INSERT INTO sys_gofunctions` 的旧表兼容用例随第 3 步一并改写。
- **验证**：`go test ./internal/systemdb/... ./internal/api/... ./internal/app/...`。

### B2. Legacy 项目级 `/data/collections` 路由 + DataHandler 隐式首库回退
- **证据**：
  - `router.go:278-283` 挂载的 6 条「无 databaseID、隐式取第一个库」路由，注释明写 `Legacy project-scoped routes`；
  - UI（`http-api.ts` 的 `dataCollectionsPath`）、JS-SDK（`collections.ts` 一律带 `databases/${db}`）、docs（`docs/sdk/documents.md` 只写 database-scoped 映射）**全部走 database-scoped 版本**，legacy 路由 0 消费方；
  - `data_handler.go:289-299` 的「无 :databaseID 时取 databases[0]」回退逻辑仅服务这 6 条路由（256 行已注明 database-scoped 路径绝不回退）。
- **动作**：
  1. 删除 `router.go` 中 6 条 legacy 路由；
  2. 删除 `data_handler.go` 的隐式首库分支（保留 `:databaseID` 强制路径）；
  3. 迁移/删除 `data_handler_test.go:86-88`、`handler_branches_test.go:358+` 中挂在 legacy 路径上的用例（改挂 database-scoped 路径，覆盖面不减）。
- **验证**：`go test ./internal/api/` + UI `yarn test`（mock 不受影响）+ `examples/node-documents` 手跑。

### B3. `removedDatabaseAction` open/close 404 桩
- **证据**：`router.go:263-264` 为已下线的 open/close 返回固定 404；SDK/UI/docs 无任何 open/close 调用（已全量 grep）；`database_handler.go:253-254` 为其实现。
- **动作**：删除两条路由 + handler 函数 + 相关注释（Echo 405 的历史兼容说明一并移除）。
- **验证**：`go test ./internal/api/`。

### B4. `LastInsertID` 废弃字段全链路
- **证据**：
  - `database.QueryResult.LastInsertID`：DuckLake 下恒 0，注释明示「API 层已废弃该字段」；
  - `api.ExecuteResponse` 与 `BatchResultItem` 的 `last_insert_id`（均 omitempty，永不输出）；
  - `sql_handler.go:249-250` 的非零拷贝分支为不可达死代码；
  - 仅测试（`sql_handler_test.go:232`、`handler_branches_test.go:266+`）在喂假值。
- **动作**：删除 `QueryResult.LastInsertID`、两处 DTO 字段、`sql_handler.go` 接线；同步修改 4 处测试断言。
- **验证**：`go build ./... && go test ./internal/...`。协议层零影响（字段从未序列化输出）。

### B5. `app.go` KVInitializer 双重构造
- **证据**：`app.go:165` 与 `app.go:769`（`initKVFunc`）各建一个 `api.NewKVInitializer`，功能重复。
- **动作**：`initKVFunc` 复用已构造的 `kvInit`（装配期保存到 App 字段或闭包引用），删一处构造。
- **验证**：`go test ./internal/app/`。

### B6. 孤儿系统表 `sys_log_exports`（v1.1 新增）
- **证据**：`migrate.go:292` 定义 DDL，但全仓 grep **零处** INSERT/SELECT/UPDATE（`logs.go` 无任何 export 逻辑；`sys_s3_sync_runs`、`sys_jobs` 等其余表均有活跃读写或明确的迁移保留注释，唯独此表无消费且无说明）。
- **动作**：删除 `migrate.go` 中该 DDL 条目（迁移器按 name 幂等执行，新部署不再建表；旧部署已存在的空表无害）。
- **保守选项**：若担心未来「日志导出」功能很快上线，可改为在 DDL 处补注释「预留，暂无读写」并保留——默认删除。
- **验证**：`go test ./internal/systemdb/...`。

---

## C. 过期文档修正

### C1. 根 `README.md`
- `plan/plan.md` 链接失效（文件不存在）→ 改为指向 `plan/` 目录或最新 planv4.0；
- 「`litellm/` 为独立客户端库」及目录结构中的 `litellm/` → 实际已改为 go module `github.com/voocel/litellm`，仓库内无该目录；
- 目录结构一节缺：`database/kv`、`database/lease`、`cronjob`、`crontab`、`sandbox`、`systemdb`、`web`、`gofunction/`。
- **动作**：按实际结构重写「目录结构」小节，修两处失效引用。

### C2. `internal/README.md`
- 模块清单缺：`cronjob`、`crontab`、`sandbox`、`systemdb`、`web`、`database/kv`、`database/lease`；`api` 小节缺 kv/gofunction/cronjob/agent_schedule/users/auth/cloudagent_access 等 handler 说明；依赖关系图不含 gofunction/cronjob 链路。
- **动作**：补齐模块清单与依赖图（与 `internal/AGENTS.md` 对齐）。

### C3. `internal/database/README.md`（v1.1 新增，过期最严重）
- **证据**（与实际文件系统比对）：
  - `### turso/` 小节（第 10-11 行）—— 目录**不存在**，Turso/libSQL 引擎已整体退役（根 README 明示），「Phase 4 退役」已完成但文档未删；
  - 顶层文件清单引用 `transaction.go`、`errors.go`、`tursofactory.go`（38/40/41 行）—— **均不存在**（实际只有 `runtime.go`、`query.go`、`serialize.go`）；
  - `sqlguard/` 描述缺「DuckDB 管理面拒绝清单（ATTACH/SET/CALL/COPY 等）、多语句与 NUL 拦截」（实际实现核心）；
  - 子包清单缺 `kv/`（KV 仓库）与 `lease/`（写租约），且未提及 `catalog_engine` 迁移约束已有文档在根 README。
- **动作**：删除 turso 小节与三个幽灵文件条目；补 `kv/`、`lease/` 子包与 sqlguard 实际边界描述；顶层文件清单对齐三文件实际。
- **附带核对结论**：`docs/` 全目录仅 `ops/migration.md` 含 turso 字样且已自标 Deprecated 存档——合规，不动。

---

## D. 退役候选（需要决策，默认本轮不动）

### D1. `cmd/perfbench` + `internal/api/perfbench` + stage_timing 体系
- **现状**：api-db-perf-validation-plan 已验收（bench_report 归档）；perfbench 为独立二进制不进生产；stage_timing 由 `observability.perf_stage_timing` 开关控制（默认 false，本地 config.yaml 为 true）。
- **建议**：**保留**。理由：DuckLake+S3 延迟敏感，是唯一可复现的闭环压测工具；代码隔离好、零生产耦合。若未来确定退役，应整体一次性移除：`cmd/perfbench`、`internal/api/perfbench/`、`api/stage_timing*.go`、`observability/stage_timing_metrics.go`、config 三处字段、`MetricsRegistry` 桥接。

### D2. `/v1/projects/:p/llm/sessions` 读 CRUD（system_handlers.go 的 llmSessionHandler）
- **现状**：UI/SDK 均 0 消费（UI 只用 `/llm/settings` GET/PUT）；`persistChat` 写入侧仍活跃（llm_handler 落 chat 记录），读侧（List/Get/Delete/ListMessages/PostMessage）无前端使用；proto.http 仅存示例。
- **建议**：保留写入路径与 settings；sessions 读 CRUD 可在下一轮 API 面收紧时移除（需先确认无外部调用方）。本轮不动。

### D3. `internal/log` 并入 `internal/observability`
- **现状**：`log`（245 行 zap 封装）仅被 `observability/logger.go`（70 行门面）import，两层薄封装。
- **建议**：可选合并（move + 改 import 一处）；收益小，非必须。

### D4. `plan/planv1.0~v3.0` 历史计划文档
- **现状**：43 个文件，代码注释中大量 `plan §x.x` 引用指向这些文档。
- **建议**：**保留全部**，是注释引用的锚点。

### D5. `packages/js-sdk/dist` 构建产物入库（v1.1 新增）
- **现状**：6 个文件（84KB）tsup 构建产物被 git 跟踪；`package.json` 的 `files: ["dist", ...]` 表明 npm publish 需要 dist。
- **决策点**：若无独立 npm 发布 CI（从源码 build 后 publish），则 dist 入库是「tag 即发布」流程的一部分，**必须保留**；若有（或计划建），应 gitignore + 发布流水线构建。
- **建议**：本轮保留，待 npm 发布流程明确后处理。84KB 体量小，不紧迫。

---

## E. 防误删清单（已核实为活跃代码）

| 项 | 证据 |
| --- | --- |
| `internal/api/kv_commands.go`（37.9KB，Redis 风格命令） | UI `KeyValue.vue` + `/v1/projects/:p/kv` 在用；`database/kv` 各 Repo 方法 46 个全部被 kv_commands 调用 |
| `internal/api/error.go` 的 kv 错误映射（190-202 行） | `kv.ErrNotFound/ErrKeyType/ErrKeyExists/ErrValueType/ErrArgument` 五条活跃映射 |
| `internal/database/kv/sweeper.go` + TTL 体系 | app.go 装配 KV TTL 后台清扫 |
| `internal/database/lease/` 写租约 | multi-instance-consistency-plan P1；app.go WriteGate 在用 |
| `objectstore/probe.go` CAS 探针 | app.go 启动路径在用 |
| `catalog/cache.go`、`auth/cache.go`、`api/db_stats_cache.go` | §7.2 性能缓存与列表行数缓存，均在装配链上 |
| `auth/jwt.go`（SignJWT/VerifyJWT/LooksLikeJWT） | session.go 登录态签发/校验在用 |
| `usage/adapter.go` LLMRecorder | llmgateway 用量计量装配在用 |
| `llmgateway/fallback_resolver.go` | app.go:560 实例级 provider 注入在用 |
| `systemdb` 全部 `sys_*` DDL（含 sys_jobs、sys_gofunctions） | 迁移历史保留（应用层不再读写，但 DDL 保留；唯一例外是 B6 的 sys_log_exports） |
| `sys_s3_sync_runs` | s3index.go 活跃读写 |
| `docs/ops/migration.md` | 文件头已自标 Deprecated 存档，README 链接有效 |
| `gofunction/` 解释器全部文件（含 testdata/、value/、importer/） | `go build`/`go vet` 全量通过；生产链路 RunJSON/ResolveActiveSource 活跃；`packages/` blank import 被 app.go 依赖 |
| `benchmarks/gofunction`（含 7 个 Benchmark） | 原 `examples/gofunction` 迁移至独立压测目录，仍纳入 `go test ./...` |
| `examples/shop-service`、`mini-game-service`、`booking-service`、`community-service`、`ticket-service` | 新业务案例，分别覆盖数据库、KV、云函数、定时任务和可信 BFF/私有前端产物 |
| `mock.js` + `mock-kv.js`（2125 行） | `VITE_USE_MOCK` 开关的 mock 实现，与 http-api 同 `Api` 接口签名（ui/AGENTS.md 约定） |
| UI 业务组件 `ai/AiChat`、`ai/AgentScheduleModal`、`ProjectScope` | 分别被 `AgentManager.vue`（AiChat×2 处）、`AgentManager.vue`、7 个页面消费——**易被误判为 /llm 退役遗留，实际全部活跃** |
| UI `src/docs/`（DocsWiki 数据源） | DocsWiki.vue 经 catalog.ts 渲染 `docs/<module>/*.md` |
| `packages/js-sdk/`、`docs/` | 文档与 SDK 活跃交付物 |
| `litellm`（go module 依赖） | llmgateway/streaming、adapters_plan79 在用 |
| `uglyer/go-sqlite3` 依赖 | 23 个测试文件的内存 SQL 驱动，保留 |

---

## 执行顺序

1. **A 组**（互相独立，逐项单独 commit）：A1→A6、A7（mv）、A9（修复或删除 workflow）。
2. **B 组**（每项独立 commit，顺序建议 B1→B4→B3→B2→B5→B6，先删纯死代码再动路由与 DDL）：
   - 每项完成后跑 `go build ./... && go vet ./... && go test ./internal/... -count=1`；
   - B2 完成后加跑 UI `yarn test` 与 `packages/js-sdk` vitest。
3. **C 组**：文档更新放最后（避免与代码改动冲突），C3 过期最严重优先。
4. **D 组**：逐项在 issue/讨论中决策，不在本轮执行。

## 验证命令汇总

```bash
go build ./... && go vet ./...
go test ./internal/... -count=1
go test ./gofunction/... ./benchmarks/... ./examples/... -count=1
cd ui && yarn test                       # B2 后
cd packages/js-sdk && npx vitest run     # B2 后
git ls-files | grep -E "(gofunction.test|datasets/|output/cfbee|skills/)"   # 应为空
grep -rn "cmd/http" .github/             # A9 后应为空
du -sh thirdparty/ simplebased perfbench 2>/dev/null   # A5/A6 后应大幅缩小/不存在
```

## 风险与回滚

- 所有 B 组改动均为「无消费方」删除，协议层无字段输出变化（LastInsertID 从未序列化、legacy 路由无调用方）；
- 每项独立 commit，出问题按 commit 单独 revert；
- B1 涉及 3 个测试文件迁移，是本计划唯一需要「改写测试」的项，预留最多时间；
- B6 删 DDL 对存量部署无影响（迁移器幂等、表已存在则跳过），但需在发布说明中标注新部署不再建 `sys_log_exports`；
- A9 修 workflow 后建议打一个测试 tag 冒烟验证镜像发布链路；
- 不动 `go.mod`。

---

## 修订记录

- **v1.0**（2026-09-29 上午）：初版，A×8 / B×5 / C×2 / D×4。
- **v1.1**（2026-09-29 下午，复核补充）：
  - 新增 **A9**：`build-image.yml` 失效工作流（`cmd/http` 不存在、Go 1.20、lessdb 镜像 tag——tag push 必然失败的死工作流）；顺带记录 `build1.21.yml` 文件名漂移。
  - 新增 **B6**：孤儿表 `sys_log_exports` DDL（全仓零读写，无迁移保留注释）。
  - 新增 **C3**：`internal/database/README.md` 过期最严重（幽灵 `turso/` 小节、`transaction.go`/`errors.go`/`tursofactory.go` 幽灵文件条目、缺 `kv/`、`lease/` 子包）。
  - 新增 **D5**：`packages/js-sdk/dist` 构建产物入库（84KB，依赖 npm 发布流程决策）。
  - **B1 动作补充**：`cron_jobs.go:37` 旧表名注释修订、`coverage_agents_more_test.go:429` 直插 `sys_gofunctions` 旧表的用例处理。
  - **E 组补充**：UI 业务组件防误判（AiChat/AgentScheduleModal/ProjectScope 全活跃）、`mock.js`/`mock-kv.js`、`error.go` kv 错误映射、`fallback_resolver`、`sys_s3_sync_runs`、`examples/gofunction` CI 编译属预期。
  - 文档比对结论：`docs/` 全目录仅 `ops/migration.md` 含历史字样且已合规标注，无清理项。
