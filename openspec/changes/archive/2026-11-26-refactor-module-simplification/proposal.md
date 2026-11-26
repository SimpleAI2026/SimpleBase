# Proposal

## Why

`gocloudagent`、`gosandbox`、`gofunction`、`godatabase` 四个嵌套模块与 `internal/` 在 DDD 分层迁移后已经能通过构建和架构门禁，但还留着迁移期的结构负担：

- `internal/api` 留有 4 个 `*_compat.go` 同包转发面（共 21 个别名 / 转发），实际外部调用方只剩 `internal/app` 的 1 个类型。
- 有几个超大文件和超长函数：`gocloudagent/group/engine.go` 1407 行；`gosandbox/proto/codec.go` 的 `Encode` / `Decode` 分别 219 / 269 行；`internal/config` 的 `applyEnv` / `applyYAML` / `Redacted` / `defaults` 都超过 120 行。
- 同一模块内有重复实现：`gosandbox/agent` 与 `gosandbox/host` 各有一份 `openPTY` / `setWinsize`；3 处手写 `itoa`；2 处 `truncate` 只是一行转发。

本变更按 `skills/simplify-code`（v2.2.0）的 Refactor 模式，把上述问题整理成可以分批执行、每批都能单独回滚的等价重构计划。

## What Changes

- 只做等价重构：不改公开 API、HTTP 路径、错误形状与文本、SSE 帧格式、wire 协议字节布局、DDL、配置键名与覆盖优先级，不新增依赖。
- 先记录基线、补特征测试，再搬移或抽取代码。每个检查点只处理一项职责。
- 候选分为 apply / propose / skip 三类。跨模块共享、热路径改写、公共 API 新增都只列为 propose，需要单独批准。
- `godatabase` 内部重构继续由 `add-godatabase-runtime-refactor` 负责。`internal/api/perfbench` 外移（K3）、`Executor.Compile` 空实现（K2）由 `close-legacy-backlog` 负责。本变更不重复登记这些项。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

（无）

本变更不改变任何对外可见行为，`.openspec.yaml` 设置 `skip_specs: true`。实施中如果发现某项必须改变契约，就停在 propose，另开变更并补 delta spec。

## Impact

- 受影响代码：`internal/api`（compat 转发面）、`internal/config`、`internal/app`（别名调用方）、`gocloudagent/{group,plan,cli,cloudagent}`、`gosandbox/{proto,agent,host}`、`internal/api/sandboxhttp`、`internal/agentbridge`。
- 需要同步的文档 / 门禁：`internal/api/kernel_residue_test.go` 的文件清单、根 `AGENTS.md` §2.3 的「29 个」、`internal/README.md`、`internal/AGENTS.md`、`openspec/specs/platform-architecture/spec.md`（只在文件清单描述受影响时同步）。
- 不影响：系统库 schema、对象键布局、S3 manifest、gofunction 公共门面、四个模块的独立性（R9）。
