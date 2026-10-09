# SimpleBase Agent 指南

面向 AI 编码代理（Codex / CodeBuddy 等）的项目级规范。子目录有更细的约束文件，修改对应区域前必须先读：

- [`internal/AGENTS.md`](internal/AGENTS.md) — 后端分层、系统库保护、API 协议
- [`ui/AGENTS.md`](ui/AGENTS.md) — 前端目录职责、API 层、样式定稿规范
- [`gofunction/README.md`](gofunction/README.md) — 解释器公共 API、能力边界、已注册包清单

## 项目概述

SimpleBase 是单写实例的云端数据库服务：DuckLake + S3 在线持久层做存储，Echo v4 提供 REST API，`go:embed` 内嵌 Vue 3 控制台，进程内 `gofunction` 解释器运行 Go 云函数，llmgateway 封装多供应商 LLM 与云 Agent。模块 `github.com/linkxzhou/SimpleBase`（Go 1.25，单 go.mod，gofunction 无独立模块）。

## 常用命令

在仓库根目录执行：

```bash
# ── 开发 ──────────────────────────────────────────────
./build.sh dev                  # 一键：后端 :8080 + Vite 前端 :5173（proxy /v1 /health /go）
./build.sh dev --no-open --api-port 8081   # 不开浏览器 / 指定后端端口

# ── 构建 ──────────────────────────────────────────────
./build.sh                      # 前端 yarn build → 同步 internal/web/dist → go build -o simplebased
./build.sh --skip-ui            # 仅编译后端（需先完整构建一次）
./build.sh --ui-only            # 仅构建前端并同步 embed 目录
go build ./internal/... ./cmd/...           # 快速编译检查

# ── 测试 ──────────────────────────────────────────────
go test ./...                               # 后端全量
go test ./gofunction/... -short             # 云函数解释器（-short 跳过外网 HTTP 用例）
go vet ./gofunction/...

cd ui && yarn test                          # 前端 Vitest（coverage 阈值 95%，jsdom）；当前 functions 94.24% 未达标，见 planv5.0 §7.2
cd ui && yarn build                         # 前端构建必须通过
cd ui && yarn typecheck                     # tsc --noEmit；当前因 *.vue 缺 shim 报 241 处 TS2307，见 planv5.0 §7.1
./scripts/test.sh [包...] [-race]           # 云 Agent 相关包一键后端测试（planv4.1 §5.5）
./scripts/smoke.sh                          # 构建 + vet + 云 Agent 全链路 + 前端构建
cd packages/js-sdk && npm run typecheck && npm test   # SDK

# ── 运维 / 压测 ───────────────────────────────────────
./build.sh reset [-y]                       # 删本地 cache_dir（不动远程 S3）
./simplebased reset --confirm=<instance_id> [--local-only] [--dry-run]
./build.sh perf [--http|--db|--kv|--all] [-c N] [-n N]    # 经 cmd/perfbench 压测
```

DevMode 种子 API Key：`sb_live_dev_key_12345`。`config.yaml` 不存在时 `./build.sh dev` 会从 `config.example.yaml` 自动生成本地副本（已 gitignore）。

## 代码结构与依赖方向

```text
cmd/simplebased → app → api → auth / catalog / database / objectstore
                             ↓
                       systemdb（常驻系统库连接）
                       cloudagent → llmgateway / catalog / systemdb
                       sandbox（唯一 import microsandbox SDK 的包）
                       cronjob → crontab / gofunction
                       usage / audit → catalog
```

- 依赖只能自上而下，**禁止反向 import**（如 `catalog` import `api`）。
- `app` 是唯一装配点：跨模块依赖在 `assembleDeps` 按序创建，`Close` 反向释放；新增模块必须挂进装配顺序并在失败路径回收。
- handler 不直接依赖具体实现，必须走 `api.Dependencies` 接口 + adapter。
- 对象键使用内部 UUID，**禁止**把用户可控字符串直接拼进 S3 键。

## 硬性约束

1. **系统库保护**：系统库（kind=system，默认 `simplebase-system`）禁止删除、禁止写入、只允许 SELECT；写路径 handler 前置 `IsSystemDatabase` 拒绝，`systemLeaseAdapter` 双保险。不得经 registry 的用户库 factory 打开系统库（CacheDir 不同会得到空 catalog）。
2. **配置只从 `config.yaml` + `SIMPLEBASE_*` 环境变量加载**（`internal/config`）：YAML 为非密钥权威来源，env 覆盖；不得引入其他配置来源；密钥不得落 YAML（`dev_mode: false` 时非空密钥直接拒绝）。
3. **审计与日志脱敏**：审计只记录 SHA-256 与元数据，不得输出 SQL 参数或 LLM 正文。
4. **依赖管理**：不新增第三方依赖、不升级版本、不改 `go.mod` / `package.json` / `tsconfig.json` / `vite.config.ts`，除非用户明确要求。
5. **SQL 边界在 `database/sqlguard`**：拒绝清单、只读检测、多语句与 NUL 拦截不得散落到 handler。
6. **写操作 handler 必须检查 `writable`**（实例只读时返回 `ErrWriterUnavailable`）。

## 代码风格

**Go**：遵循 Go 官方风格；错误统一走 `WriteError` + 领域错误（`catalog/errors.go` 风格），不得裸返回字符串；每个模块带单元测试。当前 `internal/` 有 37 个文件未过 `gofmt`（见 planv5.0 §8.1），改动这些文件时顺手格式化，不要一次性全仓重排。

**TypeScript / Vue**（`ui/` 与 `packages/js-sdk`）：

- 对象形状一律用 `interface` 定义，**不用 `type`**。
- 不写 `any` 落盘新代码；跨层契约集中在 `ui/src/services/types.ts`，后端 snake_case 经映射函数（`toProjectItem` 风格）转 camelCase，不透传 raw。`services/http-api.ts` 现存 35 处 `any`/`Record<string, any>`，只减不增。
- 样式写 template 的 Tailwind class，不写自定义 CSS 文件；提示用 `vue-sonner`，确认用 `ConfirmAction`，弹窗用 `SbModal`，空态用 `SbEmptyState`，分页用 `TablePager`。
- **样式钩子必须匹配 reka-ui 真实属性**：本仓库锁定 reka-ui 2.10.5，它只输出 `data-state`（`checked`/`unchecked`、`active`/`inactive`、`open`/`closed`）与 `data-disabled`/`data-highlighted`/`data-selected` 等，**不输出裸 `data-checked` / `data-unchecked` / `data-active` / `data-open` / `data-closed`**。写状态样式一律用 `data-[state=checked]:…` 形式；裸写法编译出的选择器永不命中（现有失效点见 planv5.0 §4.1）。
- `src/components/ui/` 视为本地 fork 的库代码，只增删组件不改风格约定；已删除的零引用组件（checkbox / drawer / dropdown-menu / pagination / radio-group）不得重新引入。
- 设备无关的组件测试**不要桩掉 `src/components/ui/` 下的交互组件**（尤其 `Switch`）。`src/test/helpers.ts` 里的 `Switch` 桩用的是已废弃的 `checked`/`update:checked` API，会让失效的开关在测试里假通过；需要时用 reka 真实的 `modelValue` 契约（见 planv5.0 §7.3）。
- 路由 redirect 保持现状（`/sql`→`/databases`、`/llm`→`/agents`、`/settings`→`/?settings=1` 等）；新页面必须在 `router/index.ts` 注册并配 `meta.title`。
- admin（系统）项目的只读判定唯一入口是 `stores/project.ts` 的 `isAdmin`；前端只读只是体验层，后端才是真正保护。

**gofunction**：公共 API 与限制以 `gofunction/README.md` 为准；`Executor.Compile` / `SetProfile` / `Runtime.Cleanup` 等是预留空实现，不要当已实现功能使用或宣传。

**设计计划文档的工作目录**：`plan/` 按版本切目录（`planv1.0` … `planv5.0`）。`planv4.1` 不是独立目录——v4.1 云 Agent 优化计划全文放在 `plan/planv4.0/cloud-agent-optimization-plan.md`，代码注释里的 `planv4.1 §X` / `BUG-0X` 均指向该文件，改动前请到那里查。

## 测试要求

- 后端：改动必须保持 `go build ./internal/... ./cmd/...` 与 `go test ./...` 通过；handler 测试用 fake service，不依赖真实 DuckDB/S3；**新增写路径必须补「系统库拒绝」用例**。新增/修改 `LLMService` 等跨层接口时，所有 fake 实现（`internal/api/*_test.go` 的 `fakeGW`、`fakeLLMSvc`）要同步补齐方法，否则整包编译失败。
- 前端：`yarn build` 与 `yarn test` 必须通过（coverage 语句/分支/functions 均 95% 阈值）；`src/components/ui/`、`*.d.ts`、`main.ts`、`types.ts`、`src/test/` 不计入覆盖。当前基线：statements 99.24% / branches 95.07% / functions **94.24%（未达标）**。
- **数据层组件不要用测试桩替换**：`Switch`、`Tabs` 之类承载真实交互语义的 `ui/` 组件被桩掉后，失效的状态类会假通过（见 §7.3）；测试断言应打在真实 DOM 属性/事件上，而不是桩的 emit 上。
- gofunction：默认 `go test ./gofunction/... -short`（外网 HTTP 用例仅在非 -short 下跑）。

## 提交前自查

1. 相关命令全部通过（见「常用命令」）。
2. 未引入新依赖、未改配置文件结构。
3. 新增路由已挂权限（`auth.Require`）与 project context 中间件；带 `:projectID` 的路由必须经 `projectContextMiddlewareEcho`。
4. **写路径必须同时检查 `writable`**：实例只读（`Config.Instance.Writable=false`）时返回 `ErrWriterUnavailable`/503。当前 `agent_handler`（CreateAgent/PatchAgent/DeleteAgent/CreateThread/PatchThread/DeleteThread）、`agent_schedule_handler`、`s3_handler` 写接口尚未检查，见 planv5.0 §8.2。
5. 日志/审计无密钥、无 SQL 参数、无 LLM 正文。
6. 前端改动未违反 `ui/AGENTS.md` 的样式定稿与组件约定，且新写的状态类用的是 reka 真实属性（`data-[state=…]`）。

## Cursor Cloud specific instructions

- 本地 `dev_mode` 不需要 S3。`./build.sh dev --no-open` 在缺少 `config.yaml` 时从 `config.example.yaml` 生成副本（gitignore）。种子 API Key 是 `sb_live_dev_key_12345`，项目 ID 是 `dev-shop`。控制台无 access token 时会弹出登录框；同一把种子 Key 仍可直接调 API。引导账号 `simplebase2026` / `simplebase2026`，首次登录必须改密。
- `ui/yarn.lock` 是 Yarn 4。`ui/.yarnrc.yml` 里的 `approvedGitRepositories` 会被 Yarn 4.9 及更早版本拒绝。在 `ui/` 使用 Yarn 4.17：`corepack prepare yarn@4.17.0 --activate`，然后 `yarn install --immutable`。Yarn Classic 1 会把这份 lockfile 重写成 v1 格式。
- `packages/js-sdk/yarn.lock` 是 Yarn Classic 1.22。不要在该目录运行 Yarn 4，否则会迁移 lockfile。使用 `~/.cache/node/corepack/v1/yarn/1.22.22/bin/yarn install --frozen-lockfile`。
- 默认 PATH 上的 `/exec-daemon/node` 是 v22.14.0，排在 nvm 之前。UI 依赖要求 Node `^22.22.2`。镜像里的 nvm Node v22.23.3 必须排在 `/exec-daemon` 前面；环境安装脚本把它链接到 `/usr/local/cargo/bin`。
- DuckDB 需要 CGO。镜像已有 gcc，编译与测试保持 `CGO_ENABLED=1`。

## 备注

- `plan/` 存放设计计划文档（路由注释中的 `planX §Y` 指向这些文件），改动行为前先查对应 plan。
- `internal/web/dist` 是前端构建产物的 embed 目录，**不要手工编辑**；由 `./build.sh` 同步生成。
- `output/perf/` 为压测原始数据，`examples/`、`docs/` 为文档与示例资源。
- 仓库卫生（待处理，见 planv5.0 §9.3）：`fakellm-server`（8.5MB 构建产物）未被 `.gitignore` 覆盖；`scripts/`、`internal/testutil/`、`ui/src/components/agent/`、`ui/src/composables/useAgentConversation.ts` 及 v4.1 的 e2e 测试仍为未跟踪状态。
