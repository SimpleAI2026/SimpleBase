---
title: 鉴权与安全
order: 4
group: JavaScript
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

## Scoped API Key

创建 API Key 时可附带 `scope`：`resources`、`actions`、`hidden_fields`、`readonly_fields`、`expires_at`。资源支持 `database:*`、`collection:orders`、`object:public/`、`function:report`、`cron:*`；动作支持 `read/write/invoke/presign`。无 scope 的旧 Key 行为不变。越权返回 403 `key_scope_denied`，不返回允许范围。字段隐藏作用于文档响应，readonly 字段出现在写请求中会被拒绝。

Scoped Key 是资源与字段级收缩，**不是行级 RLS**。签发者已有 scope 时，新 Key 只能继承或缩小范围；策略写入失败不会留下无限制 Key。

## 浏览器警告（必读）

**浏览器暴露 Key = 拥有该 Key 的全部权限。**

SimpleBase **当前没有行级 RLS**（与 Supabase「anon key + RLS」不同）。公开站点应：

- 使用**仅 Read** 的受限 Key，或
- 只走后端 BFF，前端不持写权限 Key。

写操作、上传、删库的 Key **禁止**进前端生产包。
