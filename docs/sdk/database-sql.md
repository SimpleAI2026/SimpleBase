---
title: 数据库与 SQL
order: 5
---

# 数据库与 SQL

## 数据库

```ts
await sb.databases.list({ limit?: number, cursor?: string })
await sb.databases.get(databaseId)
await sb.databases.create({ name: string })  // 需 DatabaseAdmin；成功后即可查询
await sb.databases.remove(databaseId)
```

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
