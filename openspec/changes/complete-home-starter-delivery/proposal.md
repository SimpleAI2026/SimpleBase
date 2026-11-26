# Proposal

## Why

首页「一句话建应用」目前只完成入口：提示词、创建项目、打开搭建会话可用，但服务端没有实现分阶段运行（`starter_stage` 与技能白名单只在 `docs/workbench/agent.md` 中描述），也没有交付卡片（`sb:delivery` 协议）。用户进入会话后仍要自己一轮轮驱动，完成后拿不到可点击的访问入口。

## What Changes

- 搭建会话服务端契约：`POST /agent-threads` 接受白名单内的 `starter_template`；`POST /agent-threads/:id/runs` 接受 `starter_stage`，技能只由服务端按阶段白名单注入，非搭建会话传阶段返回 `400 starter_stage_invalid`。
- 前端分阶段驱动：`useStarterRun` 在上一阶段成功后自动发送下一阶段；失败停在原阶段并提供「重试本阶段 / 跳过 / 我来接手」；用户手动发消息即退出自动模式。
- 交付块协议：助手在 verify 阶段输出 `<!-- sb:delivery {...} -->`，前端解析并渲染交付卡片；只信结构不信 URL，字段缺失逐项隐藏，整体缺失回退为普通消息 + 资源入口。
- 搭建提示词：`starterModulePrompt` 约束分阶段、只追加迁移、前端单文件（`web/index.html`、Hash 路由、无构建、≤200KiB、无外链 CDN、不含 `sb_live_*`）。
- 文档与实现对齐：`docs/workbench/agent.md` 只描述已实现行为。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `agent-collaboration`: 增加搭建会话的模板 / 阶段白名单与交付块协议。
- `console-ui`: 增加分阶段自动执行、交付卡片渲染与失败恢复交互。

## Impact

- 受影响代码：`internal/systemdb/agent_threads.go`（已有 `StarterTemplate`）、`internal/agentrun`（阶段技能白名单）、`internal/api/agenthttp`（请求校验）、`gocloudagent/cloudagent/modules.go`（starter 提示词）、`ui/src/composables/{useStarterLaunch,useStarterThread}.ts`、`ui/src/components/agent/StarterDeliveryCard.vue`（新增）、`ui/src/pages/AgentManager.vue`、`ui/src/locales/`、`docs/workbench/agent.md`。
- 不改变：普通通用会话不加载技能；破坏性命令与对外公开仍需确认；委托令牌仍按调用者权限签发。
