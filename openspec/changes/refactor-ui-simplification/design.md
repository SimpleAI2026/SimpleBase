# Design

依据：`skills/simplify-code/SKILL.md`（Refactor 模式）及 `references/refactoring.md`、`references/frontend.md`。评估基于 2026-11-26 工作区（HEAD `c7203ba` 加未提交改动），用只读方式采样：文件行数、超 150 行的函数（tree-sitter outline）、跨文件同名私有函数、列表页样板模式四条线筛选。这是采样结论，不代表 `ui/` 已没有其他问题。

## 冻结契约（Frozen surface）

- 路由路径与 redirect 映射（`router/index.ts`）、`meta.titleKey` / `meta.requiresRole`。
- 组件对外 API：props / emits 名称与载荷、slots、`v-model` 参数、`data-*` 测试钩子（`data-plan-*`、`data-tag-conflict`、`data-icon` 等）。
- i18n：key 树结构（`zh-CN.ts` / `en-US.ts` 两棵树 key 集合一致由 `tests/i18n.test.ts` 锁定）、`useI18n()` / `t()` / `resolveText()` 的使用边界。
- API 层：`api.*` 唯一入口、`http-api/{platform,data,agent}.ts` 分域、converters 映射函数名、`types.ts` 桶文件、SSE 帧格式与分发语义（`agent-stream.ts` 的 `_sse_id` 去重、`settled` 后 close 为空操作等既有注释即契约）。
- 三态语义：160ms 占位延迟、`emptyOn` 空资源判定、`toast.error(errorMessage(...))` 失败提示模式。
- 本地存储键：`sb_settings_v1`、`sb_access_token` / `sb_refresh_token` / `sb_api_key`、各 BI 看板持久化键。
- 测试基建契约：`helpers.ts` 的 stub 集合（`uiStubs` / `real*Stubs`）与交互辅助、测试不桩 reka 交互组件的约定。
- 构建与门禁：`vite.config.ts` 不改（coverage 90 阈值、include/exclude 清单）、`yarn typecheck` 0 错误、`yarn test` 全绿、`yarn build` 通过。
- `components/ui/**` 视为 fork 库代码：本变更完全不触碰。

## 可变内部面（Free surface）

composable 内部函数边界与文件划分、组件文件归属目录、私有 helper 位置、页面内脚本组织、测试文件的分组与命名。搬移组件前确认无按路径的源码断言测试（已核对：`modal-input-layout.test.ts` 只扫 `modal/` 下输入布局约定，分域迁移后需同步其路径清单）。

## 候选清单

优先级：P1 = 维护负担大、收益明确；P2 = 收益有界的局部收敛；P3 = 可选项。分类 apply / propose / skip 依据 `references/assessment.md`。

| ID | P | 位置 | 问题 → 方案 | 风险 | 分类 |
| --- | --- | --- | --- | --- | --- |
| A-01 | P1 | `composables/useAgentCollaboration.ts`（557 行单函数，22 个内部函数） | 同文件内把闭包拆成纯函数段：事件应用（`upsertTask` / `applyTaskEvent` / `ensureTurnContainer`）抽到 `agent-collab-events.ts`、计划与确认（`applyPlan` / `buildHandlers` 的 plan 分支、`confirmPlan`）抽到 `agent-collab-plan.ts`，以 `createXxx(deps)` 工厂注入共享 refs，保持返回对象 24 个字段完全不变 | 中：闭包变量众多，拆分即改依赖注入；先补「同 seq 重放 / 计划确认后状态」特征测试 | apply（返回面不变，`useAgentRunLayer` 与测试都消费同一返回对象） |
| A-02 | P2 | `services/agent-stream.ts` `openAgentRunStream`（231 行） | 按 `obj.type` 帧族拆私有分发函数：`applyIdentityFrame`（run/turn/_sse_id）、`applyPlanFrame`（plan/plan_confirmation/task/human）、`applyTokenFrame`（token/chunk/tool_call_delta）、`applyToolFrame`（tool_* / confirmation_*）、`applyMetaFrame`（thinking/usage/end/error），分发顺序与短路语义逐分支保持 | 中：帧分发顺序即行为；`tests/agent-turn-stream.test.ts`、`agent-skills-stream.test.ts` 已有覆盖，先确认每族都有用例再动 | apply |
| A-03 | P2 | `components/cronjob/useCronJobForm.ts`（302 行单函数） | 按职责抽段：字段定义与校验、调度字段（复用既有 `CronJobScheduleFields` 语义）、保存提交；返回字段与 `CronJobModal` 对 `vm.form` 的访问路径不变 | 低：已有 `CronJobModal.test.ts` 锁表单行为 | apply |
| A-04 | P2 | `composables/useAgentWorkspace.ts`（242 行）、`useThreadNavigation.ts`（197 行）、`useAgentConversation.ts`（241 行） | 各自按注释段抽内部函数组；导出面不变（`tests/agent-workspace-composables.test.ts`、`useThreadNavigation.test.ts` 直接消费返回对象） | 中：闭包拆分同 A-01 | apply（排在 A-01 之后，复用其拆法） |
| B-01 | P1 | `TrendChart.vue` 与 `utils/bi-chart.ts` 重复的 `cssVar` / `withAlpha` / `compact`；`useBiTheme.ts` 的 `cssVar` | 收敛到 `utils/chart-theme.ts`（导出 `cssVar` / `withAlpha` / `compact` / `themeColors`），`TrendChart`、`bi-chart`、`useBiTheme` 改为引用；两份实现的 fallback 值差异（`compact` 的阈值、`withAlpha` 的正则）以逐字相同为前提，不同则在合并时保留参数化 | 低：纯函数，`bi-chart.test.ts` 已锁定输出 | apply |
| B-02 | P2 | `ConnectionPanel` / `ModelDefaultsPanel` / `SandboxDrawer` 三份 `copyText` 薄壳 | 保留薄壳但统一签名（`copyText(text, okMsg, failMsg)` 直接透传 `utils/clipboard`），消除三份各自拼 label 的差异；不新增公共组件 | 低 | apply |
| B-03 | P3 | `SchemaRowsModal` 与 `BiWidgetCard` 的 `cellText` | 两份实现语义相同（null/undefined → '-'，对象 JSON 化）；抽到 `utils/format.ts` 导出 `cellText`，两处引用 | 低 | apply |
| C-01 | P1 | `components/modal/` 21 个文件混 8 个域 | 按域归位：cronjob（CronJobModal / CronJobRunsModal）、databases（CreateCollectionModal / DocumentListModal / DocumentKvModal / SqlWorkModal / SqlResultPanel）、agent（AgentTagModal / AgentGroupMembersModal / ThreadRenameModal / McpServerModal 归 settings 或 agent，按引用方定）、platform（CreateProjectModal / UserFormModal / LoginModal）、gofunction（GoFunction* 3 个）、s3（S3PreviewModal）、sandbox（SandboxCreateModal）；`SbModal` 留在顶层通用件。同步更新全部 import、`tests/modal-input-layout.test.ts` 扫描路径、`ui/AGENTS.md` / `README.md` 目录职责 | 中：改动面约 60 处 import（含 tests 23 处），一次性原子迁移 | propose（目录结构属于团队约定，批准后执行） |
| C-02 | P2 | `components/ai/AiChatComposer.vue`、`components/chat/MessageScroller.vue` 只被 agent 域引用 | 移入 `components/agent/`，更新 import；`ai/`、`chat/` 目录删除 | 低：引用方仅 3-4 处 + 测试 | propose（与 C-01 同批决策） |
| C-03 | P3 | `components/async-region-state.ts` 是组件目录里唯一的类型文件 | 移到 `composables/` 或保持现状（`SbAsyncRegion` 与页面都引用它）；若移动需同步 `README` 的类型说明 | 低 | skip（现状可用，收益小于改动面） |
| D-01 | P2 | 七个列表页样板：`usePagination` + `useLoadState` + `useProjectReload` + `useCanWrite` + `SbListCard` + `SbRefreshButton` + ConfirmAction 删除 + toast 模式 | 不强行抽象「列表页框架」（页面差异真实存在：admin 只读、静态空态、副列表加载）。只收敛两种确定重复：①`toggling/invoke/remove` 的 `Set<string>` 行内操作锁抽成 `useRowAction`（`run(id, task)` 自动加锁 / finally 解锁 / toast 错误）；②错误 toast 的 `errorMessage(e, t(...))` 保持不变（已是统一模式） | 中：过度抽象会伤可测性；收敛边界见左侧 | apply（只做 ①；页面骨架不动） |
| D-02 | P3 | `pages/AgentManager.vue`（553 行）继续瘦身 | E3 已拆过一轮；剩余大头是模板区（208 行）。把侧栏（`AgentWorkspaceSidebar` 已存在）之外的群/会话切换头部再抽一个展示组件，脚本编排保持 | 中：模板抽取影响 DOM 钩子 | propose（收益 / 风险比一般，排最后） |
| T-01 | P2 | `src/test/api-mock.ts`（674 行单桶，1 处 `as any`） | 按域拆 `src/test/api-mock/{platform,data,agent}.ts` + `index.ts` 组装；`resetApiMocks` / `applyApiDefaults` 语义不变；第 650 行 `as any` 一并类型化（`kv.exec` 的测试专用签名拆到 agent 段） | 中：162 个测试文件里 83 个 import 它；原子迁移一次完成 | propose（测试基建，批准后执行） |
| T-02 | P3 | `tests/` 顶层 162 个平铺文件 | 按域建子目录（pages/ components/ composables/ services/ …）分组 | 中：vitest glob 无影响，但 CI 与 IDE 习惯会变 | skip（无功能收益，纯组织偏好） |

### 明确不在本变更内

- `components/ui/**`：fork 库代码，只在「删除零引用组件前全局确认」规则下被动响应，本变更不主动改。
- BI 看板视觉与增强（R3/R4）：`complete-bi-dashboard` 负责。
- E2（工具栏间距 / 状态文案统一）、E4（截图回归）：`close-legacy-backlog` 负责。
- 首页引导（starter 卡）：`complete-home-starter-delivery` 负责。
- `locales/zh-CN.ts`（2283 行）/ `en-US.ts`（2270 行）：大是因为文案多，不是结构问题；`i18n.test.ts` 已锁两棵树一致性。不按页面拆文件（会破坏「权威树」约定）。
- 性能优化（`index-*.js` 1007kB / `GoFunctions` 2.7MB chunk）：属构建产物优化，不是等价重构；且改 `vite.config.ts` 违反冻结契约。列入 skip，若要做另开变更。

## 迁移策略

- **闭包拆分（A 系列）**：每项先确认特征测试覆盖（存在即引用，缺失先补），再拆。拆分方式统一为「同文件或相邻文件内工厂函数注入依赖」，不引入类、不引入新的响应式模式。每个 composable 一个检查点。
- **纯搬移（C 系列）**：目录迁移一次性原子提交（搬移 + 全部 import 更新 + 测试路径同步 + 文档更新在同一个检查点），不保留转发文件，方便按 rename 对比 diff。
- **重复收敛（B 系列）**：先写「三处实现输出一致」的对照断言，再合并到唯一实现。
- **样板收敛（D 系列）**：只抽有明确语义边界的 `useRowAction`；抽完后逐页替换并跑该页测试。
- 每个检查点完成后跑 `yarn typecheck` + 相关测试；每批完成后跑全量 `yarn test` + `yarn build`。

## 验证

| 范围 | 命令 |
| --- | --- |
| 静态 | `yarn typecheck`（含 tests/，基线 0 错误） |
| 单测 | `yarn test`（162 文件 / 1537 用例全绿；coverage 不低于基线：lines 97.53% / functions 91.42% / branches 93.31%，且任何维度 ≥90% 阈值） |
| 构建 | `yarn build` 通过；产物 chunk 尺寸与基线相比无异常增长（±5% 以内视为等价，`GoFunctions` / `index` 等 vendor 主因不变） |
| i18n | `tests/i18n.test.ts` 全绿（两棵树 key 一致、无空文案、无中文残留） |
| 源码断言测试 | `modal-input-layout.test.ts`、`table-align.test.ts`、`ui-state-attributes.test.ts`、`loading-surfaces.test.ts` 在目录迁移后同步路径清单并通过 |

基线（2026-11-26 实测，HEAD `c7203ba` + 未提交改动）：

- `yarn typecheck`：0 错误。
- `yarn test`：162 文件 / 1537 用例全部通过；coverage lines 97.53%（27434/28127）、functions 91.42%（1737/1900）、branches 93.31%（9018/9664）。
- `yarn build`：6.62s 通过；主要 chunk：`index` 1007.63kB（gzip 370.74kB）、`GoFunctions` 2692.38kB（gzip 698.76kB，monaco 主因）、`DataBrowser` 200.68kB、`AgentManager` 113.80kB。
- `ui/src` 生产代码（除 `components/ui/`）211 个文件 39048 行；tests/ 162 个文件。
- 已知唯一 `as any`：`src/test/api-mock.ts:650`（T-01 处理）。

## 回滚

只撤销当前检查点自己的改动；不整目录 restore，不 `reset --hard`。目录迁移（C 系列）如出问题整体 revert 该提交（rename diff 使 revert 干净）。纯搬移出现回归时先确认是否混进了逻辑改动。

## 实施记录

（实施时逐批回填：完成 / 部分 / 保留 / 取消，实际执行的命令与未验证的边界；不提前打勾。）
