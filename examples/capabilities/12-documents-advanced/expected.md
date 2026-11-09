# 预期结果

| 步骤 | 通过条件 |
|---|---|
| 准备数据库 | 复用已有用户库，或用 `cap12-<suffix>` 新建集合库；`data.id` 非空 |
| 建集合 | `ok=true` |
| insert | `data.id` 非空，`title=hello`、`body=world` |
| patch | `title` 保持 `hello`、`body=patched`、`n=1`（merge） |
| batch | `data.items` 2 篇，`data.title` 集合为 `{b1,b2}` |
| list | `data.rows` 共 3 行 |
| schema-put | 3 个字段，`name` 为集合名 |
| schema-get | `title` 字段 `required=true` |
| filter-fields | `n>=2` 命中 2 行、`total=2`，行内只有 `title`（无 `body`） |
| export | `data.bytes>0` 且 `data.file` 非空；文件恰好 3 行 |
| import | `scanned=3`、`imported=3`、`dry_run=false` |
| list-imported | 3 行，标题集合为 `{b1,b2,hello}` |
| strict-schema-put | `title` 为 required，`qty` 为 number |
| strict-insert-invalid | 插入缺 `title` 的文档失败，`error.code=required_field` |
| strict-insert-valid | 补齐 `title` 后插入成功 |
| truncate / list-truncated | `ok=true`，之后列表 0 行 |
| delete-db | 本次新建的数据库删除成功（复用的库不动） |

> 失败详情里会带 CLI 的错误 envelope（已脱敏）。注意 CLI 的错误信封写在 stderr，成功信封写在 stdout。
