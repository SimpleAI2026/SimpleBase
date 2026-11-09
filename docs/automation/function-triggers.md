---
title: 事件触发器
order: 15
group: 事件触发
description: 在文档、对象或 HTTP 事件发生后调用项目云函数。
---

# 事件触发器

事件触发器把记录变更、对象上传/删除或显式 HTTP 调用映射到一个云函数导出函数，适合写入后的轻量通知、数据规整和异步处理。触发器定义保存在系统库，事件执行使用项目云函数的**活动版本**。

## 触发器类型

| `kind` | `target` | `events` | 触发时机 |
|---|---|---|---|
| `record_change` | 集合名或 `*` | `create`、`update`、`delete` | 文档 API 写成功后 |
| `object_event` | 对象 key 前缀或 `*` | `upload`、`delete` | 对象 API 写成功后 |
| `http` | 留空 | 不需要 | 显式调用该触发器的 `invoke` 接口 |

`events: []` 或 `["*"]` 表示匹配该类型的全部动作。记录与对象触发器可设置 `filter`，语法为 `字段=JSON值`，例如 `status="open"` 或 `priority=2`；它只匹配事件 `data` 的顶层字段。

**SQL 直写不会产生记录触发事件。** 对象触发器的目标按 key 前缀匹配，例如 `reports/` 可匹配 `reports/2026/summary.csv`。

## 创建记录触发器

先在[云函数](/docs/automation/gofunction)中保存目标函数并确认活动版本可用。管理和读取触发器需要 `project:admin`：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/function-triggers" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{
    "kind":"record_change",
    "target":"orders",
    "events":["create","update"],
    "func_file":"order_hooks",
    "func_export":"Handle",
    "filter":"status=\"open\"",
    "enabled":true
  }'
```

创建成功返回 201 和触发器对象，保存响应中的 `id`。`func_file` 可写 `order_hooks` 或 `order_hooks.go`，服务端统一按无扩展名保存。目标函数或活动版本不存在时返回 404 `gofunction_not_found`。

云函数收到的参数是事件 JSON，可声明对应结构：

```go
package main

type Event struct {
	Kind     string         `json:"kind"`
	Target   string         `json:"target"`
	Action   string         `json:"action"`
	RecordID string         `json:"record_id"`
	Changed  []string       `json:"changed"`
	Data     map[string]any `json:"data"`
}

type Result struct {
	Accepted bool `json:"accepted"`
}

func Handle(event Event) Result {
	return Result{Accepted: event.Action != ""}
}
```

## 管理与调用

接口前缀为 `/v1/projects/:projectID/function-triggers`：

| 操作 | 方法与路径 | 权限 | 成功响应 |
|---|---|---|---|
| 列表（含累计 `dropped`） | `GET /` | `project:admin` | 200 |
| 创建 | `POST /` | `project:admin` | 201 |
| 详情 | `GET /:id` | `project:admin` | 200 |
| 更新或启停 | `PATCH /:id` | `project:admin` | 200 |
| 归档删除 | `DELETE /:id` | `project:admin` | 204 |
| 调用 HTTP 触发器 | `POST /:id/invoke` | `database:write` | 202 |

停用触发器：

```bash
curl -sS -X PATCH "$BASE/v1/projects/$PROJECT/function-triggers/$TRIGGER_ID" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"enabled":false}'
```

只有已启用的 HTTP 触发器可以显式调用。请求体是 JSON 对象，会放入事件的 `data`；接口只负责入队，返回 202 不代表函数已执行完成：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/function-triggers/$TRIGGER_ID/invoke" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"source":"daily-job","order_id":"o-1001"}'
```

控制台「自动化 → 事件触发器」也可创建、启停、删除和手动调用触发器。

## 队列、重试与限制

- 事件进入容量 1024 的进程内队列，默认 4 个 worker。记录/对象事件在队列满时丢弃并累计 `dropped`；HTTP 调用在队列满时返回 429 `trigger_queue_full`。
- 失败时最多重试三次，即首次执行加三次重试，采用短间隔指数退避。每次尝试都写云函数调用流水。
- 队列不是持久队列，进程重启会丢失未处理事件，也不保证 exactly-once。目标函数应按事件标识做幂等，关键工作流需要自行补持久化与对账。
- `/go`、在线试跑、cron 和触发器共享 `limits.max_concurrent_go_runs` 并发上限，函数仍受单次 10 秒执行限制。
- 事件正文不写入日志；系统项目不提供事件触发器，实例只读时写接口返回 503。

需要实时向客户端推送同一批文档 API 写事件时，使用[文档 Realtime](/docs/data/realtime)；需要按时间排期时，使用[定时任务](/docs/automation/cronjob)。
