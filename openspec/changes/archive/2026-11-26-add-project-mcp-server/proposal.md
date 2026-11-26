# Proposal

## Why

云助手已经支持连接外部 MCP Server，但项目自身的能力没有以 MCP 形式暴露：用户无法用任意 MCP 客户端操作当前项目，内置工具也无法复用同一套协议。把项目级 API 封装为内置 MCP Server，可以让「工作台 → 模型 → MCP」与外部工具使用同一种接入方式，并为后续内置 Server 建立注册表。

## What Changes

- 新增项目级内置 MCP Server：`POST /v1/projects/:projectID/mcp`（Streamable HTTP），使用 Bearer API Key 或委托令牌鉴权，只允许当前项目范围。
- 精选工具集（≤64 个）覆盖数据库查询、集合文档、KV、对象存储、云函数与定时任务的常用操作；不把约 150 条路由原样暴露。
- 内置 Server 以保留名注册，自动出现在控制台的 MCP 列表中；启停状态存项目设置，不新增系统表。
- 云助手访问内置 Server MUST 走进程内会话，不通过回环 HTTP（现有连接器拒绝回环与私网地址）。
- 为后续内置 Server 建立注册表：`/mcp` 为主入口，`/mcp/builtin/:name` 为扩展入口。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `agent-collaboration`: 增加项目级内置 MCP Server、精选工具集与进程内访问语义。

## Impact

- 受影响代码：`gocloudagent/mcp`（Server 封装，根模块不得直接 import go-sdk）、`internal/agentbridge`（连接器与进程内传输）、`internal/api/agenthttp` 或内核 MCP 派发、`internal/systemdb`（复用项目设置）、`ui/src/components/settings/McpServersPanel.vue`、`docs`。
- 不改变：外部 MCP 配置表 `sys_mcp_servers`、现有 `/mcp/servers` 管理 API、客户端工具上限常量。
- 依赖：不新增第三方依赖；go-sdk 只在 `gocloudagent` 模块内使用。
