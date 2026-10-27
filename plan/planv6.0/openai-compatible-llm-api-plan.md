# LLM 出口统一为 OpenAI 兼容协议计划（v6.0）

> **仓库**：SimpleBase（`github.com/linkxzhou/SimpleBase`，Go 1.25，单 go.mod）
> **日期**：2026-10-14
> **状态**：设计稿，待确认开放问题后实施
> **核对基线**：`main` @ `2896f82` + 2026-10-14 工作区未提交改动。实施前必须重新核对 `git status --short`（claude CLI 与 v6.0 系列计划均未提交）
> **依据**：[`internal/AGENTS.md`](../../internal/AGENTS.md)（API 协议约定）、[`internal/llmgateway/service.go`](../../internal/llmgateway/service.go)、[`plan/planv1.0/plan8.md`](../planv1.0/plan8.md)（LLM Gateway 原始设计）、[`pi-readme.md`](../../internal/gocloudagent/pi-readme.md) §4.6（压缩与协议）、[`gocloudagent-cli-and-cronjob-unification-plan.md`](./gocloudagent-cli-and-cronjob-unification-plan.md) §5（CLI 模型出口决策 D-D1）
> **约束**：遵循 `AGENTS.md` / `internal/AGENTS.md` / `ui/AGENTS.md`；**不新增第三方依赖**、不改 `go.mod` 结构；对象形状用 TS `interface`；错误统一走领域错误 + `WriteError`；本计划**不含任何 DDL**，无需用户执行迁移

---

## 1. 两个问题的核查结论

### 1.1 gocloudagent 是否用 function calling 协议

**分路径回答，结论不同**：

| 路径 | 是否 function calling | 证据 |
| --- | --- | --- |
| **云端 Runtime**（云助手） | **是**（`auto`：先原生，失败降级文本） | `runtime.go:198-199` 设 `chatModel.Protocol = r.ToolProtocol`；`r.ToolProtocol` 来自 `config.LLM.EffectiveAgentToolProtocol()`（`app.go:944`），默认 `auto`（`config.example.yaml:138`） |
| **CLI**（`cmd/gocloudagent`） | **否，恒为文本协议** | `cli/runner.go:86` 调 `agentcore.NewModel(...)` 后**从不设 `Protocol`**；`NewModel` 默认 `Protocol: "text"`（`agentcore/model.go:49`） |

云端之所以不受影响，是因为它**不走 HTTP**：`Runtime.LLM` 是 `ChatClient` 接口（`runtime.go:79`），由 `llmChatClient` 在**进程内**实现（`api/gocloudagent_access.go:200-243`），直接把 `ChatRequest.Tools` 转成 `LLMRequest.Tools`（同文件 `:218-220`）。

CLI 走 HTTP，因此受下面 §1.2 的 DTO 断点影响，只能退回文本协议。

### 1.2 llmgateway 是否支持 function calling

**完整支持，且从网关到供应商的链路已经通了**：

| 层 | 支持情况 | 证据 |
| --- | --- | --- |
| litellm v1.5.8 | 支持 `tools` / `tool_choice` | `providers/provider.go:82-83`；各驱动按需转换（`deepseek.go:229`、`gemini.go:217`、`bedrock.go:411`、`anthropic.go:208`） |
| llmgateway 请求 | 透传 | `Request.Tools []providers.Tool`（`service.go:55`）→ `prepareRequest`（`:278`） |
| llmgateway 响应 | 透传 | `Response.ToolCalls []providers.ToolCall`（`service.go:65`），`Chat` 回填（`:298`） |
| llmgateway 流式 | 透传增量 | `StreamChunk.ToolCallDelta *ToolCallDelta`（litellm `provider.go:131`），含 `Index`/`FunctionName`/`ArgumentsDelta` |
| api 适配层 | **已透传** | `toGatewayRequest` 把 `req.Tools` 转成 `providers.Tool`（`adapter.go:296-305`）；`llmStreamReaderAdapter` 把 `ToolCallDelta` 铺进 `LLMStreamChunk`（`adapter.go:341-346`）；`llmChatClient` 同样透传（`gocloudagent_access.go:218-220, 231-233`） |
| **api HTTP DTO** | **丢弃（唯一断点）** | 见下 |

**断点全在 `internal/api/llm_handler.go` 的 DTO，共四处**：

```28:35:internal/api/llm_handler.go
type llmChatRequest struct {
	Model       string          `json:"model,omitempty"`
	Provider    string          `json:"provider,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	Messages    []llmMessageDTO `json:"messages"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
}
```

| # | 断点 | 位置 | 后果 |
| --- | --- | --- | --- |
| B1 | 请求无 `tools` 字段 | `llm_handler.go:28-35` | 客户端声明了工具也无法传给上游 |
| B2 | 消息无 `tool_calls` / `tool_call_id` | `llm_handler.go:37-40` | 工具结果无法回灌（`LLMRequest.Messages` 有这两个字段，`router.go:137-138`） |
| B3 | 响应无 `tool_calls` | `llm_handler.go:42-48` | 非流式下工具调用被丢弃 |
| B4 | 流式块的工具字段是 `json:"-"` | `router.go:186-189`，注释明写「不出现在 `/llm/stream` 的 JSON 中」 | 流式下工具调用被丢弃 |

即：`llmChatRequest.Messages → LLMMessage` 的映射（`llm_handler.go:68-71`、`:110-113`）只搬运 `Role`/`Content`，其余字段在类型层面就不存在，与后端的完整能力**没有交集**。

**因此**：CLI 的文本协议不是设计选择，而是 HTTP DTO 能力缺失的**被迫回退**。这也解释了 2026-10-14 观察到的现象——`GLM-5.3-Flash` 在文本协议下格式遵从度不稳（漏写 `arguments`、话痨加编造 `TOOL_RESULT`），而云端同一模型走原生协议没有该问题。

### 1.3 现状端点与消费方

| 端点 | 权限 | 消费方 |
| --- | --- | --- |
| `POST /v1/projects/:projectID/llm/chat` | `DatabaseRead` | 前端 `ui/src/services/http-api.ts:1269`、CLI `cli/client.go:102`、`examples/community/server/main.go:152` |
| `POST /v1/projects/:projectID/llm/stream` | `DatabaseRead` | 前端 `http-api.ts:242`、CLI `cli/client.go:187` |
| `GET /v1/projects/:projectID/llm/providers` | `DatabaseRead` | CLI `cli/client.go:238`、`examples/lib/init/run.mjs:220`、`docs/workbench/dashboard.md:77` |
| `GET/PUT /v1/projects/:projectID/llm/settings` | `DatabaseRead` / `ProjectAdmin` | 前端 `http-api.ts:1277,1281`（**模型配置，与本次无关，不动**） |

路由挂载点：`internal/api/router.go:411-417`。

---

## 2. 目标

1. **对外 LLM 出口统一为 OpenAI Chat Completions 协议**，路径形如
   `http://127.0.0.1:8080/v1/projects/<projectID>/v1/chat/completions`，
   使官方 OpenAI SDK / 各类兼容客户端可直接指向 SimpleBase：

   ```python
   client = OpenAI(base_url="http://127.0.0.1:8080/v1/projects/faayefdq/v1",
                   api_key=simplebase_api_key)
   client.chat.completions.create(model="...", messages=[...], tools=[...])
   ```

   **双 `/v1` 是刻意的**：第一段 `/v1` 是 SimpleBase 自身的 API 版本前缀，第二段是 OpenAI SDK 约定的资源前缀。只有保留 `base_url` 尾部这个 `/v1`，SDK 才能原样拼接 `/chat/completions`。`plan8.md` 原始设计把兼容端点写作 `v1.POST("/llm/chat/completions", ...)`（仅单前缀），本计划调整为 `projects/:projectID/v1/...` 以同时满足**项目隔离**与**SDK 直连**。

2. **顺带补齐 function calling 的 HTTP 能力**（B1-B4），使 CLI 可以切到原生协议。这不是附带好处，而是本次的主要收益：文本协议下工具调用的可靠性问题无法靠解析容错根治。

3. **收敛熵**：一个端点、一套 DTO、一套错误形状；旧 `/llm/*` 三端点进入迁移期。

---

## 3. 服务端设计

### 3.1 路由

```go
// router.go：deps.LLM != nil 分支内新增（旧端点同处保留，见 §5）
oa := p.Group("/v1")   // → /v1/projects/:projectID/v1
oa.POST("/chat/completions", oh.ChatCompletions, require(auth.LLMInvoke))
oa.GET("/models", oh.ListModels, require(auth.LLMInvoke))
```

- 挂在既有 `p := v1.Group("/projects/:projectID", projectContextMiddlewareEcho(deps))`（`router.go:353`）之下，**自动获得**认证 + project context 注入，满足 `internal/AGENTS.md`「所有带 `:projectID` 的路由经过 `projectContextMiddlewareEcho`」。
- 无路由冲突：`/v1/projects/:projectID/v1/...` 与既有 `/llm/...`、`/settings`、`/agents` 等并列，不共享前缀。
- `web.Register(e)`（SPA fallback，`router.go:278`）在 `mountV1Routes` 之后调用，新路由先注册，不会被 SPA 吞掉。
- **权限用 `auth.LLMInvoke`**（`auth/principal.go:14`），而非旧端点的 `DatabaseRead`：这正是 `plan8.md` 的原始设计（`Require(auth.LLMInvoke)`）。三个内置角色都已持有该权限（`auth/role.go:53,56,58`），且 API Key 签发路径已支持（`api/api_keys_handler.go:223`），因此**不是权限收紧**，而是语义对齐。
- **不加 `writeGate`**：对话本身不改用户数据。`persistChat` 写的是系统库元数据（`sys_llm_sessions`），与既有 `/llm/chat` 行为一致。

### 3.2 请求 DTO

```go
type oaiChatRequest struct {
	Model       string            `json:"model"`
	Messages    []oaiMessage      `json:"messages"`
	Tools       []oaiTool         `json:"tools,omitempty"`
	ToolChoice  json.RawMessage   `json:"tool_choice,omitempty"`
	MaxTokens   *int              `json:"max_tokens,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
	StreamOpts  *oaiStreamOptions `json:"stream_options,omitempty"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiTool struct {
	Type     string `json:"type"`              // 仅接受 "function"
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON 字符串（OpenAI 就是字符串，不是对象）
	} `json:"function"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}
```

映射到既有的 `LLMRequest`（`router.go:123-153`）：`Tools → []LLMTool`、`Messages[].ToolCalls → []LLMToolCall`、`ToolCallID` 直传。**`LLMRequest` 已有全部字段，无需改动后端接口与 adapter。**

校验（返回 OpenAI 错误形状，见 §3.5）：
- `messages` 为空 → 400 `invalid_request_error`
- `tools[].type != "function"` → 400（不伪造不支持的 `code_interpreter` 等类型）
- 请求体大小受全局 `BodyLimit` 约束（`router.go:243-248`），不额外放宽

### 3.3 响应 DTO（非流式）

```go
type oaiChatResponse struct {
	ID      string      `json:"id"`      // "chatcmpl-" + uuid
	Object  string      `json:"object"`  // "chat.completion"
	Created int64       `json:"created"` // unix 秒
	Model   string      `json:"model"`
	Choices []oaiChoice `json:"choices"`
	Usage   oaiUsage    `json:"usage"`
}

type oaiChoice struct {
	Index        int        `json:"index"`
	Message      oaiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"` // stop | length | tool_calls
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// ReasoningTokens 是 SimpleBase 扩展，OpenAI 官方放在
	// completion_tokens_details.reasoning_tokens；两者都输出，后者兼容严格 SDK
	CompletionTokensDetails *oaiCompletionDetails `json:"completion_tokens_details,omitempty"`
}
```

`FinishReason` 直接透传上游值（litellm 原生协议下工具调用为 `tool_calls`）。`LLMResponse` 已有 `ToolCalls`（`router.go:162`），无需改动。

### 3.4 流式 SSE

标准 OpenAI 分块：

```
data: {"id":"chatcmpl-…","object":"chat.completion.chunk","created":…,"model":"…","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {…,"choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null}]}

data: {…,"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"shell","arguments":""}}]},"finish_reason":null}]}

data: {…,"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]},"finish_reason":null}]}

data: {…,"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {…,"choices":[],"usage":{"prompt_tokens":…,"completion_tokens":…,"total_tokens":…}}

data: [DONE]
```

要点：
1. **`[DONE]` 结束哨兵**替代现有的 `{"type":"end"}` 帧。
2. **工具增量按 `index` 聚合**：`LLMStreamChunk` 已有 `ToolCallIndex`/`ToolCallID`/`ToolCallName`/`ToolCallArgs`（`router.go:186-190`），只需在 SSE 编码器里把它们写成 `tool_calls[]`，**去掉 `json:"-"`**。
3. **首帧先发 `delta.role="assistant"`**：部分严格客户端依赖它建立消息。已有 `Type` 字段可判定。
4. **usage 尾帧**：`stream_options.include_usage=true` 时，在 `finish_reason` 之后、`[DONE]` 之前发一帧 `choices: []` + `usage`。SimpleBase 自身计费**不受该开关影响**——用量由 `llmgateway/streaming.go` 的 `usageRecordingReader` 独立记录。
5. **必须调用 `beginSSE(c)`**（`api/sse.go:22`）解除写超时，否则 SSE 会在 `write_timeout`（默认 30s）被切断。这是既有 `/llm/stream` 已踩过的坑（`sse_write_timeout_test.go`）。
6. **不在流中插入非标准帧**（如进度、心跳 JSON）。若需保活，用 SSE 注释行 `: ping`（`readSse` 与主流 SDK 都会忽略 `:` 开头行）。

### 3.5 错误协议

**OpenAI 兼容端点必须返回 OpenAI 错误形状**，否则 SDK 解析失败：

```json
{"error": {"message": "...", "type": "invalid_request_error", "param": null, "code": "model_not_found"}}
```

这是本计划**唯一**需要偏离 `internal/AGENTS.md`「错误统一走 `WriteError`」的地方，且偏离是刻意的、范围受限的：只在 `oa` 路由组内的 handler 使用一个 `writeOAIError(c, status, type, code, message)` 辅助函数；`type` 与 `code` 仍从 `api/error.go` 的既有映射派生（`error.go:114-125` 已有 `unauthenticated` / `invalid_api_key` / `forbidden` 等），不新造错误分类体系。

映射对照：

| 内部 code（`error.go`） | HTTP | OpenAI `type` |
| --- | --- | --- |
| `unauthenticated` / `invalid_api_key` / `api_key_revoked` | 401 | `authentication_error` |
| `forbidden` / `cross_project_denied` | 403 | `permission_error` |
| `invalid_request` | 400 | `invalid_request_error` |
| `not_found` | 404 | `invalid_request_error` |
| `rate_limited` | 429 | `rate_limit_error` |
| `llm_model_not_configured` / `llm_model_not_allowed` | 400 | `invalid_request_error` |
| 上游/超时/未知 | 5xx | `api_error` |

**约束不变**：错误 message 不得含密钥、SQL、LLM 正文。上游错误经既有 `mapEchoError` 脱敏后再包装。

### 3.6 落库与审计

保持 `persistChat` 现有行为（`llm_handler.go:184-214`）：写 `sys_llm_sessions` / `sys_llm_messages`，只存 Role/Content/用量，不存工具参数正文。审计沿用 `recordAudit`，`Kind` 改为 `llm_chat_completions` 以便与旧端点区分。

**注意**：`persistChat` 目前只落最后一条 user 与最终 assistant（`llm_handler.go:203-213`）。带工具回合的会话会出现「user → assistant(tool_calls) → tool → assistant」多条，本次**不改**该逻辑（保持最小改动），在开放问题 Q4 记录。

---

## 4. 客户端改造

### 4.1 CLI（`internal/gocloudagent/cli`）

| 改动 | 位置 | 说明 |
| --- | --- | --- |
| 路径切换 | `client.go:36-38` 的 `url()`、`:102`、`:187`、`:238` | `/llm/chat`→`/v1/chat/completions`；`/llm/stream`→同端点（`stream:true`）；`/llm/providers`→`/v1/models` |
| **wire 形状改 OpenAI 标准** | `client.go:41-66` | `wireTool` 要包成 `{"type":"function","function":{…}}`（当前是平铺的 `{name,description,parameters}`）；`wireToolCall` 同理包 `function`；`wireMessage` 的 `tool_calls`/`tool_call_id` 保持顶层（已正确） |
| 响应解析 | `client.go:68-79`、`:116-131` | 从 `choices[0].message` 取 `content`/`tool_calls`；`finish_reason` 从 `choices[0]` 取（不再是顶层） |
| 流式解析 | `client.go:134-171` 的 `httpTokenStream` | 解析 `choices[0].delta.content` 与 `choices[0].delta.tool_calls[]`；识别 `[DONE]` 结束；usage 尾帧 |
| **开启原生协议** | `cli/runner.go:86` | `agentcore.NewModel(...)` 后设 `model.Protocol = "auto"`，让网关优先原生、遇 400/422 自动降级文本（`agentcore/model.go:63-76` 已有降级与缓存） |
| 探活 | `client.go:237-252` 的 `Probe` | 改打 `/v1/models` |

`Protocol="auto"` 而非 `"native"`：`auto` 保留了对不支持 tools 的供应商（项目配置任意 base_url）的自动降级，是更稳的默认。降级后 CLI 仍能工作，只是回到文本协议。

**CLI 的 `--json` 事件契约不变**：`CLIEvent`（`cli/runner.go:23-32`）是 CLI 自己的展示层协议，与 HTTP wire 解耦，脚本无需改动。

### 4.2 前端（`ui/`）

| 改动 | 位置 | 说明 |
| --- | --- | --- |
| 端点与载荷 | `http-api.ts:242, 1269`、`toLlmPayload`(`:65-72`) | 改路径；`toLlmPayload` 已输出 `messages`/`max_tokens`/`temperature`，与 OpenAI 形状一致，无需改字段名 |
| SSE 解析 | `http-api.ts:191-225` 的 `readSse`、`:263-267` 的事件分发 | ① `readSse` 需识别 `[DONE]` 哨兵（当前只 `JSON.parse`，非 JSON 帧被静默忽略 → `onEnd` 永不触发）；② 事件分发已有 `obj?.choices?.[0]?.delta?.content` 兜底（`:265`），**天然兼容**，可删去 `obj.delta`/`obj.content` 旧分支 |
| 类型 | `ui/src/services/types.ts:394-423` | `LlmMessage` 加可选 `tool_calls`/`tool_call_id`；新增 `LlmTool`、`LlmToolCall`，对象形状用 **`interface`**（仓库硬规则）。**若不接工具，可只改解析不动类型**——前端当前只做「无工具对话」，建议本次前端**不引入工具调用 UI**，仅完成协议切换 |
| `/llm/providers` | `http-api.ts` 未见调用 | 前端未消费，无需改 |

前端**不引入工具调用 UI**：本次目标是协议统一，把工具能力交给 CLI 与云助手（它们已有各自的工具渲染）。避免范围蔓延与 `yarn test` 覆盖率压力（当前 functions 94.24% 未达标，见 `planv5.0 §7.2`）。

### 4.3 其他消费方

| 位置 | 改动 |
| --- | --- |
| `examples/community/server/main.go:144,152` | 路径切换到新端点 |
| `examples/lib/init/run.mjs:220` | `/llm/providers` → `/v1/models`，注意响应形状从 `{providers:[...]}` 变为 OpenAI list |
| `docs/workbench/dashboard.md:77` | 端点表更新，并新增 OpenAI 兼容调用示例（含 `base_url` 写法） |
| `internal/web/dist/**` | **禁止手工编辑**，由 `./build.sh` 重新生成 |

---

## 5. 旧端点处置

**分两个阶段，不一次性删除**：

| 阶段 | `/llm/chat`、`/llm/stream`、`/llm/providers` | 理由 |
| --- | --- | --- |
| **P1（本次）** | 保留，行为不变，路由上标注 `Deprecated` 注释 | 消费方（前端/CLI/examples）切换需与后端同批验证；直接删会同时打断三条链路，出问题时无法二分定位 |
| **P2（后续版本）** | 删除，或改为 301/410 | 待 P1 全链路验收通过、且确认无外部集成后执行 |

**不建议**把旧端点改成「内部转发到新端点」：旧响应形状（`{content,model,provider}`）与新形状（`choices[]`）不同，转发必然要写形状转换，等于维护两套 DTO——比保留两份 handler 更糟。P1 期间两套并存、各自独立。

---

## 6. 实施顺序

严格串行，每步可独立编译与测试：

| 步 | 内容 | 验证 |
| --- | --- | --- |
| **S1** | 服务端：`oai_handler.go`（新 DTO + `ChatCompletions` + `ListModels` + `writeOAIError`），路由挂载，`LLMInvoke` 权限 | `go build`；新增 `oai_handler_test.go` 覆盖非流式/流式/工具/错误形状 |
| **S2** | 服务端：流式工具增量与 `[DONE]`、usage 尾帧 | SSE 单测断言帧序列；`beginSSE` 有 `sse_write_timeout_test.go` 风格用例 |
| **S3** | CLI：wire 形状改 OpenAI 标准 + 路径切换 + `Protocol="auto"` | `go test ./internal/gocloudagent/...`；**端到端**：真实模型跑「用 shell 执行 pwd」，断言出现 `tool_call` 且工具真实执行 |
| **S4** | 前端：路径 + `readSse` 哨兵识别 + 旧分支清理 | `cd ui && yarn test && yarn build`；覆盖率不低于基线 |
| **S5** | examples / docs 同步 | 示例可跑；文档端点表更新 |
| **S6** | 全量回归 + 手动验收（§9） | `go test ./...`；`./scripts/smoke.sh` |

**S1 与 S3 之间必须人工验证一次**（curl 或 OpenAI SDK 直连），确认新端点在对端可解析，再动 CLI。否则 CLI 出问题时会难以区分是服务端还是客户端。

---

## 7. 硬性约束

1. **不新增依赖、不改 `go.mod`**。OpenAI DTO 全部用标准库 `encoding/json` 手写，不引入 SDK 类型包。
2. **项目隔离不放松**：新路由必须挂 `projectContextMiddlewareEcho`，handler 内用 `ProjectFromContext`，不得从 body 推断 project。
3. **密钥/正文不入日志与审计**：沿用 `recordAudit` 的元数据口径；错误 message 经脱敏（`internal/AGENTS.md` 的 trace 例外边界不变，`agent.trace_log` 仍是唯一正文出口且只写 zap）。
4. **系统库保护不变**：`persistChat` 经 `systemdb.Store` 写元数据，不经 registry 的用户库 factory。
5. **`WriteError` 的唯一例外是 OpenAI 错误形状**（§3.5），且限定在 `oa` 路由组内；其余 handler 不动。
6. **`internal/AGENTS.md` 与 `docs/` 需同步**：新增「OpenAI 兼容出口」段落，说明双 `/v1` 的由来与错误形状例外。
7. **不新增迁移、不执行 DDL**。本计划不触碰任何表结构。
8. **`internal/web/dist` 不手工编辑**。

---

## 8. 测试要求

**后端**：

- `oai_handler_test.go`：
  - 非流式：`content` / `tool_calls` / `finish_reason` 三种响应形态
  - 请求侧：`tools` 与 `messages[].tool_calls` / `tool_call_id` 是否正确进入 `LLMRequest`（用 fake service 断言入参，仿 `llm_handler_test.go` 的 `fakeLLMSvc` 风格）
  - 错误形状：401/403/400/429 各自返回的 `error.type` 与 `error.code`
  - `messages` 为空、`tools[].type` 非 function 的 400
- 流式：帧序列断言（首帧 `delta.role`、内容帧、工具增量按 index 聚合、`finish_reason`、usage 帧、`[DONE]`）；`beginSSE` 解除超时
- `router` 层：新路由需要认证（无凭证 401）、需要 project context
- **跨层一致性**：`LLMService` 接口新增/修改时，`internal/api/*_test.go` 的 `fakeGW`、`fakeLLMSvc` 必须同步补齐方法（`AGENTS.md` 明确要求）。本次**不新增接口方法**，风险有限，但仍需确认

**前端**：`yarn test`（覆盖率 statements/branches/functions 均 95% 阈值）+ `yarn build`。新增解析分支必须补测，不得沿用当前 94.24% 的未达标基线。

**CLI**：`go test ./internal/gocloudagent/...`；wire 形状用 `httptest.Server` 断言请求体是标准 OpenAI 嵌套，响应解析覆盖 `choices[0]` 与 `[DONE]`。

---

## 9. 验收

1. `curl` 直连新端点：非流式返回 `object: "chat.completion"`、`choices[0].message`；流式以 `data: [DONE]` 收尾。
2. **官方 OpenAI SDK 直连**（Python 或 Node）：`base_url=http://127.0.0.1:8080/v1/projects/<id>/v1`，一次普通对话 + 一次带 `tools` 的调用，`tool_calls` 能正确返回。
3. **CLI 端到端**：`./gocloudagent -p "用 shell 执行 pwd"` 出现 `tool_call` 事件、工具真实执行、结果回灌、模型据此作答（对照 2026-10-14 文本协议下的一轮即止）。
4. **CLI 带工具的多轮**：连续两次工具调用后给出最终答案。
5. **降级路径仍可用**：把一个不支持 tools 的供应商配为项目默认，`Protocol="auto"` 时自动降级文本协议，对话不中断。
6. **前端**：控制台 AI 对话流式输出正常、`onEnd` 正常触发（不被 `[DONE]` 卡住）、`yarn test`/`yarn build` 通过。
7. `go test ./...`、`./scripts/smoke.sh` 通过；`gofmt` / `go vet` 干净。
8. 未改 `go.mod`、未新增依赖、未手工编辑 `internal/web/dist`。

---

## 10. 风险

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 严格 SDK 对额外帧报错 | 客户端解析失败 | 流中只发标准帧；保活用 `: ping` 注释行，不发明 JSON 帧 |
| `finish_reason` 取值差异 | 客户端判断工具调用失败 | litellm 原生协议返回 `tool_calls`；在 handler 做一次归一（`stop`/`length`/`tool_calls`），其余值透传 |
| 双 `/v1` 路径被误认为笔误而"修正" | SDK 无法直连 | 在 `internal/AGENTS.md` 与代码注释写明由来（§2.1），并加路由测试锁定路径 |
| CLI 切原生后上游不支持 tools | 对话中断 | `Protocol="auto"` + `agentcore` 既有降级缓存（`model.go:63-76`）；§9.5 专项验收 |
| 前端 `readSse` 漏改哨兵 | 流式 UI 不结束（永远转圈） | S4 必须补 `[DONE]` 识别测试；先改解析再接路径 |
| 新旧端点并存期间的语义漂移 | 两端行为不一致 | P1 期间旧端点冻结行为（只保留，不加功能）；新能力只进新端点 |

---

## 11. 开放问题

| # | 问题 | 选项 | 本计划倾向 |
| --- | --- | --- | --- |
| **Q1** | 旧端点何时删除？ | A 下一版本删／B 保留两个版本／C 永久保留 | **B**：给外部集成留迁移窗口；P1 完成后单独决策 |
| **Q2** | 是否支持 `tool_choice`？ | A 透传到上游（litellm 已支持）／B 不暴露 | **A**：`providers.Request.ToolChoice` 已存在（litellm `provider.go:83`），透传零成本；但需确认 `llmgateway.Request` 是否要加字段（当前**没有**），若加则属接口扩展 |
| **Q3** | 是否暴露 `/v1/responses`（OpenAI 新版 API）？ | A 不做／B 返回 501 | **B**：`plan8.md` 已定「返回 501 `feature_not_implemented`，不要伪造 OpenAI 完整兼容」。**本次不做**，仅记录 |
| **Q4** | `persistChat` 只落最后一条 user + 最终 assistant，带工具回合的中间消息不落库 | A 保持／B 全量落库 | **A（本次保持）**：改动会牵动 `sys_llm_messages` 的数据语义与前端会话展示，超出协议统一范围 |
| **Q5** | `/v1/models` 返回哪些模型？ | A `ProviderModels` 聚合（全部 provider 的模型）／B 仅默认 provider 的模型 | **A**：`LLMService.ProviderModels` 已返回 `map[provider][]model`（`router.go:120`），展平成 OpenAI list 即可，无需新接口 |
| **Q6** | 记忆：这次发现的「DTO 是唯一断点」是否回写到 `plan8.md`？ | A 回写／B 不回写（plan8 是历史文档） | **B**：在本文与 `internal/AGENTS.md` 记录，不改历史 plan |

---

## 12. 与既有计划的关系

- [`plan8.md`](../planv1.0/plan8.md)：LLM Gateway 的原始设计**本就要求** `chat/completions` 与 `LLMInvoke` 权限。本计划是把它补完，不是新方向；唯一偏离是把兼容路径放到 `projects/:projectID/v1/...` 之下（原设计只写了 `v1.POST("/llm/chat/completions")`，未含 project 段）。
- [`gocloudagent-cli-and-cronjob-unification-plan.md`](./gocloudagent-cli-and-cronjob-unification-plan.md) §5 / 决策 D-D1：CLI「走服务端 HTTP API、不自管供应商密钥」的决策不变。本次只是让它**用得对**——文本协议是当时 DTO 不支持工具造成的意外回退，不是 D-D1 的本意。
- [`gocloudagent-cli-and-cronjob-unification-plan.md`](./gocloudagent-cli-and-cronjob-unification-plan.md) §13.5 / P7 压缩：**不冲突**。压缩作用于会话历史与摘要，与工具协议正交；`agentcore.Model.Protocol` 变化不影响 `compaction.go` 的 `summarize` 调用（它不传 tools）。
- [`backend-code-simplification-plan.md`](./backend-code-simplification-plan.md)：本文新增的 `oai_handler.go` 应遵守其分层约定（handler 只依赖 `Dependencies` 接口 + adapter），不直接依赖 `llmgateway`。
