# 死代码清理与代码优化计划（v6.6）

> 承接 `plan/planv6.5/code-simplification-and-cleanup-plan.md`（CLN/UI/GDB/SBX/GCA/GFN/INT 已完成，APV-1~4 待确认）。
> 本计划只登记**本轮新扫描**到的问题，不重复 v6.5 已交付项。

## 0. 扫描方法与口径（2026-11-15）

| 工具 | 范围 | 结果文件（会话临时目录） |
| --- | --- | --- |
| `deadcode -test ./...` | 根模块 + 4 个独立模块分别跑（`GOWORK=off`） | `dc-*.txt` |
| `staticcheck ./...` | 同上 | `sc-*.txt` |
| `go vet ./...` | 根模块 | 0 告警 |
| `gofmt -l` | 全部 Go 源码 | 14 个文件 |
| 自写脚本 | `ui/src` 未被生产代码 import 的模块 / 导出 | 3 个导出 |
| `npx tsc --noEmit` | `ui/` | 16 处错误，**全部在 `ui/tests/`** |

**误报说明（不处理）**：

- 根模块 `deadcode` 报的 `gocloudagent/cli/*`、`gocloudagent/tools/*` 约 100 个函数：只由 `gocloudagent/cmd/gocloudagent`（独立二进制）使用，根模块不构建它。模块内单独扫描时只剩 `callID` 一项。
- `gofunction` 公开 API（`Run` / `NewExecutor` / `NewExecutorPool` 等）、`godatabase/testing.go` 的 `Recording*`、`gosandbox/vm/fake`：属于对外 API 或测试替身。
- `godatabase` 模块内扫描报的 `kv.Sweeper`、`EnsureSchema`、`UnionStore/DiffStore`、`EnableProjectScope`：宿主 `internal/` 在用。
- `gofunction/packages/std_http.go` 的 `io/ioutil`：给脚本注册 `io/ioutil` 包用的，不能删。
- `gosandbox/host/*_darwin.go` 的 `SYS_PROC_INFO` / `SYS_IOCTL`：x/sys 没有 libSystem 包装，保持原样。

---

## 1. 死代码（确认无生产调用）

### DC-1 根模块：零引用，可直接删除（P0，零风险）

| 位置 | 内容 | 依据 |
| --- | --- | --- |
| `internal/sbcli/dev.go:496` | `devIsNotFound` | 全仓 0 调用（U1000） |
| `internal/alerts/bus.go:63` | `Bus.dropped` 字段 | 从未读写（U1000）；注释也说明锁内不丢弃 |
| `internal/backup/scheduler_test.go:15-20` | `fakeSchedRepo` 及方法 | 测试内未使用（U1000） |
| `internal/auth/user.go:81` | `ValidatePasswordStrict` | 生产与测试均 0 调用 |
| `internal/api/auth_handler.go:67` + `login_guard.go:95` | `WithLoginGuardAudit` → `WithLockHook` 调用链 | 整条链 0 外部调用、0 测试；先确认锁定审计是否已由别处覆盖，再删 |

### DC-2 独立模块：零引用的私有符号（P0）

| 位置 | 内容 |
| --- | --- |
| `gocloudagent/cli/mcp_command.go:140` | 常量 `mcpToolLimit` |
| `gocloudagent/cli/mcp_tool.go:161` | `(*mcpLocalTool).callID` |
| `gofunction/internal/interp/compilefast.go:27` | 类型 `slotGetter`（注释描述的抽象已不存在） |
| `gofunction/internal/interp/call_test.go:223` | 测试辅助 `setSSAType` |
| `gofunction/importer/coverage_test.go:126` | 未用字段 `b` |

### DC-3 只有测试在用的生产代码（P1，需逐项决定「删」还是「接入」）

这些函数只在 `_test.go` 中出现，属于「为测试而存在」的生产代码，或者功能只做了一半：

| 位置 | 符号 | 建议 |
| --- | --- | --- |
| `godatabase/ducklake/orphan_ledger.go` | `OrphanLedger.Record`、`MaintenanceGate.Check`、`BlockedReason` | **功能未接线**：宿主只用到了 `DuckLakeOrphanLedgerKey`。决定是接入 compaction / GC 流程还是整体移除 |
| `godatabase/ducklake/errors.go:42,47` | `IsCatalogEngineMismatch` / `IsCatalogEngineUnknown` | 宿主改用 `errors.Is` 判断，或删除 |
| `godatabase/ducklake/inspect.go` | `ListSnapshots` / `ListTableFiles` | 若为运维诊断预留，挂到 `simplebase doctor`；否则删除 |
| `godatabase/ducklake/{factory,manifest}.go` | `layoutFor`、`manifestSeqFromKey` | 测试改调真实入口后删除 |
| `godatabase/kv` | `KeyMeta.TTLms`、`WithNowFunc`、`KeyRepo.rawCount`、`HashRepo.Scan` | `WithNowFunc` 是测试注入点，可移到 `export_test.go`；其余删除 |
| `godatabase/registry/registry.go:414` | `Registry.OpenedIDs` | 删除 |
| `gocloudagent/skills.go` | `SkillPrefixes` / `KnownSkillIDs` / `SkillBody` / `EmbeddedSkillReadOnly` | 只被 `agentbridge` 的对账测试使用；保留，但在注释中写明「对账测试专用」 |
| `gocloudagent/mcp/client.go:100` | `ConnectStdio` | 0 生产、0 测试 → 删除，或补测试 |
| `gofunction/internal/interp` | `runFrame`、`testLayout` | 移到 `_test.go` |
| `gosandbox/host` | `BwrapArgs`、`ScratchDirs`、`ExpandGlob`、`ResetProbeCache`、`WaitTimeout`、`vm/local.Driver.wsDir` | 公开 API 保留；`wsDir` 删除 |
| `internal/backup/scheduler.go:159` | `Scheduler.RunOnce` | 若仅测试用，移到 `export_test.go` |
| `examples/lib/goexample` | `Runtime.Compute`、`Int`、`Poll`、`StartWorker` | 样例库，确认 4 个站点都不用后删除 |

### DC-4 前端死导出（P1）

| 位置 | 符号 | 说明 |
| --- | --- | --- |
| `ui/src/utils/agent-errors.ts` | `isRetryableError` | 只有单测在用 |
| `ui/src/utils/format.ts` | `shortTime` | 只有单测在用 |
| `ui/src/services/http.ts:77` | `setUnauthorizedHandler` + `onUnauthorized` | 生产代码从未注册 handler，401 已改走 `useAuthStore().markUnauthorized()`；连同单测一起删除 |

删除后需同步删除对应的单测用例，并重新确认 coverage 阈值。

### DC-5 遗留兼容层（P1，需要审批，即 v6.5 APV-1 / APV-3 的延续）

1. 后端 `/agent-schedules`：写入与触发统一返回 410，只读接口仍在。前端 `AgentManager.vue:502` 仍调用 `agentSchedules.list` 展示只读摘要。
   - **建议定版本下线**：把只读摘要迁到 `/cron-jobs`（按 agent 过滤）→ 删 `agent_schedule_handler.go`、7 条路由、`systemdb/agent_schedules.go`、前端 `agentSchedules` 整组 API 和对应语言条目。
2. `ApiClient` 中 `agentSchedules.{create,patch,remove,runs,trigger}`、`llmTraces.list`、`llm.{chat,stream}`：只调用 410 端点或已无页面使用（APV-3）。

---

## 2. 代码质量 / 优化

### OPT-1 文件名异常（P0）

- `godatabase/sqlguard/classifier-desens.go`：文件名带 `-desens`，疑似脱敏工具残留；Go 文件名惯例也不使用连字符。执行 `git mv` 改回 `classifier.go`，同时 gofmt（它就在 gofmt 未通过清单中）。
- 根目录与 `plan/` 下的 `.box-desens/` 已被 `.gitignore` 覆盖，未入库；本地可手动清理。

### OPT-2 弃用 API 迁移（P1）

| 位置 | 现状 | 替换 | 风险 |
| --- | --- | --- | --- |
| `internal/objectstore/client.go:158-174` | `WithEndpointResolverWithOptions` + `aws.Endpoint` | `s3.Options.BaseEndpoint` + `UsePathStyle` | **中**：MinIO / R2 / 自定义 region 的签名可能变化，需要在本地盘以外跑一遍 S3 兼容存储冒烟 |
| `gocloudagent/agentcore/model.go:495` | `model.ChatModel` | `model.ToolCallingChatModel`（`WithTools` 不可变） | 中：涉及 native→text 回退，需要跑 agentcore 全量测试 |
| `gofunction/importer/importer.go:205,263,310` | `types.NewSignature` / `NewInterface` / `reflect.PtrTo` | `NewSignatureType` / `NewInterfaceType` / `reflect.PointerTo` | 低 |
| `godatabase/ducklake/gated_conn.go:218-267` | 非 Context 版 `Stmt.Exec/Query` 回退 | 保留回退，加注释说明「驱动未实现 `StmtExecContext` 时的兜底」 | 不改行为 |

### OPT-3 测试代码质量（P2，staticcheck）

- `SA1012` 传入 nil `Context`，约 27 处：`internal/api/b3_sandbox_routes_test.go`、`sandbox_handler_test.go`、`stage_timing_test.go` → 改为 `context.Background()`。
- `SA4006` 赋值后未使用：`gocloudagent_access_test.go:271`、`handler_branches_test.go:100,536`、`gocloudagent/tools_more_test.go:73-90`（5 处）。这类赋值通常意味着**断言漏写**，要补断言，不能只删变量。
- `SA5011`：`gofunction/importer/parser_test.go:115-121` 判 nil 后仍解引用 → 改为 `t.Fatal`。
- 简化：`S1040`（2 处）、`S1025`、`S1009`。

### OPT-4 前端类型检查（P1）

`yarn typecheck` 现在只剩 16 处错误，**全部在 `ui/tests/`**（AGENTS.md 中「241 处 TS2307」的描述已过时）：

- 测试直接访问 `wrapper.vm.load/remove/download/exportCSV`（`S3Manager`、`Logs`）→ 改为 `defineExpose`，或改成通过 DOM 触发。
- 夹具类型漂移：`SandboxItem.createdAt`、`AuthUser`、`SettingsTab 'models'`、`http-api.test.ts:613`（number 传给 string 参数）→ 按当前类型修正夹具。
- `DOMWrapper.exists` 的 Omit 报错（4 处）与 `NodeListOf` 迭代 → 改用 `find().exists()`，并在 tsconfig 中补 `DOM.Iterable`（**改配置需审批**，可以改用 `Array.from` 绕开）。
- 清零后在 CI 中加入 `yarn typecheck`。

### OPT-5 gofmt（P2）

14 个文件未通过 gofmt：`internal/api/enduser_handler.go`、`internal/auth/enduser.go`、`internal/objectstore/blob.go`、`internal/observability/metrics.go`、`godatabase/cache/evict.go`、`godatabase/sqlguard/classifier-desens.go`、`gofunction/internal/interp/compile.go`，以及 7 个测试文件。可以单独提交一次 `gofmt -w`（纯格式，blame 噪音可接受），然后在 `scripts/smoke.sh` 中加 `gofmt -l` 门禁。

### OPT-6 大文件拆分（P3，按需）

| 文件 | 行数 | 建议 |
| --- | --- | --- |
| `ui/src/services/http-api.ts` | 2199 | 按领域拆成 `services/api/{db,kv,s3,agent,llm,cron,...}.ts`，`http-api.ts` 只负责聚合；顺带把 30 处 `any` 降下来 |
| `ui/src/services/types.ts` | 1578 | 与上面同口径按领域拆分，入口 re-export，不影响 import |
| `internal/app/app.go` | 1762 | 继续 v6.5 INT-2：把 `assembleDeps` 各阶段移到 `assemble_*.go` |
| `internal/api/agent_handler.go` | 1299 | 拆出 thread / run / SSE 三部分 |
| `internal/sbcli/commands.go` | 1534 | 按资源拆分（db / kv / s3 / fn / cron） |
| `ui/src/pages/AgentManager.vue` | 967 | 抽出 schedule 摘要（DC-5 下线后自然变小）和线程列表 |

`internal/api/kv_commands.go` 维持 v6.5 §4.6 的结论，不拆。

### OPT-7 文档漂移（P2）

`AGENTS.md`：
- 「typecheck 241 处 TS2307」→ 更新为实际情况（16 处，全在 tests）。
- 「`http-api.ts` 35 处 any」→ 30 处。
- plan 目录列表补上 `planv6.5` / `planv6.6`。

### OPT-8 仓库体积（P3，仅检查）

本地 `thirdparty/`（519M，只跟踪 README）、`skills/`（440M，7 个 submodule + 若干 eval 工作区）、`.skill-backups/`（2M）。需确认 `skills/code-refactor-workspace/**`、`.skill-backups/` 是否被误跟踪；如有，`git rm --cached` 并加忽略规则（**改 `.gitignore` 需审批**）。

---

## 3. 实施批次

| 批次 | 内容 | 风险 | 是否需审批 |
| --- | --- | --- | --- |
| **B1** | DC-1、DC-2、OPT-1、OPT-3、OPT-5 | 零风险：删零引用符号 + 测试与格式修正 | 否 |
| **B2** | DC-4、OPT-4、OPT-7、`gofunction` 弃用 API（OPT-2 低风险项） | 低 | 否 |
| **B3** | DC-3（逐项决定删除或接入）、OPT-2 中的 S3 endpoint 与 eino `ToolCallingChatModel` | 中 | 是：DC-3 涉及导出符号；S3 需要真实 S3 兼容存储验证 |
| **B4** | DC-5（`agent-schedules` 下线，含 v6.5 APV-1/3）、OPT-6、OPT-8 | 中 | 是 |

### 每批验证门禁

```bash
go build ./internal/... ./cmd/... && go vet ./... && go test ./...
(cd godatabase   && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... -timeout 900s)
(cd gofunction   && go vet ./... && go test ./... -short)
(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...)
(cd gosandbox    && go build ./... && go vet ./... && go test ./... && GOOS=linux CGO_ENABLED=0 go vet ./...)
cd ui && yarn build && yarn test && yarn typecheck
~/go/bin/staticcheck ./... && ~/go/bin/deadcode -test ./...   # 对比本计划基线，只减不增
# B4 额外：scripts/examples-verify/verify.sh --ports 18090,18091
```

### 预期收益

- Go：删除约 40 个无生产引用的符号；staticcheck 告警从 67 条降到约 10 条（只剩已登记保留的弃用项）。
- 前端：`typecheck` 清零并接入 CI；删除 3 个死导出和整组 `agentSchedules` 写接口。
- 一个文件名异常（`classifier-desens.go`）恢复为正常命名。

---

## 4. 执行状态（2026-11-16）

| 批次 | 状态 | 说明 |
| --- | --- | --- |
| **B1** | ✅ 已完成 | DC-1 全部删除。`WithLoginGuardAudit` / `WithLockHook` / `onLock` 整条链已删，`AuthHandler` 改为 `WithAudit(deps.Audit)`，并在 `router.go` 的两处 `NewAuthHandler` 接上。**原先审计服务从未注入**，`auth.login_failed` / `auth.login_locked` / `auth.password_initial` 此前都不会落库，现在会记录。DC-2 全部删除；`coverage_test.go` 的 `b` 字段保留（用于覆盖未导出字段分支），改为显式引用。OPT-1 已 `git mv` 到 `classifier.go`，并同步文件头注释。OPT-3 中 SA4006 全部补了断言：`/databases/` 尾斜杠实测返回 404；tools 5 项要求输出是合法 JSON、非 `is_error`、并包含桩数据；token stream 断言 EOF 且 finish=true。另外顺带修了 `gosandbox/vm/logged_test.go` 的 S1040。OPT-5 对 14 个文件执行了 `gofmt -w`，`scripts/smoke.sh` 加了 `gofmt -l` 门禁（排除 `examples/` 云函数源码：11 个文件只差末尾换行，属于样例资源） |
| **B2** | ✅ 已完成 | DC-4 的 3 个导出连同单测一起删除；`http.test.ts` 改为断言 `markUnauthorized`。OPT-4 `yarn typecheck` 已清零：S3Manager / Logs 的 `vm` 访问改用局部类型视图，`get().exists()` 改为 `find().exists()`，`NodeListOf` 改用 `Array.from`（**未改 tsconfig**），夹具按当前类型修正。gofunction 的 3 处弃用 API 已迁移。OPT-7 已更新 `AGENTS.md`（typecheck、any=30、plan 目录、gofmt 门禁、覆盖率基线） |
| B3 | ✅ 已完成 | DC-3：删除 `Registry.OpenedIDs`、`layoutFor`、`manifestSeqFromKey`、`IsCatalogEngineMismatch/Unknown`（测试改 `errors.Is`）；`WithNowFunc`/`rawCount` 移入 `export_test.go`；`HashRepo.Scan`/`ConnectStdio`/`KeyMeta.TTLms` 经查证有生产调用予以保留；`runFrame`/`testLayout`/`wsDir` 移入 `_test.go`；`Scheduler.RunOnce` 删除；`goexample` 删 `Poll`/`StartWorker`/`worker.go`/`Runtime.Compute` 等。`orphan_ledger` 评估后接入 prune 路径：`pruneVersions` 删除快照前记账，`CatalogSyncer` 派生 `MaintenanceGate`（修正 `testMeta()` 的 `ProjectID` 7 位校验失败问题后全包测试通过）。S3 已迁 `s3.Options.BaseEndpoint` 并跑通腾讯云 COS 真实冒烟（PutIfAbsent/Get/Delete）。eino 已迁 `ToolCallingChatModel`，删 `BindTools`，测试改不可变 `WithTools`；非 Context 版本加 `//lint:ignore SA1019` 保留注释。staticcheck 清零 |
| B4 | ✅ 已完成 | DC-5：`/agent-schedules` 整体下线——删 `agent_schedule_handler.go`（含测试）、router 7 条路由、`systemdb/agent_schedules.go`（DDL 保留防旧库启动失败，`TestAgentSchedulesTablesArchived` 锁定）；`b3_agent_routes_test.go` 改为断言路由不存在；E2E 17 的委托令牌测试改用 `/agent-threads`。前端删 `agentSchedules`/`llm.{chat,stream}`/`llmTraces.list` 整组 API 与类型，SSE 错误路径测试迁移到 `agentThreads.streamRun`；`AgentManager.vue` 的 `loadSchedules` 改从 `cronjobs.list`（targetKind=agent）映射，`agentScheduleFromCronJob` 从 `http-api.ts` re-export；文档 `agent.md`/`agent-tools.md` 同步改为指向 `/cron-jobs`。OPT-6：`app.go` 1762→1535（拆 `app_alerts.go`/`app_lease.go`）；`agent_handler.go` 1299→854（拆 `agent_run_handler.go` 498）；`sbcli/commands.go` 1534→417（拆 `commands_resources.go` 1157）；`http-api.ts` 2034→909（拆 `http-api-converters.ts` 1185）。OPT-8：`skills/code-refactor-workspace`、`.skill-backups/` 均未被 git 跟踪，`thirdparty/` 只跟踪 README，pack 10.27 MiB，无需处理 |

**遗留问题**

- 前端覆盖率：B4 后 functions 为 94.26%、branches 为 94.8%，仍低于 95% 阈值。主要缺口在 `http-api.ts`（拆分后转换函数集中于 `http-api-converters.ts`）。阈值问题在本计划之前就已存在（planv5.0 §7.2），需要单独补测。
- 「CI 中加入 `yarn typecheck`」：已在 `.github/workflows/build1.25.yml` 的 `ui` job 中，于 `yarn test` 之前加入。注意 CI 的 `yarn test` 仍会因上一条的覆盖率阈值失败（这是既有问题）。
- staticcheck：根模块与子模块均已清零（SA1019 弃用项已全部处理：迁移或加 `//lint:ignore` 保留注释）。
- `AgentSchedule` 前端类型保留为展示形状（数据来自 cronjobs 映射），不再对应后端资源。
