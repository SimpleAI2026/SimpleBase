---
title: SQL 与集合
order: 1
group: 数据管理
---

# SQL 与集合

每个项目只有一个数据库，对应一个 DuckDB 或 SQLite 文件；创建数据库时选择数据形态，已有数据库时再次创建返回 409。用户数据库必须属于 URL 中的项目。创建成功后即可使用，不需要打开或关闭。

| 形态 | `data_model` | 控制台 |
|---|---|---|
| 集合文档（默认） | `collection` | SQL 工作台，以及集合与文档 |
| SQL 数据 | `sql` | SQL 工作台，以及表结构（建表、加列）。不提供集合与文档 |

创建 SQL 库时可以带 `init_sql`。服务端按分号拆句，每条语句必须通过写权限校验，且首关键字只能是 `CREATE`、`ALTER` 或 `INSERT`。校验失败不会建库；执行失败会删掉刚建的库，同一个名称可以重试。

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"name":"shop","data_model":"sql","init_sql":"CREATE TABLE users (id INTEGER, email VARCHAR)"}'
```

表结构只对用户 SQL 库开放：

| 操作 | HTTP 路径（前缀 `/v1/projects/:projectID/databases/:databaseID`） | 权限 |
|---|---|---|
| 列出表和列 | `GET /schema` | `database:read` |
| 新建表 | `POST /schema/tables`，body `{name, columns:[{name,type,nullable?}]}` | `database:write` |
| 添加列 | `POST /schema/columns`，body `{table, name, type, nullable?}` | `database:write` |

列类型使用服务端白名单（如 `INTEGER`、`VARCHAR`、`BOOLEAN`、`TIMESTAMP`、`JSON`）。`nullable` 省略表示可空。集合库调用这些接口会得到 `data_model_mismatch`；系统库写入仍被拒绝。

## SQL 工作台

创建数据库后，在控制台「数据管理」可以进入 SQL 工作台。集合文档库还可以展开「查看数据」管理集合与文档；SQL 库展开的是表结构。

| 操作 | HTTP 路径（前缀 `/v1/projects/:projectID/databases/:databaseID`） | 权限 |
|---|---|---|
| 查询 | `POST /query` | `database:read` |
| 执行写语句 | `POST /execute` | `database:write` |
| 批量执行 | `POST /batch` | `database:write` |

只读查询示例（先将 `BASE` / `PROJECT` / `DATABASE` / `SB_KEY` 按[上手指南](/docs/getting-started/quickstart)设置）：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases/$DATABASE/query" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"sql":"SELECT ? AS answer","args":[42],"max_rows":10}'
```

响应包含 `columns`、`rows`、`row_count`、`duration_ms`。参数请用 `?` 与 `args` 传入，不拼接用户输入。写语句用相同请求体发送至 `/execute`；批量执行请求为 `{"statements":[{"sql":"…","args":[]}],"transactional":true}`。`sqlguard` 拦截跨权限和敏感语句；更多限制见 [DuckLake 使用须知](/docs/data/ducklake-notes)。

## 集合与文档

前缀 `/v1/projects/:projectID/databases/:databaseID/data/collections`：

| 操作 | 路径 | 权限 |
|---|---|---|
| 列集合 / 新建集合 | `GET /` / `POST /` | read / write |
| 读取 / 覆盖 / 注销 Schema | `GET` / `PUT` / `DELETE /:collection/meta` | read / write / write |
| 列文档 / 清空集合 | `GET` / `DELETE /:collection` | read / project:admin |
| 新建 / 批量新建文档 | `POST /:collection/documents`、`POST /:collection/documents/batch` | write |
| 单条读取 | `GET /:collection/documents/:id` | read |
| 整体替换 / 部分更新 / 删除 | `PUT` / `PATCH` / `DELETE /:collection/documents/:id` | write |

列表支持 `limit`（1–200，默认 50）、`cursor`、`offset`、`sort`、`filter`、`fields` 和 `count`。响应含结构化 `items`、可选 `next_cursor/total/request_id`；旧 `rows` 扁平字段暂时保留。cursor 与 offset 互斥，且仅用于默认 `-created_at,id DESC` 排序；自定义排序请使用 offset。

`filter` 支持 `&&`、`||`、括号、比较、`~`、`!~`、`in`、`is [not] null`。字段必须先在 Schema 注册，值始终参数化，不接受 SQL 片段或任意 JSON path。例如：

```bash
curl -sS --get "$BASE/v1/projects/$PROJECT/databases/$DATABASE/data/collections/orders" \
  -H "Authorization: Bearer $SB_KEY" \
  --data-urlencode 'filter=status="open" && priority>=2' \
  --data-urlencode 'sort=-created_at,title' \
  --data-urlencode 'fields=id,title,status' \
  --data-urlencode 'limit=50&count=1'
```

JSON 列过滤有全表扫描成本，超过 `limits.max_filter_scan_rows` 返回 `filter_scan_limit_exceeded`。`count=1` 默认关闭，仅在需要总数时开启。`fields` 支持 `payload.owner.name` 形式的嵌套裁剪，`id` 始终保留。

Schema 支持 `text/number/bool/date/email/url/select/json`，以及 required、unique、default、长度/范围、pattern、select 候选值、hidden 和 readonly。`PUT meta?dry_run=1` 只检查现有文档，不落库；删除字段或注销 Schema 不删除文档中的旧键。未注册 Schema 的旧集合继续保持宽松 JSON 模式。批量写入在单事务执行，错误中的 `failed_index` 是零基序号。

`PATCH` 使用 JSON merge patch：`null` 删除键，嵌套对象递归合并。`PUT` 是整体替换。

集合属于**用户数据库**；[项目级 Key-Value](/docs/data/kv) 不要求选择数据库，两者不是同一个接口。读写细节也可参见 [JS SDK 文档集合](/docs/sdk/documents) 和 [Go SDK 文档集合](/docs/sdk/go-documents)。

## 导入导出与逻辑视图

- `POST /:collection/import` 接受 `text/csv` 或 `application/x-ndjson`，默认整批一个事务；每 100 条校验并写入一批，但任一记录失败会回滚整个导入。`?dry_run=1` 只校验。请求体受 `limits.max_import_bytes`（默认 128MiB）限制，超限返回 413。CSV 按 Schema 转换 number/bool/json/select；`id/created_at/updated_at` 由服务端生成，readonly 字段不允许由客户端提供。
- `GET /:collection/export?format=csv|ndjson` 逐行流式输出，支持 `filter/sort/fields`，隐藏字段不会导出，响应含 `Content-Disposition`。跨域前端需要暴露该响应头（默认配置已包含）。原始导出不允许 Agent 委托调用，CLI 在 `SIMPLEBASE_AGENT_RUN=1` 下也拒绝未脱敏导出。
- `POST /data/collections/meta/dry-run-view` 以 `LIMIT 20` 试跑逻辑视图并推断字段。视图通过 `PUT /:collection/meta` 保存 `schema_mode:"view"` 和 `view_query`，需要 `project:admin`。查询必须是单条 `SELECT` 或 `WITH`，并通过只读 SQL 校验；返回 `id,data,created_at,updated_at` 四列，`data` 是 JSON 对象，时间使用 RFC3339。
- 逻辑视图是元数据子查询，不创建物理表或物理 VIEW。列表、单条 GET 和导出均可使用；所有写路径返回 `collection_read_only`。视图 cursor 依赖合法的 `created_at`，其它情况使用 offset 分页。

## 事件、Realtime 与迁移状态

文档 API 写成功后可以把事件推送给 SSE 客户端，或异步调用云函数；SQL 直写不产生这些事件。订阅、续传与正文安全边界见[文档 Realtime](/docs/data/realtime)，触发器配置、调用与队列限制见[事件触发器](/docs/automation/function-triggers)。

系统库采用最终态 `schema.go`，不提供 down 或自动迁移 API。admin 项目下 `GET /migrations/status` 与 `GET /migrations/plan` 只读报告最终态表是否齐全及待执行 DDL；空库检查不会建表。CLI 对应 `simplebase migrate status|plan`。`GET /v1/openapi.json` 从真实 Echo 路由生成 OpenAPI 3.0.3。

## 使用场景

以下示例沿用[5 分钟上手](/docs/getting-started/quickstart)中的 `BASE`、`PROJECT`、`DATABASE`、`SB_KEY`。

### 场景一：建表并批量写入

背景：导入一批商品。用 `/batch` 并设置 `transactional: true`，任一条失败整体回滚。

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases/$DATABASE/batch" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{
    "transactional": true,
    "statements": [
      {"sql": "CREATE TABLE IF NOT EXISTS products (id VARCHAR, name VARCHAR, price DOUBLE)"},
      {"sql": "INSERT INTO products VALUES (?, ?, ?)", "args": ["p-1", "键盘", 199]},
      {"sql": "INSERT INTO products VALUES (?, ?, ?)", "args": ["p-2", "鼠标", 89.5]}
    ]
  }'
```

成功响应含 `results`（每条的 `rows_affected`）与 `durability`。事务回滚时响应里的 `error.failed_index` 指出失败的语句序号，注意此时 HTTP 状态可能仍是 200，调用方要检查响应里的 `error` 字段。

### 场景二：分页与聚合查询

`/query` 只允许只读语句。用 `LIMIT`/`OFFSET` 分页，并用 `max_rows` 限制返回量；`max_rows` 为 0 表示使用服务端上限，超出上限会返回 400。

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases/$DATABASE/query" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"sql":"SELECT name, price FROM products ORDER BY price DESC LIMIT ? OFFSET ?","args":[20,0],"max_rows":20}'

curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases/$DATABASE/query" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"sql":"SELECT count(*) AS n, avg(price) AS avg_price FROM products"}'
```

### 场景三：幂等写入，避免重复订单

不要依赖未经验证的唯一约束。需要「存在则更新、不存在则插入」时，DuckDB 项目用 `MERGE INTO` 按业务键合并（SQLite 项目改用 `INSERT ... ON CONFLICT`）：

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/databases/$DATABASE/execute" \
  -H "Authorization: Bearer $SB_KEY" -H 'Content-Type: application/json' \
  -d '{"sql":"MERGE INTO products AS t USING (SELECT ? AS id, ? AS name, ? AS price) AS s ON t.id = s.id WHEN MATCHED THEN UPDATE SET name = s.name, price = s.price WHEN NOT MATCHED THEN INSERT VALUES (s.id, s.name, s.price)","args":["p-1","机械键盘",219]}'
```

对「同一请求被重复提交」这类场景，可以先用 KV 的 `SET NX` 占位再写库，见 [Key-Value 场景](/docs/data/kv)。

### 场景四：出报表用的只读 Key

给报表服务签发只带 `database:read` 的 Key（见 [SDK 鉴权与安全](/docs/sdk/auth)）。该 Key 可以调用 `/query` 和 `/schema`，调用 `/execute`、`/batch` 会被拒绝，写入类意外不会发生。

### 场景五：建 SQL 库并初始化结构

创建数据库时选 `data_model: "sql"` 并带 `init_sql`，建库同时建表。`init_sql` 只允许 `CREATE`、`ALTER`、`INSERT` 开头的语句，失败会回滚建库，名称可重试。

## 常见问题

| 现象 | 可能原因 | 处理 |
|---|---|---|
| `/query` 报只读限制 | 在 `/query` 里发了 `INSERT` / `CREATE` | 写语句改用 `/execute` 或 `/batch` |
| 提示不允许的语句 | 使用了 `ATTACH`、`COPY`、`SET`、`PRAGMA`、`CALL` 等管理语句，或多条语句 | 一次只发一条语句；管理类能力不对用户开放 |
| 写入返回 503 `writer_unavailable` | 当前实例不是可写实例 | 确认连接的是写实例，参见[部署与故障处理](/docs/ops/deployment) |
| 对系统库写入被拒绝 | 系统库只允许 `SELECT` | 业务数据请建在普通项目的用户库里 |
