---
title: 工作台概览
order: 0
---

# 工作台

工作台是控制台里「用自然语言操作」和「看运行状况」的入口，包含两个子主题：

| 子主题 | 做什么 | 文档 |
|---|---|---|
| 云助手 | 用自然语言探索数据库、日志、沙盒，并管理 Agent 与定时调度 | [云助手概览](/docs/workbench/agent)、[工具与模块](/docs/workbench/agent-tools)、[模型联调与验收](/docs/workbench/agent-e2e) |
| 监控大盘 | 查看项目的请求量、错误率、延迟分位数与资源数量 | [监控大盘](/docs/workbench/dashboard) |

## 怎么选

- 想知道「项目里有哪些表」「最近的报错是什么」，但不想写 SQL：用**云助手**，它只调用只读工具。
- 想知道「服务是否健康」「错误率是不是在涨」：用**监控大盘**，需要逐条排错再转 [日志与审计](/docs/ops/logs)。
- 需要周期执行一段提示词：在云助手里配置 Agent 调度；需要固定入参调用 Go 函数：用 [定时任务](/docs/automation/cronjob)。

## 权限

云助手与监控大盘都只需要 `database:read`。沙盒相关工具还要求部署时已启用云沙盒，具体以 `GET /v1/projects/:projectID/agents/modules` 返回的模块为准。
