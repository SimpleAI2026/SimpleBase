# Agent Collaboration Specification

## Purpose

定义云助手 run、协作群轮次与计划工具的生命周期和协议，使 HTTP、cron 与本地 CLI 共享同一状态机与安全边界。

## Requirements

### Requirement: Run 生命周期与流式协议

每次 Agent run SHALL 有唯一 run 标识与终态（`completed` / `failed` / `canceled`），终态不可被后续更新覆盖。流式响应 MUST 以 `run` 帧开始，按需发送 `token`、`thinking`、`tool_call`、`tool_progress`、`tool_result`、`error`、`end` 帧，并以 `[DONE]` 结束；响应头 MUST 带 `X-Run-Id`（群轮次带 `X-Turn-Id`）。断线重放 MUST 检测游标缺口并下发 `stream_gap`。

#### Scenario: 非流式 run 被客户端取消
- **WHEN** 非流式请求的 HTTP 上下文被取消
- **THEN** run 被取消并落 `canceled`，错误码为空

#### Scenario: 流式连接断开
- **WHEN** 流式客户端断开连接
- **THEN** run 继续执行并落库，重连时按最后事件序号补齐或收到 `stream_gap`

### Requirement: 计划只经 plan 工具产生

计划 MUST 只由 `gocloudagent/plan` 的 `plan` 工具生成；服务端 MUST NOT 写死阶段、阶段技能白名单或阶段完成判定。群会话的每条消息 SHALL 经栅栏产出整份计划决策，可经 `POST /agent-turns/:id/plan-confirmations` 整份确认；固定助手单聊的计划从历史中的 `plan` 工具卡片重放。

#### Scenario: 群会话提交含计划的回答
- **WHEN** 助手在群轮次中调用 `plan` 工具
- **THEN** 生成可确认的计划决策并以 `plan` 帧下发，未确认前不执行计划中的写操作

#### Scenario: 重新会话
- **WHEN** 用户继续一个已有会话
- **THEN** 当前计划由历史中的 `plan` 工具调用重放得到，不依赖服务端阶段状态

### Requirement: 协作群轮次

群会话（`dispatch_mode=collaborative`）的消息 MUST 只经 `POST /agent-threads/:id/turns` 提交；在群会话上调用 `/runs` MUST 返回 400 `turns_required`。一轮内可点名多个助手，系统 SHALL 按依赖串行执行全部被点名助手，不得替换或丢弃。需要人工参与时 SHALL 生成有对象、有状态的人工任务，人工回复后在同一轮继续。

#### Scenario: 群会话上直接发起 run
- **WHEN** 客户端对 `dispatch_mode=collaborative` 的会话调用 `/runs`
- **THEN** 返回 400 `turns_required`，不创建 run

#### Scenario: 多助手分工
- **WHEN** 用户在一轮中 `@` 两个存在依赖关系的助手
- **THEN** 被依赖的助手先执行，依赖满足后另一个助手执行，两者结果都保留在轮次中

#### Scenario: 人工待办
- **WHEN** 助手请求当前人处理
- **THEN** 轮次产生人工任务并等待；人工回复后轮次恢复执行

### Requirement: 审批、取消与恢复

需要确认的工具调用 SHALL 产生 `confirmation_required` 事件，用户决定后发出 `confirmation_resolved`；未确认的调用 MUST NOT 执行。取消 SHALL 停止 run 或轮次并落取消态；服务重启后 `waiting_human` 与 `awaiting_plan_confirm` 轮次保持待办，其余运行中的轮次收敛为 `interrupted`。

#### Scenario: 审批被拒绝
- **WHEN** 用户拒绝一个待确认的工具调用
- **THEN** 工具不执行，run 收到拒绝结果并可继续或结束

#### Scenario: 服务重启
- **WHEN** 进程重启时存在运行中的轮次
- **THEN** 这些轮次被标记为 `interrupted`，人工待办仍可恢复

### Requirement: 安全边界与 fail-closed

Agent 提示词 MUST 按固定段落组装，且 MUST NOT 包含 S3 对象键、provider 密钥、`credential_ref` 原文、DSN 或存储前缀；敏感内容只做一次脱敏。命令策略或 MCP 策略缺失、分类出错时 MUST 拒绝执行并返回 `command_policy_unavailable`。cron 触发的 run SHALL 使用只读内置主体；委托令牌 MUST 只携带降权后的权限。

#### Scenario: 策略不可用
- **WHEN** `RuntimeProvider.Policy` 缺失或 `Classify` 报错
- **THEN** CLI 工具不确认、不执行，返回 `command_policy_unavailable`

#### Scenario: 定时任务越权
- **WHEN** cron 触发的 Agent 尝试写数据库
- **THEN** 因只读主体被拒绝，run 记录失败原因

### Requirement: 项目级内置 MCP Server

系统 SHALL 在 `/v1/projects/:projectID/mcp` 提供 Streamable HTTP 的 MCP Server，鉴权使用 Bearer API Key 或委托令牌，调用范围 MUST 限制在令牌所属项目。内置 Server MUST 使用保留名，MUST NOT 与用户配置的 Server 重名；启停状态 SHALL 持久化在项目设置中，不新增系统表。

#### Scenario: 外部 MCP 客户端连接
- **WHEN** 客户端携带项目 API Key 连接 `/v1/projects/<pid>/mcp`
- **THEN** 完成 MCP 握手并列出该项目的精选工具

#### Scenario: 跨项目令牌
- **WHEN** 客户端使用项目 A 的令牌连接项目 B 的 MCP 端点
- **THEN** 鉴权失败，不列出任何工具

### Requirement: 精选工具集与限额

内置 Server MUST 只暴露精选工具（单 Server ≤64 个），每个工具 MUST 有明确的项目范围与只读/写入标注；单次工具结果 MUST ≤32KiB。系统 MUST NOT 把全部 `/v1` 路由原样注册为工具。

#### Scenario: 工具数量
- **WHEN** 内置 Server 初始化
- **THEN** 注册工具数不超过 64，列表稳定且带描述

#### Scenario: 超大结果
- **WHEN** 工具返回超过 32KiB 的结果
- **THEN** 结果被截断并标注，不突破客户端上限

### Requirement: 云助手进程内访问

云助手访问内置 Server MUST 使用进程内传输，MUST NOT 经回环或私网 HTTP 回连；外部 MCP 连接器继续拒绝回环与私网地址。内置 Server 的启用状态 SHALL 影响云助手可用工具列表。

#### Scenario: 云助手调用内置工具
- **WHEN** 助手在 run 中引用内置 Server 的工具
- **THEN** 通过进程内会话执行，不产生回环 HTTP 请求

#### Scenario: 内置 Server 关闭
- **WHEN** 项目关闭内置 Server
- **THEN** 控制台与云助手都不再列出其工具，直接调用返回未找到
