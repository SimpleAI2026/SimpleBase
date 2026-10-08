---
title: 入门
order: 1
description: 从启动 SimpleBase 到操作项目、数据库、云沙盒与自动化。
---

# SimpleBase 入门

SimpleBase 以**项目**为隔离单位，集中管理 DuckLake 数据库、项目级 Key-Value、对象存储和自动化功能。控制台适合交互操作；HTTP API 与 JS / Go SDK 适合应用和脚本集成。

## 第一次使用

1. [5 分钟上手](/docs/getting-started/quickstart)：启动本地服务，使用 DevMode API Key 建库、执行 SQL 与 KV 命令。
2. [核心概念](/docs/getting-started/concepts)：了解项目、数据库、系统项目、用户和 API Key 的权限边界。
3. [SQL 与集合](/docs/database/sql)：查询数据、管理集合与文档。
4. [SDK](/docs/sdk)：在服务端应用中使用 JavaScript 或 Go SDK。

## 按场景查找

| 场景 | 指南 |
|---|---|
| 查询数据或管理文档 | [数据库](/docs/database)、[Key-Value](/docs/database/kv) |
| 上传和下载对象 | [对象存储](/docs/storage) |
| 运行 Go 代码或定时作业 | [云函数](/docs/gofunction/overview)、[定时任务](/docs/cronjob/overview) |
| 隔离执行脚本与 CI 用例 | [云沙盒](/docs/sandbox) |
| 用自然语言调用工具 | [云助手](/docs/agent) |
| 观察服务与管理成员 | [部署](/docs/ops/deployment)、[日志](/docs/ops/logs)、[用户与角色](/docs/ops/users) |

## 使用边界

云函数代码在 Go 解释器中运行，不是操作系统沙盒；需要 Linux 隔离环境时使用云沙盒。对外提供服务前，先阅读[部署与故障处理](/docs/ops/deployment)和[鉴权与安全](/docs/sdk/auth)。历史迁移指南仅供维护旧系统参考，不属于新用户上手步骤。
