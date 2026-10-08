---
title: SQL 与集合
order: 2
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

响应包含 `columns`、`rows`、`row_count`、`duration_ms`。参数请用 `?` 与 `args` 传入，不拼接用户输入。写语句用相同请求体发送至 `/execute`；批量执行请求为 `{"statements":[{"sql":"…","args":[]}],"transactional":true}`。`sqlguard` 拦截跨权限和敏感语句；更多限制见 [DuckLake 使用须知](/docs/database/ducklake-notes)。

## 集合与文档

前缀 `/v1/projects/:projectID/databases/:databaseID/data/collections`：

| 操作 | 路径 | 权限 |
|---|---|---|
| 列集合 / 新建集合 | `GET /` / `POST /` | read / write |
| 列文档 | `GET /:collection` | read |
| 新建文档 | `POST /:collection/documents` | write |
| 更新 / 删除文档 | `PUT` / `DELETE /:collection/documents/:id` | write |

集合属于**用户数据库**；[项目级 Key-Value](/docs/database/kv) 不要求选择数据库，两者不是同一个接口。读写细节也可参见 [JS SDK 文档集合](/docs/sdk/documents) 和 [Go SDK 文档集合](/docs/sdk/go-documents)。
