---
title: 快速开始
order: 3
group: JavaScript
---

# 快速开始

本地 `./build.sh dev` 启动后，在控制台创建项目并在「设置 → 连接」重置签发 API Key（示例值请替换为你自己的）：

| 项 | 值 |
|---|---|
| URL | `http://127.0.0.1:8080` |
| API Key | `<控制台签发的 sb_live_... Key>` |
| Project ID | `<控制台创建的项目 ID>` |

```ts
import { createClient } from '@simplebase/sdk'

const sb = createClient({
  url: 'http://127.0.0.1:8080',
  apiKey: process.env.SIMPLEBASE_API_KEY,
  projectId: process.env.SIMPLEBASE_PROJECT_ID,
  // databaseId: '<uuid>'   // 可选默认库
})

// 项目 KV 开箱可用；用户数据库请先在控制台创建。
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
