---
title: 自动化概览
order: 0
---

# 自动化

「自动化」分组提供四类能力：**云函数**负责短时 Go 计算，**事件触发器**负责响应文档、对象与 HTTP 事件，**定时任务**负责按排期触发云函数，**云沙盒**负责需要独立 Linux 环境的脚本执行。

| 子主题 | 定位 | 文档 |
|---|---|---|
| 云函数 | 保存一段 Go 源码，导出函数经 HTTP+JSON 调用 | [云函数概览](/docs/automation/gofunction)、[调用指南](/docs/automation/gofunction-invoke) |
| 事件触发器 | 文档、对象或 HTTP 事件发生后调用云函数 | [事件触发器](/docs/automation/function-triggers) |
| 定时任务 | 到点自动调用项目内的云函数 | [定时任务概览](/docs/automation/cronjob)、[HTTP API](/docs/automation/cronjob-api) |
| 云沙盒 | 项目级隔离 Linux 环境，执行 shell 与脚本 | [云沙盒概览](/docs/automation/sandbox)、[HTTP API](/docs/automation/sandbox-api)、[e2e 与 CI](/docs/automation/sandbox-e2e) |

## 怎么选

| 需求 | 选择 | 原因 |
|---|---|---|
| 一段确定入参、确定输出的计算 | 云函数 | 无需部署，保存即生效 |
| 每天固定时间跑一次汇总 | 定时任务 + 云函数 | 定时任务负责排期，目标是云函数导出函数 |
| 文档或对象写成功后异步处理 | 事件触发器 + 云函数 | 按事件类型、目标和过滤条件触发 |
| 需要 `pip install`、跑 Python/Node 脚本、访问文件系统 | 云沙盒 | 云函数是解释器，不能装依赖、不能操作 OS |
| 需要对外提供 HTTP 接口 | 云函数 | 调用路径是 `POST /go/:projectID/:name/:Export` |
| 需要长期常驻进程 | 都不适合 | 云函数单次执行 10 秒上限，沙盒单次 VM 最长 30 分钟 |

## 边界

- 云函数是**解释器**，不是操作系统沙盒；沙盒命令与文件只在隔离 VM 内，不读写 SimpleBase 的库、KV 或对象存储。
- 四者都按项目隔离；系统项目不提供云函数、事件触发器与定时任务。
- 都是写操作，需要对应写权限，且实例必须可写。
