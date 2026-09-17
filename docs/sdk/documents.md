---
title: 文档集合
order: 6
---

# 文档集合（Document KV）

集合名：`[A-Za-z_][A-Za-z0-9_]*`。

```ts
const db = sb.database(databaseId)

await db.collections.list()
await db.collections.create('users')

const users = db.collection('users')
await users.list()
await users.insert({ name: 'ada' })
await users.update(id, { name: 'Ada' })
await users.remove(id)
```

映射：`/v1/projects/:projectId/databases/:databaseId/data/collections...`。
