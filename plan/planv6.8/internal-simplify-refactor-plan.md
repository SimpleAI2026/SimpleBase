# internal/ 重构、精简与清理计划（planv6.8，simplify-code）

> **状态：✅ 已完成并关闭（2026-11-24 复核）。** v3 的 P0–P5（§8）与 v4 的第二轮候选（§10，含 §9.2 A-2）均已实施，Bug-1~6 全部修复；S-6 / S-9 / G-1 / G-4 / A-5 / O-12~O-14 为有意 skip / 可选未做（§9.2）。2026-11-24 门禁复核全部通过：`go build / vet ./internal/... ./cmd/...`、`go test ./internal/...`（30 包全 ok）、gocloudagent 与 gosandbox 的 `GOWORK=off` build / vet / test、前端 `tsc --noEmit` 0 错误 + `vitest` 160 文件 / 1519 用例全绿 + `yarn build`（500 kB chunk 为既有警告）。复核时修复了两处在途编译错误（`gocloudagent/mcp/client.go` 缺 `encoding/json`、`gocloudagent/cli/repl.go` 缺 `mcp` 导入，属 gocloudagent 清理改动的遗漏，不在本计划条目内）。改动仍在工作区未提交；§8.1 / §10.1 列明的「未做」（启动冒烟、`scripts/smoke.sh`、真实 DuckDB / S3 集成）保持开口。
> 方法：`skills/simplify-code`，先走 R-branch（只读评估）。v1/v2 只写本文件；v3（2026-11-23）：P0–P5 已实施（未提交），逐项状态、偏差与实测门禁见 §8，实施后的第二轮候选见 §9；v4（2026-11-23）：第二轮候选已全部实施（§10）。
> 承接：`plan/planv6.6/internal-simplify-code-plan.md`（B1–B4 已执行：S-2/S-3 头注释错引、D-1 局部、T-1~T-9、Bug-2/Bug-3 已核实清零，见 §0.3）。
> 目标：让 `internal/` **更短的函数、更少的重复知识、一处定义一个规则**。
>
> **v2 修订（2026-11-23，已审批，策略 = 激进）**：
> - **A1 文件头注释不删除**：C-1 改为「保留并随结构调整同步更新」，不再计入删减收益（§1.1、§6.3）。
> - **A4 操作人标识统一，不兼容旧数据**：所有落库 / 比较 / 透传的操作人一律走 `auth.Principal.Actor()`，格式 `user:<id> | key:<id> | system:<name>`；删除 `principalIdentity`、`principalKey`、`actorID`、`principalActor` 及所有内联拼接；历史行不迁移（§1.9）。
> - **A8 `agentbridge/bench`、`agentbridge/docs` 迁出 `internal/`**（§1.10）。
> - **其余待审批项按激进策略默认处理**：不保留兼容包装、不保留只给测试用的生产符号、已确认的 bug 在对应批次直接修（§3）。唯一仍不做的是 G-1 配置五处登记的结构改造（风险与收益不对等，列为可选 P6）。
> - 对外契约中**允许变化**的只有：操作人字段取值格式（`created_by` / `initiator_id` / `resolved_by` / `assignee_id` / 审计 `principal_id` / `actor`）、委托 JWT 的 `sub`、`function-triggers` 路由多一道 `policyGate`。HTTP 路由路径、错误码、JSON 字段名、CLI 输出结构、系统库表结构、S3 key 布局仍不变。
>
> **v3 回写（2026-11-23，第二轮评估；本轮只改本文件，源码零改动）**：
> - **状态**：§1 的 77 个条目 = 63 已完成 / 6 部分完成（落点或范围与计划不同）/ 8 按计划可选未做；Bug-1 ~ Bug-6 全部修复。逐项见 §8.2。下文 §0–§7 是评估当时的记录，保留作依据；与 §8/§9 冲突处以 §8/§9 为准。
> - **门禁**：build / vet / test / gofmt / staticcheck / deadcode -test 全部达标；deadcode 生产项 14（逐项等于 §5 保留清单）；路由集合与 HEAD 逐条一致。实测值见 §8.1。
> - **对计划自身的修正**：§4 目标结构与 §6.2 门禁里和实际不一致的地方已就地标注「实际：」；新增 §9 列出实施后才看得出的候选，并把「评估后不做」的项写明理由，避免下次重评。

## 0. Scope 与基线

### 0.1 范围

| 项 | 内容 |
| --- | --- |
| 模式 | 只读评估；所有条目按 apply / propose / skip 分级 |
| 范围 | `internal/` 31 个包，生产 `.go` 约 **6.4 万行**（`api` 3.07 万、`systemdb` 0.96 万、`sbcli` 0.42 万、`app` 0.35 万、`auth` 0.34 万、`config` 0.32 万、`agentbridge` 0.30 万…），测试约 5.1 万行 |
| 排除 | `web/dist`（构建产物）、`agentbridge/bench/*.py`（评测脚本，非 Go）、`agentbridge/agentskills/simplebase/*.md`（技能文档）、`testdata/` |
| 修订 | HEAD `f494d99`；**工作区有大量未提交改动**：`internal/` 下 209 个条目（151 个已跟踪文件改动 +2780/−3257，另有约 58 个未跟踪新文件：`agentbridge/{callgate,cliexec,context,modules,plan_*,ports,prompt,runtime_provider,skilltool,tools*}.go`、`api/{agent_dispatch,agent_group_handler,agent_tag_handler,collab_*,mcp_builtin*,mcp_dispatch,plan_handler,public_base}.go`、`systemdb/{agent_collab*,agent_tags,plans}.go`） |
| 工具 | `go build`、`go vet`、`gofmt -l`、`staticcheck -checks 'all,-ST*'`、`deadcode`（±`-test`）、自写 AST 脚本（函数行数 / 嵌套深度 / token 归一化结构重复），以及 `rg` 逐项核对 |

### 0.2 基线（改动前，已实际运行）

| 命令 | 结果 |
| --- | --- |
| `go build ./internal/... ./cmd/...` | 通过 |
| `go vet ./internal/...` | 0 告警 |
| `go test ./internal/...` | 全部 ok（`api/perfbench`、`testutil/fakellm` 无测试文件） |
| `gofmt -l internal cmd` | 1 个：`cmd/simplebase/main.go`（缺末尾换行；不在 internal，但同批顺手修） |
| `staticcheck -checks 'all,-ST*' ./internal/...` | 1 条：`api/plan_handler_test.go:80 errorCode is unused (U1000)` |
| `deadcode ./cmd/...`（生产不可达） | `internal/` 下 **24** 个（v6.6 时 43 个） |
| `deadcode -test ./cmd/... ./internal/...` | 1 个：同上 `errorCode`（**真正的死代码**） |
| 文件头注释（AST 统计） | 生产文件头合计 **5639 行**；其中 **195 个文件**含「调用架构 / 主要符号 / [同包] / [上游]」生成式段落 |
| 注释中的计划编号 | 148 个生产文件、**573 处** `planvX.Y / §N / xxx-plan §` 引用 |

### 0.3 v6.6 项目回核（以代码为准）

| v6.6 ID | 现状 | 结论 |
| --- | --- | --- |
| S-1、S-2、S-3 | `e2bproto/watch.go` 已格式化；`agent_schedule*.go`、`benchmarks/dbread` 引用 0 处；`[同包]/[关键引用]` 中的文件名逐个核对**全部存在** | 完成 |
| D-1 | `isGoFuncDuplicate` 已并入 `systemdb/seed.go isDuplicate`；`catalog/sql_repository.go isUniqueViolation` **口径不同**（只认 `UNIQUE constraint failed` / `constraint failed: UNIQUE` / `duplicate`，不认 `already exists`）| 完成；`isUniqueViolation` 不并（见 §5） |
| D-2/D-3、T-1~T-9 | `APIKeyMiddleware`、`env*`、`loadFromEnv`、`SystemTableCount`、`ResetDefault`、`CatalogOf`、`ConnStats`、`AssertHealth`、`Skipped()` 生产代码中均为 0 | 完成 |
| T-11 / Bug-1 | `Descriptor.Validate` 已不存在 | 完成 |
| Bug-2 / Bug-3 | `readonly_mode`、`readonly_instance`、`missing project context` 生产代码 0 处；缺项目统一 `ErrProjectMissing` | 完成 |
| Q-2 | 带轮次前缀的测试文件已改名；残留 `agentbridge/tools_more_test.go`、`*_branches_test.go`（4 个） | 本轮 §1.8 Q-1 |
| R-1 | `sbcli/commands_resources.go` 已按资源拆为 `commands_{database,collection,function,cron,kv,object,sandbox,admin,backup}.go` | 完成 |
| R-2 | `app/app.go` 1615 → 849 行，`assemble_*.go` 已拆出；但 `NewWithRegistry`（236 行）与 `assembleDeps`（219 行，嵌套 5）仍是两大函数 | 本轮 §1.6 A-1/A-2 继续 |
| R-3 | `mountV1Routes` 246 行（v6.6 估 234）；`CreateRun` 已拆成 `createDirectRun`/`createAutoRun`，但 `createDirectRun` 129 行、`runStream` 131 行 | 本轮 §1.5 |

**结论**：v6.6 的「错误信息 / 测试专用符号」清理已基本到位，本轮重点转向 **结构与重复知识**——大函数、复制粘贴的仓储样板、三套主体标识口径、生成式头注释与计划编号注释的整体处置。

## 1. 发现清单（按文件落点）

优先级 = 可维护性收益（P1 大 / P2 中 / P3 可选），与风险、置信度分开写。置信度：**已验证**（读过源码 + 命令核对）/ **推理**（读过契约未执行）/ **未核实**。
处置：**apply**（等价且有测试兜底，可直接做）/ **propose**（需审批）/ **skip**。

### 1.1 注释治理（P1，纯注释，零行为风险，收益最大）

| ID | 文件 / 位置 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| C-1 | 195 个生产文件头（如 `api/router.go:1-31`、`systemdb/agents.go:1-29`） | 「文件说明 / 用法 / 调用架构 / 主要符号」段落**保留**（A1 已决）。由于本计划会拆分、合并、删除大量文件与符号，头注释必然漂移，因此改为**同步维护规则**：① 被拆的文件，原文件头的「用法 / 主要符号 / [同包]」按拆后实际内容重写；② 新文件按同一格式写完整文件头（文件说明 + 用法 + 调用架构 + 主要符号）；③ 被删除的符号 / 文件，`rg` 全仓头注释中的引用一并改掉；④ 每批结束跑 §6.1-7 的漂移脚本，结果必须为 0 | 无（仅注释） | 已验证 | **apply（A1 = 保留 + 同步）** |
| C-2 | 148 个文件、573 处计划编号注释（`planv6.0 D1`、`§8.1.6`、`P0-3`、`WR-01`、`SEC-04`、`AGC-ROUTE §3.3`、`B3 提取` 等） | 旧计划目录已删（见 planv6.7 §2 锚点表），读者需跳两层才知含义。→ **规则**：注释写「为什么」本身，编号最多保留一个作为可 grep 的锚点。例：`router.go:387` `// require 不带 delegable：路由对委托令牌默认拒绝（planv6.0 D1）` 已自解释，保留；`router.go:605` `// 云 Agent 路由块（B3 提取；…）`、`router.go:617`、`router.go:632-635`、`router.go:670-675` 中「B3 提取 / 原 mountV1Routes 尾部连续块」是**重构历史**，删除 | 无 | 已验证（抽样 40 处） | apply（只在本计划动到的文件里处理，不单开全仓批次） |
| C-3 | `api/router.go:252-255` | `NewRouter` 注释「按 plan1 规定的中间件顺序…业务路由由后续 plan 在此函数内挂载」已失真（顺序实际还有 site/CORS/security/perf/gzip/ratelimit）。→ 改为与代码一致的顺序列表 | 无 | 已验证 | apply |
| C-4 | `app/app.go:674-676` | `Start` 有两段重复的 doc（「Start 启动 HTTP server。阻塞调用者直到 Shutdown。」+「Start 启动 HTTP server（阻塞直到关闭）…」）→ 合并为一段 | 无 | 已验证 | apply |
| C-5 | `systemdb/cron_jobs.go:426` | 「v49 新增列由 ALTER ... DEFAULT 补齐」与 `AGENTS.md`「无版本化迁移、改 schema.go 并 reset」矛盾 → 核实 `schema.go` 中这些列是否 `NOT NULL DEFAULT`；若是，`NullString` 兜底可留但注释改为「防御旧行」；若列为 `NOT NULL`，激进处理：去掉 `NullString` 兜底直接扫描到 `string` | 低 | 推理 | apply（P2 批先读 `schema.go` 再定） |
| C-6 | `internal/README.md` | 仍描述已不存在的 `database/`（ducklake/registry/cache/sqlguard）、`llm_handler.go` 的 chat/stream/providers、`gocloudagent` 为 internal 模块；依赖图与 `AGENTS.md` 不一致；漏列 `agentbridge`、`sandbox`、`sbcli`、`systemdb`、`functionrun`、`trigger`、`collection`、`dbbridge`、`e2bproto`、`backup`、`alerts`、`realtime`、`cronjob` | 无 | 已验证 | apply：按 `go list` 实际 import 重写模块清单与依赖图（见 §6 附录 A） |

### 1.2 `api` 包：路由装配（P1）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| R-1 | `api/router.go:385` `mountV1Routes`（246 行） | 一个函数装 15 组资源。→ **按资源拆成同签名小函数**，`mountV1Routes` 只剩顺序调用：`mountAuthRoutes(v1)`、`mountProjectRoutes(v1,p)`、`mountDatabaseRoutes(p)`（db/sql/schema/data/meta）、`mountBackupRealtimeRoutes`、`mountKVRoutes`、`mountLLMRoutes`（含双段 v1 注释原样搬迁）、`mountUsageAuditRoutes`、`mountS3Routes`、`mountSystemRoutes`（metrics/logs/settings/llm traces/migrations）。把 `require` / `delegable` / `writeGate` 三个闭包提为一个小结构 `routeGuards{require, delegable, writeGate}`，替代现在 `mountAgentRoutes(p, deps, require, writeGate, bm)`、`mountGoFunctionAndCronRoutes(p, deps, delegable, writeGate)` 这种「闭包当参数逐个传」的签名 | 中：**注册顺序**影响 echo 路由匹配与 `buildOpenAPI(e.Routes())` 输出顺序；`writeGateRoutes` 驱动测试、`route_meta_test.go`、`TestOpenAIRoutePaths` 锁定路径 | 已验证（读过全文） | apply：纯搬移，**保持调用顺序与每行注册顺序完全一致**；验收加一条「`e.Routes()` 序列化前后逐行 diff 为空」的临时测试 |
| R-2 | `api/router.go:659-666` | `function-triggers` 6 条路由直接写 `auth.Require(auth.ProjectAdmin, PrincipalFromContext)`，**绕过了 `policyGate`**，与同文件其余路由（`require()` = `policyGate(auth.Require(...))`）不一致 | 行为变化：受 policy 限制的 API Key 在这 6 条路由上可能变成 403 | 已验证 | **apply（激进）**：改为 `g.require(...)`，与 R-1 的 `routeGuards` 同批；补一条「受限 Key 访问 function-triggers 被拒」测试 |
| R-3 | `api/router.go:796-798` | `for _, prefix := range []string{e2bPrefix + "/*"} { e.Any(prefix, …) }` 单元素循环 → `e.Any(e2bPrefix+"/*", sh.unavailable)` | 低 | 已验证 | apply |
| R-4 | `api/router.go:780` `mountE2BRoutes`（103 行） | 控制面 `/sandboxes/...` 与 `/v2/sandboxes/...` 成对重复注册 10 组。→ 局部 `both := func(method, path string, h echo.HandlerFunc)` 一次登记两条，顺序按「v1 → v2」保持 | 中：注册顺序有变化时 OpenAPI 输出顺序变 | 已验证 | apply（保持原相对顺序：现代码有时 v1/v2 交错、有时成组，需逐行对照；若无法保持就 skip） |
| R-5 | `api/router.go:256` `NewRouter`（113 行） | 中间件链（site → requestID → CORS → security → perf → recover → accessLog → gzip → preAuth limit → bodyLimit）抽 `useGlobalMiddleware(e, deps)`；匿名登录/终端用户路由抽 `mountPublicAuthRoutes(e, deps)` | 低 | 已验证 | apply |
| R-6 | `api/router.go:676` `mountAgentRoutes` | 函数内既**装配**（confirmHub 注入 `deps.CloudAgent.Confirm` / provider、`newLLMDispatcher`、`CollabRunner.Attach`、`convergeCollaboration` 启动期副作用）又**挂路由**。→ 装配部分抽 `newCloudAgentHandler(deps) *cloudAgentHandler`；`convergeCollaboration(context.Background())` 是启动期 I/O，**不能**因拆分而改变调用时机（仍在路由注册前） | 中：副作用顺序 | 已验证 | apply（仅抽函数，不改调用点顺序） |

### 1.3 `api` 包：handler 重复知识（P1/P2）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| H-1 | 主体标识多套口径 | **A4 已决：激进统一**，完整方案、调用点清单与测试改动见 **§1.9 ID-1 ~ ID-8** | 高（持久化格式变化，已批准不兼容） | 已验证 | **apply（§1.9）** |
| H-2 | `api/cronjob_handler.go:797`、`api/gofunction_handler.go:794`、`api/llm_handler.go:168`、`api/mcp_builtin_admin.go:114`、`api/s3_site_handler.go:243`、`api/backup_handler.go:146`、`api/sandbox_handler.go:187` 七个审计写入点 | 结构相同：`audit==nil` 早退 → 取主体 → 拼 `AuditEvent{ProjectID, PrincipalID, Kind, RequestID, Status, Detail}`。→ 包级 `recordAudit(c echo.Context, svc AuditService, kind, projectID, status, detail string)`：内部统一 `PrincipalID = principalOf(c).Actor()`、`RequestID = requestID(c)`、ctx 统一用 `context.WithoutCancel(c.Request().Context())`（客户端断开不丢审计；cron 版原来的 `context.Background()` 与 gofunction 版的请求 ctx 两种写法一并收敛）。`s3_site_handler` 走的是 `systemdb.RecordAudit`（另一张表 `sys_audit_events`），保留独立入口但 `Actor` 同样取 `Actor()` | 中：ctx 语义统一为「不随请求取消」 | 已验证 | **apply（激进，与 §1.9 同批）** |
| H-3 | 只读实例拒写 4 种写法：`DataHandler.writeGuard`（返回 err）、`FunctionTriggerHandler.writeGuard`、`cloudAgentHandler.denyReadonly`（`*bool`，直接写响应）、15 处内联 `if !h.writable`；字段类型有 `bool`（12 个 handler）与 `*bool`（`S3Handler`、`cloudAgentHandler`）两种 | 路由层已有统一 `writeGate`（router.go:402）。v6.6 §4 决定「handler 内显式检查不抽 helper」，本轮维持；**只统一类型**：`*bool` 两处改 `bool`（`&deps.Config.Instance.Writable` 指向的是 `Dependencies` 值拷贝里的字段，运行期不会变，指针无意义） | 低：测试里的 `deps.Config.Instance.Writable = false`（5 处）都发生在 `NewRouter` 之前，构造期拷贝即可覆盖；`mountV1Routes(e, deps Dependencies)` 是值参数，指针指向函数内副本，外部无法翻转 | 已验证 | apply |
| H-4 | `api/agent_run_handler.go:906 pcOfContext`、`api/data_handler.go:873 projectID/projectIDFrom`、`api/collab_handler.go:240 principalOf`，以及 102 处 `pc, ok := ProjectFromContext(c.Request().Context()); if !ok { return WriteError(c, ErrProjectMissing) }` | 三个「吞掉 ok」的薄包装分散在三个业务文件里，且与 `context.go` 的正式访问器并存。→ 全部挪到 `api/context.go`，命名统一为 `projectOf(c)` / `principalOf(c)`；新增 `requireProject(c) (ProjectContext, error)`（返回 `ErrProjectMissing`），102 处样板逐文件替换为 `pc, err := requireProject(c); if err != nil { return WriteError(c, err) }` | 低：等价；行数收益约 −200 | 已验证 | apply（按文件分批，同批跑 `go test ./internal/api/`） |
| H-5 | 146 处 `RequestIDFromContext(c.Request().Context())`；68 处 `NewAPIError(http.StatusBadRequest, "invalid_request", msg, rid)` | → `context.go` 增加 `requestID(c echo.Context) string`；`error.go` 增加 `badRequest(c, msg) *APIError`（只封装 `invalid_request` 这一种 code，**其它 code 不收**，避免造万能工厂） | 低 | 已验证 | apply |
| H-6 | `api/collection_aggregate.go:59-78` 与 `api/schema_aggregate.go:23-42` | `WithAggregateLimits` + `aggregateLimits` 逐字相同（结构重复检测命中）。→ 抽 `type aggregateLimits struct{ timeout time.Duration; scan int }`，带 `with(timeout, scan)` 与 `effective()` 两个方法，两个 handler 内嵌该字段；导出方法 `WithAggregateLimits` 签名不变（`app` 在调用） | 低 | 已验证 | apply |
| H-7 | `api/migration_handler.go:46-72` `Status` / `Plan` | 仅差一个函数与一句文案。→ 私有 `serveAdminOnly(c, what string, fn func(ctx, *sql.DB) (any, error))`；错误文案 `"migration status requires the admin project"` / `"migration plan …"` 必须逐字保留 | 低 | 已验证 | apply |
| H-8 | `api/oai_dto.go:319 newOAIID` 与 `api/oai_responses.go:502 newRespID` | 仅分隔符不同（`-` vs `_`），属 OpenAI 两套协议的 ID 格式，**是有意差异**。→ 合并为 `randomID(prefix, sep string)`，两个原名保留为一行包装或直接替换调用点 | 低 | 已验证 | apply |
| H-9 | `api/agent_handler.go` 1082 行，混合 agent CRUD、thread CRUD、approval、run cancel、models、DTO 转换 | 按资源拆文件（同包同符号，零逻辑改动）：`agent_crud_handler.go`（ListModules/ListAgents/Create/Get/Patch/Delete/normalizeAgent）、`agent_thread_handler.go`（Threads + Messages）、`agent_approval_handler.go`（approval DTO + ListApprovals + CancelRun + canCancelConfirm）、`agent_dto.go`（全部 `*DTO` 与 `to*DTO`）；`agent_handler.go` 只留 `cloudAgentHandler` 结构体与小访问器 | 低：纯移动，`git mv` + 剪切 | 已验证 | apply |
| H-10 | `api/agent_handler.go:373` 方法 `normalizeAgent` 与同文件 `:398` 函数 `normalizeAgent` 同名 | 读代码时易混。→ 方法改名 `validateAgentCapabilities`（只做能力校验那一半），函数保持 | 低（私有） | 已验证 | apply |
| H-11 | `api/kv_commands.go` 1411 行，其中 `kvCommandTable` 是**一个 1100 行的 map 字面量**（62 条命令） | 按 Redis 类型拆文件，每个文件用 `init()` 之外的方式合并——推荐：`kv_cmd_key.go`、`kv_cmd_string.go`、`kv_cmd_hash.go`、`kv_cmd_list.go`、`kv_cmd_set.go`、`kv_cmd_zset.go` 各定义一个 `var kvXxxCommands = map[string]kvCmdSpec{…}`，`kv_commands.go` 的 `kvCommandTable` 用一个小函数合并并在重复键时 panic（启动期检测）。解析辅助（`kvNeedArgs`/`parseSetArgs`/`kvScanReply`…）留在 `kv_commands.go` | 中：包级变量初始化顺序——`kvCommandTable` 若在其它包级 var 初始化中被引用需保证依赖顺序（Go 按依赖排序，合并函数满足） | 已验证（读过表头与辅助函数） | apply |
| H-12 | `api/collab_coordinator.go` 1108 行 | 前半是协作轮次状态机（CreateTurn→prepare→dispatch→advance→execute→settle），后半是 `turnStream` SSE 写出与 DTO/查询 handler。→ 拆出 `collab_stream.go`（`turnStream` 及其 `emit*`）、`collab_query_handler.go`（`ListThreadTurns`/`GetTurn`/`TurnEvents`/`buildTurnDTO`/`replay*`/DTO） | 低 | 已验证 | apply |
| H-13 | `api/agent_run_handler.go` | `createDirectRun` 129 行（嵌套 3）、`runStream` 131 行。`runStream` 中「事件 → 落 approval → 写 SSE → 计数 → 终态」与 `plan_handler.go:605` 的事件回调、`collab_coordinator.executeAgentTask` 的回调是**同一事件管线的三份写法**。→ 抽 `h.onRunEvent(runCtx, projectID, run, agent) func(cloudagent.Event)`，统一「recordApprovalEvent + 补 RunID + events.append」三步；SSE 写出仍由各自调用方追加 | 中：三处是否都需要补 RunID 需逐个核对 | 推理（plan_handler 版已读；另两处只读了签名） | apply（先读全另两处，确认一致后合并；不一致则只合并一致部分） |
| H-14 | `api/plan_handler.go:461 runPlan`（68 行，**嵌套 7 层**，全包最深） | `for → if → for(proposal) → if/else → for(DependsOn) → for(stages) → if`。→ 抽两个纯函数：`applyStageProposal(next *AgentPlanStage, stages, proposal) ` 返回更新后的阶段（Goal/SkipReason/Status/Detail），以及 `pauseAt(stage)` 的小闭包；外层循环保持「每轮重读 plan 与 stages」不变。注意 `continue` 语义：当前 skipped/blocked 分支在 `UpdateAgentPlanStage` 成功且非 blocked 时 `continue` 回到循环顶重读——提取后必须保持 | 中：控制流 | 已验证 | apply（先补表驱动测试覆盖 skip / blocked / 依赖未完成 / proposal 解析失败四种） |
| H-15 | `api/plan_handler.go:451` 包级 `var planMu sync.Map` | 每个 planID 存一个 `*sync.Mutex`，**永不删除**（与 `routeCancels` 不同，后者 `LoadAndDelete`）。长期运行进程中随计划数线性增长（Bug-2） | 中：改并发原语 | 已验证 | **apply（激进）**：改用 S-3 新建的 `internal/utils/keyedlock`（ctx 可取消、解锁即删键），`planLock` 包级变量改为 `cloudAgentHandler.planLocks` 字段；补并发测试（同 planID 串行、不同 planID 并行、结束后 map 为空） |
| H-16 | `api/data_handler.go` 904 行 | `ListCollections` 87 行嵌套 4；`BatchDocuments` 78 行嵌套 4。尾部 `rowDocument`…`isMissingRelation` 约 150 行纯函数。→ 纯函数移 `collection_doc.go`；`ListCollections` 的「合并物理表与元数据」循环抽 `mergeCollectionInfos` | 低 | 已验证（签名层面）/ 推理（函数体） | apply |
| H-17 | `api/sandbox_handler.go:447 List`（114 行） | E2B 列表的过滤 / 分页 / 状态映射混在一起 → 抽 `parseSandboxListQuery(c)` 与 `toE2BSandboxList(rows)` | 低 | 推理 | apply（读全后） |
| H-18 | `api/storage_settings_handler.go:189 PutStorage`（117 行） | 校验 → 读 YAML 节点树 → 改写 → 原子写回。→ 抽 `validateStorageBody` 与 `rewriteStorageYAML(root *yaml.Node, body)` | 低 | 推理 | apply（读全后） |

### 1.4 `systemdb` 包：仓储样板（P1）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| S-1 | 174 处 `if s == nil \|\| s.db == nil { return …, ErrUnavailable }` | 每个公开方法第一行都是它。→ 增加 `func (s *Store) conn() (*sql.DB, error)`，把「nil Store 可调用」契约集中到一处 | 低 | 已验证 | **apply（激进）**：S-2/S-4/S-5 的泛型辅助内部调用 `conn()`，被这些辅助吸收的方法自然去掉守卫；其余方法在 S-7 拆文件时逐文件替换，不单开批次 |
| S-2 | Get-by-key 样板（结构重复检测命中 **7+3+3 组**）：`GetAgentApproval`、`GetAgentTagRow`、`GetCloudAgent`、`GetCronJob`、`GetCronJobByName`、`GetMCPServer`、`GetMCPServerByName`、`GetAgentTurn`、`GetAgentTask`、`GetAgentGroupCron`、`GetSandbox*`（3） | 形如「QueryRow → scanX → `if errors.Is(err, sql.ErrNoRows) { return X{}, sql.ErrNoRows }` → return」。其中 `ErrNoRows` 那段**是空操作**（scan 出错时 scanX 已返回零值 + err；14 处）。→ 泛型 `func queryOne[T any](ctx, db, scan func(rowScanner) (T, error), q string, args ...any) (T, error)`，每个 Get 缩成 3–4 行 | 低：需确认每个 `scanX` 出错时返回零值（已抽查 `scanCronJob:432` 是 `return CronJob{}, err`） | 已验证（抽查 2 个） | apply（逐个确认 scanX 零值返回后替换） |
| S-3 | `systemdb/agent_tags.go:579 lockAgentTag` 与 `systemdb/agents.go:885 lockAgentSeed` | 30 行**逐字同构**（字段名不同）；`store.go:73` 注释自己写着「与 seedMu/seedFlights 同构」。→ 新建 `internal/utils/keyedlock`（只 import 标准库，符合 `internal/utils/README.md` 入库标准）：`type Map struct{ mu sync.Mutex; m map[string]chan struct{} }` + `func (k *Map) Lock(ctx, key string) (unlock func(), err error)`（解锁时删键并 close 通知等待者）。`Store.seedMu/seedFlights/tagMu/tagFlights` 四个字段换成 `seedLocks, tagLocks keyedlock.Map`；`lockAgentTag`/`lockAgentSeed` 删除，调用点直接 `s.tagLocks.Lock(ctx, projectID)`；`api` 的 `planMu` 复用（H-15） | 低：等价搬移 | 已验证 | apply |
| S-4 | 软删除样板：`ArchiveAgentThread`、`ArchiveCronJob`、`ArchiveGoFunc`（18 行×3），`ArchiveMCPServer`、`SoftDeleteSandbox`（17 行×2），`ArchiveAgentGroupCron*`（12 行×2） | 「UPDATE … SET archived_at=?, updated_at=? WHERE … AND archived_at IS NULL → RowsAffected==0 → ErrNoRows → notifyWrite」。→ `func (s *Store) execAffectOne(ctx, q string, args ...any) error`（0 行返回 `sql.ErrNoRows`，成功 `notifyWrite`）。**注意** `n, _ := res.RowsAffected()` 吞错误的写法需原样保留（DuckDB 驱动不一定实现），不要「顺手」改成报错 | 低 | 已验证（读过 `ArchiveGoFunc`） | apply |
| S-5 | `systemdb/async_flush.go:154/170`、`systemdb/traces.go:193` 三个 `flush*Batch` | 同构：insert 失败把 batch 退回缓冲头部。→ 泛型 `flushBatch[T](s, ctx, batch []T, insert func(ctx, *sql.DB, []T) error, requeue func([]T))` | 中：三个缓冲区受同一把 `s.mu` 保护，requeue 闭包需在锁内 | 已验证 | apply |
| S-6 | `systemdb/observe.go:34/52` `observeRead` / `observeReadPart` | 仅多一个 `part` 字段与日志 message 不同。→ 保留两个入口，内部共用 `observeFields(…)` | 低 | 已验证 | P3，可选 |
| S-7 | `systemdb/agents.go` 935 行、`systemdb/agent_collab.go` 874 行、`systemdb/agent_tags.go` 608 行 | 按实体拆：`agents.go` → `agents.go`（CloudAgent）+ `agent_threads.go` + `agent_messages.go` + `agent_runs.go` + `agent_seed.go`；`agent_collab.go` → `agent_turns.go` + `agent_tasks.go` + `agent_turn_events.go` | 低：纯移动 | 推理（按符号列表） | apply |
| S-8 | `systemdb/sandboxes.go:248/260/272` 三个 `List*Sandboxes` 与 `app/sandbox_store.go:129/140/151` 三个适配器 | 适配器三份「查 → 循环 toSandbox」同构 → `mapSandboxes(rows []systemdb.SandboxRow, err error) ([]sandbox.Sandbox, error)` 一行包装 | 低 | 已验证 | apply |
| S-9 | `systemdb/mcp_servers.go:200/210` `ParseMCPToolPolicies` / `ParseMCPHeaders` | 同构 JSON 解析。→ 私有 `parseJSONMap[T]` | 低 | 已验证 | P3，可选 |

### 1.5 `config` 包（P2）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| G-1 | **一个字段要在 5 处登记**：`config.go` 结构体 + `defaults()`（130 行）+ `yaml.go` 镜像结构体 `yaml*`（指针字段，25 个类型）+ `applyYAML*`（156 行）+ `env.go applyEnv`（174 行）+ `Redacted()`（124 行）。例：`MaxConcurrentGoRuns` 出现在 `config.go:601/837/956-959`、`yaml.go:305/597`、`env.go:167` | 这是本包复杂度的根源。可选方案：(a) 用 `yaml.Node` 解码到 `Config` 再记录「显式出现的路径集合」替代镜像结构体；(b) 用 struct tag + 反射生成 env 覆盖。两者都会改变**错误文本**、**未知字段处理**和**零值语义**，风险高 | 高：配置是启动契约，`load_test.go`（761 行）锁定大量边界 | 已验证（问题）/ 未核实（方案等价性） | **P6 可选（A6）**，本轮不做；v6.6 §4「平铺字段清单长但不复杂」结论维持，记录为长期债 |
| G-2 | `config/yaml.go:400 applyYAML` | `Instance.Lease` 的 5 个字段手写 `if yc.X != nil { cfg.X = *yc.X }` / `if .set {}`，同文件已有 `setBool` / `setDur` 辅助 → 改用辅助函数，与其余字段一致 | 低：等价 | 已验证（`yaml.go:404-416`） | apply |
| G-3 | `config/config.go` 1303 行 | 结构体定义 + `Effective*` 访问器 + 校验 + 脱敏混在一起。→ 拆 `config_types.go`（全部 `*Config` 类型与 `Effective*`）、`validate.go`（`Validate`/`validate*`/`joinErrors`）、`redact.go`（`Redacted`）；`config.go` 只留 `Load`/`defaults`/路径 | 低：纯移动 | 已验证（符号列表） | apply |
| G-4 | `config/env.go:398-441` `setEnvBool/Int/Int64/Float64/Duration` | 5 个同构函数（检测命中 Bool/Int）。→ 泛型 `setEnv[T](dst *T, key string, parse func(string) (T, error))` | 低 | 已验证 | P3，可选 |
| G-5 | `config/config.go:956-959` | `Validate()` 里把 `MaxConcurrentGoRuns==0` 改写为 16。`Validate` 是**值接收者**（`func (c Config) Validate()`，config.go:891），唯一调用点 `config.go:703 cfg.Validate()` 看不到这次赋值 → 这行是**无效赋值**；显式配 `max_concurrent_go_runs: 0` 时实际值仍是 0，下游 `functionrun.NewRunner(…, 0)` 的语义决定结果 | 行为变化：显式 0 将变为 16 | 已验证（接收者与调用点） | **apply（激进）**：把「0 → 16」归一化移到 `Load()` 中 `applyEnv` 之后、`Validate` 之前的 `normalize(&cfg)`；`Validate` 改为纯校验、不再赋值；同时 `rg 'c\.[A-Z].* = ' config/config.go` 检查 `Validate` 里是否还有其它同类无效赋值，一并迁移；补 `load_test` 用例「yaml 写 0 → 16」 |

### 1.6 `app` 包：装配（P2）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| A-1 | `app/app.go:206 NewWithRegistry`（236 行） | 装配 handler、cron、backup、kvSweeper、`api.Dependencies` 字面量、HTTP server。→ 抽 `a.assembleDatabaseHandler(cfg)`（:243-277，row count 缓存一段）、`a.assembleFunctionPlane(cfg, recordEvents)`（:283-295）、`a.assembleScheduler(cfg, runner, runtime)`（:315-338）、`a.buildAPIDeps(...) api.Dependencies`（:360-401）、`a.buildHTTPServers(cfg, deps)`（:410-432）。**装配顺序与失败时 `a.Close` 的时机不能变**（AGENTS.md 硬约束） | 中 | 已验证（读过全文） | apply（单独一批，`go test ./internal/app/` + 启动冒烟） |
| A-2 | `app/app.go:454 assembleDeps`（219 行，嵌套 5） | 嵌套 5 来自 CAS 探针块（:491-533）。→ 抽 `a.connectObjectStore(ctx, cfg, s3cfg, dbRemote)`；内部 `if a.metrics != nil { if res.CASSupported { Set(1) } else { Set(0) } }` 写成 `a.metrics.CASSupported.Set(boolGauge(res.CASSupported))`。再抽 `a.bootSystemDatabase(ctx, cfg, keys, sysFactory)`（:563-588）、`a.assembleCatalog(ctx, cfg, store, keys)`（:590-607）、`a.assembleRegistryAndCache(cfg, userFactory, regOpts)`（:611-650） | 中：错误包装文本（`create objectstore client: %w` 等）必须逐字保留，启动日志 step 名称也是运维可观察输出 | 已验证 | apply |
| A-3 | `app/app.go:473-482` 与 `:595-601` | `objectstore.Config` 与 `objectstore.DuckLakeStorage` 各从 `cfg.S3` 手抄 4–8 个字段。→ `objectstore` 不能 import `config`（依赖方向），在 `app` 内加 `s3ConfigFrom(cfg)`、`duckLakeStorageFrom(cfg)` 两个小函数；`backup`、`app/reset.go` 若也手抄则一并使用（需 `rg 'Bucket: *cfg.S3.Bucket'` 核对） | 低 | 已验证（app.go 两处）/ 推理（其他处） | apply |
| A-4 | `app/app.go:553-558` | `if fs, err := os.Stat(dir); err != nil \|\| !fs.IsDir() { MkdirAll }`——`MkdirAll` 本身对已存在目录幂等，Stat 预检多余；但**路径存在且是文件**时：原代码会 `MkdirAll` 报错，直接 `MkdirAll` 也报错，结果一致 | 低 | 推理 | apply |
| A-5 | `app/reset.go:114 resetWith`（98 行）、`app/doctor.go:86 RunDoctor`（113 行） | 均为顺序步骤脚本，可按步骤抽函数；但它们是 CLI 输出驱动，步骤文案是可观察输出 | 低 | 推理 | P3，可选 |

### 1.7 其它包（P2/P3）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| O-1 | `sbcli/commands.go:127 dispatch`（170 行 switch）+ `:54 readOnlyCommands` map + `:298 dispatchNested` | 同一组「resource action」字符串**登记两次**（handler 表、只读表），新增命令要改两处，漏改只读表会被 fail-closed 当写命令（安全但功能错）。→ 合并为一张表 `var commands = map[string]commandSpec{ "database list": {run: cmdDatabaseList, readOnly: true}, … }`；`dispatch`、`CommandReadOnly` 都查它。`CommandReadOnlyWithArgv` 的子动作特例保留 | 中：`CommandReadOnly` 是 `gocloudagent` 写权限判定的**唯一事实来源**（commands.go:46 注释），合并后要用测试锁定「只读集合与合并前完全相等」 | 已验证 | apply（先写快照测试：遍历旧 map 与旧 switch，生成集合，合并后断言相等） |
| O-2 | `sbcli/commands_*.go` 中 6 个「取一个 id → 调 SDK 删除 → 返回 `{"ok":true}`」（`cmdUserDelete`、`cmdAPIKeyRevoke`、`cmdCronDelete`、`cmdFunctionDelete`、`cmdSandboxDelete`、`cmdSandboxPause`）+ 5 个「取 id → Get → 返回」（`cmdUserGet`、`cmdCronGet`、`cmdCronRuns`、`cmdCronTrigger`、`cmdFunctionGet`）+ 4 个 11 行同构 | → `idAction(name string, fn func(ctx, *gosdk.Client, string) error) commandFunc` 与 `idQuery[T](name, fn func(ctx, *gosdk.Client, string) (T, error)) commandFunc`；在 O-1 的表里直接写 `run: idAction("id", (*gosdk.Client).DeleteUser)` | 低：输出 `{"ok": true}` 与错误文本不变 | 已验证 | apply（与 O-1 同批） |
| O-3 | `sbcli/dev.go` 1128 行、`sbcli/dev_workspace.go` 708 行 | `devReplayer.functions`（102 行嵌套 5）、`devReplayer.databases`（80 行嵌套 4）、`devWorkspace.checkDatabases`（58 行**嵌套 6**）、`devSQLStatements`（80 行嵌套 5）。→ 拆文件：`dev_lock.go`（devLock 读写）、`dev_replay_db.go`、`dev_replay_fn.go`、`dev_replay_cron.go`；嵌套最深的两个函数用「先收集再处理」+ guard clause 降到 ≤3 | 中：`dev plan/apply` 输出是 CLI 契约（`dev_test.go` 1031 行锁定） | 已验证（符号）/ 推理（函数体） | apply |
| O-4 | `sbcli/dev_workspace.go:202 loadDevWorkspace` | 生产不可达（deadcode），只有 `dev_test.go` 用 → 移入 `dev_test.go` 或改测试用正式入口 | 低 | 已验证 | apply |
| O-5 | `sandbox/manager.go:851 WriteFile` 与 `:997 WriteFileForThread` | 后者重复了 CleanPath / MaxFileBytes / withLock / driver.WriteFile / markActive 全流程，唯一差别是 **不做** `Source=="agent" → ErrBusy`、`expired → ErrGone` 检查，并且日志多 `thread_id`、出错变量名不同。→ 抽私有 `m.writeFile(ctx, projectID, id, p string, data []byte, check func(Sandbox) error, logFields ...zap.Field)`，`WriteFile` 传 agent+expired 检查，`WriteFileForThread` 传 nil。已核对 `ReadFile`（:815）只检查 `expired`、不检查 agent，所以 `ReadFileForThread` 直接委托是正确的，**不是 bug**。`WriteFileForThread` 不检查 `expired`：`EnsureForThread` 刚确保过沙盒可用，窗口极小，维持现状 | 低 | 已验证 | apply |
| O-6 | `sandbox/manager.go` 1151 行 | 按职责拆：`manager_lifecycle.go`（Create/Start/Stop/Pause/Resume/Delete/SetExpiry）、`manager_files.go`（Read/Write/Remove/ListDir）、`manager_thread.go`（*ForThread/EnsureForThread/ReleaseThread）、`reaper.go`（freezeIdle/ReapOnce/StartReaper） | 低：纯移动 | 已验证（符号） | apply |
| O-7 | `agentbridge/aliases.go` | 9 个类型别名 + `hashSummary` 一行转发。已统计：`api`、`app` 对 `agentbridge.{RunRequest,RunResult,Event,ChatRequest,ChatResponse,TokenStream,ToolDecision,ConfirmationPreparer}` 的引用为 **0**（外部一律直接用 `cloudagent.*`），别名只在 `agentbridge` 包内当简写。→ 删除 `aliases.go`，包内改用 `cloudagent.*`（`tracer` 小写别名同理），`hashSummary` 直接调用 `cloudagent.HashSummary` | 低（编译器兜底；导出别名理论上可能有仓外调用方，但 `internal/` 不可被外部模块 import） | 已验证 | apply |
| O-8 | `agentbridge/agentskills/skills.go` 中 `Known`、`Prefixes`、`Body`、`EmbeddedAllows`、`EmbeddedReadOnly`、`walkSkillDocs`、`parseAllow`、`parseCommentList`（8 个，deadcode 生产不可达） | 只有 `agentbridge/*_test.go`（`skills_test.go`、`module_skills_test.go`）在用。**激进处理**：生产代码不留只给测试用的导出 API。若测试断言的是「嵌入技能文档的 allow/readonly 声明与 `sbcli.CommandReadOnly` 一致」这类有价值的不变量，把这 8 个函数整体移到 `agentskills/skills_test.go`（同包测试），`agentbridge` 侧的跨包测试改为调用 `agentskills` 同包测试或迁入该包；否则连测试一起删 | 低 | 已验证 | **apply（激进）** |
| O-9 | `agentbridge/context.go:38 withRunContext` | 生产不可达，7 个测试文件在用 → 移到 `agentbridge/testutil_test.go`（该文件已存在） | 低 | 已验证 | apply |
| O-10 | `agentbridge/bench/`、`agentbridge/docs/` | **A8 已决：迁出 `internal/`**，目标路径、引用改动与验证见 **§1.10** | 无（非 Go） | 已验证 | **apply（§1.10）** |
| O-11 | `catalog/sql_repository.go:213/229` `GetDatabase` / `GetDatabaseByName` | 14 行同构，同 S-2 → 私有 `getDatabaseWhere(ctx, where string, args...)` | 低 | 已验证 | apply |
| O-12 | `api/collection_filter.go:163/178` `parseOr` / `parseAnd` | 递归下降的两层，同构仅运算符与下一层函数不同 → `parseBinary(op string, next func() (expr, error))` | 低：解析优先级不变 | 已验证 | P3，可选（当前写法对读者其实更直观，**倾向 skip**） |
| O-13 | `observability/metrics.go:94 NewMetrics`（151 行） | 平铺的指标注册清单，长但不复杂 | — | 已验证 | skip（同 v6.6 对 `applyEnv` 的判断） |
| O-14 | `api/perfbench/cases.go:128 dbCases`（291 行）、`:421 kvCases`（161 行） | 基准用例表，数据驱动 | — | 已验证 | skip |

### 1.8 测试卫生（P3）

| ID | 位置 | 方案 |
| --- | --- | --- |
| Q-1 | `api/plan_handler_test.go:80 errorCode` | 唯一一条 staticcheck U1000 + `deadcode -test` 真死代码 → 删除（或改用它替换同文件里手写的 `json.Unmarshal` 取 code） |
| Q-2 | `agentbridge/tools_more_test.go`、`auth/branches_test.go`、`cronjob/scheduler_branches_test.go`、`api/handler_branches_test.go`、`llmgateway/client_branches_test.go` | 文件名只说明「为覆盖率补的分支」，不说明测什么 → 读后按被测符号改名或并入对应 `xxx_test.go`（`git mv`，内容不变） |
| Q-3 | `cmd/simplebase/main.go` | `gofmt -w`（缺末尾换行） |
| Q-4 | 拆文件类条目（H-9/H-11/H-12/S-7/G-3/O-3/O-6） | 测试文件**不跟着拆**，除非原测试文件已 >1000 行；避免一次性大面积 rename |

### 1.9 操作人标识统一（A4 已决，激进，P1）

**现状（已逐个读码核实）**：同一个「谁做的」在 `internal/` 里有 **7 种写法**，落到 6 类持久化字段：

| # | 写法 | 位置 | 产出示例（登录用户 u1 / API Key k1） | 落点 |
| --- | --- | --- | --- | --- |
| 1 | `principalIdentity(p)` | `api/agent_handler.go:933`；调用 15 处（`agent_handler` 639/949、`agent_group_handler:272`、`plan_handler` 204/563/598、`run_events` 385/395、`collab_coordinator` 168/600/619、`agent_tag_handler:376`、`agent_run_handler` 188/229/345/553、`collab_handler:360`） | `u1` / `apikey:k1` | `sys_agent_threads.created_by`、`sys_agent_runs.initiator_id`、`sys_agent_approvals.resolved_by`、`sys_agent_turns.initiator_id`、`sys_agent_tags.created_by`、`sys_agent_plans.created_by`、`RunRequest.Subject`（确认闸门主体） |
| 2 | `principalKey(p)` | `agentbridge/context.go:59`（与 #1 逐字相同）；`skilltool.go` 131/143 | 同 #1 | 确认闸门 `Prepare/Wait` 的主体比较 |
| 3 | `actorID(c)` | `api/gofunction_handler.go:756`；10 处（gofunction 289/296/452/603/642/691/695、`function_trigger_handler:89`、`sandbox_handler` 187/437） | `u1` / `k1`（**无前缀，用户与 Key 撞名不可区分**） | `sys_go_funcs.created_by`、`sys_go_func_versions.created_by`、函数调用流水 actor、`sys_function_triggers.created_by`、沙盒 `created_by`、沙盒审计 |
| 4 | `principalActor(c)` | `api/s3_site_handler.go:251`；1 处 | `user:u1` / `key:k1` | `sys_audit_events.actor`（站点发布） |
| 5 | 内联 `principal.APIKeyID` | `catalog/service.go:509`、`api/mcp_builtin_admin.go:125`、`api/backup_handler.go:146`、`api/cronjob_handler.go:797`、`api/llm_handler.go:196-198`、`api/e2b_terminal.go:363` | `""` / `k1`（**登录用户为空**——Bug-1 的根因，影响 6 处，不止 cron） | `sys_operations.principal_id`、审计、LLM 会话 `created_by`、SSH 终端 actor |
| 6 | 内联 `UserID` 回落 `APIKeyID` | `api/gofunction_handler.go:794`、`api/e2b_terminal.go:101-104` | `u1` / `k1` | 审计、终端票据 |
| 7 | 内联 `UserID` | `api/users_handler.go:147`、`collab_coordinator.go:152 humanID`、`auth/delegation.go:144-150`（委托 JWT `sub`：`u1` / `apikey:k1` / `agent`） | `u1` / — | `sys_users.created_by`、人类待办 `assignee_id`、JWT `sub` |

另有三个**系统主体**字面量：`agentbridge.CronAgentPrincipalID = "system-scheduler"`、`audit.Service.Record` 对空值回落 `"system"`、`app/app_alerts.go` 的 `Actor: "system"`、`functionrun` 的 `Actor: "cron"` / `"trigger"`，以及 `systemdb/audit.go:24` 早已写下但只有 #4 遵守的约定 `user:<id> | key:<id> | enduser:<id> | system`。

**目标（唯一规则）**：

```go
// internal/auth/principal.go
// Actor 返回主体的规范操作人标识，所有落库、比较、透传一律用它。
//   登录用户 → "user:<UserID>"；API Key → "key:<APIKeyID>"；都为空 → ""。
// UserID 优先：委托令牌同时携带 UserID 与 APIKeyID 时按用户计。
func (p Principal) Actor() string

// 系统主体常量，取代散落的字面量。
const (
	ActorSystem    = "system"
	ActorScheduler = "system:scheduler"
	ActorTrigger   = "system:trigger"
	ActorCron      = "system:cron"
)
func EndUserActor(id string) string { return "enduser:" + id }
```

| ID | 改动 | 文件 |
| --- | --- | --- |
| ID-1 | 新增 `Principal.Actor()`、`Actor*` 常量、`EndUserActor`；表驱动单测覆盖：仅 UserID、仅 APIKeyID、两者都有（取 user）、都空、委托主体 | `auth/principal.go`、`auth/principal_test.go` |
| ID-2 | 删除 `principalIdentity`、`principalKey`、`actorID`、`principalActor`；全部 26 处调用改为 `p.Actor()` / `principalOf(c).Actor()` | `api/agent_handler.go`、`api/agent_group_handler.go`、`api/plan_handler.go`、`api/run_events.go`、`api/collab_coordinator.go`、`api/agent_tag_handler.go`、`api/agent_run_handler.go`、`api/collab_handler.go`、`api/gofunction_handler.go`、`api/function_trigger_handler.go`、`api/sandbox_handler.go`、`api/s3_site_handler.go`、`agentbridge/context.go`、`agentbridge/skilltool.go` |
| ID-3 | 内联写法 #5/#6 全部改 `Actor()`：`catalog.Service.DeleteDatabase` 的 `PrincipalID`、`mcp_builtin_admin`、`backup_handler`、`cronjob_handler`、`llm_handler` 的 `createdBy`、`e2b_terminal` 的票据 owner 与 SSH `Identity.Actor` | `catalog/service.go`、`api/mcp_builtin_admin.go`、`api/backup_handler.go`、`api/cronjob_handler.go`、`api/llm_handler.go`、`api/e2b_terminal.go`、`api/gofunction_handler.go` |
| ID-4 | 写法 #7：`users_handler` 的 `CreatedBy` 改 `p.Actor()`；**人类待办 `humanID` / `assignee_id` 也改为 `Actor()`**（`collab_coordinator.go:152` 仍只允许登录用户担任人类，判定改为 `principal.UserID != ""` 后取 `principal.Actor()`），`collab_handler.go:361` 的 `task.AssigneeID == caller` 两侧同格式自然成立 | `api/users_handler.go`、`api/collab_coordinator.go`、`api/collab_handler.go` |
| ID-5 | 委托 JWT：`auth/delegation.go:144-150` 的 `sub` 改为 `p.Actor()`，无主体时用 `auth.ActorSystem`（替代 `"agent"`）。`claims.UserID` / `APIKeyID` 两个结构化字段不变，鉴权仍靠它们恢复 `Principal`，`sub` 只作标识 | `auth/delegation.go` |
| ID-6 | 系统主体字面量换常量：`agentbridge.CronAgentPrincipalID` 删除改用 `auth.ActorScheduler`（`cron_agent.go` 48/204/248、`collab_cron_runner.go:169`）；`audit.Service.Record` 空值回落 `auth.ActorSystem`；`app_alerts.go` 3 处；`trigger/dispatcher.go:180` → `auth.ActorTrigger`；`app/cron_runner.go:50` → `auth.ActorCron`；`enduser_handler.go` 96/130 与 `auth_handler.go:258` 的 `Detail: "enduser:"/"user:"+id` 改用 `EndUserActor` / `Principal{UserID:…}.Actor()`。依赖方向检查：`trigger`、`audit` 新增 import `auth`（`auth` 只依赖 `utils/strutil`，无环） | `agentbridge/cron_agent.go`、`api/collab_cron_runner.go`、`audit/service.go`、`app/app_alerts.go`、`trigger/dispatcher.go`、`app/cron_runner.go`、`api/enduser_handler.go`、`api/auth_handler.go` |
| ID-7 | 权限比较点同步：`canCancelConfirm`（`agent_handler.go:945`）、确认闸门 `confirms.Resolve(…, Actor)` 与 `Prepare/Wait`（`run_events.go:385`、`skilltool.go`、`gocloudagent` 侧只透传 `RunRequest.Subject` 不解析，无需改）、`collab_handler.go:360` 待办归属——三处两侧均来自 `Actor()`，不做旧格式兜底 | 同 ID-2 |
| ID-8 | 测试：`rg -l '"apikey:\|"system-scheduler"\|"key:\|"user:' internal --glob '*_test.go'` 当前 4 处断言 + 15 个测试文件涉及 `InitiatorID/CreatedBy/PrincipalID/AssigneeID` 字面量，全部改成新格式；`agentbridge/skilltool_test.go:123-129` 的 `principalKey` 用例迁到 ID-1 的 `Actor()` 单测。前端只**展示**这些字段（`ApprovalsPanel.vue:79/97`、`AgentHumanTaskCard.vue:9`、用户列表 `createdBy`），不做比较，无需改代码；展示上会出现 `user:` / `key:` 前缀，属预期 | 15 个 `*_test.go` |

**不兼容的后果（已接受，写进发布说明）**：
- 升级前的历史行保持旧格式（`u1` / `apikey:k1` / `k1` / 空串 / `system-scheduler`），**不迁移**；升级后新行全部是新格式。按发起人筛选 / 「只能取消自己发起的 run」对历史 run 失效（`initiator_id` 旧值 ≠ 新 `Actor()`），只能由 `ProjectAdmin` 取消——已在审批范围内。
- 升级瞬间仍在等待确认的 run：`Prepare` 记录的是旧格式主体，升级后进程重启，内存态确认本就丢失（confirmHub 不持久化），无额外影响。
- 委托 JWT `sub` 格式变化：令牌 TTL ≤30 分钟且进程重启后委托令牌随 run 失效，无需兼容窗口。
- 若需要清库：`./build.sh reset -y`（符合 AGENTS.md「改表结构直接 reset」的同类做法），不写迁移脚本。

**验收**：
```bash
rg -n 'principalIdentity|principalKey|actorID\(|principalActor|CronAgentPrincipalID|"system-scheduler"|"apikey:' internal --glob '*.go'   # 期望 0
rg -n 'PrincipalID: *principal\.APIKeyID|createdBy = p\.APIKeyID|Actor: *p\.APIKeyID' internal --glob '*.go'                  # 期望 0
go test ./internal/auth/ ./internal/api/ ./internal/agentbridge/ ./internal/catalog/ ./internal/audit/ ./internal/trigger/ -count=1
```

### 1.10 `agentbridge/bench` 与 `agentbridge/docs` 迁出 `internal/`（A8 已决，P0）

| ID | 源 | 目标 | 说明 |
| --- | --- | --- | --- |
| MV-1 | `internal/agentbridge/bench/`（README、harbor_agent.py、run_{core,mbpp,smoke,swebench,terminal_bench}.py、requirements*.txt、`tasks/`，git 跟踪 18 个文件，532KB） | **`bench/agent/`**（仓库根新建 `bench/`，与 `godatabase/bench`、`gofunction/bench_report.md` 的「模块自带基准」区分：这里是跨模块的端到端评测） | `git mv`，保留历史 |
| MV-2 | `internal/agentbridge/docs/eino-guide.md`、`pi-guide.md`（第三方框架研究笔记，与宿主适配无关） | **`gocloudagent/docs/`** | 两份文档讨论的是 agent 运行循环（Eino `ChatModelAgent`、Pi loop、确认中断），对应代码在 `gocloudagent/agentcore` 与 `gocloudagent/cloudagent`，放到被讨论的模块旁边；**不放 `docs/`**：`ui/src/docs/catalog.ts:34` 用 `import.meta.glob('../../../docs/**/*.md')` 收录全部 `docs/`，研究笔记会被发布进站内文档 |

需要同步修改的引用（已全仓 `rg` 核对）：

| 文件 | 行 | 改动 |
| --- | --- | --- |
| `bench/agent/README.md` | 3、23–29、37 | 「放在 `internal/agentbridge/bench/`」及 9 条命令路径 → `bench/agent/`；第 3 行的放置理由改为「跨模块端到端评测，构建 SimpleBase 服务并验证宿主集成」 |
| `bench/agent/run_smoke.py` | 24、36 | docstring「脚本现在位于 internal/agentbridge/bench/」→ `bench/agent/`；`_repo_root()` 兜底 `here.parents[3]` → `here.parents[2]`（按标志文件查找的主逻辑不变） |
| `bench/agent/run_smoke.py` | 41 `REPORT = ROOT / "plan" / "planv6.1" / …` | **悬空路径**：`plan/planv6.1` 已在 planv6.7 删除，`run_core.py:279`、`run_mbpp.py:309` 写报告会 `FileNotFoundError`。→ 改为 `bench/agent/reports/gocloudagent-smoke-report.md`（不存在时创建，并把该目录加入 `bench/agent/.gitignore` 以外的跟踪清单由用户决定）。登记 Bug-5 |
| `bench/agent/run_swebench.py` | 485 | 报告文本中的清单路径 → `bench/agent/tasks/core/swebench_verified_50.txt` |
| `gocloudagent/docs/eino-guide.md:49`、`pi-guide.md:11` | — | 相对链接 `[internal/AGENTS.md](../../AGENTS.md)` 在原位置指向 `internal/AGENTS.md`，迁移后会指到仓库根 `AGENTS.md`（错）→ 改为 `../../internal/AGENTS.md`；其余 `eino-guide.md` 同目录链接不变 |
| `gocloudagent/README.md` | — | 增加一行「设计笔记见 `docs/`」 |
| `internal/AGENTS.md` | 33 行 `agentbridge` 职责 | 不涉及 bench/docs，无需改；`internal/README.md`（C-6 重写）不再提 bench |
| `plan/planv6.6/gocloudagent-directory-reorg-plan.md:145`、`plan/planv6.7/legacy-plan-consolidation-plan.md` 128/433/435/439/573/597 | — | 历史计划**不改正文**，在 planv6.7 §2.4「外部文件引用」表追加一行：`internal/agentbridge/{bench,docs}` → `bench/agent/`、`gocloudagent/docs/`（planv6.8 MV-1/MV-2） |
| `.dockerignore` | — | 追加 `/bench/`（Go 构建不需要，减小上下文） |

验收：
```bash
rg -n 'internal/agentbridge/(bench|docs)' --glob '!plan/**' .     # 期望 0
test ! -e internal/agentbridge/bench && test ! -e internal/agentbridge/docs
python3 bench/agent/run_smoke.py --self-check && python3 bench/agent/run_core.py --self-check
go build ./internal/... && go test ./internal/agentbridge/...       # 确认无 embed / testdata 依赖这两个目录
```

## 2. Bugs found

激进策略下，已确认的 bug **在对应批次直接修**并补回归测试；仍与重构 diff 分开提交，便于回滚。

| ID | 位置 | 级别 | 说明 / 复现 | 修复归属 |
| --- | --- | --- | --- | --- |
| Bug-1 | `api/cronjob_handler.go:797`、`api/backup_handler.go:146`、`api/mcp_builtin_admin.go:125`、`api/llm_handler.go:196`、`catalog/service.go:509`、`api/e2b_terminal.go:363` | 已确认（读码） | 只取 `principal.APIKeyID`，登录用户操作时审计 / `created_by` / SSH actor 为空串（`audit.Record` 再回落成 `"system"`，**把用户操作记成系统操作**）。复现：登录态 `POST /cron-jobs` 后查 `GET /audit` | §1.9 ID-3 |
| Bug-2 | `api/plan_handler.go:451 planMu` | 已确认（读码） | 包级 `sync.Map` 只 `LoadOrStore` 不删除，进程内单调增长 | H-15 |
| Bug-3 | `config/config.go:956-959` | 已确认（读码） | 值接收者 `Validate()` 内赋值无效；显式写 0 时「回落 16」不会发生 | G-5 |
| Bug-4 | `api/router.go:659-666` | 已确认（读码） | `function-triggers` 未经 `policyGate`，API Key policy 对这 6 条路由不生效 | R-2 |
| Bug-5 | `internal/agentbridge/bench/run_smoke.py:41` | 已确认（`ls plan/planv6.1` 不存在） | `REPORT` 指向已删除的 `plan/planv6.1/…`，`run_core.py:279` / `run_mbpp.py:309` 写报告时 `FileNotFoundError` | §1.10 MV-1 |
| Bug-6 | `api/gofunction_handler.go:756 actorID` | 已确认（读码） | 无前缀，`UserID` 与 `APIKeyID` 同值时 `created_by` 无法区分来源 | §1.9 ID-2 |

## 3. 决策记录（2026-11-23 已审批）

| ID | 决策 | 结论 | 落点 |
| --- | --- | --- | --- |
| A1 | 生成式文件头是否删除 | **不删除**；改为随结构调整同步维护，新文件按同格式写 | C-1、§6.1-6/7 |
| A2 | 计划编号注释保留规则 | 按激进策略默认：只在本计划动到的文件里清理「重构历史」类编号 | C-2 |
| A3 | `function-triggers` 补 `policyGate` | **补**（激进默认） | R-2 / Bug-4 |
| A4 | 操作人标识统一 | **统一为 `Principal.Actor()`，`user:` / `key:` / `system:*` / `enduser:`，不兼容历史数据** | §1.9 |
| A5 | `planMu` 改可回收锁 | **改**（激进默认） | H-15 / S-3 |
| A6 | 配置五处登记的结构改造 | **本轮仍不做**，列为 P6 可选：风险集中在启动契约，收益是维护便利而非正确性 | G-1 |
| A7 | `agentskills` 8 个测试专用导出 | **移入测试或删除**（激进默认） | O-8 |
| A8 | `agentbridge/bench`、`agentbridge/docs` 迁出 | **迁出**：`bench/agent/`、`gocloudagent/docs/` | §1.10 |
| A9 | Bug-1 ~ Bug-6 | **直接修**，单独提交 | §2 |

激进策略的统一口径（对所有条目生效）：
- 内部符号改名 / 删除**不留兼容包装**（如 `principalIdentity` 不保留为 `Actor()` 的别名、`aliases.go` 直接删）。
- 生产代码不保留只给测试用的符号（O-4、O-8、O-9）；v6.6 §4 中出于「改导出面成本」保留的 `systemdb.NewStoreForTest` 等仍保留（20 个跨包测试文件在用，移动只是换位置不减复杂度）。
- 已确认的不一致直接统一（审计 ctx、主体格式、只读拒写字段类型），不再逐项征求意见。

## 4. 目标结构（重构后）

只列本轮会动结构的文件；所有新建 / 拆分文件按 A1 写完整格式的文件头。

> v3：下面是计划时的目标；实际落点不同的条目用【实际：…】就地标出，详见 §8.2。

```text
bench/agent/                 ← internal/agentbridge/bench（MV-1）
gocloudagent/docs/           ← internal/agentbridge/docs（MV-2）
internal/auth/
  principal.go              + Principal.Actor() / Actor* 常量 / EndUserActor（ID-1）
internal/utils/keyedlock/
  keyedlock.go              按键串行锁，解锁即删键（S-3、H-15）
internal/api/
  router.go                 NewRouter + useGlobalMiddleware + mountPublicAuthRoutes + routeGuards（≤150 行）
                            【实际 623 行：全局中间件与 errorHandler 仍在文件内，见 §9.1 R-7】
  routes_v1.go              mountV1Routes 拆出的 mountAuth/Project/Database/KV/LLM/Usage/S3/System
  routes_guards.go          routeGuards + newRouteGuards【实际新增】
  routes_agent.go           newCloudAgentHandler + mountAgentRoutes
  routes_function.go        mountGoFunctionAndCronRoutes + mountGoRoutes（function-triggers 走 g.require）
  routes_e2b.go             mountE2BRoutes
  context.go                + requireProject / projectOf / principalOf / requestID
  error.go                  + badRequest
  audit_helper.go           recordAudit（H-2，主体统一取 Actor()）
  agent_handler.go          cloudAgentHandler 结构体与访问器（+ planLocks）
  agent_crud_handler.go     agent CRUD（H-9）
  agent_thread_handler.go   thread / message
  agent_approval_handler.go approval / cancel
  agent_dto.go              全部 DTO 与 to*DTO
  collab_coordinator.go     轮次状态机（≈650 行）【实际 798 行】
  collab_stream.go          turnStream（H-12）
  collab_query_handler.go   轮次查询 handler + DTO
  kv_commands.go            解析辅助 + kvCommandTable 合并（≈300 行）
  kv_cmd_{key,string,hash,list,set,zset}.go
  collection_doc.go         data_handler 尾部纯函数（H-16）
internal/systemdb/
  query.go                  conn() / queryOne[T] / execAffectOne / flushBatch[T]（S-1~S-5）
  agents.go → agents.go + agent_threads.go + agent_messages.go + agent_runs.go + agent_seed.go
                            【实际无 agent_messages.go：消息读写并入 agent_threads.go】
  agent_collab.go → agent_turns.go + agent_tasks.go + agent_turn_events.go
                            【实际：轮次留在 agent_collab.go（484 行），只拆出 agent_tasks.go + agent_events.go】
internal/config/
  config.go (Load/defaults/normalize/路径) + config_types.go + validate.go + redact.go
                            【实际 config_agent_types.go：只拆出 LLM / Agent / MCP / 沙盒类型；config.go 593 行】
internal/app/
  app.go                    NewWithRegistry / assembleDeps 只剩步骤调用（各 ≤80 行）
  assemble_objectstore.go   connectObjectStore + s3ConfigFrom + duckLakeStorageFrom
  assemble_system.go        bootSystemDatabase + assembleCatalog + assembleRegistryAndCache
                            【实际合并为 assemble_storage.go + s3_config.go】
  assemble_api.go           assembleDatabaseHandler + assembleFunctionPlane + assembleScheduler + buildAPIDeps + buildHTTPServers
internal/sbcli/
  commands.go               单一命令表 commands（O-1）+ idAction/idQuery（O-2）
  dev.go → dev.go + dev_lock.go + dev_replay_{db,fn,cron}.go
                            【实际 dev_lock.go + dev_replay.go + dev_replay_collections.go + dev_replay_runtime.go；dev_workspace.go 未拆】
internal/sandbox/
  manager.go → manager.go + manager_lifecycle.go + manager_files.go + manager_thread.go + reaper.go
                            【实际 manager_exec.go（Exec + 文件读写）+ manager_reaper.go】
internal/agentbridge/
  aliases.go                删除（O-7）
  bench/ docs/              删除（迁出，§1.10）
```

## 5. Not changed（看起来能简化，但不动）

- **v6.6 §4 全部延续**：`KeyBuilder` 转发方法、内存版 FileStore/BlobStore（`NewMemoryBlobStore`/`NewMemoryFileStore` 等 deadcode 项）、`fieldString`/`fieldDuration`、`e2bproto.DecodeFrame`、`runEventHub.sweep`、`confirmHub.pendingCount`、`sbcli.DispatchKnown`、`agentbridge.InProcessCLI`、`systemdb.NewStoreForTest`、6 处 `ctx == nil` 兜底、`sbcli` 的 `io/ioutil` 白名单字符串、`sort.*` 不换 `slices.*`。
- **系统库保护判断**（`if catalog.IsSystemDatabase(db)`）与 handler 内 `writable` 检查：保持显式、可 grep，不抽中间件（AGENTS.md 硬约束 1 的落点）。
- **文件头注释**（A1）：不删除、不缩短，只同步更新。
- `catalog/sql_repository.go isUniqueViolation` 与 `systemdb/seed.go isDuplicate`：匹配串不同（前者不认 `already exists`、后者不认 `constraint failed: UNIQUE` 精确串），合并会改变「哪些错误被当作重复」，不合并。
- `oai_dto.go` 与 `oai_responses.go` 的两套 DTO：Chat Completions 与 Responses 是两个协议（AGENTS.md「Responses 是 chat 之上的协议转换」），共享结构会把协议差异藏进条件分支。
- `config/env.go applyEnv`、`config/yaml.go rejectYAMLSecrets`、`observability.NewMetrics`、`perfbench` 用例表：平铺清单，长但线性，拆了反而难对照。
- `api/collection_filter.go parseOr/parseAnd`：递归下降的分层写法比参数化 `parseBinary` 更直观（O-12 倾向 skip）。
- `systemdb` 174 处 nil 守卫：按 S-1 集中到 `conn()`，但不保留「每个方法自己判 nil」的旧写法。
- `n, _ := res.RowsAffected()` 吞错误：DuckDB 驱动兼容性考虑，原样保留。
- 双段 `/v1/.../v1/chat/completions` 路径及其注释：搬迁时**逐字保留**（AGENTS.md 明确禁止"顺手修正"）。

## 6. 实施批次

所有批次的通用前提：**等 `internal/` 下 209 个未提交改动落地（提交或用户确认）之后再开始**，否则搬文件会与在途改动冲突。每批「一次只做一种变换」，改完先跑该包测试，再对照 `skills/simplify-code/references/behavior-traps.md` 想一遍「什么输入会让新旧不同」。

| 批次 | 内容 | 风险 | 单独提交 | 预估净变化 |
| --- | --- | --- | --- | --- |
| **P0 卫生 + 迁移** | Q-1、Q-3、C-3、C-4、C-6（README 重写）、O-4、O-8、O-9、R-3、**MV-1/MV-2（§1.10，含 Bug-5）** | 极低 | 是 | −50 行 Go；`internal/` 少 2 个非 Go 目录 |
| **P1 操作人统一** | **§1.9 ID-1 ~ ID-8**、H-2（审计入口）、Bug-1、Bug-6 | 高（持久化格式，已批准） | 是，且**先于其它重构**：其余批次都会动到这些调用点，先统一可避免冲突与重复改 | −80 行；7 种写法 → 1 |
| **P2 重复知识** | H-3、H-4、H-5、H-6、H-7、H-8、H-10、S-1、S-2、S-3（含 `utils/keyedlock`）、S-4、S-5、S-8、O-5、O-7、O-11、G-2、A-3、A-4 | 低–中 | 是 | −700～−900 行 |
| **P3 拆大函数 + 修行为** | R-1、R-2（Bug-4）、R-4、R-5、R-6、H-13、H-14（先补测试）、H-15（Bug-2）、A-1、A-2、G-5（Bug-3）、O-1+O-2（先写快照测试）、O-3 | 中 | 是；行为修复（R-2/H-15/G-5）与纯结构分两个提交 | 最长函数 246 → ≤100，最深嵌套 7 → ≤4 |
| **P4 拆大文件** | H-9、H-11、H-12、H-16、H-17、H-18、S-7、G-3、O-6 | 低（纯移动 + 文件头同步） | 是 | ≈0（单文件 ≤600 行） |
| **P5 注释同步** | C-1（只同步，不删）、C-2 | 无 | 是 | 头注释漂移为 0 |
| P6（可选） | G-1 配置结构改造、S-6、S-9、G-4、A-5、O-12 | 中–高 | — | 视情况 |

顺序 P0 → P1 → P2 → P3 → P4 → P5。P1 前置的原因：§1.9 涉及 26 处调用点 + 6 处内联写法，分布在 P2–P4 要拆的文件里（`agent_handler.go`、`collab_coordinator.go`、`gofunction_handler.go`、`router.go` 等），先改语义、再搬结构，diff 才可审。

### 6.1 动手前的固定检查

1. 移动 / 删除导出符号前：`rg '\bSymbol\b' internal cmd gocloudagent --glob '*.go'` 确认调用方，含跨包测试与嵌套模块。
2. 拆路由（R-1/R-4/R-5）前，先加一个临时测试：把 `NewRouter(deps).Routes()` 按注册顺序序列化为 `method path name` 列表写入 golden；拆分后 diff 必须为空（R-2 只改中间件不改路由，不影响该列表），验收后删除该临时测试。
3. 合并 CLI 命令表（O-1）前，先加快照测试：旧 `readOnlyCommands` 键集合 + 旧 `dispatch` 能返回非 nil 的键集合；合并后两集合分别相等。
4. H-14 `runPlan` 动手前补表驱动测试：proposal 解析失败 → paused；阶段 skip → 下一阶段；依赖未完成 → blocked + paused；全部完成 → completed。
5. 泛型辅助（S-2/S-5）引入前确认 `go.mod` 的 `go 1.25.0` 满足（满足）。
6. 新建 / 拆分出的文件按 A1 写完整文件头（文件说明 + 用法 + 调用架构 + 主要符号），格式照抄同包现有文件。
7. 每批结束跑文件头漂移检查，结果必须为空：
   ```bash
   cd internal && for f in $(rg -l --glob '*.go' '\[同包\]|\[关键引用\]|同包调用'); do d=$(dirname $f); rg -o --no-filename '[a-z0-9_]+\.go' $f | sort -u | while read g; do [ -f "$d/$g" ] || echo "$f -> $g"; done; done
   rg -n 'principalIdentity|principalKey|actorID|principalActor|lockAgentTag|lockAgentSeed|aliases\.go' --glob '*.go' .   # 头注释里的已删符号
   ```

### 6.2 每批验证门禁

```bash
export PATH=$PATH:~/go/bin
go build ./internal/... ./cmd/... && go vet ./internal/... && go test ./internal/...
gofmt -l internal cmd                                     # 期望为空
staticcheck -checks 'all,-ST*' ./internal/...             # 期望 0（基线 1：errorCode）
deadcode ./cmd/... | grep '^internal/' | wc -l            # 基线 24；实测 14（逐项等于 §5 保留清单；原写 ≤13 少算了 memoryReader.ReadAt）
deadcode -test ./cmd/... ./internal/... | grep '^internal/' | wc -l   # 期望 0（基线 1）
# 涉及路由 / 鉴权 / CLI / 装配 / 操作人时追加：
go test ./internal/api/ -run 'Route|WriteGate|OpenAI|Policy|Cancel|Confirm|Human|Audit' -count=1
go test ./internal/sbcli/ ./internal/app/ ./internal/auth/ ./internal/agentbridge/ -count=1
(cd gocloudagent && go test ./...)                        # O-1 与 ID-7 会经 CommandReadOnly / RunRequest.Subject 影响嵌套模块
./build.sh smoke   # 若脚本存在该目标；否则启动一次 simplebase 并 curl /health/ready
```

### 6.3 预期收益（可量化指标）

| 指标 | 基线 | 目标 |
| --- | --- | --- |
| 生产代码最长函数（不含数据表） | 246 行（`mountV1Routes`） | ≤100 行 |
| 最深嵌套 | 7（`runPlan`） | ≤4 |
| >100 行的函数数 | 33（含 `perfbench` 用例表） | ≤10（剩余为平铺清单类） |
| >800 行的生产文件 | 16 | ≤4（平铺清单类，如 `observability/metrics.go`） |
| 文件头注释 | 5639 行 / 195 文件，漂移 0 | **保留**；拆分后文件数增加，漂移仍为 0 |
| 操作人标识写法 | 7 种 + 4 个系统字面量 | 1 个方法 + 4 个常量 |
| 逐字重复实现 | `principalIdentity/principalKey`、`lockAgentTag/lockAgentSeed`、`aggregateLimits`×2、Get 样板 13 处、Archive 样板 7 处、flush 3 处、sandbox 适配 3 处 | 各收敛为 1 处 |
| 命令登记处 | 2（dispatch + readOnly） | 1 |
| `internal/` 内非 Go 目录 | `agentbridge/bench`、`agentbridge/docs` | 0（`agentskills/simplebase` 为 `go:embed` 资源，保留） |
| 已确认 bug | 6 | 0 |
| staticcheck / deadcode -test | 1 / 1 | 0 / 0 |

## 7. Verification（本轮评估）

- **已运行（只读）**：§0.2 全部命令；`go test ./internal/...` 通过；AST 脚本（函数长度 / 嵌套 / 结构重复）放在 `.session_tmps/`，未进入仓库。
- **已读源码核实**：`router.go` 250–889、`app.go` 200–679、`plan_handler.go` 440–659、`kv_commands.go` 表头、`sandbox/manager.go` 的 ReadFile/WriteFile/WriteFileForThread、`systemdb` Get/Archive/flush/lock 样板、`sbcli/commands.go` 双表、`config` 的 Validate 接收者与 `applyYAML` 开头、五个 `recordAudit*`、四个主体标识函数。
- **推理未执行**：H-13 另两处事件回调是否完全同构、H-16/H-17/H-18/O-3 的函数体细节、A-3 中 `backup`/`reset.go` 是否也手抄 S3 字段、C-5 的列是否 `NOT NULL`。执行对应条目前必须先读全。
- **未运行**：`gocloudagent` / `godatabase` / `gofunction` / `gosandbox` 独立模块、前端、`scripts/examples-verify`；这些不在 `internal/` 范围，但 O-1（`CommandReadOnly`）被 `gocloudagent` 间接依赖，P2 批需追加 `cd gocloudagent && go test ./...`。
- **范围声明**：大函数与重复检测覆盖 `internal/` 全部生产 `.go`（排除 `bench/`、`perfbench/` 用于重复检测）；结构重复只检测「函数体 token 归一化后完全相同」的情况，近似重复（如 H-2 五个 `recordAudit`）靠人工发现，**不代表已穷尽**。

## 8. 执行结果与偏差（v3，2026-11-23 回写）

P0–P5 已按 §6 顺序实施，**尚未提交**（工作区的在途改动还包含与本计划无关的内容）。本节状态全部由「读源码 + 只读命令」核对得出，不凭计划勾选。图例：✅ 已完成　◐ 部分完成或落点不同　⏭ 按计划可选、未做。

### 8.1 门禁实测（darwin/arm64，Go 1.26.1）

| 检查 | 基线（§0.2） | 实测 |
| --- | --- | --- |
| `go build ./internal/... ./cmd/...` | 通过 | 通过 |
| `go vet ./internal/...` | 0 | 0 |
| `go test ./internal/... -count=1` | 全部 ok | 全部 ok（30 个包） |
| `gofmt -l internal cmd` | 1 | 0 |
| `staticcheck -checks 'all,-ST*' ./internal/...` | 1 | 0 |
| `deadcode ./cmd/...` 生产不可达 | 24 | 14，逐项等于 §5 保留清单 |
| `deadcode -test ./cmd/... ./internal/...` | 1 | 0 |
| 文件头漂移脚本（§6.1-7） | 未测 | 0（291 个带「文件说明」头的生产文件） |
| 路由集合 | — | 166 条 `method + path + handler` 与 HEAD 逐条一致 |
| `cd gocloudagent && go build / vet / test` | — | 全部通过（O-1、ID-7 的跨模块影响） |
| 最长业务函数（不含平铺清单） | 246 | 129（`createDirectRun`） |
| 控制流嵌套 ≥ 5 的函数 | 1（7 层） | 4（`assembleWriteLease` 见 §9.1 N-A1，`connectObjectStore` 见 §9.2；另两个是词法器与删库流程，不动） |
| > 800 行的生产文件 | 16 | 4，且都不是平铺清单（见 §9.1 N-H1 ~ N-H4） |
| 计划编号注释 | 148 文件 / 573 处 | 160 文件 / 539 处（文件数因拆分上升） |

说明：

- 路由集合的核对方式：`git archive HEAD` 导出到 `/tmp`（补上未入库的 `internal/web/dist` 与 `packages/`），两边各用 `go test -overlay` 挂一个临时测试导出 `NewRouter(fullRouterDeps(t)).Routes()`，按 `method path handler` 排序后 diff 为空；工作区不留文件。**`Routes()` 按 Echo 内部 map 迭代，顺序不可观测**，所以 §1.2 R-1 要求的「序列化逐行 diff」只能证明集合不变，注册顺序是否保持只能靠拆分时逐行对照。
- 本轮未做：启动冒烟、`./scripts/smoke.sh`（会跑前端构建）、真实 DuckDB / S3 集成。

### 8.2 逐项状态

| ID | 状态 | 证据 / 偏差 |
| --- | --- | --- |
| C-1 | ✅ | 头注释保留并随拆分同步；漂移脚本 0 |
| C-2 | ◐ | 只在动到的文件里处理；未逐处区分「锚点」与「重构历史」 |
| C-3 | ✅ | `plan1` 说法已清；`router.go:62` 还留一句「各字段可由后续 plan 逐步填充」（§9.1 N-D2） |
| C-4 | ✅ | `app.go:398` 合并为一段 |
| C-5 | ✅ | 核实 `schema.go`：四列可空（只有 DEFAULT），`NullString` 兜底保留，只改注释 |
| C-6 | ✅ | `internal/README.md` 按实际模块重写 |
| R-1 ~ R-6 | ✅ | `routes_v1.go` / `routes_guards.go` / `routes_agent.go` / `routes_function.go` / `routes_e2b.go`；R-2 六条全部 `g.require(...)`，`policy_gate_test.go` 有受限 Key 用例；R-3 已内联；路由集合不变 |
| H-1 ~ H-18 | ✅ | H-2 → `audit_helper.go`（含「客户端断开不丢审计」用例）；H-4 / H-5 的样板归零（剩余命中是辅助函数自身与测试）；H-6 结构体名为 `aggregateLimitConfig`；H-12 → `collab_stream.go` + `collab_query_handler.go`；H-14 → `applyStageProposal` + `runPlanStage`，`TestApplyStageProposal` 表驱动；H-15 → `planLocks keyedlock.Map` + `TestPlanLocksSerializeAndReclaim`；H-16 → `collection_doc.go`（`appendPhysicalCollections` / `appendViewCollections`）；H-18 → `validateStorageBody` + `patchYAML` / `patchStorageNode` |
| S-1 ~ S-5 | ✅ | `systemdb/query.go`：`conn` / `queryOne[T]` / `execAffectOne` / `execNotify` / `flushBatch[T]`；`utils/keyedlock` 带测试 |
| S-6、S-9 | ⏭ | 可选，未做；§9.2 给出再评估结论 |
| S-7 | ◐ | `agents.go` → `agent_threads.go`（含消息读写）/ `agent_runs.go` / `agent_seed.go`；`agent_collab.go` → `agent_tasks.go` / `agent_events.go`，轮次仍在 `agent_collab.go`（484 行） |
| S-8 | ✅ | `app/sandbox_store.go` 的 `mapSandboxes` |
| G-1、G-4 | ⏭ | A6 / 可选 |
| G-2、G-5 | ✅ | `setBool` / `setDur`；`normalize(&cfg)` + `normalize_test.go`（yaml 写 0 → 16） |
| G-3 | ◐ | `validate.go` / `redact.go` 已拆；类型只拆出 LLM / Agent / MCP / 沙盒到 `config_agent_types.go`，`config.go` 593 行（已 ≤600，不再拆） |
| A-1 ~ A-4 | ✅ | `assemble_api.go`；`connectObjectStore` 等四个在 `assemble_storage.go`；`s3_config.go`；`MkdirAll` 去掉 Stat 预检。`connectObjectStore` 嵌套仍为 5（§9.2 A-2） |
| A-5、O-12 ~ O-14 | ⏭ | 可选 / skip |
| O-1 | ✅ | 单表 `commands` + `commands_snapshot_test.go` |
| O-2 | ◐ | `idAction` / `idQuery` 覆盖 cron / function / sandbox；database / schema / object 的 4 个同构命令未并入（§9.1 N-S3） |
| O-3 | ◐ | `dev.go` 344 行 + `dev_lock.go` + `dev_replay*.go`，嵌套全部 ≤ 4；`dev_workspace.go` 未拆，708 → 745 行 |
| O-4、O-5、O-7 ~ O-11 | ✅ | `loadDevWorkspace` 移入 `dev_test.go`；`writeFile(…, check, extra…)`；`aliases.go` 删除；8 个技能函数移入 `agentskills/skills_test.go`；`withRunContext` 移入 `testutil_test.go`；`getDatabaseWhere` |
| O-6 | ✅ | `manager_lifecycle.go` / `manager_thread.go` / `manager_reaper.go` / `manager_exec.go`（Exec 与文件读写同文件） |
| Q-1 ~ Q-4 | ✅ | `errorCode` 已删；4 个测试文件按被测内容改名（`handler_branches` → `handler_validation`、`scheduler_branches` → `scheduler_tick_execute`、`client_branches` → `client_chat_usage`、`tools_more` → `tools_build_errors`；`auth/branches_test.go` 在 HEAD 之前已不存在）；`gofmt` 0 |
| ID-1 ~ ID-8 | ✅ | `Principal.Actor()` / `Actor*` 常量 / `EndUserActor` + `principal_test.go`；旧写法 0。§1.9 的验收命令 `rg 'principalKey'` 仍命中 10 行，全部是 `api/context.go:50` 的 context key 常量与 `auth/service_test.go:329` 的测试 key 类型——与已删函数同名，不是残留（§9.1 N-C1） |
| MV-1 | ◐ | 已迁到 `bench/agent/`，引用、`parents[2]`、`REPORT` 路径与建目录均已改；`run_core.py --self-check` 通过。`run_smoke.py --self-check` **在 macOS 无法运行**（需要 `unshare` 与可写的 `/home/ubuntu`），Linux 评测机待跑；`bench/agent/reports/` 尚未加 `.gitignore` |
| MV-2 | ✅ | `gocloudagent/docs/`；相对链接已改，README 加一行，`.dockerignore` 加 `/bench/` |
| Bug-1 ~ Bug-6 | ✅ | 分别随 ID-3、H-15、G-5、R-2、MV-1、ID-2 修复，各有用例 |

## 9. 第二轮评估（实施后，只评估未动代码）

方法同 §0：AST 脚本找「token 归一后结构相同」的函数、控制流嵌套深度、超长函数与文件，再 `rg` 与读码逐个确认。缩写：P = 优先级，R = 风险。**v4：§9.1 全部条目与 §9.2 的 A-2 已实施，结果见 §10；§9.2 / §9.3 的「skip / 不做」结论维持。**

本版本关闭时：§8.2 的 ◐ / ⏭ 项、§9.1 未做的候选与开口验证项并入 `planv6.7/legacy-plan-consolidation-plan.md` §2 的锚点表（同 v6.4 → v6.7 的做法）；根 `AGENTS.md`「当前保留 planv6.5 – planv6.7」一句同步改。

### 9.1 值得做的候选

| ID | P / R | 位置 | 问题 → 方案 | 证据 |
| --- | --- | --- | --- | --- |
| N-A1 | P2 / 中 | `app/app_lease.go:44 assembleWriteLease` | 108 行、嵌套 6（全仓最深）：失租回调、`WriteGate`、`TryDo` 回调三层闭包叠在一起，`ValidFor(scope)` / `Has(scope)` 在 `TryDo` 外内各写一遍。→ 抽 `a.onLeaseLost(cfg)`、`a.leaseWriteGate(ctx, cfg, lm)` 与一个「拒绝原因」小函数；两个 `if a.metrics != nil && x != nil` 合成一个 `if a.metrics != nil`。不改判断顺序、日志与错误文案 | 读全文；这是单写保护的核心路径，靠 `go test ./internal/app/` 兜底，单独提交 |
| N-S1 | P2 / 低 | `systemdb`：`ListAgentTagRows` / `ListCronJobs` / `ListMCPServers`（各 23 行），`ListExpiredSandboxes` / `ListStaleRunSandboxes` / `ListIdleRunningSandboxes`，`CountCloudAgents` / `CountMCPServers`、`CountGoFuncs` / `CountSandboxes` | 「conn → Query → rows 循环 → scan → rows.Err()」与「COUNT 标量」两套样板，是 S-2 `queryOne` 的对称面。→ `queryList[T](ctx, s, scan, q, args...)` 与 `countRows(ctx, s, q, args...)` 放 `query.go`；保持 `make([]T, 0)` 的非 nil 返回 | AST 脚本（107 / 42 / 49 / 46 token） |
| N-H1 | P2 / 低–中 | `api/agent_run_handler.go`（950 行） | 直接 run 的创建与 SSE（`createDirectRun` 129 行、`openStream` / `sseWriter` / `runStream` / `startRunStreamWorkers` / `newRunStreamOutput`）与自动路由 run（`createAutoRun` 99 行、`decideRoute` / `runAutoRoute` / `streamAutoRun` / `completeAutoRun`）同在一个文件。→ 先纯移动出 `agent_run_auto.go` 与 `run_sse.go`；再把 `createDirectRun` 的「重试装载、归属与能力校验、落库」分段 | 函数清单已核对；先移动后拆函数，两个提交 |
| N-H2 | P2 / 低 | `api/agent_run_handler.go:655 sseWriter` 与 `api/collab_stream.go:50 turnStream` | `{c, flusher, mu, seq}` + `write()` + 「`beginSSE` → 两个响应头 → `WriteHeader`」重复。→ `turnStream` 内嵌 `*sseWriter`，打开逻辑抽 `newSSEWriter(c, header)`；帧格式与响应头不变 | AST 脚本 + 读码；与 N-H1 的 `run_sse.go` 同批 |
| N-S3 | P3 / 低 | `sbcli/commands_database.go` 的 `cmdDatabaseGet` / `cmdDatabaseDelete` / `cmdSchemaShow`、`commands_object.go` 的 `cmdObjectDelete`（11 行 × 4）；`cmdSQLExec` / `cmdSQLQuery`；`dev.go` 的 `cmdDevPlan` / `cmdDevApply` | O-2 的 `idQuery[T ~[]byte]` 约束过窄（这四个 SDK 方法返回结构体）。→ 改成 `idQuery[T any]` 后直接写进命令表，输出不变；后两对各抽「公共前半段 + 一个差异参数」 | AST 脚本；`commands_snapshot_test.go` 锁定只读集合 |
| N-H3 | P3 / 低 | `api/sandbox_handler.go`（859 行） | E2B 映射（`sourceFor` … `created`，约 150 行）与列表查询（`parseSandboxListQuery` / `toE2BSandboxList` / `parseMetadataFilter` / `matchMetadata`）夹在 handler 之间。→ 纯移动 `sandbox_e2b_dto.go` + `sandbox_list.go` | 函数清单 |
| N-H4 | P3 / 低–中 | `api/gofunction_handler.go`（841 行）、`api/cronjob_handler.go`（800 行） | gofunction 的执行面（槽位、`TestVersion` / `Invoke`、`pickSource`、指标）与错误映射各成一组 → `gofunction_invoke.go`、`gofunction_errors.go`；cronjob 的目标校验与群绑定（`validateTarget*` / `lookupGroup` / `saveGroupBinding` / `build*Index`）→ `cronjob_target.go`；`validateCronJobBody`（101 行）按 schedule / target / group 三段拆 | 函数清单；先移动后拆 |
| N-C1 | P3 / 无 | `api/context.go:50` 常量 `principalKey`、`auth/service_test.go:329` 类型 `principalKey` | 与已删函数同名，使 §1.9 的验收命令命中 10 行。→ 改名 `principalCtxKey`，让验收命令真正归零 | 验收命令实测 |
| N-D1 | P3 / 低 | `e2bproto.DecodeFrame`、`runEventHub.sweep`、`confirmHub.pendingCount` | 各只被同包的 1 个测试文件使用。v2 的口径是「生产不留只给测试用的符号」（O-8 / O-9 已照做），这 3 个是同类遗留 → 移入 `_test.go`。跨包测试在用的 `NewMemoryBlobStore` / `NewMemoryFileStore` / `NewStoreForTest` / `DispatchKnown` / `InProcessCLI` **维持**（搬动只换位置，不减复杂度） | `deadcode` + `rg` 调用方 |
| N-D2 | P3 / 无 | `api/router.go:62`；`.gitignore` | 前者「各字段可由后续 plan 逐步填充」是过期说法 → 改一句；后者补 `/bench/agent/reports/`（评测报告是生成物） | 读码 |
| R-7 | P3 / 低 | `api/router.go`（623 行） | R-5 只抽了两个装配函数，中间件本体（`skipS3UploadBodyLimit` / `skipLargeUploadBodyLimit` / `bodyLimit` / `requestIDMiddleware` / `isValidRequestID` / `shouldRecordRequestLog` / `accessLogMiddleware`）与 `errorHandler` / `safeMessage` 仍在 `router.go`，而同类已有 `middleware_ratelimit.go` / `middleware_http.go`。→ 纯移动 `middleware_accesslog.go`（含 requestID 与 bodyLimit）与 `error_handler.go`，`router.go` 回到 §4 的目标形态 | 函数清单；`router_test.go` / `router_stack_test.go` 兜底 |

建议顺序：N-D2 / N-C1 / N-D1（无风险卫生）→ N-S1 / N-S3（重复知识）→ A-2 的探针抽取（§9.2，随 `assemble_storage.go` 顺手）→ N-H2 + N-H1（先移动后拆）→ N-H3 / N-H4 / R-7（纯移动）→ N-A1（单独一批）。每项动手前仍按 §6.1 做 `rg` 调用方核对与文件头同步。

### 9.2 上一轮未做的可选项：再评估结论

| ID | 结论 | 理由 |
| --- | --- | --- |
| S-6 `observeRead` / `observeReadPart` | **skip** | 两者字段不同（行数 vs 阶段名）、日志 message 不同，共用 `observeFields` 只省约 8 行，多一层间接 |
| S-9 `ParseMCPHeaders` / `ParseMCPToolPolicies` | 可做，P3 | `parseJSONMap[T]` 省约 6 行；只在动到 `mcp_servers.go` 时顺带 |
| G-4 `setEnv*` | **skip** | 只有 Bool / Int 两对结构相同，泛型 + parse 函数不如现在的平铺好读 |
| A-5 `resetWith` / `RunDoctor` | **skip** | 顺序步骤脚本，步骤文案是 CLI 输出；拆函数不改善可读性 |
| O-12 `parseOr` / `parseAnd` | **skip**（维持） | 递归下降按层写更直观 |
| G-1 配置五处登记 | **不做**（A6） | 风险与收益不对等 |
| A-2 的 `connectObjectStore`（嵌套 5） | 可做，P3 | 探针块再抽 `a.probeCreateIfAbsent(ctx, cfg, store) error`，内部用 guard clause（`if !res.CASSupported && cfg.Instance.Lease.Enabled { return err }`）；日志顺序与错误文本逐字保留 |
| 继续拆 `agent_collab.go`（484 行）、`dev_workspace.go`（745 行） | **不做** | 前者 15 个函数全是轮次状态机，已内聚；后者是 `dev plan/apply` 的单一校验流程，拆文件只换位置 |

### 9.3 评估后不做的（下次不必重评）

- `sandbox` 的 `CanLogs` / `CanWatch` / `CanPorts` / `CanMetrics` / `CanSnapshot`（7 行 × 5）：每个是一个类型断言 + `Available()`，抽 `supports(ok bool)` 只各省 2 行；五个带文档的导出方法比一个布尔参数的辅助更直白。
- `api/system_redact.go` 的 `systemSQLReferencesSensitive` 与 `godatabase/sqlguard/classifier.go` 的 `skipString` / `skipQuotedIdent`：确实有两份手写的「跳过字符串 / 注释 / 引号标识符」逻辑。合并要在 `godatabase/sqlx` 新增导出 API（跨模块公共契约），且两边的转义规则没有逐条比对 → **propose**，登记为重复知识的长期项，不在本计划做。
- `auth/keyring.go hmacHex` 与 `config/profile.go DeriveSessionSecret`（各 5 行 HMAC-SHA256）：共享要在 `utils/` 新增包，收益 5 行。
- `api/adapter.go llmServiceAdapter.Stream` 与 `api/gocloudagent_access.go llmChatClient.Stream`：两个适配器把同一调用包成两种接口形状，合并会让两个接口耦合。
- ≤ 10 行的适配样板（`sqlServiceAdapter` / `kvServiceAdapter` 转发、`permissionStrings` / `toStrings`、`marshalMentions` / `encodeMetadata`、`dbbridge` 的两个 observer、`perfbench` 的 `byteReader`）：各是所在接口的最小实现，不合并。
- §5 的全部条目继续有效。

## 10. 第二轮候选的执行结果（v4，2026-11-23，激进策略）

§9.1 / §9.2 标为「值得做 / 可做」的条目已全部实施，**尚未提交**。源码改动之外同步了文件头（漂移脚本 0）与 `.gitignore`。

| ID | 状态 | 落点 / 与计划的差异 |
| --- | --- | --- |
| N-D2 | ✅ | `router.go` 的 Dependencies 注释改为与现状一致；`.gitignore` 加 `/bench/agent/reports/` |
| N-C1 | ✅ | `api/context.go` 的 context key 改名 `principalCtxKey`，`auth/service_test.go` 的测试 key 改名 `testPrincipalKey`；§1.9 的验收命令现在真正为 0 |
| N-D1 | ✅ | `e2bproto.DecodeFrame` → `e2bproto_test.go`；`runEventHub.sweep` → `run_events_lifecycle_test.go`；`confirmHub.pendingCount` → `run_events_test.go`。deadcode 生产项 14 → 11 |
| N-S1 | ✅ | `systemdb/query.go` 新增 `queryList[T]` 与 `Store.countRows`。比计划多收了 10 个同形的 List（`ListAgentApprovals` / `ListRunningAgentTurns` / `ListAgentGroupCrons` / `ListAgentTasks` / `ListHumanAgentTasks` / `ListCloudAgents` / `ListDueCronJobs` / `ListCronJobRuns` / `QueryLLMTraces` / `QueryTraceSpans` / `ListFunctionTriggers` / `querySandboxes`），共 15 个 List + 5 个 Count。`scanLLMTrace` 的入参统一为 `rowScanner`；`sandboxes.go` 四个 wrapper 去掉冗余的 `conn()` 预检；`ListCloudAgents` 保留预检，避免库不可用时多出一条读观测日志 |
| N-S3 | ✅ | `idQuery[T ~[]byte]` 改为 **`idResult[T any]`**（改名：现在也用于 delete，返回 SDK 结果，`json.RawMessage` 走 `resultOf` 解码）；`database get / delete`、`schema show`、`object delete` 直接写进命令表；`sqlCommand(exec)`、`devCommand(name, apply)` 取代 4 个薄包装；`cmdDatabaseGet` 等 6 个函数删除，4 个文件头同步 |
| A-2 | ✅ | `connectObjectStore` 抽出 `probeCreateIfAbsent`，嵌套 5 → 2；日志与错误文案逐字保留 |
| N-H2 | ✅ | `sseWriter` / `writeSSEFrame` / 新的 `newSSEWriter(c, idHeader, idValue)` 放进已有的 `sse.go`（与 `beginSSE` 同文件，而非新建 `run_sse.go`）；`turnStream` 内嵌 `*sseWriter`，自己的 `write` 删除 |
| N-H1 | ✅ | 新增 `agent_run_auto.go`（10 个自动路由函数）；`createDirectRun` 129 → 101 行，抽出 `resolveDirectAgent` 与 `historyWithoutUserTurn`；`agent_run_handler.go` 950 → 547 行 |
| N-H3 | ✅ | `sandbox_e2b_dto.go`（DTO + 映射 + `decodeE2BBody`）与 `sandbox_list.go`（含 `List` handler，比计划多移一个函数，列表功能内聚）；`sandbox_handler.go` 859 → 462 行 |
| N-H4 | ✅ | `gofunction_invoke.go`（槽位 / `TestVersion` / `Invoke` / `pickSource` / 指标）与 `gofunction_errors.go`；`cronjob_target.go`；`validateCronJobBody` 改为 `(req *upsertCronJobBody, rid, create) error`，拆成 `validateCronSchedule` / `validateCronTarget` / `validateGroupScheduleOptions`，两个调用点同步 |
| R-7 | ✅（偏差） | 中间件按职责分两个文件：`middleware_accesslog.go`（request ID + 访问日志）与 `middleware_bodylimit.go`（计划写的是合成一个文件）；`errorHandler` / `safeMessage` 并入已有的 `error.go`（计划写的是新建 `error_handler.go`）；`router.go` 623 → 388 行 |
| N-A1 | ✅ | `assembleWriteLease` 108 行 / 嵌套 6 → 34 行 + `newLeaseOnLost`（25）+ `newLeaseWriteGate`（32）+ `leaseLost` / `leaseAcquireError` / `markLeaseHeld`；新增 `app/lease_write_gate_test.go`（首次写取租 → 持租放行 → 续约被抢占后 `onLost` 触发并拒绝新写，`-race` 通过）。**实施中自查出一次顺序错误**：抽 `leaseLost` 时先判了 `Has`，而持租时 `Has` 也为真，会把持租的库判成失租；对照 `lease.Manager` 的语义改回「先 `ValidFor` 再 `leaseLost`」，并由上面的测试钉住 |

### 10.1 门禁实测

| 检查 | 结果 |
| --- | --- |
| `go build ./internal/... ./cmd/...` / `go vet ./internal/...` | 通过 / 0 |
| `go test ./internal/... -count=1` | 30 个包全部 ok |
| `gofmt -l internal cmd` / `staticcheck -checks 'all,-ST*' ./internal/...` | 0 / 0 |
| `deadcode ./cmd/...` 生产不可达 / `deadcode -test` | 11（仍是 §5 保留清单）/ 0 |
| 文件头漂移脚本（§6.1-7） | 0；被搬移符号在其它文件头里没有残留引用 |
| > 800 行的生产文件 | 16 → **1**（`api/oai_responses.go` 852，§5 已定不拆） |
| 控制流嵌套 ≥ 5 | 7 → **2**（`systemSQLReferencesSensitive` 词法器、`DeleteDatabaseSync`，§9.3 已定不动） |
| 最长业务函数（不含平铺清单） | 246 → **115**（`sandbox/sshd/session.go handleSession`）；`createDirectRun` 101 |

未做：启动冒烟、`./scripts/smoke.sh`、真实 DuckDB / S3 集成（同 §8.1）。

## 附录 A：`internal/` 实际依赖图（`go list` 生成，供 C-6 重写 README）

```text
app → agentbridge alerts api audit auth backup catalog collection config cronjob dbbridge
      functionrun llmgateway objectstore observability sandbox sandbox/sshd systemdb trigger usage
api → agentbridge audit auth backup catalog collection config cronjob e2bproto functionrun llmgateway
      objectstore observability realtime sandbox sandbox/sshd systemdb trigger usage web utils/{crontab,strutil}
agentbridge → agentbridge/agentskills auth cronjob llmgateway sbcli systemdb utils/{mask,strutil}
systemdb    → auth catalog dbbridge objectstore observability utils/{mask,strutil}
llmgateway  → catalog observability systemdb
usage       → catalog llmgateway observability
backup      → catalog config dbbridge objectstore utils/crontab
catalog     → auth objectstore observability
cronjob     → systemdb ;  functionrun → systemdb ;  trigger → functionrun systemdb
audit       → catalog observability ;  dbbridge → catalog observability
sandbox     → config ;  sandbox/sshd → sandbox ;  sbcli → utils/mask
alerts / objectstore → observability ;  auth / config / observability → utils/strutil
collection / e2bproto / realtime / web / utils/* → （无 internal 依赖）
```

核对结论：无反向 import（`catalog`、`systemdb` 不 import `api`/`app`；`dbbridge` 不 import `api`/`app`），符合 `internal/AGENTS.md` 分层。注意 `catalog → auth`、`systemdb → auth` 两条与 AGENTS.md 图示（`api → auth / catalog`）并列关系不完全一致，属文档未画出而非违规，C-6 重写时一并画出。
