# Tasks

## 1. 第 0 批：基线与特征测试确认

- [ ] 1.1 复核基线（不覆盖未提交改动）：`yarn typecheck` / `yarn test`（含 coverage 四项数值）/ `yarn build`（记录主要 chunk 尺寸）；与 design「基线」一节对比，不一致先更新 design
- [ ] 1.2 A-01 前置：确认 `useAgentCollaboration` 的特征测试覆盖同 seq 重放、计划确认后状态、`syncTasks` 增量合并；缺失的在 `tests/useAgentCollaboration.test.ts` 补齐
- [ ] 1.3 A-02 前置：确认 `agent-stream` 每族帧（identity / plan / token / tool / meta）都有正反用例（`agent-turn-stream.test.ts`、`agent-skills-stream.test.ts`）；缺的补齐，特别是 `_sse_id` 重复丢弃与 `settled` 后 close 空操作
- [ ] 1.4 B-01 前置：写对照断言——`TrendChart` 与 `bi-chart` 的 `cssVar` / `withAlpha` / `compact` 在相同输入下输出一致（含 fallback 值差异逐字核对），放进 `tests/bi-chart.test.ts`
- [ ] 1.5 D-01 前置：盘点七个列表页的行内操作锁用法（`toggling` / `invoking` / `removing` 各自的加锁 / 解锁 / 错误处理），确定 `useRowAction` 签名不丢任何一页的细节（如 FunctionTriggers 的 admin 空列表短路）

## 2. 第 1 批：agent 域大单元拆分（A 系列）

- [ ] 2.1 A-01 `useAgentCollaboration` 按 1.2 拆出 `agent-collab-events.ts` / `agent-collab-plan.ts`（工厂注入依赖），返回对象 24 字段不变；跑 `useAgentCollaboration.test.ts` + `agent-collab-ui.test.ts` + `useAgentRunLayer.test.ts`
- [ ] 2.2 A-02 `openAgentRunStream` 按 1.3 拆五个帧族分发函数，分发顺序逐分支保持；跑 `agent-turn-stream` / `agent-skills-stream` / `agent-errors` 相关用例
- [ ] 2.3 A-03 `useCronJobForm` 按字段定义 / 调度 / 提交三段抽函数；`vm.form` 访问路径不变；跑 `CronJobModal.test.ts` + `CronJobs.test.ts`
- [ ] 2.4 A-04 `useAgentWorkspace` / `useThreadNavigation` / `useAgentConversation` 各自抽内部函数组；跑各自测试 + `AgentManager.test.ts`
- [ ] 2.5 批后验证：`yarn typecheck` + 全量 `yarn test`，coverage 不低于批前

## 3. 第 2 批：重复收敛（B 系列）

- [ ] 3.1 B-01 新建 `utils/chart-theme.ts`，合并 `cssVar` / `withAlpha` / `compact` / `themeColors`；`TrendChart` / `bi-chart` / `useBiTheme` 改引用；1.4 的对照断言转为对唯一实现的直测
- [ ] 3.2 B-02 三份 `copyText` 薄壳统一签名并直接透传 `utils/clipboard`；跑 `ConnectionPanel` / `ModelSettings` / `SandboxDrawer` 相关测试
- [ ] 3.3 B-03 `cellText` 合并到 `utils/format.ts` 导出；`SchemaRowsModal` / `BiWidgetCard` 引用；跑 `SchemaRowsModal` / `BiWidgetCard` 测试 + `utils.test.ts`
- [ ] 3.4 批后验证：`yarn typecheck` + 全量 `yarn test`

## 4. 第 3 批：归位迁移（C 系列，propose——批准后执行）

- [ ] 4.0 等待批准：C-01 modal 分域 + C-02 ai/chat 并入 agent 的目录方案（含 `McpServerModal` 归属判定：按引用方定 settings 或 agent）
- [ ] 4.1 C-01 `components/modal/` 21 文件按域迁移（cronjob / databases / agent / platform / gofunction / s3 / sandbox；`SbModal` 留顶层）；同一提交内更新全部 import（src 约 36 处 + tests 约 23 处）、`tests/modal-input-layout.test.ts` 路径清单、`ui/AGENTS.md` 与 `ui/README.md` 目录职责
- [ ] 4.2 C-02 `AiChatComposer` / `MessageScroller` 移入 `components/agent/`，删除 `ai/` `chat/` 空目录；更新引用与文档
- [ ] 4.3 批后验证：`yarn typecheck` + 全量 `yarn test` + `yarn build`；`git diff --stat` 确认只含 rename 与 import 行

## 5. 第 4 批：样板收敛（D-01）

- [ ] 5.1 新建 `composables/useRowAction.ts`（`run(id, task)` 自动加锁 / finally 解锁 / 统一错误 toast 可选关闭），补单测
- [ ] 5.2 逐页替换：CronJobs（`toggling`）、FunctionTriggers（`toggling` / `invoking` / `removing`）、GoFunctions、S3Manager、Sandboxes、Databases、Logs 中确实同构的用法；每页替换后跑该页测试；不同构的保留原样并在实施记录注明
- [ ] 5.3 批后验证：`yarn typecheck` + 全量 `yarn test`

## 6. 第 5 批：测试基建（T-01，propose——批准后执行）

- [ ] 6.0 等待批准：`api-mock` 按域拆分方案（`src/test/api-mock/{platform,data,agent}.ts` + `index.ts`；`resetApiMocks` / `applyApiDefaults` 语义不变）
- [ ] 6.1 拆分执行；第 650 行 `as any` 一并类型化；83 个测试文件的 `@/test/api-mock` import 路径不变（桶文件转发），只有内部结构变
- [ ] 6.2 批后验证：`yarn typecheck` + 全量 `yarn test`（重点观察 mock 重置顺序无关性）

## 7. 收尾

- [ ] 7.1 全量复跑 `yarn typecheck` + `yarn test` + `yarn build`，coverage 四项与基线对比；chunk 尺寸对比记录进实施记录
- [ ] 7.2 检查最终 diff：没有混入无关改动、没有残留转发文件或死 import、组件 props/emits 未变（对比迁移前后 `defineProps` / `defineEmits`）、i18n key 集合未变
- [ ] 7.3 回填 design「实施记录」与 tasks：完成 / 部分 / 保留 / 取消逐项标注，记录实际执行的命令与未验证的边界（如真实浏览器视觉回归未做，指向 E4）；不提前打勾
- [ ] 7.4 同步 `ui/AGENTS.md` 目录职责表与 composable 清单、`ui/README.md` 目录结构与测试文件数
- [ ] 7.5 更新 `openspec/README.md` 在途变更表进度；完成后 `openspec archive refactor-ui-simplification`
