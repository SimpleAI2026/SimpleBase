# 全仓代码简化与清理计划（v6.5）

> **状态：** Batch 1–3（`CLN-1~3`、`UI-1~4`、`GDB-1~2`、`SBX-1~3`、`GCA-1`、`GFN-1`、`INT-1~3`）✅ 已于 2026-11-13 执行并验证（§8）；`BUG-APP-01` ✅ 已于 2026-11-14 独立修复并补回归测试，站点发布链路（编译 → 上传对象存储 → 子域名 URL）✅ 端到端验证（§9）。仍待用户确认：§5 `APV-1~4`（涉及导出接口、无迁移红线、`.gitignore` 与追踪文档，按 Golden Rule 2 不自动执行）。
> **范围：** `internal/`、`gofunction/`、`godatabase/`、`gocloudagent/`、`gosandbox/`、`ui/src/`（排除 `ui/src/components/ui/**`、`internal/web/dist/**`、`ui/src/locales/**` 正文词条）
> **基线：** `HEAD @ 726621e` + 当前工作区 v6.5 未提交改动（116 项）；实施每张任务卡前须重新核对 `git status` 与源码行号
> **依据：** `simplify-code`（Golden Rules 1–7、`references/assessment.md`、`references/rules.md`、`references/languages-tier1.md`、`references/behavior-traps.md`），以及根目录 `AGENTS.md`、`internal/AGENTS.md`、`ui/AGENTS.md`、各独立模块 `README.md`
> **与既有计划的关系：** 承接并避开 [`planv6.0/backend-code-simplification-plan.md`](../planv6.0/backend-code-simplification-plan.md) 与 [`planv6.0/frontend-code-simplification-plan.md`](../planv6.0/frontend-code-simplification-plan.md) 已落地的 A1–B4 批次，重点聚焦 **v6.1–v6.5 演进引入的新复杂度、功能迁移遗留的死代码、四个独立 Go 模块内部重复，以及仓库卫生清理**。

---

## 1. 目标、黄金准则与项目硬边界

本计划的目标是将各模块收敛到**最小可读等价实现（Smallest Readable Equivalent）**：消除已证实的死代码、重复知识、无意义薄包装与噪音注释，降低长函数的嵌套和局部认知负担；**不追求最少字符数，不引入投机性抽象框架**。

### 1.1 必须严格保持的不变量（Golden Rules 1–3）

1. **行为完全等价**：输入校验先后、返回值、副作用次数与顺序、HTTP 状态码、错误码与文案、JSON/SSE 字段（含 `nil` vs `[]`）、资源获取与 `defer` 释放时机、锁粒度与 goroutine 生命周期保持不变。任何改变可观察行为或修复缺陷的提案均单列于 **§6（需确认项）** 或 **§7（顺带发现的缺陷）**，绝不混入纯简化提交。
2. **公共契约与模块物理边界不变**：
   - `gofunction`、`godatabase`、`gocloudagent`、`gosandbox` 四个嵌套独立模块**严禁 import 根模块任何包（含 `internal/...`）**；`gosandbox/host` 不得反向 import `gosandbox/vm`；`gocloudagent/agentcore` 不得反向 import `gocloudagent`；`godatabase` 根包不 import 子包。
   - S3 对象键布局与 manifest JSON 字段属于磁盘契约（受 `internal/objectstore/testdata/keys.golden` 锁定），严禁改动。
   - `simplebase dev`（`internal/sbcli/dev.go`）与 `skills/simplebase/scripts/sbdev.py` 的校验文案、动作序列、lock 字段顺序保持逐字一致。
   - OpenAI 兼容出口双 `v1` 路径（`/v1/projects/:projectID/v1/...`）与 `writeOAIError` 范围保持不变。
3. **安全与工程红线不退化**：
   - 系统库保护（`IsSystemDatabase` 拒写、`systemLeaseAdapter` 双保险、不经用户库 registry 打开）与实例级 `writable` 门禁不省略。
   - 权限判定只走 `auth.Principal.HasPermission` / `PermissionImplied`；沙盒与本机 shell 保持 fail-closed（隔离或策略不可用时绝不退回裸执行）。
   - 不新增第三方依赖，不修改 `go.mod` / `package.json` / `tsconfig.json` / `vite.config.ts` / `.gitignore`（除非用户针对具体清理项单独批准）。
   - 前端保持 `interface` 定义对象形状、`services/api.ts` 唯一请求出口、reka-ui `data-[state=...]` 真实状态选择器、中英双语 key 完全同构。

---

## 2. 全仓扫描总览与优先级矩阵

优先级定义遵循 `simplify-code/references/assessment.md`：**P1** = 显著消除重复维护负担或大块死代码；**P2** = 边界清晰的中小规模去重、卫语句或局部收敛；**P3** = 低成本可读性与代码卫生清理。

| ID | 优先级 | 模块与位置 | 冗余 / 坏味道 → 简化提案 | 收益 | 风险 | 证据等级 | 需审批 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **CLN-1** | P1 | `gofunction/main` (15.0MB)、`gosandbox/main_other` (2.4MB) | 源码目录残留未追踪编译二进制产物 → 直接删除残留产物（可选批准补充 `.gitignore` 模式） | 消除 17.4MB 工作区脏产物与误打包风险 | 低 | 已核实（`git ls-files` 未追踪） | 否（改 `.gitignore` 需批） |
| **UI-1** | P1 | `ui/src/components/ai/AgentScheduleModal.vue` (332 行)、`AgentManager.vue:179-188,303-304,604-619`、`http-api.ts:792-804,1973-1994` | `/agent-schedules` 写接口已 410 退役，`openSchedule` 已改为路由跳转 `/console/cron-jobs`，`scheduleModalOpen` 全仓永不为 `true` → 删除不可达弹窗组件、死回调及 5 个永不调用的写 API 方法（保留只读 `list` 供旧排期徽标展示） | 净删 ~450 行前端死代码，消除 `AgentManager.vue` 未覆盖死函数对 95% functions 阈值的拖累 | 低 | 已核实（全仓引用扫描） | 否（若连 `ApiClient` 类型删方法见 §6） |
| **UI-2** | P1 | `ui/src/composables/useAsyncAction.ts`、`ui/src/composables/useAiChat.ts` | `useAsyncAction` 在 `ui/src/` 零生产调用；`useAiChat` 函数在 `AiChat.vue` 下线后零生产调用（仅 `ChatMsg` 接口被 3 处 `import type` 引用） → 将 `ChatMsg` 迁入 `services/types.ts`，下线两个零调用 composable 及对应孤立单测 | 收敛契约到 `types.ts`，消除两处失活抽象维护负担 | 低 | 已核实（AST/文本全量检索） | 否 |
| **SBX-1** | P1 | `gosandbox/host/start_linux.go:88-177`、`start_darwin.go:62-130`、`start_posix.go` | `ptyWriter` 类型、`(*process).Resize`、管道/PTY 双分支脚手架、进程组取消监听 goroutine 在 Linux 与 Darwin 两份文件中逐字重复 → 上提至已有的 `start_posix.go` 共用 | 消除跨平台双份维护与平台漂移风险 | 中 | 已核实（双文件逐行比对） | 否 |
| **SBX-2** | P2 | `gosandbox/host/run_linux.go:53-117`、`start_linux.go:46-87,125-136` | Linux `Run` 与 `Start` 重复 45+ 行前置校验、`Probe`、`resolveHelper`、`BwrapArgs`、策略匿名管道写入与 `cgroupGroup` 初始化 → 提取包内私有 `prepareLinuxBwrap` 与 `setupCgroup` | 隔离策略管道协议与 cgroup 生命周期单一事实来源 | 中 | 已核实 | 否 |
| **SBX-3** | P2 | `gosandbox/vm/local/local.go:312-352,581-629` | `Driver.Exec` 与 `Driver.Start` 重复 `Shell`/`Cmd` `LookPath` 解析与 `baseEnv` + `r.Env` 拼装 → 提取私有 `resolveCommandArgv` 与 `mergeEnv`（保留 `Exec` 127 退出码 vs `Start` error 的各自返回语义） | 降低 660 行 `local.go` 重复分支 | 低 | 已核实 | 否 |
| **GDB-1** | P2 | `godatabase/filedb/remote.go:490` (`sqlQuote`)、`godatabase/dialect.go:168` (`sqlLiteral`) | 两处手写 `strings.ReplaceAll(s, "'", "''")` 与同模块 `godatabase/sqlx.Literal` 字节级重复 → 统一调用 `sqlx.Literal` 并删除私有副本 | 统一 SQL 字面量转义入口 | 低 | 已核实 | 否 |
| **GDB-2** | P2 | `godatabase/kv/{key,string,set,hash,zset}.go` | ① `key.go` 的 `getMeta` 与 `getMetaAny` 重复 6 列 `Scan` 与 `ErrNoRows → ErrNotFound` 映射；② 5 个文件共 7 处手写 `ph := make([]string, ...)` + `strings.Join(ph, ",")` 构造 `IN (?,...)` → 提取包内 `scanKeyMeta` 与 `inPlaceholders` | 消除 KV 层机械样板代码 | 低 | 已核实 | 否 |
| **INT-1** | P2 | `internal/api/gofunction_handler.go:242-605` | `Create`、`Patch`、`CreateVersion`、`ActivateVersion`、`TestVersion` 重复项目上下文提取、`writable` 检查、`IsAdminProject` 拦截、`store == nil` 守卫及源码编译校验 + 默认激活分支 → 提取私有 `requireWritableStore` 与 `validateAndExtractExports`（严格保持各端点对 `IsAdminProject` 的既有覆盖范围） | 压缩 928 行 handler 中的前置守卫噪音 | 低 | 已核实 | 否 |
| **INT-2** | P2 | `internal/app/app.go:326-395` | `NewWithRegistry` 在沙盒 reaper、cron、backup、kvSweeper 4 处重复懒初始化 `schedulerBaseCtx`/`schedulerCancel` → 提取私有 `a.ensureSchedulerCtx()`，并将沙盒/备份装配按连续阶段收敛为同包私有方法 | 缩短 262 行 `NewWithRegistry` 主干，提升装配可读性 | 低 | 已核实 | 否 |
| **INT-3** | P3 | `internal/sandbox/paths.go:95`、`internal/sbcli/dev.go:89`、`internal/api/kv_commands.go:1412,1423`、`internal/systemdb/traces.go:202-205`、`internal/api/gocloudagent_access.go:52-68` | 无业务语义的单行透传包装与未用 `internal/utils` 的残留：`truncateUTF8` 直转 `strutil.TruncateValidUTF8`、`devSHA256` 直转 `hashx.SumString`、`sortStrings` 直转 `sort.Strings`、`traces.go` 手写 bool→0/1（已有 `sqlval.BoolInt`）、`NewCloudAgent*` 双层构造函数 → 内联私有透传包装、复用 `sqlval.BoolInt` | 消除无意义的调用跳数（`rules.md` §3/§20.3） | 低 | 已核实 | 否 |
| **GCA-1** | P2 | `gocloudagent/cli_tool.go:87-135`、`gocloudagent/runtime.go:630-641` | `callGate.wait` 与 `callGate.release` 重复锁内 map 懒初始化与 channel 获取；`runtime.go` 的 `consumeVariant` 为纯一行透传 `agentcore.ConsumeVariant` → 提取 `callGate.slot(id)` 并内联 `consumeVariant` | 并发门控状态读写集中在一处，减少锁样板 | 低 | 已核实 | 否 |
| **GFN-1** | P3 | `gofunction/internal/interp/jsonrun.go:129-133`、`compilefast.go:68-113` | `ValidateHTTPFuncs` 手写双层循环插入排序（模块已是 Go 1.25）；`compilefast.go` 多处重复 `trySlot` 快慢路径包装 → 用 `slices.SortFunc` + `cmp.Compare` 替换手写排序；按需收敛无特化的单目槽位写回辅助 | 符合 Go 1.25 标准库惯用法，不触碰热路径闭包分配语义 | 低 | 已核实 | 否 |
| **UI-3** | P2 | `ui/src/services/http-api.ts:651-730,1889`、`ui/src/components/models/TraceIOBlock.vue:74`、`ui/src/docs/catalog.ts:23` | v6.5 新增的 LLM Trace 映射函数引入了 6 处新 `Record<string, any>`（违反 `AGENTS.md`「`http-api.ts` 现存 `any` 只减不增」），`TraceIOBlock.vue` 含 `value: any`，`catalog.ts` 用 `type MetaFile = {...}` 定义对象形状 → 在 `types.ts` 补齐私有/导出 wire `interface`，将 `TraceIOBlock` 改为 `unknown`，`MetaFile` 改为 `interface` | 恢复前端类型纪律，净减 `any` 数量 | 低 | 已核实 | 否 |
| **UI-4** | P2 | `ui/src/components/models/TraceDetailModal.vue:11-16,60-108` | ① 手写 `pending`/`error`/`empty` `<p>` 段落而非项目统一三态容器 `SbAsyncRegion`；② `spanKind !== 'tool'` 与 `v-else` 复制了两套完全同构的 `<details>` IO 折叠区（48 行） → 合并为单一 `<details>`（按 `span.spanKind === 'tool'` 计算 label），三态接入 `SbAsyncRegion` 并补齐 `TraceDetailModal` / `TraceIOBlock` 单测 | 对齐 `ui/AGENTS.md` 三态规范，消除模板重复并抬升函数覆盖率（当前总基线 94.24% < 95%） | 低 | 已核实 | 否 |
| **CLN-2** | P3 | 全仓 395 个 Go 文件顶部的自动生成注释块（`// 文件说明：... // 调用架构 ... // 主要符号`）与 24 个未 `gofmt` 文件 | 约 10,000+ 行机械生成的头部调用图注释与 `go doc`/`outline` 完全重复，且多处因同名方法匹配错误（如把 `sql.Rows.Next`/`io.Closer.Close` 算作业务跨文件调用）产生误导；另有 24 个近期改动文件未过 `gofmt` → **随触随清（Boy Scout）或按包分批**清理误导性自动头部横幅，仅保留真正表达模块职责与「为什么」的包注释，并对已改动文件执行 `gofmt` | 大幅降低阅读噪音与 AI 上下文 token 开销（`rules.md` §7, §13, §18） | 低 | 已核实（`grep` 395 处、`gofmt -l` 24 处） | 分批策略见 §3.1 |
| **CLN-3** | P3 | `gocloudagent/eino-readme.md` (107KB)、`gocloudagent/pi-readme.md` (85KB) | 模块根目录放置两份大型上游参考文档（共 192KB），而模块自身缺少 `gocloudagent/README.md` → 归档至 `docs/` 参考目录或补齐精简 `gocloudagent/README.md` 说明模块边界 | 让 `gocloudagent/` 与 `godatabase/`、`gosandbox/`、`gofunction/` 目录规范一致 | 低 | 已核实 | 移动追踪文档需确认 |

---

## 3. 分模块详细清理与重构方案（行为不变）

### 3.1 仓库卫生与跨模块共性清理（`CLN-1` ~ `CLN-3`）

1. **清理未追踪编译产物（`CLN-1`，P1）**
   - **现状证据**：`gofunction/main`（15.0MB）与 `gosandbox/main_other`（2.4MB）残留在模块源码根目录，`git ls-files` 确认均未被 Git 追踪（系本地 `go build` 未指定 `-o build/bin/...` 产生）。
   - **动作**：直接删除上述两个残留二进制文件；遵循 `AGENTS.md` 规范，二进制产物统一输出到 `build/bin/` 或临时目录。若用户同意更新 `.gitignore`，可补充 `/gofunction/main` 与 `/gosandbox/main_*` 规则。
2. **机械头部注释横幅与未格式化文件治理（`CLN-2`，P3）**
   - **现状证据**：全仓共 395 个 `.go` 文件顶部带有约 25–30 行的自动生成注释块（`// 文件说明`、`// 用法`、`// 调用架构`、`// 主要符号`）。抽查 `internal/api/gocloudagent_access.go:8` 与 `gosandbox/host/start_linux.go:9` 可见，生成器按方法名粗暴匹配，把 `Close()`、`Next()`、`ptmx` 等局部/标准库接口调用误标为跨文件关键引用，且 `[同包] ... 等 22 个` 随文件增删已失真，属于 `simplify-code/references/rules.md` §7 明确要求移除的「重述型噪音与失真注释」。同时 `gofmt -l` 检出 24 个文件（含 v6.5 刚改动的 `internal/api/enduser_handler.go`、`internal/systemdb/traces.go`、`godatabase/filedb/remote.go` 等）未格式化。
   - **动作**：严格遵守 `AGENTS.md`「不要一次性全仓重排，改动这些文件时顺手格式化」的规则——
     - **立即批次**：仅对当前 `git status` 已处于修改态（M）且在 `gofmt -l` 名单中的文件执行 `gofmt -w`，不触碰干净历史文件以防污染 git blame。
     - **随触随清（Boy Scout）**：在执行下述各模块任务卡触及对应 Go 文件时，同步移除顶部的机械调用图注释块（保留真正说明业务约束的包注释或文件级设计说明）。
3. **`gocloudagent` 根目录文档整理（`CLN-3`，P3）**
   - **现状证据**：`godatabase/README.md`、`gosandbox/README.md`、`gofunction/README.md` 均清晰定义了独立模块契约，唯独 `gocloudagent/` 缺少 `README.md`，却在模块根放置了 `eino-readme.md`（107KB）与 `pi-readme.md`（85KB）两份外部框架参考文档。
   - **动作**：新增精炼的 `gocloudagent/README.md`（对齐根 `AGENTS.md` 中 `agentcore`/`cli`/`tools` 分层与零根模块依赖约束）；在用户确认后将两份上游参考文档迁至非核心源码目录或删除。

---

### 3.2 `internal/` 宿主后端模块（`INT-1` ~ `INT-3`）

> `planv6.0` 已完成 `catalog` 双重缓存失效、`llmgateway.prepareRequest`、`systemdb` 同实体列常量、`config` S3/Observability 拆分、`sql_handler` 写门禁、`router` 连续块挂载及 `app.assembleDeps` 五阶段拆分。本轮聚焦新增与遗留热点。

1. **`INT-1`：`internal/api/gofunction_handler.go` 写前置守卫与源码校验去重（P2）**
   - **位置**：`Create`（242–314）、`Patch`（348–384）、`Delete`（387–410）、`CreateVersion`（450–520）、`ActivateVersion`（549–571）、`TestVersion`（592–665）。
   - **重复事实**：
     - 5 个写/试跑方法重复 `ProjectFromContext` → `RequestIDFromContext` → `!h.writable`（返回 `godatabase.ErrWriterUnavailable`）→ `h.store == nil`（返回 `systemdb.ErrUnavailable`）。
     - `Create`（264–306）与 `CreateVersion`（477–503）重复 `validateGoFunctionSource(req.Source)` → `h.validateSource(c, req.Source)` → `mapGoFunctionValidationError` → `activate := true; if req.Activate != nil { activate = *req.Activate }` 默认激活分支。
   - **简化方案**：
     - 提取包内私有方法 `(h *GoFunctionHandler) writableProject(c echo.Context, forbidAdmin bool) (ProjectContext, string, error)`：统一收口 `ProjectFromContext`、`!h.writable`、可选 `systemdb.IsAdminProject`（**注意等价性陷阱**：当前仅 `Create` 与 `CreateVersion` 显式带 `IsAdminProject` 403 守卫，`Patch`/`ActivateVersion`/`TestVersion` 未带；通过显式布尔或拆为 `writableProject` + `rejectAdminProject` 保持各路由原错误码，不在纯简化中暗改行为）。
     - 提取私有方法 `compileExports(c echo.Context, source, rid string) ([]string, error)` 与 `resolveActivate(flag *bool) bool`，供 `Create` 与 `CreateVersion` 共用。
   - **验证门**：`go test -count=1 ./internal/api -run TestGoFunction`。

2. **`INT-2`：`internal/app/app.go` 调度器 Context 懒初始化与 `NewWithRegistry` 连续段提取（P2）**
   - **位置**：`NewWithRegistry`（207–469，共 262 行）。
   - **重复事实**：第 326–328（沙盒 reaper）、352–354（cronScheduler）、380–382（backup scheduler）、391–393（kvSweeper）连续 4 次重复：
     ```go
     if a.schedulerCancel == nil {
         a.schedulerBaseCtx, a.schedulerCancel = context.WithCancel(context.Background())
     }
     ```
     同时沙盒驱动装配（301–335）与备份服务装配（358–385）各自是自包含的连续阶段。
   - **简化方案**：
     - 新增 `(a *App) ensureSchedulerCtx() context.Context` 统一懒创建并返回 `a.schedulerBaseCtx`，替换全部重复块。
     - 按 `planv6.0` B4 同等口径，将沙盒与终端桥装配（301–335）提为 `a.assembleSandbox(ctx, cfg, logger)`、备份装配（358–385）提为 `a.assembleBackup(cfg)`，严格保持 `registerCloser` 注册先后不变。
   - **验证门**：`go test -race -count=1 ./internal/app`。

3. **`INT-3`：消除浅层一行透传包装并补齐 `internal/utils` 复用（P3）**
   - **位置与动作**：
     - `internal/sandbox/paths.go:95`：`truncateUTF8` 仅有一行 `return strutil.TruncateValidUTF8(b, max)`，包内调用处直接改调 `strutil.TruncateValidUTF8` 并删除私有别名。
     - `internal/sbcli/dev.go:89`：`devSHA256` 仅有一行 `return hashx.SumString(text)`，同包调用处直接改调 `hashx.SumString`。
     - `internal/api/kv_commands.go:1423`：`sortStrings(s)` 仅有一行 `sort.Strings(s)`（Go 1.25 可直接 `slices.Sort(fields)`）；`kvBoolInt`（1412–1417）保留返回 `int` 或对齐说明（JSON 序列化 `int` 与 `int64` 在 `any` 切片中完全一致，但单测若断言 Go 具体动态类型 `int` 则须保留 `int(sqlval.BoolInt(b))` 或维持原状——**必须在 `behavior-traps` 复核单测类型断言**）。
     - `internal/systemdb/traces.go:202-205`：`stream := int64(0); if t.Stream { stream = 1 }` 直接替换为同模块已有的 `sqlval.BoolInt(t.Stream)`（返回 `int64`，类型 100% 匹配）。
     - `internal/api/gocloudagent_access.go:52-68`：`NewCloudAgentDB` → `newCloudAgentDB`、`NewCloudAgentObj` → `newCloudAgentObj`、`NewCloudAgentLLM` → `newLLMChatClient` 若包内无第二个调用方，合并进导出构造函数，减少一层无意义跳转。

---

### 3.3 `godatabase/` 独立数据库运行时模块（`GDB-1` ~ `GDB-2`）

1. **`GDB-1`：收敛重复 SQL 字符串字面量转义到 `godatabase/sqlx`（P2）**
   - **位置**：`godatabase/filedb/remote.go:490`（`func sqlQuote(s string) string`）与 `godatabase/dialect.go:168`（`func sqlLiteral(s string) string`）。
   - **现状证据**：两处均为 `return "'" + strings.ReplaceAll(s, "'", "''") + "'"`，与 `godatabase/sqlx/sqlx.go:41` 的 `sqlx.Literal(s)` 逐字节一致。根据 `godatabase/README.md` 依赖方向（`filedb → blob → godatabase（根）→ sqlx`），`godatabase` 根包与 `filedb` 子包均可合法直接 import `github.com/linkxzhou/SimpleBase/godatabase/sqlx` 而不成环。
   - **动作**：将 `remote.go:472` 的 `sqlQuote(target)` 与 `dialect.go` 的 `sqlLiteral(...)` 替换为 `sqlx.Literal(...)`，删除两处私有重复函数。
   - **验证门**：`(cd godatabase && GOWORK=off go test ./ ./filedb/... ./sqlx/...)`。

2. **`GDB-2`：`godatabase/kv` 元数据扫描与 `IN (?,...)` 占位符去重（P2）**
   - **位置**：
     - `godatabase/kv/key.go:47-81`：`getMeta` 与 `getMetaAny` 除了 SQL 是否追加 `AND (etime IS NULL OR etime > ?)` 外，6 个字段的 `Scan`、`sql.ErrNoRows → ErrNotFound`、错误包装前缀 `"kv: get key meta: %w"` 及 `TypeID(typ)` 转换完全相同。
     - `godatabase/kv/{key,string,set,hash,zset}.go`：共 7 处重复构造 `ph := make([]string, len(xs))` + 循环填 `"?"` + `strings.Join(ph, ",")`（其中 `set.go:236` 已局部写过占位符辅助函数）。
   - **简化方案**：
     - 在 `godatabase/kv/key.go` 提取 `scanKeyMeta(row *sql.Row) (KeyMeta, error)`，由 `getMeta` 与 `getMetaAny` 共享。
     - 在 `godatabase/kv/` 提取包内纯函数 `placeholders(n int) string`（例如 `n <= 0` 返回 `""`，`n == 1` 返回 `"?"`，其余用 `strings.Repeat("?,", n-1) + "?"`，零切片分配），替换 5 个文件中的 7 处手工 `ph` 切片循环。
   - **等价性复核**：所有调用点在 `len == 0` 时均已提前返回（如 `GetMany` 68 行、`Delete` 99 行）；新 `placeholders(n)` 输出字符串与原 `strings.Join(ph, ",")` 对任意 `n >= 1` 逐字符恒等。
   - **验证门**：`(cd godatabase && GOWORK=off go test -race ./kv/...)`。

---

### 3.4 `gosandbox/` 独立沙盒执行模块（`SBX-1` ~ `SBX-3`）

1. **`SBX-1`：上提 Linux / Darwin 完全重复的长会话 POSIX 脚手架到 `start_posix.go`（P1）**
   - **位置**：`gosandbox/host/start_linux.go:88-177` 与 `gosandbox/host/start_darwin.go:62-130`；同目录已有 `start_posix.go`（`//go:build linux || darwin`）。
   - **重复事实**：
     - `type ptyWriter struct{ f *os.File }` 及其 `Write`、`Close` 方法在两文件逐字重复。
     - `func (p *process) Resize(size PTYSize) error` 在两文件逐字重复。
     - 非 PTY 分支的双管道创建（`pipeR/pipeW` + `inR/inW`）、失败回滚关闭、`releasers` 注册及父端 `closeParentFDs` 完全一致。
     - `ctx.Done()` 监听并向进程组发送 `syscall.Kill(-pid, syscall.SIGKILL)` 的守护 goroutine 在两文件逐字重复。
   - **简化方案**：
     - 将 `ptyWriter`、`(*process).Resize`、`attachStdio(c *exec.Cmd, pty *PTYSize, setCTTYAttr bool)`（注意等价性边界：**Linux 的 PTY 控制终端由 bwrap `--new-session` + helper `--ctty` 完成，`SysProcAttr` 保持 `Setpgid: true` 且不能设 `Setsid`；Darwin 的 PTY 分支则需要 `Setsid: true, Setctty: true, Ctty: 0`**——必须通过平台参数保留该精确差异！）以及 `watchContextKillPG(ctx, pid, proc.Done())` 迁入 `start_posix.go`。
   - **验证门**：macOS 本机跑 `(cd gosandbox && go build ./... && go vet ./... && go test -race ./...)`，同时用 `GOOS=linux GOARCH=amd64 go build ./...` 与 `GOOS=linux go test -c ./host` 交叉编译校验 Linux 分支无编译退化（遵循 `assessment.md` 跨平台验证准则）。

2. **`SBX-2`：收敛 `gosandbox/host` Linux `Run` 与 `Start` 的 bwrap + cgroup 准备逻辑（P2）**
   - **位置**：`gosandbox/host/run_linux.go:53-117` 与 `gosandbox/host/start_linux.go:46-87,125-136`。
   - **简化方案**：在 `run_linux.go`（或新建 `prepare_linux.go`，`//go:build linux`）提取私有 `prepareBwrapCmd(ctx context.Context, p Policy, cmd Command, ctty bool) (*exec.Cmd, *os.File, Caps, error)`（含 `Isolates`、`Probe`、`resolveHelper`、空 `argv`、默认 `Dir`、`BwrapArgs`、策略 JSON 匿名管道创建写入）与 `prepareCgroup(p Policy, caps Caps) (*cgroupGroup, error)`。调用方各自持有 `defer pr.Close()` 与进程启动后的等待/托管逻辑，生命周期完全等价。

3. **`SBX-3`：收敛 `gosandbox/vm/local/local.go` 中 `Exec` 与 `Start` 的命令及环境构造（P2）**
   - **位置**：`Exec`（312–352）与 `Start`（581–629）。
   - **简化方案**：提取私有 `resolveExecArgv(shell, cmd string, args []string) (argv []string, missingBin string)` 与 `(d *Driver) buildEnv(ws string, extra map[string]string) []string`。
   - **等价性保护**：当 `missingBin != ""` 时，`Exec` 仍返回 `vm.ExecResult{Stderr: []byte(bin + ": command not found\n"), ExitCode: 127}, nil`（非 error），而 `Start` 仍返回 `nil, d.fail("start", t.Name, fmt.Errorf("%s: command not found", bin))`；当 `shell == "" && cmd == ""` 时，仅 `Start` 回退 `[]string{"/bin/sh", "-i"}`。

---

### 3.5 `gocloudagent/` 与 `gofunction/` 独立模块（`GCA-1`、`GFN-1`）

1. **`GCA-1`：`gocloudagent` 门控通道去重与薄包装内联（P2）**
   - **位置**：`gocloudagent/cli_tool.go:87-135`（`callGate.wait` / `callGate.release`）与 `gocloudagent/runtime.go:630-632`（`consumeVariant`）。
   - **简化方案**：
     - 在 `callGate` 提取私有 `(g *callGate) slot(id string) chan struct{}` 收敛互斥锁下的 `g.ch` map 初始化与 channel 获取/创建；`wait` 保留 `defer` 删除登记，`release` 保留非阻塞 `close(ch)`。
     - 删除 `runtime.go:630` 无额外逻辑的 `consumeVariant` 包装，第 298 行直接调用 `agentcore.ConsumeVariant(mv)`（与 `cli/runner.go:207` 保持一致）。`consumeAssistant` 因桥接 `func(Event)` 到 `func(agentcore.ConsumeEvent)` 具有真实适配价值，予以保留。
   - **验证门**：`(cd gocloudagent && GOWORK=off go test -race ./...)`。

2. **`GFN-1`：`gofunction` 标准库排序替换与编译辅助微调（P3）**
   - **位置**：`gofunction/internal/interp/jsonrun.go:129-133`。
   - **现状证据**：`ValidateHTTPFuncs` 末尾手写 5 行双层 `for` 插入排序按 `FuncInfo.Name` 升序排列；`gofunction/go.mod` 已声明 Go 1.25。
   - **简化方案**：替换为 `slices.SortFunc(infos, func(a, b FuncInfo) int { return cmp.Compare(a.Name, b.Name) })`。由于 `infos` 来自同一 `mainPkg.Members` map，函数名天然唯一，不存在相等键稳定性差异（通过 `behavior-traps.md` 排序稳定性门禁）。
   - **验证门**：`(cd gofunction && go test ./... -short && go vet ./...)`。

---

### 3.6 `ui/src/` 前端控制台模块（`UI-1` ~ `UI-4`）

1. **`UI-1`：清理已退役的 `AgentScheduleModal` 死组件及不可达回调链（P1）**
   - **现状证据**：
     - 后端 `/v1/projects/:projectID/agent-schedules` 的写接口（`POST` / `PATCH` / `DELETE` / `POST .../run`）已于 v6.0 全部冻结为 `410 Gone`（迁移至 `/cron-jobs`）。
     - `ui/src/pages/AgentManager.vue:600` 的 `openSchedule(a)` 已改为 `router.push({ name: 'cron-jobs', query: { agent: a.id } })`。
     - 全仓检索确认 `AgentManager.vue:303` 的 `scheduleModalOpen` 初始为 `ref(false)`，**没有任何代码路径将其置为 `true`**；因此模板第 179–188 行 `<AgentScheduleModal ... />`、脚本第 303–304 行 `scheduleModalOpen`/`scheduleAgent`、第 604–619 行 `onScheduleSaved`/`onScheduleRemoved`/`onViewScheduleThread`，以及整个 `ui/src/components/ai/AgentScheduleModal.vue`（332 行）在运行时 100% 不可达。
     - 这些永不执行的死函数直接拉低了前端 Vitest `functions` 覆盖率（根 `AGENTS.md` 记录当前基线为 **94.24%，低于 95% 门禁**）。
   - **清理范围**：
     - 删除 `ui/src/components/ai/AgentScheduleModal.vue` 与孤立单测 `ui/tests/AgentScheduleModal.test.ts`。
     - 从 `AgentManager.vue` 移除 `<AgentScheduleModal>` 挂载、`scheduleModalOpen`、`scheduleAgent`、`onScheduleSaved`、`onScheduleRemoved`、`onViewScheduleThread`，同步移除 `ui/tests/AgentManager.test.ts` 中的 `AgentScheduleModal` stub。
     - **保留只读兼容部分**：`AgentManager.vue` 的 `loadSchedules()`（调 `api.agentSchedules.list`）与 `scheduleSummary()`，以及 `locales/{zh-CN,en-US}.ts` 中仍被 `scheduleSummary` 消费的 `preset15Min` / `presetHourly` / `presetDaily8` / `presetWeeklyMon8` / `enabled` / `disabled` 6 个词条必须保留（展示历史遗留只读排期摘要）；其余仅被 `AgentScheduleModal.vue` 消费的 20+ 个 `components.agentSchedule.*` 词条在两棵语言树同步删除（守住 `ui/tests/i18n.test.ts` 同构断言）。

2. **`UI-2`：下线零生产引用的 `useAsyncAction.ts` 与 `useAiChat.ts`，收敛 `ChatMsg` 到 `services/types.ts`（P1）**
   - **现状证据**：
     - `ui/src/composables/useAsyncAction.ts` 在 `ui/src/` 下为 **0 处引用**（仅 `ui/tests/useAsyncAction.test.ts` 自测）。
     - `ui/src/composables/useAiChat.ts` 的运行时函数 `useAiChat()` 在 `AiChat.vue` 下线后为 **0 处生产调用**；`AgentManager.vue`、`useAgentConversation.ts`、`ConversationView.vue` 三处仅写了 `import type { ChatMsg } from '.../useAiChat'`。
   - **清理方案**：
     - 将 `export interface ChatMsg` 原样迁入 `ui/src/services/types.ts`（符合 `ui/AGENTS.md`「跨层与共享契约集中在 `services/types.ts`」），更新上述 3 个源码文件与 4 个测试文件的 `import type` 路径。
     - 删除 `ui/src/composables/useAsyncAction.ts`、`ui/src/composables/useAiChat.ts` 及其孤立单测 `ui/tests/useAsyncAction.test.ts`、`ui/tests/useAiChat.test.ts`，同步更新 `ui/AGENTS.md` 与 `ui/README.md` 的目录说明。

3. **`UI-3`：消除 v6.5 新增的 `Record<string, any>` 与对象 `type` 违规（P2）**
   - **现状证据**：
     - 根 `AGENTS.md` 规定「不写 `any` 落盘新代码；`services/http-api.ts` 现存 35 处 `any`/`Record<string, any>`，只减不增；对象形状一律用 `interface` 定义，不用 `type`」。
     - v6.5 新增的 `toLLMTrace`（651 行）、`toLLMTraceListResult`（678、682 行）、`toLLMTraceGroup`（701 行）、`toLLMTraceGroupListResult`（723、727 行）及 `llmTraces.detail`（1889 行）新增了 **7 处 `Record<string, any>`**；`ui/src/components/models/TraceIOBlock.vue:74` 写了 `value: any`；`ui/src/docs/catalog.ts:23` 写了 `type MetaFile = { ... }`。
   - **简化方案**：
     - 在 `http-api.ts`（或 `types.ts`）为 LLM Trace / TraceGroup 响应补齐 `interface LLMTraceWire` 与 `interface LLMTraceGroupWire`（显式声明 snake/camel/Pascal 可选字段为 `unknown`，保持原 `String(d.x ?? '')`、`Number(d.n ?? 0)` 与 `|| undefined` 的 falsy/nullish 转换行为 **100% 不变**，严防 `behavior-traps.md` 中的运行时隐式类型收窄陷阱）。
     - `TraceIOBlock.vue:74` 将 `computed<{ ok: boolean; value: any }>` 改为 `interface ParsedPayload { ok: boolean; value: unknown }`，并在缩进序列化分支做窄化。
     - `ui/src/docs/catalog.ts:23` 将 `type MetaFile = { ... }` 改为 `interface MetaFile { ... }`。

4. **`UI-4`：`TraceDetailModal.vue` 模板去重、三态标准化与测试补齐（P2）**
   - **位置**：`ui/src/components/models/TraceDetailModal.vue:11-16,60-108`。
   - **简化方案**：
     - 合并第 60–86 行（`v-if="span.spanKind !== 'tool'"`）与第 87–108 行（`v-else`）两段重复的 `<details>` 折叠块为单一 `<details>`：`<TraceIOBlock v-if="span.inputJson" :label="t(span.spanKind === 'tool' ? 'pages.traces.detail.arguments' : 'pages.traces.detail.input')" ... />`、`<TraceIOBlock v-if="span.spanKind !== 'tool' && span.toolsJson" ... />`、`<TraceIOBlock v-if="span.outputJson" :label="t(span.spanKind === 'tool' ? 'pages.traces.detail.result' : 'pages.traces.detail.output')" ... />` 及共用的 `errorMessage` 展示，净省 ~26 行重复模板。
     - 将手写的 `pending` / `error` / `empty` 裸 `<p>` 替换为项目统一的 `SbAsyncRegion` + `useLoadState`（遵循 `ui/AGENTS.md`「错误态只有一个实现 `SbErrorState`，不得手写错误段落」）。
     - 为当前 `test_refs=0` 的 `TraceDetailModal.vue`、`TraceIOBlock.vue`、`AgentMarkdown.vue` 补齐真实 DOM 单测（不桩掉 `ui/` 基础组件），协助前端 `functions` 覆盖率回到 ≥95% 门禁之上。

---

## 4. 明确保持原状与拒绝改写的项（`Not changed`）

依据 `simplify-code` Golden Rules 4–5 与 `behavior-traps.md`，以下看似重复或冗长的代码经逐项核查**明确不予改写**：

1. **跨独立模块的截断与输出限制副本不合并**：`gocloudagent/agentcore/redact.go`（`Truncate` 按 rune 截断并追加标记）、`gocloudagent/tools/tools.go`（`truncateText` 按字节截断）、`gocloudagent/cli_runner.go`（`limitOut`）与根模块 `internal/utils/strutil` 物理隔离。强行抽出第五个共享模块收益远小于维护多模块 `replace` 的成本（已在 `internal/utils/README.md` 备案）。
2. **`godatabase/ducklake/factory.go:502` 的 `filepathToSlash` 不替换为标准库 `filepath.ToSlash`**：
   - **行为陷阱证明**：在 Linux/macOS 上 `os.PathSeparator == '/'`，标准库 `filepath.ToSlash("a\\b")` 会原样返回 `"a\\b"`，而现有 `strings.ReplaceAll(p, "\\", "/")` 会无条件把反斜杠替换为正斜杠。若改为 `filepath.ToSlash`，非 Windows 宿主处理带 `\` 的路径参数时行为会发生静默变化。仅可重命名为意图更清晰的 `backslashToSlash`（P3 可选），绝不改为 `filepath.ToSlash`。
3. **`gocloudagent/runtime.go:StartRun` 与 `gocloudagent/cli/runner.go:Run` 的事件循环不抽通用框架**：
   - 两者虽同调 `adk.NewRunner`，但云端 `StartRun` 处理 `callGate` 释放、4000 rune 工具卡片聚合、不落盘截断与 trace 埋点；本地 `cli/runner` 处理 `Compact`、JSONL 先写 tool_call 后写 tool_result 的严格落盘顺序、脱敏不截断与 `repeatDetector`。强行抽象需要注入 7+ 个回调钩子，属于典型的浅模块过度设计（`rules.md` §3, §6）。
4. **`gofunction` 预留空方法不删除**：`Executor.Compile`、`Runtime.Cleanup` 等虽为空实现，但属于独立模块对外公开签名且在 `gofunction/README.md` 明确登记为预留扩展点，按 Golden Rule 2 保持不动。
5. **`gofunction/testdata/` 同类样本不合并**：`controlflow.go`（用法样本，单测按文件名枚举）与 `flowcontrol.go`（SSA 对拍集）职责互补，已在 `gofunction/README.md` 明确说明不合并。
6. **`internal/api/kv_commands.go`（1425 行）命令表不拆碎**：文件虽长，但主体是扁平、同构的 Redis 命令路由表（`kvCmdSpec` 切片），每个命令闭包 10–25 行且内聚无嵌套，强行拆成十几个碎片文件反而增加跨文件跳转成本。

---

## 5. 需用户确认的契约 / 架构对齐项（`Needs approval`）

以下各项涉及导出接口调整或架构红线取舍，**默认不自动执行**，待用户逐项确认后独立提交：

| 编号 | 位置 | 提案内容 | 为什么需要单独审批（受影响的输入/契约） |
| --- | --- | --- | --- |
| **APV-1** | `internal/systemdb/agent_schedules.go`（83–334 行） | 删除 8 个零生产调用的导出写方法：`CreateAgentSchedule`、`GetAgentScheduleByAgent`、`UpdateAgentSchedule`、`ArchiveAgentSchedule`、`ListDueAgentSchedules`、`ClaimAgentSchedule`、`InsertAgentScheduleRun`、`UpdateAgentScheduleRunStatus`，仅保留 `agentScheduleHandler` 实际调用的 3 个只读查询方法（`ListAgentSchedules`、`GetAgentSchedule`、`ListAgentScheduleRuns`） | 虽无任何外部/生产调用方，但属于 `systemdb.Store` 的导出方法集，删除后需同步精简 `agent_history_test.go:511-637` 的写路径测试。 |
| **APV-2** | `internal/systemdb/schema.go:622,641-657`（`ensureTraceColumns`） | 评估移除启动期 `ALTER TABLE sys_llm_traces ADD COLUMN ...` 兜底逻辑，保留 `schema.go` 最终态 `CREATE TABLE` 定义 | `internal/AGENTS.md` 第 30 行明确规定：`systemdb` **禁止新增版本化迁移或 `ALTER` 补列：改表结构直接改 `schema.go` 并 reset**。但移除后，若本地存在 v6.5 一期未 reset 的旧 `data/_system` 库，启动查询新列会报错（属于对未 reset 旧开发环境的可观察行为变化）。 |
| **APV-3** | `ui/src/services/types.ts` & `http-api.ts` | 从前端 `ApiClient` 接口中一并移除零生产调用的：① `agentSchedules.{create,patch,remove,runs,trigger}`；② `llmTraces.list`（已被二期 `groups` + `detail` 取代）；③ `llm.{chat,stream}`（`useAiChat` 下线后前端无任何页面调用，仅单测桩调用） | 改动 `ui/src/services/types.ts` 导出的 `ApiClient` 契约及 `ui/src/test/api-mock.ts`、`ui/tests/http-api.test.ts` 对应单测；不影响后端 HTTP 路由（后端 OpenAI 兼容出口与 `GET /llm/traces` 保持原样）。 |
| **APV-4** | `.gitignore` & `gocloudagent/{eino,pi}-readme.md` | ① 在 `.gitignore` 增加 `/gofunction/main`、`/gosandbox/main_*`；② 将 `gocloudagent/eino-readme.md`、`pi-readme.md` 移入 `docs/` 参考区或移除 | 触及根目录 `.gitignore`（受 `AGENTS.md` 硬约束 #4 保护）及已纳入 Git 追踪的文档路径。 |

---

## 6. 顺带发现的独立缺陷（`Bugs found — 独立修复，不混入简化`）

遵循 `simplify-code` Golden Rule 1 与 `assessment.md` 规范，静态评估中确认的潜在缺陷单独登记如下，建议另立 bugfix 提交修复并配回归测试：

- **`BUG-APP-01`（已确认推理链）：`internal/app/app.go:373-379` 装配失败路径泄漏已启动资源与后台协程**
  - **位置**：`NewWithRegistry` 第 373–379 行（`backup.NewScheduler` 错误分支）。
  - **现象与触发条件**：当 `cfg.Backup.Enabled == true` 且 `cfg.Backup.Schedule` 非空但表达式不合法（或 `NewScheduler` 返回错误）时，`NewWithRegistry` 直接执行 `return nil, fmt.Errorf("backup schedule: %w", err)`，**漏调了 `_ = a.Close(context.Background())`**。
  - **后果**：此前在 `assembleDeps`（230 行）已打开的 `systemStore`、`registry`、`writeLease`（分布式写租约心跳）、第 329 行已启动的 `a.sandboxMgr.StartReaper` 协程、以及第 355 行已启动的 `cronScheduler.Start` 协程全部泄漏未关闭（对比第 231 行 `assembleDeps` 失败时有 `_ = a.Close(context.Background())`）。
  - **独立修复建议**：在 `NewWithRegistry` 入口成功创建 `a` 后使用具名返回值或失败守卫 `ok := false; defer func() { if !ok { _ = a.Close(context.Background()) } }()`，覆盖 `assembleDeps` 之后的全部错误返回分支，并补 `TestNewWithRegistry_InvalidBackupScheduleClosesResources` 单测。

---

## 7. 分批实施路线与验证矩阵

### 7.1 推荐实施批次（单次一卡，小步可回退）

1. **Batch 1（零风险清理与前端死代码瘦身）**：`CLN-1`（删残留二进制）→ `UI-1`（删 `AgentScheduleModal` 死链）→ `UI-2`（迁 `ChatMsg`、删 `useAsyncAction`/`useAiChat`）→ `UI-3` & `UI-4`（`TraceDetailModal` 去重、三态统一、消新 `any` 并补单测）。
2. **Batch 2（独立模块内部收敛）**：`GDB-1`（`sqlx.Literal` 统一）→ `GDB-2`（`kv` 扫描与占位符去重）→ `SBX-1` & `SBX-2` & `SBX-3`（`gosandbox` POSIX 上提、Linux 准备提取、`vm/local` 去重）→ `GCA-1` & `GFN-1`（`callGate.slot`、`slices.SortFunc`）。
3. **Batch 3（宿主后端局部提炼与随触格式化）**：`INT-1`（`gofunction_handler` 守卫与编译提取）→ `INT-2`（`app.ensureSchedulerCtx` 与阶段提取）→ `INT-3`（内联单行透传包装 + `sqlval.BoolInt`）→ `CLN-2`（对本轮触及的 Go 文件清理失真自动头部注释并执行 `gofmt`）。
4. **Batch 4（待用户确认后执行）**：按用户审批结果推进 `APV-1` ~ `APV-4` 及独立修复 `BUG-APP-01`。

### 7.2 每张任务卡的强制六步门禁

1. **核对工作区**：执行 `git status --short -- <目标文件>` 与 `git diff -- <目标文件>`，不覆盖工作区未提交的并行修改。
2. **建立表征基线**：运行目标包现有测试，确认输入/输出/错误码/副作用基线；缺少覆盖的分支先补表征单测。
3. **单步等价变换**：一次只做一类重构（删除死代码 / 提取同包私有函数 / 内联薄包装），不在同一提交夹带行为变更。
4. **`behavior-traps.md` 逐行复核**：重点核查 Go typed-nil interface、`nil` vs `[]` JSON 编码、`defer` 作用域、锁内外顺序、跨平台条件编译标签，以及 TS `||` vs `??`、响应式 `ref` 身份。
5. **运行模块级验证命令**（见 7.3）。
6. **诚实记录结果**：报告实际运行的命令、退出码及跳过项。

### 7.3 验证状态与实施期必跑命令清单

- **本次评估阶段实际已执行的只读检查**：
  - `git rev-parse --short HEAD`（`726621e`）与 `git status --short`（确认 116 项工作区状态）；
  - `git ls-files --error-unmatch gofunction/main gosandbox/main_other ...`（确认两处残留二进制未入库）；
  - `gofmt -l internal godatabase gocloudagent gofunction gosandbox cmd`（检出 24 个未格式化 Go 文件）；
  - 全仓符号/引用静态扫描（确认 `AgentScheduleModal`、`useAsyncAction`、`useAiChat`、`Store.*AgentSchedule*` 写方法、`sqlQuote`/`sqlLiteral`、`start_linux`/`start_darwin` 重复段及 395 处头部生成注释）。
- **本次评估阶段未运行的构建/测试命令（因当前为只读评估模式，未触发重型编译与测试写盘）**：实施对应批次时须逐条运行并核对退出码：

```bash
# 1. 宿主后端（Batch 3 / APV-1 / APV-2 / BUG-APP-01）
go build ./internal/... ./cmd/...
go vet ./internal/... ./cmd/...
go test -race -count=1 ./internal/api ./internal/app ./internal/sandbox ./internal/systemdb ./internal/sbcli

# 2. godatabase 独立模块（Batch 2: GDB-1, GDB-2）
(cd godatabase && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test -race ./ ./filedb/... ./kv/... ./sqlx/...)

# 3. gosandbox 独立模块 + 跨平台编译门禁（Batch 2: SBX-1 ~ SBX-3）
(cd gosandbox && go build ./... && go vet ./... && go test -race ./...)
(cd gosandbox && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./host/... ./vm/... ./cmd/...)

# 4. gocloudagent 独立模块（Batch 2: GCA-1）
(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test -race ./...)

# 5. gofunction 独立模块（Batch 2: GFN-1）
(cd gofunction && go test ./... -short && go vet ./...)

# 6. 前端控制台（Batch 1: UI-1 ~ UI-4 / APV-3）
cd ui && yarn build && yarn test
```


---

## 8. 实施交付与验证记录（2026-11-13）

### 8.1 已完成任务卡（Batch 1–3）

| 任务卡 | 状态 | 实际交付摘要 |
| --- | --- | --- |
| **CLN-1** | ✅ 已验证 | 删除未追踪编译残留 `gofunction/main`（15.0MB）与 `gosandbox/main_other`（2.4MB）。 |
| **UI-1** | ✅ 已验证 | 删除不可达死组件 `ui/src/components/ai/AgentScheduleModal.vue`（332 行）及 `ui/tests/AgentScheduleModal.test.ts`；清理 `AgentManager.vue` 中 `scheduleModalOpen`/`scheduleAgent` 与 3 个死回调；双语语言树同步移除 29 个仅该弹窗使用的词条（保留只读摘要使用的 6 个词条）。 |
| **UI-2** | ✅ 已验证 | 将共享 `ChatMsg` 接口迁入 `ui/src/services/types.ts` 并更新 6 处类型导入；下线零生产调用的 `useAsyncAction.ts`、`useAiChat.ts` 及其孤立单测；同步更新 `ui/AGENTS.md` 与 `ui/README.md`。 |
| **UI-3** | ✅ 已验证 | 在 `ui/src/services/http-api.ts` 新增 `LLMTraceWire` / `LLMTraceListWire` / `LLMTraceGroupWire` / `LLMTraceGroupListWire` 消除 7 处新增 `Record<string, any>`；`TraceIOBlock.vue` 改用 `unknown`；`docs/catalog.ts` 的 `MetaFile` 由 `type` 改为 `interface`。 |
| **UI-4** | ✅ 已验证 | 合并 `TraceDetailModal.vue` 两份同构 `<details>` 折叠模板，接入 `SbAsyncRegion` + `useLoadState` 统一三态（支持错误重试与空 traceId 重置），并在 `ui/tests/ModelTraces.test.ts` 补齐 `TraceDetailModal` 与 `TraceIOBlock` 单测。 |
| **GDB-1** | ✅ 已验证 | `godatabase/dialect.go`（`QuoteIdent`、`TableColumnsSQL`）与 `godatabase/filedb/remote.go`（`snapshot`）统一调用 `godatabase/sqlx.Ident` / `sqlx.Literal`，移除重复的 `sqlLiteral` 与 `sqlQuote`。 |
| **GDB-2** | ✅ 已验证 | `godatabase/kv/key.go` 提取 `scanKeyMeta` 共用行扫描；`set.go` 的 `placeholders(n)` 改为零切片分配实现，并替换 `key.go`/`string.go`/`set.go`/`hash.go`/`zset.go` 中其余 6 处手工 `IN (?,...)` 构造。 |
| **SBX-1** | ✅ 已验证 | 将 `ptyWriter`、`(*process).Resize`、`attachStdio`、`watchContextKillGroup`、`closeAll` 上提至 `gosandbox/host/start_posix.go`，消除 `start_linux.go` 与 `start_darwin.go` 重复实现（通过 `setDarwinAttr` 严格保留 Linux bwrap 与 macOS Seatbelt 的 CTTY `SysProcAttr` 差异）。 |
| **SBX-2** | ✅ 已验证 | 在 `gosandbox/host/run_linux.go` 提取 `prepareBwrapCmd` 与 `prepareCgroup`，由 Linux `Run` 与 `Start` 共用。 |
| **SBX-3** | ✅ 已验证 | 在 `gosandbox/vm/local/local.go` 提取 `resolveCommandArgv`（保持空串 `""` 边界在 `Exec` 返回 127、`Start` 走 `/bin/sh -i` 的原语义）与 `commandEnv`。 |
| **GCA-1 / CLN-3** | ✅ 已验证 | `gocloudagent/cli_tool.go` 提取 `callGate.slot(id)`；`gocloudagent/runtime.go` 内联一行包装 `consumeVariant`（同步更新 `coverage_test.go`）；新增 `gocloudagent/README.md` 说明模块边界（跟踪中的上游参考文档按 `APV-4` 待确认）。 |
| **GFN-1** | ✅ 已验证 | `gofunction/internal/interp/jsonrun.go` 的 `ValidateHTTPFuncs` 用 `slices.SortFunc` + `cmp.Compare` 替换手写双层插入排序。 |
| **INT-1** | ✅ 已验证 | `internal/api/gofunction_handler.go` 提取 `requireReadStore`、`requireWriteStore`（按端点精确保留 `forbidAdmin` 原行为）、`compileExports`、`shouldActivate`、`parseVersionParam`。 |
| **INT-2** | ✅ 已验证 | `internal/app/app.go` 提取 `ensureSchedulerCtx`（收敛 5 处重复懒初始化）、`assembleSandbox`、`assembleBackup`（保持 `BUG-APP-01` 原行为不混入纯简化）。 |
| **INT-3** | ✅ 已验证 | 内联 `sandbox.truncateUTF8`、`sbcli.devSHA256`、`api.sortStrings`、`api.newCloudAgentDB/Obj/newLLMChatClient`；`api.hashScanReply` 复用 `hashFlatReply`；`systemdb/traces.go` 复用 `sqlval.BoolInt`。 |
| **CLN-2** | ✅ 已验证 | 移除本轮全部触及 Go 文件顶部的失真自动调用图注释横幅，并对工作区所有处于修改态（dirty）的未格式化 Go 文件执行 `gofmt -w`（14 个未触碰的历史干净文件保持不动以免制造无关 blame 噪音）。 |

### 8.2 实际运行命令与验证结果

1. **前端构建与单测**
   - `cd ui && yarn build` → **通过**（`✓ built in 5.81s`）。
   - `cd ui && yarn test --run` → **117/118 文件通过，859/860 用例通过**（0 新增失败；唯一失败仍为基线已知的 `tests/docs-content.test.ts` 文档链接项 `getting-started/local-cloud-collab.md: /docs/cli`）。
   - `read_lints`（`AgentManager.vue`、`TraceDetailModal.vue`、`http-api.ts`）→ **0 error / 0 warning**。
2. **`godatabase` 模块**
   - `cd godatabase && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test -race -count=1 ./ ./filedb/... ./kv/... ./sqlx/...` → **全部通过**（exit 0；`ducklake` 与 `lease` 未改动，未跑慢速全量套件）。
3. **`gosandbox` 模块（含跨平台编译校验）**
   - `cd gosandbox && go build ./... && go vet ./... && go test -race -count=1 ./...` → **全部通过**（macOS host + vm + fake + local，exit 0）。
   - `cd gosandbox && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./... && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...` → **全部通过**（Linux 分支交叉编译与静态检查 0 告警，未在 Linux 内核真机运行 bwrap）。
4. **`gocloudagent` 与 `gofunction` 模块**
   - `cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test -race -count=1 ./...` → **全部通过**（exit 0）。
   - `cd gofunction && go build ./... && go vet ./... && go test -race ./... -short -count=1` → **全部通过**（exit 0；`-short` 跳过外网 HTTP 用例）。
5. **宿主后端 `internal/` 与 CLI 对拍**
   - `go build ./internal/... ./cmd/... && go vet ./internal/... ./cmd/...` → **全部通过**（exit 0）。
   - `go test -race -count=1 ./internal/api ./internal/app ./internal/sandbox ./internal/systemdb ./internal/sbcli` → **全部通过**（exit 0）。
   - `cd skills/simplebase && python3 -m unittest discover -s scripts -p 'test_sbdev.py'` → **29 tests OK**（exit 0）。
   - `git diff --check` → **通过**（无尾随空白或冲突标记）。


---

## 9. 完成度复核与站点发布端到端（2026-11-14）

### 9.1 逐项完成度

| 项 | 状态 | 说明 |
| --- | --- | --- |
| CLN-1~3、UI-1~4、GDB-1~2、SBX-1~3、GCA-1、GFN-1、INT-1~3 | ✅ | 复核：残留二进制已不存在；`AgentScheduleModal` / `useAsyncAction` / `useAiChat` / `sqlQuote` / `sqlLiteral` / `truncateUTF8` / `devSHA256` / `sortStrings` / `consumeVariant` / `newCloudAgentDB` 全仓 0 处残留 |
| BUG-APP-01 | ✅ 已修复 | `internal/app/app.go`：`assembleBackup` 失败时调用 `a.Close`；新增 `TestNewInvalidBackupScheduleClosesResources`（非法 cron 启动失败后同一 `data_dir` 可再次启动）。去掉修复时该测试失败（DuckLake catalog 仍被泄漏实例持有），恢复后通过 |
| APV-1~4 | ⏸ 待确认 | 删导出方法 / 移除 `ensureTraceColumns` / 精简 `ApiClient` / 改 `.gitignore` 与移动追踪文档，均需用户逐项批准 |

### 9.2 站点发布：返回子域名与 URL

`sbdev.py site deploy` 在原有「扫描密钥 → `simplebase object upload` 逐文件上传 → `PUT /s3/site` → `PUT /s3/public` → 探活」之后，固定输出：

```text
subdomain: <project_id>.<base_domain>
url:       <scheme>://<project_id>.<base_domain>[:port]/<prefix>
```

加 `--json` 时最后一行输出 `{"project","subdomain","base_url","url","prefix","files","http_status"}`，便于脚本/助手直接取用。新增 `SiteDeployTests`（上传产物、公开根目录、返回子域名与 URL、密钥拦截不上传）。

### 9.3 端到端实测（隔离实例，`sites.enabled=true base_domain=localhost url_port=18087`）

1. 首次改密 → 建项目 `site6501` → 签发 `database:read/write/admin` Key（只经请求头与 `.env` 传递）。
2. 用 `ui/node_modules/.bin/esbuild` 把 `src/main.js` 编译打包为 `dist/assets/app.js`，连同 `index.html`、`assets/app.css` 组成构建产物。
3. `sbdev.py site deploy --dist dist --json` 输出：`subdomain: site6501.localhost`、`url: http://site6501.localhost:18087/`，探活 HTTP 200；对象落盘 `site6501/files/{index.html, assets/app.js, assets/app.css}`。
4. 子域名响应：`/` → `200 text/html; charset=utf-8, no-cache`；`/assets/app.js` → `200 text/javascript; charset=utf-8, max-age=300`；`/assets/app.css` → `200 text/css`；未公开/不存在 → 404；控制台源直接请求 `/__site/index.html` → 404。
5. 无头 Chrome 打开 `http://site6501.localhost:18087/`：DOM 渲染为 `<h1>SimpleBase Site OK</h1><p id="js">js-executed …</p>`，确认编译后的 JS 在子域名上实际执行。

### 9.4 本轮实际运行的命令

- `go vet ./internal/app && go test -race -count=1 ./internal/app ./internal/api` → 通过。
- `cd skills/simplebase && python3 -m unittest discover -s scripts -p 'test_sbdev.py'` → 31 tests OK。
- 端到端脚本与无头浏览器验证见 9.3（临时实例与数据位于会话临时目录，不进入仓库）。
