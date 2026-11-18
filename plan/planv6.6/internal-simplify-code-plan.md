# internal/ 代码精简与清理计划（v6.6，simplify-code 评估）

> 方法：`skills/simplify-code`（R-branch 只读评估）。本文是评估结果与分批方案，**本轮没有改动任何源码**，唯一写入是本文件。
> 承接：`plan/planv6.7/legacy-plan-consolidation-plan.md`（N1–N9 当时标为「需审批、未执行」）与本目录 `dead-code-and-optimization-plan.md`（B1–B4 已完成）。
> 本文只登记**新扫描**到的项，以及对 v6.3 N4–N7 的重新定级；v6.3 §8「明确不做」清单继续有效。

## 0. Scope 与基线

| 项 | 内容 |
| --- | --- |
| 模式 | 只读评估，源码未改 |
| 范围 | `internal/` 全部 26 个包，生产 238 个 `.go` 文件（约 5.6 万行）+ 测试；不含 `web/dist` |
| 修订 | HEAD `d855e65`，**工作区有未提交改动**（`internal/` 下 45 个文件已修改，另有未跟踪的 `e2bproto/watch.go`）。基线在工作区上测得 |
| 扫描 | `deadcode ./cmd/...`（不带 `-test`）、`deadcode -test`、`staticcheck -checks 'all,-ST*'`、`go vet`、`gofmt -l`、符号引用统计、文件头统计 |
| 限制 | 大函数与大文件只统计了行数，没有逐个通读；标注「未核实」的项不能当结论 |

基线（改动前，均已实际运行）：

| 命令 | 结果 |
| --- | --- |
| `go build ./internal/... ./cmd/...` | 通过 |
| `go vet ./internal/...` | 0 告警 |
| `go test ./internal/...` | 全部 ok（约 1 分 49 秒） |
| `gofmt -l internal cmd` | 1 个文件：`internal/e2bproto/watch.go`（未跟踪，缺末尾换行） |
| `staticcheck -checks 'all,-ST*'` | 4 条 SA9003（空分支），全在测试里 |
| `deadcode ./cmd/...` | `internal/` 下 43 个「生产不可达」符号 |
| `deadcode -test ./cmd/... ./internal/...` | `internal/` 下 **0 个** |

后两行的含义：`internal/` 里**没有彻底死掉的代码**，那 43 个全是「只有测试在调用的生产代码」。所以本轮的重点不是删，而是**把测试专用符号从生产二进制里挪走**，以及消除重复和过期注释。

## 1. 发现清单

优先级按可维护性收益定，不代表严重程度。风险与置信度分开写：置信度 = 已验证 / 推理 / 未核实。

### 1.1 注释与格式（P2，纯注释，零行为风险）

| ID | 位置 | 问题 → 方案 | 收益 | 风险 | 置信度 |
| --- | --- | --- | --- | --- | --- |
| S-1 | `e2bproto/watch.go` | 缺末尾换行 → `gofmt -w`。注意 `smoke.sh` 的门禁用 `git ls-files`，**未跟踪文件逃过检查**，提交前先 `git add` | 低 | 无 | 已验证 |
| S-2 | 78 个文件的头注释 | `[同包]` / `[关键引用]` / `同包调用` 仍写着已删除的 `agent_schedule_handler.go`（70 次）和 `agent_schedules.go`（33 次）。`systemdb/store.go`、`api/kv_commands.go` 等都中招 | 中 | 无 | 已验证（脚本逐个核对文件是否存在） |
| S-3 | 59 个文件的头注释 | `[上游]` 写着 `benchmarks/dbread`，该目录已不存在（已迁到 `godatabase/bench`） | 中 | 无 | 已验证 |
| S-4 | 全部生产文件头 | 头注释合计约 5700 行，201 个文件的头 ≥20 行，内含「调用架构 / 主要符号 / 等 N 个」，是生成物且已多处漂移。没有生成脚本留在仓库（`scripts/` 里没有）。见 §3 的决策点 | 中 | 无（仅注释） | 推理；还需确认没有测试或工具解析 `文件说明` 头 |

S-2、S-3 是确认的错误信息，应直接修。S-4 是「继续维护还是整体去掉」的取舍，见 §3 A1。

### 1.2 重复逻辑（P2）

| ID | 位置 | 问题 → 方案 | 收益 | 风险 | 置信度 |
| --- | --- | --- | --- | --- | --- |
| D-1 | `systemdb/seed.go:150` `isDuplicate` 与 `systemdb/gofuncs.go:392` `isGoFuncDuplicate` | 函数体逐字相同（nil→false；匹配 `UNIQUE` / `duplicate` / `already exists`）。合并为一个私有函数，两处调用点改名 | 中 | 低：同包私有，等价 | 已验证（两段源码对照）。`catalog/sql_repository.go:561` 的 `isUniqueViolation` 是否也能并入**未核实**，先读再定 |
| D-2 | `auth/middleware.go` `AuthMiddlewareWithDelegation` 的 JWT 分支 | 连续两个 `if secret != ""`，各自调一次 `VerifyJWT(secret, raw, time.Now())`：第一次判 `aud=enduser` 拒绝，第二次判委托令牌。合并为一次解析、两次判断，**判断顺序保持不变**（先 aud 后委托） | 中 | 中：鉴权关键路径；两次 `time.Now()` 变一次，差距在微秒级，仅在令牌恰好卡在过期边界时可见 | 推理。跑 `auth` 与 `api` 的 e2e 后再定 |
| D-3 | `auth/middleware.go` `APIKeyMiddleware` | 它的函数体与 `AuthMiddlewareWithDelegation` 末尾的 API Key 通道重复，且只有 `auth` 包内测试在用（`service_test.go`、`coverage_test.go`）。见 T-2，处理后重复自然消失 | 中 | 低 | 已验证 |

### 1.3 只有测试在用的生产代码（P2）

按处理方式分三类。**优先「挪进 `_test.go`」**：保留测试、行为零变化，只是让这些符号不再进生产二进制。动手前对每个符号先 grep **跨包**测试引用，有跨包引用的保持原位（原因见 §2）。

**甲类：移入同包 `_test.go`（保留测试）**

| ID | 符号 | 位置 | 说明 |
| --- | --- | --- | --- |
| T-1 | `envBool` / `envInt` / `envInt64` / `envDuration` / `envFloat64` / `loadLLMProviders` / `loadFromEnv` | `config/env.go:443-521`、`config/config.go:862` | v6.3 N4。生产走的是另一组 `setEnv*`。两组对非法值的处理其实一致：`env*` 返回默认值，`setEnv*` 保持目标原值，而目标就是默认值，所以结果相同。`loadLLMProviders` 内部就是 `defaults()` + `applyEnvLLM`。测试可保持不变，只换文件位置 |
| T-2 | `auth.APIKeyMiddleware` | `auth/middleware.go:45` | 只有 `auth` 包内测试在用。移走后 D-3 的重复在生产代码里消失 |
| T-3 | `loginGuard.WithClock`、`loginGuard.isExemptIP` | `api/login_guard.go:92,181` | 只有 `production_entry_test.go` 在用 |
| T-4 | `SystemTableCount` | `systemdb/schema.go:704` | 只在 systemdb 测试里当断言辅助 |
| T-5 | `observability.Default` / `ResetDefault` / `syncDefault` | `observability/zaplog.go:64,68,155` | v6.3 N6。删除前确认包级变量 `Info` / `Warn` / `Error` 没有别的用途（`ResetDefault` 会改写它们） |

**乙类：连同测试一起删（需确认）**

| ID | 符号 | 位置 | 说明 |
| --- | --- | --- | --- |
| T-6 | `dbbridge.CatalogOf` / `WriteGate` / `IsSystem` | `dbbridge/dbbridge.go:54,66,187` | `CatalogOf` 注释自己写着「仅用于测试与过渡」；`app/app_lease.go:97` 自己内联写了 `WriteGate` 闭包，没用这个适配器。测试测的是一个生产不用的适配器 |
| T-7 | `systemdb.ConnStats` / `ObserveConnWait` | `systemdb/observe.go:74,83` | 纯函数，生产里是 `Store.observeConnStats` 自己采样。测试只在测这两个纯函数 |
| T-8 | `objectstore.AssertHealth` | `objectstore/health.go:41` | 只是 `c.Check(ctx)` 加一个 nil 判断 |
| T-9 | `hashx.Short` | `utils/hashx/hashx.go:44` | 零生产调用；`godatabase` 有自己的 `hashx`。注意 `internal/utils/` 的约定是「零业务依赖纯函数」，没有调用方就不该留 |

**丙类：先决定「接线」还是「删」（行为相关，需审批）**

| ID | 符号 | 说明 |
| --- | --- | --- |
| T-10 | `backup.Scheduler.Skipped` + `skipped` 字段 | 只有测试读。若想让它有价值，应接到指标；否则删字段与方法 |
| T-11 | `objectstore.Descriptor.Validate` | 见 §2 Bug-1，**不能直接删** |

### 1.4 测试质量（P2）

| ID | 位置 | 问题 → 方案 | 风险 |
| --- | --- | --- | --- |
| Q-1 | `api/agent_more_test.go:174`、`auth/coverage_test.go:87`、`config/config_more_test.go:714`、`objectstore/coverage_test.go:135` | 4 处 `if cond { /* 只有注释 */ }`（SA9003）。这是**漏写的断言**，不是多余代码，不能只删。分别补：删除缺失线程应返回 404 或 204（否则 `t.Fatalf`）；撤销的 key 响应文本；`joinErrors(nil)` 的实际返回；`isAWSHost("")` 的实际返回（注释里写着「空串按实现算 AWS host」，应固化成断言） | 低。补完断言若失败，说明发现了真问题，记到 Bugs，**不要为了通过而放宽断言** |
| Q-2 | 29 个测试文件名带轮次或临时前缀：`b1_/b2_/b3_/b4_/c1_/c2_`、`dbg16_test.go`、`*_more_test.go`、`coverage_test.go` | 文件名无法说明测它什么。按领域重命名（如 `dbg16_test.go` 只读过文件头，它引入了 `cloudagent`，内容归属**未核实**，改名前先通读）。只改名不改内容，`git mv` 保留历史 | 低，但 blame 噪音大，P3，可放最后 |

### 1.5 结构（P3，仅提议，本轮不要求执行）

| ID | 位置 | 观察 | 说明 |
| --- | --- | --- | --- |
| R-1 | `sbcli/commands_resources.go` 1157 行 | 本目录 OPT-6 当时计划「按资源拆 db / kv / s3 / fn / cron」，实际只做了「从 `commands.go` 整体搬出」，自己又成了大文件 | 按资源再拆。纯文件搬移，同包同符号 |
| R-2 | `app/app.go` 1615 行 | OPT-6 完成时是 1535 行，现在涨了 80 行；`NewWithRegistry`（约 233 行）与 `assembleDeps`（约 223 行）仍是两大函数 | 继续 v6.5 INT-2：阶段移到 `assemble_*.go`。**装配顺序与失败回收顺序不能变**（`AGENTS.md`），建议单独一批，且在工作区改动落地之后做 |
| R-3 | `api/mountV1Routes`（约 234 行）、`cloudAgentHandler.CreateRun`（约 171 行）、`streamRun`（约 151 行） | 行数是用脚本估的，**未通读** | 未核实，本轮不下结论 |

> 提醒：我用脚本统计过「最长函数」，但脚本会把后面紧跟的 `type`/`var` 块算进去（例如 `kvParseFloat` 被算成 1116 行）。R-3 以外的行数不可信，不要据此拆分。

## 2. Bugs found (not fixed)

与精简无关，单独记录，不混入上面的计数。

| ID | 位置 | 级别 | 说明 |
| --- | --- | --- | --- |
| Bug-1 | `objectstore/descriptor.go:91` `Descriptor.Validate` | 怀疑（未复现） | 注释写「调用时机：`PutJSON` 前校验；从 S3 读取后再次校验，防止被篡改或格式漂移」，但生产代码里**没有任何调用点**，只有测试在调。要么是校验没接线，要么是注释失真。接线会改变读 descriptor 时遇到坏数据的行为（从静默接受变成报错），需审批；如果决定不接，就改注释并按 T-11 处理 |
| Bug-2 | `api/cronjob_handler.go:282,332,418`、`api/sandbox_handler.go:160`、`api/e2b_envd_handler.go:112`、`api/router.go:396` | 已确认的不一致 | 实例只读时的拒写响应有 4 种形态：`ErrWriterUnavailable`、`readonly_mode` 503、echo 默认 503 文本、`readonly_instance` 503（router 中间件）。前端或 SDK 若按 `code` 判断，会因路径不同而行为不同。这是 v6.3 N3，仍未处理。统一会改响应 `code`，需审批 |
| Bug-3 | `api` 的「缺少 project context」写法 | 已确认的不一致 | 6 处用 `echo.NewHTTPError` / `errors.New` / `NewAPIError` 并存（v6.3 N2）。`errors.New` 经 `WriteError` 会落进未知分支成 500。需审批，见 `AGENTS.md` 硬性约束第 9 条 |

## 3. 需要审批的决策

| ID | 决策 | 会变的输入或行为 |
| --- | --- | --- |
| A1 | S-4：文件头里的「调用架构 / 主要符号 / `[同包]` / `[上游]`」整体去掉，还是做一个生成脚本并接入 `smoke.sh` 校验 | 只影响注释。去掉：约 5000 行注释消失，blame 噪音大，但不再漂移；生成脚本：保留信息，但要维护脚本。不建议维持现状（继续手改会继续漂移） |
| A2 | 乙类 T-6 ~ T-9 连测试一起删 | 删掉对应的单测用例；覆盖率统计会变 |
| A3 | Bug-1 接线或改注释 | 接线：读到不合法 descriptor 时由静默接受变为报错 |
| A4 | Bug-2 / Bug-3 统一 | 响应体 `code` 或 HTTP 状态会变；先查前端 `ui/src` 与 `packages/js-sdk` 是否按 `code` 分支 |
| A5 | T-10 `Skipped` 接指标还是删 | 接指标会新增一个指标名 |
| A6 | D-2 合并 JWT 解析 | 令牌恰在过期边界的微秒级差异；见上 |

## 4. Not changed（看起来能简化，但不动）

- **v6.3 §8 全部延续**：`objectstore.KeyBuilder` 的 15 个转发方法、`objectstore` 内存版 FileStore / BlobStore、`api/context.go` 的 `fieldString` / `fieldDuration`、`e2bproto.DecodeFrame`、`runEventHub.sweep`、`confirmHub.pendingCount`、`sbcli.DispatchKnown`、`agentbridge.InProcessCLI`。
- `systemdb.NewStoreForTest`：生产文件里的测试构造器，但被 20 个测试文件跨包使用，挪走要改导出面，收益不抵成本。
- **系统库保护的重复判断**：`api` 里 `if catalog.IsSystemDatabase(db) { return ErrSystemProtected }` 出现在 `data_handler`、`schema_handler`（2 处）、`sql_handler`（2 处）、`kv_init` 等处。它们是 `AGENTS.md` 硬性约束第 1 条的落点，**保持显式、可 grep**，不要抽成隐式中间件。
- 写路径里 `if !h.writable { return WriteError(c, godatabase.ErrWriterUnavailable) }`（约 19 处）：形态一致，抽成 helper 能省行数，但会让「每个写 handler 都检查了 writable」这件事更难肉眼审查。收益有限，不做。
- `agentbridge.limitOut` 与 `gocloudagent/cloudagent` 的同名函数：口径不同（后者面对已被 `limitWriter` 按字节封顶的缓冲区，边界处理不同），且前者只服务测试用的 `InProcessCLI`，不合并。
- 6 处 `ctx == nil` 兜底（`sbcli/run.go`、`agentbridge/cli.go`、`cronjob/scheduler.go`、`api/stage_timing.go`、`catalog/tenant_ctx.go`）：防御性代码，调用方传 nil 的行为是可观察的，不删。
- `sbcli/dev_workspace.go` 里的 `io/ioutil`：是给脚本白名单用的字符串，不是 import，不能删。
- 21 处 `sort.Strings` / `sort.Slice`：换成 `slices.Sort` 收益很小，v6.3 已决定不单独开批。
- `config/yaml.go` 的 `rejectYAMLSecrets`、`config/env.go` 的 `applyEnv`：是平铺的字段清单，长但不复杂，拆开反而难对照。

## 5. 实施批次

每个批次内「一次只做一种变换，改完先跑该包测试，再对照 `behavior-traps.md` 想一遍『什么输入会让新旧不同』」。

| 批次 | 内容 | 风险 | 审批 |
| --- | --- | --- | --- |
| **B1** | S-1、S-2、S-3、D-1、Q-1 | 极低。S-2 / S-3 是注释；D-1 是同包私有函数合并；Q-1 只补断言 | 否 |
| **B2** | T-1 ~ T-5（甲类，移入 `_test.go`）、D-3（随 T-2 自然完成） | 低：只改文件归属，符号与测试内容不变 | 否 |
| **B3** | D-2、A1（头注释处置）、A2（乙类）、T-10 | 中 | 是：A1 / A2 / A5 / A6 |
| **B4** | Bug-1 / Bug-2 / Bug-3 的处置、Q-2、R-1 / R-2 | 中，涉及响应契约或大范围搬文件 | 是：A3 / A4 |

建议顺序 B1 → B2 → B3 → B4，**所有批次都在 `internal/` 的未提交改动落地之后再开始**（45 个文件在改，大范围改注释或搬文件会和它们冲突）。

### 动手前的固定检查

1. 移动或删除任何导出符号前，用 `rg '\bSymbol\b' --glob '*_test.go'` 确认**没有跨包**测试引用。有跨包引用的保持原位。
2. 「移到 `_test.go`」之后要确认没有文件靠它做 `var _ X = ...` 之类的编译期断言。
3. S-2 / S-3 改完用同样的脚本重扫，必须为 0。

### 每批验证门禁

```bash
go build ./internal/... ./cmd/... && go vet ./internal/... && go test ./internal/...
gofmt -l internal cmd                                   # 期望为空
staticcheck -checks 'all,-ST*' ./internal/... ./cmd/... # SA9003 清零，不新增
deadcode ./cmd/... | grep '^internal/' | wc -l          # 基线 43，B2 后应明显下降
deadcode -test ./cmd/... ./internal/... | grep '^internal/' | wc -l   # 必须仍为 0
# 涉及 sbcli / 删库 / 权限时再加：
go test ./internal/sbcli/ ./internal/app/ ./internal/auth/
```

### 预期收益

- 消除 78 + 59 个文件头里的错误引用，S-4 决定后不再漂移。
- 生产二进制中「只给测试用」的符号从 43 个降到约 20 个（B2 + B3 乙类之后；剩下的是 §4 明确保留的）。
- 2 处重复实现（`isDuplicate`、API Key 鉴权通道）收敛为 1 处。
- 4 处空断言补全，`staticcheck` 恢复 0 告警。
- 发现并登记 3 个不一致 / 疑似未接线问题，交由审批。

## 6. Verification（本轮评估）

- 已运行：见 §0 基线表，全部是只读命令；`go test ./internal/...` 通过，没有改动任何源码。
- 未运行：`gocloudagent` / `godatabase` / `gofunction` / `gosandbox` 独立模块的测试与扫描（不在 `internal/` 范围）；前端；`scripts/examples-verify`。
- 推理而非执行的结论：D-2 的等价性、T-1 中 `env*` 与 `setEnv*` 对非法值的一致性、T-5 的包级变量依赖。执行对应批次时必须实测。
- `deadcode` 不带 `-test` 的 43 项是该工具在当前构建标签下的结果；条件编译或反射注册的符号不在此列。
