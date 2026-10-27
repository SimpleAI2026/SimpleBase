# 后端代码简化与清理计划（v6.0）

> 范围：`internal/`；本计划只规划，不修改业务代码、配置、数据库或存储。
> 基线：`main` @ `5a03d47` + 2026-10-13 工作区未提交改动；实施前重新核对源码与差异。
> 依据：`skills/code-simplifier/code-simplifier.md`、`skills/simplify-code/SKILL.md` 及其 `references/rules.md`、`references/languages-tier1.md`（Go）、`references/behavior-traps.md`，以及 `AGENTS.md`、`internal/AGENTS.md`。
> 与 [`planv5.0/backend-optimization-v3-plan.md`](../planv5.0/backend-optimization-v3-plan.md) 的关系：本计划仅处理**行为不变的简化**，不重复其安全修复、竞态修复、迁移与解释器工作；两份计划命中同一文件时，先确保相关缺陷修复和回归测试稳定，再执行本计划。
> 证据口径：下列位置是静态阅读发现的**候选**，不是已完成重构或测试通过的宣称；行号以实施时的文件为准。

## 1. 目标与边界

目标是降低阅读和修改成本，而不是追求最少行数、微观性能收益或覆盖率数字。优先去除可证实的重复，按业务阶段降低长函数的认知负担；只有提取后接口更简单、职责更清楚时才拆分，避免制造转发型 helper、通用 CRUD 框架或跨模块依赖。

**保持不变**：导出符号与签名、HTTP 路由/权限/中间件顺序、错误码和错误文本、JSON/SSE/CLI 字段、SQL 列序和扫描顺序、缓存失效时机、资源释放顺序、锁范围与并发状态机、默认值与配置优先级、写入和持久化时序。行为修复与纯简化分开交付；发现旧行为有问题时先记录，另立修复任务，不混入本计划。

**后端硬边界**：系统库只能经 `systemdb.Store` 内部直连或只读桥接，不通过用户库 registry；`IsSystemDatabase` 拒写、`sqlguard` 验证、`writable` 双层门禁不省略；`app` 仍是唯一装配点并逆序关闭；对象键不拼接用户可控路径；审计和日志不泄漏参数、正文或密钥。配置仍只来自项目规定的 YAML 与 `SIMPLEBASE_*` 环境变量。不新增依赖，不改配置结构，不执行 `init_db`、生产/运行库 DDL/ALTER 或数据库结构变更，不新增迁移；已有测试在隔离临时库内的初始化仅按 §6.2 审计后执行。若评估发现确需迁移，仅提供方案和手动命令，由用户决定并亲自执行。

`code-simplifier.md` 中 React、ES 模块等规则不适用于 Go；其“只处理最近改动”的默认范围被本次用户明确指定的 `internal/` 覆盖，但**每批仍限制在少量相关函数和同一行为边界**。与 `simplify-code` 的“保留可观察行为、先建安全网、按风险小步推进”一致。

## 2. 优先级与候选清单

| 批次 | 位置（当前代码） | 具体简化动作 | 必须锁定的行为 / 测试入口 |
| --- | --- | --- | --- |
| A1 · 低风险 | `catalog/service.go:192-198`、`catalog/cache.go:150-158` | 建 KV 库成功后连续调用两次 `InvalidateDatabase`；确认 `dbCache`、`kvCache` 一次均失效后，只移除第二次调用。 | 建库和 KV 缓存可见性、失败后的 degraded 路径；`catalog/cache_test.go`、`catalog/service_test.go`。若存在并发/观测上依赖第二次失效的证据则不删。 |
  - 状态：**已完成（2026-10-13）**。删除门证据：两次调用之间零语句；`dbCache.put`/`kvCache.put` 仅在未命中回填时执行，创建期间该键无填充方；第二次删除与第一次完全同键（`projectID/databaseID` 与 `projectID`）；`SetDatabaseReady` 成功后 `InvalidateAllCatalogCache` 全量兜底；`InvalidateDatabase` 无返回值/日志/指标，删除无可观察差异。仅删除 KV 条件内第二次调用；既有表征测试 `a1_invalidate_test.go`（含 `kvQueries=2` 断言）删除后原样通过。验证：`go test -short/race -count=1 ./internal/catalog`。 |
| A2 · 低风险 | `llmgateway/service.go` 的 `Chat`、`Stream`、`resolveModel` | 当前已有 `resolveModel`，不要重复提取模型解析；`Chat` 与 `Stream` 仍重复取 client 和生成 `providers.Request`。只抽包内请求准备 helper，两种执行和用量记录流程保留。 | `getClient`/校验调用顺序、原错误文本、stream reader 的使用量记录时机；`llmgateway/coverage_test.go`、`agent_tools_test.go`。 |
  - 状态：**已完成（2026-10-13）**。提取私有 `prepareRequest`（原序 `getClient(ctx, projectID, "") → resolveModel → 构造请求`，不额外包装错误）；Chat 保持 `Response{}, err`、Stream 保持 `nil, err`，执行错误包装与用量记录时机不变。表征测试 `a2_request_prep_test.go`。验证：`go test -short -count=1 ./internal/llmgateway`。 |
| A3 · 低风险 | `systemdb/agents.go:121-155`；同类候选 `cron_jobs.go:106-150`、`agent_schedules.go:78-118` | 每次只整理**同一实体**多处重复的 SELECT 列表；先锁定字段顺序，再以同实体私有常量复用，避免跨表 SQL builder。 | `scanCloudAgent`/对应 scan 函数列序、归档过滤和排序；`systemdb/agent_history_test.go`、`cron_jobs_test.go` 及对应排期测试。不涉及 schema/迁移。 |
  - 状态：**已完成（2026-10-13）**。三个实体独立交付：`cloudAgentColumns`（agents.go）、`cronJobColumns`（cron_jobs.go）、`agentScheduleColumns`（agent_schedules.go）；逐查询替换并逐列复核 Scan 数量/顺序，跨实体零共享，WHERE/ORDER BY 未动。验证：`go test -short -count=1 ./internal/systemdb`。 |
| B1 · 中风险 | `config/yaml.go:243-410`、`config/env.go:11-147` | 对 `applyYAML` / `applyEnv` 按配置域做内部阶段提取；每次仅移动一个连续域，不引入反射自动映射。 | `config/config.go:327-364` 中默认值 → YAML → env → YAML 密钥拒绝 → Validate 的顺序；显式零值、别名、密钥拒绝；`config/config_more_test.go`、`sandbox_test.go`。 |
  - 状态：**已完成（2026-10-13）**。env 侧盘点：已有 `applyEnvAgent/Sandbox/LLM`，未重复造。YAML 侧提取两个连续域：`applyYAMLS3`（含 secrets 标记传递）与 `applyYAMLObservability`；语句原序移动，调用留在原位置。新增表征测试 `b1_config_domains_test.go`（nil/显式空串/非默认值/secrets 标记/yamlDur set 语义；期间修正一处错误断言——`setStr` 非 nil 即覆盖含空串）。验证：`go test -short/race -count=1 ./internal/config`。 |
| B2 · 中风险 | `api/sql_handler.go:198-370` | 先抽重复的 durability 选择；仅在表征测试齐全后考虑提取 `Execute`/`Batch` 完全相同的取库/系统库拒写段。`defer` 释放 lease 留在各 handler，不能抽取后立即释放。 | decode → `sqlguard.Validate` → GetDatabase → 系统库拒写 → semaphore → Acquire 的顺序；Batch 事务失败仍 HTTP 200 且 `batch_rolled_back`；`api/sql_handler_test.go`，新增 Execute/Batch 系统库写入不得 Acquire、只读实例拒写的表征测试。 |
  - 状态：**已完成（2026-10-13）**。B2-durability：提取 `durabilityFor`（nil 返回 `committed_local`，否则原样返回 callback 结果），替换 Execute 一处 + Batch 三处，回调次数/时机不变。B2-database：提取 `writableDatabase`（原连续 `GetDatabase → IsSystemDatabase` 段），project/principal/sqlguard/StageTimer/semaphore/Acquire/defer 全留 handler。表征测试 `b2_durability_test.go`、`b2_database_gate_test.go`。验证：`go test -short -count=1 ./internal/api`。 |
| B3 · 中风险 | `api/router.go:300-547` | 按现有**连续路由块**抽取私有挂载函数；保留原 `require`/`delegable`/`writeGate` 的顺序、依赖存在性和路由注册先后，不用 HTTP method 推断写权限。 | 从真实 `Routes()` 比较 method + path + middleware 实际授权行为；扩充 `api/write_gate_test.go`、`delegation_route_table_test.go`，覆盖可选依赖为 nil 时不挂载。特别保留 `POST /kv` 的 handler 内读写分类和 `POST /agent-threads/:threadID/runs` 的 `DatabaseRead + writeGate`。 |
  - 状态：**已完成（2026-10-13）**。四个连续块原序提取：`mountGoFunctionAndCronRoutes`、`mountAgentRoutes`、`mountSandboxRoutes`、`mountGoRoutes`（外层 `mountV1Routes` 为既有）；中间件参数顺序、nil 依赖不挂载语义、`POST /kv` handler 内读写分类均未动。表征测试 `b3_agent_routes_test.go`、`b3_sandbox_routes_test.go`、`b3_gofunction_cron_routes_test.go`。验证：`go test -short -count=1 ./internal/api`。 |
| B4 · 中风险 | `app/app.go:345-680`、`app/app.go:138-330` | 优先以现有业务阶段为单位提取 `assembleDeps` 的**连续片段**；之后再整理 `NewWithRegistry` 的 handler/deps 构建与 server 配置。不要一次拆整个 `app.go`。 | 只读提前返回、CAS/引擎检查在系统库 bootstrap 前、租约获取及失租回调、种子和后台任务的启动顺序、closer 逆序回收；`app/app_test.go`、`helpers_test.go`、`start_test.go`，补失败装配的释放与 CAS/失租回归。 |
  - 状态：**已完成（2026-10-13）**。五个业务阶段原序提取：`assembleLLMService`、`assembleFileStore`、`assembleWriteLease`、`assembleAuthServices`、`seedAndStartMaintenance`；closer 注册顺序、只读提前返回、CAS/引擎检查位置均未动。LLM 因 service 未暴露 resolver 访问器改用行为表征（`ProviderModels`/`ListProviders`）。表征测试 `b4_llm_assembly_test.go`、`b4_filestore_test.go`、`b4_lease_test.go`、`b4_auth_seed_test.go`。验证：`go test -short -count=1 ./internal/app`。 |
| C1 · 高风险，后置 | `api/agent_handler.go:544-710,778-909` | 先提纯验证/历史筛选，再考虑隔离 SSE 中持锁写事件段；不改 claim、run 落库、worker 关闭协议。 | retry 复用 user 消息、非最新 run 拒绝、claim 失败释放；事件 ID/缓存/输出同锁且结束前停 worker。`api/agent_e2e_usage_retry_test.go`、`agent_e2e_stream_test.go`、`run_events_lifecycle_test.go`，配合 race。与 v5 竞态修复完成状态先对齐。 |
  - 状态：**已完成表征与审计，零生产修改（2026-10-13）**。补 `c1_run_validation_test.go`（非最新 run 拒绝重试、retry 复用 user 消息等）；claim/run 落库/worker 协议未动。用户未选择进一步提取，按计划 C 类默认仅分析+表征。验证：`go test -short/race -count=1 ./internal/api`。 |
| C2 · 高风险，后置 | `sandbox/manager.go:213-342` | 按锁内幂等、输入归一化/校验、落库整理为私有逻辑；提取前先明确 `in` 的变更及 `Start` 在锁外执行。 | 项目锁覆盖配额检查与创建、幂等 TTL/容量淘汰、同名与资源错误、重用时 Start 语义；`sandbox/manager_lock_test.go` 加完整 Create 表征测试。 |
  - 状态：**已完成表征与审计，零生产修改（2026-10-13）**。补 `c2_create_test.go`（幂等命中跳过校验、同名优先于配额、归一化在锁内等审计结论固化）；锁/幂等/Start 语义未动。用户未选择进一步提取，按计划 C 类默认仅分析+表征。验证：`go test -short/race -count=1 ./internal/sandbox`。 |
| C3 · 高风险，后置 | `gocloudagent/runtime.go:104-316` | 在稳定的 SSE 事件测试基础上，仅按事件归集/结果生成阶段拆解 `StartRun`。 | 事件顺序、tool call/result 配对、取消和超时、截断、用量计数；`gocloudagent/runtime_test.go`、`run_limits_test.go`、`cli_tool_planv6_test.go`。不得改沙盒/CLI 信任边界。 |
  - 状态：**已完成表征与审计，零生产修改（2026-10-13）**。补 `c3_startrun_events_test.go`（usage 缺失、tool 失败、多次增量等事件轨迹）；channel 所有权/取消/超时未动。用户未选择进一步提取，按计划 C 类默认仅分析+表征。验证：`go test -short/race -count=1 ./internal/gocloudagent`。 |
| C4 · 高风险，最后评估 | `database/registry/registry.go:83-173`、`database/ducklake/catalog_syncer.go:525-763` | registry 先审计 `entry.openErr` 等状态的读写方；仅确认不可观察且无并发意义才删。syncer 仅在更强回归后考虑锁内捕获/锁外发布/水位推进的保序提取。 | 单 writer、失败重试、关闭/失租不 flush、快照 → manifest → 水位、CAS 冲突与 split-brain 判定；`registry/registry_test.go`、`always_open_test.go`、`ducklake/catalog_syncer_test.go`、`manifest_chain_test.go`、`write_gate_test.go`。缺测试或无法证明等价时保持原状。 |

**本轮不强行统一**：`api/kv_handler.go` 的 typed 写入与 `/kv` 命令写入（读命令共用 POST，且历史项目可在读取时补建 KV 行）；`objectstore` 的 S3/本地/内存 FileStore（默认列表上限、排序、错误语义不同）；`catalog/sql_repository.go` 的不同实体创建流程；两个 scheduler 的公开 Trigger 接口及锁约束。仅在后续有相同业务语义和充分测试时重新评估，不因代码形似即抽象。

### 2.1 候选与 skill 准则对照

下表引用仓库中的 [`code-simplifier.md`](../../skills/code-simplifier/code-simplifier.md)（简写 CS）和 [`simplify-code/SKILL.md`](../../skills/simplify-code/SKILL.md)（简写 SC）；`rules.md` 指 [`simplify-code/references/rules.md`](../../skills/simplify-code/references/rules.md)。CS §1–5 对应文件中的编号原则；SC 的 Golden rules、Workflow 和 `rules.md` §N 分别按原文件编号。**准则是判断方法，不是对具体代码改动的授权；每项仍以表征测试证明等价为前提。**

| 候选 | CS 准则 | SC 准则 | 对应理由与实施限制 |
| --- | --- | --- | --- |
| A1 缓存失效 | §3 消除冗余；§1 保持功能 | Golden rule 1；`rules.md` §6 去重 | 连续两次调用外观重复，但须先排除两次调用之间并发重新填充缓存造成的行为差异；不能证明时不删。 |
| A2 Chat/Stream 请求准备 | §3 合并相关逻辑；§4 防止过度简化 | `rules.md` §6 去重、§3 深模块；Workflow 4 小步重构 | 只共用真正相同的准备阶段，不引入比重复代码更复杂的 helper，也不合并执行路径。 |
| A3 SELECT 列表 | §3 消除冗余、保持一致 | `rules.md` §6 单一知识来源；Golden rule 1 | 同实体列清单只维护一份；查询列序和扫描契约不变。 |
| B1 配置映射 | §3 降低复杂度；§4 保持平衡 | `rules.md` §2 函数职责、§3 深模块；Golden rule 3 项目约定 | 按域拆分但不引入反射；保留零值、别名、配置加载顺序。 |
| B2 SQL handler | §3 合并重复；§1 保持功能 | `rules.md` §6 去重、§10 资源清理；Golden rules 1–2 | 只抽等价片段；各 handler 自己持有 lease 的 `defer`，不改变 HTTP/错误协议。 |
| B3 路由挂载 | §3 降低结构复杂度；§4 保留有效组织 | `rules.md` §2 同一抽象层、§13 文件布局；Golden rule 2 | 连续区块提取，不为了缩短文件而改路由、权限或中间件顺序。 |
| B4 装配函数 | §3 减少复杂度；§4 避免碎片化 | `rules.md` §2 函数职责、§3 深模块、§16 长函数；Workflow 4 小步重构 | 只提取有业务含义的连续阶段，仍由 `app` 唯一装配且逆序释放。 |
| C1 Agent 运行与 SSE | §3 提高清晰度；§1 保持功能 | `rules.md` §2 函数职责；Golden rule 1；Workflow 2 安全网 | 优先纯验证；事件提取不得改变同锁写入、worker 收尾和事件顺序。 |
| C2 沙盒创建 | §3 降低嵌套与复杂度；§4 避免混合职责 | `rules.md` §2 函数职责、§5 降低嵌套；Workflow 2 安全网 | 项目锁保护的检查/创建和锁外 `Start` 不能因提取而移动。 |
| C3 Agent `StartRun` | §3 按职责整理；§4 避免难调试的抽象 | `rules.md` §2 函数职责、§16 长函数；Workflow 2、4 | 在事件、取消、用量表征测试稳定后才拆分运行阶段。 |
| C4 registry / DuckLake | §1 保持功能；§4 防止过度简化 | Golden rules 1、4；`rules.md` §7 删除前确认死代码；Workflow 5 核对行为陷阱 | 先证实状态确实无用、持久化阶段确实等价；缺证据则不实施。 |

全批次共用的约束：CS §1（行为不变）、§2（服从项目规范）、§4（可读性优先于行数）、§5（限定范围）；SC Golden rules 1–5（行为、公共 API、项目惯例、不过度设计、范围），Workflow 1–6（理解调用方 → 安全网 → 规划 → 小步重构 → 验证 → 报告），以及 [`behavior-traps.md`](../../skills/simplify-code/references/behavior-traps.md) 的逐行反例检查。第 3–5 节是这些流程准则的落地，不表示本次已执行测试。上文“不强行统一”对应 CS §4、SC Golden rule 4 及 `rules.md` §3、§6 的“拒绝浅封装与错误抽象”。CS §2 中的 React/ES 模块条款不适用于 Go，Go 实施细节依 [`languages-tier1.md` Go 节](../../skills/simplify-code/references/languages-tier1.md) 和项目 `AGENTS.md`。

## 3. 实施步骤与交付门

1. **冻结每批基线**：记录改动包的 `git diff`、现有测试与当前失败；工作区已有大量未提交修改，不覆盖、不回退、不自动提交。用调用方/引用定位导出接口、动态注册和存储格式。若某项已由并行工作完成，跳过，避免重复改动。
2. **补表征测试**：先验证正常、空值/零值、错误、权限、顺序/计数、并发和超时路径。测试只断言可观察行为；不用真实 S3 或对用户数据库操作。未能建立可靠安全网的候选降级为“暂不实施”。
3. **按 A → B → C 单项推进**：一次只做一个可解释的等价变换，优先小范围去重、命名、连续阶段提取；避免公共接口变化、跨包提炼框架与整仓 `gofmt`。每项单独检查 diff 与相关测试，再进入下一项。
4. **逐行等价复核**：以 `behavior-traps.md` 对照输入和旧/新输出，尤其核对 Go `nil` interface、`defer` 时机、goroutine 退出、`errors.Is`/原错误文本；SQL `NULL`/`ORDER BY`/列序；默认值为 `0`/空串的配置；锁作用域、资源取得与释放顺序、S3 manifest 提交点。找出任何反例就保留旧行为，并另立变更议题。
5. **每批报告**：记录变更前后的职责、减少的重复或嵌套、仍保留的必要复杂度、未实施项、具体通过/失败/未运行的测试；绝不把静态检查当成已验证成果。

## 4. 验证矩阵（实施时执行，本次未运行）

```sh
# 每个候选：实际包按本批涉及范围选择；全量命令不要在每个微改后重复跑
go test -short -count=1 ./internal/catalog ./internal/llmgateway ./internal/systemdb
go test -short -count=1 ./internal/config ./internal/api ./internal/app
go test -race -short -count=3 ./internal/api ./internal/app ./internal/database/registry ./internal/gocloudagent ./internal/sandbox
go vet ./internal/...
go build ./internal/... ./cmd/...
go test ./...
git diff --check
```

仅执行改动 Go 文件的 `gofmt`，并核查格式差异没有扩散到无关区域。API 变更额外跑真实 router 的路由/权限测试；存储与并发改动额外跑对应包的 `-race` 和已有失败注入测试。若 `go test ./...` 包含外网、环境不可满足的用例，记录具体跳过原因并运行对应 `-short` 替代，**不得声称全量通过**。测试和构建不可调用 `init_db`、不得对运行库执行 ALTER/DDL；发现测试会触发用户库迁移时先停下，由用户手动决定。

## 5. 完成定义

- 每个被实施的候选都有变更前可观察行为表征、差异复核和变更后包级测试；高风险并发改动还需 race 验证。
- 导出 API、权限、系统库只读、写门禁、SQL/JSON/SSE 协议、持久化顺序与错误语义均保持不变；没有新依赖、schema 修改或未授权的数据库操作。
- `go build ./internal/... ./cmd/...` 与可运行的后端测试通过；不能运行或已有失败逐项注明。不以“文件变短”替代可读性和正确性证明。

## 6. 面向低成本模型的执行协议

### 6.1 如何使用本计划

本节起是实施操作手册；前文候选表不是自动执行授权。**一次会话只执行一个任务卡的一个子任务**，不要求一次完成全部候选。本文细化完成不代表业务代码已修改、测试已运行，也不能保证模型绝不出错；以安全网和明确停点控制风险。

默认顺序：`P0 → A1审计 → A2 → A3-agent → A3-cron → A3-schedule → B2-durability`。A1 无法证明等价时跳过，不阻塞其余独立项。B1/B3/B4 只有各自安全网完整才进入；C1–C4 **默认仅分析和补测试，不改生产逻辑**，由用户明确选择具体子项后再推进。编号表示优先级，不代表所有任务必须实施。

状态只用：`待核对 / 待表征 / 可实施 / 已验证 / 已存在 / 暂缓 / 阻塞`。只有旧实现上的表征测试通过且允许文件范围明确，才能从“待表征”进入“可实施”。编译通过不等于已验证。

### 6.2 P0：每项都要重新建立基线

1. 在仓库根目录确认 `pwd`、`git rev-parse --short HEAD`，读取根目录及目标区域规范。不要依赖本文行号定位；先查函数名再阅读完整函数、相关类型和调用方。
2. 用 `git status --short -- <允许文件>`、`git diff -- <允许文件>`、`git diff --cached -- <允许文件>` 核对已有变更。本文写作时工作区大量文件已修改，命令输出可能截断；缩小到本项文件，不把截断输出当完整基线。
3. 检查目标改动是否已经存在；已存在则记录对应符号和证据，不重复添加 helper。并行修改涉及同一函数时暂停该项，不覆盖他人变更。
4. 阅读准备运行的测试及其初始化路径：查 `TestMain`、`Bootstrap`、迁移、外部连接、环境变量和路径来源。测试必须只使用 fake/进程内数据或 `t.TempDir()` 隔离目录；不能读取真实 `config.yaml`、`.env`、`data/` 中的凭据或连接运行库。
5. 区分“用户库迁移”与“测试临时库初始化”：不得新增迁移/DDL，不得对运行库执行数据库命令；已有测试若在隔离临时库内部初始化表，仅可在确认隔离后执行。未确认前不运行 systemdb/app/ducklake 测试，更不能启动服务来代替测试。
6. 执行本项包级基线命令，记录命令、退出码、失败用例。基线失败先区分已有问题与环境问题；不能关闭断言、改期望或顺便修复其他模块来让它变绿。
7. 写下本项输入、输出、副作用和调用顺序表，然后才补测试。旧实现不满足新增断言时，先检查断言是否错误；若确为旧缺陷，暂缓相关子项并交给独立修复计划。

禁止 `git checkout/restore/reset/stash` 来清理工作区；禁止自动 commit、安装依赖或整仓格式化。撤销只能精准撤销自己本轮产生且仍能确认的片段；不能确认时保留现场并报告。

### 6.3 每个子任务固定六步

| 步骤 | 要做的事 | 进入下一步的条件 |
| --- | --- | --- |
| S1 定位 | 读取目标函数、类型、调用方；列允许修改文件和禁止触碰部分 | 当前实现与任务卡一致；不一致先调整卡片证据 |
| S2 表征 | 在现有测试文件新增针对可观察行为的测试；不改生产逻辑 | 旧实现通过新增测试及相关包基线 |
| S3 单次变换 | 只移动一段连续代码或替换一类重复表达式 | 无新增业务分支、接口、锁、goroutine、配置项 |
| S4 差异复核 | 对比输入、错误、返回、调用次数、时序、资源释放 | 每个被删/移动语句都有新位置或删除理由 |
| S5 验证 | 仅格式化实际改动 Go 文件；跑本项包测试，必要时 race | 命令真实完成且退出码为 0；环境阻塞不能记成功 |
| S6 交接 | 用 §10 模板记录证据和剩余项 | 本子任务已验证或明确暂缓；不得隐式继续下一卡 |

测试不能只断言 helper 返回正确：还必须从原入口调用，证明 handler/service 仍正确使用 helper。测试不快照内部 helper 名称，不用 `time.Sleep` 猜同步完成；并发场景用 channel/barrier 和有界超时。`-race` 通过不能证明业务顺序或缓存失效等价。

## 7. A 类任务卡：小范围、机械式变换

### A1：重复缓存失效，先审计，默认不直接删

**允许范围**：`internal/catalog/service.go` 的建库成功尾部、`cache.go` 的 `InvalidateDatabase` 阅读；测试优先放现有 `cache_test.go` / `service_test.go`。不改缓存结构、TTL、状态迁移、degraded 路径。

1. 阅读 `CreateDatabase` 成功及失败路径，逐句确认两次调用之间有没有语句；阅读 `InvalidateDatabase` 及两个 cache 的锁和填充路径。
2. 搜索 `cachedDatabase`、KV cache 填充和所有失效调用，列出竞争者是否能在两次调用之间重新填充缓存。连续调用也不代表原子操作。
3. 用现有 fake 建立“已缓存 → 成功创建/状态改变 → 下一次读取可见新状态”测试；补 KV 与普通库、创建失败/degraded 的原行为。不要直接以“失效调用次数从二变一”为正确性断言。
4. **删除门**：必须有源码级证据证明第二次失效不改变可观察语义，例如更外层互斥覆盖两次调用及所有填充方。仅测试通过、没有复现、TTL 很短均不构成证明。
5. 无证明则状态“暂缓”，生产代码零修改；有证明才只删 KV 条件内的第二次调用，不顺便重写条件或其他失效点。
6. 验证：`go test -short -count=1 ./internal/catalog`；实际实施还跑 `go test -race -short -count=3 ./internal/catalog`。

**验收**：失效涵盖两类缓存；错误路径不变；提供并发等价理由。找不到理由就是有效的“暂缓”交付。

### A2：Chat/Stream 只共用请求准备

**允许范围**：`internal/llmgateway/service.go` 的 `Chat`、`Stream` 和一个新增包内 helper；现有 llmgateway 测试。禁止改 `resolveModel`、client 缓存、resolver 优先级、usage reader。

**已核实事实**：当前 `resolveModel` 会 trim 请求/默认模型、拒绝无模型、校验 allowlist 并记录解析日志；旧计划的模型解析位置已过时。Chat/Stream 都用空 provider 名取默认 client，不能因请求 Model 字符串切换 provider。

1. 读取 `getClient`、`resolveModel`、Request 类型、`usageRecordingReader`；确认 client 的实际类型再设计 helper 签名，不凭空定义新接口。
2. 经 Chat/Stream 两个入口测试：显式模型、带空格模型、默认模型、两者为空、allowlist 拒绝、resolver/client 获取失败。失败时断言上游调用为零、原领域错误和文本保持。
3. 上游 fake 捕获请求，逐字段断言 `Model/Messages/MaxTokens/Temperature/Tools`；覆盖零值和 nil，不能把 nil slice 自动变成空 slice或复制/排序工具。
4. 新增一个私有请求准备方法，返回现有 client 类型、原 ProviderConfig（Stream 包装仍需要 cfg.Name）、`*providers.Request` 和 error。内部原序执行 `getClient(ctx, projectID, "") → resolveModel → 构造请求`；不额外包装错误。
5. Chat 只替换原准备片段，保持 `Response{}, err`；Stream 同样替换，保持 `nil, err`。上游执行错误各自保留 `llmgateway: chat:` / `llmgateway: stream:` 包装。
6. 验证 Chat 用量错误不改变成功响应；Stream 仍只在原 reader 生命周期时机计费，不能在 helper 或 Stream 返回前记账。保留原 provider/model 来源，禁止统一为响应字段。
7. 验证：`go test -short -count=1 ./internal/llmgateway`。

**验收**：请求准备只维护一份；每次入口解析一次；执行、输出映射、计费及日志调用位置不变。如果 helper 返回值/抽象比重复更难读，暂缓而不是拆成多个转发方法。

### A3：SELECT 列清单，每个实体独立交付

**子任务及允许范围**：`A3-agent` 只改 `internal/systemdb/agents.go`；`A3-cron` 只改 `cron_jobs.go`；`A3-schedule` 只改 `agent_schedules.go`。测试只能改对应现有测试文件；先搜索实际名称，不假定有某个 schedule 测试文件。每个子任务单独验证。

1. 搜索当前实体全部 SELECT 和 scan 函数，按序写出“列号 → SQL 字段 → Scan 目标”。带 alias、JOIN、聚合或不同字段集的查询排除在替换范围外。
2. 在旧实现测试完整字段 round-trip、可为空字段、Get/List 一致性、归档过滤、排序；空结果与不存在的错误也按原行为断言。测试环境必须满足 P0 隔离要求。
3. 只把完全相同的列文本提取为同文件私有 string 常量。沿用现有字符串拼接风格；不建 SELECT builder，不添加表前缀，不改 WHERE/ORDER BY、参数、空格边界。
4. 逐条替换匹配查询，逐列复核 SELECT 和 Scan 数量/顺序。INSERT、UPDATE、schema 和扫描函数不在本子任务修改范围。
5. 验证：`go test -short -count=1 ./internal/systemdb`。列清单替换不涉及并发逻辑，通常无需新增 race 验证。

**验收**：同实体一份列清单，所有替换点有明确列表；不同实体没有共享常量；原 SQL 查询含义和错误不变。

## 8. B 类任务卡：先锁行为，再移动阶段

### B1：配置域提取，不改配置文件

**允许范围**：`internal/config/yaml.go` 或 `env.go`，每项只选其中一个文件的一个连续配置域，以及现有 config 测试。读取 `config.go` 的加载顺序，不修改其流程；禁止改 YAML 文件、env 名称、默认值、解析器及配置结构。

1. 当前 env 已有 `applyEnvAgent/applyEnvSandbox/applyEnvLLM`，先盘点已有阶段；不能重复造同名/同义 helper。为尚未提取的域列出起止语句、读取键、目标字段、别名覆盖顺序和是否读取前域结果。
2. 表征至少覆盖：未提供值、显式 `0/false/""`、有效非默认值、无效格式、主键与别名同时给值。YAML pointer 判空与 env 空串回退语义不同，分别断言；错误若被原实现忽略，不能在此变成返回错误。
3. 用 `t.Setenv` 隔离 env；涉及 env 的测试不并行执行。准备仅临时目录 YAML 测试输入，禁止把真实密钥写进 fixture。
4. 把一个连续域原封不动移动到包内函数，传入原配置指针与实际需要的 YAML 输入；调用留在原位置。不要改变条件、常量、赋值语句及先后顺序。
5. 对该域每条赋值建立旧/新对应表，确认每条只执行一次。验证默认值 → YAML → env → YAML 密钥拒绝 → Validate 顺序未变。
6. 验证：`go test -short -count=1 ./internal/config`。通过后停止；下一个域使用新的独立任务卡。

**停点**：域不是连续片段、有跨域局部变量且需要重排、必须改变函数公开签名时暂缓。

### B2-durability：优先实施的 SQL 子任务

**允许范围**：`internal/api/sql_handler.go` 与 `sql_handler_test.go`；只替换 Execute 一处和 Batch 三处 durability 选择，不抽取其余执行逻辑。

**当前真实顺序**：`writable → timeout/context → project → principal → decode/StageTimer → sqlguard → GetDatabase → 系统库拒写 → semaphore → Acquire → 执行`。保留计时 scope 与 defer；前文简写从 decode 开始，不能误认为 writable 在解码后。

1. 旧实现表征四条响应路径：Execute 成功、Batch 事务成功、事务回滚、非事务部分失败。每条分别测试 callback 为 nil、返回自定义值、返回空字符串。
2. 默认值严格是 `committed_local`；callback 返回空串仍是空串，不做兜底。用 spy 断言 callback 收到原 URL databaseID，且每次上述响应只调用一次；Acquire/执行前失败不调用。
3. 新增私有 `durabilityFor(databaseID string) string`：nil 时返回原默认值，否则直接返回 callback 结果。函数名以现有引用检查无冲突为前提；不改公开字段 `DurabilityFor`。
4. 在四个原位置用 helper 替换赋值片段，不提前到 handler 顶部，不合并 Batch 响应分支。事务失败继续 HTTP 200、`batch_rolled_back`、原 failed index/message 和 `DurationMS`。
5. 验证：`go test -short -count=1 ./internal/api`。

**验收**：四个调用位置和次数不变；没有 lease/semaphore/响应结构改动。

### B2-database：默认暂缓的第二子任务

只有 B2-durability 已验证且用户选择继续，才评估取库与系统库拒写 helper。先以 fake 验证 Execute/Batch 两个入口：只读实例不解码/不调用服务、非法 SQL 不 GetDatabase、系统库不 Acquire、GetDatabase 失败不 Acquire、Acquire 失败仅释放已取得的 semaphore、成功 lease 恰好 Release 一次。断言错误 HTTP/code，而不只断言 Go 返回值。

仅允许提取 `GetDatabase → IsSystemDatabase` 原连续段；project/principal、sqlguard、StageTimer、semaphore、Acquire 和所有 defer 留在 handler。不得让 helper 创建 timeout、获取 lease 或调用 `WriteError`。如果参数数量/错误分支让逻辑更绕，保持重复，状态“暂缓”。验证命令同 B2-durability。

### B3：路由块提取，一次只移动一个连续块

**允许范围**：`internal/api/router.go`，已有 `write_gate_test.go` / `delegation_route_table_test.go` 和必要的现有 router 测试。默认 helper 留在原文件；不要一开始创建多文件路由框架。

1. 读取注册函数完整上下文和 require/delegable/writeGate 定义；选择一个按业务命名的连续路由块。列出 method、完整 path、handler、middleware 参数顺序、外层 group、可选依赖条件及相对注册位置。
2. 在旧真实 router 上断言完整预期 method/path 集合和选中块路由数量，排除 Echo 自动 HEAD 等差异时必须说明原行为。`Routes()` 不暴露真实授权语义，不能用其替代请求测试。
3. 对选中块经 ServeHTTP 测试：未认证、权限不足、正常权限、delegated 身份、writable=false。status/code 取自当前实现，不盲写所有未授权都为 403。
4. 对块内每个可选依赖分别置 nil，断言路由不存在；已有 nil 行为若是 panic，先记录旧缺陷，不在简化里修复。校验 project context 仍注入。
5. 原封不动移动块到一个包内挂载函数；原调用点保留原 nil 判断或将整个含判断的连续块一起移动，二者只选一种，不重复判断。保留 group 对象，不重建带额外中间件的新 group。
6. 不从 method 推断权限：`POST /kv` 仍由 handler 分类读写；agent run 创建的 `DatabaseRead + writeGate` 不改成 DatabaseWrite。这些路由若不在本块，不能顺便移动。
7. 验证：`go test -short -count=1 ./internal/api`，复核全路由集合、授权响应和注册先后证据。完成一个块后停止。

**停点**：块依赖大量局部闭包、需要重排注册次序、需要改变 Dependencies 或改 handler 时暂缓。

### B4：装配提取，先选不含生命周期的片段

**允许范围**：`internal/app/app.go` 和对应现有 app 测试；不改公开构造函数签名、App 字段、依赖接口或生产 bootstrap 行为。

当前 `assembleDeps` 的阶段地图（实施时重新定位）：

| 原连续阶段 | 必须保持的锚点 |
| --- | --- |
| 存储身份与 objectstore/CAS 探针 | 身份检查先执行；CAS 失败的拒启/日志/指标语义不变 |
| 文件存储选择 | 位于 writable 提前返回之前；只读也会装配 fileStore |
| writable 提前返回、catalog 引擎检查 | 引擎检查位于系统库 Bootstrap 之前 |
| factory/Bootstrap/catalog repair | systemStore 赋值后原位置注册 closer；repair 错误原样包装 |
| lease manager 与回调、registry | 失租 CloseNoFlush → 原退出策略 → registry.Remove → gate.Clear；保留闭包捕获时机 |
| auth/user/session、Seed、retention/flush | OnWrite 通知不遗漏；种子先于后台维护 |
| maintenance、cache manager、usage/audit | 启动与 closer 注册顺序不变；后续失败仍进入原清理路径 |
| LLM resolver 链 | 项目动态凭据 → catalog → 实例回退，保持构造和优先级 |

1. 首选末尾 `if cfg.LLM.Enabled` 连续块；只有读取 resolver 类型、现有测试并锁定禁用/无实例配置/有实例配置的链顺序后，才提取一个私有装配方法。其余阶段本轮不动。
2. 对后续片段先补资源轨迹测试：fake closer 按注册顺序记录，失败时倒序且不重复释放。阅读 NewWithRegistry、Close、Shutdown、registerCloser，不能只测试 helper 成功返回。
3. 将阶段需要的局部输入列清楚；移动后必须继续使用原对象，不重新调用 factory/resolver，不通过 nil interface 改存在性判断，不提前启动 goroutine。
4. 提取 error 返回时保留包装只发生一次、原文本不变；保持父函数原失败清理入口，不在 helper 另加 defer Close。
5. 带 Bootstrap/Seed/maintenance 的阶段先检查测试隔离和初始化路径；不能启动真实 app、连接 S3 或改运行库获取证据。缺 fake 证据则暂缓，不为了测试大改装配接口。
6. 验证：`go test -short -count=1 ./internal/app`；触及闭包、启动/关闭时再跑 `go test -race -short -count=3 ./internal/app`。

NewWithRegistry 的 handler/server 整理是独立后续子任务，不与 assembleDeps 混做。任何 lease、CAS、syncer 片段不得作为低成本模型首个重构项。

## 9. C 类任务卡：证据不足时零生产修改

C 类除 P0/S1–S6 外，还必须提交“锁/状态/事件轨迹表 + 失败注入清单”。用户没有明确选择子项时，只完成审计和测试设计，不自动重构。缺 race、事件时序或持久化故障证据，状态为暂缓，而非完成。

### C1：Agent 验证与历史筛选

1. 阅读 CreateRun/相关 SSE 入口、claim 释放、run 写入与 worker 收尾；核对 v5 竞态修复对应源码和测试，不能只以文档勾选判断完成。
2. 先测 retry 复用已有 user 消息、非最新 run 拒绝、claim 失败/后续失败释放；列出每条路径 claim、写库和启动 worker 次数。
3. 用户选择后只提取不涉及锁/IO 的连续验证或历史筛选片段；保持过滤顺序、nil/空集合、错误优先级。需要查询仓库或修改状态的片段不叫“纯验证”。
4. SSE 提取另立子任务；事件 ID 分配、缓存更新、输出必须仍在同一锁边界，结束前停 worker，不把 defer/goroutine 移入 helper。
5. 验证 `go test -short -count=1 ./internal/api` 及 `go test -race -short -count=3 ./internal/api`；明确实际覆盖 `agent_e2e_usage_retry_test.go`、`agent_e2e_stream_test.go`、`run_events_lifecycle_test.go`，没有运行不能声明事件安全。

### C2：Sandbox.Create 输入处理

1. 当前序列是：Available/key 校验 → 项目锁 → 幂等命中 → name 生成/冲突 → quota → image/resources/network/idle 校验 → CreateSandbox → idem 写入 → 解项目锁 → 按 Start/pending 判断启动。**不得把归一化提前到锁或幂等检查之前**。
2. 测试幂等命中跳过名称/配额/资源校验、过期与读取失败走原创建路径、同名优先于配额、创建失败不记 idem、Start 失败保留原落库语义、复用 pending 时原 Start 行为。随机名称只断言格式/非空，不固定值。
3. 用受控时钟测 TTL 边界：复用用 Before，清理用 After，不擅自改为 `<=`；容量淘汰任意条目，不添加排序/确定性淘汰。
4. 用户选择后首选只提取 image 至 idle 的连续归一化/校验段。用 `*CreateInput` 或返回更新后的输入都必须明确保存原闭包内变更，否则后续落库会用旧值。项目锁、m.mu、Start 调用位置不动。
5. 验证 `go test -short -count=1 ./internal/sandbox` 和 `go test -race -short -count=3 ./internal/sandbox`；使用已有 fake driver/store，禁止连接真实沙盒。

### C3：gocloudagent.StartRun 事件阶段

1. 对现有 runtime 测试列出正常、tool call/result、多工具、取消、超时、截断、usage 的事件轨迹；保留事件名称/ID/先后、最终响应和完成状态。
2. 确认 nil 与空 content、usage 缺失、多次增量及 tool 失败的旧行为，不能顺便修正计数或补发事件。
3. 用户选择后仅提取一个连续的事件归集或结果生成片段，不改 channel 所有权、context、取消传播、超时计时或 goroutine 数量。
4. 不更改 CLI 参数、凭据、sandbox 接口和工具信任边界；不得因“纯结果 helper”调用外部工具。
5. 验证 `go test -short -count=1 ./internal/gocloudagent` 和 `go test -race -short -count=3 ./internal/gocloudagent`；检查 runtime/run_limits/cli_tool_planv6 对应测试实际存在和执行。

### C4-registry 与 C4-syncer：两个独立研究项

- **registry**：搜索 `openErr` 所有读写、entry 生命周期、open 等待者、失败重试、Close/Remove。写完状态表后才判断死状态；没有普通读取不意味着能删同步语句。用户选择后最多删除一个被证明无可观察作用的字段及其写入，不重构 Acquire 锁/等待机制。
  - 状态：**已完成（2026-10-13）**。审计结论：`openErr` 全仓唯一写点在 `openAndAcquire` 失败分支（写后立即删 entry），零读取方；等待者通过 `close(opening)` channel 唤醒后重读 entry，不消费该字段；删除该字段及其写入通过 `go build ./internal/...`、`go vet`、`go test -race -count=1 ./internal/database/registry ./internal/database/ducklake`（含表征测试 `c4_syncer_lease_test.go`）。Acquire 锁/等待机制未动。
- **syncer**：先画捕获快照 → 写对象 → 发布 manifest → 推进水位的状态图，列出每阶段失败、CAS 冲突、失租/CloseNoFlush、关闭与并发 Flush 的预期。用户选择后一次只提取一段原连续逻辑，不把锁内读改成锁外读，不提前推进水位，不在 Close 兜底 flush。
  - 状态：**已完成（2026-10-13）**。状态图：S1 captureCatalog（锁内捕获）→ S2 上传快照（PutIfAbsent 幂等）→ S3 发布 manifest（CAS 冲突=split-brain）→ S4 锁内推进水位 → S5 本地 state 落盘（仅警告）→ S6 指标/日志。新增表征测试 `c4_syncer_ordering_test.go`：S3 成功后 manifest/内存水位/本地 state 三者一致；S5 失败（local-state.json 被目录占位致 Rename 失败）不改 Sync 结果且远端仍发布。单次变换：S2+S3 原序提取为 `publishSnapshot(ctx, meta, snap, data) (seq, epoch, snapKey, err)`，锁内读未外移、水位未提前、Close 未加兜底 flush。既有失败保序/split-brain/失租测试全数通过。验证：`go test -short -count=1 ./internal/database/ducklake`（144s 全绿）+ `go test -race -short -count=1`（80s）。 |
- 分别验证 `go test -short -count=1 ./internal/database/registry`、`go test -race -short -count=3 ./internal/database/registry`；syncer 用对应 `./internal/database/ducklake` 两条命令。执行前审计 DuckDB/扩展下载及测试临时目录，环境不满足则阻塞，不安装依赖、不连远端。
- 完成标准不是减少字段/行数，而是提供删除证明或保序故障测试；不能提供就保留原实现。

## 10. 交付、停止与续接

### 10.1 每项交接模板

实施代理在回复中填写以下信息；若需持久化，更新本计划对应状态，不另造未经要求的报告文件：

```text
任务 ID / 子任务：
状态：待核对 / 待表征 / 可实施 / 已验证 / 已存在 / 暂缓 / 阻塞
基线：HEAD、允许文件已有差异说明
允许文件 / 实际修改文件：
源码证据：函数名、原连续片段、调用方
表征证据：新增或复用的真实测试名，旧实现运行结果
等价复核：错误、nil/零值、时序、次数、资源/锁逐项结果
验证：完整命令、退出码；未运行或失败的具体原因
保留复杂度 / 未实施项：
下一步：仅建议下一个子任务，不自动执行
```

未经运行的“预计通过”、测试文件存在、静态代码阅读均不能填为测试通过。新增测试与生产改动要在报告中区分；自测新增测试通过不替代全包验证。

### 10.2 立即停止本子任务的条件

- 必须改变导出签名、协议/错误文本、权限、配置优先级、锁/资源时序或 schema 才能实施。
- 表征测试在旧实现失败且不能证明断言错误；发现 v5 未修复缺陷影响本候选。
- 测试会连接用户库、真实 S3/沙盒，或需安装依赖、执行迁移/重置数据。
- 修改文件被并行工作改变；差异无法区分原有变更与本轮变更。
- 测试失败定位超出允许范围；不能通过删测试、放宽断言、加 retry/sleep 绕过。

停止只影响相关子任务；独立任务可以在下一会话重新 P0 后执行。已改但未验证必须明确报告现场，不能标“完成”。

### 10.3 阶段级最终验收

每个微改跑本项包级测试；A 阶段及每个完成的 B 阶段再执行 `git diff --check` 和 `go build ./internal/... ./cmd/...`。所有选定任务结束后，审计环境再执行 §4 的 vet/全量命令；对实际触及并发/存储的包执行 race，不能用其他包 race 代替。

`git diff --check` 在大量既有差异下可能报告旧问题：记录具体文件行号，区分本轮新增；不顺手修全仓。审查实际修改文件集合、导出声明、SQL 和配置差异，确认没有扩出卡片范围。文档细化本身只做链接/编号/内容一致性检查，不宣称后端构建或测试已通过。

### 10.4 可直接交给下一模型的任务指令

> 阅读本计划 §6 执行协议和任务卡 `<ID>`，本次只执行 `<子任务>`。先核对当前源码、既有差异和测试隔离；在旧实现上补并运行表征测试，通过后才执行卡片允许的单次等价变换。不得改变公开 API、权限、错误、锁/资源/持久化时序，不安装依赖、不改配置、不访问运行数据库。证据不足按停点暂缓，不猜测实现；完成后按 §10.1 报告真实验证结果并停止，不自动继续其他任务。
