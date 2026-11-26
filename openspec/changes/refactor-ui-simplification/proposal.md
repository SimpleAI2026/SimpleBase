# Proposal

## Why

`ui/` 在多轮功能交付（planv6.x）后功能面已稳定（1537 个用例、coverage 97.5%），但积累了结构性负担：

- **超大单元**：`useAgentCollaboration` 单函数 557 行（闭包内 22 个内部函数）；`agent-stream.ts` 的 `openAgentRunStream` 231 行、`useCronJobForm` 302 行、`useAgentWorkspace` 242 行、`useThreadNavigation` 197 行、`useAgentConversation` 241 行。`AgentManager.vue` 已在 E3 拆过一轮仍剩 553 行。
- **同构重复**：`TrendChart.vue` 与 `utils/bi-chart.ts` 各有一份 `cssVar` / `withAlpha` / `compact`；三处弹窗各有一份带 toast 的 `copyText` 薄壳；`SchemaRowsModal` 与 `BiWidgetCard` 各有一份 `cellText`；列表页「分页 + 三态 + 项目重载 + canWrite + 刷新 + 删除确认」样板在 CronJobs / Databases / GoFunctions / FunctionTriggers / S3Manager / Logs / Sandboxes 七页近乎相同。
- **归位漂移**：`components/modal/` 是跨域大杂烩（21 个文件混 8 个域）；`ai/AiChatComposer`、`chat/MessageScroller` 只被 agent 域引用；顶层 `async-region-state.ts` 是唯一躺在组件目录里的 `.ts`。
- **测试脆弱点**：`src/test/api-mock.ts` 674 行单一 `vi.fn()` 桶（第 650 行有一处 `as any`），全部测试共用同一实例，重置与类型安全都靠约定。

本变更按 `skills/simplify-code`（Refactor 模式 + `references/frontend.md`）把上述问题整理成可分批执行、每批可单独回滚的等价重构计划。

## What Changes

- 只做等价重构：不改路由路径、组件 props/emits 契约、i18n key、API 路径与请求形状、本地存储键（`sb_settings_v1` 等）、三态语义（160ms 延迟）、coverage 阈值与脚本命令；不新增依赖、不改 `package.json` / `tsconfig.json` / `vite.config.ts`。
- 先记录基线、确认特征测试存在，再搬移或抽取代码。每个检查点只处理一项职责；纯搬移（文件移动、重命名）与逻辑抽取分开提交。
- 候选分 apply / propose / skip 三类。涉及测试基建改造（api-mock 拆分）、目录批量迁移（modal 分域）影响面大的列为 propose，需要单独批准后再执行。
- E2（工具栏间距 / 状态文案）、E4（截图回归）、BI 看板 R3/R4 视觉与增强由 `close-legacy-backlog` 与 `complete-bi-dashboard` 负责，本变更不重复登记。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

（无）

本变更不改变任何对外可见行为，`.openspec.yaml` 设置 `skip_specs: true`。实施中如果发现某项必须改变契约（如组件 emits 形状、i18n key、路由），停在 propose，另开变更补 delta spec。

## Impact

- 受影响代码：`ui/src/composables/{useAgentCollaboration,useAgentConversation,useAgentWorkspace,useThreadNavigation,useCronJobForm 所在的 components/cronjob,useBiDataSource}`、`ui/src/services/agent-stream.ts`、`ui/src/components/TrendChart.vue`、`ui/src/utils/bi-chart.ts`、`ui/src/components/modal/**`、`ui/src/components/{ai,chat}`、`ui/src/components/async-region-state.ts`、`ui/src/pages/{CronJobs,Databases,GoFunctions,FunctionTriggers,S3Manager,Logs,Sandboxes,AgentManager}.vue`、`ui/src/test/api-mock.ts`、`ui/src/stores/auth.ts`。
- 需要同步的文档 / 门禁：`ui/AGENTS.md`（目录职责表、composable 清单）、`ui/README.md`（目录结构图、测试文件数）、`ui/tests/**`（import 路径、源码断言型测试如 `modal-input-layout.test.ts`、`table-align.test.ts` 扫描路径）。
- 不影响：`components/ui/**`（本地 fork 库代码，只增删不改风格）、后端 API、`openspec/specs/console-ui` 既有 requirement、Go 构建产物（`internal/web/dist` 为生成物不动）。
