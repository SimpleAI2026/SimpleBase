# 文档 Realtime：SSE 事件流

用一条 `curl -N` SSE 连接订阅集合的文档写入事件，依次验证：

1. 首帧是 `event: connect`，带 `client_id`；
2. 插入文档后收到 `record.create`，`include_data=1` 时事件还能读到文档正文；
3. 用 `client_id` 更新订阅（改成只收 `update`/`delete`）后，`PATCH` 文档收到 `record.update`；
4. 删除文档收到 `record.delete`；
5. 带 `Last-Event-ID: 0` 重连可以回放之前的 `record.create`。

Realtime 是**文档 API 写事件**的进程内广播：SQL 直写、云函数直连数据库都不产生事件；
缓冲区只保留最近 2000 帧，不持久化、不保证 exactly-once。

## 前置

- 服务端已启动，`SIMPLEBASE_BIN` / `SIMPLEBASE_URL` / `SIMPLEBASE_TOKEN` / `SIMPLEBASE_PROJECT_ID` 已导出。
- 目标项目有 `database:read` 权限即可；本用例会用当前 token（通常是超管）连接。
- 目标项目可以还没有用户数据库，用例会自己建；已有则直接复用（每个项目只允许一个用户数据库）。

## 运行

```bash
export CASE=10-realtime
./examples/capabilities/10-realtime/run.sh
```

## 接口

```bash
# 订阅（首帧 event: connect，data 里有 client_id）
curl -N -H "Authorization: Bearer $SIMPLEBASE_TOKEN" \
  "$SIMPLEBASE_URL/v1/projects/$PROJECT/realtime?collections=notes_<suffix>&events=create,update,delete&include_data=1"

# 更新订阅
curl -X POST "$SIMPLEBASE_URL/v1/projects/$PROJECT/realtime/subscriptions" \
  -H "Authorization: Bearer $SIMPLEBASE_TOKEN" -H 'content-type: application/json' \
  -d '{"client_id":"<client_id>","collections":["notes_<suffix>"],"events":["update","delete"]}'

# 断线续传
curl -N -H "Authorization: Bearer $SIMPLEBASE_TOKEN" -H 'Last-Event-ID: 0' \
  "$SIMPLEBASE_URL/v1/projects/$PROJECT/realtime?collections=notes_<suffix>&events=create"
```

事件帧格式：

```text
id: 42
event: record.update
data: {"id":"42","event":"record.update","collection":"notes_x","record_id":"...","action":"update","changed":["title"],"data":{"title":"rt-updated"}}
```

## 断言点

- 连接 5 秒内出现 `event: connect`，且能解析出 `client_id`。
- 插入文档后 5 秒内收到同一 `record_id` 的 `record.create`；事件含 `id/collection/action/at`
  字段，且 `include_data=1` 时 `data.title` 等于插入值；该事件恰好出现 1 次。
- `POST /realtime/subscriptions` 返回 204；之后 `PATCH` 文档收到 `record.update`，`data.title` 为新值。
- `DELETE` 文档收到 `record.delete`。
- 带 `Last-Event-ID: 0` 重连时能回放本次的 `record.create` 事件。

## 已知边界

- 服务端 URL 必须可从运行 `run.sh` 的机器访问；SSE 读取用 `curl -N` 后台进程，退出时统一清理。
- 只用文档 API 写入；`simplebase sql exec` 直写不会推送事件。
- 连续 5 分钟无事件会关闭空闲连接，单连接最长 30 分钟；服务端每 15 秒发一次心跳注释行。
- 每项目连接数受 `limits.realtime_max_conns` 限制（默认 200），超限返回 429 `realtime_connection_limit`。
- Scoped Key 订阅集合时会逐个校验 `read` 范围，越权返回 403 `key_scope_denied`；本用例用普通 token。
