---
title: 对象存储 HTTP API
order: 2
---

# 对象存储 HTTP API

请求前缀：`/v1/projects/:projectID/s3`；Bearer API Key 必须可访问该项目。以下接口须配置对象存储后才可用。

| 方法 | 路径 | 权限 | 请求或响应 |
|---|---|---|---|
| GET | `/objects?prefix=reports/` | `database:read` | 对象数组 `{key,size,lastModified?}`，`refresh=1` 可与远端同步索引 |
| POST | `/objects` | `database:write` | `multipart/form-data`：`key` 文本字段 + `file` 文件字段；返回对象元数据 |
| DELETE | `/objects?key=reports/a.txt` | `database:write` | `{ "ok": true }` |
| GET | `/presign?key=reports/a.txt` | `database:read` | `{ "url": "…" }`，下载链接约 15 分钟有效 |

## 上传文件

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/s3/objects" \
  -H "Authorization: Bearer $SB_KEY" \
  -F 'key=reports/a.txt' -F 'file=@a.txt'
```

文件名应符合服务端 Key 校验规则；不能直接在物理 S3 Key 中手工添加项目 ID。上传正文使用 multipart，不能把文件内容直接作为 JSON 发给 `/objects`。

## 获取下载地址

```bash
curl -sS "$BASE/v1/projects/$PROJECT/s3/presign?key=reports%2Fa.txt" \
  -H "Authorization: Bearer $SB_KEY"
```

预签名 URL 应在短时间内使用，勿将其写入公开日志。上传大小由实例路由 body 限制和后端存储服务共同约束，超过配置限制会失败；实际数值以部署配置为准。读 API Key 不能上传或删除。
