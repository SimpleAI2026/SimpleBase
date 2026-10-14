---
title: SQL 与集合
order: 1
group: 数据库管理
---

# SQL 与集合

创建数据库时选择数据形态。用户数据库必须属于 URL 中的项目。两种形态都在创建成功后即可使用，不需要打开或关闭。存量库没有单独迁移数据文件，一律视为集合文档。

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

创建数据库后，在控制台「数据库管理」可以进入 SQL 工作台。集合文档库还可以展开「查看数据」管理集合与文档；SQL 库展开的是表结构。

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
| 列文档 | `GET /:collection` | read |
| 新建文档 | `POST /:collection/documents` | write |
| 更新 / 删除文档 | `PUT` / `DELETE /:collection/documents/:id` | write |

集合属于**用户数据库**；[项目级 Key-Value](/docs/data/kv) 不要求选择数据库，两者不是同一个接口。读写细节也可参见 [JS SDK 文档集合](/docs/sdk/documents) 和 [Go SDK 文档集合](/docs/sdk/go-documents)。

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

DuckLake 没有可靠的唯一约束。需要「存在则更新、不存在则插入」时，用 `MERGE INTO` 按业务键合并：

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
