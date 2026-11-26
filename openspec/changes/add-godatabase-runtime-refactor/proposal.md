# Proposal

## Why

`godatabase` 是独立运行时库，承担单文件库、DuckLake、注册表、KV、租约与对象存储接口。planv6.8 的评估发现：根包分层规范与源码不一致、同步 / 快照 / 关闭流程有重复骨架、部分候选抽象缺乏准入条件验证。当前只有评估与计划，没有实施。

## What Changes

- 按「契约 → 局部精简 → 同包拆分 → 受控共享 → 性能验证 → 回填」六批推进，每批独立可回滚。
- 只做等价重构：不改公开 API、不改对象键布局与 manifest 字段、不改 DDL、不改错误文本与事件顺序、不新增依赖。
- 先补特征测试与基线，再删除或搬移代码；高风险锁边界与状态机保持原样。
- 性能候选按 profile 选择，不追求全部实施；无法建立可信验证的候选停在提案阶段。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

（无）

本变更不改变任何对外可见行为，`.openspec.yaml` 设置 `skip_specs: true`；若实施中发现契约变化，必须撤回该标记并补 delta spec。

## Impact

- 受影响代码：`godatabase/` 全模块（`filedb`、`ducklake`、`registry`、`kv`、`lease`、`cache`、`blob`、`sqlguard`、公共工具）。
- 不影响根模块公开 API、系统库 schema、S3 键布局与配置来源。
- 验证：`GOWORK=off go build ./... && go vet ./... && go test ./... -timeout 900s`，以及宿主受影响包的单测。
