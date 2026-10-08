# 云 Agent 前后端功能优化 v4.1（修订）

> **仓库**：SimpleBase
> **状态**：已实施（2026-10-08）。S0–S5 全部落地：fakellm 测试基建与 E01–E16 全链路用例、后端 BUG 修复（02/03/06/07/08/11）、前端两列布局与 `components/agent/` 组件族、`useAgentConversation` 状态层、脚本（test.sh / smoke.sh / llm-integration.sh / fakellm-server）。验证：`./scripts/test.sh` 后端全绿；前端 `npm test` 463 用例全过（分支覆盖 95.07%，函数 94.24% 为存量缺口）。
> **前置**：`plan/planv2.0/cloud-agent-plan.md`、`plan/planv3.0/cloud-agent-sandbox-plan.md`、`plan/planv4.0/cloud-sandbox-plan.md`
> **关联代码**：`internal/cloudagent/{runtime,model,modules,access}.go`、`internal/api/{agent_handler,cloudagent_access}.go`、`internal/systemdb/agents.go`、`ui/src/pages/AgentManager.vue`、`ui/src/components/ai/*`、`ui/src/composables/useAgentRun.ts`、`ui/src/services/http-api.ts`
> **联调模型**：SiliconFlow `deepseek-ai/DeepSeek-V4-Flash`（OpenAI 兼容）；默认测试一律用本地 fake upstream，不访问外网。

---

## 0. 本版变化（相对 v4.0）

| 方向 | v4.0 现状 | v4.1 目标 |
|---|---|---|
| 布局 | 三列：Agent 卡片 300px + 会话列表 220px + 对话；Agent 卡片含描述、定时、三颗按钮，信息过密 | 两列：**左侧简化 Agent 卡片列表**；会话切换收进对话区头部；整体统一为一个「工作台」样式 |
| Agent | 默认只播种 Database / S3 / Logs；General 模块无工具 | 新增并默认播种 **通用 Agent**（全部只读工具 + 可选沙盒），作为默认选中项与未 @ 时的兜底 |
| 对话组件 | `AiChat` 同时服务 LLM 页与 Agent 页，Agent 特有能力（工具卡、思考、错误、状态条）以 props/slot 拼接 | 抽出通用 `AgentConversation` 组件族：消息流、后端输出（token / thinking / tool / usage / error）、输入框、状态条 |
| 测试 | 后端单测以 fake `ChatClient` 为主；全链路（router → handler → runtime → llmgateway → HTTP upstream）只有需要真实 key 的 `llm_integration` | 新增 **fake OpenAI 兼容 upstream**，默认 `go test` 即可跑通全链路 SSE；新增可脚本化的冒烟脚本与本地 fake 模型进程 |
| Bug | 见 §2 | 全部修复并各配回归用例 |

非目标不变：不新增第三方依赖、不升级 litellm / eino、不做多 Agent Team、不引入浏览器 e2e 框架。

---

## 1. 验证未通过的现象（2026-10-08 代码走查推断，需在 S0 用失败用例复现确认）

| # | 预期现象 | 根因（代码走查） |
|---|---|---|
| V1 | 控制台发送后正文不逐字出现，结束时一次性出现；工具卡片也是结束后才出现 | 前端响应式失效，见 BUG-01 |
| V2 | 工具执行较慢（如 `readonly_sql` 2s+、沙盒命令）时状态条从「调用 xx…」跳回「思考中」 | 后端 idle 心跳在工具执行期间仍发 `thinking`，见 BUG-02 |
| V3 | 失败后点「重试」，刷新页面看到同一句用户消息出现两次，模型上下文也重复 | 重试重新 POST，后端再次落库用户消息，见 BUG-03 |
| V4 | 删除非当前会话时，当前正在运行的回复被标记「已停止」 | `removeThread` 无条件 `onStop()`，见 BUG-04 |
| V5 | 点「停止」后会话列表预览/时间不更新；刷新后被停止或失败的回复不再显示「已停止 / 失败」 | 只在 `onEnd` 刷新列表；消息历史不带 run 状态，见 BUG-05、BUG-06 |
| V6 | 模型输入框只能看到默认模型 | `/llm/settings` 只返回 `defaultModel`，见 BUG-07 |
| V7 | 「思考过程」折叠框从不出现 | `StreamDelta` 无 reasoning 字段，链路上没有思考正文（已知限制，见 §4.4） |
| V8 | `llm_integration` 只覆盖 2 个场景；R01–R10 未执行，无法在无 key 环境复现 | 缺少离线全链路测试，见 §5 |

---

## 2. Bug 清单与修复方案

| ID | 位置 | 问题 | 修复 | 回归用例 |
|---|---|---|---|---|
| BUG-01 | `AgentManager.vue` `onSend` | `const reply = {...}; chatMessages.value.push(reply)` 后所有回调改的是**原始对象** `reply`，不经 Proxy，不触发渲染；只有 `sending` 变化时顺带重绘 → 前端「假流式」 | `push` 后改为 `const reply = chatMessages.value[chatMessages.value.length - 1]`（取回响应式代理），或 `reactive<ChatMsg>({...})` 后再 push；该逻辑迁入 `useAgentConversation`（§3.4） | U12：mock 流逐帧推送，每帧后 `await nextTick()` 断言 DOM 文本递增、工具卡片数量递增 |
| BUG-02 | `agent_handler.go` `streamRun` thinking ticker | 只按「距上次输出 ≥2s」判断，工具执行期间也会发 `thinking` | 引入 `phase`（`model` / `tool`）：收到 `tool_call` 置 `tool`，收到对应 `tool_result` 回 `model`；`tool` 阶段不发 `thinking`，改发 `{"type":"tool_progress","call_id","elapsed_ms"}`（可选，前端用于「调用 xx · 3s」） | A15：fake 工具阻塞 50ms、心跳间隔注入 10ms，断言 `tool_call` 与 `tool_result` 之间无 `thinking` |
| BUG-03 | `CreateRun` + `retryLast` | 重试会重复落库用户消息，历史出现两条相同 user turn | `createRunBody` 增加 `retry_of_run_id`：校验该 run 属于本 thread、状态为 `failed/canceled`、且是 thread 最新一次 run；复用其 user message（不再 Append），新 run 记录 `retry_of`；前端重试传该字段 | A16：失败 run 后带 `retry_of_run_id` 重试 → user 消息总数不变、新 run 关联；对非最新 run / 跨 thread / completed run 返回 400 |
| BUG-04 | `AgentManager.vue` `removeThread` | 删除任意会话都会停止当前运行 | 仅 `id === threadId` 时 stop；运行中删除当前会话需 `ConfirmAction` 文案提示「将停止当前回复」 | U13 |
| BUG-05 | `AgentManager.vue` `onStop` / `onError` | 停止与失败后不刷新会话列表（预览、`updated_at`） | 统一在 `finally` 语义的 `onSettled` 中刷新列表（end / error / stop 三条路径） | U14 |
| BUG-06 | `GET /agent-threads/:id/messages` | assistant 消息不带所属 run 的状态，刷新后丢失「已停止 / 失败」标记 | 消息 DTO 增加 `run_status`、`error_code`（JOIN `sys_agent_runs`，只读）；前端据此还原 `canceled` / `error` 展示（失败消息显示固定中文提示，不含上游原文） | A17、U15 |
| BUG-07 | 模型选择 | 前端只能拿到默认模型 | 新增 `GET /v1/projects/:projectID/agents/models`（`database:read`）：返回 `{default_model, models:[{provider, name}]}`，来源为 provider `allowed_models`，**只返回名字，不返回 base_url / key**；前端表单改为 `Combobox`（项目已有 `components/ui/combobox`），允许列表外自由输入但提示「需在 allowed_models 中」 | A18（含断言响应体不含 `base_url`/`api_key`）、U06 扩展 |
| BUG-08（潜在） | `runtime.go` `StartRun` | `tc.ID == ""` 时 `StartRun` 本地补的 `call_` id 不会传回 eino 工具消息，`tool_result` 的 `call_id` 对不上，会多出一张只有结果的卡片。目前 `assistantToolMessage` / `parseAssistant` 已补 id，但 `consumeAssistant` 流式拼接路径未保证 | 在 `gatewayChatModel` 出口统一保证 ID 非空，`StartRun` 内删除补 id 逻辑；`tool_result` 的 `CallID` 为空时按「最近一个同名且未完成」配对并记 warn 日志 | B19：fake 返回无 id 的原生 tool_calls，断言 call/result 恰好一对且 id 相同 |
| BUG-09 | `runtime.go` 工具卡落库 | SSE 中 `tool_result` 截断到 2000 字，但 `ToolCallsJSON` 存全文：大结果撑大 `sys_agent_messages`，刷新后卡片内容与实时不一致 | 落库与 SSE 同一上限（`maxToolCardChars=4000`，配置常量），超出加 `truncated:true` 标记；前端显示「结果已截断」 | B20 |
| BUG-10 | `useAgentRun.stop` | `stop()` 先 `close` 连接，`onEnd` 不再触发；`phase` 置 `canceled` 但 `metrics` 不更新，状态条无耗时 | stop 后调用 `GET .../runs?limit=1` 回填指标（失败忽略）；或在 cancel 接口响应中返回 run 指标 | U16 |
| BUG-11 | 种子数据 | `SeedDefaultCloudAgents` 在已有任意 agent 时整体跳过，老项目永远拿不到新增的通用 Agent | 改为按 `module + name` 幂等补齐「系统内置」Agent：新增列 `builtin BOOLEAN`（迁移 +1），内置 Agent 可编辑 prompt/模型、可停用，**删除即归档且不再自动补回**（记录 `seed_dismissed`） | S01–S03（§5.1） |
| BUG-12 | `AgentThreadList` / 会话切换 | 运行中切换会话会静默停止 | 运行中切换弹 `ConfirmAction`「切换将停止当前回复」；确认后 stop 再切换 | U05 扩展 |

> 说明：BUG-01 是 V1「功能验证未通过」的主因；v4.0 的 vitest 只在流结束后断言最终文本，未覆盖中间帧渲染，这是测试设计缺口，§5.3 的 U12 专门补上。

---

## 3. 前端方案：布局重设计 + 通用对话组件

遵守 `ui/AGENTS.md`：`interface` 定义形状、Tailwind class、现有 `components/ui/*`（card / badge / button / combobox / sheet / tooltip / scroll-area / textarea）、`vue-sonner`、`ConfirmAction`、`SbModal`、`SbEmptyState`；不新增依赖。

### 3.1 页面布局

```text
┌──────────────── PageContainer ────────────────────────────────────────────┐
│ ┌ Agent 列表 (w-64) ┐ ┌ 对话工作台 ───────────────────────────────────────┐ │
│ │ [搜索]      [+]   │ │ 头部：@通用助手 · 模型 · [会话 ▾ 标题] [新会话] [⋯] │ │
│ │ ● 通用助手   通用 │ ├──────────────────────────────────────────────────┤ │
│ │ ○ Database  DB  ⏱ │ │ 消息流（MessageScroller）                          │ │
│ │ ○ S3        S3    │ │   用户气泡 / 助手气泡（工具卡 · 思考 · 正文 · 状态）│ │
│ │ ○ Logs      日志  │ ├──────────────────────────────────────────────────┤ │
│ │ ○ Sandbox   沙盒  │ │ 状态条：思考中 · 3s / 调用 readonly_sql · 1s       │ │
│ │                   │ │ 输入框（@ 点名、Enter 发送、Shift+Enter 换行、停止）│ │
│ └───────────────────┘ └──────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────────────────┘
```

- 两列 `md:grid-cols-[256px_minmax(0,1fr)]`，工作台高度 `h-[calc(100dvh-<header>)]`，消息区内部滚动，输入框固定在底部（不再随消息增长把页面撑长）。
- 移动端（< md）：Agent 列表收进 `Sheet`（左侧抽屉），头部显示当前 Agent 名作为抽屉触发器。
- 会话列表从常驻列改为头部 `Popover` 下拉（`AgentThreadSwitcher.vue`）：搜索框 + 列表（标题、预览、相对时间）+ 行内重命名 / 删除；「新会话」为头部独立按钮。
- 头部「⋯」菜单：编辑 Agent、定时任务、查看运行记录（`Sheet` 展示 `GET .../runs` 指标）。

### 3.2 简化 Agent 卡片（`AgentCard.vue`，新）

| 元素 | 规则 |
|---|---|
| 行高 | 单行紧凑卡片，约 52px：图标 + 名称 + 模块 badge |
| 图标 | 按模块固定 lucide 图标（通用 `Sparkles`、database `Database`、s3 `HardDrive`、logs `ScrollText`、sandbox `Terminal`） |
| 次要信息 | 描述不再常驻；以 `title` / `Tooltip` 展示。定时任务只显示 `ClockIcon` + 启用状态圆点 |
| 操作 | 卡片不再放「编辑 / 定时 / 删除」三按钮；hover 显示「⋯」`Popover`（编辑、定时、删除），键盘可达 |
| 选中态 | `border-primary bg-primary/8`，`aria-pressed`；内置 Agent 显示小号「内置」badge |
| 运行态 | 当前 Agent 正在运行时名称旁显示 `Spinner`（size-3） |
| 空态 / 加载 | 列表加载用 3 条 `Skeleton`；无 Agent 用 `SbEmptyState` |

列表顶部：搜索（按名称/模块过滤，前端过滤）、「+」新建。刷新按钮移除（页面切换 / 保存后自动刷新）。

### 3.3 通用对话组件族（`ui/src/components/agent/`，新目录）

把 Agent 对话从 `AiChat` 中拆出，`AiChat` 保持给 LLM 页使用、不再承担 Agent 逻辑（删除其中 Agent 专用 props 与 `AgentToolCard` 引用，相应测试迁移）。

| 组件 | 职责 | 主要 props / emits |
|---|---|---|
| `AgentConversation.vue` | 容器：消息流 + 状态条 + 输入框；无业务请求，只消费 `useAgentConversation` 状态 | `messages`、`phase`、`statusText`、`mentionAgents`；emits `send(text, mentions)`、`stop`、`retry(runId)` |
| `AgentMessage.vue` | 单条消息：用户气泡右对齐；助手消息全宽无边框（类 ChatGPT），顺序为 思考 → 工具卡 → 正文 → 尾部状态 | `message: AgentChatMessage`、`streaming` |
| `AgentMarkdown.vue` | 助手正文 Markdown 渲染：复用已有 `marked` + `dompurify`（均为现有依赖），代码块走 `SbCodeBlock`；流式中每 50ms 节流渲染 | `content` |
| `AgentThinking.vue` | 「思考中 · Ns」占位 + 有内容时可折叠思考正文 | `elapsedMs`、`content` |
| `AgentToolCard.vue` | 迁入本目录；头部「工具名 · 状态（运行中 / 成功 / 失败） · 耗时」，默认折叠；展开显示参数与结果（SQL 表格 / 沙盒 stdout·stderr·exit / JSON） | `tool` |
| `AgentStatusBar.vue` | 输入框上方单行状态 | `phase`、`statusText` |
| `AgentComposer.vue` | 基于 `AiChatComposer` 的 @ 点名能力；底部提示「工具只读 / 沙盒在隔离环境执行」由当前 Agent 决定 | `sending`、`placeholder`、`mentionAgents` |
| `AgentErrorBlock.vue` | 错误 code → 中文文案 + 「重试」；`agent_thread_busy` 不显示重试，显示「等待当前回复结束」 | `code`、`message`、`retryable` |

错误文案表从 `AgentManager.vue` 移入 `ui/src/services/agent-errors.ts`（纯函数，100% 覆盖）。

### 3.4 状态：`useAgentConversation`（替换 `useAgentRun`）

- 单一 composable 管理：`messages`（`reactive` 数组，修复 BUG-01）、`phase`、`elapsedMs`、`currentRunId`、`lastRequest`、`metrics`。
- 方法：`load(threadId)`、`send(text, mentions)`、`stop()`、`retry()`、`dispose()`；`onSettled` 统一刷新会话列表回调（BUG-05）。
- 事件 → 状态映射（唯一事实来源）：

| SSE 帧 | phase | 消息变化 |
|---|---|---|
| `run` | thinking | 记录 `run_id` 到 assistant 消息 |
| `thinking` | thinking | 更新 `elapsedMs`，有 content 则追加思考正文 |
| `token` | streaming | 追加正文 |
| `tool_call` | tool | 按 `call_id` 新增卡片（`status=running`） |
| `tool_progress` | tool | 更新卡片耗时 |
| `tool_result` | thinking | 按 `call_id` 填结果（`status=ok/error`） |
| `usage` | — | 写 `metrics` |
| `error` | error | 写 `error.code/message`，`retryable` 由 code 决定 |
| `end` | done / canceled | `reason=canceled` 标记已停止；`max_iterations` 尾部显示提示 |

- 旧 `useAgentRun.ts` 删除，测试迁移到 `useAgentConversation.test.ts`。

### 3.5 视觉统一

- 卡片圆角 `rounded-xl`、间距 `gap-3`、正文 `text-sm leading-relaxed`；助手消息不加边框，用户气泡 `bg-primary/10`。
- 工具卡与思考块使用 `bg-muted/50 border-dashed`，与正文区分。
- 所有图标按钮带 `aria-label`；状态条 `role="status" aria-live="polite"`；消息流 `aria-live` 只在流结束时播报（避免逐字朗读）。
- 暗色模式只使用设计 token（`bg-card`、`text-muted-foreground` 等），不写死颜色。

---

## 4. 后端方案

### 4.1 通用 Agent（新增内置）

| 项 | 值 |
|---|---|
| 名称 / 模块 | `通用助手` / `general` |
| 默认工具 | `list_databases`、`list_collections`、`readonly_sql`、`list_objects`、`head_object`、`search_logs`、`log_level_stats`；云沙盒可用（`sandbox_available=true`）时追加四个 `sandbox_*` |
| 模块 prompt | 重写 `generalModulePrompt`：说明可跨数据库 / 对象存储 / 日志只读排查，按问题选择工具，无法确定时先列出资源；沙盒可用时说明隔离与 `/workspace` 约束 |
| 播种顺序 | 通用助手排第一，前端默认选中 |
| 兜底 | `CreateRun` 未 @ 任何 Agent 时（当前直接取第一个），改为优先使用内置通用助手；不存在则返回 400 `agent_required` |

`Modules()` 中 general 的 `DefaultTools` 改为上述只读工具集；`DefaultToolsForModule("general")` 同步。工具权限仍由 `ToolIDs` 白名单 + sqlguard + 沙盒策略控制，通用 Agent 不放宽任何限制。

### 4.2 内置 Agent 播种（BUG-11）

- 迁移（版本号 = 当前最大值 +1）：`sys_cloud_agents` 增加 `builtin_key VARCHAR NULL`（`general` / `database` / `s3` / `logs`），唯一索引 `(project_id, builtin_key)`；新表或列 `sys_cloud_agent_seed_dismissed(project_id, builtin_key)`。
- `SeedDefaultCloudAgents` 改为：对每个内置 spec，若 `(project_id, builtin_key)` 不存在且未 dismissed 则创建；存量同名 Agent（旧版本播种）按 `module + name` 回填 `builtin_key`，不重复创建。
- 删除内置 Agent → 归档 + 写 dismissed；不再补回。
- 写路径遵守 `writable=false` → `ErrWriterUnavailable`；只读请求下播种失败不阻断列表（记 warn，返回现有列表）。

### 4.3 SSE 协议补充

```text
data: {"type":"run","run_id":"r1"}
data: {"type":"thinking","elapsed_ms":2000}
data: {"type":"tool_call","call_id":"c1","name":"readonly_sql","arguments":"{...}"}
data: {"type":"tool_progress","call_id":"c1","elapsed_ms":2000}      # 新增，可选
data: {"type":"tool_result","call_id":"c1","name":"readonly_sql","content":"...","duration_ms":2310,"is_error":false,"truncated":false}   # 新增 is_error/truncated
data: {"type":"token","content":"..."}
data: {"type":"usage",...}
data: {"type":"end","reason":"stop"}
```

- `tool_result.duration_ms`：v4.0 计划有、实现未发，本版补齐（runtime 在 tool_call 时记起点）。
- `tool_result.is_error`：工具返回错误（sqlguard 拒绝、沙盒非零退出不算错误）时为 true。
- 新增帧均为增量字段，旧前端忽略。

### 4.4 推理内容（V7，保持已知限制）

不升级 litellm 前，思考正文仍不可得，前端只展示「思考中 · Ns」。在 `StreamDelta` 预留 `Reasoning string` 字段与 runtime 映射代码路径（fake upstream 可注入，测试覆盖），真实 provider 待依赖升级后自动生效。思考正文不落库、不进审计。

### 4.5 重试（BUG-03）与消息状态（BUG-06）

- `POST /agent-threads/:id/runs` body 增加 `retry_of_run_id`；校验失败 400 `invalid_retry`。
- `GET /agent-threads/:id/messages` assistant 项增加 `run_status`、`error_code`；user 项增加 `run_id`（前端据此确定重试目标）。

### 4.6 模型列表（BUG-07）

`GET /v1/projects/:projectID/agents/models`，`database:read`：

```json
{"default_model":"deepseek-ai/DeepSeek-V4-Flash","models":[{"provider":"openai","name":"deepseek-ai/DeepSeek-V4-Flash"}]}
```

LLM 未启用时返回 `{"default_model":"","models":[]}`（200），前端显示「模型服务未配置」。

---

## 5. 测试方案

原则：默认 `go test ./...` 与 `yarn test` **不访问外网、不需要 key**，但要覆盖到真实 HTTP 链路；真实模型只做补充冒烟。

### 5.1 测试基础设施（新增）

| 组件 | 位置 | 说明 |
|---|---|---|
| fake OpenAI 兼容 upstream | `internal/testutil/fakellm/server.go` | `httptest.Server` 实现 `/v1/chat/completions`（stream / 非 stream）。按「剧本」返回：正文分块（可设每块延迟）、`reasoning_content` 增量、原生 `tool_calls` 增量（含缺 id）、文本协议 `TOOL_CALL`、usage 尾帧、指定 HTTP 状态（400 tools 不支持 / 401 / 429 / 500）、中途断流、永不返回（测超时）。记录每次请求体供断言（是否带 `tools`、消息历史内容） |
| 剧本匹配 | 同上 | 按最后一条 user 消息关键字或请求序号选择响应；剧本以 Go 结构体声明，可 JSON 序列化（供 §5.5 进程模式复用） |
| 全链路 harness | `internal/api/agent_e2e_harness_test.go` | 用 fakellm 的 URL 构造真实 `config.LLM` → 真实 `llmgateway.Service` → `cloudagent.Runtime` → 真实 router（`httptest`）；DB / S3 / Logs 工具依赖用 fake 实现；systemdb 用现有测试用嵌入式库 |
| SSE 解析辅助 | `internal/api/sse_test_helpers_test.go` | 读取响应体为 `[]Event`，带时间戳（测首帧延迟、帧顺序） |

> fakellm 不含任何真实 key；harness 用占位 key `test-key`，并断言日志 / 审计 / SSE 中不出现该字符串（验证脱敏路径）。

### 5.2 后端用例

**模型适配与 runtime（`internal/cloudagent`，fake ChatClient 或 fakellm）**

| ID | 用例 | 断言 |
|---|---|---|
| B19 | 原生 tool_calls 缺 id（BUG-08） | call/result 恰好一对、id 非空且一致；`ToolCallsJSON` 只有 1 张卡 |
| B20 | 工具结果 10KB（BUG-09） | SSE 与 `ToolCallsJSON` 内容都 ≤ 4000 字且 `truncated=true` |
| B21 | `tool_result.duration_ms` | 工具 sleep 30ms → `duration_ms ≥ 30`，落库同值 |
| B22 | 工具返回 error（sqlguard 拒绝 DROP） | `is_error=true`，run 继续，模型拿到错误文本后给出解释 |
| B23 | 同一轮多个原生 tool_calls（index 0/1） | 两对事件按 index 顺序，`call_id` 各不相同 |
| B24 | 流中途断开（上游 EOF 无 finish） | 已收到的正文保留；run 以 `llm_upstream_error` 失败，`RunResult.Content` 非空 |
| B25 | `reasoning` 增量（fake 注入） | 产生带 content 的 `thinking` 事件；不进入 `RunResult.Content`、不落库 |
| B26 | 通用 Agent 工具集 | `sandbox_available=false` 时 7 个只读工具；true 时 11 个；`buildTools` 不报错 |
| B27 | general 模块 prompt | 含只读约束与沙盒约束（沙盒可用时） |
| B28 | 未 @ Agent 的兜底 | 优先内置通用助手；无内置时 `agent_required` |

**播种（`internal/systemdb`）**

| ID | 用例 | 断言 |
|---|---|---|
| S01 | 新项目播种 | 4 个内置 Agent，通用助手排第一，`builtin_key` 正确 |
| S02 | 老项目（已有旧版 3 个 + 自建 1 个） | 只补通用助手；旧 3 个回填 `builtin_key`；自建不变；重复调用幂等 |
| S03 | 删除内置 Agent 后再次播种 | 不补回；`writable=false` 播种返回 `ErrWriterUnavailable` 且列表接口仍 200 |

**全链路（`internal/api`，fakellm harness，默认运行）**

| ID | 用例 | 断言 |
|---|---|---|
| E01 | 纯对话流式，fake 每块间隔 50ms 输出 5 块 | 首个 `token` 帧在上游首块后 ≤100ms 到达；`token` 帧数 ≥ 5（真流式回归） |
| E02 | 文本协议 R02 场景 | 恰好 1 次 `list_databases`；最终正文含 fake 数据库名；fakellm 第二次请求体含 `TOOL_RESULT` |
| E03 | 原生协议 R02 场景 | 请求体含 `tools`；第二次请求含 `role=tool` + 匹配的 `tool_call_id` |
| E04 | `auto` 遇 400 `tools not supported` | 同次 run 内降级 text 成功；第二个 run 首请求即不带 `tools` |
| E05 | 两轮记忆（R05） | 第二轮 fakellm 收到的历史含 `TOOL_HISTORY` 与上轮结果 |
| E06 | 上游 401 / 429 / 500 | SSE `error.code` 分别为 `llm_auth_failed` / `llm_rate_limited` / `llm_upstream_error`，随后 `end`；响应、审计中不含 `test-key` 与 fakellm URL |
| E07 | 上游不返回，`agent_run_timeout=1s`（测试注入） | `llm_timeout`；run 状态 failed、`error_code=llm_timeout` |
| E08 | 模型不在 allowed_models | `llm_model_not_allowed`，fakellm 请求计数为 0 |
| E09 | 运行中 `POST .../cancel` | ≤500ms 收到 `end(reason=canceled)`；部分正文落库；run 状态 canceled |
| E10 | 客户端断开（关闭响应 body） | 同 E09 落库语义；线程锁释放（随后新 run 成功） |
| E11 | 同 thread 并发 | 第二个 409 `agent_thread_busy`；第一个完成后可再次运行 |
| E12 | 重试（BUG-03） | `retry_of_run_id` 后 user 消息数不变；fakellm 收到的历史中该 user 消息只出现一次 |
| E13 | 消息状态（BUG-06） | 取消 / 失败后 `GET messages` 的 assistant 项带 `run_status/error_code` |
| E14 | 工具执行期间心跳（BUG-02） | 注入心跳 10ms、工具阻塞 60ms：`tool_call` 与 `tool_result` 之间无 `thinking`，有 `tool_progress` |
| E15 | 指标 | `GET .../runs` 返回 fakellm usage 中的 token 数、`tool_calls`、`duration_ms>0` |
| E16 | 通用助手跨模块 | 剧本依次调 `list_databases` → `search_logs`，两对事件、两张卡、正文含两类结果 |
| E17 | 只读 key | 创建 run 403；`GET messages`/`runs`/`models` 200 |
| E18 | 跨项目 | 他项目的 thread / run / retry_of_run_id 一律 404 |
| E19 | `models` 接口 | 返回 allowed_models 名称；响应体不含 `base_url`、`api_key`、fakellm 地址 |

**handler 单测补充（fake runtime）**：A15（BUG-02 帧序）、A16（retry 校验分支）、A17（messages DTO）、A18（models DTO 与 LLM 未启用）。

### 5.3 前端 vitest（覆盖率门槛 95% 不下调）

| ID | 文件 | 用例 |
|---|---|---|
| U12 | `useAgentConversation.test.ts` | 逐帧推送 token / tool_call / tool_result，每帧后 `nextTick` 断言渲染内容递增（BUG-01 回归） |
| U13 | `AgentManager.test.ts` | 运行中删除非当前会话不触发 stop；删除当前会话弹确认 |
| U14 | 同上 | stop / error / end 三条路径均刷新会话列表 |
| U15 | `AgentMessage.test.ts` | `run_status=canceled` 显示已停止；`error_code` 显示中文文案 |
| U16 | `useAgentConversation.test.ts` | stop 后回填 metrics；回填失败不报错 |
| U17 | `AgentCard.test.ts` | 单行渲染、模块图标、定时圆点、⋯ 菜单编辑/定时/删除、键盘 Enter 选中、内置 badge、运行中 spinner |
| U18 | `AgentList.test.ts` | 搜索过滤、Skeleton、空态、移动端 Sheet 打开 |
| U19 | `AgentThreadSwitcher.test.ts` | 搜索、切换、重命名、删除、运行中切换需确认（BUG-12）、加载更多 |
| U20 | `AgentToolCard.test.ts` | running / ok / error 三态、`truncated` 提示、折叠展开、duration 显示 |
| U21 | `AgentMarkdown.test.ts` | Markdown 渲染、代码块、`<script>` 被 DOMPurify 去除 |
| U22 | `agent-errors.test.ts` | 每个 code 的文案与 retryable |
| U23 | `AgentManager.test.ts` | 首次进入默认选中通用助手；模型 Combobox 显示 allowed_models |
| U24 | `AiChat.test.ts` | 拆分后 LLM 页行为不回归 |

v4.0 的 U01–U11 保留并迁移到新组件路径。

### 5.4 真实模型冒烟（`llm_integration`，不进默认 CI）

`internal/api/agent_llm_integration_test.go` 复用 §5.1 harness，只是把 fakellm URL 换成环境变量中的真实 provider。补齐 R01–R10（定义同 v4.0），新增：

| ID | 用例 | 通过条件 |
|---|---|---|
| R11 | 通用助手「项目里有哪些库，最近有没有 error 日志」 | 至少调用 1 个数据库工具与 1 个日志工具 |
| R12 | 重试：R08 失败后恢复正确 key 重试 | user 消息不重复，第二次成功 |

每个用例输出一行 `R0x PASS|FAIL ttfb_ms=.. total_ms=.. tool_calls=..`，供写入 `IMPLEMENTATION_SUMMARY.md`。

### 5.5 测试脚本（新增 `scripts/agent/`）

| 脚本 | 用途 |
|---|---|
| `scripts/agent/fakellm/main.go` | 以进程方式启动 §5.1 的 fakellm（`go run ./scripts/agent/fakellm -addr :18081 -script default`），让本地 dev server 不配真实 key 也能完整联调 UI |
| `scripts/agent/test.sh` | 一键回归：`go build ./internal/... ./cmd/...` → `go test ./internal/cloudagent ./internal/api ./internal/llmgateway ./internal/systemdb ./internal/config` → `go test -race` 同范围 → `cd ui && yarn build && yarn test`；任一失败即退出非零 |
| `scripts/agent/smoke.sh` | 对运行中的服务做 curl 冒烟：创建 thread → 选通用助手 → 流式 run → 校验帧序（run → token → … → usage → end）、`call_id` 配对、取消、重试、runs 指标；依赖 `curl`、`jq`；参数 `BASE_URL`、`PROJECT_ID`、`SIMPLEBASE_API_KEY` 从环境读取，**不接受命令行明文 key** |
| `scripts/agent/llm-integration.sh` | 检查 `SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY` 是否存在（只打印「已设置/未设置」），运行 `go test -tags llm_integration ./internal/api -run 'TestLLM' -v -timeout 10m`，汇总 R01–R12 结果行 |

脚本统一 `set -euo pipefail`、变量加引号；`smoke.sh` 对 SSE 输出做脱敏后再打印（过滤 `Authorization`）。`docs/agent/e2e.md` 改为引用这些脚本，删除内联长命令。

本地 UI 联调流程：

```bash
go run ./scripts/agent/fakellm -addr :18081 &
SIMPLEBASE_LLM_ENABLED=true SIMPLEBASE_LLM_PROVIDERS=openai \
SIMPLEBASE_LLM_PROVIDER_OPENAI_BASE_URL=http://127.0.0.1:18081/v1 \
SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY=test-key \
SIMPLEBASE_LLM_PROVIDER_OPENAI_DEFAULT_MODEL=fake-model \
SIMPLEBASE_LLM_PROVIDER_OPENAI_ALLOWED_MODELS=fake-model \
./build.sh dev --no-open
PROJECT_ID=<id> SIMPLEBASE_API_KEY=<dev key> ./scripts/agent/smoke.sh
```

---

## 6. 配置变更

沿用 v4.0 的 `agent_tool_protocol`、`agent_max_iterations`、`agent_run_timeout`。新增仅测试可注入的内部参数（不进 YAML）：`streamRun` 心跳 / thinking 间隔、`maxToolCardChars`，通过 handler 构造选项传入。

---

## 7. 实施顺序

每步结束 `./scripts/agent/test.sh` 通过（S0 前用等价命令）。

| 步骤 | 内容 | 测试 |
|---|---|---|
| S0 | 测试基础设施：fakellm、全链路 harness、SSE 辅助、`scripts/agent/test.sh` | E01–E03 先以**失败用例**落地，确认能复现 V1/V8 |
| S1 | 后端 bug：BUG-02、03、06、08、09 + `tool_result.duration_ms/is_error` | B19–B24、E06–E15、A15–A17 |
| S2 | 通用 Agent + 内置播种迁移 + 模型列表接口（BUG-07、11） | B26–B28、S01–S03、E16–E19、A18 |
| S3 | 前端状态层：`useAgentConversation`、`agent-errors.ts`（BUG-01、05、10） | U12、U14、U16、U22 |
| S4 | 前端 UI：Agent 列表/卡片、会话切换器、对话组件族、AiChat 拆分（BUG-04、12） | U13、U15、U17–U21、U23、U24 + 迁移 U01–U11 |
| S5 | 脚本：fakellm 进程、`smoke.sh`、`llm-integration.sh`；文档 `docs/agent/e2e.md` 更新 | 本地 fakellm 跑通 smoke；有 key 时跑 R01–R12 |

S0–S2 一个 PR（后端）；S3–S4 一个 PR（前端）；S5 单独 PR。

### 7.1 v4.0 已完成项（保留，不重复实施）

真流式前缀嗅探、事件去重与 call_id、工具历史摘要、迭代上限 / 超时 / 线程锁 / 取消落库、SSE 心跳与 usage 帧、run 指标迁移（v40）、原生 tools + auto 降级、会话分页 / 重命名 / runs 接口。v4.0 详细设计见 git 历史中本文件的上一版本。

---

## 8. 验收

1. 无 key 环境：`./scripts/agent/test.sh` 全绿；fakellm + dev server 下 `smoke.sh` 全部检查通过。
2. 控制台（fakellm 或 DeepSeek-V4-Flash）：正文逐字出现；工具卡在调用时即出现「运行中」，结果到达后变「成功 / 失败」并显示耗时；工具执行期间状态条不回跳「思考中」。
3. 左侧为单行简化 Agent 卡片，默认选中「通用助手」；会话在头部切换；移动端抽屉可用；键盘可完成选择 Agent、切换会话、发送、停止。
4. 失败后重试不产生重复用户消息；刷新后已停止 / 失败状态仍可见。
5. 老项目自动补出通用助手，删除后不再补回。
6. 模型下拉显示 allowed_models；接口与页面不暴露 base_url / key。
7. 有 key 时 `llm-integration.sh` 的 R01–R12 结果记入 `IMPLEMENTATION_SUMMARY.md`；未执行则明确标注「未执行」。

WCAG 相关只做到语义与键盘可达；完整无障碍合规仍需借助辅助技术人工测试。

---

## 9. 风险与取舍

| 风险 | 取舍 |
|---|---|
| 拆分 AiChat 影响 LLM 页 | AiChat 只删 Agent 专用代码路径，U24 回归；不改其对外 props 语义 |
| 内置 Agent 迁移改变老项目列表 | 只补通用助手，存量 Agent 只回填 `builtin_key` 不改内容；删除可永久关闭 |
| 通用 Agent 工具多导致模型乱调工具 | prompt 明确「先判断问题类别」；`agent_max_iterations` 兜底；R11 观察实际表现 |
| fakellm 与真实 provider 行为偏差 | 剧本按 §1 实测（v4.0）的帧格式编写；真实差异由 `llm_integration` 补充 |
| Markdown 渲染 XSS | 统一经 DOMPurify；U21 覆盖 |
| `retry_of_run_id` 被滥用重放 | 仅允许 thread 最新且 failed/canceled 的 run，且受原有配额检查 |
| 思考正文仍不可见 | 保持 v4.0 取舍：只显示耗时，待 litellm 升级（需用户同意） |
| `scripts/` 新增 Go main 包会被 `go test ./...` 编译 | 包内无外部依赖、无 init 副作用；`go vet` 覆盖 |
