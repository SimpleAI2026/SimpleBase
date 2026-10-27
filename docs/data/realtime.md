---
title: 文档 Realtime
order: 2
group: 数据管理
description: 通过 SSE 订阅集合文档变更，并处理订阅更新、断线续传与事件缺口。
---

# 文档 Realtime

Realtime 通过 Server-Sent Events（SSE）推送集合文档的创建、更新和删除事件，适合控制台刷新、协同编辑提示和轻量通知。它是**文档 API 写事件的实时通道**，不是数据库变更捕获系统，也不是持久消息队列。

## 建立连接

连接和更新订阅都需要 `database:read`。以下示例沿用[5 分钟上手](/docs/getting-started/quickstart)中的环境变量：

```bash
curl -N -H "Authorization: Bearer $SB_KEY" \
  "$BASE/v1/projects/$PROJECT/realtime?collections=orders&events=create,update"
```

首帧是 `connect`，`client_id` 用于后续修改订阅：

```text
event: connect
data: {"client_id":"<客户端 ID>"}
```

查询参数：

| 参数 | 默认值 | 说明 |
|---|---|---|
| `collections` | `*` | 逗号分隔的集合名，`*` 表示全部集合 |
| `events` | `create,update,delete` | 逗号分隔的事件动作 |
| `include_data` | 关闭 | 设为 `1` 时在事件中携带文档正文；Scoped Key 不支持 |

文档事件名为 `record.create`、`record.update` 或 `record.delete`，数据包含 `id`、`collection`、`record_id`、`action`、`changed`（如有）和 `at`。未开启 `include_data=1` 时不含 `data`。

```text
id: 42
event: record.update
data: {"id":"42","event":"record.update","collection":"orders","record_id":"o-1001","action":"update","changed":["status"],"at":"2026-10-27T08:00:00Z"}
```

浏览器原生 `EventSource` 不能自定义 `Authorization` 请求头；需要 Bearer 鉴权时，请使用支持自定义请求头的 SSE/fetch 客户端。

## 更新订阅

保持当前 SSE 连接，使用首帧中的 `client_id` 替换集合与事件过滤器。成功返回 204：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/realtime/subscriptions" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"client_id":"<客户端 ID>","collections":["orders","customers"],"events":["update","delete"]}'
```

订阅不存在或连接已关闭时返回 404 `realtime_subscription_not_found`。Scoped Key 还会逐个校验集合的 `read` 范围，越权返回 403 `key_scope_denied`。

## 断线续传与缺口

重连时把最后收到的事件 `id` 放入 `Last-Event-ID`：

```bash
curl -N -H "Authorization: Bearer $SB_KEY" \
  -H 'Last-Event-ID: 42' \
  "$BASE/v1/projects/$PROJECT/realtime?collections=orders"
```

服务会重放缓冲区中 ID 大于 42 且符合当前订阅的事件。进程内只保留最近 2000 帧；如果请求的位置已经早于可用缓冲区，会先收到 `gap` 事件。收到 `gap` 后应重新查询相关业务数据，不能把后续事件当作无缺口续传。服务重启后缓冲区不会保留。

## 连接与事件边界

- 每项目连接数由 `limits.realtime_max_conns` 控制，默认 200；超限返回 429 `realtime_connection_limit`。
- 服务每 15 秒发送心跳；连续 5 分钟没有文档事件会关闭空闲连接，单条连接最长 30 分钟。客户端应自动重连。
- **只有文档 API 成功写入产生的事件会推送**。SQL 直写、云函数直连数据库和其它引擎写入不会推送。
- 事件缓冲与投递都在进程内，不提供持久化或 exactly-once 保证；关键业务状态仍以数据库查询为准。
- 默认不携带文档正文，优先根据 `record_id` 按需查询。普通 Key 确需正文时才使用 `include_data=1`，避免在事件流中扩散敏感数据。

记录变更的后续自动化处理见[事件触发器](/docs/automation/function-triggers)，集合查询见[SQL 与集合](/docs/data/sql)。
