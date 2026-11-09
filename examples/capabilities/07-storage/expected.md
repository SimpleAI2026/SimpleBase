# 预期结果

| 步骤 | 通过条件 |
|---|---|
| upload | `ok=true`，`data.key` 等于上传的 key |
| list | `data` 数组里有该 key |
| presign | `data.url` 非空（人类 CLI） |
| delete | `ok=true` |
| list again | 数组里不再有该 key |

文件内容是一行 `hello-storage`。
