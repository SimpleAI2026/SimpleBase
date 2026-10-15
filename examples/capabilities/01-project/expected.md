# 预期结果

| 步骤 | 通过条件 |
|---|---|
| `project list` | `ok=true`，`data.projects` 是数组 |
| `project create --name cap-$RUN_ID` | `ok=true`，`data.id` 为 8 位，`data.name` 等于所给名称 |
| `settings set` | `ok=true` |
| `settings get` | `data.settings` 中有 `Key=examples.note` |
| `quota get` | `ok=true` |
| `audit list --limit 5` | `ok=true` |
| `log search --limit 5` | `ok=true` |
| `log retention get` | `ok=true`，`data.keep_days` 为数字 |

设置接口直接编码 Go 结构体，字段名是 `Key` / `ValueJSON`，不是 snake_case。
