---
title: 对象存储概览
order: 1
---

# 对象存储概览

每个项目都有独立的对象命名空间：SimpleBase 将项目 ID 拼入物理 S3 Key，API 返回的是去掉项目前缀的对象名。控制台「对象存储」可浏览、上传、删除和获取下载链接。

## 何时使用

使用对象存储保留文件、图片或导出的数据；结构化查询用[数据库](/docs/database)，短期状态用[Key-Value](/docs/database/kv)。对象不会作为 SQL 表自动出现。

## 最小示例

```bash
curl -sS -X POST "$BASE/v1/projects/$PROJECT/s3/objects" \
  -H "Authorization: Bearer $SB_KEY" \
  -F 'key=reports/hello.txt' -F 'file=@hello.txt'
curl -sS "$BASE/v1/projects/$PROJECT/s3/objects?prefix=reports/" \
  -H "Authorization: Bearer $SB_KEY"
```

上传需要 `database:write`；列举与预签名下载需要 `database:read`。设置 `BASE` / `PROJECT` / `SB_KEY` 参见[上手指南](/docs/getting-started/quickstart)。本示例要求先创建本地 `hello.txt` 且已配置可用的 S3（本地开发也可使用兼容服务）。

## 相关

完整请求参数见[对象存储 HTTP API](/docs/storage/api)，SDK 调用见 [JavaScript](/docs/sdk/storage) 与 [Go](/docs/sdk/go-storage)。生产环境请使用私有 bucket、TLS 和服务端加密；实际部署约束见[部署指南](/docs/ops/deployment)。
