# internal/ 重构、精简与清理计划（planv6.8，simplify-code）

> 方法：`skills/simplify-code`，先走 R-branch（只读评估）。**本轮只写本文件，源码零改动。**
> 承接：`plan/planv6.6/internal-simplify-code-plan.md`（B1–B4 已执行：S-2/S-3 头注释错引、D-1 局部、T-1~T-9、Bug-2/Bug-3 已核实清零，见 §0.3）。
> 目标：让 `internal/` **更短的函数、更少的重复知识、更少的过期注释、一处定义一个规则**；不改变任何对外契约（HTTP 路由/错误码/JSON 字段/CLI 输出/系统库表结构/S3 key 布局）。

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
| C-1 | 195 个生产文件头（如 `api/router.go:1-31`、`systemdb/agents.go:1-29`） | 「用法 / 调用架构 / [上游] [下游] [同包] … 等 N 个 / 主要符号 … 另有 N 个顶层符号」是一次性生成物，内容与 `go doc`、IDE 重复，且 v6.6 已证明会漂移（当时 70+33+59 处错引）。仓库里**没有生成脚本**，只能手改。→ 每个文件头只保留 **1–3 行「文件说明：做什么 / 不做什么 / 关键约束」**，删除「用法」「调用架构」「主要符号」三段 | 无（仅注释）；需确认无工具解析 `文件说明`（本轮 `rg '文件说明' scripts build.sh .github` 为空） | 已验证 | **propose（A1）**：v6.6 A1 未决，本轮建议「整体去掉」，预计删 **约 4500 行**注释 |
| C-2 | 148 个文件、573 处计划编号注释（`planv6.0 D1`、`§8.1.6`、`P0-3`、`WR-01`、`SEC-04`、`AGC-ROUTE §3.3`、`B3 提取` 等） | 旧计划目录已删（见 planv6.7 §2 锚点表），读者需跳两层才知含义。→ **规则**：注释写「为什么」本身，编号最多保留一个作为可 grep 的锚点。例：`router.go:387` `// require 不带 delegable：路由对委托令牌默认拒绝（planv6.0 D1）` 已自解释，保留；`router.go:605` `// 云 Agent 路由块（B3 提取；…）`、`router.go:617`、`router.go:632-635`、`router.go:670-675` 中「B3 提取 / 原 mountV1Routes 尾部连续块」是**重构历史**，删除 | 无 | 已验证（抽样 40 处） | apply（随各文件改动顺带处理，不单开全仓批次，避免 blame 噪音） |
| C-3 | `api/router.go:252-255` | `NewRouter` 注释「按 plan1 规定的中间件顺序…业务路由由后续 plan 在此函数内挂载」已失真（顺序实际还有 site/CORS/security/perf/gzip/ratelimit）。→ 改为与代码一致的顺序列表 | 无 | 已验证 | apply |
| C-4 | `app/app.go:674-676` | `Start` 有两段重复的 doc（「Start 启动 HTTP server。阻塞调用者直到 Shutdown。」+「Start 启动 HTTP server（阻塞直到关闭）…」）→ 合并为一段 | 无 | 已验证 | apply |
| C-5 | `systemdb/cron_jobs.go:426` | 「v49 新增列由 ALTER ... DEFAULT 补齐」与 `AGENTS.md`「无版本化迁移、改 schema.go 并 reset」矛盾 → 核实 `schema.go` 中这些列是否 `NOT NULL DEFAULT`；若是，`NullString` 兜底可留但注释改为「防御旧行」；若列已非空，见 §1.4 S-6 | 无 | 推理 | apply（注释）/ propose（代码） |
| C-6 | `internal/README.md` | 仍描述已不存在的 `database/`（ducklake/registry/cache/sqlguard）、`llm_handler.go` 的 chat/stream/providers、`gocloudagent` 为 internal 模块；依赖图与 `AGENTS.md` 不一致；漏列 `agentbridge`、`sandbox`、`sbcli`、`systemdb`、`functionrun`、`trigger`、`collection`、`dbbridge`、`e2bproto`、`backup`、`alerts`、`realtime`、`cronjob` | 无 | 已验证 | apply：按 `go list` 实际 import 重写模块清单与依赖图（见 §6 附录 A） |

### 1.2 `api` 包：路由装配（P1）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| R-1 | `api/router.go:385` `mountV1Routes`（246 行） | 一个函数装 15 组资源。→ **按资源拆成同签名小函数**，`mountV1Routes` 只剩顺序调用：`mountAuthRoutes(v1)`、`mountProjectRoutes(v1,p)`、`mountDatabaseRoutes(p)`（db/sql/schema/data/meta）、`mountBackupRealtimeRoutes`、`mountKVRoutes`、`mountLLMRoutes`（含双段 v1 注释原样搬迁）、`mountUsageAuditRoutes`、`mountS3Routes`、`mountSystemRoutes`（metrics/logs/settings/llm traces/migrations）。把 `require` / `delegable` / `writeGate` 三个闭包提为一个小结构 `routeGuards{require, delegable, writeGate}`，替代现在 `mountAgentRoutes(p, deps, require, writeGate, bm)`、`mountGoFunctionAndCronRoutes(p, deps, delegable, writeGate)` 这种「闭包当参数逐个传」的签名 | 中：**注册顺序**影响 echo 路由匹配与 `buildOpenAPI(e.Routes())` 输出顺序；`writeGateRoutes` 驱动测试、`route_meta_test.go`、`TestOpenAIRoutePaths` 锁定路径 | 已验证（读过全文） | apply：纯搬移，**保持调用顺序与每行注册顺序完全一致**；验收加一条「`e.Routes()` 序列化前后逐行 diff 为空」的临时测试 |
| R-2 | `api/router.go:659-666` | `function-triggers` 6 条路由直接写 `auth.Require(auth.ProjectAdmin, PrincipalFromContext)`，**绕过了 `policyGate`**，与同文件其余路由（`require()` = `policyGate(auth.Require(...))`）不一致 | **行为差异**：改成 `require()` 会让这 6 条路由多过一道 API Key policy 闸门 | 已验证 | **propose（A3）**：先确认是有意还是遗漏；不论结论，加注释说明 |
| R-3 | `api/router.go:796-798` | `for _, prefix := range []string{e2bPrefix + "/*"} { e.Any(prefix, …) }` 单元素循环 → `e.Any(e2bPrefix+"/*", sh.unavailable)` | 低 | 已验证 | apply |
| R-4 | `api/router.go:780` `mountE2BRoutes`（103 行） | 控制面 `/sandboxes/...` 与 `/v2/sandboxes/...` 成对重复注册 10 组。→ 局部 `both := func(method, path string, h echo.HandlerFunc)` 一次登记两条，顺序按「v1 → v2」保持 | 中：注册顺序有变化时 OpenAPI 输出顺序变 | 已验证 | apply（保持原相对顺序：现代码有时 v1/v2 交错、有时成组，需逐行对照；若无法保持就 skip） |
| R-5 | `api/router.go:256` `NewRouter`（113 行） | 中间件链（site → requestID → CORS → security → perf → recover → accessLog → gzip → preAuth limit → bodyLimit）抽 `useGlobalMiddleware(e, deps)`；匿名登录/终端用户路由抽 `mountPublicAuthRoutes(e, deps)` | 低 | 已验证 | apply |
| R-6 | `api/router.go:676` `mountAgentRoutes` | 函数内既**装配**（confirmHub 注入 `deps.CloudAgent.Confirm` / provider、`newLLMDispatcher`、`CollabRunner.Attach`、`convergeCollaboration` 启动期副作用）又**挂路由**。→ 装配部分抽 `newCloudAgentHandler(deps) *cloudAgentHandler`；`convergeCollaboration(context.Background())` 是启动期 I/O，**不能**因拆分而改变调用时机（仍在路由注册前） | 中：副作用顺序 | 已验证 | apply（仅抽函数，不改调用点顺序） |

### 1.3 `api` 包：handler 重复知识（P1/P2）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| H-1 | **主体标识三套口径**：`api/agent_handler.go:933 principalIdentity`、`agentbridge/context.go:59 principalKey`（逐字相同：`UserID` → `"apikey:"+APIKeyID` → `""`）；`api/gofunction_handler.go:756 actorID`（`UserID` → 裸 `APIKeyID`）；`api/s3_site_handler.go:251 principalActor`（`"user:"+UserID` / `"key:"+APIKeyID`）；`api/cronjob_handler.go:797 recordAudit` 只取 `APIKeyID`（**登录用户审计 PrincipalID 为空**）；`api/gofunction_handler.go:794 recordAudit` 先 APIKey 后 UserID 覆盖 | 前两者逐字相同 → 在 `auth` 增加 `func (p Principal) Identity() string`（与现两者同口径），两处改为调用。其余三种口径写入了 DB（`created_by`、审计、站点发布记录），**不能**静默统一 | 前两者：低（纯函数，等价）；其余：高（持久化格式） | 已验证 | `principalIdentity`/`principalKey`：apply；`actorID`/`principalActor`/cron 审计：**propose（A4）**，并登记 Bug-1 |
| H-2 | `api/cronjob_handler.go:797`、`api/gofunction_handler.go:794`、`api/llm_handler.go:168`、`api/mcp_builtin_admin.go:114`、`api/s3_site_handler.go:243` 五个 `recordAudit*` | 结构相同：`audit==nil` 早退 → 取主体 → 拼 `AuditEvent{ProjectID, PrincipalID, Kind, RequestID, Status, Detail}`。→ 包级 `recordHandlerAudit(c, svc AuditService, ev AuditEvent)` 只负责 nil 判断与 `RequestID` 填充；**PrincipalID 由调用方按现口径传入**（不顺带统一，见 H-1）。注意 cron 版用 `context.Background()`、gofunction 版用请求 ctx——保留各自 ctx 选择（取消语义不同） | 中：ctx 差异 | 已验证 | apply（保持 ctx 与 PrincipalID 原样） |
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
| H-15 | `api/plan_handler.go:451` 包级 `var planMu sync.Map` | 每个 planID 存一个 `*sync.Mutex`，**永不删除**（与 `routeCancels` 不同，后者 `LoadAndDelete`）。长期运行进程中随计划数线性增长 | 低（每条 ~100B）但属于泄漏 | 已验证 | 登记 Bug-2；修复属行为改动 → **propose（A5）**：用引用计数或 `singleflight` 式 flights（`systemdb` 已有同构实现，见 S-3） |
| H-16 | `api/data_handler.go` 904 行 | `ListCollections` 87 行嵌套 4；`BatchDocuments` 78 行嵌套 4。尾部 `rowDocument`…`isMissingRelation` 约 150 行纯函数。→ 纯函数移 `collection_doc.go`；`ListCollections` 的「合并物理表与元数据」循环抽 `mergeCollectionInfos` | 低 | 已验证（签名层面）/ 推理（函数体） | apply |
| H-17 | `api/sandbox_handler.go:447 List`（114 行） | E2B 列表的过滤 / 分页 / 状态映射混在一起 → 抽 `parseSandboxListQuery(c)` 与 `toE2BSandboxList(rows)` | 低 | 推理 | apply（读全后） |
| H-18 | `api/storage_settings_handler.go:189 PutStorage`（117 行） | 校验 → 读 YAML 节点树 → 改写 → 原子写回。→ 抽 `validateStorageBody` 与 `rewriteStorageYAML(root *yaml.Node, body)` | 低 | 推理 | apply（读全后） |

### 1.4 `systemdb` 包：仓储样板（P1）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| S-1 | 174 处 `if s == nil \|\| s.db == nil { return …, ErrUnavailable }` | 每个公开方法第一行都是它。→ 增加 `func (s *Store) conn() (*sql.DB, error)`；方法体改为 `db, err := s.conn(); if err != nil { return X{}, err }`。行数不减多少，但把「nil Store 可调用」这一契约集中到一处并带注释 | 低 | 已验证 | **skip（默认）**：收益有限且 174 处改动 blame 噪音大；仅在 S-2/S-4 动到的函数里顺带使用 |
| S-2 | Get-by-key 样板（结构重复检测命中 **7+3+3 组**）：`GetAgentApproval`、`GetAgentTagRow`、`GetCloudAgent`、`GetCronJob`、`GetCronJobByName`、`GetMCPServer`、`GetMCPServerByName`、`GetAgentTurn`、`GetAgentTask`、`GetAgentGroupCron`、`GetSandbox*`（3） | 形如「QueryRow → scanX → `if errors.Is(err, sql.ErrNoRows) { return X{}, sql.ErrNoRows }` → return」。其中 `ErrNoRows` 那段**是空操作**（scan 出错时 scanX 已返回零值 + err；14 处）。→ 泛型 `func queryOne[T any](ctx, db, scan func(rowScanner) (T, error), q string, args ...any) (T, error)`，每个 Get 缩成 3–4 行 | 低：需确认每个 `scanX` 出错时返回零值（已抽查 `scanCronJob:432` 是 `return CronJob{}, err`） | 已验证（抽查 2 个） | apply（逐个确认 scanX 零值返回后替换） |
| S-3 | `systemdb/agent_tags.go:579 lockAgentTag` 与 `systemdb/agents.go:885 lockAgentSeed` | 30 行**逐字同构**（字段名不同）；`store.go:73` 注释自己写着「与 seedMu/seedFlights 同构」。→ 抽 `type keyedFlight struct{ mu sync.Mutex; m map[string]chan struct{} }` + `func (f *keyedFlight) lock(ctx, key) (func(), error)`，`Store` 持有 `seedFlight`、`tagFlight` 两个实例。同一结构可供 H-15 的 `planMu` 复用（放在 `systemdb` 内私有，`api` 不可见；`api` 侧另起一份或提到 `internal/utils`——**后者需评估是否满足 utils 入库标准**） | 低：等价搬移 | 已验证 | apply |
| S-4 | 软删除样板：`ArchiveAgentThread`、`ArchiveCronJob`、`ArchiveGoFunc`（18 行×3），`ArchiveMCPServer`、`SoftDeleteSandbox`（17 行×2），`ArchiveAgentGroupCron*`（12 行×2） | 「UPDATE … SET archived_at=?, updated_at=? WHERE … AND archived_at IS NULL → RowsAffected==0 → ErrNoRows → notifyWrite」。→ `func (s *Store) execAffectOne(ctx, q string, args ...any) error`（0 行返回 `sql.ErrNoRows`，成功 `notifyWrite`）。**注意** `n, _ := res.RowsAffected()` 吞错误的写法需原样保留（DuckDB 驱动不一定实现），不要「顺手」改成报错 | 低 | 已验证（读过 `ArchiveGoFunc`） | apply |
| S-5 | `systemdb/async_flush.go:154/170`、`systemdb/traces.go:193` 三个 `flush*Batch` | 同构：insert 失败把 batch 退回缓冲头部。→ 泛型 `flushBatch[T](s, ctx, batch []T, insert func(ctx, *sql.DB, []T) error, requeue func([]T))` | 中：三个缓冲区受同一把 `s.mu` 保护，requeue 闭包需在锁内 | 已验证 | apply |
| S-6 | `systemdb/observe.go:34/52` `observeRead` / `observeReadPart` | 仅多一个 `part` 字段与日志 message 不同。→ 保留两个入口，内部共用 `observeFields(…)` | 低 | 已验证 | P3，可选 |
| S-7 | `systemdb/agents.go` 935 行、`systemdb/agent_collab.go` 874 行、`systemdb/agent_tags.go` 608 行 | 按实体拆：`agents.go` → `agents.go`（CloudAgent）+ `agent_threads.go` + `agent_messages.go` + `agent_runs.go` + `agent_seed.go`；`agent_collab.go` → `agent_turns.go` + `agent_tasks.go` + `agent_turn_events.go` | 低：纯移动 | 推理（按符号列表） | apply |
| S-8 | `systemdb/sandboxes.go:248/260/272` 三个 `List*Sandboxes` 与 `app/sandbox_store.go:129/140/151` 三个适配器 | 适配器三份「查 → 循环 toSandbox」同构 → `mapSandboxes(rows []systemdb.SandboxRow, err error) ([]sandbox.Sandbox, error)` 一行包装 | 低 | 已验证 | apply |
| S-9 | `systemdb/mcp_servers.go:200/210` `ParseMCPToolPolicies` / `ParseMCPHeaders` | 同构 JSON 解析。→ 私有 `parseJSONMap[T]` | 低 | 已验证 | P3，可选 |

### 1.5 `config` 包（P2）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| G-1 | **一个字段要在 5 处登记**：`config.go` 结构体 + `defaults()`（130 行）+ `yaml.go` 镜像结构体 `yaml*`（指针字段，25 个类型）+ `applyYAML*`（156 行）+ `env.go applyEnv`（174 行）+ `Redacted()`（124 行）。例：`MaxConcurrentGoRuns` 出现在 `config.go:601/837/956-959`、`yaml.go:305/597`、`env.go:167` | 这是本包复杂度的根源。可选方案：(a) 用 `yaml.Node` 解码到 `Config` 再记录「显式出现的路径集合」替代镜像结构体；(b) 用 struct tag + 反射生成 env 覆盖。两者都会改变**错误文本**、**未知字段处理**和**零值语义**，风险高 | 高：配置是启动契约，`load_test.go`（761 行）锁定大量边界 | 已验证（问题）/ 未核实（方案等价性） | **propose（A6）**，本轮不做；v6.6 §4「平铺字段清单长但不复杂」结论维持，但记录为长期债 |
| G-2 | `config/yaml.go:400 applyYAML` | `Instance.Lease` 的 5 个字段手写 `if yc.X != nil { cfg.X = *yc.X }` / `if .set {}`，同文件已有 `setBool` / `setDur` 辅助 → 改用辅助函数，与其余字段一致 | 低：等价 | 已验证（`yaml.go:404-416`） | apply |
| G-3 | `config/config.go` 1303 行 | 结构体定义 + `Effective*` 访问器 + 校验 + 脱敏混在一起。→ 拆 `config_types.go`（全部 `*Config` 类型与 `Effective*`）、`validate.go`（`Validate`/`validate*`/`joinErrors`）、`redact.go`（`Redacted`）；`config.go` 只留 `Load`/`defaults`/路径 | 低：纯移动 | 已验证（符号列表） | apply |
| G-4 | `config/env.go:398-441` `setEnvBool/Int/Int64/Float64/Duration` | 5 个同构函数（检测命中 Bool/Int）。→ 泛型 `setEnv[T](dst *T, key string, parse func(string) (T, error))` | 低 | 已验证 | P3，可选 |
| G-5 | `config/config.go:956-959` | `Validate()` 里把 `MaxConcurrentGoRuns==0` 改写为 16。`Validate` 是**值接收者**（`func (c Config) Validate()`，config.go:891），唯一调用点 `config.go:703 cfg.Validate()` 看不到这次赋值 → 这行是**无效赋值**；显式配 `max_concurrent_go_runs: 0` 时实际值仍是 0，下游 `functionrun.NewRunner(…, 0)` 的语义决定结果 | 视 `NewRunner(0)` 语义而定 | 已验证（接收者与调用点） | 登记 Bug-3；**propose**：删掉这行（保持现行为）或把归一化移到 `Load()`（改变行为） |

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
| O-8 | `agentbridge/agentskills/skills.go` 中 `Known`、`Prefixes`、`Body`、`EmbeddedAllows`、`EmbeddedReadOnly`、`walkSkillDocs`、`parseAllow`、`parseCommentList`（8 个，deadcode 生产不可达） | 只有 `agentbridge/*_test.go` 在用。这是新增未提交代码（`agentskills/` 未跟踪），说明**新模块带了一组没接线的 API**。→ 先问作者：是待接线还是测试辅助？测试辅助则移到 `export_test.go` / 测试文件 | 低 | 已验证 | **propose（A7）** |
| O-9 | `agentbridge/context.go:38 withRunContext` | 生产不可达，7 个测试文件在用 → 移到 `agentbridge/testutil_test.go`（该文件已存在） | 低 | 已验证 | apply |
| O-10 | `agentbridge/bench/`（532KB，Python + 任务数据）、`agentbridge/docs/`（`eino-guide.md`、`pi-guide.md`） | `internal/` 是 Go 包目录，评测脚本与外部框架笔记放这里会被当作 Go 包的一部分阅读；`docs/` 两份是第三方框架指南，与 agentbridge 职责无关 | 无（非 Go） | 已验证 | **propose（A8）**：迁到仓库根 `bench/agent/`、`docs/research/`，同步改 planv6.7 §2.4 的外部引用 |
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

## 2. Bugs found (not fixed)

与精简无关，单独登记，不计入上面的条目数；**不在重构批次里顺手修**。

| ID | 位置 | 级别 | 说明 / 复现 |
| --- | --- | --- | --- |
| Bug-1 | `api/cronjob_handler.go:797 recordAudit` | 已确认（读码） | 只取 `principal.APIKeyID`，登录用户（`UserID` 非空、`APIKeyID` 空）操作定时任务时审计 `PrincipalID` 为空串；同类的 `gofunction_handler.go:794` 会回落到 `UserID`。复现：以登录态 `POST /cron-jobs`，查 `GET /audit` 的 principal 字段 |
| Bug-2 | `api/plan_handler.go:451 planMu` | 已确认（读码） | 包级 `sync.Map` 只 `LoadOrStore` 不删除，每个历史 planID 留一个 `*sync.Mutex`，进程生命周期内单调增长 |
| Bug-3 | `config/config.go:956-959` | 已确认（读码） | 值接收者 `Validate()` 内对 `c.Limits.MaxConcurrentGoRuns` 的赋值对调用方不可见；显式写 0 时注释承诺的「回落 16」不会发生 |
| Bug-4 | `api/router.go:659-666` | 怀疑 | `function-triggers` 路由未经 `policyGate`，API Key 的 policy 限制（`api_key_policy.go`）对这 6 条路由不生效。是否有意需作者确认（见 A3） |

## 3. 需要审批的决策

| ID | 决策 | 会变的输入或行为 |
| --- | --- | --- |
| A1 | C-1：生成式文件头「用法 / 调用架构 / 主要符号」整体删除，只留 1–3 行职责说明 | 仅注释；约 −4500 行；blame 噪音。不建议继续手工维护 |
| A2 | C-2：计划编号注释的保留规则（「最多一个锚点 + 必须自解释」） | 仅注释 |
| A3 | R-2 / Bug-4：`function-triggers` 是否补 `policyGate` | 补上后受限 API Key 可能被拒（403） |
| A4 | H-1：`actorID`（裸 APIKeyID）、`principalActor`（`user:`/`key:` 前缀）、`principalIdentity`（`apikey:` 前缀）三种落库口径是否统一 | 新写入的 `created_by` / 审计 / 站点记录格式变化；历史数据不迁移则新旧混存。**建议不统一，只消除两份逐字相同的实现** |
| A5 | H-15 / Bug-2：`planMu` 改为可回收的 keyed lock | 无外部可见行为，但改并发原语，需并发测试 |
| A6 | G-1：配置五处登记的结构性改造 | 错误文本、未知字段、零值语义都可能变；本轮不做 |
| A7 | O-8：`agentskills` 8 个未接线导出函数是「待接线」还是「测试辅助」 | 删除 / 移动 / 接线三选一 |
| A8 | O-10：`agentbridge/bench`、`agentbridge/docs` 迁出 `internal/` | 路径变化；需改 `planv6.7` 与 bench README 的引用 |
| A9 | Bug-1 / Bug-3 的修复 | Bug-1：审计字段由空变为 UserID；Bug-3：删行（行为不变）或移到 `Load()`（显式 0 变 16） |

## 4. 目标结构（重构后）

只列本轮会动结构的文件；未列出的文件只做注释清理（C-1/C-2）。

```text
internal/api/
  router.go                 NewRouter + useGlobalMiddleware + mountPublicAuthRoutes + routeGuards（≤150 行）
  routes_v1.go              mountV1Routes 拆出的 mountAuth/Project/Database/KV/LLM/Usage/S3/System
  routes_agent.go           newCloudAgentHandler + mountAgentRoutes
  routes_function.go        mountGoFunctionAndCronRoutes + mountGoRoutes
  routes_e2b.go             mountE2BRoutes
  context.go                + requireProject / projectOf / principalOf / requestID
  error.go                  + badRequest
  audit_helper.go           recordHandlerAudit（H-2）
  agent_handler.go          cloudAgentHandler 结构体与访问器
  agent_crud_handler.go     agent CRUD（H-9）
  agent_thread_handler.go   thread / message
  agent_approval_handler.go approval / cancel
  agent_dto.go              全部 DTO 与 to*DTO
  collab_coordinator.go     轮次状态机（≈650 行）
  collab_stream.go          turnStream（H-12）
  collab_query_handler.go   轮次查询 handler + DTO
  kv_commands.go            解析辅助 + kvCommandTable 合并（≈300 行）
  kv_cmd_{key,string,hash,list,set,zset}.go
  collection_doc.go         data_handler 尾部纯函数（H-16）
internal/systemdb/
  query.go                  queryOne[T] / execAffectOne / flushBatch[T] / keyedFlight（S-2~S-5）
  agents.go → agents.go + agent_threads.go + agent_messages.go + agent_runs.go + agent_seed.go
  agent_collab.go → agent_turns.go + agent_tasks.go + agent_turn_events.go
internal/config/
  config.go (Load/defaults/路径) + config_types.go + validate.go + redact.go
internal/app/
  app.go                    NewWithRegistry / assembleDeps 只剩步骤调用（各 ≤80 行）
  assemble_objectstore.go   connectObjectStore + s3ConfigFrom + duckLakeStorageFrom
  assemble_system.go        bootSystemDatabase + assembleCatalog + assembleRegistryAndCache
  assemble_api.go           assembleDatabaseHandler + assembleFunctionPlane + assembleScheduler + buildAPIDeps + buildHTTPServers
internal/sbcli/
  commands.go               单一命令表 commands（O-1）+ idAction/idQuery（O-2）
  dev.go → dev.go + dev_lock.go + dev_replay_{db,fn,cron}.go
internal/sandbox/
  manager.go → manager.go + manager_lifecycle.go + manager_files.go + manager_thread.go + reaper.go
internal/agentbridge/
  aliases.go                删除（O-7）
```

## 5. Not changed（看起来能简化，但不动）

- **v6.6 §4 全部延续**：`KeyBuilder` 转发方法、内存版 FileStore/BlobStore（`NewMemoryBlobStore`/`NewMemoryFileStore` 等 deadcode 项）、`fieldString`/`fieldDuration`、`e2bproto.DecodeFrame`、`runEventHub.sweep`、`confirmHub.pendingCount`、`sbcli.DispatchKnown`、`agentbridge.InProcessCLI`、`systemdb.NewStoreForTest`、6 处 `ctx == nil` 兜底、`sbcli` 的 `io/ioutil` 白名单字符串、`sort.*` 不换 `slices.*`。
- **系统库保护判断**（`if catalog.IsSystemDatabase(db)`）与 handler 内 `writable` 检查：保持显式、可 grep，不抽中间件（AGENTS.md 硬约束 1 的落点）。
- `catalog/sql_repository.go isUniqueViolation` 与 `systemdb/seed.go isDuplicate`：匹配串不同（前者不认 `already exists`、后者不认 `constraint failed: UNIQUE` 精确串），合并会改变「哪些错误被当作重复」，不合并。
- `oai_dto.go` 与 `oai_responses.go` 的两套 DTO：Chat Completions 与 Responses 是两个协议（AGENTS.md「Responses 是 chat 之上的协议转换」），共享结构会把协议差异藏进条件分支。
- `config/env.go applyEnv`、`config/yaml.go rejectYAMLSecrets`、`observability.NewMetrics`、`perfbench` 用例表：平铺清单，长但线性，拆了反而难对照。
- `api/collection_filter.go parseOr/parseAnd`：递归下降的分层写法比参数化 `parseBinary` 更直观（O-12 倾向 skip）。
- `systemdb` 174 处 nil 守卫（S-1）：不做全量替换，只在 S-2/S-4 动到的函数里顺带。
- `n, _ := res.RowsAffected()` 吞错误：DuckDB 驱动兼容性考虑，原样保留。
- 双段 `/v1/.../v1/chat/completions` 路径及其注释：搬迁时**逐字保留**（AGENTS.md 明确禁止"顺手修正"）。

## 6. 实施批次

所有批次的通用前提：**等 `internal/` 下 209 个未提交改动落地（提交或用户确认）之后再开始**，否则搬文件会与在途改动冲突。每批「一次只做一种变换」，改完先跑该包测试，再对照 `skills/simplify-code/references/behavior-traps.md` 想一遍「什么输入会让新旧不同」。

| 批次 | 内容 | 风险 | 审批 | 预估净变化 |
| --- | --- | --- | --- | --- |
| **P0 卫生** | Q-1、Q-3、C-3、C-4、C-6（README 重写）、O-4、O-9、R-3 | 极低 | 否 | −50 行 |
| **P1 重复知识** | H-1（仅两份逐字相同的）、H-2、H-4、H-5、H-6、H-7、H-8、H-10、S-2、S-3、S-4、S-5、S-8、O-5、O-7、O-11、G-2、A-3、A-4 | 低–中 | 否 | −600～−800 行 |
| **P2 拆大函数** | R-1、R-4、R-5、R-6、H-13、H-14（先补测试）、A-1、A-2、O-1+O-2（先写快照测试）、O-3 | 中 | 否（纯结构），但单独成批、单独提交 | 函数数量 +、最长函数从 246 → ≤100，最深嵌套 7 → ≤4 |
| **P3 拆大文件** | H-9、H-11、H-12、H-16、H-17、H-18、S-7、G-3、O-6 | 低（纯移动） | 否 | ≈0（行数不变，单文件 ≤600 行） |
| **P4 注释整体处置** | C-1、C-2 | 无（仅注释） | 是：A1、A2 | −4500 行 |
| **P5 需审批项** | R-2、H-3、H-15、O-8、O-10、Bug-1~4 | 中 | 是：A3–A9 | — |

建议顺序 P0 → P1 → P2 → P3 → P4 → P5。P4 放在结构调整之后，避免先改头注释再搬文件造成二次改动；P3 与 P4 也可合并为「搬到新文件时直接写新格式的文件头」。

### 6.1 动手前的固定检查

1. 移动 / 删除导出符号前：`rg '\bSymbol\b' internal cmd --glob '*.go'` 确认调用方，含跨包测试。
2. 拆路由（R-1/R-4/R-5）前，先加一个临时测试：把 `NewRouter(deps).Routes()` 按注册顺序序列化为 `method path name` 列表写入 golden；拆分后 diff 必须为空，验收后删除该临时测试。
3. 合并 CLI 命令表（O-1）前，先加快照测试：旧 `readOnlyCommands` 键集合 + 旧 `dispatch` 能返回非 nil 的键集合；合并后两集合分别相等。
4. H-14 `runPlan` 动手前补表驱动测试：proposal 解析失败 → paused；阶段 skip → 下一阶段；依赖未完成 → blocked + paused；全部完成 → completed。
5. 泛型辅助（S-2/S-5）引入前确认 `go.mod` 的 `go 1.25.0` 满足（满足）。
6. 新增文件写 1–3 行的新格式文件头（不再生成「调用架构 / 主要符号」）。

### 6.2 每批验证门禁

```bash
export PATH=$PATH:~/go/bin
go build ./internal/... ./cmd/... && go vet ./internal/... && go test ./internal/...
gofmt -l internal cmd                                     # 期望为空
staticcheck -checks 'all,-ST*' ./internal/...             # 期望 0（基线 1：errorCode）
deadcode ./cmd/... | grep '^internal/' | wc -l            # 基线 24；P0 后 ≤22，P5(A7) 后 ≤14
deadcode -test ./cmd/... ./internal/... | grep '^internal/' | wc -l   # 期望 0（基线 1）
# 涉及路由 / 鉴权 / CLI / 装配时追加：
go test ./internal/api/ -run 'Route|WriteGate|OpenAI|Policy' -count=1
go test ./internal/sbcli/ ./internal/app/ ./internal/auth/ -count=1
./build.sh smoke   # 若脚本存在该目标；否则启动一次 simplebase 并 curl /health/ready
```

### 6.3 预期收益（可量化指标）

| 指标 | 基线 | 目标 |
| --- | --- | --- |
| 生产代码最长函数（不含数据表） | 246 行（`mountV1Routes`） | ≤100 行 |
| 最深嵌套 | 7（`runPlan`） | ≤4 |
| >100 行的函数数 | 33（含 `perfbench` 用例表） | ≤10（剩余为平铺清单类） |
| >800 行的生产文件 | 16 | ≤4（平铺清单类，如 `observability/metrics.go`） |
| 文件头生成式注释 | 5639 行 / 195 文件 | ≤1000 行 |
| 逐字重复实现 | `principalIdentity/principalKey`、`lockAgentTag/lockAgentSeed`、`aggregateLimits`×2、Get 样板 13 处、Archive 样板 7 处、flush 3 处、sandbox 适配 3 处 | 各收敛为 1 处 |
| 命令登记处 | 2（dispatch + readOnly） | 1 |
| staticcheck / deadcode -test | 1 / 1 | 0 / 0 |

## 7. Verification（本轮评估）

- **已运行（只读）**：§0.2 全部命令；`go test ./internal/...` 通过；AST 脚本（函数长度 / 嵌套 / 结构重复）放在 `.session_tmps/`，未进入仓库。
- **已读源码核实**：`router.go` 250–889、`app.go` 200–679、`plan_handler.go` 440–659、`kv_commands.go` 表头、`sandbox/manager.go` 的 ReadFile/WriteFile/WriteFileForThread、`systemdb` Get/Archive/flush/lock 样板、`sbcli/commands.go` 双表、`config` 的 Validate 接收者与 `applyYAML` 开头、五个 `recordAudit*`、四个主体标识函数。
- **推理未执行**：H-13 另两处事件回调是否完全同构、H-16/H-17/H-18/O-3 的函数体细节、A-3 中 `backup`/`reset.go` 是否也手抄 S3 字段、C-5 的列是否 `NOT NULL`。执行对应条目前必须先读全。
- **未运行**：`gocloudagent` / `godatabase` / `gofunction` / `gosandbox` 独立模块、前端、`scripts/examples-verify`；这些不在 `internal/` 范围，但 O-1（`CommandReadOnly`）被 `gocloudagent` 间接依赖，P2 批需追加 `cd gocloudagent && go test ./...`。
- **范围声明**：大函数与重复检测覆盖 `internal/` 全部生产 `.go`（排除 `bench/`、`perfbench/` 用于重复检测）；结构重复只检测「函数体 token 归一化后完全相同」的情况，近似重复（如 H-2 五个 `recordAudit`）靠人工发现，**不代表已穷尽**。

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
