# 云助手 v2：加载指示器 / SSE 完整性 / Agent 写能力 / 运行日志

> **仓库**：SimpleBase
> **日期**：2026-10-13
> **前版**：[`cloud-assistant-chat-ux-plan.md`](./cloud-assistant-chat-ux-plan.md)（下称 v1；§13 记录了响应式与头像 Spinner 的第一轮修复）
> **状态**：**已实施**（2026-10-13，代码 + 测试全部落地；执行结果见 §10）
> **约束**：[`AGENTS.md`](../../AGENTS.md)、[`internal/AGENTS.md`](../../internal/AGENTS.md)、[`ui/AGENTS.md`](../../ui/AGENTS.md)。不新增第三方依赖；对象形状用 `interface`；文案两棵树同构；不代为执行 DDL（迁移只定义，由用户手动执行或以 `TIMER_NO_RUN=0` 启动时应用）。
> **关联计划**：[`cloud-assistant-skills-cli-v2-plan.md`](./cloud-assistant-skills-cli-v2-plan.md)（D1–D6 决策，写能力方案必须与它对齐）

## 0. 结论速览

| # | 议题 | 结论 |
| --- | --- | --- |
| A | 对话中加载指示器「不转」，且不该换掉机器人头像 | v1 把助手头像替换成 `Spinner` 是**方向错误**，本版改为：头像恒为模块图标，忙碌态在气泡内用「三点跳动」表达。CSS 动画受 `prefers-reduced-motion` 影响，需要静态可识别兜底 |
| B | SSE 是否完整实现 | **骨架完整**（帧格式、Flush、双心跳、落库顺序、环形缓存 + `after` 续传都在）。但有 **5 个可确证缺陷**（续传缓存丢失返回空 200、子通道静默丢帧、`truncated` 断链、`confirmHub` 泄漏、连接建立期无法续传）和若干边界缺口 |
| C-1 | Agent 写操作 | **写路径早就存在**（`skills_cli` + 委托令牌 + CLI 调 HTTP API），缺的是 **Agent 级开关**。当前所有 Agent 只要能加载 skill 就有写能力，且没有任何一层检查「这个 Agent 是否被允许写」 |
| C-2 | skill 优化 | 现有 9 份 SKILL.md 共约 8KB，写命令与必填 flag 说明偏薄；补样例与生成规范，并新增机器可读的只读/可写标注 |
| C-3 | gocloudagent 运行日志 | 目前**零日志**。提示词与正文日志与 `internal/AGENTS.md` 的脱敏约定正面冲突，需新增显式开关作为例外，且**不得**写入系统库 logs 表 |

---

## 1. 议题 A：加载指示器改为「…」

### 1.1 现状

| 位置 | 现状 |
| --- | --- |
| `ui/src/components/agent/ConversationView.vue:18-21` | 最后一条助手消息运行中时，**头像位置**渲染 `Spinner`，否则渲染 `iconForAgentModule(m.module)` |
| `ui/src/components/ui/spinner/Spinner.vue:18` | `Loader2Icon` + `animate-spin`，带 `role="status"` 与 `aria-label="正在加载"` |
| `ui/src/style.css:173-181` | `@media (prefers-reduced-motion: reduce)` 把 `animation-duration` 压到 `0.01ms !important`、`animation-iteration-count: 1`。**系统开了减少动画时，`animate-spin` 会肉眼看上去静止** |
| `ui/src/pages/AgentManager.vue:125-127` | 输入框上方小字：`statusText` 或 `toolsHint`；`statusText` 由 `useAgentConversation.ts:41-57` 按 phase 生成（`思考中 · Ns` / `回复中 · Ns` / `执行工具 xx…` / `等待确认` / `完成 · …` / `已停止`） |

**这解释了「不转」的两种可能**：一是用户看到的是头像（v1 改的 Spinner 被气泡内容遮挡或尺寸过小），二是系统开了减少动画。仅靠换图标无法根治，需要不依赖旋转语义的表达。

### 1.2 方案

1. **头像恢复**：`ConversationView.vue` 的助手头像恒为 `iconForAgentModule(m.module)`，去掉 v1 加入的 `Spinner`。头像 `data-agent-icon` 属性保留（`ui/tests/nav-icons.test.ts` 依赖）。
2. **气泡内三点**：在助手气泡内（`AgentMarkdown` 之前）新增一个忙碌指示器，条件沿用 `sending && !confirmation && i === messages.length - 1 && m.role === 'assistant'`。用三个 `<span>` 加 `animate-bounce` 与任意值延迟类：

   ```html
   <span class="flex items-center gap-1" aria-hidden="true">
     <span class="size-1.5 rounded-full bg-current animate-bounce" />
     <span class="size-1.5 rounded-full bg-current animate-bounce [animation-delay:-0.15s]" />
     <span class="size-1.5 rounded-full bg-current animate-bounce [animation-delay:-0.3s]" />
   </span>
   ```

   不需要改 `style.css`、不需要新建 `ui/` 组件（`src/components/ui/` 是 fork 库代码，本次无需触碰）。
3. **无障碍与减少动画**：
   - 三点容器用 `aria-hidden="true"`，避免与 `Spinner` 的 `role="status"` 语义重复造成屏幕阅读器读两遍。
   - 减少动画时三点会静止但仍可见，作为静态「进行中」标记可接受；**不**用 `!important` 覆盖用户的减少动画设置（v1 §13.2 已定调）。
   - 是否把三点换成 `<Spinner>` 之外的动效（如打字机）**不做**：打字机要么新增 `@keyframes`，要么用 JS 定时器逐字，两者都比三点复杂且收益有限。
4. **`statusText` 保持原样**：它已经有 `…`（`执行工具 xx…`），不额外加动态省略号；`agentPhase.awaitingConfirm` / `canceled` 是精确文案，改了会破坏 `toBe` 断言（见 §1.3）。

### 1.3 测试与约束

| 文件 | 现有断言 | 影响 |
| --- | --- | --- |
| `ui/tests/agent-components.test.ts:87,94` | `[role="status"][aria-label="正在加载"]` 恰好 1 个 | 三点用 `aria-hidden` 不产生第二个 `role="status"`，可保持 1 个（若保留气泡内 `Spinner` 则为 1，若彻底移除 `Spinner` 需同步改断言） |
| `ui/tests/agent-components.test.ts:93,101-106` | `[data-agent-icon] svg` 含/不含 `animate-spin` | **必须改写**：v1 加的断言与本版目标相反 |
| `ui/tests/useAgentConversation.test.ts:88` | run 结束后 `[role="status"]` 不存在 | 需同步：若三点不带 `role="status"`，此断言改为断言三点容器消失 |
| `ui/tests/agent-skills-stream.test.ts:137-177` | `statusText` 的 `toContain('思考中')` / `toBe('等待确认')` / `toBe('已停止')` | 不动 `statusText` 文案即可全部保持 |

**待定项**：气泡内是否保留 `Spinner`。两个选项——(a) 只留三点，`role="status"` 由三点容器承担（`aria-label` 用 `common.loading`）；(b) 三点纯装饰 + `Spinner` 保留在气泡右下。建议 (a)，视觉更干净，且与「不要换头像」的诉求一致。

### 1.4 决策

| # | 决策 | 理由 |
| --- | --- | --- |
| D-A1 | 忙碌态不再占用头像位，改为气泡内三点 | 用户明确要求保留机器人头像；头像同时承担「哪个模块在回答」的信息 |
| D-A2 | 三点用 `animate-bounce` + 任意值延迟类，不新增 CSS | `tw-animate-css` 已引入；`style.css` 只放主题 token |
| D-A3 | 不做打字机 | 收益低于实现与维护成本；CSS 打字机还会与减少动画策略纠缠 |

---

## 2. 议题 B：SSE 完整性核查

### 2.1 已完整实现的部分（逐跳证据）

| 环节 | 位置 | 结论 |
| --- | --- | --- |
| 响应头 | `internal/api/agent_handler.go:779-781` | `text/event-stream` + `no-cache` |
| 帧格式与 Flush | `agent_handler.go:787-813` | `id: N\ndata: {...}\n\n`，每帧 `Flusher.Flush()`；事件 ID 分配、环缓存写入、SSE 输出在同一把 `mu` 内（RACE-01） |
| 注释心跳 | `agent_handler.go:835-847` | 15s `: ping\n\n`，走 `writeBytes`（不入环、不占 id） |
| thinking / tool_progress | `agent_handler.go:848-876` | 2s ticker；工具执行中发 `tool_progress`，空闲 ≥2s 发 `thinking`；两者入环 |
| 收尾顺序 | `agent_handler.go:891-907` | 停后台 goroutine → 先落库（`finishAgentRun`）→ 再 `usage` → 最后 `end`。**收到 `end` 后立刻拉 `/messages` 必然能读到助手消息** |
| 环形缓存 + 续传 | `internal/api/run_events.go:149-171`、`322-370` | `after` 语义、`stream_gap`、live 订阅都在 |
| 输出截断 | `internal/gocloudagent/runtime.go:250-264` | `maxToolCardChars=4000`，事件带 `Truncated` |
| reader 透传 | `internal/llmgateway/service.go:280-314`、`streaming.go:28-44` | `usageRecordingReader` 原样返回同一 chunk，不改内容、不吞帧 |

上游协议侧也确认支持真流式：`model.go:120-207` 用 `schema.Pipe` 逐块 `Send`，`runtime.go:573-625` 的 `consumeAssistant` 逐块 `emit(Event{Type:"token"})`；`internal/api/agent_e2e_stream_test.go:27-51` 用 fake 上游断言「≥5 个 token 帧」。

**结论：不存在「后端 API 不支持流式」的问题。** 用户在 v1 反馈的「看不到输出」由前端响应式缺陷解释（v1 §13 已修）。

### 2.2 确证缺陷（建议按此优先级修）

#### BUG-B1（P0）续传缓存整条丢失时返回 200 空体，拿不到 `stream_gap`

- `run_events.go:153-156`：`b == nil` 时返回 `gap=false`。于是 `ReplayEvents`（`:355-357`）直接返回，客户端收到 **200 空流**。
- 触发条件现实：`end` 后 2 分钟 `drop`（`:69-73`、`:140-147`）、hub 总量 512 条淘汰（`:89-111`）、无 `end` 条目 10 分钟 TTL（`:123-138`）。三者任一命中，`b` 就没了。
- 后果：前端只知道「流断了」（`ui/src/services/http-api.ts` 抛 `error.streamFailed`），重试 3 次后 `onError`，**而不是** `stream_gap` 的「请刷新消息」语义。
- **修复**：`b == nil` 时返回 `gap=true`（缓存已过期，等价于 frames 起点不可达），让客户端走 `stream_gap` 分支。是否需要区分「run 不存在」与「缓存已过期」——建议前者仍 404，后者 `stream_gap`，需要在 `replay` 签名上区分「未注册」与「已淘汰」。

#### BUG-B2（P0）子通道静默丢帧，无 gap 补偿

- `run_events.go:168`：`sub` 容量 16；`:63-68` 满即丢弃；`ReplayEvents` 的 live 循环（`:359-369`）**不校验 id 连续性**；前端也只记 `lastId` 不检测跳号（`http-api.ts:573`）。
- 触发条件：确认等待（最长 `ConfirmTimeout`，默认 2 分钟）+ 每 2s 一片心跳 + 慢客户端。
- **修复**：`append` 丢帧时记录 `gapFrom`，live 循环检测到上一帧 id 与当前帧 id 不连续时补发 `stream_gap`；前端在 `onEvent` 里检测 `_sse_id` 跳号并触发刷新。两侧都要做，任一侧单独做都不足以闭环。

#### BUG-B3（P1）`truncated` 端到端断链

- 后端：`runtime.go:263` 正确发送 `Truncated`；UI 类型与徽标也在（`ui/src/services/types.ts:414`、`ui/src/components/agent/AgentToolCard.vue:28`）。
- 断点：`ui/src/services/http-api.ts:636-637` 的 `onToolResult` 只传 `(name, content, call_id, duration_ms, is_error)`，**丢掉 `truncated`**。实时流里永远不显示「已截断」，只有刷新后从 `message.tool_calls` 载入才显示。
- **修复**：`AgentStreamHandlers.onToolResult` 增加 `truncated` 参数；`useAgentConversation.ts:156-172` 写入 `card.truncated`。注意这是跨层契约变更，`ui/src/services/types.ts` 与所有 fake 实现要同步。

#### BUG-B4（P1）`confirmHub.pending` 只增不减

- `run_events.go:225-244` 的 `prepare` 为每个 `(runID, callID)` 建 `chan bool`；`Resolve`（`:265-286`）不删；`DropRun`（`:254-261`）只删 `initiators`。每次破坏性确认留一条**永久** map 条目。
- **修复**：`Resolve` 与 `Wait` 返回路径（成功/超时/取消）都 `delete(h.pending, key)`；`DropRun` 一并清理该 run 前缀的所有 `pending` 条目（key 是 `runID + "\n" + callID`，需要遍历或改用嵌套 map）。

#### BUG-B5（P1）连接建立期无法续传

- `runID` 只由第一个 `run` 事件下发（`agent_handler.go:814`，前端 `http-api.ts:578-583`），而续传重试的前置条件是 `runId` 非空（`http-api.ts:677`）。连接在 `run` 帧到达前断开时，客户端**没有任何 runID 来源**。
- **修复**：`streamRun` 在 `WriteHeader` 之前设置 `X-Run-Id: <run.ID>`（`run.ID` 在进入 `streamRun` 前已生成），前端在 `fetch` 返回后立即读取该响应头作为 `runId` 初值。注意该头需要加进 CORS 的 expose headers（如启用了跨域）。

### 2.3 边界缺口（记录，量力而行）

| # | 缺口 | 位置 | 处理建议 |
| --- | --- | --- | --- |
| B6 | `ReplayEvents` 无心跳、无写超时 | `run_events.go:359-369` | 半开连接会长期占 handler + sub。建议加心跳与写超时，与 `streamRun` 对齐 |
| B7 | 缺 `X-Accel-Buffering: no` | `agent_handler.go:779-781` | 全仓无该头（已确认 0 命中）。nginx 类反代可能缓冲 `: ping`；需按实际部署拓扑决定，属**不确定项** |
| B8 | `textMode` 置位后不再检查 marker | `model.go:176-188` | 有意取舍：只有流开头才判定工具调用。**不改**，但要在注释里写明 |
| B9 | 短回答恰为 marker 前缀时首帧延迟到流结束 | `model.go:198-205` | 概率低、影响小。若实测复现，给暂存加长度/时间上限 |
| B10 | `Stream` 失败退 `Chat` 产生单帧流 | `model.go:101-118` | 单帧一次性输出，无 thinking 间隔。建议在 trace 日志里记 warn（见 §5），便于定位 |
| B11 | `sweep` 只挂在 `append` 上 | `run_events.go:74` | 无新 run 活动时，无 `end` 条目要等下次任意 append 才回收。可接受，建议加显式定时 sweep |
| B12 | 非 CLI 工具无 `tool_start` | `runtime.go:262-264` | sandbox/sql/obj/logs 只有 `tool_result`；执行期只能看通用 thinking 心跳。多工具并存时 `tool_progress` 只报 map 里任意一个（`agent_handler.go:864-867`） |
| B13 | `tool_result` 的 `CallID` 可能为空 | `runtime.go:232-249` | 前端按空 callID push 新卡片可能重复。**不确定项**（取决于 adk 是否总带 `ToolCallID`） |
| B14 | usage 语义不一致 | `streaming.go:37`（`accum = *u` 替换而非累加）、`:55-58`（`TotalTokens==0` 跳过）、`service.go:308` vs `:268`（流式记 `cfg.Name`、非流式记 `resp.Provider`） | 用量面板的 provider 维度可能对不上。**不确定项** |
| B15 | runtime 提前 break 未关迭代器 | `runtime.go:213-224` 无 `defer iter.Close()` | 存在数据竞争嫌疑，需 `-race` 用例确认 |
| B16 | 落库失败路径缺 `usage` 帧 | `agent_handler.go:893-897` | 只发 `error` + `end`，前端 metrics 停留旧值 |
| B17 | `h.events == nil` 时返回 404 `run not found` | `run_events.go:332-334` | 语义混淆，建议改 503 或明确文案 |

### 2.4 需要补的测试

- **新建 `internal/api/run_events_test.go`**：`replay` 的 `after` 边界（`0` / `frames[0].id-1` / `frames[0].id` / `done` / **`b == nil`**），后两者断言 `stream_gap` 而非空数组；`ReplayEvents` 在 run 已 drop 后返回 `error{code:stream_gap}`；子通道溢出（>16 帧）断言补发 `stream_gap`；`evictLocked` 淘汰后的 replay 行为。
- **`run_events_lifecycle_test.go` 增补**：`confirmHub.pending` 在 `Resolve`/timeout 后被清理、条目数受限（修 B4 后）。
- **`agent_skills_e2e_test.go` 增补**：帧 `id` 严格单调递增且与 `after` 续传连贯；`canceled` 路径「无 error 帧、有 end」；落库失败路径的 `error` + `end`；`tool_result` 的 `truncated=true` 在 >4000 runes 输入下透传到帧；**收到 `end` 后立刻拉 `/messages` 必须能读到 assistant 消息**。
- **UI**：`onToolResult` 传递 `truncated` 的单测；续传重试收到 `stream_gap` 时的用户可见行为。

---

## 3. 议题 C-1：Agent 写能力

### 3.1 现状：写能力已存在，缺的是 Agent 级开关

写路径**不是新功能**，已经跑通：

```
agent.skills_cli=true (config.go:224, 默认 true :453)
  → runtime.go:141-153 把只读数据工具整体换成 singlebase CLI 工具
  → runtime.go:143 Issuer.Issue(...) 签发委托 JWT（复制调用者 Permissions，TTL ≤30m）
  → cli_tool.go:252-289 子进程执行，环境变量 SIMPLEBASE_TOKEN=<委托令牌>
  → sbcli 用 gosdk 调既有 HTTP API（sql exec / document insert / object upload / function create …）
  → router.go:361-538 的 delegable(DatabaseWrite) + writeGate 鉴权
```

委托令牌只放行路由表标记 `delegable` 的路由（`auth/middleware.go:69,118`），`agent-*` 路由**不可委托**（防递归，`cloud-assistant-skills-cli-v2-plan.md` D2）。

**问题在于没有任何一层检查「这个 Agent 是否被允许写」**：

| 层 | 现状 |
| --- | --- |
| 工具 | CLI 模式下只有 `simplebase` 一个工具，可写；只读模式（`skills_cli=false`）下只有只读工具（`tools.go:107-267`），无写方法（`access.go:118-135`） |
| 令牌 | 调用者有 `database:write` 就有写能力；**Agent 自身没有任何约束** |
| 提示词 | `modules.go:47` 写「Tools are readonly」、`:56`「Do not run writes」，但 CLI 模式实际能写——**提示词与行为矛盾** |
| 破坏性确认 | `cli_tool.go:202-231` 只对 `destructiveArgv` 的真子集（DROP/DELETE/TRUNCATE…）要求确认；`sql exec INSERT`、`object upload`、`function create` **不需要确认**（`:424-469`） |
| 数据模型 | `systemdb/agents.go:24-37` 的 `CloudAgent` **没有** `writable` / `permissions` / `scope` 字段 |

### 3.2 数据模型

以「Agent 级布尔开关 + 可选范围」为最小方案：

| 项 | 设计 |
| --- | --- |
| 迁移 | 新增 **v48**：`ALTER TABLE sys_cloud_agents ADD COLUMN writable BIGINT DEFAULT 0`。`team_enabled` 用的就是 `BIGINT NOT NULL`（`migrate.go:309`），读写时转 `int64`（`agents.go:108-113`、`:176-181`、`:656,665`），新列沿用同一形态。DuckLake 不支持 `ADD COLUMN NOT NULL`，用 DEFAULT（v41 `builtin_key VARCHAR DEFAULT ''` 是同类先例） |
| `CloudAgent` | 增加 `Writable bool`（`systemdb/agents.go:24-37`） |
| 读写点同步 | `agents.go`：`CreateCloudAgent`(`:111-113` INSERT 列表)、`ListCloudAgents`(`:129` SELECT 列表)、`GetCloudAgent`(`:153`)、`UpdateCloudAgent`(`:179-181`)、`scanCloudAgent`(`:653-667`)。**SELECT 的列顺序与 `Scan` 参数顺序必须严格一致**，否则会静默读错字段——必须有往返测试。注意 `scanCloudAgent` 当前是 12 列（`id … builtin_key`），新列加在末尾最安全 |
| 内置助手默认值 | `defaultCloudAgentSpecs`(`agents.go:695`) 决定内置助手是否默认可写 |
| 范围限制 | **不做**。`write_scope`（允许哪些库/资源）留作后续；本期只做布尔 |

**必须由用户决策的项**：`DEFAULT 0` 会让存量 Agent **失去**现有写能力（当前是隐式全开）；`DEFAULT 1` 保持现状但没有收紧。建议 `DEFAULT 0` + 前端显式开关，并在文档里说明这是一次行为变更。

### 3.3 API 层

| 位置 | 改动 |
| --- | --- |
| `agent_handler.go:41` `agentDTO` | 增加 `writable` |
| `agent_handler.go:170` `upsertAgentBody` | 增加 `*bool Writable` |
| `agent_handler.go:298-342` `normalizeAgent` | 写入 `Writable`；**权限提升防护**：只有当前主体具备 `database:write`（或 `project:admin`）时才允许把 Agent 设为可写，否则返回 403 |
| `agent_handler.go:544` `CreateRun` | `NormalizeSkills`（`:593`）之后，若 `!agent.Writable` 则忽略请求体的写类 skill——**但见 §3.4 的说明：skill 粒度无法区分读写，真正拦截在命令层** |
| `agent_handler.go:180` `denyReadonly` | 新写路径一并走实例只读门禁 |
| `agent_handler.go:742-743` 审计 | 写操作要进审计（脱敏，见 §5） |

### 3.4 运行时双层校验（核心）

关键洞察：**skill 前缀表是按资源划分的，不是按读写划分的**（`skills.go:20-29` 里 `database` skill 同时含 `sql query` 与 `sql exec`）。所以不能靠「过滤 skills」实现读写分离，必须在**命令（argv）粒度**拦截。

```
第一层（入口降权）  runtime.go:141-150
  不可写 Agent → 签发令牌前从 req.Principal.Permissions 移除
    database:write / database:admin
  这样 HTTP 路由层的 delegable(DatabaseWrite) 会 403 兜底（纵深防御）
  注意：IssueDelegation 本就是「调用者权限的子集」，降权不破坏既有约定

第二层（命令拦截）  cli_tool.go:198 之后
  if !agent.Writable && isWriteArgv(argv) {
      return skillFail("write_not_allowed", "该助手未开启写权限"), nil
  }
  必须在 destructiveArgv/Confirm.Wait 之前拦截，
  否则用户会先看到一个对不可写 Agent 弹出的确认框

第三层（HTTP 兜底）  router.go:361-538
  降权令牌自然 403
```

**`isWriteArgv` 的事实来源**：不要新建一份读/写分类表（必然漂移），而是扩展 `internal/sbcli`：

- `internal/sbcli/commands.go:20` 的 `dispatch` 是唯一的命令事实来源（`DispatchKnown` 已在 `run.go:85` 导出）。当前 `commands.go` 有 **76** 条 `case`。
- 新增导出 `func CommandReadOnly(resource, action string) bool`，为这 76 个命令逐一标注只读性。
- gocloudagent 侧 `isWriteArgv` 直接查它；对账测试枚举 `dispatch` 的全部命令，要求**每个命令都有明确分类**（防止新增命令漏标）。

同理，`destructiveArgv`（`cli_tool.go:424-469`）语义是「需要用户确认」，与「是否写」是两回事，不能复用。

### 3.5 提示词一致性

当前矛盾必须修掉，否则模型会拒绝合法写请求：

| 位置 | 现状 | 改法 |
| --- | --- | --- |
| `prompt.go:22-31` `PromptParts` | 只有 `SkillsCLI bool` | 增加 `Writable bool` |
| `prompt.go:33-40` `platformSkillsPrompt` | 固定文案「Writes go only through the simplebase tool」 | 按 `Writable` 分支：可写时说明「可在已加载 skill 范围内写」；不可写时说明「只读，写命令会被拒绝」 |
| `modules.go:44-51` `platformBasePrompt` | 含「Tools are readonly」 | 由 `AssembleInstruction` 按 `Writable` 选择 |
| `modules.go:53-56` `databaseModulePrompt` | 「Do not run writes」 | 同上，动态拼装 |
| `modules.go:70-73` `s3ModulePrompt`、`:75-77` `logsModulePrompt` | 同样写死只读 | 同上 |

建议把「读写声明」从各模块常量里抽出来，由 `PromptParts.Writable` 统一生成一句 `Capability: readonly|writable`，避免 6 处常量各写一遍。

### 3.6 定时任务

`scheduler.go:266-272` 的调度主体只有 `auth.DatabaseRead`，`:296-299` 传 `Headless: true`，破坏性命令直接返回 `confirmation_unavailable`（`cli_tool.go:203-208`）。**定时任务天然无写能力**。

需要在计划里明确策略（不能默认放开）：

- **选项 1（建议）**：定时任务始终只读，与 `Headless` 语义一致——无人可确认，写操作风险不可控。
- **选项 2**：继承 `agent.Writable`，但仅允许**非破坏性**写命令（跳过确认环节）。需要单独评审。

### 3.7 已知风险

| # | 风险 | 说明 |
| --- | --- | --- |
| R1 | **隐式写能力是既成事实** | `skills_cli` 默认 true，所有 Agent 都能写。引入 `writable` 是**收紧**，属行为变更，需要用户确认默认值 |
| R2 | `CreateRun` 只要求 `database:read` | 这是为让只读用户也能对话而有意的设计。降权令牌是必要补丁，否则只读用户的 Agent 写请求只在 HTTP 层被 403，而非入口拒绝 |
| R3 | 客户端可任意传 `skills` | `createRunBody.Skills`（`agent_handler.go:538`）由客户端提交，服务端只校验是否已知。不可写 Agent 必须忽略写类 skill |
| R4 | 写命令分类不完整 | `destructiveArgv` 不等于 `isWriteArgv`（见 §3.4）；`TestSkillCommandsDispatchable`(`skills_test.go:107`) 只校验示例可分发，不覆盖分类，必须补对账测试 |
| R5 | **不要**把 `user`/`apikey`/`settings` 前缀加回 | `cli_tool.go:430-431` 的破坏性清单含 `user delete`、`apikey revoke`、`settings set`，但 `skillPrefixes`(`skills.go:20-29`) 已不含对应前缀，当前**不可达**。放开写能力时不要顺手加回 |
| R6 | 写操作不保证原子性 | `sql batch` 中途取消可能部分提交。run 超时/取消时已发出的 CLI 子进程由 `CommandContext` 终止，但不能回滚 |
| R7 | 迁移列顺序错位会静默读错 | `scanCloudAgent`(`agents.go:657`) 按位置扫描，必须补往返测试 |
| R8 | 不可写 Agent 的确认框 | 拦截必须在 `Confirm.Wait` 之前，否则用户体验错乱（§3.4） |

### 3.8 测试

- `internal/systemdb`：v48 迁移 + `CloudAgent` 读写往返（含 `writable` 列顺序）。
- `internal/gocloudagent`：`isWriteArgv` 分类；不可写 Agent 对写命令返回 `write_not_allowed`；不可写 Agent 的令牌不含 `database:write`；提示词随 `Writable` 变化。
- `internal/sbcli`：`CommandReadOnly` 对 72 个命令的**全量分类对账**。
- `internal/api`：CreateAgent/PatchAgent 的权限提升防护（无 `database:write` 的主体设 `writable=true` → 403）；CreateRun 时不可写 Agent 的写命令被拒；e2e 中不可写 Agent 走 `@数据库` 的写样例返回 `write_not_allowed`。
- `ui`：`AgentFormModal` 的开关；`AgentManager` 的表单状态。

---

## 4. 议题 C-2：skill 优化（样例 + 生成规范）

### 4.1 现状

- 9 份文件：`skills/simplebase/SKILL.md`（索引，708B）+ 8 个子 skill（`cron` / `database` / `gofunction` / `kv` / `logs` / `project` / `s3` / `sandbox`），每份只有一个 `SKILL.md`。
- 实测体积：索引 708B；子 skill 903B（cron）/ 1706B（database）/ 991B（gofunction）/ 775B（kv）/ 647B（logs）/ 656B（project）/ 869B（s3）/ 1021B（sandbox），**合计 8276B**。命令示例行共 **37** 条（database 11、sandbox 7、cron 4、gofunction 4、kv 3、logs 3、s3 3、project 2）。
- 加载：`skills/embed.go` 的 `//go:embed simplebase` → `internal/gocloudagent/skills.go:13` 的 `skillFS`；`SkillPrompt`(`:127-143`) 按 id 读；`AllowArgv`(`:110-124`) 校验 `argv[0]` 落在已加载 skill 的前缀表内。
- 体量：database 约 1.67KB，其余更薄。`cloud-assistant-skills-cli-v2-plan.md:23` 已记录「每个 skill 正文只有 127–637 字节，没有写 flag 和示例」；看来只补了一部分。
- 格式约束（`skills_test.go` 强制）：
  - `:76` `TestEmbeddedAllowsMatchPrefixes`：每份的 `<!-- allow: … -->` 必须与 `skillPrefixes`(`skills.go:20-29`) 一致，且**条目数 = len(skillPrefixes)+1**（含 `index`）。`EmbeddedSkillAllows`(`skills.go:153-158`) 只走 `SKILL.md`，**新增其他文件名不受影响**。
  - `:107` `TestSkillCommandsDispatchable`：正文里以 `simplebase ` 开头的行，前两个词必须通过 `sbcli.DispatchKnown`。
  - `:69` `TestDatabaseSkillUsesSupportedDataModel`：database 示例必须含 `--data-model sql`，不得含 `relational`。

### 4.2 补全规范

1. **新增 `skills/simplebase/FORMAT.md`**（生成规范，不参与 allow 对账）：
   - 文件布局与命名：`<skill-id>/SKILL.md`，id 必须出现在 `skillPrefixes`。
   - 第 2 行 `<!-- allow: … -->` 的写法与必须与宿主表一致（说明「改这里必须同步改 `skills.go`」）。
   - 命令行示例格式：行首 `simplebase <resource> <action> …`，行尾 `← 说明`；**行首出现的命令必须可被 `sbcli` 分发**。
   - 必填 flag 清单、可选 flag、默认值。
   - 确认标记约定：哪些命令需要界面确认、headless 返回 `confirmation_unavailable` 且「不要重试」。
   - 错误码章节：`error.code` → 处理建议。
   - 与 `maxSkillChars=8192`（`skills.go:15`）的关系：单文件与多文件叠加都受此约束。
2. **每个子 skill 补到 1–3KiB**：每条命令写清必填 flag + 一条最小示例 + 需确认标记 + 常见错误码。重点是**写命令**（当前最薄）：`sql exec`/`sql batch`/`schema create-table`/`schema add-column`/`document insert|update|delete`/`object upload`/`function create|update`/`cron create|update`/`kv exec`。
3. **新增机器可读的读写标注**（可选增强）：在 `<!-- allow: … -->` 同一行或下一行加 `<!-- readonly: sql query,schema show,collection list,document list -->`，由 `skills.go` 解析并在测试里与 `sbcli.CommandReadOnly` 对账。收益是「skill 文档」与「宿主判断」双事实来源自动对齐；若把 §3.4 的 `CommandReadOnly` 作为唯一事实来源，则该标注只作展示用途。

### 4.3 测试

- 沿用 `TestEmbeddedAllowsMatchPrefixes` / `TestSkillCommandsDispatchable`（新增样例自动纳入校验）。
- 新增：每份 SKILL.md 的体积下限（如 ≥1KB）与上限（≤8KB，与 `maxSkillChars` 一致）。
- 新增：若采纳读写标注，与 `sbcli.CommandReadOnly` 全量对账。
- `TestDatabaseSkillUsesSupportedDataModel` 保持。

---

## 5. 议题 C-3：gocloudagent 运行日志

### 5.1 现状：零日志

- `internal/gocloudagent` 非测试代码里**没有任何 zap / slog 调用**，`Runtime` 结构体（`runtime.go:74-101`）没有 `Logger` 字段。
- 唯一日志是 `scheduler.go:366-374` 的 `Scheduler.log`，写 `Store.RecordLog(...)` 到系统库 logs 表。**这条路径必须避开**：Agent 的 `search_logs` 工具（`tools.go:229-241`）会读回这张表——提示词与正文写进去会**跨用户泄漏到 LLM 上下文**。
- llmgateway 有 logger（`service.go:105`），但只在用量记录失败时 warn（`:268-274`），不记内容。
- `app.go:904-906` 已有 `a.logger`（zap），装配 `Runtime` 时（`app.go:911-927`）注入很容易。

### 5.2 与既有脱敏约定的冲突

| 出处 | 约定 |
| --- | --- |
| `internal/AGENTS.md:57`、`:69` | 审计只记录 SHA-256 与元数据，不落原 SQL；**不得在日志/审计中输出 SQL 参数或 LLM 正文** |
| 根 `AGENTS.md:69`、`:107` | 日志/审计无密钥、无 SQL 参数、无 LLM 正文 |
| `internal/audit/service.go:5,39,84-90` | `Detail` 必须已脱敏；`Redact` 保留前 2 后 2 字符 |

用户要求的「打印提示词 / Agent 输出 / 用户提示词」**正面冲突**。这是一次**新增例外**，必须：

1. 默认**关闭**；
2. 只在显式配置开关打开时输出；
3. 输出前脱敏 + 截断；
4. 在 `internal/AGENTS.md` 与根 `AGENTS.md` 写明例外边界。

**这是需要用户拍板的决策项（D-C3）**。若不同意例外，则只做 §5.3 的元数据层。

### 5.3 两层日志设计

**第一层：元数据层（默认 info，符合现有约定）**

| 字段 | 内容 |
| --- | --- |
| 标识 | `run_id`、`project_id`、`thread_id`、`agent_id`、`module`、`model` |
| 输入摘要 | 用户文本与 system prompt 的**字符数 + SHA-256 前缀**（不落原文） |
| 执行 | history 条数、每次模型调用耗时、工具名与耗时、token 数、`reason`、错误码 |
| 主体 | 只用 `principalKey`（`cli_tool.go:299`），不记整个 `Principal` |

打点位置（当前全是空白）：

| 位置 | 事件 |
| --- | --- |
| `runtime.go:104` `StartRun` 入口 | run 开始、模型、模块、history 条数、输入摘要 |
| `runtime.go:301-315` | 各终止原因（max_iterations / canceled / timeout / error） |
| `runtime.go:273-290`、`:229-266` | tool_call / tool_result（工具名、耗时、是否截断） |
| `model.go:51-59` | text 协议降级（warn） |
| `model.go:75-77`、`:101-109` | 上游错误与 Stream→Chat 退化（warn，对应 B10） |
| `cli_tool.go:160-200` | 命令被拒（各 `skillFail`） |
| `cli_tool.go:202-231` | 破坏性确认请求/结果 |
| `cli_tool.go:252-289` | CLI 执行耗时与退出码 |
| `runtime.go:336` | `ErrThreadBusy` |

**第二层：内容层（默认关闭，双重条件）**

- 条件：新配置开关打开 **且** logger 为 debug 级别。单独设开关是因为 `log_level=debug` 会一并打开其他 debug 日志（如 `perf_stage`），不应因此泄漏正文。
- 内容：system prompt（组装后）、用户文本、history 消息、模型输出汇总、工具参数与结果。
- 处理：先 `RedactSecrets`(`prompt.go:69-79`) 再 `truncate`（建议 2000 字符）；工具参数用 `redactArgv`(`cli_tool.go:316-362`)；stdin 只记长度；**绝不**记 `execSimplebase` 的环境变量（`cli_tool.go:252-259` 含 `SIMPLEBASE_TOKEN`）。
- 流式输出**不逐块记**，在 `runtime.go:298` 汇总后记一次。

**已知缺口**：`secretLineRE`(`prompt.go:16`) 要求 `password` 后紧跟 `:` 或 `=`，而 JSON 形式的 `"password":"x"` 中间隔着引号，**很可能匹配不到**。工具参数与结果大多是 JSON，实施时必须补测试并必要时扩展正则。

### 5.4 配置开关

- `internal/config/config.go:223-228` 的 `AgentConfig` 增加 `DebugLog bool`（或 `LogPayloads bool`）。
- 同步 `yaml.go`、`env.go`（环境变量如 `SIMPLEBASE_AGENT_DEBUG_LOG`，参照 `SIMPLEBASE_AGENT_SKILLS_CLI`(`env.go:150`)）、`config.example.yaml`。
- **不要**依赖 `zapLogger.level`：它是未导出字段（`observability/zaplog.go:26-29`），外部无法判断 debug 是否开启。用配置布尔值判断开关即可，同时用 debug 级别 `logger.Debug(...)` 让 zap 自己过滤。
- **不要**用 `observability` 的全局函数（`zaplog.go:31,213-220`），`logger.go:13` 明确禁止全局变量用法；用注入的实例 logger。

### 5.5 要改的文件

| 文件 | 改动 |
| --- | --- |
| `internal/config/config.go`、`yaml.go`、`env.go`、`config.example.yaml` | 新增开关与默认值 |
| `internal/app/app.go:911-927` | 给 `Runtime` 注入 `Logger` 与开关 |
| `internal/gocloudagent/runtime.go` | 增加字段；在入口/结束/终止分支/工具事件打点 |
| `internal/gocloudagent/model.go` | 请求与返回摘要；降级与退化 warn；内容层记最终消息列表 |
| `internal/gocloudagent/cli_tool.go`、`tools.go` | 命令被拒、确认结果、退出码、耗时 |
| **新建** `internal/gocloudagent/trace.go` | 集中放开关判断、脱敏 + 截断、hash 辅助函数，避免各处各写一套 |
| `internal/AGENTS.md`、根 `AGENTS.md` | 写明内容层例外（仅 D-C3 通过时） |

### 5.6 测试

- 复用 `observability.NewLogger("debug","json",&buf)` 做输出断言（参照 `usage/coverage_test.go:89`、`api/stage_timing_test.go:180`）。
- 用例：
  - 开关关闭且 info 级：输出里无任何正文，只有元数据与 hash。
  - 开关打开：在用户文本、history、system prompt、工具结果里埋 `sk-…`、`AKIA…`、`password=…`、`Bearer …`，断言输出全是脱敏标记。
  - **JSON 形式** `"password":"x"` 的用例（验证 §5.3 的正则缺口）。
  - 超长内容按上限截断。
  - `simplebase` 命令用 `redactArgv`；stdin 不出现；`SIMPLEBASE_TOKEN` 不出现。
  - 流式模式下不逐 token 写日志。
  - 配置测试覆盖 yaml / env / 默认值（参照 `config_more_test.go:266,334`）。

---

## 6. 连带改动与依赖顺序

```text
A（前端三点）              独立，可先做，纯 ui
B（SSE 缺陷）              独立，纯 Go（+ B3 涉及 ui 契约）
C-2（skill 样例与规范）    独立，纯文档 + 测试
C-1（Agent 写能力）        依赖 C-2 的只读/可写标注与 sbcli.CommandReadOnly
C-3（日志）                独立，但元数据层建议先落地，便于排查 B 类问题
```

建议顺序：**C-3 元数据层 → B（按 B1..B5 修）→ A → C-2 → C-1 → C-3 内容层（待 D-C3）**。

理由：日志先落地，B 类的「续传/丢帧/首帧延迟」排查才有抓手；C-1 是行为变更，放在最后，且需要 C-2 的分类事实来源先就位。

### 6.1 迁移与数据库

- 唯一迁移是 **v48**（`sys_cloud_agents.writable`）。
- 按既有要求，**代码只定义迁移，不代为执行 DDL**：需用户手动执行，或以正常模式（`TIMER_NO_RUN=0`）启动服务由启动流程应用。以 `TIMER_NO_RUN=1` 纯 API 模式运行不会应用迁移——未应用时新代码的 `SELECT` 会失败，这点必须在实施说明里写明。
- 当前最新版本号是 47（`migrate.go:652-660`），v48 追加在数组末尾。

---

## 7. 决策记录（待确认）

| # | 决策 | 建议 | 需要用户确认 |
| --- | --- | --- | --- |
| D-A1 | 忙碌态改为气泡内三点，头像保持模块图标 | 采纳 | 否 |
| D-A3 | 不做打字机效果 | 采纳 | 否 |
| D-B1 | `b == nil` 时返回 `stream_gap`（区分「run 不存在」与「缓存过期」） | 采纳 | 否 |
| D-B3 | `onToolResult` 增加 `truncated` 参数（跨层契约变更） | 采纳 | 否 |
| D-B5 | 增加 `X-Run-Id` 响应头（含 CORS expose） | 采纳 | 否 |
| **D-C1a** | `sys_cloud_agents.writable` 的默认值 | **`DEFAULT 0`**（收紧，行为变更） | **是** |
| D-C1b | 定时任务是否可写 | **始终只读** | **是** |
| D-C1c | 是否本期做 `write_scope` 范围限制 | 不做，留后续 | 否 |
| D-C2a | 新增 `skills/simplebase/FORMAT.md` 生成规范 | 采纳 | 否 |
| D-C2b | 是否新增 `<!-- readonly: … -->` 机器可读标注 | 采纳（与 `sbcli.CommandReadOnly` 对账） | 否 |
| **D-C3** | 内容层日志（提示词/正文）作为 `internal/AGENTS.md` 的显式例外 | 采纳「默认关闭 + 开关 + 脱敏 + 截断」 | **是** |

---

## 8. 验收清单

| # | 项 | 验证方式 |
| --- | --- | --- |
| 1 | 头像恒为模块图标；忙碌时气泡内三点可见 | `ui/tests/agent-components.test.ts` 改写断言；人工过一眼 |
| 2 | 系统开启减少动画时三点静止但可见 | 人工（DevTools 模拟 `prefers-reduced-motion`） |
| 3 | run 结束后三点与 `statusText` 正确收尾 | `useAgentConversation.test.ts` |
| 4 | 续传缓存过期返回 `stream_gap` 而非空 200 | 新建 `run_events_test.go` |
| 5 | 子通道溢出时客户端能感知跳号 | `run_events_test.go` + UI 单测 |
| 6 | 实时流显示「已截断」 | UI 单测 |
| 7 | `confirmHub.pending` 条目数不随确认次数增长 | `run_events_lifecycle_test.go` |
| 8 | 连接建立期断开后仍能续传 | e2e（首个事件前断开） |
| 9 | 不可写 Agent 的写命令返回 `write_not_allowed`，且不弹确认框 | `internal/gocloudagent` + `internal/api` e2e |
| 10 | 不可写 Agent 的委托令牌不含 `database:write` | `internal/api` 单测 |
| 11 | 无 `database:write` 的主体不能把 Agent 设为可写 | `agent_handler_test.go` |
| 12 | `sbcli` 全部命令的只读/可写分类完整 | 新建对账测试 |
| 13 | 提示词随 `Writable` 变化，不再自相矛盾 | `internal/gocloudagent` 单测 |
| 14 | 37 条 skill 示例（补全后应更多）全部可分发、每份体积在 1–8KB | `skills_test.go`（新增体积断言） |
| 15 | 开关关闭时日志无正文；打开时正文全部脱敏 + 截断 | `internal/gocloudagent` 单测（含 JSON 密钥形态） |
| 16 | `go build ./internal/... ./cmd/...`、`go test ./...`、`cd ui && yarn test && yarn build` 全部通过 | 命令 |

---

## 9. 不做的事

1. **不改后端流式实现本身**（帧格式、心跳、Flush 保持不变）——它已经完整；只修 §2.2 的五个缺陷。
2. **不引入新的聊天传输层**（不换 WebSocket、不加轮询）。
3. **不做 `write_scope` 范围限制**（D-C1c）。
4. **不给定时任务默认开写**（D-C1b）。
5. **不把 `user`/`apikey`/`settings` 前缀加回 skill**（R5）。
6. **不用 `Store.RecordLog` 写运行日志**（会被 Agent 读回，跨用户泄漏）。
7. **不覆盖 `prefers-reduced-motion`**。
8. **不改 `src/components/ui/` 的库组件风格**。

---

## 10. 执行状态（2026-10-13）

按 §6 建议顺序执行：**C-3 元数据层 → B → A → C-2 → C-1 → C-3 内容层**。全部落地，无跳过项。

### 10.1 逐项落地

| 议题 | 状态 | 落点 |
| --- | --- | --- |
| A 加载指示器 | 完成 | `ConversationView.vue`：头像恢复模块图标，忙碌态改为气泡内 `[data-typing-dots]` 三点（`role="status"` + `aria-label`），移除气泡内 `Spinner` 与导入 |
| B1 缓存丢失返回 gap | 完成 | `run_events.go` `replay`：`b == nil` 返回 `gap=true` |
| B2 子通道丢帧 | 完成 | `ReplayEvents` live 循环按 `id != last+1` 补发 `stream_gap`；期望值取「末帧 id」或客户端 `after` |
| B3 `truncated` 断链 | 完成 | `types.ts` `onToolResult` 增加 `truncated`；`http-api.ts` 透传；`useAgentConversation.ts` 写入 `card.truncated` |
| B4 `confirmHub` 泄漏 | 完成 | `Wait` 加 `defer release`；`Resolve` 仅在放行时回收；`DropRun` 清理该 run 全部条目；新增 `pendingCount` |
| B5 建立期无法续传 | 完成 | 后端 `streamRun` 增设 `X-Run-Id` 响应头；前端 `fetch` 后读取该头作为 `runId` 初值 |
| C-1 Agent 写能力 | 完成 | 迁移 **v48** `sys_cloud_agents.writable BIGINT DEFAULT 0`；`CloudAgent.Writable`；入口降权 `stripWritePermissions`；命令拦截 `write_not_allowed`；提示词 `capabilityWritable/Readonly`；API `writable` 贯通 + 提权防护；前端表单开关 |
| C-2 skill 优化 | 完成 | 新增 `skills/simplebase/FORMAT.md`；7 份子 skill 加 `<!-- readonly: … -->` 标注；`EmbeddedSkillReadOnly` 解析；`sbcli.CommandReadOnly` / `CommandReadOnlyWithArgv` |
| C-3 运行日志 | 完成 | 新增 `internal/gocloudagent/trace.go`（`Meta`/`Warn`/`Payload`/`Redact`/`hashSummary`）；`Runtime.Logger` + `TraceLog`；`agent.trace_log` 配置（yaml/env/example）；`AGENTS.md` 例外说明 |

### 10.2 与计划的偏差（实现时发现）

| # | 计划内容 | 实际问题 | 处理 |
| --- | --- | --- | --- |
| 1 | B4 只在 `Resolve` 与 `Wait` 回收 pending | 首版在 `Resolve` 开头无条件删除，导致「非发起人先尝试确认 → 发起人再确认」变 404 | 改为**只在真正放行时**回收；校验失败时保留，由 `Wait` 的 `defer` 兜底 |
| 2 | 既有 E2E 用例可直接复用 | 两处 E2E 依赖 B4 的**泄漏条目**才通过：`agent_skills_router_e2e_test.go` 样例 19 用内存 `ServeHTTP` 同步发 run，run 早已结束，A 的确认靠永久 pending 返回 200 | 改样例 19 走**真实 HTTP 服务**，让 run 阻塞在 `Confirm.Wait` 期间确认。同时耗时从 21s 降到 1.1s |
| 3 | 「不可写 Agent 的破坏性命令不弹确认框」 | 计划未说明与 headless 检查的先后；既有 `TestHeadlessDestructiveUnavailable` 因写门禁先行而失败 | 明确顺序为「skill 前缀 → 写门禁 → 破坏性确认 → headless」，并在该用例补可写助手、另加 `TestReadonlyAgentBeatsHeadlessCheck` |
| 4 | 计划称 DEFAULT 0 是行为变更 | 影响面比预估大：`agent_skills_e2e_test.go` 的 12 个写流程子用例、router E2E 的 16 号全部转红 | 两处 harness 显式把内置助手标为可写（`seedWritableAgents` / `setAgentsWritable`），并新增 `14b`（只读拒绝写）、`14c`（只读仍可读）两个用例 |
| 5 | §5.3 记录 `secretLineRE` 可能漏 JSON 形态 | 实测确认：`{"authorization":"Bearer abc.def"}` 的值含空格时只脱敏到 `abc` | 拆为 `jsonSecretRE`（整段引号值，先应用）+ `secretLineRE`（非 JSON 形态），测试覆盖含空格的 JSON 值 |
| 6 | `function versions` / `sandbox files` 读写分类 | 首版把整条命令登记为只读，对账测试立刻失败 | 从基础表移除，改由 `CommandReadOnlyWithArgv` 按第三段细分（`list`/`get`/`read` 为只读） |
| 7 | 计划说 `CommandReadOnly` 覆盖 76 条命令 | 实际对账测试还揪出 `apikey list` 漏登记 | 补登记；对账测试覆盖全部 76 条 + 14 组子动作 |

### 10.3 验证结果（实测）

| 检查 | 结果 |
| --- | --- |
| `go build ./internal/... ./cmd/...` | 通过 |
| `go test ./...` | **全部通过** |
| `go vet ./internal/gocloudagent/... ./internal/sbcli/... ./internal/api/...` | 通过 |
| `gofmt -l internal/` | 无输出 |
| `cd ui && yarn test` | **107 文件 / 728 用例全部通过**；statements 99.51% / branches 95.07% / **functions 95.1%**（达标） |
| `cd ui && yarn build` | 通过 |

**新增/改写的测试**：

- 后端：`gocloudagent/trace_test.go`（脱敏 + 截断 + JSON 密钥形态 + hash/argv 摘要）、`gocloudagent/writable_test.go`（只读拒绝、可写放行、降权、提示词）、`api/run_events_test.go`（replay 边界 + gap + confirmHub 回收）、`api/agent_writable_test.go`（提权防护 + DTO 契约）、`systemdb/agent_writable_test.go`（v48 往返 + 存量兼容）、`sbcli/commands_readonly_test.go`（76 命令对账 + 子动作）、`gocloudagent/skills_test.go` 增补（体积 + readonly 标注对账）。
- 前端：`http-api.test.ts` 增补（`truncated` 透传、`X-Run-Id` 初值、无 end 报错）、`agent-components.test.ts`（三点与头像改写）、`useAgentConversation.test.ts`（逐帧 DOM + 三点收尾）、`AgentManager.test.ts` / `agent-split-components.test.ts`（writable 开关）。
- 改写既有：`agent_skills_router_e2e_test.go` 样例 19、`skills_test.go` 的 headless 用例、两个 E2E harness 的写权限前置。

### 10.4 待办（需要用户操作）

1. **迁移 v48 未执行**。代码只定义迁移，未代为执行 DDL。需要你手动执行，或以正常模式（`TIMER_NO_RUN=0`）启动服务由启动流程应用。以 `TIMER_NO_RUN=1` 纯 API 模式运行不会应用迁移 —— 未应用时 `sys_cloud_agents` 的 `SELECT` 会因缺列失败。
2. **行为变更需知会**：v48 之后所有助手的 `writable` 默认 `false`，此前「`skills_cli` 打开即可写」的隐式能力被收紧。需要写能力的助手要在界面上显式开启。
3. **真实部署路径未实测**：B 类修复已有单测与 E2E 覆盖，但「反向代理是否缓冲 SSE」「真实供应商是否逐块输出」仍需在实际部署链路用浏览器开发者工具确认（§2.3 B7 属不确定项）。
4. **B6/B11/B12/B13/B15** 等 §2.3 边界缺口未处理，按计划保留为后续项。
5. **内容层日志开关默认关闭**：需要排查时设 `SIMPLEBASE_AGENT_TRACE_LOG=true`；输出仍是脱敏 + 截断后的文本，勿贴入公开渠道。

