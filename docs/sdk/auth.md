---
title: 鉴权与安全
order: 4
---

# 鉴权与安全

所有请求带：

```http
Authorization: Bearer <API_KEY>
```

路径前缀：`/v1/projects/:projectId/...`。

## 权限边界

- Key 由控制台 / 系统库签发；权限位决定能否读/写库与 S3。
- SDK **不**在客户端推导权限：`401` / `403` 原样抛出 `SimpleBaseError`。

## 浏览器警告（必读）

**浏览器暴露 Key = 拥有该 Key 的全部权限。**

SimpleBase **当前没有行级 RLS**（与 Supabase「anon key + RLS」不同）。公开站点应：

- 使用**仅 Read** 的受限 Key，或
- 只走后端 BFF，前端不持写权限 Key。

写操作、上传、删库的 Key **禁止**进前端生产包。
