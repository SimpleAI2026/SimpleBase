# SimpleBase Agent 指南

面向 AI 编码代理（Codex / CodeBuddy / Cursor 等）的项目级规范。**修改某区域前先读该区域的子目录约束**。本文件的硬性约束是全仓底线；子目录只补充更细规则，冲突时以本文件硬性约束为准，其余以更具体的文件为准。

| 文件 | 约束范围 |
| --- | --- |
| [`internal/AGENTS.md`](internal/AGENTS.md) | 后端模块职责、系统库访问路径、API 协议（含 OpenAI 兼容出口） |
| [`internal/README.md`](internal/README.md) | `internal/` 模块清单与 `go list` 依赖图（结构变化后必须同步） |
| [`ui/AGENTS.md`](ui/AGENTS.md) | 前端目录职责、API 层、i18n、样式定稿、组件与测试约定 |
| [`godatabase/AGENTS.md`](godatabase/AGENTS.md) | 数据库运行时库的依赖白名单、磁盘契约、nil 语义 |
| [`gofunction/README.md`](gofunction/README.md) | 解释器公共 API、能力边界、已注册包清单 |
| [`gosandbox/README.md`](gosandbox/README.md) | 本机 shell 隔离与云沙盒执行后端 |
| [`gocloudagent/README.md`](gocloudagent/README.md) | 通用 Agent 运行时：模型适配 / 运行循环 / Plan / 群聊 / MCP / CLI |
| [`internal/utils/README.md`](internal/utils/README.md) | 宿主工具的入库标准（零业务依赖） |
| [`openspec/README.md`](openspec/README.md) | 能力 specs、在途 changes 与维护规则 |

## 1. 项目速览

SimpleBase 是**单写实例**的云端数据库服务：每个项目一个 DuckDB 或 SQLite 单文件（SQL、集合、KV 共用；S3 模式下以快照加 manifest 同步），系统库使用 DuckLake；Echo 提供 REST API，`go:embed` 内嵌 Vue 控制台，进程内 `gofunction` 解释器运行 Go 云函数，`llmgateway` 封装多供应商 LLM 与云 Agent。

### 1.1 技术栈

| 层 | 技术 |
| --- | --- |
| 后端 | Go 1.25（CGO，DuckDB 需要）· Echo v4 · DuckDB / SQLite / DuckLake · AWS SDK v2（S3 兼容）· litellm + CloudWeGo Eino · Zap · Prometheus |
| 前端（`ui/`） | Vue 3 · TypeScript · Vite · Pinia · vue-router · vue-i18n · Tailwind CSS v4 · reka-ui 2.10.5（shadcn-vue 风格）· vue-sonner · ECharts · Monaco · Vitest；Node 22.22+ / Yarn 4.17 |
| SDK | `packages/go-sdk`（根模块内）· `packages/js-sdk`（TypeScript，Yarn Classic 1.22） |
| 工具 | `skills/simplebase/scripts/sbdev.py`（Python 3）· OpenSpec CLI |

### 1.2 模块与目录

模块 `github.com/linkxzhou/SimpleBase`。四个嵌套独立模块各带 `go.mod`，根 `go.mod` 用 `replace` 引用；**均不得 import 根模块任何包**：

| 模块 | 角色 | 额外允许依赖 |
| --- | --- | --- |
| `gofunction/` | Go 解释器；实现全在 `gofunction/internal/interp` | 仅自身 |
| `gosandbox/` | 通用沙盒执行层 | 仅自身 |
| `godatabase/` | 数据库运行时库；宿主类型经 `internal/dbbridge` 注入 | 仅自身 |
| `gocloudagent/` | 通用 Agent 运行时；宿主能力经 `internal/agentbridge` 注入 | `gosandbox` |

其余目录：`cmd/`（`simplebase` 服务与 CLI、`perfbench` 压测）、`ui/`（控制台与文档站）、`packages/`、`skills/simplebase/`（本地开发与云端重放）、`examples/`（站点与能力用例）、`scripts/`（测试 / 冒烟 / examples-verify）、`docs/`（用户文档）、`bench/agent/`（Python agent 评测）、`openspec/`（计划与变更规范唯一入口）。

## 2. 架构与边界

后端按**战略 DDD + 六边形应用服务**组织，事实来源 [`openspec/specs/platform-architecture/spec.md`](openspec/specs/platform-architecture/spec.md)。

### 2.1 分层与依赖方向

```text
cmd/simplebase → app（唯一组合根：装配、启动收敛、关停、app 私有适配）
  ├─ internal/api（入口内核：路由树 / 守卫 / 中间件 / 健康观测 / 错误兜底 / 站点分流 / 内置 MCP）
  │    ├─ api/httpkit（叶子包：错误协议 / context / SSE / 审计辅助 / 计时）
  │    └─ api/<context>http（agent · data · sandbox · function · file · iam · llm · ops · catalog · backup）
  ├─ internal/agentrun（Agent 协作应用服务）
  ├─ internal/agentbridge（宿主 ↔ gocloudagent 的 ACL）
  ├─ 领域与基础设施：auth / catalog / collection / systemdb / dbbridge / objectstore / sandbox / e2bproto
  │                  functionrun / trigger / cronjob / llmgateway / usage / audit / backup
  │                  observability / alerts / realtime / config / web / utils
  └─ 嵌套独立模块：godatabase / gocloudagent / gosandbox / gofunction
```

依赖只能自上而下，**禁止反向 import**。包级依赖图以 [`internal/README.md`](internal/README.md) 为准。

### 2.2 限界上下文

| 上下文 | 领域 / 应用包 | 入站适配 |
| --- | --- | --- |
| Agent 协作（核心） | `agentrun`、`agentbridge`、`gocloudagent` | `api/agenthttp`、cron、内置 MCP |
| 数据服务（核心） | `collection`、`godatabase`、`dbbridge` | `api/datahttp` |
| 文件与站点 | `objectstore` | `api/filehttp` |
| 自动化 | `functionrun`、`trigger`、`cronjob`、`gofunction` | `api/functionhttp` |
| LLM 网关 | `llmgateway`、`usage` | `api/llmhttp` |
| 沙盒 | `sandbox`、`e2bproto`、`gosandbox` | `api/sandboxhttp` |
| 身份与访问 | `auth` | `api/iamhttp` |
| 项目与目录 | `catalog` | `api/cataloghttp` |
| 运维治理 | `backup`、`audit`、`observability`、`alerts` | `api/backuphttp`、`api/opshttp` |
| 平台（非业务） | `app`、`config`、`systemdb`、`web`、`utils` | `api`、`api/httpkit` |

### 2.3 架构门禁

`internal/app/arch_gate_test.go`（`TestArchitectureGate`）强制：

1. **R1** 只有 `app`（及既有例外 `api/perfbench`）可 import `internal/api`；**R4** `internal/api` 不 import `app`。
2. **R2** `api/httpkit` 是叶子包：不 import `api`、`api/*`、`app`。
3. **R3** `api/<context>http` 只依赖 `httpkit`、`agentrun` 与领域包，不 import `api`、`app`、同级子包。
4. **R5** `agentrun` 不 import `api*` / `app` / Echo；**R6** `agentbridge` / `cronjob` 不 import `agentrun`（`agentrun` 实现其 `GroupCronRunner` / `RunFinalizer` 端口，由 `app` 注入）。
5. **R7** `internal/litellm` 若出现不得 import 其他 `internal/` 包；**R8** `internal/utils` 只 import 标准库。
6. **R9** 嵌套独立模块只 import 自身（`gocloudagent` 另允许 `gosandbox`）。
7. **R10** `UpdateAgentRunStatus` 只能从 `internal/agentrun` 调用，`UpdateAgentTurnStatus` 只能从 `agentrun/group_store.go` 调用。
8. **systemdb 基线只降不升**：`internal/app/testdata/arch-baseline.txt` 记录各包使用面；新包 `*systemdb.Store` 字面计数为 0，只经消费方窄接口。
9. **入口内核文件清单**：`internal/api` 的 25 个顶层生产文件由 `TestKernelResidueFiles`（`internal/api/kernel_residue_test.go`）锁定；`Dependencies` 直接引用 `<context>http` 类型，不再保留 `*_compat.go` 转发层。

### 2.4 新代码放哪

1. 有 HTTP / WS / SSH 帧或状态码 → 对应 `<context>http`；协议原语（错误、context、SSE、审计辅助、计时）→ `api/httpkit`。
2. 被多个入口（HTTP / cron / 事件 / 内置 MCP）复用或需跨请求生命周期 → `agentrun` 或已有领域包；只有一个调用方且无所有者 → 留在调用方，不建包。
3. 只是翻译外部系统或独立模块 → 对应 ACL（`agentbridge` / `dbbridge`）；仅装配处使用的适配 → `app` 私有。

领域事件类型归发布方（`collection.RecordEvent` / `objectstore.ObjectEvent`），fan-out 与 trigger 适配归 `app`；`*sql.DB` / lease / 行类型不作为新接口契约。

### 2.5 模块边界要点

- **组合根**：跨模块依赖只在 `app.assembleDeps` 按序创建，`Close` 反向释放；新增模块要挂进装配顺序并在失败路径回收。`confirms` 先于运行时、`agentrun.Service` 先于 cron；router 构造期不得改写共享运行时或回填依赖。
- **handler 子包**：只提供 handler 与构造函数 / 窄 `Deps`，不自带路由注册（路由在 `api` 的 `mount*Routes`，E2B 例外 `sandboxhttp.MountRoutes`）；handler 经接口 + adapter 依赖实现。
- **Agent 归属**：run 生命周期、群轮次、群定时、审批、事件环、规划工具都在 `agentrun`；`api/agenthttp` 只做解析、鉴权与归属校验、SSE 编码、DTO 映射。`agentbridge` 负责 SimpleBase 的模型默认值、提示词、项目工具、`CommandPolicy`、委托签发、`InProcessCLI`；身份直接传 `auth.Principal`。
- **Plan 与群会话**：Plan 只经 `plan` 工具产生，不得重新引入服务端写死的阶段流水线。`/runs` 只服务固定助手会话；群会话（`dispatch_mode=collaborative`）走 `/turns`，误发 `/runs` 返回 `400 turns_required`。run / 轮次状态机唯一权威在 `gocloudagent/group/state.go`，不在应用层复制。
- **LLM 错误**：只在最终出口归一化；运行内部保留原始 `*providers.LiteLLMError` 供回退判断。
- **技能文档**：SimpleBase 技能在 `internal/agentbridge/agentskills/simplebase/` 随宿主 embed；`gocloudagent/skills` 只是通用加载器，不内置平台内容。
- **gocloudagent 子包**：依赖自下而上 —— `tools → agentcore, mcp`；`plan → agentcore`；`group → agentcore, plan`；`cloudagent → agentcore, mcp, tools`；`cli → agentcore, mcp, plan, tools`。`cloudagent` 与 `cli` 互不 import。CLI 产物输出到 `build/bin/gocloudagent`，不进服务镜像。
- **共享工具**：`internal/utils/` 只放宿主私有零业务依赖工具；与数据库共用的工具在 `godatabase/{atomicfile,hashx,limit,singleflight,sqlval,sqlx}`，不保留副本。
- **明确不做**：`domain/application/infra` 目录化、按聚合拆 `systemdb`、事件总线 / CQRS / 微服务、通用 `internal/usecase` 树、为关停新增配置项。

## 3. 硬性约束

1. **系统库保护**：系统库（`kind=system`，默认 `simplebase-system`）禁止删除与写入、只允许 SELECT；删除返回 `ErrSystemProtected`，数据写接口前置 `IsSystemDatabase` 拒绝，`systemLeaseAdapter` 双保险。不得经用户库 registry 打开系统库。
2. **配置来源**：只从 `config.yaml` + `SIMPLEBASE_*` 环境变量加载（`internal/config`），env 覆盖 YAML；密钥不得落 YAML。
3. **日志与审计脱敏**：审计只记 SHA-256 与元数据；日志 / 审计不得含密钥、SQL 参数、LLM 正文。唯一例外 `agent.trace_log`（默认关闭）：开启后正文经脱敏 + 截断只写 zap，不写系统库 logs 表。
4. **依赖管理**：不新增 / 升级第三方依赖，不改 `go.mod`、`package.json`、`tsconfig.json`、`vite.config.ts`、锁文件，除非用户明确要求。
5. **SQL 边界**：拒绝清单、只读检测、多语句与 NUL 拦截统一在 `godatabase/sqlguard`，不散落到 handler。
6. **写入门禁**：写路由挂 `writeGate`，写 handler 检查 `writable`（只读实例返回 `ErrWriterUnavailable` / 503）。
7. **鉴权**：新路由挂权限（`auth.Require`），带 `:projectID` 的路由经 `projectContextMiddlewareEcho`。权限判定只走 `auth.Principal.HasPermission` / `auth.PermissionImplied`；层级 `database:admin ⊃ write, read`，`write` 不含 `read`。签发子集校验与鉴权同口径；只读降级要同时剥离 `write` 与 `admin`。
8. **单项目单库与删库**：每项目一个用户库。删库走 `catalog.ProjectDataClearer`：只 DROP 用户表 / 视图 / 序列并清理集合元数据，保留 `__` 内部表（KV）、项目文件、备份；整体删文件仅是无 registry 时的退路。对象键用内部 UUID，不拼用户可控字符串。
9. **错误映射**：错误走 `WriteError` + 领域错误；不把裸 `errors.New` 交给 `WriteError`（会变 500）。新哨兵加到 `api/httpkit/errors.go` 的 `sentinelErrorMappings`；文案由底层决定的用 `InvalidError`。
10. **fail-closed**：命令策略 / MCP 策略缺失或出错时拒绝执行；沙盒隔离不可用时不得退回裸执行；委托令牌只带已降权权限。
11. **dev 双实现一致**：`simplebase dev` 与 `sbdev.py` 的校验文案、动作序列、记账名（`<文件名>`、`seed:<路径>`、`seed-doc:<集合>/<id>`）必须一致，两端测试逐字比对 `internal/sbcli/testdata/dev/*/expect.json`。服务端忽略客户端文档 `id`，去重改用 `_migrations` 记账。
12. **停机语义**：`App.Shutdown` 先 `agentrun.BeginShutdown()` 取消在途 run / 轮次，再 `agentrun.Wait(10s)` 收尾；之后新建 run / 轮次 / 群定时返回 503 `service_stopping`。`waiting_human` / `awaiting_plan_confirm` 轮次不动。
13. **架构门禁是硬门禁**：改 `internal/` 后 `TestArchitectureGate` 必须通过；新增业务 handler 放 `api/<context>http`。

## 4. 代码风格

### Go

遵循 Go 官方风格，全仓（`examples/` 云函数源码除外）必须过 `gofmt`。每个模块带单元测试。新增文件沿用文件头注释块（`// 文件说明：…` + 用法 / 调用架构 / 主要符号），注释引用的文件名必须真实存在。`gofunction` 的 `Executor.Compile` / `SetProfile` / `Runtime.Cleanup` 是预留空实现，不当已实现功能使用；对外 API 只在模块根门面转发。

### TypeScript / Vue

- 对象形状用 `interface`，不用 `type`；不写 `any`（`ui/src` 已清零）。
- 跨层契约集中在 `ui/src/services/types.ts`，后端 snake_case 经 `services/converters/` 映射为 camelCase；项目路径只经 `projectPath()` 构造；页面不直接 import axios。
- 样式写 Tailwind class，不写自定义 CSS；复用 `SbModal` / `ConfirmAction` / `SbEmptyState` / `SbAsyncRegion` / `SbTabsPanel` / `TablePager` 等现有组件。
- 状态样式匹配 reka-ui 真实属性：写 `data-[state=checked]:…`，不写 `data-checked:…`。
- 文案只在 `ui/src/locales/`，`zh-CN.ts` 与 `en-US.ts` key 集合必须一致；组件外用 `@/locales` 的 `t()`；裸 `@` 写成 `{'@'}`。
- 组件测试不桩掉 `src/components/ui/` 的 reka 交互组件。详见 `ui/AGENTS.md`。

### 脚本与样例

- 兼容 macOS（BSD 工具）与 Linux：毫秒计时用 `python3` / `perl`，不用 `date +%s%3N`；不硬编码个人路径；端口参数化（默认 18xxx，避开 8080）。
- 对样例跑 `dev apply` / `sbdev.py apply` 会写 `.simplebase/`，验证一律复制到临时目录。
- 站点清单显式写 `data_model`、只声明一个库、`once` 任务写死 `run_at`。
- Node ESM 入口守卫用 `realpathSync` 比较 `process.argv[1]` 与 `import.meta.url`。

## 5. 常用命令

在仓库根执行；DuckDB 相关命令统一 `CGO_ENABLED=1`。

```bash
# 开发 / 构建
./build.sh dev [--no-open] [--api-port 8081]   # 后端 :8080 + Vite :5173
./build.sh                    # yarn build → 同步 internal/web/dist → go build -o simplebase
./build.sh --skip-ui          # 仅后端（需先完整构建一次）
./build.sh --ui-only          # 仅前端并同步 embed 目录

# 后端
CGO_ENABLED=1 go build ./internal/... ./cmd/... && CGO_ENABLED=1 go vet ./internal/... ./cmd/...
CGO_ENABLED=1 go test ./...                                # 根模块（不含四个嵌套模块）
go test ./internal/app -run TestArchitectureGate           # 架构门禁
go test ./internal/app -run TestArchitectureGate -update   # 重新生成基线（只接受下降）
go test ./internal/api -run TestKernelResidueFiles         # 内核文件清单锁
go test -race ./internal/agentrun/ ./internal/api/ ./internal/app/   # run / 轮次 / 关停

# 独立模块
(cd gofunction   && go test ./... -short && go vet ./...)
(cd gosandbox    && go build ./... && go vet ./... && go test ./...)
(cd godatabase   && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./... -timeout 900s)
(cd gocloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...)

# 前端与 SDK
(cd ui && yarn typecheck && yarn test && yarn build)       # coverage 90%，以 vite.config.ts 为准
go test ./packages/...                                     # Go SDK
(cd packages/js-sdk && yarn typecheck && yarn test)        # JS SDK（Yarn Classic 1.22）

# 回归与端到端
./scripts/test.sh [包...] [-race]                          # 云 Agent 包 + 独立模块 + 四站清单离线校验
./scripts/smoke.sh                                         # 构建 + vet + 全链路 + 架构门禁 + 前端构建
(cd skills/simplebase && python3 -m unittest discover -s scripts -p 'test_sbdev.py')
scripts/examples-verify/verify.sh --ports 18090,18091 [--only shop]   # 0 FAIL；无 profile 时 LLM 项 SKIP 属预期
./examples/run_all.sh --start --skip-build --skip-agent --api-port 18087 [--only 01-project,13-dev-replay] [--fresh]
SIMPLEBASE_BACKUP_ENABLED=true ./examples/run_all.sh ...   # 11-backup 需要开备份，否则 SKIP

# 运维 / 压测
./build.sh reset [-y]                                      # 删本地 cache_dir，不动远程 S3
./simplebase reset --confirm=<instance_id> [--local-only] [--dry-run]
go run ./cmd/perfbench -suite all -c 32 -n 2000            # suite: http|db|kv|all；产物落 output/perf/
```

验收口径：后端 build / vet / test 通过；handler 测试用 fake service，新增写路径补「系统库拒绝」用例；`gofunction` 不依赖根模块，只允许 `golang.org/x/tools` 与 `json-iterator`；前端 typecheck 0 错误且覆盖率达阈值；涉及权限、删库、dev / skill 时跑端到端验证。

## 6. 计划与变更（OpenSpec）

新功能、重构或行为变化一律走 OpenSpec（`proposal → specs → design → tasks`），不新建 `plan/` 目录。`openspec/specs/<capability>/spec.md` 是现状基线，先查再动手。

1. `openspec --version`；未安装用 `curl -fsSL https://openspec.dev/cli/install.sh | sh`。未初始化则 `openspec init`（已初始化过其他工具用 `openspec update`）。
2. `openspec new change <change-id> --description "…"`，`<change-id>` 用动词开头的 kebab-case。
3. 按 `openspec instructions --change <id> <artifact>` 依次产出 `proposal.md`、`specs/**`、`design.md`、`tasks.md`；纯重构 / 工具 / 文档类在 `.openspec.yaml` 设 `skip_specs: true`，不编造需求。
4. `openspec validate <change-id> --strict` 通过后按 `tasks.md` 实施，完成后 `openspec archive <change-id>`，delta 合入 `specs/`。

## 7. 提交前自查

1. 相关命令通过：build / vet / test / `gofmt` / 架构门禁 / 前端 `typecheck` + `test` + `build`。
2. 未引入新依赖，未改配置文件结构与锁文件。
3. 新路由挂了权限与 project context 中间件；写路由挂 `writeGate` 并检查 `writable`。
4. 日志 / 审计无密钥、无 SQL 参数、无 LLM 正文。
5. 新代码落在正确上下文，未绕过架构门禁；`api` 顶层文件数未无故增加。
6. 前端未违反 `ui/AGENTS.md` 的样式与组件约定，新状态类用的是 `data-[state=…]`。
7. 行为变化已有 OpenSpec 变更且 `--strict` 通过。
8. 动了 `internal/sbcli/dev*`、`sbdev.py`、`examples/*/simplebase.json` 或删库 / 权限逻辑时，额外跑 `go test ./internal/sbcli/ ./internal/app/ ./internal/auth/`、`test_sbdev.py` 与 `scripts/examples-verify/verify.sh`。

## 8. 环境与工具链

- 本地开发默认本地盘存储，无需 S3；`config.yaml` 缺失时 `./build.sh dev` 从 `config.example.yaml` 生成（gitignore）。API Key 在控制台「设置 → 连接」重置签发。引导账号 `simplebase2026` / `simplebase2026`，首次登录必须改密。
- `ui/` 用 Node 22.22+ 与 Yarn 4.17（`corepack prepare yarn@4.17.0 --activate`；`ui/.yarnrc.yml` 的 `approvedGitRepositories` 在 Yarn 4.9 及更早会报错），装依赖用 `yarn install --immutable`。
- `packages/js-sdk/` 是 Yarn Classic 1.22（lockfile v1），不要在此运行 Yarn 4 或 `npm install` 改写锁文件；用 `~/.cache/node/corepack/v1/yarn/1.22.22/bin/yarn install --frozen-lockfile`。
- 云镜像 PATH 上 `/exec-daemon/node`（v22.14.0）排在 nvm 之前，需让 nvm 的 Node ≥ 22.22 优先。
- `internal/web/dist` 是构建产物 embed 目录，不手工编辑；`output/perf/` 是压测原始数据，按需清理。
