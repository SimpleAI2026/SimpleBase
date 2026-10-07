---
title: 快速开始
order: 3
group: JavaScript
---

# 快速开始

DevMode 种子（本地 `./build.sh dev`，下述 Key 仅供本机联调，不可用于生产）：

| 项 | 值 |
|---|---|
| URL | `http://127.0.0.1:8080` |
| API Key | `sb_live_dev_key_12345` |
| Project ID | `dev-shop` |

```ts
import { createClient } from '@simplebase/sdk'

const sb = createClient({
  url: 'http://127.0.0.1:8080',
  apiKey: 'sb_live_dev_key_12345',
  projectId: 'dev-shop',
  // databaseId: '<uuid>'   // 可选默认库
})

// DevMode 仅预建项目 KV；请先在控制台新建一个用户数据库。
const { databases } = await sb.databases.list()
const db = sb.database(databases[0].id)

const q = await db.sql.query('SELECT 1 AS one')
console.log(q.columns, q.rows)
```

环境变量（examples 使用）：

- `SIMPLEBASE_URL`
- `SIMPLEBASE_API_KEY`
- `SIMPLEBASE_PROJECT_ID`
- `SIMPLEBASE_DATABASE_ID`
