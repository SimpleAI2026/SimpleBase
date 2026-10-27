---
title: SDK
order: 1
---

# SDK

SimpleBase 提供 JavaScript SDK（`@simplebase/sdk`）和 Go SDK（`packages/go-sdk`），统一使用 Bearer API Key 调用 `/v1`。服务端可选择对应语言的客户端；浏览器中仅使用受限读 Key，写权限 Key 不得嵌入公开前端。

## 文档目录

1. [安装](/docs/sdk/install)
2. [快速开始](/docs/sdk/quickstart)
3. [鉴权与安全](/docs/sdk/auth)
4. [数据库与 SQL](/docs/sdk/database-sql)
5. [文档集合](/docs/sdk/documents)
6. [对象存储](/docs/sdk/storage)
7. [错误处理](/docs/sdk/errors)
8. [示例](/docs/sdk/examples)

Go SDK 从 [安装](/docs/sdk/go-install) → [快速开始](/docs/sdk/go-quickstart) → [数据库与 SQL](/docs/sdk/go-database-sql) 阅读。JavaScript 包路径：`packages/js-sdk`。可运行示例见 [examples/README.md](https://github.com/linkxzhou/SimpleBase/blob/main/examples/README.md)；仓库中按业务场景组织 `shop`、`community`、`iot-telemetry` 与 `ops-assistant`。

## 该选哪个 SDK

| 场景 | 建议 |
|---|---|
| 浏览器内做只读展示 | JavaScript SDK，配合**只读** API Key；写权限 Key 不进前端 |
| Node 服务端、脚本、CI | JavaScript SDK（Node ≥ 18）或 Go SDK |
| 已有 Go 服务、需要原生类型与 `context` 超时 | Go SDK |

## 常用入口

```ts
import { createClient } from '@simplebase/sdk'

const sb = createClient({
  url: process.env.SIMPLEBASE_URL!,
  apiKey: process.env.SIMPLEBASE_API_KEY!,
  projectId: process.env.SIMPLEBASE_PROJECT_ID!,
  databaseId: process.env.SIMPLEBASE_DATABASE_ID, // 可选默认库
})

await sb.databases.list()                       // 数据库
await sb.database(id).sql.query('SELECT 1')      // SQL
await sb.storage.list({ prefix: 'reports/' })    // 对象存储
await sb.sandboxes.listPage({ limit: 20 })       // 云沙盒（需部署启用）
```

需要完整可跑的例子，直接看[示例](/docs/sdk/examples)里的「从建库到上传」脚本。文档变更订阅与写后自动化分别见[文档 Realtime](/docs/data/realtime)和[事件触发器](/docs/automation/function-triggers)；生产备份接口见[备份与恢复](/docs/ops/backups)。
