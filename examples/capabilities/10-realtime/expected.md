# 预期结果

| 步骤 | 通过条件 |
|---|---|
| 准备数据库 | 复用已有用户库，或用 `cap10-<suffix>` 新建集合库；`data.id` 非空 |
| 建集合 | `ok=true` |
| sse-connect | 5 秒内出现 `event: connect`，`data.client_id` 非空 |
| 插入文档 | `data.id` 非空，记录为本次 `DOC` |
| live-create | 5 秒内收到同一 `record_id` 的 `record.create`，事件恰好 1 次 |
| event-payload | 事件含 `id/collection/action/at`，且 `include_data=1` 时 `data.title == rt-live` |
| subscription-update | `POST /realtime/subscriptions` 返回 204 |
| 修改文档 | `data.title == rt-updated` 且未传字段保持原值（merge patch） |
| live-update | 5 秒内收到同一 `record_id` 的 `record.update`，`data.title == rt-updated` |
| 删除文档 | `ok=true` |
| live-delete | 5 秒内收到同一 `record_id` 的 `record.delete` |
| replay | 带 `Last-Event-ID: 0` 的连接能读到本次的 `record.create` 帧 |
| 清理 | 本次新建的数据库删除成功（复用的库不动） |

> SSE 断言使用事件帧解析：`event:` 名与 `data:` JSON 的 `record_id` 必须同时匹配；
> 收不到帧时步骤 FAIL 并打印最近收到的事件名列表。
