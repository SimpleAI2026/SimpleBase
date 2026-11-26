# Spec Delta

## ADDED Requirements

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
