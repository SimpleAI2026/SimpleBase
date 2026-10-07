---
title: 定时任务 HTTP API
order: 2
---

# 定时任务 HTTP API

前缀 `/v1/projects/:projectID/cron-jobs`，所有请求均需要当前项目的 Bearer Key。系统项目不可使用定时任务。

| 方法 | 路径 | 权限 | 用途 |
|---|---|---|---|
| GET | `/` | `database:read` | 列出任务；响应 `{ "jobs": […] }` |
| POST | `/` | `database:write` | 新建；响应包含 `id`、`next_run_at` |
| GET | `/:jobID` | `database:read` | 获取任务 |
| PATCH | `/:jobID` | `database:write` | 更新调度、目标或启停 |
| DELETE | `/:jobID` | `database:write` | 删除任务 |
| GET | `/:jobID/runs` | `database:read` | 运行记录，含状态、耗时与错误 |
| POST | `/:jobID/trigger` | `database:write` | 手动异步触发 |

## 创建一个定时任务

请先在[云函数](/docs/gofunction/overview)页面创建 `hello` 文件并导出 `Hello`。然后提交：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/cron-jobs" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"name":"hello-hourly","schedule_kind":"interval","interval_seconds":3600,"func_file":"hello","func_export":"Hello","input_json":"{\"name\":\"scheduled\"}","enabled":true}'
```

也可以使用 `schedule_kind:"cron"` 搭配 5 字段 UTC `cron_expr`，或选择一次性执行 `schedule_kind:"once"` 搭配 RFC3339 `run_at`。`name` 创建后不可改，`input_json` 必须是合法 JSON 字符串。

## 查看与手动触发

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/cron-jobs/$JOB_ID/trigger" \
  -H "Authorization: Bearer $SB_KEY"
curl -sS "$BASE/v1/projects/$PROJECT/cron-jobs/$JOB_ID/runs?limit=20" \
  -H "Authorization: Bearer $SB_KEY"
```

触发请求立即返回，任务执行在后台进行；同一任务运行中再次触发可能返回 409。运行失败不会自动重试；到下次排期再执行。时间、补跑语义与配额详见[概览](/docs/cronjob/overview)。
