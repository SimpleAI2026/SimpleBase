---
title: 对象存储
order: 7
---

# 对象存储（S3）

```ts
await sb.storage.list({ prefix?: string, refresh?: boolean })
await sb.storage.upload(key, body, { contentType?, filename? })
await sb.storage.presign(key)   // → { url }
await sb.storage.remove(key)
```

- `body`：`Blob` / `File` / `ArrayBuffer` / `Uint8Array` / `string`
- 上传走 `multipart/form-data`（字段 `key` + `file`）
- 列表返回对象数组（`{ key, size, lastModified? }`）
