---
title: 入门
order: 1
description: 从启动 SimpleBase 到操作项目、数据库、云沙盒与自动化。
---

# SimpleBase 入门

SimpleBase 以**项目**为隔离单位，集中管理项目数据库（DuckDB 或 SQLite 单文件）、项目级 Key-Value、对象存储和自动化功能。控制台适合交互操作；HTTP API 与 JS / Go SDK 适合应用和脚本集成。

## 第一次使用

1. [5 分钟上手](/docs/getting-started/quickstart)：启动本地服务，创建项目并签发 API Key，建库、执行 SQL 与 KV 命令。
2. [核心概念](/docs/getting-started/concepts)：了解项目、数据库、系统项目、用户和 API Key 的权限边界。
3. [SQL 与集合](/docs/data/sql)：查询数据、管理集合与文档。
4. [SDK](/docs/sdk)：在服务端应用中使用 JavaScript 或 Go SDK。

## 按场景查找

| 场景 | 指南 |
|---|---|
| 查询数据或管理文档 | [数据库](/docs/data)、[Key-Value](/docs/data/kv) |
| 上传和下载对象 | [对象存储](/docs/data/storage) |
| 运行 Go 代码或定时作业 | [云函数](/docs/automation/gofunction)、[定时任务](/docs/automation/cronjob) |
| 响应文档变更或对象事件 | [文档 Realtime](/docs/data/realtime)、[事件触发器](/docs/automation/function-triggers) |
| 隔离执行脚本与 CI 用例 | [云沙盒](/docs/automation/sandbox) |
| 用自然语言调用工具 | [云助手](/docs/workbench/agent) |
| 用 OpenAI SDK 调用项目默认模型 | [OpenAI 兼容接口](/docs/getting-started/openai-compatible) |
| 观察服务 | [监控大盘](/docs/workbench/dashboard)、[日志与审计](/docs/ops/logs)、[部署](/docs/ops/deployment) |

## 使用边界

云函数代码在 Go 解释器中运行，不是操作系统沙盒；需要 Linux 隔离环境时使用云沙盒。对外提供服务前，先阅读[部署与故障处理](/docs/ops/deployment)和[鉴权与安全](/docs/sdk/auth)。历史迁移指南仅供维护旧系统参考，不属于新用户上手步骤。

## 使用场景：搭一个订单后端

下面把几个模块串起来，做一个最小的「订单后端」。每一步都有独立文档，这里只展示它们如何配合。所有命令沿用[5 分钟上手](/docs/getting-started/quickstart)中的 `BASE`、`PROJECT`、`SB_KEY` 与 `DATABASE`。

| 步骤 | 目的 | 使用模块 |
|---|---|---|
| 1. 建库建表 | 保存订单与订单行 | [数据管理](/docs/data/sql) |
| 2. 写入订单 | 事务内写订单主表和明细 | [数据管理](/docs/data/sql) |
| 3. 缓存库存 | 热点库存放进 KV，设置过期 | [Key-Value](/docs/data/kv) |
| 4. 保存商品图 | 上传图片，控制台里点击「打开」即可预览 | [对象存储](/docs/data/storage) |
| 5. 计算金额 | 用云函数统一折扣与税费计算 | [云函数](/docs/automation/gofunction) |
| 6. 每日汇总 | 定时调用云函数生成日报 | [定时任务](/docs/automation/cronjob) |
| 7. 观察运行 | 请求量、错误率、延迟与日志 | [监控大盘](/docs/workbench/dashboard)、[日志管理](/docs/ops/logs) |

第 1、2 步的最小实现：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases/$DATABASE/execute" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"sql":"CREATE TABLE orders (id VARCHAR, user_id VARCHAR, amount DOUBLE, created_at TIMESTAMP)"}'

curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases/$DATABASE/execute" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"sql":"INSERT INTO orders VALUES (?, ?, ?, now())","args":["o-1001","u-42",199.5]}'
```

第 3 步缓存库存（60 秒后自动过期）：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/kv" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"type":"cmd","argvs":["SET","stock:sku-1","120","EX","60"]}'
```

> **提示** 订单号请用 UUID 或业务唯一编号，并在应用层做幂等；引擎差异与约束见[数据库引擎与约束](/docs/data/ducklake-notes)。

## 常见问题

| 现象 | 可能原因 | 处理 |
|---|---|---|
| 返回 401 | 未带 Key 或 Key 无效 | 检查 `Authorization: Bearer <KEY>`，本地种子 Key 见上手指南 |
| 返回 403 | Key 没有写权限，或访问了其它项目 | 换用具备 `database:write` 的 Key；确认 `PROJECT` 与 Key 所属项目一致 |
| 写入返回 503 | 实例只读或写者不可用 | 检查实例是否为只读副本，参见[部署与故障处理](/docs/ops/deployment) |
| 控制台没有云沙盒 | 部署时未启用 | 见[云沙盒概览](/docs/automation/sandbox)中的启用条件 |
