# ui/ 重构、精简与清理计划（planv6.8，simplify-code）

> **状态：✅ 已完成并关闭（2026-11-24 复核）。** v2 的 P0–P7（§8）、v3 余量复核（§8.3）与 v4 第二轮候选（§10）均已实施；A-16 / N-3、CV-2、AM-10 为有意保留（§8.3 / §10.2）。2026-11-24 门禁复核全部通过：`npx tsc --noEmit` 0 错误、`npx vitest run` 160 文件 / 1519 用例全绿、`yarn build` 通过。改动仍在工作区未提交；浏览器手工冒烟与 `internal/web/dist` 同步（由 `./build.sh` 负责）保持开口。
> 方法：`skills/simplify-code`，先走 R-branch（只读评估）。v1 只写本文件；v2：P0–P7 已执行（结果与偏差见 §8）；v3（2026-11-23）：补余量清单与复核门禁（§8.3），新增第二轮评估（§9）；v4（2026-11-23）：§9.1 候选已全部实施（§10）。
> 姊妹篇：`plan/planv6.8/internal-simplify-refactor-plan.md`（后端）。前一轮 UI 计划：`plan/planv6.7/ui-simplify-ux-plan.md`（S1–S8 / B / C / R 已落地，本计划不重复）。
> 目标：`ui/` **一处定义一个规则、页面只做编排、删掉没人用的代码**；对外可见行为（DOM、文案、请求序列、路由）保持不变，除非在 §2「需审批」中单列。

## 0. Scope 与基线

### 0.1 范围

| 项 | 内容 |
| --- | --- |
| 模式 | 只读评估；条目按 **apply**（等价且有测试兜底）/ **propose**（需审批：改配置、改外部契约、改文案树）/ **skip** 分级 |
| 范围 | `ui/src/` 业务代码（约 4.2 万行 `.vue/.ts`，其中 `locales/*` 4653 行、`services/` 4600 行、`pages/` 约 4800 行）与 `ui/tests/`（151 个文件） |
| 排除 | `src/components/ui/**`（fork 库代码，AGENTS.md 规定只增删不改风格）、`dist/`、`coverage/`、`node_modules/`、`src/docs/*` 正文 |
| 修订 | HEAD `dead32f`；`git status ui/` 干净（无在途改动） |
| 工具 | `rg`、`tsc --noEmit`、`vitest run`、自写只读脚本（未用 import、未用导出、未用 i18n key、生产未调用的 API 方法、组件引用图），脚本在 `.session_tmps/`，不入库 |

### 0.2 基线（改动前，已实际运行）

| 命令 | 结果 |
| --- | --- |
| `npx vitest run` | **151 files / 1399 tests 全部通过**（14.7s） |
| `npx tsc --noEmit` | **0 错误**（AGENTS.md 第 96 行「241 处 TS2307、不干净」已过时 → D-3） |
| 覆盖率阈值 | `vite.config.ts` 实际为 **90/90/90/90**；README / AGENTS.md 写 95% → D-3 |
| 未使用 import | 3 处（`McpServersPanel.vue: RefreshCwIcon`、`AgentManager.vue: Button, ConfirmAction`） |
| 生产零引用组件 | 3 个：`components/agent/AgentListPanel.vue`、`ThreadSwitcher.vue`，以及只被 AgentListPanel 使用的 `AgentCard.vue`（只被 6 个测试文件引用） |
| 零引用导出 | `types.ts` 6 个接口、`bi-chart.ts isChartType`；4 个「只给测试用」的导出（§1.7） |
| 生产未调用的 API 方法 | 12 个（§1.2 A-6） |
| 疑似未用 i18n key | 1953 个叶子中约 **107** 个既无字面引用也无动态前缀命中（§1.8） |
| 计划编号注释 | **105 个文件 / 282 处**（`planvX.Y`、`§N`、`AGC-*`、`BUG-*`、`DC-*`、`OPT-*`），`types.ts` 59、`AgentManager.vue` 29、`http-api-converters.ts` 15 |
| `toast.error(errorMessage(e, t(...)))` | 42 个文件 / 92 处（模式本身合理，见 §5 不改） |
| `encodeURIComponent` 手拼路径 | `http-api.ts` 35 处 `'/v1/projects/' + encodeURIComponent(projectId)`，另有 6 个 `xxxPath` 函数已存在但未统一 |
| `.DS_Store` | 6 个（已被根 `.gitignore` 忽略，未入库，仅本地清理） |
| 锁文件 | `yarn.lock` + `package-lock.json` + `pnpm-lock.yaml` **三份同时入库**（`build.sh` 只用 yarn） |

### 0.3 体量热点（`wc -l`，不含 `locales/`）

| 文件 | 行数 | 问题性质 |
| --- | --- | --- |
| `services/types.ts` | 2019 | 契约总表 + `Api` 接口（约 350 行）挤在一个文件 |
| `services/http-api-converters.ts` | 1495 | 转换函数 + 路径函数 + SSE 流（约 300 行）+ wire 接口混放；30 处 `any` |
| `pages/AgentManager.vue` | 1298 | 页面里写了「会话列表状态机 + 运行指示轮询 + 标签/成员 CRUD + 发送路由 + starter」五块逻辑 |
| `services/http-api.ts` | 1040 | 25 组 API；路径手拼、`Array.isArray(...) ? ... : []` 46 处 |
| `test/api-mock.ts` | 667 | 与 `Api` 手工同构 |
| `utils/bi-chart.ts` | 588 | 纯函数，结构尚可（只清死导出） |
| `composables/useAgentCollaboration.ts` | 515 | 工具卡合并逻辑与 `useAgentConversation.ts` 重复 |
| `pages/Databases.vue` | 499 | 4 组「打开弹窗」状态 + 建库表单 |
| `components/bi/BiWidgetEditor.vue` | 460 | 指标行 / 分组 / 排序的 setter 平铺 |
| `pages/DataBrowser.vue` | 444 | 已抽 `useBiDataSource`，剩筛选 chips + 看板编辑 |

## 1. 发现清单（按文件落点）

优先级 = 可维护性收益（P1 大 / P2 中 / P3 可选），与风险、置信度分开写。置信度：**已验证**（读过源码 + 命令核对）/ **推理**（读过契约未执行）。

### 1.1 `pages/AgentManager.vue`（1298 行 → 目标 ≤450 行，P1）

现状：脚本 1110 行，五块互不相干的状态机写在同一个 `<script setup>` 里；测试通过 `vm.*` 直接访问 30 个内部符号（`threadId / selectThread / onSend / toolsHint / section / draft / agentNamesById / collab / chatMessages / sectionThreads / loadSectionThreads …`）。

| ID | 位置 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| AM-1 | `:345-401` 运行指示轮询（`activeRuns`、`activePollTimer`、`activeDisposed`、`refreshActiveRuns`、`onVisibilityChange`、`visibilitychange` 监听、卸载清理） | 抽 `composables/useActiveRunsPoll.ts`：入参 `{ projectId: () => string, sending: Ref<boolean>, onTick?: () => void }`，返回 `{ activeRuns, refresh, dispose }`；`onTick` 承接现在顺带调用的 `loadHumanTasks()`。轮询间隔、隐藏暂停、项目切换丢弃旧响应三条语义原样搬迁 | 中：定时器生命周期 | 已验证 | apply（先补测试：隐藏页不续轮询、项目切换后旧响应不落地、无运行时停止） |
| AM-2 | `:255-270, 499-576, 845-1026` 分区会话列表（`section / sectionThreads / sectionCursor / threadsPending / groupPages / loadSectionThreads / loadMoreSectionThreads / refreshThreadsForAgent / selectSection / prefetchGroupThreads`） | 与已有 `useAgentWorkspace`（其它群的分页缓存）是**同一件事的两份实现**：当前群走页面内状态，其它群走 composable。→ 把「当前群」也并入 `useAgentWorkspace`：`ws.pages[section]` 成为唯一数据源，`groupPages` 计算属性删除（直接传 `ws.pages`），`loadMoreInGroup` 的分支消失。`project.id !== projectId \|\| section.value !== sec` 的 4 处竞态守卫收进 composable 的 `seq` 机制（它已对其它群这样做） | 中：`AgentManager.test.ts` 用 `vm.sectionThreads / vm.sectionCursor / vm.loadSectionThreads` 共 4 处 | 已验证 | apply：页面保留 `sectionThreads = computed(() => ws.pages[section]?.threads ?? [])` 与同名函数转发，测试断言不改 |
| AM-3 | `:1028-1149` 标签 / 群成员 CRUD（`openTagCreate / openTagEdit / saveTag / reloadEditingTag / archiveTag / openMembers / openMembersCreate / saveMembers / reloadMembersGroup / refreshGroupMembers`，12 个 ref） | 抽 `composables/useAgentTagEditor.ts`，对外 `{ tagModal, membersModal, openTag, openMembers(groupId, mode), saveTag, saveMembers, reloadTag, reloadMembers, archiveTag }`；两个 `*Open/*Saving/*Conflicted` 三元组合并为 `interface ModalState { open, saving, conflicted }`。同批：`openMembers` / `openMembersCreate` 只差 `mode` → 合并为一个函数；`refreshGroupMembers()` 只是 `loadTags()` 的别名 → 删除；`saveMembers` 中 `const updated = …; void updated` 无用变量 → 删除 | 低 | 已验证 | apply |
| AM-4 | `:1055`、`:1130` 两处 `(e as { status?: number })?.status === 409`；`useStarterLaunch.ts:74` 第三处 | → `utils/api-errors.ts` 增加 `isConflict(e): boolean` 与 `errorCode(e): string \| undefined`（类型走 `ApiError`），替换 3 处 409 判断与 `AgentManager.vue:318/1226/1251`、`useAgentConversation.ts:257` 的 `as { code?: string }` 强转 | 低 | 已验证 | apply |
| AM-5 | `:317-337` `collab.onError` 四个 `if (code === …) { toast.warning(...); return }` | → 常量表 `const COLLAB_WARNINGS: Record<string, string> = { agent_not_in_tag: 'components.agentWorkspace.agentNotInTag', group_member_unavailable: …, group_archived: …, turn_busy: …, human_assignee_required: … }`，函数体 3 行 | 低 | 已验证 | apply |
| AM-6 | `:403-415, 434-450` `builtinDefaultNames`（`'通用助手' / 'Database' / 'S3' / 'Logs' / '键值助手' …`），`mentionAgents` 中同一 `agents.value.find(raw => raw.id === a.id)` 每项执行两次；`AiChatComposer.vue:210-220 namesForAgent` 又硬编码 `'KV助手'` 并自己遍历两种语言 | 这张表是后端种子名（`internal/systemdb/agents.go:769-818`，已核对逐字一致）的前端副本，用来判断「用户没改过名 → 按界面语言显示」。→ 新建 `utils/agent-names.ts`：`BUILTIN_SEED_NAMES`（注释写明与后端 seed 同步）、`isSeedName(agent)`、`displayAgentName(agent, locale)`、`builtinAliases(key)`（含 `'KV助手'` 历史别名）；`AgentManager` 与 `AiChatComposer` 共用；`mentionAgents` 先建 `rawById` Map 再映射 | 低：等价搬移 | 已验证 | apply |
| AM-7 | `:422-428` `activeAgent`、`activeThreadAgent`、`activeThreadAgentName`、`threadAgentName`（= `activeThreadAgentName` 的别名） | 四个计算属性围绕同一个 id 各 `find` 一次。→ 保留 `activeThreadAgent`（原始）与 `activeAgent`（本地化），`activeThreadAgentName = computed(() => activeAgent.value?.name ?? '')`；删除别名 `threadAgentName`，模板改用 `activeThreadAgentName` | 低 | 已验证 | apply |
| AM-8 | `:551-555` `isCollaborativeThread`、`:487-492 routeHint`、`:1197 onSend`、`:424 activeThreadAgentId` | 四处各自 `sectionThreads.value.find((th) => th.id === threadId.value)`。→ 一个 `activeThread` 计算属性 | 低 | 已验证 | apply |
| AM-9 | `:785-814` `loadThreadMessages` 与 `resumeActiveRun.onError` | 两处相同的 `msgs.map((m) => toChatMessage(m, agents.value.find(...)?.module))` → `toChatMessages(list)` 一个函数；`agents.value.find((a) => a.id === x)` 在本文件出现 7 次 → `agentById = computed(() => new Map(...))` | 低 | 已验证 | apply |
| AM-10 | `:820` `await Promise.resolve(api.agentThreads.get(...)).then((r) => r).catch(() => null)` | `Promise.resolve(...)` 与 `.then((r) => r)` 均为空操作 → `await api.agentThreads.get(...).catch(() => null)`；紧随的两段 `if (!th) / if (!th.tag_id)` 同文案 → 合并为 `if (!th?.tag_id)`。`:1019 refreshThreadsForAgent` 同样去掉 `Promise.resolve` 包装 | 低：`api-mock` 返回的是 `vi.fn().mockResolvedValue`，始终是 Promise；需确认没有测试把它桩成同步值 | 推理 | apply（先 `rg "agentThreads.get.mockReturnValue" tests`） |
| AM-11 | `:1157-1234 onSend`（78 行） | 拆两个纯函数：`collectMentionTargets(mentions) → { agentIds, wantsHuman }`、`validateMentions(...) → warningKey \| ''`；`onSend` 只剩「校验 → 选协作/单助手分支 → 发起」。`usedMentions` 三元嵌套改 if | 中：`AgentManager.test.ts` 有 7 处 `vm.onSend` 覆盖各分支 | 已验证 | apply |
| AM-12 | `:1236-1258 retryLast` 与 `onSend` 单助手分支 | 两处 `conv.start(..., { onEnd: () => { refreshThreadsForAgent() }, onError: (e) => { if (code === X) { toast.warning; return } toast.error(...) } })` 同构 → `runHandlers(warnCode, warnKey, onWarn?)` 小工厂 | 低 | 已验证 | apply |
| AM-13 | `:655-728` starter 自动发送 + `replaceQuery` 串行队列 | starter 是独立功能（`useStarterLaunch` 已存在于首页侧），→ 抽 `composables/useStarterThread.ts`（`maybeStartStarter`、`buildStarterSeed`、`starterSite`）；`replaceQuery` 抽到 `composables/useQueryPatch.ts`（通用：合并 patch、`undefined` 删键、串行），`ApiPreview.vue` 的 `rememberKey/readRemembered` 若有同类 query 写入可复用 | 低 | 已验证（AgentManager 侧） | apply |
| AM-14 | `:189-227` 导入 | 删除未用 `Button`、`ConfirmAction`；相对路径 `'../services/api'` 与 `'@/...'` 混用（本文件 18 处相对、8 处 `@/`）→ 统一 `@/`（仅本次动到的文件，见 C-4） | 无 | 已验证 | apply |
| AM-15 | `:1279-1287` `retryTask`、`detachRun` 一行转发 | `detachRun()` = `conv.clear()` 被调用 6 次，保留（名字表达「只断 SSE 不取消」的意图）；`retryTask` 模板可直接 `@retry-task="collab.retryTask"` → 删除 | 低 | 已验证 | apply |

**AM 目标结构**：

```text
pages/AgentManager.vue            模板 + 编排（≤450 行）；defineExpose 维持测试依赖的同名符号
composables/useActiveRunsPoll.ts  AM-1
composables/useAgentWorkspace.ts  AM-2（并入当前群，成为唯一会话分页源）
composables/useAgentTagEditor.ts  AM-3
composables/useStarterThread.ts   AM-13
composables/useQueryPatch.ts      AM-13
utils/agent-names.ts              AM-6
utils/api-errors.ts               + isConflict / errorCode（AM-4）
```

### 1.2 `services/` API 层（P1）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| A-1 | `http-api.ts` 35 处 `'/v1/projects/' + encodeURIComponent(projectId) + …`；converters 中已有 `agentPath` / `kvPath` / `cronJobsPath` / `gofunctionsPath` / `databaseResourcePath` / `dataCollectionsPath` 6 个各写一遍前缀 | → 新建 `services/paths.ts`：`projectPath(projectId, ...segments: (string \| number)[])`（每段 `encodeURIComponent`，与现有手写等价），其余 `xxxPath` 改为基于它的一行实现并从 converters 迁到 `paths.ts`。`agentPath(projectId, '/agents/' + encodeURIComponent(id))` 这类「调用方自己编码」的写法统一改为 `projectPath(projectId, 'agents', id)` | 中：**编码位置**——必须逐个确认原路径里没有故意不编码的段（如 `suffix` 里含 `/`）；`tests/http-api*.test.ts` 断言具体 URL，可兜底 | 已验证（读过 agent/mcp/triggers 段） | apply（按 API 分组逐组替换，每组跑 `tests/http-api*.test.ts`） |
| A-2 | `kv-endpoints.ts:109 kvBasePath` 与 `http-api-converters.ts:1441 kvPath` | 逐字相同 → 删 `kvBasePath`，`KvApiPanel.vue` 与 `tests/KvApiPanel.test.ts:7,214` 改用 `paths.ts` 的 `kvPath` | 低 | 已验证 | apply |
| A-3 | `http-api.ts` 46 处 + converters 38 处 `Array.isArray(r.data?.x) ? r.data.x : []`（其中 21 处紧跟 `.map(toX)`） | → `services/wire.ts`：`listOf<T>(value: unknown, map?: (raw) => T): T[]`。`(Array.isArray(r.data?.threads) ? r.data.threads : []).map(toAgentThread)` → `listOf(r.data?.threads, toAgentThread)` | 低：纯函数等价 | 已验证 | apply |
| A-4 | `http-api.ts:848-864` `agentThreads.page` 与 `pageBy` | `page` 生产零调用（只在测试），`pageBy` 是它的超集 → 删 `page`，测试改调 `pageBy`；`Api` 接口同步删除 | 低（内部契约） | 已验证 | apply |
| A-5 | `http-api.ts:865-879` `agentThreads.create(projectId, titleOrOpts, agentId)` 的 string 重载（「兼容旧：title」） | 生产两处调用都传对象；string 形态只剩 `tests/http-api.test.ts:604` → 删 string 重载与第三参，`Api.agentThreads.create` 签名收窄为 `(projectId, opts: AgentThreadCreateOptions)`；body 构造改为 `pickDefined({ title, agent_id, tag_id, dispatch_mode, thread_kind, source })`（`title` 保留「显式 undefined 不发送」语义） | 低 | 已验证 | apply |
| A-6 | 生产未调用的 API 方法 12 个：`sandboxes.get`、`db.rows`、`db.update`、`db.document`、`s3.download`、`agents.patch`、`agents.remove`、`agentThreads.page`（A-4）**只被测试调用**；`agentThreads.turns`、`agentTags.removeMember`、`agentTags.submitEvent`、`agentTurns.capabilities` **连测试都没有** | 后 4 个是 v2 协作的预留接口（后端已有路由）。→ 前 7 个：保留（`db.*` / `agents.*` 是控制台 API 层对后端资源的完整覆盖，删了会让 ApiPreview 之外的调用面出现缺口）——**skip**；后 4 个：删除 `http-api.ts` 实现 + `Api` 接口条目 + `api-mock.ts` 桩，需要时再加 | 低 | 已验证 | 后 4 个 **propose（N-2）**，默认删 |
| A-7 | `http-api.ts:985-994 agentApprovals.resolve` 与 `:915-918 agentThreads.confirm` | 同一个端点 `POST /agent-runs/{run}/confirmations/{call}` 的两份实现 → `agentApprovals.resolve` 改为直接引用 `agentThreads.confirm` 的同一函数（保留两个入口名，因为分属两个 UI 场景） | 低 | 已验证 | apply |
| A-8 | `http-api.ts:1012-1031 mcpServers.test / testBuiltin` | 结果映射 `{ ok, tools, error }` 逐字重复 → converters `toMcpTestResult` | 低 | 已验证 | apply |
| A-9 | `http-api.ts:104 export { agentScheduleFromCronJob }` 与注释「AgentManager 直接消费」 | 已并入 P-10（整条 AgentSchedule 链只服务于死组件 `AgentListPanel`） | — | 已验证 | 见 P-10 |
| A-10 | `http-api.ts:1034-1035` 空壳注释「agentSchedules 整组 API 已删除…」 | 历史说明 → 删除 | 无 | 已验证 | apply |
| A-11 | `http-api-converters.ts` 1495 行，四类东西混放 | 按职责拆（同目录，`http-api.ts` 的 import 列表随之分组）：`converters/agent.ts`（toCloudAgent…toWorkspaceCapabilities、is* 守卫）、`converters/data.ts`（数据库 / SQL / 集合 / 文档 / 聚合 / KV）、`converters/platform.ts`（项目 / 用户 / 认证 / 日志 / 指标 / LLM / 备份 / S3 / 沙盒 / cron / gofunction）、`agent-stream.ts`（`readSse`、`openAgentRunStream`、`streamAgentRun/Turn`、`attachAgentRun`、`fetchAgentRunEvents`，约 300 行，唯一含 `fetch`/重连状态机的部分）、`paths.ts`（A-1）。`http-api-converters.ts` 删除，不留 re-export 桶文件（唯一外部引用是 `tests/http-api-bi.test.ts`，同批改 import） | 低：纯移动 | 已验证（符号清单） | apply |
| A-12 | `http-api-converters.ts` 30 处 `any`（`toX(raw: any)`） | AGENTS.md「新代码不使用 `any`」。→ 入参统一 `raw: WireRecord`（`type WireRecord = Record<string, unknown>` 已在多处手写），借 A-3 的 `str(v)` / `num(v)` / `bool(v)` / `listOf` 小工具读取字段；**只在 A-11 拆分时顺带改**，不单开批次 | 中：`unknown` 会迫使每个字段显式转换，可能暴露隐式 `undefined` 透传；逐函数跑 `tests/http-api*.test.ts` | 推理 | apply（拆分批次内逐文件做） |
| A-13 | `services/types.ts` 2019 行 | 拆为 `types/`：`common.ts`、`agent.ts`（云助手 / 标签 / 轮次 / 审批 / MCP / 流式 handlers，约 700 行）、`data.ts`（数据库 / SQL / 集合 / KV / BI）、`platform.ts`、`api.ts`（`interface Api`）；`services/types.ts` 保留为**唯一**桶文件 `export * from './types/…'`——它是 AGENTS.md 指定的契约入口（`services/api.ts` 也 `export * from './types'`），全仓 200+ 处 `from '../services/api'` / `'@/services/types'` 不动 | 低：纯移动 | 已验证 | apply |
| A-14 | `types.ts` 零引用接口：`LlmTraceQuery`、`LLMTraceListResult`、`LlmChatRequest`、`LlmChatResponse`、`LlmStreamHandlers`、`AgentGroupCronBinding` | 旧 LLM 聊天 / trace 列表 / 群定时绑定的残留 → 删除 | 无 | 已验证 | apply |
| A-15 | `services/api.ts` 的 `api = httpApi` 二次命名 | 历史上用于 mock 切换（v4.0 已退役）。保留：它是 AGENTS.md 规定的唯一入口、测试 `vi.mock('@/services/api')` 依赖这个模块路径 | — | — | skip |
| A-16 | `test/api-mock.ts` 667 行手工复刻 `Api` 结构 | → 用 `mockApi<T>(shape)` 递归把 `httpApi` 的每个函数替换为 `vi.fn()`（结构从真实对象派生，新增接口不再需要手抄）；`resetApiMocks` 中的默认返回值表保留 | 中：部分测试依赖桩上的特定默认值 | 推理 | **propose（N-3）**：收益大但触及 100+ 测试文件的公共夹具，单独批次 |

### 1.3 会话流 composable（P1）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| CV-1 | `useAgentConversation.ts:169-242` 与 `useAgentCollaboration.ts:270-302` | 工具卡合并（`onToolCallDelta` / `onToolStart` / `onToolResult` 对 `toolCalls[]` 的查找 + 原地更新 + 新增）**两份实现且已经漂移**：单助手版 delta 只在 `generating` 态追加参数、按 `name` 兜底匹配；协作版无状态判断、`onToolStart` 不按 name 兜底。→ 抽 `utils/tool-cards.ts` 纯函数：`applyToolDelta(cards, name, delta, callId)`、`applyToolCall(...)`、`applyToolStart(...)`、`applyToolResult(...)`，返回新数组；两个 composable 只负责 phase / runtime 与「把新数组写回」 | 中：两版语义不同，**合并即行为变化** | 已验证（逐行对照） | 第一步 apply：抽函数并以**单助手版**语义为准只替换 `useAgentConversation`；第二步 **propose（N-4）**：协作版改用同一组函数（等于修正协作侧的边界行为，需补测试：乱序 delta、无 callId 的 result） |
| CV-2 | `useAgentConversation.ts:145-277 bindHandlers`（132 行） | 每个回调末尾 `handlers.onX?.(...)` 透传 → 用一个 `forward` 包装（`wrap('onToken', fn)`）去掉 14 处尾行；或接受现状 | 低 | 已验证 | P3 可选 |
| CV-3 | `useAgentWorkspace.ts:179 errorText` 与 `utils/format.ts errorMessage`、`utils/agent-errors.ts agentErrorText` | 三个「错误 → 文案」函数，`errorText` 回退值是英文字面量 `'request failed'`（违反 i18n 规则） → `errorText` 删除，调用点改 `errorMessage(e, t('error.requestFailed'))`（key 若不存在复用 `error.loadFailed`） | 低：文案从英文硬编码变成 i18n（可见变化很小，且只在非 Error 抛出时） | 已验证 | apply（列入 §2 行为说明） |

### 1.4 页面与业务组件（P2）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| P-1 | `Databases.vue:400-422` `openSql / openCreateCollection / openDocList / openKv` | 四个「设 activeDb(+collection) 再开弹窗」函数 + 4 个 `*Open` ref → `const dialog = ref<'' \| 'sql' \| 'createCollection' \| 'docList' \| 'kv'>('')` + `openDialog(kind, db, collection?)`。注意：`onAddDocumentFromList` / `onDocumentCreated` 会**叠开** `docList` 与 `kv` 两个弹窗 → 单值枚举不成立，改为保留 `docListOpen`、其余三个并入枚举 | 中：弹窗叠放 | 已验证 | apply（按上述例外） |
| P-2 | `Databases.vue:385-392, 424-431` `expandedRowKeys` 数组 + `includes/filter/展开` 三处手写 | → `Set` 的 `toggle/add` 两个小函数（或 `useToggleSet`），`watch(databases)` 中的就绪过滤保持 | 低 | 已验证 | apply |
| P-3 | `canWrite = computed(() => !isAdmin.value && auth.canWrite)` 在 `Databases`、`FunctionTriggers`、`GoFunctions`、`CronJobs`、`DataBrowser(canWriteData)` 5 处；`Sandboxes.vue:10,93,103,113` 模板里内联 `!isAdminProject && auth.canWrite` / 只写 `auth.canWrite`（**行内按钮漏了 admin 判断**） | AGENTS.md 已把它定义为规则「写入口用 `canWrite = !isAdmin && auth.canWrite`」→ 放进 `stores/project.ts` 不合适（跨 store），新增 `composables/useCanWrite.ts`（3 行）；Sandboxes 的行内三个按钮统一用它 | 低；Sandboxes 行内按钮在 admin 项目下由显示变为隐藏 = **行为变化**（与规则一致） | 已验证 | 抽函数 apply；Sandboxes 行为修正列 Bug-1 |
| P-4 | `Users.vue:209` 与 `UserFormModal.vue:138` 同名 `fmtTime(s) { return formatTime(s, '—') }` | 一行包装重复 → 模板直接 `formatTime(x, '—')`，删两个包装 | 无 | 已验证 | apply |
| P-5 | `SqlWorkModal.vue:191` 与 `SqlResultPanel.vue:142` 逐字相同的 `formatCell` | 结果区拆出后宿主遗留了一份（注释 `:95` 说「既有测试直接调用」）→ 移到 `utils/format.ts formatCell`，两处 import；`SqlWorkModal` 仍 `defineExpose({ formatCell })` 以保持测试表面 | 低 | 已验证 | apply |
| P-6 | `BackupPanel.vue:111 formatSize`（KiB/MiB）、`:116 formatDate`（`toLocaleString()` 不带语言） | 与 `utils/format.ts formatBytes`（KB/MB）、`formatTime`（按界面语言、24h）是同一职责的另一套口径；备份列表在英文界面下日期仍按浏览器 locale 显示 | 统一 = **可见格式变化**（`1.5 KiB` → `1.5 KB`；日期跟随界面语言） | 已验证 | **propose（N-5）** |
| P-7 | `KvApiPanel.vue:151-167 copy()` | 自带一份 `navigator.clipboard` + `execCommand` 降级，绕过 AGENTS.md 规定的 `utils/clipboard.ts copyText`。两者差异：KV 版**失败也提示成功**、有 `textarea` 降级 | 改用 `copyText` = 失败时提示从 success 变 error | 已验证 | **propose（N-6）**：建议把 `execCommand` 降级下沉进 `copyText`（惠及全部 10 个调用方），KV 版删除 |
| P-8 | `S3Manager.vue:403`、`Databases.vue:492`、`CronJobs/GoFunctions/Sandboxes/FunctionTriggers` 的 `void load(); watch(projectId, () => { page.value = 1; void load() })` | 6 个页面同一段「首次加载 + 切项目重载 + 页码归 1」。注：`ProjectScope` 的 `:key` 只重挂**插槽内 DOM**，页面 `<script setup>` 不会重跑，所以这些 watch **是必要的**，不能删。→ `composables/useProjectReload(load, { onSwitch? })`（内部 `watch(() => project.id, …, { immediate: true })`）；FunctionTriggers 已是 `immediate` 写法，其余从「先 `void load()` 再 watch」改为 immediate（调用顺序等价：都是 setup 同步阶段触发一次） | 低 | 已验证 | apply |
| P-9 | `Sandboxes.vue`、`FunctionTriggers.vue`、`CronJobs.vue`、`GoFunctions.vue`、`Users.vue`、`Databases.vue` 的 `<SbAsyncRegion as="tbody" :pending :show-skeleton :show-empty :show-error :refreshing :error …>` | 37 处把 `useLoadState` 的 6 个字段逐个透传 → `SbAsyncRegion` 增加可选 `state` prop（接收 `LoadState`），有 `state` 时以它为准，原散装 props 保留（向后兼容，组件内部只多一个 `computed`）。各页面从解构 6 个字段改为 `const load = useLoadState(...)` + `:state="load"` | 中：`SbAsyncRegion` 是公共组件；散装 props 继续可用，调用方逐个迁移 | 已验证 | apply（先改组件 + 测试，再分 3 批迁页面） |
| P-10 | `AgentListPanel.vue`（4.8KB）、`ThreadSwitcher.vue`（2.5KB），以及**只被 `AgentListPanel` 使用**的 `AgentCard.vue`（3.5KB）与其数据链 `types.ts:1182-1203 AgentSchedule`、`http-api-converters.ts:906-924 agentScheduleFromCronJob`、`http-api.ts:28,104` 转出口 | 生产零引用（AgentManager 改分区制后不再使用），只被 `agent-components / agent-split-components / agent-toolcard-variants / template-inline-handlers / thread-switcher-time / nav-icons` 6 个测试引用 → 删除 3 个组件与上述类型/函数；对应测试用例删除（`thread-switcher-time.test.ts` 整文件删除，其余文件只删相关 `describe/it`）。同步删 `AgentManager.vue:293` 提到 ThreadSwitcher 的注释与仅被它们使用的 i18n key。A-9 并入本条 | 低：删除死代码与只测死代码的测试（不是削弱对存活代码的断言）；删前 `rg -w 'AgentCard\|AgentListPanel\|ThreadSwitcher\|AgentSchedule\|scheduleSummary' src tests` 复核 | 已验证 | apply |
| P-11 | `McpServersPanel.vue` 未用 `RefreshCwIcon` import | 删除 | 无 | 已验证 | apply |
| P-12 | `CreateProjectModal.vue:38`、`AgentTagModal.vue:62,74` 原生 `<select>` | AGENTS.md「新代码不写原生 select」，且 AGENTS.md:75 列的遗留项（`SchemaPanel`、`AgentFormModal`）已不存在/已迁移，**真正的遗留是这三处** → 迁 `ui/select`；同步更新 AGENTS.md:75 | 中：`modal-input-layout.test.ts` 与弹窗测试按 `select` 元素取值 | 已验证 | apply（测试交互改 `SelectTrigger`） |
| P-13 | `DefaultLayout.vue:62`、`UserMenu.vue:4` 在 `<Button>` 上写 `h-8 px-2.5` | 与 AGENTS.md 按钮规范冲突；但这是 S3「顶栏控件同高」的有意例外（顶栏 `h-8`） | — | 已验证 | skip（在 AGENTS.md 按钮规范处补一句「顶栏例外」，避免后人误改） |
| P-14 | `BiWidgetEditor.vue` 12 个 `setX(value)` 单行 setter | 每个都是 `draft.value = { ...draft.value, x: value }` 或其嵌套变体 → `patch(partial)` / `patchQuery(partial)` 两个函数 + 模板 `@update:model-value="(v) => patch({ limit: toLimit(v) })"`；带转换的（`setBin`、`setLimit`、`setSpan`）保留具名函数 | 低 | 推理（已看符号表，未逐行读） | apply（读全后） |
| P-15 | `DataBrowser.vue` 筛选 chips（`filterDraft / appliedFilter / history / chips / addChip / removeChip / useHistory / applyFilter / clearFilters / appendField`） | 约 70 行独立状态 → `composables/useFilterChips.ts`；页面的 `defineExpose` 与 `test/page-vm.ts` 适配层同步 | 低 | 推理 | apply（读全后） |

### 1.5 工具与 composable 清理（P2）

| ID | 文件 / 符号 | 问题 → 方案 | 风险 | 置信度 | 处置 |
| --- | --- | --- | --- | --- | --- |
| U-1 | `utils/bi-chart.ts:586 isChartType` | 全仓零引用 → 删除 | 无 | 已验证 | apply |
| U-2 | 只给测试用的导出：`constants/llmProviders.ts maskSecret`、`utils/project-id.ts isDnsSafeProjectId`、`utils/bi-compat.ts compatibleTypes`、`docs/catalog.ts isLargeDoc` | 逐个判断：若生产内部也在用（被同文件其它函数调用）则只是「导出给测试」，保留；若同文件也不用 → 删函数与测试 | 低 | 已验证（`self≤1` 即文件内仅定义处） | 4 个均为文件内仅定义、生产零调用 → apply 删除，测试用例同删 |
| U-3 | 「仅本文件使用却 `export`」的类型/函数约 70 个（如 `useBiQuery.ts BiStaleError / biQueue`、`useAgentWorkspace.ts errorText`、`http-api-converters.ts readSse / toAgentTask / isRouteKind …`） | 去掉 `export` 只是降低表面积，不减行数；converters 拆分（A-11）时顺手把内部函数去 `export` | 无 | 已验证（脚本清单） | apply（仅在 A-11 / 被动到的文件里做，不单开批次） |
| U-4 | `lib/utils.ts`（`cn`）与 `utils/` 并存 | `lib/` 是 shadcn 约定目录（`components.json` 的 `aliases.utils` 指向它）；`lib/status.ts` 是业务状态文案却放在 `lib/` → `status.ts` → `utils/status.ts`（28 个 import 点机械替换）；`lib/utils.ts` 保留 | 低 | 已验证 | apply |
| U-5 | `utils/relative-time.ts`（1 个函数 15 行）、`utils/download.ts`、`utils/clipboard.ts`、`utils/api-errors.ts` | 小文件本身没问题，**不合并**（每个都是 AGENTS.md 点名的规则入口） | — | — | skip |
| U-6 | `composables/useBiTheme.ts`、`useLocale.ts`、`usePagination.ts` | 已足够小 | — | — | skip |
| U-7 | `typeof document !== 'undefined'` 5 处（`AgentManager` 3、`useBiTheme`、`KvApiPanel`） | 运行环境只有浏览器与 jsdom，二者都有 `document` → 删除守卫 | 低：SSR 未使用（无 `vite-ssg` / `renderToString`） | 已验证 | apply |

### 1.6 注释治理（P2，零行为风险）

| ID | 位置 | 方案 | 处置 |
| --- | --- | --- | --- |
| C-1 | 105 个文件 / 282 处计划编号（`types.ts` 59、`AgentManager.vue` 29、`http-api-converters.ts` 15、`http-api.ts` 7、`Home.vue` 6、`ConversationView.vue` 6…） | 与后端计划 C-2 同规则：**注释写「为什么」本身，编号最多留一个作为 grep 锚点**；纯重构历史（「原 ThreadSwitcher 无重命名入口，这里补上」「BUG-07：优先用 …」「DC-5 已删除」「（planv6.7 v2 §4.1：群列表 + …）」）删除。旧计划目录 planv5.0 / planv6.0–6.6 已被 planv6.7 合并清理，指向它们的锚点（如 `lib/status.ts`「见 plan/planv6.7/legacy-plan-consolidation-plan.md §4.8」）保留一个即可 | apply（只在本计划动到的文件里做；`types.ts` 随 A-13 拆分时一次性处理） |
| C-2 | `types.ts` 分区标题 `（proto-http.md §3.1）` 等 | `proto-http.md` 已不存在（planv6.7 §2 锚点表已改指向）→ 去掉括号 | apply |
| C-3 | `vite.config.ts:73-74` 等配置注释 | 不动（不改配置文件） | skip |
| C-4 | 导入风格：业务代码 333 处 `'../'` 相对导入、676 处 `'@/'` | AGENTS.md 未规定；**只在被本计划修改的文件里**统一为 `@/`，不做全仓机械替换（避免 diff 噪音） | apply（随改随统一） |

### 1.7 测试与夹具（P3）

| ID | 位置 | 方案 | 风险 | 处置 |
| --- | --- | --- | --- | --- |
| T-1 | 带轮次后缀的测试文件：`DataBrowserExtra`、`DataBrowserFinal`、`template-inline-handlers-2`、`useAgentConversation-edge`、`user-login-branches`、`capability-gap-http` | 按被测对象合并或改名：`DataBrowser{,Extra,Final}.test.ts` → `DataBrowser.test.ts` + `DataBrowser.filters.test.ts` + `DataBrowser.board.test.ts`（按 describe 主题分）；`template-inline-handlers{,-2}` 合并；`useAgentConversation-edge` 并入 `useAgentConversation.test.ts`；`capability-gap-http` 按 API 组并入 `http-api*.test.ts` | 低：只移动 `it`，断言不改；用例总数必须保持 1399 − P-10/U-2 删除数 | apply |
| T-2 | `tests/SettingsPanel.test.ts` | 文件名指向已下线的 `SettingsPanel.vue`，实际测的是 `ModelDefaultsPanel` → 改名 `ModelDefaultsPanel.test.ts` | 无 | apply |
| T-3 | `src/test/helpers.ts uiStubs` | AGENTS.md「不要桩掉 ui/ 交互组件」；需检查 `uiStubs` 仍桩了哪些交互组件，与 P-12 同批核对 | 低 | apply（读全后） |

### 1.8 i18n 文案树（P2）

| ID | 位置 | 方案 | 风险 | 处置 |
| --- | --- | --- | --- | --- |
| L-1 | `locales/zh-CN.ts` / `en-US.ts` 约 107 个疑似未用 key（脚本：无字面引用、不命中任何 `` `prefix.${x}` `` 动态前缀、末段也不以字面量出现）。集中在：`common.{loadingMore,search,add,start,reset,unknown,noDescription,totalCount,pagePrev,pageNext,hint,seconds,external,inner,hide,show}`、`error.{generic,emptyMessage}`、`error.codes.{agent_prompt_required,agent_prompt_too_long,agent_not_found,invalid_target_kind,agent_schedules_merged}`、`layout.{accountMenu,changePassword,logout,roleSuper,roleAdmin,roleUser}`、`pages.agents.{createTitle,editTitle,form*,toolLabel,every15Min…}`（已下线的助手表单）、`components.agentWorkspace.threadKind.*`、`pages.traces.{provider,streamNo}` 等 | 每个 key 删除前再 `rg` 一次全名与末段；动态拼接（`t(\`error.codes.${code}\`)` 一类）要人工确认——`agent-errors.ts` 用的是显式 `CODE_KEYS` 表，表外的 `error.codes.*` 才是真死。两棵树同步删（`i18n.test.ts` 卡 key 集合一致） | 中：误删 = 运行时显示 key 原文 | apply（按命名空间分批，每批跑 `i18n.test.ts` + 全量测试；清单见附录 B 生成命令） |
| L-2 | 内置助手名 `builtinNames` 与 `AgentManager.builtinDefaultNames` 的口径不一致 | 见 AM-6 | — | propose（N-1） |

### 1.9 仓库卫生与文档（P0）

| ID | 位置 | 方案 | 处置 |
| --- | --- | --- | --- |
| D-1 | `ui/yarn.lock`、`ui/package-lock.json`、`ui/pnpm-lock.yaml` 三份入库 | `build.sh` 与 AGENTS.md 只用 yarn → 删除 `package-lock.json`、`pnpm-lock.yaml`。属于依赖/配置面变更 | **propose（N-7）** |
| D-2 | 6 个 `.DS_Store` | 已被根 `.gitignore` 忽略、未入库；本地删除即可 | apply（本地） |
| D-3 | `ui/AGENTS.md:95-96`、`ui/README.md:9,34,120` | ①「`yarn typecheck` 不干净、241 处 TS2307」→ 实测 0 错误，改为「`yarn typecheck` 必须通过」并把它加入验证门禁；② 覆盖率「95%」→ 与 `vite.config.ts` 实际值 90% 对齐（**只改文档，不改阈值**）；③ README「测试 85 个文件」→ 151；④ AGENTS.md:75 原生 select 遗留项清单更新（P-12）；⑤ AGENTS.md:14 composables 列表补 `useAgentWorkspace / useAgentCollaboration / useBiDataSource / useBiDetail / useStarterLaunch / useApiParamOptions` 与本计划新增项 | apply |
| D-4 | `ui/AGENTS.md:11` 组件清单 | 删去 `AgentListPanel` 相关描述（若有）、补 `agent/` 组件族现状 | apply |

## 2. Bugs found / 行为差异（不随等价重构静默修改）

| ID | 位置 | 级别 | 说明 / 复现 | 处置 |
| --- | --- | --- | --- | --- |
| Bug-1 | `pages/Sandboxes.vue:93,103,113` | 已确认（读码） | 行内「暂停 / 恢复 / 删除」只判 `auth.canWrite`，没有判系统项目；而卡片头「新建」判了 `!isAdminProject`。复现：admin 项目、superadminl1 登录 → 行内写按钮可见（后端会拒绝） | P-3 同批修，单独提交 + 用例 |
| ~~Bug-2~~ | `pages/AgentManager.vue:403-405 builtinDefaultNames` | **已排除** | 初看 `'Database' / 'S3' / 'Logs'` 与语言包 `'数据库助手'` 不一致；核对 `internal/systemdb/agents.go:776/804/811` 后端种子名正是 `Database / S3 / Logs`，前端表比对的是种子名而非语言包，逻辑正确。只做 AM-6 的去重 | — |
| Bug-3 | `useAgentCollaboration.ts:270-302` vs `useAgentConversation.ts:169-242` | 推理 | 协作会话的工具卡：`onToolCallDelta` 在卡片已 `running/success` 后仍会追加参数；`onToolStart` 无 callId 时不会按 name 匹配而是新增一张卡 → 可能出现重复卡 | CV-1 第二步（N-4） |
| Bug-4 | `useAgentWorkspace.ts:182` | 已确认 | 非 Error 抛出时回退英文字面量 `'request failed'`，中文界面显示英文 | CV-3 |
| Bug-5 | `KvApiPanel.vue:151-167` | 已确认 | 复制完全失败（clipboard 与 execCommand 都不可用）时仍 `toast.success` | N-6 |

## 3. 需审批（propose）

| ID | 内容 | 默认建议 |
| --- | --- | --- |
| N-1 | （已撤销：AM-6 改为等价搬移，无需审批） | — |
| N-2 | 删除 4 个从未被调用、也无测试的 API 方法（A-6） | 同意 |
| N-3 | `api-mock.ts` 改为从 `httpApi` 结构派生（A-16） | 单独一轮做，本轮不做 |
| N-4 | 协作会话工具卡合并逻辑对齐单助手版（CV-1 第二步 / Bug-3） | 同意，补乱序用例 |
| N-5 | `BackupPanel` 改用 `formatBytes` / `formatTime`（P-6） | 同意（可见格式统一） |
| N-6 | `copyText` 增加 `execCommand` 降级，`KvApiPanel` 改用它（P-7 / Bug-5） | 同意 |
| N-7 | 删除 `package-lock.json`、`pnpm-lock.yaml`（D-1） | 同意（需确认 CI 不用 npm/pnpm） |

## 4. 目标结构（重构后）

只列本轮会新增 / 拆分 / 删除的文件。

```text
ui/src/
  pages/AgentManager.vue              1298 → ≤450（AM-1~AM-15）
  pages/Databases.vue                 499 → ≈400（P-1、P-2、P-8、P-9）
  pages/DataBrowser.vue               444 → ≈370（P-15）
  composables/
    useActiveRunsPoll.ts              + AM-1
    useAgentTagEditor.ts              + AM-3
    useStarterThread.ts               + AM-13
    useQueryPatch.ts                  + AM-13
    useProjectReload.ts               + P-8
    useCanWrite.ts                    + P-3
    useFilterChips.ts                 + P-15
    useAgentWorkspace.ts              接管当前群会话分页（AM-2），删 errorText（CV-3）
    useAgentConversation.ts           工具卡合并改调 utils/tool-cards（CV-1）
    useAgentCollaboration.ts          （N-4 批准后）同上
  services/
    api.ts                            不变（唯一入口）
    http.ts                           不变
    http-api.ts                       1040 → ≈780（A-1 路径、A-3 listOf、A-4/5/6/7/8 去重删冗）
    paths.ts                          + projectPath 与全部 xxxPath（A-1、A-2）
    wire.ts                           + listOf / str / num / bool / pickDefined（A-3、A-12）
    agent-stream.ts                   ← converters 中的 SSE 读取与重连（A-11）
    converters/{agent,data,platform}.ts ← http-api-converters.ts（A-11，删除原文件）
    types.ts                          桶文件：export * from './types/*'（A-13）
    types/{common,agent,data,platform,api}.ts
  utils/
    agent-names.ts                    + AM-6
    tool-cards.ts                     + CV-1
    api-errors.ts                     + isConflict / errorCode（AM-4）
    format.ts                         + formatCell（P-5）
    status.ts                         ← lib/status.ts（U-4）
  components/
    SbAsyncRegion.vue                 + state prop（P-9）
    agent/AgentListPanel.vue          删除（P-10）
    agent/ThreadSwitcher.vue          删除（P-10）
    agent/AgentCard.vue               删除（P-10）
    modal/CreateProjectModal.vue      原生 select → ui/select（P-12）
    modal/AgentTagModal.vue           同上
  lib/utils.ts                        保留（shadcn 约定）
ui/tests/
  thread-switcher-time.test.ts        删除（P-10）
  SettingsPanel.test.ts               → ModelDefaultsPanel.test.ts（T-2）
  DataBrowser{Extra,Final}.test.ts    → 按主题合并（T-1）
  template-inline-handlers-2.test.ts  → 并入 template-inline-handlers.test.ts
  useAgentConversation-edge.test.ts   → 并入 useAgentConversation.test.ts
  capability-gap-http.test.ts         → 并入 http-api*.test.ts
  useActiveRunsPoll / useAgentTagEditor / useProjectReload / tool-cards / paths / wire 新测试
```

## 5. Not changed（看起来能简化，但不动）

- **`toast.error(errorMessage(e, t('...')))` 92 处**：每处 fallback key 不同，抽 `toastError(e, key)` 只省 10 个字符且把 `t()` 藏进 helper，破坏 AGENTS.md「组件内用 `useI18n()`」的可读性。保持。
- **`services/api.ts` 的二次命名**（A-15）：AGENTS.md 规定的唯一入口，测试 `vi.mock` 依赖其路径。
- **`ProjectScope` 下各页面的 `watch(projectId)`**：`:key` 只重挂插槽 DOM，页面 setup 不重跑，watch 是必要的；只收敛写法（P-8），不删。
- **`DefaultLayout` / `UserMenu` 按钮上的 `h-8`**（P-13）：顶栏同高的有意例外，补文档不改代码。
- **`src/components/ui/**`**：fork 库代码。`avatar`、`sheet`、`toggle`（单独）当前零业务引用，但 `toggle-group` 依赖 `toggle` 的样式变体、`sidebar` 依赖 `sheet`；`avatar` 可删但 AGENTS.md 要求「删除前全局确认零引用」且收益极小 → 本轮不动，列附录 C 供下次清理。
- **`utils/bi-*.ts` 6 个纯函数文件**：结构清晰、测试完备（bi-chart 588 行是图表类型分派表，平铺比抽象更易对照），只删死导出（U-1）。
- **`useLoadState`**：实现正确（generation 防竞态、延迟骨架），只增加 `SbAsyncRegion` 的 `state` 适配（P-9），不改它本身。
- **`locales/*.ts` 的结构**：两棵同构树 + `i18n.test.ts` 卡一致性，是 AGENTS.md 的定稿方案；只删死 key（L-1），不拆文件、不懒加载。
- **相对 import 全仓替换为 `@/`**：纯风格，diff 噪音大；只在动到的文件里统一（C-4）。
- **`api-mock.ts` 结构派生**（A-16 / N-3）：下一轮单独做。

## 6. 实施批次

前提：`git status ui/` 干净（当前满足）。每批「一次只做一种变换」，批内按文件小步提交；每批结束跑 §6.2 门禁。动手前对照 `skills/simplify-code/references/frontend.md`（Vue 响应式、异步竞态）与 `behavior-traps.md`，问一句「什么输入 / 时序会让新旧不同」。

| 批次 | 内容 | 风险 | 预估净变化 |
| --- | --- | --- | --- |
| **P0 卫生** | D-2、D-3、D-4、P-11、AM-14（未用 import）、A-14、U-1、U-2、A-10、C-2、T-2 | 极低 | −150 行 |
| **P1 删死代码** | P-10（AgentListPanel / ThreadSwitcher / AgentCard / AgentSchedule 链 + 测试）、A-4、A-5、A-6（N-2）、L-1（按命名空间分 4 小批） | 低 | −900～−1100 行（含约 107×2 行文案） |
| **P2 API 层去重** | A-1、A-2、A-3、A-7、A-8、AM-4（api-errors） | 中（URL 编码） | −250 行 |
| **P3 API 层拆文件** | A-11（含 A-12 去 `any`、U-3 去多余 export）、A-13、C-1（`types.ts` 部分） | 低（纯移动） | ≈0（单文件 ≤600 行） |
| **P4 AgentManager 拆分** | AM-1 → AM-2 → AM-3 → AM-13 → AM-5~AM-12、AM-15、AM-6（`agent-names.ts`）、C-1（本文件部分） | 中 | 页面 −850，composable +450，净 −400 |
| **P5 会话流去重** | CV-1 第一步、CV-2（可选）、CV-3（Bug-4）；N-4 批准后 CV-1 第二步（Bug-3） | 中 | −120 行 |
| **P6 页面与组件** | P-3（含 Bug-1，单独提交）、P-4、P-5、P-8、P-9（先组件后页面，3 小批）、P-1、P-2、P-12、P-14、P-15、U-4、U-7；N-5、N-6 批准后 P-6、P-7（Bug-5） | 低–中 | −350 行 |
| **P7 测试整理** | T-1、T-3 | 低 | ≈0（用例数不减，除 P1 删除的死代码用例） |
| P8（可选） | A-16 / N-3、N-7（锁文件） | 中 | 视情况 |

顺序 P0 → P1 → P2 → P3 → P4 → P5 → P6 → P7。P1 先于 P2/P3 的原因：先删掉死 API / 死类型，后面搬文件的量更少；P2 先于 P3：先在原位置完成去重（diff 可审），再纯移动。

### 6.1 动手前的固定检查

1. 删除 / 改名任何导出前：`rg -w '<Symbol>' src tests` 必须只剩预期位置（含 `vi.mock` 工厂、`defineExpose`、`test/page-vm.ts` 映射）。
2. **A-1 路径替换**：先加一个临时快照测试，遍历 `httpApi` 每个方法用固定参数（含 `a/b`、`中文`、`空格` 三种 id）调用一次，记录 `http` 桩收到的 `method + url + params`；替换后快照必须逐字相同，验收后删除临时测试。
3. **AM 系列**：`tests/AgentManager.test.ts` 通过 `vm.*` 访问 30 个内部符号（清单见附录 A）。拆分后这些名字必须仍可在 `vm` 上访问（页面 `<script setup>` 顶层保留同名常量 / 转发函数即可），**不改测试断言**；若某名字确需改，测试改动单独提交并在说明里列出。
4. **AM-1 / AM-2** 先补测试再搬：轮询（隐藏页暂停、无运行时停止、项目切换丢弃旧响应）；当前群分页（加载更多去重、run 结束刷新时保留本地新建线程 `refreshThreadsForAgent` 的合并语义）。
5. **P-9**：`SbAsyncRegion` 新增 `state` 时，`loading-region.test.ts` / `loading-surfaces.test.ts` 增加「`state` 与散装 props 等价」的参数化用例。
6. **P-12**：迁 `ui/select` 后，`helpers.ts` 的 `SelectItem` 桩渲染为 `<option>`，原 `wrapper.find('select').setValue()` 写法需改为触发 `update:modelValue`；按 AGENTS.md 不要桩掉交互组件本身。
7. **L-1**：每个命名空间批次先生成候选清单（附录 B 命令）→ 人工剔除动态拼接 → 两棵树同删 → `npx vitest run tests/i18n.test.ts` → 全量。

### 6.2 每批验证门禁

```bash
cd ui
npx vitest run --coverage          # 基线 151 files / 1399 tests 全绿；覆盖率不低于 vite.config.ts 的 90% 阈值
npx tsc --noEmit                   # 基线 0 错误，必须保持 0
yarn build                         # AGENTS.md 规定必须通过
# 未用 import / 零引用导出 / 零引用组件扫描（附录 B），结果只允许减少
# 涉及 AgentManager / 会话流时追加：
npx vitest run tests/AgentManager.test.ts tests/useAgent*.test.ts tests/agent-*.test.ts
# 涉及 services 时追加：
npx vitest run tests/http-api*.test.ts tests/http.test.ts tests/api.test.ts
```

手工冒烟（`./build.sh dev`）：云助手（新建会话 → @ 两个助手 → 工具卡 → 确认卡 → 停止 → 切群 → 深链 `?thread=`）、数据库页四个弹窗、首页 starter 一句话建应用、英文界面下内置助手名。

### 6.3 预期收益（可量化指标）

| 指标 | 基线 | 目标 |
| --- | --- | --- |
| 最大业务文件（不含 locales） | 2019（`types.ts`） / 1495 / 1298 / 1040 | 全部 ≤800；`AgentManager.vue` ≤450 |
| `AgentManager.vue` 顶层 ref / computed / function | ≈30 / ≈35 / 52 | ≤12 / ≤20 / ≤25 |
| 生产零引用组件 | 3（AgentListPanel、ThreadSwitcher、AgentCard） | 0 |
| 零引用导出 / 死 API 方法 | 7 / 4 | 0 / 0 |
| 未用 import | 3 | 0 |
| `any` | 34（converters 30） | ≤4（仅 `api-mock.ts` 与 SSE 帧解析入口） |
| `'/v1/projects/' + encodeURIComponent` 手拼 | 39 | 0（集中在 `paths.ts` 一处） |
| `Array.isArray(...) ? ... : []` | 84 | ≤10 |
| 工具卡合并实现 | 2 份（已漂移） | 1 份 |
| 「错误 → 文案」函数 | 3（含英文硬编码回退） | 2（`errorMessage`、`agentErrorText`） |
| `canWrite` 定义处 | 5 + 1 处漏判 | 1 |
| 计划编号注释 | 282 处 / 105 文件 | 只保留锚点（预计 ≤60） |
| 疑似未用 i18n key | ≈107 × 2 棵树 | 0 |
| 文档与事实不符 | 4 处（typecheck、覆盖率、测试文件数、select 遗留项） | 0 |

## 7. Verification（本轮评估）

- **已运行（只读）**：`npx vitest run`（151 / 1399 全绿）、`npx tsc --noEmit`（0 错误）、`git status ui/`（干净）；扫描脚本：未用 import、零引用导出、零引用组件、生产未调用的 `httpApi` 方法、未用 i18n key、计划编号注释、重复模式计数（`rg`）。脚本放在 `.session_tmps/`，未入库。
- **已读源码核实**：`AgentManager.vue` 全文、`Databases.vue` 脚本段、`http-api.ts` 头部与 agent/mcp 段、`http-api-converters.ts` 符号表与 SSE 段、`useAgentConversation.ts:145-277`、`useAgentCollaboration.ts:216-320`、`useLoadState.ts`、`utils/{format,clipboard,api-errors,agent-errors,relative-time}.ts`、`lib/status.ts`、`ProjectScope.vue`、`KvApiPanel.vue:145-170`、`AiChatComposer.vue:205-225`、`internal/systemdb/agents.go:765-820`（种子名）、`ui/AGENTS.md`、`vite.config.ts`。
- **推理未执行**：P-14（`BiWidgetEditor` setter 细节）、P-15（`DataBrowser` 筛选块边界）、A-12（各 converter 去 `any` 后是否暴露隐式 undefined）、T-3（`uiStubs` 完整清单）、Bug-3（协作工具卡重复卡的实际触发时序）。执行对应条目前必须先读全。
- **未运行**：`yarn build`（会写 `dist/`，只读评估不执行）、浏览器手工冒烟、`--coverage`（会写 `coverage/`）。
- **范围声明**：i18n 死 key 检测基于字面串与 `` `prefix.${x}` `` 模式，**不能识别**以变量整体传入的 key（如 `t(item.labelKey)`、常量表值）；附录 B 清单必须人工复核。重复检测靠 `rg` 计数 + 人工对照，不代表已穷尽。

## 附录 A：`tests/AgentManager.test.ts` 依赖的 `vm.*` 符号（拆分后必须保留）

```text
threadId selectThread onSend toolsHint section draft agentNamesById selectSection removeThread
loadAgents collab chatMessages sectionThreads resetThread renameThread reloadThread loadModules
agents sectionCursor retryLast refreshThreadsForAgent pickPrompt onStop modelOptions loadThreads(可选调用)
loadThreadMessages loadSectionThreads loadMoreSectionThreads failedRunId bootstrap
```

## 附录 B：只读扫描命令（复现本计划数字）

```bash
cd ui
# 零引用组件
for f in $(find src/components src/pages src/layouts -name '*.vue' -not -path 'src/components/ui/*'); do
  b=$(basename "$f" .vue); n=$(rg -l "[/ ]${b}(\.vue)?['\"]" src | grep -v "^$f$" | wc -l | tr -d ' ')
  [ "$n" = "0" ] && echo "UNUSED $f"; done
# 零引用导出（文件内仅定义一次且全仓无他处引用）
for f in $(find src -name '*.ts' -not -path 'src/components/ui/*' -not -path 'src/test/*' -not -name '*.d.ts'); do
  for s in $(grep -oE '^export (async )?(function|const|interface|type|class) [A-Za-z0-9_]+' $f | awk '{print $NF}'); do
    n=$(rg -lw "$s" src tests | grep -v "^$f$" | wc -l); [ "$n" -eq 0 ] && [ "$(rg -cw "$s" $f)" -le 1 ] && echo "DEAD $f $s"; done; done
# 计划编号注释
rg -c --glob '!src/components/ui/**' --glob '!src/locales/**' 'planv[0-9]|§[0-9]|AGC-|BUG-[0-9]|DC-[0-9]|OPT-[0-9]|UX-' src | sort -t: -k2 -rn
# 手拼项目路径
rg -c "'/v1/projects/' \+ encodeURIComponent\(projectId\)" src/services
# 未用 i18n key：见 .session_tmps 中的 python 脚本（叶子 key 收集 → 字面匹配 → 动态前缀过滤 → 末段字面量过滤）
```

## 附录 C：`components/ui/` 零业务引用（下次清理候选，本轮不动）

| 目录 | 业务引用 | 说明 |
| --- | --- | --- |
| `avatar/` | 0 | 可删；需确认 `sidebar/` 等库内组件未引用 |
| `sheet/` | 0（被 `ui/sidebar/Sidebar.vue` 引用） | 不可删 |
| `toggle/` | 0（被 `ui/toggle-group` 引用样式） | 不可删 |
| `alert-dialog/`、`empty/`、`label/`、`progress/`、`scroll-area/`、`slider/`、`sonner/` | 各 1 | 保留 |

## 8. 执行结果与偏差（2026-11-23 回写）

P0–P7 已执行完毕（未提交 git）。门禁：`tsc --noEmit` 0 错误；`vitest` 全绿（覆盖率语句 97.7% / 分支 93.7% / 函数 91.3%，阈值 90%）；`yarn build` 通过。

### 8.1 与原计划不同的落点（有意偏差）

| 条目 | 计划 | 实际 | 原因 |
|---|---|---|---|
| **AM-10** | 删 `Promise.resolve(...)` 包装 | **保留** `selectThread` 里的 `Promise.resolve(api.agentThreads.get(...)).catch(...)` | `api-mock` 的 `agentThreads.get` 默认返回 `undefined`（非 Promise），直接 `.catch` 会抛 TypeError；代码内已有注释。若日后让 mock 默认返回 Promise 再删 |
| **AM-2** | 当前群并入 `useAgentWorkspace`，竞态守卫收进 composable | 已完成：新增 `loadFirstPage` / `refreshFirstPage` / `refreshTagsAndPrefetch` / `reset`，`loadMore` 带 seq 守卫；页面的 `groupPages` 删除，侧栏直接传 `ws.pages` | — |
| **AM-1~13 总体** | 页面 ≤450 行 | `AgentManager.vue` 446 行（原 1298）。除计划列出的 `useActiveRunsPoll / useAgentTagEditor / useStarterThread / useQueryPatch` 外，另拆出 `useAgentDirectory`（助手目录与显示名）、`useWorkspaceView`（只读衍生视图）、`useThreadCapabilities`（协作判定与能力提示 + 重命名弹窗状态）、`useThreadNavigation`（切分区 / 选会话 / 新建 / 删除 / 加载消息）、`useAgentSender`（发送 / 重试 / 停止 / 人工回复）、`useHumanTasks`、`useAgentRunLayer`，以及 `utils/chat-messages.ts`、`ThreadRenameModal.vue` | 仅靠计划列出的 4 个抽取只能降到约 750 行，需要再按职责拆出会话导航与发送层才能达标；`vm.*` 附录 A 的符号全部保留，`AgentManager.test.ts` 断言未改 |
| **A-12** | converters 入参 `WireRecord`，字段逐个转换 | `Record<string, any>` / `raw: any` 全部改为 `WireRecord`（`unknown` 值）；`services/` 显式 `any` 由 32 处降为 **0**。字段读取走 `wire.ts` 的 `pick / orDefault / orUndefined / orElse / objectOf`，语义与原 `raw.x` / `??` / `\|\| undefined` / `\|\| d` 一一对应；仅在需要类型收窄的位置使用（`data.ts`、`platform.ts` 的 llm/auth/user，`agent-stream.ts` 的工具帧） | 若把全部字段改成 `String()` / `Number()` 会把缺失的 `undefined` 变成 `''` / `0`，属于行为变化；读取器保持透传语义，并在 `tests/paths-wire.test.ts` 固化 |
| **CV-3** | 删 `errorText`，改 `errorMessage(e, t('error.requestFailed'))` | 已删除。**注意**：原 `errorText` 对「非空字符串」原样返回，改后字符串也走 i18n 兜底（`errorMessage` 只认 `Error`）。对应用例已改为断言此口径 | 与计划写的替换一致，属于计划允许的可见变化 |
| **P-14** | 12 个 setter → `patch` / `patchQuery` | 只收了真正「单字段、无联动」的 3 个：新增 `patch(partial)`、`unNone`、`toLimit`；`setLimit` / `setSpan` 内联为 `patch({...})`，`setField` 改为 `patch({ [key]: unNone(v) })`。其余 setter（`setGroupField` / `setUnit` / `setBin` / `setSortKey` / `setSortDir` / `setMetricKind` / `setMetricField`）带联动或格式化，保留具名函数 | 阅读全文后确认原计划「全是同构单行」不成立 |
| **T-1** | 按被测对象合并 | 已合并并删除旧文件，用例数前后逐对核对一致：`useAgentConversation-edge` → `useAgentConversation.test.ts`；`template-inline-handlers-2` → `template-inline-handlers.test.ts`；`capability-gap-http` → `http-api.test.ts`；`user-login-branches` → 拆为 `UserFormModal.test.ts`（2 条）与 `login-auth-components.test.ts`（3 条）。`DataBrowserExtra` / `DataBrowserFinal` 按主题改名为 `DataBrowserRefresh` / `DataBrowserBoard`（三个文件的 `setup()` 夹具参数不同，强行合并会引入可选参数分支，故只改名） | — |
| **T-3** | 核对 `uiStubs` 是否桩了交互组件 | **已完成**：reka 交互类桩（`Select*` / `Switch` / `Tabs*` / `Popover*` / `Command*` / `Combobox*` / `ToggleGroup*` / `Slider` / `Dialog*` / `Sheet*` / `AlertDialog*`）全部从 `uiStubs` 移除，另删无引用的 `CardFooter / FieldTitle / Avatar*`；`uiStubs` 现 71 个，全部是展示 / 布局类或应用层壳（`SbModal / ConfirmAction / SbEmptyState / ProjectScope / PageContainer / TablePager`）。约 30 个测试文件改用真实组件，新增 `installJsdomPolyfills`、`real*Stubs` 与 `clickTab / openPopover / pickSelectOption / clickAlertDialog / closeDialog` 等辅助（见 `ui/AGENTS.md`）。改造暴露的真实问题：① `UserFormModal` 原测试创建了不存在的角色 `info`；② 多处「`if (el.exists()) click()`」式静默跳过用例，已改为确定性断言（如 `CronJobModal` 现在断言「换函数文件会清空已选导出函数」）；③ `Sandboxes` 来源列用例原靠 Select 桩把选项内联进页面文本才通过，已改读表格行；④ `ConfirmAction` 取消 / 受控确认会多发 `cancel`（无消费者，已在测试里锁定现状，未改组件）；⑤ `SbModal` 的「右上角关闭」只请求关闭不发 `cancel`，而原桩会多发。**用例数**：1462 → 1494（净增 32，含新测试与合并）。 | |
| **N-3 / A-16** | — | 按约定留到下一轮 | — |

### 8.2 新增 / 删除的主要文件

- 新增 composables：`useAgentDirectory`、`useAgentRunLayer`、`useAgentSender`、`useHumanTasks`、`useThreadCapabilities`、`useThreadNavigation`、`useWorkspaceView`、`useFilterChips`、`useProjectReload`、`useCanWrite`。
- 新增工具与组件：`utils/chat-messages.ts`、`components/modal/ThreadRenameModal.vue`。
- 已删除：`http-api-converters.ts`、`lib/status.ts`（移至 `utils/status.ts`）、`package-lock.json`、`pnpm-lock.yaml`、死组件与对应测试。

### 8.3 余量清单与复核（v3，2026-11-23，未动源码）

复核门禁：`npx tsc --noEmit` 0 错误；`npx vitest run` 155 文件 / 1494 用例全绿；`npx vitest run --coverage` 语句 97.76% / 分支 93.73% / 函数 91.83%（阈值 90，退出码 0）。`yarn build` 本轮未跑（会写 `dist/`），以 §8 的记录为准。

图例同后端计划：✅ 已完成　◐ 部分完成　⏭ 有意不做或留到下一轮。

| 条目 | 状态 | 余量 |
| --- | --- | --- |
| A-16 / N-3 | ⏭ | 按约定留到下一轮；`test/api-mock.ts` 652 行 |
| AM-10 | ⏭ | 有意保留（§8.1） |
| CV-2 | ⏭ | 可选；`useAgentConversation.ts` 已降到 282 行，不再需要 |
| P-9 | ◐ | 只有 7 个页面（8 处）改用 `:state`；**28 个组件调用点仍逐个透传 6 个字段**：`components/databases/**` 11 处（含 5 个 KV 编辑器）、`components/modal/**` 5 处、`components/settings/**` 4 处、`components/models/**` 2 处、agent / docs / sandbox 各 1 处、`pages/{Dashboard,Logs,S3Manager}.vue` |
| P-14 | ◐ | 按 §8.1 的结论收了 3 个，其余保留 |
| U-2 | ◐ | `maskSecret` / `isDnsSafeProjectId` / `compatibleTypes` 已删；`src/docs/catalog.ts` 的 `isLargeDoc` 仍只被 3 个测试文件使用 |
| L-1 | ✅ | 疑似未用 key 从 ≈107 降到 1（`common.noMatch`）。`pages.home.starters.*.prompt`（`starter-prompts.ts` 用 `${base}.prompt`）、`components.agentWorkspace.builtinTags.*`、`components.sandboxTerminal.unavailable.*` 是动态拼接，不是死 key |
| C-1 | ◐ | 计划编号注释 282 → 113 处 / 68 文件，未到「≤60」的预估；剩余多为单锚点 |
| D-3 | ◐ | `ui/AGENTS.md`、`ui/README.md` 已对齐；**根 `AGENTS.md:41、138` 仍写「95% 阈值」「functions 94.1% / branches 94.92% 未达标」**，与现状相反；同文件「当前保留 planv6.5 – planv6.7」也没有提 planv6.8 |
| §6.3 指标 | ◐ | `AgentManager.vue` 446 行达标；顶层 ref / computed 16 个（目标 ≤12，原 ≈30）、函数 6 个（达标）；`http-api.ts` 942 行，是唯一超过 800 行的业务文件（见 §9.1 HA-1） |

## 9. 第二轮评估（实施后，只评估未动代码）

方法：行窗口哈希找重复块（7 行窗口；排除 `components/ui`、`locales`、`docs`、测试），逐个读码确认，`rg` 计数。缩写：P = 优先级，R = 风险。**以下均未实施。**

本版本关闭时：§8.3 的 ◐ / ⏭ 项与 §9.1 未做的候选并入 `planv6.7/legacy-plan-consolidation-plan.md` §2 的锚点表（同 v6.4 → v6.7 的做法）。

### 9.1 值得做的候选

| ID | P / R | 位置 | 问题 → 方案 | 证据 |
| --- | --- | --- | --- | --- |
| KV-1 | P2 / 低 | `components/databases/kv/editors/{KvHash,KvList,KvSet,KvString,KvZSet}Editor.vue`（共 675 行） | 五个编辑器各写一遍同样的骨架：`useLoadState({ fallback })` 解构 6 个字段、`load()` 包 `run()`、**14 个写操作**都是 `try { await api.kv.exec(...); await load(); emit('changed') } catch (e) { toast.error(errorMessage(e, t('…Failed'))) }`、`watch(kvKey, load, { immediate: true })`，模板再透传 6 个字段。→ `composables/useKvEditor.ts`：入参 `{ projectId, kvKey, fallback, fetch }`，返回 `{ state, load, mutate(req, failKey, successKey?) }`；写操作各缩成 1–3 行，模板改 `:state="state"`。**有意差异保留在各编辑器里，不强行统一**：`KvStringEditor` 的 `save` / `incr` 成功后不 `load()`（用返回值原地更新）、Hash 的 `saveEdit` 不提示成功、List / ZSet 只取前 200 条 | 读过五个文件的 script 段；窗口哈希命中 ≈40 行 × 5；`KvEditors.test.ts` 兜底 |
| AR-1 | P2 / 低 | §8.3 P-9 余量的 28 处 | 同 P-9 的做法，组件侧分 3 小批迁到 `:state="load"`（KV 编辑器随 KV-1 一起）；每处省 6 行 props，三态语义不变 | `rg -c ':show-skeleton=' src` = 28 |
| DOC-1 | P0 / 无 | 根 `AGENTS.md:41、138` | 覆盖率阈值与基线写反（实际 90% 且达标）；「当前保留 planv6.5 – planv6.7」漏 planv6.8。→ 只改文字，并入 DOC 类下一批 | `rg '95%\|94\.' AGENTS.md` |
| HA-1 | P3 / 低 | `services/http-api.ts`（942 行，26 组） | §6.3 的「业务文件 ≤800」只差这一个，且与已拆的 converters / types 不对称。→ 按域拆 `http-api/{agent,data,platform}.ts`，各声明为 `Pick<Api, …>`，`http-api.ts` 只做组装并保持导出名不变 | 读过文件头与分组行；`http-api*.test.ts` 断言 `httpApi`，不受影响 |
| CL-1 | P3 / 无 | `locales/*.ts` 的 `common.noMatch`；`src/docs/catalog.ts` 的 `isLargeDoc` | 两棵树同删 1 个死 key；`isLargeDoc` 改用目录里已有的「大文档」元数据（`largeMeta`）或移入测试辅助 | §8.3 |
| TEST-1 | P2 / 无 | `useThreadNavigation.ts`（231 行）、`useStarterThread.ts`（95 行）、`useWorkspaceView.ts`（94 行）、`useAgentRunLayer.ts`（57 行） | 这 4 个是 P4 拆分时新出现的 composable，`tests/` 里没有任何文件直接引用，只经 `AgentManager.test.ts` 整页挂载间接覆盖；其中 `useThreadNavigation` 负责「跨群先 `selectSection` 再 `selectThread` / 新建 / 删除」的调用顺序，并消费 `useAgentWorkspace` 的 seq 守卫返回的空结果（守卫本身在 `useAgentWorkspace.test.ts` 有测），整页测试很难造出「取数期间切群」的时序。→ 按模块各补一个小测试文件，至少覆盖：切群时 `selectSection` 先于 `selectThread`、守卫返回空值时页面状态不变、starter 深链只触发一次 | `rg -l '\b<name>\b' tests` = 0（其余 7 个新 composable 都有直接测试） |

建议顺序：DOC-1 / CL-1 / TEST-1（无风险）→ KV-1（含 5 处 AR-1）→ AR-1 其余 23 处 → HA-1。每项动手前按 §6.1 做 `rg -w '<Symbol>' src tests` 核对。

### 9.2 评估后不做的（下次不必重评）

- 窗口哈希命中的「`useCronJobForm` ↔ `CronJobModal` 26 行」「`useThreadNavigation` ↔ `AgentManager.vue` 18 行」：是同一个 composable 的返回清单与解构清单，不是重复知识。
- `CollectionSchemaModal` / `DocumentKvModal` 的 props + emit 声明（各 14 行）、`ModelDefaultsPanel` / `ApiPreview` 的导入段：入参或依赖恰好相同，没有逻辑重复，不抽。
- `useQueryPatch`（25 行，有测试）只有 `AgentManager` 一个使用者：AM-13 设想的 `ApiPreview` 复用不成立（它用 sessionStorage 记忆，不是 query）。不内联回页面。
- `useAgentCollaboration.buildHandlers`（109 行）：CV-2 的 `forward` 包装可省约 14 行，但会把十几个回调的透传藏进高阶函数；两个会话 composable 已共用 `utils/tool-cards`，不再合并 handler 绑定。
- `AgentManager.vue` 的 11 个拆分 composable：数量偏多但每个职责单一（§8.1），不再合并；缺直接测试的 4 个见 §9.1 TEST-1。
- `components/ui/avatar`（零引用）：维持附录 C。

## 10. 第二轮候选的执行结果（v4，2026-11-23，激进策略）

§9.1 的候选已全部实施，**尚未提交**。

| ID | 状态 | 落点 / 与计划的差异 |
| --- | --- | --- |
| DOC-1 | ✅ | 根 `AGENTS.md` 的测试命令注释、「测试要求」基线改为 90% 阈值 + 实测数字，「当前保留 planv6.5 – planv6.8」并说明本目录是 simplify-code 的事实来源；`ui/AGENTS.md`、`ui/README.md` 同步 `http-api` 的拆分、composable 清单与 `state` 用法 |
| CL-1 | ✅ | `common.noMatch` 两棵树删除；`src/docs/catalog.ts` 的 `isLargeDoc` 删除：`docs-catalog.test.ts` 改为断言「大文档的 `loadMarkdownRaw` 为 null、普通文档是原文」，`docs-content.test.ts` 用常量 `LAZY_DOC` 跳过，`DocsArticle.error.test.ts` 的 mock 去掉该键 |
| TEST-1 | ✅ | 新增 `useThreadNavigation`（7）、`useStarterThread`（6）、`useWorkspaceView`（5）、`useAgentRunLayer`（2）四个测试文件；断言打在调用顺序（跨群先切分区再选会话）、竞态丢弃（守卫返回 null 时状态不变、创建期间切项目不插入）、`sending` 并集与运行指示的合成上 |
| KV-1 | ✅ | 新增 `composables/useKvEditor.ts`：`read`（读命令）/ `parse` / `apply` / `changed` + `mutate(body, { failKey, successKey?, onSuccess?, reload? })`。比计划多把**读命令**也收进 composable，五个编辑器不再 `import { api }`；编辑器保留顶层 `load` 绑定（测试经 `vm.load` 访问）。有意差异保留：String 的 `save` / `incr` 用 `reload: false` + `onSuccess` 原地更新、Hash 的 `saveEdit` 不提示成功、List / ZSet 只取前 200 条。五个文件 675 → 553 行，模板改 `:state="state"`（`showEmpty` 恒 false，编辑器自画空态）。新增 `useKvEditor.test.ts`（5 用例：按键加载、失败只提示、成功顺序 `提示 → onSuccess → 重读 → changed`、失败不触发后续、`reload=false`） |
| AR-1 | ✅ | 19 个 `useLoadState` 调用点改为 `const loadState = useLoadState(…)` + 只解构别处仍要的字段，区域传 `:state="loadState"`；`KvKeyListPanel` 的 6 个布尔 prop 改成单个 `state`（`KvPanel` 不再解构 6 个字段来转发），两处测试夹具同步。`AsyncRegionState` 类型移到 `src/components/async-region-state.ts`（`.vue` 的具名类型不能被 `.ts` / 其它 `.vue` 引用），`SbAsyncRegion.vue` 与 `useKvEditor.ts` 共用。**未迁（不适用）**：`AgentWorkspaceSidebar`、`CollectionSchemaModal`、`DataBrowser`、`ApiPreview`、`FunctionTriggerModal`、`BiDetailTable` / `BiWidgetCard` 的区域用的是手写 ref 或手算布尔值，不是 `useLoadState` |
| HA-1 | ✅ | `services/http-api/{platform,data,agent}.ts` 各 `satisfies Partial<Api>`（编译期按 `Api` 逐组校验），`http-api.ts` 只剩 `{ ...platformApi, ...dataApi, ...agentApi }`（15 行）；942 → 474 + 286 + 190 + 15。`http-api*.test.ts` 仍从 `@/services/http-api` 取 `httpApi`，未改 |

### 10.1 门禁实测

| 检查 | 结果 |
| --- | --- |
| `npx tsc --noEmit` | 0 错误 |
| `npx vitest run --coverage` | 160 文件 / **1519** 用例（+25）；语句 97.79% / 分支 93.90% / 函数 92.04%（阈值 90，退出 0） |
| `yarn build` | 通过（500 kB chunk 提示为既有警告） |
| §6.3 指标 | 业务文件 ≤ 800 行：达成（最大 `services/types/platform.ts` 671）；`SbAsyncRegion` 散装 6 字段透传：37 → 0（仅剩上述手写状态区域）；只给测试用的导出：`isLargeDoc` 已清 |

### 10.2 仍未做

- A-16 / N-3（`api-mock.ts` 从 `httpApi` 结构派生）、CV-2、AM-10：按 §8.3 的结论保留。
- `yarn build` 的产物未同步到 `internal/web/dist`（由 `./build.sh` 负责）。
- 浏览器手工冒烟（§6.2）未做。
