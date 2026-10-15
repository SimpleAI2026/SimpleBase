# 预期结果

| 步骤 | 通过条件 |
|---|---|
| `project list` | `ok=true`，`data.projects` 是数组 |
| `project create --name cap-$RUN_ID` | `ok=true`，`data.id` 为 8 位，`data.name` 等于所给名称 |
| `settings set` | `ok=true` |
| `settings get` | `data.settings` 中有 `key=examples.note` |
| `quota get` | `ok=true` |
| `audit list --limit 5` | `ok=true` |
| `log search --limit 5` | `ok=true` |
| `log retention get` | `ok=true`，`data.keep_days` 为数字 |

设置项字段为 snake_case：`key` / `value_json` / `updated_at`，与写入体一致。
