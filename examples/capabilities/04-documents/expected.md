# 预期结果

| 步骤 | 通过条件 |
|---|---|
| 建库 | `data.data_model` 为 `collection`（或响应未另给模型时按集合库使用） |
| 建集合 `notes` | `ok=true` |
| 插入 | `data.id` 非空，`data.title` 为 `hello` |
| 列出 | `data.rows` 里有该 id，标题为 `hello` |
| 更新 | 再列出时标题为 `edited` |
| 删除 | 再列出时没有该 id |
| 删库 | `ok=true` |
