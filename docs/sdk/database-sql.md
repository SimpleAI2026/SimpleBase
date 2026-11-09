---
title: 数据库与 SQL
order: 5
group: JavaScript
---

# 数据库与 SQL

## 数据库

```ts
await sb.databases.list({ limit?: number, cursor?: string })
await sb.databases.get(databaseId)
await sb.databases.create({ name: string, data_model?: 'collection' | 'sql', init_sql?: string })
await sb.databases.remove(databaseId)
await sb.databases.schema(databaseId)          // 仅 SQL 库
await sb.databases.createTable(databaseId, { name, columns: [{ name, type, nullable? }] })
await sb.databases.addColumn(databaseId, { table, name, type, nullable? })
```

`data_model` 省略时创建集合文档库。`init_sql` 只在 `data_model: 'sql'` 时有效，语句限于 `CREATE` / `ALTER` / `INSERT`。创建需要 `database:admin`，成功后即可查询。表结构读取需要 `database:read`，建表和加列需要 `database:write`。

## SQL

```ts
const db = sb.database(databaseId)
// 或 createClient({ databaseId }) 后用 sb.sql

await db.sql.query(sql, args?, { maxRows? })
await db.sql.execute(sql, args?)
await db.sql.batch([{ sql, args? }], { transactional?: true })
```

- `query`：只读；`rows` 为与 `columns` 对齐的二维数组。
- `execute`：写语句。
- `batch`：默认 `transactional: true`。
- 请始终参数化（`?` 占位），避免拼接用户输入。
