# 云助手对话体验 / favicon / 空数据不报错

> **仓库**：SimpleBase
> **日期**：2026-10-12
> **核对基线**：`main` @ `5a03d47` + 工作区未提交改动
> **范围**：`ui/`（主体）、`internal/systemdb`（会话归属助手的迁移）、`internal/api`（错误码、thread 接口）、`docs/`
> **约束**：[`AGENTS.md`](../../AGENTS.md)、[`ui/AGENTS.md`](../../ui/AGENTS.md)。不新增依赖；不改 `package.json` / `tsconfig.json` / `vite.config.ts`；对象形状用 `interface`；文案 `zh-CN` + `en-US` 同时补；`src/components/ui/` 只增不改风格，**不得重新引入 `dropdown-menu`**（菜单用 `Popover`）
> **状态**：**两轮均实施完成**（2026-10-12）。第一轮 A–G（见 §0–§11）；第二轮 4 点优化见 §12
> **修订**：v5。v3 记录第一轮实施；v4 追加第二轮：左栏搜索+新建、每助手独立会话（**含系统库迁移 v47**）、输入框样式；v5 确定左栏宽度为 200px（曾试 120px，实测过挤后回退）
> **迁移提示**：第二轮新增系统库迁移 **v47**（`sys_agent_threads.agent_id`）。按既有一贯要求，**代码只定义迁移，不代为执行 DDL**；需要你手动执行，或以正常模式（`TIMER_NO_RUN=0`）启动服务由启动流程应用。以 `TIMER_NO_RUN=1` 纯 API 模式运行不会应用迁移。

## 实施结果速览

| 阶段 | 状态 | 产出 |
| --- | --- | --- |
| A 逐页实测 | **部分** | 用代码审计替代了浏览器逐页点击（见 §9）；首条消息缺陷由代码路径确证，见 §10 第 1 项 |
| B favicon | 完成 | `ui/public/favicon.svg` + `ui/index.html` 一行 `<link>` |
| C 空数据不报错 | 完成 | `ApiError` 保留 status/code；`utils/api-errors.ts`；`useLoadState.emptyOn`；KV 面板与系统项目页；后端新错误码 `kv_store_not_found` |
| D 云助手布局 | 完成 | 左栏收窄；头部「⋯」菜单；主题色图标；移除两个禁用按钮；去 subtitle |
| E 提示词欢迎页 | 完成 | `AgentWelcome.vue` 挂在 `ConversationView` 的 `empty` 插槽；`focus()` 贯通 |
| F 行内确认 + 折叠 | 完成 | `AgentConfirmCard.vue`；`AgentToolCard` 成功折叠；`MessageScroller` 去掉 480px 上限 |
| G 测试与文档 | 完成 | 见下方验证结果 |
| **第二轮 1** 左栏顶部搜索 + 新建 | 完成 | `AgentListPanel` 顶部「搜索框 + 清除 + 新建图标」；按名称/描述过滤 |
| **第二轮 2** 每个助手独立会话 | 完成 | 迁移 v47 + `agent_id` 贯通（store / DTO / handler / 前端 API / 页面） |
| **第二轮 3** 输入框垂直居中 + 发送按钮主题色 | 完成 | `AiChatComposer`：`items-center`、发送按钮 `bg-primary`；删掉零引用的 `--composer-send-bg` |
| **第二轮 4** 左栏宽度定为 200px | 完成 | `md:grid-cols-[200px_minmax(0,1fr)]`（曾试 120px，实测过挤后回退） |

**验证结果（实测）**：

| 检查 | 结果 |
| --- | --- |
| `cd ui && yarn test` | **107 文件 / 695 用例全部通过** |
| 覆盖率 | statements 99.54% / branches 95.11% / **functions 95.16%**（达标）。初始基线 functions **94.88%（未达标）** |
| `cd ui && yarn build` | 通过 |
| `go build ./internal/... ./cmd/...` | 通过 |
| `go test ./...` | 全部通过（含新增 `TestAgentThreadsFilteredByAgent`） |

## 0. 需求与结论

| # | 需求 | 现状（证据） | 结论 |
| --- | --- | --- | --- |
| 1 | 页面没有浏览器图标 | `ui/index.html` 没有 `<link rel="icon">`，`ui/` 下没有 `public/` 目录；`internal/web/web.go:75` 对不存在的 `/favicon.ico` 回退返回 `index.html`（200 HTML） | 加 `ui/public/favicon.svg` + 一行 `<link>`；不改 `vite.config.ts` |
| 2 | `project has no key-value store` 应按空数据处理 | 后端两处返回 404 `kv_not_found`（`kv_handler.go:68` 系统项目、`:78` 只读实例缺 KV catalog）；`services/http.ts` 拦截器把错误压成 `new Error(message)`，**丢掉 HTTP status 与业务 code**；`useLoadState` 只要抛错就 `toast.error` + `showError` | 前端保留 status/code + `useLoadState` 支持「空资源错误」；系统项目直接不发请求 |
| 3 | 云助手重点优化 | 见 §3 | 左栏变窄、主题色图标、提示词、行内确认、减少操作 |

### 0.1 核实结论

| 结论 | 证据 |
| --- | --- |
| **改错误码是安全的**：`kv_not_found` 在 UI、测试、JS SDK 里**零引用**，只有后端 3 处 + 文档 1 处 | `grep -rn kv_not_found ui/src ui/tests packages docs` 只命中 `docs/data/kv.md:208` |
| **skills v2 的 P0 已落地**：委托白名单已改为路由表驱动（`router.go` 49 处 `delegable(`，`delegationAllowedResources` 已删除），`go test ./internal/api/ -run TestDelegation_RouteTable` **通过** | 上一版遗留的「`@云函数` 可能 403」不成立，§3.5 的提示词可以放心用 `@` 技能 |
| **会话首条消息确实看不到回复** | `settleThread` 只在 `resetThread` 被调用；新会话 `runThread` 返回 `false` → `showEmpty=true` → 渲染 `#empty` 插槽，`ConversationView` 从未挂载；`onSend` 只推数据不改 `hasData`。**已修**，见 §3.5 |

## 1. 浏览器图标

- `ui/public/favicon.svg`：圆角方块，背景 `#2563eb`（= `style.css:65` 的 `--primary` 浅色值），白色衬线 `S`，与 `DefaultLayout.vue:6-8` 的品牌块一致。
- `ui/index.html` 的 `<head>` 加 `<link rel="icon" type="image/svg+xml" href="/favicon.svg" />`。首页、控制台、文档站共用。
- Vite 默认 `publicDir=public`，构建时复制到 `ui/dist` 根，`build.sh` 同步到 `internal/web/dist`；`web.go:70` 只对「无扩展名或空路径」走 SPA fallback，带 `.svg` 正常返回文件。
- 遗留：SVG 用 `<text font-family="serif">`，各系统字形略有差异。要完全一致可把 `S` 转成 path。

## 2. 空数据不报「加载失败」

### 2.1 原链路

```
GET kv (系统项目)  → 404 {error:{code:"kv_not_found", message:"project has no key-value store"}}
http.ts 拦截器     → Promise.reject(new Error(msg))          // status / code 丢失
useLoadState.run   → catch → toast.error(msg) + error=msg   // 进入 showError
SbAsyncRegion      → SbErrorState「加载失败 + 重试」
```

顺带发现并已顺带生效：`AgentScheduleModal.vue:273` 判断 `e.status === 409`，此前错误对象上从没有 `status`，该分支恒不成立；现在能真正命中。

### 2.2 前端改动（已实施）

1. **`services/types.ts`**：新增 `interface ApiError extends Error { status?: number; code?: string }`。
2. **`services/http.ts`**：新增私有 `apiError(message, status, code)`；HTML 响应分支与错误拦截器的三条 reject 路径全部改用它，`message` 语义不变。
3. **`utils/api-errors.ts`（新）**：`EMPTY_RESOURCE_CODES = ['kv_store_not_found']` + `isEmptyResourceError(e)`：`status === 404` 且 code 在表内。
4. **`composables/useLoadState.ts`**：`UseLoadStateOptions.emptyOn?: (e: unknown) => boolean`。`catch` 命中时 `hasData=false; error=''`，不 toast，由 `finally` 落到 `settled + !hasData` → `showEmpty`。默认不传，行为不变。
5. **`components/databases/kv/KvPanel.vue`**：传 `emptyOn: isEmptyResourceError`。
6. **`pages/KeyValue.vue`**：`isAdmin` 时不渲染 `KvPanel`、不发请求，改为 `SbEmptyState`（`pages.keyValue.systemTitle/systemDesc`），刷新按钮也隐藏。
7. KV 详情里的五个编辑器未动：那是对具体 key 的读取，404 表示 key 不存在，应保持报错。

### 2.3 后端改动（已实施）

- `kv_handler.go:68,78` 改用新码 `kv_store_not_found`，状态码保持 404；`kv_not_found` 只留给 key 不存在（`error.go:242`）。
- `docs/data/kv.md` 错误表拆成两行。
- `kv_handler_test.go` 的 `TestKVSystemProjectRejected` 改断言新码。
- 已核实无需改 SDK / UI（§0.1）。

### 2.4 不改的

- `no_active_version`（`gofunction_handler.go:675` 409、`cronjob_handler.go:489` 400）是调用/创建动作的失败，保持报错。
- 503 类（`readonly_instance`、`sandbox_unavailable`、调度器未运行）是真实不可用状态，保持报错。
- 顺带记录、本计划不修：`cronjob_handler.go:486,499`、`agent_schedule_handler.go:147,161,243` 用 400 表达「目标不存在」。

## 3. 云助手

### 3.1 现状对照（改动前）

| 区域 | 改动前 |
| --- | --- |
| 整体布局 | `md:grid-cols-[280px_minmax(0,1fr)]`，右栏 `Card min-h-[560px]`；顶部一行长 subtitle |
| 左栏 | 带标题/刷新/新建的 `Card`；每条含 图标、名称、「内置」徽标、模块徽标、两行描述、定时摘要、三个操作按钮 |
| 消息区 | `MessageScroller.vue:5` 写死 `max-h-[480px]` |
| 图标颜色 | `bg-muted text-muted-foreground`（灰色） |
| 空会话 | `SbAsyncRegion` 的 `#empty` → `SbEmptyState` 一段说明文字 |
| 输入区 | 左侧**禁用**的「+」、右侧**禁用**的麦克风 |
| 破坏性确认 | 受控 `ConfirmAction`，**全屏模态**，遮住对话 |
| 工具调用 | 成功的也整块展开 |

### 3.2 目标形态（已实现）

```
┌ 200px ─────────┬──────────────────────────────────────────────┐
│ ◉ 通用助手     │  @通用助手   [会话 ▾] [＋]              [⋯]    │
│   跨库/对象/日志│ ───────────────────────────────────────────── │
│ ◉ Database     │                                              │
│   只读查询 …   │      你好，我是通用助手，可以直接让我做事        │
│ ◉ S3 / Logs    │      ┌──────────────────────────────────┐    │
│                │      │ 列出项目里所有数据库和集合           │    │
│                │      │ …（共 5 条，点一下填进输入框）       │    │
│ [＋ 新建助手]  │  ╭────────────────────────────────────╮ ➤   │
└────────────────┴──╰ 给 通用助手 发消息，@ 调用技能 ────╯────┘
```

- 右栏高度：`h-[calc(100svh-var(--header-height)-6rem)] min-h-[520px]`，消息区 `flex-1` 内部滚动，输入框贴底。
- 状态小字移到输入框上方，仅运行时出现。

### 3.3 左栏变窄（已实施）

**口径澄清**：需求是「比现在窄」。参考图里左栏约占整宽 20%（1080px 图内约 222px，折算到 1440px ≈ 300px），**绝对宽度未必比现在的 280px 窄**——它看起来紧凑，是因为**行高只有约 56px**，而我们是约 130px。

所以决定性改动是**行密度**：

- 列宽 `280px → 200px`（可调，若偏挤可放宽到 220–240px，不影响其余部分）。
- 行高 `~130px → ~56px`：去掉描述第二行、徽标行、定时摘要行、操作按钮行。
- `<md` 时左栏改为横向滚动条。
- `AgentListPanel.vue`：去掉 `Card` 外壳与刷新按钮；底部整宽「＋ 新建助手」；加载失败走 `SbAsyncRegion` 的重试。
- `AgentCard.vue`：只留 主题色图标 + 名称 + 一行描述（`truncate`）；内置助手用一个小圆点（`title`/`aria-label` 保留原文案）；定时任务改为一个时钟图标。
  - **保留** `aria-label` / `aria-pressed` / `data-agent-icon`（`nav-icons.test.ts` 直接断言）。
- **编辑 / 定时 / 删除 / 刷新** 移到右栏头部「⋯」（`Popover`，因为 `dropdown-menu` 已下线不得重引）。

### 3.4 图标用主题色（已实施）

- 助手头像与欢迎页图标统一 `bg-primary/10 text-primary`（浅色 `#2563eb`、深色 `#60a5fa`，随主题变）。
- 用户头像保持 `border-primary text-primary`。
- 侧栏导航图标不动（决策 D1）。

### 3.5 提示词（已实施）

**交互**：选中助手且当前会话为空时展示该助手的提示词；点击 = 写入 `draft` 并聚焦输入框（光标到末尾），**不自动发送**，回车才执行。

**数据**：`constants/agent-prompts.ts` 登记 key，文案在 `locales` 的 `pages.agents.prompts.<module>.<n>`。按 module 取，未登记回落 `general`。导出 `promptKeysForModule` / `promptSetForModule` / `promptI18nKeys`。

**通用助手（5 条）**：列库与集合 / 最近 1 小时错误按类型汇总 / 最大的 10 个对象 / `@键值` 写入并读回 / `@云函数` 创建并调用。
其余模块各 4–5 条（database / s3 / logs / sandbox）。

**关键修正（§10 第 1 项）**：欢迎页**不放在 `SbAsyncRegion` 的 `#empty`**，而是：

- `SbAsyncRegion` 传 **`:show-empty="false"`**（只负责骨架与错误）；
- 空态交给 `ConversationView.vue` 已有的 `empty` 插槽（`!historyLoading && !messages.length` 时渲染），默认内容是 `AgentWelcome`；消息一到自然切走。

这样一处改动同时修掉「首条消息看不见」——不需要 `settleThread(true)`，也不需要额外状态同步。

**输入框聚焦**：`AiChatComposer.vue` 新增 `focus()`（`focus()` + `setSelectionRange` 到末尾）并 `defineExpose`；`AgentComposer.vue` 透传；页面 `pickPrompt()` 调用。composer ref 类型为 `{ focus?: () => void }`，调用点用 `?.focus?.()`，避免实现变化时抛错（实施中发现过这个问题，见 §10 第 3 项）。

### 3.6 减少操作（已实施）

| # | 改动 |
| --- | --- |
| 1 | 破坏性确认 → **行内确认卡** `AgentConfirmCard.vue`：贴在该工具调用下，出现时聚焦「确认执行」，`Esc` 取消，**不用 Enter 确认**（避免与输入框发送冲突） |
| 2 | 非破坏性操作保持直接执行，UI 不新增确认 |
| 3 | 成功的工具调用折叠成一行「✓ 工具名 · 耗时 · 行数」（`data-tool-summary` 钩子）；**失败**与**待确认**默认展开 |
| 4 | 保持默认选中通用助手 |
| 5 | placeholder → 「给 {name} 发消息，`@` 调用技能」 |
| 6 | 移除禁用的「+」与麦克风（相应 i18n key 一并删除） |
| 7 | 会话区头部：会话下拉 + 「＋」图标按钮（`aria-label` 保留「新会话」） |
| 8 | 状态小字移到输入框上方 |
| 9 | `MessageScroller` 去掉 `max-h-[480px]`，改 `min-h-0 flex-1`；外层用 `100svh`（避开移动端地址栏吞高度） |
| 10 | 首条消息随 §3.5 的结构改动一并修好 |

**例外说明**：`ui/AGENTS.md` 规定确认用 `ConfirmAction`。行内确认卡是对「对话中运行确认」的**有意例外**（决策 D5），已在 `ui/AGENTS.md` 记明；删除助手/会话仍用 `ConfirmAction`。

**不做**：「本会话总是允许」之类的自动放行——会削弱 skills v2 里「确认人 = 发起人」的设计（已落地，`systemdb/agents.go:83` 的 `InitiatorID`），需单独评审。

### 3.7 选中助手时会话不新建（决策 D4）

thread 不与助手绑定，左栏选助手只改「默认被点名对象」。会话为空则展示该助手提示词；会话非空则仅切换目标。

### 3.8 头部布局（已实施，实施时比计划多补了会话操作）

「⋯」菜单分两组，并由一个 `border` 分隔：

- **当前助手**：编辑助手 / 定时执行（带摘要）/ 删除（内置助手不显示）
- **当前会话**：重命名会话 / 刷新助手列表 / 删除会话

**修正（§10 第 2 项）**：计划原本只说「删除会话放进菜单」，实施时发现 `renameThread` 与 `removeThread` 会因此**变成零调用点的死代码**——`ThreadSwitcher` 的 `@remove` 被移除后这两个函数没人再调用。因此补上「重命名会话」并新增 `SbModal` 重命名弹窗，让两条能力从菜单完整可及。

## 4. 文件清单（实施后）

| 文件 | 动作 |
| --- | --- |
| `ui/public/favicon.svg` | 新增 |
| `ui/index.html` | 加 `<link rel="icon">` |
| `ui/src/services/types.ts`、`services/http.ts` | `ApiError` + `apiError()`；拦截器带 status/code |
| `ui/src/utils/api-errors.ts` | 新增 |
| `ui/src/composables/useLoadState.ts` | `emptyOn` 选项 |
| `ui/src/components/databases/kv/KvPanel.vue`、`ui/src/pages/KeyValue.vue` | 空资源处理、系统项目空态 |
| `internal/api/kv_handler.go`、`internal/api/kv_handler_test.go`、`docs/data/kv.md` | 新错误码 `kv_store_not_found` |
| `ui/src/pages/AgentManager.vue` | 栅格、头部「⋯」（两组）、`:show-empty="false"`、`pickPrompt`、重命名弹窗、去 subtitle |
| `ui/src/components/agent/AgentListPanel.vue`、`AgentCard.vue` | 紧凑左栏、主题色图标 |
| `ui/src/components/agent/AgentWelcome.vue`、`AgentConfirmCard.vue` | 新增 |
| `ui/src/components/agent/ConversationView.vue` | 空态插槽、行内确认卡、头像主题色 |
| `ui/src/components/agent/AgentToolCard.vue` | 成功折叠 |
| `ui/src/components/agent/ThreadSwitcher.vue` | 精简为下拉 + 「＋」 |
| `ui/src/components/agent/AgentComposer.vue` | 透传 `focus` |
| `ui/src/components/chat/MessageScroller.vue` | 去掉 `max-h-[480px]` |
| `ui/src/components/ai/AiChatComposer.vue` | 移除禁用按钮、`focus()`、新 placeholder |
| `ui/src/constants/agent-prompts.ts` | 新增 |
| `ui/src/locales/zh-CN.ts`、`en-US.ts` | 提示词、欢迎页、确认卡、菜单、空态文案；删死 key |
| `ui/AGENTS.md` | 空态归属、`emptyOn`、行内确认例外 |
| `docs/workbench/agent.md` | 基本流程改为「默认已选中助手 → 点提示词或直接输入」 |
| 测试 | `agent-welcome-confirm.test.ts`（新）、`agent-components`、`agent-split-components`、`agent-toolcard-variants`、`agent-skills-stream`、`agent-composer`、`AgentManager`、`thread-switcher-time`、`template-inline-handlers`、`loading-surfaces`、`chat-components`、`KeyValue`、`http`、`useLoadState` |

**删除的死 key**：`pages.agents.subtitle`、`pages.agents.emptyConversation`、`components.conversation.emptyDesc`、`components.conversation.confirmDestructive`、`components.aiComposer.attachDisabled/attachAria/voiceDisabled/voiceAria`。

## 5. 测试（实施后）

**基线（改动前实测）**：functions **94.88%**，**运行前就是红的**（`AGENTS.md` 写的 94.24% 已过时）。

**结果（改动后实测）**：107 文件 / **692 用例全部通过**；statements 99.54% / branches 95.14% / **functions 95.15%（达标）**。

**本次改写的既有测试**（原因都是计划预告的契约变化）：

| 测试 | 原因 |
| --- | --- |
| `agent-components.test.ts` | `AgentCard` 断言改为紧凑条目契约；SQL 结果先展开 |
| `agent-split-components.test.ts` | `AgentListPanel` 不再转发 edit/schedule/remove，也没有刷新按钮 |
| `AgentManager.test.ts` | 「编辑助手/定时执行/删除」改从菜单进入；去 subtitle 断言；新增首条消息、提示词、菜单、`reloadThread` 用例 |
| `agent-toolcard-variants.test.ts` | 成功态折叠，断言前先展开；`ThreadSwitcher` 删除已移走 |
| `agent-skills-stream.test.ts` | 加 `expanded()` helper；确认弹窗改断言行内卡片 |
| `thread-switcher-time.test.ts`、`template-inline-handlers.test.ts` | 新建按钮改按 `aria-label="新会话"` 定位 |
| `chat-components.test.ts` | `MessageScroller` 选择器去掉高度类 |
| `loading-surfaces.test.ts` | 空态文案改为欢迎页问候语 |
| `KeyValue.test.ts` | 系统项目下不再渲染 `KvPanel` |

**新增测试**：

- `agent-welcome-confirm.test.ts`：`AgentWelcome` 的模块回落 / 点击 emit / 两棵树文案齐全；`promptI18nKeys` 等常量；`isEmptyResourceError` 的白名单与非命中分支；`AgentConfirmCard` 的 argv、空 argv、`Esc`
- `AgentManager.test.ts`：**空会话首条消息立即可见**（锁 §10 第 1 项）、点提示词写入但不发送、菜单四条路径、会话重命名/删除、`reloadThread` 两条分支
- `AgentComposer.test.ts`：`focus()` 真的把焦点给到 textarea
- `useLoadState.test.ts`：`emptyOn` 命中 → 空态无 toast；未命中 → 仍走错误
- `http.test.ts`：reject 的错误带 `status` 与 `code`；无 JSON 信封时不伪造 code

**覆盖率处理原则**：本次只补自己引入的缺口。历史缺口（`SchemaPanel`、`KvCreateKeyModal`、`UserFormModal`、`StoragePanel` 等 20 多个文件的模板内联处理器）**未动**——它们与本次改动无关，凑阈值式的补测会污染改动面。

## 6. 实施顺序（实际）

A（代码审计替代）→ B → C → D → E → F → G。E 与 F 都改 `ConversationView.vue`，按计划建议由同一人顺序完成。

## 7. 验收清单（逐项实际核对）

| # | 项 | 结果 |
| --- | --- | --- |
| 1 | 标签页图标；`/favicon.svg` 返回 `image/svg+xml` | 产物已生成并引用（`ui/dist/index.html:6`）；**HTTP 响应未在本机起服务实测** |
| 2 | `sb-admin` 打开 Key-Value：不发 KV 请求、显示空态、无 toast | 已实现；`KeyValue.test.ts` 断言 `KvPanel` 不渲染 + 空态文案 |
| 3 | 普通项目 KV 无 key → 空态；真失败 → 错误可重试 | `emptyOn` 只放行登记码；`useLoadState` 两条新用例覆盖 |
| 4 | 左栏 200px、条目约 56px；`<md` 横向滚动；浅/深色均主题色 | 已实现；**视觉密度需人工过一眼** |
| 5 | 空会话 5 条提示词；点击填入并聚焦、未发送；回车发送 | `AgentManager.test.ts` + `AgentComposer.test.ts` 覆盖 |
| 6 | **新建会话直接发第一条消息，回复立即可见** | `AgentManager.test.ts` 专项用例覆盖 |
| 7 | 行内确认卡；确认/取消；无全屏模态；`Esc` | `agent-skills-stream` + `agent-welcome-confirm` 覆盖 |
| 8 | 成功折叠；失败/待确认展开 | `AgentToolCard` 相关用例覆盖 |
| 9 | 消息区随视口；长对话内部滚动；移动端高度 | 已实现；**需人工过一眼** |
| 10 | `yarn build` + `yarn test`（functions ≥95%）+ Go 编译与测试 | 全部通过（见速览表） |
| 11 | 两棵树 key 一致；无新依赖；未改配置文件 | `i18n.test.ts` 通过；`package.json` / `tsconfig.json` / `vite.config.ts` 未动 |

**未完成的两项**：第 1 项的 HTTP 响应实测、第 4/9 项的视觉确认。都需要起服务或肉眼看，属于人工验收，不在自动测试范围内。

## 8. 决策记录（已确认，2026-10-12）

| # | 决策 | 落地位置 |
| --- | --- | --- |
| D1 | 「icon 为主题色」只指云助手里的图标，侧栏导航图标不动 | §3.4 |
| D2 | KV 的项目级 404 改用新错误码 `kv_store_not_found`；`kv_not_found` 只留给 key 不存在 | §2.3 |
| D3 | 移除输入框里禁用的「+」和麦克风 | §3.6 #6 |
| D4 | 选助手不新建会话，沿用「会话与助手解耦」模型 | §3.7 |
| D5 | 行内确认卡取代模态确认，作为 `ConfirmAction` 规则的例外（删除助手/会话仍用模态） | §3.6 #1；写入 `ui/AGENTS.md` |

## 9. 逐页实测（阶段 A 的替代做法）

计划要求浏览器逐页实测。本机没有可用的浏览器自动化（`glance` / `ground` / `detect` 均不存在），所以改用**代码审计 + 测试断言**的方式核对，结论如下：

| 页面/接口 | 空资源时的后端行为 | 前端表现 | 处理 |
| --- | --- | --- | --- |
| KV（`POST /kv`） | 404 `kv_store_not_found` | 原为红色错误块 | 已修（§2.2） |
| KV（系统项目） | 404 | 原为红色错误块 | 已修：不发请求，直接空态 |
| s3（未配置） | 路由不挂载 → SPA fallback 返回 **HTML** | `http.ts` 的 `text/html` 分支 → 「接口不存在或返回了 HTML 页面」 | **未处理**：这是「实例未配置存储」的部署态，不是空数据 |
| logs（未配置） | 同上，路由不挂载 | 同上 | 未处理，同上 |
| sandboxes（未配置） | `capabilities.available = false` | 已有专门空态（`pages.sandboxes.notConfigured*`） | 无需改 |
| databases / gofunctions / cron-jobs | 空数据返回 200 + 空数组 | 正常走空态 | 无需改 |
| dashboard / users | 无此类 404 | — | 无需改 |
| agents（`sb-admin`） | 见下 | — | 仅记录 |

**补充关注点（未扩范围）**：云助手页在 `sb-admin` 项目下没有 `isAdmin` 分支（`AgentManager.vue` 里搜不到 `isAdmin`）。在那里让助手做 `@键值` 相关操作会撞上同一个 404，但请求是 SSE 流、走 `agent-errors.ts` 的另一条路径，不受 §2.2 影响。是否限制系统项目用云助手属独立议题。

**s3/logs 的 HTML 回退**属于本次决策 D2 没覆盖的场景：它需要的是「实例级未配置」的前端识别，与「项目级资源未建立」是两回事。建议单独排一个小计划。

## 10. 对计划自身的修正（执行中发现）

| # | 计划原内容 | 问题 | 处理 |
| --- | --- | --- | --- |
| 1 | 欢迎页放在 `SbAsyncRegion` 的 `#empty` 插槽 | 那正是「首条消息不渲染」的同一条路径：`SbAsyncRegion` 空态接管后 `ConversationView` 根本不挂载，点提示词发送后依然看不到回复 | 改为 `:show-empty="false"` + `ConversationView` 的 `empty` 插槽（§3.5）。一处改动同时修两个问题，`settleThread(true)` 也不需要了 |
| 2 | 「删除会话放进『⋯』菜单」 | 只说删除，没意识到 `renameThread` / `removeThread` 会因此双双变成零调用点的死代码 | 菜单补「重命名会话」+ `SbModal` 弹窗，两条能力完整可及（§3.8） |
| 3 | `composer` ref 类型 `{ focus: () => void }`，直接 `composer.value?.focus()` | 测试里 composer 被桩替换时 `focus is not a function`，抛在事件处理器里（Vitest 报 unhandled error） | 改为 `{ focus?: () => void }` + `?.focus?.()` |
| 4 | 覆盖率「拉回 ≥95%」但未说明历史缺口 | 未覆盖函数共 58 个，大部分是本次改动之前就存在的模板内联处理器（SchemaPanel / UserFormModal / StoragePanel 等） | §5 明确「只补自己引入的缺口」，不动历史项。实际把 functions 从 94.88% 提到 95.15% |

## 11. 遗留

1. **favicon 的 HTTP 响应未实测**：需起服务后 `curl -I /favicon.svg` 确认 `Content-Type: image/svg+xml`。
2. **视觉密度未人工确认**：左栏 200px / 条目 56px 是按比例推算的，若偏挤可放宽到 220–240px，不影响其余实现。
3. **s3 / logs 未配置时的 HTML 回退**（§9）：前端表现是「接口不存在或返回了 HTML 页面」，语义不准。建议单独排计划。
4. **Cloud Agent 在 `sb-admin` 下的行为**（§9）：是否限制，属独立议题。
5. **多工具并发时的行内确认卡**：`useAgentConversation.ts` 收到 `confirmation_required` 会设 `confirmation`（单值），多个工具同时待确认时的表现**未验证**，需构造多工具场景确认。
6. **`AgentToolCard` 折叠 + `details` 双层的交互**：折叠行展开后内部还有参数/结果的 `details`，两层的展开手感未人工确认。

---

## 12. 第二轮：四点优化（2026-10-12）

### 12.1 需求 #1 左栏顶部「搜索框 + 搜索 icon + 新建 icon」

**改动**：`components/agent/AgentListPanel.vue`

- 顶部固定一行：`[搜索框（内含搜索图标）] [新建图标按钮]`。
- 搜索框用 `Input` + 左侧内嵌 `SearchIcon`；`Input` 组件本身**不支持 `clearable`**（`ui/input/Input.vue` 只接受 `defaultValue`/`modelValue`/`class`），而 `ui/` 视为库代码不改，所以清除按钮由本组件自绘（`XIcon`，仅在有关键字时出现）。
- 过滤在左栏内部完成（按 `name` / `description`，大小写不敏感），**不上报宿主**：助手数量是每项目个位数，没必要进 URL 或让页面持有。
- 搜索无结果时不复用 `SbAsyncRegion` 的空态（那是「一个助手都没有」），而是单独一行提示。为此 `:show-empty` 加了 `&& !keyword.trim()`。
- 底部原「＋ 新建助手」整宽按钮**已移除**（新建现在在顶部）。
- 文案：`pages.agents.searchPlaceholder` / `searchClear` / `searchNoMatch`（两棵树都补）。

**注**：此前「刷新助手列表」只在头部「⋯」菜单里，本次未改；左栏不再有任何刷新入口。

### 12.2 需求 #2 每个助手一份独立会话

这是本轮唯一**触及系统库结构**的改动。

**数据模型**

- `sys_agent_threads` 原本**没有** `agent_id` 列（`sys_agent_messages` 有），所有助手共用一份会话列表。
- 新增迁移 **v47**：`ALTER TABLE sys_agent_threads ADD COLUMN agent_id VARCHAR`（DuckLake 不支持 `ADD COLUMN NOT NULL`，用可空列；沿用 v46 `initiator_id` 的写法）。
- **空串 = 未绑定**（v47 之前的历史数据）。读取一律 `COALESCE(agent_id, '')`；`agent_id` 为空时列表**不过滤**，兼容历史调用与既有测试。

**后端**

| 位置 | 改动 |
| --- | --- |
| `internal/systemdb/migrate.go` | 新增 v47 |
| `internal/systemdb/agents.go` | `AgentThread` 增加 `AgentID`；`CreateAgentThread` 写入；`ListAgentThreads` / `PageAgentThreads` 增加 `agentID` 参数（非空才过滤）；三处 `SELECT` 加 `COALESCE(agent_id, '')` |
| `internal/api/agent_handler.go` | `threadDTO` 增加 `agent_id`；`toThreadDTO` 带上；`ListThreads` 读 `?agent_id=`；`CreateThread` 接收 `agent_id` 并**校验该助手存在**（不存在返回 404 `agent not found`） |

**前端**

| 位置 | 改动 |
| --- | --- |
| `services/types.ts` | `AgentThread` 增加 `agent_id?: string` |
| `services/http-api.ts` | `toAgentThread` 映射；`agentThreads.create(projectId, title, agentId)`；`list` / `page` 接受 `agentId` 并作为 query 参数 |
| `pages/AgentManager.vue` | 新增 `selectAgent(id)`：切换助手 → 清空当前会话 → 拉该助手的列表；新增 `loadThreads()`（取代原 `ensureThread`），列表为空时为该助手建一个会话 |

**两处必须一起改的连带项（实施中发现）**

1. **`bootstrap` 必须串行**：`loadThreads` 依赖 `activeId`，而 `activeId` 由 `loadAgents` 决定。原实现是 `Promise.all([loadAgents(), ensureThread(), ...])` 并行，改为 `loadAgents()` → `loadThreads()` 串行（调度与模型候选仍并行）。
2. **`ensureThread` 已删除**：它原本与新的 `loadThreads` 职责重合，留着就是两份会分叉的取数逻辑。测试里的调用点同步改为 `loadThreads`。

**其他连带处理**

- `removeAgent`：删掉的是当前助手时，清空会话与消息，`loadAgents` 之后自动切到新选中的助手并 `loadThreads`。
- `resetThread`：新建会话带上 `activeId`，归属当前助手。
- 取数期间切换助手的竞态：`loadThreads` 在写 `threads`/`threadId` 前比较 `activeId` 是否仍是发起时的值，不一致就丢弃结果。
- `reloadThread` / `onSend` 的 `ensureThread` 调用改为 `loadThreads`。

**迁移未由我执行**：按你的一贯要求，代码只定义迁移，**不代为运行 DDL**。需要你手动执行，或以正常模式（`TIMER_NO_RUN=0`）启动服务由启动流程应用。以 `TIMER_NO_RUN=1` 纯 API 模式运行不会应用迁移，此时 `agent_id` 列不存在，新代码的三处 `SELECT` 会失败——**这一点务必注意**。

### 12.3 需求 #3 输入框垂直居中 + 发送按钮主题色

**改动**：`components/ai/AiChatComposer.vue`

- 容器 `items-end` → **`items-center`**（单行时文字垂直居中；多行时随 textarea 增高仍居中）。
- textarea `py-1 leading-normal` → **`py-0 leading-6`**：`leading-6`(24px) 与 `min-h-6` 一致，单行时文字与 placeholder 都在盒子中线。
- 发送按钮去掉内联 `--composer-send-bg` 样式，改用 **`bg-primary text-primary-foreground hover:bg-primary/90`**，随主题色变化（浅色 `#2563eb` / 深色 `#60a5fa`）。
- 停止按钮保持 `bg-destructive`（运行中/可发送是两种语义，不该同色）。
- **顺带清理**：`--composer-send-bg` 因改动变成零引用，已从 `style.css` 的浅色与深色两块删除。

### 12.4 需求 #4 左栏宽度：定为 200px

`pages/AgentManager.vue` 的 `md:grid-cols-[...]`：第一轮从 280px 收到 200px，第二轮一度试到 **120px**，实测后**确认回到 200px**。

**决策依据**：120px 下 `AgentCard` 的「图标 + 名称 + 一行描述」过挤（图标 28px + gap 10px + 左右 padding 20px，留给文字约 60px），搜索框也只剩约 70px 可用宽度。200px 是去掉卡片嵌套、徽标行与操作按钮行之后的合适值；行高约 56px 的紧凑感来自**行密度**，不需要再压列宽。

- 名称与描述都用 `truncate`，空间不足时截断而不是换行，不会撑破栅格。
- 该值只影响这一处栅格定义，后续若要微调（如 220–240px 配合更宽的搜索框），改这一个数字即可。

### 12.5 第二轮的文件清单

| 文件 | 动作 |
| --- | --- |
| `internal/systemdb/migrate.go` | 新增迁移 v47 |
| `internal/systemdb/agents.go` | `AgentID` 字段 + 3 处 SQL + 2 个函数签名 |
| `internal/systemdb/agent_history_test.go` | 调用点同步；新增 `TestAgentThreadsFilteredByAgent` |
| `internal/api/agent_handler.go` | DTO / 列表过滤 / 创建校验 |
| `ui/src/services/types.ts`、`services/http-api.ts` | `agent_id` 贯通 |
| `ui/src/pages/AgentManager.vue` | `selectAgent` / `loadThreads` / bootstrap 串行 / 栅格 200px / 菜单会话项 |
| `ui/src/components/agent/AgentListPanel.vue` | 顶部搜索 + 新建图标 |
| `ui/src/components/ai/AiChatComposer.vue` | 垂直居中 + 主题色发送按钮 |
| `ui/src/style.css` | 删除零引用的 `--composer-send-bg` |
| `ui/src/locales/zh-CN.ts`、`en-US.ts` | 搜索相关 3 个 key |

### 12.6 第二轮的测试

**新增/改写**：

- `agent-split-components.test.ts`：顶部新建按钮改按 `aria-label="新建"` 定位；新增「搜索按名称与描述过滤条目」用例（含无结果提示与清除按钮）。
- `AgentManager.test.ts`：
  - 「每个助手独立会话」：`page` 按 `agent_id` 返回不同列表，断言切助手后 `threadId`/`threads` 都换、且请求带 `agent_id`；重复点同一助手不重复请求。
  - 「助手首个会话带自己的 agent_id」：无会话时 `create` 收到 `agentId`。
  - `reloadThread` 用例改走 `loadThreads`。
- `internal/systemdb/agent_history_test.go`：`TestAgentThreadsFilteredByAgent` 覆盖 nil store、按助手过滤、不过滤返回全部（含未绑定历史数据）、分页过滤、单条取回带 `agent_id`。

**结果**：107 文件 / **695 用例全部通过**（较第一轮 +3）；functions **95.16%**。

### 12.7 第二轮遗留

1. **迁移 v47 未应用**（见 §12.2）。未应用时新代码不可用，这是硬依赖。
2. **左栏宽度已确认**：试过 120px，实测过挤，回到 200px（见 §12.4）。
3. **未处理的接口语义**：`GET /agent-threads` 不带 `agent_id` 现在返回**全部助手**的会话（兼容历史行为）。前端总是带 `agent_id`，所以实际不会命中；但这是一个「参数可选即不过滤」的隐含约定，未来若有人不带参数调用，会拿到混合列表。
4. **`ThreadSwitcher` 的下拉只显示当前助手的会话**（数据源已过滤），符合新模型；但如果用户想在同一下拉里跨助手切换会话，现在是做不到的——需要先切助手。

---

## 13. 第三轮：对话加载动效与实时输出（2026-10-13）

> 本节是对 §12 已完成工作的后续问题记录。**前端修复与自动化回归已实施**；尚未连接用户实际模型、浏览器或部署代理，不能将自动化测试等同于生产链路验收。修复无需迁移或数据库 DDL。

### 13.1 根因与边界

| 现象 | 代码证据 | 结论 |
| --- | --- | --- |
| 对话中的 loading 图标静止 | `ui/src/components/agent/ConversationView.vue` 的助手头像由 `iconForAgentModule` 渲染，只带 `size-4`，不是加载图标；气泡内 `Spinner` 仅在「发送中 + 最后一条为助手消息 + 无待确认」时出现，而 `ui/src/components/ui/spinner/Spinner.vue` 本身已有 `animate-spin`。`ui/src/style.css` 的 `prefers-reduced-motion: reduce` 会缩短动画至近乎静止。 | 若用户看到的是头像，当前确实没有忙碌态动效；若看到的是 `Spinner` 却不转，需核查 DOM 中的 `animate-spin`、计算样式与系统减少动画设置，不能直接归咎于缺少 CSS。待确认期间 `Spinner` 被隐藏，确认卡接管状态。 |
| 模型数据可能到达但最后才显示 | `AgentManager.vue` 以 `ref<ChatMsg[]>([])` 持有消息；`useAgentConversation.ts` 创建普通对象 `reply`，`messages.push(reply)` 后，`onToken`、`onThinking`、工具卡回调和错误回调仍修改**原始 `reply`**。Vue 读取数组项时取得的是代理对象，原对象的赋值不触发依赖；`phase` / 计时器 / `sending` 的其他更新才可能顺带重绘。此前 `plan/planv4.0/cloud-agent-optimization-plan.md` BUG-01 已明确记录同类问题，迁入 composable 后仍需核验。 | **前端响应式缺陷是静态代码可确证的优先修复项**；它能解释“网络已有 token、界面只在状态改变时成批出现”。不能仅凭代码断言用户这次的真实上游也在持续发 token。 |
| 后端 API 是否不支持流式 | `AgentManager.vue` → `useAgentConversation.start` 发 `stream: true`；`ui/src/services/http-api.ts` 以 `fetch` + `ReadableStream.getReader()` 解析 SSE，并将 `token/chunk` 交给 `onToken`。`internal/llmgateway/service.go` 的 `Stream` 调 `c.Stream`、返回 reader；`internal/gocloudagent/model.go` 消费 `NextDelta` 并送 Eino 流；`internal/gocloudagent/runtime.go` 的 `consumeAssistant` 逐块 `emit(token)`；`internal/api/agent_handler.go` 的 `streamRun` 对每帧写 SSE 并 `Flush()`。`internal/api/agent_e2e_stream_test.go` 使用 fake 上游逐块输出，断言至少 5 帧 token。 | **本仓库的接口及所测兼容上游支持流式**，不应改成轮询或新增聊天 API。真实供应商、其模型及代理是否逐块输出，仍需同部署路径验证。 |
| 有哪些后端/传输层例外 | `internal/gocloudagent/model.go` 在文本工具协议中暂存可能为 `TOOL_CALL` 的前缀，确认普通文本后才发送；`m.client.Stream` 在创建 reader 前遇非上游结构化错误/取消/超时时，会退到 `m.client.Chat` 并包装成**单消息流**。工具执行阶段可能只有 `tool_progress`，没有正文 token；最终消息落库在 `StartRun` 返回后，不阻塞正常 SSE。`streamRun` 已 `Flush`，但代码无法证明部署代理未缓冲。 | 这些路径会导致首 token 晚或“单帧假流式”；若前端修复后仍延迟，再按实际帧时间定位，不盲改 Gateway。 |

### 13.2 修复顺序与实施位置

1. **先修确定的前端响应式问题**：在 `ui/src/composables/useAgentConversation.ts` 中将新建回复插入 `messages` 后，立即通过 `messages[messages.length - 1]` 取回数组内的响应式代理，或先 `reactive<ChatMsg>(...)` 再插入。所有 `onToken`、`onThinking`、工具卡、`onError`、`onEnd` 的更新都必须落到代理上；`handlers.onReply` 也传递同一代理对象。不要仅靠 `phase`/定时器变更触发重绘，避免再次引入 BUG-01。
2. **明确忙碌态图标**：只在最后一条助手消息运行中时，于 `ConversationView.vue` 的头像位置渲染已有的 `Spinner`（或为忙碌态使用等价 `animate-spin` 图标），结束后恢复模块图标；避免把非旋转对称的模块图标强行旋转。不要改 `src/components/ui/spinner/Spinner.vue` 的库风格。保留气泡内现有指示器或整合为单一忙碌指示器，避免重复；待确认时以确认卡为主。遵守 `prefers-reduced-motion`，不能靠强制覆盖无障碍设置制造动效。
3. **验证仍无增量时再排后端**：用 fake 上游固定间隔逐块发送，记录每跳首帧/帧间隔：Gateway `NextDelta` → 模型适配器 `sw.Send` → Runtime `emit(token)` → HTTP `Flush` → 浏览器 `reader.read` → DOM。只记录 run ID、时间、字节数和事件类型，**不记录 LLM 正文、密钥或 SQL 参数**。如果 `Stream` 退到 `Chat`，记录脱敏原因并评估是否应显式报错或提示非流式降级；如果卡在文本协议前缀，调整安全判定/暂存上限并补工具调用不误显测试；若后端已 Flush 而浏览器批量收到，查反向代理压缩/缓冲，并按需对 SSE 与续传响应配置禁缓冲，在实际部署路径复验。不要仅凭 fake 上游断言所有真实 provider 都能流式返回。
4. **流结束语义一并回归**：`ui/src/services/http-api.ts` 当前 `readSse()` 在 EOF 后会调用 `fireEnd()`，续传路径亦如此；若网络提前 EOF 而未收到 `end`，不应标成正常完成。实施阶段核查该分支并补中断/续传用例，确保失败可重试；不要将它与本次“正文不实时重绘”的确定根因混为一谈。

### 13.3 实施结果与验收（2026-10-13）

- `ui/src/composables/useAgentConversation.ts`：新回复先经 `reactive<ChatMsg>` 包装，再放入消息数组；逐帧回调直接修改代理。`ui/tests/useAgentConversation.test.ts` 新增真实 `ConversationView` 逐帧 DOM 用例：连续 3 个 token 各自 `nextTick` 后立即可见；再检查思考、工具卡增量与结束后指示器消失。测试在修复前已复现首帧不显示，修复后通过。
- `ui/src/components/agent/ConversationView.vue`：最后一条助手消息运行中，头像位置显示原有 `Spinner`，移除气泡内重复指示器；结束后恢复模块图标，待确认时由行内确认卡承担状态。`ui/tests/agent-components.test.ts` 覆盖三个状态。沿用 `style.css` 的 `prefers-reduced-motion`，未覆盖无障碍设置；减少动画时保留静态忙碌图标。
- `ui/src/services/http-api.ts`：初始流和续传流只有收到 `end` 帧才算完成，异常 EOF 尝试按 run ID 续传，无法续传则回调错误；错误文案复用双语 `error.streamFailed`。`ui/tests/http-api.test.ts` 增加提前 EOF 报错和提前 EOF 成功续传用例；既有 socket 故障续传测试仍通过。
- **已执行**：`cd ui && yarn test` 通过（statements 99.51%、branches 95.02%、functions 95.19%）；`cd ui && yarn build`、`go test ./...`、`go build ./internal/... ./cmd/...` 均通过。Go 服务端本次未改，保留原有 fake 上游多帧 E2E 测试。
- **仍待真实环境验收**：在实际模型与部署代理路径下，用浏览器开发者工具确认 SSE 多帧抵达时 DOM 同步增长；检查减少动画设置下的静态忙碌提示。只有再次复现“网络仍成批到达”时才执行 §13.2 第 3 项的 Gateway/代理分段定位；本轮未修改后端流式实现或配置。
