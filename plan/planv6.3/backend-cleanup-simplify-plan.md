# 后端代码清理与简化计划（v6.3，第二版）

> 范围：`internal/`（重点）、`gocloudagent/`、`godatabase/`、`gosandbox/`、`gofunction/`。
> 基线：`main` @ `7b41299` + 工作区大量未提交/未跟踪改动（`git status` 约 475 项）；行号以 2026-10-27 工作区为准，实施前重新定位。
> 依据：`skills/simplify-code/SKILL.md`（R-branch 只读评估 → 分批实施）、`references/assessment.md`（P1/P2/P3、风险、证据口径）、`references/languages-tier1.md` Go 节、`references/behavior-traps.md`；项目 `AGENTS.md`、`internal/AGENTS.md`、`internal/utils/README.md`。

## 执行状态（2026-10-27 已实施）

批次 A、B、C 已完成并通过验证；D/E 未执行（见下）。**业务代码已修改，与上文的"只规划"表述不再一致。**

| 批次 | 内容 | 状态 |
| --- | --- | --- |
| P0 | 修复 api 包编译（`protectReadonlyFields` 由并行工作补齐，本计划未改） | 已具备 |
| A1 | §3 D1–D6、D16、D18：api 包死代码 | **已完成** |
| A2 | §3 D7–D14、D17：app / auth / catalog / sandbox / sbcli / systemdb / usage 死代码 | **已完成** |
| A3 | §3 D15、D16（SA4006）与 D19（注释旧路径） | **已完成** |
| A4 | §5.1 G1–G2、§5.2 B1–B2、§5.3：嵌套模块死代码 | **已完成** |
| A5 | §6.2 转发函数内联、§6.3 utils 自清理 | **已完成** |
| A6 | §6.5 标准库替换、§6.6 agentbridge 内部简化 | **已完成** |
| B | §4.1 `agentjob` 并入 `agentbridge` + 三份文档同步 | **已完成** |
| C | §6.1 新建 `utils/sqlval`、§6.4 `sandbox.ClampPTYDim` | **已完成** |
| D | §4.2 云函数单一路径（N1） | **未执行**（需审批，且 api 包正在被并行修改） |
| E | §7 其余审批项（N2–N9）、§4.3 S1/S4 | **未执行** |

实施中相对计划的**偏差与新增**（计划原文未覆盖，实施时按同样标准处理）：

1. **`api/perfbench`**：`benchClock`、`DoRaw` 之外还删了 `stage_report.go` 里 `var _ = math.Round` / `prometheus.NewRegistry` / `json.Marshal` 三个纯占位引用，以及随之失效的 `math`、`time`、`prometheus` import。理由：占位引用只为压住"未使用 import"，删除后行为不变。
2. **`agentbridge/cron_agent.go` 的日志 Logger 名保持 `"agentjob"`**：包名虽已消失，但系统库 logs 表按该值检索，改名会改变可观察行为，因此保留并在代码里注明原因。
3. **`CronAgentPromptLimit` 复用**：`api/cronjob_handler.go` 的 `cronJobAgentPromptLimit` 改为引用 `agentbridge.CronAgentPromptLimit`（值不变，仍是 `16 * 1024`）。
4. **`singleflight.New` 一并删除**：零值 `Group[T]{}` 本身可用，`New` 只被本包测试调用；测试改为 `&Group[T]{}`。
5. **`internal/utils/mask.KeepEnds` + `repeated` 与 `audit.Redact` 三者一起删除**（`audit.Redact` 只调 `mask.KeepEnds`，且只被测试调用）。
6. **`TestSnapshotFromFactory`**：原用例测的是已删的 `snapshotFromFactory`（DuckLake）。改写为覆盖 `snapshotFromFileFactory`（filedb），断言 `MarkDirty` 后水位 +1，保留原用例意图。

验证结果（本次实际运行）：

```
go build ./internal/... ./cmd/...            OK
go vet ./internal/... ./cmd/...              OK
go test ./internal/... ./cmd/...             32 个包全部 ok（api 约 105s）
(cd gocloudagent && GOWORK=off go build/vet/test ./...)   OK
(cd godatabase  && GOWORK=off go build/vet/test ./... -timeout 900s)  OK（ducklake 142s、lease 32s）
(cd gofunction   && go build/vet/test ./... -short)       OK
(cd gosandbox    && go build/vet/test ./...)              OK
go test -race -short -count=1 ./internal/agentbridge      OK
```

静态分析前后对比：

| 检查 | 实施前 | 实施后 |
| --- | --- | --- |
| `staticcheck -checks U1000,S1*,SA4006,SA9003 ./internal/...`（仅生产文件） | 24 条 | **0 条** |
| `deadcode -test ./cmd/... ./internal/...`（仅 internal 生产文件） | 15 条 | **0 条** |
| 四个嵌套模块的同类 staticcheck（仅生产文件） | 4 条 | **0 条** |

未解决项（有意保留，见正文）：

- `gofmt -l` 仍有 4 个文件非规范：`internal/api/gocloudagent_access.go`、`internal/api/gocloudagent_access_test.go`、`internal/llmgateway/service.go`、`gocloudagent/cli_tool_planv6_test.go`、`gocloudagent/run_limits_test.go`。均属实施前既有状态，本次未触碰。
- `go test ./...` 在 `skills/simplify-code-eval-std/evals/files/20-go-minimal` 失败（eval fixture 缺 `readName`），与后端改动无关，属既有问题。

## 0. 本版相对第一版的更正

第一版有几条结论是错的，本版已改正，实施时以本版为准：

| 第一版说法 | 实际情况（证据） | 处理 |
| --- | --- | --- |
| `trigger` / `functionrun` / `event_fanout` 没有挂到生产路径，可以删掉 | `internal/app/app.go:217-227` 构造了 `functionrun.NewRunner`、`trigger.NewDispatcher`、`api.NewTriggerEventPublisher`，`router.go:562` 挂了 `FunctionTriggerHandler`。重新编译后 `deadcode ./cmd/...` 已不再把它们列为不可达 | **不删**。改为 §4.2：清掉 functionrun 接入后**留下的旧兜底路径** |
| gocloudagent 的 CLI 门面没链接，有 95 个函数不可达 | `gocloudagent/cmd/gocloudagent/main.go` 调用了 `RunCLI` / `SetCLIVersion`。那 95 个是只从 `cmd/simplebase` 算可达性时的假阳性 | 只保留**两个入口都不可达**的项（§5.1） |
| `godatabase/internal/{atomicfile,hashx,limit}` 可以改成 import `internal/utils` | `godatabase` 不能 import 根模块（AGENTS 硬约束）；反方向受 Go `internal` 规则限制，也不行。两份代码没法合成一份 | 保留两份，只删各自没用到的函数（§5.2） |
| `gofunction.NewExecutorPool` 是死代码 | 它是 `gofunction/README.md:151-175` 写明的公开 API | 不删；预留的空实现单独走审批（§7） |
| 预计减少约 860 行 | 没有依据 | 去掉这个数，改为逐项列出位置 |

## 1. 目标与边界

- **目标**：(1) 删除生产和测试都不用的代码，或只为测试存在的生产代码；(2) 合并职责重叠的小包（重点：`agentjob` 并入 `agentbridge`）；(3) 跨包重复的通用 helper 收到 `internal/utils` 或所属领域包；(4) 能用标准库（`cmp.Or`、`slices.Contains`、`strconv.Itoa`）的地方去掉手写实现；(5) 去掉 functionrun 接入后留下的双路径。
- **不变量**（SC Golden rules 1–2）：HTTP 路由、权限、中间件顺序、错误码和文本、JSON/SSE/CLI 格式、SQL 列序、锁和资源时序、配置优先级保持不变；系统库只读、`writable` 门禁、`sqlguard` 边界不动；不新增依赖、不改 `go.mod`、不改 schema。
- **模块边界**：`godatabase` / `gocloudagent` / `gosandbox` / `gofunction` 不得 import 根模块。所以 `internal/utils` 只服务根模块，嵌套模块里的重复只能在本模块内收敛。
- **语言版本**：五个模块的 `go.mod` 都是 `go 1.25.0`，可以用 `cmp.Or`（1.22+）、`slices`（1.21+）、`min`/`max` 内建函数。

## 2. 证据与基线（本次实际运行）

| 命令 | 结果 |
| --- | --- |
| `go build ./internal/... ./cmd/...` | **失败**：`internal/api/data_handler.go:366: undefined: protectReadonlyFields`（工作区里正在改的代码，不是本计划引入的）。之前一次运行时 `function_trigger_handler.go:151 c.Accepted undefined` 也报过错，后来已消失。**api 包修好之前，凡是涉及 api/app 的批次都不能建立基线** |
| `go vet ./internal/...` | 同上，api 包编译不过 |
| `go test -short -count=1 ./internal/{agentjob,agentbridge,cronjob,dbbridge,config} ./internal/utils/...` | 全部 `ok` |
| `(cd gocloudagent && GOWORK=off go vet ./...)` | ok |
| `staticcheck -checks U1000,S1*,SA4006,SA9003 ./internal/...` | 24 条落在生产代码（§3），其余在测试文件 |
| `deadcode ./cmd/...` / `deadcode -test ./cmd/... ./internal/...` | 根模块生产不可达 61 项，其中 15 项连测试也不用 |
| 嵌套模块按自身 `./cmd/...` 和 `-test ./...` 分别跑 deadcode，再与根模块结果取交集 | 见 §5 |
| `gofmt -l` | `internal/api/gocloudagent_access.go`、`internal/api/gocloudagent_access_test.go`、`internal/llmgateway/service.go`，以及 `gocloudagent/` 下两个测试文件 |

证据口径（assessment.md）：**已验证** = 上表命令结果；**推理** = 读了调用方和契约，但没执行；**待核对** = 实施前要先补证据。deadcode 不认识反射和 `//go:linkname`，每次删除前都要再 `grep -rn` 一遍。

## 3. P2/P3：根模块里可以直接删的死代码（推理，低风险）

> **状态：已完成。** D1–D19 全部落地，其中 D1–D6/D16/D18（A1）、D7–D14/D17（A2）、D15/D19（A3）。细节偏差见「执行状态」。

每项都是：staticcheck U1000 或 SA 报告，并且 `deadcode -test` 也不可达，`grep` 也没找到其他调用方。

| ID | 位置 | 动作 | 说明 |
| --- | --- | --- | --- |
| D1 | `api/kv_commands.go:70` `kvIntArgv`、`:83` `kvEmptyNone` | 删除 | 没有调用方 |
| D2 | `api/kv_commands.go:1194-1201` `kvRequestCtx` 全局变量、`kvBindContext` | 删全局变量和 `kvBindContext`，`kvCtx` 在 `c == nil` 时直接返回 `context.Background()` | `kvBindContext` 从没被调用过，所以 `kvRequestCtx` 一直是 `Background()`，改写完全等价；同时去掉了一个包级可变状态 |
| D3 | `api/collection_query.go:114` `selectDocumentsSQL`；`api/db_stats.go:91` `countTableRows` | 删除 | 已经被 `selectCollectionDocumentsSQL` / `countTableRowsWithDialect` 取代 |
| D4 | `api/policy_gate.go:12` `errKeyScopeDenied` | 删除 | 没有引用 |
| D5 | `api/sql_handler.go:83` `SQLHandler.mu` | 删除字段 | 没有读写 |
| D6 | `api/perfbench/stage_report.go:260` `benchClock`、`harness.go:315` `DoRaw` | 删除 | 没有调用方 |
| D7 | `app/app.go:74` `duckFactory`、`app.go:1133` `snapshotFromFactory` 及 `start_test.go` 中对应用例 | 删除 | 用户库已改成单文件 filedb，DuckLake 只给系统库用（`newSystemDatabaseFactory`）。这个字段没人赋值、没人读；函数只被测试用 |
| D8 | `auth/cache.go:143` `invalidateTenant`；`auth/observer.go:66-83` `authObservers` 类型 | 删除 | 注释写着"装配处使用"，实际没有实例化 |
| D9 | `auth/middleware.go:89-93` `AuthMiddleware` | 删除 | 生产只用 `AuthMiddlewareWithDelegation`，测试也没用到这个 |
| D10 | `catalog/service.go:55` `withDatabasePath` | 删除 | 已经被 `ResolveProjectStorage` 取代 |
| D11 | `sandbox/proc.go:109` `countProject` | 删除 | 没有调用方 |
| D12 | `sbcli/commands.go:265` `fs`；`sbcli/run.go:17-20` `Run` | 删除 | `cmd/simplebase` 直接调 `RunIO` |
| D13 | `systemdb/async_flush.go:31-33` `dropped` / `mu` / `closed` | 删除这三个字段 | 没有读写；真正的丢弃计数在 `Store.droppedLog` |
| D14 | `testutil/fakellm/server.go:79` `raw` | 删除字段 | 测试工具里没用到 |
| D15 | `auth/user_repository.go:95-101` | 删除前面那段拼 `q` / `args` 的代码，只保留下面的 if/else | SA4006：前面拼好的值被后面的 if/else 完全覆盖，删掉后执行的 SQL 和参数一致 |
| D16 | `api/sandbox_handler.go:668-671` | `row, err = h.svc.Start(...)` 改为 `_, err = ...` | SA4006：Start 返回的 `row` 紧接着被 `SetExpiry` 覆盖 |
| D17 | `usage/service.go:107-109` 空的 `if quota.MaxDatabases > 0 {}` | 删掉 if，保留注释 | SA9003 空分支 |
| D18 | `api/collection_query.go:209` | 去掉多余的 `fmt.Sprintf` | S1039 |
| D19 | 注释里的旧路径：`catalog/project_storage.go:16`、`observability/metrics.go:48,54`、`objectstore/blob.go:29,314`、`objectstore/keys.go:10`、`dbbridge/dbbridge.go:1`、`api/kv_commands.go:4`、`agentbridge/command.go:14`、`agentbridge/principal.go:5`、`agentjob/executor.go:5` | `internal/database/...` / `database/...` / `internal/gocloudagent` 改成 `godatabase/...` / `gocloudagent` | 只改注释 |

验证：`go test -short -count=1` 跑每个改动的包；D7 加跑 `./internal/app`；D15 加跑 `./internal/auth`（用户列表分页用例）。

## 4. P1：internal 包结构合并

> **状态：§4.1 已完成（agentjob → agentbridge）；§4.2 未执行（N1 需审批）；§4.3 S1/S4 未执行。**

### 4.1 `agentjob` 并入 `agentbridge`（用户点名）

**现状（已核实）**

- `internal/agentjob` 只有 `executor.go`（228 行）和 `executor_test.go`（453 行）。唯一的生产调用方是 `app/app.go:306,310`（`agentjob.Quota`、`agentjob.New`）。
- `agentjob` 依赖 `gocloudagent`、`agentbridge`、`auth`、`cronjob`、`systemdb`、`utils/strutil`，比 `agentbridge` 的依赖只多一个 `cronjob`。`cronjob` 只依赖 `systemdb` 和 `utils`，**不依赖 `agentbridge`**，合并后不会出现循环依赖。
- `agentbridge` 包注释和 `AGENTS.md:79`、`internal/AGENTS.md:31` 都写了"`agentbridge` 不得 import `agentjob`"。这条规则本身就说明两者是"宿主 ↔ gocloudagent 适配"这一个职责被拆成了两个包：`agentjob` 做的也是宿主类型（`systemdb.CronJob`、`auth.Principal`）和 `gocloudagent.RunRequest` 之间的转换。
- 符号冲突：核对过双方的导出名和测试文件里的顶层符号，**没有冲突**。但 `Store` / `Runner` / `Quota` / `New` 放进 `agentbridge` 后名字太泛，需要加前缀。

**方案**

1. 新建 `internal/agentbridge/cron_agent.go`，原样搬入 `executor.go` 的内容，导出名改为：`Executor` → `CronAgentExecutor`，`New` → `NewCronAgentExecutor`，`Store` → `CronAgentStore`，`Runner` → `CronAgentRunner`，`Quota` → `CronAgentQuota`，`PrincipalID` → `CronAgentPrincipalID`；`historyLimit` / `errorLimit` / `metricsWriter` 等私有名加 `cronAgent` 前缀，避免和包里已有符号撞名。函数体一字不改，`agentbridge.ToModulePrincipal(...)` 去掉包前缀直接调用。
2. `executor_test.go` → `agentbridge/cron_agent_test.go`，`package agentjob` 改成 `package agentbridge`，同步改符号名。
3. `app/app.go` 改为 import `agentbridge`，删掉对 `agentjob` 的 import。
4. 删除 `internal/agentjob/` 目录。
5. 同一批里顺手做的两处简化：
   - `SanitizeErr` 只是 `strutil.Truncate(err.Error(), errorLimit)` 加一个临时变量，`grep` 确认外部没有调用。内联成私有的 `cronAgentErr(err)`，测试 `TestSanitizeErr` 跟着改名，断言（nil → ""、截断到 300）不变。
   - `PromptLimit` 的注释说"API 层按它校验"，但 `api/cronjob_handler.go:37` 自己又定义了一个 `cronJobAgentPromptLimit = 16 * 1024`，同一个值存了两份。改为 `cronJobAgentPromptLimit = agentbridge.CronAgentPromptLimit`（api 本来就 import agentbridge），值不变。
6. 文档同步：`AGENTS.md:61,79`、`internal/AGENTS.md:12,31` 的依赖图和职责表里删掉 `agentjob`；在 `agentbridge` 的职责里加一条"定时任务 Agent 执行（实现 `cronjob.AgentRunner`）"；允许 import 清单加上 `cronjob`；`internal/utils/README.md:20` 的 `agentjob.SanitizeErr` 改成新名字。

**风险**：中。导出名变了，但 `internal/` 外部不可见，调用方只有 app，属于包内重命名。行为不变：函数体原样搬过来。
**验证**：`go build ./internal/... ./cmd/...`；`go test -short -count=1 ./internal/agentbridge ./internal/app ./internal/cronjob`；`go test -race -short -count=1 ./internal/agentbridge`；`grep -rn "agentjob" --include='*.go' .` 结果为 0。

### 4.2 云函数执行：functionrun 接入后留下的双路径

**现状（已核实）**

- 生产里执行云函数有 4 个入口：`api/gofunction_handler.go:683-701`（Invoke，有 runner 时）、同一函数 `:703-742`（没有 runner 时的旧路径）、`app/cron_runner.go:25-28`（有 runner 时）、同一文件 `:29-42`（没有 runner 时的旧路径）。另外还有 trigger 走 `functionrun.Runner.Run`。
- 生产里 runner 一定存在：`app.go:222-223` 在 `systemStore != nil` 时创建 `functionRunner`；GoFunction 路由只在 `deps.System != nil` 时挂载（`router.go:508,546`）；cron 调度器也只在 `systemStore != nil` 时创建（`app.go:303-304`）。**所以两个"没有 runner"的分支在生产里走不到**，只有 `api/gofunction_*_test.go`（7 处 `NewGoFunctionHandler`）和 `app/helpers_test.go:356,449` 会进去。
- 两条路径并不等价：旧路径不会把空输入补成 `{}`；cron 旧路径返回的错误文本是 `"gofunction not found: x"`，runner 路径直接返回 `sql.ErrNoRows`；handler 自己有一个 `runSem` 信号量（`:46-100`），runner 里也有一个 `sem`，两者容量相同。

**方案（Needs approval，因为测试可观察到的行为会变）**

1. 把 `functionrun.Runner` 定为唯一执行入口。`GoFunctionHandler` 去掉 `runSem` 和 `:703-742` 的旧路径；`acquireRunSlot` / `releaseRunSlot` 只委托给 runner（本来就是这样做的，见 `:79-95`）。调试台试跑那条路径（`:625` 直接调 `RunJSON`）也用 `runner.Acquire` / `Release`。
2. 在 `functionrun` 里新增 `(*Runner).RunFunction(ctx, projectID, funcFile, funcExport, input) ([]byte, error)`，按结构满足 `cronjob.Runner` 接口，不需要 import `cronjob`；内容就是现在 `cron_runner.go:26-27` 那两行。删除 `app/cron_runner.go` 和 `systemDBRunner`，`app.go:304` 直接传 `functionRunner`。
3. 测试改造：handler 测试构造时注入 `functionrun.NewRunner(store, n)`；`helpers_test.go` 里两个 `systemDBRunner` 用例改成测 `Runner.RunFunction`。
4. 会变的输入：(a) 没注入 runner 的测试 handler，现在走旧路径，改造后必须注入；(b) 在没有 runner 的测试配置下，cron 目标云函数不存在时，`sys_cron_job_runs.error` 由 `gofunction not found: x` 变成 `sql: no rows in result set`。**生产不受影响**，因为生产本来就走 runner 路径。如果希望保留这条文案，就让 `RunFunction` 把 `sql.ErrNoRows` / `ErrNoActiveVersion` 包装成原来的文本——这是推荐做法，这样 (b) 也没有差异。

**收益**：去掉一套重复的"解析生效版本 → 并发槽 → `RunJSON` → 记录调用"逻辑，以后只有一处地方决定空输入怎么处理、调用怎么落库。
**验证**：`go test -short -count=1 ./internal/api ./internal/app ./internal/functionrun ./internal/trigger`；对 `/go/...` 跑并发 503 用例。

### 4.3 其他包结构

| ID | 候选 | 结论 |
| --- | --- | --- |
| S1 | `internal/api/perfbench`（1712 行）只被 `cmd/perfbench` 使用，而且它 import 的是 `app`，不是 api 的子组件 | P3：挪到 `cmd/perfbench/internal/perfbench`（或 `internal/perfbench`），只改路径，`build.sh perf` 不受影响。可选 |
| S2 | `functionrun`（126 行）和 `trigger`（225 行）都只为云函数服务 | **不合并**：一个负责执行、一个负责事件分发，职责不同，依赖方向也已经是单向（trigger → functionrun）。做完 §4.2 之后 functionrun 才算完整的执行模块 |
| S3 | `realtime`、`e2bproto`、`collection`、`dbbridge`、`usage`、`audit` | 不合并：都是无依赖或依赖很少的独立模块，`dbbridge` 的边界是 AGENTS 规定的 |
| S4 | `internal/api` 有 60 个生产文件、77 个测试文件，其中按计划编号命名的测试文件（`b2_*`、`b3_*`、`c1_*`、`*_more_test.go`、`handler_branches_test.go`、`coverage_test.go` 等，各包都有） | P3：按主题并入对应的 `<topic>_test.go`，用例名不变。纯测试整理，单独成批，不和生产改动混在一起 |

## 5. 嵌套模块里的死代码

> **状态：已完成。** G1–G2、B1–B2、§5.3 全部落地；B3/B4（审批项）未执行。

### 5.1 gocloudagent（`cmd/gocloudagent` 和 `cmd/simplebase` 两个入口都不可达）

| ID | 位置 | 动作 | 风险 |
| --- | --- | --- | --- |
| G1 | `agentcore/model.go:382` `LooksLikeToolCall` | 删除，连同 `coverage_test.go` 里的用例 | 低 |
| G2 | `tools/fsutil.go:26` `globWalk`、`:113` `grepWalk`（只是 `*Skip(..., nil)` 的转发） | 删掉这两个转发，`tools_test.go` 直接调 `globWalkSkip` / `grepWalkSkip` 并传 `nil` | 低 |
| G3 | `skills.go` 里的 `KnownSkill`、`SkillPrefixes`、`KnownSkillIDs`、`SkillBody`、`EmbeddedSkillReadOnly`、`EmbeddedSkillAllows` 等 | **保留**：它们被 `internal/agentbridge/skills_test.go` 用来对账（AGENTS:82 规定对账测试放在宿主侧），属于有意的测试接口 | — |

### 5.2 godatabase（独立模块，根模块是唯一调用方）

| ID | 位置 | 动作 | 风险 / 审批 |
| --- | --- | --- | --- |
| B1 | `dialect.go:37` `IsReservedUserTableName`；`ducklake/local_state.go:35,40` `ReadLayoutState` / `SaveLayoutState`；`kv/executor.go:55` `Store.nowMs` | 删除（测试也不用） | 低；虽然是导出符号，但仓库里没有任何使用方 |
| B2 | `internal/atomicfile.WriteEnsureDir`、`internal/hashx.SumString` / `Short`、`internal/limit.Normalize`（只用了 `Clamp`）、`internal/singleflight.New`（待核对是否被泛型实例化） | 删除没用的函数和对应测试 | 低：都在模块内的 `internal`，外部用不到 |
| B3 | `registry/evictor.go`：`NewEvictor` / `Run` / `Stop`，只有测试在用，生产没装配 | Needs approval：删掉整个文件和测试（功能已经被 always-open 方案取代，待用 `git log` 确认） | 中，属于公开 API |
| B4 | `ducklake/orphan_ledger.go`：`OrphanLedger.Record`、`MaintenanceGate.Check`、`BlockedReason`；`ducklake/inspect.go` 的 `ListSnapshots` / `ListTableFiles`；`ducklake/errors.go` 的 `IsCatalogEngine{Mismatch,Unknown}`；`layoutFor`、`manifestSeqFromKey` | 待核对：生产不可达，只有测试用。如果确认是被放弃的 planv3.0 功能，和 B3 一起提交审批 | 中 |
| B5 | `spec.go:189-196` `NopObserver`、`testing.go` 的 `RecordingLogger` / `RecordingObserver` | 保留：前者是接口的空实现，后者是给测试用的公共工具 | — |
| B6 | `godatabase/internal/{atomicfile,hashx,limit}` 和 `internal/utils/*` 内容重复 | 保留两份，并在 `godatabase/README.md` 写明"这是刻意保留的副本，修改时两边要同步改" | — |

### 5.3 gofunction / gosandbox

- `value/tuple.go:98` `interfaceType`、`gosandbox/host/start.go:66` `process.once`：U1000，删除，低风险。
- `gofunction` 的 `NewExecutorPool` 等是 README 写明的公开 API，**保留**；`internal/interp` 的 `runFrame` / `testLayout` 被测试使用，保留。
- `gosandbox/vm/fake` 被 `internal/app`、`internal/sandbox` 的测试使用，保留。

## 6. 通用 helper 收敛（`internal/utils` 与领域包）

> **状态：§6.1–§6.6 已完成并更新 `internal/utils/README.md`。**

`internal/utils/README.md` 规定入库要满足三条：零业务依赖、至少两处真实复用、语义单一。下面的候选都按这三条筛过。

### 6.1 新增 `internal/utils/sqlval`（SQL 参数值转换）

| 现有实现 | 语义 |
| --- | --- |
| `collection/repository.go:187` `boolInt` → `int`；`auth/user_repository.go:264` `boolToInt` → `int64`；`systemdb/function_triggers.go:118` `triggerBoolInt` → `int64`；`api/kv_commands.go:1409` `kvBoolInt` → `int` | true → 1，false → 0 |
| `auth/user_repository.go:271`、`catalog/sql_repository.go:544` `nullTime(*time.Time) any`（**逐字相同**） | nil → nil，否则 → `t.UTC()` |
| `systemdb/sandboxes.go:320` `nullTimeValue(time.Time) any` | 零值 → nil，否则 → `t.UTC()` |
| `systemdb/cron_jobs.go:444` `nullString` | "" → nil |

- 新包导出 `BoolInt(bool) int64`、`NullTime(*time.Time) any`、`NullTimeValue(time.Time) any`、`NullString(string) any`，只 import 标准库。
- **行为陷阱**：`int` 和 `int64` 作为 `database/sql` 参数时，驱动都会转成 `int64`，所以前三处可以直接替换。`kvBoolInt` 的结果是作为 KV 命令的**返回值**序列化成 JSON 的（`kv_commands.go:124,144,164`），类型要保持 `int`，`any` 里装 `int64` 和装 `int` 在 JSON 上是一样的，但 fake 测试里可能做了类型断言——**`kvBoolInt` 这一处暂不替换**（skip，留着更稳）。
- 风险：低。验证：`go test -short ./internal/auth ./internal/catalog ./internal/systemdb ./internal/collection`。

### 6.2 已收进 utils 但调用方还在包一层的转发函数

| 位置 | 动作 |
| --- | --- |
| `cronjob/scheduler.go:343` `truncate` → `strutil.Truncate` | 内联 |
| `api/gofunction_handler.go:919` `truncateMessage` → `strutil.TruncateEllipsis` | 内联（5 处） |
| `api/gofunction_handler.go` `errWriterUnavailable()` → `godatabase.ErrWriterUnavailable` | 内联（7 处，另外 5 处本来就是直接写的），统一成同一种写法 |
| `agentbridge/command.go:131` `sensitiveFlag` → `mask.IsSensitiveKey` | 内联 |
| `api/sql_handler.go:513` `sqlSHA256`（只有测试在用；README 写它已被 `hashx` 取代） | 删除，测试改调 `hashx.SumString` |

这几项都是 SC Quick checklist 里"去掉没有附加含义的转发函数"，低风险。

### 6.3 utils 自身的清理（按 README 自己的入库标准）

- `strutil.TruncateRunes`、`atomicfile.WriteEnsureDir`、`hashx.Short` 在生产里没有调用方，删除，测试一并删掉。
- `mask.KeepEnds` / `repeated` 的唯一调用方是 `audit.Redact`，而 `audit.Redact` 也只有测试在用 → 三个一起删。注意 `mask.go:32` 的注释也引用了 `audit.Redact`，一并处理。
- `internal/utils/README.md` 的"现有包"表里列了 `sqlx` 和 `singleflight`，但 `internal/utils/` 下**没有这两个包**（实际位置分别是 `godatabase/sqlx` 和 `godatabase/internal/singleflight`）。改正这两行，并加上 §6.1 的 `sqlval`。

### 6.4 放回领域包，不放 utils

- `sandbox/sshd/session.go:201` `clamp` 和 `api/e2b_process_handler.go:252` `clampU16` **逐字相同**（PTY 尺寸，0 → 默认值，上限 1000）。这是 PTY 领域的规则，不是通用函数。收为 `sandbox.ClampPTYDim(v uint32, def uint16) uint16`（api 和 sshd 都已经 import 了 `sandbox`），`sandbox_handler_test.go:354` 的断言保持不变。低风险。

### 6.5 用标准库替换（嵌套模块内部）

| 位置 | 替换 | 等价性 |
| --- | --- | --- |
| `gocloudagent/runtime.go:628`、`cli/runner.go:308` `firstNonEmpty(a, b)` | `cmp.Or(a, b)` | 对 string 完全等价 |
| `gocloudagent/runtime.go:619` `containsSkill` | `slices.Contains` | 等价 |
| `gocloudagent/tools/fsutil.go:207`、`trace.go:100` 两份 `itoa` | `strconv.Itoa` | fsutil 那份对负数返回 `""`，但三个调用方的参数都 ≥ 0（search 的 limit 在 `:60-63` 已经归一化为正数；行号 ≥ 1；`len` ≥ 0）。推理等价 |
| `gocloudagent/prompt.go:80` `truncate` → `agentcore.Truncate` | 内联 | 等价 |
| `gocloudagent/tools/tools.go:479` `truncateText` vs `agentcore.Truncate` | **不合并**：一个按字节截断、一个按 rune 截断，标记文本也不同。看起来像，但不是同一个规则 | — |
| 根模块里 10 处 `sort.Strings` / 对 string 切片的 `sort.Slice` | P3 可选，换成 `slices.Sort`（string 全序，等价）。收益很小，不单独开批 | — |

### 6.6 `agentbridge` 内部的简化

- `principal.go:25-81` 中 `ToModulePrincipal` / `ToHostPrincipal` 有 4 段几乎一样的 map 拷贝循环。提取一个私有泛型 `convertSet[F, T ~string](m map[F]struct{}) map[T]struct{}`（nil → nil，否则 make 后逐个拷贝）。原代码对 ProjectIDs 是先 make 再在 nil 时置回 nil，结果一样。注意深拷贝的语义（注释里说明了原因）必须保留。低风险。
- `command.go:211-225` `RedactStatement`：`len(stmt) <= 200` 时函数已经返回，后面的 `if len(head) > 200` 恒为真，可以去掉。等价，P3。

## 7. 需要审批（会改行为或公开 API）

> **状态：N1–N9 全部未执行。** 本批只做无需审批的等价变换。

| ID | 内容 | 会变的输入或行为 |
| --- | --- | --- |
| N1 | §4.2 去掉云函数旧路径 | 没注入 runner 的测试配置；cron 错误文案（推荐包装成原文，可避免） |
| N2 | api 里"缺少 project context"有 4 种写法（32 处 `echo.NewHTTPError(400)`、31 处 `errors.New` → 走 `WriteError` 的默认映射、3 处 `APIError 500`、2 处 `fmt.Errorf`），想统一成一个 `requireProject(c)` | 中间件正常工作时走不到；但 status 和 code 会统一成一种，所以必须先决定用哪种 |
| N3 | 只读实例拒写有 3 种写法：`godatabase.ErrWriterUnavailable`、`cronjob_handler.go:260,310,396` 的 `readonly_mode` 503、`sandbox_handler.go:106` / `e2b_envd_handler.go:82` 的 `echo.NewHTTPError(503, "instance is read-only")` | 响应体里的 `code` 会变（`readonly_mode` / echo 默认 → `writer_unavailable`）。前端或 SDK 如果按 code 判断，会受影响 |
| N4 | `config/env.go:341-433` 有一组只给测试用的 `envBool` / `envInt` / `envInt64` / `envDuration` / `envFloat64` / `loadLLMProviders`，再加 `config.go:659` 的 `loadFromEnv`（23 处测试引用）；生产用的是另一组 `setEnv*`。两组解析逻辑可能已经不一致 | 先确认两组对非法值、空串的处理是否相同，再把测试改到 `applyEnv` 上，然后删掉测试专用那组。如果不同，旧测试其实没测到生产行为——这一点要记到"Bugs found" |
| N5 | `auth.APIKeyMiddleware` 只有测试在用 | 测试改成用 `AuthMiddlewareWithDelegation(nil, svc, inject, "", nil)`，前提是两者语义一致（待核对） |
| N6 | `observability` 的全局 `std` logger、`Default`、`ResetDefault`、`syncDefault`，根模块外部没人用 | 删除前先确认包内的 `Info` / `Warn` 这类包级变量没有其他用途 |
| N7 | `objectstore` 里只给测试用的 `AssertHealth`、`Descriptor.Validate`、`validateID`、`CatalogFileExt`；`dbbridge` 的 `CatalogOf`、`IsSystem`、`WriteGate`（注释写着"仅用于测试与过渡"）；`systemdb` 的 `ConnStats`、`ObserveConnWait`、`SystemTableCount`、`isDuplicate` | 和对应测试一起删。`CatalogFileExt` 出现在 `keys_golden_test.go`（锁定磁盘布局契约），golden 测试要改成直接调 `blob.CatalogFileExt`，**不能删掉这个断言** |
| N8 | `gofunction` 的预留空实现 `Executor.Compile`、`SetProfile` / `GetProfile`、`Runtime.Cleanup` | 外部调用方会编译失败；需要同步改 README |
| N9 | godatabase 的 B3 / B4 | 公开 API 删除 |

## 8. 明确不做（看着能简化，但不该动）

- `objectstore.KeyBuilder` 里 15 个转发到 `blob.KeyBuilder` 的方法：注释写明了为什么不用嵌入（要保持 `KeyBuilder{RootPrefix: ...}` 这种字面量写法）。
- `objectstore` 的内存版 FileStore / BlobStore 虽然只给测试用，但被 api、app、objectstore 三个包的测试依赖，挪走需要改导出面，收益很小。
- `api/context.go:97-98` 的 `fieldString` / `fieldDuration`（15 处）、`sshd.isErr`、`auth.newID`：转发函数，但调用点多，而且起到了统一命名的作用。
- `e2bproto.DecodeFrame`、`runEventHub.sweep`、`confirmHub.pendingCount`、`sbcli.DispatchKnown`：明确是给测试和对账用的入口。
- `agentbridge.InProcessCLI`：AGENTS 指定它供 api e2e 测试和单进程装配使用。
- 大文件拆分（`sbcli/commands.go` 1492 行、`api/kv_commands.go` 1422 行、`app/app.go` 1355 行）：v6.0 已经对 app 做过阶段提取；继续拆属于结构调整，不在本轮"清理"范围内。

## 9. 实施顺序与交付门

```
P0  修好 api 包的编译（protectReadonlyFields；由正在改这块的人负责）→ 建立 api/app 基线
A   §3 D1–D19 + §5.1 G1–G2 + §5.2 B1–B2 + §5.3 + §6.2 + §6.3 + §6.5 + §6.6   （机械删除和内联，按包分批）
B   §4.1 agentjob 并入 agentbridge（单独成批，含文档）
C   §6.1 utils/sqlval、§6.4 ClampPTYDim
D   §4.2 云函数单一路径（N1，审批后）
E   §7 其余审批项，逐项单独处理；§4.3 S1/S4 测试与目录整理放最后
```

每批都按 SC Workflow 的 Step 4–6 执行：一次只做一种变换 → 跑该包测试，对照 `behavior-traps.md` 想一想"什么输入会让新旧结果不同" → 完成后跑全量。

```bash
go build ./internal/... ./cmd/...
go vet ./internal/...
go test -short -count=1 ./internal/<改动包>
go test ./...                     # 每个阶段结束时跑
(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...)
(cd godatabase && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... -timeout 900s)
(cd gofunction && go test ./... -short) ; (cd gosandbox && go test ./...)
gofmt -l <改动文件>                # 只格式化改动的文件
git diff --check
~/go/bin/staticcheck -checks 'U1000,S1*,SA4006,SA9003' ./internal/...   # 对照 §3，已处理的项应当消失
~/go/bin/deadcode -test ./cmd/... ./internal/...
```

**停止条件**：要改导出签名、错误文本、权限或时序才能继续；基线本身就失败；文件被并行工作改过、分不清哪些是自己的改动——遇到任一情况，当前项停下，转成审批项或暂缓。不允许用 `git checkout`、`git stash`、`git reset` 清理工作区，不自动提交。

## 10. 交接模板

```text
Summary: <本批简化了什么>；行为/API 不变 | 需审批项见下
Changes: - <规则/类别>: <位置与原因>
Not changed: <跳过的诱人改动与原因>
Needs approval: <ID + 会变的输入>
Bugs found (not fixed): <位置、疑似/已确认、复现>
Scope: <包/文件>，基线 <HEAD>，排除项
Verification: <实际运行的命令 + 结果>；未运行的项 + 原因
```

## 附录 A：重新生成证据

```bash
cd /Volumes/my/github/SimpleBase
~/go/bin/staticcheck -checks 'U1000,S1*,SA4006,SA9003' ./internal/... ./cmd/... | grep -v _test.go
~/go/bin/deadcode ./cmd/... | grep '^internal/'                                # 生产不可达
~/go/bin/deadcode -test ./cmd/... ./internal/... | grep '^internal/' | grep -v _test.go   # 测试也不用
for m in gocloudagent gosandbox gofunction godatabase; do (cd $m && GOWORK=off ~/go/bin/deadcode -test ./...); done
# 嵌套模块要和根模块 `deadcode ./cmd/...` 的结果取交集，才能排除"另一个入口在用"的假阳性
```
