# 预期结果

| 步骤 | 通过条件 |
|---|---|
| 准备数据库 | 复用已有用户库，或用 `cap09-<suffix>` 新建集合库；`data.id` 非空 |
| 建集合 | `ok=true`，集合名带本次运行的随机后缀 |
| 建函数 | `data.name` 非空，`exports` 含 `Handle` |
| 建 record_change 触发器 | HTTP 201，`kind=record_change`、`target=集合名`、`enabled=true` |
| 插入文档 | `data.id` 非空 |
| 触发器回调 | 5 秒内本机 sink 收到 **恰好 1 次** 回调，路径含该 `record_id` 与 `action=create` |
| 触发器列表 | `data.items` 含该触发器，`dropped == 0` |
| 建 http 触发器 | HTTP 201，`kind=http` |
| invoke | HTTP 202，`queued=true` |
| http 回调 | sink 收到一次 `action=http` 的回调 |
| 更新 | `PATCH enabled=false` 后 `data.enabled == false` |
| 删除 | 两个触发器 DELETE 均返回 204；函数删除成功；本次新建的库删除成功 |

> 服务端 URL 主机不是 `127.0.0.1` / `localhost` / `::1` / `0.0.0.0` 时，两个 sink 断言打印
> `SKIP`（服务端无法回调运行脚本的机器），其余步骤仍按上表判定。
