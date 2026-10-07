---
title: 日志与审计
order: 4
---

# 日志与审计

「日志管理」展示当前项目的运行事件；请求链路、SQL 写入与云沙盒操作的审计记录用于故障分析。不要在日志中写入 API Key、SQL 参数、文件正文或沙盒输出。

## 查询运行日志

`GET /v1/projects/:projectID/logs` 需要 `database:read`；可选 `level`、`q`（消息包含词）、`from`、`to`（RFC3339）与 `limit`。响应形如 `{ "events": […] }`：

```bash
curl -sS "$BASE/v1/projects/$PROJECT/logs?level=error&limit=20" \
  -H "Authorization: Bearer $SB_KEY"
```

控制台可用筛选器按级别、时间和关键词定位；监控概览还提供 `/metrics/summary` 与 `/metrics/trend?days=7`。

## 保留期

`GET /v1/projects/:projectID/logs/retention` 返回 `scope`、`keep_days` 与 `updated_at`；`PUT` 同一路径提交 `{"keep_days":30}` 修改保留期，需要 `project:admin`。保留期受系统配置限制，超界会收到服务端错误。

## 审计边界

服务端会记录操作类别、项目、请求 ID、结果与必要的资源 ID；云沙盒执行只记录命令摘要和用量，不记录命令原文、环境变量值或输出。排查线上问题时先用请求 ID 对齐事件，再核对操作权限与实例状态。部署与故障处理参见[部署指南](/docs/ops/deployment)。
