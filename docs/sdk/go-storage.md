---
title: Go · 对象存储
order: 14
group: Go
---

# 项目对象存储

对象存储按**项目**隔离，不依赖数据库 ID。调用方负责关闭打开的文件；上传使用流式 multipart 请求，不会先将整个文件读入内存。

```go
file, err := os.Open("report.txt")
if err != nil { return err }
defer file.Close()

meta, err := client.UploadObject(ctx, "reports/report.txt", file, "report.txt", "text/plain")
if err != nil { return err }
fmt.Println(meta.Key, meta.Size)

objects, err := client.ListObjects(ctx, "reports/", false)
if err != nil { return err }
fmt.Println(objects)

link, err := client.PresignObject(ctx, meta.Key)
if err != nil { return err }
fmt.Println(link.URL)
_, err = client.DeleteObject(ctx, meta.Key)
return err
```

`ListObjects` 最多返回 1000 项；`refresh=true` 可跳过元数据缓存。预签名 URL 用于下载，目前有效期 15 分钟，不应记录到公开日志。对象键为项目内的相对路径，不得包含 `.` / `..` 路径段、反斜杠或 NUL，长度上限 1024 字节。上传服务端请求体限制约 50 MiB（另有 multipart 开销）；上传和删除需要 `database:write`，列表与预签名需要 `database:read`。
