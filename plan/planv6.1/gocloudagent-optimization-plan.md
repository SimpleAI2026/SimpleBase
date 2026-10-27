# CloudAgent 运行时优化计划（v6.1）

> 状态：**待评审**。本文只定计划，不改 `cmd/`、`internal/`、`go.mod`、`package.json`。
> 日期：2026-10-15。
> 来源：[`gocloudagent-smoke-report.md`](gocloudagent-smoke-report.md) 的冒烟 / Terminal-Bench 8 / HumanEval 164 三段实测。
> 上游计划：[`gocloudagent-agent-benchmark-plan.md`](gocloudagent-agent-benchmark-plan.md)（本文件是它的对照补丁，不替代它）。
> 基线：`5fae7d4`（冒烟）/ `46d00a7`（核心两层）。模型 `zai-org/GLM-5.3-Flash`。
> 原则：先修让评测结论不可信的项（可观测性、重试、超时），再修直接影响拿分的行为，最后动评测方法学。**不新增第三方依赖、不改 `go.mod`**——本文所有改动点用到的能力都已在现有依赖里（litellm v1.5.8、Eino v0.9.19）。

---

## 0. 结论

冒烟的 14/14 只证明管线通了，不证明能力：门槛是 HumanEval ≥1/10，实跑 10/10，且取的是最易的前 10 题。具备信息量的是后两段数据，但它们同时被非能力因素污染：

- HumanEval 15 个失败里 **9 个是超时**，且 120 秒边界上出现互相矛盾的判定（`/145` 通过 120042ms 与 `/133` 失败 120012ms）。
- Terminal-Bench 8 题 **0/8**，其中 4 题栽在镜像 glibc、1 题栽在网关超时，真正的能力失败不足一半。

所以这份计划的排序是：**先让结论可归因、成本可结算，再谈分数。**

| 组 | 主题 | 项 | 动产品代码 |
|---|---|---|---|
| P0 | 让结论与护栏成立 | #1 usage 出口、#2 重试、#3 provider 超时真透传、#4 `finish_reason=length` | 是 |
| P1 | 直接影响拿分与成本 | #5 `max_tokens` 隐式 4096、#6 墙钟预算、#7 重复动作检测、#8 停止判据 | 是 |
| P2 | 评测方法学（只在 `eval/`） | #9 分层抽样、#10 L3 钻头用例、#11 失败分类、#12 步数口径、#13 缺值不静默 | 否 |

**如果只做三件事**：`#1`（数据现成，一次改动解锁全部成本核算与「费用未知 20 题即停」护栏）、`#2+#3`（重试与超时是当前失败率的最大单一来源）、`#4+#5`（一条被静默放大的截断链，见 §3.1）。

---

## 1. 现状核实

本文的每条落点都在代码里核对过。核对过程中修正了报告与既有文档的三处口径，先列出来，避免实施时按错的前提改。

### 1.1 修正一：`litellm.ProviderConfig` **有** `Timeout` 字段，但它从未被读取

`litellm@v1.5.8` 的 `providers.ProviderConfig`（`providers/base.go:15-22`）有四个字段：

```go
type ProviderConfig struct {
    APIKey     string
    BaseURL    string
    Timeout    time.Duration     // ← 全库无任何读取点
    Extra      map[string]any
    Resilience ResilienceConfig  // ← 真正生效的是这个
    HTTPClient HTTPDoer
}
```

全库搜 `.Timeout` 只命中 `netErr.Timeout()`；`Timeout:` 的写入点（`base.go:74,77`）用的是 `resilienceConfig.RequestTimeout` 与 `.ConnectTimeout`，不是 `config.Timeout`。**`Timeout` 是个死字段。**

这修正了两种说法：不是「llmgateway 把超时丢了」（它确实没传，但传了也没用），而是**要用 `Resilience` 才能配超时**。

### 1.2 修正二：重试能力已经在依赖里，缺的只是配置

`litellm` 自带 `ResilientHTTPClient`（`resilience.go:33-48`），`createProvider`（`registry.go:33-39`）在 `config.HTTPClient == nil` 时**自动注入**它。重试条件已经写好：

| 判据 | 位置 | 覆盖 |
|---|---|---|
| `isRetryableStatusCode` | `resilience.go:183-194` | 429 / 500 / 502 / 503 / 504 |
| `isRetryableError` | `resilience.go:150-166` | 网络错误、`net.Error.Timeout()`；**显式排除** `context.Canceled` / `DeadlineExceeded` |
| 退避 | `resilience.go:139-148` | `InitialDelay × Multiplier^attempt`，上限 `MaxDelay`，带 ±25% jitter |

唯一的缺口是 `DefaultResilienceConfig()`（`providers/base.go:36-46`）里 `MaxRetries: 0`。所以 `#2` 的修法**不是**「接 Eino 的 `ModelRetryConfig`」为止——那是第二层。第一层只需要给 `litellm.NewWithProvider` 传一个非零 `MaxRetries`。

### 1.3 修正三：agent 循环「不设 max_tokens」的实际后果是每请求 4096

`agentcore` 只在调用方显式传 `options.MaxTokens` 时才写进请求（`agentcore/model.go:283-285`），agent 循环从不传。但这不等于「没有上限」：

`litellm.Client.applyDefaults`（`client.go:296-308`）在 `req.MaxTokens == nil` 时**填入 4096**，`Chat` 与 `Stream` 两条路径都会调用它（`client.go:157`、`client.go:186`）。

链路是完整的：CLI 的 `wireRequest.MaxTokens` 带 `omitempty`（`cli/client.go:74`）→ 字段被省略 → `oaiChatRequest.MaxTokens = nil`（`api/oai_dto.go:51`）→ `oaiToLLMRequest` 原样透传（`api/oai_dto.go:111`）→ `prepareRequest` 透传（`llmgateway/service.go:279`）→ `applyDefaults` 填 4096。

**结论：每一次模型调用都带着 `max_tokens: 4096`，且没有任何一处日志或事件体现它。** 这是本文最有价值的发现，也是 §3.1 那条链的起点。

### 1.4 现存的默认值分叉

| 常量 | CLI | 云端 | 配置层 | 评测脚本 |
|---|---|---|---|---|
| 轮数上限 | 12（`cli/runner.go:99-102`） | 8（`runtime.go:207-210`） | 8（1–20 校验，`config/config.go:167`） | 40（`run_smoke.py:42`） |
| 总时长 | **无** | 180s（`runtime.go:225-229`） | 10s–15m（`config/config.go:168`） | 自定义 180s / HumanEval 120s（`run_smoke.py:40-41`） |
| 单步 shell | 120s（`tools/tools.go:23`） | 同 | — | — |

CLI 那一列有两个「无」：无总时长、无单题超时。这两条是 `#6`。

---

## 2. P0：让结论与护栏成立

### 2.1 #1 `end` 事件补 usage 与 model

**为什么最高优先级**：上游计划 §6.1 定过一条安全阀——「未知费用达 20 题就停核心」。因为 token 全程为空，**这条护栏从头到尾没有生效过**，172 题一路跑完。下一层（SWE-bench Lite 20 题 + Terminal-Bench 8 题）叠加时，这是真金白银的风险，且报告里连「跑的是不是 GLM-5.3-Flash」都无法自证（非密钥模型名与 Base URL 都是空，脚本回退了默认值）。

**数据已在手上**，断点只在序列化：

| 位置 | 现状 |
|---|---|
| `cli/runner.go:55-62` `RunResult` | 已有 `PromptTokens` / `CompletionTokens` / `ToolCalls` / `DurationMS` |
| `cli/runner.go:93-98` | usage 已累加（含 `ReasoningTokens`） |
| `cli/runner.go:212-216` | 已写进 `RunResult` |
| `cli/repl.go:115-119` | 写 `end` 事件时**只发** `Content` 和 `Message` |
| `cli/runner.go:23-32` `CLIEvent` | 无 usage、无 duration、无 model |
| `cli/session.go:36-64` `Entry` | 无 usage 字段，`resume` 后无法累计 |

云端早已有对照实现（`api/agent_handler.go:925-930` 写 `usage` 事件后再写 `end`），本地 CLI 对齐即可。

**改动**

1. `cli/runner.go` 的 `CLIEvent` 增加可选字段，与上游计划 §6.2 的形状一致（省略时旧解析器仍能工作）：

   ```go
   type CLIEvent struct {
       // ... 既有字段不变
       PromptTokens     int    `json:"prompt_tokens,omitempty"`
       CompletionTokens int    `json:"completion_tokens,omitempty"`
       ReasoningTokens  int    `json:"reasoning_tokens,omitempty"`
       ToolCalls        int    `json:"tool_calls,omitempty"`
       DurationMS       int64  `json:"duration_ms,omitempty"`
       Model            string `json:"model,omitempty"`
   }
   ```

2. `cli/repl.go:116` 的 `end` 事件带上上述值。**注意**：`end` 目前只在 headless 成功路径写一次；`run_failed` 与 `canceled` 走 `fail()`（`cli/repl.go:303-317`），拿不到 `RunResult`。要么让失败路径也带上已累计的 usage（`Run` 在 `runErr != nil` 时仍然返回 `RunResult`，见 `cli/runner.go:226-229`），要么明确记录「失败轮次的 token 不可知」。**建议前者**——失败轮次恰恰是最需要归因的。
3. `model` 字段的来源：`agentcore.ChatResponse.Model` 已经在 `cli/client.go:177-180` 被解析，但没有向上传递到 `RunResult`。需要在 `agentcore` 侧新增一个 model 回调（与 `Usage` 回调同形的 `Model func(string)`），或在 adapter 里缓存首个非空 model。
4. `cli/session.go` 的 `Entry` 增加可选 usage 字段，让 `resume` 能累计。**保持 `SessionVersion = 1`**——新增 `omitempty` 字段不改格式，符合该文件既有约定（`cli/session.go:17-18`）。

**测试**：`cli/client_test.go` 已有假 HTTP 的形状（见 `client_test.go:137-143`）。新增用例：假 HTTP 返回带 usage 的流，断言 `--json` 的最后一行 `end` 含 `prompt_tokens` / `completion_tokens` / `duration_ms` 数字。**不要在日志里打印 `content`。**

**与上游计划的关系**：这就是 `gocloudagent-agent-benchmark-plan.md` 的 M1。本文把它从「M1 待办」提为 P0 首项，因为护栏在失效。

### 2.2 #2 网络级重试

**证据**：HumanEval/109 是一次 HTTP 502，耗时 4.3 秒、0 步，整题直接记为失败。全仓 `retry|backoff` 在 `internal/llmgateway` 与 `internal/gocloudagent` 下零命中。

**改动（两层，缺一不可）**

| 层 | 落点 | 覆盖场景 |
|---|---|---|
| HTTP 级 | `llmgateway/service.go:208-211`，给 `litellm.ProviderConfig` 传 `Resilience` | 首字节之前的 429/5xx/连接超时。**零代码侵入**——`ResilientHTTPClient` 已经在 `createProvider` 里注入 |
| 模型调用级 | `runtime.go:211-220` 与 `cli/runner.go:107-117` 的 `adk.ChatModelAgentConfig` 填 `ModelRetryConfig` | 首字节之后的流中途失败。Eino 的 `ModelRetryConfig`（`eino-readme.md:1949-1966`）已支持 `MaxRetries` / `IsRetryAble` / `BackoffFunc` |

**为什么两层都要**：litellm 的重试点在 `resp, err := c.client.Do(req)`（`resilience.go:79`）——响应头一旦返回 200，后续 body 中途断开不在它的覆盖范围。而 Eino 的 `eino-readme.md:1965` 明确写了「流式重试比同步重试更严：已经向下游发出的 chunk 不能无声地重来，适配器应在首个有效 chunk 之前失败」。两层合起来才闭合。

**边界**：`isRetryableError` 已排除 `context.Canceled` / `DeadlineExceeded`（`resilience.go:157-159`），所以用户 Ctrl+C 不会触发重试风暴。Eino 侧的实现必须保持同样语义，并**必须可关闭**（评测与 `--yolo` 之外的使用要能退回原行为）。

**测试**：`internal/llmgateway` 用假上游返回 502 两次再成功，断言只调用一次上游客户端逻辑但重试了；`-race` 下确认重试不与 client 缓存竞争（`s.clients` 有锁，见 `service.go:197-202`）。

### 2.3 #3 provider 超时真透传

**证据**：报告第 45 行强调「具名 provider `openai`，`SIMPLEBASE_LLM_PROVIDER_OPENAI_TIMEOUT=180s`。简写 60 秒超时没有使用」——**这个值极可能根本没生效**。

链路断在最后一跳：

```
config.ProviderConfig.Timeout        config/config.go:264
  → app.instanceLLMProviders         app/app.go:1031-1046   （Timeout 被正确搬运）
  → llmgateway.InstanceProvider      fallback_resolver.go:15（字段存在）
  → llmgateway.ProviderConfig        service.go:32-39       ← 无 Timeout 字段，值在此丢弃
  → litellm.NewWithProvider          service.go:208-211     （只传 APIKey + BaseURL）
```

实际生效的时限只有外层 ctx（`runtime.go:225-229` 的 `RunTimeout`，默认 180s）与 litellm 的 `Resilience.RequestTimeout` 默认值 **5 分钟**（`providers/base.go:43`）。

**改动**：`llmgateway.ProviderConfig` 增加 `Timeout time.Duration`；`fallback_resolver.go:60-66` 与 catalog 侧解析路径一并搬运；在 `service.go:208-211` 转成 `Resilience.RequestTimeout`（**不是** `ProviderConfig.Timeout`，见 §1.1）。同时设 `ConnectTimeout`（默认 10s 对跨境供应商偏大，建议 5s）。

**顺带解决 `roy-polymorph-cn` 的归因**：报告第 59 行记「模型请求等响应头超时，日志是空的」。这是**首字节超时**，而当前没有任何指标能区分它和「整体超时」。建议同时补一条 warn 日志：请求发出与响应头到达的时间差。这不违反脱敏约束（只记时长，不记内容）。

**测试**：`internal/config` 已有 `TestLoadLLMProvidersFlatEnvTimeout`（`config_more_test.go:190-205`）覆盖了配置层解析。需要新增的是 `internal/llmgateway` 侧的用例：断言 `InstanceProvider.Timeout` 经解析后确实出现在传给 litellm 的 `Resilience.RequestTimeout` 上。**当前没有任何测试覆盖这个断点，所以它才静默了这么久。**

### 2.4 #4 `finish_reason=length` 不执行残缺工具调用

**证据 A**：`planv6.0` 已把这条记为真实缺陷（`plan/planv6.0/gocloudagent-cli-and-cronjob-unification-plan.md:1272-1274`），P5 的状态是「采用」，但我在 `internal/gocloudagent` 全仓搜 `"length"` 与 `truncated` 的判据，**没有任何实现命中**——仍在待办。

> 「当前 `gocloudagent` 对模型输出被截断（`FinishReason == "length"`）**没有任何处理**——如果被截断的消息里带了残缺的 `ToolCalls`，ADK 会照样执行它们。这是**真实缺陷**（参数不完整 → 工具执行错误或危险操作）。」

**证据 B**：`agentcore/model.go` 从不把上游 `FinishReason` 放进 `schema.Message.ResponseMeta`（全文搜 `ResponseMeta` 在 `internal/gocloudagent` 下零命中）。所以即使想在事件循环里判也拿不到值。

这与 `risk-scorer-replay` 的「**工具参数 JSON 被截断**」（报告第 77 行）是同一个现象的两个可能解释，而 §1.3 让截断解释变得非常可疑：每个请求的输出上限被静默钉在 4096 token，一旦 `write_file` 的 `arguments` 里要装一个较长文件，模型必然写不完，参数 JSON 就断在半路——而残缺的调用会被照常执行。

**改动**

1. `agentcore/model.go` 在三条路径（非流式 / 流退化 / 流增量）把 `FinishReason` 写入 `schema.Message.ResponseMeta`。`assignToolMessage` / `parseAssistant` 一带要把 `ToolCalls` 与 `FinishReason` 一起放进去（`planv6.0:1274` 已给出落点）。
2. `cli/runner.go` 与 `runtime.go` 的事件循环里，收到 `FinishReason == "length"` 时**对每个 `ToolCall` 直接发 `tool_result` 错误、不执行**，错误码 `truncated`，并给整轮 `Reason = "length"`。

**为什么 P0**：这是**唯一一条「参数不完整但照样执行」的路径**——`shell` 工具就在这条路径上。`#5`（放大输出上限）降低触发概率，`#4`（不执行残缺调用）限制触发后的后果。两者必须一起做：只做 `#5`，迟早还会有别的截断来源。

---

## 3. P1：直接影响拿分与成本

### 3.1 #5 显式设定 `max_tokens`，不要让 4096 隐性兜底

**问题**：见 §1.3。当前的 4096 有三个坏性质——不在任何配置里、不出现在任何事件或日志里、对 agent 场景偏小（尤其 `write_file` 整文件写入时）。

**疑似后果**：`layout-config-recreation` 918 秒、43 步、verifier 11 failed（报告第 73 行）。一个合理的解释是「模型反复尝试写完一个写不完的输出」。这**是假设，不是结论**——验证方法写在下面。

**改动**

1. 在 `agentcore` 侧引入显式的输出上限，来源优先级：`GOCLOUDAGENT_MAX_OUTPUT_TOKENS` 环境变量 > `Config.MaxOutputTokens`（`cli/config.go:26-49`）> 内置默认。默认值不宜拍脑袋，建议先取一个明显高于 4096 的值（例如 8192），并**在 `end` 事件里带上它**，让每次评测都自证。
2. 云端 `runtime.go` 侧走 `config.LLM` 的对应字段，与 CLI 分离——两者的默认值本来就不该相同（见 §1.4）。
3. **不要依赖 litellm 的 `applyDefaults`**。显式传值，否则无法区分「我们设的」与「库兜底的」。

**验证方法**（实施第一步就做，先证实再改）：在 `internal/testutil/fakellm` 或一次真实调用上打印收到的 `max_tokens` 与返回的 `finish_reason`，统计 `length` 出现次数。**如果 `length` 计数为 0，这条假设被推翻**，`layout-config-recreation` 要另找解释，而 `#4` 仍然独立成立（残缺调用不该执行）。

**测试**：`agentcore` 的单测断言请求体里的 `max_tokens` 等于配置值；`cli` 侧断言 `end` 事件带 `max_tokens`。

### 3.2 #6 CLI 总时长预算

**证据**：`cli/repl.go:106-107` 用 `signal.NotifyContext(context.Background(), ...)`，**没有任何 `WithTimeout`**。上游计划 §1.2 也记着「CLI **没有** `--timeout`，harness 用进程超时杀掉」。`918 秒` 那次能跑完，只是因为 harness 的进程级超时还没到（Terminal-Bench 用 `task.toml` 时限 +60s，封顶 20 分钟）。

**改动**：`cli/config.go` 增加 `MaxRunDuration`（环境变量 `GOCLOUDAGENT_MAX_RUN_DURATION`）；`cli/repl.go:106` 把它包进 ctx；超时时 `Reason = "timeout"`，与云端 `runtime.go:364-368` 的 `DeadlineExceeded` 映射保持同名。同时在 `end` 事件里带上 `duration_ms`（`#1` 已给字段）。

**注意**：REPL 模式（`cli/repl.go:223`）是每条消息一个 ctx，预算应按**单轮**而非整个会话计；`/compact` 的一次性调用（`cli/repl.go:202-204`）不占用这个预算。

### 3.3 #7 重复动作检测

**证据**：全仓无 `stuck|repeat|no.?progress` 实现。提示词里只有建议——`cli/runner.go:290`：

> 「If a tool returns an error envelope, read the code field and adapt instead of repeating the same call.」

这是**建议**，没有运行时强制。模型换着法子重试同一个失败命令时，唯一的刹车是 `MaxIterations`（`cli/runner.go:217-229`）。

**改动**：在 `cli/runner.go` 的事件循环里对 `(tool_name, sha256(arguments))` 做滑动窗口计数。同一动作在窗口内重复 N 次（建议 N=3）时，注入一条系统提示要求换策略；达到 M 次（建议 M=5）时终止本轮，`Reason = "loop_detected"`。**必须在 `end` 事件里给出这个 reason**，否则评测侧又会把它记成 `max_iterations`。

**顺带**：`Reason` 的取值集合要从 `stop / max_iterations / canceled`（`cli/runner.go:217-229`）扩展为并集，并在报告里单列「因重复动作终止」的题——这类题**不应计入能力失败**，它们是运行时护栏起作用的证据。

### 3.4 #8 停止判据：两侧都要校准

同一批题里出现两个极端，指向同一个缺失——「什么时候算做完」没有定义：

| 题 | 表现 | 判断 |
|---|---|---|
| `photonic-waveguide-routing` | 3 步，verifier 12 passed / 2 errors | 差一步收尾就宣告完成 |
| `layout-config-recreation` | 43 步，918 秒，verifier 11 failed | 该停时不停 |

**改动**：默认指令（`cli/runner.go:283-291`）增加一条**可执行的**完成判据——「宣告完成前，必须运行题目给出的验证命令并查看输出」。并在 `cli/runner.go` 收尾处加一个弱检查：若整轮**没有执行过任何验证类命令**（可按题目 harness 传入的 `verify_command` 判定，或退化为「没有执行过非只读命令」）就宣告 `stop`，回推一轮要求自查。

**边界**：这条只对自定义题与 Terminal-Bench 生效，HumanEval 无验证命令可用，不能一刀切。实现上建议由 harness 通过环境变量显式告知，而不是让 CLI 猜。

---

## 4. P2：评测方法学（只动 `eval/`，不动产品代码）

### 4.1 #9 冒烟抽样改分层

当前取前 10 题，恰好是最易的一批：观察耗时 6–19 秒，而全量中位约 20 秒、长尾顶到 120 秒。改成固定种子 + 按耗时或难度分层抽 10 题，冒烟才有预警价值。**不引入新数据集**，只是抽样策略变化。

### 4.2 #10 L3 补确定性钻头用例

这是本次报告最需要补的一条：L3 耗时 9722ms、**步数 0**——模型压根没调 shell。哨兵仍在，隔离自检也确认哨兵目录只读，但**「命令真的发出去、被挂载挡住」那一步从未被打到**（报告第 40 行）。

**改动**：加一个不开模型的钻头用例——直接构造 `shell` 工具调用 `rm -rf /`，走与 agent 相同的入口，断言三件事：

1. 前置拦截（`IsDangerousShell`）是否命中；
2. 只读挂载是否挡住（`run_smoke.py:160-178` 的 jail 脚本）；
3. 哨兵是否仍在（`verify_l3`，`run_smoke.py:334-341`）。

**这里有一个必须写清的结构性问题**：`--yolo` 下危险命令拦截**完全失效**——`tools/tools.go:169-171` 的 `approve` 在 `Yolo` 为真时无条件 `return true`，而 `tools.go:130-134` 照样调用 `IsDangerousShell` 却拿不到任何结果。评测里 `--yolo` 是**唯一被覆盖的路径**（`run_smoke.py:274-275`），所以那 10 条正则（`tools/tools.go:85-96`）在评测中等于零。

**建议的定位**（需要评审拍板，不要默默改）：

| 方案 | 说明 |
|---|---|
| A. 保持现状，把正则降级为「交互模式的便利功能」 | 文档与 `--yolo` 的帮助文案里明确写「yolo 关闭全部危险命令确认，安全边界只由容器/挂载提供」 |
| B. 给 yolo 分档 | 例如 `--yolo` 仍拦「绝对危险」子集（`rm -rf /`、`dd if=`、`mkfs`），只跳过其余确认 |

**无论选哪个，都要顺带补正则覆盖度**——`rm -r -f`、`bash -c "rm -rf"`、`find . -delete`、变量拼接当前都不在表内。

### 4.3 #11 失败分类拆开

Terminal-Bench 现在 8 题全 0 混成一坨，实际含三类，而它们指向完全不同的动作：

| 类别 | 本次实例 |
|---|---|
| 环境 | glibc 4 题、iptables FORWARD、`roy-polymorph-cn` 网关超时 |
| 协议 | `risk-scorer-replay` 参数截断 |
| 能力 | 各题 verifier 不通过 |

**改动**：在 `record()`（`run_smoke.py:601`）与 harbor 的填充逻辑里增加 `failure_class` 列，取值与上游计划 §6.3 的 `error_code` 对齐。**不加这一列，下一轮不知道优化什么。**

### 4.4 #12 步数口径统一

报告里的「步数」是**去重后的 `tool_call` 事件数**（`run_smoke.py:118-130`、`harbor_agent.py:153-159`），而 `GOCLOUDAGENT_MAX_ITERATIONS=40` 数的是**模型轮数**（一轮可发多个工具调用，`runtime.go:319-341`）。所以出现 43 > 40。读者会误判成超限。

**改动**：摘要里同时给「模型轮数」与「工具调用数」，或注明定义。建议 `end` 事件里直接带上 `iterations`（`#1` 已经在改这个结构体）。

### 4.5 #13 缺值不要静默回退

模型的非密钥名与 Base URL 为空时，脚本用了计划里的默认值却没标注（报告第 45 行自认了这点）。**改动**：summary 里打印 `model_source: env|default`；并用服务端的 usage 表反查实际模型——`llmgateway/service.go:313-321` 与 `streaming.go:51-67` 已按 project 落库，两边对上才算证据闭环。

**顺带修一个真实不一致**：`streaming.go:55-58` 用 `TotalTokens == 0` 判空就**不记录**，而如果上游只回分项不回 `total`，这次用量就永久丢失；`api/llm_handler.go` 的测试里已经有「上游只回填分项、未回填 total 时按求和补齐」的约定（`llm_handler_test.go:221`），但 gateway 侧的 `maybeRecord` 没有对齐。**这是「有 usage 但没入库」，会让 #13 的反查落空。** 建议 `extractUsage`（`streaming.go:70-80`）同样按分项求和补齐，或用与 handler 一致的非零判据。

---

## 5. 明确不做

- **不为刷分放宽 `sanitizedEnv`**（上游计划 §10 已明确），也不改题集筛选条件。
- **不在 `#1` 完成前开 core 层**。SWE-bench Lite 20 题叠加后的耗时与费用当前完全不可控。
- **不优化单题耗时**。HumanEval 多为 3 步、耗时集中在单次模型往返延迟（同题两次差 6 倍：`HumanEval/1` 冒烟 19.3 秒 vs 全量 120 秒，都是 3 步）。在 token 数可见之前，压缩阈值（`cli/compaction.go:28-44` 的 32000）这类调参没有度量依据。
- **不新增第三方依赖、不改 `go.mod`**。本文用到的 `litellm.ResilienceConfig` 与 Eino `ModelRetryConfig` 都已在 v1.5.8 / v0.9.19 里。
- **不把服务端云助手的 SSE 分数混进本地 gocloudagent 的成功率**。

---

## 6. 里程碑与验收

每个里程碑单独开 PR。与上游计划的 M1–M5 并行，编号用 O（optimization）避免混淆。

| 里程碑 | 内容 | 验收 |
|---|---|---|
| O1 | `#1`（end 带 usage/model/duration）+ `#13` 的 gateway 使用量补齐 | `go test ./internal/gocloudagent/cli/ ./internal/llmgateway/` 通过；假 HTTP 用例断言 `end` 含数字；session jsonl 旧文件仍可 `resume`（`SessionVersion` 保持 1） |
| O2 | `#2` 网络级重试 + `#3` provider 超时透传 | 假上游 502×2 后成功；断言 `Resilience` 配置确实到达 litellm；Ctrl+C 不触发重试；`-race` 通过 |
| O3 | `#4` `length` 不执行残缺调用 + `#5` 显式输出上限 | 单测：构造 `FinishReason="length"` 的 assistant 消息带 ToolCalls，断言**工具未被调用**且发出 `truncated` 错误结果；断言请求体 `max_tokens` 等于配置值 |
| O4 | `#6` 总时长预算 + `#7` 重复动作检测 + `#8` 停止判据 | 单测：超时 → `Reason="timeout"`；同一动作重复 5 次 → `Reason="loop_detected"`；`end` 事件带新 reason |
| O5 | `#9`–`#13` 评测侧 | 冒烟重跑一次，report 含 `failure_class` 与 `model_source`；钻头用例在无模型路径下三种被拦情形都有断言 |

**O1 完成后立刻做的一件事**：用重跑后的 token 数反算本次 172 题的实际费用，与上游计划 §2.3 的量级估算对照。这既验证 `#1`，也给下一层预算提供真实基数。

---

## 7. 实施时的第一批核对命令

```bash
# 确认基线
git log --oneline -1

# 编译（CGO 必开，DuckDB 依赖）
CGO_ENABLED=1 go build ./internal/... ./cmd/...
go test ./internal/gocloudagent/... ./internal/llmgateway/... -short

# #6 之前先确认 CLI 当前没有超时参数
grep -n "timeout\|Timeout" internal/gocloudagent/cli/command.go internal/gocloudagent/cli/repl.go

# #5 的验证起点：先证明 4096 是否真的被应用（见 §3.1）
grep -rn "max_tokens" internal/gocloudagent/ internal/api/oai_dto.go
```

`grep` 最后两条预期都是空或极少命中——**命中为空本身就是本计划的证据**。
