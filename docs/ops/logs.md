---
title: 日志与审计
order: 1
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

## 统计与导出

`GET /v1/projects/:projectID/logs/stats?group_by=level|hour|path&from=&to=` 返回 `[{key,count}]`（响应包装在 `items` 中）；`GET /v1/projects/:projectID/logs/export?format=csv` 导出固定列 CSV。两者需要 `database:read`，支持相同时间范围；默认最近 24 小时，最大 30 天。CSV 最多 10 万条，仍只含现有脱敏字段。控制台日志页显示小时趋势并可导出 CSV。

## 保留期

`GET /v1/projects/:projectID/logs/retention` 返回 `scope`、`keep_days` 与 `updated_at`；`PUT` 同一路径提交 `{"keep_days":30}` 修改保留期，需要 `project:admin`。保留期受系统配置限制，超界会收到服务端错误。

## 审计边界

服务端会记录操作类别、项目、请求 ID、结果与必要的资源 ID；云沙盒执行只记录命令摘要和用量，不记录命令原文、环境变量值或输出。排查线上问题时先用请求 ID 对齐事件，再核对操作权限与实例状态。

审计记录可经 `GET /v1/projects/:projectID/audit?database_id=&limit=50` 只读查询（`database:read` 权限），响应 `{ "operations": […], "limit": 50 }`；控制台暂不展示，供运维脚本使用。部署与故障处理参见[部署指南](/docs/ops/deployment)。

## 使用场景

### 场景一：按级别和关键词定位报错

背景：[监控大盘](/docs/workbench/dashboard)显示错误率升高。

1. 在日志管理页把级别设为 `error`，输入关键词（对应接口参数 `q`，按消息包含匹配）。
2. 缩小时间范围到异常发生的前后，再逐条查看。
3. 用同一个请求 ID 对齐上下游事件。

等价的 API 调用：

```bash
curl -sS "$BASE/v1/projects/$PROJECT/logs?level=error&q=timeout&from=2026-10-10T00:00:00Z&to=2026-10-10T12:00:00Z&limit=50" \
  -H "Authorization: Bearer $SB_KEY"
```

`from` / `to` 必须是 RFC3339 时间；格式不合法的时间参数会被忽略，而不是报错，所以排查「为什么结果没有过滤」时先检查时间格式。

### 场景二：排查某个云函数的失败

云函数每次调用会记录次数、耗时和错误数（指标 `gofunction_invokes`、`gofunction_invoke_duration_ms`、`gofunction_invoke_errors`）。先在日志里用函数名作为关键词过滤 `error` 级别，再回到[云函数](/docs/automation/gofunction)用「测试」功能带着同样的入参复现。

### 场景三：调整日志保留期

默认保留天数由系统配置决定。需要延长或缩短时，用具备 `project:admin` 的 Key：

```bash
curl -sS "$BASE/v1/projects/$PROJECT/logs/retention" \
  -H "Authorization: Bearer $SB_KEY"

curl -sS -X PUT "$BASE/v1/projects/$PROJECT/logs/retention" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"keep_days":30}'
```

超出系统允许范围会返回服务端错误。保留期缩短后，更早的日志会在后续清理中被删除，无法找回，调整前先确认是否需要导出。

### 场景四：日志与审计的区别

| | 日志管理 | 审计 |
|---|---|---|
| 目的 | 看服务运行状况、定位故障 | 追溯「谁在什么时候做了什么操作」 |
| 内容 | 运行事件、错误信息 | 操作类别、项目、请求 ID、结果、资源 ID |
| 入口 | 控制台「日志管理」与 `/logs` | `GET /audit`，控制台暂不展示 |
| 敏感信息 | 不应出现 API Key、SQL 参数、文件正文 | 只记录 SHA-256 与元数据，不记录 SQL 参数或 LLM 正文 |

### 场景五：安全地提交问题线索

向他人求助时，只贴请求 ID、时间范围、错误码与日志级别，不要贴 API Key、完整 SQL 参数或沙盒命令输出。

## 常见问题

| 现象 | 可能原因 | 处理 |
|---|---|---|
| 查不到刚发生的日志 | 日志先入内存缓冲、由后台批量刷写（默认约 2 秒一批） | 等几秒后刷新；缓冲积压过多时最早的记录可能被丢弃 |
| 过滤条件不生效 | `from` / `to` 不是 RFC3339，被服务端忽略 | 改成 `2026-10-10T00:00:00Z` 这样的格式 |
| 修改保留期返回 403 | 当前 Key 没有 `project:admin` | 使用具备该权限的 Key，参见 [SDK 鉴权与安全](/docs/sdk/auth) |
