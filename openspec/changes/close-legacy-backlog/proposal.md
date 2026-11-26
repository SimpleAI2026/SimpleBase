# Proposal

## Why

`planv6.7/legacy-plan-consolidation-plan.md` §3 在回代码核实后仍登记了 60 条未完成项（A–K 共 11 组），覆盖数据保留、读性能、多实例一致性、功能缺口、前端、评测、平台能力、examples 与工程债。这份清单是 v1.0–v6.4 十份目录删除后的唯一落点，但目前仍以「计划文档」形式存在，没有进入 OpenSpec 的可执行队列。

## What Changes

- 把 §3 的未完成项整体转为 OpenSpec 任务队列，按 A–K 分组；每个任务保留来源编号与验收条件。
- 开工前必须先按代码复核现状：计划自述状态多次失真，已完成的条目直接关闭并记录证据。
- 行为变化类条目（影响对外 API、错误码、数据模型、页面交互）在实施前必须补 delta spec 或另开变更；本变更只负责收敛与验证，不预设行为契约。
- 明确不做的条目（如 C 组中确认为单实例限制的部分）写下结论与迁移说明，不再悬挂。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

（无；本变更是遗留项收敛队列，`.openspec.yaml` 设置 `skip_specs: true`。任何条目一旦确认改变外部行为，必须先在对应能力下补 delta spec，或拆出独立变更。）

## Impact

- 影响范围横跨 `internal/`、`godatabase/`、`gocloudagent/`、`gosandbox/`、`gofunction/`、`ui/`、`examples/`、`docs/` 与 CI。
- 不改变：当前已发布的公开 API、系统库 schema、对象键布局与配置来源（除非单项另行提案）。
- 风险：条目状态可能过期；所有任务以「复核 → 实施 → 验证」三步执行，禁止按旧文档直接改代码。
