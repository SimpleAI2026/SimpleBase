# 云 Agent 前后端功能优化 v4

> **仓库**：SimpleBase
> **状态**：部分完成（2026-10-07）。S1–S7 已实现并通过本地回归；S8 已写入联调文档与可选真实模型测试，但未以实际凭据运行，R01–R10 尚未全部验收。见 §8.1。
> **前置**：`plan/planv2.0/cloud-agent-plan.md`、`plan/planv3.0/cloud-agent-sandbox-plan.md`、`plan/planv4.0/cloud-sandbox-plan.md`
> **关联代码**：`internal/cloudagent/{runtime,model,tools,prompt,modules,scheduler}.go`、`internal/api/{agent_handler,cloudagent_access}.go`、`internal/llmgateway/service.go`、`ui/src/pages/AgentManager.vue`、`ui/src/components/ai/AiChat*.vue`、`ui/src/services/http-api.ts`
> **联调模型**：SiliconFlow `deepseek-ai/DeepSeek-V4-Flash`（OpenAI 兼容 `/v1/chat/completions`）

---

## 0. 一句话

让云 Agent **真流式、工具卡片准确、多轮记得住工具结果、错误可读、可观测**，前端补齐会话管理、模型选择、运行状态与思考过程展示；全部改动不新增依赖，并以 SiliconFlow DeepSeek-V4-Flash 作为真实模型做冒烟验收。

---

## 1. 联调模型实测（2026-10-06）

用给定 key 直连 `https://api.siliconflow.cn/v1/chat/completions` 实测：

| 场景 | 结果 | 对方案的影响 |
|---|---|---|
| 原生 `tools` 参数 | 200，`finish_reason=tool_calls`，`tool_calls[0].function.name=list_databases`，约 7.9s | 模型支持原生 function calling，可作为优先协议（§4.3） |
| 现有文本协议 `TOOL_CALL {...}` | 200，`content` 恰好为 `TOOL_CALL {"name":"list_databases","arguments":{}}`，约 7.6s | 现有协议可用，保留为回退 |
| 流式 | 首包 TTFB ≈ **0.82s**，总时长 ≈ **6.6s** | 现实现把整段缓冲后再切片，用户要等 6s+ 才看到第一个字（§2 P1） |
| 流式 delta | 先输出大量 `delta.reasoning_content`，`delta.content=null`，之后才出正文 | litellm `openai` provider 只解析 `ReasoningSummary`，`reasoning_content` 被丢弃 → 思考阶段前端完全静默（§2 P5） |
| usage | 带 `completion_tokens_details.reasoning_tokens` | 用量可多记一列（可选，§4.7） |

接入方式（无需代码改动即可接通）：provider 用 litellm 内置 `openai`，`base_url` 指向 SiliconFlow。litellm `buildURL` 对以 `/v1` 结尾的 base_url 直接拼 `/chat/completions`。

```bash
# .env（已 gitignore）或进程环境；密钥不得写入 config.yaml / plan / 测试代码
SIMPLEBASE_LLM_ENABLED=true
SIMPLEBASE_LLM_PROVIDERS=openai
SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY=<SiliconFlow key>
SIMPLEBASE_LLM_PROVIDER_OPENAI_BASE_URL=https://api.siliconflow.cn/v1
SIMPLEBASE_LLM_PROVIDER_OPENAI_DEFAULT_MODEL=deepseek-ai/DeepSeek-V4-Flash
SIMPLEBASE_LLM_PROVIDER_OPENAI_ALLOWED_MODELS=deepseek-ai/DeepSeek-V4-Flash
SIMPLEBASE_LLM_PROVIDER_OPENAI_TIMEOUT=120s
```

> 安全：密钥只放 `.env` / CI secret。本文件、测试代码、日志、审计、SSE 一律不得出现明文 key（AGENTS.md 硬性约束 2、3）。用户已在对话中给出 key，落地前确认是否需要轮换。

---

## 2. 问题清单（代码走查结论）

| # | 位置 | 问题 | 级别 |
|---|---|---|---|
| P1 | `cloudagent/model.go` `Stream` | 读完整个上游流后才 `parseAssistant`，再按 16 rune 切片下发；**假流式**，首字延迟 = 全量生成时间 | 高 |
| P2 | `cloudagent/runtime.go` `StartRun` + `consumeAssistant` | 流式路径下 `tool_call` 事件**发两次**（`consumeAssistant` 内一次、`StartRun` 再按 `msg.ToolCalls` 一次）；前端 `onToolCall` 追加两张卡，`onToolResult` 只填一张，留下一张空卡 | 高 |
| P3 | `api/agent_handler.go` `CreateRun` → `historyToSchema` | 工具结果只写进 assistant 消息的 `ToolCallsJSON`，历史回放只用 `Content`；下一轮模型**看不到上轮工具数据**，常重复调用 | 高 |
| P4 | `streamRun` 错误路径 | 统一发 `"run failed"`；超时、配额、provider 401/429、模型不在白名单无法区分；取消时已生成的部分回复丢失、不落库 | 中 |
| P5 | litellm openai 流解析 | `reasoning_content` 被丢弃；DeepSeek-V4 思考阶段数秒无任何 SSE 帧，前端像卡死；也无心跳，经代理可能被 idle 断开 | 中 |
| P6 | `llmgateway.Chat/Stream` | 请求不支持 `tools`；只能走文本协议，模型偶发把 `TOOL_CALL` 混进正文（`looksLikeToolCall` 粗判，正文里出现该字面量会被吞） | 中 |
| P7 | `runtime.go` | `MaxIterations: 8` 写死；达上限时报错而非给出「已达工具调用上限」的可读总结；无单次 run 总超时 | 中 |
| P8 | `CreateRun` | 同一 thread 可并发多个 run，历史交错；无 per-thread 互斥 | 中 |
| P9 | 观测 | run 不记录耗时、token、工具次数；`sys_agent_runs` 只有状态 | 低 |
| F1 | `AgentManager.vue` | 只取 `threads[0]`，**无会话列表/切换/重命名/删除** | 高 |
| F2 | `AgentManager.vue` 表单 | 后端支持 `model_override`，表单无此字段；用户无法为 Agent 指定 DeepSeek-V4-Flash | 中 |
| F3 | `onError` | 失败时 assistant 气泡停留为空，无重试按钮；`error` 后紧跟 `end` 没有区分 | 中 |
| F4 | 运行状态 | 无「思考中 / 调用工具 xx / 已用 n 秒」状态条；停止后无「已取消」标记 | 中 |
| F5 | 工具卡片 | 结果为原始 JSON 字符串；SQL 结果、沙盒 stdout/exit_code 未结构化展示 | 低 |

---

## 3. 目标与非目标

### 3.1 目标

| # | 目标 | 验收口径 |
|---|---|---|
| G1 | 真流式 | DeepSeek-V4-Flash 下首个 `token`/`thinking` SSE 帧 ≤ 上游 TTFB + 300ms |
| G2 | 工具事件准确 | 每次工具调用恰好 1 个 `tool_call` + 1 个 `tool_result`，带同一 `call_id` |
| G3 | 多轮记忆工具结果 | 第二轮问「刚才那几个库里哪个最大」不再重新调 `list_databases`（真实模型冒烟中观察，单测用 fake 断言历史内容） |
| G4 | 错误可读 | SSE `error` 带稳定 `code`；前端按 code 给中文提示与重试 |
| G5 | 前端会话管理 + 模型选择 + 运行状态 | §5 全部交互有 vitest 用例 |
| G6 | 可观测 | run 记录 `duration_ms`、`prompt/completion/reasoning tokens`、`tool_calls`；Agent 页可见 |

### 3.2 非目标

- 多 Agent 协作 / Team 模式（仍为 stub）。
- 新增第三方依赖、升级 litellm / eino 版本。
- 改动 sandbox Manager、DuckLake、S3 数据面。
- 浏览器 e2e 框架（沿用 vitest + mock）。

---

## 4. 后端方案

### 4.1 真流式（P1）

`gatewayChatModel.Stream` 改为增量转发 + 前缀嗅探：

1. 缓冲区只保留**尚不能确定是否为工具调用**的前缀。读到的累计正文（去前导空白）满足以下之一即判定：
   - 以 `TOOL_CALL` 开头 → 切到「工具模式」，继续缓冲至流结束后 `parseAssistant`；
   - 长度 ≥ `len("TOOL_CALL")` 且不以其开头 → 切到「正文模式」，先把缓冲 flush 成一个 chunk，之后每个上游 chunk 直接 `sw.Send`。
2. 正文模式下不再做 16 rune 切片。
3. 原生 tool_calls（§4.3）到达时直接组装 `schema.ToolCall`，无需嗅探。
4. 删除 `consumeAssistant` 里 `looksLikeToolCall(chunk.Content)` 的过滤（正文里合法出现 `TOOL_CALL` 字面量不再被吞）。

### 4.2 工具事件去重 + call_id（P2）

- `tool_call` 只在 `StartRun` 统一发一次：`consumeAssistant` 只负责累积并返回最后的 message，不再 emit tool_call。
- `Event` 增加 `CallID string json:"call_id,omitempty"`：`tool_call` 取 `tc.ID`，`tool_result` 取 `mv.Message.ToolCallID`。
- `toolCards` 落库结构改为 `{call_id, name, arguments, content, duration_ms}`；读取时兼容旧的 `{name, content}`。

### 4.3 原生 function calling（P6，可降级）

- `llmgateway.Request` 增加 `Tools []providers.Tool`、`ToolChoice string`；`Response` 增加 `ToolCalls`；流式 reader 透出 `tool_call_delta`（litellm 已解析）。
- `cloudagent.ChatRequest/ChatResponse/TokenStream` 同步增加 `Tools` / `ToolCalls` / 增量 delta（接口在 cloudagent 内声明，api adapter 映射，保持分层）。
- `gatewayChatModel` 策略：
  - 配置 `llm.agent_tool_protocol: auto | native | text`（默认 `auto`，env `SIMPLEBASE_LLM_AGENT_TOOL_PROTOCOL`）。
  - `auto`：先 native；若 provider 返回 4xx 且错误含 `tools`/`function` 关键词，进程内按 `provider|model` 记住降级为 text（LRU，不落库）。
  - `text`：保持现有 `toolProtocolPrompt`。
- 历史中的工具往返在 native 模式下用 `role=tool` + `tool_call_id`；text 模式保持 `TOOL_RESULT name=... id=...`。

### 4.4 推理内容与心跳（P5）

- litellm 不改（不升级依赖）。在 `llmgateway` 包装 reader：上游 provider 为 `openai` 时，若 `StreamChunk.Type=="reasoning"` 透传；对 `reasoning_content` 未被解析的情况，**不自行解析原始 SSE**（会绕开 litellm），改为由 Runtime 侧在「上游无正文输出」期间每 2s 发一个 `{"type":"thinking","elapsed_ms":N}` 心跳帧。
- 若后续 litellm 版本支持 `reasoning_content`（用户同意升级时），Runtime 将其映射为 `{"type":"thinking","content":"..."}`；前端默认折叠显示。
- `streamRun` 另起 15s 间隔的 SSE 注释心跳 `: ping\n\n`，防代理 idle 断开；写操作统一走带锁的 `write`，避免与 runtime emit 并发写 `Response`。
- 思考内容**不落库、不进审计**（与「不记 LLM 正文」同尺度；正文本身仍按现状落 `sys_agent_messages`）。

### 4.5 多轮历史带工具摘要（P3）

`historyToSchema` 对 assistant 消息：若 `ToolCallsJSON` 非空，在其前插入一条压缩摘要：

```text
TOOL_HISTORY
- list_databases {} → [{"name":"orders",...}]   (≤600 chars/条, 最多 5 条, RedactSecrets 后)
```

总预算仍受 `maxHistoryChars=12000` 约束；工具摘要优先级低于最近 2 轮对话正文，超出预算先丢最早的工具摘要。

### 4.6 运行控制（P7、P8）

- 配置 `llm.agent_max_iterations`（默认 8，范围 1–20）、`llm.agent_run_timeout`（默认 180s）。
- 达到 MaxIterations：捕获 eino 的超限错误，返回已累积正文 + 固定句「已达到工具调用上限（N 次），以上为当前结论」，run 状态 `completed`，`error_code=max_iterations`。
- per-thread 互斥：`Runtime` 内 `map[threadID]runID`；同 thread 已有 running 的 run → `CreateRun` 返回 409 `agent_thread_busy`。进程内即可（单写实例前提）。
- 取消：`StartRun` 返回 `context.Canceled` 时，把已生成的部分正文以 assistant 消息落库，`run.status=canceled`，消息内容尾部不追加任何提示（前端负责显示「已停止」）。
- 客户端断开（SSE 连接关闭）视同取消。

### 4.7 错误码与观测（P4、P9）

`internal/cloudagent/errors.go` 新增领域错误，`api/error.go` 映射；SSE `error` 帧携带 `code` 与固定中文/英文 message，不透传 provider 原文（可能含 URL / key 片段）：

| code | 触发 | HTTP（非流式） |
|---|---|---|
| `llm_not_configured` | runtime/LLM nil 或无 provider | 503 |
| `llm_auth_failed` | 上游 401/403 | 502 |
| `llm_rate_limited` | 上游 429 | 429 |
| `llm_model_not_allowed` | 模型不在 allowed_models | 400 |
| `llm_timeout` | `agent_run_timeout` 或上游超时 | 504 |
| `llm_upstream_error` | 其他上游错误 | 502 |
| `agent_thread_busy` | §4.6 | 409 |
| `quota_exceeded` | 现有 `CheckQuota` | 429 |
| `max_iterations` | 仅作为 `end` 帧附带的 `reason`，不是 error | — |

观测：
- 迁移新增 `sys_agent_runs` 列：`duration_ms INTEGER`、`prompt_tokens`、`completion_tokens`、`reasoning_tokens`、`tool_calls INTEGER`、`error_code VARCHAR`（版本号接当前最大值 +1）。
- token 来自 gateway usage（流式取最后一帧 usage）。
- SSE 新增末尾帧 `{"type":"usage","duration_ms":..,"prompt_tokens":..,"completion_tokens":..,"tool_calls":..}`，在 `end` 之前。
- 审计 `agent.run`：只记 project/agent/thread/run id、模型名、耗时、token、工具次数、error_code；不记正文。
- 新增 `GET /v1/projects/:projectID/agent-threads/:threadID/runs?limit=20`（`database:read`）返回 run 列表含上述指标。

### 4.8 会话 API 补齐（服务 F1）

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/agent-threads?limit=50&cursor=` | read | 现有；补 `cursor` keyset 分页、`last_message_preview`（≤80 字）与 `updated_at` 排序 |
| PATCH | `/agent-threads/:threadID` | write | 仅改 `title`（1–80 字） |
| GET | `/agent-threads/:threadID/runs` | read | §4.7 |

首条用户消息后若 thread 标题为默认值，自动以消息前 30 字作为标题（不额外调 LLM）。

### 4.9 SSE 协议（最终形态，向后兼容）

```text
data: {"type":"run","run_id":"r1"}
data: {"type":"thinking","elapsed_ms":2000}                 # 可选，多次
data: {"type":"token","content":"我先"}
data: {"type":"tool_call","call_id":"c1","name":"list_databases","arguments":"{}"}
data: {"type":"tool_result","call_id":"c1","name":"list_databases","content":"[...]","duration_ms":38}
data: {"type":"token","content":"项目里有 3 个库…"}
data: {"type":"usage","duration_ms":6120,"prompt_tokens":812,"completion_tokens":96,"tool_calls":1}
data: {"type":"end","reason":"stop"}                        # stop | max_iterations | canceled
# 失败：data: {"type":"error","code":"llm_rate_limited","message":"模型服务限流，请稍后重试"} 之后仍发 end
```

旧字段不删；旧前端忽略未知 type 即可工作。

---

## 5. 前端方案（`ui/`）

遵守 `ui/AGENTS.md`：`interface` 定义形状、Tailwind class、`vue-sonner` / `ConfirmAction` / `SbModal` / `SbEmptyState`，不新增依赖。

### 5.1 会话侧栏（F1）

- `AgentManager.vue` 对话卡片左侧（md 以上）增加窄列 `AgentThreadList.vue`：列表（标题、预览、相对时间）、新建、重命名（行内 Input）、删除（`ConfirmAction`）；当前会话高亮。
- 移动端以 `Select` 切换会话。
- 路由 query `?thread=<id>` 同步当前会话，刷新后恢复；`onViewScheduleThread` 改为写 query。

### 5.2 Agent 表单（F2）

- 新增「模型」字段：`Select`，选项来自现有 `GET /llm/settings`（或 providers 的 `allowed_models`）；空值显示「使用项目默认（deepseek-ai/DeepSeek-V4-Flash）」。
- 工具 badge 增加中文说明 tooltip（`title` 属性即可）。

### 5.3 运行状态与错误（F3、F4）

- `useAgentRun` composable（新文件 `ui/src/composables/useAgentRun.ts`）收拢 `onSend/onStop` 状态机：`idle → thinking → streaming → tool(name) → done | error | canceled`。
- 输入框上方状态条：`思考中 · 3s` / `调用 readonly_sql…` / `完成 · 6.1s · 908 tokens`。
- `error`：assistant 气泡显示错误 code 对应中文 + 「重试」按钮（重发上一条用户消息，复用 mentions）；`agent_thread_busy` 时提示「当前会话仍在运行」。
- `canceled`：气泡尾部灰字「已停止」。
- `thinking` 帧有 content 时显示可折叠「思考过程」。

### 5.4 工具卡片（F5）

- 按 `call_id` 匹配 call/result，不再按 name 反查。
- `readonly_sql` 结果：表格（前 20 行 + 「共 N 行」）；`sandbox_*`：stdout / stderr 分块 + `exit N` badge；其余：格式化 JSON（`<pre>`，默认折叠 > 20 行）。
- 新组件 `ui/src/components/ai/AgentToolCard.vue`。

### 5.5 服务层

- `services/types.ts`：`AgentStreamEvent`、`AgentRunMetrics`、`AgentThreadItem`（含 `lastMessagePreview`），后端 snake_case 经 `toAgentThreadItem` 映射。
- `http-api.ts` `streamAgentRun`：解析 `call_id` / `thinking` / `usage` / `error.code` / `end.reason`；`AgentStreamHandlers` 增加 `onThinking`、`onUsage`、`onEnd(reason)`；SSE 注释行（`:` 开头）忽略。
- `agentThreads.rename`、`agentThreads.runs`；`mock.js` 同步实现。

---

## 6. 配置变更

```yaml
llm:
  enabled: true
  agent_tool_protocol: auto     # auto | native | text；SIMPLEBASE_LLM_AGENT_TOOL_PROTOCOL
  agent_max_iterations: 8       # 1–20；SIMPLEBASE_LLM_AGENT_MAX_ITERATIONS
  agent_run_timeout: 180s       # SIMPLEBASE_LLM_AGENT_RUN_TIMEOUT
```

`Validate`：协议枚举、迭代范围、超时 10s–15m。`config.example.yaml` 补注释与 SiliconFlow 示例（key 只写 env 变量名）。

---

## 7. 测试用例

### 7.1 后端单元测试（默认 `go test ./...`，不访问外网）

fake `ChatClient` 以脚本化 chunk 序列驱动。

| ID | 包 / 文件 | 用例 | 断言 |
|---|---|---|---|
| B01 | `cloudagent/model_stream_test.go` | 上游依次吐 `"你"`、`"好"`、`"，世界"` | 下游收到 ≥2 个 chunk，首 chunk 在第 1 个上游 chunk 后（用 channel 阻塞上游第 2 块，断言此时已收到首块） |
| B02 | 同上 | 上游吐 `"TOOL"`、`"_CALL {\"name\":\"list_databases\",\"arguments\":{}}"` | 下游仅 1 条含 `ToolCalls[0].Function.Name=list_databases`，无正文 chunk |
| B03 | 同上 | 正文中含字面量 `"示例：TOOL_CALL 是协议关键字"` | 正文完整下发，不被解析为工具 |
| B04 | 同上 | 上游前导空白 `"\n  TOOL_CALL {...}"` | 判定为工具模式 |
| B05 | `cloudagent/runtime_events_test.go` | 流式 run，一次工具调用 | `tool_call` 事件恰好 1 个、`tool_result` 1 个，`call_id` 相同且非空 |
| B06 | 同上 | 非流式 run，两次工具调用 | `ToolCallsJSON` 两条，含 `call_id/arguments/duration_ms` |
| B07 | 同上 | 模型连续请求工具 9 次，`agent_max_iterations=8` | 返回 nil error、内容含「已达到工具调用上限（8 次）」、reason=`max_iterations` |
| B08 | 同上 | run 中途 cancel | 返回 `context.Canceled`，`RunResult.Content` 为已生成部分 |
| B09 | 同上 | 上游 8s 无输出、心跳间隔注入为 10ms | 收到 ≥1 个 `thinking` 事件，含 `elapsed_ms` 递增 |
| B10 | `cloudagent/prompt_history_test.go` | 历史 assistant 带 `ToolCallsJSON` | 生成的 schema 消息含 `TOOL_HISTORY` 与工具名；含 `AKIA...`/`sk-...` 的结果被 `RedactSecrets` |
| B11 | 同上 | 工具摘要超预算 | 最近 2 轮正文保留，最早工具摘要被丢弃，总长 ≤ `maxHistoryChars` |
| B12 | `cloudagent/model_native_test.go` | `protocol=native`，fake 返回 `ToolCalls` | 请求带 `Tools`，不注入 `toolProtocolPrompt` system 消息 |
| B13 | 同上 | `protocol=auto`，首次 400 `tools not supported` | 自动降级 text 重试成功；同 `provider|model` 第二次直接走 text |
| B14 | `cloudagent/errors_test.go` | 上游错误分别为 401 / 429 / timeout / 其它 | 映射为 `llm_auth_failed / llm_rate_limited / llm_timeout / llm_upstream_error`，message 不含上游原文与 URL |
| B15 | `llmgateway/service_tools_test.go` | `httptest` 模拟 OpenAI 兼容服务，返回 `tool_calls` | `Response.ToolCalls` 正确；请求 JSON 含 `tools` |
| B16 | 同上 | `httptest` 流式返回 usage 尾帧 | recorder 收到 prompt/completion tokens |
| B17 | `config/agent_config_test.go` | env 覆盖三项；非法协议、迭代 0/21、超时 5s | Validate 失败信息明确 |
| B18 | 同上 | YAML 写入 `llm.providers.openai.api_key` 且 `dev_mode=false` | 被 `rejectYAMLSecrets` 拒绝（回归） |

### 7.2 API / handler 测试（fake runtime，`internal/api`）

| ID | 用例 | 断言 |
|---|---|---|
| A01 | `POST /agent-threads/:id/runs` stream=true 正常 | 帧顺序 `run → token* → usage → end`，`Content-Type: text/event-stream` |
| A02 | 同 thread 并发两个 run（第一个阻塞在 fake 上） | 第二个 409 `agent_thread_busy` |
| A03 | runtime 返回 `llm_rate_limited` | 流式：`error.code=llm_rate_limited` 后紧跟 `end`；非流式：HTTP 429 + `code` |
| A04 | 客户端在首个 token 后断开 | run 状态 `canceled`；部分正文落为 assistant 消息 |
| A05 | run 完成 | `sys_agent_runs` 写入 `duration_ms/tokens/tool_calls`；`GET .../runs` 返回这些字段 |
| A06 | `PATCH /agent-threads/:id` title 为空 / 81 字 / 正常 | 400 / 400 / 200 |
| A07 | 只读 key `PATCH /agent-threads/:id` | 403 |
| A08 | 跨项目 thread id | 404（不暴露存在性） |
| A09 | `GET /agent-threads?limit=2` 共 3 条 | 返回 2 条 + `next_cursor`；带 cursor 取到第 3 条；伪造 cursor 400 |
| A10 | 首条消息后默认标题 | 标题变为消息前 30 字 |
| A11 | 实例 `writable=false` 下 `PATCH` thread | 503 `ErrWriterUnavailable` |
| A12 | admin 系统项目下 Agent 运行 `readonly_sql` 写语句 | 仍被 sqlguard 拒绝（回归，系统库保护） |
| A13 | 审计记录 | 含 run/agent/thread id、tokens、error_code；**不含** 用户消息正文与模型输出（扫描断言） |
| A14 | SSE 心跳 | 注入 10ms 间隔，响应体含 `: ping` 行 |

### 7.3 前端 vitest（`ui/tests/`，覆盖率门槛 95% 不下调）

| ID | 文件 | 用例 |
|---|---|---|
| U01 | `http-api.agentStream.test.ts` | mock fetch 返回分段 SSE（跨 chunk 切断的 `\n\n`、`: ping` 注释行）：handlers 依次收到 run/token/tool_call/tool_result/usage/end(reason) |
| U02 | 同上 | `error` 帧带 code → `onError` 的 Error 带 `code` 属性；之后 `end` 不再重复触发 onEnd |
| U03 | `useAgentRun.test.ts` | 状态机：thinking → streaming → tool → done；cancel → canceled；error → error |
| U04 | `AgentThreadList.test.ts` | 渲染列表、新建、行内重命名提交、删除确认后从列表移除、当前项高亮 |
| U05 | `AgentManager.test.ts`（扩展） | `?thread=` 恢复会话；切换会话写回 query；运行中切换会话先 stop |
| U06 | 同上 | 表单模型下拉：默认「使用项目默认」，选择 DeepSeek-V4-Flash 后 create/patch payload 含 `model_override` |
| U07 | 同上 | 错误气泡显示「模型服务限流」，点击「重试」以相同 content + mentions 重发 |
| U08 | 同上 | `agent_thread_busy` 显示 toast「当前会话仍在运行」 |
| U09 | `AgentToolCard.test.ts` | 同名工具两次调用按 `call_id` 各自配对；SQL 结果渲染表格；sandbox 结果显示 `exit 3`；超长 JSON 默认折叠 |
| U10 | 同上 | 旧格式 `{name, content}`（无 call_id）兼容显示 |
| U11 | `AgentManager.test.ts` | 状态条显示 `思考中 · Ns`，收到 usage 后显示耗时与 tokens |

### 7.4 真实模型冒烟（不进默认 CI）

实际测试文件：`internal/api/agent_llm_integration_test.go`，`//go:build llm_integration`。无 `SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY` 时 `t.Skip`。用真实 `llmgateway` + fake DB 访问层；现已覆盖 native/text 工具往返与模型白名单，其余 R01–R10 场景尚待补充及真实执行。

```bash
set -a; source .env; set +a      # 含 §1 的 SIMPLEBASE_LLM_* 变量
go test -tags llm_integration ./internal/api -run 'TestLLMRealStreamAndToolRoundtrip|TestLLMModelAllowlistRejectsBeforeUpstream' -v -timeout 5m
```

| ID | 用例 | 通过条件 |
|---|---|---|
| R01 | 纯对话「1+1=? 只回答数字」流式 | 首个 token/thinking 事件 ≤ 3s；最终正文含 `2` |
| R02 | database 模块「列出项目里的数据库」，fake 返回 `orders, users` | 恰好 1 次 `list_databases` 调用；正文同时出现 `orders` 与 `users` |
| R03 | `protocol=text` 重复 R02 | 同 R02（验证文本协议回退） |
| R04 | `protocol=native` 重复 R02 | 同 R02，且请求带 `tools` |
| R05 | 两轮：R02 后问「其中第二个库叫什么」 | 第二轮 0 次工具调用，正文含 `users`（验证 §4.5） |
| R06 | readonly_sql「统计 orders 行数」，fake SQL 返回 `[[42]]` | 调用 `readonly_sql` 且 SQL 为 SELECT；正文含 `42` |
| R07 | 诱导写入「删除 orders 表」 | 不产生 DROP/DELETE 工具调用，或调用被 sqlguard 拒绝且正文解释原因 |
| R08 | 错误 key（环境里临时替换） | 得到 `llm_auth_failed`，错误与日志中不含 key 片段 |
| R09 | 不存在的模型名 | `llm_model_not_allowed`（被 allowed_models 拦截，不发上游） |
| R10 | 取消：发出后 1s 调 `CancelRun` | 返回 canceled，≤2s 内结束 |

### 7.5 端到端 curl 脚本（手工验收）

`scripts` 不新增目录；放在文档 `docs/agent/e2e.md`：

```bash
./build.sh dev --no-open                       # 读取 .env 中的 SIMPLEBASE_LLM_*
KEY=sb_live_dev_key_12345; P=<project_id>; H="Authorization: Bearer $KEY"
TH=$(curl -s -XPOST localhost:8080/v1/projects/$P/agent-threads -H "$H" -d '{"title":"e2e"}' -H 'Content-Type: application/json' | jq -r .id)
AG=$(curl -s localhost:8080/v1/projects/$P/agents -H "$H" | jq -r '.agents[] | select(.module=="database") | .id')
curl -N -XPOST localhost:8080/v1/projects/$P/agent-threads/$TH/runs -H "$H" -H 'Content-Type: application/json' \
  -d "{\"content\":\"列出项目里的数据库\",\"mentions\":[{\"agent_id\":\"$AG\"}],\"stream\":true}"
# 期望：run → (thinking)* → token… → tool_call(call_id) → tool_result(同 call_id) → token… → usage → end
curl -s localhost:8080/v1/projects/$P/agent-threads/$TH/runs -H "$H" | jq '.runs[0] | {status,duration_ms,completion_tokens,tool_calls}'
```

控制台手工：新建 Agent 选模型 DeepSeek-V4-Flash → 发送 → 观察「思考中」→ 逐字输出 → 工具卡片 → 状态条耗时/tokens → 新建/切换/重命名会话 → 刷新恢复 → 停止显示「已停止」。

---

## 8. 实施顺序

每步结束保持 `go build ./internal/... ./cmd/...`、`go test ./...`、`cd ui && yarn build && yarn test` 通过。

| 步骤 | 内容 | 测试 |
|---|---|---|
| S1 | 配置项 + 错误码 + `cloudagent/errors.go` 映射 | B14、B17、B18 |
| S2 | 真流式 + 事件去重 + call_id（P1、P2） | B01–B06、A01 |
| S3 | 历史工具摘要、迭代上限、超时、per-thread 互斥、取消落库（P3、P7、P8） | B07、B08、B10、B11、A02、A04 |
| S4 | 心跳 + thinking 帧 + usage 帧 + runs 指标迁移（P4、P5、P9） | B09、A03、A05、A13、A14 |
| S5 | gateway 原生 tools + auto 降级（P6） | B12、B13、B15、B16 |
| S6 | 会话 API（rename、分页、预览、runs 列表） | A06–A11 |
| S7 | 前端：服务层 + useAgentRun + 线程列表 + 模型选择 + 工具卡片 + 状态条 | U01–U11 |
| S8 | 真实模型冒烟 + 文档（`docs/agent/`、`config.example.yaml`、`IMPLEMENTATION_SUMMARY.md`） | R01–R10、§7.5 |

S1–S3 一个 PR；S4–S6 一个 PR；S7–S8 一个 PR。S5 可独立推迟，不影响其余步骤。

### 8.1 实施记录（2026-10-07）

| 步骤 | 当前状态 | 证据 / 说明 |
|---|---|---|
| S1 | 已实现 | `internal/config/` 新增工具协议、迭代上限、运行超时；`internal/cloudagent/errors.go` 固定错误码；`agent_config_test.go`、`errors_test.go` |
| S2 | 已实现 | `model.go` 流式前缀嗅探及增量输出；`runtime.go` 去重工具卡、生成 call_id；`optimization_test.go` 首块、跨块工具协议和事件配对测试 |
| S3 | 已实现 | 工具历史摘要、上限/超时、线程锁与取消部分回复落库；最近 40 条会话历史修复；`run_limits_test.go` 覆盖迭代上限与取消保留正文，HTTP 断连需手工验收 |
| S4 | 已实现 | SSE 心跳、`thinking` 空闲帧、`usage` 帧、系统库 v40 run 指标迁移、审计元数据；`agent_optimization_test.go` 指标与无正文审计测试；心跳间隔注入用例未实现 |
| S5 | 已实现 | gateway 原生 function calling、流式工具 delta/usage、400/422 协议降级及模型白名单；`agent_tools_test.go`、`model_native_test.go` |
| S6 | 已实现 | 会话分页/预览/重命名与运行记录接口，原项目隔离与权限；`agent_optimization_test.go`、`agent_history_test.go` |
| S7 | 已实现 | 会话列表及 URL 恢复、自由输入模型、运行状态/重试、call_id 工具配对、SQL/沙盒卡片；`yarn test` 四项覆盖率达标（branches **95.01%**） |
| S8 | 部分完成 | `docs/agent/e2e.md`、`config.example.yaml` 与 `//go:build llm_integration` 可选测试已就绪；未在本次实现会话真实模型全量 R01–R10 / 手工浏览器验收，不标记为完成 |

**已运行验证**：`go build ./internal/... ./cmd/...`、`go test ./...`、`go test -race ./internal/cloudagent ./internal/api ./internal/llmgateway ./internal/systemdb`、`cd ui && yarn build && yarn test` 均通过。前端 `yarn typecheck` 报项目原有的 vitest/vite 类型冲突和 Vue SFC 类型声明缺失，不修改 `tsconfig.json` 或依赖。

**偏离原计划**：模型表单以 `Input + datalist` 接收模型名，而非受限 Select（项目 `/llm/settings` 只返回默认模型，不返回所有 allowed_models）；真实冒烟测试文件位于 `internal/api/agent_llm_integration_test.go`，不在 `internal/cloudagent`，用于复用项目真实 LLM 适配链。此文件无密钥常量，未配置环境变量时跳过。

---

## 9. 验收

1. DeepSeek-V4-Flash 下，控制台发送后 ≤ 3s 出现「思考中」或首个字；正文逐字出现而非一次性出现。
2. 一次工具调用只出现一张卡片，结果与调用正确配对；SQL 结果表格化。
3. 两轮对话中第二轮能引用第一轮工具结果，不重复调用（R05）。
4. 限流 / 鉴权失败 / 超时 / 会话忙均有中文提示且可重试；日志、审计、SSE 中无 key、无正文。
5. 会话可新建、切换、重命名、删除，刷新恢复；Agent 可指定模型。
6. `GET .../runs` 与界面状态条可见耗时、tokens、工具次数。
7. 默认 `go test ./...` 不访问外网；`llm_integration` 冒烟 R01–R10 全部通过并把结果记入 `IMPLEMENTATION_SUMMARY.md`。

---

## 10. 风险与取舍

| 风险 | 取舍 |
|---|---|
| 不升级 litellm，`reasoning_content` 拿不到正文 | 先用 `thinking` 心跳（只含耗时）解决「卡死感」；思考正文待用户同意升级依赖后再做 |
| 前缀嗅探在正文以 `TOOL_CALL` 开头时误判 | 仅在去空白后**以其开头**才判工具；解析失败回退为正文整体下发 |
| 原生 tools 各 provider 兼容性不一 | `auto` 降级 + 进程内记忆；可配置强制 `text` |
| per-thread 互斥仅进程内 | 与当前单写实例架构一致；多实例随 `multi-instance-consistency-plan.md` 迁到系统库租约 |
| 历史工具摘要占用上下文 | 硬预算 + 优先级裁剪；摘要单条 ≤600 字 |
| 取消时部分正文落库可能是半句 | 前端标「已停止」；这是用户可见的真实输出，保留优于丢失 |
