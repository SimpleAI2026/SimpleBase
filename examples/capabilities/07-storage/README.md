# 对象存储

上传、列举、预签名、删除一个小文本对象。本地开发时文件落在 `storage.files=local` 的数据目录，不要求 S3。

`key` 是项目内相对路径，不要带项目 ID 前缀，也不要以 `/` 开头。

## 运行

```bash
export CASE=07-storage
./examples/capabilities/07-storage/run.sh
```

## 命令

```bash
simplebase object upload --key "examples/$RUN_ID/note.txt" --file ./note.txt
simplebase object list --prefix "examples/$RUN_ID/"
simplebase object presign --key "examples/$RUN_ID/note.txt"
simplebase object delete --key "examples/$RUN_ID/note.txt"
simplebase object list --prefix "examples/$RUN_ID/"
```

人类 CLI 的 `presign` 返回 `url`。助手模式只返回 `key` 和 `expires_at`。`object delete` 在云助手里需要确认。
