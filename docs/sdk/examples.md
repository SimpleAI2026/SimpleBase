---
title: 示例
order: 9
group: JavaScript
---

# 示例

完整初始化说明见 [`examples/README.md`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/README.md)。

| 目录 | 内容 |
|---|---|
| [`examples/shop`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/shop/README.md) | JS SDK、电商事务和缓存 |
| [`examples/community`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/community/README.md) | Go SDK、文档和 SQL、对象存储 |
| [`examples/iot-telemetry`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/iot-telemetry/README.md) | HTTP 脚本、interval/once 任务 |
| [`examples/ops-assistant`](https://github.com/linkxzhou/SimpleBase/blob/main/examples/ops-assistant/README.md) | Go SDK、最小权限、可观测 |

```bash
./build.sh dev
./examples/init.sh
```

`.env` 仅供可信服务与脚本读取，不应放进浏览器代码。

## 场景：一个最小的「从建库到上传」脚本

下面这段脚本把几个模块串成一次完整流程：建库 → 建集合写入文档 → 用 SQL 查询 → 上传对象并取下载链接。每一步都做错误处理，可当作新项目的起点。

```ts
import { createClient, SimpleBaseError } from '@simplebase/sdk'

const sb = createClient({
  url: process.env.SIMPLEBASE_URL!,
  apiKey: process.env.SIMPLEBASE_API_KEY!,
  projectId: process.env.SIMPLEBASE_PROJECT_ID!,
})

async function main() {
  // 1. 建一个集合文档库
  const db = await sb.databases.create({ name: `demo-${Date.now()}` })

  // 2. 建集合并写入一条文档
  const database = sb.database(db.id)
  await database.collections.create('notes')
  const notes = database.collection('notes')
  const created = await notes.insert({ title: 'first', body: 'hello' })
  console.log('inserted id =', created.id)

  // 3. 用 SQL 读回来
  const q = await database.sql.query('SELECT count(*) AS n FROM notes')
  console.log('notes count =', q.rows[0][0])

  // 4. 上传一个 Markdown 文件并取临时链接
  await sb.storage.upload('notes/first.md', '# first\n\nhello', {
    contentType: 'text/markdown',
    filename: 'first.md',
  })
  const { url } = await sb.storage.presign('notes/first.md')
  console.log('presigned url =', url)
}

main().catch((e) => {
  if (e instanceof SimpleBaseError) {
    console.error(`failed: status=${e.status} code=${e.code} request=${e.requestId}`)
  } else {
    console.error(e)
  }
  process.exit(1)
})
```

要点：

- 建库需要 `database:admin` 权限的 Key；上传对象需要 `database:write`。
- 集合名必须是 `[A-Za-z_][A-Za-z0-9_]*`，不支持下划线以外的符号。
- 预签名链接约 15 分钟过期，不要缓存为长期地址。
- 全部错误都会被 SDK 归一化成 `SimpleBaseError`，其中 `status` / `code` / `requestId` 可直接用于排查。

Go 版同样流程见 [Go · 快速开始](/docs/sdk/go-quickstart) 与 [Go · 对象存储](/docs/sdk/go-storage)：`CreateDatabase` → `Database(id).Execute` → `UploadObject` → `PresignObject`。
