---
title: SQL 与集合
order: 2
---

# SQL 与集合

创建数据库后，在控制台「数据库管理」可以进入 SQL 工作台，也可以展开「查看数据」管理集合与文档。用户数据库必须属于 URL 中的项目。

## SQL 工作台

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
