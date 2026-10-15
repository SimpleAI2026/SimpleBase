# 预期结果

| 步骤 | 通过条件 |
|---|---|
| 建库 | `data.id` 非空，`data.data_model` 为 `sql`，`data.name` 为 `sql-$RUN_ID` |
| 建表 | `ok=true` |
| 加列 `qty` | `ok=true`，随后 `schema show` 的 `items` 含 `id`、`title`、`qty` |
| INSERT | `ok=true` |
| SELECT | 一行，`id=1`，`title=demo`，`qty=3` |
| UPDATE | 再查询时 `title=edited` |
| DELETE | 再查询时 `row_count=0` |
| `schema rows` | 预期列出 `items` 的行。当前 CLI 解码失败：接口把 `columns` 返回成列对象数组，SDK 的 `TableRows.Columns` 声明为 `[]string` |
| 删库 | `ok=true` |

查询列顺序与 `SELECT id, title, qty` 一致。数值在 JSON 里是数字。
