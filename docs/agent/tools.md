---
title: 工具与模块
order: 2
---

# 工具与模块

模块决定可选工具；是否能执行还取决于后端配置、当前项目权限与所选 Agent 的 `tool_ids`。从 `GET /v1/projects/:projectID/agents/modules` 查看实例可用模块，不要假定每个部署都有云沙盒。

## 数据与日志

数据库工具可以列数据库、列集合以及执行**只读** SQL；对象存储工具可以列出或查询对象元信息；日志工具可以搜索日志。工具受当前项目范围约束，不向模型暴露 SimpleBase 凭据。执行写入 SQL、上传 S3 对象应使用独立的受权限保护 API，而非期待 Agent 工具代执行。

## 云沙盒

启用沙盒模块后，可选 `sandbox_exec`（程序 + 参数）、`sandbox_shell`（`/bin/sh -c`）、`sandbox_read_file` 与 `sandbox_write_file`；读写路径必须位于 `/workspace` 下。沙盒每个会话对应独立资源，使用后可在「云沙盒」页面查看与清理；数量计入项目上限。该模块需要部署时启用云沙盒并有可用 Cloud 后端；本地开发可以使用 fake 后端验证控制面，但不支持真实 Python 命令。

## 会话与调度

会话通过 `POST /v1/projects/:projectID/agent-threads` 创建，run 通过 `POST /agent-threads/:threadID/runs` 启动；已运行的任务可调用 `/agent-runs/:runID/cancel`。定时运行 Agent 使用 `/agent-schedules`，而[定时任务](/docs/cronjob/overview)直接调用云函数。具体响应字段可通过控制台 Network 与服务端接口查看；流式输出不适合按普通 JSON 响应解析。

## 安全提醒

Agent 的自然语言输入与工具输出均可能包含不可信文本。不要把敏感 Key 放在提示词、沙盒命令环境变量或对话上下文中；高风险写入优先走明确的 API 权限边界。
