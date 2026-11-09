# 事件触发器：记录变更与 HTTP 调用

先部署一个云函数，再建两类触发器：`record_change`（集合 `create` 事件）和 `http`（显式 `invoke`）。
插入文档后，触发器异步调用云函数；云函数用已注册的 `net/http` 回调本机的一次性 HTTP sink，
按 `record_id` 计数，从而证明「函数被调用且只调用一次」。最后测触发器的列表、更新和归档删除。

管理接口在 `/v1/projects/:projectID/function-triggers`，读写需要 `project:admin`，
`invoke` 需要 `database:write`。触发器定义保存在系统库，不是项目文件。

## 前置

- 服务端已启动，`SIMPLEBASE_BIN` / `SIMPLEBASE_URL` / `SIMPLEBASE_TOKEN` / `SIMPLEBASE_PROJECT_ID` 已导出。
- 目标项目不是系统项目（`sb-admin` 不支持事件触发器）。
- 目标项目可以还没有用户数据库，用例会自己建；已有则直接复用（每个项目只允许一个用户数据库）。
- 服务端与运行 `run.sh` 的机器在同一台主机（sink 断言需要服务端能访问 `127.0.0.1`）。
  URL 主机不是 loopback 时，sink 相关步骤打印 SKIP，其余管理面断言照常执行。

## 运行

```bash
export CASE=09-triggers
./examples/capabilities/09-triggers/run.sh
```

## 接口与命令

```bash
# 部署函数（源码由 run.sh 生成，内嵌本机 sink 端口）
simplebase function create --name "trig_<suffix>" --file /tmp/.../trigger.go

# 记录变更触发器
curl -X POST "$SIMPLEBASE_URL/v1/projects/$PROJECT/function-triggers" \
  -H "Authorization: Bearer $SIMPLEBASE_TOKEN" -H 'content-type: application/json' \
  -d '{"kind":"record_change","target":"orders_<suffix>","events":["create"],
       "func_file":"trig_<suffix>","func_export":"Handle","enabled":true}'

# 触发：一次文档写入
simplebase document insert --database "$DB" --collection "orders_<suffix>" --json '{"item":"book","qty":2}'

# HTTP 触发器
curl -X POST "$SIMPLEBASE_URL/v1/projects/$PROJECT/function-triggers" ... \
  -d '{"kind":"http","func_file":"trig_<suffix>","func_export":"Handle","enabled":true}'
curl -X POST "$SIMPLEBASE_URL/v1/projects/$PROJECT/function-triggers/$ID/invoke" ... -d '{"source":"capability-example"}'

# 列表 / 更新 / 删除
curl "$SIMPLEBASE_URL/v1/projects/$PROJECT/function-triggers"
curl -X PATCH .../function-triggers/$ID -d '{"enabled":false}'
curl -X DELETE .../function-triggers/$ID
```

## 断言点

- `record_change` 创建返回 201，`kind/target/enabled` 与请求一致。
- 插入一条文档后 5 秒内 sink 恰好收到 1 次回调，查询参数带该文档的 `record_id` 与 `action=create`。
- 触发器列表包含刚创建的触发器，且 `dropped` 为 0（没有因队列满丢事件）。
- `http` 触发器 `invoke` 返回 202 且 `queued=true`，随后 sink 收到一次 `action=http` 的回调。
- `PATCH enabled=false` 生效；两次 DELETE 都返回 204，函数删除后项目内不再有本次资源。

## 已知边界

- 触发队列是进程内队列（容量 1024、4 个 worker），不保证 exactly-once；失败重试最多 3 次。
  本用例的函数总是返回成功，所以正常只回调一次；生产函数要按事件标识做幂等。
- SQL 直写不产生记录事件；只有文档 API 的写操作会触发。
- 服务端重启会丢未处理事件；`/function-triggers` 的 `dropped` 计数也会归零。
- 被触发的函数不能直接读写项目数据库/KV/对象存储，需要回调 SimpleBase 自己的 HTTP API。
