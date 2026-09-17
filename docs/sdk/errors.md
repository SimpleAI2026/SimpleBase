---
title: 错误处理
order: 8
---

# 错误处理

```ts
import { createClient, SimpleBaseError } from '@simplebase/sdk'

try {
  await sb.databases.list()
} catch (e) {
  if (e instanceof SimpleBaseError) {
    console.error(e.status, e.code, e.message, e.requestId)
  }
}
```

| 字段 | 含义 |
|---|---|
| `status` | HTTP 状态；网络失败为 `0` |
| `code` | 后端 `code`（如 `invalid_api_key`） |
| `requestId` | `x-request-id` 或 body `request_id` |
| `details` | 原始响应体 |
