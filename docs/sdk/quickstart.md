---
title: 快速开始
order: 3
---

# 快速开始

DevMode 种子（本地 `./build.sh dev`）：

| 项 | 值 |
|---|---|
| URL | `http://127.0.0.1:8080` |
| API Key | `sb_live_dev_key_12345` |
| Project ID | `00000000-0000-0000-0000-000000000002` |

```ts
import { createClient } from '@simplebase/sdk'

const sb = createClient({
  url: 'http://127.0.0.1:8080',
  apiKey: 'sb_live_dev_key_12345',
  projectId: '00000000-0000-0000-0000-000000000002',
  // databaseId: '<uuid>'   // 可选默认库
})

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
