# 云助手 Skill、标准 CLI 与流式执行

> **仓库**：SimpleBase（https://github.com/linkxzhou/SimpleBase）  
> **状态**：只提交计划，本分支不写实现。  
> **日期**：2026-10-09  
> **Verified against**：`main` @ `a7697ad`。路径、路由、权限位与工具名按该提交核对。  
> **关联**：[`internal/AGENTS.md`](../../internal/AGENTS.md)（cloudagent 不得直写存储、不得向沙盒注入凭据）、[`plan/planv4.0/cloud-agent-optimization-plan.md`](../planv4.0/cloud-agent-optimization-plan.md)（现有 SSE 与 function calling）、[`plan/planv5.0/cloud-assistant-sql-database-tabs-plan.md`](./cloud-assistant-sql-database-tabs-plan.md)（控制台称「云助手」）。

## 0. 目标

云助手今天只能读项目数据。本计划把它改成：**用一套与 REST API 一一对应的标准 CLI，在当前登录身份的权限内做增删改查**。模型不直接碰 DuckLake、S3 或系统库，只调用 CLI；CLI 再带短期 token 打现有 HTTP API。服务端鉴权、系统库只读、实例只读、SQL 防护保持为唯一授权边界。

五件事：

1. 提供整个 SimpleBase 的 skill，内含标准 CLI（Go，复用 `packages/go-sdk`）。
2. `@数据库`、`@云函数` 等只加载对应子 skill，不把全部写命令塞进上下文。
3. 模型文本和工具调用（参数、执行中、结果、错误）都在现有 SSE 上流式展示。
4. CLI、skill 路由、流式协议、前端 Vitest，以及「指令 → CLI → API」样例集，能在 CI 里跑。
5. 分阶段落地；本文件不包含实现代码。

## 1. 现状（已核对）

### 1.1 控制台云助手

入口是 `/console/agents`（`ui/src/pages/AgentManager.vue`）。侧栏工作台第一项，路由名 `agents`。页面是左栏 Agent 卡片 + 右栏对话：

| 文件 | 职责 |
| --- | --- |
| `ui/src/pages/AgentManager.vue` | 列表、新建/编辑、选中 Agent、把 `@` 候选传给 composer |
| `ui/src/components/agent/ConversationView.vue` | 消息流、工具卡、思考过程、错误与重试、流式光标 |
| `ui/src/components/agent/AgentToolCard.vue` | 工具名、参数 `<details>`、结果；`readonly_sql` 画表，`sandbox_*` 画出 stdout/stderr |
| `ui/src/components/agent/ThreadSwitcher.vue` | 会话切换、新建、删除 |
| `ui/src/components/agent/AgentComposer.vue` | 包一层 `AiChatComposer`，发送时带出 mentions |
| `ui/src/components/ai/AiChatComposer.vue` | 输入 `@` 后按 **Agent 名称** 过滤；发送体是 `{ agent_id }` |
| `ui/src/components/ai/AgentScheduleModal.vue` | 给某个 Agent 配定时执行 |
| `ui/src/composables/useAgentConversation.ts` | SSE 阶段：`thinking` / `streaming` / `tool` / `done` / `error` / `canceled` |

`@` 今天点的是助手，不是资源。`mentionsForSend` 在正文里找 `@` + Agent 名，再加上气泡里点过的项。后端 `resolveMentionedAgent`（`internal/api/agent_handler.go`）只用 `mentions[0].agent_id`；没有则回落到内置 `builtin_key=general`。

内置助手由 `internal/systemdb/agents.go` 的 `defaultCloudAgentSpecs` 播种：

| 名称 | builtin_key | module | 默认工具 |
| --- | --- | --- | --- |
| 通用助手 | `general` | `general` | 七个只读工具；沙盒可用时再加四个 `sandbox_*` |
| Database | `database` | `database` | `list_databases` / `list_collections` / `readonly_sql` |
| S3 | `s3` | `s3` | `list_objects` / `head_object` |
| Logs | `logs` | `logs` | `search_logs` / `log_level_stats` |

没有名为「数据库」「云函数」的内置助手。`sandbox` 是模块，不在这四条种子里。

`AgentManager.vue` 副标题写明：「工具默认只读，Sandbox 在云端隔离环境执行」。

### 1.2 后端工具、function calling、只读边界

运行时在 `internal/cloudagent`。`Runtime.StartRun` 用 eino `ChatModelAgent`，工具由 `buildTools`（`tools.go`）按 Agent 的 `tool_ids` 注册。

只读数据面（`access.go` 的接口注释就是 readonly）：

| 工具 id | 行为 |
| --- | --- |
| `list_databases` | 当前项目用户库 id / name / status |
| `list_collections` | `information_schema` 表名 |
| `readonly_sql` | 单条 SQL，`sqlguard.Validate(..., ReadOnly)`，最多 50 行 |
| `list_objects` / `head_object` | 项目相对键与元数据，不读对象正文 |
| `search_logs` / `log_level_stats` | `sys_log_events`，不改保留期 |

实现在 `internal/api/cloudagent_access.go`：查询走 `registry.Acquire(..., ReadOnly)`。系统提示（`modules.go` 的 `platformBasePrompt`）禁止 INSERT/UPDATE/DELETE/DROP 和对象上传。

沙盒工具 `sandbox_exec` / `sandbox_shell` / `sandbox_read_file` / `sandbox_write_file` 只作用于该 thread 的云沙盒 `/workspace`。`internal/AGENTS.md` 写明：`sandbox` 包禁止向沙盒注入 SimpleBase 凭据；`cloudagent` 禁止写 DuckLake / S3 / 系统库，禁止 import `sandbox` 或 microsandbox。沙盒能写的是微虚机里的文件，不是用户库。

原生 function calling 已经接通：`LLMTool` / `LLMToolCall`（`internal/api/router.go`）、`ChatToolCall`（`cloudagent/access.go`）、eino ToolsNode。一次 run 默认最多 8 轮工具、180 秒超时。工具结果卡片截断到 4000 字。历史里工具调用被收成摘要再送回模型（`historyToSchema`），不回放完整参数。

审计 `agent.run` 只记 run/thread/agent、耗时和 token 计数，不记用户正文或工具参数。`prompt.go` 的 `RedactSecrets` 会抹掉形如 `api_key=...` 的行。SQL 结果列本身没有按列名脱敏。

### 1.3 流式协议现状

`POST /v1/projects/:projectID/agent-threads/:threadID/runs`，body `{content, mentions, stream, retry_of_run_id}`。`stream: true` 时 `streamRun` 回 `text/event-stream`，帧格式是 `data: {json}\n\n`，另有 15 秒 `: ping` 注释心跳。

已有事件（`cloudagent.Event`）：

| type | 何时 |
| --- | --- |
| `run` | 开头，带 `run_id` |
| `token` | 模型文本增量（`consumeAssistant` 逐段 emit） |
| `thinking` | 超过 2 秒没有 token/工具输出时的心跳，带 `elapsed_ms` |
| `tool_call` | **整段**参数就绪后一次发出 `name` / `arguments` / `call_id` |
| `tool_progress` | 工具执行中每 2 秒，带 `call_id` 与 `elapsed_ms`（当前实现不填 `name`） |
| `tool_result` | 执行结束，`content` 已截断，可带 `truncated` |
| `usage` | 结束前的 token 与 `tool_calls` 计数 |
| `error` | `code` + `message` |
| `end` | `reason`：`stop` / `max_iterations` / `canceled` |

缺口（对照「参数、执行中、结果、错误都要流式」）：

- 参数不是流式的。网关内部有 `ToolCallDelta.ArgsDelta`（`LLMStreamChunk`），但 run 循环要等整条 assistant 消息拼完才发一个 `tool_call`。
- 没有「开始执行」帧。前端用「卡片还没有 content」推断执行中。
- `tool_result` 的 `IsError` 字段存在，运行时没有赋值；前端靠内容里有没有 `"error"` 猜。
- 取消已有：`POST /v1/projects/:projectID/agent-runs/:runID/cancel` 调 `Runtime.CancelRun`。前端 `stop()` 会 abort fetch 再调 cancel。断线没有 `id:` / `Last-Event-ID`，不能从序号续传。
- 非流式 `stream: false` 仍一次性返回 `{run, message}`。定时调度走这条，没有人在界面上看流。

### 1.4 REST 路由（Echo）

装配在 `internal/api/router.go` 的 `mountV1Routes` / `mountGoRoutes`。业务组先过 `AuthMiddleware`（JWT 或 API Key → `Principal`），带 `:projectID` 的再过 `projectContextMiddlewareEcho`。每条路由挂 `auth.Require(权限)`。错误体是 `{"error":{"code","message","request_id"}}`。

下表路径均相对于 `/v1/projects/:projectID`，除非另行注明。权限列是路由上的 `Require`；handler 里还有第二层检查。

**项目与身份**

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/v1/projects` | `database:read` | 当前身份可见项目 |
| POST | `/v1/projects` | `project:admin` | 建项目，并建 kind=kv 行 |
| POST/GET/DELETE | `.../api-keys` | 路由 `database:read`，签发在 handler `canIssue` | 创建响应带一次性 `secret` |
| GET | `/v1/auth/me` | 已登录 | |
| PUT | `/v1/auth/password` | 已登录 | 改自己的密码 |
| GET/POST/PATCH/DELETE | `/v1/users`、`/v1/users/:id` | handler：查看要 `admin` 或 `superadminl1`，写只要 `superadminl1` | 不在项目路径下 |

**数据库 / SQL / 表结构 / 集合**

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| POST/GET/GET/DELETE | `.../databases`、`.../databases/:databaseID` | 写与删除 `database:admin`；读 `database:read` |
| POST | `.../databases/:databaseID/query` | `database:read` |
| POST | `.../execute`、`.../batch` | `database:write` |
| GET | `.../schema`、`.../schema/tables/:table/rows` | `database:read` |
| POST | `.../schema/tables`、`.../schema/columns` | `database:write` |
| GET/POST | `.../data/collections` | 读 / `database:write` |
| GET | `.../data/collections/:collection` | `database:read` |
| POST/PUT/DELETE | `.../documents`、`.../documents/:id` | `database:write` |

系统库（`kind=system`，默认 `simplebase-system`）：删除走 `ErrSystemProtected`；`SQLHandler.Execute/Batch` 与四个文档写接口前置 `IsSystemDatabase`；`systemLeaseAdapter` 再拒一次写。查询只允许 SELECT 类（`sqlguard` 只读）。admin 项目前端只读是体验层，后端才是保护。

用户库 `data_model`：`collection`（默认）或 `sql`。集合 API 拒绝 SQL 库。表结构接口拒绝非 SQL 库。SQL 工作台两种库都能用，语句边界在 `database/sqlguard`（单语句、拒 ATTACH/COPY/PRAGMA 等）。

**KV**：`POST .../kv`，路由只要 `database:read`，写命令在 handler 里再要 `database:write`，实例不可写时 503 `writer_unavailable`。命令表在 `internal/api/kv_commands.go`（大小写不敏感），包括 GET/SET/DEL、Hash、List、Set、ZSet、EXPIRE、SCAN、RENAME 等。没有 FLUSHALL。

**对象存储**：`GET/POST/DELETE .../s3/objects`，`GET .../s3/presign`。上传要 `database:write`。`s3_handler` 目前没有检查实例 `writable`（`internal/AGENTS.md` 提交前自查第 4 条已记录）。

**云函数**

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET/POST | `.../gofunctions` | 读 / `database:write` |
| GET/PATCH/DELETE | `.../gofunctions/:name` | 读 / 写 |
| GET/POST | `.../versions` | 读 / 写 |
| GET | `.../versions/:ver` | 读，含源码 |
| POST | `.../activate`、`.../test` | `database:write` |
| POST | `/go/:projectID/:name/:functionName` | **`database:read`**（只跑 active 版本） |
| GET | 同上 | 405 JSON，避免掉进 SPA |

写与试跑在 `writable=false` 时 503。调用面故意是读权限。

**定时任务**：`GET/POST .../cron-jobs`，`GET/PATCH/DELETE .../cron-jobs/:jobID`，`GET .../runs`，`POST .../trigger`（202，要 `database:write`）。写路径已查 `writable`。

**云沙盒**：`GET .../sandboxes/capabilities` 始终在。启用后还有 list/create/get/patch/delete、start/stop、exec、`POST .../sandboxes/run`、文件 list/read/write/delete。写要 `database:write` 且查 `writable`。这是项目级沙盒 API，与助手 thread 内的 `sandbox_*` 工具不是同一条调用链。

**日志 / 设置 / 用量**

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `.../logs`、`.../logs/retention` | `database:read` |
| PUT | `.../logs/retention` | `project:admin` |
| GET/PUT | `.../settings` | 读 `database:read`，写 `project:admin` |
| GET/PUT | `/v1/settings` | `project:admin`（实例级） |
| GET | `.../metrics/summary`、`.../metrics/trend` | `database:read` |
| GET | `.../quota`、`.../audit` | `database:read` |
| GET/PUT | `.../llm/settings` | 读 / `project:admin` |
| GET/PUT/DELETE | `.../llm/providers/:provider` | 读脱敏；写凭据 `project:admin` |

**云助手自身**（仍在项目下）：modules、agents CRUD、threads CRUD、messages、runs、cancel、agent-schedules CRUD 与 trigger。`CreateRun` 只要 `database:read`。`PatchThread` 已查 `writable`；`CreateAgent` / `PatchAgent` / `DeleteAgent` / `CreateThread` / `DeleteThread` 以及 `agent_schedule_handler` 全部写接口仍未查实例 `writable`。

健康检查 `/health/live|ready`、`/metrics`、登录 `/v1/auth/login|refresh` 不在本 CLI 的业务面里。`PerfStageTiming` 才会挂 `/debug/pprof/*`，CLI 不暴露。

### 1.5 鉴权

`internal/auth/middleware.go` 的 `AuthMiddleware`：`Authorization: Bearer` 像 JWT（两段点）走会话；否则当 API Key（`sb_live_...`）。两者都注入 `auth.Principal`。

`Principal` 字段：`APIKeyID` 或登录态的 `UserID` / `Username` / `Role` / `SessionID` / `AccessJTI`，加上 `TenantID`、`ProjectIDs`、`Permissions`。日志只用 id，不落密钥原文。

权限位（`principal.go` / `role.go`）：

| 位 | 谁有 |
| --- | --- |
| `database:read` | super、admin、user；API Key 按签发时的集合 |
| `database:write` | super、user；**admin 角色没有** |
| `database:admin` | super、user |
| `llm:invoke` | 三个角色都有 |
| `project:admin` | super、user |
| `user:admin` | 仅 super |

`Role.CanWrite()`：`superadminl1` 与 `user` 可写，`admin` 全局只读。空 Role 表示 API Key 通道，不套角色，只看 Key 上的权限位。`CheckProjectAccess`：user 不能进系统项目；super / admin / 带 `project:admin` 的 Key 可以。跨项目是 `cross_project_denied`。

API Key 创建（`api_keys_handler.go`）只在响应里返回一次 `secret`。列表接口不回明文。

实例 `Config.Instance.Writable=false` 时，已接入的写 handler 返回 `writer_unavailable` / 503。上面点名的助手与 S3 写接口还没接上，CLI 若直接打它们会绕过实例只读。补齐是写路径阶段的验收项，不是另做一套鉴权。

### 1.6 两个 SDK 覆盖了什么

都在仓库根模块里，没有独立 `go.mod`。HTTP 都是 `Authorization: Bearer <key>`，客户端绑定一个 `projectID`。

**Go SDK** `packages/go-sdk`（标准库，`NewClient` 要求 `URL` + `APIKey` + `ProjectID`）：

| 已有 | 文件 |
| --- | --- |
| 数据库 list/get/create/delete、schema、建表、加列 | `databases.go` |
| query / execute / batch | `sql.go` |
| 集合与文档 CRUD | `documents.go` |
| 对象 list/upload/delete/presign | `storage.go` |
| 沙盒 capabilities、CRUD、start/stop、exec、run、文件 | `sandboxes.go` |
| `*APIError`：`Status` / `Code` / `Message` / `RequestID` | `errors.go` |

没有：KV、云函数、定时任务、日志、用户、项目列表、API Key、设置、配额、审计、云助手。`APIKey` 字段只是 Bearer 字符串，JWT 放进去也能发，但没有「短期委托 token」类型，也没有把 secret 从日志里剥掉的输出层。

**JS SDK** `packages/js-sdk`：`createClient` 导出 databases、sql、collections、storage、sandboxes，与 Go SDK 同一截断面。控制台不走这个包，走 `ui/src/services/http-api.ts`（该文件已有 KV、云函数、定时任务、日志、用户、助手流）。本计划的 CLI 只复用 Go SDK；不要求 JS SDK 补齐。

### 1.7 测试基线

- 后端助手流：`internal/api/agent_e2e_stream_test.go` 等，fake LLM，不打真实供应商。
- Go SDK：`go test ./packages/go-sdk`。
- 前端：`ui/vite.config.ts` 阈值 statements/lines/functions/branches 均为 95%。`AGENTS.md` 记录的基线里 functions 曾低于 95%，新代码必须自带测试，不能再拉低。
- 覆盖率排除 `src/components/ui/**`、`src/test/**`、`types.ts`、`main.ts`。交互组件不要桩掉。
- 没有 `cmd/simplebase`。现有二进制是 `cmd/simplebased`（服务）和 `cmd/perfbench`。

## 2. 决策

| # | 决策 | 含义 |
| --- | --- | --- |
| 1 | 写操作只经过现有 HTTP API | cloudagent 仍然禁止直写 DuckLake / S3 / 系统库。模型的手是 CLI，CLI 的手是 SDK |
| 2 | CLI 用 Go，放在 `cmd/simplebase` | 复用 `packages/go-sdk`。不引入 cobra 等新依赖，不改 `go.mod` |
| 3 | 助手进程内执行 CLI，不进云沙盒 | 沙盒继续禁止注入 SimpleBase 凭据。token 不进微虚机、不落盘 |
| 4 | 授权只认服务端 | CLI 不自己判断「能不能删库」。委托 token 的权限是调用方权限的子集 |
| 5 | 系统库只读不变 | 写与删除仍由现有 handler 返回 `system_database_protected` |
| 6 | `@` 增加资源 skill，与 `@助手名` 并存 | skill 决定本轮允许的 CLI 命令；助手决定人设。不点 skill 时不加载全部写命令 |
| 7 | 继续用 SSE，不改 WebSocket | 补参数增量、执行开始、确认、序号，便于断线续传 |
| 8 | 删除类必须在界面确认 | 普通写直接执行并展示。定时跑没有界面，破坏性命令直接失败 |
| 9 | 敏感列在 CLI 出口脱敏 | 模型上下文和 SSE 都看不到口令、token、API secret、DSN |
| 10 | 功能开关默认关 | `agent.skills_cli` 未开时，助手仍是今天的只读工具。验收通过后再开 |

## 3. CLI 设计

### 3.1 命令形状

```text
simplebase [--endpoint URL] [--token TOKEN] [--project ID] [--output json]
           <资源> <动作> [--flags]
```

资源与动作用空格分开，flags 在动作之后。禁止把用户字符串拼进 shell。助手调用时参数是 argv 数组，不是一条 shell 字符串：没有管道、重定向、`$(...)`、分号。

人读的 `--output text` 只给终端用户。助手与测试固定 `--output json`（也是默认值）。

成功时 stdout 一行 JSON（多行数据放在 `data` 里，不换多行 JSON，方便模型读）：

```json
{"ok":true,"data":{"id":"...","name":"shop"}}
```

失败时 stdout 为空，stderr 一行 JSON，进程退出码见 §3.4：

```json
{"ok":false,"error":{"code":"system_database_protected","message":"system database cannot be modified or deleted","http_status":403,"request_id":"..."}}
```

`code` 原样透传 API 的 `error.code`，不另起一套同义词。

### 3.2 配置：endpoint 与 token

优先级：命令行 flag > 环境变量 > 配置文件。

| 项 | flag | 环境变量 | 文件字段 |
| --- | --- | --- | --- |
| 服务地址 | `--endpoint` | `SIMPLEBASE_URL` | `endpoint` |
| 凭据 | `--token` | `SIMPLEBASE_TOKEN` | `token` |
| 项目 | `--project` | `SIMPLEBASE_PROJECT_ID` | `project_id` |
| 输出 | `--output` | `SIMPLEBASE_OUTPUT` | `output` |

文件路径：`$XDG_CONFIG_HOME/simplebase/config.json`，未设置时为 `~/.config/simplebase/config.json`。权限必须是 `0600`，否则 CLI 拒绝读取并退出 `config_insecure`。文件给本机操作者用。

助手路径不读、不写这个文件。宿主只通过子进程环境注入三个变量，用完即弃。配置里不出现第二种密钥来源；YAML 仍然禁止写密钥（现有 `internal/config` 规则）。

`SIMPLEBASE_URL` 只接受 `http` 或 `https` 的 origin，与 `gosdk.NewClient` 现有校验一致（无 userinfo、无 query）。助手场景固定为本进程监听地址，例如 `http://127.0.0.1:8080`，不走公网绕一圈。

### 3.3 语言与 SDK 补齐

`cmd/simplebase` 是薄命令表：解析 argv → 调 `packages/go-sdk` → 打印 §3.1 的信封。命令逻辑不复制 HTTP。

Go SDK 要补的方法（仍在 `packages/go-sdk`，标准库 HTTP）：

| 新文件（建议） | 方法覆盖的 API |
| --- | --- |
| `kv.go` | `POST /kv`，命令与参数原样 |
| `functions.go` | gofunctions CRUD、版本、activate、test，以及 `POST /go/:project/:name/:export` |
| `cron.go` | cron-jobs CRUD、runs、trigger |
| `logs.go` | 日志查询；retention 的读与写 |
| `users.go` | `/v1/users` CRUD。响应里丢掉 `password` |
| `projects.go` | 项目 list/create |
| `apikeys.go` | list/create/revoke。`Create` 的 `secret` 只出现在返回值，SDK 不打日志 |
| `settings.go` | 项目设置读/写，配额与审计只读 |
| `token.go` | `Options` 增加 `Token`，与 `APIKey` 二选一，都放进 Bearer。文档写明 JWT 与 API Key 同一头 |

`Options.APIKey` 保留，避免改坏现有调用。新代码用 `Token`。

JS SDK 不在本计划补齐。控制台继续用 `http-api.ts`。

### 3.4 退出码

| 退出码 | 名称 | 何时 |
| --- | --- | --- |
| 0 | ok | HTTP 2xx，信封 `ok:true` |
| 1 | usage | 缺子命令、未知 flag、参数类型不对。不发 HTTP |
| 2 | api | HTTP 非 2xx，`error.code` 为服务端原码 |
| 3 | confirmation_required | 仅助手宿主使用：破坏性命令已解析但未执行。人直接跑 CLI 时不会出现 |
| 4 | config | 缺 endpoint/token/project、URL 非法、配置文件权限不对 |

退出码 2 时模型只看 `error.code`。与现有 `internal/api/error.go` 对齐的常用码：

| code | HTTP | 助手该怎么说 |
| --- | --- | --- |
| `unauthenticated` / `invalid_api_key` / `invalid_or_expired_token` | 401 | token 失效，不要重试同一凭据 |
| `forbidden` / `role_forbidden` | 403 | 当前身份没有该权限 |
| `cross_project_denied` | 403 | 项目不属于该身份 |
| `system_database_protected` | 403 | 系统库不能改、不能删 |
| `writer_unavailable` | 503 | 实例只读 |
| `database_not_found` 等 not_found | 404 | 资源不存在 |
| `sql_not_allowed` / `write_in_read_only` / `multiple_statements` | 400 | SQL 被 sqlguard 拒绝 |
| `quota_exceeded` | 429 | 配额用尽 |
| `kv_unknown_command` / `kv_invalid_argument` | 400 | KV 命令不合法 |

CLI 自己的 `code`（不占用 HTTP）：`usage`、`config_missing`、`config_insecure`、`output_redacted`（仅表示结果被裁剪，退出码仍是 0）、`confirmation_required`。

### 3.5 与 API 的映射

全局 flag 不在表里重复。`:project` 来自 `--project`。凡是写操作，服务端原样检查权限、系统库、`writable`。

**数据库**（skill `database`）

| CLI | API | 权限 |
| --- | --- | --- |
| `database list [--limit N] [--cursor S]` | `GET .../databases` | read |
| `database get --id ID` | `GET .../databases/:id` | read |
| `database create --name NAME [--data-model collection\|sql] [--init-sql FILE]` | `POST .../databases` | admin |
| `database delete --id ID` | `DELETE .../databases/:id` | admin，**需确认** |
| `sql query --database ID --statement TEXT [--param JSON] [--max-rows N]` | `POST .../query` | read |
| `sql exec --database ID --statement TEXT [--param JSON]` | `POST .../execute` | write；DROP/DELETE/TRUNCATE 需确认 |
| `sql batch --database ID --file FILE [--transactional]` | `POST .../batch` | write；任一条是破坏性则整批确认 |
| `schema show --database ID` | `GET .../schema` | read |
| `schema rows --database ID --table NAME [--limit N]` | `GET .../schema/tables/:table/rows` | read |
| `schema create-table --database ID --name NAME --columns JSON` | `POST .../schema/tables` | write |
| `schema add-column --database ID --table NAME --column JSON` | `POST .../schema/columns` | write |
| `collection list\|create --database ID [--name NAME]` | 集合 list/create | read / write |
| `document list\|insert\|update\|delete` | 对应 documents 路由 | delete **需确认** |

`--init-sql` 读本地文件后放进现有创建请求体，不新开 API。SQL 文本来自 flag 或 stdin（`--statement -`），CLI 不拼接多语句。

**键值**（skill `kv`）

```text
simplebase kv exec --command GET --arg shop:name
simplebase kv exec --command SET --arg shop:name --arg demo
simplebase kv exec --command DEL --arg shop:name
```

对应 `POST .../kv`。命令名与 `kv_commands.go` 一致。`DEL` 以及会删键的 `SPOP`（弹出）里，`DEL` 需确认；`SPOP` 视为写但不确认（与单次弹出的数据量匹配，卡片里展示被弹出的成员）。未知命令不在 CLI 里预判，交给 API 的 `kv_unknown_command`。

**对象**（skill `s3`）

| CLI | API | 确认 |
| --- | --- | --- |
| `object list [--prefix P]` | `GET .../s3/objects` | |
| `object upload --key KEY --file PATH` | `POST .../s3/objects` | 否 |
| `object delete --key KEY` | `DELETE .../s3/objects` | 是 |
| `object presign --key KEY` | `GET .../s3/presign` | 否；URL 给用户卡片，模型上下文只留 key 与过期时间 |

**云函数**（skill `gofunction`）

| CLI | API | 确认 |
| --- | --- | --- |
| `function list\|get --name NAME` | GET | |
| `function create --name NAME --file PATH [--description S]` | POST | 否 |
| `function update --name NAME [--description S]` | PATCH | 否 |
| `function delete --name NAME` | DELETE | 是 |
| `function version list\|get\|create\|activate` | versions 路由 | activate 否 |
| `function test --name NAME --version N --export Export [--input JSON]` | `.../test` | 否 |
| `function invoke --name NAME --export Export [--input JSON]` | `POST /go/:project/:name/:export` | 否（与今天读权限调用面一致） |

源码从 `--file` 读入，上限沿用 handler 的 256KiB。invoke 的响应可能含函数 stdout，按 §3.6 脱敏后再进模型。

**定时任务**（skill `cron`）

| CLI | API | 确认 |
| --- | --- | --- |
| `cron list\|get\|runs` | GET | |
| `cron create` / `cron update` | POST / PATCH | 否 |
| `cron delete --id ID` | DELETE | 是 |
| `cron trigger --id ID` | POST trigger | 是（会立刻跑云函数） |

create/update 的 flag：`--name`、`--schedule-kind cron\|interval\|once`、`--cron`、`--interval-seconds`、`--run-at`、`--func-file`、`--func-export`、`--input JSON`、`--enabled`。校验仍在服务端（名称正则、间隔 60 秒到 30 天）。

**沙盒**（skill `sandbox`，这是项目沙盒 API，不是 thread 内 `sandbox_*`）

`sandbox capabilities|list|get|create|update|start|stop|exec|run|files` 对上 `router.go` 里 `/sandboxes` 各路由。`sandbox delete` 需确认。`exec` / `run` / 写文件不确认，但工具卡展示命令与截断后的输出。未启用时只有 `capabilities` 能成功，其余保持 API 的 404/503。

**日志**（skill `logs`）

`log search [--level L] [--q S] [--limit N]` → `GET .../logs`。`log retention get|set` → retention 路由。`set` 要 `project:admin`，且需确认（改保留期会删历史日志）。

**用户**（skill `users`）

`user list|get|create|update|disable|delete` → `/v1/users`。create/update 的密码只从 `--password-stdin` 读取，不接受放在 argv 里的密码（避免进进程列表）。响应与工具结果都不含密码。delete / disable 需确认。非 super 调用写命令时期望 `role_forbidden`，CLI 不在本地放行。

**项目**（skill `project`）

`project list`、`project create --name NAME`、`apikey list|create|revoke`、`settings get|set`、`quota get`、`audit list`。`apikey create` 的 secret 见 §3.6。`apikey revoke` 需确认。`settings set` 需确认。

### 3.6 越权、系统库、脱敏

CLI 不做第二套 ACL。每次请求都是带 Bearer 的普通 API 调用，经过 `AuthMiddleware`、`Require`、项目归属、handler 里的系统库判断和 `writable`。

委托 token（仅助手子进程使用，见 §4.4）额外限制：

- 权限位 ⊆ 当前 `Principal` 的权限位。admin 角色没有 `database:write`，委托 token 也没有。
- `project_id` 固定为本轮对话的项目。`--project` 与 token 不一致 → CLI 退出码 4，`code=project_mismatch`，不发 HTTP。
- 不能当 refresh token，不能换 API Key，不能调用 `PUT /auth/password`。
- 有效期到点后服务端 401 `invalid_or_expired_token`。

系统库：不在 CLI 里写死库名拦截。用户对 admin 项目执行 `database delete` 或 `sql exec` 时，现有 handler 返回 `system_database_protected` 或 `write_in_read_only`。只读 `sql query` 在系统库上保持今天的 SELECT 能力。测试必须覆盖这条，防止有人在 CLI 里开旁路。

实例只读：写命令打到尚未检查 `writable` 的 handler 时，行为会和「实例只读」不一致。阶段 2 在这些 handler 补上检查后再允许 CLI 暴露它们：

- `cloudAgentHandler` 的 Create/Patch/Delete Agent、Create/Delete Thread
- `agentScheduleHandler` 全部写接口
- `S3Handler` 的 Upload 与 Delete

脱敏在 CLI 打印前做，SDK 仍返回完整结构给进程内调用方（人机直接跑 CLI 时，API Key 的一次性 secret 需要给人看）。规则：

| 输出 | 助手子进程（环境变量 `SIMPLEBASE_AGENT_RUN=1`） | 人直接跑 |
| --- | --- | --- |
| 列名匹配 `(?i)password\|passwd\|secret\|api_key\|access_key\|token\|credential\|dsn\|authorization` 的 SQL/文档字段 | 值换成 `"***"` | 同样换成 `"***"` |
| `apikey create` 的 `secret` | 不出现；`data.secret_withheld=true`，只留 `id` 与权限 | stdout 打印一次，stderr 不打印 |
| 用户密码、LLM provider 凭据正文 | CLI 不提供读取明文的命令 | 同左 |
| presign URL | 模型只看 key 与 `expires_at`；完整 URL 由 SSE 的用户可见卡片单独带，不进入下一轮模型上下文 | stdout 含 URL |
| 请求日志 | 沿用服务端：不记 SQL 参数、不记 LLM 正文、不记 token | CLI 自己不写日志文件 |

`SIMPLEBASE_AGENT_RUN=1` 由宿主设置，用户在 shell 里自行设置不能借此绕过服务端（只影响 CLI 打印）。真正的秘密仍然不进审计。

下列 API **不**做成 CLI 命令：`/v1/auth/login|refresh|logout|password`、LLM provider 凭据的 PUT/DELETE、`/llm/chat|stream`、助手 runs（防止递归）、`/debug/pprof`、`/metrics` 的 Prometheus 原文。读 LLM 设置可以留在 `project` skill，响应沿用现有脱敏视图。

### 3.7 分发

`./build.sh` 在产出 `simplebased` 的同时产出同目录的 `simplebase`。助手宿主用配置 `agent.cli_path`（空则取 `simplebased` 的同目录）。测试用同一二进制打 `httptest`。不从网络下载 CLI，不把二进制复制进沙盒镜像。

## 4. Skill、`@` 路由与执行

### 4.1 目录

源码树（实现阶段新增，本 PR 只有本计划）：

```text
skills/simplebase/SKILL.md                 顶层：CLI 用法、退出码、禁止事项、子 skill 索引
skills/simplebase/database/SKILL.md
skills/simplebase/kv/SKILL.md
skills/simplebase/s3/SKILL.md
skills/simplebase/gofunction/SKILL.md
skills/simplebase/cron/SKILL.md
skills/simplebase/sandbox/SKILL.md
skills/simplebase/logs/SKILL.md
skills/simplebase/users/SKILL.md
skills/simplebase/project/SKILL.md
```

每个子目录的 `SKILL.md` 只描述该资源的命令、flag、哪条要确认、典型错误码。不复制 handler 源码。顶层文件用一段索引列出 id 与中文别名，并写明：没有被 `@` 到的命令不要编造。

`internal/cloudagent` 用 `go:embed` 打进二进制，避免运行时依赖工作目录。embed 的是 Markdown，不是凭据。

每个子 skill 在代码里还有一份允许的 argv 前缀（例如 database skill 允许 `database`、`sql`、`schema`、`collection`、`document`）。Markdown 给人看，前缀表给宿主做拒绝。两者不一致时以前缀表为准，测试锁住两边。

### 4.2 `@` 怎么解析

composer 现有逻辑保留：`@` + **助手名称** → `{agent_id}`，仍走 `mentions`。

新增资源别名表（大小写不敏感，中文精确匹配）：

| skill id | 输入里可写 |
| --- | --- |
| `database` | `@数据库` `@database` `@db` |
| `kv` | `@键值` `@kv` |
| `s3` | `@对象存储` `@s3` `@对象` |
| `gofunction` | `@云函数` `@函数` `@gofunction` |
| `cron` | `@定时任务` `@定时` `@cron` |
| `sandbox` | `@沙盒` `@sandbox` |
| `logs` | `@日志` `@logs` |
| `users` | `@用户` `@users` |
| `project` | `@项目` `@project` |

冲突规则：token 先查别名表，命中则是 skill；否则按助手名匹配。内置助手叫 `Database` / `S3` / `Logs`，与中文别名不冲突。若用户把自定义助手也起名为「数据库」，别名表优先，该气泡算 skill，不切换助手；切换助手仍用左侧卡片或完整助手名（与别名不完全相同的名字）。

`AiChatComposer` 的弹出列表分两组：「助手」「能力」。发送体扩展为：

```json
{
  "content": "给 @数据库 建一张 orders 表",
  "mentions": [{ "agent_id": "..." }],
  "skills": ["database"],
  "stream": true
}
```

`skills` 是本轮要加载的 id，去重、保持出现顺序。后端不信任正文里的 `@` 字符串单独开权限：以 `skills` 数组为准，并再用别名表校验每个 id 属于已知 skill。正文里的 `@数据库` 只用于回放。

多个 skill 取并集。`@数据库 @云函数` 加载这两份，不加载 KV。未出现的 id 不进系统提示，也不进允许前缀。

没写 skill 时：

- 选中的助手 module 是 `database` / `s3` / `logs` / `sandbox`：本轮只加载同名 skill（这是「点了数据库助手」的等价物）。`sandbox` 模块只加载沙盒 skill，不加数据写命令。
- module 是 `general` 或自定义模块：只注入顶层 `SKILL.md` 的索引，**零个**写命令前缀。模型应告诉用户用 `@数据库` 这类点名。这样满足「不点名就不加载子 skill」。

`users` 即使被点名，token 没有 `user:admin` 时仍然会 403。skill 文本里写明这一点，避免模型反复重试。

### 4.3 加载机制

`CreateRun` 在组系统提示时：

1. 平台规则（改写 `platformBasePrompt`：允许通过 CLI 写用户资源；仍禁止编造结果、禁止输出密钥、禁止要求用户粘贴 token）。
2. 助手自己的 `system_prompt`（人设）。
3. 顶层 skill 索引，或并集后的子 skill 全文。单 skill 正文超过 8KiB 时截断并留下命令清单（防止把源码示例撑爆上下文）。
4. 项目 id、助手 id、本轮 skill id 列表。不再为了「方便」把全部库表快照塞进 general；database skill 可以保留今天的只读库名快照（最多 20 条，已有 `buildSnapshot`）。

工具只注册一个数据面工具，加上既有沙盒四件套（若该助手本来就有且沙盒可用）：

```text
工具名 simplebase
参数 { "argv": ["database", "list"] }
```

`argv[0]` 必须落在本轮前缀表内，否则工具结果是 `{"ok":false,"error":{"code":"skill_not_loaded"}}`，不启动子进程。这是提示注入之外的硬限制。

`agent.skills_cli=false` 时不注册该工具，继续 `buildTools` 的只读工具。两套不要同时给模型，否则它会混用 `readonly_sql` 和 `sql exec`。

### 4.4 跑在哪、token 怎么注入

**跑在 simplebased 进程里的子进程，不跑在云沙盒。**

理由：`sandbox` 的包约束是不得注入 SimpleBase 凭据；沙盒网络也不该成为第二套控制面。thread 内 `sandbox_*` 继续管 `/workspace` 里的代码和文件，与 CLI 并列，互不调用。

一次工具调用：

1. 宿主校验 argv 前缀、长度（建议 argv 最多 64 段、单段 32KiB、合计 256KiB）和「是否破坏性」（§4.5）。
2. 破坏性且没有确认令牌 → 发 SSE `confirmation_required`，**不**起进程。
3. 通过后 `exec.CommandContext(ctx, cliPath, argv...)`，环境只保留：
   - `SIMPLEBASE_URL` = 本进程 loopback
   - `SIMPLEBASE_TOKEN` = 本轮委托 token
   - `SIMPLEBASE_PROJECT_ID`
   - `SIMPLEBASE_OUTPUT=json`
   - `SIMPLEBASE_AGENT_RUN=1`
   - `PATH` 取最小集（CLI 是静态参数，不需要用户 PATH 里的解释器）
4. 不继承宿主的 `SIMPLEBASE_*` 配置、S3 密钥、LLM 密钥。stdout/stderr 各限 64KiB，超时 60 秒（`function test` / `sandbox run` 用 120 秒，与 API 自身超时对齐）。超时杀进程，工具结果 `code=cli_timeout`。
5. 退出码映射成工具 JSON。退出码 2 把 stderr 信封交给模型。

委托 token：

| 项 | 值 |
| --- | --- |
| 形态 | 内存签发的 HMAC JWT（沿用现有 access JWT 的库与配置密钥，不新增依赖）。`aud=simplebase-cli`，`typ=agent_delegation` |
| 主体 | 当前 Principal：登录态带 `UserID`+`Role`，API Key 态带 `APIKeyID` 与原权限位 |
| 作用域 | 权限 ⊆ 调用方；项目固定；不能刷新、不能改密、不能写 LLM 凭据 |
| 有效期 | 15 分钟，或 run 结束/取消时先失效（取先到者）。配置项 `agent.delegation_ttl`，默认 `15m`，上限 `30m` |
| 绑定 | `jti` 关联 `run_id`。run 结束后 `jti` 进入进程内作废表，直到原 exp |
| 存放 | 只在子进程环境里。不写 `sys_*`，不写配置文件，不进 SSE，不进审计。工具参数回显里如果模型把 token 写进 argv，宿主拒绝该次调用（`code=token_in_argv`） |

作废表是进程内存。单写实例没有第二副本，与「单写实例」的部署假设一致。进程重启后旧 token 的签名密钥不变，所以 JWT 的 exp 必须短；重启不恢复作废表，最坏窗口是剩余 TTL，可接受。

API Key 用户在控制台里发消息时，委托 token 复制该 Key 的权限位与项目集合，而不是升级成 super。

### 4.5 写操作与破坏性确认

三类：

| 类 | 行为 | 例子 |
| --- | --- | --- |
| 读 | 立刻执行 | list/get/query、`log search`、`function get` |
| 写 | 立刻执行，卡片展示 argv 与结果 | create、insert、update、upload、`sql exec` 的 INSERT/UPDATE/CREATE、`function invoke`、`schema add-column` |
| 破坏 | 先在界面确认，取消或超时则不执行 | 下表 |

破坏性清单（宿主按 argv 判定，不靠模型自觉）：

- `database delete`
- `document delete`
- `object delete`
- `function delete`
- `cron delete`、`cron trigger`
- `sandbox delete`
- `user delete`、`user disable`
- `apikey revoke`
- `log retention set`、`settings set`
- `kv exec` 且命令为 `DEL`
- `sql exec` / `sql batch` / `database create --init-sql` 中，语句首关键字为 `DROP`、`DELETE`、`TRUNCATE`，或 `ALTER` 且含 `DROP`。分类调用 `sqlguard` 现有只读/写入判断，不在 CLI 再写一套 SQL 解析器；拿不准的语句当成破坏性

确认协议：

1. 工具尚未执行，SSE 发 `confirmation_required`（§5），payload 含 `call_id`、展示用 argv（已脱敏）、一句中文说明。
2. 前端用现有 `ConfirmAction` 弹窗，原样显示命令。
3. 用户点确认 → `POST /v1/projects/:projectID/agent-runs/:runID/confirmations/:callID`，body `{ "approve": true }`。宿主校验该 call 属于这个 run、调用者是同一 Principal，然后才起 CLI。
4. 取消、关闭、或超过 `agent.confirm_timeout`（默认 2 分钟）→ `approve: false`，工具结果 `code=confirmation_denied`，模型用用户语言说明已取消。默认拒绝。
5. 定时调度（`AgentScheduler`、没有浏览器）遇到破坏性命令：不挂起，直接 `code=confirmation_unavailable`。调度配置不提供「自动批准删除」。

普通写不弹窗。用户仍能在工具卡里看到命令；下一阶段若要「所有写都确认」，只需把写类并入同一事件，本协议不用改。

同一 run 里多条破坏性命令逐条确认，不合并成一次「全部允许」。

## 5. 流式输出

### 5.1 为什么继续 SSE

控制台已经用 `fetch` 读 `text/event-stream`（`streamAgentRun`）。代理、鉴权头、取消都按这条链路接好。WebSocket 要新的升级与鉴权，不解决「工具参数没逐字出来」的问题。本计划只扩展事件，不换传输。

帧格式在现有 `data: {json}\n\n` 上增加 SSE `id:`，值为该 run 内单调整数。注释心跳 `: ping` 保留，不占 id。

### 5.2 事件

沿用 `cloudagent.Event`，补字段时用 `omitempty`，旧前端忽略未知 type。

| type | 新增？ | 字段 | 前端 |
| --- | --- | --- | --- |
| `run` | 已有 | `run_id` | 记下 run，供取消与续传 |
| `token` | 已有 | `content` 增量 | 追加到助手气泡 |
| `thinking` | 已有 | `elapsed_ms`，可选 `content` | 工具执行期间不要把阶段打回 thinking（现有逻辑保持） |
| `tool_call_delta` | 新 | `call_id`，`name` 仅首包，`arguments` 为增量 | 卡片进入「生成参数」，参数区追加 |
| `tool_call` | 已有 | 完整 `name`/`arguments`/`call_id` | 参数收束，状态「待执行」 |
| `tool_start` | 新 | `call_id`，`name` | 状态「执行中」 |
| `tool_progress` | 已有 | 补上 `name` | 显示已运行秒数 |
| `confirmation_required` | 新 | `call_id`，`argv`（展示用），`message` | 打开 `ConfirmAction` |
| `confirmation_resolved` | 新 | `call_id`，`approve` | 关掉弹窗 |
| `tool_result` | 已有 | 补 `is_error`；`content` 为 CLI 信封 | 成功或失败；可折叠 |
| `usage` | 已有 | token 与 `tool_calls` | 状态栏 |
| `error` | 已有 | `code`，`message` | 气泡错误 + 可重试 |
| `end` | 已有 | `reason` | `stop` / `canceled` / `max_iterations` |

`tool_call_delta` 的实现点：`Runtime.StartRun` 在消费 assistant 流时，若 delta 带 `ToolCallDelta`，立刻 emit，不要等 `msg.ToolCalls` 拼完。拼完后再发一条 `tool_call` 作为收束（前端以收束为准覆盖参数，避免增量重复拼接错）。今天 `streamRun` 只在 `token`/`tool_call`/`tool_result` 时刷新空闲计时，需要把 `tool_call_delta` 和 `tool_start` 算进「有输出」，免得参数生成过程中误发 `thinking`。

工具执行在 `tool_call` 之后、子进程启动时发 `tool_start`。失败（非零退出、超时、skill 未加载、确认被拒）走 `tool_result` 且 `is_error: true`，不把 CLI 失败升级成整轮 `error`。整轮 `error` 仍只用于模型不可用、配额、保存失败。这样一条 SQL 被拒时，模型还能继续解释。

### 5.3 后端改造（实现阶段）

| 位置 | 改动 |
| --- | --- |
| `internal/cloudagent/runtime.go` | 转发参数增量；`tool_start`；`tool_result.is_error`；事件序号 |
| `internal/cloudagent/tools.go` | `simplebase` 工具；确认挂起用 run 级 channel，取消时关闭 |
| `internal/api/agent_handler.go` | `createRunBody.Skills`；`streamRun` 写 `id:`；确认路由；续传路由 |
| `internal/api/router.go` | `POST .../agent-runs/:runID/confirmations/:callID`（`database:read`，与发消息相同）；`GET .../agent-runs/:runID/events` |
| `internal/llmgateway` | 确认工具参数增量已经能从供应商流里解析；缺的供应商在适配层补齐，没有增量时退化为今天的整段 `tool_call` |
| `internal/config` | `agent.skills_cli`、`agent.cli_path`、`agent.delegation_ttl`、`agent.confirm_timeout`。只来自 YAML + `SIMPLEBASE_` 前缀 |

续传：`GET .../events?after=N` 要求同一项目、同一 Principal 可读该 run。先回放内存环里 `id>N` 的事件，run 仍在进行则接上实时帧。环只保留该 run（上限例如 2000 条），run 结束后再留 2 分钟供刷新。不把环写入系统库（避免 LLM 正文落库；最终助手消息仍按今天的方式只存截断后的正文和工具卡）。

`after` 已经超过环的起点 → `error` `code=stream_gap`，前端改为拉 `GET .../messages` 恢复已落库部分，并提示中间增量丢失。

### 5.4 ConversationView

`useAgentConversation.ts` 增加阶段 `awaiting_confirm`，但不要在工具执行时掉回 `thinking`。

`AgentToolCard.vue`：

- 状态文案：生成参数 / 待执行 / 等待确认 / 执行中 / 成功 / 失败。用 `data-[state=...]` 只在真正的 reka 组件上；卡片本身用普通 class 或 `data-status`，不要发明 reka 没有的 `data-open`。
- 参数区默认折叠；`tool_call_delta` 期间展开，方便看到正在生成的 JSON。结果默认：短于 20 行展开，否则折叠（与现有 `<details open>` 条件一致）。
- 失败用现有 `border-destructive` 与「失败」徽标，依据 `is_error`，不再只靠猜 JSON。
- CLI 信封的 `data` 用 `<pre>` 展示格式化 JSON。`sql query` 若 `data.columns` 存在，沿用现在的表。
- 等待确认时卡片内嵌只读命令；弹窗用页面级 `ConfirmAction`，不在卡片里再做一套按钮样式。

`ConversationView` 的光标 `▍` 在 `sending` 且最后一条是助手时保留。确认弹窗打开时停止光标闪烁（阶段不是 streaming）。

取消：composer 的停止按钮保持「abort fetch + `cancel`」。abort 后若 2 秒内没收到 `end`，再 `GET events?after=` 一次；若 run 已是 canceled，本地阶段改 `canceled`。不要只 abort 而不 cancel，否则服务端子进程还会跑完。

断线：`streamAgentRun` 在网络错误且已有 `run_id`、尚未 `end` 时，最多重连 3 次，间隔 1s/2s/4s，带上最后看到的 SSE id。用户点了停止则不重连。

非流式定时跑不画卡片。调度器把工具卡 JSON 照旧写入助手消息，用户稍后打开会话能看到，包括 `confirmation_unavailable`。

## 6. 测试

### 6.1 CLI 单测（不听网）

`cmd/simplebase` 与 `packages/go-sdk`：

- flag 优先级：flag 覆盖环境变量，环境变量覆盖文件。
- 配置文件权限不是 `0600` → 退出码 4。
- URL 带 userinfo → 拒绝。
- `--output json` 成功信封与失败信封。
- 列名脱敏、`SIMPLEBASE_AGENT_RUN=1` 时 secret 被扣下、人机模式 secret 只在 stdout。
- argv 含 `SIMPLEBASE_TOKEN` 字样时由宿主拒绝（宿主单测，不在 CLI 进程里）。
- 未知子命令退出码 1，不发 HTTP（`httptest` 计数为 0）。

### 6.2 CLI 集成（每类资源一轮增删改查）

用现有 handler + `httptest`，不依赖 DuckDB/S3 的测试沿用 fake；需要真实 SQL 的用例放在已有数据库测试的 build tag 或 `-short` 约定下：默认 `go test ./cmd/simplebase/...` 使用 fake catalog / fake SQL，与 `data_handler_test.go` 相同风格。

每个资源至少一条：

| 资源 | 序列 | 必须断言的拒绝 |
| --- | --- | --- |
| database | create → get → list → delete | 系统库 delete → `system_database_protected`；admin 角色 create → 403；`writable=false` → `writer_unavailable` |
| schema + sql | create sql 库 → create-table → add-column → query → exec INSERT → exec DELETE 在未确认时宿主不执行 | `sql exec` 多语句 → `multiple_statements`；系统库 exec → 拒绝 |
| document | create collection → insert → update → list → delete | SQL 形态库调用 collection → 现有 4xx |
| kv | SET → GET → DEL | 无 write 权限的 SET → 403；未知命令 → `kv_unknown_command` |
| object | upload → list → presign → delete | 助手模式 presign 信封不含完整 URL；`writable=false` 的 upload/delete → 503（补上检查之后） |
| function | create → version get → activate → invoke → test → delete | invoke 用只读身份仍 200（与 `/go` 一致）；delete 系统外的名字 404 |
| cron | create → get → trigger → runs → delete | 间隔小于 60 秒被 API 拒绝 |
| sandbox | capabilities；fake driver 下 create → exec → delete | 未配置时非 capabilities 失败 |
| log | search；retention set 要 project admin | 普通 user set → 403 |
| user | super 创建与删除；admin 角色 list 200、create 403 | 密码不出现在 stdout |
| project / apikey | list；create key 在助手模式无 secret；revoke | Key 权限不能超过签发者 |

破坏性命令的「未确认不发 HTTP」在宿主测试里断言，不在 CLI 集成里（CLI 被人直接调用时会真的发 DELETE）。

### 6.3 Skill 路由

`internal/cloudagent` 表驱动：

- `@数据库` → skills `[database]`，允许 `sql exec`，拒绝 `function delete`（`skill_not_loaded`）。
- `@云函数` → 只允许 `function *`。
- `@数据库 @云函数` → 并集。
- 无 skill 且助手是 general → 允许前缀为空，`database list` 被拒。
- 无 skill 且助手 module=database → 等价于只加载 database。
- `@Database`（助手名）不误判成 skill。
- 自定义助手名为「数据库」时，别名优先为 skill。
- `skills` 数组里的未知 id → 400，不启动 run。
- Markdown 前缀表与代码前缀表一致（测试读 embed 的一级标题或旁边的清单段落）。

### 6.4 流式协议

扩展 `internal/api/agent_e2e_stream_test.go`（fake LLM）：

- 文本增量逐帧 `token`，且每帧有递增 `id`。
- 工具参数先若干 `tool_call_delta`，再一条完整 `tool_call`，然后 `tool_start`，再 `tool_result`。
- 工具失败：`is_error=true`，其后仍可有 `token` 与 `end`，整轮不是 `error`。
- 破坏性 argv：出现 `confirmation_required` 且 httptest 上对应 DELETE 次数为 0；POST approve 之后次数为 1；超时则为 0 且 `confirmation_denied`。
- `cancel` 在 `tool_start` 之后：子进程被 context 取消，`end.reason=canceled`。
- `GET events?after=` 能补齐断开之后的帧；`after` 过旧返回 `stream_gap`。
- 心跳注释不打乱 JSON 解析。

### 6.5 前端 Vitest

新测试打在真实 DOM 上（`data-status`、按钮、`ConfirmAction`），不桩 `src/components/ui/` 的交互组件。

| 用例 | 断言 |
| --- | --- |
| `tool_call_delta` 再 `tool_call` | 参数文本从增量变成完整 JSON，不重复拼接 |
| `tool_start` 无 content | 卡片为执行中 |
| `tool_result` `is_error` | 失败徽标 |
| `confirmation_required` | `ConfirmAction` 出现且含 argv；确认后调用 confirm API；取消后阶段离开 `awaiting_confirm` |
| 停止 | 调用 `cancel`，阶段 `canceled` |
| 断线重连 | mock fetch 第一次中断、第二次用最后 id 拉 events |
| `@云函数` | 发送体 `skills: ["gofunction"]`，且不把别名当成 agent_id |
| `@` + 助手名 | 仍然只填 `mentions`，`skills` 为空 |

`yarn test` 保持 95% 阈值。新文件的函数都要被用例走到。`yarn build` 必须通过。

### 6.6 端到端样例集（助手指令 → CLI → API）

放在 `internal/api/agent_skills_e2e_test.go`（名称实现时再定）。fake LLM 不调用外网：脚本按用户句子返回固定的 `simplebase` argv。断言 CLI 打到 httptest 的路径、方法、状态码，以及 SSE 顺序。

| # | 用户句子（skills） | 模型 argv | 期望 API | 确认 |
| --- | --- | --- | --- | --- |
| 1 | 「建一个名为 shop 的 SQL 库」（`@数据库`） | `database create --name shop --data-model sql` | `POST .../databases` 201 | 否 |
| 2 | 「在 shop 建 orders 表，id 与 amount」（`@数据库`） | `schema create-table ...` | `POST .../schema/tables` | 否 |
| 3 | 「插入一行 id=1 amount=10」（`@数据库`） | `sql exec` INSERT | `POST .../execute` | 否 |
| 4 | 「查出 amount」（`@数据库`） | `sql query` SELECT | `POST .../query`，结果进 `tool_result` | 否 |
| 5 | 「删掉 shop 库」（`@数据库`） | `database delete --id ...` | 确认前 0 次 DELETE；同意后 1 次 | 是 |
| 6 | 「系统库也删掉」 | `database delete` 指向系统库 | 确认后 API 403 `system_database_protected`，`is_error` | 是 |
| 7 | 「写一个 Hello 云函数并调用」（`@云函数`） | `function create` 然后 `function invoke` | POST gofunctions，再 `POST /go/...` | 否 |
| 8 | 「每天 0 点跑这个函数」（`@定时任务` `@云函数`） | `cron create --schedule-kind cron --cron "0 0 * * *"` | `POST .../cron-jobs` | 否 |
| 9 | 「现在就跑一次」（`@定时任务`） | `cron trigger` | 确认后 `POST .../trigger` 202 | 是 |
| 10 | 「设一个键 a=1 再读出来」（`@键值`） | `kv exec SET` 然后 `GET` | 两次 `POST .../kv` | 否 |
| 11 | 「删掉键 a」（`@键值`） | `kv exec DEL` | 确认后才 POST | 是 |
| 12 | 「上传 readme」（`@对象存储`） | `object upload` | `POST .../s3/objects` | 否 |
| 13 | 只打开通用助手，不 `@`，句子是「删掉所有库」 | 模型若仍给出 `database delete` | `skill_not_loaded`，HTTP 0 次 | 不适用 |
| 14 | admin 角色「插入一行」 | `sql exec` INSERT | 403 `forbidden` | 否 |
| 15 | 「停掉」（执行中） | 任意慢命令 | cancel 后 `end.reason=canceled`，无 `end` 之前的成功写 | |

样例 7 的源码用测试夹具里的最小 Go 函数（`gofunction` 已支持的 `fmt`），不访问外网。`-short` 时样例 7 的 invoke 可以用 fake 解释器桩，但 create 的 HTTP 断言仍要跑。

### 6.7 CI 里怎么跑

不新增工作流依赖。在现有命令上加包：

```text
go test ./packages/go-sdk ./cmd/simplebase ./internal/cloudagent ./internal/api -count=1
```

`go test ./...` 必须仍然通过。前端：

```text
cd ui && yarn test && yarn build
```

样例集在 `go test ./internal/api -run AgentSkillsE2E`，无 LLM 密钥、无 S3。`./scripts/smoke.sh` 实现阶段加这一 `-run`，不把真实供应商列为必过。

`gofunction` 的外网 HTTP 用例继续只在非 `-short` 跑，与本计划无关。

## 7. 分阶段实施

每阶段都保持 `go build ./internal/... ./cmd/...` 与对应测试通过。未到的阶段不把 `agent.skills_cli` 默认打开。

### 阶段 A — Go SDK 补齐与 CLI 读命令

范围：§3.3 的 SDK 方法；`cmd/simplebase` 的配置、信封、退出码；映射表中的全部 **读** 命令（list/get/query/search/capabilities/presign）。

主要文件：`packages/go-sdk/*.go` 及测试，`cmd/simplebase/**`，`build.sh` 增加第二个二进制。

验收：§6.1 与读命令集成通过；`simplebase database list` 对 httptest 返回 JSON；不注册助手工具；默认配置 `skills_cli: false`。

### 阶段 B — CLI 写命令与授权缝

范围：映射表中的写与删除；脱敏；补上 §3.6 列出的 `writable` 检查及「系统库拒绝」测试。

主要文件：SDK 写方法，`cmd/simplebase` 写子命令，`internal/api/agent_handler.go`，`agent_schedule_handler.go`，`s3_handler.go` 及各自 `_test.go`。

验收：§6.2 全表通过，包括系统库、admin 角色、`writable=false`、API Key secret 扣留。助手运行时行为不变。

### 阶段 C — Skill 与助手执行

范围：`skills/simplebase/**`，embed，`@` 解析，`simplebase` 工具，委托 token，子进程，确认挂起与路由。`skills_cli` 仍默认 false，测试里打开。

主要文件：`skills/simplebase/`，`internal/cloudagent/modules.go`（提示词在开关打开时替换只读禁令），`tools.go`，`runtime.go`，`internal/api/agent_handler.go`，`router.go`，`internal/config`，`ui` 的 composer 与 `http-api.ts`（先能把 `skills` 传上去；确认弹窗可在阶段 D 一起做，但 API 在本阶段测完）。

验收：§6.3 与样例 1–4、7–8、10、12–14 的 HTTP 断言通过。样例 5、6、9、11 在本阶段用测试里的 approve 调用，不依赖弹窗。token 不出现在 SSE 与审计。沙盒工具测试确认环境里没有 `SIMPLEBASE_TOKEN`。

### 阶段 D — 流式事件与界面

范围：§5 的事件、续传、`ConversationView` / `AgentToolCard` / `useAgentConversation` / `ConfirmAction` 接线，取消与三次重连。

主要文件：`runtime.go`，`agent_handler.go`，`ui/src/components/agent/*`，`ui/src/composables/useAgentConversation.ts`，`ui/src/services/http-api.ts`，`types.ts`，对应 vitest。

验收：§6.4、§6.5；手工清单见 §7 末。`yarn test` 与 `yarn build` 通过。开关仍默认 false。

### 阶段 E — 样例集与默认开关

范围：§6.6 十五条全部进 `go test`；`scripts/smoke.sh` 挂上 `-run`；文档 `docs/` 里增加一页 CLI 说明（实现时写，本计划不写文档正文）。确认无回归后，把 `config.example.yaml` 的 `agent.skills_cli` 设为 `true`。已有部署的 `config.yaml` 不自动改（该文件 gitignore）；示例与代码默认值要同时说清楚：代码默认 false，直到本阶段验收才改成 true。

验收：`go test ./...`、`go test ./gofunction/... -short`、`cd ui && yarn test && yarn build`、`go test ./packages/go-sdk`。

### 实现时的手工检查（阶段 D、E）

浏览器里用 dev 种子走一遍：登录 → `/console/agents` → `@数据库` 建库、建表、插入、查询 → 卡片从参数流式到结果 → 删除时出现确认，取消后库还在，确认后库消失。再 `@云函数` 创建并调用。刷新页面后历史卡片还在。点停止后状态为已停止。admin 账号插入应看到 403 卡片而不是成功。系统项目上的删除被拒绝。桌面宽度与窄屏各看一次工具卡折叠。

## 8. 风险

| 风险 | 处理 |
| --- | --- |
| 模型不点 `@` 就编造写命令 | 宿主前缀表拒绝，样例 13 锁住 |
| 委托 token 泄漏进工具参数或日志 | 禁止 argv 携带 token 字样；审计保持现有字段；SSE 不回显环境 |
| 有人把 CLI 放进沙盒以便「模型自己装包」 | 计划明确禁止。沙盒工具与 CLI 分路，测试检查沙盒环境 |
| 子进程 + loopback 比进程内函数调用多一跳 | 换来与 API 完全同一鉴权。不在 cloudagent 里直接调 catalog |
| 实例只读被旧 handler 绕过 | 阶段 B 先补 `writable`，再暴露写命令 |
| SQL 破坏性分类漏判 | 拿不准就确认；sqlguard 仍会拒绝 ATTACH 等 |
| 确认挂起占着 thread 锁 | 计入现有 180 秒 run 超时与 2 分钟确认超时；超时按拒绝并 `end` |
| 流式参数增量因供应商不支持而缺失 | 退化为一条 `tool_call`，前端同一张卡片 |
| 续传环被刷掉 | `stream_gap` 后改拉已落库消息，不假装中间帧还在 |
| 前端覆盖率阈值 | 新交互都有 vitest；不桩 ui 组件 |
| functions 基线若仍低于 95% | 本改动的新函数必须被测到；不在本计划里顺手补历史缺口，除非不补就过不了 `yarn test` |
| 云函数 invoke 只有读权限，函数体内仍可能写库 | 这是现有 `/go` 语义。skill 文本告诉模型 invoke 有副作用；不在本计划改调用面权限 |
| 定时任务无法确认删除 | 返回 `confirmation_unavailable`，不自动批准 |

## 9. 不做的事

- 本 PR 不提交实现、不改 `go.mod` / `package.json` / 前端配置。
- 不把 SimpleBase token 注入云沙盒，不用沙盒跑 CLI。
- 不让 cloudagent 直接写 DuckLake、S3 或系统库。
- 不把系统库改成可写，不给 admin 角色加写权限。
- 不引入 WebSocket、不引入 cobra 或其它第三方 CLI 框架。
- 不补 JS SDK 的 KV/云函数/定时任务。
- 不做多 Agent 组队（现有 `team_supported: false` 保持）。
- 不让助手创建 LLM 凭据、不让助手改用户密码、不让助手递归调用自己的 runs。
- 不把一次性 API secret 放进模型上下文。
- 不为定时调度增加「无人值守自动删除」。
- 不改路由路径与 `/console/agents` 入口。
- 不在本计划里重排未改动文件的 gofmt。

## 10. 验收总表（全部阶段完成后）

1. `simplebase <资源> <动作>` 覆盖 §3.5，输出 JSON，错误码与 API 一致。
2. `@数据库` 只有数据库命令能执行；`@云函数` 同理；不点名的通用助手不能写。
3. 删除与其它破坏性命令在界面确认后才发 HTTP；取消不发。
4. 系统库写删除被拒绝；admin 角色写被拒绝；实例只读返回 `writer_unavailable`。
5. 对话里能看到文本增量、参数增量、执行中、结果与错误；停止与断线续传符合 §5。
6. §6 的单测、集成、路由、流式、Vitest 与十五条样例在 CI 命令中通过。
