# Cloud Agent（替代 LLM 对话）

> 状态：Phase 0–2 MVP（本 PR）
> 配套：`proto-http.md` §3.12、`ui-plan-v2.md`
> 运行时：`github.com/cloudwego/eino`（ChatModelAgent + 只读 Tool）
> 灵感（不移植协议）：`thirdparty/buzz` 的 agent-as-member、`@` mention、persona/prompt 分离

## 1. 目标

把现有「LLM 对话」（`/llm`、`LlmManager` / `AiChat`）升级为 **Cloud Agent**：

- 每个项目有一组 **per-module agent**（database / s3 / logs / …）
- 两栏 UI：左 Agent 列表，右对话 + composer `@Agent` 点名
- 后端用 **eino ChatModelAgent** 跑单 agent + **只读工具**
- Agent / Thread / Message / Run 存在 **系统 DuckLake**（`sys_cloud_*` / `sys_agent_*`，与 `sys_llm_*` 同模式）
- 模型与厂商密钥仍走现有 LLM Gateway / `sys_llm_*`，**不**把 vendor key 挪进 agent 表

`/v1/projects/:p/llm/*` 保持可用（deprecated）。UI `/llm` 重定向到 `/agents`。

## 2. 阶段

| 阶段 | 范围 | 本 PR |
|---|---|---|
| **0** | 计划、表、种子 3 个默认 agent | ✅ |
| **1** | 单 agent + 只读工具（db list/SQL、s3 list/head、logs search/stats） | ✅ |
| **2** | HTTP CRUD + 流式 run + 两栏 UI + `@` mention | ✅ |
| **3** | 多工具编排、更好的 snapshot、会话标题、用量卡片 | 以后 |
| **4** | Eino Host Multi-Agent / Team 协作 | 以后（本 PR 仅 `team_enabled` 字段与 modules stub） |

**明确不做（本 PR）**

- 破坏性写工具（INSERT/UPDATE/DELETE、S3 PUT/DELETE、改 retention）
- Buzz 运行时 / 协议
- Host Multi-Agent、子 agent 调度、DeepAgent
- 把 API key 写入 `sys_cloud_agents` 或 prompt

## 3. 数据模型（系统 DuckLake）

DuckLake 不用 PRIMARY KEY；唯一性由应用层保证。软删用 `archived_at`。

### `sys_cloud_agents`

| 列 | 说明 |
|---|---|
| `id` | UUID |
| `project_id` | 项目隔离 |
| `name` | 展示名（composer `@Name`） |
| `module` | `database` / `s3` / `logs` / `general` |
| `description` | 短描述 |
| `system_prompt` | 用户自定义 persona（可空；叠在 module 模板之上） |
| `tool_ids` | JSON 字符串数组，只读工具 id |
| `model_override` | 可选；空则用 `sys_llm_settings` / gateway 默认 |
| `team_enabled` | Phase 4 stub，默认 0 |
| `created_at` / `updated_at` / `archived_at` | 时间戳 |

### `sys_agent_threads`

`id`, `project_id`, `title`, `created_by`, `created_at`, `updated_at`, `archived_at`

### `sys_agent_messages`

`id`, `thread_id`, `project_id`, `role` (`user`/`assistant`/`tool`/`system`), `content`, `agent_id`, `mentions_json`, `tool_calls_json`, `run_id`, `created_at`

### `sys_agent_runs`

`id`, `thread_id`, `project_id`, `agent_id`, `status` (`queued`/`running`/`completed`/`failed`/`canceled`), `error`, `started_at`, `finished_at`, `created_at`

DevMode 种子项目与 **新项目首次列出 agents** 时幂等写入 3 个默认 agent：Database / S3 / Logs，带 module prompt 模板与默认 `tool_ids`。

## 4. Prompt 组装（禁止密钥）

顺序（后段覆盖/追加，不覆盖前段的安全约束）：

1. **Platform base**：身份、只读、禁止编造密钥、禁止输出凭据
2. **Module template**：该模块能做什么 / 不能做什么
3. **`agent.system_prompt`**：用户 persona
4. **Project env block**：`project_id`、可见库名/对象数量等**非机密**元数据
5. **Module readonly snapshot**（截断）：库列表、对象 key 列表、最近日志摘要
6. **History**（截断）
7. **User message**（含 `@Agent` 文本）

**永不进入 prompt**：S3 access/secret key、KMS、provider API key、`credential_ref` 原文、DSN、`storage_prefix`。组装后走一次 redact。

## 5. 只读工具（Phase 1）

| 模块 | 工具 id | 行为 |
|---|---|---|
| database | `list_databases` | 列出项目 user 库（id/name/status） |
| database | `list_collections` | `information_schema` 表名 |
| database | `readonly_sql` | `sqlguard.ReadOnly` + query；拒绝写语句 |
| s3 | `list_objects` | 项目对象列表（索引优先） |
| s3 | `head_object` | 元数据（size/etag/content_type），不读 body |
| logs | `search_logs` | `QueryLogs` |
| logs | `log_level_stats` | 按 level 计数 |

工具复用 `internal/catalog`、`SQLService`、`systemdb` S3 索引、`QueryLogs`。无对应 API 时返回明确错误字符串，而不是空成功。

## 6. Eino 运行时

包：`internal/cloudagent`

- `ChatModel` 适配现有 `llmgateway`（`Generate` / `Stream` + `model.WithTools`）
- `adk.NewChatModelAgent`：单 agent + 上述 Tool
- `adk.NewRunner`：流式 token / tool_call / tool_result 事件 → SSE
- 模型名：`agent.model_override` → 否则 `sys_llm_settings.default_model` → gateway 默认
- `POST .../agent-runs/:id/cancel` 取消 run context

Team / Host Multi-Agent：**只存 `team_enabled`，不调度多个 ChatModelAgent。**

## 7. HTTP（项目作用域）

前缀：`/v1/projects/:projectID`（Echo，与现网一致）。权限与 LLM 对齐：读 `DatabaseRead`，写 agent/thread `DatabaseWrite`，run 与 LLM chat 一样 `DatabaseRead`（另 `CheckQuota(..., "llm")`）。

见 `proto-http.md` §3.12。要点：

- `GET/POST /agents`，`GET/PATCH/DELETE /agents/:id`，`GET /agents/modules`
- `GET/POST /agent-threads`，`GET/DELETE /agent-threads/:id`
- `GET /agent-threads/:id/messages`
- `POST /agent-threads/:id/runs` body `{ content, mentions:[{agent_id}], stream? }`
- `POST /agent-runs/:runID/cancel`

流式：`Content-Type: text/event-stream`，帧 `data: {type, ...}\n\n`：`run` / `token` / `tool_call` / `tool_result` / `error` / `end`。

## 8. 前端

- 路由 `/agents`，title **Cloud Agent**；`/llm` → `/agents`
- NavMenu：LLM → Cloud Agent，`routeOrder` 用 `agents`
- 两栏：左 agent 列表 + 新建/编辑 `SbModal`（name、module、system prompt、tools）；右 `AiChat` + `@` mention
- `AiChat` / `AiChatComposer`：mention 选择器、可选 tool-call 卡片
- `services/types.ts` + `http-api.ts` + `mock.js` 增加 `agents` / `agentThreads` 域

## 9. 验收（MVP）

1. 导航显示 Cloud Agent；`/llm` 进入同一页
2. 两栏 UI；可创建/编辑 agent
3. `@agent` 发送后 SSE 流式出字
4. Database / S3 agent 至少各有一个真实只读工具打到项目数据
5. Prompt 不含 S3/provider 密钥（单测 redact + 组装断言）
6. `go test` 覆盖触及的包；`ui/` 下 `yarn build` 通过
