# OpenSpec 工作区

本目录是 SimpleBase 计划与变更规范的唯一入口。原 `plan/` 已于 2026-11-26 删除，不再维护旧计划映射；现状以 `specs/` 为准，历史原文可从 git 历史恢复。

## 目录职责

- `specs/<capability>/spec.md`：**现状基线**。已经实现、需要长期保持的行为契约，按限界上下文划分。
- `changes/<change-id>/`：**在途变更**。包含 `proposal.md`、`specs/**`（delta）、`design.md`（按需）与 `tasks.md`；完成后 `openspec archive <change-id>`。
- `changes/archive/`：已归档变更，归档时 delta 合入 `specs/`。

## 能力清单

| 能力 | 覆盖范围 |
| --- | --- |
| `platform-architecture` | 组合根、分层与依赖方向、架构门禁、停机语义 |
| `identity-access` | 权限层级、API Key、会话、委托令牌、项目上下文 |
| `project-catalog` | 项目与数据库生命周期、单项目单库、删库、存储身份 |
| `data-service` | SQL 守卫、集合文档、KV、实时事件、系统库只读与脱敏 |
| `file-sites` | 对象键、上传校验、下载附件、子域名站点 |
| `automation` | 云函数、触发器、定时任务、本地重放 |
| `agent-collaboration` | run、SSE、计划工具、协作群轮次、审批、安全边界 |
| `llm-gateway` | 供应商、OpenAI 兼容、Responses 子集、追踪 |
| `sandbox` | 生命周期、限额、错误码、隔离与出网、E2B / SSH |
| `ops-governance` | 备份、审计、日志、指标、设置与迁移状态 |
| `console-ui` | 三态、i18n、reka-ui、路由权限、工作台、看板、首页引导 |
| `dev-experience` | CLI 信封与退出码、SDK、示例清单、文档站 |

## 已归档变更（2026-11-26）

| 归档目录 | 合入规范 |
| --- | --- |
| `changes/archive/2026-11-26-add-api-preview-parameters` | `specs/console-ui`（+3 requirements） |
| `changes/archive/2026-11-26-add-object-storage-path-access` | `specs/file-sites`（+4 requirements） |
| `changes/archive/2026-11-26-add-project-mcp-server` | `specs/agent-collaboration`（+3 requirements） |
| `changes/archive/2026-11-26-add-row-level-access-control` | `specs/identity-access`（+2）、`specs/data-service`（+2） |
| `changes/archive/2026-11-26-add-object-storage-site-delivery` | `specs/file-sites`（+3）、`specs/dev-experience`（+2） |
| `changes/archive/2026-11-26-refactor-module-simplification` | 无 delta（`skip_specs: true`，纯等价重构；实施记录与 Linux 上的对照验证见其 `design.md`） |

## 在途变更完成度（2026-11-26 核对）

| 变更 | 进度 | 状态 |
| --- | --- | --- |
| `add-ops-maturity` | 1/15 | 仅完成死表复核（`sys_metric_rollups_hourly` 仍零读写） |
| `close-legacy-backlog` | 6/54 | 已关闭 D6 / E1 / E3 / K4 / K6 / K9 |
| `add-godatabase-runtime-refactor` | 0/25 | 未开始 |
| `refactor-ui-simplification` | 0/27 | 未开始（ui/ 等价重构：agent 大单元拆分、图表 / 复制 / 文案工具去重、modal 分域、列表页样板收敛；C / T 系列需先批准） |
| `complete-home-starter-delivery` | 0/14 | 未开始（无 `starter_stage` / 交付卡） |
| `complete-bi-dashboard` | 0/12 | 未开始（无导出 / 全屏 / 环比 / DataZoom） |

## 维护规则

1. 新计划一律 `openspec new change <change-id>`，按 proposal → specs → design → tasks 产出；`plan/` 已删除，不再新增历史计划目录。
2. 行为变化必须有 delta spec；纯重构 / 工具 / 文档类在 `.openspec.yaml` 设 `skip_specs: true`。
3. 完成后 `openspec archive <change-id>`，delta 合入 `specs/`。
4. `specs/` 描述可观察行为，不写内部类名、依赖库或实施步骤；实施细节放 `design.md` 与 `tasks.md`。
