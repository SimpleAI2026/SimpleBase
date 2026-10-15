# 预期结果

| 步骤 | 通过条件 |
|---|---|
| create | `ok=true`，`data.active_version` 为 1 |
| get | `data.name` 等于函数名，`data.published` 为 true |
| invoke v1 | `data.message` 为 `hello simplebase` |
| versions create | `ok=true`，新版本号为 2 |
| test v2 | `data.message` 为 `hello v2 simplebase`（未激活也可试跑） |
| activate 2 | `ok=true` |
| invoke 生效版本 | `data.message` 为 `hello v2 simplebase` |

`invoke` 的 HTTP 响应体就是返回值 JSON，CLI 放在 `data` 里。
