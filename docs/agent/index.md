---
title: 云助手概览
order: 1
---

# 云助手概览

云助手把大模型与项目内工具组合，按模块管理配置；控制台「工作台 → 云助手」可以创建会话，使用 `@` 选择 Agent 并查看工具调用。Agent 使用你当前项目的权限，不会自动获得跨项目访问能力。

## 何时使用

需要用自然语言探索数据库、文件或运行日志时，可在会话中让 Agent 调用只读工具；需要在独立 Linux 环境执行代码时，给 Agent 启用云沙盒模块。云沙盒命令与文件留在 VM 内，不等同于修改 SimpleBase 的 SQL、对象存储或系统库。

## 基本流程

1. 在控制台选择普通项目，打开「云助手」，检查所需模块和工具是否启用。
2. 新建会话，输入 `@` 并选择 Agent，发送问题。
3. 对话中的工具调用、结果与运行状态会随会话显示；按需取消正在运行的任务。
4. 需要周期执行提示词时用 Agent 定时调度；需要固定入参触发 Go 函数时改用[定时任务](/docs/cronjob/overview)。

## API 概览

`GET /v1/projects/:projectID/agents/modules` 列出可选模块。管理 Agent 使用 `/agents`；会话和消息使用 `/agent-threads`；`PATCH /agent-threads/:threadID` 可重命名，`GET /agent-threads/:threadID/runs` 查询耗时、token 和工具调用次数；调度用 `/agent-schedules`。创建 run 使用 `POST /agent-threads/:threadID/runs`，实际回答以流式结果返回。完整工具规则见[工具与模块](/docs/agent/tools)，模型与端到端验证见[联调与验收](/docs/agent/e2e)。

## 权限与边界

读取资源需要 `database:read`；修改 Agent 配置、会话等操作分别要求路由对应权限。Agent 的数据库和 S3 工具只读；沙盒工具只作用于隔离 VM。不可将不可信的上下文视为操作授权。
