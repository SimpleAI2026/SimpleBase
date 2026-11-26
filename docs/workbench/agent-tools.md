---
title: 工具与模块
order: 2
group: 云助手
---

# 工具与模块

模块决定可选工具；是否能执行还取决于后端配置、当前项目权限与所选 Agent 的 `tool_ids`。从 `GET /v1/projects/:projectID/agents/modules` 查看实例可用模块，不要假定每个部署都有云沙盒。

## 数据与日志

数据库工具可以列数据库、列集合以及执行**只读** SQL；对象存储工具可以列出或查询对象元信息；日志工具可以搜索日志。工具受当前项目范围约束，不向模型暴露 SimpleBase 凭据。执行写入 SQL、上传 S3 对象应使用独立的受权限保护 API，而非期待 Agent 工具代执行。

## 云沙盒

启用沙盒模块后，可选 `sandbox_exec`（程序 + 参数）、`sandbox_shell`（`/bin/sh -c`）、`sandbox_read_file` 与 `sandbox_write_file`；文件读写路径必须是 `/workspace` 下的绝对路径；命令内部的当前目录就是该工作区，请使用相对路径。沙盒每个会话对应独立资源，使用后可在「云沙盒」页面查看与清理；数量计入项目上限。该模块需要部署时启用云沙盒并有可用 Cloud 后端；本地开发可以使用 fake 后端验证控制面，但不支持真实 Python 命令。

## 会话与调度

会话通过 `POST /v1/projects/:projectID/agent-threads` 创建，run 通过 `POST /agent-threads/:threadID/runs` 启动；已运行的任务可调用 `/agent-runs/:runID/cancel`。定时运行 Agent 统一使用[定时任务](/docs/automation/cronjob)（`target_kind=agent`），定时任务也可直接调用云函数。具体响应字段可通过控制台 Network 与服务端接口查看；流式输出不适合按普通 JSON 响应解析。

## 写权限与本地 gocloudagent

内置助手默认不可写：写命令会直接返回 `write_not_allowed`，不会先弹出确认。需要在「编辑助手」里打开「允许写操作」。本地 `gocloudagent` 二进制的 shell 工具会剥离凭据类环境变量（包括 `SIMPLEBASE_TOKEN`），shell 里的 `simplebase` 需要事先配置好配置文件，详见仓库 `docs/cli.md`。

## 内置 MCP Server

SimpleBase 会把当前项目的常用 API 封装成一个内置 MCP Server，任何支持 Streamable HTTP 的 MCP 客户端都可以直接连接：

- 主入口：`POST /v1/projects/:projectID/mcp`；扩展入口：`POST /v1/projects/:projectID/mcp/builtin/:name`。
- 鉴权：`Authorization: Bearer <API Key 或委托令牌>`，调用范围限定在令牌所属项目；不支持 OAuth。
- 内置名字 `simplebase` 为保留名，用户配置的外部 Server 不能使用 `simplebase` 或 `sb-` 前缀。
- 工具集精选（单个 Server 最多 64 个），覆盖项目信息、数据库 / 集合查询、文档读写、KV、对象存储、云函数与定时任务；单次工具结果上限 32 KiB。
- 默认开启，可在「工作台 → 模型 → MCP」的内置 Server 区域关闭；启停状态存项目设置，不新增系统表。
- 云助手调用内置 Server 使用进程内会话，不经过回环 HTTP；外部 MCP 连接器仍拒绝回环与私网地址。

与外部 Server 的区别：外部 Server 由用户配置地址与凭据、存 `sys_mcp_servers`；内置 Server 不需要配置地址，工具随服务端版本更新，且不能重命名。

## 安全提醒

Agent 的自然语言输入与工具输出均可能包含不可信文本。不要把敏感 Key 放在提示词、沙盒命令环境变量或对话上下文中；高风险写入优先走明确的 API 权限边界。
