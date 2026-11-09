---
title: 文档集合
order: 6
group: JavaScript
---

# 文档集合（Document KV）

集合名：`[A-Za-z_][A-Za-z0-9_]*`。

```ts
const db = sb.database(databaseId)

await db.collections.list()
await db.collections.create('users')

const users = db.collection('users')
const page = await users.list({
  limit: 50,
  filter: 'status="open" && priority>=2',
  sort: '-created_at,title',
  fields: ['id', 'name', 'status'],
  count: true
})
await users.insert({ name: 'ada' })
await users.get(id)
await users.update(id, { name: 'Ada' })
await users.patch(id, { profile: { theme: 'dark' } })
await users.batch([{ name: 'grace' }, { name: 'linus' }])
await users.remove(id)

await users.putSchema({
  name: 'users',
  fields: [{ name: 'name', type: 'text', required: true, unique: true, options: { max: 200 } }]
})
await users.getSchema()
await users.dryRunView('SELECT id, data, created_at, updated_at FROM users')
await users.truncate()

await users.import({ format: 'ndjson', body: ndjsonStream, dryRun: true })
const response = await users.export({ format: 'csv', fields: ['id', 'name'] })
const csvStream = response.body
```

映射：`/v1/projects/:projectId/databases/:databaseId/data/collections...`。
